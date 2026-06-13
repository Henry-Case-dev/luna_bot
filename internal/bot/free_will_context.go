package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// initializeLastMessageFromDatabase инициализирует lastMessage временем последнего сообщения из БД для всех чатов
func (fws *FreeWillService) initializeLastMessageFromDatabase() {
	log.Printf("[FreeWill] initializeLastMessageFromDatabase: 🔧 Получаем список всех чатов...")

	// Проверяем что storage доступен (для тестов может быть nil)
	if fws.bot.storage == nil {
		log.Printf("[FreeWill] initializeLastMessageFromDatabase: ⚠️ Storage недоступен, пропускаем инициализацию")
		return
	}

	// Получаем список всех чатов из БД
	chatIDs, err := fws.bot.storage.GetAllChatIDs()
	if err != nil {
		log.Printf("[FreeWill] initializeLastMessageFromDatabase: ❌ Ошибка получения списка чатов: %v", err)
		return
	}

	log.Printf("[FreeWill] initializeLastMessageFromDatabase: 📊 Найдено %d чатов для инициализации", len(chatIDs))

	initializedCount := 0
	for _, chatID := range chatIDs {
		// Получаем последнее сообщение для каждого чата
		messages, err := fws.bot.storage.GetMessages(chatID, 1)
		if err != nil {
			log.Printf("[FreeWill] initializeLastMessageFromDatabase: ❌ Ошибка получения последнего сообщения для чата %d: %v", chatID, err)
			continue
		}

		if len(messages) > 0 {
			lastMessage := messages[0]
			lastMessageTime := time.Unix(int64(lastMessage.Date), 0)
			fws.lastMessage[chatID] = lastMessageTime
			initializedCount++

			log.Printf("[FreeWill] initializeLastMessageFromDatabase: ✅ Чат %d: последнее сообщение %v (ID: %d)",
				chatID, lastMessageTime.Format("15:04:05 02.01.2006"), lastMessage.MessageID)
		} else {
			log.Printf("[FreeWill] initializeLastMessageFromDatabase: ⚠️ Чат %d: нет сообщений в БД", chatID)
		}
	}

	log.Printf("[FreeWill] initializeLastMessageFromDatabase: 🎯 Инициализация завершена: %d/%d чатов", initializedCount, len(chatIDs))
}

// getContextForAnalysis получает полный контекст чата для анализа Free Will
func (fws *FreeWillService) getContextForAnalysis(chatID int64) (string, error) {
	// Получаем последние сообщения из истории для анализа
	messages, err := fws.bot.storage.GetMessages(chatID, fws.contextWindow)
	if err != nil {
		return "", fmt.Errorf("ошибка получения истории: %w", err)
	}

	if len(messages) == 0 {
		return "Нет доступной истории сообщений", nil
	}

	// Берем последние N сообщений (если их больше чем нужно)
	if len(messages) > fws.contextWindow {
		messages = messages[len(messages)-fws.contextWindow:]
	}

	// Получаем релевантные сообщения из долгосрочной памяти, если включена
	var relevantMessages []*tgbotapi.Message
	if fws.bot.config.LongTermMemoryEnabled && len(messages) > 0 {
		// Используем последнее сообщение как запрос для поиска релевантного контекста
		lastMessage := messages[len(messages)-1]
		if lastMessage.Text != "" {
			relevantMsgs, err := fws.bot.storage.SearchRelevantMessages(chatID, lastMessage.Text, fws.bot.config.LongTermMemoryFetchK)
			if err != nil {
				log.Printf("[FreeWill] Ошибка поиска релевантных сообщений: %v", err)
			} else {
				relevantMessages = relevantMsgs
			}
		}
	}
	_ = relevantMessages

	// Используем ChatML-форматирование для Decision Stage
	formatter := NewUnifiedMessageFormatter(fws.bot.storage, fws.bot.config.TimeZone)
	formatter.SetDisableUserProfiles(fws.bot.config.DisableUserProfiles)
	chatMessages := formatter.FormatSortedChatMessages(chatID, messages)

	// Build system prompt for Decision stage
	systemPrompt := fws.buildDecisionSystemPrompt(chatID)
	allMsgs := make([]llm.ChatMessage, 0, len(chatMessages)+1)
	allMsgs = append(allMsgs, llm.ChatMessage{Role: "system", Content: systemPrompt})
	allMsgs = append(allMsgs, chatMessages...)

	// Flatten to string for backward compatibility
	context := systemPrompt + "\n\n" + llm.FlattenChatMessages(allMsgs)

	// Применяем изоляцию контекста для групповых чатов
	if fws.bot.contextIsolator != nil {
		isolationType := fws.bot.contextIsolator.DetermineIsolationType(chatID, nil, 0)
		context = fws.bot.contextIsolator.IsolateContext(chatID, 0, isolationType, context)
	}

	return context, nil
}

