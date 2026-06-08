package bot

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	"github.com/Henry-Case-dev/luna_bot/internal/deepseek"
	"github.com/Henry-Case-dev/luna_bot/internal/gemini"
	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/openrouter"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	// Импортируем драйвер postgres, но не используем его напрямую здесь
	_ "github.com/lib/pq"
)

// PersonalityMemory хранит ключевые факты о "личности" бота и текущем диалоге
// (для улучшения контекстуальности и самоидентификации)
type PersonalityMemory struct {
	NameMentions      map[string]bool // Отслеживание упоминания имён (бота и других)
	RecentTopics      []string        // Недавние темы обсуждения
	SelfPerception    []string        // Как бот видит себя в диалоге
	DiscussionContext map[string]bool // Текущие темы обсуждения
	mutex             sync.RWMutex    // Для потокобезопасности
}

// Bot структура
type Bot struct {
	api                      *tgbotapi.BotAPI
	llm                      llm.LLMClient
	embeddingClient          *gemini.Client
	storage                  storage.ChatHistoryStorage // Используем интерфейс
	config                   *config.Config
	chatSettings             map[int64]*ChatSettings         // Настройки чатов (в памяти)
	pendingSettings          map[int64]string                // Отслеживание ожидаемого ввода настроек [chatID]settingKey
	directReplyTimestamps    map[int64]map[int64][]time.Time // Временные метки прямых ответов [chatID][userID]timestamps
	settingsMutex            sync.RWMutex
	stop                     chan struct{}
	summaryMutex             sync.RWMutex
	lastSummaryRequest       map[int64]time.Time
	lastWeeklySummaryRequest map[int64]time.Time
	autoSummaryTicker        *time.Ticker            // Оставляем для авто-саммари
	randSource               *rand.Rand              // Источник случайных чисел
	autoBioSemaphore         chan struct{}           // Семафор для ограничения параллельного анализа AutoBio
	moderation               *ModerationService      // Сервис модерации
	reactionTracker          *ReactionTracker        // Сервис отслеживания реакций
	reactionPoller           *ReactionPoller         // Поллер для получения реакций (ОТКЛЮЧЕН)
	reactionHandler          *ReactionHandler        // Обработчик реакций из raw JSON
	reactionAnalyzer         *ReactionAnalyzer       // Анализатор качества сообщений на основе реакций
	customUpdatesPoller      *CustomUpdatesPoller    // Кастомный поллер с поддержкой реакций
	webSearch                *WebSearchService       // Сервис веб-поиска
	reactionStats            *ReactionStatistics     // Статистика обработки реакций
	voiceMessageService      *VoiceMessageService    // Сервис голосовых сообщений
	freeWillService          *FreeWillService        // Сервис Free Will для "живого" поведения
	antiRepetitionService    *AntiRepetitionService  // Сервис предотвращения повторений
	userValidator            *UserReferenceValidator // Система дисамбигуации пользователей
	messagePostProcessor     *MessagePostProcessor   // Система постобработки сообщений
	imageGenerationService   *ImageGenerationService // Сервис генерации изображений

	// Минимальная дедупликация отправок: ключ (chatID|source|originalMessageID) → время последней отправки
	dedupMu  sync.Mutex
	dedupMap map[string]time.Time
	dedupTTL time.Duration
}

// retLlmClient — тонкий декоратор над llm.LLMClient с экспоненциальным ретраем и джиттером для транзиентных ошибок.
type retLlmClient struct {
	inner llm.LLMClient
	// Параметры можно расширить/взять из cfg при необходимости
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
	// Функция фолбэка: по requestType вернуть альтернативный клиент или nil
	getFallback func(respType llm.ResponseType) llm.LLMClient
	// Разрешенные типы для фолбэка (нормализованные строки)
	fallbackAllowed map[string]struct{}
}

func newRetryingLLMClient(inner llm.LLMClient, cfg *config.Config) llm.LLMClient {
	// Консервативные дефолты; при необходимости вынести в env
	r := &retLlmClient{
		inner:       inner,
		maxAttempts: 3,
		baseDelay:   200 * time.Millisecond,
		maxDelay:    1200 * time.Millisecond,
		getFallback: nil,
		fallbackAllowed: func() map[string]struct{} {
			m := map[string]struct{}{}
			if cfg != nil {
				for _, t := range cfg.LLMFallbackCriticalTypes {
					m[strings.ToLower(strings.TrimSpace(t))] = struct{}{}
				}
			}
			return m
		}(),
	}
	return r
}

// shouldRetry определяет, стоит ли ретраить ошибку (только явные транзиентные случаи, без логики провайдера)
func (r *retLlmClient) shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	// 5xx/429/timeout/temporarily unavailable
	return strings.Contains(s, " 5") || // грубый фильтр на коды в строке
		strings.Contains(s, "429") ||
		strings.Contains(s, "rate limit") ||
		strings.Contains(s, "timeout") ||
		strings.Contains(s, "temporar") ||
		strings.Contains(s, "unavailable")
}

func (r *retLlmClient) backoff(attempt int) {
	// экспонента + джиттер
	pow := math.Pow(2, float64(attempt-1))
	delay := time.Duration(float64(r.baseDelay) * pow)
	if delay > r.maxDelay {
		delay = r.maxDelay
	}
	// джиттер ±30%
	jitterFrac := 0.3
	jitter := (rand.Float64()*2 - 1) * jitterFrac
	jittered := time.Duration(float64(delay) * (1 + jitter))
	if jittered < 0 {
		jittered = delay
	}
	time.Sleep(jittered)
}

// Ниже — прозрачная прокси-реализация интерфейса llm.LLMClient
func (r *retLlmClient) GenerateResponse(systemPrompt string, history []*tgbotapi.Message, lastMessage *tgbotapi.Message, temperature float32) (string, error) {
	var out string
	var err error
	for attempt := 1; attempt <= r.maxAttempts; attempt++ {
		out, err = r.inner.GenerateResponse(systemPrompt, history, lastMessage, temperature)
		if err == nil || !r.shouldRetry(err) {
			return out, err
		}
		log.Printf("[LLM-Retry] GenerateResponse попытка %d/%d из-за ошибки: %v", attempt, r.maxAttempts, err)
		r.backoff(attempt)
	}
	return out, err
}

func (r *retLlmClient) GenerateResponseFromTextContext(systemPrompt string, contextText string, temperature float32) (string, error) {
	var out string
	var err error
	for attempt := 1; attempt <= r.maxAttempts; attempt++ {
		out, err = r.inner.GenerateResponseFromTextContext(systemPrompt, contextText, temperature)
		if err == nil || !r.shouldRetry(err) {
			return out, err
		}
		log.Printf("[LLM-Retry] GenerateResponseFromTextContext попытка %d/%d из-за ошибки: %v", attempt, r.maxAttempts, err)
		r.backoff(attempt)
	}
	return out, err
}

func (r *retLlmClient) GenerateArbitraryResponse(systemPrompt string, contextText string, temperature float32) (string, error) {
	var out string
	var err error
	for attempt := 1; attempt <= r.maxAttempts; attempt++ {
		out, err = r.inner.GenerateArbitraryResponse(systemPrompt, contextText, temperature)
		if err == nil || !r.shouldRetry(err) {
			return out, err
		}
		log.Printf("[LLM-Retry] GenerateArbitraryResponse попытка %d/%d из-за ошибки: %v", attempt, r.maxAttempts, err)
		r.backoff(attempt)
	}
	return out, err
}

