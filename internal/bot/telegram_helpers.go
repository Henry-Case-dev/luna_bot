package bot

import (
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	"github.com/Henry-Case-dev/luna_bot/internal/utils" // Импортируем для TruncateString
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// sendReply отправляет простое текстовое сообщение в ответ.
func (b *Bot) sendReply(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	sentMsg, err := b.api.Send(msg)
	if err != nil {
		if b.isUserBlockedError(err) {
			log.Printf("[INFO] Пользователь %d заблокировал бота (403), пропускаю отправку", chatID)
			// Возможно, стоит пометить чат как неактивный
			b.markChatAsInactive(chatID)
			return
		}
		log.Printf("Ошибка отправки ответа в чат %d: %v", chatID, err)
		return
	}

	// Сохраняем отправленное ботом сообщение в историю
	if b.storage != nil {
		b.storage.AddMessage(chatID, &sentMsg)
		if b.config.Debug {
			log.Printf("[DEBUG][sendReply] Сохранено сообщение бота ID %d в чат %d", sentMsg.MessageID, chatID)
		}
	}
}

// getAssociativeContext возвращает короткий ассоциативный контекст для подмешивания в промпты
func (b *Bot) getAssociativeContext(chatID int64, keys []string, limit int) string {
	if !b.config.AssociationCloudEnabled {
		return ""
	}
	nodes, edges, err := b.storage.GetAssocTopForContext(chatID, keys, limit, b.config.AssociationCloudDecayDays, []string{"topic", "emoji"})
	if err != nil || (len(nodes) == 0 && len(edges) == 0) {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Associative context: ")
	added := 0
	for _, n := range nodes {
		if added >= limit {
			break
		}
		sb.WriteString(n.Type + ":" + n.Key + "; ")
		added++
	}
	return sb.String()
}

// sendReplyTo отправляет текстовое сообщение в ответ на конкретное сообщение.
func (b *Bot) sendReplyTo(chatID int64, messageID int, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyToMessageID = messageID
	sentMsg, err := b.api.Send(msg)
	if err != nil {
		if b.isUserBlockedError(err) {
			log.Printf("[INFO] Пользователь %d заблокировал бота (403), пропускаю отправку", chatID)
			b.markChatAsInactive(chatID)
			return
		}
		log.Printf("Ошибка отправки ответа (ReplyTo %d) в чат %d: %v", messageID, chatID, err)
		return
	}

	// Сохраняем отправленное ботом сообщение в историю
	if b.storage != nil {
		b.storage.AddMessage(chatID, &sentMsg)
		if b.config.Debug {
			log.Printf("[DEBUG][sendReplyTo] Сохранено сообщение бота ID %d в ответ на %d в чат %d", sentMsg.MessageID, messageID, chatID)
		}
	}
}

// deleteMessage удаляет сообщение по ID. Логирует только предупреждения при ошибках.
func (b *Bot) deleteMessage(chatID int64, messageID int) {
	if messageID == 0 {
		return // Игнорируем удаление с ID = 0
	}

	deleteReq := tgbotapi.NewDeleteMessage(chatID, messageID)
	_, err := b.api.Request(deleteReq) // Используем Request для DeleteMessage
	if err != nil {
		// Проверяем различные типы ошибок
		errStr := err.Error()
		if strings.Contains(errStr, "message to delete not found") {
			if b.config.Debug {
				log.Printf("[DEBUG][deleteMessage] Сообщение ID %d в чате %d уже удалено или не найдено", messageID, chatID)
			}
		} else if strings.Contains(errStr, "message can't be deleted") {
			log.Printf("[WARN][deleteMessage] Сообщение ID %d в чате %d не может быть удалено (возможно, слишком старое)", messageID, chatID)
		} else if strings.Contains(errStr, "not enough rights") {
			log.Printf("[WARN][deleteMessage] Недостаточно прав для удаления сообщения ID %d в чате %d", messageID, chatID)
		} else {
			log.Printf("[WARN][deleteMessage] Ошибка удаления сообщения ID %d в чате %d: %v", messageID, chatID, err)
		}
	} else if b.config.Debug {
		log.Printf("[DEBUG][deleteMessage] Успешно удалено сообщение ID %d в чате %d", messageID, chatID)
	}
}

// deleteMessageSilent удаляет сообщение без избыточного логирования.
// Возвращает true, если удаление прошло успешно, false в противном случае.
func (b *Bot) deleteMessageSilent(chatID int64, messageID int) bool {
	if messageID == 0 {
		return false // Игнорируем удаление с ID = 0
	}

	deleteReq := tgbotapi.NewDeleteMessage(chatID, messageID)
	_, err := b.api.Request(deleteReq)

	if err != nil {
		// Логируем только критические ошибки (не "message not found")
		errStr := err.Error()
		if strings.Contains(errStr, "message to delete not found") {
			// Не логируем - это ожидаемая ошибка для уже удаленных сообщений
			return false
		} else if strings.Contains(errStr, "message can't be deleted") {
			// Логируем только в debug режиме
			if b.config.Debug {
				log.Printf("[DEBUG][deleteMessageSilent] Сообщение ID %d в чате %d не может быть удалено (возможно, слишком старое)", messageID, chatID)
			}
			return false
		} else if strings.Contains(errStr, "not enough rights") {
			// Права - это серьезная проблема, логируем как предупреждение
			log.Printf("[WARN][deleteMessageSilent] Недостаточно прав для удаления сообщения ID %d в чате %d", messageID, chatID)
			return false
		} else {
			// Неизвестная ошибка - логируем только в debug режиме
			if b.config.Debug {
				log.Printf("[DEBUG][deleteMessageSilent] Ошибка удаления сообщения ID %d в чате %d: %v", messageID, chatID, err)
			}
			return false
		}
	}

	// Успешное удаление - логируем только в debug режиме
	if b.config.Debug {
		log.Printf("[DEBUG][deleteMessageSilent] Успешно удалено сообщение ID %d в чате %d", messageID, chatID)
	}
	return true
}

// sendAndDeleteAfter отправляет сообщение и удаляет его через указанное время.
// ВНИМАНИЕ: Сообщение сохраняется в базу данных.
func (b *Bot) sendAndDeleteAfter(chatID int64, text string, delay time.Duration) {
	msg := tgbotapi.NewMessage(chatID, text)
	sentMsg, err := b.api.Send(msg)
	if err != nil {
		log.Printf("[ERROR][sendAndDeleteAfter] Чат %d: Не удалось отправить сообщение '%s': %v", chatID, text, err)
		return
	}

	// Сохраняем отправленное ботом сообщение в историю
	if b.storage != nil {
		b.storage.AddMessage(chatID, &sentMsg)
		if b.config.Debug {
			log.Printf("[DEBUG][sendAndDeleteAfter] Сохранено сообщение бота ID %d в чат %d", sentMsg.MessageID, chatID)
		}
	}

	// Проверяем права на удаление сообщений в этом чате
	if !b.checkBotDeletePermissions(chatID) {
		log.Printf("[WARN][sendAndDeleteAfter] Чат %d: Бот не имеет прав на удаление сообщений. Автоудаление отменено для сообщения ID %d", chatID, sentMsg.MessageID)
		return
	}

	// Запускаем таймер на удаление
	time.AfterFunc(delay, func() {
		deleted := b.deleteMessageSilent(chatID, sentMsg.MessageID) // Используем тихую версию для автоудаления
		if b.config.Debug && deleted {
			log.Printf("[DEBUG][sendAndDeleteAfter] Чат %d: Автоматически удалено сообщение ID %d ('%s...')", chatID, sentMsg.MessageID, utils.TruncateString(text, 30))
		}
	})
}

// sendTemporaryMessage отправляет служебное сообщение, удаляет его через указанное время и НЕ сохраняет в базу данных.
// Используется для служебных сообщений (статусы, уведомления), которые не должны попадать в контекст бота.
func (b *Bot) sendTemporaryMessage(chatID int64, text string, delay time.Duration) {
	msg := tgbotapi.NewMessage(chatID, text)
	sentMsg, err := b.api.Send(msg)
	if err != nil {
		log.Printf("[ERROR][sendTemporaryMessage] Чат %d: Не удалось отправить временное сообщение '%s': %v", chatID, text, err)
		return
	}

	// КРИТИЧЕСКИ ВАЖНО: НЕ сохраняем служебное сообщение в базу данных!
	if b.config.Debug {
		log.Printf("[DEBUG][sendTemporaryMessage] Отправлено временное сообщение ID %d в чат %d (НЕ сохранено в БД)", sentMsg.MessageID, chatID)
	}

	// Проверяем права на удаление сообщений в этом чате
	if !b.checkBotDeletePermissions(chatID) {
		log.Printf("[WARN][sendTemporaryMessage] Чат %d: Бот не имеет прав на удаление сообщений. Автоудаление отменено для сообщения ID %d", chatID, sentMsg.MessageID)
		return
	}

	// Запускаем таймер на удаление
	time.AfterFunc(delay, func() {
		deleted := b.deleteMessageSilent(chatID, sentMsg.MessageID) // Используем тихую версию для автоудаления
		if b.config.Debug && deleted {
			log.Printf("[DEBUG][sendTemporaryMessage] Чат %d: Автоматически удалено временное сообщение ID %d ('%s...')", chatID, sentMsg.MessageID, utils.TruncateString(text, 30))
		}
	})
}

// answerCallback отвечает на CallbackQuery (например, подтверждает нажатие кнопки).
func (b *Bot) answerCallback(callbackID string, text string) {
	callback := tgbotapi.NewCallback(callbackID, text)
	// Если текст слишком длинный, обрезаем его до 200 символов
	if utf8.RuneCountInString(text) > 200 {
		callback.Text = string([]rune(text)[:197]) + "..."
	}
	_, err := b.api.Request(callback)
	if err != nil {
		log.Printf("Ошибка ответа на CallbackQuery (%s): %v", callbackID, err)
	}
}

// sendReplyMarkdown отправляет текстовое сообщение в указанный чат с поддержкой Markdown.
// Логирует ошибки.
func (b *Bot) sendReplyMarkdown(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	sentMsg, err := b.api.Send(msg)
	if err != nil {
		if b.isUserBlockedError(err) {
			log.Printf("[INFO] Пользователь %d заблокировал бота (403), пропускаю отправку Markdown", chatID)
			b.markChatAsInactive(chatID)
			return
		}
		// Если ошибка связана с Markdown, отправляем обычный текст
		if strings.Contains(err.Error(), "markdown") || strings.Contains(err.Error(), "parse") {
			log.Printf("[ERROR] Ошибка отправки Markdown сообщения в чат %d: %v. Текст: %s...", chatID, err, utils.TruncateString(text, 50))
			plainText := tgbotapi.NewMessage(chatID, text)
			fallbackMsg, fallbackErr := b.api.Send(plainText)
			if fallbackErr == nil && b.storage != nil {
				b.storage.AddMessage(chatID, &fallbackMsg)
			} else if b.isUserBlockedError(fallbackErr) {
				log.Printf("[INFO] Пользователь %d заблокировал бота (403), пропускаю fallback отправку", chatID)
				b.markChatAsInactive(chatID)
			}
		}
		return
	}

	// Сохраняем отправленное ботом сообщение в историю
	if b.storage != nil {
		b.storage.AddMessage(chatID, &sentMsg)
		if b.config.Debug {
			log.Printf("[DEBUG][sendReplyMarkdown] Сохранено сообщение бота ID %d в чат %d", sentMsg.MessageID, chatID)
		}
	}
}

// sendAutoDeleteErrorReply отправляет сообщение об ошибке, которое автоматически удаляется через указанное время
// ВАЖНО: Технические сообщения об ошибках НЕ сохраняются в историю чата - бот не должен их видеть!
func (b *Bot) sendAutoDeleteErrorReply(chatID int64, replyToMessageID int, errorText string) {
	msg := tgbotapi.NewMessage(chatID, errorText)
	if replyToMessageID != 0 {
		msg.ReplyToMessageID = replyToMessageID
	}

	sentMsg, err := b.api.Send(msg)
	if err != nil {
		log.Printf("[ERROR][sendAutoDeleteErrorReply] Не удалось отправить сообщение об ошибке в чат %d: %v", chatID, err)
		return
	}

	// ИСПРАВЛЕНО: НЕ сохраняем технические сообщения об ошибках в историю!
	// Бот не должен знать о своих технических проблемах

	// Проверяем права на удаление сообщений в этом чате
	if !b.checkBotDeletePermissions(chatID) {
		log.Printf("[WARN][sendAutoDeleteErrorReply] Чат %d: Бот не имеет прав на удаление сообщений. Автоудаление отменено для сообщения об ошибке ID %d", chatID, sentMsg.MessageID)
		return
	}

	// Запускаем горутину для автоудаления сообщения
	go func(cID int64, mID int, delaySeconds int) {
		if delaySeconds <= 0 {
			if b.config.Debug {
				log.Printf("[DEBUG][sendAutoDeleteErrorReply] Автоудаление отключено для сообщения об ошибке (ID: %d) в чате %d", mID, cID)
			}
			return // Не удаляем, если задержка <= 0
		}

		if b.config.Debug {
			log.Printf("[DEBUG][sendAutoDeleteErrorReply] Запланировано автоудаление сообщения об ошибке (ID: %d) в чате %d через %d секунд", mID, cID, delaySeconds)
		}

		time.Sleep(time.Duration(delaySeconds) * time.Second)
		b.deleteMessageSilent(cID, mID) // Используем тихую версию для автоудаления
	}(chatID, sentMsg.MessageID, b.config.ErrorMessageAutoDeleteSeconds)
}

// getBotMember получает информацию о боте как участнике чата
func (b *Bot) getBotMember(chatID int64) (*tgbotapi.ChatMember, error) {
	memberConfig := tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: chatID,
			UserID: b.api.Self.ID,
		},
	}
	member, err := b.api.GetChatMember(memberConfig)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения информации о боте в чате %d: %w", chatID, err)
	}
	return &member, nil
}

