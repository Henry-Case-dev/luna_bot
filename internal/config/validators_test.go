package config

import (
	"strings"
	"testing"
	"time"
)

func makeValidConfig() *Config {
	return &Config{
		TelegramToken:                "test-token",
		LLMProvider:                  ProviderGemini,
		GeminiAPIKey:                 "test-api-key",
		GeminiModelName:              "gemini-2.0-flash",
		DailyTakeTime:                12,
		MinMessages:                  5,
		MaxMessages:                  50,
		ContextWindow:                300,
		SummaryIntervalHours:         0,
		StorageType:                  StorageTypeFile,
		AdminUsernames:               []string{"admin"},
		DefaultTemperature:           1.0,
		SeriousDirectPrompt:          "serious",
		DirectReplyLimitPrompt:       "limit",
		PromptEnterDirectLimitCount:  "count",
		PromptEnterDirectLimitDuration: "duration",
		ModInterval:                  5,
		ModPurgeDuration:             1 * time.Hour,
	}
}

func TestStructureValidator_ZeroConfig(t *testing.T) {
	cfg := &Config{}
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for zero-value config (missing required fields)")
	}
}

func TestRequiredFieldsValidator_MissingToken(t *testing.T) {
	cfg := makeValidConfig()
	cfg.TelegramToken = ""
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for missing TELEGRAM_TOKEN")
	}
	if !strings.Contains(err.Error(), "TELEGRAM_TOKEN") {
		t.Errorf("expected error message to mention TELEGRAM_TOKEN, got: %v", err)
	}
}

func TestRequiredFieldsValidator_MissingGeminiKey(t *testing.T) {
	cfg := makeValidConfig()
	cfg.GeminiAPIKey = ""
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for missing GEMINI_API_KEY when provider is gemini")
	}
}

func TestRequiredFieldsValidator_MissingGeminiModel(t *testing.T) {
	cfg := makeValidConfig()
	cfg.GeminiModelName = ""
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for missing GEMINI_MODEL_NAME when provider is gemini")
	}
}

func TestRequiredFieldsValidator_MissingDeepSeekKey(t *testing.T) {
	cfg := makeValidConfig()
	cfg.LLMProvider = ProviderDeepSeek
	cfg.DeepSeekAPIKey = ""
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for missing DEEPSEEK_API_KEY when provider is deepseek")
	}
}

func TestRequiredFieldsValidator_MissingDeepSeekModel(t *testing.T) {
	cfg := makeValidConfig()
	cfg.LLMProvider = ProviderDeepSeek
	cfg.DeepSeekAPIKey = "key"
	cfg.DeepSeekModelName = ""
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for missing DEEPSEEK_MODEL_NAME when provider is deepseek")
	}
}

func TestRequiredFieldsValidator_MissingOpenRouterKey(t *testing.T) {
	cfg := makeValidConfig()
	cfg.LLMProvider = ProviderOpenRouter
	cfg.OpenRouterAPIKey = ""
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for missing OPENROUTER_API_KEY when provider is openrouter")
	}
}

func TestRequiredFieldsValidator_MissingOpenRouterModel(t *testing.T) {
	cfg := makeValidConfig()
	cfg.LLMProvider = ProviderOpenRouter
	cfg.OpenRouterAPIKey = "key"
	cfg.OpenRouterModelName = ""
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for missing OPENROUTER_MODEL_NAME when provider is openrouter")
	}
}

func TestRequiredFieldsValidator_MissingAdminUsernames(t *testing.T) {
	cfg := makeValidConfig()
	cfg.AdminUsernames = []string{}
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for empty ADMIN_USERNAMES")
	}
}

func TestRequiredFieldsValidator_MissingSeriousDirectPrompt(t *testing.T) {
	cfg := makeValidConfig()
	cfg.SeriousDirectPrompt = ""
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for empty SERIOUS_DIRECT_PROMPT")
	}
}

func TestRequiredFieldsValidator_MissingDirectReplyLimitPrompt(t *testing.T) {
	cfg := makeValidConfig()
	cfg.DirectReplyLimitPrompt = ""
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for empty DIRECT_REPLY_LIMIT_PROMPT")
	}
}

