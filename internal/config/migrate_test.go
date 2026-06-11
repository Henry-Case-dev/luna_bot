package config

import "testing"

func TestMigrateConfig_BasicFields(t *testing.T) {
	old := &Config{
		LLMProvider:        "gemini",
		LLMFallbackEnabled: true,
		TelegramToken:      "test-token",
		Debug:              true,
	}
	cfg := MigrateConfig(old)
	if cfg.LLM.DefaultProvider != "gemini" {
		t.Errorf("expected DefaultProvider='gemini', got '%s'", cfg.LLM.DefaultProvider)
	}
	if cfg.LLM.FallbackEnabled != true {
		t.Error("expected FallbackEnabled=true")
	}
	if cfg.Telegram.Token != "test-token" {
		t.Errorf("expected Token='test-token', got '%s'", cfg.Telegram.Token)
	}
	if cfg.Telegram.Debug != true {
		t.Error("expected Telegram.Debug=true")
	}
}

func TestMigrateConfig_DefaultProvider(t *testing.T) {
	old := &Config{}
	cfg := MigrateConfig(old)
	if cfg.LLM.DefaultProvider != "gemini" {
		t.Errorf("expected empty provider to default to 'gemini', got '%s'", cfg.LLM.DefaultProvider)
	}
}

func TestMigrateConfig_GeminiProvider(t *testing.T) {
	old := &Config{
		GeminiAPIKey:              "gemini-key",
		GeminiAPIKeyReserve:       "reserve-key",
		GeminiKeyRotationTimeHours: 2,
		GeminiModelName:           "gemini-2.0-flash",
		GeminiTemperatureNormal:   1.0,
		GeminiTemperatureSerious:  0.8,
		GeminiBypassSafetyFilters: true,
		GeminiObfuscatePrompts:    false,
	}
	cfg := MigrateConfig(old)
	p, ok := cfg.LLM.Providers["gemini"]
	if !ok {
		t.Fatal("expected 'gemini' provider to exist")
	}
	if p.APIKey != "gemini-key" {
		t.Errorf("expected APIKey='gemini-key', got '%s'", p.APIKey)
	}
	if p.ReserveAPIKey != "reserve-key" {
		t.Errorf("expected ReserveAPIKey='reserve-key', got '%s'", p.ReserveAPIKey)
	}
	if p.KeyRotationHours != 2 {
		t.Errorf("expected KeyRotationHours=2, got %d", p.KeyRotationHours)
	}
	if p.Enabled != true {
		t.Error("expected Gemini provider to be enabled when API key is set")
	}
	if p.Safety.BypassFilters != true {
		t.Error("expected BypassFilters=true")
	}
	if p.Safety.Obfuscate != false {
		t.Error("expected Obfuscate=false")
	}
	if p.Models["text"] != "gemini-2.0-flash" {
		t.Errorf("expected text model='gemini-2.0-flash', got '%s'", p.Models["text"])
	}
	if p.Temperatures["normal"] != 1.0 {
		t.Errorf("expected normal temperature=1.0, got %f", p.Temperatures["normal"])
	}
	if p.Temperatures["serious"] != 0.8 {
		t.Errorf("expected serious temperature=0.8, got %f", p.Temperatures["serious"])
	}
}

func TestMigrateConfig_GeminiProviderDisabled(t *testing.T) {
	old := &Config{}
	cfg := MigrateConfig(old)
	p := cfg.LLM.Providers["gemini"]
	if p.Enabled != false {
		t.Error("expected Gemini provider to be disabled when no API key")
	}
}

func TestMigrateConfig_DeepSeekProvider(t *testing.T) {
	old := &Config{
		DeepSeekAPIKey:    "ds-key",
		DeepSeekModelName: "deepseek-chat",
		DeepSeekBaseURL:   "https://api.deepseek.com/v1",
	}
	cfg := MigrateConfig(old)
	p, ok := cfg.LLM.Providers["deepseek"]
	if !ok {
		t.Fatal("expected 'deepseek' provider to exist")
	}
	if p.Enabled != true {
		t.Error("expected DeepSeek provider enabled")
	}
	if p.APIKey != "ds-key" {
		t.Errorf("expected APIKey='ds-key', got '%s'", p.APIKey)
	}
	if p.BaseURL != "https://api.deepseek.com/v1" {
		t.Errorf("expected BaseURL='https://api.deepseek.com/v1', got '%s'", p.BaseURL)
	}
	if p.Models["text"] != "deepseek-chat" {
		t.Errorf("expected text model='deepseek-chat', got '%s'", p.Models["text"])
	}
}

func TestMigrateConfig_OpenRouterProvider(t *testing.T) {
	old := &Config{
		OpenRouterAPIKey:    "or-key",
		OpenRouterModelName: "deepseek/deepseek-chat",
		OpenRouterSiteURL:   "https://example.com",
		OpenRouterSiteTitle: "Luna Bot",
	}
	cfg := MigrateConfig(old)
	p, ok := cfg.LLM.Providers["openrouter"]
	if !ok {
		t.Fatal("expected 'openrouter' provider to exist")
	}
	if p.APIKey != "or-key" {
		t.Errorf("expected APIKey='or-key', got '%s'", p.APIKey)
	}
	if p.SiteURL != "https://example.com" {
		t.Errorf("expected SiteURL='https://example.com', got '%s'", p.SiteURL)
	}
	if p.SiteTitle != "Luna Bot" {
		t.Errorf("expected SiteTitle='Luna Bot', got '%s'", p.SiteTitle)
	}
}

func TestMigrateConfig_CircuitBreaker(t *testing.T) {
	old := &Config{}
	cfg := MigrateConfig(old)
	cb := cfg.LLM.CircuitBreaker
	if cb.MaxFailures != 5 {
		t.Errorf("expected MaxFailures=5, got %d", cb.MaxFailures)
	}
	if cb.CooldownSeconds != 60 {
		t.Errorf("expected CooldownSeconds=60, got %d", cb.CooldownSeconds)
	}
}

