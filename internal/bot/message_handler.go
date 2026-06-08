package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleMessage обрабатывает обычные сообщения пользователей (не команды или коллбэки)
// Эта функция была значительно упрощена, основная логика вынесена.
func (b *Bot) handleMessage(update tgbotapi.Update) {
	startTime := time.Now()
	defer func() {
		log.Printf("[DEBUG][Timing] Обработка Message (ID: %d) заняла %s", update.Message.MessageID, time.Since(startTime))
	}()

	message := update.Message
	chatID := message.Chat.ID

	// Флаг для определения, что это сообщение от бота
	isBotMessage := message.From != nil && message.From.ID == b.api.Self.ID

	// Специальная обработка голосовых сообщений от бота для расшифровки
	if isBotMessage && message.Voice != nil {
		if b.config.Debug {
			log.Printf("[DEBUG][MH] Chat %d: Обрабатываю голосовое сообщение от бота (ID: %d, MessageID: %d) для расшифровки", chatID, message.From.ID, message.MessageID)
		}
		// Вызываем handleVoiceMessage для расшифровки
		err := b.handleVoiceMessage(message)
		if err != nil {
			log.Printf("[ERROR][MH] Error handling bot voice message %d in chat %d: %v", message.MessageID, message.Chat.ID, err)
		}
		// Сохраняем оригинальное голосовое сообщение от бота в БД
		b.storage.AddMessage(chatID, message)
		if b.config.Debug {
			log.Printf("[DEBUG][MH] Chat %d: Bot voice message ID %d saved to storage", chatID, message.MessageID)
		}
		return
	}

	if b.config.Debug {
		log.Printf("[DEBUG][MH] Chat %d: Entering handleMessage for message ID %d (from bot: %t).", chatID, message.MessageID, isBotMessage)
	}

	var textMessage *tgbotapi.Message // Сообщение для обработки (оригинальное или транскрибированное)

	// === Обработка голосовых сообщений (только от пользователей) ===
	if !isBotMessage && message.Voice != nil {
		// Вызываем handleVoiceMessage, который теперь возвращает только error
		err := b.handleVoiceMessage(message)
		if err != nil {
			log.Printf("[ERROR][MH] Error handling voice message %d in chat %d: %v", message.MessageID, message.Chat.ID, err)
			// Ошибка уже должна быть отправлена пользователю и залогирована в handleVoiceMessage
			return // Прекращаем дальнейшую обработку
		}
		// Если ошибки нет, значит, сообщение успешно транскрибировано и ОТПРАВЛЕНО в чат.
		// Дальнейшая обработка этого сообщения (как текстового) здесь не нужна.
		// Однако, нам все еще нужно сохранить оригинальное сообщение в БД.
		// Поэтому мы НЕ выходим из handleMessage, а просто устанавливаем textMessage = message
		textMessage = message // Используем оригинальное сообщение для сохранения в БД
		log.Printf("[DEBUG][MH] Voice message %d processed and sent by handleVoiceMessage. Proceeding to save original message.", message.MessageID)
	} else if !isBotMessage && message.Photo != nil && len(message.Photo) > 0 {
		// === Обработка фотографий (только от пользователей) ===
		err := b.handlePhotoMessage(context.Background(), message)
		if err != nil {
			log.Printf("[ERROR][MH] Error handling photo message %d in chat %d: %v", message.MessageID, message.Chat.ID, err)
			// Ошибка уже должна быть отправлена пользователю в handlePhotoMessage
			return // Прекращаем дальнейшую обработку
		}
		// Аналогично голосовым сообщениям, сохраняем оригинальное сообщение
		textMessage = message
		log.Printf("[DEBUG][MH] Photo message %d processed and sent by handlePhotoMessage. Proceeding to save original message.", message.MessageID)
	} else {
		// Если это не голосовое и не фото, используем оригинальное сообщение
		textMessage = message
	}
	// === Конец обработки голосовых и фото ===

	// Теперь используем textMessage для дальнейшей обработки
	// (В случае голоса/фото это будет оригинальное сообщение для сохранения)
	if textMessage == nil {
		log.Printf("[ERROR][MH] textMessage is nil after voice/photo handling check for update %d", update.UpdateID)
		return
	}

	// Обновляем ключевые переменные на основе textMessage (оригинального сообщения)
	message = textMessage    // Используем textMessage как основное сообщение далее
	chatID = message.Chat.ID // Убедимся, что chatID актуален
	// username := message.From.UserName // Обновим username на всякий случай

	// === Settings Read Start ===
	b.settingsMutex.RLock() // Use RLock for reading settings
	settings, exists := b.chatSettings[chatID]
	localIsActive := exists && settings.Active
	localPendingSetting := ""
	localSrachEnabled := false
	localVoiceEnabled := b.config.VoiceTranscriptionEnabledDefault

	needsReset := false // Flag to indicate if PendingSetting needs reset later - Restored

	if exists {
		localPendingSetting = settings.PendingSetting

		// --- Читаем настройки из БД для Srach и Voice ---
		// Не разблокируем мьютекс настроек памяти здесь
		// Вместо этого прочитаем настройки БД внутри RLock
		dbSettings, errDb := b.storage.GetChatSettings(chatID)
		if errDb == nil && dbSettings != nil { // Успешно получили настройки из БД
			if dbSettings.SrachAnalysisEnabled != nil {
				localSrachEnabled = *dbSettings.SrachAnalysisEnabled
			}
			if dbSettings.VoiceTranscriptionEnabled != nil {
				localVoiceEnabled = *dbSettings.VoiceTranscriptionEnabled
			}
		} else { // Ошибка чтения или нет настроек в БД
			if errDb != nil {
				log.Printf("[ERROR][MH] Chat %d: Ошибка получения настроек из DB внутри RLock: %v. Используем дефолты.", chatID, errDb)
			} else {
				// Настроек нет, используем дефолты (уже установлены)
			}
			localSrachEnabled = b.config.SrachAnalysisEnabled
			localVoiceEnabled = b.config.VoiceTranscriptionEnabledDefault
		}
		// --- Конец чтения Srach/Voice из БД ---

		// Determine if we need to reset PendingSetting based on its current value
		if localPendingSetting != "" {
			needsReset = true
			log.Printf("[DEBUG][MH Pending Check] Чат %d: Обнаружен PendingSetting '%s'. Установлен флаг needsReset.", chatID, localPendingSetting)
			// No need to reset here, just set the flag
		}

	} else {
		// Settings don't exist for this chat yet (should have been created by ensureChatInitialized)
		// We can potentially log a warning here if this state is unexpected
		log.Printf("[WARN][MH] Chat %d: Настройки чата не найдены во время чтения в handleMessage. Используем дефолты.", chatID)
		localSrachEnabled = b.config.SrachAnalysisEnabled
		localVoiceEnabled = b.config.VoiceTranscriptionEnabledDefault
	}
	// Single RUnlock after all necessary values are read
	b.settingsMutex.RUnlock() // (Unlock 1a)
	// === Settings Read Complete, Lock Released ===

	if b.config.Debug {
		log.Printf("[DEBUG][MH] Chat %d: Read settings (Active: %t, Pending: '%s', Srach: %t, Voice: %t). Lock released.",
			chatID, localIsActive, localPendingSetting, localSrachEnabled, localVoiceEnabled)
	}

	// Если сообщение пришло от пользователя (НЕ от бота) и ожидается ввод для настройки
	var pendingSettingKey string
	if !isBotMessage {
		b.settingsMutex.RLock()
		if settings, exists := b.chatSettings[chatID]; exists {
			pendingSettingKey = settings.PendingSetting
		}
		b.settingsMutex.RUnlock()

		if pendingSettingKey != "" && message.Text != "" && !strings.HasPrefix(message.Text, "/") && b.isAdmin(message.From) {
			if b.config.Debug {
				log.Printf("[DEBUG][MH] Chat %d User %d (%s): Обнаружен ожидаемый ввод для ключа '%s'. Текст: '%s'. Вызов handlePendingSettingInput...", chatID, message.From.ID, message.From.UserName, pendingSettingKey, message.Text)
			}
			// Вызываем обработчик из input_handler.go (или settings_input_handler.go)
			err := b.handlePendingSettingInput(chatID, message.From.ID, message.From.UserName, pendingSettingKey, message.Text, message.MessageID)
			if err != nil {
				log.Printf("[WARN][MH] Chat %d User %d: Ошибка обработки ожидаемого ввода для '%s': %v", chatID, message.From.ID, pendingSettingKey, err)
				// Не выходим, возможно, нужно обработать как обычное сообщение?
			} else {
				// Если ввод успешно обработан, выходим
				if b.config.Debug {
					log.Printf("[DEBUG][MH] Chat %d: Ожидаемый ввод для '%s' успешно обработан. Прерываем дальнейшую обработку сообщения.", chatID, pendingSettingKey)
				}
				return
			}
		}
	}

	// Continue normal logic if no pending setting was handled

	// --- Check Activity ---
	if !localIsActive { // Use local variable read earlier
		if b.config.Debug {
			log.Printf("[DEBUG][MH] Chat %d: Bot is inactive. Exiting handleMessage.", chatID)
		}
		return // If bot is inactive, exit
	}

	// === ВЫЗОВ СЕРВИСА МОДЕРАЦИИ (для всех сообщений) ===
	if b.moderation != nil {
		b.moderation.ProcessIncomingMessage(message)
	}
	// === КОНЕЦ ВЫЗОВА СЕРВИСА МОДЕРАЦИИ ===

	// Добавляем сообщение в хранилище (если оно еще не сохранено)
	// Для сообщений бота - они уже сохранены при отправке, но это не страшно (дубликаты отфильтруются по MessageID)
	b.storage.AddMessage(chatID, message)
	if b.config.Debug {
		log.Printf("[DEBUG][MH] Chat %d: Message ID %d (IsVoice: %t, FromBot: %t) added/updated in storage.", chatID, message.MessageID, message.Voice != nil, isBotMessage)
	}

	// Увеличиваем счетчик сообщений для голосовых сообщений (для всех сообщений)
	if b.voiceMessageService != nil {
		b.voiceMessageService.OnMessage(chatID)
	}

	// Обновляем профиль пользователя (ТОЛЬКО для сообщений НЕ от бота)
	if !isBotMessage && message.From != nil {
		b.updateUserProfileIfNeeded(chatID, message.From, message.Date)
	}

	// === ВЫЗОВ FREE WILL - переместим после проверки прямых обращений ===
	// Free Will будет вызван либо через OnMessage, либо через OnDirectMention,
	// но не для одного и того же сообщения дважды

	// === Остальная обработка ТОЛЬКО для сообщений от пользователей ===
	if isBotMessage {
		if b.config.Debug {
			log.Printf("[DEBUG][MH] Chat %d: Завершаем обработку сообщения от бота (ID: %d) - остальная логика пропускается", chatID, message.MessageID)
		}
		return // Для сообщений бота прекращаем обработку здесь
	}

	// === Код ниже выполняется ТОЛЬКО для сообщений пользователей ===

	// --- Check for Direct Reply / Mention ---
	if b.config.Debug {
		log.Printf("[DEBUG][MH] Chat %d: Checking for reply to bot or mention.", chatID)
	}
	// Используем message для проверки
	isReplyToBot := message.ReplyToMessage != nil && message.ReplyToMessage.From != nil && message.ReplyToMessage.From.ID == b.api.Self.ID
	mentionsBot := message.Entities != nil && func() bool {
		for _, entity := range message.Entities {
			if entity.Type == "mention" {
				mention := message.Text[entity.Offset : entity.Offset+entity.Length]
				if mention == "@"+b.api.Self.UserName {
					return true
				}
			}
		}
		return false
	}()

	// Проверяем обращение по имени из BOT_NAMES
	mentionsByName := b.checkMentionByBotNames(message.Text)

	// Определяем есть ли прямое обращение к боту
	hasDirectMention := isReplyToBot || mentionsBot || mentionsByName

	if hasDirectMention {
		// Проверяем включен ли DIRECT_PROMPT
		if b.config.DirectPromptEnabled {
			// Используем классический DIRECT_PROMPT
			if b.config.Debug {
				log.Printf("[DEBUG][MH] Chat %d: IsReplyToBot: %t, MentionsBot: %t. Checking direct reply limit.", chatID, isReplyToBot, mentionsBot)
			}
			// Проверяем лимит прямых ответов
			limitEnabled, _, _ := b.getDirectReplyLimitSettings(chatID) // Используем _ для неиспользуемых count и duration
			if limitEnabled {
				if !b.checkDirectReplyLimit(chatID, message.From.ID) { // ИНВЕРТИРОВАНО: ! checkDirectReplyLimit возвращает false, если лимит превышен
					if b.config.Debug {
						log.Printf("[DEBUG][MH] Chat %d: Direct reply limit EXCEEDED.", chatID)
					}
					b.sendDirectLimitExceededReply(chatID, message.MessageID)
					// Выход после обработки прямого ответа (лимит превышен)
					log.Printf("[DEBUG][MH EXIT POINT] Chat %d: Reached EXIT point after direct reply/mention (limit exceeded).", chatID)
					return
				} else {
					// Лимит не превышен, продолжаем с прямым ответом
					if b.config.Debug {
						log.Printf("[DEBUG][MH] Chat %d: Direct reply limit NOT exceeded. Proceeding with direct response.", chatID)
					}
				}
			}

			// Если лимит выключен ИЛИ не превышен - отправляем прямой ответ
			b.sendDirectResponse(chatID, message)
			// Выход после обработки прямого ответа
			log.Printf("[DEBUG][MH EXIT POINT] Chat %d: Reached EXIT point after direct reply/mention (sent direct response).", chatID)
			return
		} else {
			// DIRECT_PROMPT отключен, но есть прямое обращение
			// Передаем в Free Will для принятия решения
			if b.config.Debug {
				log.Printf("[DEBUG][MH] Chat %d: DIRECT_PROMPT отключен, передаем обращение по имени/reply в Free Will Direct Response", chatID)
			}
			if b.freeWillService != nil {
				b.freeWillService.OnDirectMention(chatID, message)
			}
			// ✅ ДЕЛАЕМ return здесь - Free Will принял решение, не должно быть дополнительных ответов
			log.Printf("[DEBUG][MH EXIT POINT] Chat %d: Reached EXIT point after Free Will Direct Response", chatID)
			return
		}
	}

	// === ВЫЗОВ FREE WILL для сообщений БЕЗ прямого обращения ===
	// Если не было прямого обращения, передаем сообщение в обычный Free Will
	if !hasDirectMention && !isBotMessage && b.freeWillService != nil {
		if b.config.Debug {
			log.Printf("[DEBUG][MH] 🤖 Передаем сообщение без прямого обращения в Free Will: чат:%d пользователь:%d", chatID, message.From.ID)
		}
		b.freeWillService.OnMessage(chatID, message)
	} else if !hasDirectMention && !isBotMessage && b.freeWillService == nil {
		if b.config.Debug {
			log.Printf("[DEBUG][MH] ❌ freeWillService = nil, пропускаем обработку Free Will")
		}
	}

	// --- Check AI Response Condition ---
	if b.config.Debug {
		log.Printf("[DEBUG][MH] Chat %d: No direct mention. Checking conditions for AI response.", chatID)
	}
	b.settingsMutex.Lock()
	settings, exists = b.chatSettings[chatID] // Перепроверяем settings под мьютексом
	shouldReply := false
	if exists && settings.Active && b.config.IntervalMessagesEnabled {
		settings.MessageCount++
		log.Printf("[DEBUG][MH] Chat %d: Message count incremented to %d (target: %d).", chatID, settings.MessageCount, settings.NextTargetMessageCount)
		if settings.MessageCount >= settings.NextTargetMessageCount {
			shouldReply = true
			settings.MessageCount = 0
			// Генерируем новый таргет для следующего раза
			settings.NextTargetMessageCount = settings.MinMessages + b.randSource.Intn(settings.MaxMessages-settings.MinMessages+1)
			settings.LastMessageID = message.MessageID
			log.Printf("[DEBUG][MH] Chat %d: AI reply condition met (Count >= Target). Resetting count. New target: %d. LastMessageID set to %d.", chatID, settings.NextTargetMessageCount, settings.LastMessageID)
		} else {
			log.Printf("[DEBUG][MH] Chat %d: Checking AI reply condition: Count(%d) < Target(%d)", chatID, settings.MessageCount, settings.NextTargetMessageCount)
		}
	}
	b.settingsMutex.Unlock()
	log.Printf("[DEBUG][MH] Chat %d: Settings mutex unlocked after AI response check. ShouldReply: %t.", chatID, shouldReply)

	if shouldReply {
		// Запускаем генерацию ответа AI в отдельной горутине
		go b.sendAIResponse(chatID)
	}

	// --- Сброс PendingSetting, если он был установлен и обработан (или просто обнаружен) ---
	if needsReset {
		b.settingsMutex.Lock()
		if settings, exists := b.chatSettings[chatID]; exists {
			if settings.PendingSetting != "" { // Дополнительная проверка, чтобы не логировать лишний раз
				log.Printf("[DEBUG][MH Pending Reset] Чат %d: Сброс PendingSetting (был '%s').", chatID, settings.PendingSetting)
				settings.PendingSetting = ""
				// settings.PendingSettingUserID = 0 // Поле удалено
			}
		}
		b.settingsMutex.Unlock()
	}

	// === Обновление личности с вероятностью 1% ===
	if b.randSource.Intn(100) == 0 {
		if b.config.Debug {
			log.Printf("[DEBUG][MH] Chat %d: Запуск обновления personality_memory (вероятностно 1/100)", chatID)
		}
		go b.updatePersonalityForChat(chatID)
	}

	// === Затухание каузальной памяти с вероятностью 0.5% ===
	if b.randSource.Intn(200) == 0 {
		if b.config.Debug {
			log.Printf("[DEBUG][MH] Chat %d: Запуск затухания каузальной памяти (вероятностно 1/200)", chatID)
		}
		go b.ApplyRelevanceDecayToCausalMemory(chatID)
	}

	// === ЭМОЦИОНАЛЬНАЯ ОБРАТНАЯ СВЯЗЬ (ЭТАП 2) ===
	// Проверяем, является ли это сообщение потенциальной обратной связью на ответ бота
	if !isBotMessage && message.From != nil && message.ReplyToMessage != nil {
		// Проверяем, отвечает ли пользователь на сообщение бота
		if message.ReplyToMessage.From != nil && message.ReplyToMessage.From.ID == b.api.Self.ID {
			// Асинхронно обрабатываем эмоциональную обратную связь
			go func() {
				botResponseText := message.ReplyToMessage.Text
				if message.ReplyToMessage.Voice != nil {
					// Если это голосовое сообщение, используем заглушку
					botResponseText = "[Голосовое сообщение]"
				}

				err := b.ProcessEmotionalFeedback(chatID, message.From.ID, botResponseText, message)
				if err != nil {
					log.Printf("[DEBUG][EF] Ошибка обработки эмоциональной обратной связи: %v", err)
				}
			}()
		}
	}

	// === ИНТЕГРАЦИЯ НОВЫХ АРХИТЕКТУР ===
	if !isBotMessage && message.From != nil {
		go func() {
			// Определяем тип взаимодействия
			interactionType := "message"
			if message.ReplyToMessage != nil && message.ReplyToMessage.From != nil && message.ReplyToMessage.From.ID == b.api.Self.ID {
				interactionType = "reply_to_bot"
			}
			if hasDirectMention {
				interactionType = "direct_mention"
			}
			if strings.Contains(strings.ToLower(message.Text), "спасибо") || strings.Contains(strings.ToLower(message.Text), "хорошо") {
				interactionType = "positive_reaction"
			}
			if strings.Contains(strings.ToLower(message.Text), "плохо") || strings.Contains(strings.ToLower(message.Text), "глуп") {
				interactionType = "negative_reaction"
			}

			// Определяем тональность сообщения (простейший анализ)
			sentiment := "neutral"
			messageTextLower := strings.ToLower(message.Text)
			if strings.Contains(messageTextLower, "хорошо") || strings.Contains(messageTextLower, "спасибо") || strings.Contains(messageTextLower, "отлично") {
				sentiment = "positive"
			} else if strings.Contains(messageTextLower, "плохо") || strings.Contains(messageTextLower, "ужасно") || strings.Contains(messageTextLower, "глуп") {
				sentiment = "negative"
			}

			// Обновляем отношения (социальная архитектура)
			b.UpdateRelationshipFromInteraction(chatID, message.From.ID, interactionType, sentiment, message.Text)

			// Обновляем внутренний монолог (когнитивная архитектура)
			if b.config.InternalMonologueEnabled {
				b.InternalMonologue(chatID, message.Text, fmt.Sprintf("Пользователь %s написал: %s", message.From.UserName, message.Text))
			}

			// Периодическая саморефлексия (раз в 50 сообщений)
			if b.config.SelfReflectionEnabled && b.randSource.Intn(50) == 0 {
				b.SelfReflection(chatID)
			}
		}()
	}
}