func TestTypeValidator_UnknownProvider(t *testing.T) {
	cfg := makeValidConfig()
	cfg.LLMProvider = "unknown_provider"
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for unknown LLM_PROVIDER")
	}
	if !strings.Contains(err.Error(), "unknown_provider") {
		t.Errorf("expected error message to contain 'unknown_provider', got: %v", err)
	}
}

func TestRangeValidator_Temperature(t *testing.T) {
	cfg := makeValidConfig()

	cfg.DefaultTemperature = -0.1
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for temperature < 0.0")
	}

	cfg.DefaultTemperature = 2.1
	err = ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for temperature > 2.0")
	}

	cfg.DefaultTemperature = 1.0
	err = ValidateConfig(cfg)
	if err != nil {
		t.Errorf("expected no error for valid temperature 1.0, got: %v", err)
	}
}

func TestRangeValidator_DailyTakeTime(t *testing.T) {
	cfg := makeValidConfig()

	cfg.DailyTakeTime = -1
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for DailyTakeTime < 0")
	}

	cfg.DailyTakeTime = 24
	err = ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for DailyTakeTime > 23")
	}

	cfg.DailyTakeTime = 0
	err = ValidateConfig(cfg)
	if err != nil {
		t.Errorf("expected no error for DailyTakeTime=0, got: %v", err)
	}

	cfg.DailyTakeTime = 23
	err = ValidateConfig(cfg)
	if err != nil {
		t.Errorf("expected no error for DailyTakeTime=23, got: %v", err)
	}
}

func TestRangeValidator_MinMaxMessages(t *testing.T) {
	cfg := makeValidConfig()

	cfg.MinMessages = 0
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for MinMessages < 1")
	}

	cfg.MinMessages = 10
	cfg.MaxMessages = 5
	err = ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for MinMessages > MaxMessages")
	}

	cfg.MaxMessages = 0
	err = ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for MaxMessages < 1")
	}
}

func TestRangeValidator_ContextWindow(t *testing.T) {
	cfg := makeValidConfig()

	cfg.ContextWindow = 0
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for ContextWindow < 1")
	}

	cfg.ContextWindow = -5
	err = ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for negative ContextWindow")
	}
}

func TestRangeValidator_SummaryInterval(t *testing.T) {
	cfg := makeValidConfig()

	cfg.SummaryIntervalHours = -1
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for negative SummaryIntervalHours")
	}

	cfg.SummaryIntervalHours = 0
	err = ValidateConfig(cfg)
	if err != nil {
		t.Errorf("expected no error for SummaryIntervalHours=0, got: %v", err)
	}
}

func TestConsistencyValidator_StoragePostgres(t *testing.T) {
	cfg := makeValidConfig()
	cfg.StorageType = StorageTypePostgres
	cfg.PostgresqlHost = "localhost"
	cfg.PostgresqlUser = "user"
	cfg.PostgresqlDbname = "db"

	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("expected no error for valid postgres config, got: %v", err)
	}
}

func TestConsistencyValidator_StoragePostgresMissingHost(t *testing.T) {
	cfg := makeValidConfig()
	cfg.StorageType = StorageTypePostgres
	cfg.PostgresqlHost = ""
	cfg.PostgresqlUser = "user"
	cfg.PostgresqlDbname = "db"

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for missing PostgreSQL HOST")
	}
}

func TestConsistencyValidator_StorageMongo(t *testing.T) {
	cfg := makeValidConfig()
	cfg.StorageType = StorageTypeMongo
	cfg.MongoDbURI = "mongodb://localhost"
	cfg.MongoDbName = "luna_bot"
	cfg.MongoDbMessagesCollection = "messages"
	cfg.MongoDbUserProfilesCollection = "profiles"
	cfg.MongoDbSettingsCollection = "settings"

	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("expected no error for valid mongo config, got: %v", err)
	}
}

func TestConsistencyValidator_StorageMongoMissingURI(t *testing.T) {
	cfg := makeValidConfig()
	cfg.StorageType = StorageTypeMongo
	cfg.MongoDbName = "luna_bot"
	cfg.MongoDbMessagesCollection = "messages"
	cfg.MongoDbUserProfilesCollection = "profiles"
	cfg.MongoDbSettingsCollection = "settings"

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for missing MongoDB URI")
	}
}