// checkBotDeletePermissions проверяет, может ли бот удалять сообщения в данном чате
func (b *Bot) checkBotDeletePermissions(chatID int64) bool {
	// В приватных чатах бот всегда может удалять свои сообщения
	if chatID > 0 {
		return true
	}

	// В группах и супергруппах нужно проверить права
	member, err := b.getBotMember(chatID)
	if err != nil {
		log.Printf("[WARN][checkBotDeletePermissions] Не удалось получить информацию о боте в чате %d: %v", chatID, err)
		return false
	}

	// Проверяем права в зависимости от статуса
	switch member.Status {
	case "administrator":
		// Администратор: проверяем права на удаление сообщений
		return member.CanDeleteMessages
	case "member":
		// Обычный участник: может удалять только свои сообщения (в течение 48 часов)
		return true
	case "restricted":
		// Ограниченный участник: зависит от разрешений
		return member.CanDeleteMessages
	case "left", "kicked":
		// Бот исключен или покинул чат
		return false
	default:
		log.Printf("[WARN][checkBotDeletePermissions] Неизвестный статус бота в чате %d: %s", chatID, member.Status)
		return false
	}
}

// sendAutoDeleteMessage отправляет сообщение и удаляет его через указанную задержку
func (b *Bot) sendAutoDeleteMessage(chatID int64, text string, delay time.Duration) {
	msg := tgbotapi.NewMessage(chatID, text)
	sentMsg, err := b.api.Send(msg)
	if err != nil {
		log.Printf("[sendAutoDeleteMessage ERROR] Чат %d: Ошибка отправки сообщения для автоудаления: %v", chatID, err)
		return
	}

	// Сохраняем отправленное ботом сообщение в историю
	if b.storage != nil {
		b.storage.AddMessage(chatID, &sentMsg)
		if b.config.Debug {
			log.Printf("[DEBUG][sendAutoDeleteMessage] Сохранено сообщение бота ID %d в чат %d", sentMsg.MessageID, chatID)
		}
	}

	// Проверяем права на удаление сообщений в этом чате
	if !b.checkBotDeletePermissions(chatID) {
		log.Printf("[WARN][sendAutoDeleteMessage] Чат %d: Бот не имеет прав на удаление сообщений. Автоудаление отменено для сообщения ID %d", chatID, sentMsg.MessageID)
		return
	}

	if b.config.Debug {
		log.Printf("[DEBUG][sendAutoDeleteMessage] Чат %d: Запланировано удаление сообщения ID %d через %v ('%s...')",
			chatID, sentMsg.MessageID, delay, utils.TruncateString(text, 30))
	}

	// Запускаем таймер для удаления сообщения
	time.AfterFunc(delay, func() {
		b.deleteMessageSilent(chatID, sentMsg.MessageID) // Используем тихую версию для автоудаления
	})
}

