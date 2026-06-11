package config

import "time"

// ConfigV2 — корневая структура конфигурации (новая версия).
// Загружается из luna_bot.yaml с переопределением через env LUNA_*.
//
// Существует параллельно с устаревшим Config из types.go.
// Будет использоваться после завершения миграции (CFG-06).
type ConfigV2 struct {
	LLM                   LLMConfig                  `yaml:"llm"`
	TTS                   TTSConfig                  `yaml:"tts"`
	Telegram              TelegramConfig             `yaml:"telegram"`
	Chat                  ChatConfig                 `yaml:"chat"`
	Summary               SummaryConfig              `yaml:"summary"`
	RateLimit             RateLimitConfig            `yaml:"rate_limit"`
	SettingsPrompts       SettingsPromptsConfig      `yaml:"settings_prompts"`
	Srach                 SrachConfig                `yaml:"srach"`
	DirectReplyLimits     DirectReplyLimitsConfig    `yaml:"direct_reply_limits"`
	Donate                DonateConfig               `yaml:"donate"`
	FreeWill              FreeWillConfig             `yaml:"free_will"`
	VoiceMessages         VoiceMessagesConfig        `yaml:"voice_messages"`
	Moderation            ModerationConfig           `yaml:"moderation"`
	AntiRepetition        AntiRepetitionConfig       `yaml:"anti_repetition"`
	Disambiguation        DisambiguationConfig       `yaml:"disambiguation"`
	MessagePostProcessor  MessagePostProcessorConfig `yaml:"message_post_processor"`
	AutoBio               AutoBioConfig              `yaml:"auto_bio"`
	Personality           PersonalityConfig          `yaml:"personality"`
	Reactions             ReactionsConfig            `yaml:"reactions"`
	WebSearch             WebSearchConfig            `yaml:"web_search"`
	CausalLearning        CausalLearningConfig       `yaml:"causal_learning"`
	EmotionalLearning     EmotionalLearningConfig    `yaml:"emotional_learning"`
	BeliefLearning        BeliefLearningConfig       `yaml:"belief_learning"`
	CognitiveArchitecture CognitiveArchConfig        `yaml:"cognitive_architecture"`
	SocialArchitecture    SocialArchConfig           `yaml:"social_architecture"`
	AssociationCloud      AssociationCloudConfig     `yaml:"association_cloud"`
	Storage               StorageConfig              `yaml:"storage"`
	Embedding             EmbeddingConfig            `yaml:"embedding"`
	Prompts               PromptsConfig              `yaml:"prompts"`
}

// ============================================================================
// LLM Config
// ============================================================================

// LLMConfig — корневая секция LLM.
type LLMConfig struct {
	DefaultProvider        string                    `yaml:"default_provider"`
	FallbackEnabled        bool                      `yaml:"fallback_enabled"`
	FallbackCriticalTypes  []string                  `yaml:"fallback_critical_types"`
	FallbackProviderOrder  []string                  `yaml:"fallback_provider_order"`
	CircuitBreaker         CircuitBreakerConfig      `yaml:"circuit_breaker"`
	Providers              map[string]ProviderConfig `yaml:"providers"`
	ResponseTypes          map[string]RoutingProfile `yaml:"response_types"`
}

// CircuitBreakerConfig — настройки circuit breaker.
type CircuitBreakerConfig struct {
	MaxFailures     int `yaml:"max_failures"`
	CooldownSeconds int `yaml:"cooldown_seconds"`
}

// ProviderConfig — конфигурация одного LLM-провайдера.
type ProviderConfig struct {
	Enabled          bool               `yaml:"enabled"`
	APIKey           string             `yaml:"api_key"`
	ReserveAPIKey    string             `yaml:"reserve_api_key"`
	KeyRotationHours int                `yaml:"key_rotation_hours"`
	BaseURL          string             `yaml:"base_url"`
	SiteURL          string             `yaml:"site_url"`
	SiteTitle        string             `yaml:"site_title"`
	Debug            bool               `yaml:"debug"`
	Safety           SafetyConfig       `yaml:"safety"`
	Models           map[string]string  `yaml:"models"`
	Temperatures     map[string]float64 `yaml:"temperatures"`
	Extra            map[string]any     `yaml:"extra"`
}

