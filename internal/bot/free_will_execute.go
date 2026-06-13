package bot

import (
	"log"
	"strings"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
)

// executeDecision выполняет принятое решение
func (fws *FreeWillService) executeDecision(chatID int64, decision *FreeWillDecision) {
	log.Printf("[INFO][FreeWill] FinalDecision: chat=%d, reply_type=%s, is_voice=%t, mood=%s, reason=%q",
		chatID, decision.ReplyType, decision.IsVoice, decision.Mood, decision.Reason)

	executionStart := time.Now()
	switch decision.ReplyType {
	case "direct_reply":
		log.Printf("[FreeWill] executeDecision: Отправляем прямой ответ для чата %d (target_message_id: %d)",
			chatID, decision.TargetMessageID)
		fws.sendDirectReply(chatID, decision)
	case "general":
		log.Printf("[FreeWill] executeDecision: Отправляем общее сообщение для чата %d", chatID)
		fws.sendGeneralMessage(chatID, decision)
	case "context_based":
		log.Printf("[FreeWill] executeDecision: Отправляем контекстное сообщение для чата %d", chatID)
		fws.sendContextBasedMessage(chatID, decision)
	case "silence_response":
		log.Printf("[FreeWill] executeDecision: Отправляем ответ на тишину для чата %d", chatID)
		fws.sendSilenceResponse(chatID, decision)
	case "mood_based":
		log.Printf("[FreeWill] executeDecision: Отправляем сообщение на основе настроения для чата %d", chatID)
		fws.sendMoodBasedMessage(chatID, decision)
	case "voice":
		log.Printf("[FreeWill] executeDecision: Отправляем голосовое сообщение для чата %d", chatID)
		fws.sendVoiceMessage(chatID, decision)
	case "take_response":
		log.Printf("[FreeWill] executeDecision: Отправляем ответ на тейк для чата %d", chatID)
		fws.sendTakeResponse(chatID, decision)
	default:
		log.Printf("[FreeWill] executeDecision: Неизвестный тип ответа для чата %d: %s", chatID, decision.ReplyType)
	}
	log.Printf("[FreeWill] executeDecision: Завершено выполнение решения для чата %d (время: %v)",
		chatID, time.Since(executionStart))
}