func TestMigrateConfig_TTS(t *testing.T) {
	old := &Config{
		ElevenLabsAPIKey:  "el-key",
		ElevenLabsVoiceID: "voice-123",
		ElevenLabsModel:   "eleven_multilingual_v2",
		ElevenLabsPlan:    "starter",
		ElevenLabsStability:       0.5,
		ElevenLabsSimilarityBoost: 0.8,
		ElevenLabsStyle:           0.0,
		ElevenLabsUseSpeakerBoost: true,
		ElevenLabsSpeed:           1.0,
		ElevenLabsRandomVoice:     false,
		ElevenLabsStylePrompt:     "style-prompt",
		ElevenLabsEmotionPrompt:   "emotion-prompt",
		ElevenLabsPacePrompt:      "pace-prompt",
	}
	cfg := MigrateConfig(old)
	if cfg.TTS.Provider != "elevenlabs" {
		t.Errorf("expected TTS provider='elevenlabs', got '%s'", cfg.TTS.Provider)
	}
	el := cfg.TTS.ElevenLabs
	if el.APIKey != "el-key" {
		t.Errorf("expected ElevenLabs APIKey='el-key', got '%s'", el.APIKey)
	}
	if el.VoiceID != "voice-123" {
		t.Errorf("expected VoiceID='voice-123', got '%s'", el.VoiceID)
	}
	if el.VoiceSettings.Stability != 0.5 {
		t.Errorf("expected Stability=0.5, got %f", el.VoiceSettings.Stability)
	}
	if el.VoiceSettings.SimilarityBoost != 0.8 {
		t.Errorf("expected SimilarityBoost=0.8, got %f", el.VoiceSettings.SimilarityBoost)
	}
	if el.VoiceSettings.UseSpeakerBoost != true {
		t.Error("expected UseSpeakerBoost=true")
	}
	if el.RandomVoice != false {
		t.Error("expected RandomVoice=false")
	}
	if el.Prompts.Style != "style-prompt" {
		t.Errorf("expected StylePrompt='style-prompt', got '%s'", el.Prompts.Style)
	}
	if el.Prompts.Emotion != "emotion-prompt" {
		t.Errorf("expected EmotionPrompt='emotion-prompt', got '%s'", el.Prompts.Emotion)
	}
}

func TestMigrateConfig_Telegram(t *testing.T) {
	old := &Config{
		TelegramToken:              "bot-token",
		BotNames:                   []string{"Luna", "luna"},
		AdminUsernames:             []string{"admin"},
		AdminID:                    12345,
		Debug:                      true,
		TimeZone:                   "UTC",
		ErrorMessageAutoDeleteSeconds: 10,
	}
	cfg := MigrateConfig(old)
	if cfg.Telegram.Token != "bot-token" {
		t.Errorf("expected Token='bot-token', got '%s'", cfg.Telegram.Token)
	}
	if len(cfg.Telegram.BotNames) != 2 || cfg.Telegram.BotNames[0] != "Luna" {
		t.Error("expected BotNames=[Luna, luna]")
	}
	if len(cfg.Telegram.AdminIDs) != 1 || cfg.Telegram.AdminIDs[0] != 12345 {
		t.Error("expected AdminIDs=[12345]")
	}
	if cfg.Telegram.Timezone != "UTC" {
		t.Errorf("expected Timezone='UTC', got '%s'", cfg.Telegram.Timezone)
	}
	if cfg.Telegram.ErrorAutoDeleteSeconds != 10 {
		t.Errorf("expected ErrorAutoDeleteSeconds=10, got %d", cfg.Telegram.ErrorAutoDeleteSeconds)
	}
}

func TestMigrateConfig_FreeWill(t *testing.T) {
	old := &Config{
		FreeWillEnabled:                true,
		FreeWillMinIntervalMinutes:     15,
		FreeWillMaxIntervalMinutes:     60,
		FreeWillContextWindow:          50,
		FreeWillMoodUpdateProbability:  0.1,
		FreeWillMaxDecisionsPerHour:    10,
		FreeWillVoiceProbability:       0.3,
		FreeWillSilenceMinMinutes:      3.0,
		FreeWillSilenceMaxMinutes:      20.0,
		FreeWillReactionsEnabled:       true,
		FreeWillReactionsProbability:   0.2,
		FreeWillReactionsCooldownMinutes: 5,
		FreeWillReactionsMaxPerHour:    15,
		FreeWillDirectResponseMaxPerHour:         30,
		FreeWillDirectResponseMinIntervalSeconds: 5.0,
		FreeWillDirectResponseIndependentLimits:  true,
		IntervalMessagesEnabled: true,
	}
	cfg := MigrateConfig(old)
	if cfg.FreeWill.Enabled != true {
		t.Error("expected FreeWill.Enabled=true")
	}
	if cfg.FreeWill.Intervals.MinMinutes != 15 {
		t.Errorf("expected Intervals.MinMinutes=15, got %f", cfg.FreeWill.Intervals.MinMinutes)
	}
	if cfg.FreeWill.Intervals.MaxMinutes != 60 {
		t.Errorf("expected Intervals.MaxMinutes=60, got %f", cfg.FreeWill.Intervals.MaxMinutes)
	}
	if cfg.FreeWill.ContextWindow != 50 {
		t.Errorf("expected ContextWindow=50, got %d", cfg.FreeWill.ContextWindow)
	}
	if cfg.FreeWill.MoodUpdateProbability != 0.1 {
		t.Errorf("expected MoodUpdateProbability=0.1, got %f", cfg.FreeWill.MoodUpdateProbability)
	}
	if cfg.FreeWill.MaxDecisionsPerHour != 10 {
		t.Errorf("expected MaxDecisionsPerHour=10, got %d", cfg.FreeWill.MaxDecisionsPerHour)
	}
	if cfg.FreeWill.Silence.MinMinutes != 3.0 {
		t.Errorf("expected Silence.MinMinutes=3.0, got %f", cfg.FreeWill.Silence.MinMinutes)
	}
	if cfg.FreeWill.Silence.MaxMinutes != 20.0 {
		t.Errorf("expected Silence.MaxMinutes=20.0, got %f", cfg.FreeWill.Silence.MaxMinutes)
	}
	if cfg.FreeWill.Reactions.Enabled != true {
		t.Error("expected Reactions.Enabled=true")
	}
	if cfg.FreeWill.Reactions.Probability != 0.2 {
		t.Errorf("expected Reactions.Probability=0.2, got %f", cfg.FreeWill.Reactions.Probability)
	}
	if cfg.FreeWill.DirectResponse.MaxPerHour != 30 {
		t.Errorf("expected DirectResponse.MaxPerHour=30, got %d", cfg.FreeWill.DirectResponse.MaxPerHour)
	}
	if cfg.FreeWill.DirectResponse.IndependentLimits != true {
		t.Error("expected DirectResponse.IndependentLimits=true")
	}
	if cfg.FreeWill.IntervalMessages.Enabled != true {
		t.Error("expected IntervalMessages.Enabled=true")
	}
}