// SafetyConfig — настройки безопасности (Gemini-specific).
type SafetyConfig struct {
	BypassFilters bool `yaml:"bypass_filters"`
	Obfuscate     bool `yaml:"obfuscate"`
}

// RoutingProfile — профиль маршрутизации для ResponseType.
type RoutingProfile struct {
	Provider         string  `yaml:"provider"`
	Model            string  `yaml:"model"`
	Temperature      float64 `yaml:"temperature"`
	FallbackProvider string  `yaml:"fallback_provider"`
	Priority         int     `yaml:"priority"`
}

// ============================================================================
// TTS Config
// ============================================================================

// TTSConfig — настройки Text-to-Speech.
type TTSConfig struct {
	Provider   string           `yaml:"provider"`
	Fallback   string           `yaml:"fallback"`
	ElevenLabs ElevenLabsConfig `yaml:"elevenlabs"`
	GeminiTTS  GeminiTTSConfig  `yaml:"gemini_tts"`
}

// ElevenLabsConfig — настройки ElevenLabs.
type ElevenLabsConfig struct {
	APIKey        string              `yaml:"api_key"`
	VoiceID       string              `yaml:"voice_id"`
	Model         string              `yaml:"model"`
	Plan          string              `yaml:"plan"`
	VoiceSettings VoiceSettingsConfig `yaml:"voice_settings"`
	Prompts       VoicePromptsConfig  `yaml:"prompts"`
	RandomVoice   bool                `yaml:"random_voice"`
}

// VoiceSettingsConfig — настройки голоса ElevenLabs.
type VoiceSettingsConfig struct {
	Stability       float64 `yaml:"stability"`
	SimilarityBoost float64 `yaml:"similarity_boost"`
	Style           float64 `yaml:"style"`
	UseSpeakerBoost bool    `yaml:"use_speaker_boost"`
	Speed           float64 `yaml:"speed"`
}

// VoicePromptsConfig — промпты для ElevenLabs.
type VoicePromptsConfig struct {
	Style   string `yaml:"style"`
	Emotion string `yaml:"emotion"`
	Pace    string `yaml:"pace"`
}

// GeminiTTSConfig — настройки Gemini TTS.
type GeminiTTSConfig struct {
	Model     string `yaml:"model"`
	VoiceName string `yaml:"voice_name"`
}

// ============================================================================
// Telegram Config
// ============================================================================

// TelegramConfig — настройки Telegram.
type TelegramConfig struct {
	Token                      string   `yaml:"token"`
	BotUserID                  int64    `yaml:"bot_user_id"`
	BotNames                   []string `yaml:"bot_names"`
	AdminIDs                   []int64  `yaml:"admin_ids"`
	AdminUsernames             []string `yaml:"admin_usernames"`
	Debug                      bool     `yaml:"debug"`
	Timezone                   string   `yaml:"timezone"`
	ErrorAutoDeleteSeconds     int      `yaml:"error_message_auto_delete_seconds"`
	UseStructuredMessageFormat bool     `yaml:"use_structured_message_format"`
}

// ============================================================================
// Free Will Config
// ============================================================================

// FreeWillConfig — настройки Free Will.
type FreeWillConfig struct {
	Enabled               bool                    `yaml:"enabled"`
	Intervals             FreeWillIntervalsConfig `yaml:"intervals"`
	ContextWindow         int                     `yaml:"context_window"`
	MoodUpdateProbability float64                 `yaml:"mood_update_probability"`
	MaxDecisionsPerHour   int                     `yaml:"max_decisions_per_hour"`
	VoiceProbability      float64                 `yaml:"voice_probability"`
	Silence               SilenceConfig           `yaml:"silence"`
	Reactions             FreeWillReactionsConfig `yaml:"reactions"`
	DirectResponse        DirectResponseConfig    `yaml:"direct_response"`
	ImageGeneration       ImageGenLimitsConfig    `yaml:"image_generation"`
	IntervalMessages      IntervalMessagesConfig  `yaml:"interval_messages"`
}

