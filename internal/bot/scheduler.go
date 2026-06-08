package bot

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// scheduleDailyTake запускает планировщик для ежедневного тейка
func (b *Bot) scheduleDailyTake(dailyTakeTime int, timeZone string) {
	log.Println("[Scheduler DEBUG] Запущен цикл планировщика DailyTake.")
	// Получаем локацию из конфига
	loc, err := time.LoadLocation(timeZone)
	if err != nil {
		log.Printf("Ошибка загрузки часового пояса, используем UTC: %v", err)
		loc = time.UTC
	}

	for {
		// Проверяем, не остановлен ли бот
		select {
		case <-b.stop:
			log.Println("Остановка планировщика DailyTake...")
			return
		default:
			// Продолжаем работу
		}

		now := time.Now().In(loc)
		targetTime := time.Date(
			now.Year(), now.Month(), now.Day(),
			dailyTakeTime, 0, 0, 0,
			loc,
		)

		// Если сейчас уже после времени запуска, планируем на завтра
		if now.After(targetTime) {
			targetTime = targetTime.Add(24 * time.Hour)
		}

		// Вычисляем время до следующего запуска
		sleepDuration := targetTime.Sub(now)
		log.Printf("Следующий тейк запланирован через %v (в %s по %s)",
			sleepDuration.Round(time.Second), targetTime.Format("15:04"), timeZone)

		// Используем time.After для ожидания, чтобы можно было прервать через b.stop
		timer := time.NewTimer(sleepDuration)
		select {
		case <-timer.C:
			// Время пришло, отправляем тейк
			b.sendDailyTakeToAllChats()
		case <-b.stop:
			// Остановка во время ожидания
			timer.Stop() // Останавливаем таймер
			log.Println("Планировщик DailyTake остановлен во время ожидания.")
			return
		}
	}
}

// sendDailyTakeToAllChats отправляет ежедневный тейк во все активные чаты
func (b *Bot) sendDailyTakeToAllChats() {
	if b.config.Debug {
		log.Printf("[DEBUG] Запуск ежедневного тейка для всех активных чатов")
	}

	// Используем только промпт для ежедневного тейка без комбинирования
	dailyTakePrompt := b.enrichPromptWithPersonality(b.config.DailyTakePrompt, 0, "daily_take") // chatID = 0 для общих сообщений

	// Генерируем тейк с промптом
	take, err := b.llm.GenerateResponseByType(llm.ResponseTypeDailyTake, dailyTakePrompt, "", float32(b.config.GeminiTemperatureNormal))
	if err != nil {
		log.Printf("[ERROR][DailyTake] Ошибка генерации темы дня: %v", err)
		return
	}

	// Очищаем ответ от возможных метаданных перед отправкой
	take = cleanupLLMResponse(take)

	message := "🔥 *Тема дня:*\n\n" + take

	// Отправляем во все активные чаты
	b.settingsMutex.RLock()
	activeChatIDs := make([]int64, 0, len(b.chatSettings))
	for chatID, settings := range b.chatSettings {
		if settings.Active {
			activeChatIDs = append(activeChatIDs, chatID)
		}
	}
	b.settingsMutex.RUnlock()

	activeCount := len(activeChatIDs)
	if b.config.Debug {
		log.Printf("[DEBUG] Найдено %d активных чатов для отправки тейка.", activeCount)
	}

	var wg sync.WaitGroup
	for _, chatID := range activeChatIDs {
		wg.Add(1)
		go func(cid int64) {
			defer wg.Done()
			b.sendReply(cid, message)
		}(chatID)
	}
	wg.Wait() // Ждем завершения отправки во все чаты

	if b.config.Debug {
		log.Printf("[DEBUG] Тема дня отправлена в %d активных чатов.", activeCount)
	}
}

// scheduleAutoSummary запускает планировщик для автоматического саммари
func (b *Bot) scheduleAutoSummary() {
	log.Println("[Scheduler DEBUG] Запущен цикл планировщика AutoSummary.")
	// Используем общий тикер, чтобы проверять все чаты раз в час (например)
	// Точность до секунды тут не нужна
	checkInterval := time.Hour
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	log.Printf("Планировщик AutoSummary запущен с интервалом проверки: %v", checkInterval)

	for {
		select {
		case <-ticker.C:
			b.runAutoSummaryForAllChats()
		case <-b.stop:
			log.Println("Остановка планировщика AutoSummary...")
			return
		}
	}
}