func TestMigrateConfig_VoiceMessages(t *testing.T) {
	old := &Config{
		VoiceMessagesEnabled: true,
		MinVoiceMessages:     50,
		MaxVoiceMessages:     100,
		VoiceMessageTempDir:  "/tmp/voice",
	}
	cfg := MigrateConfig(old)
	if cfg.VoiceMessages.Enabled != true {
		t.Error("expected VoiceMessages.Enabled=true")
	}
	if cfg.VoiceMessages.Interval.Min != 50 {
		t.Errorf("expected Interval.Min=50, got %d", cfg.VoiceMessages.Interval.Min)
	}
	if cfg.VoiceMessages.Interval.Max != 100 {
		t.Errorf("expected Interval.Max=100, got %d", cfg.VoiceMessages.Interval.Max)
	}
	if cfg.VoiceMessages.TempDir != "/tmp/voice" {
		t.Errorf("expected TempDir='/tmp/voice', got '%s'", cfg.VoiceMessages.TempDir)
	}
}

func TestMigrateConfig_Moderation(t *testing.T) {
	old := &Config{
		ModEnabled:          true,
		ModInterval:         5,
		ModMuteTimeMin:      10,
		ModKickTimeMin:      1,
		ModBanTimeMin:       60,
		ModCheckAdminRights: true,
		ModDefaultNotify:    false,
	}
	cfg := MigrateConfig(old)
	if cfg.Moderation.Enabled != true {
		t.Error("expected Moderation.Enabled=true")
	}
	if cfg.Moderation.IntervalMinutes != 5 {
		t.Errorf("expected IntervalMinutes=5, got %d", cfg.Moderation.IntervalMinutes)
	}
	if cfg.Moderation.MuteTimeMinutes != 10 {
		t.Errorf("expected MuteTimeMinutes=10, got %d", cfg.Moderation.MuteTimeMinutes)
	}
	if cfg.Moderation.CheckAdminRights != true {
		t.Error("expected CheckAdminRights=true")
	}
}

func TestMigrateConfig_AntiRepetition(t *testing.T) {
	old := &Config{
		AntiRepetitionEnabled:                true,
		AntiRepetitionMaxResponsesPerChat:    20,
		AntiRepetitionSimilarityThreshold:    0.75,
		AntiRepetitionTimeWindowHours:        24,
		AntiRepetitionCleanupIntervalHours:   1,
		AntiRepetitionReworkEnabled:          true,
		AntiRepetitionMaxReworkAttempts:      2,
		AntiRepetitionReworkTemperature:      0.8,
		AntiRepetitionLocalReworkEnabled:     true,
		AntiRepetitionLocalReworkMaxLength:   50,
	}
	cfg := MigrateConfig(old)
	if cfg.AntiRepetition.Enabled != true {
		t.Error("expected AntiRepetition.Enabled=true")
	}
	if cfg.AntiRepetition.SimilarityThreshold != 0.75 {
		t.Errorf("expected SimilarityThreshold=0.75, got %f", cfg.AntiRepetition.SimilarityThreshold)
	}
	if cfg.AntiRepetition.Rework.Enabled != true {
		t.Error("expected Rework.Enabled=true")
	}
	if cfg.AntiRepetition.Rework.MaxAttempts != 2 {
		t.Errorf("expected Rework.MaxAttempts=2, got %d", cfg.AntiRepetition.Rework.MaxAttempts)
	}
	if cfg.AntiRepetition.Rework.LocalRework.Enabled != true {
		t.Error("expected LocalRework.Enabled=true")
	}
	if cfg.AntiRepetition.Rework.LocalRework.MaxLength != 50 {
		t.Errorf("expected LocalRework.MaxLength=50, got %d", cfg.AntiRepetition.Rework.LocalRework.MaxLength)
	}
}

func TestMigrateConfig_Disambiguation(t *testing.T) {
	old := &Config{
		DisambiguationEnabled: true,
	}
	cfg := MigrateConfig(old)
	if cfg.Disambiguation.Enabled != true {
		t.Error("expected Disambiguation.Enabled=true")
	}
}

func TestMigrateConfig_AutoBio(t *testing.T) {
	old := &Config{
		AutoBioEnabled:                true,
		AutoBioIntervalHours:          24,
		AutoBioMessagesLookbackDays:   30,
		AutoBioMinMessagesForAnalysis: 10,
		AutoBioMaxMessagesForAnalysis: 1000,
	}
	cfg := MigrateConfig(old)
	if cfg.AutoBio.Enabled != true {
		t.Error("expected AutoBio.Enabled=true")
	}
	if cfg.AutoBio.IntervalHours != 24 {
		t.Errorf("expected IntervalHours=24, got %d", cfg.AutoBio.IntervalHours)
	}
	if cfg.AutoBio.LookbackDays != 30 {
		t.Errorf("expected LookbackDays=30, got %d", cfg.AutoBio.LookbackDays)
	}
}

