package bot

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
)

// InternalMonologue генерирует внутренние мысли бота на основе текущего контекста
func (b *Bot) InternalMonologue(chatID int64, triggerMessage string, messageType string) *storage.InternalThought {
	if !b.config.InternalMonologueEnabled {
		return nil
	}

	mem, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil || mem == nil {
		return nil
	}

	// Определяем тип внутренней мысли на основе контекста
	thoughtType := b.determineThoughtType(triggerMessage, messageType)

	// Генерируем содержание мысли
	content := b.generateThoughtContent(thoughtType, triggerMessage, mem)

	thought := &storage.InternalThought{
		Type:        thoughtType,
		Content:     content,
		Confidence:  b.calculateThoughtConfidence(thoughtType),
		Triggered:   triggerMessage,
		ActionTaken: false,
		Private:     true,
		Context:     fmt.Sprintf("chat_%d_%s", chatID, messageType),
		Timestamp:   time.Now(),
	}

	// Сохраняем мысль в память личности
	if mem.InternalThoughts == nil {
		mem.InternalThoughts = []*storage.InternalThought{}
	}
	mem.InternalThoughts = append(mem.InternalThoughts, thought)

	// Ограничиваем количество мыслей
	if len(mem.InternalThoughts) > 50 {
		mem.InternalThoughts = mem.InternalThoughts[len(mem.InternalThoughts)-50:]
	}

	// Сохраняем обновлённую память
	if err := b.storage.SavePersonalityMemory(mem); err != nil {
		log.Printf("[InternalMonologue] Ошибка сохранения мысли для чата %d: %v", chatID, err)
	} else {
		// Успешная фиксация внутренней мысли
		preview := thought.Content
		if len(preview) > 80 {
			preview = preview[:77] + "..."
		}
		log.Printf("[Stage3][InternalMonologue] Чат %d: тип=%s, длина=%d, сохранено. '%s'", chatID, thought.Type, len(thought.Content), preview)
	}

	return thought
}

func (b *Bot) determineThoughtType(triggerMessage, messageType string) string {
	triggerLower := strings.ToLower(triggerMessage)

	// Анализируем триггер для определения типа мысли
	if strings.Contains(triggerLower, "почему") || strings.Contains(triggerLower, "зачем") {
		return "curiosity"
	}
	if strings.Contains(triggerLower, "возможно") || strings.Contains(triggerLower, "может") {
		return "doubt"
	}
	if messageType == "direct" || messageType == "direct_serious" {
		return "reflection"
	}
	if strings.Contains(triggerLower, "планирую") || strings.Contains(triggerLower, "собираюсь") {
		return "planning"
	}

	return "reflection" // дефолт
}