func TestConsistencyValidator_StorageMongoMissingCollections(t *testing.T) {
	tests := []struct {
		name                string
		modify              func(cfg *Config)
	}{
		{"missing dbname", func(cfg *Config) { cfg.MongoDbName = "" }},
		{"missing messages collection", func(cfg *Config) { cfg.MongoDbMessagesCollection = "" }},
		{"missing user profiles collection", func(cfg *Config) { cfg.MongoDbUserProfilesCollection = "" }},
		{"missing settings collection", func(cfg *Config) { cfg.MongoDbSettingsCollection = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := makeValidConfig()
			cfg.StorageType = StorageTypeMongo
			cfg.MongoDbURI = "mongodb://localhost"
			cfg.MongoDbName = "luna_bot"
			cfg.MongoDbMessagesCollection = "messages"
			cfg.MongoDbUserProfilesCollection = "profiles"
			cfg.MongoDbSettingsCollection = "settings"
			tt.modify(cfg)
			err := ValidateConfig(cfg)
			if err == nil {
				t.Errorf("expected error for %s", tt.name)
			}
		})
	}
}

func TestConsistencyValidator_LongTermMemoryNeedsGeminiKey(t *testing.T) {
	cfg := makeValidConfig()
	cfg.LLMProvider = ProviderDeepSeek
	cfg.DeepSeekAPIKey = "key"
	cfg.DeepSeekModelName = "model"
	cfg.LongTermMemoryEnabled = true
	cfg.GeminiAPIKey = ""

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error: LongTermMemory with DeepSeek needs Gemini key")
	}
}

func TestConsistencyValidator_VoiceTranscriptionNeedsGeminiKey(t *testing.T) {
	cfg := makeValidConfig()
	cfg.GeminiAPIKey = ""
	cfg.VoiceTranscriptionEnabledDefault = true

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error: VoiceTranscription needs GEMINI_API_KEY")
	}
}

func TestConsistencyValidator_PhotoAnalysisNeedsGeminiKey(t *testing.T) {
	cfg := makeValidConfig()
	cfg.GeminiAPIKey = ""
	cfg.PhotoAnalysisEnabled = true

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error: PhotoAnalysis needs GEMINI_API_KEY")
	}
}

func TestConsistencyValidator_MongoCleanupWrongStorageType(t *testing.T) {
	cfg := makeValidConfig()
	cfg.MongoCleanupEnabled = true
	cfg.StorageType = StorageTypeFile

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error: MongoCleanup requires StorageType='mongo'")
	}
}

func TestConsistencyValidator_MongoCleanupInvalidSize(t *testing.T) {
	cfg := makeValidConfig()
	cfg.StorageType = StorageTypeMongo
	cfg.MongoDbURI = "mongodb://localhost"
	cfg.MongoDbName = "luna_bot"
	cfg.MongoDbMessagesCollection = "messages"
	cfg.MongoDbUserProfilesCollection = "profiles"
	cfg.MongoDbSettingsCollection = "settings"
	cfg.MongoCleanupEnabled = true
	cfg.MongoCleanupSizeLimitMB = 0
	cfg.MongoCleanupIntervalMinutes = 60
	cfg.MongoCleanupChunkDurationHours = 24

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for MongoCleanup SizeLimitMB <= 0")
	}
}

func TestConsistencyValidator_MongoCleanupInvalidInterval(t *testing.T) {
	cfg := makeValidConfig()
	cfg.StorageType = StorageTypeMongo
	cfg.MongoDbURI = "mongodb://localhost"
	cfg.MongoDbName = "luna_bot"
	cfg.MongoDbMessagesCollection = "messages"
	cfg.MongoDbUserProfilesCollection = "profiles"
	cfg.MongoDbSettingsCollection = "settings"
	cfg.MongoCleanupEnabled = true
	cfg.MongoCleanupSizeLimitMB = 450
	cfg.MongoCleanupIntervalMinutes = 0
	cfg.MongoCleanupChunkDurationHours = 24

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for MongoCleanup IntervalMinutes <= 0")
	}
}