// getGeneralContext получает контекст для общих сообщений (используя CONTEXT_WINDOW)
func (fws *FreeWillService) getGeneralContext(chatID int64) (string, error) {
	log.Printf("[FreeWill] getGeneralContext: Начинаем получение общего контекста для чата %d", chatID)

	// Для general сообщений используем стандартное окно контекста
	messages, err := fws.bot.storage.GetMessages(chatID, fws.bot.config.ContextWindow)
	if err != nil {
		log.Printf("[FreeWill] getGeneralContext: Ошибка получения сообщений: %v", err)
		return "", fmt.Errorf("ошибка получения истории: %w", err)
	}

	log.Printf("[FreeWill] getGeneralContext: Получено %d сообщений из хранилища для чата %d", len(messages), chatID)

	if len(messages) == 0 {
		log.Printf("[FreeWill] getGeneralContext: Нет сообщений в истории для чата %d", chatID)
		return "Нет доступной истории сообщений", nil
	}

	// Берем последние сообщения
	if len(messages) > fws.bot.config.ContextWindow {
		messages = messages[len(messages)-fws.bot.config.ContextWindow:]
		log.Printf("[FreeWill] getGeneralContext: Обрезано до %d последних сообщений для чата %d", len(messages), chatID)
	}

	// Получаем релевантные сообщения из долгосрочной памяти
	var relevantMessages []*tgbotapi.Message
	if fws.bot.config.LongTermMemoryEnabled && len(messages) > 0 {
		lastMessage := messages[len(messages)-1]
		if lastMessage.Text != "" {
			log.Printf("[FreeWill] getGeneralContext: Ищем релевантные сообщения для чата %d", chatID)
			relevantMsgs, err := fws.bot.storage.SearchRelevantMessages(chatID, lastMessage.Text, fws.bot.config.LongTermMemoryFetchK)
			if err != nil {
				log.Printf("[FreeWill] getGeneralContext: Ошибка поиска релевантных сообщений для general: %v", err)
			} else {
				relevantMessages = relevantMsgs
				log.Printf("[FreeWill] getGeneralContext: Найдено %d релевантных сообщений для чата %d", len(relevantMessages), chatID)
			}
		}
	}

	log.Printf("[FreeWill] getGeneralContext: Вызываем formatDirectReplyContext для чата %d", chatID)
	result := formatDirectReplyContext(chatID, nil, nil, messages, relevantMessages, fws.bot.storage, fws.bot.config, fws.bot.config.TimeZone)

	log.Printf("[FreeWill] getGeneralContext: Результат форматирования для чата %d (длина: %d символов): %.200s...", chatID, len(result), result)

	return result, nil
}

// getContextBasedContext получает расширенный контекст для контекстных сообщений (используя FREE_WILL_CONTEXT_WINDOW)
func (fws *FreeWillService) getContextBasedContext(chatID int64) (string, error) {
	// Для context-based сообщений используем расширенное окно контекста
	messages, err := fws.bot.storage.GetMessages(chatID, fws.contextWindow)
	if err != nil {
		return "", fmt.Errorf("ошибка получения истории: %w", err)
	}

	if len(messages) == 0 {
		return "Нет доступной истории сообщений", nil
	}

	// Берем последние сообщения (больше чем для general)
	if len(messages) > fws.contextWindow {
		messages = messages[len(messages)-fws.contextWindow:]
	}

	// Получаем релевантные сообщения из долгосрочной памяти
	var relevantMessages []*tgbotapi.Message
	if fws.bot.config.LongTermMemoryEnabled && len(messages) > 0 {
		lastMessage := messages[len(messages)-1]
		if lastMessage.Text != "" {
			relevantMsgs, err := fws.bot.storage.SearchRelevantMessages(chatID, lastMessage.Text, fws.bot.config.LongTermMemoryFetchK)
			if err != nil {
				log.Printf("[FreeWill] Ошибка поиска релевантных сообщений для context: %v", err)
			} else {
				relevantMessages = relevantMsgs
			}
		}
	}

	return formatDirectReplyContext(chatID, nil, nil, messages, relevantMessages, fws.bot.storage, fws.bot.config, fws.bot.config.TimeZone), nil
}

