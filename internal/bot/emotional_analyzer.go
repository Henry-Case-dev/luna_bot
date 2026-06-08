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

// EmotionalAnalysisResult структура для результата эмоционального анализа
type EmotionalAnalysisResult struct {
	PrimaryEmotion     string             `json:"primary_emotion"`     // Основная эмоция
	EmotionIntensity   float64            `json:"emotion_intensity"`   // Интенсивность (0.0-1.0)
	EmotionScores      map[string]float64 `json:"emotion_scores"`      // Баллы по всем эмоциям
	Trigger            string             `json:"trigger"`             // Что вызвало эмоцию
	Context            string             `json:"context"`             // Контекст ситуации
	Confidence         float64            `json:"confidence"`          // Уверенность в анализе
	ResponseSuggestion string             `json:"response_suggestion"` // Предложение как реагировать
}

// EmotionalAdaptationResult структура для результата анализа эмоциональной адаптации
type EmotionalAdaptationResult struct {
	ToneAdjustment       string   `json:"tone_adjustment"`       // Настройка тона
	StyleChanges         []string `json:"style_changes"`         // Изменения в стиле
	AvoidTopics          []string `json:"avoid_topics"`          // Темы для избегания
	RecommendedApproach  string   `json:"recommended_approach"`  // Рекомендуемый подход
	EmpathyLevel         float64  `json:"empathy_level"`         // Уровень эмпатии
	HumorAppropriateness float64  `json:"humor_appropriateness"` // Уместность юмора
}

// EmotionalFeedbackResult структура для результата анализа эмоциональной обратной связи
type EmotionalFeedbackResult struct {
	ReactionType           string   `json:"reaction_type"`           // Тип реакции
	EmotionalChange        string   `json:"emotional_change"`        // Изменение эмоций
	ApproachEffectiveness  float64  `json:"approach_effectiveness"`  // Эффективность подхода
	SuccessfulElements     []string `json:"successful_elements"`     // Удачные элементы
	ProblematicElements    []string `json:"problematic_elements"`    // Проблемные элементы
	LearningInsights       []string `json:"learning_insights"`       // Выводы для обучения
	RecommendedAdjustments []string `json:"recommended_adjustments"` // Рекомендуемые корректировки
}

// startEmotionalAnalyzer запускает фоновую задачу для анализа эмоций
func (b *Bot) startEmotionalAnalyzer() {
	if !b.config.EmotionalLearningEnabled {
		log.Printf("[EmotionalAnalyzer] Эмоциональное обучение отключено в конфигурации")
		return
	}

	log.Printf("[EmotionalAnalyzer] Запуск анализатора эмоций...")

	// Запускаем первый анализ через 3 минуты после старта
	initialDelay := 3 * time.Minute
	go func() {
		time.Sleep(initialDelay)
		select {
		case <-b.stop:
			return
		default:
			log.Println("[EmotionalAnalyzer] Запуск первичного анализа эмоций...")
			b.analyzeEmotionsForAllChats()
		}
	}()

	// Запускаем периодический анализ эмоций
	analysisInterval := time.Duration(b.config.EmotionalAnalysisIntervalHours) * time.Hour
	if analysisInterval <= 0 {
		analysisInterval = 2 * time.Hour // По умолчанию каждые 2 часа
	}

	ticker := time.NewTicker(analysisInterval)
	go func() {
		defer ticker.Stop()
		log.Printf("[EmotionalAnalyzer] Запущен с интервалом %v", analysisInterval)

		for {
			select {
			case <-ticker.C:
				log.Println("[EmotionalAnalyzer] Выполняю запланированный анализ эмоций...")
				b.analyzeEmotionsForAllChats()
			case <-b.stop:
				log.Println("[EmotionalAnalyzer] Остановка анализатора эмоций...")
				return
			}
		}
	}()
}

// analyzeEmotionsForAllChats анализирует эмоции для всех активных чатов
func (b *Bot) analyzeEmotionsForAllChats() {
	chatIDs, err := b.storage.GetAllChatIDs()
	if err != nil {
		log.Printf("[EmotionalAnalyzer ERROR] Не удалось получить список чатов: %v", err)
		return
	}

	for _, chatID := range chatIDs {
		select {
		case <-b.stop:
			return
		default:
			err := b.analyzeEmotionsForChat(chatID)
			if err != nil {
				log.Printf("[EmotionalAnalyzer ERROR] Ошибка анализа эмоций для чата %d: %v", chatID, err)
			}
		}
	}
}

