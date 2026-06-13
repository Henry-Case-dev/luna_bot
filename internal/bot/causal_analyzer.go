package bot

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// CausalAnalysisResult структура для результата анализа LLM
type CausalAnalysisResult struct {
	CausalLinks []CausalLink `json:"causal_links"`
}

// CausalLink представляет одну причинно-следственную связь
type CausalLink struct {
	Event        string   `json:"event"`         // Что произошло
	Cause        string   `json:"cause"`         // Причина
	Effect       string   `json:"effect"`        // Следствие/изменение во мне
	Category     string   `json:"category"`      // Категория связи
	Confidence   float64  `json:"confidence"`    // Уверенность в связи (0.0-1.0)
	TriggerType  string   `json:"trigger_type"`  // Тип триггера
	Keywords     []string `json:"keywords"`      // Ключевые слова
	UserContext  string   `json:"user_context"`  // Контекст пользователя
	TopicContext string   `json:"topic_context"` // Контекст темы
	Importance   float64  `json:"importance"`    // Важность (0.0-1.0)
}

// BehavioralInfluenceResult структура для результата анализа влияния
type BehavioralInfluenceResult struct {
	BehavioralAdjustments []BehavioralAdjustment `json:"behavioral_adjustments"`
	TriggeredMemories     []string               `json:"triggered_memories"`
	OverallStrategy       string                 `json:"overall_strategy"`
}

// BehavioralAdjustment представляет корректировку поведения
type BehavioralAdjustment struct {
	Aspect     string  `json:"aspect"`     // Аспект поведения
	Adjustment string  `json:"adjustment"` // Конкретное изменение
	Reason     string  `json:"reason"`     // Причина из каузальной памяти
	Confidence float64 `json:"confidence"` // Уверенность в корректировке
}

// startCausalAnalyzer запускает фоновую задачу для анализа каузальных связей
func (b *Bot) startCausalAnalyzer() {
	if !b.config.CausalLearningEnabled {
		log.Printf("[CausalAnalyzer] Каузальное обучение отключено в конфигурации")
		return
	}

	log.Printf("[CausalAnalyzer] Запуск анализатора каузальных связей...")

	// Запускаем первый анализ через 2 минуты после старта
	initialDelay := 2 * time.Minute
	go func() {
		time.Sleep(initialDelay)
		select {
		case <-b.stop:
			return
		default:
			log.Println("[CausalAnalyzer] Запуск первичного анализа каузальных связей...")
			b.analyzeCausalLinksForAllChats()
		}
	}()

	// Запускаем периодический анализ
	analysisInterval := time.Duration(b.config.CausalAnalysisIntervalHours) * time.Hour
	if analysisInterval <= 0 {
		analysisInterval = 4 * time.Hour
	}

	ticker := time.NewTicker(analysisInterval)
	go func() {
		defer ticker.Stop()
		log.Printf("[CausalAnalyzer] Запущен с интервалом %v", analysisInterval)

		for {
			select {
			case <-ticker.C:
				log.Println("[CausalAnalyzer] Выполняю запланированный анализ каузальных связей...")
				b.analyzeCausalLinksForAllChats()
			case <-b.stop:
				log.Println("[CausalAnalyzer] Остановка анализатора каузальных связей...")
				return
			}
		}
	}()
}

// analyzeCausalLinksForAllChats анализирует каузальные связи для всех активных чатов
func (b *Bot) analyzeCausalLinksForAllChats() {
	// Получаем список всех чатов
	chatIDs, err := b.storage.GetAllChatIDs()
	if err != nil {
		log.Printf("[ERROR][CausalAnalyzer] Не удалось получить список чатов: %v", err)
		return
	}

	successCount := 0
	for _, chatID := range chatIDs {
		// Проверяем, активен ли чат
		b.settingsMutex.RLock()
		settings, exists := b.chatSettings[chatID]
		isActive := exists && settings.Active
		b.settingsMutex.RUnlock()

		if isActive {
			if err := b.analyzeCausalLinksForChat(chatID); err != nil {
				log.Printf("[ERROR][CausalAnalyzer] Ошибка анализа каузальных связей для чата %d: %v", chatID, err)
			} else {
				successCount++
			}

			// Пауза между обработкой чатов
			time.Sleep(200 * time.Millisecond)
		}
	}

	log.Printf("[CausalAnalyzer] Анализ завершен для %d активных чатов", successCount)
}