// scheduleDonate запускает планировщик для отправки сообщений о донате
func (b *Bot) scheduleDonate() {
	// Читаем флаг включения из ResponseTypeConfigs (DONATE_PROMPT_ENABLED)
	if cfg, ok := b.config.ResponseTypeConfigs[string(llm.ResponseTypeDonate)]; ok {
		if !cfg.Enabled {
			log.Println("[Scheduler] Планировщик донатов не запущен (DONATE_PROMPT_ENABLED=false).")
			return
		}
	}
	if b.config.DonateTimeHours <= 0 {
		log.Println("[Scheduler] Планировщик донатов не запущен (DonateTimeHours <= 0).")
		return
	}

	log.Println("[Scheduler DEBUG] Запущен цикл планировщика Donate.")

	// Период отправки сообщений о донате (из конфига)
	donateInterval := time.Duration(b.config.DonateTimeHours) * time.Hour

	// Добавляем начальную задержку перед первой отправкой
	initialTimer := time.NewTimer(donateInterval)
	select {
	case <-initialTimer.C:
		b.sendDonateMessageToAllChats()
	case <-b.stop:
		initialTimer.Stop()
		log.Println("Планировщик Donate остановлен во время начальной задержки.")
		return
	}

	// Основной цикл планировщика
	for {
		// Проверяем, не остановлен ли бот
		select {
		case <-b.stop:
			log.Println("Остановка планировщика Donate...")
			return
		default:
			// Продолжаем работу
		}

		// Ожидаем до следующей отправки
		timer := time.NewTimer(donateInterval)
		select {
		case <-timer.C:
			// Время пришло, отправляем сообщение о донате
			b.sendDonateMessageToAllChats()
		case <-b.stop:
			// Остановка во время ожидания
			timer.Stop() // Останавливаем таймер
			log.Println("Планировщик Donate остановлен во время ожидания.")
			return
		}
	}
}

// sendDonateMessageToAllChats отправляет сообщение о донате с фотографией во все активные чаты
func (b *Bot) sendDonateMessageToAllChats() {
	if b.config.Debug {
		log.Printf("[DEBUG] Отправка сообщений о донате во все активные чаты")
	}

	// Повторная проверка флага включения, на случай ручного вызова
	if cfg, ok := b.config.ResponseTypeConfigs[string(llm.ResponseTypeDonate)]; ok {
		if !cfg.Enabled {
			if b.config.Debug {
				log.Printf("[DEBUG] DONATE_PROMPT_ENABLED=false — отправка сообщения о донате пропущена")
			}
			return
		}
	}

	// Проверяем, есть ли промпт для доната
	if b.config.DonatePrompt == "" {
		log.Printf("[WARNING] DonatePrompt не задан в конфигурации, сообщения о донате не будут отправлены")
		return
	}

	log.Printf("[DonateReminder] Генерируем сообщение о донате...")

	// Генерируем сообщение о донате с личностью
	donatePrompt := b.enrichPromptWithPersonality(b.config.DonatePrompt, 0, "donate") // chatID = 0 для общих сообщений
	donateMessage, err := b.llm.GenerateResponseByType(llm.ResponseTypeDonate, donatePrompt, "", float32(b.config.GeminiTemperatureNormal))
	if err != nil {
		log.Printf("[ERROR][DonateReminder] Ошибка генерации сообщения о донате: %v", err)
		return
	}

	// Очищаем ответ от возможных метаданных перед отправкой
	donateMessage = cleanupLLMResponse(donateMessage)

	// Форматируем сообщение с добавлением статичной фразы
	message := donateMessage + "\n\n[Поддержать проект](https://donate.stream/luna_bot)"

	// Выбираем случайное изображение из папки donate_images
	imageFile, err := b.getRandomDonateImage()
	if err != nil {
		log.Printf("[ERROR] Ошибка при выборе изображения для доната: %v", err)
		// Если не удалось получить изображение, отправляем только текст
		b.sendDonateTextToAllChats(message)
		return
	}

	// Отправляем во все активные чаты
	b.settingsMutex.RLock()
	activeChatIDs := make([]int64, 0, len(b.chatSettings))
	for chatID, settings := range b.chatSettings {
		if settings.Active {
			activeChatIDs = append(activeChatIDs, chatID)
		}
	}
	b.settingsMutex.RUnlock()

	activeCount := len(activeChatIDs)
	if b.config.Debug {
		log.Printf("[DEBUG] Найдено %d активных чатов для отправки сообщения о донате.", activeCount)
	}

	var wg sync.WaitGroup
	for _, chatID := range activeChatIDs {
		wg.Add(1)
		go func(cid int64) {
			defer wg.Done()
			err := b.sendPhotoWithCaption(cid, imageFile, message)
			if err != nil {
				if b.isUserBlockedError(err) {
					log.Printf("[INFO] Пользователь %d заблокировал бота (403), пропускаю отправку доната", cid)
					b.markChatAsInactive(cid)
					return
				}
				log.Printf("[ERROR] Ошибка отправки фото с сообщением о донате в чат %d: %v", cid, err)
				// При ошибке пробуем отправить только текст
				b.sendReplyMarkdown(cid, "💰 "+message)
			}
		}(chatID)
	}
	wg.Wait() // Ждем завершения отправки во все чаты

	if b.config.Debug {
		log.Printf("[DEBUG] Сообщение о донате отправлено в %d активных чатов.", activeCount)
	}
}