// analyzeEmotionsForChat анализирует эмоции для конкретного чата
func (b *Bot) analyzeEmotionsForChat(chatID int64) error {
	// Получаем последние сообщения для анализа
	lookbackMessages := b.config.EmotionalAnalysisLookbackMessages
	if lookbackMessages == 0 {
		lookbackMessages = 100 // По умолчанию
	}

	messages, err := b.storage.GetMessages(chatID, lookbackMessages)
	if err != nil {
		return fmt.Errorf("ошибка получения сообщений: %w", err)
	}

	if len(messages) < 3 {
		log.Printf("[EmotionalAnalyzer] Недостаточно сообщений в чате %d для анализа эмоций", chatID)
		return nil
	}

	// Анализируем только недавние взаимодействия (последние 24 часа)
	recentCutoff := time.Now().Add(-24 * time.Hour)
	var recentMessages []*tgbotapi.Message
	for _, msg := range messages {
		msgTime := time.Unix(int64(msg.Date), 0)
		if msgTime.After(recentCutoff) {
			recentMessages = append(recentMessages, msg)
		}
	}

	if len(recentMessages) < 3 {
		log.Printf("[EmotionalAnalyzer] Недостаточно недавних сообщений в чате %d для анализа эмоций", chatID)
		return nil
	}

	// Группируем сообщения по пользователям для индивидуального анализа
	userMessages := make(map[int64][]*tgbotapi.Message)
	for _, msg := range recentMessages {
		if msg.From != nil && msg.From.ID != b.api.Self.ID {
			userMessages[msg.From.ID] = append(userMessages[msg.From.ID], msg)
		}
	}

	// Анализируем эмоции для каждого пользователя
	for userID, messages := range userMessages {
		if len(messages) < 2 {
			continue // Нужно минимум 2 сообщения для анализа
		}

		err := b.analyzeEmotionsForUser(chatID, userID, messages, recentMessages)
		if err != nil {
			log.Printf("[EmotionalAnalyzer ERROR] Ошибка анализа эмоций для пользователя %d в чате %d: %v", userID, chatID, err)
		}
	}

	return nil
}

// analyzeEmotionsForUser анализирует эмоции конкретного пользователя
func (b *Bot) analyzeEmotionsForUser(chatID, userID int64, userMessages, allMessages []*tgbotapi.Message) error {
	// Получаем профиль пользователя
	profile, err := b.storage.GetUserProfile(chatID, userID)
	if err != nil {
		return fmt.Errorf("ошибка получения профиля пользователя: %w", err)
	}

	// Используем новый унифицированный форматтер
	formatter := NewUnifiedMessageFormatter(b.storage, b.config.TimeZone)
	allMessagesFormatted := formatter.FormatMessages(chatID, allMessages)
	userMessagesFormatted := formatter.FormatMessages(chatID, userMessages)

	// Создаём специальный формат для эмоционального анализа
	userName := "Пользователь"
	if profile != nil {
		if profile.Alias != "" {
			userName = profile.Alias
		} else if profile.RealName != "" {
			userName = profile.RealName
		} else if profile.Username != "" {
			userName = profile.Username
		}
	}

	analysisText := fmt.Sprintf("=== АНАЛИЗ ЭМОЦИЙ ПОЛЬЗОВАТЕЛЯ: %s (ID: %d) ===\n\n", userName, userID)
	analysisText += "КОНТЕКСТ ЧАТА:\n" + allMessagesFormatted + "\n\n"
	analysisText += fmt.Sprintf("СООБЩЕНИЯ ПОЛЬЗОВАТЕЛЯ %s:\n", userName) + userMessagesFormatted

	log.Printf("[EmotionalAnalyzer] Chat %d: Использован унифицированный форматтер для анализа пользователя %d", chatID, userID)

	// Запрашиваем анализ у LLM
	response, err := b.llm.GenerateResponseByType(llm.ResponseTypeEmotionalAnalysis, b.config.EmotionalAnalysisPrompt, analysisText, float32(b.config.EmotionalAnalysisPromptTemperature))
	if err != nil {
		return fmt.Errorf("ошибка генерации анализа эмоций: %w", err)
	}

	// Парсим результат
	analysisResult, err := b.parseEmotionalAnalysisResponse(response)
	if err != nil {
		log.Printf("[EmotionalAnalyzer WARN] Не удалось распарсить результат анализа эмоций: %v", err)
		return nil
	}

	// Сохраняем эмоциональную память
	err = b.saveEmotionalMemory(chatID, userID, analysisResult, profile)
	if err != nil {
		return fmt.Errorf("ошибка сохранения эмоциональной памяти: %w", err)
	}

	// Обновляем эмоциональное состояние бота
	err = b.updateBotEmotionalState(chatID, analysisResult)
	if err != nil {
		log.Printf("[EmotionalAnalyzer WARN] Не удалось обновить эмоциональное состояние бота: %v", err)
	}

	log.Printf("[EmotionalAnalyzer] Анализ эмоций завершен для пользователя %d в чате %d: %s (%.2f)",
		userID, chatID, analysisResult.PrimaryEmotion, analysisResult.EmotionIntensity)

	return nil
}

