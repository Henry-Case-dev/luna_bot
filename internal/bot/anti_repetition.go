package bot

import (
	"crypto/md5"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
)

// AntiRepetitionService предотвращает повторения в ответах бота
type AntiRepetitionService struct {
	bot *Bot

	// Хранилище недавних ответов бота по чатам
	recentResponses map[int64][]ResponseMemory

	// Мьютекс для потокобезопасности
	mutex sync.RWMutex

	// Настройки
	maxResponsesPerChat int           // Максимум ответов для хранения на чат
	similarityThreshold float64       // Порог схожести (0.0-1.0)
	timeWindow          time.Duration // Временное окно для проверки
	cleanupInterval     time.Duration // Интервал очистки старых записей

	// --- НОВЫЕ настройки переработки ---
	reworkEnabled        bool    // Включена ли переработка повторений
	maxReworkAttempts    int     // Максимальное количество попыток переработки
	reworkTemperature    float64 // Температура LLM для переработки
	localReworkEnabled   bool    // Включена ли локальная переработка
	localReworkMaxLength int     // Максимальная длина для локальной переработки

	// Статистика
	blockedRepetitions  int // Счетчик заблокированных повторений (при отключенной переработке)
	reworkedRepetitions int // Счетчик переработанных повторений
	successfulReworks   int // Счетчик успешных переработок
	failedReworks       int // Счетчик неудачных переработок
}

// ResponseMemory хранит информацию об ответе бота
type ResponseMemory struct {
	Text             string    // Текст ответа
	NormalizedText   string    // Нормализованный текст для сравнения
	Timestamp        time.Time // Время отправки
	ChatID           int64     // ID чата
	UserID           int64     // ID пользователя, которому отвечали (0 если общий ответ)
	ReplyToMessageID int       // НОВОЕ: ID сообщения на которое отвечали (0 если общий ответ)
	Topic            string    // Тема/контекст ответа
	Hash             string    // MD5 хэш для быстрого сравнения
	ResponseType     string    // Тип ответа (direct, general, free_will, etc.)
}

// NewAntiRepetitionService создает новый сервис анти-повторений
func NewAntiRepetitionService(bot *Bot) *AntiRepetitionService {
	// Защита от некорректных значений конфигурации
	cleanupIntervalHours := bot.config.AntiRepetitionCleanupIntervalHours
	if cleanupIntervalHours <= 0 {
		log.Printf("[AntiRepetition][WARNING] Некорректный интервал очистки %d часов, используется значение по умолчанию: 1 час", cleanupIntervalHours)
		cleanupIntervalHours = 1
	}

	timeWindowHours := bot.config.AntiRepetitionTimeWindowHours
	if timeWindowHours <= 0 {
		log.Printf("[AntiRepetition][WARNING] Некорректное временное окно %d часов, используется значение по умолчанию: 24 часа", timeWindowHours)
		timeWindowHours = 24
	}

	service := &AntiRepetitionService{
		bot:                 bot,
		recentResponses:     make(map[int64][]ResponseMemory),
		maxResponsesPerChat: bot.config.AntiRepetitionMaxResponsesPerChat,
		similarityThreshold: bot.config.AntiRepetitionSimilarityThreshold,
		timeWindow:          time.Duration(timeWindowHours) * time.Hour,
		cleanupInterval:     time.Duration(cleanupIntervalHours) * time.Hour,

		// --- Инициализация новых полей переработки ---
		reworkEnabled:        bot.config.AntiRepetitionReworkEnabled,
		maxReworkAttempts:    bot.config.AntiRepetitionMaxReworkAttempts,
		reworkTemperature:    bot.config.AntiRepetitionReworkTemperature,
		localReworkEnabled:   bot.config.AntiRepetitionLocalReworkEnabled,
		localReworkMaxLength: bot.config.AntiRepetitionLocalReworkMaxLength,
	}

	// Запускаем горутину для периодической очистки
	go service.startCleanupRoutine()

	log.Printf("[AntiRepetition] Сервис инициализирован: порог схожести %.2f, окно %v, макс ответов %d, переработка %v",
		service.similarityThreshold, service.timeWindow, service.maxResponsesPerChat, service.reworkEnabled)

	return service
}