// sendDirectReply отправляет ответ на конкретное сообщение
func (fws *FreeWillService) sendDirectReply(chatID int64, decision *FreeWillDecision) {
	log.Printf("[FreeWill] sendDirectReply: Начинаем отправку прямого ответа для чата %d", chatID)

	if decision.TargetMessageID == 0 {
		log.Printf("[FreeWill] sendDirectReply: Нет target_message_id для прямого ответа в чате %d", chatID)
		return
	}

	// Получаем информацию о целевом сообщении для лучшего логирования
	targetMessageInfo := fws.getTargetMessageInfo(chatID, decision.TargetMessageID)
	log.Printf("[FreeWill] sendDirectReply: Целевое сообщение для ответа: %d (%s)", decision.TargetMessageID, targetMessageInfo)

	// Определяем targetUserID — автора сообщения, на которое отвечаем
	targetMsg, _ := fws.bot.storage.GetMessageByID(chatID, decision.TargetMessageID)
	var targetUserID int64
	if targetMsg != nil && targetMsg.From != nil {
		targetUserID = int64(targetMsg.From.ID)
	}

	// Получаем полный контекст с цепочкой ответов для прямого ответа
	log.Printf("[FreeWill] sendDirectReply: Получаем контекст для прямого ответа в чате %d", chatID)
	context, err := fws.getDirectReplyContext(chatID, decision.TargetMessageID)
	if err != nil {
		log.Printf("[FreeWill] sendDirectReply: Ошибка получения контекста для direct reply в чате %d: %v", chatID, err)
		return
	}
	log.Printf("[FreeWill] sendDirectReply: Контекст получен для чата %d (длина: %d символов)", chatID, len(context))

	// Детерминированная подсказка стиля на основе отношений с автором исходного сообщения
	if targetMsg != nil && targetMsg.From != nil {
		uid := int64(targetMsg.From.ID)
		before := len(context)
		context = fws.bot.ApplyRelationshipStyleToContext(chatID, uid, context)
		if len(context) > before {
			style := fws.bot.GetRelationshipInfluencedCommunicationStyle(chatID, uid)
			log.Printf("[INFO][FW-DR] Chat %d: Применен стиль общения на основе отношений (user %d): %s", chatID, uid, style)
		}
	} else {
		// Безопасный фолбэк: добавляем нейтральный подсказчик для детерминизма
		before := len(context)
		context = fws.bot.ApplyRelationshipStyleToContext(chatID, 0, context)
		if len(context) > before {
			log.Printf("[INFO][FW-DR] Chat %d: Применен стиль общения на основе отношений (fallback, без userID)", chatID)
		}
	}

	// Если analyzeDirectResponse уже сгенерировал текст (Этап 2) — используем его,
	// иначе делаем безопасный фолбэк с генерацией на месте.
	var text string
	if strings.TrimSpace(decision.Text) != "" {
		log.Printf("[FreeWill] sendDirectReply: Используем уже сгенерированный текст из ЭТАПА 2 для чата %d", chatID)
		text = decision.Text
	} else {
		// Генерируем текст ответа с учетом контекста (фолбэк)
		log.Printf("[FreeWill] sendDirectReply: Текст отсутствует в решении — выполняем локальную генерацию. Формируем промпт для чата %d", chatID)
		// Строим промпт и обогащаем личностью для консистентного тона
		prompt := fws.buildDirectReplyPrompt(chatID, decision)
		prompt = fws.bot.enrichPromptWithPersonality(prompt, chatID, "free_will_direct")
		log.Printf("[FreeWill] sendDirectReply: Промпт (enriched) сформирован для чата %d (длина: %d символов)", chatID, len(prompt))

		// Добавляем веб-поиск если включен
		fullContext := context
		if fws.bot.webSearch != nil && fws.bot.webSearch.IsEnabled() {
			log.Printf("[FreeWill] sendDirectReply: Smart веб-поиск для чата %d", chatID)
			// Пытаемся использовать целевое сообщение как основной запрос, затем — уже готовый текст (если есть)
			queryCandidate := ""
			if decision.Text != "" {
				queryCandidate = decision.Text
			}
			enhanced := fws.bot.webSearch.EnhanceContextWithSmartWebSearch(fullContext, queryCandidate)
			if enhanced != fullContext {
				fullContext = enhanced
				log.Printf("[FreeWill] sendDirectReply: Контекст расширен smart веб-поиском для direct reply в чате %d", chatID)
			} else {
				log.Printf("[FreeWill] sendDirectReply: Smart веб-поиск не потребовался для чата %d", chatID)
			}
		}

		log.Printf("[FreeWill] sendDirectReply: Генерируем текст ответа (fallback) для чата %d", chatID)

		// Build ChatML context
		chatMsgs := fws.buildResponseChatContext(chatID, fws.bot.config.ContextWindow, targetUserID)
		if chatMsgs == nil {
			return
		}
		// Prepend task prompt to system message
		chatMsgs[0].Content = prompt + "\n\n" + chatMsgs[0].Content

		// Web search enrichment (append to system message)
		if fws.bot.webSearch != nil && fws.bot.webSearch.IsEnabled() {
			if results := fws.bot.webSearch.SearchAndFormat(""); results != "" {
				chatMsgs[0].Content = chatMsgs[0].Content + "\n\n=== РЕЗУЛЬТАТЫ ПОИСКА ===\n" + results
			}
		}

		fws.bot.setTypingAction(chatID)
		textGenStart := time.Now()
		genText, err := fws.bot.llm.GenerateChatResponse(
			llm.ResponseTypeFreeWillDirect,
			chatMsgs,
			float32(responseTemperature), // 0.7 for creative text generation
		)
		textGenDuration := time.Since(textGenStart)
		if err != nil {
			log.Printf("[FreeWill] sendDirectReply: Ошибка генерации прямого ответа (fallback) для чата %d: %v", chatID, err)
			return
		}
		log.Printf("[FreeWill] sendDirectReply: Текст ответа (fallback) сгенерирован для чата %d (время: %v, длина: %d символов)",
			chatID, textGenDuration, len(genText))
		text = genText
	}

	originalText := text
	log.Printf("🧹 [FREE_WILL] Очистка DirectReply ответа для чата %d (исходная длина: %d)", chatID, len(text))
	text = cleanupLLMResponse(text)
	if originalText != text {
		log.Printf("[FreeWill] sendDirectReply: Текст очищен от служебных символов для чата %d", chatID)
	}
	log.Printf("[FreeWill] sendDirectReply: Финальный текст для чата %d: %s", chatID, text)

	if decision.IsVoice {
		log.Printf("[FreeWill] sendDirectReply: Отправляем голосовой ответ для чата %d", chatID)
		fws.sendVoiceReply(chatID, decision.TargetMessageID, text)
	} else {
		log.Printf("[FreeWill] sendDirectReply: Отправляем текстовый ответ для чата %d", chatID)
		// Используем обертку с анти-повторениями (userID 0 для Free Will)
		fws.bot.sendReplyToWithAntiRepetition(chatID, decision.TargetMessageID, text, 0, "free_will_direct")
	}

	log.Printf("[FreeWill] sendDirectReply: Завершена отправка прямого ответа для чата %d (reply to %d)",
		chatID, decision.TargetMessageID)
}