func TestConsistencyValidator_MongoCleanupInvalidChunk(t *testing.T) {
	cfg := makeValidConfig()
	cfg.StorageType = StorageTypeMongo
	cfg.MongoDbURI = "mongodb://localhost"
	cfg.MongoDbName = "luna_bot"
	cfg.MongoDbMessagesCollection = "messages"
	cfg.MongoDbUserProfilesCollection = "profiles"
	cfg.MongoDbSettingsCollection = "settings"
	cfg.MongoCleanupEnabled = true
	cfg.MongoCleanupSizeLimitMB = 450
	cfg.MongoCleanupIntervalMinutes = 60
	cfg.MongoCleanupChunkDurationHours = 0

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for MongoCleanup ChunkDurationHours <= 0")
	}
}

func TestConsistencyValidator_AutoBio(t *testing.T) {
	cfg := makeValidConfig()
	cfg.AutoBioEnabled = true
	cfg.AutoBioIntervalHours = 24
	cfg.AutoBioInitialAnalysisPrompt = "prompt %s"
	cfg.AutoBioUpdatePrompt = "update %s %s %s"
	cfg.AutoBioMessagesLookbackDays = 30
	cfg.AutoBioMinMessagesForAnalysis = 0
	cfg.AutoBioMaxMessagesForAnalysis = 1000

	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("expected no error for valid AutoBio config, got: %v", err)
	}
}

func TestConsistencyValidator_AutoBioMissingPrompts(t *testing.T) {
	cfg := makeValidConfig()
	cfg.AutoBioEnabled = true
	cfg.AutoBioIntervalHours = 24
	cfg.AutoBioInitialAnalysisPrompt = ""
	cfg.AutoBioUpdatePrompt = "update"
	cfg.AutoBioMessagesLookbackDays = 30
	cfg.AutoBioMinMessagesForAnalysis = 0
	cfg.AutoBioMaxMessagesForAnalysis = 1000

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for missing AutoBioInitialAnalysisPrompt")
	}
}

func TestConsistencyValidator_AutoBioUpdatePromptMissing(t *testing.T) {
	cfg := makeValidConfig()
	cfg.AutoBioEnabled = true
	cfg.AutoBioIntervalHours = 24
	cfg.AutoBioInitialAnalysisPrompt = "prompt"
	cfg.AutoBioUpdatePrompt = ""
	cfg.AutoBioMessagesLookbackDays = 30
	cfg.AutoBioMinMessagesForAnalysis = 0
	cfg.AutoBioMaxMessagesForAnalysis = 1000

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for missing AutoBioUpdatePrompt")
	}
}

func TestConsistencyValidator_AutoBioInvalidInterval(t *testing.T) {
	cfg := makeValidConfig()
	cfg.AutoBioEnabled = true
	cfg.AutoBioIntervalHours = 0
	cfg.AutoBioInitialAnalysisPrompt = "prompt"
	cfg.AutoBioUpdatePrompt = "update"
	cfg.AutoBioMessagesLookbackDays = 30
	cfg.AutoBioMinMessagesForAnalysis = 0
	cfg.AutoBioMaxMessagesForAnalysis = 1000

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for AutoBio IntervalHours <= 0")
	}
}

func TestConsistencyValidator_AutoBioInvalidLookback(t *testing.T) {
	cfg := makeValidConfig()
	cfg.AutoBioEnabled = true
	cfg.AutoBioIntervalHours = 24
	cfg.AutoBioInitialAnalysisPrompt = "prompt"
	cfg.AutoBioUpdatePrompt = "update"
	cfg.AutoBioMessagesLookbackDays = 0
	cfg.AutoBioMinMessagesForAnalysis = 0
	cfg.AutoBioMaxMessagesForAnalysis = 1000

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for AutoBio MessagesLookbackDays <= 0")
	}
}

func TestConsistencyValidator_Moderation(t *testing.T) {
	cfg := makeValidConfig()
	cfg.ModInterval = 5
	cfg.ModMuteTimeMin = 0
	cfg.ModBanTimeMin = 0
	cfg.ModPurgeDuration = 30 * time.Second
	cfg.ModPurgeDelay = 0

	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("expected no error for valid moderation config, got: %v", err)
	}
}

func TestConsistencyValidator_ModerationInvalidInterval(t *testing.T) {
	cfg := makeValidConfig()
	cfg.ModInterval = 0
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for ModInterval <= 0")
	}
}

func TestConsistencyValidator_ModerationNegativeTimes(t *testing.T) {
	cfg := makeValidConfig()
	cfg.ModMuteTimeMin = -1
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for negative ModMuteTimeMin")
	}
}