// CheckSimilarity проверяет, похож ли новый ответ на недавние
func (ar *AntiRepetitionService) CheckSimilarity(chatID int64, newText string, userID int64, responseType string, replyToMessageID int) (bool, string) {
	if strings.TrimSpace(newText) == "" {
		return false, ""
	}

	ar.mutex.RLock()
	defer ar.mutex.RUnlock()

	responses, exists := ar.recentResponses[chatID]
	if !exists || len(responses) == 0 {
		return false, ""
	}

	normalizedNew := ar.normalizeText(newText)
	newHash := ar.calculateHash(normalizedNew)

	// Извлекаем тему из нового текста
	newTopic := ar.extractTopic(newText)

	now := time.Now()

	for _, resp := range responses {
		// Пропускаем старые ответы (вне временного окна)
		if now.Sub(resp.Timestamp) > ar.timeWindow {
			continue
		}

		// НОВАЯ проверка: ответы на одно сообщение
		if replyToMessageID > 0 && resp.ReplyToMessageID == replyToMessageID {
			timeSinceReply := now.Sub(resp.Timestamp)
			// Блокируем повторные ответы на одно сообщение в течение 30 минут
			if timeSinceReply < 30*time.Minute {
				similarity := ar.calculateSimilarity(normalizedNew, resp.NormalizedText)
				if similarity >= 0.3 { // Более низкий порог для ответов на одно сообщение
					if ar.bot.config.Debug {
						log.Printf("[AntiRepetition][DEBUG] Повторный ответ на сообщение %d в чате %d (схожесть %.1f%%, %v назад)",
							replyToMessageID, chatID, similarity*100, ar.formatDuration(timeSinceReply))
					}
					ar.blockedRepetitions++
					return true, fmt.Sprintf("Повторный ответ на сообщение %d (схожесть %.1f%%, %v назад)",
						replyToMessageID, similarity*100, ar.formatDuration(timeSinceReply))
				}
			}
		}

		// Проверяем точное совпадение хэшей
		if resp.Hash == newHash {
			if ar.bot.config.Debug {
				log.Printf("[AntiRepetition][DEBUG] Точное совпадение хэша в чате %d: %.30s...",
					chatID, newText)
			}
			ar.blockedRepetitions++
			return true, "Точное совпадение текста"
		}

		// Проверяем схожесть текста
		similarity := ar.calculateSimilarity(normalizedNew, resp.NormalizedText)
		if similarity >= ar.similarityThreshold {
			if ar.bot.config.Debug {
				log.Printf("[AntiRepetition][DEBUG] Высокая схожесть %.2f в чате %d: '%s' vs '%s'",
					similarity, chatID, ar.truncate(newText, 30), ar.truncate(resp.Text, 30))
			}
			ar.blockedRepetitions++
			return true, fmt.Sprintf("Схожесть %.1f%% с ответом %v назад",
				similarity*100, ar.formatDuration(now.Sub(resp.Timestamp)))
		}

		// Проверяем повторение темы для одного пользователя
		if userID > 0 && resp.UserID == userID && newTopic != "" && resp.Topic == newTopic {
			if now.Sub(resp.Timestamp) < 10*time.Minute { // В течение 10 минут
				if ar.bot.config.Debug {
					log.Printf("[AntiRepetition][DEBUG] Повторение темы '%s' для пользователя %d в чате %d",
						newTopic, userID, chatID)
				}
				ar.blockedRepetitions++
				return true, fmt.Sprintf("Повторение темы '%s' для пользователя", newTopic)
			}
		}
	}

	return false, ""
}

