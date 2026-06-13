package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// === Вспомогательные функции для саммари ===

// createAndSendSummary генерирует и отправляет саммари чата
func (b *Bot) createAndSendSummary(chatID int64) {
	log.Printf("[Summary START] Chat %d: Начало генерации саммари через Gemini (независимо от LLM_PROVIDER)...", chatID)
	startTime := time.Now()

	// 1. Получаем ID последнего информационного сообщения (чтобы потом его обновить)
	b.settingsMutex.RLock()
	settings, exists := b.chatSettings[chatID]
	if !exists {
		b.settingsMutex.RUnlock()
		log.Printf("[Summary ERROR] Chat %d: Настройки не найдены в памяти!", chatID)
		// Попытаться отправить сообщение об ошибке?
		return
	}
	lastInfoMsgID := settings.LastInfoMessageID
	b.settingsMutex.RUnlock()

	// 2. Получаем историю сообщений за последние 24 часа
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute) // Таймаут на получение истории
	defer cancel()

	history, err := b.storage.GetMessagesSince(ctx, chatID, 0, time.Now().Add(-24*time.Hour), 5000) // Лимит 5000 для саммари
	if err != nil {
		log.Printf("[Summary ERROR] Chat %d: Ошибка получения истории: %v", chatID, err)
		b.updateOrSendMessage(chatID, lastInfoMsgID, "❌ Ошибка получения истории чата для саммари.", "❌ Ошибка получения истории чата для саммари.", "")
		return
	}

	if len(history) == 0 {
		log.Printf("[Summary INFO] Chat %d: Нет сообщений за последние 24 часа для саммари.", chatID)
		b.updateOrSendMessage(chatID, lastInfoMsgID, "🤷 За последние 24 часа нет сообщений для саммари.", "🤷 За последние 24 часа нет сообщений для саммари.", "")
		return
	}

	// 3. Фильтруем служебные/бот-сообщения и собираем диагностику состава
	var userHistory []*tgbotapi.Message
	total := len(history)
	botCount := 0
	userCount := 0
	textCount := 0
	mediaCount := 0
	for _, m := range history {
		if m == nil {
			continue
		}
		// Исключаем сообщения, отправленные самим ботом
		if m.From != nil && m.From.ID == b.api.Self.ID {
			botCount++
			continue
		}
		// Подсчёт типов
		msgText := m.Text
		if msgText == "" {
			msgText = m.Caption
		}
		if strings.TrimSpace(msgText) != "" {
			textCount++
		} else if m.Voice != nil || len(m.Photo) > 0 || m.Video != nil || m.Document != nil || m.Audio != nil || m.Sticker != nil {
			mediaCount++
		}
		userCount++
		userHistory = append(userHistory, m)
	}
	log.Printf("[Summary DIAG] Chat %d: всего=%d; от бота=%d; от участников=%d; текст=%d; медиа=%d", chatID, total, botCount, userCount, textCount, mediaCount)

	if len(userHistory) == 0 {
		log.Printf("[Summary INFO] Chat %d: Нет пользовательских сообщений за 24ч (бот/служебные исключены)", chatID)
		b.updateOrSendMessage(chatID, lastInfoMsgID, "🤷 За последние 24 часа нет сообщений для саммари.", "🤷 За последние 24 часа нет сообщений для саммари.", "")
		return
	}

	// Используем новый унифицированный форматтер
	formatter := NewUnifiedMessageFormatter(b.storage, b.config.TimeZone)
	formatter.SetDisableUserProfiles(b.config.DisableUserProfiles)
	formattedHistory := formatter.FormatMessagesXML(chatID, userHistory)

	// Диагностика размера контекста
	rh := utf8.RuneCountInString(formattedHistory)
	estTokens := rh / 4 // грубая оценка токенов
	log.Printf("[Summary INFO] Chat %d: История для саммари (унифицированный форматтер): сообщений=%d, символов=%d, ~токенов≈%d", chatID, len(userHistory), len(formattedHistory), estTokens)

	// 4. Генерируем саммари с помощью Gemini (всегда, независимо от основного LLM провайдера)
	var summary string
	llmStartTime := time.Now()
	maxRetries := 3
	retryDelay := 5 * time.Second

	for i := 0; i < maxRetries; i++ {
		// ИСПРАВЛЕНИЕ: Всегда используем Gemini для саммари, независимо от LLM_PROVIDER
		// Встраиваем личность в промпт саммари
		enrichedSummaryPrompt := b.enrichPromptWithPersonality(b.config.SummaryPrompt, chatID, "summary")
		if i == 0 { // логируем только один раз, усечённо
			log.Printf("[Summary DEBUG] Chat %d: Используется промпт саммари (усечён): %.120s", chatID, enrichedSummaryPrompt)
		}
		summary, err = b.embeddingClient.GenerateResponseByType(llm.ResponseTypeSummary, enrichedSummaryPrompt, formattedHistory, float32(b.config.GeminiTemperatureNormal))
		if err == nil {
			break // Успех
		}

		// Проверяем на ошибку rate limit (429)
		if strings.Contains(err.Error(), "429") || strings.Contains(strings.ToLower(err.Error()), "rate limit") {
			log.Printf("[Summary WARN] Chat %d: Ошибка Rate Limit (429) при генерации саммари (Попытка %d/%d). Ожидание %v...", chatID, i+1, maxRetries, retryDelay)
			time.Sleep(retryDelay)
			retryDelay *= 2 // Экспоненциальная задержка
		} else {
			// Другая ошибка, прекращаем попытки
			log.Printf("[Summary ERROR] Chat %d: Неисправимая ошибка Gemini при генерации саммари: %v", chatID, err)
			break
		}
	}

	// Если после всех попыток ошибка все еще есть
	if err != nil {
		log.Printf("[Summary ERROR] Chat %d: Не удалось сгенерировать саммари через Gemini после %d попыток: %v", chatID, maxRetries, err)
		b.updateOrSendMessage(chatID, lastInfoMsgID, "❌ Ошибка: Не удалось сгенерировать саммари.", "Произошла ошибка. Попробуйте позже.", "")
		return
	}

	// Очищаем саммари от возможных метаданных
	log.Printf("🧹 [SUMMARY] Очистка дневного саммари для чата %d (исходная длина: %d)", chatID, len(summary))
	summary = cleanupLLMResponse(summary)

	llmDuration := time.Since(llmStartTime)

	if summary == "" {
		log.Printf("[Summary WARN] Chat %d: Gemini вернул пустое саммари. Пытаюсь fallback-композицию.", chatID)
		fallback := basicFallbackSummary(formattedHistory)
		if strings.TrimSpace(fallback) == "" {
			b.updateOrSendMessage(chatID, lastInfoMsgID, "🤔 Gemini не смог сгенерировать саммари (вернул пустой ответ).", "🤔 Gemini не смог сгенерировать саммари (вернул пустой ответ).", "")
			return
		}
		summary = fallback
	}

	// 5. Подготовка к отправке
	finalSummary := strings.TrimSpace(summary)

	// Санитизируем Markdown разметку, чтобы избежать ошибок парсинга
	finalSummary = sanitizeMarkdown(finalSummary)

	// --- Логика выбора ParseMode и экранирования УДАЛЕНА ---
	// Всегда используем стандартный Markdown
	parseMode := tgbotapi.ModeMarkdown

	// 6. Отправка сообщения с разбивкой на части
	// Разбиваем на части, если превышает лимит Telegram
	messageParts := b.splitMessageForTelegram(finalSummary, b.config.SummaryMaxParts)
	log.Printf("[Summary][HANDLER] Chat %d: Сообщение разбито на %d частей (лимит: %d)", chatID, len(messageParts), b.config.SummaryMaxParts)

	// Если есть сообщение для обновления и только одна часть - обновляем его
	if lastInfoMsgID != 0 && len(messageParts) == 1 {
		// Пытаемся обновить существующее сообщение
		b.updateOrSendMessage(chatID, lastInfoMsgID, messageParts[0], messageParts[0], parseMode)
	} else {
		// Отправляем как новое сообщение (возможно в нескольких частях)
		for i, part := range messageParts {
			if i > 0 {
				// Добавляем небольшую задержку между частями
				time.Sleep(1 * time.Second)
			}

			// Добавляем индикацию части, если сообщение было разбито
			if len(messageParts) > 1 {
				partPrefix := fmt.Sprintf("📄 *Часть %d/%d*\n\n", i+1, len(messageParts))
				part = partPrefix + part
			}

			log.Printf("[Summary][HANDLER] Chat %d: Отправка части %d/%d суточного саммари (длина: %d символов)", chatID, i+1, len(messageParts), len(part))
			b.sendSummaryReply(chatID, part)
			log.Printf("[Summary][HANDLER] Chat %d: Часть %d/%d успешно отправлена", chatID, i+1, len(messageParts))
		}
	}

	// 7. Очищаем LastInfoMessageID после успешной отправки/обновления
	b.settingsMutex.Lock()
	if settings, exists := b.chatSettings[chatID]; exists {
		settings.LastInfoMessageID = 0
	}
	b.settingsMutex.Unlock()

	// Логирование завершения
	totalDuration := time.Since(startTime)
	log.Printf("[Summary COMPLETE] Chat %d: Саммари сгенерировано через Gemini и отправлено в %d частях. Gemini: %v, Total: %v",
		chatID, len(messageParts), llmDuration, totalDuration)
}

