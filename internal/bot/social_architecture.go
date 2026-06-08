package bot

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/storage"
)

// UpdateRelationshipFromInteraction обновляет отношения с пользователем на основе взаимодействия
func (b *Bot) UpdateRelationshipFromInteraction(chatID int64, userID int64, interactionType string, sentiment string, messageContent string) {
	if !b.config.RelationshipTrackingEnabled {
		return
	}

	mem, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil || mem == nil {
		return
	}

	if mem.Relationships == nil {
		mem.Relationships = make(map[string]*storage.Relationship)
	}

	key := fmt.Sprintf("%d", userID)
	rel, exists := mem.Relationships[key]
	if !exists {
		rel = &storage.Relationship{
			UserID:             userID,
			ChatID:             chatID,
			Intimacy:           0.1,
			Trust:              0.5,
			Respect:            0.5,
			Affection:          0.3,
			Familiarity:        0.0,
			KeyMoments:         []storage.RelationshipEvent{},
			Conflicts:          []storage.ConflictMemory{},
			SharedExperiences:  []storage.SharedMemory{},
			InsideJokes:        []string{},
			CommunicationStyle: "neutral",
			PreferredTopics:    []string{},
			AvoidedTopics:      []string{},
			Humor:              "unknown",
			LastInteraction:    time.Now(),
			TotalInteractions:  0,
			AverageGap:         0,
		}
		mem.Relationships[key] = rel
	}

	// Обновляем статистику взаимодействий
	now := time.Now()
	if rel.TotalInteractions > 0 {
		gap := now.Sub(rel.LastInteraction)
		rel.AverageGap = time.Duration((int64(rel.AverageGap)*int64(rel.TotalInteractions) + int64(gap)) / int64(rel.TotalInteractions+1))
	}
	rel.LastInteraction = now
	rel.TotalInteractions++

	// Обновляем параметры отношений
	b.updateRelationshipParameters(rel, interactionType, sentiment, messageContent)

	// Сохраняем обновлённую память
	if err := b.storage.SavePersonalityMemory(mem); err != nil {
		log.Printf("[RelationshipUpdate] Ошибка сохранения отношений для чата %d, пользователь %d: %v", chatID, userID, err)
	}
}

func (b *Bot) updateRelationshipParameters(rel *storage.Relationship, interactionType, sentiment, messageContent string) {
	// Увеличиваем знакомство при любом взаимодействии
	rel.Familiarity = clamp01(rel.Familiarity + 0.01)

	switch interactionType {
	case "positive_reaction":
		rel.Affection = clamp01(rel.Affection + 0.1)
		rel.Trust = clamp01(rel.Trust + 0.05)
		b.addRelationshipEvent(rel, "positive_feedback", "Получил позитивную реакцию на свой ответ", 0.3)

	case "negative_reaction":
		rel.Trust = clamp01(rel.Trust - 0.1)
		if strings.Contains(strings.ToLower(messageContent), "глуп") {
			rel.Respect = clamp01(rel.Respect - 0.15)
			b.addRelationshipEvent(rel, "disrespect", "Был назван глупым", -0.4)
		}
		b.addConflictMemory(rel, "negative_reaction", messageContent)

	case "direct_message":
		rel.Intimacy = clamp01(rel.Intimacy + 0.02)
		if sentiment == "positive" {
			rel.Affection = clamp01(rel.Affection + 0.05)
		}

	case "question":
		rel.Trust = clamp01(rel.Trust + 0.03)
		rel.Respect = clamp01(rel.Respect + 0.02)

	case "joke_shared":
		rel.Affection = clamp01(rel.Affection + 0.08)
		rel.Intimacy = clamp01(rel.Intimacy + 0.05)
		b.addSharedExperience(rel, "humor", "Поделились шуткой")

	case "personal_info":
		rel.Intimacy = clamp01(rel.Intimacy + 0.15)
		rel.Trust = clamp01(rel.Trust + 0.1)
		b.addRelationshipEvent(rel, "disclosure", "Поделился личной информацией", 0.5)
	}

	// Применяем естественное затухание
	rel.Trust = clamp01(rel.Trust - b.config.TrustDecayRate)
	rel.Intimacy = clamp01(rel.Intimacy + b.config.IntimacyGrowthRate*0.1) // Медленный рост
}

func (b *Bot) addRelationshipEvent(rel *storage.Relationship, eventType, description string, impact float64) {
	event := storage.RelationshipEvent{
		Type:        eventType,
		Description: description,
		Impact:      impact,
		Timestamp:   time.Now(),
	}

	rel.KeyMoments = append(rel.KeyMoments, event)

	// Ограничиваем количество ключевых моментов
	if len(rel.KeyMoments) > 20 {
		rel.KeyMoments = rel.KeyMoments[len(rel.KeyMoments)-20:]
	}
}

func (b *Bot) addConflictMemory(rel *storage.Relationship, issue, details string) {
	conflict := storage.ConflictMemory{
		Issue:      issue,
		Resolution: "", // Пока не разрешён
		Learned:    "Нужно быть осторожнее в подобных ситуациях",
		Resolved:   false,
		Impact:     -0.3,
		Timestamp:  time.Now(),
	}

	rel.Conflicts = append(rel.Conflicts, conflict)

	// Ограничиваем количество конфликтов
	if len(rel.Conflicts) > 10 {
		rel.Conflicts = rel.Conflicts[len(rel.Conflicts)-10:]
	}
}

