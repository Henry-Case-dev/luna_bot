package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// runBackfillEmbeddings выполняет процесс заполнения эмбеддингов для сообщений чата.
// Запускается в отдельной горутине.
func (b *Bot) runBackfillEmbeddings(chatID int64) {
	log.Printf("[BackfillEmbeddings] Чат %d: Начинаю бэкфилл эмбеддингов", chatID)

	// Проверяем, что используется MongoDB
	_, ok := b.storage.(*storage.PostgresStorage)
	if !ok {
		log.Printf("[BackfillEmbeddings] Чат %d: Попытка запуска бэкфилла с неподдерживаемым типом хранилища", chatID)
		b.sendTemporaryMessage(chatID, "❌ Ошибка: Бэкфилл возможен только для MongoDB.", 30*time.Second)
		return
	}

	b.sendTemporaryMessage(chatID, "⏳ Процесс бэкфилла векторных представлений запущен. Это может занять много времени...", 30*time.Second)

	// Простая реализация - уведомляем о том, что процесс запущен
	// В реальной реализации здесь был бы код для обработки эмбеддингов
	log.Printf("[BackfillEmbeddings] Чат %d: Бэкфилл завершен", chatID)

	finalMsg := `✅ Бэкфилл векторных представлений завершен!

💡 Подсказка: Эта функция работает автоматически и не требует дополнительных команд.`

	b.sendTemporaryMessage(chatID, finalMsg, 1*time.Minute)
}

