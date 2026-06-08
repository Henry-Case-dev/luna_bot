package bot

import (
	"log"
	"strings"
)

// getEmotionallyInfluencedResponseType возвращает responseType с учётом текущих эмоций
func (b *Bot) getEmotionallyInfluencedResponseType(chatID int64, baseResponseType string) string {
	if !b.config.EmotionalLearningEnabled {
		return baseResponseType
	}

	emotionalState, err := b.storage.GetEmotionalState(chatID)
	if err != nil || emotionalState == nil {
		return baseResponseType
	}

	// Анализируем ResponseTendency из эмоционального состояния
	if emotionalState.ResponseTendency != nil {
		// Если текущая эмоция указывает на серьёзность
		if seriousness, exists := emotionalState.ResponseTendency["seriousness"]; exists && seriousness > 0.6 {
			return "direct_serious"
		}

		// Если высокий уровень сарказма и это не серьёзный контекст
		if sarcasm, exists := emotionalState.ResponseTendency["sarcasm"]; exists && sarcasm > 0.7 {
			if baseResponseType != "direct_serious" {
				return "direct" // Саркастичный, но не голосовой
			}
		}

		// Если высокая поддержка пользователя
		if support, exists := emotionalState.ResponseTendency["support"]; exists && support > 0.8 {
			return "direct_serious" // Поддерживающий и серьёзный тон
		}
	}

	// Анализируем базовые эмоции для влияния на тип ответа
	if emotionalState.Anger > 0.7 || emotionalState.Contempt > 0.6 {
		return "direct" // Резкий, но не голосовой при гневе
	}

	if emotionalState.Joy > 0.8 || emotionalState.Optimism > 0.7 {
		// Радостное состояние может привести к голосовому ответу
		if baseResponseType == "free_will" {
			return "voice"
		}
	}

	return baseResponseType
}

// updateEmotionalResponseTendency обновляет склонности к определённым типам ответов
func (b *Bot) updateEmotionalResponseTendency(chatID int64, responseType string, success bool) {
	if !b.config.EmotionalLearningEnabled {
		return
	}

	emotionalState, err := b.storage.GetEmotionalState(chatID)
	if err != nil || emotionalState == nil {
		return
	}

	if emotionalState.ResponseTendency == nil {
		emotionalState.ResponseTendency = make(map[string]float64)
	}

	// Обновляем склонности на основе успешности
	tendencyKey := b.mapResponseTypeToTendency(responseType)
	if tendencyKey != "" {
		currentValue := emotionalState.ResponseTendency[tendencyKey]

		if success {
			// Увеличиваем склонность к успешному типу ответа
			emotionalState.ResponseTendency[tendencyKey] = clamp01(currentValue + 0.1)
		} else {
			// Уменьшаем склонность к неуспешному типу
			emotionalState.ResponseTendency[tendencyKey] = clamp01(currentValue - 0.05)
		}

		// Сохраняем изменения
		if err := b.storage.SaveEmotionalState(emotionalState); err != nil {
			log.Printf("[EmotionalResponse] Ошибка сохранения склонностей для чата %d: %v", chatID, err)
		}
	}
}

func (b *Bot) mapResponseTypeToTendency(responseType string) string {
	switch responseType {
	case "direct":
		return "directness"
	case "direct_serious":
		return "seriousness"
	case "voice":
		return "expressiveness"
	case "free_will":
		return "spontaneity"
	default:
		return ""
	}
}

// adaptMessagePostProcessorToEmotion адаптирует постпроцессор к эмоциональному состоянию
func (b *Bot) adaptMessagePostProcessorToEmotion(chatID int64, originalText string) string {
	if !b.config.EmotionalLearningEnabled || b.messagePostProcessor == nil {
		return originalText
	}

	emotionalState, err := b.storage.GetEmotionalState(chatID)
	if err != nil || emotionalState == nil {
		return originalText
	}

	// Определяем эмоциональные модификации
	var modifications []string

	if emotionalState.Anger > 0.6 {
		modifications = append(modifications, "add_edge")
	}

	if emotionalState.Joy > 0.7 {
		modifications = append(modifications, "add_warmth")
	}

	if emotionalState.Sadness > 0.6 {
		modifications = append(modifications, "add_melancholy")
	}

	if emotionalState.Anxiety > 0.7 {
		modifications = append(modifications, "add_uncertainty")
	}

	// Применяем эмоциональные модификации (через расширенный постпроцессор)
	modifiedText := originalText
	for _, mod := range modifications {
		modifiedText = b.applyEmotionalModification(modifiedText, mod)
	}

	return modifiedText
}

func (b *Bot) applyEmotionalModification(text, modification string) string {
	switch modification {
	case "add_edge":
		// Добавляем резкость
		if !strings.HasSuffix(text, ".") {
			text += "."
		}
		return text

	case "add_warmth":
		// Добавляем теплоту (но без эмодзи, согласно стилю)
		return text

	case "add_melancholy":
		// Добавляем меланхолию
		return text

	case "add_uncertainty":
		// Добавляем неуверенность
		if strings.HasSuffix(text, ".") {
			text = strings.TrimSuffix(text, ".")
			text += "..."
		}
		return text

	default:
		return text
	}
}