// RecordResponse записывает новый ответ бота в память
func (ar *AntiRepetitionService) RecordResponse(chatID int64, text string, userID int64, responseType string, replyToMessageID int) {
	if strings.TrimSpace(text) == "" {
		return
	}

	ar.mutex.Lock()
	defer ar.mutex.Unlock()

	normalizedText := ar.normalizeText(text)
	hash := ar.calculateHash(normalizedText)
	topic := ar.extractTopic(text)

	response := ResponseMemory{
		Text:             text,
		NormalizedText:   normalizedText,
		Timestamp:        time.Now(),
		ChatID:           chatID,
		UserID:           userID,
		ReplyToMessageID: replyToMessageID,
		Topic:            topic,
		Hash:             hash,
		ResponseType:     responseType,
	}

	// Добавляем в начало списка
	if ar.recentResponses[chatID] == nil {
		ar.recentResponses[chatID] = make([]ResponseMemory, 0, ar.maxResponsesPerChat)
	}

	ar.recentResponses[chatID] = append([]ResponseMemory{response}, ar.recentResponses[chatID]...)

	// Ограничиваем количество записей
	if len(ar.recentResponses[chatID]) > ar.maxResponsesPerChat {
		ar.recentResponses[chatID] = ar.recentResponses[chatID][:ar.maxResponsesPerChat]
	}

	if ar.bot.config.Debug {
		log.Printf("[AntiRepetition][DEBUG] Записан ответ в чате %d: %.30s... (тип: %s, topик: %s, реплай на: %d)",
			chatID, text, responseType, topic, replyToMessageID)
	}
}

// normalizeText нормализует текст для сравнения
func (ar *AntiRepetitionService) normalizeText(text string) string {
	// Приводим к нижнему регистру
	normalized := strings.ToLower(text)

	// Удаляем знаки препинания
	punctuation := regexp.MustCompile(`[^\p{L}\p{N}\s]+`)
	normalized = punctuation.ReplaceAllString(normalized, " ")

	// Удаляем лишние пробелы
	spaces := regexp.MustCompile(`\s+`)
	normalized = spaces.ReplaceAllString(normalized, " ")

	return strings.TrimSpace(normalized)
}

// calculateHash вычисляет MD5 хэш для быстрого сравнения
func (ar *AntiRepetitionService) calculateHash(text string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(text)))
}