func TestMigrateConfig_Personality(t *testing.T) {
	old := &Config{
		PersonalityUpdateIntervalHours: 1,
		PersonalityMessagesLookback:    50,
		MaxNameMentions:                10,
		MaxRecentTopics:                10,
		MaxSelfPerceptions:             5,
		MaxDiscussionContexts:          3,
	}
	cfg := MigrateConfig(old)
	if cfg.Personality.UpdateIntervalHours != 1 {
		t.Errorf("expected UpdateIntervalHours=1, got %d", cfg.Personality.UpdateIntervalHours)
	}
	if cfg.Personality.MessagesLookback != 50 {
		t.Errorf("expected MessagesLookback=50, got %d", cfg.Personality.MessagesLookback)
	}
	if cfg.Personality.MaxNameMentions != 10 {
		t.Errorf("expected MaxNameMentions=10, got %d", cfg.Personality.MaxNameMentions)
	}
}

func TestMigrateConfig_Reactions(t *testing.T) {
	old := &Config{
		ReactionsEnabled:         true,
		ClownResponseProbability: 40,
		ClownCooldownSeconds:     30,
		MaxClownResponsesPerHour: 10,
	}
	cfg := MigrateConfig(old)
	if cfg.Reactions.Enabled != true {
		t.Error("expected Reactions.Enabled=true")
	}
	if cfg.Reactions.Clown.ResponseProbability != 40 {
		t.Errorf("expected ResponseProbability=40, got %d", cfg.Reactions.Clown.ResponseProbability)
	}
	if cfg.Reactions.Clown.CooldownSeconds != 30 {
		t.Errorf("expected CooldownSeconds=30, got %d", cfg.Reactions.Clown.CooldownSeconds)
	}
}

func TestMigrateConfig_WebSearch(t *testing.T) {
	old := &Config{
		WebSearchEnabled:     true,
		GoogleSearchAPIKey:   "g-key",
		GoogleSearchEngineID: "engine-id",
		WebSearchMaxResults:  5,
	}
	cfg := MigrateConfig(old)
	if cfg.WebSearch.Enabled != true {
		t.Error("expected WebSearch.Enabled=true")
	}
	if cfg.WebSearch.GoogleAPIKey != "g-key" {
		t.Errorf("expected GoogleAPIKey='g-key', got '%s'", cfg.WebSearch.GoogleAPIKey)
	}
	if cfg.WebSearch.SearchEngineID != "engine-id" {
		t.Errorf("expected SearchEngineID='engine-id', got '%s'", cfg.WebSearch.SearchEngineID)
	}
	if cfg.WebSearch.MaxResults != 5 {
		t.Errorf("expected MaxResults=5, got %d", cfg.WebSearch.MaxResults)
	}
}

func TestMigrateConfig_CausalLearning(t *testing.T) {
	old := &Config{
		CausalLearningEnabled:           true,
		CausalAnalysisIntervalHours:     4,
		CausalMinConfidence:             0.3,
		CausalTemporalWindowMinutes:     60,
		CausalMaxEntriesPerChat:         500,
		CausalAnalysisLookbackMessages:  100,
	}
	cfg := MigrateConfig(old)
	if cfg.CausalLearning.Enabled != true {
		t.Error("expected CausalLearning.Enabled=true")
	}
	if cfg.CausalLearning.AnalysisIntervalHours != 4 {
		t.Errorf("expected AnalysisIntervalHours=4, got %d", cfg.CausalLearning.AnalysisIntervalHours)
	}
	if cfg.CausalLearning.MinConfidence != 0.3 {
		t.Errorf("expected MinConfidence=0.3, got %f", cfg.CausalLearning.MinConfidence)
	}
}

func TestMigrateConfig_EmotionalLearning(t *testing.T) {
	old := &Config{
		EmotionalLearningEnabled:          true,
		EmotionalAnalysisIntervalHours:    2,
		EmotionalAnalysisLookbackMessages: 100,
		EmotionalMemoryRetentionDays:      30,
	}
	cfg := MigrateConfig(old)
	if cfg.EmotionalLearning.Enabled != true {
		t.Error("expected EmotionalLearning.Enabled=true")
	}
	if cfg.EmotionalLearning.MemoryRetentionDays != 30 {
		t.Errorf("expected MemoryRetentionDays=30, got %d", cfg.EmotionalLearning.MemoryRetentionDays)
	}
}

func TestMigrateConfig_BeliefLearning(t *testing.T) {
	old := &Config{
		BeliefLearningEnabled:           true,
		BeliefAnalysisIntervalHours:     6,
		BeliefAnalysisLookbackMessages:  150,
	}
	cfg := MigrateConfig(old)
	if cfg.BeliefLearning.Enabled != true {
		t.Error("expected BeliefLearning.Enabled=true")
	}
	if cfg.BeliefLearning.AnalysisIntervalHours != 6 {
		t.Errorf("expected AnalysisIntervalHours=6, got %d", cfg.BeliefLearning.AnalysisIntervalHours)
	}
}

func TestMigrateConfig_CognitiveArchitecture(t *testing.T) {
	old := &Config{
		InternalMonologueEnabled:  true,
		InternalMonologueTemperature: 0.4,
		SelfReflectionEnabled:     true,
		SelfReflectionTemperature: 0.5,
		ConfidenceCalibrationEnabled: true,
	}
	cfg := MigrateConfig(old)
	if cfg.CognitiveArchitecture.InternalMonologue.Enabled != true {
		t.Error("expected InternalMonologue.Enabled=true")
	}
	if cfg.CognitiveArchitecture.InternalMonologue.Temperature != 0.4 {
		t.Errorf("expected InternalMonologue.Temperature=0.4, got %f", cfg.CognitiveArchitecture.InternalMonologue.Temperature)
	}
	if cfg.CognitiveArchitecture.SelfReflection.Enabled != true {
		t.Error("expected SelfReflection.Enabled=true")
	}
	if cfg.CognitiveArchitecture.SelfReflection.Temperature != 0.5 {
		t.Errorf("expected SelfReflection.Temperature=0.5, got %f", cfg.CognitiveArchitecture.SelfReflection.Temperature)
	}
	if cfg.CognitiveArchitecture.ConfidenceCalibration.Enabled != true {
		t.Error("expected ConfidenceCalibration.Enabled=true")
	}
}

