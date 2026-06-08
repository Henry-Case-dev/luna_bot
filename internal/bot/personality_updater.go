package bot

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"math"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Регулярные выражения для извлечения ключевых элементов из сообщений
var (
	nameRegex        = regexp.MustCompile(`(?i)(([А-ЯЁ][а-яё]+)\s+([А-ЯЁ][а-яё]+)|@([a-zA-Z0-9_]+))`)
	topicRegex       = regexp.MustCompile(`(?i)(говор[яиео]т|обсужда[яеюо]т|дума[яеюо]т|размышля[яеюо]т)\s+(?:о|об|про)?\s+([а-яёА-ЯЁ\s]{3,30})`)
	selfReflectRegex = regexp.MustCompile(`(?i)(?:Я|Меня|Мне|Мной)\s+([а-яёА-ЯЁ\s]{3,50})`)
)

// startPersonalityUpdater запускает фоновую задачу для обновления памяти личности
// бота на основе последних сообщений
func (b *Bot) startPersonalityUpdater() {
	log.Printf("[PersonalityUpdater] Запуск планировщика обновления личности бота...")

	// Проверяем наличие промптов для анализа
	if b.config.PersonalityAnalysisPrompt == "" {
		log.Printf("[WARN][PersonalityUpdater] Не установлен PERSONALITY_ANALYSIS_PROMPT, использую значение по умолчанию")
		// Устанавливаем значение по умолчанию
		b.config.PersonalityAnalysisPrompt = `Проанализируй последние сообщения в чате и выдели:
1. Имена людей и объектов, которые упоминаются (только реальные имена)
2. Основные темы обсуждения (3-5 тем)
3. Как бот должен себя воспринимать на основе взаимодействия

Формат ответа строго в JSON:
{
  "names": ["Имя1", "Имя2", ...],
  "topics": ["Тема1", "Тема2", ...],
  "self_perceptions": ["Я - бот, который...", "Моя роль - ...", ...]
}
`
	}

	// Запускаем первое обновление через 1 минуту после старта
	initialDelay := 1 * time.Minute
	go func() {
		time.Sleep(initialDelay)
		select {
		case <-b.stop: // Проверяем, не остановлен ли бот
			return
		default:
			log.Println("[PersonalityUpdater] Запуск первичного обновления личности...")
			b.updatePersonalityForAllChats()
		}
	}()

	// Запускаем периодическое обновление
	updateInterval := time.Duration(b.config.PersonalityUpdateIntervalHours) * time.Hour
	if updateInterval <= 0 {
		updateInterval = 1 * time.Hour // Значение по умолчанию, если не указано или отрицательное
	}

	ticker := time.NewTicker(updateInterval)
	go func() {
		defer ticker.Stop()
		log.Printf("[PersonalityUpdater] Запущен с интервалом %v", updateInterval)

		for {
			select {
			case <-ticker.C:
				log.Println("[PersonalityUpdater] Выполняю запланированное обновление личности...")
				b.updatePersonalityForAllChats()
			case <-b.stop:
				log.Println("[PersonalityUpdater] Остановка планировщика личности...")
				return
			}
		}
	}()
}

// updatePersonalityForAllChats обновляет личность для всех активных чатов
func (b *Bot) updatePersonalityForAllChats() {
	// Получаем список всех чатов
	chatIDs, err := b.storage.GetAllChatIDs()
	if err != nil {
		log.Printf("[ERROR][PersonalityUpdater] Не удалось получить список чатов: %v", err)
		return
	}

	// Для каждого активного чата обновляем личность в отдельной горутине
	var wg sync.WaitGroup
	for _, chatID := range chatIDs {
		// Проверяем, активен ли чат
		b.settingsMutex.RLock()
		settings, exists := b.chatSettings[chatID]
		isActive := exists && settings.Active
		b.settingsMutex.RUnlock()

		if isActive {
			wg.Add(1)
			go func(cid int64) {
				defer wg.Done()
				if err := b.updatePersonalityForChat(cid); err != nil {
					log.Printf("[ERROR][PersonalityUpdater] Ошибка обновления личности для чата %d: %v", cid, err)
				}

				// Небольшая пауза между обработкой чатов
				time.Sleep(100 * time.Millisecond)
			}(chatID)
		}
	}
	wg.Wait()
}

