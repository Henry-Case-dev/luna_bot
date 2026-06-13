package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/bot/prompts"
	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// parseShouldReplyDecision парсит ответ первого этапа
func (fws *FreeWillService) parseShouldReplyDecision(response string) (*FreeWillShouldReplyDecision, error) {
	response = strings.TrimSpace(response)
	response = cleanJSONFromMarkdown(response)

	startIdx := strings.Index(response, "{")
	endIdx := strings.LastIndex(response, "}")

	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		return nil, fmt.Errorf("JSON не найден в ответе этапа 1: %s", response)
	}

	jsonStr := response[startIdx : endIdx+1]

	var decision FreeWillShouldReplyDecision
	if err := json.Unmarshal([]byte(jsonStr), &decision); err != nil {
		// FALLBACK: Пробуем парсить как generic map для извлечения значений с неправильными полями
		log.Printf("[FreeWill] parseShouldReplyDecision: Обычный парсинг не удался, пробуем fallback: %v", err)

		var rawJSON map[string]interface{}
		if err2 := json.Unmarshal([]byte(jsonStr), &rawJSON); err2 != nil {
			return nil, fmt.Errorf("ошибка парсинга JSON этапа 1: %w (оригинал: %v)", err2, err)
		}

		// Инициализируем с дефолтными значениями
		decision = FreeWillShouldReplyDecision{}

		// Пробуем извлечь should_reply из различных полей
		if val, ok := rawJSON["should_reply"]; ok {
			if b, ok := val.(bool); ok {
				decision.ShouldReply = b
			}
		} else if val, ok := rawJSON["should_respond"]; ok { // FALLBACK для неправильного поля
			if b, ok := val.(bool); ok {
				decision.ShouldReply = b
			}
		} else if val, ok := rawJSON["decision"]; ok { // FALLBACK для поля "decision" (часто возвращается LLM)
			if s, ok := val.(string); ok {
				// Конвертируем строковые значения в bool
				switch strings.ToLower(s) {
				case "reply", "respond", "yes", "true":
					decision.ShouldReply = true
				case "no_reply", "no_respond", "no", "false":
					decision.ShouldReply = false
				}
				log.Printf("[FreeWill] parseShouldReplyDecision: Конвертируем decision '%s' в should_reply=%t", s, decision.ShouldReply)
			}
		}

		// Пробуем извлечь reply_type
		if val, ok := rawJSON["reply_type"]; ok {
			if s, ok := val.(string); ok {
				decision.ReplyType = s
			}
		} else if val, ok := rawJSON["response_type"]; ok { // FALLBACK для неправильного поля
			if s, ok := val.(string); ok {
				decision.ReplyType = s
			}
		} else {
			// Если reply_type не найден, устанавливаем дефолтное значение на основе других данных
			if decision.TargetMessageID > 0 {
				decision.ReplyType = "direct_reply"
				log.Printf("[FreeWill] parseShouldReplyDecision: reply_type не найден, но есть target_message_id, устанавливаем 'direct_reply'")
			} else {
				decision.ReplyType = "general"
				log.Printf("[FreeWill] parseShouldReplyDecision: reply_type не найден, устанавливаем 'general'")
			}
		}

		// Пробуем извлечь target_message_id
		if val, ok := rawJSON["target_message_id"]; ok {
			if f, ok := val.(float64); ok {
				decision.TargetMessageID = int(f)
			}
		}

		// Пробуем извлечь reason
		if val, ok := rawJSON["reason"]; ok {
			if s, ok := val.(string); ok {
				decision.Reason = s
			}
		} else if val, ok := rawJSON["response"]; ok { // FALLBACK: если LLM поместил текст в "response"
			if s, ok := val.(string); ok {
				decision.Reason = "LLM дал готовый ответ: " + s
				log.Printf("[FreeWill] parseShouldReplyDecision: LLM поместил текст ответа в поле 'response', используем как reason")
			}
		} else if val, ok := rawJSON["text"]; ok { // FALLBACK: если LLM поместил готовый текст (частая ошибка)
			if s, ok := val.(string); ok {
				decision.Reason = "LLM дал готовый ответ: " + s
				log.Printf("[FreeWill] parseShouldReplyDecision: LLM поместил готовый текст в поле 'text', используем как reason")
			}
		}

		log.Printf("[FreeWill] parseShouldReplyDecision: Fallback извлечение данных: should_reply=%t, reply_type=%s, target=%d, reason=%s",
			decision.ShouldReply, decision.ReplyType, decision.TargetMessageID, decision.Reason)
	}

	return &decision, nil
}