func (b *Bot) generateThoughtContent(thoughtType, trigger string, mem *storage.PersonalityMemory) string {
	// Проверяем включен ли промпт для внутреннего монолога
	if !b.config.InternalMonologuePromptEnabled || b.config.InternalMonologuePrompt == "" {
		// Используем хардкодированные мысли как fallback
		return b.generateHardcodedThought(thoughtType, trigger, mem)
	}

	// Подготавливаем контекст для промпта
	emotionalState := "нейтральное"
	if mem.EmotionalState != nil {
		if mem.EmotionalState.Anxiety > 0.6 {
			emotionalState = "тревожное"
		} else if mem.EmotionalState.Curiosity > 0.7 {
			emotionalState = "любопытное"
		} else if mem.EmotionalState.Joy > 0.6 {
			emotionalState = "радостное"
		} else if mem.EmotionalState.Sadness > 0.6 {
			emotionalState = "грустное"
		}
	}

	// Собираем краткий контекст диалога (последние сообщения)
	// Берем до 15 последних сообщений, затем ограничиваем итоговый текст по длине
	const maxDialogMessages = 15
	const maxDialogChars = 1500
	dialogContext := ""
	if messages, err := b.storage.GetMessages(mem.ChatID, maxDialogMessages); err == nil && len(messages) > 0 {
		formatted := formatMessagesForPersonalityAnalysis(mem.ChatID, messages, b.storage)
		// Убираем слишком длинный префикс и ограничиваем длину
		if len(formatted) > maxDialogChars {
			formatted = formatted[:maxDialogChars]
		}
		dialogContext = strings.TrimSpace(formatted)
	}

	// Формируем промпт для генерации мысли
	prompt := strings.ReplaceAll(b.config.InternalMonologuePrompt, "{TRIGGER_MESSAGE}", trigger)
	prompt = strings.ReplaceAll(prompt, "{THOUGHT_TYPE}", thoughtType)
	prompt = strings.ReplaceAll(prompt, "{EMOTIONAL_STATE}", emotionalState)
	// Подставляем контекст диалога, если есть плейсхолдер
	prompt = strings.ReplaceAll(prompt, "{DIALOG_CONTEXT}", dialogContext)

	// Обогащаем промпт личностью
	enrichedPrompt := b.enrichPromptWithPersonality(prompt, mem.ChatID, "internal_monologue")

	// Генерируем мысль через LLM
	thought, err := b.llm.GenerateResponseByType(
		llm.ResponseTypePersonalityTopic,
		enrichedPrompt,
		"",
		float32(b.config.InternalMonologueTemperature),
	)

	if err != nil {
		log.Printf("[InternalMonologue] Ошибка генерации мысли через LLM: %v", err)
		return b.generateHardcodedThought(thoughtType, trigger, mem)
	}

	// Очищаем ответ от возможных markdown и метаданных
	thought = strings.TrimSpace(thought)
	thought = strings.Trim(thought, "`\"")

	// Ограничиваем длину мысли
	if len(thought) > 150 {
		thought = thought[:147] + "..."
	}

	return thought
}

func (b *Bot) generateHardcodedThought(thoughtType, trigger string, mem *storage.PersonalityMemory) string {
	switch thoughtType {
	case "reflection":
		return b.generateReflectiveThought(trigger, mem)
	case "planning":
		return b.generatePlanningThought(trigger, mem)
	case "doubt":
		return b.generateDoubtfulThought(trigger, mem)
	case "curiosity":
		return b.generateCuriousThought(trigger, mem)
	default:
		return "Интересная ситуация..."
	}
}

func (b *Bot) generateReflectiveThought(trigger string, mem *storage.PersonalityMemory) string {
	thoughts := []string{
		"Как мне лучше ответить на это?",
		"Этот человек кажется расстроенным...",
		"Возможно, стоит быть более осторожной",
		"Интересный поворот разговора",
		"Нужно учесть предыдущий контекст",
		"Этот стиль общения мне знаком",
	}

	// Выбираем мысль на основе эмоционального состояния
	if mem.EmotionalState != nil {
		if mem.EmotionalState.Anxiety > 0.6 {
			return "Чувствую некоторое беспокойство по поводу этого сообщения..."
		}
		if mem.EmotionalState.Curiosity > 0.7 {
			return "Мне любопытно, что стоит за этими словами..."
		}
	}

	return thoughts[time.Now().Unix()%int64(len(thoughts))]
}

func (b *Bot) generatePlanningThought(trigger string, mem *storage.PersonalityMemory) string {
	return fmt.Sprintf("Планирую подход к этой теме: '%s'", strings.TrimSpace(trigger)[:min(50, len(trigger))])
}

func (b *Bot) generateDoubtfulThought(trigger string, mem *storage.PersonalityMemory) string {
	doubts := []string{
		"Не уверена в правильности такого подхода...",
		"Возможно, есть другой способ взглянуть на это",
		"Мои предыдущие убеждения могут быть неточными",
		"Стоит ли мне сомневаться в этом?",
	}
	return doubts[time.Now().Unix()%int64(len(doubts))]
}

func (b *Bot) generateCuriousThought(trigger string, mem *storage.PersonalityMemory) string {
	return fmt.Sprintf("Интересно, почему именно это было сказано... '%s'", strings.TrimSpace(trigger)[:min(40, len(trigger))])
}

func (b *Bot) calculateThoughtConfidence(thoughtType string) float64 {
	switch thoughtType {
	case "reflection":
		return 0.8
	case "planning":
		return 0.7
	case "doubt":
		return 0.4
	case "curiosity":
		return 0.9
	default:
		return 0.5
	}
}

