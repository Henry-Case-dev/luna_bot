package bot

import (
	"fmt"
	"log"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ChatSettings содержит настройки для каждого чата (в памяти)
// Используем оригинальные имена полей для совместимости с bot.go
type ChatSettings struct {
	Active bool
	// CustomPrompt string // Пока не используется
	MinMessages            int       // Минимальное количество сообщений (из config)
	MaxMessages            int       // Максимальное количество сообщений (из config)
	MessageCount           int       // Текущий счетчик сообщений
	NextTargetMessageCount int       // Следующее целевое значение для отправки сообщения
	LastMessageID          int       // ID последнего сообщения (для определения таргета)
	DailyTakeTime          int       // Время для "темы дня" (из config)
	PendingSetting         string    // Какую настройку ожидаем ввести
	SummaryIntervalHours   int       // Интервал авто-саммари (из config)
	LastAutoSummaryTime    time.Time // Время последнего авто-саммари

	// Поля, связанные с анализом срачей (хранятся в памяти)
	SrachAnalysisEnabled bool      `json:"srach_analysis_enabled"`  // Включен ли анализ срачей (это поле теперь в storage.ChatSettings)
	SrachState           string    `json:"srach_state"`             // Текущее состояние срача ("none", "detected", "analyzing")
	SrachStartTime       time.Time `json:"srach_start_time"`        // Время начала обнаруженного срача
	SrachMessages        []string  `json:"srach_messages"`          // Сообщения, собранные во время срача для анализа
	LastSrachTriggerTime time.Time `json:"last_srach_trigger_time"` // Время последнего триггерного сообщения для таймера завершения
	SrachLlmCheckCounter int       `json:"srach_llm_check_counter"` // Счетчик сообщений для LLM проверки срача

	// IDs for deletable messages (в памяти)
	LastMenuMessageID     int `json:"last_menu_message_id,omitempty"`     // ID последнего сообщения с главным меню
	LastSettingsMessageID int `json:"last_settings_message_id,omitempty"` // ID последнего сообщения с меню настроек
	LastInfoMessageID     int `json:"last_info_message_id,omitempty"`     // ID последнего информационного сообщения (запроса ввода)
}

// formatSummaryInterval форматирует интервал саммари для отображения
func formatSummaryInterval(intervalHours int) string {
	if intervalHours <= 0 {
		return "Выкл."
	}
	return fmt.Sprintf("%d ч.", intervalHours)
}

// formatEnabled форматирует текст и callback кнопки в зависимости от статуса (включено/выключено)
func formatEnabled(textEnabled, textDisabled string, isEnabled bool, callbackEnabled, callbackDisabled string) (string, string) {
	if isEnabled {
		return fmt.Sprintf("%s: Вкл ✅", textEnabled), callbackEnabled
	}
	return fmt.Sprintf("%s: Выкл ❌", textDisabled), callbackDisabled
}

// getEnabledStatusText возвращает текстовое представление статуса (Вкл/Выкл)
func getEnabledStatusText(enabled bool) string {
	if enabled {
		return "Вкл ✅"
	}
	return "Выкл ❌"
}

// getSettingsKeyboard создает клавиатуру с текущими настройками чата
// Принимает *storage.ChatSettings (из БД) и *config.Config для дефолтов
func getSettingsKeyboard(dbSettings *storage.ChatSettings, cfg *config.Config) tgbotapi.InlineKeyboardMarkup {
	rows := [][]tgbotapi.InlineKeyboardButton{}

	// Если dbSettings nil (например, ошибка загрузки), создаем пустой, чтобы не было паники
	if dbSettings == nil {
		dbSettings = &storage.ChatSettings{}
		log.Println("[WARN][getSettingsKeyboard] dbSettings был nil, используются пустые значения.")
	}
	// Если cfg nil, тоже создаем пустой (хотя это маловероятно)
	if cfg == nil {
		cfg = &config.Config{}
		log.Println("[WARN][getSettingsKeyboard] cfg был nil, используются пустые значения.")
	}

	// 1. Интервал сообщений - Глобальная настройка из cfg
	intervalText := fmt.Sprintf("Интервал: %d-%d сообщ.", cfg.MinMessages, cfg.MaxMessages)
	intervalRow := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(intervalText, "change_interval"), // TODO: Убедиться, что это действительно глобальная настройка
	}
	rows = append(rows, intervalRow)

	// 2. Время темы дня - Глобальная настройка из cfg
	dailyTakeText := fmt.Sprintf("Тема дня: %02d:00", cfg.DailyTakeTime)
	dailyTakeRow := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(dailyTakeText, "change_daily_time"), // TODO: Убедиться, что это действительно глобальная настройка
	}
	rows = append(rows, dailyTakeRow)

	// 3. Интервал авто-саммари - Глобальная настройка из cfg
	summaryIntervalText := fmt.Sprintf("Авто-саммари: %s", formatSummaryInterval(cfg.SummaryIntervalHours))
	summaryIntervalRow := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(summaryIntervalText, "change_summary_interval"), // TODO: Убедиться, что это действительно глобальная настройка
	}
	rows = append(rows, summaryIntervalRow)

	// 4. Анализ срачей (Вкл/Выкл) - Настройка чата из dbSettings
	srachEnabled := cfg.SrachAnalysisEnabled    // Дефолтное значение из глобального конфига
	if dbSettings.SrachAnalysisEnabled != nil { // Если в БД есть значение (не nil)
		srachEnabled = *dbSettings.SrachAnalysisEnabled // Используем его
	}
	// TODO: Удалить эту заглушку после реализации чтения из dbSettings
	// srachEnabled := false // ЗАГЛУШКА
	srachText, srachCallback := formatEnabled("🤬 Анализ срачей", "🤬 Анализ срачей", srachEnabled, "toggle_srach_analysis", "toggle_srach_analysis")
	srachRow := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(srachText, srachCallback),
	}
	rows = append(rows, srachRow)

	// 5. Транскрипция голоса (Вкл/Выкл) - Настройка чата из dbSettings
	voiceEnabled := cfg.VoiceTranscriptionEnabledDefault // Дефолтное значение из конфига
	if dbSettings.VoiceTranscriptionEnabled != nil {     // Если в БД есть значение (не nil)
		voiceEnabled = *dbSettings.VoiceTranscriptionEnabled // Используем его
	}
	voiceText, voiceCallback := formatEnabled("🎤 Распознавание голоса", "🎤 Распознавание голоса", voiceEnabled, "toggle_voice_transcription", "toggle_voice_transcription")
	voiceRow := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(voiceText, voiceCallback),
	}
	rows = append(rows, voiceRow)

	// --- Новые кнопки для лимита прямых обращений ---
	limitEnabled := cfg.DirectReplyLimitEnabledDefault
	if dbSettings.DirectReplyLimitEnabled != nil {
		limitEnabled = *dbSettings.DirectReplyLimitEnabled
	}
	limitCount := cfg.DirectReplyLimitCountDefault
	if dbSettings.DirectReplyLimitCount != nil {
		limitCount = *dbSettings.DirectReplyLimitCount
	}
	limitDurationMinutes := int(cfg.DirectReplyLimitDurationDefault.Minutes())
	if dbSettings.DirectReplyLimitDuration != nil {
		limitDurationMinutes = *dbSettings.DirectReplyLimitDuration
	}

	limitToggleText, limitToggleCallback := formatEnabled("Лимит прямых обращений", "Лимит прямых обращений", limitEnabled, "toggle_direct_limit", "toggle_direct_limit")
	limitToggleRow := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(limitToggleText, limitToggleCallback),
	)
	rows = append(rows, limitToggleRow)

	// Кнопки для изменения значений лимита (показываем только если лимит включен)
	if limitEnabled {
		limitValueText := fmt.Sprintf("%d за %d мин", limitCount, limitDurationMinutes)
		limitValueRow := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("Значение: %s", limitValueText), "change_direct_limit_values"), // Общий коллбэк
		)
		rows = append(rows, limitValueRow)
	}

	// 6. Добавляем кнопку "Назад"
	backRow := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", "back_to_main"),
	}
	rows = append(rows, backRow)

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}