// parseResponseTypeDecision парсит ответ второго этапа
func (fws *FreeWillService) parseResponseTypeDecision(response string) (*FreeWillResponseTypeDecision, error) {
	response = strings.TrimSpace(response)
	response = cleanJSONFromMarkdown(response)

	startIdx := strings.Index(response, "{")
	endIdx := strings.LastIndex(response, "}")

	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		return nil, fmt.Errorf("JSON не найден в ответе этапа 2: %s", response)
	}

	jsonStr := response[startIdx : endIdx+1]

	var decision FreeWillResponseTypeDecision
	if err := json.Unmarshal([]byte(jsonStr), &decision); err != nil {
		return nil, fmt.Errorf("ошибка парсинга JSON этапа 2: %w", err)
	}

	return &decision, nil
}

// parseDirectResponseShouldReplyDecision парсит ответ LLM для принятия решения (Этап 1)
func (fws *FreeWillService) parseDirectResponseShouldReplyDecision(response string) (*FreeWillShouldReplyDecision, error) {
	// Пытаемся распарсить как JSON
	var decision FreeWillShouldReplyDecision

	// Очищаем ответ от markdown (специально для JSON)
	cleanResponse := cleanJSONFromMarkdown(response)

	log.Printf("[DEBUG][FreeWill] parseDirectResponseShouldReplyDecision: Исходный ответ: %s", response)
	log.Printf("[DEBUG][FreeWill] parseDirectResponseShouldReplyDecision: Очищенный ответ: %s", cleanResponse)

	if err := json.Unmarshal([]byte(cleanResponse), &decision); err != nil {
		// Если JSON не парсится, возвращаем решение не отвечать
		log.Printf("[WARN][FreeWill] parseDirectResponseShouldReplyDecision: Не удалось распарсить JSON решения: %v", err)
		log.Printf("[WARN][FreeWill] parseDirectResponseShouldReplyDecision: Проблемный JSON: %s", cleanResponse)
		return &FreeWillShouldReplyDecision{
			ShouldReply: false,
			ReplyType:   "ignore",
			Reason:      "Не удалось распарсить решение LLM",
		}, nil
	}

	log.Printf("[DEBUG][FreeWill] parseDirectResponseShouldReplyDecision: ✅ JSON успешно распарсен: should_reply=%t, reply_type=%s",
		decision.ShouldReply, decision.ReplyType)
	return &decision, nil
}