// getRandomDonateImage возвращает путь к случайному изображению из папки donate_images
func (b *Bot) getRandomDonateImage() (string, error) {
	// Открываем директорию с изображениями для донатов
	donateDir := "donate_images"
	files, err := os.ReadDir(donateDir)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения директории %s: %w", donateDir, err)
	}

	// Фильтруем только PNG и JPG файлы
	var imageFiles []string
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		if strings.HasSuffix(strings.ToLower(name), ".png") ||
			strings.HasSuffix(strings.ToLower(name), ".jpg") ||
			strings.HasSuffix(strings.ToLower(name), ".jpeg") {
			imageFiles = append(imageFiles, filepath.Join(donateDir, name))
		}
	}

	if len(imageFiles) == 0 {
		return "", fmt.Errorf("в директории %s нет подходящих изображений (PNG или JPG)", donateDir)
	}

	// Выбираем случайное изображение
	randomIndex := b.randSource.Intn(len(imageFiles))
	return imageFiles[randomIndex], nil
}

// sendPhotoWithCaption отправляет фото с указанной подписью в чат
func (b *Bot) sendPhotoWithCaption(chatID int64, imagePath string, caption string) error {
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FilePath(imagePath))
	photo.Caption = caption
	photo.ParseMode = "Markdown"

	_, err := b.api.Send(photo)
	// Проверяем ошибку 403 и в этой функции для более точной обработки
	if err != nil && b.isUserBlockedError(err) {
		log.Printf("[INFO] sendPhotoWithCaption: Пользователь %d заблокировал бота (403)", chatID)
		b.markChatAsInactive(chatID)
	}
	return err
}

// sendDonateTextToAllChats отправляет только текстовое сообщение о донате во все активные чаты
func (b *Bot) sendDonateTextToAllChats(message string) {
	b.settingsMutex.RLock()
	activeChatIDs := make([]int64, 0, len(b.chatSettings))
	for chatID, settings := range b.chatSettings {
		if settings.Active {
			activeChatIDs = append(activeChatIDs, chatID)
		}
	}
	b.settingsMutex.RUnlock()

	var wg sync.WaitGroup
	for _, chatID := range activeChatIDs {
		wg.Add(1)
		go func(cid int64) {
			defer wg.Done()
			b.sendReplyMarkdown(cid, "💰 "+message)
		}(chatID)
	}
	wg.Wait()
}