// FreeWillIntervalsConfig — интервалы Free Will.
type FreeWillIntervalsConfig struct {
	MinMinutes float64 `yaml:"min_minutes"`
	MaxMinutes float64 `yaml:"max_minutes"`
}

// SilenceConfig — настройки реакции на тишину.
type SilenceConfig struct {
	MinMinutes float64 `yaml:"min_minutes"`
	MaxMinutes float64 `yaml:"max_minutes"`
}

// FreeWillReactionsConfig — настройки реакций Free Will.
type FreeWillReactionsConfig struct {
	Enabled         bool    `yaml:"enabled"`
	Probability     float64 `yaml:"probability"`
	CooldownMinutes int     `yaml:"cooldown_minutes"`
	MaxPerHour      int     `yaml:"max_per_hour"`
}

// DirectResponseConfig — настройки прямых ответов.
type DirectResponseConfig struct {
	MaxPerHour         int     `yaml:"max_per_hour"`
	MinIntervalSeconds float64 `yaml:"min_interval_seconds"`
	IndependentLimits  bool    `yaml:"independent_limits"`
}

// ImageGenLimitsConfig — лимиты генерации изображений.
type ImageGenLimitsConfig struct {
	MaxPerInterval         int  `yaml:"max_per_interval"`
	IntervalHours          int  `yaml:"interval_hours"`
	MinDecisionIntervalMin int  `yaml:"min_decision_interval_minutes"`
	IndependentLimits      bool `yaml:"independent_limits"`
	FrequencyHours         int  `yaml:"frequency_hours"`
}

// IntervalMessagesConfig — настройки интервальных сообщений.
type IntervalMessagesConfig struct {
	Enabled bool `yaml:"enabled"`
}

// ============================================================================
// Voice Messages Config
// ============================================================================

// VoiceMessagesConfig — настройки голосовых сообщений.
type VoiceMessagesConfig struct {
	Enabled  bool           `yaml:"enabled"`
	Interval IntervalConfig `yaml:"interval"`
	TempDir  string         `yaml:"temp_dir"`
}

// IntervalConfig — min/max интервал.
type IntervalConfig struct {
	Min int `yaml:"min"`
	Max int `yaml:"max"`
}

// ============================================================================
// Moderation Config
// ============================================================================

// ModerationConfig — настройки модерации.
type ModerationConfig struct {
	Enabled          bool          `yaml:"enabled"`
	IntervalMinutes  int           `yaml:"interval_minutes"`
	MuteTimeMinutes  int           `yaml:"mute_time_minutes"`
	KickTimeMinutes  int           `yaml:"kick_time_minutes"`
	BanTimeMinutes   int           `yaml:"ban_time_minutes"`
	PurgeWindow      time.Duration `yaml:"purge_window_duration"`
	PurgeDelay       time.Duration `yaml:"purge_delay_duration"`
	CheckAdminRights bool          `yaml:"check_admin_rights"`
	DefaultNotify    bool          `yaml:"default_notify"`
	Rules            []string      `yaml:"rules"`
}

// ============================================================================
// Anti-Repetition Config
// ============================================================================

// AntiRepetitionConfig — настройки анти-повторений.
type AntiRepetitionConfig struct {
	Enabled              bool                    `yaml:"enabled"`
	MaxResponsesPerChat  int                     `yaml:"max_responses_per_chat"`
	SimilarityThreshold  float64                 `yaml:"similarity_threshold"`
	TimeWindowHours      int                     `yaml:"time_window_hours"`
	CleanupIntervalHours int                     `yaml:"cleanup_interval_hours"`
	Rework               AntiRepetitionReworkCfg `yaml:"rework"`
}