// parseDirectResponseContentDecision парсит ответ LLM для генерации контента (Этап 2)
// ВАЖНО: возвращает ошибку если JSON не распарсился — вызывающий код ДОЛЖЕН сделать fallback-генерацию,
// а НЕ отправлять сырой JSON в чат!
func (fws *FreeWillService) parseDirectResponseContentDecision(response string) (*FreeWillResponseTypeDecision, error) {
	var decision FreeWillResponseTypeDecision

	// Очищаем ответ от markdown (специально для JSON)
	cleanResponse := cleanJSONFromMarkdown(response)

	log.Printf("[DEBUG][FreeWill] parseDirectResponseContentDecision: Исходный ответ: %s", response)
	log.Printf("[DEBUG][FreeWill] parseDirectResponseContentDecision: Очищенный ответ: %s", cleanResponse)

	if err := json.Unmarshal([]byte(cleanResponse), &decision); err != nil {
		// P0 FIX: НЕ отправляем сырой JSON в чат!
		// Проверяем — не является ли ответ JSON-структурой с мыслями модели
		if looksLikeInternalJSON(response) {
			log.Printf("[ERROR][FreeWill] parseDirectResponseContentDecision: ОТВЕТ ПОХОЖ НА ВНУТРЕННИЙ JSON (галлюцинация) — возвращаем ошибку для fallback-генерации")
			log.Printf("[ERROR][FreeWill] parseDirectResponseContentDecision: Проблемный ответ: %s", response)
			return nil, fmt.Errorf("ответ является внутренним JSON, требуется fallback-генерация")
		}

		// Если это не JSON — пробуем извлечь чистый текст (без JSON-структур)
		textResponse := cleanupLLMResponse(response)
		// Дополнительная защита: если после очистки остались JSON-ключи — не отправляем
		if looksLikeInternalJSON(textResponse) {
			log.Printf("[ERROR][FreeWill] parseDirectResponseContentDecision: После очистки остались JSON-признаки — возвращаем ошибку")
			return nil, fmt.Errorf("после очистки ответ всё ещё содержит JSON-структуры")
		}

		log.Printf("[WARN][FreeWill] parseDirectResponseContentDecision: JSON не распарсен, но ответ не похож на внутренний JSON — используем как текст")
		return &FreeWillResponseTypeDecision{
			Text:    textResponse,
			IsVoice: false,
			Mood:    "neutral",
		}, nil
	}

	// Дополнительная проверка: даже если JSON распарсился, проверяем что text не содержит мусора
	if looksLikeInternalJSON(decision.Text) {
		log.Printf("[WARN][FreeWill] parseDirectResponseContentDecision: Поле text содержит JSON-подобный мусор, очищаем")
		decision.Text = stripJSONLikeContent(decision.Text)
	}

	if decision.Text == "" {
		log.Printf("[WARN][FreeWill] parseDirectResponseContentDecision: JSON распарсен но text пустой — возвращаем ошибку для fallback")
		return nil, fmt.Errorf("JSON распарсен но поле text пустое")
	}

	log.Printf("[DEBUG][FreeWill] parseDirectResponseContentDecision: ✅ JSON успешно распарсен: text_length=%d, is_voice=%t, mood=%s",
		len(decision.Text), decision.IsVoice, decision.Mood)
	return &decision, nil
}

// looksLikeInternalJSON проверяет, похож ли текст на внутренний JSON модели
// (защита от галлюцинаций — модель выдала свои «мысли» вместо ответа)
func looksLikeInternalJSON(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	// Признаки внутреннего JSON: содержит ключи решений/состояний
	internalKeys := []string{
		"shouldreply", "should_reply",
		"replytype", "reply_type",
		"targetmessage_id", "target_message_id",
		"\"reason\":", "\"mood\":",
		"\"confidence\":", "\"is_voice\":",
	}
	braceCount := strings.Count(text, "{") + strings.Count(text, "}")
	if braceCount >= 2 {
		for _, key := range internalKeys {
			if strings.Contains(text, key) {
				return true
			}
		}
	}
	return false
}

// stripJSONLikeContent удаляет JSON-подобные конструкции из текста
func stripJSONLikeContent(text string) string {
	// Удаляем всё что в фигурных скобках (включая скобки)
	re := regexp.MustCompile(`\{[^}]*\}`)
	text = re.ReplaceAllString(text, "")
	// Удаляем оставшиеся JSON-ключи
	text = regexp.MustCompile(`"?\w+"?\s*:\s*[^,\s]+[,\s]*`).ReplaceAllString(text, "")
	return strings.TrimSpace(text)
}

// parseDirectResponseDecision парсит ответ LLM для Free Will Direct Response (УСТАРЕЛ - для обратной совместимости)
func (fws *FreeWillService) parseDirectResponseDecision(response string) (*FreeWillDecision, error) {
	// Пытаемся распарсить как JSON
	var decision FreeWillDecision

	// Очищаем ответ от markdown (специально для JSON)
	cleanResponse := cleanJSONFromMarkdown(response)

	log.Printf("[DEBUG][FreeWill] parseDirectResponseDecision: Исходный ответ: %s", response)
	log.Printf("[DEBUG][FreeWill] parseDirectResponseDecision: Очищенный ответ: %s", cleanResponse)

	if err := json.Unmarshal([]byte(cleanResponse), &decision); err != nil {
		// Если JSON не парсится, возвращаем простое решение не отвечать
		log.Printf("[WARN][FreeWill] parseDirectResponseDecision: Не удалось распарсить JSON: %v", err)
		log.Printf("[WARN][FreeWill] parseDirectResponseDecision: Проблемный JSON: %s", cleanResponse)
		return &FreeWillDecision{
			ShouldReply: false,
			ReplyType:   "ignore",
			Reason:      "Не удалось распарсить решение LLM",
		}, nil
	}

	log.Printf("[DEBUG][FreeWill] parseDirectResponseDecision: ✅ JSON успешно распарсен: should_reply=%t, reply_type=%s",
		decision.ShouldReply, decision.ReplyType)
	return &decision, nil
}