// runAutoSummaryForAllChats проверяет и запускает авто-саммари для всех чатов, если пришло время
func (b *Bot) runAutoSummaryForAllChats() {
	now := time.Now()
	if b.config.Debug {
		log.Printf("[DEBUG][AutoSummary] Проверка необходимости авто-саммари для всех чатов (%s)...", now.Format(time.Kitchen))
	}

	b.settingsMutex.Lock() // Блокируем на время итерации и возможного изменения LastAutoSummaryTime
	defer b.settingsMutex.Unlock()

	chatsToCheck := make([]int64, 0, len(b.chatSettings))
	for chatID := range b.chatSettings {
		chatsToCheck = append(chatsToCheck, chatID)
	}

	triggeredCount := 0
	for _, chatID := range chatsToCheck {
		settings, exists := b.chatSettings[chatID] // Получаем настройки внутри блокировки
		if !exists || !settings.Active || settings.SummaryIntervalHours <= 0 {
			continue // Пропускаем неактивные чаты или чаты с выключенным авто-саммари
		}

		interval := time.Duration(settings.SummaryIntervalHours) * time.Hour
		// Если время последнего саммари не установлено или прошло достаточно времени
		if settings.LastAutoSummaryTime.IsZero() || now.Sub(settings.LastAutoSummaryTime) >= interval {
			if b.config.Debug {
				log.Printf("[DEBUG][AutoSummary] Запускаю авто-саммари для чата %d (Интервал: %dh, Последний: %v)",
					chatID, settings.SummaryIntervalHours, settings.LastAutoSummaryTime)
			}
			settings.LastAutoSummaryTime = now // Обновляем время последнего запуска СРАЗУ
			triggeredCount++
			// Запускаем генерацию в горутине, чтобы не блокировать проверку других чатов
			log.Printf("[DEBUG][AutoSummary] -> Запуск горутины createAndSendSummary для чата %d", chatID)
			go b.createAndSendSummary(chatID)
		}
	}

	if triggeredCount > 0 && b.config.Debug {
		log.Printf("[DEBUG][AutoSummary] Запущено авто-саммари для %d чатов.", triggeredCount)
	}
}

// --- НОВЫЙ Планировщик для Auto Bio Analysis ---

// runAutoBioAnalysisForChat запускает анализ профилей для всех пользователей в указанном чате.
func (b *Bot) runAutoBioAnalysisForChat(chatID int64) {
	b.runAutoBioAnalysisForChatInternal(chatID, false)
}

// runAutoBioAnalysisForChatForced принудительно запускает полный анализ профилей для всех пользователей в указанном чате (для команды /trigger_autobio).
func (b *Bot) runAutoBioAnalysisForChatForced(chatID int64) {
	b.runAutoBioAnalysisForChatInternal(chatID, true)
}

// runAutoBioAnalysisForChatInternal внутренняя функция для анализа профилей с возможностью принудительного полного анализа
func (b *Bot) runAutoBioAnalysisForChatInternal(chatID int64, forceFullAnalysis bool) {
	if !b.config.AutoBioEnabled {
		return // Не запускаем, если выключено
	}

	// Получаем список пользователей в этом чате
	profiles, err := b.storage.GetAllUserProfiles(chatID)
	if err != nil {
		log.Printf("[AutoBio ERROR] Чат %d: Не удалось получить профили пользователей: %v", chatID, err)
		return // Выходим, если не удалось получить профили
	}

	if b.config.Debug {
		log.Printf("[AutoBio DEBUG] Чат %d: Найдено %d профилей для анализа (принудительный: %t).", chatID, len(profiles), forceFullAnalysis)
	}

	// Для каждого пользователя запускаем анализ в горутине
	for _, profile := range profiles {
		// Проверяем стоп-сигнал перед запуском горутины
		select {
		case <-b.stop:
			log.Printf("[AutoBio] Остановка во время итерации по пользователям чата %d.", chatID)
			return
		default:
		}

		b.autoBioSemaphore <- struct{}{}  // Захватываем семафор перед запуском горутины
		go func(p *storage.UserProfile) { // Передаем копию указателя в горутину
			defer func() {
				<-b.autoBioSemaphore // Освобождаем семафор после завершения
			}()

			// ИСПРАВЛЕНИЕ: Используем принудительную версию при необходимости
			if forceFullAnalysis {
				b.analyzeAndUpdateProfileForced(p.ChatID, p.UserID)
			} else {
				b.analyzeAndUpdateProfile(p.ChatID, p.UserID)
			}
		}(profile)

		// Небольшая задержка, чтобы не перегружать API/DB
		// Если семафор используется, можно сделать меньше или убрать
		time.Sleep(100 * time.Millisecond)
	}

	if b.config.Debug {
		log.Printf("[AutoBio DEBUG] Чат %d: Завершение запуска анализа для всех профилей чата (принудительный: %t).", chatID, forceFullAnalysis)
	}
}