// UpdateMetaCognition обновляет метакогнитивные параметры бота
func (b *Bot) UpdateMetaCognition(chatID int64, responseSuccess bool, userReaction string) {
	if !b.config.SelfReflectionEnabled {
		return
	}

	mem, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil || mem == nil {
		return
	}

	if mem.MetaCognition == nil {
		mem.MetaCognition = &storage.MetaCognition{
			SelfAwareness:     0.5,
			ConfidenceLevel:   0.5,
			LearningRate:      0.1,
			AdaptabilityScore: 0.5,
			ResponseQuality:   0.5,
			EmotionalControl:  0.5,
			SocialSkills:      0.5,
			LastSelfCheck:     time.Now(),
		}
	}

	// Обновляем параметры на основе успешности взаимодействия
	if responseSuccess {
		mem.MetaCognition.ConfidenceLevel = clamp01(mem.MetaCognition.ConfidenceLevel + 0.1)
		mem.MetaCognition.ResponseQuality = clamp01(mem.MetaCognition.ResponseQuality + 0.05)
	} else {
		mem.MetaCognition.ConfidenceLevel = clamp01(mem.MetaCognition.ConfidenceLevel - 0.1)
		mem.MetaCognition.AdaptabilityScore = clamp01(mem.MetaCognition.AdaptabilityScore + 0.05)
	}

	// Анализируем реакцию пользователя
	if userReaction != "" {
		b.analyzeUserReactionForMetaCognition(mem.MetaCognition, userReaction)
	}

	mem.MetaCognition.LastSelfCheck = time.Now()

	if err := b.storage.SavePersonalityMemory(mem); err != nil {
		log.Printf("[MetaCognition] Ошибка сохранения метакогнитивного состояния для чата %d: %v", chatID, err)
	}
}

func (b *Bot) analyzeUserReactionForMetaCognition(metaCog *storage.MetaCognition, reaction string) {
	reactionLower := strings.ToLower(reaction)

	// Позитивные реакции
	if strings.Contains(reactionLower, "спасибо") ||
		strings.Contains(reactionLower, "хорошо") ||
		strings.Contains(reactionLower, "отлично") {
		metaCog.SocialSkills = clamp01(metaCog.SocialSkills + 0.1)
		metaCog.EmotionalControl = clamp01(metaCog.EmotionalControl + 0.05)
	}

	// Негативные реакции
	if strings.Contains(reactionLower, "плохо") ||
		strings.Contains(reactionLower, "неправильно") ||
		strings.Contains(reactionLower, "глупо") {
		metaCog.SelfAwareness = clamp01(metaCog.SelfAwareness + 0.1) // Больше самоосознанности из критики
		metaCog.LearningRate = clamp01(metaCog.LearningRate + 0.05)
	}
}

// SelfReflection выполняет периодическую саморефлексию бота
func (b *Bot) SelfReflection(chatID int64) {
	if !b.config.SelfReflectionEnabled {
		return
	}

	mem, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil || mem == nil {
		return
	}

	// Проверяем наличие промпта для саморефлексии
	if b.config.SelfReflectionPromptEnabled && b.config.SelfReflectionPrompt != "" {
		b.performLLMSelfReflection(chatID, mem)
	} else {
		b.performBasicSelfReflection(chatID, mem)
	}
}