// sendSystemMessage отправляет системное сообщение (приветствие, инфо) которое НЕ сохраняется в БД
// Это предотвращает попадание технических сообщений в контекст бота
func (b *Bot) sendSystemMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	sentMsg, err := b.api.Send(msg)
	if err != nil {
		log.Printf("[sendSystemMessage ERROR] Чат %d: Ошибка отправки системного сообщения: %v", chatID, err)
		return
	}

	// ВАЖНО: НЕ сохраняем системные сообщения в историю!
	// Бот не должен видеть свои приветствия и инфо-сообщения в контексте

	if b.config.Debug {
		log.Printf("[DEBUG][sendSystemMessage] Отправлено системное сообщение ID %d в чат %d (НЕ сохранено в БД)", sentMsg.MessageID, chatID)
	}
}

// isUserBlockedError проверяет, является ли ошибка блокировкой бота пользователем (403)
func (b *Bot) isUserBlockedError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "403") &&
		(strings.Contains(errStr, "Forbidden: bot was blocked by the user") ||
			strings.Contains(errStr, "bot was blocked"))
}

// markChatAsInactive помечает чат как неактивный при блокировке пользователем
func (b *Bot) markChatAsInactive(chatID int64) {
	// Проверяем, является ли это приватным чатом (положительный ID)
	if chatID > 0 {
		b.settingsMutex.Lock()
		defer b.settingsMutex.Unlock()

		if settings, exists := b.chatSettings[chatID]; exists {
			if settings.Active {
				settings.Active = false
				log.Printf("[INFO] Чат %d помечен как неактивный (пользователь заблокировал бота)", chatID)

				// Для хранилищ с БД нужно также обновить статус в базе
				if b.config.StorageType != config.StorageTypeFile {
					// Получаем настройки из БД и обновляем их
					dbSettings, err := b.storage.GetChatSettings(chatID)
					if err != nil {
						log.Printf("[ERROR] Не удалось получить настройки чата %d из БД: %v", chatID, err)
						return
					}

					// Создаем новый объект storage.ChatSettings если его нет
					if dbSettings == nil {
						dbSettings = &storage.ChatSettings{
							ChatID: chatID,
						}
					}

					// Помечаем как неактивный (для файлового хранилища это поле не используется)
					// Для MongoDB/PostgreSQL можно добавить поле Active в storage.ChatSettings

					// Пока просто логируем, так как в storage.ChatSettings нет поля Active
					log.Printf("[INFO] Чат %d отмечен как неактивный в памяти (в БД структура не поддерживает статус)", chatID)
				}
			}
		}
	}
}