func (r *retLlmClient) GenerateResponseByType(responseType llm.ResponseType, systemPrompt string, contextText string, temperature float32) (string, error) {
	var out string
	var err error
	for attempt := 1; attempt <= r.maxAttempts; attempt++ {
		out, err = r.inner.GenerateResponseByType(responseType, systemPrompt, contextText, temperature)
		if err == nil || !r.shouldRetry(err) {
			return out, err
		}
		log.Printf("[LLM-Retry] GenerateResponseByType(%s) попытка %d/%d из-за ошибки: %v", string(responseType), attempt, r.maxAttempts, err)
		r.backoff(attempt)
	}
	// Если есть фолбэк и тип разрешен — пробуем альтернативный клиент
	if r.getFallback != nil {
		if _, ok := r.fallbackAllowed[strings.ToLower(string(responseType))]; ok {
			fb := r.getFallback(responseType)
			if fb != nil {
				log.Printf("[LLM-Fallback] Переключение на альтернативный провайдер для типа %s", responseType)
				return fb.GenerateResponseByType(responseType, systemPrompt, contextText, temperature)
			}
		}
	}
	return out, err
}

func (r *retLlmClient) TranscribeAudio(audioData []byte, mimeType string) (string, error) {
	// аудио как правило короткие — без ретрая, прямой вызов
	return r.inner.TranscribeAudio(audioData, mimeType)
}

func (r *retLlmClient) EmbedContent(text string) ([]float32, error) {
	// эмбеддинги могут быть многочисленны — ретрай не обязателен
	return r.inner.EmbedContent(text)
}

func (r *retLlmClient) GenerateContentWithImage(ctx context.Context, systemPrompt string, imageData []byte, caption string) (string, error) {
	// изображения редки — без ретрая, прямой вызов
	return r.inner.GenerateContentWithImage(ctx, systemPrompt, imageData, caption)
}

func (r *retLlmClient) GenerateImageWithEdit(ctx context.Context, baseImageData []byte, editPrompt string) ([]byte, error) {
	// генерация изображений редка — без ретрая, прямой вызов
	return r.inner.GenerateImageWithEdit(ctx, baseImageData, editPrompt)
}

func (r *retLlmClient) Close() error { return r.inner.Close() }