// getDirectReplyContext получает полный контекст для прямого ответа на сообщение
func (fws *FreeWillService) getDirectReplyContext(chatID int64, targetMessageID int) (string, error) {
	log.Printf("[FreeWill] getDirectReplyContext: Начинаем получение контекста для чата %d, targetMessageID: %d", chatID, targetMessageID)

	// Получаем цепочку ответов для целевого сообщения
	replyChain, err := fws.bot.storage.GetReplyChain(context.Background(), chatID, targetMessageID, 10)
	if err != nil {
		log.Printf("[FreeWill] Ошибка получения цепочки ответов: %v", err)
		// Продолжаем без цепочки ответов
	} else {
		log.Printf("[FreeWill] getDirectReplyContext: Получена цепочка ответов длиной %d", len(replyChain))
	}

	// Получаем общий контекст чата
	messages, err := fws.bot.storage.GetMessages(chatID, fws.bot.config.ContextWindow)
	if err != nil {
		log.Printf("[FreeWill] getDirectReplyContext: Ошибка получения сообщений: %v", err)
		return "", fmt.Errorf("ошибка получения истории: %w", err)
	}
	log.Printf("[FreeWill] getDirectReplyContext: Получено %d сообщений из хранилища", len(messages))

	// Получаем релевантные сообщения из долгосрочной памяти
	var relevantMessages []*tgbotapi.Message
	if fws.bot.config.LongTermMemoryEnabled && len(messages) > 0 {
		// Ищем сообщение с указанным ID для поиска релевантного контекста
		var targetMessage *tgbotapi.Message
		for _, msg := range messages {
			if msg.MessageID == targetMessageID {
				targetMessage = msg
				break
			}
		}

		if targetMessage != nil && targetMessage.Text != "" {
			log.Printf("[FreeWill] getDirectReplyContext: Найдено целевое сообщение ID:%d, текст: %.50s...", targetMessageID, targetMessage.Text)
			relevantMsgs, err := fws.bot.storage.SearchRelevantMessages(chatID, targetMessage.Text, fws.bot.config.LongTermMemoryFetchK)
			if err != nil {
				log.Printf("[FreeWill] Ошибка поиска релевантных сообщений для direct reply: %v", err)
			} else {
				relevantMessages = relevantMsgs
				log.Printf("[FreeWill] getDirectReplyContext: Найдено %d релевантных сообщений", len(relevantMessages))
			}
		} else {
			log.Printf("[FreeWill] getDirectReplyContext: Целевое сообщение ID:%d не найдено или пустое", targetMessageID)
		}
	}

	// Находим целевое сообщение
	var triggeringMessage *tgbotapi.Message
	for _, msg := range messages {
		if msg.MessageID == targetMessageID {
			triggeringMessage = msg
			break
		}
	}

	if triggeringMessage != nil {
		log.Printf("[FreeWill] getDirectReplyContext: Найдено триггерное сообщение ID:%d от пользователя %d", targetMessageID, triggeringMessage.From.ID)
	} else {
		log.Printf("[FreeWill] getDirectReplyContext: ВНИМАНИЕ! Триггерное сообщение ID:%d НЕ НАЙДЕНО в %d сообщениях", targetMessageID, len(messages))
	}

	log.Printf("[FreeWill] getDirectReplyContext: Вызываем formatDirectReplyContext с параметрами:")
	log.Printf("  - chatID: %d", chatID)
	log.Printf("  - triggeringMessage: %v", triggeringMessage != nil)
	log.Printf("  - replyChain: %d сообщений", len(replyChain))
	log.Printf("  - messages: %d сообщений", len(messages))
	log.Printf("  - relevantMessages: %d сообщений", len(relevantMessages))

	result := formatDirectReplyContext(chatID, triggeringMessage, replyChain, messages, relevantMessages, fws.bot.storage, fws.bot.config, fws.bot.config.TimeZone)

	// Применяем изоляцию контекста для прямого ответа
	if fws.bot.contextIsolator != nil && triggeringMessage != nil && triggeringMessage.From != nil {
		targetUserID := int64(triggeringMessage.From.ID)
		result = fws.bot.contextIsolator.IsolateContext(chatID, targetUserID, IsolationUserSpecific, result)
	} else if fws.bot.contextIsolator != nil {
		result = fws.bot.contextIsolator.IsolateContext(chatID, 0, IsolationGeneral, result)
	}

	log.Printf("[FreeWill] getDirectReplyContext: Результат форматирования (длина: %d символов): %.200s...", len(result), result)

	return result, nil
}

