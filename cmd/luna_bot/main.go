package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/anthropic"
	"github.com/Henry-Case-dev/luna_bot/internal/bot"
	"github.com/Henry-Case-dev/luna_bot/internal/config"
	"github.com/Henry-Case-dev/luna_bot/internal/deepseek"
	"github.com/Henry-Case-dev/luna_bot/internal/gemini"
	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/local"
	"github.com/Henry-Case-dev/luna_bot/internal/openai"
	"github.com/Henry-Case-dev/luna_bot/internal/openrouter"
	"github.com/joho/godotenv"
)

// handleRoot - простой обработчик HTTP запросов
func handleRoot(w http.ResponseWriter, r *http.Request) {
	// Логируем каждый входящий запрос к этому обработчику
	log.Printf("--> HTTP Request Received: Method=%s, Path=%s, RemoteAddr=%s", r.Method, r.URL.Path, r.RemoteAddr)
	fmt.Fprintf(w, "Hello from Luna Bot server!")
	// Логируем после отправки ответа
	log.Printf("<-- HTTP Response Sent for: %s %s", r.Method, r.URL.Path)
}

// handleStatus - обработчик статуса сервисов
func handleStatus(w http.ResponseWriter, r *http.Request) {
	log.Printf("--> Status Request: Method=%s, Path=%s, RemoteAddr=%s", r.Method, r.URL.Path, r.RemoteAddr)

	status := map[string]string{
		"bot":       "running",
		"grafana":   "http://127.0.0.1:3000/",
		"loki":      "http://127.0.0.1:3100/",
		"timestamp": time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status": %q, "services": {"grafana": %q, "loki": %q}, "timestamp": %q}`,
		status["bot"], status["grafana"], status["loki"], status["timestamp"])

	log.Printf("<-- Status Response Sent for: %s %s", r.Method, r.URL.Path)
}