// sendGeneralMessage отправляет общее сообщение в чат (используя CONTEXT_WINDOW)
func (fws *FreeWillService) sendGeneralMessage(chatID int64, decision *FreeWillDecision) {
	log.Printf("[FreeWill] sendGeneralMessage: Начинаем отправку общего сообщения для чата %d", chatID)

	// Build ChatML context
	log.Printf("[FreeWill] sendGeneralMessage: Получаем ChatML контекст для чата %d", chatID)
	chatMsgs := fws.buildResponseChatContext(chatID, fws.bot.config.ContextWindow, 0)
	if chatMsgs == nil {
		log.Printf("[FreeWill] sendGeneralMessage: Ошибка получения контекста для general в чате %d", chatID)
		return
	}
	log.Printf("[FreeWill] sendGeneralMessage: Контекст получен для чата %d (%d сообщений)", chatID, len(chatMsgs))

	// Подмешиваем лёгкий ассоциативный контекст (зафичено флагом)
	if assoc := fws.bot.getAssociativeContext(chatID, fws.bot.buildAssociativeKeys(chatID), 3); assoc != "" {
		chatMsgs[0].Content = chatMsgs[0].Content + "\n\n" + assoc
	}

	log.Printf("[FreeWill] sendGeneralMessage: Формируем промпт для чата %d", chatID)
	prompt := fws.buildGeneralPrompt(chatID, decision)
	chatMsgs[0].Content = prompt + "\n\n" + chatMsgs[0].Content
	log.Printf("[FreeWill] sendGeneralMessage: Промпт сформирован для чата %d (длина: %d символов)", chatID, len(prompt))

	// Добавляем веб-поиск если включен
	if fws.bot.webSearch != nil && fws.bot.webSearch.IsEnabled() {
		log.Printf("[FreeWill] sendGeneralMessage: Smart веб-поиск для чата %d", chatID)
		if results := fws.bot.webSearch.SearchAndFormat("актуальные события"); results != "" {
			chatMsgs[0].Content = chatMsgs[0].Content + "\n\n=== РЕЗУЛЬТАТЫ ПОИСКА ===\n" + results
			log.Printf("[FreeWill] sendGeneralMessage: Контекст расширен smart веб-поиском для general в чате %d", chatID)
		} else {
			log.Printf("[FreeWill] sendGeneralMessage: Smart веб-поиск не потребовался для чата %d", chatID)
		}
	}

	log.Printf("[FreeWill] sendGeneralMessage: Генерируем текст общего сообщения для чата %d", chatID)
	fws.bot.setTypingAction(chatID)
	textGenStart := time.Now()
	text, err := fws.bot.llm.GenerateChatResponse(
		llm.ResponseTypeFreeWillGeneral,
		chatMsgs,
		float32(responseTemperature),
	)
	textGenDuration := time.Since(textGenStart)

	if err != nil {
		log.Printf("[FreeWill] sendGeneralMessage: Ошибка генерации общего сообщения для чата %d: %v", chatID, err)
		return
	}
	log.Printf("[FreeWill] sendGeneralMessage: Текст сгенерирован для чата %d (время: %v, длина: %d символов)",
		chatID, textGenDuration, len(text))

	originalText := text
	log.Printf("🧹 [FREE_WILL] Очистка GeneralMessage ответа для чата %d (исходная длина: %d)", chatID, len(text))
	text = cleanupLLMResponse(text)
	if originalText != text {
		log.Printf("[FreeWill] sendGeneralMessage: Текст очищен от служебных символов для чата %d", chatID)
	}
	log.Printf("[FreeWill] sendGeneralMessage: Финальный текст для чата %d: %s", chatID, text)

	if decision.IsVoice {
		log.Printf("[FreeWill] sendGeneralMessage: Отправляем голосовое общее сообщение для чата %d", chatID)
		fws.sendVoiceWithText(chatID, text)
	} else {
		log.Printf("[FreeWill] sendGeneralMessage: Отправляем текстовое общее сообщение для чата %d", chatID)
		// Используем обертку с анти-повторениями (userID 0 для Free Will general)
		fws.bot.sendReplyWithAntiRepetition(chatID, text, 0, "free_will_general")
	}

	log.Printf("[FreeWill] sendGeneralMessage: Завершена отправка общего сообщения для чата %d", chatID)
}