// parseEmotionalAnalysisResponse парсит ответ LLM с анализом эмоций
func (b *Bot) parseEmotionalAnalysisResponse(llmResponse string) (*EmotionalAnalysisResult, error) {
	// Очищаем ответ от markdown
	cleanResponse := b.cleanJSONFromMarkdown(llmResponse)

	var result EmotionalAnalysisResult
	err := json.Unmarshal([]byte(cleanResponse), &result)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга JSON: %w", err)
	}

	// Валидируем результат
	if result.PrimaryEmotion == "" {
		return nil, fmt.Errorf("отсутствует primary_emotion")
	}

	if result.EmotionIntensity < 0 || result.EmotionIntensity > 1 {
		result.EmotionIntensity = 0.5 // Устанавливаем значение по умолчанию
	}

	if result.Confidence < 0 || result.Confidence > 1 {
		result.Confidence = 0.5 // Устанавливаем значение по умолчанию
	}

	return &result, nil
}

// saveEmotionalMemory сохраняет эмоциональную память о пользователе
func (b *Bot) saveEmotionalMemory(chatID, userID int64, analysis *EmotionalAnalysisResult, profile *storage.UserProfile) error {
	userContext := "unknown"
	if profile != nil {
		if profile.Alias != "" {
			userContext = profile.Alias
		} else if profile.Username != "" {
			userContext = profile.Username
		}
	}

	// Создаем запись эмоциональной памяти
	memory := &storage.EmotionalMemory{
		ChatID:           chatID,
		UserID:           userID,
		UserContext:      userContext,
		Trigger:          analysis.Trigger,
		PrimaryEmotion:   analysis.PrimaryEmotion,
		EmotionIntensity: analysis.EmotionIntensity,
		Response:         analysis.ResponseSuggestion,
		Outcome:          "pending", // Будет обновлено позже на основе реакций
		Success:          false,     // Будет обновлено позже
		Reinforcement:    0.0,       // Нейтральное подкрепление
		Frequency:        1,
		TopicContext:     analysis.Context,
		Keywords:         []string{analysis.PrimaryEmotion, analysis.Trigger},
	}

	return b.storage.AddEmotionalMemory(memory)
}