// runAutoBioAnalysisForAllUsers запускает анализ профилей для всех пользователей во всех активных чатах.
func (b *Bot) runAutoBioAnalysisForAllUsers() {
	if !b.config.AutoBioEnabled {
		return // Не запускаем, если выключено
	}
	log.Printf("[AutoBio Scheduler] Начало цикла анализа профилей для ВСЕХ чатов...")
	if b.config.Debug {
		log.Printf("[AutoBio Scheduler DEBUG] Запуск runAutoBioAnalysisForAllUsers...")
	}

	// Получаем список активных чатов
	b.settingsMutex.RLock()
	activeChatIDs := make([]int64, 0, len(b.chatSettings))
	for chatID, settings := range b.chatSettings {
		if settings.Active {
			activeChatIDs = append(activeChatIDs, chatID)
		}
	}
	b.settingsMutex.RUnlock()

	if b.config.Debug {
		log.Printf("[AutoBio Scheduler DEBUG] Найдено %d активных чатов для анализа профилей.", len(activeChatIDs))
	}

	// Для каждого активного чата
	for _, chatID := range activeChatIDs {
		// Проверяем стоп-сигнал перед обработкой следующего чата
		select {
		case <-b.stop:
			log.Println("[AutoBio Scheduler] Остановка во время итерации по чатам.")
			return
		default:
		}

		if b.config.Debug {
			log.Printf("[AutoBio Scheduler DEBUG] Запуск анализа для чата %d...", chatID)
		}
		// Вызываем новую функцию для анализа конкретного чата
		b.runAutoBioAnalysisForChat(chatID)

		// Небольшая задержка между чатами
		time.Sleep(5 * time.Second) // Увеличим задержку между чатами
	}

	if b.config.Debug {
		log.Printf("[AutoBio Scheduler DEBUG] Завершение цикла runAutoBioAnalysisForAllUsers.")
	}
	log.Printf("[AutoBio Scheduler] Завершение цикла анализа профилей для ВСЕХ чатов.")
}

