package config

import (
	"strconv"
	"time"
)

// MigrateConfig конвертирует старый плоский Config в новый структурированный ConfigV2.
// Используется на переходный период, когда бот загружается из .env.
// После полной миграции на YAML эта функция будет удалена (CFG-14).
func MigrateConfig(old *Config) *ConfigV2 {
	if old == nil {
		return &ConfigV2{}
	}

	cfg := &ConfigV2{}

	cfg.LLM = migrateLLMConfig(old)
	cfg.TTS = migrateTTSConfig(old)
	cfg.Telegram = migrateTelegramConfig(old)
	cfg.Chat = migrateChatConfig(old)
	cfg.Summary = migrateSummaryConfig(old)
	cfg.RateLimit = migrateRateLimitConfig(old)
	cfg.SettingsPrompts = migrateSettingsPromptsConfig(old)
	cfg.Srach = migrateSrachConfig(old)
	cfg.DirectReplyLimits = migrateDirectReplyLimitsConfig(old)
	cfg.Donate = migrateDonateConfig(old)
	cfg.FreeWill = migrateFreeWillConfig(old)
	cfg.VoiceMessages = migrateVoiceMessagesConfig(old)
	cfg.Moderation = migrateModerationConfig(old)
	cfg.AntiRepetition = migrateAntiRepetitionConfig(old)
	cfg.Disambiguation = migrateDisambiguationConfig(old)
	cfg.AutoBio = migrateAutoBioConfig(old)
	cfg.Personality = migratePersonalityConfig(old)
	cfg.Reactions = migrateReactionsConfig(old)
	cfg.WebSearch = migrateWebSearchConfig(old)
	cfg.CausalLearning = migrateCausalLearningConfig(old)
	cfg.EmotionalLearning = migrateEmotionalLearningConfig(old)
	cfg.BeliefLearning = migrateBeliefLearningConfig(old)
	cfg.CognitiveArchitecture = migrateCognitiveArchConfig(old)
	cfg.SocialArchitecture = migrateSocialArchConfig(old)
	cfg.AssociationCloud = migrateAssociationCloudConfig(old)
	cfg.Storage = migrateStorageConfig(old)
	cfg.Embedding = migrateEmbeddingConfig(old)
	cfg.Prompts = migratePromptsConfig(old)

	return cfg
}

// ============================================================================
// LLM
// ============================================================================