// sendContextBasedMessage отправляет сообщение на основе контекста (используя FREE_WILL_CONTEXT_WINDOW)
func (fws *FreeWillService) sendContextBasedMessage(chatID int64, decision *FreeWillDecision) {
	// Build ChatML context (uses FREE_WILL_CONTEXT_WINDOW)
	chatMsgs := fws.buildResponseChatContext(chatID, fws.contextWindow, 0)
	if chatMsgs == nil {
		log.Printf("[FreeWill] Ошибка получения контекста для context-based")
		return
	}

	// Подмешиваем лёгкий ассоциативный контекст (зафичено флагом)
	if assoc := fws.bot.getAssociativeContext(chatID, fws.bot.buildAssociativeKeys(chatID), 3); assoc != "" {
		chatMsgs[0].Content = chatMsgs[0].Content + "\n\n" + assoc
	}

	prompt := fws.buildContextPrompt(chatID, decision)
	chatMsgs[0].Content = prompt + "\n\n" + chatMsgs[0].Content

	// Добавляем веб-поиск если включен
	if fws.bot.webSearch != nil && fws.bot.webSearch.IsEnabled() {
		if results := fws.bot.webSearch.SearchAndFormat(decision.Text); results != "" {
			chatMsgs[0].Content = chatMsgs[0].Content + "\n\n=== РЕЗУЛЬТАТЫ ПОИСКА ===\n" + results
			log.Printf("[FreeWill] Контекст расширен smart веб-поиском для context-based")
		}
	}

	fws.bot.setTypingAction(chatID)
	text, err := fws.bot.llm.GenerateChatResponse(
		llm.ResponseTypeFreeWillContext,
		chatMsgs,
		float32(responseTemperature),
	)
	if err != nil {
		log.Printf("[FreeWill] Ошибка генерации контекстного сообщения: %v", err)
		return
	}

	log.Printf("🧹 [FREE_WILL] Очистка ContextBased ответа для чата %d (исходная длина: %d)", chatID, len(text))
	text = cleanupLLMResponse(text)

	if decision.IsVoice {
		fws.sendVoiceWithText(chatID, text)
	} else {
		// Используем обертку с анти-повторениями
		fws.bot.sendReplyWithAntiRepetition(chatID, text, 0, "free_will_context")
	}

	log.Printf("[FreeWill] Отправлено контекстное сообщение в чат %d", chatID)
}