func TestMigrateConfig_SocialArchitecture(t *testing.T) {
	old := &Config{
		RelationshipTrackingEnabled: true,
		SocialLearningEnabled:       true,
		IntimacyGrowthRate:          0.02,
		TrustDecayRate:              0.01,
	}
	cfg := MigrateConfig(old)
	if cfg.SocialArchitecture.RelationshipTracking.Enabled != true {
		t.Error("expected RelationshipTracking.Enabled=true")
	}
	if cfg.SocialArchitecture.SocialLearning.Enabled != true {
		t.Error("expected SocialLearning.Enabled=true")
	}
	if cfg.SocialArchitecture.IntimacyGrowthRate != 0.02 {
		t.Errorf("expected IntimacyGrowthRate=0.02, got %f", cfg.SocialArchitecture.IntimacyGrowthRate)
	}
	if cfg.SocialArchitecture.TrustDecayRate != 0.01 {
		t.Errorf("expected TrustDecayRate=0.01, got %f", cfg.SocialArchitecture.TrustDecayRate)
	}
}

func TestMigrateConfig_AssociationCloud(t *testing.T) {
	old := &Config{
		AssociationCloudEnabled:  true,
		AssociationCloudMaxNodes: 5000,
		AssociationCloudMaxEdges: 50000,
		AssociationCloudDecayDays: 30,
	}
	cfg := MigrateConfig(old)
	if cfg.AssociationCloud.Enabled != true {
		t.Error("expected AssociationCloud.Enabled=true")
	}
	if cfg.AssociationCloud.MaxNodes != 5000 {
		t.Errorf("expected MaxNodes=5000, got %d", cfg.AssociationCloud.MaxNodes)
	}
	if cfg.AssociationCloud.MaxEdges != 50000 {
		t.Errorf("expected MaxEdges=50000, got %d", cfg.AssociationCloud.MaxEdges)
	}
}

func TestMigrateConfig_Storage(t *testing.T) {
	old := &Config{
		StorageType:                 "mongo",
		PostgresqlHost:              "localhost",
		PostgresqlPort:              "5432",
		PostgresqlUser:              "postgres",
		PostgresqlPassword:          "pass",
		PostgresqlDbname:            "lunadb",
		MongoDbURI:                  "mongodb://localhost:27017",
		MongoDbName:                 "luna_bot",
		MongoDbMessagesCollection:   "messages",
		MongoDbUserProfilesCollection: "profiles",
		MongoDbSettingsCollection:   "settings",
		MongoVectorIndexName:        "vector_idx",
		LongTermMemoryEnabled:       true,
		GeminiEmbeddingModelName:    "embedding-001",
		LongTermMemoryFetchK:        5,
		BackfillBatchSize:           100,
		MongoCleanupEnabled:         true,
		MongoCleanupSizeLimitMB:     450,
		MongoCleanupIntervalMinutes: 60,
		MongoCleanupChunkDurationHours: 24,
	}
	cfg := MigrateConfig(old)
	if cfg.Storage.Type != "mongo" {
		t.Errorf("expected Storage.Type='mongo', got '%s'", cfg.Storage.Type)
	}
	if cfg.Storage.PostgreSQL.Host != "localhost" {
		t.Errorf("expected PostgreSQL.Host='localhost', got '%s'", cfg.Storage.PostgreSQL.Host)
	}
	if cfg.Storage.PostgreSQL.Port != 5432 {
		t.Errorf("expected PostgreSQL.Port=5432, got %d", cfg.Storage.PostgreSQL.Port)
	}
	if cfg.Storage.PostgreSQL.User != "postgres" {
		t.Errorf("expected PostgreSQL.User='postgres', got '%s'", cfg.Storage.PostgreSQL.User)
	}
	if cfg.Storage.MongoDB.URI != "mongodb://localhost:27017" {
		t.Errorf("expected MongoDB.URI='mongodb://localhost:27017', got '%s'", cfg.Storage.MongoDB.URI)
	}
	if cfg.Storage.MongoDB.VectorIndexName != "vector_idx" {
		t.Errorf("expected MongoDB.VectorIndexName='vector_idx', got '%s'", cfg.Storage.MongoDB.VectorIndexName)
	}
	if cfg.Storage.LongTermMemory.Enabled != true {
		t.Error("expected LongTermMemory.Enabled=true")
	}
	if cfg.Storage.LongTermMemory.EmbeddingModel != "embedding-001" {
		t.Errorf("expected EmbeddingModel='embedding-001', got '%s'", cfg.Storage.LongTermMemory.EmbeddingModel)
	}
	if cfg.Storage.LongTermMemory.FetchK != 5 {
		t.Errorf("expected FetchK=5, got %d", cfg.Storage.LongTermMemory.FetchK)
	}
	if cfg.Storage.LongTermMemory.Backfill.BatchSize != 100 {
		t.Errorf("expected Backfill.BatchSize=100, got %d", cfg.Storage.LongTermMemory.Backfill.BatchSize)
	}
	if cfg.Storage.Cleanup.Enabled != true {
		t.Error("expected Cleanup.Enabled=true")
	}
	if cfg.Storage.Cleanup.SizeLimitMB != 450 {
		t.Errorf("expected Cleanup.SizeLimitMB=450, got %d", cfg.Storage.Cleanup.SizeLimitMB)
	}
}

