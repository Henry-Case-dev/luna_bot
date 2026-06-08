package bot

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// BeliefAnalysisResult описывает JSON-схему от LLM для анализа убеждений
type BeliefAnalysisResult struct {
	CoreBeliefs []BeliefChange    `json:"core_beliefs"` // список изменений по ключевым убеждениям
	Triggers    []BeliefTriggerR  `json:"triggers"`     // события-триггеры
	Conflicts   []BeliefConflictR `json:"conflicts"`    // обнаруженные конфликты
}

type BeliefChange struct {
	Topic      string   `json:"topic"`
	Content    string   `json:"content"`
	Strength   float64  `json:"strength"`   // 0..1
	Confidence float64  `json:"confidence"` // 0..1
	Evidence   []string `json:"evidence"`
	Stability  float64  `json:"stability"` // 0..1
	Source     string   `json:"source"`
}

type BeliefTriggerR struct {
	Event      string   `json:"event"`
	Topic      string   `json:"topic"`
	Evidence   []string `json:"evidence"`
	Confidence float64  `json:"confidence"`
	Timestamp  string   `json:"timestamp"`
}

type BeliefConflictR struct {
	TopicA    string  `json:"topic_a"`
	TopicB    string  `json:"topic_b"`
	Severity  float64 `json:"severity"`
	Rationale string  `json:"rationale"`
}

// startBeliefAnalyzer запускает фоновую задачу для анализа убеждений
func (b *Bot) startBeliefAnalyzer() {
	if !b.config.BeliefLearningEnabled {
		return
	}
	log.Printf("[BeliefAnalyzer] Запуск анализатора убеждений...")

	// Первый запуск через 4 минуты после старта, чтобы дать собраться контексту
	initialDelay := 4 * time.Minute
	go func() {
		time.Sleep(initialDelay)
		select {
		case <-b.stop:
			return
		default:
			log.Println("[BeliefAnalyzer] Запуск первичного анализа убеждений...")
			b.analyzeBeliefsForAllChats()
		}
	}()

	// Периодический анализ
	interval := time.Duration(b.config.BeliefAnalysisIntervalHours) * time.Hour
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		log.Printf("[BeliefAnalyzer] Запущен с интервалом %v", interval)
		for {
			select {
			case <-ticker.C:
				log.Println("[BeliefAnalyzer] Плановый анализ убеждений...")
				b.analyzeBeliefsForAllChats()
			case <-b.stop:
				log.Println("[BeliefAnalyzer] Остановка анализатора убеждений...")
				return
			}
		}
	}()
}