// analyzeCausalLinksForChat анализирует каузальные связи для конкретного чата
func (b *Bot) analyzeCausalLinksForChat(chatID int64) error {
	if b.config.Debug {
		log.Printf("[DEBUG][CausalAnalyzer] Начало анализа каузальных связей для чата %d", chatID)
	}

	// 1. Получаем последние сообщения для анализа
	lookbackCount := b.config.CausalAnalysisLookbackMessages
	if lookbackCount <= 0 {
		lookbackCount = 100
	}

	messages, err := b.storage.GetMessages(chatID, lookbackCount)
	if err != nil {
		return fmt.Errorf("ошибка получения сообщений для анализа: %w", err)
	}

	if len(messages) < 5 {
		if b.config.Debug {
			log.Printf("[DEBUG][CausalAnalyzer] Чат %d: недостаточно сообщений для анализа (%d)", chatID, len(messages))
		}
		return nil
	}

	// 2. Формируем контекст для LLM, используя временное окно для связывания событий
	temporalWindowMinutes := b.config.CausalTemporalWindowMinutes
	if temporalWindowMinutes <= 0 {
		temporalWindowMinutes = 60 // По умолчанию 60 минут
	}
	if b.config.Debug {
		log.Printf("[DEBUG][CausalAnalyzer] Используется временное окно: %d минут", temporalWindowMinutes)
	}
	formattedContext := b.formatMessagesForCausalAnalysis(chatID, messages)

	// 3. Формируем промпт для анализа (обогащение личности через унифицированный метод)
	analysisPrompt := b.enrichPromptWithPersonality(b.config.CausalAnalysisPrompt, chatID, "causal_analysis")

	// Добавляем историю сообщений в конец промпта
	fullPrompt := analysisPrompt + "\n\n" + formattedContext

	if b.config.Debug {
		log.Printf("[DEBUG][CausalAnalyzer] Чат %d: Отправка запроса к LLM для анализа каузальных связей", chatID)
	}

	// 5. Отправляем запрос к LLM
	llmResponse, err := b.llm.GenerateResponseByType(llm.ResponseTypeCausalAnalysis, fullPrompt, "", float32(b.config.GeminiTemperatureNormal))
	if err != nil {
		return fmt.Errorf("ошибка генерации анализа каузальных связей от LLM: %w", err)
	}

	if b.config.Debug {
		log.Printf("[DEBUG][CausalAnalyzer] Чат %d: Сырой ответ LLM:\n%s", chatID, llmResponse)
	}

	// 6. Парсим ответ LLM
	causalLinks, err := b.parseCausalAnalysisResponse(llmResponse)
	if err != nil {
		log.Printf("[WARN][CausalAnalyzer] Чат %d: Ошибка парсинга ответа LLM: %v", chatID, err)
		return nil
	}

	// 7. Сохраняем найденные каузальные связи
	savedCount := 0
	for _, link := range causalLinks {
		if link.Confidence >= b.config.CausalMinConfidence {
			if err := b.saveCausalLink(chatID, link); err != nil {
				log.Printf("[WARN][CausalAnalyzer] Чат %d: Ошибка сохранения каузальной связи: %v", chatID, err)
			} else {
				savedCount++
			}
		}
	}

	if b.config.Debug {
		log.Printf("[DEBUG][CausalAnalyzer] Чат %d: Сохранено %d каузальных связей из %d найденных", chatID, savedCount, len(causalLinks))
	}

	// 8. Очищаем старые записи при необходимости
	if err := b.cleanupCausalMemoryIfNeeded(chatID); err != nil {
		log.Printf("[WARN][CausalAnalyzer] Чат %d: Ошибка очистки каузальной памяти: %v", chatID, err)
	}

	return nil
}