// sendSilenceResponse отправляет сообщение для оживления тишины
func (fws *FreeWillService) sendSilenceResponse(chatID int64, decision *FreeWillDecision) {
	// Build ChatML context
	chatMsgs := fws.buildResponseChatContext(chatID, fws.bot.config.ContextWindow, 0)
	if chatMsgs == nil {
		log.Printf("[FreeWill] Ошибка получения контекста для silence")
		return
	}

	// Подмешиваем лёгкий ассоциативный контекст (зафичено флагом)
	if assoc := fws.bot.getAssociativeContext(chatID, fws.bot.buildAssociativeKeys(chatID), 3); assoc != "" {
		chatMsgs[0].Content = chatMsgs[0].Content + "\n\n" + assoc
	}

	prompt := fws.buildSilencePrompt(chatID, decision)
	chatMsgs[0].Content = prompt + "\n\n" + chatMsgs[0].Content

	// Smart веб-поиск: используем последние темы как кандидат
	if fws.bot.webSearch != nil && fws.bot.webSearch.IsEnabled() {
		if results := fws.bot.webSearch.SearchAndFormat("актуальные события"); results != "" {
			chatMsgs[0].Content = chatMsgs[0].Content + "\n\n=== РЕЗУЛЬТАТЫ ПОИСКА ===\n" + results
		}
	}

	fws.bot.setTypingAction(chatID)
	text, err := fws.bot.llm.GenerateChatResponse(
		llm.ResponseTypeFreeWillSilence,
		chatMsgs,
		float32(responseTemperature),
	)
	if err != nil {
		log.Printf("[FreeWill] Ошибка генерации ответа на тишину: %v", err)
		return
	}

	log.Printf("🧹 [FREE_WILL] Очистка Silence ответа для чата %d (исходная длина: %d)", chatID, len(text))
	text = cleanupLLMResponse(text)

	if decision.IsVoice {
		fws.sendVoiceWithText(chatID, text)
	} else {
		// Используем обертку с анти-повторениями
		fws.bot.sendReplyWithAntiRepetition(chatID, text, 0, "free_will_silence")
	}

	log.Printf("[FreeWill] Отправлен ответ на тишину в чат %d", chatID)
}