// calculateSimilarity вычисляет схожесть двух текстов (алгоритм Жаккара)
func (ar *AntiRepetitionService) calculateSimilarity(text1, text2 string) float64 {
	if text1 == text2 {
		return 1.0
	}

	words1 := strings.Fields(text1)
	words2 := strings.Fields(text2)

	if len(words1) == 0 && len(words2) == 0 {
		return 1.0
	}
	if len(words1) == 0 || len(words2) == 0 {
		return 0.0
	}

	// Создаем множества слов
	set1 := make(map[string]bool)
	set2 := make(map[string]bool)

	for _, word := range words1 {
		set1[word] = true
	}
	for _, word := range words2 {
		set2[word] = true
	}

	// Вычисляем пересечение и объединение
	intersection := 0
	union := len(set1)

	for word := range set2 {
		if set1[word] {
			intersection++
		} else {
			union++
		}
	}

	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// extractTopic извлекает основную тему из текста
func (ar *AntiRepetitionService) extractTopic(text string) string {
	// Простая эвристика: берем первые значимые слова
	normalized := ar.normalizeText(text)
	words := strings.Fields(normalized)

	// Игнорируем стоп-слова
	stopWords := map[string]bool{
		"а": true, "и": true, "или": true, "но": true, "да": true, "нет": true,
		"что": true, "как": true, "где": true, "когда": true, "почему": true,
		"это": true, "то": true, "все": true, "еще": true, "уже": true,
		"не": true, "ни": true, "за": true, "на": true, "в": true, "с": true,
		"ну": true, "вот": true, "так": true, "тут": true, "там": true,
	}

	var topicWords []string
	for _, word := range words {
		if len(word) > 2 && !stopWords[word] {
			topicWords = append(topicWords, word)
			if len(topicWords) >= 3 { // Берем максимум 3 ключевых слова
				break
			}
		}
	}

	return strings.Join(topicWords, " ")
}

// startCleanupRoutine запускает периодическую очистку старых записей
func (ar *AntiRepetitionService) startCleanupRoutine() {
	// Дополнительная защита на случай некорректного значения интервала
	interval := ar.cleanupInterval
	if interval <= 0 {
		log.Printf("[AntiRepetition][ERROR] Критическая ошибка: некорректный интервал очистки %v, используется fallback: 1 час", interval)
		interval = time.Hour
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("[AntiRepetition][DEBUG] Запущена горутина очистки с интервалом %v", interval)

	for range ticker.C {
		ar.cleanup()
	}
}

// cleanup удаляет старые записи вне временного окна
func (ar *AntiRepetitionService) cleanup() {
	ar.mutex.Lock()
	defer ar.mutex.Unlock()

	now := time.Now()
	totalRemoved := 0

	for chatID, responses := range ar.recentResponses {
		validResponses := make([]ResponseMemory, 0, len(responses))

		for _, resp := range responses {
			if now.Sub(resp.Timestamp) <= ar.timeWindow {
				validResponses = append(validResponses, resp)
			} else {
				totalRemoved++
			}
		}

		if len(validResponses) == 0 {
			delete(ar.recentResponses, chatID)
		} else {
			ar.recentResponses[chatID] = validResponses
		}
	}

	if totalRemoved > 0 && ar.bot.config.Debug {
		log.Printf("[AntiRepetition][DEBUG] Очистка: удалено %d старых записей", totalRemoved)
	}
}

// GetStats возвращает статистику сервиса
func (ar *AntiRepetitionService) GetStats() map[string]interface{} {
	ar.mutex.RLock()
	defer ar.mutex.RUnlock()

	totalResponses := 0
	for _, responses := range ar.recentResponses {
		totalResponses += len(responses)
	}

	stats := map[string]interface{}{
		"total_chats":             len(ar.recentResponses),
		"total_responses":         totalResponses,
		"blocked_repetitions":     ar.blockedRepetitions,
		"reworked_repetitions":    ar.reworkedRepetitions,
		"successful_reworks":      ar.successfulReworks,
		"failed_reworks":          ar.failedReworks,
		"similarity_threshold":    ar.similarityThreshold,
		"time_window":             ar.timeWindow,
		"max_responses_per_chat":  ar.maxResponsesPerChat,
		"cleanup_interval":        ar.cleanupInterval,
		"rework_enabled":          ar.reworkEnabled,
		"max_rework_attempts":     ar.maxReworkAttempts,
		"rework_temperature":      ar.reworkTemperature,
		"local_rework_enabled":    ar.localReworkEnabled,
		"local_rework_max_length": ar.localReworkMaxLength,
	}

	// Добавляем процентную статистику
	totalRepetitions := ar.blockedRepetitions + ar.reworkedRepetitions
	if totalRepetitions > 0 {
		stats["success_rate"] = fmt.Sprintf("%.1f%%", float64(ar.successfulReworks)/float64(ar.reworkedRepetitions)*100)
		stats["rework_rate"] = fmt.Sprintf("%.1f%%", float64(ar.reworkedRepetitions)/float64(totalRepetitions)*100)
	} else {
		stats["success_rate"] = "0%"
		stats["rework_rate"] = "0%"
	}

	return stats
}

// Helper functions

func (ar *AntiRepetitionService) truncate(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

func (ar *AntiRepetitionService) formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0f сек", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0f мин", d.Minutes())
	}
	return fmt.Sprintf("%.1f ч", d.Hours())
}