// updateMood обновляет настроение бота на основе анализа контекста
func (fws *FreeWillService) updateMood(chatID int64, _ string) {
	log.Printf("[FreeWill] updateMood: Начинаем обновление настроения для чата %d", chatID)

	moodRoll := fws.randSource.Float64()
	log.Printf("[FreeWill] updateMood: Проверка вероятности обновления настроения для чата %d: %.3f > %.3f",
		chatID, moodRoll, fws.moodUpdateProbability)

	if moodRoll > fws.moodUpdateProbability {
		log.Printf("[FreeWill] updateMood: Обновление настроения пропущено для чата %d (не прошла проверка вероятности)", chatID)
		return // Не каждый раз обновляем настроение
	}

	log.Printf("[FreeWill] updateMood: Приступаем к анализу настроения для чата %d", chatID)

	// Build ChatML context
	chatMsgs := fws.buildResponseChatContext(chatID, fws.bot.config.ContextWindow, 0)
	if chatMsgs == nil {
		log.Printf("[FreeWill] updateMood: Ошибка получения ChatML контекста для чата %d", chatID)
		return
	}

	// Prepend mood analysis prompt to system message
	prompt := fws.bot.enrichPromptWithPersonality(fws.bot.config.FreeWillMoodAnalysisPrompt, chatID, "free_will_mood_analysis")
	chatMsgs[0].Content = prompt + "\n\n" + chatMsgs[0].Content

	log.Printf("[FreeWill] updateMood: Отправляем запрос к LLM для анализа настроения чата %d", chatID)
	moodAnalysisStart := time.Now()
	response, err := fws.bot.llm.GenerateChatResponse(llm.ResponseTypeFreeWillMoodAnalysis, chatMsgs, float32(decisionTemperature))
	moodAnalysisDuration := time.Since(moodAnalysisStart)

	if err != nil {
		log.Printf("[FreeWill] updateMood: Ошибка анализа настроения для чата %d: %v", chatID, err)
		return
	}
	log.Printf("[FreeWill] updateMood: Получен ответ LLM для анализа настроения чата %d (время: %v, длина: %d символов)",
		chatID, moodAnalysisDuration, len(response))
	log.Printf("[FreeWill] updateMood: Ответ LLM для настроения чата %d: %s", chatID, response)

	// Очищаем ответ от markdown code blocks и backticks перед парсингом JSON
	response = cleanJSONFromMarkdown(response)
	log.Printf("[FreeWill] updateMood: Очищенный ответ для парсинга: %s", response)

	// Парсим JSON ответ
	log.Printf("[FreeWill] updateMood: Парсим JSON ответ для настроения чата %d", chatID)
	var moodData struct {
		CurrentMood   string  `json:"current_mood"`
		MoodIntensity float64 `json:"mood_intensity"`
		TriggerReason string  `json:"trigger_reason"`
	}

	if err := json.Unmarshal([]byte(response), &moodData); err != nil {
		log.Printf("[FreeWill] updateMood: Ошибка парсинга JSON настроения для чата %d: %v", chatID, err)
		log.Printf("[FreeWill] updateMood: Сырой ответ для отладки: %s", response)
		return
	}
	log.Printf("[FreeWill] updateMood: JSON настроения распарсен для чата %d: mood=%s, intensity=%.2f, reason=%s",
		chatID, moodData.CurrentMood, moodData.MoodIntensity, moodData.TriggerReason)

	// Получаем текущее настроение для сравнения
	currentMood := fws.getMood(chatID)
	log.Printf("[FreeWill] updateMood: Текущее настроение для чата %d: %s (интенсивность: %.2f)",
		chatID, currentMood.CurrentMood, currentMood.MoodIntensity)

	// Создаем новое состояние настроения
	newMoodState := &storage.MoodState{
		ChatID:         chatID,
		CurrentMood:    moodData.CurrentMood,
		MoodIntensity:  moodData.MoodIntensity,
		LastMoodUpdate: time.Now(),
		TriggerReason:  moodData.TriggerReason,
		UpdatedAt:      time.Now(),
	}

	// Сохраняем в БД
	log.Printf("[FreeWill] updateMood: Сохраняем новое настроение в БД для чата %d", chatID)
	err = fws.bot.storage.SaveMoodState(newMoodState)
	if err != nil {
		log.Printf("[FreeWill] updateMood: Ошибка сохранения настроения в БД для чата %d: %v", chatID, err)
		return
	}

	log.Printf("[FreeWill] updateMood: Настроение успешно обновлено для чата %d: %s -> %s (интенсивность: %.2f -> %.2f, причина: %s)",
		chatID, currentMood.CurrentMood, moodData.CurrentMood, currentMood.MoodIntensity, moodData.MoodIntensity, moodData.TriggerReason)
}