// sendTakeResponse отправляет развернутый ответ на тейк
func (fws *FreeWillService) sendTakeResponse(chatID int64, decision *FreeWillDecision) {
	log.Printf("[FreeWill] sendTakeResponse: Начинаем отправку ответа на тейк для чата %d", chatID)

	if decision.TargetMessageID == 0 {
		log.Printf("[FreeWill] sendTakeResponse: Нет target_message_id для ответа на тейк в чате %d", chatID)
		return
	}

	// Получаем информацию о целевом сообщении
	targetMessageInfo := fws.getTargetMessageInfo(chatID, decision.TargetMessageID)
	log.Printf("[FreeWill] sendTakeResponse: Целевое сообщение (тейк): %d (%s)", decision.TargetMessageID, targetMessageInfo)

	// Build ChatML context
	chatMsgs := fws.buildResponseChatContext(chatID, fws.bot.config.ContextWindow, 0)
	if chatMsgs == nil {
		log.Printf("[FreeWill] sendTakeResponse: Ошибка получения контекста для take response в чате %d", chatID)
		return
	}

	// Подмешиваем лёгкий ассоциативный контекст (зафичено флагом)
	if assoc := fws.bot.getAssociativeContext(chatID, fws.bot.buildAssociativeKeys(chatID), 3); assoc != "" {
		chatMsgs[0].Content = chatMsgs[0].Content + "\n\n" + assoc
	}

	// Строим промпт для ответа на тейк
	log.Printf("[FreeWill] sendTakeResponse: Формируем промпт для ответа на тейк в чате %d", chatID)
	prompt := fws.buildTakeResponsePrompt(chatID, decision)
	chatMsgs[0].Content = prompt + "\n\n" + chatMsgs[0].Content
	log.Printf("[FreeWill] sendTakeResponse: Промпт сформирован для чата %d (длина: %d символов)", chatID, len(prompt))

	// Добавляем веб-поиск для поддержки аргументов
	if fws.bot.webSearch != nil && fws.bot.webSearch.IsEnabled() {
		log.Printf("[FreeWill] sendTakeResponse: Smart веб-поиск для поддержки аргументов в чате %d", chatID)
		if results := fws.bot.webSearch.SearchAndFormat(decision.Text); results != "" {
			chatMsgs[0].Content = chatMsgs[0].Content + "\n\n=== РЕЗУЛЬТАТЫ ПОИСКА ===\n" + results
			log.Printf("[FreeWill] sendTakeResponse: Контекст расширен smart веб-поиском для take response в чате %d", chatID)
		}
	}

	log.Printf("[FreeWill] sendTakeResponse: Генерируем развернутый ответ на тейк для чата %d", chatID)
	fws.bot.setTypingAction(chatID)
	textGenStart := time.Now()
	text, err := fws.bot.llm.GenerateChatResponse(
		llm.ResponseTypeFreeWillTakeResponse,
		chatMsgs,
		float32(responseTemperature),
	)
	textGenDuration := time.Since(textGenStart)

	if err != nil {
		log.Printf("[FreeWill] sendTakeResponse: Ошибка генерации ответа на тейк для чата %d: %v", chatID, err)
		return
	}
	log.Printf("[FreeWill] sendTakeResponse: Текст ответа на тейк сгенерирован для чата %d (время: %v, длина: %d символов)",
		chatID, textGenDuration, len(text))

	// Очищаем ответ
	originalText := text
	text = cleanupLLMResponse(text)
	if originalText != text {
		log.Printf("[FreeWill] sendTakeResponse: Текст очищен от служебных символов для чата %d", chatID)
	}
	log.Printf("[FreeWill] sendTakeResponse: Финальный ответ на тейк для чата %d: %s", chatID, text)

	if decision.IsVoice {
		log.Printf("[FreeWill] sendTakeResponse: Отправляем голосовой ответ на тейк для чата %d", chatID)
		fws.sendVoiceReply(chatID, decision.TargetMessageID, text)
	} else {
		log.Printf("[FreeWill] sendTakeResponse: Отправляем текстовый ответ на тейк для чата %d", chatID)
		// Используем обертку с анти-повторениями для ответов на тейки
		fws.bot.sendReplyToWithAntiRepetition(chatID, decision.TargetMessageID, text, 0, "free_will_take_response")
	}

	log.Printf("[FreeWill] sendTakeResponse: Завершена отправка ответа на тейк для чата %d (reply to %d)",
		chatID, decision.TargetMessageID)
}

// sendMoodBasedMessage отправляет сообщение на основе настроения
func (fws *FreeWillService) sendMoodBasedMessage(chatID int64, decision *FreeWillDecision) {
	// Build ChatML context
	chatMsgs := fws.buildResponseChatContext(chatID, fws.bot.config.ContextWindow, 0)
	if chatMsgs == nil {
		log.Printf("[FreeWill] Ошибка получения контекста для mood-based")
		return
	}

	// Подмешиваем лёгкий ассоциативный контекст (зафичено флагом)
	if assoc := fws.bot.getAssociativeContext(chatID, fws.bot.buildAssociativeKeys(chatID), 3); assoc != "" {
		chatMsgs[0].Content = chatMsgs[0].Content + "\n\n" + assoc
	}

	prompt := fws.buildMoodBasedPrompt(chatID, decision)
	chatMsgs[0].Content = prompt + "\n\n" + chatMsgs[0].Content

	fws.bot.setTypingAction(chatID)
	text, err := fws.bot.llm.GenerateChatResponse(
		llm.ResponseTypeFreeWillMoodBasedMessage,
		chatMsgs,
		float32(responseTemperature),
	)
	if err != nil {
		log.Printf("[FreeWill] Ошибка генерации сообщения по настроению: %v", err)
		return
	}

	text = cleanupLLMResponse(text)

	if decision.IsVoice {
		fws.sendVoiceWithText(chatID, text)
	} else {
		// Используем обертку с анти-повторениями
		fws.bot.sendReplyWithAntiRepetition(chatID, text, 0, "free_will_mood")
	}

	log.Printf("[FreeWill] Отправлено сообщение по настроению в чат %d", chatID)
}