// basicFallbackSummary — простой резервный механизм, когда LLM вернул пусто
func basicFallbackSummary(formattedHistory string) string {
	if formattedHistory == "" {
		return ""
	}
	// Берём первые ~1200 символов как конденсированный контекст
	max := 1200
	if len(formattedHistory) > max {
		return "Короткое саммари (fallback):\n" + formattedHistory[:max] + "…"
	}
	return "Короткое саммари (fallback):\n" + formattedHistory
}

// updateOrSendMessage пытается обновить сообщение messageIDToEdit текстом editText.
// Если messageIDToEdit = 0 или обновление не удается, отправляет новое сообщение с текстом sendText.
func (b *Bot) updateOrSendMessage(chatID int64, messageIDToEdit int, editText string, sendText string, parseMode string) {
	// Проверяем, является ли сообщение ошибкой (содержит символ ❌)
	isErrorMessage := strings.Contains(editText, "❌") || strings.Contains(sendText, "❌")

	// Если это сообщение об ошибке и оно не редактирование существующего, используем автоудаление
	if isErrorMessage && messageIDToEdit == 0 {
		b.sendAutoDeleteErrorReply(chatID, 0, sendText)
		return
	}

	updated := false
	isMarkdownError := false

	// Сначала попробуем отправить с Markdown
	if messageIDToEdit != 0 {
		// Пытаемся обновить существующее сообщение с использованием Markdown
		msg := tgbotapi.NewEditMessageText(chatID, messageIDToEdit, editText)
		if parseMode != "" {
			msg.ParseMode = parseMode
		}
		_, err := b.api.Send(msg)

		if err == nil {
			updated = true
			log.Printf("[DEBUG][updateOrSendMessage] Chat %d: Сообщение %d успешно обновлено с ParseMode=%s.", chatID, messageIDToEdit, parseMode)
		} else {
			// Проверяем, связана ли ошибка с Markdown
			errMsg := err.Error()
			if strings.Contains(errMsg, "can't parse entities") ||
				strings.Contains(errMsg, "markdown") ||
				strings.Contains(errMsg, "parse") {
				isMarkdownError = true
				log.Printf("[WARN][updateOrSendMessage] Chat %d: Ошибка Markdown при обновлении сообщения %d: %v. Пробуем без Markdown.",
					chatID, messageIDToEdit, err)

				// Если ошибка связана с Markdown, отправляем без форматирования
				msg.ParseMode = "" // Убираем ParseMode
				_, errPlain := b.api.Send(msg)
				if errPlain == nil {
					updated = true
					log.Printf("[DEBUG][updateOrSendMessage] Chat %d: Сообщение %d успешно обновлено без Markdown.", chatID, messageIDToEdit)
				} else {
					log.Printf("[ERROR][updateOrSendMessage] Chat %d: Не удалось обновить сообщение %d даже без Markdown: %v",
						chatID, messageIDToEdit, errPlain)
				}
			} else if strings.Contains(errMsg, "message to edit not found") ||
				strings.Contains(errMsg, "message can't be edited") ||
				strings.Contains(errMsg, "message identifier is not specified") ||
				strings.Contains(errMsg, "message is not modified") {
				log.Printf("[INFO][updateOrSendMessage] Chat %d: Не удалось обновить сообщение %d (%v), будет отправлено новое.",
					chatID, messageIDToEdit, err)
			} else {
				// Другая, возможно, более серьезная ошибка
				log.Printf("[ERROR][updateOrSendMessage] Chat %d: Неожиданная ошибка при обновлении сообщения %d: %v",
					chatID, messageIDToEdit, err)
			}
		}
	}

	// Если обновление не удалось или messageIDToEdit = 0, отправляем новое сообщение
	if !updated {
		// Если это сообщение об ошибке, используем автоудаление
		if isErrorMessage {
			b.sendAutoDeleteErrorReply(chatID, 0, sendText)
			return
		}

		// Отправляем новое сообщение
		msg := tgbotapi.NewMessage(chatID, sendText)

		// Если была ошибка Markdown при обновлении, не используем его и при отправке
		if !isMarkdownError && parseMode != "" {
			msg.ParseMode = parseMode
		}

		sentMsg, err := b.api.Send(msg)
		if err != nil {
			// Если ошибка при отправке с Markdown, пробуем без него
			if strings.Contains(err.Error(), "can't parse entities") ||
				strings.Contains(err.Error(), "markdown") ||
				strings.Contains(err.Error(), "parse") {

				log.Printf("[WARN][updateOrSendMessage] Chat %d: Ошибка Markdown при отправке нового сообщения: %v. Пробуем без Markdown.", chatID, err)

				// Отправляем без форматирования
				msg.ParseMode = ""
				sentMsg, err = b.api.Send(msg)
				if err != nil {
					log.Printf("[ERROR][updateOrSendMessage] Chat %d: Не удалось отправить сообщение даже без Markdown: %v", chatID, err)
					return
				}
			} else {
				log.Printf("[ERROR][updateOrSendMessage] Chat %d: Ошибка отправки сообщения: %v", chatID, err)
				return
			}
		}

		// Сохраняем ID отправленного сообщения в настройках чата
		b.settingsMutex.Lock()
		if settings, exists := b.chatSettings[chatID]; exists {
			if messageIDToEdit != 0 {
				// Если было сообщение, которое пытались обновить, сохраняем его ID
				settings.LastInfoMessageID = sentMsg.MessageID
			}
		}
		b.settingsMutex.Unlock()
	}
}