// GetRecentTopicsForChat возвращает до n последних тем из PersonalityMemory для чата
func (b *Bot) GetRecentTopicsForChat(chatID int64, n int) []string {
	if b == nil || b.storage == nil {
		return nil
	}
	memory, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil || memory == nil || len(memory.RecentTopics) == 0 {
		return nil
	}
	topics := memory.RecentTopics
	if n > 0 && len(topics) > n {
		topics = topics[len(topics)-n:]
	}
	// Копия, чтобы избежать непреднамеренных изменений
	out := make([]string, 0, len(topics))
	for _, t := range topics {
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// buildAssociativeKeys формирует ключи для ассоциативного контекста на основе недавних тем и доп. подсказок
func (b *Bot) buildAssociativeKeys(chatID int64, extra ...string) []string {
	// Собираем последние темы (до 5) + дополнительные подсказки
	const maxKeys = 5
	keys := make([]string, 0, maxKeys)

	// Добавляем explicit extra сначала (если есть)
	for _, e := range extra {
		if e != "" {
			keys = append(keys, e)
		}
	}

	// Добавляем недавние темы
	recent := b.GetRecentTopicsForChat(chatID, maxKeys)
	keys = append(keys, recent...)

	// Уникализация и усечение
	seen := make(map[string]struct{}, len(keys))
	uniq := make([]string, 0, len(keys))
	for _, k := range keys {
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		uniq = append(uniq, k)
		if len(uniq) >= maxKeys {
			break
		}
	}
	return uniq
}

// New создает и инициализирует новый экземпляр бота
func New(cfg *config.Config) (*Bot, error) {
	log.Println("Инициализация Telegram API...")
	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, fmt.Errorf("ошибка инициализации Telegram API: %w", err)
	}

	// Устанавливаем Debug для Telegram API согласно настройке в конфиге
	api.Debug = cfg.Debug
	log.Printf("Авторизован как @%s", api.Self.UserName)
	if cfg.Debug {
		log.Println("Режим отладки включен (включая Telegram API Debug).")
	} else {
		log.Println("Режим отладки выключен.")
	}

	// Инициализация LLM клиентов с маршрутизацией по типам ответов
	log.Println("Инициализация LLM клиентов с поддержкой маршрутизации по ResponseTypeConfigs...")
	var llmClient llm.LLMClient
	var embeddingClient *gemini.Client

	// Всегда создаем клиент Gemini для эмбеддингов, фото, аудио, изображений
	embeddingClient, err = gemini.New(cfg, cfg.GeminiModelName, cfg.GeminiEmbeddingModelName, cfg.Debug)
	if err != nil {
		return nil, fmt.Errorf("ошибка инициализации клиента Gemini для эмбеддингов: %w", err)
	}
	log.Println("✓ Gemini (эмбеддинги, фото, аудио, изображения) успешно инициализирован")

	// Инициализируем все конфигурированные провайдеры в кэше
	llmClients := make(map[config.LLMProvider]llm.LLMClient)
	llmClients[config.ProviderGemini] = embeddingClient

	// Инициализируем DeepSeek если конфигурирован
	if cfg.DeepSeekAPIKey != "" {
		dsClient, err := deepseek.New(cfg.DeepSeekAPIKey, cfg.DeepSeekModelName, cfg.DeepSeekBaseURL, cfg.Debug)
		if err != nil {
			log.Printf("[WARN] Ошибка инициализации DeepSeek: %v (будет недоступен для ResponseTypeConfigs)", err)
		} else {
			llmClients[config.ProviderDeepSeek] = dsClient
			log.Println("✓ DeepSeek успешно инициализирован")
		}
	}

	// Инициализируем OpenRouter если конфигурирован
	if cfg.OpenRouterAPIKey != "" {
		orClient, err := openrouter.New(cfg.OpenRouterAPIKey, cfg.OpenRouterModelName, cfg.OpenRouterSiteURL, cfg.OpenRouterSiteTitle, cfg)
		if err != nil {
			log.Printf("[WARN] Ошибка инициализации OpenRouter: %v (будет недоступен для ResponseTypeConfigs)", err)
		} else {
			llmClients[config.ProviderOpenRouter] = orClient
			log.Println("✓ OpenRouter успешно инициализирован")
		}
	}

	// Создаем LLMRouter для маршрутизации запросов по типам ответов
	router := NewLLMRouter(cfg, llmClients, embeddingClient, cfg.Debug)
	llmClient = router

	// Оборачиваем router в ретраящий декоратор с поддержкой фолбэка
	rClient := newRetryingLLMClient(llmClient, cfg).(*retLlmClient)
	rClient.getFallback = func(respType llm.ResponseType) llm.LLMClient {
		if cfg == nil || !cfg.LLMFallbackEnabled {
			return nil
		}
		// Порядок провайдеров из конфига
		for _, p := range cfg.LLMFallbackProviderOrder {
			name := strings.ToLower(strings.TrimSpace(p))
			if client, ok := llmClients[config.LLMProvider(name)]; ok && client != nil {
				return client
			}
		}
		return nil
	}
	llmClient = rClient

	log.Println("✓ LLM маршрутизация успешно инициализирована (Gemini → основной + ResponseTypeConfigs)")

	// Логируем таблицу маршрутизации для отладки
	if len(cfg.ResponseTypeConfigs) > 0 {
		config.LogResponseTypeConfigs(cfg.ResponseTypeConfigs)
	}

	// Инициализация источника случайных чисел
	source := rand.NewSource(time.Now().UnixNano())
	randGen := rand.New(source)

	// Инициализация настроек в памяти
	chatSettings := make(map[int64]*ChatSettings)

	// Инициализация хранилища
	log.Printf("Инициализация хранилища: %s", cfg.StorageType)
	var storageImpl storage.ChatHistoryStorage
	var initErr error // Используем initErr для ошибок инициализации хранилища
	switch cfg.StorageType {
	case config.StorageTypeFile:
		storageImpl = storage.NewFileStorage(cfg.ContextWindow, true) // Используем =
		log.Println("Используется файловое хранилище")
	case config.StorageTypePostgres:
		// Используем = для storageImpl и initErr
		storageImpl, initErr = storage.NewPostgresStorage(
			cfg.PostgresqlHost,
			cfg.PostgresqlPort,
			cfg.PostgresqlUser,
			cfg.PostgresqlPassword,
			cfg.PostgresqlDbname,
			cfg.ContextWindow,
			cfg.Debug,
		)
		if initErr != nil {
			return nil, fmt.Errorf("ошибка создания PostgreSQL хранилища: %w", initErr)
		}
		log.Println("Используется PostgreSQL хранилище")
	case config.StorageTypeMongo:
		log.Println("[WARN] MongoDB хранилище больше не поддерживается. Используйте STORAGE_TYPE=postgres.")
		return nil, fmt.Errorf("MongoDB не поддерживается. Установите STORAGE_TYPE=postgres")
	default:
		return nil, fmt.Errorf("неизвестный тип хранилища: %s", cfg.StorageType)
	}

	// Создание экземпляра бота
	b := &Bot{
		api:                      api,
		llm:                      llmClient,
		embeddingClient:          embeddingClient,
		storage:                  storageImpl,
		config:                   cfg,
		chatSettings:             chatSettings,
		pendingSettings:          make(map[int64]string),
		directReplyTimestamps:    make(map[int64]map[int64][]time.Time),
		settingsMutex:            sync.RWMutex{},
		stop:                     make(chan struct{}),
		summaryMutex:             sync.RWMutex{},
		lastSummaryRequest:       make(map[int64]time.Time),
		lastWeeklySummaryRequest: make(map[int64]time.Time),
		autoSummaryTicker:        nil,
		randSource:               randGen,
		autoBioSemaphore:         make(chan struct{}, 1), // Инициализация семафора
		// Инициализация структуры дедупликации
		dedupMap: make(map[string]time.Time),
		dedupTTL: 5 * time.Second,
	}

	// Инициализация сервиса модерации ПОСЛЕ создания объекта Bot
	b.moderation = NewModerationService(b)

	// Инициализация сервиса отслеживания реакций
	b.reactionTracker = NewReactionTracker(b, storageImpl)

	// Инициализация поллера реакций
	b.reactionPoller = NewReactionPoller(b)

	// Инициализация обработчика реакций
	b.reactionHandler = NewReactionHandler(b)

	// Инициализация анализатора реакций
	b.reactionAnalyzer = NewReactionAnalyzer(b)

	// Инициализация кастомного поллера с поддержкой реакций
	b.customUpdatesPoller = NewCustomUpdatesPoller(b)

	// Инициализация сервиса веб-поиска
	b.webSearch = NewWebSearchService(b)

	// Инициализация статистики реакций
	b.reactionStats = NewReactionStatistics()

	// Инициализация сервиса голосовых сообщений
	voiceService, voiceErr := NewVoiceMessageService(b)
	if voiceErr != nil {
		log.Printf("[WARN] Ошибка инициализации сервиса голосовых сообщений: %v", voiceErr)
		// Не прерываем инициализацию бота, только логируем ошибку
	}
	b.voiceMessageService = voiceService

	// Инициализация сервиса Free Will
	log.Printf("[Bot] 🤖 Создаем сервис Free Will...")
	b.freeWillService = NewFreeWillService(b)
	if b.freeWillService == nil {
		log.Printf("[Bot] ❌ КРИТИЧЕСКАЯ ОШИБКА: freeWillService = nil после создания!")
	} else {
		log.Printf("[Bot] ✅ Сервис Free Will создан. Адрес: %p", b.freeWillService)
		log.Printf("[Bot] 📊 Сервис Free Will инициализирован. Статус: %s",
			map[bool]string{true: "включен", false: "выключен"}[b.freeWillService.IsEnabled()])
	}

	// Инициализация сервиса анти-повторений
	log.Printf("[Bot] 🔄 Создаем сервис AntiRepetition...")
	b.antiRepetitionService = NewAntiRepetitionService(b)
	if b.antiRepetitionService == nil {
		log.Printf("[Bot] ❌ КРИТИЧЕСКАЯ ОШИБКА: antiRepetitionService = nil после создания!")
	} else {
		log.Printf("[Bot] ✅ Сервис AntiRepetition создан и запущен")
	}

	// Инициализация системы дисамбигуации пользователей
	if b.config.DisambiguationEnabled {
		log.Printf("[Bot] 👥 Создаем систему дисамбигуации пользователей...")
		b.userValidator = NewUserReferenceValidator(storageImpl)
		if b.userValidator == nil {
			log.Printf("[Bot] ❌ КРИТИЧЕСКАЯ ОШИБКА: userValidator = nil после создания!")
		} else {
			log.Printf("[Bot] ✅ Система дисамбигуации пользователей создана")
		}
	} else {
		b.userValidator = nil
		log.Printf("[Bot] 👥 Система дисамбигуации пользователей отключена по конфигу (DISAMBIGUATION_ENABLED=false)")
	}

	// Инициализация системы постобработки сообщений
	log.Printf("[Bot] 🔧 Создаем систему постобработки сообщений...")
	b.messagePostProcessor = NewMessagePostProcessor(b)

	// Инициализируем сервис генерации изображений
	b.imageGenerationService = NewImageGenerationService(b, b.config)
	log.Printf("[Bot] ✅ Сервис генерации изображений инициализирован")
	if b.messagePostProcessor == nil {
		log.Printf("[Bot] ❌ КРИТИЧЕСКАЯ ОШИБКА: messagePostProcessor = nil после создания!")
	} else {
		log.Printf("[Bot] ✅ Система постобработки сообщений создана. Статус: %s",
			map[bool]string{true: "включена", false: "выключена"}[b.messagePostProcessor.IsEnabled()])
	}

	// Загрузка всех настроек чатов при старте
	b.loadAllChatSettingsFromStorage()

	globalBotInstance = b // Устанавливаем глобальный указатель для доступа к памяти личности

	log.Println("Инициализация бота завершена.")
	return b, nil
}

// Start запускает основного бота
func (b *Bot) Start() error {
	log.Println("=== START: Начало функции Start() ===")
	b.stop = make(chan struct{}) // Пересоздаем канал при старте

	// Отключаем webhook, чтобы избежать конфликтов с polling
	log.Println("=== START: Отключение webhook ===")
	deleteWebhookConfig := tgbotapi.DeleteWebhookConfig{DropPendingUpdates: true}
	_, err := b.api.Request(deleteWebhookConfig)
	if err != nil {
		log.Printf("[WARN] Ошибка отключения webhook: %v", err)
	} else {
		log.Println("Webhook успешно отключен")
	}

	// Загрузка истории для существующих чатов при старте
	log.Println("=== START: Начинаю загрузку истории для существующих чатов ===")
	// Загрузка ID чатов из хранилища
	chatIDsToLoad, err := b.storage.GetAllChatIDs()
	if err != nil {
		log.Printf("[ERROR] Не удалось получить список ChatID из хранилища для загрузки истории: %v", err)
	} else {
		log.Printf("Найдено %d чатов в хранилище для загрузки истории.", len(chatIDsToLoad))
		for _, chatID := range chatIDsToLoad {
			log.Printf("=== START: Обрабатываю чат %d ===", chatID)
			// Проверяем, есть ли настройки для этого чата (могли быть удалены)
			b.settingsMutex.RLock()
			_, settingsExist := b.chatSettings[chatID]
			b.settingsMutex.RUnlock()
			if settingsExist {
				log.Printf("=== START: Запускаю загрузку истории для чата %d ===", chatID)
				go b.loadChatHistory(chatID) // Запускаем загрузку только если есть настройки
			} else {
				log.Printf("[WARN] Пропуск загрузки истории для чата %d: настройки не найдены.", chatID)
			}
		}
		log.Printf("Запущена фоновая загрузка истории для %d чатов (с существующими настройками).", len(chatIDsToLoad))
	}

	log.Println("=== START: Запуск планировщиков ===")
	// Запуск планировщиков
	go b.scheduleDailyTake(b.config.DailyTakeTime, b.config.TimeZone)
	log.Println("=== START: Планировщик Daily Take запущен ===")

	if b.config.SummaryIntervalHours > 0 {
		go b.scheduleAutoSummary()
		log.Println("=== START: Планировщик Auto Summary запущен ===")
	} else {
		log.Println("Автоматическое саммари отключено (SUMMARY_INTERVAL_HOURS <= 0).")
	}

	// Запуск планировщика для отправки сообщений о донате (с учетом DONATE_PROMPT_ENABLED)
	if cfg, ok := b.config.ResponseTypeConfigs["donate"]; ok && !cfg.Enabled {
		log.Println("Отправка сообщений о донате отключена (DONATE_PROMPT_ENABLED=false).")
	} else if b.config.DonateTimeHours > 0 {
		go b.scheduleDonate()
		log.Printf("Запущен планировщик сообщений о донате с интервалом %d часов", b.config.DonateTimeHours)
	} else {
		log.Println("Отправка сообщений о донате отключена (DONATE_TIME_HOURS <= 0).")
	}

	// Запуск планировщика еженедельного саммари
	if b.config.WeeklySummaryEnabled {
		go b.scheduleWeeklySummary()
		log.Printf("Запущен планировщик еженедельного саммари (день: %d, время: %02d:%02d)", b.config.WeeklySummaryDay, b.config.WeeklySummaryHour, b.config.WeeklySummaryMinute)
	} else {
		log.Println("Еженедельное саммари отключено (WEEKLY_SUMMARY_ENABLED = false).")
	}

	log.Println("=== START: Запуск планировщика Auto Bio Analysis ===")
	// Запуск планировщика Auto Bio Analysis
	go b.scheduleAutoBioAnalysis()
	log.Println("=== START: Планировщик Auto Bio Analysis запущен ===")

	log.Println("=== START: Запуск планировщика обновления личности бота ===")
	// Запуск планировщика обновления личности бота
	go b.startPersonalityServices()
	log.Println("=== START: Планировщик обновления личности бота запущен ===")

	// Запускаем планировщик для анализа профилей пользователей
	go b.scheduleAutoBioAnalysis()

	// Запускаем планировщик для проверки тишины Free Will
	go b.scheduleFreeWillSilenceCheck()

	log.Println("=== START: Запуск периодического логирования статистики реакций ===")
	// Запуск периодического логирования статистики реакций
	go b.reactionStats.StartPeriodicLogging(b.stop)
	log.Println("=== START: Периодическое логирование статистики реакций запущено ===")

	// Поллер реакций отключен - реакции обрабатываются в основном потоке
	// b.reactionPoller.Start()

	log.Println("=== START: Запуск автоочистки MongoDB ===")
	// Запуск автоочистки MongoDB (ПЕРЕД циклом)
	if b.config.StorageType == config.StorageTypeMongo && b.config.MongoCleanupEnabled {
		log.Println("Запуск фоновой задачи автоочистки MongoDB...")
		go func() {
			// Используем начальную задержку, чтобы не стартовать сразу при запуске бота
			initialDelay := time.Duration(1) * time.Minute
			select {
			case <-time.After(initialDelay):
				log.Println("[Cleanup] Начало периодической проверки коллекций MongoDB.")
			case <-b.stop:
				log.Println("[Cleanup] Остановка до начала первой проверки.")
				return
			}

			ticker := time.NewTicker(time.Duration(b.config.MongoCleanupIntervalMinutes) * time.Minute)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					if b.config.Debug {
						log.Println("[Cleanup DEBUG] Начало цикла проверки автоочистки.")
					}
					mongoStore, ok := b.storage.(*storage.PostgresStorage)
					if !ok {
						// Эта проверка избыточна, т.к. мы уже проверили StorageType, но оставим для надежности
						log.Printf("[Cleanup ERROR] Хранилище не является MongoStorage, очистка невозможна.")
						return // Выходим из горутины, если тип не тот
					}

					// Запускаем общую проверку размера БД и очистку при необходимости
					go func() {
						if b.config.Debug {
							log.Printf("[Cleanup DEBUG] Запуск EnsureTotalDBSizeWithinLimit")
						}
						cleaned, err := mongoStore.EnsureTotalDBSizeWithinLimit(b.config)
						if err != nil {
							log.Printf("[Cleanup ERROR] Ошибка во время проверки размера БД: %v", err)
						} else if cleaned {
							log.Printf("[Cleanup INFO] В результате проверки размера БД были удалены сообщения.")
						} else if b.config.Debug {
							log.Printf("[Cleanup DEBUG] Проверка размера БД завершена (сообщения не удалялись).")
						}
					}()

				case <-b.stop:
					log.Println("[Cleanup] Остановка планировщика автоочистки.")
					return // Выход из горутины автоочистки
				}
			}
		}()
	} else {
		log.Println("Автоочистка MongoDB отключена.")
	}
	// --- Конец запуска автоочистки ---
	log.Println("=== START: Автоочистка MongoDB настроена ===")

	log.Println("=== START: Запуск кастомного поллера ===")
	// Запуск кастомного поллера с поддержкой реакций
	updates := b.customUpdatesPoller.Start()
	log.Println("=== START: Кастомный поллер запущен ===")

	// КРИТИЧЕСКИ ВАЖНО: Устанавливаем глобальную ссылку на Bot для доступа к памяти личности
	globalBotInstance = b
	log.Println("=== START: Глобальная ссылка на Bot установлена ===")

	// === ОТПРАВКА STARTUP MESSAGE ВО ВСЕ АКТИВНЫЕ ЧАТЫ ===
	log.Println("=== START: Отправка startup message во все активные чаты ===")
	go b.sendStartupMessageToAllChats()

	log.Println("Бот начал слушать обновления (с поддержкой реакций)...")
	log.Println("=== START: Запуск основного цикла обработки обновлений ===")

	// Основной цикл обработки обновлений
	for {
		select {
		case update := <-updates:
			go b.handleUpdate(update)
		case <-b.stop:
			log.Println("Получен сигнал остановки, завершение работы...")
			return nil // Выход из функции Start
		}
	}
}