// updateBotEmotionalState обновляет эмоциональное состояние бота на основе анализа
func (b *Bot) updateBotEmotionalState(chatID int64, analysis *EmotionalAnalysisResult) error {
	// Получаем текущее эмоциональное состояние
	currentState, err := b.storage.GetEmotionalState(chatID)
	if err != nil {
		return fmt.Errorf("ошибка получения эмоционального состояния: %w", err)
	}

	// Обновляем эмоции на основе анализа
	emotions := make(map[string]float64)

	// Применяем влияние анализированной эмоции
	influence := analysis.EmotionIntensity * 0.1 // Небольшое влияние

	switch analysis.PrimaryEmotion {
	case "joy", "happiness", "радость":
		emotions["joy"] = math.Min(1.0, currentState.Joy+influence)
		emotions["optimism"] = math.Min(1.0, currentState.Optimism+influence*0.5)
	case "sadness", "грусть", "печаль":
		emotions["sadness"] = math.Min(1.0, currentState.Sadness+influence)
		emotions["empathy"] = math.Min(1.0, currentState.Empathy+influence*0.3)
	case "anger", "гнев", "злость":
		emotions["anger"] = math.Min(1.0, currentState.Anger+influence)
		emotions["irritability"] = math.Min(1.0, currentState.Irritability+influence*0.4)
	case "fear", "страх":
		emotions["fear"] = math.Min(1.0, currentState.Fear+influence)
		emotions["anxiety"] = math.Min(1.0, currentState.Anxiety+influence*0.6)
	case "surprise", "удивление":
		emotions["surprise"] = math.Min(1.0, currentState.Surprise+influence)
		emotions["curiosity"] = math.Min(1.0, currentState.Curiosity+influence*0.7)
	default:
		// Для неизвестных эмоций увеличиваем uncertainty
		emotions["uncertainty"] = math.Min(1.0, currentState.Uncertainty+influence*0.3)
	}

	// Обновляем интенсивность
	intensity := math.Min(1.0, currentState.Intensity+influence*0.2)

	return b.storage.UpdateEmotionalState(chatID, emotions, intensity, analysis.Trigger)
}

// GetEmotionalContext возвращает эмоциональный контекст для генерации ответов
func (b *Bot) GetEmotionalContext(chatID, userID int64) string {
	// Получаем эмоциональную память о пользователе
	memories, err := b.storage.GetEmotionalMemories(chatID, userID, 5)
	if err != nil || len(memories) == 0 {
		return "Нет эмоциональной истории с этим пользователем."
	}

	var context strings.Builder
	context.WriteString("ЭМОЦИОНАЛЬНАЯ ПАМЯТЬ:\n")

	for _, memory := range memories {
		age := time.Since(memory.CreatedAt)
		context.WriteString(fmt.Sprintf("- %s (интенсивность: %.2f, %s назад): %s\n",
			memory.PrimaryEmotion, memory.EmotionIntensity,
			b.formatDuration(age), memory.Trigger))
	}

	// Получаем текущее эмоциональное состояние бота
	state, err := b.storage.GetEmotionalState(chatID)
	if err == nil && state != nil {
		context.WriteString(fmt.Sprintf("\nТЕКУЩЕЕ НАСТРОЕНИЕ: интенсивность %.2f, стабильность %.2f\n",
			state.Intensity, state.Stability))

		// Добавляем доминирующие эмоции
		dominantEmotions := b.getDominantEmotions(state)
		if len(dominantEmotions) > 0 {
			context.WriteString("ДОМИНИРУЮЩИЕ ЭМОЦИИ: ")
			for i, emotion := range dominantEmotions {
				if i > 0 {
					context.WriteString(", ")
				}
				context.WriteString(emotion)
			}
			context.WriteString("\n")
		}
	}

	return context.String()
}