// formatMessagesForCausalAnalysis форматирует сообщения для анализа каузальных связей
func (b *Bot) formatMessagesForCausalAnalysis(chatID int64, messages []*tgbotapi.Message) string {
	if len(messages) == 0 {
		return "Нет сообщений для анализа."
	}

	var result strings.Builder
	result.WriteString("=== ИСТОРИЯ СООБЩЕНИЙ ДЛЯ АНАЛИЗА КАУЗАЛЬНЫХ СВЯЗЕЙ ===\n\n")

	// Получаем профили пользователей для именования
	profiles, _ := b.storage.GetAllUserProfiles(chatID)
	profileMap := make(map[int64]*storage.UserProfile)
	for _, profile := range profiles {
		profileMap[profile.UserID] = profile
	}

	for i, msg := range messages {
		if msg == nil {
			continue
		}

		// Определяем имя отправителя
		senderName := b.getUserDisplayName(msg.From, profileMap[msg.From.ID])

		// Форматируем сообщение
		timestamp := msg.Time().Format("15:04")

		if msg.IsCommand() {
			result.WriteString(fmt.Sprintf("[%s] %s: КОМАНДА %s\n", timestamp, senderName, msg.Text))
		} else if msg.Text != "" {
			result.WriteString(fmt.Sprintf("[%s] %s: %s\n", timestamp, senderName, msg.Text))
		} else if msg.Caption != "" {
			result.WriteString(fmt.Sprintf("[%s] %s: [МЕДИА] %s\n", timestamp, senderName, msg.Caption))
		} else {
			result.WriteString(fmt.Sprintf("[%s] %s: [МЕДИА БЕЗ ПОДПИСИ]\n", timestamp, senderName))
		}

		// Добавляем разделитель после каждых 20 сообщений для лучшей читаемости
		if (i+1)%20 == 0 && i+1 < len(messages) {
			result.WriteString("\n--- ПРОДОЛЖЕНИЕ ---\n\n")
		}
	}

	return result.String()
}

// parseCausalAnalysisResponse парсит ответ LLM и извлекает каузальные связи
func (b *Bot) parseCausalAnalysisResponse(llmResponse string) ([]CausalLink, error) {
	// Очищаем ответ от markdown обертки
	cleanedResponse := b.cleanJSONFromMarkdown(llmResponse)

	// Пытаемся распарсить JSON
	var result CausalAnalysisResult
	if err := json.Unmarshal([]byte(cleanedResponse), &result); err != nil {
		return nil, fmt.Errorf("ошибка парсинга JSON: %w", err)
	}

	// Валидируем и фильтруем результаты
	var validLinks []CausalLink
	for _, link := range result.CausalLinks {
		if b.validateCausalLink(link) {
			validLinks = append(validLinks, link)
		}
	}

	return validLinks, nil
}

// validateCausalLink проверяет валидность каузальной связи
func (b *Bot) validateCausalLink(link CausalLink) bool {
	// Проверяем обязательные поля
	if link.Event == "" || link.Cause == "" || link.Effect == "" {
		return false
	}

	// Проверяем валидность категории
	validCategories := map[string]bool{
		"opinion":      true,
		"relationship": true,
		"worldview":    true,
		"habit":        true,
		"preference":   true,
	}
	if !validCategories[link.Category] {
		return false
	}

	// Проверяем валидность типа триггера
	validTriggerTypes := map[string]bool{
		"conversation": true,
		"reaction":     true,
		"pattern":      true,
		"conflict":     true,
	}
	if !validTriggerTypes[link.TriggerType] {
		return false
	}

	// Проверяем диапазоны значений
	if link.Confidence < 0.0 || link.Confidence > 1.0 {
		return false
	}
	if link.Importance < 0.0 || link.Importance > 1.0 {
		return false
	}

	return true
}

// saveCausalLink сохраняет каузальную связь в базу данных
func (b *Bot) saveCausalLink(chatID int64, link CausalLink) error {
	entry := &storage.CausalMemoryEntry{
		ChatID:       chatID,
		Event:        link.Event,
		Cause:        link.Cause,
		Effect:       link.Effect,
		Category:     link.Category,
		Confidence:   link.Confidence,
		TriggerType:  link.TriggerType,
		Keywords:     link.Keywords,
		UserContext:  link.UserContext,
		TopicContext: link.TopicContext,
		Importance:   link.Importance,
		Relevance:    1.0, // Новые записи имеют максимальную актуальность
	}

	return b.storage.AddCausalEntry(entry)
}