// allowSendOnce — разрешает единственную отправку в пределах dedupTTL для ключа (chatID|source|originalMessageID)
func (b *Bot) allowSendOnce(chatID int64, source string, originalMessageID int) bool {
	if source == "" {
		source = "default"
	}
	key := fmt.Sprintf("%d|%s|%d", chatID, source, originalMessageID)

	now := time.Now()
	b.dedupMu.Lock()
	defer b.dedupMu.Unlock()

	if ts, ok := b.dedupMap[key]; ok {
		if now.Sub(ts) < b.dedupTTL {
			return false
		}
	}
	b.dedupMap[key] = now

	// Ленивая очистка устаревших записей, чтобы карта не разрасталась
	if len(b.dedupMap) > 2048 {
		cutoff := now.Add(-b.dedupTTL)
		for k, t := range b.dedupMap {
			if t.Before(cutoff) {
				delete(b.dedupMap, k)
			}
		}
	}
	return true
}

// sendReplyWithAntiRepetition отправляет сообщение с проверкой на повторения и переработкой
func (b *Bot) sendReplyWithAntiRepetition(chatID int64, text string, userID int64, responseType string) {
	// Мини-дедупликация по (chatID, source=responseType, originalMessageID=0)
	if !b.allowSendOnce(chatID, responseType, 0) {
		if b.config.Debug {
			log.Printf("[Dedup] Пропуск отправки (chat=%d, source=%s, msgID=%d)", chatID, responseType, 0)
		}
		return
	}
	// Проверяем на повторения только если система включена
	if b.config.AntiRepetitionEnabled && b.antiRepetitionService != nil {
		isRepetitive, reason := b.antiRepetitionService.CheckSimilarity(chatID, text, userID, responseType, 0) // 0 = общий ответ
		if isRepetitive {
			// Обрабатываем повторение: переработка или блокировка
			reworkedText, shouldSend := b.antiRepetitionService.ProcessRepetition(chatID, text, userID, responseType, reason)
			if !shouldSend {
				// Сообщение заблокировано (переработка отключена)
				if b.config.Debug {
					log.Printf("[AntiRepetition][BLOCKED] Чат %d: Заблокирован повторяющийся ответ. Причина: %s. Текст: %.50s...",
						chatID, reason, text)
				}
				return
			}
			// Используем переработанный текст
			text = reworkedText
		}
	}

	// Постобработка сообщения через MessagePostProcessor
	if b.messagePostProcessor != nil {
		// Определяем тип сообщения на основе responseType
		var messageType MessageType
		switch responseType {
		case "direct":
			messageType = MessageTypeDirect
		case "direct_serious":
			messageType = MessageTypeDirectSerious
		case "free_will":
			messageType = MessageTypeFreeWill
		case "voice":
			messageType = MessageTypeVoice
		default:
			messageType = MessageTypeDefault
		}

		processedText, err := b.messagePostProcessor.ProcessMessage(text, messageType, chatID)
		if err != nil {
			log.Printf("[MessagePostProcessor] Ошибка постобработки для чата %d: %v", chatID, err)
			// Продолжаем с оригинальным текстом при ошибке
		} else {
			text = processedText
		}
	}

	// Отправляем сообщение (оригинальное или переработанное)
	b.sendReply(chatID, text)

	// Записываем в память анти-повторений только если система включена
	if b.config.AntiRepetitionEnabled && b.antiRepetitionService != nil {
		b.antiRepetitionService.RecordResponse(chatID, text, userID, responseType, 0) // 0 = общий ответ
	}
}

// sendReplyToWithAntiRepetition отправляет сообщение в ответ на конкретное сообщение с проверкой на повторения и переработкой
func (b *Bot) sendReplyToWithAntiRepetition(chatID int64, messageID int, text string, userID int64, responseType string) {
	// Мини-дедупликация по (chatID, source=responseType, originalMessageID=messageID)
	if !b.allowSendOnce(chatID, responseType, messageID) {
		if b.config.Debug {
			log.Printf("[Dedup] Пропуск отправки (chat=%d, source=%s, msgID=%d)", chatID, responseType, messageID)
		}
		return
	}
	// Проверяем на повторения только если система включена
	if b.config.AntiRepetitionEnabled && b.antiRepetitionService != nil {
		isRepetitive, reason := b.antiRepetitionService.CheckSimilarity(chatID, text, userID, responseType, messageID)
		if isRepetitive {
			// Обрабатываем повторение: переработка или блокировка
			reworkedText, shouldSend := b.antiRepetitionService.ProcessRepetition(chatID, text, userID, responseType, reason)
			if !shouldSend {
				// Сообщение заблокировано (переработка отключена)
				if b.config.Debug {
					log.Printf("[AntiRepetition][BLOCKED] Чат %d: Заблокирован повторяющийся ответ (ReplyTo %d). Причина: %s. Текст: %.50s...",
						chatID, messageID, reason, text)
				}
				return
			}
			// Используем переработанный текст
			text = reworkedText
		}
	}

	// Постобработка сообщения через MessagePostProcessor
	if b.messagePostProcessor != nil {
		// Определяем тип сообщения на основе responseType
		var messageType MessageType
		switch responseType {
		case "direct":
			messageType = MessageTypeDirect
		case "direct_serious":
			messageType = MessageTypeDirectSerious
		case "free_will":
			messageType = MessageTypeFreeWill
		case "voice":
			messageType = MessageTypeVoice
		default:
			messageType = MessageTypeDefault
		}

		processedText, err := b.messagePostProcessor.ProcessMessage(text, messageType, chatID)
		if err != nil {
			log.Printf("[MessagePostProcessor] Ошибка постобработки для чата %d: %v", chatID, err)
			// Продолжаем с оригинальным текстом при ошибке
		} else {
			text = processedText
		}
	}

	// Отправляем сообщение (оригинальное или переработанное)
	b.sendReplyTo(chatID, messageID, text)

	// Записываем в память анти-повторений только если система включена
	if b.config.AntiRepetitionEnabled && b.antiRepetitionService != nil {
		b.antiRepetitionService.RecordResponse(chatID, text, userID, responseType, messageID)
	}
}