func TestConsistencyValidator_ModerationNegativeBan(t *testing.T) {
	cfg := makeValidConfig()
	cfg.ModBanTimeMin = -1
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for negative ModBanTimeMin")
	}
}

func TestConsistencyValidator_ModerationInvalidPurgeDuration(t *testing.T) {
	cfg := makeValidConfig()
	cfg.ModPurgeDuration = 0
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for ModPurgeDuration <= 0")
	}
}

func TestConsistencyValidator_ModerationNegativePurgeDelay(t *testing.T) {
	cfg := makeValidConfig()
	cfg.ModPurgeDelay = -1 * time.Second
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for negative ModPurgeDelay")
	}
}

func TestConsistencyValidator_ModerationRuleInvalidPunishment(t *testing.T) {
	cfg := makeValidConfig()
	cfg.ModRules = []ModerationRule{
		{
			RuleName:   "test_rule",
			ChatID:     "none",
			UserID:     "none",
			Keywords:   []string{"test"},
			Punishment: "invalid_punishment",
		},
	}
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for invalid punishment type")
	}
}

func TestConsistencyValidator_ModerationRuleEmptyKeywords(t *testing.T) {
	cfg := makeValidConfig()
	cfg.ModRules = []ModerationRule{
		{
			RuleName:   "test_rule",
			ChatID:     "none",
			UserID:     "none",
			Keywords:   []string{},
			Punishment: PunishMute,
		},
	}
	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("expected error for empty keywords in moderation rule")
	}
}

func TestConsistencyValidator_ModerationRuleValid(t *testing.T) {
	cfg := makeValidConfig()
	cfg.ModRules = []ModerationRule{
		{
			RuleName:        "anti_spam",
			ChatID:          "none",
			UserID:          "none",
			Keywords:        []string{"spam", "казино"},
			Punishment:      PunishMute,
			NotifyUser:      true,
		},
		{
			RuleName:        "anti_flood",
			ChatID:          "none",
			UserID:          "none",
			Keywords:        []string{"flood"},
			Punishment:      PunishBan,
			NotifyUser:      false,
		},
	}
	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("expected no error for valid rules, got: %v", err)
	}
}

func TestConsistencyValidator_ModerationRuleAllPunishments(t *testing.T) {
	validTypes := []PunishmentType{PunishMute, PunishKick, PunishBan, PunishPurge, PunishNone, PunishEdit}
	for _, pt := range validTypes {
		cfg := makeValidConfig()
		cfg.ModRules = []ModerationRule{
			{
				RuleName:   "test",
				ChatID:     "none",
				UserID:     "none",
				Keywords:   []string{"test"},
				Punishment: pt,
			},
		}
		err := ValidateConfig(cfg)
		if err != nil {
			t.Errorf("expected no error for punishment '%s', got: %v", pt, err)
		}
	}
}

func TestValidateConfig_AllValid(t *testing.T) {
	cfg := makeValidConfig()
	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("expected no error for fully valid config, got: %v", err)
	}
}

func TestValidateConfig_DeepSeekValid(t *testing.T) {
	cfg := makeValidConfig()
	cfg.LLMProvider = ProviderDeepSeek
	cfg.DeepSeekAPIKey = "ds-key"
	cfg.DeepSeekModelName = "deepseek-chat"

	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("expected no error for valid DeepSeek config, got: %v", err)
	}
}

func TestValidateConfig_OpenRouterValid(t *testing.T) {
	cfg := makeValidConfig()
	cfg.LLMProvider = ProviderOpenRouter
	cfg.OpenRouterAPIKey = "or-key"
	cfg.OpenRouterModelName = "model"

	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("expected no error for valid OpenRouter config, got: %v", err)
	}
}

func TestConsistencyValidator_DeepSeekLongTermMemoryEmbeddingFallback(t *testing.T) {
	cfg := makeValidConfig()
	cfg.LLMProvider = ProviderDeepSeek
	cfg.DeepSeekAPIKey = "key"
	cfg.DeepSeekModelName = "model"
	cfg.GeminiAPIKey = "gem-key"
	cfg.LongTermMemoryEnabled = true
	cfg.GeminiEmbeddingModelName = ""

	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("expected no error (warning only), got: %v", err)
	}
}