// AntiRepetitionReworkCfg — настройки переработки повторений.
type AntiRepetitionReworkCfg struct {
	Enabled     bool              `yaml:"enabled"`
	MaxAttempts int               `yaml:"max_attempts"`
	Temperature float64           `yaml:"temperature"`
	LocalRework LocalReworkConfig `yaml:"local_rework"`
}

// LocalReworkConfig — локальная переработка.
type LocalReworkConfig struct {
	Enabled   bool `yaml:"enabled"`
	MaxLength int  `yaml:"max_length"`
}

// ============================================================================
// Disambiguation Config
// ============================================================================

// DisambiguationConfig — настройки дисамбигуации пользователей.
type DisambiguationConfig struct {
	Enabled bool `yaml:"enabled"`
}

// ============================================================================
// Message Post-Processor Config
// ============================================================================

// MessagePostProcessorConfig — настройки постобработки сообщений.
type MessagePostProcessorConfig struct {
	Enabled              bool                          `yaml:"enabled"`
	RandomizationEnabled bool                          `yaml:"randomization_enabled"`
	Probabilities        PostProcessorProbabilitiesCfg `yaml:"probabilities"`
	Length               PostProcessorLengthCfg        `yaml:"length"`
	Performance          PostProcessorPerfCfg          `yaml:"performance"`
	Cache                PostProcessorCacheCfg         `yaml:"cache"`
	ExcludeTypes         []string                      `yaml:"exclude_types"`
	WeeklySummaryExclude bool                          `yaml:"weekly_summary_exclude"`
	Debug                PostProcessorDebugCfg         `yaml:"debug"`
}

// PostProcessorProbabilitiesCfg — вероятности типов постобработки.
type PostProcessorProbabilitiesCfg struct {
	SingleWord     float64 `yaml:"single_word"`
	ShortSentences float64 `yaml:"short_sentences"`
	LongMessages   float64 `yaml:"long_messages"`
}

// PostProcessorLengthCfg — настройки длины.
type PostProcessorLengthCfg struct {
	Min                          int `yaml:"min"`
	Max                          int `yaml:"max"`
	LongMessageThreshold         int `yaml:"long_message_threshold"`
	ForceLongProcessingThreshold int `yaml:"force_long_processing_threshold"`
}

// PostProcessorPerfCfg — настройки производительности.
type PostProcessorPerfCfg struct {
	TimeoutSeconds int     `yaml:"timeout_seconds"`
	Temperature    float64 `yaml:"temperature"`
}

// PostProcessorCacheCfg — настройки кэша.
type PostProcessorCacheCfg struct {
	Enabled          bool `yaml:"enabled"`
	TTLMinutes       int  `yaml:"ttl_minutes"`
	ReplacementCache struct {
		Enabled    bool `yaml:"enabled"`
		TTLMinutes int  `yaml:"ttl_minutes"`
	} `yaml:"replacement_cache"`
}

// PostProcessorDebugCfg — настройки отладки.
type PostProcessorDebugCfg struct {
	Logging             bool `yaml:"logging"`
	LogOriginalMessages bool `yaml:"log_original_messages"`
}

// ============================================================================
// Auto Bio Config
// ============================================================================

// AutoBioConfig — настройки автоматического анализа профилей.
type AutoBioConfig struct {
	Enabled                bool `yaml:"enabled"`
	IntervalHours          int  `yaml:"interval_hours"`
	LookbackDays           int  `yaml:"lookback_days"`
	MinMessagesForAnalysis int  `yaml:"min_messages_for_analysis"`
	MaxMessagesForAnalysis int  `yaml:"max_messages_for_analysis"`
}

// ============================================================================
// Personality Config
// ============================================================================