func migrateLLMConfig(old *Config) LLMConfig {
	cfg := LLMConfig{
		DefaultProvider:       string(old.LLMProvider),
		FallbackEnabled:       old.LLMFallbackEnabled,
		FallbackCriticalTypes: old.LLMFallbackCriticalTypes,
		FallbackProviderOrder: old.LLMFallbackProviderOrder,
		CircuitBreaker: CircuitBreakerConfig{
			MaxFailures:     5,
			CooldownSeconds: 60,
		},
		Providers:     make(map[string]ProviderConfig),
		ResponseTypes: make(map[string]RoutingProfile),
	}

	if cfg.DefaultProvider == "" {
		cfg.DefaultProvider = "gemini"
	}

	// Gemini
	cfg.Providers["gemini"] = ProviderConfig{
		Enabled:          old.GeminiAPIKey != "",
		APIKey:           old.GeminiAPIKey,
		ReserveAPIKey:    old.GeminiAPIKeyReserve,
		KeyRotationHours: old.GeminiKeyRotationTimeHours,
		Safety: SafetyConfig{
			BypassFilters: old.GeminiBypassSafetyFilters,
			Obfuscate:     old.GeminiObfuscatePrompts,
		},
		Models: map[string]string{
			"text":       old.GeminiModelName,
			"audio":      old.AudioTranscriptionModel,
			"image":      old.ImageGenerationModel,
			"embedding":  old.GeminiEmbeddingModelName,
		},
		Temperatures: map[string]float64{
			"normal":  old.GeminiTemperatureNormal,
			"serious": old.GeminiTemperatureSerious,
			"audio":   old.AudioTranscriptionTemperature,
			"image":   old.ImageGenerationTemperature,
		},
	}

	// DeepSeek
	cfg.Providers["deepseek"] = ProviderConfig{
		Enabled: old.DeepSeekAPIKey != "",
		APIKey:  old.DeepSeekAPIKey,
		BaseURL: old.DeepSeekBaseURL,
		Models: map[string]string{
			"text": old.DeepSeekModelName,
		},
	}

	// OpenRouter
	cfg.Providers["openrouter"] = ProviderConfig{
		Enabled:   old.OpenRouterAPIKey != "",
		APIKey:    old.OpenRouterAPIKey,
		SiteURL:   old.OpenRouterSiteURL,
		SiteTitle: old.OpenRouterSiteTitle,
		Models: map[string]string{
			"text": old.OpenRouterModelName,
		},
	}

	// Мигрируем ResponseTypeConfigs
	if old.ResponseTypeConfigs != nil {
		for respType, rc := range old.ResponseTypeConfigs {
			cfg.ResponseTypes[respType] = RoutingProfile{
				Provider:    string(rc.Provider),
				Model:       rc.ModelName,
				Temperature: float64(rc.Temperature),
				Priority:    10,
			}
		}
	}

	// Добавляем промпт-специфичные роутинги, которые есть отдельными полями
	addPromptRouting(cfg.ResponseTypes, "causal_analysis",
		old.CausalAnalysisPromptProvider, old.CausalAnalysisPromptModel,
		old.CausalAnalysisPromptTemperature, old.CausalAnalysisPromptEnabled)
	addPromptRouting(cfg.ResponseTypes, "causal_influence",
		old.CausalInfluencePromptProvider, old.CausalInfluencePromptModel,
		old.CausalInfluencePromptTemperature, old.CausalInfluencePromptEnabled)
	addPromptRouting(cfg.ResponseTypes, "emotional_analysis",
		old.EmotionalAnalysisPromptProvider, old.EmotionalAnalysisPromptModel,
		old.EmotionalAnalysisPromptTemperature, old.EmotionalAnalysisPromptEnabled)
	addPromptRouting(cfg.ResponseTypes, "emotional_adaptation",
		old.EmotionalAdaptationPromptProvider, old.EmotionalAdaptationPromptModel,
		old.EmotionalAdaptationPromptTemperature, old.EmotionalAdaptationPromptEnabled)
	addPromptRouting(cfg.ResponseTypes, "emotional_feedback",
		old.EmotionalFeedbackPromptProvider, old.EmotionalFeedbackPromptModel,
		old.EmotionalFeedbackPromptTemperature, old.EmotionalFeedbackPromptEnabled)
	addPromptRouting(cfg.ResponseTypes, "internal_monologue",
		old.InternalMonologuePromptProvider, old.InternalMonologuePromptModel,
		old.InternalMonologueTemperature, old.InternalMonologuePromptEnabled)
	addPromptRouting(cfg.ResponseTypes, "self_reflection",
		old.SelfReflectionPromptProvider, old.SelfReflectionPromptModel,
		old.SelfReflectionTemperature, old.SelfReflectionPromptEnabled)
	addPromptRouting(cfg.ResponseTypes, "relationship_analysis",
		old.RelationshipAnalysisProvider, old.RelationshipAnalysisModel,
		old.RelationshipAnalysisTemp, old.RelationshipAnalysisEnabled)
	addPromptRouting(cfg.ResponseTypes, "belief_analysis",
		old.BeliefAnalysisPromptProvider, old.BeliefAnalysisPromptModel,
		old.BeliefAnalysisPromptTemperature, old.BeliefAnalysisPromptEnabled)

	return cfg
}

func addPromptRouting(routing map[string]RoutingProfile, name, provider, model string, temperature float64, enabled bool) {
	if _, exists := routing[name]; exists {
		return
	}
	if !enabled {
		return
	}
	if provider == "" {
		provider = "gemini"
	}
	routing[name] = RoutingProfile{
		Provider:    provider,
		Model:       model,
		Temperature: temperature,
		Priority:    5,
	}
}

// ============================================================================
// TTS
// ============================================================================