// Stop gracefully stops the bot
func (b *Bot) Stop() {
	log.Println("Получен сигнал остановки бота...")

	// Сигнализируем всем горутинам о необходимости остановиться
	close(b.stop) // Закрытие канала stop сигнализирует всем слушателям

	// --- Сохранение истории и настроек ---
	// Сохранение истории теперь управляется планировщиком и вызывается при закрытии канала stop
	// saveAllChatHistories() вызывается внутри scheduleHistorySaving при получении сигнала <-b.stop

	log.Println("Сохранение всех настроек чатов...")
	b.settingsMutex.RLock()
	settingsToSave := make(map[int64]*ChatSettings, len(b.chatSettings))
	for id, settings := range b.chatSettings {
		// Копируем настройки, чтобы избежать гонки данных при сохранении в горутинах (если бы оно было)
		copiedSettings := *settings
		settingsToSave[id] = &copiedSettings
	}
	b.settingsMutex.RUnlock()

	var wg sync.WaitGroup
	for chatID, settings := range settingsToSave {
		wg.Add(1)
		go func(cid int64, s *ChatSettings) {
			defer wg.Done()
			if err := saveChatSettings(cid, s); err != nil { // Вызываем как обычную функцию
				log.Printf("Ошибка сохранения настроек для чата %d при остановке: %v", cid, err)
			}
		}(chatID, settings)
	}
	wg.Wait()
	log.Println("Сохранение настроек чатов завершено.")

	// Останавливаем поллер реакций
	if b.reactionPoller != nil {
		b.reactionPoller.Stop()
	}

	// Останавливаем кастомный поллер
	if b.customUpdatesPoller != nil {
		b.customUpdatesPoller.Stop()
	}

	// Закрываем LLM клиент
	if b.llm != nil {
		log.Println("Закрытие LLM клиента...")
		if err := b.llm.Close(); err != nil {
			log.Printf("Ошибка при закрытии LLM клиента: %v", err)
		} else {
			log.Println("LLM клиент успешно закрыт.")
		}
	}

	// Закрываем хранилище (важно для PostgreSQL)
	if b.storage != nil {
		log.Println("Закрытие хранилища...")
		if err := b.storage.Close(); err != nil {
			log.Printf("Ошибка при закрытии хранилища: %v", err)
		} else {
			log.Println("Хранилище успешно закрыто.")
		}
	}

	log.Println("Бот успешно остановлен.")
}