// reworkWithLLM выполняет переработку текста с использованием LLM
func (ar *AntiRepetitionService) reworkWithLLM(chatID int64, repeatedText string, reason string) (string, error) {
	if ar.bot.llm == nil {
		return "", fmt.Errorf("LLM недоступен")
	}

	// Получаем последние ответы для формирования списка запрещенных фраз
	ar.mutex.RLock()
	recentResponses := ar.getRecentResponseTexts(chatID, 5)
	ar.mutex.RUnlock()

	// Формируем список запрещенных фраз
	forbiddenPhrases := make([]string, 0, len(recentResponses))
	for _, resp := range recentResponses {
		// Извлекаем ключевые фразы (2-3 слова)
		words := strings.Fields(resp)
		if len(words) >= 2 {
			forbiddenPhrases = append(forbiddenPhrases, strings.Join(words[:2], " "))
		}
		if len(words) >= 3 {
			forbiddenPhrases = append(forbiddenPhrases, strings.Join(words[:3], " "))
		}
	}

	// Подготавливаем промпт
	promptTemplate := ar.bot.enrichPromptWithPersonality(ar.bot.config.AntiRepetitionReworkPrompt, chatID, "anti_repetition")

	reworkPrompt := strings.ReplaceAll(promptTemplate, "{reason}", reason)
	reworkPrompt = strings.ReplaceAll(reworkPrompt, "{repeated_text}", repeatedText)
	reworkPrompt = strings.ReplaceAll(reworkPrompt, "{recent_responses}", strings.Join(recentResponses, "; "))
	reworkPrompt = strings.ReplaceAll(reworkPrompt, "{forbidden_phrases}", strings.Join(forbiddenPhrases, ", "))

	if ar.bot.config.Debug {
		log.Printf("[AntiRepetition][DEBUG] LLM переработка для чата %d. Промпт: %.200s...", chatID, reworkPrompt)
	}

	// Генерируем переработанный ответ с пониженной температурой
	reworkedResponse, err := ar.bot.llm.GenerateResponseByType(llm.ResponseTypeAntiRepetition, reworkPrompt, "", float32(ar.reworkTemperature))
	if err != nil {
		return "", fmt.Errorf("ошибка LLM переработки: %w", err)
	}

	// Очищаем ответ от markdown и лишних символов
	cleanedResponse := cleanupLLMResponse(reworkedResponse)

	if ar.bot.config.Debug {
		log.Printf("[AntiRepetition][DEBUG] LLM переработка завершена: '%s' → '%s'", repeatedText, cleanedResponse)
	}

	return cleanedResponse, nil
}

// getRecentResponseTexts возвращает тексты недавних ответов для чата (НЕ потокобезопасная)
func (ar *AntiRepetitionService) getRecentResponseTexts(chatID int64, limit int) []string {
	responses, exists := ar.recentResponses[chatID]
	if !exists || len(responses) == 0 {
		return []string{}
	}

	var texts []string
	count := 0
	now := time.Now()

	for _, resp := range responses {
		if count >= limit {
			break
		}
		// Учитываем только недавние ответы (в пределах временного окна)
		if now.Sub(resp.Timestamp) <= ar.timeWindow {
			texts = append(texts, resp.Text)
			count++
		}
	}

	return texts
}