// getCurrentMood возвращает текущее настроение в старом формате для совместимости
func (fws *FreeWillService) getCurrentMood(chatID int64) *FreeWillMoodState {
	// Получаем настроение из БД
	moodFromDB := fws.getMood(chatID)

	// Конвертируем в старый формат
	return &FreeWillMoodState{
		CurrentMood:    moodFromDB.CurrentMood,
		MoodIntensity:  moodFromDB.MoodIntensity,
		LastMoodUpdate: moodFromDB.LastMoodUpdate,
		TriggerReason:  moodFromDB.TriggerReason,
	}
}

// getCurrentMoodName возвращает название текущего настроения для direct reply
// Используется вместо парсинга JSON от LLM (P1 fix)
func (fws *FreeWillService) getCurrentMoodName(chatID int64) string {
	mood := fws.getMood(chatID)
	if mood == nil || mood.CurrentMood == "" {
		return "neutral"
	}
	return mood.CurrentMood
}

// getMood возвращает текущее настроение бота для чата
func (fws *FreeWillService) getMood(chatID int64) *storage.MoodState {
	// Получаем настроение из БД
	mood, err := fws.bot.storage.GetMoodState(chatID)
	if err != nil {
		log.Printf("[FreeWill] Ошибка получения настроения из БД для чата %d: %v", chatID, err)
		// Возвращаем базовое настроение
		return &storage.MoodState{
			ChatID:         chatID,
			CurrentMood:    "neutral",
			MoodIntensity:  0.5,
			LastMoodUpdate: time.Now(),
			TriggerReason:  "Default fallback mood",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
	}

	return mood
}

// ================================ СИСТЕМА РЕАКЦИЙ ================================

// analyzeForReaction анализирует сообщение для постановки реакции
func (fws *FreeWillService) analyzeForReaction(chatID int64, message *tgbotapi.Message) {
	// Проверяем основные условия ДО обращения к полям message
	if message == nil {
		log.Printf("[FreeWill] analyzeForReaction: Сообщение отсутствует (nil)")
		return
	}
	// Копируем значение, чтобы дальше работать с непустой структурой и избежать предупреждений анализатора
	msg := *message
	if msg.Text == "" {
		log.Printf("[FreeWill] analyzeForReaction: Сообщение без текста")
		return
	}

	log.Printf("[FreeWill] analyzeForReaction: Начинаем анализ реакции для сообщения %d в чате %d", msg.MessageID, chatID)

	// Проверяем cooldown и лимиты
	if !fws.canReact(chatID) {
		log.Printf("[FreeWill] analyzeForReaction: Нельзя реагировать (cooldown или лимит)")
		return
	}

	// Build ChatML context for reaction analysis
	chatMsgs := fws.buildResponseChatContext(chatID, fws.bot.config.ContextWindow, 0)
	if chatMsgs == nil {
		log.Printf("[FreeWill] analyzeForReaction: Ошибка получения ChatML контекста")
		return
	}
	// Append current message
	chatMsgs = append(chatMsgs, llm.ChatMessage{Role: "user", Content: msg.Text})
	// Prepend reaction prompt to system message
	chatMsgs[0].Content = fws.bot.config.FreeWillReactionPrompt + "\n\n" + chatMsgs[0].Content

	log.Printf("[FreeWill] analyzeForReaction: Отправляем запрос к LLM для выбора реакции")

	response, err := fws.bot.llm.GenerateChatResponse(
		llm.ResponseTypeFreeWillReaction,
		chatMsgs,
		float32(decisionTemperature),
	)

	if err != nil {
		log.Printf("[FreeWill] analyzeForReaction: Ошибка получения ответа от LLM: %v", err)
		return
	}

	log.Printf("[FreeWill] analyzeForReaction: Получен ответ от LLM: %s", response)

	// Парсим решение о реакции
	reactionDecision, err := fws.parseReactionDecision(response)
	if err != nil {
		log.Printf("[FreeWill] analyzeForReaction: Ошибка парсинга решения: %v", err)
		return
	}

	if !reactionDecision.ShouldReact {
		log.Printf("[INFO][FreeWill] ReactionDecision: chat=%d, message_id=%d, should_react=%t, reason=%q",
			chatID, msg.MessageID, reactionDecision.ShouldReact, reactionDecision.Reason)
		return
	}
	log.Printf("[INFO][FreeWill] ReactionDecision: chat=%d, message_id=%d, should_react=%t, reaction=%s, reason=%q",
		chatID, msg.MessageID, reactionDecision.ShouldReact, reactionDecision.Reaction, reactionDecision.Reason)

	// Ставим реакцию
	fws.setReaction(chatID, msg.MessageID, reactionDecision.Reaction)
}

// parseReactionDecision парсит решение LLM о реакции
func (fws *FreeWillService) parseReactionDecision(response string) (*FreeWillReactionDecision, error) {
	// Очищаем ответ от markdown
	cleanResponse := cleanJSONFromMarkdown(response)

	var decision FreeWillReactionDecision
	err := json.Unmarshal([]byte(cleanResponse), &decision)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга JSON решения о реакции: %w", err)
	}

	return &decision, nil
}