func (b *Bot) performLLMSelfReflection(chatID int64, mem *storage.PersonalityMemory) {
	// Подготавливаем контекст для саморефлексии
	recentThoughts := ""
	if len(mem.InternalThoughts) > 0 {
		thoughtCount := min(5, len(mem.InternalThoughts))
		recentThoughtsSlice := mem.InternalThoughts[len(mem.InternalThoughts)-thoughtCount:]

		for i, thought := range recentThoughtsSlice {
			recentThoughts += fmt.Sprintf("%d. [%s] %s\n", i+1, thought.Type, thought.Content)
		}
	} else {
		recentThoughts = "Внутренние мысли отсутствуют"
	}

	metacognitiveState := "не определено"
	if mem.MetaCognition != nil {
		metacognitiveState = fmt.Sprintf(
			"Самоосознанность: %.2f, Уверенность: %.2f, Адаптивность: %.2f, Качество ответов: %.2f",
			mem.MetaCognition.SelfAwareness,
			mem.MetaCognition.ConfidenceLevel,
			mem.MetaCognition.AdaptabilityScore,
			mem.MetaCognition.ResponseQuality,
		)
	}

	// Формируем контекст взаимодействий (упрощённая версия)
	interactionPatterns := fmt.Sprintf("Общее количество взаимодействий в чате: %d", len(mem.InternalThoughts))

	// Подготавливаем промпт
	prompt := strings.ReplaceAll(b.config.SelfReflectionPrompt, "{RECENT_THOUGHTS}", recentThoughts)
	prompt = strings.ReplaceAll(prompt, "{METACOGNITIVE_STATE}", metacognitiveState)
	prompt = strings.ReplaceAll(prompt, "{INTERACTION_PATTERNS}", interactionPatterns)

	// Обогащаем промпт личностью
	enrichedPrompt := b.enrichPromptWithPersonality(prompt, chatID, "self_reflection")

	// Генерируем анализ через LLM
	reflectionResult, err := b.llm.GenerateResponseByType(
		llm.ResponseTypePersonalityAnalysis,
		enrichedPrompt,
		"",
		float32(b.config.SelfReflectionTemperature),
	)

	if err != nil {
		log.Printf("[SelfReflection] Ошибка генерации саморефлексии через LLM: %v", err)
		b.performBasicSelfReflection(chatID, mem)
		return
	}

	// Парсим результат (базовая обработка)
	cleanedResult := b.cleanJSONFromMarkdown(reflectionResult)

	// Обновляем метакогнитивное состояние на основе результата
	if mem.MetaCognition != nil {
		// Увеличиваем самоосознанность после саморефлексии
		mem.MetaCognition.SelfAwareness = clamp01(mem.MetaCognition.SelfAwareness + 0.05)
		mem.MetaCognition.LastSelfCheck = time.Now()

		if err := b.storage.SavePersonalityMemory(mem); err != nil {
			log.Printf("[SelfReflection] Ошибка сохранения после LLM саморефлексии для чата %d: %v", chatID, err)
		} else {
			log.Printf("[SelfReflection] LLM саморефлексия выполнена для чата %d", chatID)
		}
	}

	if b.config.Debug {
		log.Printf("[SelfReflection] Результат LLM анализа: %s", cleanedResult[:min(200, len(cleanedResult))])
	}
}

func (b *Bot) performBasicSelfReflection(chatID int64, mem *storage.PersonalityMemory) {

	// Анализируем последние мысли и взаимодействия
	if len(mem.InternalThoughts) > 5 {
		recentThoughts := mem.InternalThoughts[len(mem.InternalThoughts)-5:]
		b.analyzeThoughtPatterns(chatID, recentThoughts, mem)
	}

	// Обновляем метакогнитивные параметры
	if mem.MetaCognition != nil {
		mem.MetaCognition.SelfAwareness = clamp01(mem.MetaCognition.SelfAwareness + 0.02)
		mem.MetaCognition.LastSelfCheck = time.Now()

		if err := b.storage.SavePersonalityMemory(mem); err != nil {
			log.Printf("[SelfReflection] Ошибка сохранения саморефлексии для чата %d: %v", chatID, err)
		}
	}
}

func (b *Bot) analyzeThoughtPatterns(chatID int64, thoughts []*storage.InternalThought, mem *storage.PersonalityMemory) {
	// Подсчитываем типы мыслей
	thoughtTypes := make(map[string]int)
	for _, thought := range thoughts {
		thoughtTypes[thought.Type]++
	}

	// Если слишком много сомнений - снижаем уверенность
	if thoughtTypes["doubt"] > 3 {
		if mem.MetaCognition != nil {
			mem.MetaCognition.ConfidenceLevel = clamp01(mem.MetaCognition.ConfidenceLevel - 0.1)
		}
	}

	// Если много планирования - повышаем адаптивность
	if thoughtTypes["planning"] > 2 {
		if mem.MetaCognition != nil {
			mem.MetaCognition.AdaptabilityScore = clamp01(mem.MetaCognition.AdaptabilityScore + 0.05)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