// Структура для десериализации ответа LLM
type PersonalityAnalysisResult struct {
	Names                 []string           `json:"names"`
	Topics                []string           `json:"topics"`
	SelfPerceptions       []string           `json:"self_perceptions"`
	CurrentViews          []string           `json:"current_views"`          // НОВОЕ
	TemporalTraits        map[string]float64 `json:"temporal_traits"`        // НОВОЕ
	ContextualAdaptations []string           `json:"contextual_adaptations"` // НОВОЕ
}

// updatePersonalityForChat обновляет личность бота для конкретного чата
// с помощью LLM, который анализирует последние сообщения
func (b *Bot) updatePersonalityForChat(chatID int64) error {
	if b.config.Debug {
		log.Printf("[DEBUG][PersonalityUpdater] Начало обновления личности для чата %d", chatID)
	}

	// 1. Получаем последние сообщения для анализа
	lookbackCount := b.config.PersonalityMessagesLookback
	if lookbackCount <= 0 {
		lookbackCount = 50 // По умолчанию анализируем последние 50 сообщений
	}

	messages, err := b.storage.GetMessages(chatID, lookbackCount)
	if err != nil {
		log.Printf("[ERROR][PersonalityUpdater] Ошибка получения сообщений для чата %d: %v", chatID, err)
		return err
	}

	if len(messages) == 0 {
		log.Printf("[INFO][PersonalityUpdater] Чат %d: нет сообщений для анализа", chatID)
		return nil
	}

	// 2. Получаем текущую личность (или создаем новую, если не существует)
	memory, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil {
		log.Printf("[WARN][PersonalityUpdater] Не удалось получить существующую память личности: %v", err)
		// Создаем новую базовую личность
		memory = &storage.PersonalityMemory{
			ChatID:            chatID,
			NameMentions:      map[string]bool{},
			RecentTopics:      []string{},
			SelfPerception:    []string{},
			DiscussionContext: map[string]bool{},
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		}
	}

	// Формируем промпт для анализа
	// ВАЖНО: Правильная замена плейсхолдеров - последовательно три раза
	// 1. Имя из конфига
	// 2. История сообщений
	// 3. Как бот должен себя воспринимать на основе взаимодействия
	analysisPrompt := b.config.PersonalityAnalysisPrompt

	// 3. Формируем контекст для LLM
	formattedContext := formatMessagesForPersonalityAnalysis(chatID, messages, b.storage)

	log.Printf("[PersonalityUpdater DEBUG] Чат %d: Отправка запроса к LLM для анализа личности. Промпт: %s, Контекст (начало): %.100s...", chatID, analysisPrompt, formattedContext)
	// 4. Отправляем запрос к LLM для анализа сообщений (с пониженной температурой для стабильного JSON)
	// Используем консервативную температуру для повышения детерминизма JSON
	stableTemp := float32(0.2)
	llmResponse, err := b.llm.GenerateResponseByType(llm.ResponseTypePersonalityAnalysis, analysisPrompt+"\nОтветь строго в формате JSON без пояснений, без текста вне JSON.", formattedContext, stableTemp)
	if err != nil {
		return fmt.Errorf("ошибка генерации анализа личности от LLM: %w", err)
	}

	// Логируем сырой ответ от LLM до очистки
	log.Printf("[DEBUG][PersonalityUpdater] Chat %d: Сырой ответ LLM для анализа личности:\n%s", chatID, llmResponse)

	// Очищаем ответ от возможных метаданных и markdown-форматирования
	cleanedResponse := cleanupLLMResponse(llmResponse)

	// Попытка строгого извлечения JSON и починки распространенных проблем
	rawJSON := extractJSONBlock(cleanedResponse)
	rawJSON = repairCommonJSONIssues(rawJSON)
	// Дополнительно очищаем от markdown code blocks (защита от вложенных случаев)
	rawJSON = cleanJSONFromMarkdown(rawJSON)

	var res PersonalityAnalysisResult
	unmarshalErr := json.Unmarshal([]byte(rawJSON), &res)

	if unmarshalErr != nil {
		log.Printf("[ERROR][PersonalityUpdater] Chat %d: Ошибка парсинга JSON: %v. Попытка дополнительного ремонта...", chatID, unmarshalErr)
		// Вторая попытка: более агрессивная починка
		repaired := aggressiveRepairJSON(rawJSON)
		if err2 := json.Unmarshal([]byte(repaired), &res); err2 != nil {
			log.Printf("[ERROR][PersonalityUpdater] Chat %d: Парсинг не удался после ремонта: %v. JSON (усечён): %.400s", chatID, err2, truncateString(rawJSON, 400))
		} else {
			log.Printf("[INFO][PersonalityUpdater] Chat %d: JSON успешно распознан после ремонта.", chatID)
		}
	} else {
		log.Printf("[DEBUG][PersonalityUpdater] Chat %d: Распарсенный результат анализа личности: %+v", chatID, res)
	}

	// Даже если ошибка анмаршалинга, res будет пустым, и циклы ниже просто не выполнятся
	// что является безопасным поведением по умолчанию.

	// 6. Обновляем память личности с учетом ограничений

	// 6.1 Обновляем упоминания имен
	if len(res.Names) > 0 {
		// Создаем новый список имен
		newNameMentions := map[string]bool{}

		// Добавляем новые имена с ограничением количества
		namesCount := 0
		for _, name := range res.Names {
			if name != "" && namesCount < b.config.MaxNameMentions {
				newNameMentions[name] = true
				namesCount++
			}
		}
		memory.NameMentions = newNameMentions
	}

	// 6.2 Обновляем недавние темы
	if len(res.Topics) > 0 {
		// Полностью заменяем список тем из результата LLM
		newTopics := make([]string, 0, b.config.MaxRecentTopics)
		topicsAdded := 0

		for _, topic := range res.Topics {
			if topic != "" && topicsAdded < b.config.MaxRecentTopics {
				newTopics = append(newTopics, topic)
				topicsAdded++
			}
		}
		memory.RecentTopics = newTopics
	}

	// 6.3 Обновляем самовосприятие
	if len(res.SelfPerceptions) > 0 {
		// Оставляем базовое самовосприятие и добавляем новые
		newSelfPerception := []string{}

		for _, perception := range res.SelfPerceptions {
			// Добавляем только если не пустое и не дублирует базовое
			if perception != "" &&

				len(newSelfPerception) < b.config.MaxSelfPerceptions {
				newSelfPerception = append(newSelfPerception, perception)
			}
		}
		memory.SelfPerception = newSelfPerception
	}

	// 6.4 Обновляем текущие взгляды (НОВОЕ)
	if len(res.CurrentViews) > 0 {
		// Добавляем новые взгляды, ограничивая количество
		memory.CurrentViews = limitStringSlice(append(memory.CurrentViews, res.CurrentViews...), 5)
	}

	// 6.5 Обновляем временные черты (НОВОЕ)
	if len(res.TemporalTraits) > 0 {
		if memory.TemporalTraits == nil {
			memory.TemporalTraits = make(map[string]float64)
		}
		// Обновляем временные черты
		for trait, intensity := range res.TemporalTraits {
			memory.TemporalTraits[trait] = intensity
		}
		// Ограничиваем количество черт (оставляем 5 с наибольшей интенсивностью)
		if len(memory.TemporalTraits) > 5 {
			// Создаем слайс для сортировки
			type traitPair struct {
				trait     string
				intensity float64
			}
			var traits []traitPair
			for trait, intensity := range memory.TemporalTraits {
				traits = append(traits, traitPair{trait, intensity})
			}
			// Оставляем только топ-5 по интенсивности
			if len(traits) > 5 {
				// Простая сортировка по убыванию интенсивности
				for i := 0; i < 5; i++ {
					maxIdx := i
					for j := i + 1; j < len(traits); j++ {
						if traits[j].intensity > traits[maxIdx].intensity {
							maxIdx = j
						}
					}
					traits[i], traits[maxIdx] = traits[maxIdx], traits[i]
				}
				// Оставляем только топ-5
				newTraits := make(map[string]float64)
				for i := 0; i < 5; i++ {
					newTraits[traits[i].trait] = traits[i].intensity
				}
				memory.TemporalTraits = newTraits
			}
		}
	}

	// 6.6 Обновляем контекстуальные адаптации (НОВОЕ)
	if len(res.ContextualAdaptations) > 0 {
		memory.ContextualAdaptations = limitStringSlice(append(memory.ContextualAdaptations, res.ContextualAdaptations...), 5)
	}

	// 7. Обновляем DiscussionContext (активные темы) из RecentTopics
	newDiscussionContext := make(map[string]bool)
	for i, topic := range memory.RecentTopics {
		if i >= b.config.MaxDiscussionContexts {
			break
		}
		newDiscussionContext[topic] = true
	}
	memory.DiscussionContext = newDiscussionContext

	// 8. Обновляем timestamp
	memory.UpdatedAt = time.Now()

	// === ИНТЕГРАЦИЯ С КАУЗАЛЬНЫМ ОБУЧЕНИЕМ (ЭТАП 1) ===
	// Получаем влияние каузальной памяти на формирование личности
	if b.config.CausalLearningEnabled {
		causalInfluence, err := b.getCausalInfluenceForPersonality(chatID, &res)
		if err != nil {
			log.Printf("[WARN][PersonalityUpdater] Ошибка получения каузального влияния: %v", err)
		} else if causalInfluence != nil {
			// Применяем каузальные корректировки к личности
			if b.applyCausalInfluenceToPersonality(memory, causalInfluence) {
				log.Printf("[DEBUG][PersonalityUpdater] Чат %d: Применены каузальные корректировки к личности", chatID)
			}
		}
	}

	// Диагностика: сводка изменений
	log.Printf("[INFO][PersonalityUpdater] Chat %d: Итог обновления: names=%d, topics=%d, self=%d, views=%d, traits=%d, adaptations=%d", chatID, len(memory.NameMentions), len(memory.RecentTopics), len(memory.SelfPerception), len(memory.CurrentViews), len(memory.TemporalTraits), len(memory.ContextualAdaptations))

	// 9. Сохраняем обновленную память в хранилище
	err = b.storage.SavePersonalityMemory(memory)
	if err != nil {
		log.Printf("[ERROR][PersonalityUpdater] Не удалось сохранить обновленную память личности: %v", err)
		return err
	}

	if b.config.Debug {
		log.Printf("[DEBUG][PersonalityUpdater] Успешно обновлена память личности для чата %d", chatID)
	}
	return nil
}