// canReact проверяет, можно ли поставить реакцию (cooldown + лимиты)
func (fws *FreeWillService) canReact(chatID int64) bool {
	fws.mutex.Lock()
	defer fws.mutex.Unlock()

	now := time.Now()

	// Ленивая инициализация map при необходимости
	if fws.lastReactionTimes == nil {
		fws.lastReactionTimes = make(map[int64]time.Time)
	}
	if fws.reactionHourResetTime == nil {
		fws.reactionHourResetTime = make(map[int64]time.Time)
	}
	if fws.reactionCountThisHour == nil {
		fws.reactionCountThisHour = make(map[int64]int)
	}

	// Проверяем cooldown
	if lastReaction, exists := fws.lastReactionTimes[chatID]; exists {
		if now.Sub(lastReaction) < fws.reactionsCooldownPeriod {
			log.Printf("[FreeWill] canReact: Cooldown не прошел для чата %d", chatID)
			return false
		}
	}

	// Проверяем часовой лимит, используя границы часа
	resetTime, exists := fws.reactionHourResetTime[chatID]
	if !exists || now.Truncate(time.Hour) != resetTime.Truncate(time.Hour) {
		// Наступил новый час - сбрасываем счетчик
		fws.reactionCountThisHour[chatID] = 0
		fws.reactionHourResetTime[chatID] = now.Truncate(time.Hour)
		log.Printf("[FreeWill] canReact: Сброс часового счетчика реакций для чата %d", chatID)
	}

	// Проверяем и инициализируем счетчик если нужно
	count := 0
	if existingCount, exists := fws.reactionCountThisHour[chatID]; exists {
		count = existingCount
	} else {
		fws.reactionCountThisHour[chatID] = 0
	}

	if count >= fws.reactionsMaxPerHour {
		log.Printf("[FreeWill] canReact: Достигнут часовой лимит реакций для чата %d", chatID)
		return false
	}

	// Обновляем счетчики под защитой мьютекса
	fws.lastReactionTimes[chatID] = now
	fws.reactionCountThisHour[chatID]++

	return true
}