func TestMigrateConfig_StorageInvalidPort(t *testing.T) {
	old := &Config{
		PostgresqlPort: "invalid",
	}
	cfg := MigrateConfig(old)
	if cfg.Storage.PostgreSQL.Port != 0 {
		t.Errorf("expected Port=0 for invalid port string, got %d", cfg.Storage.PostgreSQL.Port)
	}
}

func TestMigrateConfig_Prompts(t *testing.T) {
	old := &Config{
		DefaultPrompt: "test default prompt",
	}
	cfg := MigrateConfig(old)
	if cfg.Prompts.Inline["default"] != "test default prompt" {
		t.Errorf("expected Inline['default']='test default prompt', got '%s'", cfg.Prompts.Inline["default"])
	}
}

func TestMigrateConfig_Prompts_FallbackDefaults(t *testing.T) {
	old := &Config{}
	cfg := MigrateConfig(old)
	if cfg.Prompts.Inline["default"] != "Ты - участник чата." {
		t.Errorf("expected default prompt fallback, got '%s'", cfg.Prompts.Inline["default"])
	}
	if cfg.Prompts.Inline["welcome"] != "Привет, чат! Я ваш новый спутник в беседе. Погнали!" {
		t.Errorf("expected welcome prompt fallback, got '%s'", cfg.Prompts.Inline["welcome"])
	}
	if cfg.Prompts.Inline["direct"] != "Отвечай прямо и по делу." {
		t.Errorf("expected direct prompt fallback, got '%s'", cfg.Prompts.Inline["direct"])
	}
}

func TestMigrateConfig_Prompts_AllMappings(t *testing.T) {
	old := &Config{
		DefaultPrompt:                           "dp",
		DirectPrompt:                            "dirp",
		DailyTakePrompt:                         "dtp",
		SummaryPrompt:                           "sum",
		WeeklySummaryPrompt:                     "wsum",
		VoiceMessagesPrompt:                     "vmp",
		WelcomePrompt:                           "wp",
		VoiceFormatPrompt:                       "vfp",
		DonatePrompt:                            "donp",
		RateLimitPrompt:                         "rlp",
		WebSearchTriggerPrompt:                  "wstp",
		ClassifyDirectMessagePrompt:             "cdmp",
		SeriousDirectPrompt:                     "sdp",
		PhotoAnalysisPrompt:                     "pap",
		AutoBioInitialAnalysisPrompt:            "abia",
		AutoBioUpdatePrompt:                     "abup",
		PersonalityAnalysisPrompt:               "pap2",
		PersonalityNameAnalysisPrompt:           "pnap",
		PersonalityTopicAnalysisPrompt:          "ptap",
		PersonalitySelfUpdatePrompt:             "psup",
		ClownReactionPrompt:                     "crp",
		ReactionAnalysisPrompt:                  "rap",
		AntiRepetitionReworkPrompt:              "arrp",
		ImageGenPrePrompt:                       "igpp",
		CausalAnalysisPrompt:                    "cap",
		CausalInfluencePrompt:                   "cip",
		EmotionalAnalysisPrompt:                 "eap",
		EmotionalAdaptationPrompt:               "eadp",
		EmotionalFeedbackPrompt:                 "efp",
		BeliefAnalysisPrompt:                    "bap",
		InternalMonologuePrompt:                 "imp",
		SelfReflectionPrompt:                    "srp",
		RelationshipAnalysisPrompt:              "rlp2",
	}
	cfg := MigrateConfig(old)
	tests := map[string]string{
		"default":                            "dp",
		"direct":                             "dirp",
		"daily_take":                         "dtp",
		"summary":                            "sum",
		"weekly_summary":                     "wsum",
		"voice_message":                      "vmp",
		"welcome":                            "wp",
		"voice_format":                       "vfp",
		"donate":                             "donp",
		"rate_limit":                         "rlp",
		"web_search_trigger":                 "wstp",
		"classify_direct_message":            "cdmp",
		"serious_direct":                     "sdp",
		"photo_analysis":                     "pap",
		"auto_bio_initial_analysis":          "abia",
		"auto_bio_update":                    "abup",
		"personality_analysis":               "pap2",
		"personality_name_analysis":          "pnap",
		"personality_topic_analysis":         "ptap",
		"personality_self_update":            "psup",
		"clown_reaction":                     "crp",
		"reaction_analysis":                  "rap",
		"anti_repetition_rework":             "arrp",
		"image_gen_pre_prompt":               "igpp",
		"causal_analysis":                    "cap",
		"causal_influence":                   "cip",
		"emotional_analysis":                 "eap",
		"emotional_adaptation":               "eadp",
		"emotional_feedback":                 "efp",
		"belief_analysis":                    "bap",
		"internal_monologue":                 "imp",
		"self_reflection":                    "srp",
		"relationship_analysis":              "rlp2",
	}
	for key, expected := range tests {
		if cfg.Prompts.Inline[key] != expected {
			t.Errorf("expected Inline[%s]='%s', got '%s'", key, expected, cfg.Prompts.Inline[key])
		}
	}
}

func TestMigrateConfig_NilConfig(t *testing.T) {
	cfg := MigrateConfig(nil)
	if cfg == nil {
		t.Error("expected non-nil ConfigV2")
	}
}

func TestMigrateConfig_NilConfig_Empty(t *testing.T) {
	cfg := MigrateConfig(nil)
	if cfg == nil {
		t.Fatal("expected non-nil ConfigV2 from nil input")
	}
	cfg2 := MigrateConfig(&Config{})
	if cfg2.LLM.DefaultProvider != "gemini" {
		t.Errorf("expected empty config to default LLM provider to 'gemini', got '%s'", cfg2.LLM.DefaultProvider)
	}
	if cfg2.LLM.CircuitBreaker.MaxFailures != 5 {
		t.Errorf("expected empty config CircuitBreaker.MaxFailures=5, got %d", cfg2.LLM.CircuitBreaker.MaxFailures)
	}
	if cfg2.LLM.CircuitBreaker.CooldownSeconds != 60 {
		t.Errorf("expected empty config CircuitBreaker.CooldownSeconds=60, got %d", cfg2.LLM.CircuitBreaker.CooldownSeconds)
	}
}

