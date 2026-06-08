package bot

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/storage"
)

// buildPersonalityContext создает контекст личности для встраивания в промпты
func (b *Bot) buildPersonalityContext(chatID int64, includeStatic bool, includeDynamic bool) string {
	memory, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil || memory == nil {
		// FALLBACK: Используем базовую личность
		if includeStatic {
			return b.getDefaultPersonalityFallback()
		}
		return ""
	}

	// Инициализируем только динамические поля, если они не заданы
	if includeStatic {
		b.initializeStaticPersonality(memory)
		b.storage.SavePersonalityMemory(memory)
	}

	var sb strings.Builder

	if includeStatic {
		// СТАТИЧЕСКАЯ ЛИЧНОСТЬ (ВЫСШИЙ ПРИОРИТЕТ)
		if memory.StaticPersonality != "" {
			sb.WriteString("=== ОСНОВНАЯ ЛИЧНОСТЬ ===\n")
			sb.WriteString(memory.StaticPersonality)
			sb.WriteString("\n\n")
		}

		// ИНСТРУКЦИИ ПОВЕДЕНИЯ (ВЫСШИЙ ПРИОРИТЕТ)
		if memory.StyleInstructions != "" {
			sb.WriteString("=== СТИЛЬ ОБЩЕНИЯ ===\n")
			sb.WriteString(memory.StyleInstructions)
			sb.WriteString("\n\n")
		}
	}

	if includeDynamic {
		// ДИНАМИЧЕСКАЯ ЛИЧНОСТЬ
		sb.WriteString("=== ТЕКУЩЕЕ СОСТОЯНИЕ ===\n")

		if len(memory.CurrentViews) > 0 {
			sb.WriteString("Текущие взгляды: ")
			sb.WriteString(strings.Join(memory.CurrentViews, ", "))
			sb.WriteString("\n")
		}

		if len(memory.TemporalTraits) > 0 {
			sb.WriteString("Временные черты характера: ")
			for trait, intensity := range memory.TemporalTraits {
				sb.WriteString(fmt.Sprintf("%s (%.1f), ", trait, intensity))
			}
			sb.WriteString("\n")
		}

		if len(memory.ContextualAdaptations) > 0 {
			sb.WriteString("Адаптации к контексту: ")
			sb.WriteString(strings.Join(memory.ContextualAdaptations, ", "))
			sb.WriteString("\n")
		}

		// Существующая динамическая личность
		if len(memory.SelfPerception) > 0 {
			sb.WriteString("Самовосприятие: ")
			sb.WriteString(strings.Join(memory.SelfPerception, ", "))
			sb.WriteString("\n")
		}

		if len(memory.RecentTopics) > 0 {
			sb.WriteString("Недавние темы: ")
			sb.WriteString(strings.Join(memory.RecentTopics, ", "))
			sb.WriteString("\n")
		}

		// === ЭМОЦИОНАЛЬНОЕ СОСТОЯНИЕ (Этап 2) ===
		if memory.EmotionalState != nil {
			sb.WriteString("Эмоциональное состояние: ")
			sb.WriteString(fmt.Sprintf("интенсивность %.2f, стабильность %.2f",
				memory.EmotionalState.Intensity, memory.EmotionalState.Stability))

			// Добавляем доминирующие эмоции
			if memory.EmotionalState.Joy > 0.6 {
				sb.WriteString(", доминирует радость")
			} else if memory.EmotionalState.Sadness > 0.6 {
				sb.WriteString(", доминирует грусть")
			} else if memory.EmotionalState.Anger > 0.6 {
				sb.WriteString(", доминирует гнев")
			} else if memory.EmotionalState.Cynicism > 0.6 {
				sb.WriteString(", доминирует цинизм")
			} else if memory.EmotionalState.Empathy > 0.6 {
				sb.WriteString(", доминирует эмпатия")
			} else if memory.EmotionalState.Irritability > 0.6 {
				sb.WriteString(", повышенная раздражительность")
			}
			sb.WriteString("\n")
		}

		// === КОГНИТИВНАЯ АРХИТЕКТУРА (Этап 4) ===
		if memory.MetaCognition != nil && b.config.SelfReflectionEnabled {
			sb.WriteString(fmt.Sprintf("Метакогнитивное состояние: уверенность %.2f, самоосознанность %.2f, адаптивность %.2f",
				memory.MetaCognition.ConfidenceLevel,
				memory.MetaCognition.SelfAwareness,
				memory.MetaCognition.AdaptabilityScore))
			sb.WriteString("\n")
		}

		// === СИСТЕМА УБЕЖДЕНИЙ (Этап 1) ===
		if memory.BeliefSystem != nil && b.config.BeliefLearningEnabled {
			if len(memory.BeliefSystem.CoreBeliefs) > 0 {
				// Показываем только самые сильные убеждения (топ-5)
				strongBeliefs := make([]string, 0)
				for topic, belief := range memory.BeliefSystem.CoreBeliefs {
					if belief.Strength > 0.6 && belief.Confidence > 0.5 {
						strengthDesc := "слабо"
						if belief.Strength > 0.8 {
							strengthDesc = "сильно"
						} else if belief.Strength > 0.7 {
							strengthDesc = "умеренно"
						}
						strongBeliefs = append(strongBeliefs, fmt.Sprintf("%s (%s)", topic, strengthDesc))
					}
				}

				if len(strongBeliefs) > 0 {
					if len(strongBeliefs) > 5 {
						strongBeliefs = strongBeliefs[:5] // Ограничиваем до 5 убеждений
					}
					sb.WriteString("Убеждения: ")
					sb.WriteString(strings.Join(strongBeliefs, ", "))
					sb.WriteString("\n")
				}
			}

			// Показываем недавние конфликты убеждений, если есть
			if len(memory.BeliefSystem.BeliefConflicts) > 0 {
				recentConflicts := 0
				for _, conflict := range memory.BeliefSystem.BeliefConflicts {
					if !conflict.Resolved && conflict.Severity > 0.5 {
						recentConflicts++
					}
				}
				if recentConflicts > 0 {
					sb.WriteString(fmt.Sprintf("Внутренние противоречия: %d неразрешенных конфликтов", recentConflicts))
					sb.WriteString("\n")
				}
			}
		}

		// Добавляем недавние внутренние мысли
		if len(memory.InternalThoughts) > 0 && b.config.InternalMonologueEnabled {
			recentThoughts := memory.InternalThoughts
			if len(recentThoughts) > 3 {
				recentThoughts = recentThoughts[len(recentThoughts)-3:] // Последние 3 мысли
			}

			thoughtSummary := make([]string, len(recentThoughts))
			for i, thought := range recentThoughts {
				thoughtLength := min(len(thought.Content), 50)
				thoughtSummary[i] = fmt.Sprintf("[%s] %s", thought.Type, thought.Content[:thoughtLength])
			}

			sb.WriteString("Недавние внутренние мысли: ")
			sb.WriteString(strings.Join(thoughtSummary, "; "))
			sb.WriteString("\n")
		}

		// === СОЦИАЛЬНАЯ АРХИТЕКТУРА (Этап 4) ===
		if len(memory.Relationships) > 0 && b.config.RelationshipTrackingEnabled {
			// Находим наиболее значимые отношения
			significantRels := 0
			for _, rel := range memory.Relationships {
				if rel.TotalInteractions > 10 || rel.Intimacy > 0.5 || rel.Trust > 0.7 {
					significantRels++
				}
			}

			if significantRels > 0 {
				sb.WriteString(fmt.Sprintf("Значимые отношения: %d пользователей", significantRels))

				// Добавляем пример стиля общения
				for _, rel := range memory.Relationships {
					if rel.TotalInteractions > 20 {
						sb.WriteString(fmt.Sprintf(" (пользователь %d: %s стиль, близость %.2f)",
							rel.UserID, rel.CommunicationStyle, rel.Intimacy))
						break // Показываем только один пример
					}
				}
				sb.WriteString("\n")
			}
		}

		if len(memory.EmotionalMemories) > 0 {
			// Показываем только последние 3 эмоциональные воспоминания
			recentCount := len(memory.EmotionalMemories)
			if recentCount > 3 {
				recentCount = 3
			}

			sb.WriteString("Недавние эмоциональные воспоминания: ")
			for i := 0; i < recentCount; i++ {
				emMem := memory.EmotionalMemories[i]
				age := time.Since(emMem.CreatedAt)
				if age < 24*time.Hour {
					sb.WriteString(fmt.Sprintf("%s от %s (%.1f)",
						emMem.PrimaryEmotion, emMem.UserContext, emMem.EmotionIntensity))
					if i < recentCount-1 {
						sb.WriteString(", ")
					}
				}
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// enrichPromptWithPersonality встраивает личность в промпт
func (b *Bot) enrichPromptWithPersonality(basePrompt string, chatID int64, promptType string) string {
	log.Printf("[PersonalityContext] enrichPromptWithPersonality: Запрос для чата %d, тип: %s", chatID, promptType)
	log.Printf("[PersonalityContext] enrichPromptWithPersonality: ИСХОДНЫЙ ПРОМПТ (длина %d):\n%s", len(basePrompt), basePrompt)

	// ИСПРАВЛЕНО: Не обогащаем промпты анализа профилей пользователей личностью бота
	// Это предотвращает смешивание личности бота с AutoBio пользователей
	if promptType == "autobio_initial" || promptType == "autobio_update" {
		log.Printf("[PersonalityContext] enrichPromptWithPersonality: Тип '%s' исключен из обогащения личностью - возвращаем исходный промпт", promptType)
		return basePrompt
	}

	// Определяем, какие элементы личности включать для разных типов промптов
	includeStatic := true  // Всегда включаем статическую личность
	includeDynamic := true // Всегда включаем динамическую личность

	// Для некоторых промптов можем ограничить динамическую часть
	switch promptType {
	case "summary":
		includeDynamic = false // Для саммари используем только статическую личность
		log.Printf("[PersonalityContext] enrichPromptWithPersonality: Для типа '%s' отключена динамическая личность", promptType)
	case "daily_take":
		// Включаем все для генерации темы дня
		log.Printf("[PersonalityContext] enrichPromptWithPersonality: Для типа '%s' включены все элементы личности", promptType)
	default:
		log.Printf("[PersonalityContext] enrichPromptWithPersonality: Для типа '%s' включены все элементы личности (default)", promptType)
	}

	personalityContext := b.buildPersonalityContext(chatID, includeStatic, includeDynamic)
	log.Printf("[PersonalityContext] enrichPromptWithPersonality: PERSONALITY_CONTEXT (длина %d):\n%s", len(personalityContext), personalityContext)

	// Получаем style instructions из памяти личности
	memory, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil {
		log.Printf("[PersonalityContext] enrichPromptWithPersonality: Ошибка получения памяти личности: %v", err)
		memory = &storage.PersonalityMemory{}
		b.initializeStaticPersonality(memory)
	}

	styleInstructions := memory.StyleInstructions
	log.Printf("[PersonalityContext] enrichPromptWithPersonality: STYLE_INSTRUCTIONS (длина %d):\n%s", len(styleInstructions), styleInstructions)

	// Проверяем наличие плейсхолдеров
	hasPersonalityPlaceholder := strings.Contains(basePrompt, "{PERSONALITY_CONTEXT}")
	hasStylePlaceholder := strings.Contains(basePrompt, "{STYLE_INSTRUCTIONS}")

	if !hasPersonalityPlaceholder && !hasStylePlaceholder {
		log.Printf("[PersonalityContext] enrichPromptWithPersonality: НИ ОДИН ПЛЕЙСХОЛДЕР НЕ НАЙДЕН в промпте!")
		return basePrompt
	}

	enrichedPrompt := basePrompt

	// Встраиваем личность
	if hasPersonalityPlaceholder {
		enrichedPrompt = strings.ReplaceAll(enrichedPrompt, "{PERSONALITY_CONTEXT}", personalityContext)
		log.Printf("[PersonalityContext] enrichPromptWithPersonality: Заменен {PERSONALITY_CONTEXT}")
	}

	// Встраиваем style instructions
	if hasStylePlaceholder {
		enrichedPrompt = strings.ReplaceAll(enrichedPrompt, "{STYLE_INSTRUCTIONS}", styleInstructions)
		log.Printf("[PersonalityContext] enrichPromptWithPersonality: Заменен {STYLE_INSTRUCTIONS}")
	}

	log.Printf("[PersonalityContext] enrichPromptWithPersonality: ИТОГОВЫЙ ПРОМПТ (длина %d):\n%s", len(enrichedPrompt), enrichedPrompt)

	return enrichedPrompt
}

// getDefaultPersonalityFallback возвращает базовую личность для fallback
func (b *Bot) getDefaultPersonalityFallback() string {
	return ""
}

// initializeStaticPersonality инициализирует статическую личность, если она отсутствует
func (b *Bot) initializeStaticPersonality(memory *storage.PersonalityMemory) {
	// Поля StaticPersonality и StyleInstructions остаются пустыми по умолчанию
	// Они должны быть заполнены через админ-команды или другие методы

	// Если NameMentions пустые, инициализируем
	if len(memory.NameMentions) == 0 {
		memory.NameMentions = map[string]bool{}
	}

	if memory.PersonalityVersion == 0 {
		memory.PersonalityVersion = 1
	}
	memory.LastManualUpdate = time.Now()

	// Инициализируем динамические поля если пустые
	if memory.NameMentions == nil {
		memory.NameMentions = map[string]bool{}
	}
	if memory.RecentTopics == nil {
		memory.RecentTopics = []string{}
	}
	if memory.SelfPerception == nil {
		memory.SelfPerception = []string{}
	}
	if memory.DiscussionContext == nil {
		memory.DiscussionContext = map[string]bool{}
	}
	if memory.CurrentViews == nil {
		memory.CurrentViews = []string{}
	}
	if memory.TemporalTraits == nil {
		memory.TemporalTraits = map[string]float64{}
	}
	if memory.ContextualAdaptations == nil {
		memory.ContextualAdaptations = []string{}
	}
}

// limitStringSlice ограничивает размер слайса строк
func limitStringSlice(slice []string, maxSize int) []string {
	if len(slice) <= maxSize {
		return slice
	}
	return slice[len(slice)-maxSize:]
}