// ensureChatInitializedAndWelcome проверяет, инициализирован ли чат, и приветствует, если нет.
// Возвращает настройки чата и флаг, был ли чат только что инициализирован.
func (b *Bot) ensureChatInitializedAndWelcome(update tgbotapi.Update) (*ChatSettings, bool) {
	var chatID int64
	var chatTitle string
	if update.Message != nil {
		chatID = update.Message.Chat.ID
		chatTitle = update.Message.Chat.Title
	} else if update.CallbackQuery != nil {
		chatID = update.CallbackQuery.Message.Chat.ID
		chatTitle = update.CallbackQuery.Message.Chat.Title
	} else {
		return nil, false // Неизвестный тип апдейта
	}

	b.settingsMutex.RLock()
	settings, exists := b.chatSettings[chatID]
	b.settingsMutex.RUnlock()

	if exists && settings.Active {
		return settings, false // Чат уже активен
	}

	// --- Чат не существует или неактивен, инициализируем --- \
	justInitialized := !exists || !settings.Active
	log.Printf("Инициализация нового или неактивного чата: %d (%s)", chatID, chatTitle)

	b.settingsMutex.Lock()
	defer b.settingsMutex.Unlock()

	// Перепроверяем существование после захвата мьютекса
	settings, exists = b.chatSettings[chatID]
	if exists && settings.Active {
		return settings, false
	}

	// Определяем начальный message count
	settings = &ChatSettings{
		Active:                 true,
		MinMessages:            b.config.MinMessages,
		MaxMessages:            b.config.MaxMessages,
		DailyTakeTime:          b.config.DailyTakeTime,
		SummaryIntervalHours:   b.config.SummaryIntervalHours,
		MessageCount:           0,
		NextTargetMessageCount: b.config.MinMessages + b.randSource.Intn(b.config.MaxMessages-b.config.MinMessages+1),
		SrachState:             "none",
		SrachAnalysisEnabled:   b.config.SrachAnalysisEnabled,
	}
	b.chatSettings[chatID] = settings

	// Отправляем приветствие только при ПЕРВОЙ инициализации
	if justInitialized {
		log.Printf("Чат %d: Отправка приветственного сообщения...", chatID)
		welcomePrompt := b.enrichPromptWithPersonality(b.config.WelcomePrompt, chatID, "welcome")
		welcomeText, err := b.llm.GenerateResponseByType(llm.ResponseTypeWelcome, welcomePrompt, "", float32(b.config.GeminiTemperatureNormal))
		if err != nil {
			log.Printf("[ERROR][Bot] Ошибка генерации приветственного сообщения для чата %d: %v", chatID, err)
			// Если ошибка, отправляем стандартное приветствие
			welcomeText = "Привет! Я ваш новый спутник в чате. Рад быть здесь!"
		} else {
			// Очищаем ответ от возможных метаданных перед отправкой
			welcomeText = cleanupLLMResponse(welcomeText)
		}
		b.sendSystemMessage(chatID, welcomeText)

		// После приветствия загружаем историю
		go b.loadChatHistory(chatID)
		// Активируем модерацию для нового чата
		go b.moderation.CheckAdminRightsAndActivate(chatID)

		// Отправляем стартовое сообщение после небольшой задержки, чтобы дать время на инициализацию
		go func() {
			time.Sleep(2 * time.Second) // Даем время для завершения инициализации
			b.sendStartupMessage(chatID)
		}()

		// Инициализируем personal memory для нового чата
		go func(cid int64) {
			memory := &storage.PersonalityMemory{
				ChatID:            cid,
				NameMentions:      map[string]bool{},
				RecentTopics:      []string{},
				SelfPerception:    []string{},
				DiscussionContext: make(map[string]bool),
			}

			if err := b.storage.SavePersonalityMemory(memory); err != nil {
				log.Printf("[ERROR][Bot] Ошибка инициализации personality_memory для нового чата %d: %v", cid, err)
			} else if b.config.Debug {
				log.Printf("[DEBUG][Bot] Инициализирована personality_memory для нового чата %d", cid)
			}
		}(chatID)
	}

	return settings, justInitialized
}

