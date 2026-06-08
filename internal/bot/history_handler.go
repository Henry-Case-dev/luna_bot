package bot

import (
	"log"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// loadChatHistory загружает историю сообщений для указанного чата
func (b *Bot) loadChatHistory(chatID int64) {
	if b.config.Debug {
		log.Printf("[DEBUG][Load History] Чат %d: Начинаю загрузку истории.", chatID)
	}

	// 1. УБРАНО: промежуточные сообщения о загрузке - теперь используется централизованное стартовое сообщение
	// initialStatus := b.storage.GetStatus(chatID) // Получаем статус ДО загрузки
	// initialMsgText := fmt.Sprintf("⏳ Загружаю историю чата...\nСтатус хранилища: %s", initialStatus)
	// initialMsg, errInit := b.sendReplyReturnMsg(chatID, initialMsgText)
	// var initialMsgID int
	// if errInit != nil {
	// 	log.Printf("[WARN][Load History] Чат %d: Не удалось отправить начальное сообщение: %v", chatID, errInit)
	// } else if initialMsg != nil {
	// 	initialMsgID = initialMsg.MessageID
	// }

	// 2. Загружаем историю (специфично для FileStorage)
	var history []*tgbotapi.Message
	var loadErr error
	if b.config.StorageType == config.StorageTypeFile {
		history, loadErr = b.storage.LoadChatHistory(chatID)
		if loadErr != nil {
			// Логируем ошибку, но не останавливаемся, просто начинаем без истории
			log.Printf("[ERROR][Load History] Чат %d: Ошибка загрузки истории из файла: %v", chatID, loadErr)
			// УБРАНО: отправка сообщений об ошибках - теперь используется централизованное стартовое сообщение
			// finalStatus := b.storage.GetStatus(chatID) // Статус ПОСЛЕ ошибки
			// finalMsgText := fmt.Sprintf("⚠️ Не удалось загрузить историю чата из файла.\nСтатус хранилища: %s", finalStatus)
			// b.sendReplyAndDeleteInitial(chatID, finalMsgText, initialMsgID)
			_ = b.storage.ClearChatHistory(chatID) // Очищаем память на всякий случай
			return
		}
	}

	// 3. Формируем итоговый текст сообщения
	// УБРАНО: переменные больше не используются после отключения отправки сообщений
	// finalMsgText := ""
	// loadedCount := 0

	if b.config.StorageType == config.StorageTypeFile {
		// Логика для FileStorage (как раньше)
		if history == nil { // Файл не найден
			if b.config.Debug {
				log.Printf("[DEBUG][Load History] Чат %d: История не найдена или файл не существует.", chatID)
			}
			// finalMsgText = "✅ История чата не найдена в файле."
		} else if len(history) == 0 { // Файл пуст
			if b.config.Debug {
				log.Printf("[DEBUG][Load History] Чат %d: Загружена пустая история (файл был пуст или содержал []).", chatID)
			}
			// finalMsgText = "✅ История чата в файле пуста."
		} else { // История из файла загружена
			loadCount := len(history)
			if loadCount > b.config.ContextWindow {
				log.Printf("[DEBUG][Load History] Чат %d: История из файла (%d) длиннее окна (%d), обрезаю.", chatID, loadCount, b.config.ContextWindow)
				history = history[loadCount-b.config.ContextWindow:]
				loadCount = len(history)
			}
			log.Printf("[DEBUG][Load History] Чат %d: Добавляю %d загруженных сообщений из файла в контекст.", chatID, loadCount)
			b.storage.AddMessagesToContext(chatID, history)
			// loadedCount = loadCount
			// finalMsgText = fmt.Sprintf("✅ Контекст загружен из файла: %d сообщений.", loadedCount)
		}
	} else {
		// Логика для MongoDB/PostgreSQL
		// История всегда "найдена", если есть подключение.
		// Просто выводим актуальный статус хранилища.
		// finalMsgText = "✅ Инициализация хранилища завершена."
		// Загрузка последних N сообщений в память не требуется для БД,
		// но можно запросить текущие GetMessages для логов
		if b.config.Debug {
			currentMsgs, errGet := b.storage.GetMessages(chatID, b.config.ContextWindow) // Используем ContextWindow как лимит
			if errGet != nil {
				log.Printf("[DEBUG][Load History] Чат %d: Ошибка при вызове GetMessages для лога: %v", chatID, errGet)
			} else {
				log.Printf("[DEBUG][Load History] Чат %d: Хранилище (%s) инициализировано. Текущий контекст (из GetMessages): %d сообщ.", chatID, b.config.StorageType, len(currentMsgs))
			}
		}
	}

	// 4. Отправляем итоговое сообщение со статусом и удаляем начальное
	// УБРАНО: теперь используется централизованное стартовое сообщение
	// finalStatus := b.storage.GetStatus(chatID) // Статус ПОСЛЕ загрузки/инициализации
	// b.sendReplyAndDeleteInitial(chatID, fmt.Sprintf("%s\nСтатус хранилища: %s", finalMsgText, finalStatus), initialMsgID)
}

// УДАЛЕНО: функции sendReplyReturnMsg и sendReplyAndDeleteInitial больше не используются
// после внедрения централизованного стартового сообщения