// GetEmotionalAdaptation получает рекомендации по эмоциональной адаптации для пользователя
func (b *Bot) GetEmotionalAdaptation(chatID, userID int64) (*EmotionalAdaptationResult, error) {
	if !b.config.EmotionalLearningEnabled {
		return nil, nil
	}

	// Получаем эмоциональный контекст пользователя
	emotionalContext := b.GetEmotionalContext(chatID, userID)
	if emotionalContext == "" {
		return nil, nil
	}

	// Получаем контекст личности для анализа
	personalityContext, err := b.getPersonalityContext(chatID, "emotional_adaptation")
	if err != nil {
		return nil, fmt.Errorf("ошибка получения контекста личности: %w", err)
	}

	// Формируем входные данные для анализа
	analysisText := fmt.Sprintf(
		"Пользователь %d в чате %d:\n\n%s",
		userID, chatID, emotionalContext,
	)

	// Заменяем плейсхолдеры в промпте
	prompt := strings.ReplaceAll(b.config.EmotionalAdaptationPrompt, "{PERSONALITY_CONTEXT}", personalityContext)
	prompt = strings.ReplaceAll(prompt, "{EMOTIONAL_CONTEXT}", emotionalContext)

	// Запрашиваем анализ у LLM
	response, err := b.llm.GenerateResponseByType(llm.ResponseTypeEmotionalAdaptation, prompt, analysisText, float32(b.config.EmotionalAdaptationPromptTemperature))
	if err != nil {
		return nil, fmt.Errorf("ошибка генерации анализа эмоциональной адаптации: %w", err)
	}

	// Парсим JSON ответ
	var result EmotionalAdaptationResult
	if err := json.Unmarshal([]byte(cleanJSONFromMarkdown(response)), &result); err != nil {
		log.Printf("[ERROR] Ошибка парсинга JSON ответа эмоциональной адаптации: %v", err)
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	log.Printf("[DEBUG] Получены рекомендации по эмоциональной адаптации для пользователя %d: тон=%s, эмпатия=%.2f, юмор=%.2f",
		userID, result.ToneAdjustment, result.EmpathyLevel, result.HumorAppropriateness)

	return &result, nil
}

// ApplyEmotionalAdaptationToPrompt применяет эмоциональную адаптацию к промпту
func (b *Bot) ApplyEmotionalAdaptationToPrompt(basePrompt string, chatID, userID int64) string {
	if !b.config.EmotionalLearningEnabled {
		return basePrompt
	}

	// Получаем рекомендации по адаптации
	adaptation, err := b.GetEmotionalAdaptation(chatID, userID)
	if err != nil {
		log.Printf("[WARN] Ошибка получения эмоциональной адаптации: %v", err)
		return basePrompt
	}

	if adaptation == nil {
		return basePrompt
	}

	// Формируем дополнительные инструкции на основе эмоциональной адаптации
	var adaptationInstructions strings.Builder
	adaptationInstructions.WriteString("\n\n=== ЭМОЦИОНАЛЬНАЯ АДАПТАЦИЯ ===\n")

	// Настройка тона
	if adaptation.ToneAdjustment != "" && adaptation.ToneAdjustment != "neutral" {
		adaptationInstructions.WriteString(fmt.Sprintf("Тон: %s\n", adaptation.ToneAdjustment))
	}

	// Изменения в стиле
	if len(adaptation.StyleChanges) > 0 {
		adaptationInstructions.WriteString(fmt.Sprintf("Изменения в стиле: %s\n", strings.Join(adaptation.StyleChanges, ", ")))
	}

	// Темы для избегания
	if len(adaptation.AvoidTopics) > 0 {
		adaptationInstructions.WriteString(fmt.Sprintf("Избегай: %s\n", strings.Join(adaptation.AvoidTopics, ", ")))
	}

	// Рекомендуемый подход
	if adaptation.RecommendedApproach != "" {
		adaptationInstructions.WriteString(fmt.Sprintf("Подход: %s\n", adaptation.RecommendedApproach))
	}

	// Уровень эмпатии
	if adaptation.EmpathyLevel > 0.7 {
		adaptationInstructions.WriteString("Проявляй повышенную эмпатию и понимание\n")
	} else if adaptation.EmpathyLevel < 0.3 {
		adaptationInstructions.WriteString("Будь более сдержанным и нейтральным\n")
	}

	// Уместность юмора
	if adaptation.HumorAppropriateness < 0.3 {
		adaptationInstructions.WriteString("Избегай юмора и легкомыслия\n")
	} else if adaptation.HumorAppropriateness > 0.7 {
		adaptationInstructions.WriteString("Можешь использовать юмор и легкость\n")
	}

	return basePrompt + adaptationInstructions.String()
}

// getDominantEmotions возвращает список доминирующих эмоций
func (b *Bot) getDominantEmotions(state *storage.EmotionalState) []string {
	emotions := map[string]float64{
		"радость":           state.Joy,
		"грусть":            state.Sadness,
		"гнев":              state.Anger,
		"страх":             state.Fear,
		"доверие":           state.Trust,
		"отвращение":        state.Disgust,
		"удивление":         state.Surprise,
		"предвкушение":      state.Anticipation,
		"цинизм":            state.Cynicism,
		"эмпатия":           state.Empathy,
		"раздражительность": state.Irritability,
	}

	var dominant []string
	for emotion, value := range emotions {
		if value > 0.6 { // Порог для "доминирующей" эмоции
			dominant = append(dominant, emotion)
		}
	}

	return dominant
}

// formatDuration форматирует продолжительность времени
func (b *Bot) formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "меньше минуты"
	} else if d < time.Hour {
		return fmt.Sprintf("%d мин", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%d ч", int(d.Hours()))
	} else {
		return fmt.Sprintf("%d дн", int(d.Hours()/24))
	}
}