// handleUpdate обрабатывает входящие обновления
func (b *Bot) handleUpdate(update tgbotapi.Update) {
	startTime := time.Now()

	// Гарантируем, что для чата существуют настройки в памяти
	_, justInitialized := b.ensureChatInitializedAndWelcome(update)
	// Если чат только что инициализирован, вероятно, не нужно обрабатывать сообщение дальше
	// (кроме CallbackQuery, которые могут прийти из старых сообщений)
	if justInitialized && update.Message != nil && update.Message.NewChatMembers == nil {
		chatID := update.Message.Chat.ID
		if b.config.Debug {
			log.Printf("[DEBUG][handleUpdate] Чат %d только что инициализирован, сообщение ID %d не обрабатывается (кроме приветствия).", chatID, update.Message.MessageID)
		}
		return
	}

	// Обработка реакций уже выполнена в CustomUpdatesPoller через ReactionHandler
	// Старый reactionTracker больше не используется

	// Обработка CallbackQuery (нажатия кнопок)
	if update.CallbackQuery != nil {
		// log.Printf("[DEBUG][Bot] Получен CallbackQuery: ID=%s, Data=%s", update.CallbackQuery.ID, update.CallbackQuery.Data)
		isReactionCB := false
		if b.reactionTracker != nil && b.config.ReactionsEnabled { // Добавлена проверка ReactionsEnabled
			// Попытка декодировать данные как реакцию
			// Это для обратной совместимости или если реакции приходят как callback
			_, _, isReaction := b.reactionTracker.reactionsAPI.DecodeReactionData(update.CallbackQuery.Data)
			if isReaction {
				isReactionCB = true
				// log.Printf("[DEBUG][Bot] CallbackQuery определен как реакция. Data: %s. Передача в ReactionTracker.", update.CallbackQuery.Data)
				// GOVNO: ЗАКОММЕНТИРОВАТЬ ВРЕМЕННО ЧТОБЫ УБРАТЬ ДУБЛИРОВАНИЕ
				// b.reactionTracker.HandleReactionUpdate(update) // <--- Эта строка будет закомментирована/удалена
			}
		}

		if !isReactionCB { // Если это не колбек-реакция, обрабатываем как обычный колбек
			go b.handleCallback(update.CallbackQuery)
			return // Выходим после обработки колбэка
		}
	}

	// Обработка обычных сообщений
	if update.Message != nil {
		// ВАЖНО: Уведомляем Free Will о ЛЮБОМ типе сообщения (включая системные)
		// Free Will обрабатывается в handleMessage(), убираем дублирование

		// Обработка команд
		if update.Message.IsCommand() {
			go b.handleCommand(update.Message)
		} else {
			// Обработка обычных текстовых сообщений
			go b.handleMessage(update) // handleMessage теперь принимает Update
		}
	} else if update.MyChatMember != nil {
		// Обработка изменений статуса бота в чате (например, удаление)
		go b.handleChatMemberUpdate(update.MyChatMember)
	}

	// Логирование времени обработки для не-CallbackQuery
	if update.Message != nil {
		processingTime := time.Since(startTime)
		if b.config.Debug {
			log.Printf("[DEBUG][Timing] Обработка Message (ID: %d) заняла %s", update.Message.MessageID, processingTime.Round(time.Millisecond))
		}
	}
}

// loadAllChatSettingsFromStorage загружает настройки всех известных чатов из хранилища в память
// (используется при старте бота)
func (b *Bot) loadAllChatSettingsFromStorage() {
	log.Println("Загрузка настроек всех чатов из хранилища...")
	chatIDs, err := b.storage.GetAllChatIDs()
	if err != nil {
		log.Printf("[ERROR] Не удалось получить список chatID из хранилища: %v", err)
		return
	}

	log.Printf("Найдено %d чатов в хранилище.", len(chatIDs))

	// Собираем список чатов для модерации ВНЕ блокировки
	chatsToActivate := make([]int64, 0, len(chatIDs))

	b.settingsMutex.Lock()
	loadedCount := 0
	failedCount := 0
	for _, chatID := range chatIDs {
		// Проверяем, есть ли уже настройки в памяти (маловероятно, но на всякий случай)
		if _, exists := b.chatSettings[chatID]; exists {
			continue
		}

		// Создаем базовые настройки в памяти
		memSettings := &ChatSettings{
			Active:                 true,
			MinMessages:            b.config.MinMessages,
			MaxMessages:            b.config.MaxMessages,
			DailyTakeTime:          b.config.DailyTakeTime,
			SummaryIntervalHours:   b.config.SummaryIntervalHours,
			MessageCount:           0,
			NextTargetMessageCount: b.config.MinMessages + b.randSource.Intn(b.config.MaxMessages-b.config.MinMessages+1),
			SrachState:             "none",
			SrachAnalysisEnabled:   b.config.SrachAnalysisEnabled,
		}
		b.chatSettings[chatID] = memSettings
		loadedCount++

		// Добавляем в список для активации модерации ПОСЛЕ освобождения блокировки
		chatsToActivate = append(chatsToActivate, chatID)

		if b.config.Debug {
			log.Printf("[DEBUG][LoadSettings] Chat %d: Инициализированы настройки в памяти с NextTargetMessageCount=%d (интервал %d-%d)",
				chatID, memSettings.NextTargetMessageCount, memSettings.MinMessages, memSettings.MaxMessages)
		}
	}
	b.settingsMutex.Unlock() // КРИТИЧЕСКИ ВАЖНО: освобождаем блокировку ДО активации модерации

	// Теперь активируем модерацию для всех чатов БЕЗ блокировки settingsMutex
	log.Printf("Активация модерации для %d чатов...", len(chatsToActivate))
	for _, chatID := range chatsToActivate {
		// Активируем модерацию для этого чата после загрузки настроек при старте
		go b.moderation.CheckAdminRightsAndActivate(chatID)

		// Убираем дублирование - стартовое сообщение отправляется централизованно через sendStartupMessageToAllChats()
		// Старый код удален для предотвращения конфликтов
	}

	log.Printf("Загружено настроек для %d чатов. Ошибок: %d.", loadedCount, failedCount)
}

// handleChatMemberUpdate обрабатывает изменения статуса бота в чате
func (b *Bot) handleChatMemberUpdate(update *tgbotapi.ChatMemberUpdated) {
	chatID := update.Chat.ID
	myStatus := update.NewChatMember.Status
	userName := update.From.UserName

	if update.NewChatMember.User.ID == b.api.Self.ID {
		log.Printf("Статус бота в чате %d изменен на '%s' пользователем @%s", chatID, myStatus, userName)
		b.settingsMutex.Lock()
		defer b.settingsMutex.Unlock()

		if myStatus == "left" || myStatus == "kicked" {
			// Бот удален или кикнут
			log.Printf("Бот удален из чата %d. Удаляю настройки из памяти.", chatID)
			delete(b.chatSettings, chatID)
			delete(b.pendingSettings, chatID)
			delete(b.directReplyTimestamps, chatID)
			b.summaryMutex.Lock()
			delete(b.lastSummaryRequest, chatID)
			b.summaryMutex.Unlock()
			// TODO: Опционально: Очистить историю в хранилище? Или оставить?
			// err := b.storage.ClearChatHistory(chatID)
			// if err != nil {
			// 	 log.Printf("[WARN] Не удалось очистить историю для чата %d после удаления бота: %v", chatID, err)
			// }
		} else if myStatus == "member" {
			// Бот добавлен или вернулся
			log.Printf("Бот добавлен или вернулся в чат %d.", chatID)
			// Настройки должны были быть созданы в ensureChatInitializedAndWelcome
			// Запускаем проверку прав для активации модерации
			go b.moderation.CheckAdminRightsAndActivate(chatID)
		} else if myStatus == "administrator" {
			// Боту дали права администратора
			log.Printf("Бот получил права администратора в чате %d.", chatID)
			// Запускаем проверку прав для активации модерации
			go b.moderation.CheckAdminRightsAndActivate(chatID)
		}
	} else {
		// Изменение статуса другого пользователя (не бота)
		if b.config.Debug {
			log.Printf("[DEBUG] Статус пользователя %d (@%s) в чате %d изменен на '%s' пользователем @%s",
				update.NewChatMember.User.ID, update.NewChatMember.User.UserName, chatID, myStatus, userName)
		}
		// TODO: Возможно, обновлять профиль пользователя (например, если он покинул чат)
	}
}