func (b *Bot) analyzeBeliefsForAllChats() {
	chatIDs, err := b.storage.GetAllChatIDs()
	if err != nil {
		log.Printf("[BeliefAnalyzer ERROR] Не удалось получить список чатов: %v", err)
		return
	}
	for _, chatID := range chatIDs {
		b.settingsMutex.RLock()
		settings, exists := b.chatSettings[chatID]
		// Если настроек чата нет в памяти — считаем активным по умолчанию,
		// чтобы не пропускать обновление убеждений для действующих чатов.
		isActive := true
		if exists {
			isActive = settings.Active
		}
		b.settingsMutex.RUnlock()
		if !isActive {
			if b.config.Debug {
				log.Printf("[BeliefAnalyzer] Пропуск чата %d: помечен как неактивный", chatID)
			}
			continue
		}
		if err := b.analyzeBeliefsForChat(chatID); err != nil {
			log.Printf("[BeliefAnalyzer ERROR] Чат %d: %v", chatID, err)
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func (b *Bot) analyzeBeliefsForChat(chatID int64) error {
	// 1. Загружаем сообщения
	lookback := b.config.BeliefAnalysisLookbackMessages
	if lookback <= 0 {
		lookback = 150
	}
	messages, err := b.storage.GetMessages(chatID, lookback)
	if err != nil {
		return fmt.Errorf("ошибка получения сообщений: %w", err)
	}
	if len(messages) < 5 {
		return nil
	}

	// 2. Формируем контекст
	ctx := b.formatMessagesForBeliefAnalysis(chatID, messages)

	// 3. Подготавливаем промпт
	prompt := b.config.BeliefAnalysisPrompt
	if prompt == "" {
		return nil // Промпт не настроен, пропускаем
	}

	// Обогащаем промпт контекстом личности
	enrichedPrompt := b.enrichPromptWithPersonality(prompt, chatID, "belief_analysis")
	enrichedPrompt = strings.ReplaceAll(enrichedPrompt, "{RECENT_MESSAGES}", ctx)
	enrichedPrompt = strings.ReplaceAll(enrichedPrompt, "{CAUSAL_PATTERNS}", "Каузальные связи анализируются отдельно")
	enrichedPrompt = strings.ReplaceAll(enrichedPrompt, "{EMOTIONAL_MEMORIES}", "Эмоциональные воспоминания интегрированы в контекст личности")

	// 4. Запрос к LLM (используем обогащенный промпт с подстановками)
	resp, err := b.llm.GenerateResponseByType(
		llm.ResponseTypeBeliefAnalysis,
		enrichedPrompt,
		"",
		float32(b.config.BeliefAnalysisPromptTemperature),
	)
	if err != nil {
		return fmt.Errorf("ошибка запроса belief_analysis к LLM: %w", err)
	}

	// 5. Парсинг
	parsed, err := b.parseBeliefAnalysisResponse(resp)
	if err != nil {
		log.Printf("[BeliefAnalyzer WARN] Чат %d: ошибка парсинга ответа LLM: %v", chatID, err)
		return nil
	}

	// 6. Сохранение в PersonalityMemory.BeliefSystem
	if err := b.applyBeliefAnalysis(chatID, parsed); err != nil {
		return fmt.Errorf("ошибка применения анализа убеждений: %w", err)
	}
	return nil
}

func (b *Bot) formatMessagesForBeliefAnalysis(chatID int64, messages []*tgbotapi.Message) string {
	var sb strings.Builder
	sb.WriteString("=== ИСТОРИЯ СООБЩЕНИЙ ДЛЯ АНАЛИЗА УБЕЖДЕНИЙ ===\n\n")
	profiles, _ := b.storage.GetAllUserProfiles(chatID)
	pmap := map[int64]*storage.UserProfile{}
	for _, p := range profiles {
		pmap[p.UserID] = p
	}
	for i, m := range messages {
		if m == nil {
			continue
		}
		name := b.getUserDisplayName(m.From, pmap[m.From.ID])
		text := m.Text
		if text == "" {
			text = m.Caption
		}
		if text == "" {
			continue
		}
		ts := m.Time().Format("15:04")
		sb.WriteString(fmt.Sprintf("%d. [%s] %s: %s\n", i+1, ts, name, text))
		if (i+1)%25 == 0 {
			sb.WriteString("\n--- ПРОДОЛЖЕНИЕ ---\n\n")
		}
	}
	return sb.String()
}

func (b *Bot) parseBeliefAnalysisResponse(llmResponse string) (*BeliefAnalysisResult, error) {
	cleaned := cleanJSONFromMarkdown(llmResponse)
	var res BeliefAnalysisResult
	if err := json.Unmarshal([]byte(cleaned), &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (b *Bot) applyBeliefAnalysis(chatID int64, result *BeliefAnalysisResult) error {
	mem, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil {
		return err
	}
	if mem == nil {
		mem = &storage.PersonalityMemory{ChatID: chatID}
	}
	if mem.BeliefSystem == nil {
		mem.BeliefSystem = &storage.BeliefSystem{
			CoreBeliefs:     map[string]*storage.BeliefEntry{},
			BeliefTriggers:  []*storage.BeliefTrigger{},
			BeliefConflicts: []*storage.BeliefConflict{},
			BeliefVersion:   1,
		}
	}

	// Применяем изменения по убеждениям
	for _, c := range result.CoreBeliefs {
		topic := strings.TrimSpace(c.Topic)
		if topic == "" {
			continue
		}
		if mem.BeliefSystem.CoreBeliefs[topic] == nil {
			mem.BeliefSystem.CoreBeliefs[topic] = &storage.BeliefEntry{
				Topic:      topic,
				Strength:   clamp01(c.Strength),
				Confidence: clamp01(c.Confidence),
				Content:    c.Content,
				Evidence:   dedupNonEmpty(c.Evidence, 5),
				Stability:  clamp01(c.Stability),
				Source:     fallback(c.Source, "belief_analysis"),
				LastUpdate: time.Now(),
			}
		} else {
			be := mem.BeliefSystem.CoreBeliefs[topic]
			// мягкое обновление: увеличиваем силу на 20% от предложенного отклонения
			be.Strength = clamp01(be.Strength + (c.Strength-be.Strength)*0.2)
			be.Confidence = math.Max(be.Confidence, clamp01(c.Confidence))
			if c.Content != "" && c.Confidence > 0.6 {
				be.Content = c.Content
			}
			be.Stability = clamp01((be.Stability*3 + clamp01(c.Stability)) / 4)
			be.Source = fallback(c.Source, be.Source)
			be.Evidence = append(be.Evidence, c.Evidence...)
			if len(be.Evidence) > 5 {
				be.Evidence = be.Evidence[len(be.Evidence)-5:]
			}
			be.LastUpdate = time.Now()
		}
	}

	// Триггеры
	for _, t := range result.Triggers {
		ts := time.Now()
		if parsed, err := time.Parse(time.RFC3339, t.Timestamp); err == nil {
			ts = parsed
		}
		trig := &storage.BeliefTrigger{
			Event:       t.Event,
			Topic:       t.Topic,
			OldStrength: 0,
			NewStrength: 0,
			Evidence:    dedupNonEmpty(t.Evidence, 5),
			Confidence:  clamp01(t.Confidence),
			Timestamp:   ts,
		}
		mem.BeliefSystem.BeliefTriggers = append(mem.BeliefSystem.BeliefTriggers, trig)
		if len(mem.BeliefSystem.BeliefTriggers) > 20 {
			mem.BeliefSystem.BeliefTriggers = mem.BeliefSystem.BeliefTriggers[len(mem.BeliefSystem.BeliefTriggers)-20:]
		}
	}

	// Конфликты
	for _, c := range result.Conflicts {
		if c.TopicA == "" || c.TopicB == "" {
			continue
		}
		mem.BeliefSystem.BeliefConflicts = append(mem.BeliefSystem.BeliefConflicts, &storage.BeliefConflict{
			Topic1:   c.TopicA,
			Topic2:   c.TopicB,
			Conflict: c.Rationale,
			// Resolution intentionally empty for now
			Severity: clamp01(c.Severity),
			Detected: time.Now(),
			Resolved: false,
		})
		if len(mem.BeliefSystem.BeliefConflicts) > 20 {
			mem.BeliefSystem.BeliefConflicts = mem.BeliefSystem.BeliefConflicts[len(mem.BeliefSystem.BeliefConflicts)-20:]
		}
	}

	// Проверяем и пытаемся разрешить конфликты убеждений
	b.resolveBeliefConflicts(mem.BeliefSystem)

	mem.BeliefSystem.LastBeliefUpdate = time.Now()
	mem.BeliefSystem.BeliefVersion++
	return b.storage.SavePersonalityMemory(mem)
}

func dedupNonEmpty(in []string, max int) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

func fallback(s, fb string) string {
	if strings.TrimSpace(s) != "" {
		return s
	}
	return fb
}

// clamp01 clamps a float to [0,1]
// use clamp01 from bot.go

// resolveBeliefConflicts пытается разрешить обнаруженные конфликты между убеждениями
func (b *Bot) resolveBeliefConflicts(beliefSystem *storage.BeliefSystem) {
	for _, conflict := range beliefSystem.BeliefConflicts {
		if conflict.Resolved {
			continue
		}

		belief1, exists1 := beliefSystem.CoreBeliefs[conflict.Topic1]
		belief2, exists2 := beliefSystem.CoreBeliefs[conflict.Topic2]

		if !exists1 || !exists2 {
			continue
		}

		// Простая стратегия разрешения: более уверенное убеждение побеждает
		if belief1.Confidence > belief2.Confidence && (belief1.Confidence-belief2.Confidence) > 0.3 {
			belief2.Strength *= 0.7 // Ослабляем менее уверенное убеждение
			conflict.Resolution = fmt.Sprintf("Убеждение '%s' ослаблено из-за более сильного убеждения '%s'", conflict.Topic2, conflict.Topic1)
			conflict.Resolved = true
		} else if belief2.Confidence > belief1.Confidence && (belief2.Confidence-belief1.Confidence) > 0.3 {
			belief1.Strength *= 0.7
			conflict.Resolution = fmt.Sprintf("Убеждение '%s' ослаблено из-за более сильного убеждения '%s'", conflict.Topic1, conflict.Topic2)
			conflict.Resolved = true
		} else if conflict.Severity > 0.8 {
			// При очень серьёзном конфликте ослабляем оба убеждения
			belief1.Strength *= 0.8
			belief2.Strength *= 0.8
			conflict.Resolution = "Оба конфликтующих убеждения ослаблены из-за серьёзного противоречия"
			conflict.Resolved = true
		}
	}
}

// getBeliefInfluencedResponseType возвращает responseType с учётом текущих убеждений
func (b *Bot) getBeliefInfluencedResponseType(chatID int64, baseResponseType string) string {
	mem, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil || mem == nil || mem.BeliefSystem == nil {
		return baseResponseType
	}

	// Если есть сильные убеждения о серьёзности тем, влияем на responseType
	for topic, belief := range mem.BeliefSystem.CoreBeliefs {
		if belief.Strength > 0.7 && belief.Confidence > 0.6 {
			if strings.Contains(strings.ToLower(topic), "серьёз") ||
				strings.Contains(strings.ToLower(topic), "важн") ||
				strings.Contains(strings.ToLower(topic), "ответственн") {
				return "direct_serious"
			}
		}
	}

	return baseResponseType
}