func TestMigrateConfig_MessagePostProcessor(t *testing.T) {
	old := &Config{
		MessagePostProcessorEnabled:                  true,
		MessagePostProcessorRandomizationEnabled:     true,
		MessagePostProcessorSingleWordProbability:    0.20,
		MessagePostProcessorShortSentencesProbability: 0.35,
		MessagePostProcessorLongMessagesProbability:  0.25,
		MessagePostProcessorMinLength:                10,
		MessagePostProcessorMaxLength:                2000,
		MessagePostProcessorLongMessageThreshold:     100,
		MessagePostProcessorForceLongProcessingThreshold: 200,
		MessagePostProcessorTimeoutSeconds:           15,
		MessagePostProcessorTemperature:              0.9,
		MessagePostProcessorCacheEnabled:             true,
		MessagePostProcessorCacheTTLMinutes:          30,
		MessagePostProcessorReplacementCacheEnabled:  true,
		MessagePostProcessorReplacementCacheTTLMinutes: 10,
		MessagePostProcessorExcludeTypes:             []string{"system", "error"},
		MessagePostProcessorWeeklySummaryExclude:     true,
		MessagePostProcessorDebugLogging:             true,
		MessagePostProcessorLogOriginalMessages:      true,
	}
	cfg := MigrateConfig(old)
	if cfg.MessagePostProcessor.Enabled != true {
		t.Error("expected MessagePostProcessor.Enabled=true")
	}
	if cfg.MessagePostProcessor.RandomizationEnabled != true {
		t.Error("expected RandomizationEnabled=true")
	}
	if cfg.MessagePostProcessor.Probabilities.SingleWord != 0.20 {
		t.Errorf("expected SingleWord=0.20, got %f", cfg.MessagePostProcessor.Probabilities.SingleWord)
	}
	if cfg.MessagePostProcessor.Probabilities.ShortSentences != 0.35 {
		t.Errorf("expected ShortSentences=0.35, got %f", cfg.MessagePostProcessor.Probabilities.ShortSentences)
	}
	if cfg.MessagePostProcessor.Length.Min != 10 {
		t.Errorf("expected Length.Min=10, got %d", cfg.MessagePostProcessor.Length.Min)
	}
	if cfg.MessagePostProcessor.Length.ForceLongProcessingThreshold != 200 {
		t.Errorf("expected ForceLongProcessingThreshold=200, got %d", cfg.MessagePostProcessor.Length.ForceLongProcessingThreshold)
	}
	if cfg.MessagePostProcessor.Performance.TimeoutSeconds != 15 {
		t.Errorf("expected TimeoutSeconds=15, got %d", cfg.MessagePostProcessor.Performance.TimeoutSeconds)
	}
	if cfg.MessagePostProcessor.Performance.Temperature != 0.9 {
		t.Errorf("expected Temperature=0.9, got %f", cfg.MessagePostProcessor.Performance.Temperature)
	}
	if cfg.MessagePostProcessor.Cache.Enabled != true {
		t.Error("expected Cache.Enabled=true")
	}
	if cfg.MessagePostProcessor.Cache.TTLMinutes != 30 {
		t.Errorf("expected Cache.TTLMinutes=30, got %d", cfg.MessagePostProcessor.Cache.TTLMinutes)
	}
	if cfg.MessagePostProcessor.Cache.ReplacementCache.Enabled != true {
		t.Error("expected ReplacementCache.Enabled=true")
	}
	if cfg.MessagePostProcessor.WeeklySummaryExclude != true {
		t.Error("expected WeeklySummaryExclude=true")
	}
	if cfg.MessagePostProcessor.Debug.Logging != true {
		t.Error("expected Debug.Logging=true")
	}
	if cfg.MessagePostProcessor.Debug.LogOriginalMessages != true {
		t.Error("expected Debug.LogOriginalMessages=true")
	}
	if len(cfg.MessagePostProcessor.ExcludeTypes) != 2 {
		t.Errorf("expected 2 exclude types, got %d", len(cfg.MessagePostProcessor.ExcludeTypes))
	}
}

func TestMigrateConfig_FreeWillImageGen(t *testing.T) {
	old := &Config{
		FreeWillImageGenerationMaxDecisionsPerInterval:    3,
		FreeWillImageGenerationIntervalHours:              6,
		FreeWillImageGenerationMinDecisionIntervalMinutes: 30,
		FreeWillImageGenerationIndependentLimits:          true,
		ImageGenFrequencyHours:                            12,
	}
	cfg := MigrateConfig(old)
	ig := cfg.FreeWill.ImageGeneration
	if ig.MaxPerInterval != 3 {
		t.Errorf("expected MaxPerInterval=3, got %d", ig.MaxPerInterval)
	}
	if ig.IntervalHours != 6 {
		t.Errorf("expected IntervalHours=6, got %d", ig.IntervalHours)
	}
	if ig.MinDecisionIntervalMin != 30 {
		t.Errorf("expected MinDecisionIntervalMin=30, got %d", ig.MinDecisionIntervalMin)
	}
	if ig.IndependentLimits != true {
		t.Error("expected IndependentLimits=true")
	}
	if ig.FrequencyHours != 12 {
		t.Errorf("expected FrequencyHours=12, got %d", ig.FrequencyHours)
	}
}

