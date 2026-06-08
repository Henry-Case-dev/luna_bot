package bot

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const summaryRequestInterval = 10 * time.Minute // Ограничение на вызов /summary

// handleCommand обрабатывает команды
func (b *Bot) handleCommand(message *tgbotapi.Message) {
	command := message.Command()
	chatID := message.Chat.ID
	userID := message.From.ID
	username := message.From.UserName

	// Get current settings for the chat
	b.settingsMutex.RLock()
	settings, exists := b.chatSettings[chatID]
	if !exists {
		// If settings don't exist, they should be created in handleUpdate before this
		log.Printf("[ERROR][CmdHandler] Chat %d: Settings not found for command /%s", chatID, command)
		b.settingsMutex.RUnlock()
		return
	}
	lastMenuMsgID := settings.LastMenuMessageID
	lastSettingsMsgID := settings.LastSettingsMessageID
	b.settingsMutex.RUnlock()

	// Delete the command message itself to keep the chat clean
	b.deleteMessageSilent(chatID, message.MessageID)

	// Check if the user is an admin for admin-only commands
	isUserAdmin := b.isAdmin(message.From)

	switch command {
	case "start":
		// Usually handled by ensureChatInitializedAndWelcome
		// Send main menu anyway
		b.sendMainMenu(chatID, lastMenuMsgID)
	case "status":
		// Отправка startup message вручную
		log.Printf("[CMD] Пользователь %s (%d) запросил статус бота в чате %d", username, userID, chatID)
		b.sendStartupMessage(chatID)
	case "menu":
		b.sendMainMenu(chatID, lastMenuMsgID)
	case "settings":
		b.sendSettingsKeyboard(chatID, lastSettingsMsgID)
	case "summary":
		// Check rate limit
		now := time.Now()
		b.summaryMutex.Lock() // Используем мьютекс для lastSummaryRequest
		lastReq, ok := b.lastSummaryRequest[chatID]
		durationSinceLast := now.Sub(lastReq)
		if ok && !lastReq.IsZero() && durationSinceLast < summaryRequestInterval {
			remainingTime := summaryRequestInterval - durationSinceLast
			log.Printf("[CMD] Chat %d: /summary отклонен из-за rate limit. Прошло: %v < %v. Осталось: %v", chatID, durationSinceLast, summaryRequestInterval, remainingTime)
			b.summaryMutex.Unlock()

			// Generate dynamic rate limit message
			prompt := b.enrichPromptWithPersonality(b.config.RateLimitPrompt, chatID, "rate_limit")
			generatedText, err := b.llm.GenerateResponseByType(llm.ResponseTypeDailyTake, prompt, "", float32(b.config.GeminiTemperatureNormal))
			if err != nil {
				log.Printf("[ERROR][CMD] Chat %d: Ошибка генерации текста о лимите: %v", chatID, err)
				b.sendTemporaryMessage(chatID, "Я не могу отвечать слишком часто. Пожалуйста, подождите немного.", 30*time.Second)
				return
			}

			generatedText = cleanupLLMResponse(generatedText)
			fullMessage := fmt.Sprintf("%s %s\nПодожди еще: %s",
				b.config.RateLimitStaticText,
				generatedText,
				remainingTime.Round(time.Second))

			b.sendTemporaryMessage(chatID, fullMessage, 30*time.Second)
			return
		}
		b.lastSummaryRequest[chatID] = now
		b.summaryMutex.Unlock()

		// Clear last info message
		b.settingsMutex.Lock()
		if settings, exists := b.chatSettings[chatID]; exists {
			if settings.LastInfoMessageID != 0 {
				b.deleteMessageSilent(chatID, settings.LastInfoMessageID)
			}
		}
		b.settingsMutex.Unlock()

		// Send generation message
		msg := tgbotapi.NewMessage(chatID, "Генерирую саммари, подождите...")
		sentMsg, err := b.api.Send(msg)
		if err == nil {
			log.Printf("[CMD] Chat %d: Уведомление о генерации саммари отправлено (MsgID: %d)", chatID, sentMsg.MessageID)
			b.settingsMutex.Lock()
			if set, ok := b.chatSettings[chatID]; ok {
				set.LastInfoMessageID = sentMsg.MessageID
			}
			b.settingsMutex.Unlock()
		} else {
			log.Printf("[CMD ERROR] Chat %d: Ошибка отправки сообщения 'Генерирую саммари...': %v", chatID, err)
		}

		// Start generation in goroutine
		go func() {
			log.Printf("[CMD] Chat %d: Запуск генерации саммари в отдельной горутине", chatID)
			b.createAndSendSummary(chatID)
			log.Printf("[CMD] Chat %d: Завершение выполнения генерации саммари", chatID)
		}()

	// --- Admin Command: /profile_set ---
	case "profile_set":
		if !isUserAdmin {
			b.sendTemporaryMessage(chatID, "🚫 У вас нет прав для выполнения этой команды.", 10*time.Second)
			return
		}

		// Инструкция по формату ввода
		instructionText := "📝 Введите данные профиля в следующем сообщении в формате:\\n`@никнейм - Прозвище - Пол (male/female/other) - Настоящее имя (если известно) - Био`\\n\\n_Пол, Наст\\.имя и Био могут быть пустыми или отсутствовать\\. Пол можно указать как m/f\\._\\n\\n_Это сообщение будет удалено через 15 секунд\\._"
		instructionMsg := tgbotapi.NewMessage(chatID, instructionText)
		instructionMsg.ParseMode = tgbotapi.ModeMarkdown // Используем стандартный Markdown

		sentInstruction, err := b.api.Send(instructionMsg)
		if err != nil {
			log.Printf("[ERROR][CmdHandler /profile_set] Ошибка отправки инструкции в чат %d: %v", chatID, err)
			return
		}

		// Устанавливаем состояние ожидания ввода профиля
		b.settingsMutex.Lock()
		if settings, exists := b.chatSettings[chatID]; exists {
			settings.PendingSetting = "profile_data"               // Используем это поле для ожидания данных профиля
			settings.LastInfoMessageID = sentInstruction.MessageID // Сохраняем ID инструкции для последующего удаления
		}
		b.settingsMutex.Unlock()

		// Запускаем удаление инструкции через 15 секунд
		go func() {
			time.Sleep(15 * time.Second)
			b.deleteMessageSilent(chatID, sentInstruction.MessageID)
			// Опционально: сбросить PendingSetting, если пользователь ничего не ввел за 15 сек?
			// Пока не будем, дадим время ввести.
		}()

		log.Printf("[ADMIN CMD] Пользователь %s (%d) инициировал команду /profile_set в чате %d. Ожидание ввода данных.", username, userID, chatID)
		// Выходим, основная логика будет в handleMessage при получении следующего сообщения

	// --- Admin Commands: Embeddings ---
	case "backfill_embeddings":
		if !isUserAdmin {
			b.sendTemporaryMessage(chatID, "🚫 У вас нет прав для выполнения этой команды.", 10*time.Second)
			return
		}

		// Check if MongoDB is used
		if b.config.StorageType != "mongo" {
			b.sendTemporaryMessage(chatID, "🚫 Команда доступна только при использовании MongoDB.", 10*time.Second)
			return
		}

		if !b.config.LongTermMemoryEnabled {
			b.sendTemporaryMessage(chatID, "🚫 Долгосрочная память (векторный поиск) выключена в настройках.", 10*time.Second)
			return
		}

		// Run backfill in goroutine
		log.Printf("[ADMIN CMD] Пользователь %s (%d) запустил backfill_embeddings в чате %d.", username, userID, chatID)
		go b.sendAndDeleteAfter(chatID, "⏳ Запускаю процесс заполнения векторных представлений для сообщений в этом чате. Это может занять много времени...", 15*time.Second)
		go b.runBackfillEmbeddings(chatID)

	// --- Admin Command: /autobio_reset ---
	case "autobio_reset":
		if !isUserAdmin {
			b.sendTemporaryMessage(chatID, "⚠️ Эта команда доступна только администраторам бота.", 10*time.Second)
			return
		}

		// Reset AutoBio timestamps for all users in chat
		err := b.storage.ResetAutoBioTimestamps(chatID)
		if err != nil {
			log.Printf("[ADMIN CMD ERROR] Ошибка сброса временных меток AutoBio для чата %d: %v", chatID, err)
			b.sendTemporaryMessage(chatID, fmt.Sprintf("❌ Ошибка сброса временных меток: %v", err), 30*time.Second)
			return
		}

		b.sendTemporaryMessage(chatID, "✅ Временные метки AutoBio сброшены. При следующем запуске будет выполнен полный анализ.", 30*time.Second)

	// --- Admin Command: /autobio_force ---
	case "autobio_force":
		if !isUserAdmin {
			b.sendTemporaryMessage(chatID, "⚠️ Эта команда доступна только администраторам бота.", 10*time.Second)
			return
		}

		if !b.config.AutoBioEnabled {
			b.sendTemporaryMessage(chatID, "🚫 Функция AutoBio отключена в конфигурации.", 10*time.Second)
			return
		}

		// Force AutoBio analysis for all users in chat
		log.Printf("[ADMIN CMD] Пользователь %s (%d) запустил принудительный анализ AutoBio в чате %d.", username, userID, chatID)
		go b.sendAndDeleteAfter(chatID, "⏳ Запускаю ПРИНУДИТЕЛЬНЫЙ полный анализ AutoBio для всех пользователей этого чата. Это может занять некоторое время...", 15*time.Second)
		go b.runAutoBioAnalysisForChatForced(chatID)

	// --- Admin Command: /stop_purge ---
	case "stop_purge":
		if !isUserAdmin {
			b.sendTemporaryMessage(chatID, "🚫 У вас нет прав для выполнения этой команды.", 10*time.Second)
			return
		}

		args := strings.Fields(message.CommandArguments())
		if len(args) != 1 {
			go b.sendAndDeleteAfter(chatID, "⚠️ Укажите @username пользователя, для которого нужно остановить очистку сообщений. Пример: `/stop_purge @имя_пользователя`", 15*time.Second)
			return
		}

		targetUsername := strings.TrimPrefix(args[0], "@")
		if targetUsername == "" {
			go b.sendAndDeleteAfter(chatID, "⚠️ Укажите @username пользователя, для которого нужно остановить очистку сообщений. Пример: `/stop_purge @имя_пользователя`", 15*time.Second)
			return
		}

		// Try to find user by username
		targetUserID, err := b.getUserIDByUsername(chatID, targetUsername)
		if err != nil {
			go b.sendAndDeleteAfter(chatID, fmt.Sprintf("⚠️ Не удалось найти пользователя %s в этом чате.", targetUsername), 15*time.Second)
			return
		}

		// Stop purge for user
		stopped := b.moderation.StopPurge(chatID, targetUserID)
		if stopped {
			go b.sendAndDeleteAfter(chatID, fmt.Sprintf("✅ Активная задача очистки сообщений для %s остановлена.", targetUsername), 10*time.Second)
		} else {
			go b.sendAndDeleteAfter(chatID, fmt.Sprintf("ℹ️ Для пользователя %s не найдено активных задач очистки сообщений.", targetUsername), 10*time.Second)
		}

	// --- Admin Command: /update_personality ---
	case "update_personality":
		if !isUserAdmin {
			b.sendTemporaryMessage(chatID, "🚫 У вас нет прав для выполнения этой команды.", 10*time.Second)
			return
		}

		arg := strings.TrimSpace(message.CommandArguments())

		if arg == "all" {
			go b.sendAndDeleteAfter(chatID, "⏳ Запускаю обновление памяти личности для всех активных чатов...", 15*time.Second)
			go func() {
				b.updatePersonalityForAllChats()
				b.sendTemporaryMessage(chatID, "✅ Обновление памяти личности завершено для всех активных чатов", 30*time.Second)
			}()
		} else {
			go b.sendAndDeleteAfter(chatID, "⏳ Запускаю обновление памяти личности для текущего чата...", 15*time.Second)
			go func() {
				if err := b.updatePersonalityForChat(chatID); err != nil {
					b.sendTemporaryMessage(chatID, fmt.Sprintf("❌ Ошибка обновления памяти личности: %v", err), 30*time.Second)
				} else {
					b.sendTemporaryMessage(chatID, "✅ Обновление памяти личности успешно завершено", 30*time.Second)
				}
			}()
		}

	// --- Admin Command: /message_stats ---
	case "message_stats":
		if !isUserAdmin {
			b.sendTemporaryMessage(chatID, "🚫 У вас нет прав для выполнения этой команды.", 10*time.Second)
			return
		}

		// Show basic message info
		response := "📊 Статистика сообщений:\n\n📝 Функция временно недоступна"
		b.sendTemporaryMessage(chatID, response, 30*time.Second)

	// --- Admin Command: /test_reaction ---
	case "test_reaction":
		if !isUserAdmin {
			b.sendTemporaryMessage(chatID, "🚫 У вас нет прав для выполнения этой команды.", 10*time.Second)
			return
		}

		args := strings.Fields(message.CommandArguments())
		if len(args) != 2 {
			go b.sendAndDeleteAfter(chatID, "⚠️ Укажите ID сообщения и эмодзи. Пример: `/test_reaction 123 👍`", 15*time.Second)
			return
		}

		messageIDStr := args[0]
		emoji := args[1]

		if messageIDStr == "" || emoji == "" {
			go b.sendAndDeleteAfter(chatID, "⚠️ Укажите ID сообщения и эмодзи. Пример: `/test_reaction 123 👍`", 15*time.Second)
			return
		}

		messageID, err := strconv.Atoi(messageIDStr)
		if err != nil {
			go b.sendAndDeleteAfter(chatID, "⚠️ Неверный ID сообщения. Должно быть число.", 15*time.Second)
			return
		}

		// Try to set reaction
		if b.reactionTracker != nil {
			err = b.reactionTracker.SetBotReaction(chatID, messageID, emoji)
			if err != nil {
				log.Printf("[ADMIN CMD ERROR] Ошибка установки реакции %s на сообщение %d в чате %d: %v", emoji, messageID, chatID, err)
				go b.sendAndDeleteAfter(chatID, fmt.Sprintf("❌ Ошибка установки реакции: %v", err), 15*time.Second)
			} else {
				go b.sendAndDeleteAfter(chatID, "✅ Реакция установлена", 10*time.Second)
			}
		} else {
			go b.sendAndDeleteAfter(chatID, "❌ Система реакций не инициализирована", 15*time.Second)
		}

	// --- Команды управления личностью ---
	case "personality_show", "personality_update_static", "personality_update_style", "personality_reset_dynamic", "personality_stats", "belief_clear", "emotional_clear", "cognitive_clear", "social_clear", "personality_clear_advanced":
		args := strings.Fields(message.CommandArguments())
		b.handlePersonalityCommands(message, command, args)

	// --- Команды каузального обучения (Этап 1) ---
	case "causal_analyze", "causal_show", "causal_stats", "causal_clear", "causal_test", "causal_beliefs":
		args := strings.Fields(message.CommandArguments())
		b.handleCausalCommands(message, command, args)

	// --- Admin Command: /clown_stats ---
	case "clown_stats":
		if !isUserAdmin {
			b.sendTemporaryMessage(chatID, "🚫 У вас нет прав для выполнения этой команды.", 10*time.Second)
			return
		}

		if b.reactionHandler != nil && b.reactionHandler.clownLimiter != nil {
			stats := b.reactionHandler.clownLimiter.GetStats()
			reactionStats := ""
			if b.reactionStats != nil {
				b.reactionStats.LogCurrentStats() // Выводим в логи
				reactionStats = "\n\n📊 Общая статистика реакций:\n(смотрите логи для подробностей)"
			}

			message := fmt.Sprintf(`🤡 Статистика лимитера реакций клоуна:

🔧 Настройки:
• Вероятность ответа: %d%%
• Cooldown: %d секунд
• Лимит в час: %d ответов

📈 Текущее состояние:
• Пользователей на cooldown: %v
• Ответов за последний час: %v
• Всего пользователей: %v%s`,
				b.config.ClownResponseProbability,
				b.config.ClownCooldownSeconds,
				b.config.MaxClownResponsesPerHour,
				stats["users_on_cooldown"],
				stats["responses_last_hour"],
				stats["total_recorded_users"],
				reactionStats)

			b.sendTemporaryMessage(chatID, message, 1*time.Minute)
		} else {
			b.sendTemporaryMessage(chatID, "❌ Система реакций не инициализирована", 10*time.Second)
		}
		log.Printf("[ADMIN CMD] Пользователь %s (%d) запросил статистику клоуна в чате %d.", username, userID, chatID)

	// --- Admin Command: /voice ---
	case "voice":
		if !isUserAdmin {
			b.sendTemporaryMessage(chatID, "⚠️ Эта команда доступна только администраторам бота.", 10*time.Second)
			return
		}
		// Запускаем генерацию голосового сообщения
		if b.voiceMessageService != nil {
			err := b.voiceMessageService.ForceVoiceMessage(chatID)
			if err != nil {
				b.sendTemporaryMessage(chatID, fmt.Sprintf("❌ Ошибка отправки голосового сообщения: %v", err), 30*time.Second)
			}
		} else {
			b.sendTemporaryMessage(chatID, "❌ Сервис голосовых сообщений недоступен", 10*time.Second)
		}
		log.Printf("[ADMIN CMD] Пользователь %s (%d) инициировал команду /voice в чате %d.", username, userID, chatID)

	// --- Admin Command: /websearch_stats ---
	case "websearch_stats":
		if !isUserAdmin {
			b.sendTemporaryMessage(chatID, "⚠️ Эта команда доступна только администраторам бота.", 10*time.Second)
			return
		}

		args := message.CommandArguments()

		// Показываем статистику веб-поиска
		if b.webSearch == nil {
			b.sendAutoDeleteErrorReply(chatID, 0, "❌ Сервис веб-поиска не инициализирован")
			return
		}

		if !b.webSearch.IsEnabled() {
			b.sendTemporaryMessage(chatID, "⚠️ Веб-поиск отключен", 10*time.Second)
			return
		}

		// Получаем метрики
		metrics := b.webSearch.GetMetrics()
		cacheHitRate := b.webSearch.GetCacheHitRate()
		cacheSize, maxCacheSize, ttl := b.webSearch.GetCacheStats()

		// Формируем статистику
		statsMsg := fmt.Sprintf(`📊 Статистика веб-поиска:

🔢 Всего поисков: %d
📈 Кэш: попадания=%d, промахи=%d
💯 Процент попаданий в кэш: %.1f%%
🗂️ Размер кэша: %d/%d записей
⏰ TTL кэша: %v

🎯 Триггеры:
   • Ключевые слова: %d
   • LLM рекомендации: %d

❌ Ошибки API: %d
📊 Среднее результатов: %.1f

⏱️ Время работы: %v`,
			metrics.TotalSearches,
			metrics.CacheHits, metrics.CacheMisses,
			cacheHitRate,
			cacheSize, maxCacheSize, ttl,
			metrics.KeywordTriggers,
			metrics.LLMTriggers,
			metrics.APIErrors,
			metrics.AverageResultsNum,
			time.Since(metrics.LastResetTime).Round(time.Second))

		if args == "reset" {
			b.webSearch.ResetMetrics()
			statsMsg += "\n\n✅ Метрики сброшены"
		} else {
			statsMsg += "\n\nℹ️ Используйте `/websearch_stats reset` для сброса метрик"
		}

		b.sendTemporaryMessage(chatID, statsMsg, 1*time.Minute)
		log.Printf("[ADMIN CMD] Пользователь %s (%d) запросил статистику веб-поиска в чате %d.", username, userID, chatID)

	// --- Command: /weeklysummary ---
	case "weeklysummary":
		log.Printf("[WeeklySummary][CMD] Chat %d: Пользователь %s (%d) вызвал команду /weeklysummary", chatID, username, userID)

		// Check if feature is enabled
		if !b.config.WeeklySummaryEnabled {
			log.Printf("[WeeklySummary][CMD] Chat %d: Функция еженедельного саммари отключена в конфиге", chatID)
			b.sendTemporaryMessage(chatID, "🚫 Функция еженедельного саммари отключена.", 10*time.Second)
			return
		}

		// Check rate limit (weekly summary can be requested less frequently than daily)
		now := time.Now()
		b.summaryMutex.Lock()
		lastReq, ok := b.lastWeeklySummaryRequest[chatID]
		durationSinceLast := now.Sub(lastReq)
		weeklySummaryInterval := 30 * time.Minute // Rate limit for weekly summary
		if ok && !lastReq.IsZero() && durationSinceLast < weeklySummaryInterval {
			remainingTime := weeklySummaryInterval - durationSinceLast
			log.Printf("[WeeklySummary][CMD] Chat %d: /weeklysummary отклонен из-за rate limit. Прошло: %v < %v. Осталось: %v", chatID, durationSinceLast, weeklySummaryInterval, remainingTime)
			b.summaryMutex.Unlock()

			// Generate dynamic rate limit message
			prompt := b.enrichPromptWithPersonality(b.config.RateLimitPrompt, chatID, "rate_limit")
			generatedText, err := b.llm.GenerateResponseByType(llm.ResponseTypeDailyTake, prompt, "", float32(b.config.GeminiTemperatureNormal))
			if err != nil {
				log.Printf("[ERROR][CMD] Chat %d: Ошибка генерации текста о лимите: %v", chatID, err)
				b.sendTemporaryMessage(chatID, "Я не могу создавать еженедельное саммари слишком часто. Пожалуйста, подождите немного.", 30*time.Second)
				return
			}

			generatedText = cleanupLLMResponse(generatedText)
			fullMessage := fmt.Sprintf("%s %s\nПодожди еще: %s",
				b.config.RateLimitStaticText,
				generatedText,
				remainingTime.Round(time.Second))

			b.sendTemporaryMessage(chatID, fullMessage, 30*time.Second)
			return
		}
		b.lastWeeklySummaryRequest[chatID] = now
		b.summaryMutex.Unlock()

		// Clear last info message
		b.settingsMutex.Lock()
		lastInfoMsgID := b.chatSettings[chatID].LastInfoMessageID
		b.settingsMutex.Unlock()

		if lastInfoMsgID != 0 {
			b.deleteMessageSilent(chatID, lastInfoMsgID)
		}

		// Send generation message
		log.Printf("[WeeklySummary][CMD] Chat %d: Отправка уведомления о генерации еженедельного саммари", chatID)
		msg := tgbotapi.NewMessage(chatID, "Генерирую еженедельное саммари, подождите...")
		sentMsg, err := b.api.Send(msg)
		if err == nil {
			log.Printf("[WeeklySummary][CMD] Chat %d: Уведомление отправлено (MsgID: %d)", chatID, sentMsg.MessageID)
			b.settingsMutex.Lock()
			if set, ok := b.chatSettings[chatID]; ok {
				set.LastInfoMessageID = sentMsg.MessageID
			}
			b.settingsMutex.Unlock()
		} else {
			log.Printf("[WeeklySummary][CMD ERROR] Chat %d: Ошибка отправки сообщения 'Генерирую еженедельное саммари...': %v", chatID, err)
		}

		// Start generation in goroutine
		log.Printf("[WeeklySummary][CMD] Chat %d: Запуск генерации еженедельного саммари в отдельной горутине", chatID)
		go func() {
			log.Printf("[WeeklySummary][CMD] Chat %d: Начало выполнения генерации еженедельного саммари", chatID)
			b.createAndSendWeeklySummary(chatID)
			log.Printf("[WeeklySummary][CMD] Chat %d: Завершение выполнения генерации еженедельного саммари", chatID)
		}()

	// --- Admin Commands: User Disambiguation ---
	case "user_conflicts", "user_resolve", "user_cache_refresh":
		if !isUserAdmin {
			b.sendTemporaryMessage(chatID, "⚠️ Эта команда доступна только администраторам бота.", 10*time.Second)
			return
		}
		log.Printf("[ADMIN CMD] Пользователь %s (%d) вызвал команду /%s в чате %d.", username, userID, command, chatID)
		b.handleAdminCommand(message)

	// --- Admin Commands: Image Generation ---
	case "image_generate", "image_status":
		if !isUserAdmin {
			b.sendTemporaryMessage(chatID, "⚠️ Эта команда доступна только администраторам бота.", 10*time.Second)
			return
		}
		log.Printf("[ADMIN CMD] Пользователь %s (%d) вызвал команду /%s в чате %d.", username, userID, command, chatID)
		b.handleAdminCommand(message)

	default:
		log.Printf("[WARN] Неизвестная команда: %s", command)
	}
}