// getContextForResponseType получает контекст для второго этапа
func (fws *FreeWillService) getContextForResponseType(chatID int64, shouldReplyDecision *FreeWillShouldReplyDecision) (string, error) {
	log.Printf("[FreeWill] getContextForResponseType: Начинаем получение контекста для чата %d, reply_type: %s", chatID, shouldReplyDecision.ReplyType)

	// В зависимости от типа ответа используем разные методы получения контекста
	switch shouldReplyDecision.ReplyType {
	case "direct_reply":
		log.Printf("[FreeWill] getContextForResponseType: Обрабатываем direct_reply для чата %d", chatID)
		if shouldReplyDecision.TargetMessageID != 0 {
			log.Printf("[FreeWill] getContextForResponseType: Используем getDirectReplyContext с targetMessageID: %d", shouldReplyDecision.TargetMessageID)
			return fws.getDirectReplyContext(chatID, shouldReplyDecision.TargetMessageID)
		}
		// Если нет target_message_id, используем общий контекст
		log.Printf("[FreeWill] getContextForResponseType: Нет target_message_id, используем getGeneralContext")
		return fws.getGeneralContext(chatID)
	case "take_response":
		log.Printf("[FreeWill] getContextForResponseType: Обрабатываем take_response для чата %d", chatID)
		// Для ответа на тейк используем контекст с цепочкой ответов
		if shouldReplyDecision.TargetMessageID != 0 {
			log.Printf("[FreeWill] getContextForResponseType: Используем getDirectReplyContext с targetMessageID: %d", shouldReplyDecision.TargetMessageID)
			return fws.getDirectReplyContext(chatID, shouldReplyDecision.TargetMessageID)
		}
		// Если нет target_message_id, используем общий контекст
		log.Printf("[FreeWill] getContextForResponseType: Нет target_message_id, используем getGeneralContext")
		return fws.getGeneralContext(chatID)
	case "context_based":
		log.Printf("[FreeWill] getContextForResponseType: Обрабатываем context_based для чата %d", chatID)
		return fws.getContextBasedContext(chatID)
	case "silence_response":
		log.Printf("[FreeWill] getContextForResponseType: Обрабатываем silence_response для чата %d", chatID)
		// Для ответа на тишину нужен минимальный контекст
		return fws.getGeneralContext(chatID)
	default: // "general"
		log.Printf("[FreeWill] getContextForResponseType: Обрабатываем general (default) для чата %d", chatID)
		return fws.getGeneralContext(chatID)
	}
}

// getMessageAuthorAlias получает алиас автора сообщения с кешированием профилей и дисамбигуацией
func (fws *FreeWillService) getMessageAuthorAlias(chatID int64, msg *tgbotapi.Message, profilesCache map[int64]*storage.UserProfile) string {
	if msg.From == nil {
		if msg.SenderChat != nil {
			if msg.SenderChat.Title != "" {
				return msg.SenderChat.Title
			}
			return fmt.Sprintf("Chat_%d", msg.SenderChat.ID)
		}
		return "Неизвестный"
	}

	userID := msg.From.ID

	// НОВОЕ: Используем систему дисамбигуации для Free Will (decision_making контекст)
	if fws.bot.userValidator != nil {
		return fws.bot.userValidator.FormatUserWithDisambiguation(chatID, userID, "decision_making", msg)
	}

	// Гарантируем, что кеш профилей инициализирован (nil-safe)
	if profilesCache == nil {
		profilesCache = make(map[int64]*storage.UserProfile)
	}

	// Fallback к старой логике если валидатор недоступен
	// Проверяем кеш профилей
	profile, found := profilesCache[userID]
	if !found {
		// Загружаем профиль, если не в кеше
		loadedProfile, err := fws.bot.storage.GetUserProfile(chatID, userID)
		if err != nil {
			log.Printf("[WARN] Chat %d: Ошибка загрузки профиля для userID %d: %v", chatID, userID, err)
		} else if loadedProfile != nil {
			profilesCache[userID] = loadedProfile // Сохраняем в кеш
			profile = loadedProfile
		}
	}

	// Определяем алиас
	if profile != nil && profile.Alias != "" {
		return profile.Alias
	} else if msg.From.FirstName != "" {
		return msg.From.FirstName
	} else if msg.From.UserName != "" {
		return msg.From.UserName
	} else {
		return fmt.Sprintf("User_%d", userID)
	}
}