// PersonalityConfig — настройки памяти личности.
type PersonalityConfig struct {
	UpdateIntervalHours   int `yaml:"update_interval_hours"`
	MessagesLookback      int `yaml:"messages_lookback"`
	MaxNameMentions       int `yaml:"max_name_mentions"`
	MaxRecentTopics       int `yaml:"max_recent_topics"`
	MaxSelfPerceptions    int `yaml:"max_self_perceptions"`
	MaxDiscussionContexts int `yaml:"max_discussion_contexts"`
}

// ============================================================================
// Reactions Config
// ============================================================================

// ReactionsConfig — настройки реакций.
type ReactionsConfig struct {
	Enabled bool        `yaml:"enabled"`
	Clown   ClownConfig `yaml:"clown"`
}

// ClownConfig — настройки реакций на клоунов.
type ClownConfig struct {
	ResponseProbability int `yaml:"response_probability"`
	CooldownSeconds     int `yaml:"cooldown_seconds"`
	MaxResponsesPerHour int `yaml:"max_responses_per_hour"`
}

// ============================================================================
// Web Search Config
// ============================================================================

// WebSearchConfig — настройки веб-поиска.
type WebSearchConfig struct {
	Enabled        bool              `yaml:"enabled"`
	GoogleAPIKey   string            `yaml:"google_api_key"`
	SearchEngineID string            `yaml:"search_engine_id"`
	MaxResults     int               `yaml:"max_results"`
	Cache          WebSearchCacheCfg `yaml:"cache"`
}

// WebSearchCacheCfg — настройки кэша веб-поиска.
type WebSearchCacheCfg struct {
	TTL     time.Duration `yaml:"ttl"`
	MaxSize int           `yaml:"max_size"`
}

// ============================================================================
// Learning Subsystems Config
// ============================================================================

// CausalLearningConfig — настройки каузального обучения.
type CausalLearningConfig struct {
	Enabled                  bool    `yaml:"enabled"`
	AnalysisIntervalHours    int     `yaml:"analysis_interval_hours"`
	MinConfidence            float64 `yaml:"min_confidence"`
	TemporalWindowMinutes    int     `yaml:"temporal_window_minutes"`
	MaxEntriesPerChat        int     `yaml:"max_entries_per_chat"`
	AnalysisLookbackMessages int     `yaml:"analysis_lookback_messages"`
}

// EmotionalLearningConfig — настройки эмоциональной системы.
type EmotionalLearningConfig struct {
	Enabled                  bool `yaml:"enabled"`
	AnalysisIntervalHours    int  `yaml:"analysis_interval_hours"`
	AnalysisLookbackMessages int  `yaml:"analysis_lookback_messages"`
	MemoryRetentionDays      int  `yaml:"memory_retention_days"`
}

// BeliefLearningConfig — настройки системы убеждений.
type BeliefLearningConfig struct {
	Enabled                  bool `yaml:"enabled"`
	AnalysisIntervalHours    int  `yaml:"analysis_interval_hours"`
	AnalysisLookbackMessages int  `yaml:"analysis_lookback_messages"`
}

// ============================================================================
// Cognitive Architecture Config
// ============================================================================

// CognitiveArchConfig — настройки когнитивной архитектуры.
type CognitiveArchConfig struct {
	InternalMonologue     InternalMonologueCfg `yaml:"internal_monologue"`
	SelfReflection        SelfReflectionCfg    `yaml:"self_reflection"`
	ConfidenceCalibration ConfidenceCalibCfg   `yaml:"confidence_calibration"`
}

// InternalMonologueCfg — настройки внутреннего монолога.
type InternalMonologueCfg struct {
	Enabled     bool    `yaml:"enabled"`
	Temperature float64 `yaml:"temperature"`
}

// SelfReflectionCfg — настройки саморефлексии.
type SelfReflectionCfg struct {
	Enabled     bool    `yaml:"enabled"`
	Temperature float64 `yaml:"temperature"`
}

// ConfidenceCalibCfg — настройки калибровки уверенности.
type ConfidenceCalibCfg struct {
	Enabled bool `yaml:"enabled"`
}

// ============================================================================
// Social Architecture Config
// ============================================================================