// --- Функции для обрезки строк (дублируются?) ---
// TODO: Вынести в utils или использовать существующие из utils?

// truncateString обрезает строку до максимальной длины, добавляя "..."
// func truncateString(s string, maxLen int) string {
// 	if utf8.RuneCountInString(s) <= maxLen {
// 		return s
// 	}
// 	if maxLen < 3 {
// 		return "..."[:maxLen] // Возвращаем часть "..."
// 	}
// 	runes := []rune(s)
// 	return string(runes[:maxLen-3]) + "..."
// }

// truncateStringEnd обрезает строку до максимальной длины без добавления "..."
func truncateStringEnd(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen])
}

// === Функции для еженедельного саммари ===

// createAndSendWeeklySummary генерирует и отправляет еженедельное саммари чата
func (b *Bot) createAndSendWeeklySummary(chatID int64) {
	log.Printf("[WeeklySummary START] Chat %d: Начало генерации еженедельного саммари...", chatID)
	startTime := time.Now()

	// Удаляем информационное сообщение "Генерирую еженедельное саммари, подождите..."
	b.settingsMutex.Lock()
	if set, ok := b.chatSettings[chatID]; ok && set.LastInfoMessageID != 0 {
		// Сбрасываем, чтобы не пытаться редактировать старое инфо-сообщение
		set.LastInfoMessageID = 0
	}
	b.settingsMutex.Unlock()

	// Проверяем, включено ли еженедельное саммари
	if !b.config.WeeklySummaryEnabled {
		log.Printf("[WeeklySummary SKIP] Chat %d: Еженедельное саммари отключено в конфиге", chatID)
		return
	}

	// Получаем временной диапазон для последних 7 дней
	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7)

	// Получаем локацию из конфига
	loc, err := time.LoadLocation(b.config.TimeZone)
	if err != nil {
		log.Printf("[WeeklySummary WARN] Chat %d: Ошибка загрузки часового пояса, используем UTC: %v", chatID, err)
		loc = time.UTC
	}

	// Приводим время к локальному часовому поясу
	nowLocal := now.In(loc)
	weekAgoLocal := weekAgo.In(loc)

	log.Printf("[WeeklySummary][HANDLER] Chat %d: Ищем саммари с %v по %v (локальное время: %s)", chatID, weekAgoLocal.Format("2006-01-02 15:04:05"), nowLocal.Format("2006-01-02 15:04:05"), loc.String())

	// Получаем ежедневные саммари за последнюю неделю
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Получаем ID бота из конфига
	botUserID := int64(7592685771) // Хардкодим, так как это константа из env.txt

	dailySummaries, err := b.storage.GetDailySummariesForWeek(ctx, chatID, botUserID, weekAgoLocal, nowLocal)
	if err != nil {
		log.Printf("[WeeklySummary ERROR] Chat %d: Ошибка получения ежедневных саммари: %v", chatID, err)
		// Отправляем сообщение об ошибке пользователю
		b.sendTemporaryMessage(chatID, "❌ Ошибка получения ежедневных саммари для создания еженедельного отчета.", 30*time.Second)
		return
	}

	if len(dailySummaries) == 0 {
		log.Printf("[WeeklySummary] Chat %d: Нет ежедневных саммари за последние 7 дней", chatID)
		b.sendTemporaryMessage(chatID, "🤷 За последнюю неделю нет ежедневных саммари для создания еженедельного отчета.", 30*time.Second)
		return
	}

	log.Printf("[WeeklySummary][HANDLER] Chat %d: Найдено %d ежедневных саммари для еженедельного отчета", chatID, len(dailySummaries))

	// Детальная информация о найденных саммари
	for i, summary := range dailySummaries {
		date := time.Unix(int64(summary.Date), 0).Format("2006-01-02 15:04:05")
		length := len(summary.Text)
		preview := summary.Text
		if len(preview) > 150 {
			preview = preview[:150] + "..."
		}
		log.Printf("[WeeklySummary][HANDLER] Chat %d: Дневное саммари #%d - Дата: %s, MsgID: %d, Длина: %d символов", chatID, i+1, date, summary.MessageID, length)
		log.Printf("[WeeklySummary][HANDLER] Chat %d: Превью саммари #%d: \"%s\"", chatID, i+1, preview)
	}

	// Форматируем ежедневные саммари для передачи в LLM
	formattedSummaries := b.formatDailySummariesForWeekly(dailySummaries)
	log.Printf("[WeeklySummary][HANDLER] Chat %d: Форматирование саммари завершено. Общая длина текста для LLM: %d символов", chatID, len(formattedSummaries))

	// Генерируем еженедельное саммари
	var weeklySummary string
	llmStartTime := time.Now()
	maxRetries := 3
	retryDelay := 5 * time.Second

	log.Printf("[WeeklySummary][HANDLER] Chat %d: Начало генерации еженедельного саммари через LLM (%s)", chatID, b.config.LLMProvider)

	for i := 0; i < maxRetries; i++ {
		// Встраиваем личность в промпт еженедельного саммари
		enrichedPrompt := b.enrichPromptWithPersonality(b.config.WeeklySummaryPrompt, chatID, "weekly_summary")

		// Используем провайдер из конфига для еженедельного саммари
		weeklySummary, err = b.llm.GenerateResponseByType(llm.ResponseTypeWeeklySummary, enrichedPrompt, formattedSummaries, float32(b.config.GeminiTemperatureNormal))
		if err == nil {
			break // Успех
		}

		// Проверяем на ошибку rate limit (429)
		if strings.Contains(err.Error(), "429") || strings.Contains(strings.ToLower(err.Error()), "rate limit") {
			log.Printf("[WeeklySummary WARN] Chat %d: Ошибка Rate Limit (429) при генерации еженедельного саммари (Попытка %d/%d). Ожидание %v...", chatID, i+1, maxRetries, retryDelay)
			time.Sleep(retryDelay)
			retryDelay *= 2 // Экспоненциальная задержка
		} else {
			// Другая ошибка, прекращаем попытки
			log.Printf("[WeeklySummary ERROR] Chat %d: Неисправимая ошибка LLM при генерации еженедельного саммари: %v", chatID, err)
			break
		}
	}

	if err != nil {
		log.Printf("[WeeklySummary ERROR] Chat %d: Не удалось сгенерировать еженедельное саммари после %d попыток: %v", chatID, maxRetries, err)
		b.sendTemporaryMessage(chatID, "❌ Ошибка: Не удалось сгенерировать еженедельное саммари.", 30*time.Second)
		return
	}

	if weeklySummary == "" {
		log.Printf("[WeeklySummary ERROR] Chat %d: LLM вернул пустой ответ для еженедельного саммари", chatID)
		// Отправляем сообщение об ошибке пользователю
		b.sendTemporaryMessage(chatID, "🤔 LLM не смог сгенерировать еженедельное саммари (вернул пустой ответ).", 30*time.Second)
		return
	}

	// Очищаем саммари от возможных метаданных
	log.Printf("🧹 [SUMMARY] Очистка еженедельного саммари для чата %d (исходная длина: %d)", chatID, len(weeklySummary))
	weeklySummary = cleanupLLMResponse(weeklySummary)
	llmDuration := time.Since(llmStartTime)
	log.Printf("[WeeklySummary][HANDLER] Chat %d: LLM сгенерировал еженедельное саммари длиной %d символов за %v", chatID, len(weeklySummary), llmDuration)

	// Санитизируем Markdown разметку
	finalSummary := strings.TrimSpace(weeklySummary)
	finalSummary = sanitizeMarkdown(finalSummary)

	// Добавляем заголовок к еженедельному саммари
	finalSummary = "📅 *Еженедельное саммари*\n\n" + finalSummary
	log.Printf("[WeeklySummary][HANDLER] Chat %d: Добавлен заголовок. Финальная длина: %d символов", chatID, len(finalSummary))

	// Разбиваем на части, если превышает лимит Telegram
	messageParts := b.splitMessageForTelegram(finalSummary, b.config.WeeklySummaryMaxParts)
	log.Printf("[WeeklySummary][HANDLER] Chat %d: Сообщение разбито на %d частей (лимит: %d)", chatID, len(messageParts), b.config.WeeklySummaryMaxParts)

	// Отправляем все части
	for i, part := range messageParts {
		if i > 0 {
			// Добавляем небольшую задержку между частями
			time.Sleep(1 * time.Second)
		}

		// Добавляем индикацию части, если сообщение было разбито
		if len(messageParts) > 1 {
			partPrefix := fmt.Sprintf("📄 *Часть %d/%d*\n\n", i+1, len(messageParts))
			part = partPrefix + part
		}

		log.Printf("[WeeklySummary][HANDLER] Chat %d: Отправка части %d/%d еженедельного саммари (длина: %d символов)", chatID, i+1, len(messageParts), len(part))
		b.sendWeeklySummaryReply(chatID, part)
		log.Printf("[WeeklySummary][HANDLER] Chat %d: Часть %d/%d успешно отправлена", chatID, i+1, len(messageParts))
	}

	// Логирование завершения
	totalDuration := time.Since(startTime)
	log.Printf("[WeeklySummary COMPLETE] Chat %d: Еженедельное саммари сгенерировано и отправлено в %d частях. LLM: %v, Total: %v",
		chatID, len(messageParts), llmDuration, totalDuration)
}