func main() {
	// Добавить эти строки в самое начало main()
	_ = godotenv.Load(".env")
	_ = godotenv.Load(".env.secrets")

	// Определяем путь к логам в зависимости от окружения
	logfilePath := "luna_runtime.log"
	if os.Getenv("PRODUCTION") == "true" || os.Getenv("DOCKER") == "true" {
		// В production/Docker логи в стандартной директории
		logfilePath = "/var/log/luna_bot/luna_runtime.log"
		// Создаем директорию если её нет
		if err := os.MkdirAll("/var/log/luna_bot", 0755); err != nil {
			log.Printf("Не удалось создать директорию логов: %v", err)
		}
	}

	// Удаляем старый файл логов, если он существует
	if _, err := os.Stat(logfilePath); err == nil {
		if err := os.Remove(logfilePath); err != nil {
			log.Printf("Не удалось удалить старый файл логов '%s': %v", logfilePath, err)
			// Продолжаем работу, даже если не удалось удалить старый файл
		} else {
			log.Printf("Старый файл логов '%s' успешно удален.", logfilePath)
		}
	}

	f, err := os.OpenFile(logfilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		log.Printf("Не удалось открыть файл логов '%s': %v", logfilePath, err)
		// Если не удалось открыть файл логов, продолжаем работу с выводом в stdout
		log.SetOutput(os.Stdout)
	} else {
		defer f.Close()
		log.SetOutput(f) // Перенаправляем все вызовы log.* в файл
	}
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds) // Используем стандартные флаги + микросекунды для большей точности

	log.Println("=== Application Starting ===")
	log.Printf("Timestamp: %s", time.Now().UTC().Format(time.RFC3339))

	cfg, err := config.Load()
	if err != nil {
		log.Printf("!!! FATAL: Ошибка загрузки конфигурации: %v", err)
		time.Sleep(15 * time.Second)
		panic(fmt.Sprintf("Configuration error: %v", err))
	}
	log.Println("--- Legacy Configuration Loaded ---")

	// YAML — единственный источник конфигурации.
	// .env используется ТОЛЬКО для ${ENV_VAR} резолва секретов (TELEGRAM_TOKEN, API_KEY и т.д.)
	source := config.NewYAMLConfigSource("configs/luna_bot.yaml")
	cfgV2, err := source.Load(context.Background())
	if err != nil {
		log.Fatalf("[FATAL] Ошибка загрузки luna_bot.yaml: %v", err)
	}
	log.Println("[INFO] Конфигурация загружена из luna_bot.yaml (секреты из .env)")

	// Синхронизируем YAML-настройки в legacy-конфиг (единственный источник правды — YAML)
	//
	// Chat
	cfg.MinMessages = cfgV2.Chat.MinMessages
	cfg.MaxMessages = cfgV2.Chat.MaxMessages
	cfg.ContextWindow = cfgV2.Chat.ContextWindow
	cfg.ImageGenerationContextWindow = cfgV2.Chat.ImageGenerationContextWindow
	cfg.DailyTakeTime = cfgV2.Chat.DailyTakeTime

	// Telegram
	cfg.Debug = cfgV2.Telegram.Debug
	cfg.TimeZone = cfgV2.Telegram.Timezone
	cfg.ErrorMessageAutoDeleteSeconds = cfgV2.Telegram.ErrorAutoDeleteSeconds
	cfg.UseStructuredMessageFormat = cfgV2.Telegram.UseStructuredMessageFormat
	cfg.BotNames = cfgV2.Telegram.BotNames
	if len(cfgV2.Telegram.AdminIDs) > 0 {
		cfg.AdminID = cfgV2.Telegram.AdminIDs[0]
	}
	cfg.AdminUsernames = cfgV2.Telegram.AdminUsernames

	// LLM
	cfg.LLMProvider = config.LLMProvider(cfgV2.LLM.DefaultProvider)
	cfg.LLMFallbackEnabled = cfgV2.LLM.FallbackEnabled
	cfg.LLMFallbackCriticalTypes = cfgV2.LLM.FallbackCriticalTypes
	cfg.LLMFallbackProviderOrder = cfgV2.LLM.FallbackProviderOrder

	// LLM Providers: Gemini
	if gemini, ok := cfgV2.LLM.Providers["gemini"]; ok {
		cfg.GeminiAPIKey = gemini.APIKey
		cfg.GeminiAPIKeyReserve = gemini.ReserveAPIKey
		cfg.GeminiKeyRotationTimeHours = gemini.KeyRotationHours
		cfg.GeminiBypassSafetyFilters = gemini.Safety.BypassFilters
		cfg.GeminiObfuscatePrompts = gemini.Safety.Obfuscate
		if m, ok2 := gemini.Models["text"]; ok2 {
			cfg.GeminiModelName = m
		}
		if m, ok2 := gemini.Models["audio"]; ok2 {
			cfg.AudioTranscriptionModel = m
		}
		if m, ok2 := gemini.Models["image_gen"]; ok2 {
			cfg.ImageGenerationModel = m
		}
		if m, ok2 := gemini.Models["embed"]; ok2 {
			cfg.GeminiEmbeddingModelName = m
		}
		if t, ok2 := gemini.Temperatures["normal"]; ok2 {
			cfg.GeminiTemperatureNormal = t
		}
		if t, ok2 := gemini.Temperatures["serious"]; ok2 {
			cfg.GeminiTemperatureSerious = t
		}
		if t, ok2 := gemini.Temperatures["audio"]; ok2 {
			cfg.AudioTranscriptionTemperature = t
		}
		if t, ok2 := gemini.Temperatures["image"]; ok2 {
			cfg.ImageGenerationTemperature = t
		}
	}

	// LLM Providers: DeepSeek
	if deepseek, ok := cfgV2.LLM.Providers["deepseek"]; ok {
		cfg.DeepSeekAPIKey = deepseek.APIKey
		cfg.DeepSeekBaseURL = deepseek.BaseURL
		if m, ok2 := deepseek.Models["text"]; ok2 {
			cfg.DeepSeekModelName = m
		}
	}

	// LLM Providers: OpenRouter
	if openrouter, ok := cfgV2.LLM.Providers["openrouter"]; ok {
		cfg.OpenRouterAPIKey = openrouter.APIKey
		cfg.OpenRouterSiteURL = openrouter.SiteURL
		cfg.OpenRouterSiteTitle = openrouter.SiteTitle
		if m, ok2 := openrouter.Models["text"]; ok2 {
			cfg.OpenRouterModelName = m
		}
	}

	// Summary
	cfg.SummaryIntervalHours = cfgV2.Summary.IntervalHours
	cfg.SummaryMaxParts = cfgV2.Summary.MaxParts
	cfg.WeeklySummaryEnabled = cfgV2.Summary.Weekly.Enabled
	cfg.WeeklySummaryDay = cfgV2.Summary.Weekly.Day
	cfg.WeeklySummaryHour = cfgV2.Summary.Weekly.Hour
	cfg.WeeklySummaryMinute = cfgV2.Summary.Weekly.Minute
	cfg.WeeklySummaryMaxParts = cfgV2.Summary.Weekly.MaxParts

	// RateLimit
	cfg.RateLimitStaticText = cfgV2.RateLimit.StaticText

	// Settings Prompts
	cfg.PromptEnterMinMessages = cfgV2.SettingsPrompts.EnterMinMessages
	cfg.PromptEnterMaxMessages = cfgV2.SettingsPrompts.EnterMaxMessages
	cfg.PromptEnterDailyTime = cfgV2.SettingsPrompts.EnterDailyTime
	cfg.PromptEnterSummaryInterval = cfgV2.SettingsPrompts.EnterSummaryInterval
	cfg.PromptEnterDirectLimitCount = cfgV2.SettingsPrompts.EnterDirectLimitCount
	cfg.PromptEnterDirectLimitDuration = cfgV2.SettingsPrompts.EnterDirectLimitDuration

	// Srach
	cfg.SrachAnalysisEnabled = cfgV2.Srach.AnalysisEnabled

	// DirectReplyLimits
	cfg.DirectReplyLimitEnabledDefault = cfgV2.DirectReplyLimits.EnabledDefault
	cfg.DirectReplyLimitCountDefault = cfgV2.DirectReplyLimits.CountDefault
	cfg.DirectReplyLimitDurationDefault = time.Duration(cfgV2.DirectReplyLimits.DurationMinutesDefault) * time.Minute

	// Donate
	cfg.DonateTimeHours = cfgV2.Donate.TimeHours

	// FreeWill
	cfg.FreeWillEnabled = cfgV2.FreeWill.Enabled
	cfg.FreeWillMinIntervalMinutes = cfgV2.FreeWill.Intervals.MinMinutes
	cfg.FreeWillMaxIntervalMinutes = cfgV2.FreeWill.Intervals.MaxMinutes
	cfg.FreeWillContextWindow = cfgV2.FreeWill.ContextWindow
	cfg.FreeWillMoodUpdateProbability = cfgV2.FreeWill.MoodUpdateProbability
	cfg.FreeWillMaxDecisionsPerHour = cfgV2.FreeWill.MaxDecisionsPerHour
	cfg.FreeWillVoiceProbability = cfgV2.FreeWill.VoiceProbability
	cfg.FreeWillSilenceMinMinutes = cfgV2.FreeWill.Silence.MinMinutes
	cfg.FreeWillSilenceMaxMinutes = cfgV2.FreeWill.Silence.MaxMinutes
	cfg.FreeWillReactionsEnabled = cfgV2.FreeWill.Reactions.Enabled
	cfg.FreeWillReactionsProbability = cfgV2.FreeWill.Reactions.Probability
	cfg.FreeWillReactionsCooldownMinutes = cfgV2.FreeWill.Reactions.CooldownMinutes
	cfg.FreeWillReactionsMaxPerHour = cfgV2.FreeWill.Reactions.MaxPerHour
	cfg.FreeWillDirectResponseMaxPerHour = cfgV2.FreeWill.DirectResponse.MaxPerHour
	cfg.FreeWillDirectResponseMinIntervalSeconds = cfgV2.FreeWill.DirectResponse.MinIntervalSeconds
	cfg.FreeWillDirectResponseIndependentLimits = cfgV2.FreeWill.DirectResponse.IndependentLimits
	cfg.FreeWillImageGenerationMaxDecisionsPerInterval = cfgV2.FreeWill.ImageGeneration.MaxPerInterval
	cfg.FreeWillImageGenerationIntervalHours = cfgV2.FreeWill.ImageGeneration.IntervalHours
	cfg.FreeWillImageGenerationMinDecisionIntervalMinutes = cfgV2.FreeWill.ImageGeneration.MinDecisionIntervalMin
	cfg.FreeWillImageGenerationIndependentLimits = cfgV2.FreeWill.ImageGeneration.IndependentLimits
	cfg.ImageGenFrequencyHours = cfgV2.FreeWill.ImageGeneration.FrequencyHours
	cfg.IntervalMessagesEnabled = cfgV2.FreeWill.IntervalMessages.Enabled

	// VoiceMessages
	cfg.VoiceMessagesEnabled = cfgV2.VoiceMessages.Enabled
	cfg.MinVoiceMessages = cfgV2.VoiceMessages.Interval.Min
	cfg.MaxVoiceMessages = cfgV2.VoiceMessages.Interval.Max
	cfg.VoiceMessageTempDir = cfgV2.VoiceMessages.TempDir

	// Moderation
	cfg.ModEnabled = cfgV2.Moderation.Enabled
	cfg.ModInterval = cfgV2.Moderation.IntervalMinutes
	cfg.ModMuteTimeMin = cfgV2.Moderation.MuteTimeMinutes
	cfg.ModKickTimeMin = cfgV2.Moderation.KickTimeMinutes
	cfg.ModBanTimeMin = cfgV2.Moderation.BanTimeMinutes
	cfg.ModPurgeDuration = cfgV2.Moderation.PurgeWindow
	cfg.ModPurgeDelay = cfgV2.Moderation.PurgeDelay
	cfg.ModCheckAdminRights = cfgV2.Moderation.CheckAdminRights
	cfg.ModDefaultNotify = cfgV2.Moderation.DefaultNotify

	// AntiRepetition
	cfg.AntiRepetitionEnabled = cfgV2.AntiRepetition.Enabled
	cfg.AntiRepetitionMaxResponsesPerChat = cfgV2.AntiRepetition.MaxResponsesPerChat
	cfg.AntiRepetitionSimilarityThreshold = cfgV2.AntiRepetition.SimilarityThreshold
	cfg.AntiRepetitionTimeWindowHours = cfgV2.AntiRepetition.TimeWindowHours
	cfg.AntiRepetitionCleanupIntervalHours = cfgV2.AntiRepetition.CleanupIntervalHours
	cfg.AntiRepetitionReworkEnabled = cfgV2.AntiRepetition.Rework.Enabled
	cfg.AntiRepetitionMaxReworkAttempts = cfgV2.AntiRepetition.Rework.MaxAttempts
	cfg.AntiRepetitionReworkTemperature = cfgV2.AntiRepetition.Rework.Temperature
	cfg.AntiRepetitionLocalReworkEnabled = cfgV2.AntiRepetition.Rework.LocalRework.Enabled
	cfg.AntiRepetitionLocalReworkMaxLength = cfgV2.AntiRepetition.Rework.LocalRework.MaxLength

	// Disambiguation
	cfg.DisambiguationEnabled = cfgV2.Disambiguation.Enabled

	// AutoBio
	cfg.AutoBioEnabled = cfgV2.AutoBio.Enabled
	cfg.AutoBioIntervalHours = cfgV2.AutoBio.IntervalHours
	cfg.AutoBioMessagesLookbackDays = cfgV2.AutoBio.LookbackDays
	cfg.AutoBioMinMessagesForAnalysis = cfgV2.AutoBio.MinMessagesForAnalysis
	cfg.AutoBioMaxMessagesForAnalysis = cfgV2.AutoBio.MaxMessagesForAnalysis

	// Personality
	cfg.PersonalityUpdateIntervalHours = cfgV2.Personality.UpdateIntervalHours
	cfg.PersonalityMessagesLookback = cfgV2.Personality.MessagesLookback
	cfg.MaxNameMentions = cfgV2.Personality.MaxNameMentions
	cfg.MaxRecentTopics = cfgV2.Personality.MaxRecentTopics
	cfg.MaxSelfPerceptions = cfgV2.Personality.MaxSelfPerceptions
	cfg.MaxDiscussionContexts = cfgV2.Personality.MaxDiscussionContexts

	// Reactions
	cfg.ReactionsEnabled = cfgV2.Reactions.Enabled
	cfg.ClownResponseProbability = cfgV2.Reactions.Clown.ResponseProbability
	cfg.ClownCooldownSeconds = cfgV2.Reactions.Clown.CooldownSeconds
	cfg.MaxClownResponsesPerHour = cfgV2.Reactions.Clown.MaxResponsesPerHour

	// WebSearch
	cfg.WebSearchEnabled = cfgV2.WebSearch.Enabled
	cfg.GoogleSearchAPIKey = cfgV2.WebSearch.GoogleAPIKey
	cfg.GoogleSearchEngineID = cfgV2.WebSearch.SearchEngineID
	cfg.WebSearchMaxResults = cfgV2.WebSearch.MaxResults
	cfg.WebSearchCacheTTL = cfgV2.WebSearch.Cache.TTL
	cfg.WebSearchCacheMaxSize = cfgV2.WebSearch.Cache.MaxSize

	// TTS / ElevenLabs
	cfg.ElevenLabsAPIKey = cfgV2.TTS.ElevenLabs.APIKey
	cfg.ElevenLabsVoiceID = cfgV2.TTS.ElevenLabs.VoiceID
	cfg.ElevenLabsModel = cfgV2.TTS.ElevenLabs.Model
	cfg.ElevenLabsPlan = cfgV2.TTS.ElevenLabs.Plan
	cfg.ElevenLabsStability = cfgV2.TTS.ElevenLabs.VoiceSettings.Stability
	cfg.ElevenLabsSimilarityBoost = cfgV2.TTS.ElevenLabs.VoiceSettings.SimilarityBoost
	cfg.ElevenLabsStyle = cfgV2.TTS.ElevenLabs.VoiceSettings.Style
	cfg.ElevenLabsUseSpeakerBoost = cfgV2.TTS.ElevenLabs.VoiceSettings.UseSpeakerBoost
	cfg.ElevenLabsSpeed = cfgV2.TTS.ElevenLabs.VoiceSettings.Speed
	cfg.ElevenLabsStylePrompt = cfgV2.TTS.ElevenLabs.Prompts.Style
	cfg.ElevenLabsEmotionPrompt = cfgV2.TTS.ElevenLabs.Prompts.Emotion
	cfg.ElevenLabsPacePrompt = cfgV2.TTS.ElevenLabs.Prompts.Pace
	cfg.ElevenLabsRandomVoice = cfgV2.TTS.ElevenLabs.RandomVoice

	// Storage
	cfg.StorageType = config.StorageType(cfgV2.Storage.Type)
	cfg.PostgresqlHost = cfgV2.Storage.PostgreSQL.Host
	cfg.PostgresqlPort = fmt.Sprintf("%d", cfgV2.Storage.PostgreSQL.Port)
	cfg.PostgresqlUser = cfgV2.Storage.PostgreSQL.User
	cfg.PostgresqlPassword = cfgV2.Storage.PostgreSQL.Password
	cfg.PostgresqlDbname = cfgV2.Storage.PostgreSQL.DBName
	cfg.MongoDbURI = cfgV2.Storage.MongoDB.URI
	cfg.MongoDbName = cfgV2.Storage.MongoDB.DBName
	cfg.MongoDbMessagesCollection = cfgV2.Storage.MongoDB.MessagesCollection
	cfg.MongoDbUserProfilesCollection = cfgV2.Storage.MongoDB.UserProfilesCollection
	cfg.MongoDbSettingsCollection = cfgV2.Storage.MongoDB.SettingsCollection
	cfg.MongoVectorIndexName = cfgV2.Storage.MongoDB.VectorIndexName
	cfg.LongTermMemoryEnabled = cfgV2.Storage.LongTermMemory.Enabled
	cfg.LongTermMemoryFetchK = cfgV2.Storage.LongTermMemory.FetchK
	cfg.BackfillBatchSize = cfgV2.Storage.LongTermMemory.Backfill.BatchSize
	cfg.BackfillBatchDelay = cfgV2.Storage.LongTermMemory.Backfill.BatchDelay
	cfg.MongoCleanupEnabled = cfgV2.Storage.Cleanup.Enabled
	cfg.MongoCleanupSizeLimitMB = cfgV2.Storage.Cleanup.SizeLimitMB
	cfg.MongoCleanupIntervalMinutes = cfgV2.Storage.Cleanup.IntervalMinutes
	cfg.MongoCleanupChunkDurationHours = cfgV2.Storage.Cleanup.ChunkDurationHours
	if cfgV2.Storage.LongTermMemory.EmbeddingModel != "" {
		cfg.GeminiEmbeddingModelName = cfgV2.Storage.LongTermMemory.EmbeddingModel
	}

	// CausalLearning
	cfg.CausalLearningEnabled = cfgV2.CausalLearning.Enabled
	cfg.CausalAnalysisIntervalHours = cfgV2.CausalLearning.AnalysisIntervalHours
	cfg.CausalMinConfidence = cfgV2.CausalLearning.MinConfidence
	cfg.CausalTemporalWindowMinutes = cfgV2.CausalLearning.TemporalWindowMinutes
	cfg.CausalMaxEntriesPerChat = cfgV2.CausalLearning.MaxEntriesPerChat
	cfg.CausalAnalysisLookbackMessages = cfgV2.CausalLearning.AnalysisLookbackMessages

	// EmotionalLearning
	cfg.EmotionalLearningEnabled = cfgV2.EmotionalLearning.Enabled
	cfg.EmotionalAnalysisIntervalHours = cfgV2.EmotionalLearning.AnalysisIntervalHours
	cfg.EmotionalAnalysisLookbackMessages = cfgV2.EmotionalLearning.AnalysisLookbackMessages
	cfg.EmotionalMemoryRetentionDays = cfgV2.EmotionalLearning.MemoryRetentionDays
	cfg.EmotionalMinMessagesForAnalysis = cfgV2.EmotionalLearning.MinMessagesForAnalysis
	cfg.EmotionalAnalysisDebounceHours = cfgV2.EmotionalLearning.AnalysisDebounceHours

	// BeliefLearning
	cfg.BeliefLearningEnabled = cfgV2.BeliefLearning.Enabled
	cfg.BeliefAnalysisIntervalHours = cfgV2.BeliefLearning.AnalysisIntervalHours
	cfg.BeliefAnalysisLookbackMessages = cfgV2.BeliefLearning.AnalysisLookbackMessages

	// CognitiveArchitecture
	cfg.InternalMonologueEnabled = cfgV2.CognitiveArchitecture.InternalMonologue.Enabled
	cfg.InternalMonologueTemperature = cfgV2.CognitiveArchitecture.InternalMonologue.Temperature
	cfg.SelfReflectionEnabled = cfgV2.CognitiveArchitecture.SelfReflection.Enabled
	cfg.SelfReflectionTemperature = cfgV2.CognitiveArchitecture.SelfReflection.Temperature
	cfg.ConfidenceCalibrationEnabled = cfgV2.CognitiveArchitecture.ConfidenceCalibration.Enabled

	// SocialArchitecture
	cfg.RelationshipTrackingEnabled = cfgV2.SocialArchitecture.RelationshipTracking.Enabled
	cfg.SocialLearningEnabled = cfgV2.SocialArchitecture.SocialLearning.Enabled
	cfg.IntimacyGrowthRate = cfgV2.SocialArchitecture.IntimacyGrowthRate
	cfg.TrustDecayRate = cfgV2.SocialArchitecture.TrustDecayRate

	// AssociationCloud
	cfg.AssociationCloudEnabled = cfgV2.AssociationCloud.Enabled
	cfg.AssociationCloudMaxNodes = cfgV2.AssociationCloud.MaxNodes
	cfg.AssociationCloudMaxEdges = cfgV2.AssociationCloud.MaxEdges
	cfg.AssociationCloudDecayDays = cfgV2.AssociationCloud.DecayDays

	// cfgV2 также содержит конфигурацию для нового роутера

	// Инициализация ProviderRegistry (новая архитектура LLM)
	registry := llm.NewProviderRegistry()

	// Регистрация фабрик провайдеров
	registry.Register("gemini", gemini.NewProvider)
	registry.Register("deepseek", deepseek.NewProvider)
	registry.Register("openrouter", openrouter.NewProvider)
	registry.Register("local", local.NewProvider)
	registry.Register("anthropic", anthropic.NewProvider)
	registry.Register("openai", openai.NewProvider)
	// ElevenLabs не регистрируется как LLM-провайдер (отдельный сервис)

	// Создаём LLMRouterV2 (основной роутер)
	routerV2 := bot.NewLLMRouterV2(registry, cfgV2, cfg.Debug)
	log.Println("[INFO] LLMRouterV2 инициализирован с", len(registry.FindByCapability(llm.CapTextGeneration)), "TextGenerator-провайдерами")

	// Передаём V2 роутер в конструктор бота
	botInstance, err := bot.New(cfg, routerV2)
	if err != nil {
		log.Printf("!!! FATAL: Ошибка создания бота: %v", err)
		time.Sleep(15 * time.Second)
		panic(fmt.Sprintf("Bot creation error: %v", err))
	}
	log.Println("--- Bot Initialized ---")

	// Запускаем бота в отдельной горутине
	go func() {
		if startErr := botInstance.Start(); startErr != nil {
			log.Printf("!!! CRITICAL: Критическая ошибка запуска бота: %v", startErr)
		}
	}()
	log.Println("--- Bot Start Goroutine Launched ---")
	log.Println("Бот запущен.")

	// --- Запуск Dummy HTTP сервера ---
	http.HandleFunc("/", handleRoot)         // Регистрируем обработчик
	http.HandleFunc("/status", handleStatus) // Статус сервисов

	// Выбор порта в зависимости от режима
	serverAddr := ":8080" // Дефолтный порт для микросервисной архитектуры
	if os.Getenv("NGINX_MODE") == "true" {
		serverAddr = ":8080" // Внутренний порт для nginx проксирования (тот же)
	}

	log.Printf("--- Starting HTTP server on %s ---", serverAddr)

	go func() {
		// Логируем перед запуском сервера
		log.Printf("[HTTP Goroutine] Attempting to start HTTP server on %s", serverAddr)
		if httpErr := http.ListenAndServe(serverAddr, nil); httpErr != nil {
			// Логируем ошибку, если ListenAndServe вернул ее
			log.Printf("!!! [HTTP Goroutine] HTTP Server Error: %v", httpErr)
		}
		// Логируем, если ListenAndServe завершился (даже без ошибки, хотя это маловероятно)
		log.Printf("[HTTP Goroutine] ListenAndServe on %s finished.", serverAddr)
	}()
	// Добавляем лог сразу после запуска горутины
	log.Printf("--- HTTP Server Goroutine Launched on %s ---", serverAddr)
	// --- Конец HTTP сервера ---

	log.Printf("--- Application Ready. Waiting indefinitely. ---")

	// Ожидаем бесконечно, игнорируем сигналы завершения.
	// Это нужно для Amvera, чтобы контейнер оставался RUNNING.
	select {}

	// Этот код больше никогда не будет выполнен в Amvera.
	// Оставляем его закомментированным на случай локальных тестов.
	/*
		log.Println("Остановка бота (из main)...") // Добавляем лог для ясности
		bot.Stop()
		log.Println("Приложение остановлено")
	*/
}