// cleanupCausalMemoryIfNeeded очищает каузальную память если достигнут лимит
func (b *Bot) cleanupCausalMemoryIfNeeded(chatID int64) error {
	// Получаем настройки каузальной памяти
	causalMemory, err := b.storage.GetCausalMemory(chatID)
	if err != nil {
		return err
	}

	// Если превышен лимит записей, очищаем
	if causalMemory.TotalEntries > b.config.CausalMaxEntriesPerChat {
		if b.config.Debug {
			log.Printf("[DEBUG][CausalAnalyzer] Чат %d: Запуск очистки каузальной памяти (%d > %d)",
				chatID, causalMemory.TotalEntries, b.config.CausalMaxEntriesPerChat)
		}
		return b.storage.CleanupCausalMemory(chatID)
	}

	return nil
}

// cleanJSONFromMarkdown удаляет markdown обертку из JSON ответа
func (b *Bot) cleanJSONFromMarkdown(text string) string {
	// Удаляем markdown блоки ```json ... ```
	jsonRegex := regexp.MustCompile("(?s)```json\\s*(.+?)\\s*```")
	if matches := jsonRegex.FindStringSubmatch(text); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Удаляем обычные блоки ``` ... ```
	codeBlockRegex := regexp.MustCompile("(?s)```\\s*(.+?)\\s*```")
	if matches := codeBlockRegex.FindStringSubmatch(text); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Ищем JSON как есть
	jsonStartRegex := regexp.MustCompile("(?s)\\{\\s*\"")
	jsonEndRegex := regexp.MustCompile("(?s)\"\\s*\\}\\s*$")

	jsonStartMatch := jsonStartRegex.FindStringIndex(text)
	jsonEndMatch := jsonEndRegex.FindStringIndex(text)

	if jsonStartMatch != nil && jsonEndMatch != nil {
		return text[jsonStartMatch[0]:jsonEndMatch[1]]
	}

	return strings.TrimSpace(text)
}

// getUserDisplayName получает отображаемое имя пользователя
func (b *Bot) getUserDisplayName(user *tgbotapi.User, profile *storage.UserProfile) string {
	if user == nil {
		return "Unknown"
	}

	// Приоритет: профиль alias > профиль real_name > Telegram FirstName > Username
	if profile != nil {
		if profile.Alias != "" {
			return profile.Alias
		}
		if profile.RealName != "" {
			return profile.RealName
		}
	}

	if user.FirstName != "" {
		return user.FirstName
	}

	if user.UserName != "" {
		return user.UserName
	}

	return fmt.Sprintf("User%d", user.ID)
}

// GetCausalInfluence анализирует влияние каузальной памяти на текущее поведение
func (b *Bot) GetCausalInfluence(chatID int64, currentSituation string) (*BehavioralInfluenceResult, error) {
	if !b.config.CausalLearningEnabled {
		return nil, fmt.Errorf("каузальное обучение отключено")
	}

	// Получаем релевантные каузальные записи
	queryOptions := storage.CausalQueryOptions{
		MinConfidence: b.config.CausalMinConfidence,
		MinRelevance:  0.3,
		SortBy:        "relevance",
		Limit:         20,
	}

	entries, err := b.storage.GetCausalEntries(chatID, queryOptions)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения каузальных записей: %w", err)
	}

	if len(entries) == 0 {
		return &BehavioralInfluenceResult{
			BehavioralAdjustments: []BehavioralAdjustment{},
			TriggeredMemories:     []string{},
			OverallStrategy:       "default",
		}, nil
	}

	// Формируем контекст каузальной памяти
	causalContext := b.formatCausalContext(entries)

	// Формируем промпт для анализа влияния (обогащение личности через унифицированный метод)
	influencePrompt := b.enrichPromptWithPersonality(b.config.CausalInfluencePrompt, chatID, "causal_influence")
	influencePrompt = strings.ReplaceAll(influencePrompt, "{causal_context}", causalContext)
	influencePrompt = strings.ReplaceAll(influencePrompt, "{current_situation}", currentSituation)

	// Отправляем запрос к LLM
	llmResponse, err := b.llm.GenerateResponseByType(llm.ResponseTypeCausalInfluence, influencePrompt, "", float32(b.config.GeminiTemperatureNormal))
	if err != nil {
		return nil, fmt.Errorf("ошибка анализа влияния каузальной памяти: %w", err)
	}

	// Парсим результат
	result, err := b.parseBehavioralInfluenceResponse(llmResponse)
	if err != nil {
		log.Printf("[WARN][CausalAnalyzer] Ошибка парсинга влияния каузальной памяти: %v", err)
		// Возвращаем пустой результат вместо ошибки
		return &BehavioralInfluenceResult{
			BehavioralAdjustments: []BehavioralAdjustment{},
			TriggeredMemories:     []string{},
			OverallStrategy:       "default",
		}, nil
	}

	return result, nil
}