// formatDailySummariesForWeekly форматирует ежедневные саммари для передачи в LLM
func (b *Bot) formatDailySummariesForWeekly(dailySummaries []*tgbotapi.Message) string {
	if len(dailySummaries) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Ежедневные саммари за последнюю неделю:\n\n")

	// Получаем локацию из конфига
	loc, err := time.LoadLocation(b.config.TimeZone)
	if err != nil {
		loc = time.UTC
	}

	// Названия дней недели на русском
	dayNames := []string{"Воскресенье", "Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота"}

	for i, msg := range dailySummaries {
		msgTime := time.Unix(int64(msg.Date), 0).In(loc)
		dayName := dayNames[msgTime.Weekday()]
		dateStr := msgTime.Format("02.01.2006")

		sb.WriteString(fmt.Sprintf("=== %s, %s ===\n", dayName, dateStr))
		sb.WriteString(msg.Text)

		if i < len(dailySummaries)-1 {
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

// splitMessageForTelegram разбивает длинное сообщение на части для Telegram
func (b *Bot) splitMessageForTelegram(message string, maxParts int) []string {
	const telegramMaxLength = 4096

	// Если сообщение помещается в один блок, возвращаем как есть
	if len(message) <= telegramMaxLength {
		return []string{message}
	}

	var parts []string
	remaining := message

	for len(parts) < maxParts && len(remaining) > 0 {
		if len(remaining) <= telegramMaxLength {
			parts = append(parts, remaining)
			break
		}

		// Ищем хорошее место для разрыва (по параграфам, предложениям)
		cutIndex := telegramMaxLength

		// Пытаемся найти разрыв по двойному переносу строки (параграф)
		if pos := strings.LastIndex(remaining[:cutIndex], "\n\n"); pos > cutIndex/2 {
			cutIndex = pos
		} else if pos := strings.LastIndex(remaining[:cutIndex], "\n"); pos > cutIndex/2 {
			// Иначе по одиночному переносу
			cutIndex = pos
		} else if pos := strings.LastIndex(remaining[:cutIndex], ". "); pos > cutIndex/2 {
			// Иначе по концу предложения
			cutIndex = pos + 1
		} else if pos := strings.LastIndex(remaining[:cutIndex], " "); pos > cutIndex/2 {
			// Иначе по пробелу
			cutIndex = pos
		}

		// Добавляем часть
		parts = append(parts, remaining[:cutIndex])
		remaining = remaining[cutIndex:]

		// Убираем лишние пробелы и переносы в начале оставшегося текста
		remaining = strings.TrimLeft(remaining, " \n")
	}

	// Если остался текст, но достигли лимита частей, добавляем остаток к последней части
	if len(remaining) > 0 && len(parts) == maxParts {
		if len(parts) > 0 {
			parts[len(parts)-1] += "\n\n" + remaining
		} else {
			parts = append(parts, remaining)
		}
	}

	return parts
}

// CreateWeeklySummaryForChat создает еженедельное саммари для конкретного чата (экспортируемый метод)
func (b *Bot) CreateWeeklySummaryForChat(chatID int64) {
	b.createAndSendWeeklySummary(chatID)
}
