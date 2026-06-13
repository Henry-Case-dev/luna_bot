// DEPRECATED: используйте ConfigV2 из config.go. Будет удалён в v3.0.
package config

import "time"

// LLMProvider определяет тип для выбора LLM провайдера
type LLMProvider string

const (
	// Константы для типов LLM провайдеров
	ProviderGemini     LLMProvider = "gemini"
	ProviderDeepSeek   LLMProvider = "deepseek"
	ProviderOpenRouter LLMProvider = "openrouter"
)

// ResponseTypeConfig определяет конфигурацию LLM для конкретного типа ответа
type ResponseTypeConfig struct {
	Provider    LLMProvider `json:"provider"`    // Провайдер LLM (gemini, deepseek, openrouter)
	ModelName   string      `json:"model_name"`  // Название модели для данного провайдера
	Temperature float32     `json:"temperature"` // Температура для генерации (-1 для использования температуры из вызова)
	Enabled     bool        `json:"enabled"`     // Включена ли кастомная конфигурация для этого типа
}

// StorageType определяет тип используемого хранилища.
type StorageType string

const (
	StorageTypeFile     StorageType = "file"
	StorageTypePostgres StorageType = "postgres"
	StorageTypeMongo    StorageType = "mongo"
)

// PunishmentType определяет тип наказания в правилах модерации
type PunishmentType string

const (
	PunishMute  PunishmentType = "mute"
	PunishKick  PunishmentType = "kick"
	PunishBan   PunishmentType = "ban"
	PunishPurge PunishmentType = "purge"
	PunishNone  PunishmentType = "none"
	PunishEdit  PunishmentType = "edit" // Новый тип наказания: редактировать сообщение
)

// ModerationRule определяет структуру одного правила модерации
type ModerationRule struct {
	RuleName        string         `json:"rule_name"`
	ChatID          string         `json:"chat_id"` // "none" or Telegram ID as string
	UserID          string         `json:"user_id"` // "none" or Telegram ID as string
	Keywords        []string       `json:"keywords"`
	LLMInstruction  string         `json:"llm_instruction"` // "none" or prompt for LLM
	Punishment      PunishmentType `json:"punishment"`
	NotifyUser      bool           `json:"notify_user"`
	NotifyChat      bool           `json:"notify_chat"`
	PunishmentNote  string         `json:"punishment_note"`
	ReplacementText string         `json:"replacement_text"` // Текст для замены при PunishEdit
	// Parsed fields (filled after loading)
	ParsedChatID int64 `json:"-"`
	ParsedUserID int64 `json:"-"`
}