// ProcessRepetition обрабатывает обнаруженное повторение (переработка или блокировка)
func (ar *AntiRepetitionService) ProcessRepetition(chatID int64, originalText string, userID int64, responseType string, reason string) (string, bool) {
	if !ar.reworkEnabled {
		// Если переработка отключена, блокируем как раньше
		ar.blockedRepetitions++
		if ar.bot.config.Debug {
			log.Printf("[AntiRepetition][BLOCKED] Чат %d: Заблокирован повторяющийся ответ (переработка отключена). Причина: %s", chatID, reason)
		}
		return "", false
	}

	ar.reworkedRepetitions++

	// Пытаемся переработать сообщение
	for attempt := 1; attempt <= ar.maxReworkAttempts; attempt++ {
		var reworkedText string
		var err error

		// Выбираем метод переработки
		if len(originalText) <= ar.localReworkMaxLength && ar.localReworkEnabled {
			// Локальная переработка для коротких текстов
			reworkedText = ar.localRework(originalText)
			if ar.bot.config.Debug {
				log.Printf("[AntiRepetition][REWORK] Чат %d: Попытка #%d (локальная) - '%s' → '%s'",
					chatID, attempt, originalText, reworkedText)
			}
		} else {
			// LLM переработка для длинных текстов
			reworkedText, err = ar.reworkWithLLM(chatID, originalText, reason)
			if err != nil {
				if ar.bot.config.Debug {
					log.Printf("[AntiRepetition][ERROR] Чат %d: Ошибка LLM переработки (попытка #%d): %v", chatID, attempt, err)
				}
				// Пробуем локальную переработку как fallback
				if ar.localReworkEnabled {
					reworkedText = ar.localRework(originalText)
					if ar.bot.config.Debug {
						log.Printf("[AntiRepetition][FALLBACK] Чат %d: Fallback на локальную переработку", chatID)
					}
				} else {
					continue // Переходим к следующей попытке
				}
			}
		}

		// Проверяем, не является ли переработанный текст тоже повторением
		isStillRepetitive, newReason := ar.CheckSimilarity(chatID, reworkedText, userID, responseType, 0)
		if !isStillRepetitive {
			// Успешная переработка!
			ar.successfulReworks++
			if ar.bot.config.Debug {
				log.Printf("[AntiRepetition][SUCCESS] Чат %d: Успешная переработка за %d попыток. Финальный текст: '%s'",
					chatID, attempt, reworkedText)
			}
			return reworkedText, true
		}

		if ar.bot.config.Debug {
			log.Printf("[AntiRepetition][RETRY] Чат %d: Попытка #%d неудачна, переработанный текст тоже повторение: %s",
				chatID, attempt, newReason)
		}

		// Для следующей попытки используем переработанный текст как исходный
		originalText = reworkedText
	}

	// Все попытки исчерпаны
	ar.failedReworks++
	if ar.bot.config.Debug {
		log.Printf("[AntiRepetition][FAILED] Чат %d: Исчерпаны попытки переработки (%d), отправляем последний вариант",
			chatID, ar.maxReworkAttempts)
	}

	// Возвращаем последний переработанный вариант (он лучше оригинала)
	return originalText, true
}

// localRework выполняет локальную переработку текста с использованием синонимов
func (ar *AntiRepetitionService) localRework(text string) string {
	if !ar.localReworkEnabled {
		return text
	}

	// Простая замена на синонимы для базовых фраз
	replacements := map[string]string{
		"понятно":    "ясно",
		"привет":     "здарова",
		"конечно":    "естественно",
		"хорошо":     "норм",
		"отлично":    "збс",
		"да":         "ага",
		"нет":        "неа",
		"спасибо":    "спс",
		"пожалуйста": "не за что",
		"интересно":  "любопытно",
		"круто":      "зачет",
		"ладно":      "окей",
		"понял":      "въехал",
		"серьезно":   "реально",
		"точно":      "именно",
		"правильно":  "верно",
		"странно":    "чудно",
		"смешно":     "прикольно",
		"ужасно":     "кошмар",
		"глупо":      "тупо",
		"классно":    "клево",
		"плохо":      "херово",
		"хорошая":    "неплохая",
		"плохая":     "дерьмовая",
		"думаю":      "считаю",
		"знаю":       "ведаю",
		"вижу":       "замечаю",
		"слышу":      "слушаю",
	}

	// Добавляем случайные вводные слова
	introWords := []string{
		"короче",
		"ну хз",
		"слушай",
		"кстати",
		"между прочим",
		"в общем",
		"типа",
		"блин",
	}

	result := text

	// Заменяем синонимы
	for original, replacement := range replacements {
		if strings.Contains(strings.ToLower(result), original) {
			// Заменяем с учетом регистра
			if strings.Contains(result, titleCase(original)) {
				result = strings.ReplaceAll(result, titleCase(original), titleCase(replacement))
			} else {
				result = strings.ReplaceAll(result, original, replacement)
			}
			break // Заменяем только одно слово за раз
		}
	}

	// Иногда добавляем вводное слово (30% вероятность)
	if len(introWords) > 0 && len(result) < 100 {
		// Простая псевдослучайность на основе длины текста
		if len(result)%3 == 0 {
			introWord := introWords[len(result)%len(introWords)]
			if !strings.HasPrefix(strings.ToLower(result), introWord) {
				result = introWord + ", " + result
			}
		}
	}

	if ar.bot.config.Debug {
		log.Printf("[AntiRepetition][DEBUG] Локальная переработка: '%s' → '%s'", text, result)
	}

	return result
}