// validateTargetMessageID проверяет, что target_message_id существует в недавних сообщениях
func (fws *FreeWillService) validateTargetMessageID(chatID int64, targetMessageID int) error {
	// Получаем недавние сообщения из того же контекста, что используется для анализа
	messages, err := fws.bot.storage.GetMessages(chatID, fws.contextWindow)
	if err != nil {
		return fmt.Errorf("ошибка получения сообщений для валидации: %w", err)
	}

	// Ограничиваем до contextWindow как в getContextForAnalysis
	if len(messages) > fws.contextWindow {
		messages = messages[len(messages)-fws.contextWindow:]
	}

	// Проверяем, есть ли сообщение с таким ID
	for _, msg := range messages {
		if msg.MessageID == targetMessageID {
			log.Printf("[FreeWill] validateTargetMessageID: Найдено валидное сообщение %d в чате %d", targetMessageID, chatID)
			return nil
		}
	}

	return fmt.Errorf("сообщение с ID %d не найдено в контексте чата %d", targetMessageID, chatID)
}

// getTargetMessageInfo получает краткую информацию о целевом сообщении для логирования
func (fws *FreeWillService) getTargetMessageInfo(chatID int64, targetMessageID int) string {
	messages, err := fws.bot.storage.GetMessages(chatID, fws.contextWindow)
	if err != nil {
		return "ошибка получения сообщения"
	}

	// Ограничиваем до contextWindow
	if len(messages) > fws.contextWindow {
		messages = messages[len(messages)-fws.contextWindow:]
	}

	// Ищем целевое сообщение
	for _, msg := range messages {
		if msg.MessageID == targetMessageID {
			profiles := make(map[int64]*storage.UserProfile)
			authorAlias := fws.getMessageAuthorAlias(chatID, msg, profiles)

			msgText := msg.Text
			if msgText == "" {
				msgText = msg.Caption
			}

			// Ограничиваем длину для логирования
			if len(msgText) > 50 {
				msgText = msgText[:47] + "..."
			}

			return fmt.Sprintf("от %s: %s", authorAlias, msgText)
		}
	}

	return "сообщение не найдено"
}

// markMessageProcessed отмечает сообщение как обработанное
func (fws *FreeWillService) markMessageProcessed(chatID int64, messageID int) {
	fws.mutex.Lock()
	defer fws.mutex.Unlock()

	key := fmt.Sprintf("%d:%d", chatID, messageID)
	fws.processedMessages[key] = true

	log.Printf("[FreeWill] AntiDup: ✅ Сообщение %s отмечено как обработанное", key)
}

// isMessageProcessed проверяет, было ли сообщение уже обработано
func (fws *FreeWillService) isMessageProcessed(chatID int64, messageID int) bool {
	fws.mutex.RLock()
	defer fws.mutex.RUnlock()

	key := fmt.Sprintf("%d:%d", chatID, messageID)
	processed := fws.processedMessages[key]

	log.Printf("[FreeWill] AntiDup: 🔍 Проверка сообщения %s: обработано=%t", key, processed)
	return processed
}

// formatFreeWillDecisionContext форматирует контекст специально для принятия решений Free Will
// Теперь использует унифицированное форматирование с акцентом на MessageID для target_message_id
func (fws *FreeWillService) formatFreeWillDecisionContext(chatID int64, messages []*tgbotapi.Message, relevantMessages []*tgbotapi.Message) string {
	// Используем новый унифицированный форматтер
	formatter := NewUnifiedMessageFormatter(fws.bot.storage, fws.bot.config.TimeZone)
	formatter.SetDisableUserProfiles(fws.bot.config.DisableUserProfiles)
	formattedHistory := formatter.FormatMessagesXML(chatID, messages)

	log.Printf("[FreeWill] Chat %d: Использован унифицированный форматтер для %d сообщений", chatID, len(messages))
	return formattedHistory
}

// buildDecisionSystemPrompt строит system-промпт для Decision Stage
func (fws *FreeWillService) buildDecisionSystemPrompt(chatID int64) string {
	var sb strings.Builder
	sb.WriteString("Ты — Luna, участница чата. Ниже история сообщений в формате ChatML.\n")
	sb.WriteString("Анализируй контекст и принимай решение о необходимости ответа.\n")
	sb.WriteString("Обращай внимание на имена пользователей в квадратных скобках перед сообщениями.\n")
	return sb.String()
}