func migrateTTSConfig(old *Config) TTSConfig {
	cfg := TTSConfig{
		Provider: "elevenlabs",
		ElevenLabs: ElevenLabsConfig{
			APIKey:     old.ElevenLabsAPIKey,
			VoiceID:    old.ElevenLabsVoiceID,
			Model:      old.ElevenLabsModel,
			Plan:       old.ElevenLabsPlan,
			RandomVoice: old.ElevenLabsRandomVoice,
			VoiceSettings: VoiceSettingsConfig{
				Stability:       old.ElevenLabsStability,
				SimilarityBoost: old.ElevenLabsSimilarityBoost,
				Style:           old.ElevenLabsStyle,
				UseSpeakerBoost: old.ElevenLabsUseSpeakerBoost,
				Speed:           old.ElevenLabsSpeed,
			},
			Prompts: VoicePromptsConfig{
				Style:   old.ElevenLabsStylePrompt,
				Emotion: old.ElevenLabsEmotionPrompt,
				Pace:    old.ElevenLabsPacePrompt,
			},
		},
	}
	return cfg
}

// ============================================================================
// Telegram
// ============================================================================

func migrateTelegramConfig(old *Config) TelegramConfig {
	cfg := TelegramConfig{
		Token:                      old.TelegramToken,
		BotUserID:                  0,
		BotNames:                   old.BotNames,
		AdminUsernames:             old.AdminUsernames,
		AdminIDs:                   []int64{old.AdminID},
		Debug:                      old.Debug,
		Timezone:                   old.TimeZone,
		ErrorAutoDeleteSeconds:     old.ErrorMessageAutoDeleteSeconds,
		UseStructuredMessageFormat: old.UseStructuredMessageFormat,
	}
	return cfg
}

// ============================================================================
// Chat
// ============================================================================

func migrateChatConfig(old *Config) ChatConfig {
	return ChatConfig{
		MinMessages:               old.MinMessages,
		MaxMessages:               old.MaxMessages,
		ContextWindow:             old.ContextWindow,
		ImageGenerationContextWindow: old.ImageGenerationContextWindow,
		DailyTakeTime:             old.DailyTakeTime,
	}
}

// ============================================================================
// Summary
// ============================================================================

func migrateSummaryConfig(old *Config) SummaryConfig {
	return SummaryConfig{
		IntervalHours: old.SummaryIntervalHours,
		MaxParts:      old.SummaryMaxParts,
		Weekly: WeeklySummaryCfg{
			Enabled:         old.WeeklySummaryEnabled,
			Day:             old.WeeklySummaryDay,
			Hour:            old.WeeklySummaryHour,
			Minute:          old.WeeklySummaryMinute,
			MaxParts:        old.WeeklySummaryMaxParts,
		},
	}
}

// ============================================================================
// Rate Limit
// ============================================================================

func migrateRateLimitConfig(old *Config) RateLimitConfig {
	return RateLimitConfig{
		StaticText: old.RateLimitStaticText,
	}
}

// ============================================================================
// Settings Prompts
// ============================================================================

func migrateSettingsPromptsConfig(old *Config) SettingsPromptsConfig {
	return SettingsPromptsConfig{
		EnterMinMessages:        old.PromptEnterMinMessages,
		EnterMaxMessages:        old.PromptEnterMaxMessages,
		EnterDailyTime:          old.PromptEnterDailyTime,
		EnterSummaryInterval:    old.PromptEnterSummaryInterval,
		EnterDirectLimitCount:   old.PromptEnterDirectLimitCount,
		EnterDirectLimitDuration: old.PromptEnterDirectLimitDuration,
	}
}

// ============================================================================
// Srach
// ============================================================================

func migrateSrachConfig(old *Config) SrachConfig {
	return SrachConfig{
		AnalysisEnabled: old.SrachAnalysisEnabled,
	}
}

// ============================================================================
// Direct Reply Limits
// ============================================================================

func migrateDirectReplyLimitsConfig(old *Config) DirectReplyLimitsConfig {
	return DirectReplyLimitsConfig{
		EnabledDefault:         old.DirectReplyLimitEnabledDefault,
		CountDefault:           old.DirectReplyLimitCountDefault,
		DurationMinutesDefault: int(old.DirectReplyLimitDurationDefault.Minutes()),
	}
}

// ============================================================================
// Donate
// ============================================================================

func migrateDonateConfig(old *Config) DonateConfig {
	return DonateConfig{
		TimeHours:                  old.DonateTimeHours,
		PaymentReminderIntervalHours: 6,
	}
}

// ============================================================================
// Embedding
// ============================================================================