// Commenting out moved function sendReplyAndDeleteAfter
/*
func (b *Bot) sendReplyAndDeleteAfter(chatID int64, text string, delay time.Duration) (*tgbotapi.Message, error) {
	// ... implementation ...
}
*/

// checkMentionByBotNames проверяет есть ли обращение к боту по именам из BOT_NAMES
func (b *Bot) checkMentionByBotNames(messageText string) bool {
	if len(b.config.BotNames) == 0 {
		return false
	}

	messageTextLower := strings.ToLower(messageText)

	for _, name := range b.config.BotNames {
		nameLower := strings.ToLower(strings.TrimSpace(name))
		if nameLower == "" {
			continue
		}

		// Проверяем содержит ли сообщение имя бота
		if strings.Contains(messageTextLower, nameLower) {
			if b.config.Debug {
				log.Printf("[DEBUG][MH] Найдено обращение по имени '%s' в сообщении: %s", name, messageText)
			}
			return true
		}
	}

	return false
}

// --- Неиспользуемая функция downloadFile удалена --- //

/*
// checkDirectReplyLimit была перемещена в responder.go
func (b *Bot) checkDirectReplyLimit(chatID int64, userID int64) bool {
	// ... (код перемещен)
}
*/

/*
// sendDirectLimitExceededReply была перемещена в responder.go
func (b *Bot) sendDirectLimitExceededReply(chatID int64, replyToMessageID int) {
	// ... (код перемещен)
}
*/

/*
// updateUserProfileIfNeeded была перемещена в profile_handler.go
func (b *Bot) updateUserProfileIfNeeded(chatID int64, user *tgbotapi.User, messageDate int) {
    // ... (код удален)
}
*/

func (b *Bot) generateDailyTake(chatID int64) {
	prompt := b.enrichPromptWithPersonality(b.config.DailyTakePrompt, chatID, "daily_take")
	topic, err := b.llm.GenerateResponseByType(llm.ResponseTypePersonalityTopic, prompt, "", float32(b.config.GeminiTemperatureNormal))
	if err != nil {
		log.Printf("[ERROR][generateDailyTake] Ошибка генерации темы дня для %d: %v", chatID, err)
		return
	}

	// Очищаем ответ от возможных метаданных перед отправкой
	topic = cleanupLLMResponse(topic)

	// Отправляем тему дня
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🔥 Тема дня: %s", topic))
	_, err = b.api.Send(msg)
	if err != nil {
		log.Printf("[ERROR][generateDailyTake] Ошибка отправки темы дня для %d: %v", chatID, err)
	}
}