// SocialArchConfig — настройки социальной архитектуры.
type SocialArchConfig struct {
	RelationshipTracking RelationshipTrackingCfg `yaml:"relationship_tracking"`
	SocialLearning       SocialLearningCfg       `yaml:"social_learning"`
	IntimacyGrowthRate   float64                 `yaml:"intimacy_growth_rate"`
	TrustDecayRate       float64                 `yaml:"trust_decay_rate"`
}

// RelationshipTrackingCfg — настройки отслеживания отношений.
type RelationshipTrackingCfg struct {
	Enabled bool `yaml:"enabled"`
}

// SocialLearningCfg — настройки социального обучения.
type SocialLearningCfg struct {
	Enabled bool `yaml:"enabled"`
}

// ============================================================================
// Association Cloud Config
// ============================================================================

// AssociationCloudConfig — настройки облака ассоциаций.
type AssociationCloudConfig struct {
	Enabled   bool `yaml:"enabled"`
	MaxNodes  int  `yaml:"max_nodes"`
	MaxEdges  int  `yaml:"max_edges"`
	DecayDays int  `yaml:"decay_days"`
}

// ============================================================================
// Storage Config
// ============================================================================

// StorageConfig — настройки хранилища.
type StorageConfig struct {
	Type           string               `yaml:"type"`
	PostgreSQL     PostgreSQLConfig     `yaml:"postgresql"`
	MongoDB        MongoDBConfig        `yaml:"mongodb"`
	LongTermMemory LongTermMemoryConfig `yaml:"long_term_memory"`
	Cleanup        CleanupConfig        `yaml:"cleanup"`
}

// PostgreSQLConfig — настройки PostgreSQL.
type PostgreSQLConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
}

// MongoDBConfig — настройки MongoDB.
type MongoDBConfig struct {
	URI                    string `yaml:"uri"`
	DBName                 string `yaml:"dbname"`
	MessagesCollection     string `yaml:"messages_collection"`
	UserProfilesCollection string `yaml:"user_profiles_collection"`
	SettingsCollection     string `yaml:"settings_collection"`
	VectorIndexName        string `yaml:"vector_index_name"`
}

// LongTermMemoryConfig — настройки долгосрочной памяти.
type LongTermMemoryConfig struct {
	Enabled        bool           `yaml:"enabled"`
	EmbeddingModel string         `yaml:"embedding_model"`
	FetchK         int            `yaml:"fetch_k"`
	Backfill       BackfillConfig `yaml:"backfill"`
}

// BackfillConfig — настройки бэкфилла.
type BackfillConfig struct {
	BatchSize  int           `yaml:"batch_size"`
	BatchDelay time.Duration `yaml:"batch_delay"`
}

// CleanupConfig — настройки автоочистки.
type CleanupConfig struct {
	Enabled            bool `yaml:"enabled"`
	SizeLimitMB        int  `yaml:"size_limit_mb"`
	IntervalMinutes    int  `yaml:"interval_minutes"`
	ChunkDurationHours int  `yaml:"chunk_duration_hours"`
}

// ============================================================================
// Prompts Config
// ============================================================================

// PromptsConfig — настройки промптов.
type PromptsConfig struct {
	Source string            `yaml:"source"` // "files" | "inline" | "both"
	Files  map[string]string `yaml:"files"`  // response_type → путь к .txt
	Inline map[string]string `yaml:"inline"` // response_type → текст промпта
}

// ============================================================================
// Legacy-секции (мигрированы из старого Config)
// ============================================================================

// ChatConfig — настройки контекста чата.
type ChatConfig struct {
	MinMessages               int `yaml:"min_messages"`
	MaxMessages               int `yaml:"max_messages"`
	ContextWindow             int `yaml:"context_window"`
	ImageGenerationContextWindow int `yaml:"image_generation_context_window"`
	DailyTakeTime             int `yaml:"daily_take_time"`
}

