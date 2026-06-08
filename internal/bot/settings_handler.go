package bot

import (
	"fmt"
	"log"

	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// sendSettingsKeyboard отправляет клавиатуру настроек, удаляя предыдущую (если есть)
// и сохраняя ID нового сообщения.
func (b *Bot) sendSettingsKeyboard(chatID int64, lastSettingsMsgID int) {
	// Удаляем предыдущее сообщение настроек, если оно есть
	if lastSettingsMsgID != 0 {
		b.deleteMessageSilent(chatID, lastSettingsMsgID)
	}

	// Получаем актуальные настройки для отображения
	b.settingsMutex.RLock()
	settings, exists := b.chatSettings[chatID]
	if !exists {
		b.settingsMutex.RUnlock()
		log.Printf("sendSettingsKeyboard: Настройки для чата %d не найдены!", chatID)
		// Можно отправить сообщение об ошибке, но лучше просто не отправлять клавиатуру
		return
	}
	b.settingsMutex.RUnlock() // Разблокируем после получения указателя на settings в памяти

	// Загружаем настройки из БД для клавиатуры и текста
	dbSettings, err := b.storage.GetChatSettings(chatID)
	if err != nil {
		log.Printf("[ERROR][sendSettingsKeyboard] Чат %d: Ошибка получения настроек из DB: %v", chatID, err)
		// Используем пустые dbSettings, чтобы избежать паники
		dbSettings = &storage.ChatSettings{}
	}

	// Формируем текст сообщения
	msgText := `⚙️ *Настройки чата:*`
	if settings != nil { // Глобальные настройки из памяти/cfg
		msgText += fmt.Sprintf("\nИнтервал ответа: %d-%d сообщ. (текущий: %d)", settings.MinMessages, settings.MaxMessages, settings.NextTargetMessageCount)
		msgText += fmt.Sprintf("\nВремя 'темы дня': %02d:00", settings.DailyTakeTime)
		msgText += fmt.Sprintf("\nИнтервал авто-саммари: %s", formatSummaryInterval(settings.SummaryIntervalHours))
	} else {
		msgText += "\n(Не удалось загрузить глобальные настройки из памяти)"
	}

	// Добавляем специфичные для чата настройки из dbSettings
	// Срач анализ
	srachStatus := b.config.SrachAnalysisEnabled // Дефолт из конфига
	if dbSettings.SrachAnalysisEnabled != nil {
		srachStatus = *dbSettings.SrachAnalysisEnabled
	}
	msgText += fmt.Sprintf("\n🤬 Анализ срачей: %s", getEnabledStatusText(srachStatus))

	// Транскрипция голоса
	voiceStatus := b.config.VoiceTranscriptionEnabledDefault // Дефолт из конфига
	if dbSettings.VoiceTranscriptionEnabled != nil {
		voiceStatus = *dbSettings.VoiceTranscriptionEnabled
	}
	msgText += fmt.Sprintf("\n🎤 Распознавание голоса: %s", getEnabledStatusText(voiceStatus))

	limitEnabled := b.config.DirectReplyLimitEnabledDefault
	if dbSettings.DirectReplyLimitEnabled != nil {
		limitEnabled = *dbSettings.DirectReplyLimitEnabled
	}
	limitCount := b.config.DirectReplyLimitCountDefault
	if dbSettings.DirectReplyLimitCount != nil {
		limitCount = *dbSettings.DirectReplyLimitCount
	}
	limitDurationMinutes := int(b.config.DirectReplyLimitDurationDefault.Minutes())
	if dbSettings.DirectReplyLimitDuration != nil {
		limitDurationMinutes = *dbSettings.DirectReplyLimitDuration
	}
	msgText += fmt.Sprintf("\n🚫 Лимит прямых обращений: %s (%d за %d мин)",
		getEnabledStatusText(limitEnabled),
		limitCount,
		limitDurationMinutes)

	// Получаем клавиатуру настроек
	keyboard := getSettingsKeyboard(dbSettings, b.config)

	// Отправляем новое сообщение с клавиатурой
	msg := tgbotapi.NewMessage(chatID, msgText)
	msg.ReplyMarkup = keyboard
	msg.ParseMode = "Markdown"

	sentMsg, err := b.api.Send(msg)
	if err != nil {
		log.Printf("Ошибка отправки клавиатуры настроек в чат %d: %v", chatID, err)
		return
	}

	// Сохраняем ID нового сообщения настроек
	b.settingsMutex.Lock()
	if settings, exists := b.chatSettings[chatID]; exists { // Проверяем еще раз на всякий случай
		settings.LastSettingsMessageID = sentMsg.MessageID
		if b.config.Debug {
			log.Printf("[DEBUG] Сохранен новый LastSettingsMessageID: %d для чата %d", sentMsg.MessageID, chatID)
		}
	} else {
		log.Printf("[WARN] Настройки для чата %d не найдены при попытке сохранить новый LastSettingsMessageID.", chatID)
	}
	b.settingsMutex.Unlock()
}

// updateSettingsKeyboard обновляет существующее сообщение настроек
func (b *Bot) updateSettingsKeyboard(query *tgbotapi.CallbackQuery) {
	chatID := query.Message.Chat.ID
	messageID := query.Message.MessageID

	// Получаем актуальные настройки из памяти (для глобальных)
	b.settingsMutex.RLock()
	settings, exists := b.chatSettings[chatID]
	if !exists {
		b.settingsMutex.RUnlock()
		log.Printf("updateSettingsKeyboard: Настройки для чата %d не найдены!", chatID)
		b.answerCallback(query.ID, "Ошибка: настройки чата не найдены.")
		return
	}
	b.settingsMutex.RUnlock() // Разблокируем после чтения

	// Нам нужны настройки из БД (dbSettings) для клавиатуры и текста
	dbSettings, err := b.storage.GetChatSettings(chatID)
	if err != nil {
		log.Printf("[ERROR][updateSettingsKeyboard] Чат %d: Ошибка получения настроек из DB: %v", chatID, err)
		dbSettings = &storage.ChatSettings{}
	}

	// Формируем новый текст сообщения
	msgText := `⚙️ *Настройки чата:*`
	if settings != nil { // Глобальные настройки из памяти/cfg
		msgText += fmt.Sprintf("\nИнтервал ответа: %d-%d сообщ. (текущий: %d)", settings.MinMessages, settings.MaxMessages, settings.NextTargetMessageCount)
		msgText += fmt.Sprintf("\nВремя 'темы дня': %02d:00", settings.DailyTakeTime)
		msgText += fmt.Sprintf("\nИнтервал авто-саммари: %s", formatSummaryInterval(settings.SummaryIntervalHours))
	} else {
		msgText += "\n(Не удалось загрузить глобальные настройки из памяти)"
	}

	// Добавляем специфичные для чата настройки из dbSettings
	// Срач анализ
	srachStatus := b.config.SrachAnalysisEnabled // Дефолт из конфига
	if dbSettings.SrachAnalysisEnabled != nil {
		srachStatus = *dbSettings.SrachAnalysisEnabled
	}
	msgText += fmt.Sprintf("\n🤬 Анализ срачей: %s", getEnabledStatusText(srachStatus))

	// Транскрипция голоса
	voiceStatus := b.config.VoiceTranscriptionEnabledDefault // Дефолт из конфига
	if dbSettings.VoiceTranscriptionEnabled != nil {
		voiceStatus = *dbSettings.VoiceTranscriptionEnabled
	}
	msgText += fmt.Sprintf("\n🎤 Распознавание голоса: %s", getEnabledStatusText(voiceStatus))

	limitEnabled := b.config.DirectReplyLimitEnabledDefault
	if dbSettings.DirectReplyLimitEnabled != nil {
		limitEnabled = *dbSettings.DirectReplyLimitEnabled
	}
	limitCount := b.config.DirectReplyLimitCountDefault
	if dbSettings.DirectReplyLimitCount != nil {
		limitCount = *dbSettings.DirectReplyLimitCount
	}
	limitDurationMinutes := int(b.config.DirectReplyLimitDurationDefault.Minutes())
	if dbSettings.DirectReplyLimitDuration != nil {
		limitDurationMinutes = *dbSettings.DirectReplyLimitDuration
	}
	msgText += fmt.Sprintf("\n🚫 Лимит прямых обращений: %s (%d за %d мин)",
		getEnabledStatusText(limitEnabled),
		limitCount,
		limitDurationMinutes)

	// Получаем обновленную клавиатуру
	keyboard := getSettingsKeyboard(dbSettings, b.config)

	// Создаем конфиг для редактирования сообщения
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, msgText)
	editMsg.ReplyMarkup = &keyboard
	editMsg.ParseMode = "Markdown"

	_, errSend := b.api.Send(editMsg)
	if errSend != nil {
		log.Printf("Ошибка обновления клавиатуры настроек в чате %d: %v", chatID, errSend)
		b.answerCallback(query.ID, "Ошибка обновления настроек.")
	}
}