func migrateEmbeddingConfig(old *Config) EmbeddingConfig {
	return EmbeddingConfig{
		RequestsPerMinute:  240,
		RequestsPerDay:     24000,
		BatchSize:          100,
		RequestDelay:       300 * time.Millisecond,
		BatchDelay:         60 * time.Second,
		AdaptiveThrottling: true,
		SafetyMargin:       0.8,
		MaxRetries:         3,
		Cache: EmbeddingCacheCfg{
			Enabled: true,
			Dir:     "./cache/embeddings",
		},
		Migration: MigrationCfg{
			ResumeEnabled: true,
			StateFile:     "./migration_state.json",
		},
	}
}

// ============================================================================
// Free Will
// ============================================================================

func migrateFreeWillConfig(old *Config) FreeWillConfig {
	cfg := FreeWillConfig{
		Enabled: old.FreeWillEnabled,
		Intervals: FreeWillIntervalsConfig{
			MinMinutes: old.FreeWillMinIntervalMinutes,
			MaxMinutes: old.FreeWillMaxIntervalMinutes,
		},
		ContextWindow:         old.FreeWillContextWindow,
		MoodUpdateProbability: old.FreeWillMoodUpdateProbability,
		MaxDecisionsPerHour:   old.FreeWillMaxDecisionsPerHour,
		VoiceProbability:      old.FreeWillVoiceProbability,
		Silence: SilenceConfig{
			MinMinutes: old.FreeWillSilenceMinMinutes,
			MaxMinutes: old.FreeWillSilenceMaxMinutes,
		},
		Reactions: FreeWillReactionsConfig{
			Enabled:         old.FreeWillReactionsEnabled,
			Probability:     old.FreeWillReactionsProbability,
			CooldownMinutes: old.FreeWillReactionsCooldownMinutes,
			MaxPerHour:      old.FreeWillReactionsMaxPerHour,
		},
		DirectResponse: DirectResponseConfig{
			MaxPerHour:         old.FreeWillDirectResponseMaxPerHour,
			MinIntervalSeconds: old.FreeWillDirectResponseMinIntervalSeconds,
			IndependentLimits:  old.FreeWillDirectResponseIndependentLimits,
		},
		ImageGeneration: ImageGenLimitsConfig{
			MaxPerInterval:         old.FreeWillImageGenerationMaxDecisionsPerInterval,
			IntervalHours:          old.FreeWillImageGenerationIntervalHours,
			MinDecisionIntervalMin: old.FreeWillImageGenerationMinDecisionIntervalMinutes,
			IndependentLimits:      old.FreeWillImageGenerationIndependentLimits,
			FrequencyHours:         old.ImageGenFrequencyHours,
		},
		IntervalMessages: IntervalMessagesConfig{
			Enabled: old.IntervalMessagesEnabled,
		},
	}
	return cfg
}

// ============================================================================
// Voice Messages
// ============================================================================

func migrateVoiceMessagesConfig(old *Config) VoiceMessagesConfig {
	cfg := VoiceMessagesConfig{
		Enabled: old.VoiceMessagesEnabled,
		Interval: IntervalConfig{
			Min: old.MinVoiceMessages,
			Max: old.MaxVoiceMessages,
		},
		TempDir: old.VoiceMessageTempDir,
	}
	return cfg
}

// ============================================================================
// Moderation
// ============================================================================

func migrateModerationConfig(old *Config) ModerationConfig {
	cfg := ModerationConfig{
		Enabled:          old.ModEnabled,
		IntervalMinutes:  old.ModInterval,
		MuteTimeMinutes:  old.ModMuteTimeMin,
		KickTimeMinutes:  old.ModKickTimeMin,
		BanTimeMinutes:   old.ModBanTimeMin,
		PurgeWindow:      old.ModPurgeDuration,
		PurgeDelay:       old.ModPurgeDelay,
		CheckAdminRights: old.ModCheckAdminRights,
		DefaultNotify:    old.ModDefaultNotify,
	}
	return cfg
}

// ============================================================================
// Anti-Repetition
// ============================================================================