// scheduleAutoBioAnalysis запускает планировщик для анализа профилей пользователей
func (b *Bot) scheduleAutoBioAnalysis() {
	if !b.config.AutoBioEnabled {
		log.Println("[AutoBio Scheduler] Анализ профилей отключен (AUTO_BIO_ENABLED=false).")
		return
	}

	log.Println("[AutoBio Scheduler] Запуск планировщика AutoBio Analysis.")

	// УБРАНО: Первичный запуск через 2 минуты после старта
	// Это вызывало коллизии с ручными командами /trigger_autobio
	// Теперь анализ запускается только по расписанию (каждые AUTO_BIO_INTERVAL_HOURS)

	// Создаем тикер для периодического запуска
	interval := time.Duration(b.config.AutoBioIntervalHours) * time.Hour
	if interval <= 0 {
		log.Printf("[AutoBio Scheduler WARN] Интервал AUTO_BIO_INTERVAL_HOURS (%d) некорректен, периодический запуск невозможен.", b.config.AutoBioIntervalHours)
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	log.Printf("[AutoBio Scheduler] Периодический анализ профилей настроен с интервалом %v.", interval)

	// Основной цикл планировщика
	for {
		select {
		case <-ticker.C:
			if b.config.Debug {
				log.Printf("[AutoBio Scheduler DEBUG] Сработал тикер, запуск runAutoBioAnalysisForAllUsers...")
			}
			b.runAutoBioAnalysisForAllUsers()
		case <-b.stop:
			log.Println("[AutoBio Scheduler] Остановка планировщика анализа профилей...")
			return
		}
	}
}

// scheduleFreeWillSilenceCheck запускает планировщик для проверки тишины Free Will
func (b *Bot) scheduleFreeWillSilenceCheck() {
	log.Printf("[FreeWill Scheduler] === ЗАПУСК ПЛАНИРОВЩИКА ПРОВЕРКИ ТИШИНЫ ===")
	log.Printf("[FreeWill Scheduler] FreeWillEnabled: %t", b.config.FreeWillEnabled)

	if !b.config.FreeWillEnabled {
		log.Println("[FreeWill Scheduler] ❌ Проверка тишины отключена (FREE_WILL_ENABLED=false).")
		return
	}

	log.Printf("[FreeWill Scheduler] ✅ Free Will включен, продолжаем инициализацию...")
	log.Printf("[FreeWill Scheduler] Адрес freeWillService: %p", b.freeWillService)

	if b.freeWillService == nil {
		log.Printf("[FreeWill Scheduler] ❌ КРИТИЧЕСКАЯ ОШИБКА: freeWillService = nil!")
		return
	}

	// Проверяем тишину каждую минуту для точного контроля
	checkInterval := 1 * time.Minute
	log.Printf("[FreeWill Scheduler] 🕐 Создаем тикер с интервалом: %v", checkInterval)

	ticker := time.NewTicker(checkInterval)
	defer func() {
		log.Printf("[FreeWill Scheduler] 🛑 Останавливаем тикер...")
		ticker.Stop()
		log.Printf("[FreeWill Scheduler] ✅ Тикер остановлен")
	}()

	log.Printf("[FreeWill Scheduler] ✅ Планировщик проверки тишины настроен с интервалом %v.", checkInterval)
	log.Printf("[FreeWill Scheduler] 🔄 Запускаем основной цикл планировщика...")

	tickCount := 0
	startTime := time.Now()

	// Основной цикл планировщика
	for {
		select {
		case tickTime := <-ticker.C:
			tickCount++
			log.Printf("[FreeWill Scheduler] ⏰ TICK #%d в %v (работаем %v)",
				tickCount, tickTime.Format("15:04:05"), time.Since(startTime))

			log.Printf("[FreeWill Scheduler] 🔍 Начинаем проверку тишины в чатах...")

			if b.freeWillService != nil {
				log.Printf("[FreeWill Scheduler] ✅ freeWillService доступен, вызываем CheckSilence()")
				b.freeWillService.CheckSilence()
				log.Printf("[FreeWill Scheduler] ✅ CheckSilence() завершен")
			} else {
				log.Printf("[FreeWill Scheduler] ❌ ОШИБКА: freeWillService стал nil!")
			}

			log.Printf("[FreeWill Scheduler] 🔄 Ждем следующего тика через %v...", checkInterval)

		case <-b.stop:
			log.Printf("[FreeWill Scheduler] 🛑 Получен сигнал остановки после %d тиков", tickCount)
			log.Println("[FreeWill Scheduler] Остановка планировщика проверки тишины...")
			return
		}
	}
}

// scheduleWeeklySummary запускает планировщик для еженедельного саммари
func (b *Bot) scheduleWeeklySummary() {
	// Проверяем, включено ли еженедельное саммари
	if !b.config.WeeklySummaryEnabled {
		log.Println("[WeeklySummary][SCHEDULER] Планировщик еженедельного саммари не запущен (отключено в конфиге).")
		return
	}

	log.Printf("[WeeklySummary][SCHEDULER] === ЗАПУСК ПЛАНИРОВЩИКА ЕЖЕНЕДЕЛЬНОГО САММАРИ ===")
	log.Printf("[WeeklySummary][SCHEDULER] Настройки: День недели: %d, Время: %02d:%02d, Часовой пояс: %s, Максимум частей: %d",
		b.config.WeeklySummaryDay, b.config.WeeklySummaryHour, b.config.WeeklySummaryMinute, b.config.TimeZone, b.config.WeeklySummaryMaxParts)

	// Получаем локацию из конфига
	loc, err := time.LoadLocation(b.config.TimeZone)
	if err != nil {
		log.Printf("[WeeklySummary][SCHEDULER ERROR] Ошибка загрузки часового пояса %s, используем UTC: %v", b.config.TimeZone, err)
		loc = time.UTC
	} else {
		log.Printf("[WeeklySummary][SCHEDULER] Часовой пояс %s успешно загружен", b.config.TimeZone)
	}

	for {
		// Проверяем, не остановлен ли бот
		select {
		case <-b.stop:
			log.Println("Остановка планировщика еженедельного саммари...")
			return
		default:
			// Продолжаем работу
		}

		now := time.Now().In(loc)

		// Вычисляем следующий запланированный момент для еженедельного саммари
		targetTime := b.getNextWeeklySummaryTime(now)

		// Вычисляем время до следующего запуска
		sleepDuration := targetTime.Sub(now)
		log.Printf("[WeeklySummary][SCHEDULER] Текущее время: %s", now.Format("2006-01-02 15:04:05 (Monday)"))
		log.Printf("[WeeklySummary][SCHEDULER] Следующее еженедельное саммари запланировано через %v (в %s по %s)",
			sleepDuration.Round(time.Second), targetTime.Format("Monday 15:04 02.01.2006"), b.config.TimeZone)

		// Используем time.After для ожидания, чтобы можно было прервать через b.stop
		timer := time.NewTimer(sleepDuration)
		select {
		case actualTime := <-timer.C:
			// Время пришло, отправляем еженедельное саммари
			log.Printf("[WeeklySummary][SCHEDULER] ⏰ ВРЕМЯ ПРИШЛО! Запуск еженедельного саммари в %s", actualTime.Format("2006-01-02 15:04:05 (Monday)"))
			b.sendWeeklySummaryToAllChats()
		case <-b.stop:
			// Остановка во время ожидания
			timer.Stop() // Останавливаем таймер
			log.Println("[WeeklySummary][SCHEDULER] Планировщик еженедельного саммари остановлен во время ожидания.")
			return
		}
	}
}

// getNextWeeklySummaryTime вычисляет следующий момент времени для еженедельного саммари
func (b *Bot) getNextWeeklySummaryTime(now time.Time) time.Time {
	// День недели из конфига (0 = воскресенье, 1 = понедельник, ..., 6 = суббота)
	targetWeekday := time.Weekday(b.config.WeeklySummaryDay)
	targetHour := b.config.WeeklySummaryHour
	targetMinute := b.config.WeeklySummaryMinute

	// Вычисляем целевое время на текущей неделе
	targetTime := time.Date(
		now.Year(), now.Month(), now.Day(),
		targetHour, targetMinute, 0, 0,
		now.Location(),
	)

	// Корректируем день недели
	daysUntilTarget := int(targetWeekday - now.Weekday())
	if daysUntilTarget < 0 {
		// Целевой день уже прошел на этой неделе, переходим на следующую неделю
		daysUntilTarget += 7
	} else if daysUntilTarget == 0 && now.After(targetTime) {
		// Сегодня целевой день, но время уже прошло, переходим на следующую неделю
		daysUntilTarget = 7
	}

	targetTime = targetTime.AddDate(0, 0, daysUntilTarget)
	return targetTime
}

// sendWeeklySummaryToAllChats отправляет еженедельное саммари во все активные чаты
func (b *Bot) sendWeeklySummaryToAllChats() {
	startTime := time.Now()
	log.Printf("[WeeklySummary][SCHEDULER] 🚀 Запуск массовой отправки еженедельного саммари во все активные чаты")

	// Отправляем во все активные чаты
	b.settingsMutex.RLock()
	activeChatIDs := make([]int64, 0, len(b.chatSettings))
	for chatID, settings := range b.chatSettings {
		if settings.Active {
			activeChatIDs = append(activeChatIDs, chatID)
		}
	}
	b.settingsMutex.RUnlock()

	activeCount := len(activeChatIDs)
	log.Printf("[WeeklySummary][SCHEDULER] Найдено %d активных чатов для отправки еженедельного саммари", activeCount)
	log.Printf("[WeeklySummary][SCHEDULER] Список активных чатов: %v", activeChatIDs)

	if activeCount == 0 {
		log.Printf("[WeeklySummary][SCHEDULER WARN] Нет активных чатов для отправки еженедельного саммари.")
		return
	}

	var wg sync.WaitGroup
	var successCount int32
	var errorCount int32

	for i, chatID := range activeChatIDs {
		wg.Add(1)
		go func(cid int64, index int) {
			defer wg.Done()
			chatStartTime := time.Now()
			log.Printf("[WeeklySummary][SCHEDULER] Чат %d/%d: Начало генерации еженедельного саммари для чата %d", index+1, activeCount, cid)

			// Обработка с таймаутом для каждого чата
			done := make(chan bool, 1)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[WeeklySummary][SCHEDULER ERROR] Паника при генерации саммари для чата %d: %v", cid, r)
						atomic.AddInt32(&errorCount, 1)
					}
					done <- true
				}()

				b.createAndSendWeeklySummary(cid)
				atomic.AddInt32(&successCount, 1)
			}()

			select {
			case <-done:
				chatDuration := time.Since(chatStartTime)
				log.Printf("[WeeklySummary][SCHEDULER] Чат %d/%d: Завершена генерация саммари для чата %d за %v", index+1, activeCount, cid, chatDuration)
			case <-time.After(5 * time.Minute):
				log.Printf("[WeeklySummary][SCHEDULER ERROR] Таймаут генерации саммари для чата %d (5 минут)", cid)
				atomic.AddInt32(&errorCount, 1)
			}
		}(chatID, i)
	}
	wg.Wait() // Ждем завершения отправки во все чаты

	totalDuration := time.Since(startTime)
	log.Printf("[WeeklySummary][SCHEDULER] ✅ Массовая отправка еженедельного саммари завершена за %v", totalDuration)
	log.Printf("[WeeklySummary][SCHEDULER] Статистика: Успешно: %d, Ошибок: %d, Всего: %d", successCount, errorCount, activeCount)
}