// sendSummaryReply отправляет суточное саммари с пометкой флагом summary=true
func (b *Bot) sendSummaryReply(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown

	sentMsg, err := b.api.Send(msg)
	if err != nil {
		if b.isUserBlockedError(err) {
			log.Printf("[WARN] Чат %d: Пользователь заблокировал бота при отправке саммари (403), пропускаю.", chatID)
			b.markChatAsInactive(chatID)
			return
		}
		log.Printf("[ERROR] Не удалось отправить суточное саммари в чат %d: %v", chatID, err)
		// Fallback: пробуем отправить без Markdown
		msg.ParseMode = ""
		sentMsg, err = b.api.Send(msg)
		if err != nil {
			if b.isUserBlockedError(err) {
				log.Printf("[WARN] Чат %d: Пользователь заблокировал бота при fallback отправке саммари (403), пропускаю.", chatID)
				b.markChatAsInactive(chatID)
				return
			}
			log.Printf("[ERROR] Не удалось отправить fallback суточное саммари в чат %d: %v", chatID, err)
			return
		}
	}

	// Добавляем отправленное сообщение в историю чата
	b.storage.AddMessage(chatID, &sentMsg)

	// Помечаем сообщение как суточное саммари в БД
	if mongoStorage, ok := b.storage.(*storage.PostgresStorage); ok {
		err = mongoStorage.MarkMessageAsSummary(chatID, sentMsg.MessageID, true, false)
		if err != nil {
			log.Printf("[ERROR] Не удалось пометить сообщение %d как суточное саммари в чате %d: %v", sentMsg.MessageID, chatID, err)
		} else {
			log.Printf("[Summary] Сообщение %d в чате %d помечено как суточное саммари", sentMsg.MessageID, chatID)
		}
	}
}

// sendWeeklySummaryReply отправляет еженедельное саммари с пометкой флагом weekly_summary=true
func (b *Bot) sendWeeklySummaryReply(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown

	sentMsg, err := b.api.Send(msg)
	if err != nil {
		log.Printf("[ERROR] Не удалось отправить еженедельное саммари в чат %d: %v", chatID, err)
		// Fallback: пробуем отправить без Markdown
		msg.ParseMode = ""
		sentMsg, err = b.api.Send(msg)
		if err != nil {
			log.Printf("[ERROR] Не удалось отправить fallback еженедельное саммари в чат %d: %v", chatID, err)
			return
		}
	}

	// Добавляем отправленное сообщение в историю чата
	b.storage.AddMessage(chatID, &sentMsg)

	// Помечаем сообщение как еженедельное саммари в БД
	if mongoStorage, ok := b.storage.(*storage.PostgresStorage); ok {
		err = mongoStorage.MarkMessageAsSummary(chatID, sentMsg.MessageID, false, true)
		if err != nil {
			log.Printf("[ERROR] Не удалось пометить сообщение %d как еженедельное саммари в чате %d: %v", sentMsg.MessageID, chatID, err)
		} else {
			log.Printf("[WeeklySummary] Сообщение %d в чате %d помечено как еженедельное саммари", sentMsg.MessageID, chatID)
		}
	}
}