func migrateAntiRepetitionConfig(old *Config) AntiRepetitionConfig {
	cfg := AntiRepetitionConfig{
		Enabled:              old.AntiRepetitionEnabled,
		MaxResponsesPerChat:  old.AntiRepetitionMaxResponsesPerChat,
		SimilarityThreshold:  old.AntiRepetitionSimilarityThreshold,
		TimeWindowHours:      old.AntiRepetitionTimeWindowHours,
		CleanupIntervalHours: old.AntiRepetitionCleanupIntervalHours,
		Rework: AntiRepetitionReworkCfg{
			Enabled:     old.AntiRepetitionReworkEnabled,
			MaxAttempts: old.AntiRepetitionMaxReworkAttempts,
			Temperature: old.AntiRepetitionReworkTemperature,
			LocalRework: LocalReworkConfig{
				Enabled:   old.AntiRepetitionLocalReworkEnabled,
				MaxLength: old.AntiRepetitionLocalReworkMaxLength,
			},
		},
	}
	return cfg
}

// ============================================================================
// Disambiguation
// ============================================================================

func migrateDisambiguationConfig(old *Config) DisambiguationConfig {
	return DisambiguationConfig{
		Enabled: old.DisambiguationEnabled,
	}
}

// ============================================================================
// Auto Bio
// ============================================================================

func migrateAutoBioConfig(old *Config) AutoBioConfig {
	return AutoBioConfig{
		Enabled:                old.AutoBioEnabled,
		IntervalHours:          old.AutoBioIntervalHours,
		LookbackDays:           old.AutoBioMessagesLookbackDays,
		MinMessagesForAnalysis: old.AutoBioMinMessagesForAnalysis,
		MaxMessagesForAnalysis: old.AutoBioMaxMessagesForAnalysis,
	}
}

// ============================================================================
// Personality
// ============================================================================

func migratePersonalityConfig(old *Config) PersonalityConfig {
	return PersonalityConfig{
		UpdateIntervalHours:   old.PersonalityUpdateIntervalHours,
		MessagesLookback:      old.PersonalityMessagesLookback,
		MaxNameMentions:       old.MaxNameMentions,
		MaxRecentTopics:       old.MaxRecentTopics,
		MaxSelfPerceptions:    old.MaxSelfPerceptions,
		MaxDiscussionContexts: old.MaxDiscussionContexts,
	}
}

// ============================================================================
// Reactions
// ============================================================================

func migrateReactionsConfig(old *Config) ReactionsConfig {
	return ReactionsConfig{
		Enabled: old.ReactionsEnabled,
		Clown: ClownConfig{
			ResponseProbability: old.ClownResponseProbability,
			CooldownSeconds:     old.ClownCooldownSeconds,
			MaxResponsesPerHour: old.MaxClownResponsesPerHour,
		},
	}
}

// ============================================================================
// Web Search
// ============================================================================

func migrateWebSearchConfig(old *Config) WebSearchConfig {
	return WebSearchConfig{
		Enabled:        old.WebSearchEnabled,
		GoogleAPIKey:   old.GoogleSearchAPIKey,
		SearchEngineID: old.GoogleSearchEngineID,
		MaxResults:     old.WebSearchMaxResults,
		Cache: WebSearchCacheCfg{
			TTL:     old.WebSearchCacheTTL,
			MaxSize: old.WebSearchCacheMaxSize,
		},
	}
}

// ============================================================================
// Causal Learning
// ============================================================================

func migrateCausalLearningConfig(old *Config) CausalLearningConfig {
	return CausalLearningConfig{
		Enabled:                  old.CausalLearningEnabled,
		AnalysisIntervalHours:    old.CausalAnalysisIntervalHours,
		MinConfidence:            old.CausalMinConfidence,
		TemporalWindowMinutes:    old.CausalTemporalWindowMinutes,
		MaxEntriesPerChat:        old.CausalMaxEntriesPerChat,
		AnalysisLookbackMessages: old.CausalAnalysisLookbackMessages,
	}
}

// ============================================================================
// Emotional Learning
// ============================================================================

func migrateEmotionalLearningConfig(old *Config) EmotionalLearningConfig {
	return EmotionalLearningConfig{
		Enabled:                  old.EmotionalLearningEnabled,
		AnalysisIntervalHours:    old.EmotionalAnalysisIntervalHours,
		AnalysisLookbackMessages: old.EmotionalAnalysisLookbackMessages,
		MemoryRetentionDays:      old.EmotionalMemoryRetentionDays,
		MinMessagesForAnalysis:   old.EmotionalMinMessagesForAnalysis,
		AnalysisDebounceHours:    old.EmotionalAnalysisDebounceHours,
	}
}