func TestMigrateConfig_FallbackSettings(t *testing.T) {
	old := &Config{
		LLMFallbackEnabled:       true,
		LLMFallbackCriticalTypes: []string{"type1", "type2"},
		LLMFallbackProviderOrder: []string{"gemini", "openrouter"},
	}
	cfg := MigrateConfig(old)
	if cfg.LLM.FallbackEnabled != true {
		t.Error("expected FallbackEnabled=true")
	}
	if len(cfg.LLM.FallbackCriticalTypes) != 2 {
		t.Errorf("expected 2 fallback critical types, got %d", len(cfg.LLM.FallbackCriticalTypes))
	}
	if cfg.LLM.FallbackCriticalTypes[0] != "type1" {
		t.Errorf("expected FallbackCriticalTypes[0]='type1', got '%s'", cfg.LLM.FallbackCriticalTypes[0])
	}
	if len(cfg.LLM.FallbackProviderOrder) != 2 {
		t.Errorf("expected 2 fallback providers, got %d", len(cfg.LLM.FallbackProviderOrder))
	}
}

func TestMigrateConfig_ResponseTypes(t *testing.T) {
	old := &Config{
		ResponseTypeConfigs: map[string]ResponseTypeConfig{
			"test_type": {
				Provider:    ProviderGemini,
				ModelName:   "test-model",
				Temperature: 0.5,
				Enabled:     true,
			},
		},
	}
	cfg := MigrateConfig(old)
	rt, ok := cfg.LLM.ResponseTypes["test_type"]
	if !ok {
		t.Fatal("expected 'test_type' response type to exist")
	}
	if rt.Provider != "gemini" {
		t.Errorf("expected Provider='gemini', got '%s'", rt.Provider)
	}
	if rt.Model != "test-model" {
		t.Errorf("expected Model='test-model', got '%s'", rt.Model)
	}
	if rt.Temperature != 0.5 {
		t.Errorf("expected Temperature=0.5, got %f", rt.Temperature)
	}
	if rt.Priority != 10 {
		t.Errorf("expected Priority=10, got %d", rt.Priority)
	}
}

func TestMigrateConfig_PromptRouting(t *testing.T) {
	old := &Config{
		CausalAnalysisPromptEnabled:     true,
		CausalAnalysisPromptProvider:    "deepseek",
		CausalAnalysisPromptModel:       "ds-model",
		CausalAnalysisPromptTemperature: 0.7,
	}
	cfg := MigrateConfig(old)
	rt, ok := cfg.LLM.ResponseTypes["causal_analysis"]
	if !ok {
		t.Fatal("expected 'causal_analysis' routing to exist")
	}
	if rt.Provider != "deepseek" {
		t.Errorf("expected Provider='deepseek', got '%s'", rt.Provider)
	}
	if rt.Model != "ds-model" {
		t.Errorf("expected Model='ds-model', got '%s'", rt.Model)
	}
	if rt.Temperature != 0.7 {
		t.Errorf("expected Temperature=0.7, got %f", rt.Temperature)
	}
	if rt.Priority != 5 {
		t.Errorf("expected Priority=5 for prompt routing, got %d", rt.Priority)
	}
}

func TestMigrateConfig_PromptRoutingDefaultProvider(t *testing.T) {
	old := &Config{
		CausalAnalysisPromptEnabled: true,
	}
	cfg := MigrateConfig(old)
	rt, ok := cfg.LLM.ResponseTypes["causal_analysis"]
	if !ok {
		t.Fatal("expected 'causal_analysis' routing to exist")
	}
	if rt.Provider != "gemini" {
		t.Errorf("expected default Provider='gemini', got '%s'", rt.Provider)
	}
}

func TestMigrateConfig_PromptRoutingDisabled(t *testing.T) {
	old := &Config{
		CausalAnalysisPromptEnabled: false,
	}
	cfg := MigrateConfig(old)
	_, ok := cfg.LLM.ResponseTypes["causal_analysis"]
	if ok {
		t.Error("expected 'causal_analysis' routing NOT to exist when disabled")
	}
}

func TestMigrateConfig_PromptRoutingNoOverride(t *testing.T) {
	old := &Config{
		ResponseTypeConfigs: map[string]ResponseTypeConfig{
			"causal_analysis": {
				Provider:    ProviderDeepSeek,
				ModelName:   "existing-model",
				Temperature: 0.5,
				Enabled:     true,
			},
		},
		CausalAnalysisPromptEnabled:     true,
		CausalAnalysisPromptProvider:    "gemini",
		CausalAnalysisPromptModel:       "should-not-override",
		CausalAnalysisPromptTemperature: 0.9,
	}
	cfg := MigrateConfig(old)
	rt := cfg.LLM.ResponseTypes["causal_analysis"]
	if rt.Provider != "deepseek" {
		t.Errorf("expected existing provider='deepseek' not overridden, got '%s'", rt.Provider)
	}
	if rt.Model != "existing-model" {
		t.Errorf("expected existing model='existing-model' not overridden, got '%s'", rt.Model)
	}
}

func TestMigrateConfig_GetRoutingProfile(t *testing.T) {
	old := &Config{
		LLMProvider: "gemini",
	}
	cfg := MigrateConfig(old)

	rp := cfg.GetRoutingProfile("non_existent")
	if rp.Provider != "gemini" {
		t.Errorf("expected fallback provider='gemini', got '%s'", rp.Provider)
	}
	if rp.Temperature != 1.0 {
		t.Errorf("expected fallback temperature=1.0, got %f", rp.Temperature)
	}
}

func TestMigrateConfig_GetRoutingProfile_WithDefault(t *testing.T) {
	cfg := MigrateConfig(&Config{})
	cfg.LLM.ResponseTypes["default"] = RoutingProfile{
		Provider:    "deepseek",
		Temperature: 0.7,
	}
	rp := cfg.GetRoutingProfile("unknown_type")
	if rp.Provider != "deepseek" {
		t.Errorf("expected 'default' profile provider='deepseek', got '%s'", rp.Provider)
	}
	if rp.Temperature != 0.7 {
		t.Errorf("expected 'default' profile temperature=0.7, got %f", rp.Temperature)
	}
}