// SummaryConfig — настройки саммари.
type SummaryConfig struct {
	IntervalHours int              `yaml:"interval_hours"`
	MaxParts      int              `yaml:"max_parts"`
	Weekly        WeeklySummaryCfg `yaml:"weekly"`
}

// WeeklySummaryCfg — настройки еженедельного саммари.
type WeeklySummaryCfg struct {
	Enabled         bool   `yaml:"enabled"`
	Day             int    `yaml:"day"`
	Hour            int    `yaml:"hour"`
	Minute          int    `yaml:"minute"`
	MaxParts        int    `yaml:"max_parts"`
	SearchMethod    string `yaml:"search_method"`
	FlagsEnabled    bool   `yaml:"flags_enabled"`
	KeywordsEnabled bool   `yaml:"keywords_enabled"`
}

// RateLimitConfig — настройки лимита запросов.
type RateLimitConfig struct {
	StaticText string `yaml:"static_text"`
}

// SettingsPromptsConfig — UI-тексты для ввода настроек.
type SettingsPromptsConfig struct {
	EnterMinMessages        string `yaml:"enter_min_messages"`
	EnterMaxMessages        string `yaml:"enter_max_messages"`
	EnterDailyTime          string `yaml:"enter_daily_time"`
	EnterSummaryInterval    string `yaml:"enter_summary_interval"`
	EnterDirectLimitCount   string `yaml:"enter_direct_limit_count"`
	EnterDirectLimitDuration string `yaml:"enter_direct_limit_duration"`
}

// SrachConfig — настройки детекции споров/конфликтов.
type SrachConfig struct {
	Keywords        []string `yaml:"keywords"`
	AnalysisEnabled bool     `yaml:"analysis_enabled"`
}

// DirectReplyLimitsConfig — настройки лимитов прямых обращений.
type DirectReplyLimitsConfig struct {
	EnabledDefault         bool `yaml:"enabled_default"`
	CountDefault           int  `yaml:"count_default"`
	DurationMinutesDefault int  `yaml:"duration_minutes_default"`
}

// DonateConfig — настройки сообщений о донатах.
type DonateConfig struct {
	TimeHours                  int `yaml:"time_hours"`
	PaymentReminderIntervalHours int `yaml:"payment_reminder_interval_hours"`
}

// EmbeddingConfig — настройки эмбеддингов и миграции.
type EmbeddingConfig struct {
	RequestsPerMinute  int               `yaml:"requests_per_minute"`
	RequestsPerDay     int               `yaml:"requests_per_day"`
	BatchSize          int               `yaml:"batch_size"`
	RequestDelay       time.Duration     `yaml:"request_delay"`
	BatchDelay         time.Duration     `yaml:"batch_delay"`
	AdaptiveThrottling bool              `yaml:"adaptive_throttling"`
	SafetyMargin       float64           `yaml:"safety_margin"`
	MaxRetries         int               `yaml:"max_retries"`
	Cache              EmbeddingCacheCfg `yaml:"cache"`
	Migration          MigrationCfg      `yaml:"migration"`
}

// EmbeddingCacheCfg — настройки кэша эмбеддингов.
type EmbeddingCacheCfg struct {
	Enabled bool   `yaml:"enabled"`
	Dir     string `yaml:"dir"`
}

// MigrationCfg — настройки миграции.
type MigrationCfg struct {
	ResumeEnabled bool   `yaml:"resume_enabled"`
	StateFile     string `yaml:"state_file"`
}

// ============================================================================
// Методы Config
// ============================================================================

// GetRoutingProfile возвращает routing-профиль для ResponseType.
// Если явный профиль не найден — возвращает default.
func (c *ConfigV2) GetRoutingProfile(responseType string) RoutingProfile {
	if profile, ok := c.LLM.ResponseTypes[responseType]; ok {
		return profile
	}
	if defaultProfile, ok := c.LLM.ResponseTypes["default"]; ok {
		return defaultProfile
	}
	return RoutingProfile{
		Provider:    c.LLM.DefaultProvider,
		Temperature: 1.0,
	}
}