// Config содержит все параметры конфигурации бота
type Config struct {
	TelegramToken string
	// Общие настройки LLM
	LLMProvider   LLMProvider
	DefaultPrompt string
	// --- Новые промпты для классификации и серьезного ответа ---
	ClassifyDirectMessagePrompt string
	SeriousDirectPrompt         string
	// --- Конец новых промптов ---
	DailyTakePrompt string
	SummaryPrompt   string
	// --- Новые поля для настроек по умолчанию ---
	DefaultTemperature       float64 // Температура по умолчанию
	// --- Конец новых полей ---
	// Настройки Gemini
	GeminiAPIKey    string
	GeminiModelName string
	// --- НОВЫЕ поля для температур Gemini ---
	GeminiTemperatureNormal  float64 `json:"-"`
	GeminiTemperatureSerious float64 `json:"-"`
	// --- Конец НОВЫХ полей для температур Gemini ---
	// --- Отдельные настройки для аудио транскрибации ---
	AudioTranscriptionModel       string  `json:"-"`
	AudioTranscriptionTemperature float64 `json:"-"`
	// --- Отдельные настройки для генерации изображений ---
	ImageGenerationModel       string  `json:"-"`
	ImageGenerationTemperature float64 `json:"-"`
	// --- Конец отдельных настроек ---
	// --- Настройки резервного ключа Gemini ---
	GeminiAPIKeyReserve        string    // Резервный ключ API Gemini
	GeminiUsingReserveKey      bool      // Флаг использования резервного ключа
	GeminiKeyRotationTimeHours int       // Время в часах, через которое пробовать вернуться к основному ключу
	GeminiLastKeyRotationTime  time.Time // Время последнего переключения ключа
	// --- Конец настроек резервного ключа ---
	// Настройки DeepSeek
	DeepSeekAPIKey    string
	DeepSeekModelName string
	DeepSeekBaseURL   string // Опционально, для кастомного URL
	// --- НОВЫЕ Настройки OpenRouter ---
	OpenRouterAPIKey    string
	OpenRouterModelName string
	OpenRouterSiteURL   string // Optional HTTP-Referer
	OpenRouterSiteTitle string // Optional X-Title
	// --- КОНЕЦ Настроек OpenRouter ---
	// Настройки поведения бота
	RateLimitStaticText string // Статический текст для сообщения о лимите
	RateLimitPrompt     string // Промпт для LLM для сообщения о лимите
	// --- Настройки донатов ---
	DonatePrompt    string // Промпт для генерации сообщения о донате
	DonateTimeHours int    // Интервал отправки сообщений о донате в часах
	// --- Конец настроек донатов ---
	// Промпты для ввода настроек
	PromptEnterMinMessages     string
	PromptEnterMaxMessages     string
	PromptEnterDailyTime       string
	PromptEnterSummaryInterval string
	// Промпты для анализа срачей
	SRACH_WARNING_PROMPT  string
	SRACH_ANALYSIS_PROMPT string
	SRACH_CONFIRM_PROMPT  string
	SrachAnalysisEnabled  bool // Значение по умолчанию из .env
	// Настройки времени и интервалов
	DailyTakeTime        int
	TimeZone             string
	SummaryIntervalHours int
	// --- Настройки еженедельного саммари ---
	WeeklySummaryEnabled  bool   // Включить/выключить еженедельное саммари
	WeeklySummaryDay      int    // День недели (0=воскресенье, 1=понедельник, ..., 6=суббота)
	WeeklySummaryHour     int    // Час отправки (0-23)
	WeeklySummaryMinute   int    // Минута отправки (0-59)
	WeeklySummaryMaxParts int    // Максимальное количество частей при разбивке длинного еженедельного саммари
	SummaryMaxParts       int    // Максимальное количество частей при разбивке длинного суточного саммари
	WeeklySummaryPrompt   string // Промпт для генерации еженедельного саммари

	// --- Конец настроек еженедельного саммари ---
	MinMessages                  int
	MaxMessages                  int
	ContextWindow                int
	ImageGenerationContextWindow int
	Debug                        bool
	// --- Настройки автоудаления сообщений об ошибках ---
	ErrorMessageAutoDeleteSeconds int // Время в секундах до автоудаления сообщений об ошибках
	// --- Конец настроек автоудаления ---
	// Настройки базы данных PostgreSQL - ИСПОЛЬЗУЕМ ПРЕФИКС POSTGRESQL_
	PostgresqlHost     string
	PostgresqlPort     string
	PostgresqlUser     string
	PostgresqlPassword string
	PostgresqlDbname   string
	// Настройки MongoDB
	MongoDbURI                    string // Строка подключения MongoDB
	MongoDbName                   string // Имя базы данных MongoDB
	MongoDbMessagesCollection     string // Имя коллекции для сообщений MongoDB
	MongoDbUserProfilesCollection string // Имя коллекции для профилей MongoDB
	MongoDbSettingsCollection     string // Имя коллекции для настроек чатов MongoDB
	// Тип хранилища ("file", "postgres" или "mongo")
	StorageType StorageType
	// Список администраторов бота (через запятую)
	AdminUsernames []string
	// Промпт для приветствия
	WelcomePrompt string
	// Промпт для приветствия при запуске бота (Startup Greeting)
	StartupGreetingPrompt string
	// Промпт для форматирования голоса
	VoiceFormatPrompt string
	// Включена ли авто-транскрипция голоса по умолчанию
	VoiceTranscriptionEnabledDefault bool
	// --- Настройки лимита прямых обращений (дефолтные) ---
	DirectReplyLimitEnabledDefault  bool
	DirectReplyLimitCountDefault    int
	DirectReplyLimitDurationDefault time.Duration // Храним сразу как Duration
	DirectReplyLimitPrompt          string
	PromptEnterDirectLimitCount     string
	PromptEnterDirectLimitDuration  string
	// --- Настройки долгосрочной памяти ---
	LongTermMemoryEnabled    bool   // Включить/выключить долгосрочную память
	GeminiEmbeddingModelName string // Модель Gemini для создания эмбеддингов
	MongoVectorIndexName     string // Имя векторного индекса в MongoDB Atlas
	LongTermMemoryFetchK     int    // Сколько релевантных сообщений извлекать
	// --- Настройки бэкфилла эмбеддингов ---
	BackfillBatchSize  int           // Размер пакета для бэкфилла
	BackfillBatchDelay time.Duration // Задержка между пакетами бэкфилла
	// --- Настройки для обработки фотографий ---
	PhotoAnalysisEnabled bool   // Включить/выключить анализ фотографий с помощью Gemini
	PhotoAnalysisPrompt  string // Промпт для анализа изображений через Gemini
	// --- Настройки автоочистки MongoDB ---
	MongoCleanupEnabled            bool // Включить/выключить автоочистку MongoDB
	MongoCleanupSizeLimitMB        int  // Максимальный размер коллекции в МБ перед очисткой
	MongoCleanupIntervalMinutes    int  // Интервал проверки коллекций в минутах
	MongoCleanupChunkDurationHours int  // Длительность удаляемого "куска" старых сообщений в часах
	// --- НОВЫЕ Настройки Auto Bio ---
	AutoBioEnabled                bool   // Включен ли автоматический анализ профилей
	AutoBioIntervalHours          int    // Интервал анализа в часах
	AutoBioInitialAnalysisPrompt  string // Промпт для первого анализа
	AutoBioUpdatePrompt           string // Промпт для обновления существующего био
	AutoBioMessagesLookbackDays   int    // На сколько дней назад смотреть сообщения при первом анализе
	AutoBioMinMessagesForAnalysis int    // Мин. кол-во сообщений пользователя для анализа
	AutoBioMaxMessagesForAnalysis int    // Макс. кол-во сообщений пользователя для анализа (для LLM)
	// --- КОНЕЦ Настроек Auto Bio ---
	// --- НОВЫЕ Настройки модерации ---
	ModEnabled          bool             `env:"MOD_ENABLED" envDefault:"false"`            // Включена ли модерация глобально
	ModInterval         int              `json:"-"`                                        // Interval for checking messages
	ModMuteTimeMin      int              `env:"MOD_MUTE_TIME_MIN" envDefault:"5"`          // Default mute time in minutes
	ModKickTimeMin      int              `env:"MOD_KICK_TIME_MIN" envDefault:"1"`          // Default kick time in minutes (0 = permanent kick/ban)
	ModBanTimeMin       int              `env:"MOD_BAN_TIME_MIN" envDefault:"60"`          // Default ban time in minutes (0 = permanent ban)
	ModPurgeDuration    time.Duration    `env:"MOD_PURGE_WINDOW_DURATION" envDefault:"1h"` // Duration for purging messages (window)
	ModPurgeDelay       time.Duration    `env:"MOD_PURGE_DELAY_DURATION" envDefault:"0s"`  // Delay before starting purge
	ModCheckAdminRights bool             `env:"MOD_CHECK_ADMIN_RIGHTS" envDefault:"true"`  // Check if bot is admin before activating moderation
	ModDefaultNotify    bool             `env:"MOD_DEFAULT_NOTIFY" envDefault:"false"`     // Default notification setting for rules without explicit value
	ModRules            []ModerationRule `env:"MOD_RULES"`                                 // Moderation rules (JSON string)
	// --- КОНЕЦ Настроек модерации ---

	// --- НОВЫЕ настройки каузального обучения (Этап 1) ---
	CausalLearningEnabled           bool    `env:"CAUSAL_LEARNING_ENABLED" envDefault:"true"`                  // Включить/выключить каузальное обучение
	CausalAnalysisIntervalHours     int     `env:"CAUSAL_ANALYSIS_INTERVAL_HOURS" envDefault:"4"`              // Интервал анализа каузальных связей
	CausalMinConfidence             float64 `env:"CAUSAL_MIN_CONFIDENCE" envDefault:"0.3"`                     // Минимальная уверенность для сохранения связи
	CausalTemporalWindowMinutes     int     `env:"CAUSAL_TEMPORAL_WINDOW_MINUTES" envDefault:"60"`             // Окно для связывания событий в минутах
	CausalMaxEntriesPerChat         int     `env:"CAUSAL_MAX_ENTRIES_PER_CHAT" envDefault:"500"`               // Максимальное количество записей на чат
	CausalAnalysisLookbackMessages  int     `env:"CAUSAL_ANALYSIS_LOOKBACK_MESSAGES" envDefault:"100"`         // Количество сообщений для анализа
	CausalAnalysisPrompt            string  `env:"CAUSAL_ANALYSIS_PROMPT"`                                     // Промпт для анализа каузальных связей
	CausalAnalysisPromptProvider    string  `env:"CAUSAL_ANALYSIS_PROMPT_PROVIDER" envDefault:"gemini"`        // Провайдер для каузального анализа
	CausalAnalysisPromptModel       string  `env:"CAUSAL_ANALYSIS_PROMPT_MODEL" envDefault:"gemini-2.5-flash"` // Модель для каузального анализа
	CausalAnalysisPromptTemperature float64 `env:"CAUSAL_ANALYSIS_PROMPT_TEMPERATURE" envDefault:"0.7"`        // Температура для каузального анализа
	CausalAnalysisPromptEnabled     bool    `env:"CAUSAL_ANALYSIS_PROMPT_ENABLED" envDefault:"true"`           // Включен ли каузальный анализ

	// Настройки промпта для извлечения влияния каузальной памяти
	CausalInfluencePrompt            string  `env:"CAUSAL_INFLUENCE_PROMPT"`                                     // Промпт для анализа влияния каузальной памяти
	CausalInfluencePromptProvider    string  `env:"CAUSAL_INFLUENCE_PROMPT_PROVIDER" envDefault:"gemini"`        // Провайдер для анализа влияния
	CausalInfluencePromptModel       string  `env:"CAUSAL_INFLUENCE_PROMPT_MODEL" envDefault:"gemini-2.0-flash"` // Модель для анализа влияния
	CausalInfluencePromptTemperature float64 `env:"CAUSAL_INFLUENCE_PROMPT_TEMPERATURE" envDefault:"0.6"`        // Температура для анализа влияния
	CausalInfluencePromptEnabled     bool    `env:"CAUSAL_INFLUENCE_PROMPT_ENABLED" envDefault:"true"`           // Включен ли анализ влияния
	// --- КОНЕЦ настроек каузального обучения ---

	// --- НОВЫЕ настройки системы убеждений (Этап 1: анализ убеждений) ---
	BeliefLearningEnabled           bool    `env:"BELIEF_LEARNING_ENABLED" envDefault:"false"`                 // Включить/выключить анализ убеждений
	BeliefAnalysisIntervalHours     int     `env:"BELIEF_ANALYSIS_INTERVAL_HOURS" envDefault:"6"`              // Интервал анализа убеждений
	BeliefAnalysisLookbackMessages  int     `env:"BELIEF_ANALYSIS_LOOKBACK_MESSAGES" envDefault:"150"`         // Кол-во сообщений для анализа
	BeliefAnalysisPrompt            string  `env:"BELIEF_ANALYSIS_PROMPT"`                                     // Промпт для анализа убеждений
	BeliefAnalysisPromptProvider    string  `env:"BELIEF_ANALYSIS_PROMPT_PROVIDER" envDefault:"gemini"`        // Провайдер
	BeliefAnalysisPromptModel       string  `env:"BELIEF_ANALYSIS_PROMPT_MODEL" envDefault:"gemini-2.0-flash"` // Модель
	BeliefAnalysisPromptTemperature float64 `env:"BELIEF_ANALYSIS_PROMPT_TEMPERATURE" envDefault:"0.6"`        // Температура
	BeliefAnalysisPromptEnabled     bool    `env:"BELIEF_ANALYSIS_PROMPT_ENABLED" envDefault:"true"`           // Включен ли промпт
	// --- КОНЕЦ настроек системы убеждений ---

	// --- НОВЫЕ настройки памяти личности ---
	PersonalityUpdateIntervalHours int    `env:"PERSONALITY_UPDATE_INTERVAL_HOURS" envDefault:"1"` // Интервал обновления памяти личности в часах
	PersonalityMessagesLookback    int    `env:"PERSONALITY_MESSAGES_LOOKBACK" envDefault:"50"`    // Количество последних сообщений для анализа
	PersonalityAnalysisPrompt      string `env:"PERSONALITY_ANALYSIS_PROMPT"`                      // Промпт для анализа контекста и обновления личности
	PersonalityNameAnalysisPrompt  string `env:"PERSONALITY_NAME_ANALYSIS_PROMPT"`                 // Промпт для анализа имён
	PersonalityTopicAnalysisPrompt string `env:"PERSONALITY_TOPIC_ANALYSIS_PROMPT"`                // Промпт для анализа тем
	PersonalitySelfUpdatePrompt    string `env:"PERSONALITY_SELF_UPDATE_PROMPT"`                   // Промпт для обновления самовосприятия
	MaxNameMentions                int    `env:"MAX_NAME_MENTIONS" envDefault:"10"`                // Максимальное количество имён в памяти
	MaxRecentTopics                int    `env:"MAX_RECENT_TOPICS" envDefault:"10"`                // Максимальное количество недавних тем
	MaxSelfPerceptions             int    `env:"MAX_SELF_PERCEPTIONS" envDefault:"5"`              // Максимальное количество элементов самовосприятия
	MaxDiscussionContexts          int    `env:"MAX_DISCUSSION_CONTEXTS" envDefault:"3"`           // Максимальное количество тем в текущем контексте
	// --- КОНЕЦ настроек памяти личности ---

	// --- НОВЫЕ настройки реакций ---
	ReactionsEnabled       bool   `env:"REACTIONS_ENABLED" envDefault:"true"`
	ClownReactionPrompt    string `env:"CLOWN_REACTION_PROMPT"`
	ReactionAnalysisPrompt string `env:"REACTION_ANALYSIS_PROMPT"`
	// Новые настройки для предотвращения бесконечного цикла
	ClownResponseProbability int `env:"CLOWN_RESPONSE_PROBABILITY" envDefault:"40"`   // Вероятность ответа на клоуна в процентах
	ClownCooldownSeconds     int `env:"CLOWN_COOLDOWN_SECONDS" envDefault:"30"`       // Cooldown между ответами от одного пользователя
	MaxClownResponsesPerHour int `env:"MAX_CLOWN_RESPONSES_PER_HOUR" envDefault:"10"` // Максимум ответов на клоуна в час
	// --- КОНЕЦ настроек реакций ---

	// --- НОВЫЕ настройки веб-поиска ---
	WebSearchEnabled       bool   `env:"WEB_SEARCH_ENABLED" envDefault:"true"`
	GoogleSearchAPIKey     string `env:"GOOGLE_SEARCH_API_KEY"`
	GoogleSearchEngineID   string `env:"GOOGLE_SEARCH_ENGINE_ID"`
	WebSearchMaxResults    int    `env:"WEB_SEARCH_MAX_RESULTS" envDefault:"3"`
	WebSearchTriggerPrompt string `env:"WEB_SEARCH_TRIGGER_PROMPT"`
	ImageGenPrePrompt      string `env:"IMAGE_GEN_PRE_PROMPT"`
	FreeWillImageGenPrompt string `env:"FREE_WILL_IMAGEGEN_PROMPT"`
	ImageGenFrequencyHours int    `env:"IMAGE_GEN_FREQUENCY_HOURS" envDefault:"12"`
	// --- Настройки кэширования веб-поиска ---
	WebSearchCacheTTL     time.Duration `env:"WEB_SEARCH_CACHE_TTL" envDefault:"5m"`       // Время жизни кэша результатов поиска
	WebSearchCacheMaxSize int           `env:"WEB_SEARCH_CACHE_MAX_SIZE" envDefault:"100"` // Максимальное количество записей в кэше
	// --- КОНЕЦ настроек веб-поиска ---

	// --- НОВЫЕ настройки ElevenLabs ---
	ElevenLabsAPIKey  string `env:"ELEVENLABS_API_KEY"`                                    // API ключ ElevenLabs
	ElevenLabsVoiceID string `env:"ELEVENLABS_VOICE_ID" envDefault:"Obuyk6KKzg9olSLPaCbl"` // Voice ID (Arcadias по умолчанию)
	ElevenLabsModel   string `env:"ELEVENLABS_MODEL" envDefault:"eleven_multilingual_v2"`  // Модель TTS
	ElevenLabsPlan    string `env:"ELEVENLABS_PLAN" envDefault:"starter"`                  // Тарифный план

	// --- Расширенные настройки голоса ElevenLabs ---
	ElevenLabsStability       float64 `env:"ELEVENLABS_STABILITY" envDefault:"0.5"`          // Стабильность голоса (0.0-1.0)
	ElevenLabsSimilarityBoost float64 `env:"ELEVENLABS_SIMILARITY_BOOST" envDefault:"0.8"`   // Усиление сходства (0.0-1.0)
	ElevenLabsStyle           float64 `env:"ELEVENLABS_STYLE" envDefault:"0.0"`              // Стиль речи (0.0-1.0)
	ElevenLabsUseSpeakerBoost bool    `env:"ELEVENLABS_USE_SPEAKER_BOOST" envDefault:"true"` // Улучшение голоса спикера
	ElevenLabsSpeed           float64 `env:"ELEVENLABS_SPEED" envDefault:"1.0"`              // Скорость речи (0.7-1.2)

	// --- Промпт-настройки для управления стилем ---
	ElevenLabsStylePrompt   string `env:"ELEVENLABS_STYLE_PROMPT"`   // Базовый промпт стиля
	ElevenLabsEmotionPrompt string `env:"ELEVENLABS_EMOTION_PROMPT"` // Промпт эмоциональной окраски
	ElevenLabsPacePrompt    string `env:"ELEVENLABS_PACE_PROMPT"`    // Промпт темпа речи

	// --- Дополнительные голоса ---
	ElevenLabsRandomVoice bool `env:"ELEVENLABS_RANDOM_VOICE" envDefault:"false"` // Случайный выбор голоса

	// --- Настройки голосовых сообщений ---
	VoiceMessagesEnabled bool   `env:"VOICE_MESSAGES_ENABLED" envDefault:"true"`                // Включить/выключить старые интервальные голосовые сообщения
	MinVoiceMessages     int    `env:"MIN_VOICE_MESSAGES" envDefault:"50"`                      // Минимальный интервал голосовых сообщений
	MaxVoiceMessages     int    `env:"MAX_VOICE_MESSAGES" envDefault:"100"`                     // Максимальный интервал голосовых сообщений
	VoiceMessageTempDir  string `env:"VOICE_MESSAGE_TEMP_DIR" envDefault:"/tmp/voice_messages"` // Директория для временных файлов
	VoiceMessagesPrompt  string // Промпт для генерации голосовых сообщений (из VOICE_MESSAGE_PROMPT)

	// --- КОНЕЦ настроек ElevenLabs ---

	// --- НОВЫЕ настройки Free Will ---
	FreeWillEnabled               bool    `env:"FREE_WILL_ENABLED" envDefault:"false"`
	FreeWillMinIntervalMinutes    float64 `env:"FREE_WILL_MIN_INTERVAL_MINUTES" envDefault:"0.5"`
	FreeWillMaxIntervalMinutes    float64 `env:"FREE_WILL_MAX_INTERVAL_MINUTES" envDefault:"5"` // Изменено на дробные значения
	FreeWillContextWindow         int     `env:"FREE_WILL_CONTEXT_WINDOW" envDefault:"50"`
	FreeWillMoodUpdateProbability float64 `env:"FREE_WILL_MOOD_UPDATE_PROBABILITY" envDefault:"0.1"`
	FreeWillMaxDecisionsPerHour   int     `env:"FREE_WILL_MAX_DECISIONS_PER_HOUR" envDefault:"10"`
	FreeWillVoiceProbability      float64 `env:"FREE_WILL_VOICE_PROBABILITY" envDefault:"0.3"`

	// Настройки реакции на тишину
	FreeWillSilenceMinMinutes float64 `env:"FREE_WILL_SILENCE_MIN_MINUTES" envDefault:"0.5"` // Минимальное время тишины для реакции
	FreeWillSilenceMaxMinutes float64 `env:"FREE_WILL_SILENCE_MAX_MINUTES" envDefault:"3"`   // Максимальное время тишины для реакции

	// Промпты Free Will
	FreeWillShouldReplyPrompt  string `env:"FREE_WILL_SHOULD_REPLY_PROMPT"`  // ЭТАП 1: решение о необходимости ответа
	FreeWillResponseTypePrompt string `env:"FREE_WILL_RESPONSE_TYPE_PROMPT"` // ЭТАП 2: определение типа ответа
	FreeWillDirectPrompt       string `env:"FREE_WILL_DIRECT_PROMPT"`
	FreeWillGeneralPrompt      string `env:"FREE_WILL_GENERAL_PROMPT"`
	FreeWillContextPrompt      string `env:"FREE_WILL_CONTEXT_PROMPT"`
	FreeWillSilencePrompt      string `env:"FREE_WILL_SILENCE_PROMPT"`
	FreeWillMoodAnalysisPrompt string `env:"FREE_WILL_MOOD_ANALYSIS_PROMPT"`

	// Контроль старого интервального механизма
	IntervalMessagesEnabled bool `env:"INTERVAL_MESSAGES_ENABLED" envDefault:"true"`

	// Промпт для ответа на тейки (интегрированный в Free Will)
	FreeWillTakeResponsePrompt string `env:"FREE_WILL_TAKE_RESPONSE_PROMPT"`

	// --- НОВЫЕ настройки Free Will Direct Response ---
	BotNames                             []string `env:"BOT_NAMES"`                                 // Список имен бота для обращений
	FreeWillDirectResponseDecisionPrompt string   `env:"FREE_WILL_DIRECT_RESPONSE_DECISION_PROMPT"` // Промпт для принятия решения (Этап 1)
	FreeWillDirectResponsePrompt         string   `env:"FREE_WILL_DIRECT_RESPONSE_PROMPT"`          // Промпт для генерации ответа (Этап 2)

	// --- НОВЫЕ настройки лимитов для прямых обращений ---
	FreeWillDirectResponseMaxPerHour         int     `env:"FREE_WILL_DIRECT_RESPONSE_MAX_PER_HOUR" envDefault:"30"`         // Максимальное количество прямых ответов в час (отдельно от общих лимитов)
	FreeWillDirectResponseMinIntervalSeconds float64 `env:"FREE_WILL_DIRECT_RESPONSE_MIN_INTERVAL_SECONDS" envDefault:"5"`  // Минимальный интервал между прямыми ответами в секундах
	FreeWillDirectResponseIndependentLimits  bool    `env:"FREE_WILL_DIRECT_RESPONSE_INDEPENDENT_LIMITS" envDefault:"true"` // Независимые лимиты для прямых обращений (не учитываются в общих лимитах)
	// --- КОНЕЦ настроек лимитов для прямых обращений ---

	// --- НОВЫЕ настройки лимитов для генерации изображений ---
	FreeWillImageGenerationMaxDecisionsPerInterval    int  `env:"FREE_WILL_IMAGE_GENERATION_MAX_DECISIONS_PER_INTERVAL" envDefault:"3"`     // Максимальное количество попыток принятия решения за интервал
	FreeWillImageGenerationIntervalHours              int  `env:"FREE_WILL_IMAGE_GENERATION_INTERVAL_HOURS" envDefault:"6"`                 // Интервал в часах для лимита изображений
	FreeWillImageGenerationMinDecisionIntervalMinutes int  `env:"FREE_WILL_IMAGE_GENERATION_MIN_DECISION_INTERVAL_MINUTES" envDefault:"30"` // Минимальный интервал между решениями в минутах
	FreeWillImageGenerationIndependentLimits          bool `env:"FREE_WILL_IMAGE_GENERATION_INDEPENDENT_LIMITS" envDefault:"true"`          // Независимые лимиты для изображений
	// --- КОНЕЦ настроек лимитов для генерации изображений ---
	// --- КОНЕЦ настроек Free Will Direct Response ---

	// --- НОВЫЕ настройки реакций ---
	FreeWillReactionsEnabled         bool    `env:"FREE_WILL_REACTIONS_ENABLED" envDefault:"true"`
	FreeWillReactionsProbability     float64 `env:"FREE_WILL_REACTIONS_PROBABILITY" envDefault:"0.2"`
	FreeWillReactionsCooldownMinutes int     `env:"FREE_WILL_REACTIONS_COOLDOWN_MINUTES" envDefault:"5"`
	FreeWillReactionsMaxPerHour      int     `env:"FREE_WILL_REACTIONS_MAX_PER_HOUR" envDefault:"15"`
	FreeWillReactionPrompt           string  `env:"FREE_WILL_REACTION_PROMPT"`
	// --- КОНЕЦ настроек Free Will ---

	// --- НОВЫЕ настройки системы анти-повторений ---
	AntiRepetitionEnabled              bool    `env:"ANTI_REPETITION_ENABLED" envDefault:"true"`
	AntiRepetitionMaxResponsesPerChat  int     `env:"ANTI_REPETITION_MAX_RESPONSES_PER_CHAT" envDefault:"20"`
	AntiRepetitionSimilarityThreshold  float64 `env:"ANTI_REPETITION_SIMILARITY_THRESHOLD" envDefault:"0.75"`
	AntiRepetitionTimeWindowHours      int     `env:"ANTI_REPETITION_TIME_WINDOW_HOURS" envDefault:"24"`
	AntiRepetitionCleanupIntervalHours int     `env:"ANTI_REPETITION_CLEANUP_INTERVAL_HOURS" envDefault:"1"`

	// --- НОВАЯ настройка: Сервис дисамбигуации пользователей ---
	DisambiguationEnabled bool `env:"DISAMBIGUATION_ENABLED" envDefault:"true"` // Включить/выключить систему дисамбигуации

	// --- Диагностический флаг: отключение профилей пользователей ---
	DisableUserProfiles bool `env:"DISABLE_USER_PROFILES" envDefault:"false"` // Отключить загрузку профилей пользователей

	// --- НОВЫЕ настройки переработки повторений ---
	AntiRepetitionReworkEnabled        bool    `env:"ANTI_REPETITION_REWORK_ENABLED" envDefault:"true"`
	AntiRepetitionMaxReworkAttempts    int     `env:"ANTI_REPETITION_MAX_REWORK_ATTEMPTS" envDefault:"2"`
	AntiRepetitionReworkTemperature    float64 `env:"ANTI_REPETITION_REWORK_TEMPERATURE" envDefault:"0.8"`
	AntiRepetitionReworkPrompt         string  `env:"ANTI_REPETITION_REWORK_PROMPT"`
	AntiRepetitionLocalReworkEnabled   bool    `env:"ANTI_REPETITION_LOCAL_REWORK_ENABLED" envDefault:"true"`
	AntiRepetitionLocalReworkMaxLength int     `env:"ANTI_REPETITION_LOCAL_REWORK_MAX_LENGTH" envDefault:"50"`
	// --- КОНЕЦ настроек системы анти-повторений ---



	// --- НОВЫЕ настройки эмоциональной системы (Этап 2) ---
	EmotionalLearningEnabled          bool `env:"EMOTIONAL_LEARNING_ENABLED" envDefault:"true"`
	EmotionalAnalysisIntervalHours    int  `env:"EMOTIONAL_ANALYSIS_INTERVAL_HOURS" envDefault:"2"`
	EmotionalAnalysisLookbackMessages int  `env:"EMOTIONAL_ANALYSIS_LOOKBACK_MESSAGES" envDefault:"100"`
	EmotionalMemoryRetentionDays      int  `env:"EMOTIONAL_MEMORY_RETENTION_DAYS" envDefault:"30"`
	EmotionalMinMessagesForAnalysis   int  `env:"EMOTIONAL_MIN_MESSAGES_FOR_ANALYSIS" envDefault:"20"`
	EmotionalAnalysisDebounceHours    int  `env:"EMOTIONAL_ANALYSIS_DEBOUNCE_HOURS" envDefault:"6"`

	EmotionalAnalysisPrompt            string  `env:"EMOTIONAL_ANALYSIS_PROMPT"`
	EmotionalAnalysisPromptProvider    string  `env:"EMOTIONAL_ANALYSIS_PROMPT_PROVIDER" envDefault:"gemini"`
	EmotionalAnalysisPromptModel       string  `env:"EMOTIONAL_ANALYSIS_PROMPT_MODEL" envDefault:"gemini-2.0-flash"`
	EmotionalAnalysisPromptTemperature float64 `env:"EMOTIONAL_ANALYSIS_PROMPT_TEMPERATURE" envDefault:"0.8"`
	EmotionalAnalysisPromptEnabled     bool    `env:"EMOTIONAL_ANALYSIS_PROMPT_ENABLED" envDefault:"true"`

	EmotionalAdaptationPrompt            string  `env:"EMOTIONAL_ADAPTATION_PROMPT"`
	EmotionalAdaptationPromptProvider    string  `env:"EMOTIONAL_ADAPTATION_PROMPT_PROVIDER" envDefault:"gemini"`
	EmotionalAdaptationPromptModel       string  `env:"EMOTIONAL_ADAPTATION_PROMPT_MODEL" envDefault:"gemini-2.0-flash"`
	EmotionalAdaptationPromptTemperature float64 `env:"EMOTIONAL_ADAPTATION_PROMPT_TEMPERATURE" envDefault:"0.7"`
	EmotionalAdaptationPromptEnabled     bool    `env:"EMOTIONAL_ADAPTATION_PROMPT_ENABLED" envDefault:"true"`

	EmotionalFeedbackPrompt            string  `env:"EMOTIONAL_FEEDBACK_PROMPT"`
	EmotionalFeedbackPromptProvider    string  `env:"EMOTIONAL_FEEDBACK_PROMPT_PROVIDER" envDefault:"gemini"`
	EmotionalFeedbackPromptModel       string  `env:"EMOTIONAL_FEEDBACK_PROMPT_MODEL" envDefault:"gemini-2.0-flash"`
	EmotionalFeedbackPromptTemperature float64 `env:"EMOTIONAL_FEEDBACK_PROMPT_TEMPERATURE" envDefault:"0.6"`
	EmotionalFeedbackPromptEnabled     bool    `env:"EMOTIONAL_FEEDBACK_PROMPT_ENABLED" envDefault:"true"`
	// --- КОНЕЦ настроек эмоциональной системы ---

	// --- Конфигурации моделей для разных типов ответов ---
	ResponseTypeConfigs map[string]ResponseTypeConfig `json:"response_type_configs"` // Конфигурации модели для каждого типа ответа
	// --- КОНЕЦ конфигураций моделей ---

	// --- Новые поля для настроек администратора ---
	AdminID int64 `env:"ADMIN_ID"`

	// --- Настройки обхода блокировок Gemini ---
	GeminiBypassSafetyFilters bool `env:"GEMINI_BYPASS_SAFETY_FILTERS" envDefault:"true"` // Отключать ли фильтры безопасности
	GeminiObfuscatePrompts    bool `env:"GEMINI_OBFUSCATE_PROMPTS" envDefault:"false"`    // Применять ли обфускацию промптов

	// === НОВАЯ ОПЦИЯ СТРУКТУРИРОВАННОГО ФОРМАТИРОВАНИЯ ===
	// Включает новый формат сообщений с тегами [MSG_START]/[MSG_END]
	// для лучшего понимания LLM структуры сообщений и метаданных
	UseStructuredMessageFormat bool
	// Association Cloud (Associative Memory Graph)
	AssociationCloudEnabled   bool `env:"ASSOCIATION_CLOUD_ENABLED" envDefault:"false"`   // Включить/выключить облако ассоциаций
	AssociationCloudMaxNodes  int  `env:"ASSOCIATION_CLOUD_MAX_NODES" envDefault:"5000"`  // Лимит узлов на чат
	AssociationCloudMaxEdges  int  `env:"ASSOCIATION_CLOUD_MAX_EDGES" envDefault:"50000"` // Лимит рёбер на чат
	AssociationCloudDecayDays int  `env:"ASSOCIATION_CLOUD_DECAY_DAYS" envDefault:"30"`   // Срок свежести для выборок

	// === КОГНИТИВНАЯ АРХИТЕКТУРА (ЭТАП 3) ===
	InternalMonologueEnabled        bool    `env:"INTERNAL_MONOLOGUE_ENABLED" envDefault:"false"`
	SelfReflectionEnabled           bool    `env:"SELF_REFLECTION_ENABLED" envDefault:"false"`
	ConfidenceCalibrationEnabled    bool    `env:"CONFIDENCE_CALIBRATION_ENABLED" envDefault:"false"`
	InternalMonologuePrompt         string  `env:"INTERNAL_MONOLOGUE_PROMPT"`
	InternalMonologuePromptModel    string  `env:"INTERNAL_MONOLOGUE_PROMPT_MODEL" envDefault:"gemini-2.0-flash"`
	InternalMonologuePromptProvider string  `env:"INTERNAL_MONOLOGUE_PROMPT_PROVIDER" envDefault:"gemini"`
	InternalMonologuePromptEnabled  bool    `env:"INTERNAL_MONOLOGUE_PROMPT_ENABLED" envDefault:"true"`
	InternalMonologueTemperature    float64 `env:"INTERNAL_MONOLOGUE_TEMPERATURE" envDefault:"0.4"`

	// === САМОРЕФЛЕКСИЯ ===
	SelfReflectionPrompt         string  `env:"SELF_REFLECTION_PROMPT"`
	SelfReflectionPromptModel    string  `env:"SELF_REFLECTION_PROMPT_MODEL" envDefault:"gemini-2.0-flash"`
	SelfReflectionPromptProvider string  `env:"SELF_REFLECTION_PROMPT_PROVIDER" envDefault:"gemini"`
	SelfReflectionPromptEnabled  bool    `env:"SELF_REFLECTION_PROMPT_ENABLED" envDefault:"true"`
	SelfReflectionTemperature    float64 `env:"SELF_REFLECTION_TEMPERATURE" envDefault:"0.5"`

	// === СОЦИАЛЬНАЯ АРХИТЕКТУРА (ЭТАП 4) ===
	RelationshipTrackingEnabled  bool    `env:"RELATIONSHIP_TRACKING_ENABLED" envDefault:"false"`
	SocialLearningEnabled        bool    `env:"SOCIAL_LEARNING_ENABLED" envDefault:"false"`
	RelationshipAnalysisPrompt   string  `env:"RELATIONSHIP_ANALYSIS_PROMPT"`
	RelationshipAnalysisEnabled  bool    `env:"RELATIONSHIP_ANALYSIS_PROMPT_ENABLED" envDefault:"true"`
	RelationshipAnalysisModel    string  `env:"RELATIONSHIP_ANALYSIS_PROMPT_MODEL" envDefault:"gemini-2.0-flash"`
	RelationshipAnalysisProvider string  `env:"RELATIONSHIP_ANALYSIS_PROMPT_PROVIDER" envDefault:"gemini"`
	RelationshipAnalysisTemp     float64 `env:"RELATIONSHIP_ANALYSIS_PROMPT_TEMPERATURE" envDefault:"0.6"`
	IntimacyGrowthRate           float64 `env:"INTIMACY_GROWTH_RATE" envDefault:"0.02"`
	TrustDecayRate               float64 `env:"TRUST_DECAY_RATE" envDefault:"0.01"`

	// --- Настройки отказоустойчивости LLM ---
	LLMFallbackEnabled       bool     `env:"LLM_FALLBACK_ENABLED" envDefault:"true"`                                        // Включить фолбэк-провайдера на критичных путях
	LLMFallbackCriticalTypes []string `env:"LLM_FALLBACK_CRITICAL_TYPES" envDefault:"free_will_silence,free_will_reaction"` // Типы ответов, где разрешен фолбэк
	LLMFallbackProviderOrder []string `env:"LLM_FALLBACK_PROVIDER_ORDER" envDefault:"gemini,deepseek,openrouter"`           // Порядок провайдеров для фолбэка
}

// IsAdmin проверяет, является ли пользователь администратором бота
func (c *Config) IsAdmin(userID int64) bool {
	return userID == c.AdminID
}