// formatMessagesForPersonalityAnalysis форматирует сообщения для анализа LLM
func formatMessagesForPersonalityAnalysis(chatID int64, messages []*tgbotapi.Message, store storage.ChatHistoryStorage) string {
	var sb strings.Builder
	sb.WriteString("Последние сообщения в чате:\n\n")

	// Составляем карту профилей для быстрого доступа
	profileMap := make(map[int64]*storage.UserProfile)
	profiles, _ := store.GetAllUserProfiles(chatID)
	for _, profile := range profiles {
		profileMap[profile.UserID] = profile
	}

	for _, msg := range messages {
		// Получаем имя пользователя
		userName := msg.From.UserName
		if userName == "" {
			userName = msg.From.FirstName
		}

		// Проверяем, есть ли сохраненный профиль для этого пользователя
		profile, exists := profileMap[msg.From.ID]
		if exists && profile.Alias != "" {
			userName = profile.Alias
		}

		// Форматируем сообщение
		text := msg.Text
		if text == "" {
			text = msg.Caption
		}

		if text != "" {
			// Добавляем информацию о пользователе и текст сообщения
			sb.WriteString(userName)
			if msg.From.IsBot {
				sb.WriteString(" [бот]")
			}
			sb.WriteString(": ")
			sb.WriteString(text)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// --- JSON repair helpers (local to personality updater) ---
// extractJSONBlock пытается выбрать JSON-объект из произвольного текста
func extractJSONBlock(s string) string {
	if s == "" {
		return s
	}
	// Сначала ищем fenced блоки ```json ... ```
	reFence := regexp.MustCompile("(?s)```json\\s*(\\{.*?\\})\\s*```")
	if m := reFence.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	// Затем ищем первый и последний фигурные скобки
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

// repairCommonJSONIssues устраняет типичные проблемы: висячие запятые, одинарные кавычки, trailing текст
func repairCommonJSONIssues(s string) string {
	out := strings.TrimSpace(s)
	// Удаляем невидимые символы вокруг
	out = strings.Trim(out, "`\n\r ")
	// Заменяем одинарные кавычки на двойные, если нет двойных
	if !strings.Contains(out, "\"") && strings.Contains(out, "'") {
		out = strings.ReplaceAll(out, "'", "\"")
	}
	// Убираем висячие запятые перед закрывающими скобками
	trailingComma := regexp.MustCompile(`,\s*([}\]])`)
	out = trailingComma.ReplaceAllString(out, "$1")
	return out
}

// aggressiveRepairJSON — более агрессивная попытка: удаление всего вне внешних { }
func aggressiveRepairJSON(s string) string {
	// Оставляем только подстроку от первой { до последней }
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		s = s[start : end+1]
	}
	return repairCommonJSONIssues(s)
}

// truncateString безопасно обрезает строку до N символов
func truncateString(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}

// startPersonalityServices запускает все сервисы, связанные с личностью бота
func (b *Bot) startPersonalityServices() {
	b.startPersonalityUpdater()

	// Запускаем каузальный анализатор (Этап 1)
	go b.startCausalAnalyzer()

	// Запускаем эмоциональный анализатор (Этап 2)
	go b.startEmotionalAnalyzer()

	// Облако ассоциаций (легковесно, зафичено флагом)
	go b.startAssociationCloud()

	// Анализатор убеждений (Этап 3 — Belief System)
	go b.startBeliefAnalyzer()
}

// startAssociationCloud — периодически обновляет лёгкий граф ассоциаций по сообщениям
func (b *Bot) startAssociationCloud() {
	if !b.config.AssociationCloudEnabled {
		return
	}
	interval := 2 * time.Hour
	ticker := time.NewTicker(interval)
	log.Printf("[AssociationCloud] Запуск с интервалом %v", interval)
	go func() {
		defer ticker.Stop()
		// первичный прогон через 3 минуты
		select {
		case <-time.After(3 * time.Minute):
			b.updateAssociationCloudAllChats()
		case <-b.stop:
			return
		}
		for {
			select {
			case <-ticker.C:
				b.updateAssociationCloudAllChats()
			case <-b.stop:
				return
			}
		}
	}()
}

func (b *Bot) updateAssociationCloudAllChats() {
	if !b.config.AssociationCloudEnabled {
		return
	}
	chatIDs, err := b.storage.GetAllChatIDs()
	if err != nil {
		log.Printf("[AssociationCloud] Не удалось получить список чатов: %v", err)
		return
	}
	for _, chatID := range chatIDs {
		b.settingsMutex.RLock()
		settings, exists := b.chatSettings[chatID]
		isActive := exists && settings.Active
		b.settingsMutex.RUnlock()
		if !isActive {
			continue
		}
		if err := b.updateAssociationCloudForChat(chatID); err != nil {
			log.Printf("[AssociationCloud] Чат %d: ошибка обновления: %v", chatID, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (b *Bot) updateAssociationCloudForChat(chatID int64) error {
	// берём последние N сообщений и выделяем простые сущности
	lookback := 150
	msgs, err := b.storage.GetMessages(chatID, lookback)
	if err != nil || len(msgs) == 0 {
		return err
	}

	// профили для имён
	profiles, _ := b.storage.GetAllUserProfiles(chatID)
	pmap := map[int64]*storage.UserProfile{}
	for _, p := range profiles {
		pmap[p.UserID] = p
	}

	// собираем инкременты связей: автор ↔ тема/эмодзи/слова
	batch := &storage.AssocUpdateBatch{Nodes: []*storage.AssocUpdate{}, Decay: 0.98}
	emojiRe := regexp.MustCompile(`([\p{So}\p{Sk}])`)
	topicRe := regexp.MustCompile(`(?i)#?(\p{L}[\p{L}\p{Nd}_-]{3,20})`)

	for _, m := range msgs {
		if m == nil {
			continue
		}
		author := b.getUserDisplayName(m.From, pmap[m.From.ID])
		text := m.Text
		if text == "" {
			text = m.Caption
		}
		if text == "" {
			continue
		}

		// автор — узел типа user
		u := &storage.AssocUpdate{
			NodeType:      "user",
			NodeKey:       author,
			Increments:    map[string]float64{},
			NeighborTypes: map[string]string{},
		}

		// эмодзи как узлы типа emoji
		for _, em := range emojiRe.FindAllString(text, -1) {
			u.Increments[em] += 0.5
			u.NeighborTypes[em] = "emoji"
		}
		// упрощённые темы/теги как узлы типа topic
		for _, tok := range topicRe.FindAllString(text, -1) {
			if len(tok) < 3 || len(tok) > 20 {
				continue
			}
			u.Increments[strings.ToLower(tok)] += 0.3
			u.NeighborTypes[strings.ToLower(tok)] = "topic"
		}
		if len(u.Increments) > 0 {
			batch.Nodes = append(batch.Nodes, u)
		}
	}

	if len(batch.Nodes) == 0 {
		return nil
	}
	// ограничение на размер батча во избежание перегрузки
	if len(batch.Nodes) > 500 {
		batch.Nodes = batch.Nodes[:500]
	}
	return b.storage.UpdateAssocGraph(chatID, batch)
}

// Методы для работы с PersonalityMemory для конкретного чата

// AddNameMentionForChat добавляет имя в список упоминаний для конкретного чата
func (b *Bot) AddNameMentionForChat(chatID int64, name string) error {
	if name == "" {
		return nil // Пропускаем пустые имена
	}

	memory, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil {
		log.Printf("[ERROR][AddNameMention] Чат %d: %v", chatID, err)
		return err
	}

	// Если не существует карты упоминаний, создаем
	if memory.NameMentions == nil {
		memory.NameMentions = make(map[string]bool)
	}

	// Добавляем в карту упоминаний
	if len(memory.NameMentions) < b.config.MaxNameMentions {
		memory.NameMentions[name] = true
		memory.UpdatedAt = time.Now()

		if err := b.storage.SavePersonalityMemory(memory); err != nil {
			log.Printf("[ERROR][AddNameMention] Чат %d: %v", chatID, err)
			return err
		}
	}

	return nil
}

// AddRecentTopicForChat добавляет тему в список недавних для конкретного чата
func (b *Bot) AddRecentTopicForChat(chatID int64, topic string) error {
	if topic == "" {
		return nil // Пропускаем пустые темы
	}

	memory, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil {
		log.Printf("[ERROR][AddRecentTopic] Чат %d: %v", chatID, err)
		return err
	}

	// Проверяем, не содержится ли тема уже в списке
	for _, existingTopic := range memory.RecentTopics {
		if strings.EqualFold(existingTopic, topic) {
			return nil // Тема уже есть, не дублируем
		}
	}

	// Добавляем тему, смещая старые, если достигнут лимит
	if len(memory.RecentTopics) >= b.config.MaxRecentTopics {
		// Сдвигаем список тем (удаляем самую старую)
		if len(memory.RecentTopics) > 1 {
			memory.RecentTopics = append(memory.RecentTopics[1:], topic)
		} else {
			memory.RecentTopics = append(memory.RecentTopics, topic)
		}
	} else {
		memory.RecentTopics = append(memory.RecentTopics, topic)
	}

	memory.UpdatedAt = time.Now()
	return b.storage.SavePersonalityMemory(memory)
}

// AddSelfPerceptionForChat добавляет элемент самовосприятия для конкретного чата
func (b *Bot) AddSelfPerceptionForChat(chatID int64, perception string) error {
	if perception == "" {
		return nil // Пропускаем пустые восприятия
	}

	memory, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil {
		log.Printf("[ERROR][AddSelfPerception] Чат %d: %v", chatID, err)
		return err
	}

	// Проверяем, не содержится ли восприятие уже в списке
	for _, existingPerception := range memory.SelfPerception {
		if strings.EqualFold(existingPerception, perception) {
			return nil // Восприятие уже есть, не дублируем
		}
	}

	// Добавляем восприятие с ограничением количества
	if len(memory.SelfPerception) >= b.config.MaxSelfPerceptions {
		// Оставляем первое (базовое) и заменяем последнее
		if len(memory.SelfPerception) > 0 {
			memory.SelfPerception[len(memory.SelfPerception)-1] = perception
		}
	} else {
		memory.SelfPerception = append(memory.SelfPerception, perception)
	}

	memory.UpdatedAt = time.Now()
	return b.storage.SavePersonalityMemory(memory)
}

// AddDiscussionContextForChat добавляет тему в текущий контекст обсуждения для конкретного чата
func (b *Bot) AddDiscussionContextForChat(chatID int64, topic string) error {
	if topic == "" {
		return nil // Пропускаем пустые темы
	}

	memory, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil {
		log.Printf("[ERROR][AddDiscussionContext] Чат %d: %v", chatID, err)
		return err
	}

	// Если не существует карты контекста, создаем
	if memory.DiscussionContext == nil {
		memory.DiscussionContext = make(map[string]bool)
	}

	// Добавляем тему, с ограничением количества
	if len(memory.DiscussionContext) >= b.config.MaxDiscussionContexts {
		// Если достигнут лимит, очищаем контекст и добавляем новую тему
		memory.DiscussionContext = map[string]bool{topic: true}
	} else {
		memory.DiscussionContext[topic] = true
	}

	memory.UpdatedAt = time.Now()
	return b.storage.SavePersonalityMemory(memory)
}

// ClearDiscussionContextForChat очищает текущий контекст обсуждения для конкретного чата
func (b *Bot) ClearDiscussionContextForChat(chatID int64) error {
	memory, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil {
		log.Printf("[ERROR][ClearDiscussionContext] Чат %d: %v", chatID, err)
		return err
	}

	memory.DiscussionContext = make(map[string]bool)
	memory.UpdatedAt = time.Now()
	return b.storage.SavePersonalityMemory(memory)
}

// getCausalInfluenceForPersonality получает влияние каузальной памяти на формирование личности
func (b *Bot) getCausalInfluenceForPersonality(chatID int64, analysisResult *PersonalityAnalysisResult) (*BehavioralInfluenceResult, error) {
	// Формируем контекст ситуации на основе результатов анализа личности
	situationContext := fmt.Sprintf(
		"Обновление личности бота. Новые данные: темы=%v, самовосприятие=%v, взгляды=%v, черты=%v, адаптации=%v",
		analysisResult.Topics,
		analysisResult.SelfPerceptions,
		analysisResult.CurrentViews,
		analysisResult.TemporalTraits,
		analysisResult.ContextualAdaptations,
	)

	return b.GetCausalInfluence(chatID, situationContext)
}

// applyCausalInfluenceToPersonality применяет каузальные корректировки к личности
func (b *Bot) applyCausalInfluenceToPersonality(memory *storage.PersonalityMemory, influence *BehavioralInfluenceResult) bool {
	changed := false

	// Инициализируем BeliefSystem если не существует
	if memory.BeliefSystem == nil {
		memory.BeliefSystem = &storage.BeliefSystem{
			CoreBeliefs:      make(map[string]*storage.BeliefEntry),
			BeliefTriggers:   []*storage.BeliefTrigger{},
			BeliefConflicts:  []*storage.BeliefConflict{},
			LastBeliefUpdate: time.Now(),
			BeliefVersion:    1,
		}
		changed = true
	}

	// Применяем поведенческие корректировки
	for _, adjustment := range influence.BehavioralAdjustments {
		switch adjustment.Aspect {
		case "worldview":
			// Обновляем убеждения на основе каузальных связей
			if b.updateBeliefFromCausal(memory.BeliefSystem, adjustment) {
				changed = true
			}
		case "style":
			// Обновляем контекстуальные адаптации
			if b.updateStyleFromCausal(memory, adjustment) {
				changed = true
			}
		case "preference":
			// Обновляем предпочтения в темах
			if b.updatePreferencesFromCausal(memory, adjustment) {
				changed = true
			}
		case "relationship":
			// Обновляем отношения с пользователями
			if b.updateRelationshipsFromCausal(memory, adjustment) {
				changed = true
			}
		}
	}

	// Обновляем версию системы убеждений
	if changed {
		memory.BeliefSystem.LastBeliefUpdate = time.Now()
		memory.BeliefSystem.BeliefVersion++
	}

	return changed
}

// updateBeliefFromCausal обновляет убеждения на основе каузальных связей
func (b *Bot) updateBeliefFromCausal(beliefSystem *storage.BeliefSystem, adjustment BehavioralAdjustment) bool {
	// Извлекаем тему убеждения из корректировки
	topic := adjustment.Aspect
	if topic == "" {
		return false
	}

	// Создаем или обновляем убеждение
	if beliefSystem.CoreBeliefs[topic] == nil {
		beliefSystem.CoreBeliefs[topic] = &storage.BeliefEntry{
			Topic:      topic,
			Strength:   adjustment.Confidence,
			Confidence: adjustment.Confidence,
			Content:    adjustment.Adjustment,
			Evidence:   []string{adjustment.Reason},
			LastUpdate: time.Now(),
			Source:     "causal_learning",
			Stability:  0.5, // Средняя стабильность для новых убеждений
		}
		return true
	}

	// Обновляем существующее убеждение
	belief := beliefSystem.CoreBeliefs[topic]
	oldStrength := belief.Strength

	// Усиливаем убеждение на основе каузальной связи
	strengthAdjustment := adjustment.Confidence * 0.3 // Максимум 30% изменения
	belief.Strength = math.Min(1.0, belief.Strength+strengthAdjustment)
	belief.Confidence = math.Max(belief.Confidence, adjustment.Confidence)

	// Обновляем содержание если уверенность выше
	if adjustment.Confidence > 0.7 {
		belief.Content = adjustment.Adjustment
	}

	// Добавляем новое доказательство
	belief.Evidence = append(belief.Evidence, adjustment.Reason)
	if len(belief.Evidence) > 5 {
		belief.Evidence = belief.Evidence[len(belief.Evidence)-5:] // Храним последние 5 доказательств
	}

	belief.LastUpdate = time.Now()

	// Добавляем триггер изменения убеждения
	trigger := &storage.BeliefTrigger{
		Event:       "causal_influence",
		Topic:       topic,
		OldStrength: oldStrength,
		NewStrength: belief.Strength,
		Evidence:    []string{adjustment.Reason},
		Confidence:  adjustment.Confidence,
		Timestamp:   time.Now(),
	}
	beliefSystem.BeliefTriggers = append(beliefSystem.BeliefTriggers, trigger)

	// Ограничиваем количество триггеров
	if len(beliefSystem.BeliefTriggers) > 20 {
		beliefSystem.BeliefTriggers = beliefSystem.BeliefTriggers[len(beliefSystem.BeliefTriggers)-20:]
	}

	return true
}

// updateStyleFromCausal обновляет стиль общения на основе каузальных связей
func (b *Bot) updateStyleFromCausal(memory *storage.PersonalityMemory, adjustment BehavioralAdjustment) bool {
	// Добавляем каузальную адаптацию стиля
	adaptationText := fmt.Sprintf("[КАУЗАЛЬНАЯ] %s: %s", adjustment.Aspect, adjustment.Adjustment)

	// Проверяем, не дублируется ли адаптация
	for _, existing := range memory.ContextualAdaptations {
		if existing == adaptationText {
			return false
		}
	}

	memory.ContextualAdaptations = append(memory.ContextualAdaptations, adaptationText)

	// Ограничиваем количество адаптаций
	if len(memory.ContextualAdaptations) > 5 {
		memory.ContextualAdaptations = memory.ContextualAdaptations[len(memory.ContextualAdaptations)-5:]
	}

	return true
}

// updatePreferencesFromCausal обновляет предпочтения на основе каузальных связей
func (b *Bot) updatePreferencesFromCausal(memory *storage.PersonalityMemory, adjustment BehavioralAdjustment) bool {
	// Добавляем каузальное влияние на взгляды
	viewText := fmt.Sprintf("[КАУЗАЛЬНОЕ] %s", adjustment.Adjustment)

	// Проверяем, не дублируется ли взгляд
	for _, existing := range memory.CurrentViews {
		if existing == viewText {
			return false
		}
	}

	memory.CurrentViews = append(memory.CurrentViews, viewText)

	// Ограничиваем количество взглядов
	if len(memory.CurrentViews) > 5 {
		memory.CurrentViews = memory.CurrentViews[len(memory.CurrentViews)-5:]
	}

	return true
}

// updateRelationshipsFromCausal обновляет отношения с пользователями на основе каузальных связей
func (b *Bot) updateRelationshipsFromCausal(memory *storage.PersonalityMemory, adjustment BehavioralAdjustment) bool {
	// Обновляем черты характера связанные с отношениями
	if memory.TemporalTraits == nil {
		memory.TemporalTraits = make(map[string]float64)
	}

	traitKey := fmt.Sprintf("relationship_%s", adjustment.Aspect)
	oldValue := memory.TemporalTraits[traitKey]
	newValue := math.Min(1.0, adjustment.Confidence)

	memory.TemporalTraits[traitKey] = newValue

	// Возвращаем true если значение изменилось
	return math.Abs(oldValue-newValue) > 0.1
}