// sendVoiceMessage отправляет голосовое сообщение через Free Will
func (fws *FreeWillService) sendVoiceMessage(chatID int64, decision *FreeWillDecision) {
	// Build ChatML context
	chatMsgs := fws.buildResponseChatContext(chatID, fws.bot.config.ContextWindow, 0)
	if chatMsgs == nil {
		log.Printf("[FreeWill] Ошибка получения контекста для voice")
		return
	}

	// Подмешиваем лёгкий ассоциативный контекст (зафичено флагом)
	if assoc := fws.bot.getAssociativeContext(chatID, nil, 3); assoc != "" {
		chatMsgs[0].Content = chatMsgs[0].Content + "\n\n" + assoc
	}

	prompt := fws.buildVoicePrompt(chatID, decision)
	chatMsgs[0].Content = prompt + "\n\n" + chatMsgs[0].Content

	fws.bot.setTypingAction(chatID)
	text, err := fws.bot.llm.GenerateChatResponse(
		llm.ResponseTypeFreeWillVoiceMessage,
		chatMsgs,
		float32(responseTemperature),
	)
	if err != nil {
		log.Printf("[FreeWill] Ошибка генерации текста для голоса: %v", err)
		return
	}

	text = cleanupLLMResponse(text)
	fws.sendVoiceWithText(chatID, text)

	log.Printf("[FreeWill] Инициировано голосовое сообщение в чат %d", chatID)
}

// sendVoiceWithText отправляет голосовое сообщение с заданным текстом
func (fws *FreeWillService) sendVoiceWithText(chatID int64, text string) {
	log.Printf("[FreeWill] sendVoiceWithText: Отправляем голосовое сообщение для чата %d (текст: %s)", chatID, text)

	if fws.bot.voiceMessageService == nil {
		log.Printf("[FreeWill] sendVoiceWithText: VoiceMessageService недоступен для чата %d", chatID)
		return
	}

	log.Printf("[FreeWill] sendVoiceWithText: Запускаем генерацию голосового сообщения для чата %d", chatID)
	// Используем специальный метод для Free Will, который не зависит от VOICE_MESSAGES_ENABLED
	go fws.bot.voiceMessageService.generateAndSendVoiceMessageForFreeWill(chatID, text)
}

// sendVoiceReply отправляет голосовое сообщение как ответ на другое сообщение
func (fws *FreeWillService) sendVoiceReply(chatID int64, replyToMessageID int, text string) {
	log.Printf("[FreeWill] sendVoiceReply: Отправляем голосовой ответ для чата %d (reply to %d, текст: %s)",
		chatID, replyToMessageID, text)

	if fws.bot.voiceMessageService == nil {
		log.Printf("[FreeWill] sendVoiceReply: VoiceMessageService недоступен для чата %d", chatID)
		return
	}

	log.Printf("[FreeWill] sendVoiceReply: Запускаем генерацию голосового ответа для чата %d", chatID)
	// Используем специальный метод для Free Will с reply, который не зависит от VOICE_MESSAGES_ENABLED
	go fws.bot.voiceMessageService.generateAndSendVoiceMessageReplyForFreeWill(chatID, text, replyToMessageID)
}