// SendStartupMessage отправляет информацию о состоянии бота в чат.
// Сообщение автоматически удаляется через минуту и НЕ сохраняется в истории.
func (b *Bot) sendStartupMessage(chatID int64) {
	log.Printf("[StartupMessage][%d] === НАЧАЛО ОТПРАВКИ STARTUP MESSAGE ===", chatID)

	// Дополнительная проверка критических компонентов
	if b.api == nil {
		log.Printf("[StartupMessage][%d] ❌ КРИТИЧЕСКАЯ ОШИБКА: b.api = nil", chatID)
		return
	}
	if b.config == nil {
		log.Printf("[StartupMessage][%d] ❌ КРИТИЧЕСКАЯ ОШИБКА: b.config = nil", chatID)
		return
	}
	if b.storage == nil {
		log.Printf("[StartupMessage][%d] ❌ КРИТИЧЕСКАЯ ОШИБКА: b.storage = nil", chatID)
		return
	}

	log.Printf("[StartupMessage][%d] Критические компоненты проверены - все в порядке", chatID)

	// Собираем информацию о статусе всех сервисов
	var sb strings.Builder
	sb.WriteString("🚀 Запуск бота Luna\n\n")

	// Информация о хранилище
	sb.WriteString("💾 Хранилище данных:\n")
	sb.WriteString(fmt.Sprintf("📦 Тип: %s\n", b.config.StorageType))

	// Получаем статус хранилища и проверяем реальную работоспособность
	storageStatus := b.storage.GetStatus(chatID)
	storageWorking := true
	messageCount := 0

	// Проверяем реальную работоспособность хранилища
	if mongoStore, ok := b.storage.(*storage.PostgresStorage); ok {
		if count, err := mongoStore.GetTotalMessagesCount(chatID); err == nil {
			messageCount = int(count)
		} else {
			storageWorking = false
			storageStatus = "❌ ошибка подключения"
		}
	} else if fileStore, ok := b.storage.(*storage.FileStorage); ok {
		if messages, err := fileStore.GetMessages(chatID, 100000); err == nil {
			messageCount = len(messages)
		} else {
			storageWorking = false
			storageStatus = "❌ ошибка чтения файла"
		}
	} else {
		// Проверяем общую работоспособность через GetMessages
		if _, err := b.storage.GetMessages(chatID, 1); err != nil {
			storageWorking = false
			storageStatus = "❌ ошибка доступа"
		}
	}

	if storageWorking {
		sb.WriteString(fmt.Sprintf("📊 Статус: ✅ %s\n", storageStatus))
		sb.WriteString(fmt.Sprintf("📨 Сообщений в базе: %d\n", messageCount))
	} else {
		sb.WriteString(fmt.Sprintf("📊 Статус: %s\n", storageStatus))
	}

	sb.WriteString("\n🔧 Сервисы и модули:\n")

	// Модерация - проверяем ФАКТИЧЕСКОЕ состояние работы сервиса
	var moderationStatus string
	if b.moderation != nil {
		b.moderation.mutex.RLock()
		if isActive, exists := b.moderation.activeChats[chatID]; exists && isActive {
			moderationStatus = "✅ включена"
		} else if exists {
			// Модерация неактивна в чате - проверяем причину
			if !b.config.ModEnabled {
				moderationStatus = "❌ отключена (MOD_ENABLED=false)"
			} else {
				moderationStatus = "⚠️ неактивна (нет прав администратора)"
			}
		} else {
			// Чат не инициализирован - проверяем причину
			if !b.config.ModEnabled {
				moderationStatus = "❌ отключена (MOD_ENABLED=false)"
			} else {
				moderationStatus = "⏳ инициализируется"
			}
		}
		b.moderation.mutex.RUnlock()
	} else {
		moderationStatus = "❌ сервис не инициализирован"
	}
	sb.WriteString(fmt.Sprintf("🛡️ Модерация: %s\n", moderationStatus))

	// Free Will - проверяем ФАКТИЧЕСКОЕ состояние работы сервиса
	var freeWillStatus string
	if b.freeWillService != nil && b.freeWillService.IsEnabled() {
		freeWillStatus = "✅ включена"
	} else if b.freeWillService != nil && !b.freeWillService.IsEnabled() {
		// Сервис инициализирован, но выключен - проверяем причину
		if !b.config.FreeWillEnabled {
			freeWillStatus = "❌ отключена (FREE_WILL_ENABLED=false)"
		} else {
			freeWillStatus = "⚠️ неактивна (сервис выключен)"
		}
	} else {
		// Сервис не инициализирован - проверяем причину
		if !b.config.FreeWillEnabled {
			freeWillStatus = "❌ отключена (FREE_WILL_ENABLED=false)"
		} else {
			freeWillStatus = "❌ сервис не инициализирован"
		}
	}
	sb.WriteString(fmt.Sprintf("🧠 Свобода воли: %s\n", freeWillStatus))

	// Постпроцессор сообщений - проверяем ФАКТИЧЕСКОЕ состояние
	var postProcessorStatus string
	if b.messagePostProcessor != nil && b.messagePostProcessor.IsEnabled() {
		postProcessorStatus = "✅ включен"
	} else if b.messagePostProcessor != nil && !b.messagePostProcessor.IsEnabled() {
		postProcessorStatus = "❌ отключен (MESSAGE_POST_PROCESSOR_ENABLED=false)"
	} else {
		postProcessorStatus = "❌ сервис не инициализирован"
	}
	sb.WriteString(fmt.Sprintf("⚙️ Постпроцессор: %s\n", postProcessorStatus))

	// Анализ изображений - проверяем ФАКТИЧЕСКОЕ состояние
	var photoAnalysisStatus string
	if b.config.PhotoAnalysisEnabled {
		if b.embeddingClient != nil {
			photoAnalysisStatus = "✅ включен"
		} else {
			photoAnalysisStatus = "❌ включен (нет клиента Gemini)"
		}
	} else {
		photoAnalysisStatus = "❌ отключен (PHOTO_ANALYSIS_ENABLED=false)"
	}
	sb.WriteString(fmt.Sprintf("🖼️ Анализ изображений: %s\n", photoAnalysisStatus))

	// Голосовые сообщения (TTS) - проверяем ФАКТИЧЕСКОЕ состояние работы
	var ttsStatus string
	if b.voiceMessageService != nil && b.config.ElevenLabsAPIKey != "" {
		if b.config.VoiceMessagesEnabled {
			ttsStatus = "✅ включены"
		} else {
			ttsStatus = "❌ отключены (VOICE_MESSAGES_ENABLED=false)"
		}
	} else if b.voiceMessageService != nil && b.config.ElevenLabsAPIKey == "" {
		ttsStatus = "❌ отключены (нет API ключа ElevenLabs)"
	} else if b.voiceMessageService == nil {
		if !b.config.VoiceMessagesEnabled {
			ttsStatus = "❌ отключены (VOICE_MESSAGES_ENABLED=false)"
		} else {
			ttsStatus = "❌ сервис не инициализирован"
		}
	}
	sb.WriteString(fmt.Sprintf("🗣️ Голосовые сообщения: %s\n", ttsStatus))

	// Веб-поиск - проверяем ФАКТИЧЕСКОЕ состояние работы
	var webSearchStatus string
	if b.webSearch != nil && b.config.GoogleSearchAPIKey != "" && b.config.GoogleSearchEngineID != "" {
		if b.config.WebSearchEnabled {
			webSearchStatus = "✅ включен"
		} else {
			webSearchStatus = "❌ отключен (WEB_SEARCH_ENABLED=false)"
		}
	} else if b.webSearch != nil && (b.config.GoogleSearchAPIKey == "" || b.config.GoogleSearchEngineID == "") {
		webSearchStatus = "❌ отключен (нет API ключей Google)"
	} else if b.webSearch == nil {
		if !b.config.WebSearchEnabled {
			webSearchStatus = "❌ отключен (WEB_SEARCH_ENABLED=false)"
		} else {
			webSearchStatus = "❌ сервис не инициализирован"
		}
	}
	sb.WriteString(fmt.Sprintf("🔍 Веб-поиск: %s\n", webSearchStatus))

	// Отслеживание реакций - проверяем ФАКТИЧЕСКОЕ состояние работы
	var reactionsStatus string
	if b.reactionTracker != nil && b.reactionHandler != nil {
		if b.config.ReactionsEnabled {
			reactionsStatus = "✅ включено"
		} else {
			reactionsStatus = "❌ отключено (REACTIONS_ENABLED=false)"
		}
	} else {
		if !b.config.ReactionsEnabled {
			reactionsStatus = "❌ отключено (REACTIONS_ENABLED=false)"
		} else {
			reactionsStatus = "❌ сервисы не инициализированы"
		}
	}
	sb.WriteString(fmt.Sprintf("😊 Реакции: %s\n", reactionsStatus))

	// Долгосрочная память - проверяем ФАКТИЧЕСКОЕ состояние работы
	var memoryStatus string
	if mongoStore, ok := b.storage.(*storage.PostgresStorage); ok && b.embeddingClient != nil {
		if b.config.LongTermMemoryEnabled {
			// Проверяем доступность MongoDB для эмбеддингов
			if _, err := mongoStore.GetMessages(chatID, 1); err == nil {
				memoryStatus = "✅ включена"
			} else {
				memoryStatus = "❌ включена (ошибка MongoDB)"
			}
		} else {
			memoryStatus = "❌ отключена (LONG_TERM_MEMORY_ENABLED=false)"
		}
	} else {
		if !b.config.LongTermMemoryEnabled {
			memoryStatus = "❌ отключена (LONG_TERM_MEMORY_ENABLED=false)"
		} else if _, ok := b.storage.(*storage.PostgresStorage); !ok {
			memoryStatus = "❌ отключена (требуется MongoDB)"
		} else if b.embeddingClient == nil {
			memoryStatus = "❌ отключена (нет клиента эмбеддингов)"
		}
	}
	sb.WriteString(fmt.Sprintf("💭 Долгосрочная память: %s\n", memoryStatus))

	// Автоматическое саммари - проверяем ФАКТИЧЕСКОЕ состояние
	var summaryStatus string
	if b.config.SummaryIntervalHours > 0 {
		summaryStatus = fmt.Sprintf("✅ включено (каждые %d ч)", b.config.SummaryIntervalHours)
	} else {
		summaryStatus = "❌ отключено (SUMMARY_INTERVAL_HOURS=0)"
	}
	sb.WriteString(fmt.Sprintf("📝 Автосаммари: %s\n", summaryStatus))

	// Анализ срачей - проверяем ФАКТИЧЕСКОЕ состояние
	var srachStatus string
	if b.config.SrachAnalysisEnabled {
		srachStatus = "✅ включен"
	} else {
		srachStatus = "❌ отключен (SRACH_ANALYSIS_ENABLED=false)"
	}
	sb.WriteString(fmt.Sprintf("⚔️ Анализ срачей: %s\n", srachStatus))

	// Система анти-повторений - проверяем ФАКТИЧЕСКОЕ состояние работы
	var antiRepetitionStatus string
	if b.antiRepetitionService != nil {
		if b.config.AntiRepetitionEnabled {
			antiRepetitionStatus = "✅ включена"
		} else {
			antiRepetitionStatus = "❌ отключена (ANTI_REPETITION_ENABLED=false)"
		}
	} else {
		if !b.config.AntiRepetitionEnabled {
			antiRepetitionStatus = "❌ отключена (ANTI_REPETITION_ENABLED=false)"
		} else {
			antiRepetitionStatus = "❌ сервис не инициализирован"
		}
	}
	sb.WriteString(fmt.Sprintf("🔄 Анти-повторения: %s\n", antiRepetitionStatus))

	// Каузальный анализатор (Этап 1) - проверяем ФАКТИЧЕСКОЕ состояние
	var causalAnalyzerStatus string
	if b.config.CausalLearningEnabled {
		// Получаем реальную статистику из каузальной памяти
		causalMemoryStats := make(map[int64]int)
		totalCausalEntries := 0

		// Проверяем каузальную память для текущего чата
		if causalMemory, err := b.storage.GetCausalMemory(chatID); err == nil {
			totalCausalEntries = causalMemory.TotalEntries
			causalMemoryStats[chatID] = causalMemory.TotalEntries
		}

		// Проверяем интервал анализа
		intervalHours := b.config.CausalAnalysisIntervalHours
		if intervalHours <= 0 {
			intervalHours = 4 // дефолтное значение
		}

		causalAnalyzerStatus = fmt.Sprintf("✅ включен (анализ каждые %d ч, записей: %d)", intervalHours, totalCausalEntries)
	} else {
		causalAnalyzerStatus = "❌ отключен (CAUSAL_LEARNING_ENABLED=false)"
	}
	sb.WriteString(fmt.Sprintf("🧠 Каузальный анализатор: %s\n", causalAnalyzerStatus))

	// Эмоциональная архитектура (Этап 2) - проверяем ФАКТИЧЕСКОЕ состояние
	var emotionalArchitectureStatus string
	if b.config.EmotionalLearningEnabled {
		// Получаем реальную статистику эмоциональной памяти
		totalEmotionalMemories := 0
		emotionalStateExists := false

		// Проверяем эмоциональную память для текущего чата
		if emotionalMemories, err := b.storage.GetEmotionalMemories(chatID, 0, 1000); err == nil {
			totalEmotionalMemories = len(emotionalMemories)
		}

		// Проверяем наличие эмоционального состояния
		if emotionalState, err := b.storage.GetEmotionalState(chatID); err == nil && emotionalState != nil {
			emotionalStateExists = true
		}

		// Проверяем интервал анализа
		intervalHours := b.config.EmotionalAnalysisIntervalHours
		if intervalHours <= 0 {
			intervalHours = 2 // дефолтное значение
		}

		// Проверяем доступность промптов
		promptsAvailable := b.config.EmotionalAnalysisPromptEnabled &&
			b.config.EmotionalAdaptationPromptEnabled &&
			b.config.EmotionalFeedbackPromptEnabled

		if promptsAvailable {
			emotionalArchitectureStatus = fmt.Sprintf("✅ включена (анализ каждые %d ч, воспоминаний: %d, состояние: %s)",
				intervalHours, totalEmotionalMemories,
				map[bool]string{true: "активно", false: "не инициализировано"}[emotionalStateExists])
		} else {
			emotionalArchitectureStatus = "⚠️ включена (промпты отключены)"
		}
	} else {
		emotionalArchitectureStatus = "❌ отключена (EMOTIONAL_LEARNING_ENABLED=false)"
	}
	sb.WriteString(fmt.Sprintf("🎭 Эмоциональная архитектура: %s\n", emotionalArchitectureStatus))

	// Облако ассоциаций (Association Cloud) — показываем реальный статус и причину
	var assocStatus string
	if b.config.AssociationCloudEnabled {
		// Пытаемся получить небольшую выборку; если бэкенд не поддерживает/ошибка — отразим причину
		var nodes []*storage.AssocNode
		var edges []*storage.AssocEdge
		var err error
		if b.storage != nil {
			nodes, edges, err = b.storage.GetAssocTopForContext(chatID, nil, 3, b.config.AssociationCloudDecayDays, []string{"topic", "emoji"})
		}
		if err != nil {
			// Определим вероятную причину
			if _, ok := b.storage.(*storage.PostgresStorage); !ok {
				assocStatus = "⚠️ включено (бэкенд без полной поддержки)"
			} else {
				assocStatus = "⚠️ включено (ошибка доступа к данным)"
			}
		} else {
			if len(nodes) == 0 && len(edges) == 0 {
				assocStatus = "✅ включено (данных пока нет)"
			} else {
				assocStatus = fmt.Sprintf("✅ включено (узлов: %d, связей: %d)", len(nodes), len(edges))
			}
		}
	} else {
		assocStatus = "❌ отключено (ASSOCIATION_CLOUD_ENABLED=false)"
	}
	sb.WriteString(fmt.Sprintf("☁️ Облако ассоциаций: %s\n", assocStatus))

	// Система убеждений (Belief Analyzer) — реальный статус
	var beliefStatus string
	if b.config.BeliefLearningEnabled {
		// Попробуем получить персональность для этого чата и показать краткую сводку
		interval := b.config.BeliefAnalysisIntervalHours
		if interval <= 0 {
			interval = 6
		}
		beliefStatus = fmt.Sprintf("✅ включена (анализ каждые %d ч)", interval)
		if mem, err := b.storage.GetPersonalityMemory(chatID); err == nil && mem != nil && mem.BeliefSystem != nil {
			beliefCount := len(mem.BeliefSystem.CoreBeliefs)
			last := "нет данных"
			if !mem.BeliefSystem.LastBeliefUpdate.IsZero() {
				last = mem.BeliefSystem.LastBeliefUpdate.Format("2006-01-02 15:04")
			}
			beliefStatus = fmt.Sprintf("✅ включена (анализ каждые %d ч, убеждений: %d, обновлено: %s)", interval, beliefCount, last)
		}
	} else {
		beliefStatus = "❌ отключена (BELIEF_LEARNING_ENABLED=false)"
	}
	sb.WriteString(fmt.Sprintf("🧠 Система убеждений: %s\n", beliefStatus))

	sb.WriteString("\n🤖 Основные параметры:\n")

	// Проверяем работоспособность LLM клиента
	llmStatus := fmt.Sprintf("🔤 LLM провайдер: %s", b.config.LLMProvider)
	if b.llm != nil {
		llmStatus += " ✅"
	} else {
		llmStatus += " ❌ (не инициализирован)"
	}
	sb.WriteString(fmt.Sprintf("%s\n", llmStatus))

	sb.WriteString(fmt.Sprintf("💬 Интервал ответов: %d-%d сообщений\n", b.config.MinMessages, b.config.MaxMessages))
	sb.WriteString(fmt.Sprintf("📏 Окно контекста: %d сообщений\n", b.config.ContextWindow))

	// Подсчитываем общее количество проблем
	problemCount := 0
	if !storageWorking {
		problemCount++
	}
	if b.config.FreeWillEnabled && (b.freeWillService == nil || !b.freeWillService.IsEnabled()) {
		problemCount++
	}
	if b.config.VoiceMessagesEnabled && (b.voiceMessageService == nil || b.config.ElevenLabsAPIKey == "") {
		problemCount++
	}
	if b.config.WebSearchEnabled && (b.webSearch == nil || b.config.GoogleSearchAPIKey == "" || b.config.GoogleSearchEngineID == "") {
		problemCount++
	}
	if b.config.LongTermMemoryEnabled {
		if b.embeddingClient == nil {
			problemCount++
		} else if _, ok := b.storage.(*storage.PostgresStorage); !ok {
			problemCount++
		}
	}
	if b.config.ReactionsEnabled && (b.reactionTracker == nil || b.reactionHandler == nil) {
		problemCount++
	}
	if b.config.AntiRepetitionEnabled && b.antiRepetitionService == nil {
		problemCount++
	}
	if b.config.CausalLearningEnabled {
		if causalMemory, err := b.storage.GetCausalMemory(chatID); err != nil || causalMemory == nil {
			problemCount++
		}
	}
	if b.config.EmotionalLearningEnabled {
		// Проверяем доступность эмоциональных промптов
		if !b.config.EmotionalAnalysisPromptEnabled ||
			!b.config.EmotionalAdaptationPromptEnabled ||
			!b.config.EmotionalFeedbackPromptEnabled {
			problemCount++
		}
		// Проверяем наличие эмоционального состояния
		if emotionalState, err := b.storage.GetEmotionalState(chatID); err != nil || emotionalState == nil {
			problemCount++
		}
	}
	if b.llm == nil {
		problemCount++
	}

	// Итоговая статистика
	if problemCount == 0 {
		sb.WriteString("\n🎉 Все системы работают нормально!")
	} else {
		sb.WriteString(fmt.Sprintf("\n⚠️ Обнаружено проблем: %d", problemCount))
	}

	sb.WriteString("\n\n⏰ Сообщение автоматически удалится через 1 минуту")

	// Отправляем сообщение с автоудалением
	msg := tgbotapi.NewMessage(chatID, sb.String())
	// Убираем parse_mode чтобы избежать ошибок парсинга Markdown с эмодзи
	// msg.ParseMode = tgbotapi.ModeMarkdown

	log.Printf("[StartupMessage][%d] Подготовлено сообщение для отправки. Длина: %d символов", chatID, len(sb.String()))
	log.Printf("[StartupMessage][%d] Отправка startup message в чат %d...", chatID, chatID)

	sentMsg, err := b.api.Send(msg)
	if err != nil {
		log.Printf("[StartupMessage][%d] ❌ ОШИБКА при отправке: %v", chatID, err)
		if b.isUserBlockedError(err) {
			log.Printf("[StartupMessage][%d] Пользователь %d заблокировал бота (403), пропускаю отправку стартового сообщения", chatID, chatID)
			b.markChatAsInactive(chatID)
			return
		}
		log.Printf("[StartupMessage][%d] ❌ КРИТИЧЕСКАЯ ОШИБКА отправки стартового сообщения в чат %d: %v", chatID, chatID, err)
		return
	}

	log.Printf("[StartupMessage][%d] ✅ Сообщение успешно отправлено! MessageID: %d", chatID, sentMsg.MessageID)

	// НЕ сохраняем стартовое сообщение в БД - оно не должно попадать в историю

	// Запланируем удаление сообщения через 1 минуту
	time.AfterFunc(1*time.Minute, func() {
		log.Printf("[StartupMessage][%d] Попытка удаления startup message (ID: %d)", chatID, sentMsg.MessageID)
		if b.checkBotDeletePermissions(chatID) {
			if b.deleteMessageSilent(chatID, sentMsg.MessageID) {
				log.Printf("[StartupMessage][%d] ✅ Startup message удален из чата %d (ID: %d)", chatID, chatID, sentMsg.MessageID)
			} else {
				log.Printf("[StartupMessage][%d] ❌ Не удалось удалить startup message из чата %d (ID: %d)", chatID, chatID, sentMsg.MessageID)
			}
		} else {
			log.Printf("[StartupMessage][%d] ⚠️ Нет прав на удаление startup message в чате %d", chatID, chatID)
		}
	})

	log.Printf("[StartupMessage][%d] ✅ Startup message отправлен в чат %d (ID: %d), автоудаление через 1 минуту", chatID, chatID, sentMsg.MessageID)
	log.Printf("[StartupMessage][%d] === КОНЕЦ ОТПРАВКИ STARTUP MESSAGE ===", chatID)
}