// ============================================================================
// Belief Learning
// ============================================================================

func migrateBeliefLearningConfig(old *Config) BeliefLearningConfig {
	return BeliefLearningConfig{
		Enabled:                  old.BeliefLearningEnabled,
		AnalysisIntervalHours:    old.BeliefAnalysisIntervalHours,
		AnalysisLookbackMessages: old.BeliefAnalysisLookbackMessages,
	}
}

// ============================================================================
// Cognitive Architecture
// ============================================================================

func migrateCognitiveArchConfig(old *Config) CognitiveArchConfig {
	return CognitiveArchConfig{
		InternalMonologue: InternalMonologueCfg{
			Enabled:     old.InternalMonologueEnabled,
			Temperature: old.InternalMonologueTemperature,
		},
		SelfReflection: SelfReflectionCfg{
			Enabled:     old.SelfReflectionEnabled,
			Temperature: old.SelfReflectionTemperature,
		},
		ConfidenceCalibration: ConfidenceCalibCfg{
			Enabled: old.ConfidenceCalibrationEnabled,
		},
	}
}

// ============================================================================
// Social Architecture
// ============================================================================

func migrateSocialArchConfig(old *Config) SocialArchConfig {
	return SocialArchConfig{
		RelationshipTracking: RelationshipTrackingCfg{
			Enabled: old.RelationshipTrackingEnabled,
		},
		SocialLearning: SocialLearningCfg{
			Enabled: old.SocialLearningEnabled,
		},
		IntimacyGrowthRate: old.IntimacyGrowthRate,
		TrustDecayRate:     old.TrustDecayRate,
	}
}

// ============================================================================
// Association Cloud
// ============================================================================

func migrateAssociationCloudConfig(old *Config) AssociationCloudConfig {
	return AssociationCloudConfig{
		Enabled:   old.AssociationCloudEnabled,
		MaxNodes:  old.AssociationCloudMaxNodes,
		MaxEdges:  old.AssociationCloudMaxEdges,
		DecayDays: old.AssociationCloudDecayDays,
	}
}

// ============================================================================
// Storage
// ============================================================================

func migrateStorageConfig(old *Config) StorageConfig {
	port, _ := strconv.Atoi(old.PostgresqlPort)
	cfg := StorageConfig{
		Type: string(old.StorageType),
		PostgreSQL: PostgreSQLConfig{
			Host:     old.PostgresqlHost,
			Port:     port,
			User:     old.PostgresqlUser,
			Password: old.PostgresqlPassword,
			DBName:   old.PostgresqlDbname,
		},
		MongoDB: MongoDBConfig{
			URI:                    old.MongoDbURI,
			DBName:                 old.MongoDbName,
			MessagesCollection:     old.MongoDbMessagesCollection,
			UserProfilesCollection: old.MongoDbUserProfilesCollection,
			SettingsCollection:     old.MongoDbSettingsCollection,
			VectorIndexName:        old.MongoVectorIndexName,
		},
		LongTermMemory: LongTermMemoryConfig{
			Enabled:        old.LongTermMemoryEnabled,
			EmbeddingModel: old.GeminiEmbeddingModelName,
			FetchK:         old.LongTermMemoryFetchK,
			Backfill: BackfillConfig{
				BatchSize:  old.BackfillBatchSize,
				BatchDelay: old.BackfillBatchDelay,
			},
		},
		Cleanup: CleanupConfig{
			Enabled:            old.MongoCleanupEnabled,
			SizeLimitMB:        old.MongoCleanupSizeLimitMB,
			IntervalMinutes:    old.MongoCleanupIntervalMinutes,
			ChunkDurationHours: old.MongoCleanupChunkDurationHours,
		},
	}
	return cfg
}

// ============================================================================
// Prompts
// ============================================================================

func migratePromptsConfig(old *Config) PromptsConfig {
	return PromptsConfig{
		Source: "inline",
	}
}

// Ensure time import is used (for BackfillConfig.BatchDelay referenced above)
var _ = time.Duration(0)

// Ensure strconv import is used (for StorageConfig PostgreSQL Port parsing)
var _ = strconv.Atoi