// Методы для работы с памятью личности через хранилище

// AddNameMention добавляет имя в список упоминаний бота для конкретного чата
func (b *Bot) AddNameMention(name string) {
	// Добавляем для всех активных чатов
	chatIDs, err := b.storage.GetAllChatIDs()
	if err != nil {
		log.Printf("[ERROR][Personality] Ошибка получения списка ID чатов: %v", err)
		return
	}

	for _, chatID := range chatIDs {
		b.settingsMutex.RLock()
		settings, exists := b.chatSettings[chatID]
		isActive := exists && settings.Active
		b.settingsMutex.RUnlock()

		if isActive {
			_ = b.AddNameMentionForChat(chatID, name)
		}
	}
}

// AddRecentTopic добавляет тему в список недавних обсуждений для конкретного чата
func (b *Bot) AddRecentTopic(topic string) {
	// Добавляем для всех активных чатов
	chatIDs, err := b.storage.GetAllChatIDs()
	if err != nil {
		log.Printf("[ERROR][Personality] Ошибка получения списка ID чатов: %v", err)
		return
	}

	for _, chatID := range chatIDs {
		b.settingsMutex.RLock()
		settings, exists := b.chatSettings[chatID]
		isActive := exists && settings.Active
		b.settingsMutex.RUnlock()

		if isActive {
			_ = b.AddRecentTopicForChat(chatID, topic)
		}
	}
}

// AddSelfPerception добавляет элемент самовосприятия бота для конкретного чата
func (b *Bot) AddSelfPerception(perception string) {
	// Добавляем для всех активных чатов
	chatIDs, err := b.storage.GetAllChatIDs()
	if err != nil {
		log.Printf("[ERROR][Personality] Ошибка получения списка ID чатов: %v", err)
		return
	}

	for _, chatID := range chatIDs {
		b.settingsMutex.RLock()
		settings, exists := b.chatSettings[chatID]
		isActive := exists && settings.Active
		b.settingsMutex.RUnlock()

		if isActive {
			_ = b.AddSelfPerceptionForChat(chatID, perception)
		}
	}
}

// AddDiscussionContext добавляет тему в текущий контекст обсуждения для конкретного чата
func (b *Bot) AddDiscussionContext(topic string) {
	// Добавляем для всех активных чатов
	chatIDs, err := b.storage.GetAllChatIDs()
	if err != nil {
		log.Printf("[ERROR][Personality] Ошибка получения списка ID чатов: %v", err)
		return
	}

	for _, chatID := range chatIDs {
		b.settingsMutex.RLock()
		settings, exists := b.chatSettings[chatID]
		isActive := exists && settings.Active
		b.settingsMutex.RUnlock()

		if isActive {
			_ = b.AddDiscussionContextForChat(chatID, topic)
		}
	}
}

// ClearDiscussionContext очищает текущий контекст обсуждения для всех активных чатов
func (b *Bot) ClearDiscussionContext() {
	// Очищаем для всех активных чатов
	chatIDs, err := b.storage.GetAllChatIDs()
	if err != nil {
		log.Printf("[ERROR][Personality] Ошибка получения списка ID чатов: %v", err)
		return
	}

	for _, chatID := range chatIDs {
		b.settingsMutex.RLock()
		settings, exists := b.chatSettings[chatID]
		isActive := exists && settings.Active
		b.settingsMutex.RUnlock()

		if isActive {
			_ = b.ClearDiscussionContextForChat(chatID)
		}
	}
}

// GetReactionTracker возвращает экземпляр ReactionTracker
func (b *Bot) GetReactionTracker() *ReactionTracker {
	return b.reactionTracker
}

func (b *Bot) GetReactionHandler() *ReactionHandler {
	return b.reactionHandler
}

func (b *Bot) GetCustomUpdatesPoller() *CustomUpdatesPoller {
	return b.customUpdatesPoller
}

// GetWebSearchService возвращает экземпляр WebSearchService
func (b *Bot) GetWebSearchService() *WebSearchService {
	return b.webSearch
}

// GetReactionAnalyzer возвращает экземпляр ReactionAnalyzer
func (b *Bot) GetReactionAnalyzer() *ReactionAnalyzer {
	return b.reactionAnalyzer
}

// GetReactionStats возвращает экземпляр ReactionStatistics
func (b *Bot) GetReactionStats() *ReactionStatistics {
	return b.reactionStats
}

// GetVoiceMessageService возвращает сервис голосовых сообщений
func (b *Bot) GetVoiceMessageService() *VoiceMessageService {
	return b.voiceMessageService
}

// GetFreeWillService возвращает сервис Free Will
func (b *Bot) GetFreeWillService() *FreeWillService {
	return b.freeWillService
}

// GetAntiRepetitionService возвращает сервис анти-повторений
func (b *Bot) GetAntiRepetitionService() *AntiRepetitionService {
	return b.antiRepetitionService
}

func (b *Bot) GetUserValidator() *UserReferenceValidator {
	return b.userValidator
}

// RecordInternalThought добавляет запись внутреннего монолога в память личности (ЭТАП 3)
func (b *Bot) RecordInternalThought(chatID int64, thought *storage.InternalThought) {
	if !b.config.InternalMonologueEnabled || b.storage == nil || thought == nil {
		return
	}
	mem, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil || mem == nil {
		return
	}
	mem.InternalThoughts = append(mem.InternalThoughts, thought)
	// Ограничим размер истории до 100 последних
	if len(mem.InternalThoughts) > 100 {
		mem.InternalThoughts = mem.InternalThoughts[len(mem.InternalThoughts)-100:]
	}
	_ = b.storage.SavePersonalityMemory(mem)
}