func (b *Bot) addSharedExperience(rel *storage.Relationship, experienceType, description string) {
	experience := storage.SharedMemory{
		Experience:   fmt.Sprintf("%s: %s", experienceType, description),
		Significance: 0.6,
		References:   1,
		Created:      time.Now(),
		LastMention:  time.Now(),
	}

	rel.SharedExperiences = append(rel.SharedExperiences, experience)

	// Ограничиваем количество опытов
	if len(rel.SharedExperiences) > 15 {
		rel.SharedExperiences = rel.SharedExperiences[len(rel.SharedExperiences)-15:]
	}
}

// GetRelationshipInfluencedCommunicationStyle возвращает стиль общения с учётом отношений
func (b *Bot) GetRelationshipInfluencedCommunicationStyle(chatID int64, userID int64) string {
	if !b.config.RelationshipTrackingEnabled {
		return "neutral"
	}

	mem, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil || mem == nil || mem.Relationships == nil {
		return "neutral"
	}

	key := fmt.Sprintf("%d", userID)
	rel, exists := mem.Relationships[key]
	if !exists {
		return "neutral"
	}

	// Определяем стиль на основе параметров отношений
	if rel.Intimacy > 0.7 && rel.Affection > 0.6 {
		return "warm_casual"
	}

	if rel.Respect > 0.8 && rel.Trust > 0.7 {
		return "respectful_professional"
	}

	if rel.Familiarity > 0.8 && len(rel.SharedExperiences) > 5 {
		return "familiar_friendly"
	}

	if rel.Trust < 0.3 || len(rel.Conflicts) > 3 {
		return "cautious_distant"
	}

	return "neutral"
}

// AnalyzeRelationshipForPrompt добавляет контекст отношений в промпт
func (b *Bot) AnalyzeRelationshipForPrompt(chatID int64, userID int64) string {
	if !b.config.RelationshipTrackingEnabled {
		return ""
	}

	mem, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil || mem == nil || mem.Relationships == nil {
		return ""
	}

	key := fmt.Sprintf("%d", userID)
	rel, exists := mem.Relationships[key]
	if !exists || rel.TotalInteractions < 3 {
		return ""
	}

	var context strings.Builder
	context.WriteString("Контекст отношений с пользователем:\n")

	// Описываем уровень близости
	if rel.Intimacy > 0.6 {
		context.WriteString("- Близкие отношения, можно быть более открытой\n")
	} else if rel.Intimacy < 0.3 {
		context.WriteString("- Дистантные отношения, стоит быть осторожной\n")
	}

	// Описываем уровень доверия
	if rel.Trust > 0.7 {
		context.WriteString("- Высокое доверие, пользователь ценит мои советы\n")
	} else if rel.Trust < 0.4 {
		context.WriteString("- Низкое доверие, нужно работать над восстановлением\n")
	}

	// Упоминаем недавние конфликты
	if len(rel.Conflicts) > 0 {
		recentConflict := rel.Conflicts[len(rel.Conflicts)-1]
		if time.Since(recentConflict.Timestamp) < 24*time.Hour {
			context.WriteString(fmt.Sprintf("- Недавний конфликт: %s\n", recentConflict.Issue))
		}
	}

	// Упоминаем общий опыт
	if len(rel.SharedExperiences) > 0 {
		context.WriteString("- Есть общие воспоминания и опыт\n")
	}

	return context.String()
}

// BuildStyleCue формирует краткую инструкцию для LLM на основе стиля общения
func BuildStyleCue(style string) string {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "warm_casual":
		return "Тон: тёплый, дружелюбный и разговорный; уместна лёгкая фамильярность, эмодзи — умеренно. [style=warm_casual]"
	case "respectful_professional":
		return "Тон: вежливый, профессиональный и уважительный; избегай сленга и эмодзи. [style=respectful_professional]"
	case "familiar_friendly":
		return "Тон: свойский, шутливый, на «ты»; допускается лёгкий сленг. [style=familiar_friendly]"
	case "cautious_distant":
		return "Тон: сдержанный, фактический, без фамильярности; избегай эмодзи. [style=cautious_distant]"
	default:
		return "Тон: нейтральный, дружелюбный. [style=neutral]"
	}
}

// ApplyRelationshipStyleToContext добавляет в начало контекста подсказку тона на основе отношений
func (b *Bot) ApplyRelationshipStyleToContext(chatID, userID int64, context string) string {
	// Если отслеживание отношений выключено — не меняем контекст
	if b == nil || b.config == nil || !b.config.RelationshipTrackingEnabled {
		return context
	}
	style := b.GetRelationshipInfluencedCommunicationStyle(chatID, userID)
	// Всегда формируем cue (в т.ч. для neutral) для детерминизма тестов
	cue := BuildStyleCue(style)
	if strings.TrimSpace(cue) == "" {
		return context
	}
	return "[tone_hint]: " + cue + "\n\n" + context
}