// AnalyzeEmotionalFeedback анализирует эмоциональную обратную связь пользователя
func (b *Bot) AnalyzeEmotionalFeedback(chatID, userID int64, interactionHistory, userReaction string) (*EmotionalFeedbackResult, error) {
	if !b.config.EmotionalLearningEnabled {
		return nil, nil
	}

	// Получаем контекст личности для анализа
	personalityContext, err := b.getPersonalityContext(chatID, "emotional_feedback")
	if err != nil {
		return nil, fmt.Errorf("ошибка получения контекста личности: %w", err)
	}

	// Заменяем плейсхолдеры в промпте
	prompt := strings.ReplaceAll(b.config.EmotionalFeedbackPrompt, "{PERSONALITY_CONTEXT}", personalityContext)
	prompt = strings.ReplaceAll(prompt, "{INTERACTION_HISTORY}", interactionHistory)
	prompt = strings.ReplaceAll(prompt, "{USER_REACTION}", userReaction)

	// Формируем входные данные для анализа
	analysisText := fmt.Sprintf(
		"Анализ эмоциональной обратной связи для пользователя %d в чате %d:\n\nИстория взаимодействия:\n%s\n\nРеакция пользователя:\n%s",
		userID, chatID, interactionHistory, userReaction,
	)

	// Запрашиваем анализ у LLM
	response, err := b.llm.GenerateResponseByType(llm.ResponseTypeEmotionalFeedback, prompt, analysisText, float32(b.config.EmotionalFeedbackPromptTemperature))
	if err != nil {
		return nil, fmt.Errorf("ошибка генерации анализа эмоциональной обратной связи: %w", err)
	}

	// Парсим JSON ответ
	var result EmotionalFeedbackResult
	if err := json.Unmarshal([]byte(cleanJSONFromMarkdown(response)), &result); err != nil {
		log.Printf("[ERROR] Ошибка парсинга JSON ответа эмоциональной обратной связи: %v", err)
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	log.Printf("[DEBUG] Анализ эмоциональной обратной связи от пользователя %d: тип=%s, эффективность=%.2f",
		userID, result.ReactionType, result.ApproachEffectiveness)

	return &result, nil
}

// ProcessEmotionalFeedback обрабатывает эмоциональную обратную связь и обновляет систему обучения
func (b *Bot) ProcessEmotionalFeedback(chatID, userID int64, botResponse string, userReaction *tgbotapi.Message) error {
	if !b.config.EmotionalLearningEnabled {
		return nil
	}

	// Формируем историю взаимодействия
	interactionHistory := fmt.Sprintf("Ответ бота: %s", botResponse)

	// Формируем реакцию пользователя
	userReactionText := ""
	if userReaction != nil {
		userReactionText = fmt.Sprintf("Сообщение: %s", userReaction.Text)
		if userReaction.ReplyToMessage != nil {
			userReactionText += " (в ответ на сообщение бота)"
		}
	}

	// Анализируем эмоциональную обратную связь
	feedback, err := b.AnalyzeEmotionalFeedback(chatID, userID, interactionHistory, userReactionText)
	if err != nil {
		log.Printf("[WARN] Ошибка анализа эмоциональной обратной связи: %v", err)
		return err
	}

	if feedback == nil {
		return nil
	}

	// Сохраняем результаты обучения
	if err := b.SaveEmotionalLearning(chatID, userID, feedback); err != nil {
		log.Printf("[WARN] Ошибка сохранения результатов эмоционального обучения: %v", err)
		return err
	}

	// Обновляем эмоциональную память пользователя на основе обратной связи
	if err := b.UpdateEmotionalMemoryFromFeedback(chatID, userID, feedback); err != nil {
		log.Printf("[WARN] Ошибка обновления эмоциональной памяти: %v", err)
		return err
	}

	return nil
}

// SaveEmotionalLearning сохраняет результаты эмоционального обучения
func (b *Bot) SaveEmotionalLearning(chatID, userID int64, feedback *EmotionalFeedbackResult) error {
	// Получаем текущую память личности
	memory, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil {
		return fmt.Errorf("ошибка получения памяти личности: %w", err)
	}

	if memory == nil {
		memory = &storage.PersonalityMemory{
			ChatID:            chatID,
			EmotionalMemories: []*storage.EmotionalMemory{},
		}
	}

	// Создаем новую запись эмоционального обучения
	learningEntry := &storage.EmotionalMemory{
		ChatID:           chatID,
		UserID:           userID,
		PrimaryEmotion:   feedback.ReactionType,
		EmotionIntensity: feedback.ApproachEffectiveness,
		UserContext:      strings.Join(feedback.LearningInsights, "; "),
		Trigger:          strings.Join(feedback.ProblematicElements, "; "),
		CreatedAt:        time.Now(),
	}

	// Добавляем запись в память
	if memory.EmotionalMemories == nil {
		memory.EmotionalMemories = []*storage.EmotionalMemory{}
	}

	memory.EmotionalMemories = append(memory.EmotionalMemories, learningEntry)

	// Ограничиваем количество записей (оставляем только последние 50)
	if len(memory.EmotionalMemories) > 50 {
		memory.EmotionalMemories = memory.EmotionalMemories[len(memory.EmotionalMemories)-50:]
	}

	// Сохраняем обновленную память
	return b.storage.SavePersonalityMemory(memory)
}

// UpdateEmotionalMemoryFromFeedback обновляет эмоциональную память пользователя на основе обратной связи
func (b *Bot) UpdateEmotionalMemoryFromFeedback(chatID, userID int64, feedback *EmotionalFeedbackResult) error {
	// Получаем текущее эмоциональное состояние пользователя
	memory, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil {
		return fmt.Errorf("ошибка получения памяти личности: %w", err)
	}

	if memory == nil || memory.EmotionalState == nil {
		return nil // Нет данных для обновления
	}

	// Корректируем эмоциональное состояние на основе эффективности подхода
	effectivenessAdjustment := (feedback.ApproachEffectiveness - 0.5) * 0.1 // Небольшая корректировка

	// Обновляем стабильность эмоций
	if feedback.ApproachEffectiveness > 0.7 {
		memory.EmotionalState.Stability = math.Min(1.0, memory.EmotionalState.Stability+effectivenessAdjustment)
	} else if feedback.ApproachEffectiveness < 0.3 {
		memory.EmotionalState.Stability = math.Max(0.0, memory.EmotionalState.Stability+effectivenessAdjustment)
	}

	// Обновляем интенсивность эмоций
	if feedback.ReactionType == "positive" {
		memory.EmotionalState.Intensity = math.Min(1.0, memory.EmotionalState.Intensity+effectivenessAdjustment)
	} else if feedback.ReactionType == "negative" {
		memory.EmotionalState.Intensity = math.Max(0.0, memory.EmotionalState.Intensity-effectivenessAdjustment)
	}

	memory.EmotionalState.LastUpdate = time.Now()

	// ЭТАП 4: небольшое социальное обучение (гейтом)
	if b.config.RelationshipTrackingEnabled {
		var dTrust, dIntimacy float64
		switch feedback.ReactionType {
		case "positive":
			dTrust = 0.02
			dIntimacy = 0.01
		case "negative":
			dTrust = -0.02
			dIntimacy = -0.01
		default:
			// нейтрально
		}
		if dTrust != 0 || dIntimacy != 0 {
			b.UpdateRelationship(chatID, userID, dTrust, dIntimacy, "feedback")
		}
	}

	// Сохраняем обновленную память
	return b.storage.SavePersonalityMemory(memory)
}