// UpdateRelationship применяет маленькие сдвиги отношений на основе события (ЭТАП 4)
func (b *Bot) UpdateRelationship(chatID, userID int64, deltaTrust, deltaIntimacy float64, note string) {
	if !b.config.RelationshipTrackingEnabled || b.storage == nil {
		return
	}
	mem, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil || mem == nil {
		return
	}
	if mem.Relationships == nil {
		mem.Relationships = make(map[string]*storage.Relationship)
	}
	key := fmt.Sprintf("%d", userID)
	rel, ok := mem.Relationships[key]
	if !ok || rel == nil {
		rel = &storage.Relationship{UserID: userID, ChatID: chatID}
		mem.Relationships[key] = rel
	}
	// Применяем капы 0..1 и небольшие коэффициенты роста/спада из конфига
	rel.Trust = clamp01(rel.Trust + deltaTrust - b.config.TrustDecayRate*0.0)
	rel.Intimacy = clamp01(rel.Intimacy + deltaIntimacy + b.config.IntimacyGrowthRate*0.0)
	rel.TotalInteractions++
	rel.LastInteraction = time.Now()
	if note != "" {
		rel.KeyMoments = append(rel.KeyMoments, storage.RelationshipEvent{Type: "update", Description: note, Impact: deltaTrust + deltaIntimacy, Timestamp: time.Now()})
		if len(rel.KeyMoments) > 50 {
			rel.KeyMoments = rel.KeyMoments[len(rel.KeyMoments)-50:]
		}
	}
	_ = b.storage.SavePersonalityMemory(mem)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// EnableDisambiguation включает систему дисамбигуации во время рантайма
func (b *Bot) EnableDisambiguation() {
	if b.userValidator != nil {
		return
	}
	b.userValidator = NewUserReferenceValidator(b.storage)
	log.Printf("[Bot] 👥 Система дисамбигуации пользователей включена (runtime)")
}

// DisableDisambiguation выключает систему дисамбигуации во время рантайма
func (b *Bot) DisableDisambiguation() {
	if b.userValidator == nil {
		return
	}
	b.userValidator = nil
	log.Printf("[Bot] 👥 Система дисамбигуации пользователей отключена (runtime)")
}

// GetMessagePostProcessor возвращает систему постобработки сообщений
func (b *Bot) GetMessagePostProcessor() *MessagePostProcessor {
	return b.messagePostProcessor
}

// GetPersonalitySummary получает строковое представление личности бота для конкретного чата
func (b *Bot) GetPersonalitySummary(chatID int64) (string, error) {
	// Получаем личность из хранилища
	memory, err := b.storage.GetPersonalityMemory(chatID)
	if err != nil {
		return "", err
	}

	// Формируем текстовое представление
	var sb strings.Builder
	sb.WriteString("\n=== ЛИЧНОСТЬ ===\n")

	// Добавляем данные о недавних темах
	if len(memory.RecentTopics) > 0 {
		sb.WriteString("Недавние темы: ")
		for i, topic := range memory.RecentTopics {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(topic)
		}
		sb.WriteString("\n")
	}

	// Добавляем самовосприятие
	if len(memory.SelfPerception) > 0 {
		sb.WriteString("Самоидентификация:\n")
		for _, perception := range memory.SelfPerception {
			sb.WriteString("- ")
			sb.WriteString(perception)
			sb.WriteString("\n")
		}
	} else {
		// Базовое восприятие по умолчанию
		sb.WriteString("Самоидентификация:\n- Личность еще не определена\n")
	}

	// Добавляем важные имена
	if len(memory.NameMentions) > 0 {
		sb.WriteString("Важные имена: ")
		i := 0
		for name := range memory.NameMentions {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(name)
			i++
		}
		sb.WriteString("\n")
	}

	// Добавляем текущий контекст
	if len(memory.DiscussionContext) > 0 {
		sb.WriteString("Текущие темы: ")
		i := 0
		for topic := range memory.DiscussionContext {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(topic)
			i++
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

func (b *Bot) runMongoDBCleanupTask(mongoStore *storage.PostgresStorage) {
	ticker := time.NewTicker(time.Duration(b.config.MongoCleanupIntervalMinutes) * time.Minute)
	defer ticker.Stop()

	// Немедленный запуск при старте
	log.Printf("[Cleanup INFO] Первоначальный запуск задачи очистки MongoDB...")
	if deleted, err := mongoStore.EnsureTotalDBSizeWithinLimit(b.config); err != nil {
		log.Printf("[Cleanup ERROR] Ошибка во время первоначальной глобальной очистки MongoDB: %v", err)
	} else if deleted {
		log.Printf("[Cleanup INFO] В результате первоначальной глобальной очистки MongoDB были удалены сообщения.")
	} else {
		log.Printf("[Cleanup INFO] Первоначальная глобальная очистка MongoDB завершена, сообщений для удаления не найдено или размер в норме.")
	}

	for {
		select {
		case <-ticker.C:
			log.Printf("[Cleanup INFO] Запуск плановой задачи очистки MongoDB...")
			if deleted, err := mongoStore.EnsureTotalDBSizeWithinLimit(b.config); err != nil {
				log.Printf("[Cleanup ERROR] Ошибка во время плановой глобальной очистки MongoDB: %v", err)
			} else if deleted {
				log.Printf("[Cleanup INFO] В результате плановой глобальной очистки MongoDB были удалены сообщения.")
			} else {
				log.Printf("[Cleanup INFO] Плановая глобальная очистка MongoDB завершена, сообщений для удаления не найдено или размер в норме.")
			}
		case <-b.stop:
			log.Println("[Cleanup INFO] Остановка задачи очистки MongoDB.")
			return
		}
	}
}

// scheduleDailyTake планирует ежедневную отправку темы для обсуждения.
// ... existing code ...

// sendStartupMessageToAllChats отправляет startup message во все активные чаты при запуске бота
func (b *Bot) sendStartupMessageToAllChats() {
	// Увеличиваем задержку для гарантии завершения инициализации
	time.Sleep(10 * time.Second)

	log.Printf("[StartupMessage] Начинаем отправку startup message во все активные чаты...")

	// Дополнительная проверка инициализации критических компонентов
	if b.api == nil {
		log.Printf("[StartupMessage] ❌ КРИТИЧЕСКАЯ ОШИБКА: b.api = nil")
		return
	}
	if b.storage == nil {
		log.Printf("[StartupMessage] ❌ КРИТИЧЕСКАЯ ОШИБКА: b.storage = nil")
		return
	}

	// Получаем список всех активных чатов
	b.settingsMutex.RLock()
	if b.chatSettings == nil {
		log.Printf("[StartupMessage] ❌ КРИТИЧЕСКАЯ ОШИБКА: b.chatSettings = nil")
		b.settingsMutex.RUnlock()
		return
	}

	activeChatIDs := make([]int64, 0, len(b.chatSettings))
	for chatID, settings := range b.chatSettings {
		if settings != nil && settings.Active {
			activeChatIDs = append(activeChatIDs, chatID)
		}
	}
	b.settingsMutex.RUnlock()

	activeCount := len(activeChatIDs)
	log.Printf("[StartupMessage] Найдено %d активных чатов для отправки startup message", activeCount)

	if activeCount == 0 {
		log.Printf("[StartupMessage] ⚠️ Нет активных чатов для отправки startup message")
		// Давайте также покажем, что у нас есть в chatSettings
		b.settingsMutex.RLock()
		log.Printf("[StartupMessage] Всего чатов в памяти: %d", len(b.chatSettings))
		for chatID, settings := range b.chatSettings {
			if settings == nil {
				log.Printf("[StartupMessage] Чат %d: настройки = nil", chatID)
			} else {
				log.Printf("[StartupMessage] Чат %d: Active=%v", chatID, settings.Active)
			}
		}
		b.settingsMutex.RUnlock()
		return
	}

	// Отправляем startup message во все активные чаты
	var successCount int32
	var errorCount int32

	var wg sync.WaitGroup
	for _, chatID := range activeChatIDs {
		wg.Add(1)
		go func(cid int64) {
			defer wg.Done()

			log.Printf("[StartupMessage] Отправка startup message в чат %d...", cid)
			// Добавляем небольшую случайную задержку для равномерной нагрузки
			time.Sleep(time.Duration(b.randSource.Intn(3000)) * time.Millisecond)

			// Отправляем startup message
			b.sendStartupMessage(cid)

			atomic.AddInt32(&successCount, 1)
			log.Printf("[StartupMessage] ✅ Startup message отправлен в чат %d", cid)
		}(chatID)
	}

	// Ждем завершения отправки во все чаты
	wg.Wait()

	log.Printf("[StartupMessage] 🎉 Завершена отправка startup message: успешно=%d, ошибок=%d, всего=%d",
		successCount, errorCount, activeCount)
}