// formatCausalContext форматирует каузальные записи для промпта
func (b *Bot) formatCausalContext(entries []*storage.CausalMemoryEntry) string {
	if len(entries) == 0 {
		return "Каузальная память пуста."
	}

	var result strings.Builder
	result.WriteString("=== КАУЗАЛЬНАЯ ПАМЯТЬ ===\n\n")

	for i, entry := range entries {
		result.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, strings.ToUpper(entry.Category), entry.Event))
		result.WriteString(fmt.Sprintf("   Причина: %s\n", entry.Cause))
		result.WriteString(fmt.Sprintf("   Следствие: %s\n", entry.Effect))
		result.WriteString(fmt.Sprintf("   Уверенность: %.2f, Важность: %.2f\n", entry.Confidence, entry.Importance))
		if entry.UserContext != "" {
			result.WriteString(fmt.Sprintf("   Пользователь: %s\n", entry.UserContext))
		}
		if entry.TopicContext != "" {
			result.WriteString(fmt.Sprintf("   Тема: %s\n", entry.TopicContext))
		}
		result.WriteString("\n")
	}

	return result.String()
}

// parseBehavioralInfluenceResponse парсит ответ о влиянии каузальной памяти
func (b *Bot) parseBehavioralInfluenceResponse(llmResponse string) (*BehavioralInfluenceResult, error) {
	cleanedResponse := b.cleanJSONFromMarkdown(llmResponse)

	var result BehavioralInfluenceResult
	if err := json.Unmarshal([]byte(cleanedResponse), &result); err != nil {
		return nil, fmt.Errorf("ошибка парсинга JSON влияния: %w", err)
	}

	return &result, nil
}

// ApplyRelevanceDecayToCausalMemory применяет затухание релевантности к каузальной памяти
func (b *Bot) ApplyRelevanceDecayToCausalMemory(chatID int64) error {
	if !b.config.CausalLearningEnabled {
		return nil
	}

	// Получаем все записи каузальной памяти для чата
	options := storage.CausalQueryOptions{
		Limit:  1000, // Большой лимит для получения всех записей
		SortBy: "created_at",
	}

	entries, err := b.storage.GetCausalEntries(chatID, options)
	if err != nil {
		return fmt.Errorf("ошибка получения каузальной памяти: %w", err)
	}

	now := time.Now()
	updatedCount := 0

	for _, entry := range entries {
		// Рассчитываем время с момента создания записи
		timeElapsed := now.Sub(entry.CreatedAt)

		// Базовая скорость затухания (0.05 в день = 5% в день)
		decayRate := 0.05

		// Рассчитываем коэффициент затухания на основе времени
		// Формула: новая_релевантность = старая_релевантность * (1 - decayRate * дни)
		daysElapsed := timeElapsed.Hours() / 24.0
		decayFactor := 1.0 - (decayRate * daysElapsed)

		if decayFactor < 0 {
			decayFactor = 0.1 // Минимальная релевантность 10%
		}

		// Применяем затухание к важности (используем Importance как релевантность)
		oldImportance := entry.Importance
		newImportance := entry.Importance * decayFactor

		// Ограничиваем минимальную релевантность значением 0.1
		if newImportance < 0.1 {
			newImportance = 0.1
		}

		// Обновляем только если изменение значительное (больше 0.05)
		if oldImportance-newImportance > 0.05 {
			entry.Importance = newImportance

			// Сохраняем обновлённую запись
			err := b.storage.UpdateCausalEntry(entry)
			if err != nil {
				log.Printf("[WARN][CausalDecay] Ошибка обновления записи каузальной памяти %d: %v", entry.ID, err)
				continue
			}

			updatedCount++

			if b.config.Debug {
				log.Printf("[DEBUG][CausalDecay] Обновлена релевантность записи: %.2f -> %.2f (%.1f дней)",
					oldImportance, newImportance, daysElapsed)
			}
		}
	}

	if updatedCount > 0 {
		log.Printf("[INFO][CausalDecay] Чат %d: Обновлено релевантности %d записей каузальной памяти", chatID, updatedCount)
	}

	return nil
}