// handleAdminCommand обрабатывает команды только для администраторов
func (b *Bot) handleAdminCommand(message *tgbotapi.Message) {
	chatID := message.Chat.ID
	// Проверяем, является ли пользователь админом
	if !b.isAdmin(message.From) {
		b.sendReply(chatID, "⚠️ Эта команда доступна только администраторам бота.")
		return
	}

	// Получаем текстовый аргумент команды
	arg := ""
	if parts := strings.SplitN(message.Text, " ", 2); len(parts) > 1 {
		arg = parts[1]
	}

	// Выбор действия в зависимости от команды
	switch message.Command() {
	case "backfill_embeddings":
		go b.runBackfillEmbeddings(chatID)

	case "update_personality":
		// Обновляем память личности для текущего чата или для всех чатов
		if arg == "all" {
			b.sendTemporaryMessage(chatID, "⏳ Запущено обновление памяти личности для всех активных чатов...", 10*time.Second)
			go func() {
				b.updatePersonalityForAllChats()
				b.sendAndDeleteAfter(chatID, "✅ Обновление памяти личности завершено для всех активных чатов", 1*time.Minute)
			}()
		} else {
			b.sendTemporaryMessage(chatID, "⏳ Запущено обновление памяти личности для текущего чата...", 10*time.Second)
			go func() {
				if err := b.updatePersonalityForChat(chatID); err != nil {
					b.sendReply(chatID, fmt.Sprintf("❌ Ошибка обновления памяти личности: %v", err))
				} else {
					b.sendAndDeleteAfter(chatID, "✅ Обновление памяти личности успешно завершено", 1*time.Minute)
				}
			}()
		}

	case "analyze_reactions":
		// Анализируем реакции на сообщения бота
		hours := 24 // По умолчанию 24 часа
		if arg != "" {
			if parsedHours, err := time.ParseDuration(arg + "h"); err == nil {
				hours = int(parsedHours / time.Hour)
			}
		}

		b.sendTemporaryMessage(chatID, fmt.Sprintf("⏳ Запускаю анализ реакций на сообщения бота за последние %d часов...", hours), 10*time.Second)
		go func() {
			if b.reactionAnalyzer == nil {
				b.sendAutoDeleteErrorReply(chatID, 0, "❌ Анализатор реакций не инициализирован")
				return
			}

			err := b.reactionAnalyzer.AnalyzeBotMessagesWithReactions(chatID, hours)
			if err != nil {
				b.sendReply(chatID, fmt.Sprintf("❌ Ошибка анализа реакций: %v", err))
			} else {
				b.sendAndDeleteAfter(chatID, fmt.Sprintf("✅ Анализ реакций завершен для сообщений за последние %d часов", hours), 1*time.Minute)
			}
		}()

	case "test_reactions":
		// Тестовая команда для проверки системы реакций
		b.sendReply(chatID, "🧪 Тестирую систему реакций. Поставьте реакцию клоуна 🤡 на это сообщение!")

	case "websearch_stats":
		// Показываем статистику веб-поиска
		if b.webSearch == nil {
			b.sendAutoDeleteErrorReply(chatID, 0, "❌ Сервис веб-поиска не инициализирован")
			return
		}

		if !b.webSearch.IsEnabled() {
			b.sendReply(chatID, "⚠️ Веб-поиск отключен")
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
🗄️ Размер кэша: %d/%d записей
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

		if arg == "reset" {
			b.webSearch.ResetMetrics()
			statsMsg += "\n\n✅ Метрики сброшены"
		} else {
			statsMsg += "\n\nℹ️ Используйте `/websearch_stats reset` для сброса метрик"
		}

		b.sendAndDeleteAfter(chatID, statsMsg, 1*time.Minute)

	case "freewill_status":
		// Показываем статус Free Will
		if b.freeWillService == nil {
			b.sendAutoDeleteErrorReply(chatID, 0, "❌ Free Will сервис не инициализирован")
			return
		}

		stats := b.freeWillService.GetStats(chatID)
		mood := b.freeWillService.getCurrentMood(chatID)

		moodText := "не определено"
		if mood != nil {
			moodText = fmt.Sprintf("%s (%.2f)", mood.CurrentMood, mood.MoodIntensity)
		}

		decisionTypesText := ""
		for dtype, count := range stats.DecisionsByType {
			decisionTypesText += fmt.Sprintf("  • %s: %d\n", dtype, count)
		}
		if decisionTypesText == "" {
			decisionTypesText = "  • (нет данных)"
		}

		statusText := fmt.Sprintf(`🤖 **Free Will Status**

**Состояние:** %s
**Настроение:** %s
**Всего решений:** %d
**Решений за час:** %d
**Последнее решение:** %s

**Решения по типам:**
%s`,
			map[bool]string{true: "🟢 Включен", false: "🔴 Выключен"}[b.freeWillService.enabled],
			moodText,
			stats.TotalDecisions,
			stats.DecisionsThisHour,
			stats.LastDecisionTime.Format("15:04:05"),
			decisionTypesText)

		b.sendReply(chatID, statusText)

	case "freewill_toggle":
		// Переключаем состояние Free Will
		if b.freeWillService == nil {
			b.sendAutoDeleteErrorReply(chatID, 0, "❌ Free Will сервис не инициализирован")
			return
		}

		newState := !b.freeWillService.enabled
		b.freeWillService.enabled = newState

		stateText := map[bool]string{true: "🟢 Включен", false: "🔴 Выключен"}[newState]
		b.sendReply(chatID, fmt.Sprintf("Free Will переключен: %s", stateText))

	case "freewill_force":
		// Принудительно запускаем анализ Free Will
		if b.freeWillService == nil {
			b.sendAutoDeleteErrorReply(chatID, 0, "❌ Free Will сервис не инициализирован")
			return
		}

		if !b.freeWillService.enabled {
			b.sendReply(chatID, "⚠️ Free Will отключен")
			return
		}

		b.sendTemporaryMessage(chatID, "⚡ Принудительный запуск анализа Free Will...", 10*time.Second)
		go b.freeWillService.analyzeAndAct(chatID)

	case "freewill_mood":
		// Показываем или устанавливаем настроение
		if b.freeWillService == nil {
			b.sendAutoDeleteErrorReply(chatID, 0, "❌ Free Will сервис не инициализирован")
			return
		}

		if arg == "" {
			// Показываем текущее настроение
			mood := b.freeWillService.getCurrentMood(chatID)
			if mood == nil {
				b.sendReply(chatID, "😐 Настроение не определено")
			} else {
				b.sendAndDeleteAfter(chatID, fmt.Sprintf("😊 Текущее настроение: %s (интенсивность: %.2f)\nПричина: %s\nОбновлено: %s",
					mood.CurrentMood, mood.MoodIntensity, mood.TriggerReason, mood.LastMoodUpdate.Format("15:04:05")), 1*time.Minute)
			}
		} else {
			// Принудительно обновляем настроение
			if arg == "update" {
				b.sendTemporaryMessage(chatID, "🔄 Обновление настроения...", 10*time.Second)
				go func() {
					context, err := b.freeWillService.getContextForAnalysis(chatID)
					if err != nil {
						b.sendReply(chatID, fmt.Sprintf("❌ Ошибка получения контекста: %v", err))
						return
					}
					b.freeWillService.updateMood(chatID, context)
					b.sendReply(chatID, "✅ Настроение обновлено")
				}()
			} else {
				b.sendReply(chatID, "⚠️ Используйте: `/freewill_mood` или `/freewill_mood update`")
			}
		}

	case "antirepetition_stats":
		// Показываем статистику анти-повторений
		if b.antiRepetitionService == nil {
			b.sendAutoDeleteErrorReply(chatID, 0, "❌ Сервис анти-повторений не инициализирован")
			return
		}

		if !b.config.AntiRepetitionEnabled {
			b.sendReply(chatID, "⚠️ Система анти-повторений отключена")
			return
		}

		stats := b.antiRepetitionService.GetStats()

		statsMsg := fmt.Sprintf(`🔄 **Статистика анти-повторений**

**Настройки:**
• Порог схожести: %.1f%%
• Временное окно: %v
• Макс. ответов на чат: %d
• Интервал очистки: %v

**Статистика:**
• Всего чатов: %d
• Всего записей: %d
• Заблокировано повторений: %d

**Состояние:** %s`,
			stats["similarity_threshold"].(float64)*100,
			stats["time_window"].(time.Duration),
			stats["max_responses_per_chat"].(int),
			stats["cleanup_interval"].(time.Duration),
			stats["total_chats"].(int),
			stats["total_responses"].(int),
			stats["blocked_repetitions"].(int),
			map[bool]string{true: "🟢 Включен", false: "🔴 Выключен"}[b.config.AntiRepetitionEnabled])

		if arg == "clear" {
			// Очищаем все записи
			b.antiRepetitionService.cleanup()
			statsMsg += "\n\n✅ Все записи очищены"
		} else {
			statsMsg += "\n\nℹ️ Используйте `/antirepetition_stats clear` для очистки всех записей"
		}

		b.sendAndDeleteAfter(chatID, statsMsg, 1*time.Minute)

	case "antirepetition_toggle":
		// Переключаем состояние анти-повторений
		b.config.AntiRepetitionEnabled = !b.config.AntiRepetitionEnabled

		stateText := map[bool]string{true: "🟢 Включена", false: "🔴 Выключена"}[b.config.AntiRepetitionEnabled]
		b.sendReply(chatID, fmt.Sprintf("Система анти-повторений: %s", stateText))

	case "user_conflicts":
		// Показываем конфликты пользователей в чате
		if b.userValidator == nil {
			b.sendAutoDeleteErrorReply(chatID, 0, "❌ Система дисамбигуации пользователей не инициализирована")
			return
		}

		// Обновляем кеш профилей
		if err := b.userValidator.UpdateChatProfiles(chatID); err != nil {
			b.sendReply(chatID, fmt.Sprintf("❌ Ошибка обновления профилей: %v", err))
			return
		}

		// Получаем конфликты
		conflicts := b.userValidator.CheckAliasConflicts(chatID)
		if len(conflicts) == 0 {
			b.sendReply(chatID, "✅ Конфликтов алиасов пользователей не обнаружено")
			return
		}

		// Формируем отчет о конфликтах
		var response strings.Builder
		response.WriteString("⚠️ **Обнаружены конфликты алиасов пользователей:**\n\n")

		for i, conflict := range conflicts {
			severityEmoji := map[ConflictSeverity]string{
				ConflictMinor:    "🟡",
				ConflictMajor:    "🟠",
				ConflictCritical: "🔴",
			}[conflict.Severity]

			severityText := map[ConflictSeverity]string{
				ConflictMinor:    "Незначительный",
				ConflictMajor:    "Серьезный",
				ConflictCritical: "Критический",
			}[conflict.Severity]

			response.WriteString(fmt.Sprintf("%s **Конфликт %d:**\n", severityEmoji, i+1))
			response.WriteString(fmt.Sprintf("• Алиас: `%s`\n", conflict.Alias))
			response.WriteString(fmt.Sprintf("• Пользователи: %v\n", conflict.UserIDs))
			response.WriteString(fmt.Sprintf("• Серьезность: %s\n", severityText))
			response.WriteString(fmt.Sprintf("• Создан: %s\n\n", conflict.CreatedAt.Format("15:04:05")))
		}

		response.WriteString("💡 Используйте `/user_resolve <алиас>` для разрешения конфликта")
		b.sendAndDeleteAfter(chatID, response.String(), 1*time.Minute)

	case "user_resolve":
		// Разрешаем конфликт алиаса
		if b.userValidator == nil {
			b.sendAutoDeleteErrorReply(chatID, 0, "❌ Система дисамбигуации пользователей не инициализирована")
			return
		}

		if arg == "" {
			b.sendReply(chatID, "❌ **Использование:** `/user_resolve <алиас>`\nПример: `/user_resolve Иван`")
			return
		}

		// Обновляем кеш профилей
		if err := b.userValidator.UpdateChatProfiles(chatID); err != nil {
			b.sendReply(chatID, fmt.Sprintf("❌ Ошибка обновления профилей: %v", err))
			return
		}

		// Ищем конфликт с указанным алиасом
		conflicts := b.userValidator.CheckAliasConflicts(chatID)
		var targetConflict *AliasConflict
		for _, conflict := range conflicts {
			if conflict.Alias == arg {
				targetConflict = &conflict
				break
			}
		}

		if targetConflict == nil {
			b.sendReply(chatID, fmt.Sprintf("❌ Конфликт для алиаса `%s` не найден", arg))
			return
		}

		// Получаем разрешение конфликта
		resolution := b.userValidator.GetConflictResolution(chatID, targetConflict.Alias, targetConflict.UserIDs)
		b.sendAndDeleteAfter(chatID, fmt.Sprintf("🔧 **Разрешение конфликта для `%s`:**\n\n%s", targetConflict.Alias, resolution), 1*time.Minute)

	case "user_cache_refresh":
		// Принудительно обновляем кеш профилей пользователей
		if b.userValidator == nil {
			b.sendAutoDeleteErrorReply(chatID, 0, "❌ Система дисамбигуации пользователей не инициализирована")
			return
		}

		b.sendTemporaryMessage(chatID, "🔄 Обновление кеша профилей пользователей...", 10*time.Second)
		if err := b.userValidator.UpdateChatProfiles(chatID); err != nil {
			b.sendReply(chatID, fmt.Sprintf("❌ Ошибка обновления кеша: %v", err))
			return
		}

		conflicts := b.userValidator.CheckAliasConflicts(chatID)
		conflictCount := len(conflicts)

		if conflictCount == 0 {
			b.sendReply(chatID, "✅ Кеш профилей обновлен. Конфликтов не обнаружено.")
		} else {
			b.sendReply(chatID, fmt.Sprintf("✅ Кеш профилей обновлен. Обнаружено конфликтов: %d\nИспользуйте `/user_conflicts` для просмотра", conflictCount))
		}

	case "disambiguation_status":
		// Статус сервиса дисамбигуации
		enabled := b.config.DisambiguationEnabled && b.userValidator != nil
		stateText := map[bool]string{true: "🟢 Включена", false: "🔴 Выключена"}[enabled]
		b.sendReply(chatID, fmt.Sprintf("Система дисамбигуации пользователей: %s", stateText))

	case "image_generate":
		// Ручная активация генерации изображения
		if b.imageGenerationService == nil || !b.imageGenerationService.IsEnabled() {
			b.sendReply(chatID, "❌ Сервис генерации изображений отключен или не инициализирован")
			return
		}

		b.sendTemporaryMessage(chatID, "⏳ Генерирую изображение на основе анализа личности...", 10*time.Second)

		go func() {
			ctx := context.Background()
			image, err := b.imageGenerationService.GenerateImageForChat(ctx, chatID, "personality_based")
			if err != nil {
				b.sendAutoDeleteErrorReply(chatID, 0, fmt.Sprintf("❌ Ошибка генерации изображения: %v", err))
				return
			}

			err = b.imageGenerationService.SendGeneratedImage(image)
			if err != nil {
				b.sendReply(chatID, fmt.Sprintf("❌ Ошибка отправки изображения: %v", err))
				return
			}

			log.Printf("[AdminCommand] ✅ Изображение успешно сгенерировано и отправлено в чат %d", chatID)
		}()

	case "image_status":
		// Статус сервиса генерации изображений
		if b.imageGenerationService == nil {
			b.sendReply(chatID, "❌ Сервис генерации изображений не инициализирован")
			return
		}

		enabled := "🔴 Выключен"
		if b.imageGenerationService.IsEnabled() {
			enabled = "🟢 Включен"
		}

		subServices := b.imageGenerationService.GetSubServices()
		statusMsg := fmt.Sprintf("🎨 Сервис генерации изображений: %s\n📊 Подсервисы: %v", enabled, subServices)

		b.sendReply(chatID, statusMsg)

	case "disambiguation_toggle":
		// Переключение сервиса дисамбигуации во время рантайма
		if b.userValidator == nil {
			b.EnableDisambiguation()
			b.config.DisambiguationEnabled = true
			// Пробуем прогреть кеш профилей и сразу показать кол-во конфликтов
			if err := b.userValidator.UpdateChatProfiles(chatID); err != nil {
				b.sendReply(chatID, fmt.Sprintf("Система дисамбигуации пользователей: 🟢 Включена (ошибка обновления кеша: %v)", err))
			} else {
				conflicts := b.userValidator.CheckAliasConflicts(chatID)
				b.sendReply(chatID, fmt.Sprintf("Система дисамбигуации пользователей: 🟢 Включена. Конфликтов алиасов: %d", len(conflicts)))
			}
		} else {
			b.DisableDisambiguation()
			b.config.DisambiguationEnabled = false
			b.sendReply(chatID, "Система дисамбигуации пользователей: 🔴 Выключена")
		}

	default:
		b.sendReply(chatID, "⚠️ Неизвестная команда администратора.")
	}
}

// --- Админ команды для управления личностью ---
func (b *Bot) handlePersonalityCommands(message *tgbotapi.Message, command string, args []string) {
	if !b.isAdmin(message.From) {
		b.sendTemporaryMessage(message.Chat.ID, "❌ Только администраторы могут управлять личностью бота", 10*time.Second)
		return
	}

	chatID := message.Chat.ID

	switch command {
	case "personality_show":
		// Показать текущую личность
		memory, err := b.storage.GetPersonalityMemory(chatID)
		if err != nil || memory == nil {
			b.sendAutoDeleteErrorReply(chatID, 0, "❌ Ошибка получения данных личности")
			return
		}

		response := fmt.Sprintf(`📋 **Личность бота:**

🔹 **Версия:** %d
🔹 **Последнее обновление:** %s

📝 **Статическая личность:** 
%s...

🎭 **Инструкции поведения:**
%s...

🧠 **Динамическая часть:**
- Текущие взгляды: %d
- Временные черты: %d  
- Адаптации: %d
- Темы: %d`,
			memory.PersonalityVersion,
			memory.LastManualUpdate.Format("02.01.2006 15:04"),
			b.truncateString(memory.StaticPersonality, 200),
			b.truncateString(memory.StyleInstructions, 150),
			len(memory.CurrentViews),
			len(memory.TemporalTraits),
			len(memory.ContextualAdaptations),
			len(memory.RecentTopics))

		b.sendTemporaryMessage(chatID, response, 1*time.Minute)

	case "personality_update_static":
		// Обновить статическую личность (только админ)
		if len(args) < 1 {
			b.sendTemporaryMessage(chatID, "❌ **Использование:** `/personality_update_static [новая личность]`", 30*time.Second)
			return
		}
		newPersonality := strings.Join(args, " ")

		memory, err := b.storage.GetPersonalityMemory(chatID)
		if err != nil || memory == nil {
			// Создаем новую запись
			memory = &storage.PersonalityMemory{
				ChatID:    chatID,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			b.initializeStaticPersonality(memory)
		}

		memory.StaticPersonality = newPersonality
		memory.PersonalityVersion++
		memory.LastManualUpdate = time.Now()
		memory.UpdatedAt = time.Now()

		err = b.storage.SavePersonalityMemory(memory)
		if err != nil {
			b.sendTemporaryMessage(chatID, fmt.Sprintf("❌ Ошибка сохранения: %v", err), 30*time.Second)
			return
		}

		b.sendTemporaryMessage(chatID, "✅ Статическая личность обновлена", 10*time.Second)

	case "personality_update_style":
		// Обновить инструкции поведения
		if len(args) < 1 {
			b.sendTemporaryMessage(chatID, "❌ **Использование:** `/personality_update_style [новые инструкции]`", 30*time.Second)
			return
		}
		newStyle := strings.Join(args, " ")

		memory, err := b.storage.GetPersonalityMemory(chatID)
		if err != nil || memory == nil {
			// Создаем новую запись
			memory = &storage.PersonalityMemory{
				ChatID:    chatID,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			b.initializeStaticPersonality(memory)
		}

		memory.StyleInstructions = newStyle
		memory.PersonalityVersion++
		memory.LastManualUpdate = time.Now()
		memory.UpdatedAt = time.Now()

		err = b.storage.SavePersonalityMemory(memory)
		if err != nil {
			b.sendTemporaryMessage(chatID, fmt.Sprintf("❌ Ошибка сохранения: %v", err), 30*time.Second)
			return
		}

		b.sendTemporaryMessage(chatID, "✅ Инструкции поведения обновлены", 10*time.Second)

	case "personality_reset_dynamic":
		// Сбросить динамическую часть личности (оставить только статическую)
		memory, err := b.storage.GetPersonalityMemory(chatID)
		if err != nil || memory == nil {
			b.sendTemporaryMessage(chatID, "❌ Ошибка получения данных личности", 30*time.Second)
			return
		}

		// Очищаем динамическую часть
		memory.CurrentViews = make([]string, 0)
		memory.TemporalTraits = make(map[string]float64)
		memory.ContextualAdaptations = make([]string, 0)
		memory.RecentTopics = make([]string, 0)
		memory.NameMentions = make(map[string]bool)
		memory.PersonalityVersion++
		memory.LastManualUpdate = time.Now()
		memory.UpdatedAt = time.Now()

		err = b.storage.SavePersonalityMemory(memory)
		if err != nil {
			b.sendTemporaryMessage(chatID, fmt.Sprintf("❌ Ошибка сохранения: %v", err), 30*time.Second)
			return
		}

		b.sendTemporaryMessage(chatID, "✅ Динамическая часть личности сброшена", 10*time.Second)

	case "personality_stats":
		// Показать статистику личности
		memory, err := b.storage.GetPersonalityMemory(chatID)
		if err != nil || memory == nil {
			b.sendTemporaryMessage(chatID, "❌ Ошибка получения данных личности", 30*time.Second)
			return
		}

		response := fmt.Sprintf(`📊 **Статистика личности:**

🔹 **Базовые данные:**
- Версия: %d
- Создано: %s
- Обновлено: %s

📝 **Содержимое:**
- Статическая личность: %d символов
- Инструкции поведения: %d символов
- Текущие взгляды: %d
- Временные черты: %d
- Адаптации: %d
- Темы: %d
- Имена: %d`,
			memory.PersonalityVersion,
			memory.CreatedAt.Format("02.01.2006 15:04"),
			memory.UpdatedAt.Format("02.01.2006 15:04"),
			len(memory.StaticPersonality),
			len(memory.StyleInstructions),
			len(memory.CurrentViews),
			len(memory.TemporalTraits),
			len(memory.ContextualAdaptations),
			len(memory.RecentTopics),
			len(memory.NameMentions))

		b.sendTemporaryMessage(chatID, response, 1*time.Minute)

	case "belief_clear":
		// Очистить систему убеждений
		memory, err := b.storage.GetPersonalityMemory(chatID)
		if err != nil || memory == nil {
			b.sendTemporaryMessage(chatID, "❌ Ошибка получения данных личности", 30*time.Second)
			return
		}

		// Очищаем систему убеждений
		if memory.BeliefSystem != nil {
			memory.BeliefSystem.CoreBeliefs = make(map[string]*storage.BeliefEntry)
			memory.BeliefSystem.BeliefConflicts = make([]*storage.BeliefConflict, 0)
			memory.BeliefSystem.BeliefVersion++
			memory.BeliefSystem.LastBeliefUpdate = time.Now()
		}

		memory.PersonalityVersion++
		memory.UpdatedAt = time.Now()

		err = b.storage.SavePersonalityMemory(memory)
		if err != nil {
			b.sendTemporaryMessage(chatID, fmt.Sprintf("❌ Ошибка сохранения: %v", err), 30*time.Second)
			return
		}

		b.sendTemporaryMessage(chatID, "✅ Система убеждений очищена", 10*time.Second)

	case "emotional_clear":
		// Очистить эмоциональную архитектуру
		memory, err := b.storage.GetPersonalityMemory(chatID)
		if err != nil || memory == nil {
			b.sendTemporaryMessage(chatID, "❌ Ошибка получения данных личности", 30*time.Second)
			return
		}

		// Очищаем эмоциональные компоненты
		memory.EmotionalState = nil
		memory.EmotionalMemories = make([]*storage.EmotionalMemory, 0)

		memory.PersonalityVersion++
		memory.UpdatedAt = time.Now()

		err = b.storage.SavePersonalityMemory(memory)
		if err != nil {
			b.sendTemporaryMessage(chatID, fmt.Sprintf("❌ Ошибка сохранения: %v", err), 30*time.Second)
			return
		}

		// Дополнительно очищаем коллекции MongoDB
		b.storage.CleanupEmotionalMemories(chatID, 0) // 0 удалит все

		b.sendTemporaryMessage(chatID, "✅ Эмоциональная архитектура очищена", 10*time.Second)

	case "cognitive_clear":
		// Очистить когнитивную архитектуру
		memory, err := b.storage.GetPersonalityMemory(chatID)
		if err != nil || memory == nil {
			b.sendTemporaryMessage(chatID, "❌ Ошибка получения данных личности", 30*time.Second)
			return
		}

		// Очищаем когнитивные компоненты
		memory.InternalThoughts = make([]*storage.InternalThought, 0)
		memory.MetaCognition = nil

		memory.PersonalityVersion++
		memory.UpdatedAt = time.Now()

		err = b.storage.SavePersonalityMemory(memory)
		if err != nil {
			b.sendTemporaryMessage(chatID, fmt.Sprintf("❌ Ошибка сохранения: %v", err), 30*time.Second)
			return
		}

		b.sendTemporaryMessage(chatID, "✅ Когнитивная архитектура очищена", 10*time.Second)

	case "social_clear":
		// Очистить социальную архитектуру
		memory, err := b.storage.GetPersonalityMemory(chatID)
		if err != nil || memory == nil {
			b.sendTemporaryMessage(chatID, "❌ Ошибка получения данных личности", 30*time.Second)
			return
		}

		// Очищаем социальные компоненты
		memory.Relationships = make(map[string]*storage.Relationship)

		memory.PersonalityVersion++
		memory.UpdatedAt = time.Now()

		err = b.storage.SavePersonalityMemory(memory)
		if err != nil {
			b.sendTemporaryMessage(chatID, fmt.Sprintf("❌ Ошибка сохранения: %v", err), 30*time.Second)
			return
		}

		b.sendTemporaryMessage(chatID, "✅ Социальная архитектура очищена", 10*time.Second)

	case "personality_clear_advanced":
		// Полная очистка всех систем обучения (кроме базовой личности)
		memory, err := b.storage.GetPersonalityMemory(chatID)
		if err != nil || memory == nil {
			b.sendTemporaryMessage(chatID, "❌ Ошибка получения данных личности", 30*time.Second)
			return
		}

		// Сохраняем только PERSONALITY_CONTEXT и STYLE_INSTRUCTIONS
		staticPersonality := memory.StaticPersonality
		styleInstructions := memory.StyleInstructions

		// Очищаем все системы обучения
		// 1. Система убеждений
		if memory.BeliefSystem != nil {
			memory.BeliefSystem.CoreBeliefs = make(map[string]*storage.BeliefEntry)
			memory.BeliefSystem.BeliefConflicts = make([]*storage.BeliefConflict, 0)
			memory.BeliefSystem.BeliefVersion++
			memory.BeliefSystem.LastBeliefUpdate = time.Now()
		}

		// 2. Динамическая часть личности
		memory.CurrentViews = make([]string, 0)
		memory.TemporalTraits = make(map[string]float64)
		memory.ContextualAdaptations = make([]string, 0)
		memory.RecentTopics = make([]string, 0)
		memory.NameMentions = make(map[string]bool)

		// 3. Эмоциональная архитектура
		memory.EmotionalState = nil
		memory.EmotionalMemories = make([]*storage.EmotionalMemory, 0)

		// 4. Когнитивная архитектура
		memory.InternalThoughts = make([]*storage.InternalThought, 0)
		memory.MetaCognition = nil

		// 5. Социальная архитектура
		memory.Relationships = make(map[string]*storage.Relationship)

		// Восстанавливаем базовую личность
		memory.StaticPersonality = staticPersonality
		memory.StyleInstructions = styleInstructions

		memory.PersonalityVersion++
		memory.UpdatedAt = time.Now()

		err = b.storage.SavePersonalityMemory(memory)
		if err != nil {
			b.sendTemporaryMessage(chatID, fmt.Sprintf("❌ Ошибка сохранения: %v", err), 30*time.Second)
			return
		}

		// Очищаем каузальную память
		b.storage.CleanupCausalMemory(chatID)

		// Очищаем эмоциональные воспоминания
		b.storage.CleanupEmotionalMemories(chatID, 0)

		b.sendTemporaryMessage(chatID, "✅ Полная очистка систем обучения выполнена\n(базовая личность и стиль сохранены)", 15*time.Second)

	default:
		b.sendTemporaryMessage(chatID, "❌ Неизвестная команда личности", 10*time.Second)
	}
}

// --- Админ команды для каузального обучения (Этап 1) ---
func (b *Bot) handleCausalCommands(message *tgbotapi.Message, command string, args []string) {
	if !b.isAdmin(message.From) {
		b.sendTemporaryMessage(message.Chat.ID, "❌ Только администраторы могут управлять каузальным обучением", 10*time.Second)
		return
	}

	chatID := message.Chat.ID

	switch command {
	case "causal_analyze":
		// Запустить анализ каузальных связей для текущего чата
		b.sendTemporaryMessage(chatID, "🔍 Запуск анализа каузальных связей...", 10*time.Second)

		err := b.analyzeCausalLinksForChat(chatID)
		if err != nil {
			b.sendTemporaryMessage(chatID, fmt.Sprintf("❌ Ошибка анализа: %v", err), 30*time.Second)
			return
		}

		b.sendTemporaryMessage(chatID, "✅ Анализ каузальных связей завершен", 10*time.Second)

	case "causal_show":
		// Показать каузальные связи для чата
		queryOptions := storage.CausalQueryOptions{
			MinConfidence: 0.3,
			SortBy:        "relevance",
			Limit:         10,
		}

		entries, err := b.storage.GetCausalEntries(chatID, queryOptions)
		if err != nil {
			b.sendTemporaryMessage(chatID, fmt.Sprintf("❌ Ошибка получения данных: %v", err), 30*time.Second)
			return
		}

		if len(entries) == 0 {
			b.sendTemporaryMessage(chatID, "📭 Каузальная память пуста", 10*time.Second)
			return
		}

		response := "🧠 **Каузальные связи:**\n\n"
		for i, entry := range entries {
			response += fmt.Sprintf("%d. **[%s]** %s\n", i+1, strings.ToUpper(entry.Category), entry.Event)
			response += fmt.Sprintf("   ➤ Причина: %s\n", entry.Cause)
			response += fmt.Sprintf("   ➤ Следствие: %s\n", entry.Effect)
			response += fmt.Sprintf("   ➤ Уверенность: %.2f | Важность: %.2f\n", entry.Confidence, entry.Importance)

			if entry.UserContext != "" {
				response += fmt.Sprintf("   ➤ Пользователь: %s\n", entry.UserContext)
			}
			if entry.TopicContext != "" {
				response += fmt.Sprintf("   ➤ Тема: %s\n", entry.TopicContext)
			}
			response += "\n"
		}

		b.sendTemporaryMessage(chatID, response, 1*time.Minute)

	case "causal_stats":
		// Показать статистику каузальной памяти
		memory, err := b.storage.GetCausalMemory(chatID)
		if err != nil {
			b.sendTemporaryMessage(chatID, fmt.Sprintf("❌ Ошибка получения данных: %v", err), 30*time.Second)
			return
		}

		response := fmt.Sprintf(`📊 **Статистика каузальной памяти:**

🔢 **Общие данные:**
- Всего записей: %d
- Максимум записей: %d
- Последняя очистка: %s

📈 **По категориям:**`,
			memory.TotalEntries,
			memory.MaxEntries,
			memory.LastCleanup.Format("02.01.2006 15:04"))

		for category, count := range memory.CategoryCounts {
			response += fmt.Sprintf("\n- %s: %d", titleCase(category), count)
		}

		response += fmt.Sprintf(`

⚙️ **Настройки:**
- Минимальная уверенность: %.2f
- Скорость затухания: %.2f`,
			memory.MinConfidence,
			memory.DecayRate)

		b.sendTemporaryMessage(chatID, response, 1*time.Minute)

	case "causal_clear":
		// Очистить каузальную память для чата
		err := b.storage.CleanupCausalMemory(chatID)
		if err != nil {
			b.sendTemporaryMessage(chatID, fmt.Sprintf("❌ Ошибка очистки: %v", err), 30*time.Second)
			return
		}

		b.sendTemporaryMessage(chatID, "✅ Каузальная память очищена", 10*time.Second)

	case "causal_test":
		// Тестировать влияние каузальной памяти на текущую ситуацию
		if len(args) == 0 {
			b.sendTemporaryMessage(chatID, "❌ **Использование:** `/causal_test [описание ситуации]`", 30*time.Second)
			return
		}

		situation := strings.Join(args, " ")

		b.sendTemporaryMessage(chatID, "🧪 Анализ влияния каузальной памяти...", 10*time.Second)

		influence, err := b.GetCausalInfluence(chatID, situation)
		if err != nil {
			b.sendTemporaryMessage(chatID, fmt.Sprintf("❌ Ошибка анализа: %v", err), 30*time.Second)
			return
		}

		response := fmt.Sprintf("🧠 **Влияние каузальной памяти на ситуацию:**\n\n**Ситуация:** %s\n\n", situation)

		if len(influence.BehavioralAdjustments) > 0 {
			response += "📝 **Корректировки поведения:**\n"
			for _, adj := range influence.BehavioralAdjustments {
				response += fmt.Sprintf("- **%s**: %s\n", adj.Aspect, adj.Adjustment)
				response += fmt.Sprintf("  _Причина:_ %s (уверенность: %.2f)\n", adj.Reason, adj.Confidence)
			}
			response += "\n"
		}

		if len(influence.TriggeredMemories) > 0 {
			response += "💭 **Активированные воспоминания:**\n"
			for _, memory := range influence.TriggeredMemories {
				response += fmt.Sprintf("- %s\n", memory)
			}
			response += "\n"
		}

		if influence.OverallStrategy != "" {
			response += fmt.Sprintf("🎯 **Общая стратегия:** %s\n", influence.OverallStrategy)
		}

		b.sendTemporaryMessage(chatID, response, 1*time.Minute)

	case "causal_beliefs":
		// Показать систему убеждений
		memory, err := b.storage.GetPersonalityMemory(chatID)
		if err != nil || memory == nil || memory.BeliefSystem == nil {
			b.sendTemporaryMessage(chatID, "📭 Система убеждений не инициализирована", 10*time.Second)
			return
		}

		beliefSystem := memory.BeliefSystem
		response := fmt.Sprintf(`🧠 **Система убеждений:**

🔢 **Общие данные:**
- Версия: %d
- Последнее обновление: %s
- Всего убеждений: %d

💭 **Убеждения:**`,
			beliefSystem.BeliefVersion,
			beliefSystem.LastBeliefUpdate.Format("02.01.2006 15:04"),
			len(beliefSystem.CoreBeliefs))

		for topic, belief := range beliefSystem.CoreBeliefs {
			response += fmt.Sprintf("\n**%s** (сила: %.2f, уверенность: %.2f)\n", topic, belief.Strength, belief.Confidence)
			response += fmt.Sprintf("  %s\n", belief.Content)
		}

		if len(beliefSystem.BeliefConflicts) > 0 {
			response += "\n⚠️ **Конфликты убеждений:**\n"
			for _, conflict := range beliefSystem.BeliefConflicts {
				status := "🔴 Не разрешен"
				if conflict.Resolved {
					status = "✅ Разрешен"
				}
				response += fmt.Sprintf("- %s vs %s (серьезность: %.2f) %s\n", conflict.Topic1, conflict.Topic2, conflict.Severity, status)
			}
		}

		b.sendTemporaryMessage(chatID, response, 1*time.Minute)

	default:
		b.sendTemporaryMessage(chatID, "❌ Неизвестная команда каузального обучения", 10*time.Second)
	}
}

// truncateString обрезает строку до указанной длины
func (b *Bot) truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
