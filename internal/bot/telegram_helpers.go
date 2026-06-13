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
	text = SanitizeThinkTags(text)
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
	text = SanitizeThinkTags(text)
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
	text = SanitizeThinkTags(text)
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
	text = SanitizeThinkTags(text)
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
	text = SanitizeThinkTags(text)
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
	text = SanitizeThinkTags(text)
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

	// Отправляем сообщение (оригинальное или переработанное)
	b.sendReplyTo(chatID, messageID, text)

	// Записываем в память анти-повторений только если система включена
	if b.config.AntiRepetitionEnabled && b.antiRepetitionService != nil {
		b.antiRepetitionService.RecordResponse(chatID, text, userID, responseType, messageID)
	}
}

// sendSummaryReply отправляет суточное саммари с пометкой флагом summary=true
func (b *Bot) sendSummaryReply(chatID int64, text string) {
	text = SanitizeThinkTags(text)
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
	text = SanitizeThinkTags(text)
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

// sendStatusMessage отправляет информативное сообщение о состоянии всех систем бота.
// Вызывается по команде /status или через меню Настройки → Статус.
// Сообщение автоматически удаляется через 1 минуту и НЕ сохраняется в истории.
func (b *Bot) sendStatusMessage(chatID int64) {
	log.Printf("[StatusMessage][%d] Запрос статуса систем", chatID)

	if b.api == nil || b.config == nil || b.storage == nil {
		log.Printf("[StatusMessage][%d] ❌ Критические компоненты не инициализированы", chatID)
		return
	}

	var sb strings.Builder
	sb.WriteString("📊 Статус систем Luna\n\n")

	// ── Аптайм ──
	if !b.startTime.IsZero() {
		uptime := time.Since(b.startTime).Round(time.Second)
		sb.WriteString(fmt.Sprintf("⏱ Аптайм: %s\n\n", formatDuration(uptime)))
	}

	// ── Модели и их healthcheck ──
	sb.WriteString("🤖 Модели:\n")

	// Текстовая LLM (основная, для free_will и итоговых ответов)
	textProvider := b.getProviderDisplayName()
	textModel := b.getCurrentModelName()
	llmHealth := "✅"
	if b.llm == nil {
		llmHealth = "❌"
	}
	sb.WriteString(fmt.Sprintf("  💬 Текст (LLM): %s / %s %s\n", textProvider, textModel, llmHealth))

	// STT / аудио транскрибация
	sttModel := b.config.AudioTranscriptionModel
	if sttModel == "" {
		// Используем ту же модель что и для текста, если провайдер local
		sttModel = b.getCurrentModelName()
	}
	sttHealth := "✅"
	if b.embeddingClient == nil {
		sttHealth = "❌"
	}
	sb.WriteString(fmt.Sprintf("  🎤 Распознавание речи (STT): %s %s\n", sttModel, sttHealth))

	// TTS
	ttsModel := b.config.ElevenLabsModel
	ttsProvider := "ElevenLabs"
	ttsHealth := "✅"
	if b.voiceMessageService == nil || b.config.ElevenLabsAPIKey == "" {
		ttsHealth = "⚠️"
		ttsProvider = "Gemini TTS (fallback)"
		ttsModel = "gemini-2.5-flash-preview-tts"
	}
	sb.WriteString(fmt.Sprintf("  🗣️ Озвучка (TTS): %s / %s %s\n", ttsProvider, ttsModel, ttsHealth))

	// Image generation
	imgModel := b.config.ImageGenerationModel
	if imgModel == "" {
		imgModel = "gemini-2.5-flash-image-preview"
	}
	imgHealth := "✅"
	if b.imageGenerationService == nil || !b.imageGenerationService.IsEnabled() {
		imgHealth = "❌ откл."
	}
	sb.WriteString(fmt.Sprintf("  🖼️ Генерация изображений: %s %s\n", imgModel, imgHealth))

	// Embedding model
	embModel := b.config.GeminiEmbeddingModelName
	if embModel == "" {
		embModel = "embedding-001"
	}
	sb.WriteString(fmt.Sprintf("  🧬 Эмбеддинги: %s\n\n", embModel))

	// ── Хранилище ──
	sb.WriteString("💾 Хранилище:\n")
	sb.WriteString(fmt.Sprintf("  Тип: %s\n", b.config.StorageType))
	msgCount := 0
	storageOK := true
	if pgStore, ok := b.storage.(*storage.PostgresStorage); ok {
		if count, err := pgStore.GetTotalMessagesCount(chatID); err == nil {
			msgCount = int(count)
		} else {
			storageOK = false
		}
	} else if fileStore, ok := b.storage.(*storage.FileStorage); ok {
		if messages, err := fileStore.GetMessages(chatID, 100000); err == nil {
			msgCount = len(messages)
		} else {
			storageOK = false
		}
	}
	if storageOK {
		sb.WriteString(fmt.Sprintf("  Сообщений в БД: %d ✅\n\n", msgCount))
	} else {
		sb.WriteString(fmt.Sprintf("  Сообщений в БД: ошибка доступа ❌\n\n"))
	}

	// ── Сервисы и модули ──
	sb.WriteString("🔧 Сервисы:\n")

	// Модерация
	modStatus := "❌ отключена"
	if b.moderation != nil {
		b.moderation.mutex.RLock()
		if isActive, exists := b.moderation.activeChats[chatID]; exists && isActive {
			modStatus = "✅ активна"
		} else if !b.config.ModEnabled {
			modStatus = "❌ отключена в конфиге"
		} else {
			modStatus = "⚠️ нет прав админа"
		}
		b.moderation.mutex.RUnlock()
	}
	sb.WriteString(fmt.Sprintf("  🛡️ Модерация: %s\n", modStatus))

	// Free Will
	fwStatus := "❌ отключена"
	if b.freeWillService != nil && b.freeWillService.IsEnabled() {
		fwStatus = "✅ активна"
	} else if !b.config.FreeWillEnabled {
		fwStatus = "❌ отключена в конфиге"
	}
	sb.WriteString(fmt.Sprintf("  🧠 Свобода воли: %s\n", fwStatus))

	// Анализ фото
	photoStatus := "❌ отключен"
	if b.config.PhotoAnalysisEnabled {
		if b.embeddingClient != nil {
			photoStatus = "✅ включен"
		} else {
			photoStatus = "❌ нет клиента"
		}
	}
	sb.WriteString(fmt.Sprintf("  🖼️ Анализ фото: %s\n", photoStatus))

	// Веб-поиск
	webStatus := "❌ отключен"
	if b.webSearch != nil && b.config.GoogleSearchAPIKey != "" && b.config.GoogleSearchEngineID != "" {
		webStatus = "✅ включен"
	} else if !b.config.WebSearchEnabled {
		webStatus = "❌ отключен в конфиге"
	} else {
		webStatus = "❌ нет API ключей"
	}
	sb.WriteString(fmt.Sprintf("  🔍 Веб-поиск: %s\n", webStatus))

	// Реакции
	rxnStatus := "❌ отключены"
	if b.reactionTracker != nil && b.reactionHandler != nil && b.config.ReactionsEnabled {
		rxnStatus = "✅ включены"
	}
	sb.WriteString(fmt.Sprintf("  😊 Реакции: %s\n", rxnStatus))

	// Долгосрочная память
	memStatus := "❌ отключена"
	if b.config.LongTermMemoryEnabled && b.embeddingClient != nil {
		memStatus = "✅ включена"
	}
	sb.WriteString(fmt.Sprintf("  💭 Долгосрочная память: %s\n", memStatus))

	// Авто-саммари
	sumStatus := "❌ отключено"
	if b.config.SummaryIntervalHours > 0 {
		sumStatus = fmt.Sprintf("✅ каждые %d ч", b.config.SummaryIntervalHours)
	}
	sb.WriteString(fmt.Sprintf("  📝 Автосаммари: %s\n", sumStatus))

	// Анализ срачей
	srachStatus := "❌ отключен"
	if b.config.SrachAnalysisEnabled {
		srachStatus = "✅ включен"
	}
	sb.WriteString(fmt.Sprintf("  ⚔️ Анализ срачей: %s\n", srachStatus))

	// Анти-повторения
	antiStatus := "❌ отключена"
	if b.antiRepetitionService != nil && b.config.AntiRepetitionEnabled {
		antiStatus = "✅ включена"
	}
	sb.WriteString(fmt.Sprintf("  🔄 Анти-повторения: %s\n", antiStatus))

	// Эмоциональная система
	emoStatus := "❌ отключена"
	if b.config.EmotionalLearningEnabled {
		emoStatus = "✅ включена"
	}
	sb.WriteString(fmt.Sprintf("  🎭 Эмоциональная система: %s\n", emoStatus))

	// Каузальное обучение
	causalStatus := "❌ отключено"
	if b.config.CausalLearningEnabled {
		causalStatus = "✅ включено"
	}
	sb.WriteString(fmt.Sprintf("  🧠 Каузальное обучение: %s\n", causalStatus))

	// Система убеждений
	beliefStatus := "❌ отключена"
	if b.config.BeliefLearningEnabled {
		beliefStatus = "✅ включена"
	}
	sb.WriteString(fmt.Sprintf("  💡 Система убеждений: %s\n", beliefStatus))

	// Облако ассоциаций
	assocStatus := "❌ отключено"
	if b.config.AssociationCloudEnabled {
		assocStatus = "✅ включено"
	}
	sb.WriteString(fmt.Sprintf("  ☁️ Облако ассоциаций: %s\n\n", assocStatus))

	// ── Админ-права ──
	sb.WriteString("👑 Админ-права:\n")
	if len(b.config.AdminUsernames) > 0 {
		sb.WriteString(fmt.Sprintf("  Пользователи: %s\n", strings.Join(b.config.AdminUsernames, ", ")))
	}
	hasAdmin := b.config.AdminID != 0 || len(b.config.AdminUsernames) > 0
	if b.config.AdminID != 0 {
		sb.WriteString(fmt.Sprintf("  ID: %d\n", b.config.AdminID))
	}
	sb.WriteString(fmt.Sprintf("  Проверка прав: %s\n\n", map[bool]string{true: "✅ настроена", false: "⚠️ админы не заданы"}[hasAdmin]))

	// ── Основные параметры ──
	sb.WriteString("⚙️ Параметры:\n")
	sb.WriteString(fmt.Sprintf("  Интервал ответов: %d–%d сообщений\n", b.config.MinMessages, b.config.MaxMessages))
	sb.WriteString(fmt.Sprintf("  Окно контекста: %d сообщений\n", b.config.ContextWindow))
	sb.WriteString(fmt.Sprintf("  Часовой пояс: %s\n", b.config.TimeZone))
	sb.WriteString(fmt.Sprintf("  Тема дня: %02d:00\n\n", b.config.DailyTakeTime))

	// ── Итого ──
	sb.WriteString("⏰ Сообщение удалится через 1 минуту")

	// Отправка
	msg := tgbotapi.NewMessage(chatID, sb.String())
	sentMsg, err := b.api.Send(msg)
	if err != nil {
		log.Printf("[StatusMessage][%d] ❌ Ошибка отправки: %v", chatID, err)
		if b.isUserBlockedError(err) {
			b.markChatAsInactive(chatID)
		}
		return
	}

	log.Printf("[StatusMessage][%d] ✅ Отправлен (MsgID: %d), автоудаление через 1 мин", chatID, sentMsg.MessageID)

	// Автоудаление через минуту
	time.AfterFunc(1*time.Minute, func() {
		if b.checkBotDeletePermissions(chatID) {
			b.deleteMessageSilent(chatID, sentMsg.MessageID)
		}
	})
}

// sendStartupMessage — устаревшая обёртка, перенаправляет на sendStatusMessage.
// Оставлена для обратной совместимости с /status командой.
func (b *Bot) sendStartupMessage(chatID int64) {
	b.sendStatusMessage(chatID)
}

// formatDuration форматирует time.Duration в читаемый вид.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%d ч %d мин", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%d мин %d сек", m, s)
	}
	return fmt.Sprintf("%d сек", s)
}

// setTypingAction отправляет статус "печатает..." в чат.
// Используется перед LLM-вызовами для индикации генерации ответа.
// Telegram сам отменяет действие через ~5 секунд или при отправке сообщения.
func (b *Bot) setTypingAction(chatID int64) {
	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	if _, err := b.api.Request(action); err != nil {
		// Тихая ошибка — typing-индикатор не критичен для работы
		if b.config.Debug {
			log.Printf("[DEBUG][typing] Не удалось установить статус печати для чата %d: %v", chatID, err)
		}
	}
}