// setReaction ставит реакцию на сообщение
func (fws *FreeWillService) setReaction(chatID int64, messageID int, reaction string) {
	log.Printf("[FreeWill] setReaction: Ставим реакцию %s на сообщение %d в чате %d", reaction, messageID, chatID)

	// Ставим реакцию через ReactionTracker
	if fws.bot.reactionTracker != nil {
		err := fws.bot.reactionTracker.SetBotReaction(chatID, messageID, reaction)
		if err != nil {
			log.Printf("[FreeWill] setReaction: Ошибка постановки реакции: %v", err)
			return
		}
	} else {
		log.Printf("[FreeWill] setReaction: ReactionTracker не инициализирован")
		return
	}

	log.Printf("[FreeWill] setReaction: Реакция %s успешно поставлена", reaction)
}

// checkImageGeneration проверяет и выполняет генерацию изображений в рамках Free Will
func (fws *FreeWillService) checkImageGeneration(chatID int64, decision *FreeWillDecision) {
	// Проверяем, что сервис генерации изображений доступен
	if fws.bot.imageGenerationService == nil || !fws.bot.imageGenerationService.IsEnabled() {
		return
	}

	// Собираем контекстные данные для принятия решения
	contextData := map[string]interface{}{
		"mood":            decision.Mood,
		"reply_type":      decision.ReplyType,
		"is_voice":        decision.IsVoice,
		"response_reason": decision.Reason,
		"decision_time":   time.Now(),
	}

	// Используем механизм принятия решений сервиса изображений
	shouldGenerate := fws.bot.imageGenerationService.DecisionMechanismShouldGenerate(chatID, contextData)

	if !shouldGenerate {
		log.Printf("[FreeWill] checkImageGeneration: Генерация изображения не требуется для чата %d", chatID)
		return
	}

	log.Printf("[FreeWill] checkImageGeneration: 🎨 Принято решение о генерации изображения для чата %d", chatID)

	// Запускаем генерацию изображения в отдельной горутине, чтобы не блокировать основной ответ
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		image, err := fws.bot.imageGenerationService.GenerateImageForChat(ctx, chatID, "personality_based")
		if err != nil {
			log.Printf("[FreeWill] checkImageGeneration: ❌ Ошибка генерации изображения для чата %d: %v", chatID, err)
			return
		}

		err = fws.bot.imageGenerationService.SendGeneratedImage(image)
		if err != nil {
			log.Printf("[FreeWill] checkImageGeneration: ❌ Ошибка отправки изображения для чата %d: %v", chatID, err)
			return
		}

		log.Printf("[FreeWill] checkImageGeneration: ✅ Изображение успешно сгенерировано и отправлено в чат %d", chatID)
	}()
}

