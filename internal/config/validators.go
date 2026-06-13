package config

import (
	"fmt"
	"time"
)

// ValidationError — ошибка валидации с путём в YAML.
type ValidationError struct {
	Path    string // например, "llm.providers.gemini.api_key"
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// Validator — интерфейс валидатора.
type Validator interface {
	Validate(cfg *ConfigV2) []error
}

// ============================================================================
// StructureValidator
// ============================================================================

// StructureValidator проверяет структуру конфигурации.
type StructureValidator struct{}

func (v *StructureValidator) Validate(cfg *ConfigV2) []error {
	if cfg == nil {
		return []error{&ValidationError{Path: "", Message: "config is nil"}}
	}
	var errs []error

	errs = append(errs, v.validateLLM(cfg)...)
	errs = append(errs, v.validateTelegram(cfg)...)
	errs = append(errs, v.validateStorage(cfg)...)
	errs = append(errs, v.validatePrompts(cfg)...)

	return errs
}

func (v *StructureValidator) validateLLM(cfg *ConfigV2) []error {
	var errs []error

	if cfg.LLM.DefaultProvider == "" {
		errs = append(errs, &ValidationError{
			Path:    "llm.default_provider",
			Message: "default_provider must not be empty",
		})
	}

	if len(cfg.LLM.Providers) == 0 {
		errs = append(errs, &ValidationError{
			Path:    "llm.providers",
			Message: "at least one LLM provider must be configured",
		})
	}

	for name, p := range cfg.LLM.Providers {
		prefix := fmt.Sprintf("llm.providers.%s", name)
		if p.Enabled {
			if p.Models == nil || p.Models["text"] == "" {
				errs = append(errs, &ValidationError{
					Path:    prefix + ".models.text",
					Message: "enabled provider must have a text model configured",
				})
			}
		}
	}

	return errs
}

func (v *StructureValidator) validateTelegram(cfg *ConfigV2) []error {
	var errs []error

	if cfg.Telegram.Token == "" {
		errs = append(errs, &ValidationError{
			Path:    "telegram.token",
			Message: "token must not be empty",
		})
	}

	if len(cfg.Telegram.AdminUsernames) == 0 && len(cfg.Telegram.AdminIDs) == 0 {
		errs = append(errs, &ValidationError{
			Path:    "telegram.admin_usernames",
			Message: "at least one admin username or admin ID must be configured",
		})
	}

	return errs
}

func (v *StructureValidator) validateStorage(cfg *ConfigV2) []error {
	var errs []error

	if cfg.Storage.Type == "" {
		errs = append(errs, &ValidationError{
			Path:    "storage.type",
			Message: "storage type must not be empty (file, postgres, mongo)",
		})
		return errs
	}

	validTypes := map[string]bool{"file": true, "postgres": true, "mongo": true}
	if !validTypes[cfg.Storage.Type] {
		errs = append(errs, &ValidationError{
			Path:    "storage.type",
			Message: fmt.Sprintf("unknown storage type '%s', expected: file, postgres, mongo", cfg.Storage.Type),
		})
	}

	return errs
}

func (v *StructureValidator) validatePrompts(cfg *ConfigV2) []error {
	var errs []error

	if cfg.Prompts.Source == "" {
		errs = append(errs, &ValidationError{
			Path:    "prompts.source",
			Message: "prompts.source must not be empty (files, inline, both)",
		})
		return errs
	}

	validSources := map[string]bool{"files": true, "inline": true, "both": true}
	if !validSources[cfg.Prompts.Source] {
		errs = append(errs, &ValidationError{
			Path:    "prompts.source",
			Message: fmt.Sprintf("unknown prompts source '%s', expected: files, inline, both", cfg.Prompts.Source),
		})
	}

	return errs
}

// ============================================================================
// TypeValidator
// ============================================================================

// TypeValidator проверяет типы и значения полей.
type TypeValidator struct{}

func (v *TypeValidator) Validate(cfg *ConfigV2) []error {
	if cfg == nil {
		return []error{&ValidationError{Path: "", Message: "config is nil"}}
	}
	var errs []error

	errs = append(errs, v.validatePositiveInts(cfg)...)
	errs = append(errs, v.validateNonNegativeDuration(cfg)...)

	return errs
}

func (v *TypeValidator) validatePositiveInts(cfg *ConfigV2) []error {
	var errs []error

	checks := []struct {
		path  string
		value int
	}{
		{"free_will.context_window", cfg.FreeWill.ContextWindow},
		{"free_will.max_decisions_per_hour", cfg.FreeWill.MaxDecisionsPerHour},
		{"voice_messages.interval.min", cfg.VoiceMessages.Interval.Min},
		{"voice_messages.interval.max", cfg.VoiceMessages.Interval.Max},
		{"moderation.interval_minutes", cfg.Moderation.IntervalMinutes},
		{"anti_repetition.max_responses_per_chat", cfg.AntiRepetition.MaxResponsesPerChat},
		{"anti_repetition.cleanup_interval_hours", cfg.AntiRepetition.CleanupIntervalHours},
		{"auto_bio.max_messages_for_analysis", cfg.AutoBio.MaxMessagesForAnalysis},
		{"auto_bio.lookback_days", cfg.AutoBio.LookbackDays},
		{"personality.max_name_mentions", cfg.Personality.MaxNameMentions},
		{"personality.max_recent_topics", cfg.Personality.MaxRecentTopics},
		{"personality.max_self_perceptions", cfg.Personality.MaxSelfPerceptions},
		{"personality.max_discussion_contexts", cfg.Personality.MaxDiscussionContexts},
		{"reactions.clown.max_responses_per_hour", cfg.Reactions.Clown.MaxResponsesPerHour},
		{"web_search.max_results", cfg.WebSearch.MaxResults},
		{"web_search.cache.max_size", cfg.WebSearch.Cache.MaxSize},
		{"causal_learning.max_entries_per_chat", cfg.CausalLearning.MaxEntriesPerChat},
		{"causal_learning.analysis_lookback_messages", cfg.CausalLearning.AnalysisLookbackMessages},
		{"causal_learning.temporal_window_minutes", cfg.CausalLearning.TemporalWindowMinutes},
		{"emotional_learning.analysis_lookback_messages", cfg.EmotionalLearning.AnalysisLookbackMessages},
		{"emotional_learning.memory_retention_days", cfg.EmotionalLearning.MemoryRetentionDays},
		{"belief_learning.analysis_lookback_messages", cfg.BeliefLearning.AnalysisLookbackMessages},
		{"social_architecture.intimacy_growth_rate", int(cfg.SocialArchitecture.IntimacyGrowthRate)},
		{"association_cloud.max_nodes", cfg.AssociationCloud.MaxNodes},
		{"association_cloud.max_edges", cfg.AssociationCloud.MaxEdges},
		{"association_cloud.decay_days", cfg.AssociationCloud.DecayDays},
		{"storage.long_term_memory.fetch_k", cfg.Storage.LongTermMemory.FetchK},
		{"storage.long_term_memory.backfill.batch_size", cfg.Storage.LongTermMemory.Backfill.BatchSize},
		{"free_will.image_generation.max_per_interval", cfg.FreeWill.ImageGeneration.MaxPerInterval},
	}

	for _, c := range checks {
		if c.value < 0 {
			errs = append(errs, &ValidationError{
				Path:    c.path,
				Message: fmt.Sprintf("value %d must be >= 0", c.value),
			})
		}
	}

	// Strictly > 0 checks
	strictPositive := []struct {
		path  string
		value int
	}{
		{"free_will.direct_response.max_per_hour", cfg.FreeWill.DirectResponse.MaxPerHour},
		{"free_will.image_generation.interval_hours", cfg.FreeWill.ImageGeneration.IntervalHours},
		{"free_will.reactions.max_per_hour", cfg.FreeWill.Reactions.MaxPerHour},
		{"reactions.clown.cooldown_seconds", cfg.Reactions.Clown.CooldownSeconds},
		{"anti_repetition.time_window_hours", cfg.AntiRepetition.TimeWindowHours},
		{"anti_repetition.rework.max_attempts", cfg.AntiRepetition.Rework.MaxAttempts},
		{"personality.update_interval_hours", cfg.Personality.UpdateIntervalHours},
		{"personality.messages_lookback", cfg.Personality.MessagesLookback},
		{"auto_bio.interval_hours", cfg.AutoBio.IntervalHours},
		{"causal_learning.analysis_interval_hours", cfg.CausalLearning.AnalysisIntervalHours},
		{"emotional_learning.analysis_interval_hours", cfg.EmotionalLearning.AnalysisIntervalHours},
		{"belief_learning.analysis_interval_hours", cfg.BeliefLearning.AnalysisIntervalHours},
		{"storage.cleanup.size_limit_mb", cfg.Storage.Cleanup.SizeLimitMB},
		{"storage.cleanup.interval_minutes", cfg.Storage.Cleanup.IntervalMinutes},
		{"storage.cleanup.chunk_duration_hours", cfg.Storage.Cleanup.ChunkDurationHours},
	}

	for _, c := range strictPositive {
		if c.value <= 0 {
			errs = append(errs, &ValidationError{
				Path:    c.path,
				Message: fmt.Sprintf("value %d must be > 0", c.value),
			})
		}
	}

	return errs
}

func (v *TypeValidator) validateNonNegativeDuration(cfg *ConfigV2) []error {
	var errs []error

	checks := []struct {
		path  string
		value time.Duration
	}{
		{"moderation.purge_window_duration", cfg.Moderation.PurgeWindow},
		{"moderation.purge_delay_duration", cfg.Moderation.PurgeDelay},
		{"web_search.cache.ttl", cfg.WebSearch.Cache.TTL},
		{"storage.long_term_memory.backfill.batch_delay", cfg.Storage.LongTermMemory.Backfill.BatchDelay},
	}

	for _, c := range checks {
		if c.value < 0 {
			errs = append(errs, &ValidationError{
				Path:    c.path,
				Message: fmt.Sprintf("duration %v must not be negative", c.value),
			})
		}
	}

	return errs
}

// ============================================================================
// RangeValidator
// ============================================================================

// RangeValidator проверяет диапазоны значений.
type RangeValidator struct{}

func (v *RangeValidator) Validate(cfg *ConfigV2) []error {
	if cfg == nil {
		return []error{&ValidationError{Path: "", Message: "config is nil"}}
	}
	var errs []error

	errs = append(errs, v.validateTemperatures(cfg)...)
	errs = append(errs, v.validateCircuitBreaker(cfg)...)
	errs = append(errs, v.validateProbabilities(cfg)...)

	return errs
}

func (v *RangeValidator) validateTemperatures(cfg *ConfigV2) []error {
	var errs []error

	// Проверка температур в провайдерах
	for provName, p := range cfg.LLM.Providers {
		for tempKey, tempVal := range p.Temperatures {
			if tempVal < 0.0 || tempVal > 2.0 {
				errs = append(errs, &ValidationError{
					Path:    fmt.Sprintf("llm.providers.%s.temperatures.%s", provName, tempKey),
					Message: fmt.Sprintf("temperature %.2f must be in range [0.0, 2.0]", tempVal),
				})
			}
		}
	}

	// Проверка температур в routing-профилях
	for respType, profile := range cfg.LLM.ResponseTypes {
		if profile.Temperature < 0.0 || profile.Temperature > 2.0 {
			errs = append(errs, &ValidationError{
				Path:    fmt.Sprintf("llm.response_types.%s.temperature", respType),
				Message: fmt.Sprintf("temperature %.2f must be in range [0.0, 2.0]", profile.Temperature),
			})
		}
	}

	// Отдельные температуры в подсистемах
	tempChecks := []struct {
		path  string
		value float64
	}{
		{"free_will.mood_update_probability", cfg.FreeWill.MoodUpdateProbability},
		{"free_will.voice_probability", cfg.FreeWill.VoiceProbability},
		{"free_will.reactions.probability", cfg.FreeWill.Reactions.Probability},
		{"anti_repetition.similarity_threshold", cfg.AntiRepetition.SimilarityThreshold},
		{"anti_repetition.rework.temperature", cfg.AntiRepetition.Rework.Temperature},
		{"causal_learning.min_confidence", cfg.CausalLearning.MinConfidence},
		{"social_architecture.intimacy_growth_rate", cfg.SocialArchitecture.IntimacyGrowthRate},
		{"social_architecture.trust_decay_rate", cfg.SocialArchitecture.TrustDecayRate},
		{"cognitive_architecture.internal_monologue.temperature", cfg.CognitiveArchitecture.InternalMonologue.Temperature},
		{"cognitive_architecture.self_reflection.temperature", cfg.CognitiveArchitecture.SelfReflection.Temperature},
		{"tts.elevenlabs.voice_settings.stability", cfg.TTS.ElevenLabs.VoiceSettings.Stability},
		{"tts.elevenlabs.voice_settings.similarity_boost", cfg.TTS.ElevenLabs.VoiceSettings.SimilarityBoost},
		{"tts.elevenlabs.voice_settings.style", cfg.TTS.ElevenLabs.VoiceSettings.Style},
	}

	for _, c := range tempChecks {
		if c.value < 0.0 || c.value > 2.0 {
			errs = append(errs, &ValidationError{
				Path:    c.path,
				Message: fmt.Sprintf("value %.2f must be in range [0.0, 2.0]", c.value),
			})
		}
	}

	// ElevenLabs speed: [0.7, 1.2]
	speed := cfg.TTS.ElevenLabs.VoiceSettings.Speed
	if speed < 0.7 || speed > 1.2 {
		errs = append(errs, &ValidationError{
			Path:    "tts.elevenlabs.voice_settings.speed",
			Message: fmt.Sprintf("speed %.2f must be in range [0.7, 1.2]", speed),
		})
	}

	return errs
}

func (v *RangeValidator) validateCircuitBreaker(cfg *ConfigV2) []error {
	var errs []error

	cb := cfg.LLM.CircuitBreaker
	if cb.MaxFailures < 1 || cb.MaxFailures > 100 {
		errs = append(errs, &ValidationError{
			Path:    "llm.circuit_breaker.max_failures",
			Message: fmt.Sprintf("max_failures %d must be in range [1, 100]", cb.MaxFailures),
		})
	}
	if cb.CooldownSeconds < 1 || cb.CooldownSeconds > 3600 {
		errs = append(errs, &ValidationError{
			Path:    "llm.circuit_breaker.cooldown_seconds",
			Message: fmt.Sprintf("cooldown_seconds %d must be in range [1, 3600]", cb.CooldownSeconds),
		})
	}

	return errs
}

func (v *RangeValidator) validateProbabilities(cfg *ConfigV2) []error {
	var errs []error

	probChecks := []struct {
		path  string
		value float64
	}{
		{"free_will.mood_update_probability", cfg.FreeWill.MoodUpdateProbability},
		{"free_will.voice_probability", cfg.FreeWill.VoiceProbability},
		{"free_will.reactions.probability", cfg.FreeWill.Reactions.Probability},
		{"anti_repetition.similarity_threshold", cfg.AntiRepetition.SimilarityThreshold},
		{"causal_learning.min_confidence", cfg.CausalLearning.MinConfidence},
		{"social_architecture.intimacy_growth_rate", cfg.SocialArchitecture.IntimacyGrowthRate},
		{"social_architecture.trust_decay_rate", cfg.SocialArchitecture.TrustDecayRate},
		{"reactions.clown.response_probability", float64(cfg.Reactions.Clown.ResponseProbability)},
	}

	for _, c := range probChecks {
		if c.value < 0.0 || c.value > 1.0 {
			errs = append(errs, &ValidationError{
				Path:    c.path,
				Message: fmt.Sprintf("probability %.2f must be in range [0.0, 1.0]", c.value),
			})
		}
	}

	// Clown response probability: [0, 100]
	clownProb := cfg.Reactions.Clown.ResponseProbability
	if clownProb < 0 || clownProb > 100 {
		errs = append(errs, &ValidationError{
			Path:    "reactions.clown.response_probability",
			Message: fmt.Sprintf("response_probability %d must be in range [0, 100]", clownProb),
		})
	}

	return errs
}

// ============================================================================
// RequiredFieldsValidator
// ============================================================================

// RequiredFieldsValidator проверяет обязательные поля.
type RequiredFieldsValidator struct{}

func (v *RequiredFieldsValidator) Validate(cfg *ConfigV2) []error {
	if cfg == nil {
		return []error{&ValidationError{Path: "", Message: "config is nil"}}
	}
	var errs []error

	// TELEGRAM_TOKEN не пустой
	if cfg.Telegram.Token == "" {
		errs = append(errs, &ValidationError{
			Path:    "telegram.token",
			Message: "TELEGRAM_TOKEN must not be empty",
		})
	}

	// Для включённых LLM провайдеров API-ключ не пустой
	for name, p := range cfg.LLM.Providers {
		if p.Enabled && p.APIKey == "" {
			errs = append(errs, &ValidationError{
				Path:    fmt.Sprintf("llm.providers.%s.api_key", name),
				Message: fmt.Sprintf("provider '%s' is enabled but api_key is empty", name),
			})
		}
	}

	// TTS провайдер: если указан, API-ключ не пустой
	if cfg.TTS.Provider == "elevenlabs" && cfg.TTS.ElevenLabs.APIKey == "" {
		errs = append(errs, &ValidationError{
			Path:    "tts.elevenlabs.api_key",
			Message: "TTS provider is elevenlabs but api_key is empty",
		})
	}
	if cfg.TTS.Provider == "gemini_tts" {
		gemini, ok := cfg.LLM.Providers["gemini"]
		if !ok || !gemini.Enabled || gemini.APIKey == "" {
			errs = append(errs, &ValidationError{
				Path:    "tts.provider",
				Message: "TTS provider is gemini_tts but gemini LLM provider is not configured or missing API key",
			})
		}
	}

	// WebSearch: если включен, нужен Google API ключ
	if cfg.WebSearch.Enabled && cfg.WebSearch.GoogleAPIKey == "" {
		errs = append(errs, &ValidationError{
			Path:    "web_search.google_api_key",
			Message: "web_search is enabled but google_api_key is empty",
		})
	}
	if cfg.WebSearch.Enabled && cfg.WebSearch.SearchEngineID == "" {
		errs = append(errs, &ValidationError{
			Path:    "web_search.search_engine_id",
			Message: "web_search is enabled but search_engine_id is empty",
		})
	}

	// Storage-specific required fields
	switch cfg.Storage.Type {
	case "postgres":
		if cfg.Storage.PostgreSQL.Host == "" {
			errs = append(errs, &ValidationError{Path: "storage.postgresql.host", Message: "storage type is postgres but host is empty"})
		}
		if cfg.Storage.PostgreSQL.User == "" {
			errs = append(errs, &ValidationError{Path: "storage.postgresql.user", Message: "storage type is postgres but user is empty"})
		}
		if cfg.Storage.PostgreSQL.DBName == "" {
			errs = append(errs, &ValidationError{Path: "storage.postgresql.dbname", Message: "storage type is postgres but dbname is empty"})
		}
	case "mongo":
		if cfg.Storage.MongoDB.URI == "" {
			errs = append(errs, &ValidationError{Path: "storage.mongodb.uri", Message: "storage type is mongo but uri is empty"})
		}
		if cfg.Storage.MongoDB.DBName == "" {
			errs = append(errs, &ValidationError{Path: "storage.mongodb.dbname", Message: "storage type is mongo but dbname is empty"})
		}
	}

	return errs
}

// ============================================================================
// ConsistencyValidator
// ============================================================================

// ConsistencyValidator проверяет консистентность конфигурации.
type ConsistencyValidator struct{}

func (v *ConsistencyValidator) Validate(cfg *ConfigV2) []error {
	if cfg == nil {
		return []error{&ValidationError{Path: "", Message: "config is nil"}}
	}
	var errs []error

	errs = append(errs, v.validateAudioTranscriber(cfg)...)
	errs = append(errs, v.validateAudioGenerator(cfg)...)
	errs = append(errs, v.validateLongTermMemory(cfg)...)
	errs = append(errs, v.validateMongoCleanup(cfg)...)
	errs = append(errs, v.validateVoiceMessages(cfg)...)

	return errs
}

// validateAudioTranscriber проверяет, что если голосовые сообщения включены,
// есть провайдер с моделью для аудио (AudioTranscriber capability).
func (v *ConsistencyValidator) validateAudioTranscriber(cfg *ConfigV2) []error {
	var errs []error

	if !cfg.VoiceMessages.Enabled {
		return errs
	}

	hasAudioProvider := false
	for _, p := range cfg.LLM.Providers {
		if p.Enabled && p.Models != nil && p.Models["audio"] != "" {
			hasAudioProvider = true
			break
		}
	}

	if !hasAudioProvider {
		errs = append(errs, &ValidationError{
			Path:    "voice_messages.enabled",
			Message: "voice_messages is enabled but no LLM provider has an audio model configured (AudioTranscriber capability)",
		})
	}

	return errs
}

// validateAudioGenerator проверяет, что если TTS провайдер указан,
// соответствующий провайдер сконфигурирован (AudioGenerator capability).
func (v *ConsistencyValidator) validateAudioGenerator(cfg *ConfigV2) []error {
	var errs []error

	if cfg.TTS.Provider == "" {
		return errs
	}

	switch cfg.TTS.Provider {
	case "elevenlabs":
		if cfg.TTS.ElevenLabs.APIKey == "" {
			errs = append(errs, &ValidationError{
				Path:    "tts.elevenlabs.api_key",
				Message: "TTS provider is elevenlabs but api_key is not configured",
			})
		}
	case "gemini_tts":
		gemini, ok := cfg.LLM.Providers["gemini"]
		if !ok || !gemini.Enabled {
			errs = append(errs, &ValidationError{
				Path:    "tts.provider",
				Message: "TTS provider is gemini_tts but gemini LLM provider is not enabled",
			})
		}
	}

	return errs
}

// validateLongTermMemory проверяет, что для долгосрочной памяти
// настроен embedding-провайдер.
func (v *ConsistencyValidator) validateLongTermMemory(cfg *ConfigV2) []error {
	var errs []error

	if !cfg.Storage.LongTermMemory.Enabled {
		return errs
	}

	if cfg.Storage.LongTermMemory.EmbeddingModel == "" {
		errs = append(errs, &ValidationError{
			Path:    "storage.long_term_memory.embedding_model",
			Message: "long_term_memory is enabled but embedding_model is empty",
		})
	}

	hasEmbeddingProvider := false
	for _, p := range cfg.LLM.Providers {
		if p.Enabled && p.Models != nil && p.Models["embedding"] != "" {
			hasEmbeddingProvider = true
			break
		}
	}

	if !hasEmbeddingProvider {
		errs = append(errs, &ValidationError{
			Path:    "storage.long_term_memory.enabled",
			Message: "long_term_memory is enabled but no LLM provider has an embedding model configured",
		})
	}

	return errs
}

// validateMongoCleanup проверяет, что автоочистка MongoDB включена только
// при использовании MongoDB как типа хранилища.
func (v *ConsistencyValidator) validateMongoCleanup(cfg *ConfigV2) []error {
	var errs []error

	if cfg.Storage.Cleanup.Enabled && cfg.Storage.Type != "mongo" {
		errs = append(errs, &ValidationError{
			Path:    "storage.cleanup.enabled",
			Message: fmt.Sprintf("cleanup is enabled but storage type is '%s', expected 'mongo'", cfg.Storage.Type),
		})
	}

	return errs
}

// validateVoiceMessages проверяет консистентность интервалов голосовых сообщений.
func (v *ConsistencyValidator) validateVoiceMessages(cfg *ConfigV2) []error {
	var errs []error

	if cfg.VoiceMessages.Interval.Min > cfg.VoiceMessages.Interval.Max {
		errs = append(errs, &ValidationError{
			Path:    "voice_messages.interval",
			Message: fmt.Sprintf("min (%d) must be <= max (%d)", cfg.VoiceMessages.Interval.Min, cfg.VoiceMessages.Interval.Max),
		})
	}

	if cfg.FreeWill.Intervals.MinMinutes > cfg.FreeWill.Intervals.MaxMinutes {
		errs = append(errs, &ValidationError{
			Path:    "free_will.intervals",
			Message: fmt.Sprintf("min_minutes (%.2f) must be <= max_minutes (%.2f)", cfg.FreeWill.Intervals.MinMinutes, cfg.FreeWill.Intervals.MaxMinutes),
		})
	}

	if cfg.FreeWill.Silence.MinMinutes > cfg.FreeWill.Silence.MaxMinutes {
		errs = append(errs, &ValidationError{
			Path:    "free_will.silence",
			Message: fmt.Sprintf("min_minutes (%.2f) must be <= max_minutes (%.2f)", cfg.FreeWill.Silence.MinMinutes, cfg.FreeWill.Silence.MaxMinutes),
		})
	}

	return errs
}

// ============================================================================
// ValidateConfig for ConfigV2
// ============================================================================

// ValidateConfigV2 запускает все валидаторы и возвращает список ошибок.
func ValidateConfigV2(cfg *ConfigV2) []error {
	validators := []Validator{
		&StructureValidator{},
		&TypeValidator{},
		&RangeValidator{},
		&RequiredFieldsValidator{},
		&ConsistencyValidator{},
	}
	var errs []error
	for _, v := range validators {
		errs = append(errs, v.Validate(cfg)...)
	}
	return errs
}