// checkImageGenerationForAllChats проверяет возможность генерации изображений для всех активных чатов независимо от текстовых ответов
func (fws *FreeWillService) checkImageGenerationForAllChats() {
	if fws.imageGenerationMaxDecisionsPerInterval <= 0 {
		return
	}

	if fws.bot.imageGenerationService == nil || !fws.bot.imageGenerationService.IsEnabled() {
		return
	}

	fws.mutex.RLock()
	// Копируем данные под RLock для минимизации времени блокировки
	lastMessagesCopy := make(map[int64]time.Time)
	for chatID, lastTime := range fws.lastMessage {
		lastMessagesCopy[chatID] = lastTime
	}
	fws.mutex.RUnlock()

	now := time.Now()
	for chatID, lastMessageTime := range lastMessagesCopy {
		// Проверяем базовые условия для генерации изображений
		timeSinceLastMessage := now.Sub(lastMessageTime)

		// Генерируем изображения реже, чем текстовые ответы (минимальный интервал из конфигурации)
		if timeSinceLastMessage < fws.imageGenerationMinDecisionInterval {
			continue
		}

		// Проверяем лимиты генерации изображений
		if !fws.canGenerateImage(chatID) {
			continue
		}

		// Базовые контекстные данные для принятия решения
		contextData := map[string]interface{}{
			"decision_time":      now,
			"silence_duration":   timeSinceLastMessage,
			"generation_trigger": "silence_based",
		}

		// Используем механизм принятия решений сервиса изображений
		shouldGenerate := fws.bot.imageGenerationService.DecisionMechanismShouldGenerate(chatID, contextData)

		if !shouldGenerate {
			continue
		}

		log.Printf("[FreeWill] checkImageGenerationForAllChats: 🎨 Принято решение о генерации изображения для чата %d (тишина: %v)",
			chatID, timeSinceLastMessage)

		// Обновляем статистику генерации изображений
		fws.updateImageGenerationStats(chatID)

		// Запускаем генерацию изображения в отдельной горутине
		go func(cID int64) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			image, err := fws.bot.imageGenerationService.GenerateImageForChat(ctx, cID, "personality_based")
			if err != nil {
				log.Printf("[FreeWill] checkImageGenerationForAllChats: ❌ Ошибка генерации изображения для чата %d: %v", cID, err)
				return
			}

			err = fws.bot.imageGenerationService.SendGeneratedImage(image)
			if err != nil {
				log.Printf("[FreeWill] checkImageGenerationForAllChats: ❌ Ошибка отправки изображения для чата %d: %v", cID, err)
				return
			}

			log.Printf("[FreeWill] checkImageGenerationForAllChats: ✅ Изображение успешно сгенерировано и отправлено в чат %d", cID)
		}(chatID)
	}
}

// buildResponseChatContext builds a ChatML message array for Free Will response generation.
// Fetches up to limit messages from storage, formats them as ChatML, builds the system
// message via BuildChatSystemMessage, and returns the combined array.
// Returns nil (not empty slice) on storage error to simplify nil-checking at call sites.
func (fws *FreeWillService) buildResponseChatContext(chatID int64, limit int, targetUserID int64) []llm.ChatMessage {
	messages, err := fws.bot.storage.GetMessages(chatID, limit)
	if err != nil {
		log.Printf("[FreeWill] buildResponseChatContext: Ошибка получения сообщений: %v", err)
		return nil
	}

	formatter := NewUnifiedMessageFormatter(fws.bot.storage, fws.bot.config.TimeZone)
	formatter.SetDisableUserProfiles(fws.bot.config.DisableUserProfiles)
	chatHistory := formatter.FormatMessagesXML(chatID, messages)

	// Проверка конфликтов алиасов для ChatML-контекста
	var disambigWarning string
	if fws.bot.userValidator != nil {
		userIDs := collectUniqueUserIDs(messages)
		disambigWarning = fws.bot.userValidator.BuildDisambiguationWarning(chatID, userIDs)
	}

	var stateData *prompts.TemplateData
	if fws.bot.stateProvider != nil {
		stateData = fws.bot.stateProvider.CollectState(chatID, targetUserID)
	}

	systemMsg := fws.bot.BuildChatSystemMessage(chatID, targetUserID, stateData)
	if disambigWarning != "" {
		systemMsg.Content = disambigWarning + "\n\n" + systemMsg.Content
	}

	allMsgs := make([]llm.ChatMessage, 0, 2)
	allMsgs = append(allMsgs, systemMsg)
	if chatHistory != "" {
		allMsgs = append(allMsgs, llm.ChatMessage{Role: "user", Content: chatHistory})
	}

	return allMsgs
}

// collectUniqueUserIDs собирает уникальные ID пользователей из слайса сообщений.
func collectUniqueUserIDs(messages []*tgbotapi.Message) []int64 {
	seen := make(map[int64]bool)
	var ids []int64
	for _, msg := range messages {
		if msg != nil && msg.From != nil {
			uid := int64(msg.From.ID)
			if !seen[uid] {
				seen[uid] = true
				ids = append(ids, uid)
			}
		}
	}
	return ids
}
