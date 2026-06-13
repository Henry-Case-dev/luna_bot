// DEPRECATED: используйте YAMLConfigSource из source.go. Будет удалён в v3.0.
package config

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/bot/prompts"
)

// Примечание: инструкции форматирования Markdown определены в клиентах LLM.

// Load загружает конфигурацию из переменных окружения или использует значения по умолчанию
func Load() (*Config, error) {
	// .env файлы уже загружены в main.go, просто используем переменные окружения

	// --- Загрузка переменных Telegram ---
	telegramToken := getEnvOrDefault("TELEGRAM_TOKEN", "")

	// --- Загрузка переменных LLM ---
	llmProviderStr := strings.ToLower(getEnvOrDefault("LLM_PROVIDER", string(ProviderGemini)))
	defaultPrompt := getEnvOrDefault("DEFAULT_PROMPT", "Ты - участник чата.")
	defaultTemperature := parseFloatOrDefault(getEnvOrDefault("DEFAULT_TEMPERATURE", "0.7"), 0.7)

	// --- Загрузка переменных Gemini ---
	geminiAPIKey := getEnvOrDefault("GEMINI_API_KEY", "")
	geminiModelName := getEnvOrDefault("GEMINI_MODEL_NAME", "gemini-1.5-flash-latest")
	geminiEmbeddingModelName := getEnvOrDefault("GEMINI_EMBEDDING_MODEL_NAME", "embedding-001")

	// --- НОВЫЙ КОД: Загрузка температур Gemini ---
	geminiTemperatureNormalStr := getEnvOrDefault("GEMINI_TEMPERATURE_NORMAL", "0.7")
	geminiTemperatureNormal := parseFloatOrDefault(geminiTemperatureNormalStr, 0.7)
	geminiTemperatureSeriousStr := getEnvOrDefault("GEMINI_TEMPERATURE_SERIOUS", "0.4")
	geminiTemperatureSerious := parseFloatOrDefault(geminiTemperatureSeriousStr, 0.4)
	// --- КОНЕЦ НОВОГО КОДА ---

	// --- Загрузка отдельных настроек для аудио и изображений ---
	audioTranscriptionModel := getEnvOrDefault("AUDIO_TRANSCRIPTION_MODEL", "gemini-2.0-flash")
	audioTranscriptionTemperature := parseFloatOrDefault(getEnvOrDefault("AUDIO_TRANSCRIPTION_TEMPERATURE", "0.3"), 0.3)
	imageGenerationModel := getEnvOrDefault("IMAGE_GENERATION_MODEL", "gemini-2.5-flash-image-preview")
	imageGenerationTemperature := parseFloatOrDefault(getEnvOrDefault("IMAGE_GENERATION_TEMPERATURE", "0.7"), 0.7)
	// --- Конец загрузки отдельных настроек ---

	// --- Загрузка переменных для резервного ключа Gemini ---
	geminiAPIKeyReserve := getEnvOrDefault("GEMINI_API_KEY_RESERVE", "")
	geminiKeyRotationTimeHours := parseIntOrDefault(getEnvOrDefault("GEMINI_KEY_ROTATION_TIME_HOURS", "1"), 1)

	// --- Загрузка переменных DeepSeek ---
	deepSeekAPIKey := getEnvOrDefault("DEEPSEEK_API_KEY", "")
	deepSeekModelName := getEnvOrDefault("DEEPSEEK_MODEL_NAME", "deepseek-chat")
	deepSeekBaseURL := getEnvOrDefault("DEEPSEEK_BASE_URL", "")

	// --- Загрузка переменных OpenRouter ---
	openRouterAPIKey := getEnvOrDefault("OPENROUTER_API_KEY", "")
	openRouterModelName := getEnvOrDefault("OPENROUTER_MODEL_NAME", "")
	openRouterSiteURL := getEnvOrDefault("OPENROUTER_SITE_URL", "")
	openRouterSiteTitle := getEnvOrDefault("OPENROUTER_SITE_TITLE", "")

	// --- Загрузка переменных для донатов ---
	donateTimeHoursStr := getEnvOrDefault("DONATE_TIME_HOURS", "24") // По умолчанию раз в сутки
	donateTimeHours, err := strconv.Atoi(donateTimeHoursStr)
	if err != nil || donateTimeHours < 0 {
		donateTimeHours = 24
	}

	// --- Загрузка переменных для прямых обращений ---
	classifyDirectMessagePrompt := getEnvOrDefault("CLASSIFY_DIRECT_MESSAGE_PROMPT", "Классифицируй сообщение как serious или casual")
	seriousDirectPrompt := getEnvOrDefault("SERIOUS_DIRECT_PROMPT", "Дай серьезный ответ")

	// --- Загрузка переменных для Free Will Direct Response ---
	botNamesStr := getEnvOrDefault("BOT_NAMES", "Катя,катя,Катюша,катюша,Luna,luna")
	var botNames []string
	if botNamesStr != "" {
		botNames = strings.Split(botNamesStr, ",")
		for i := range botNames {
			botNames[i] = strings.TrimSpace(botNames[i])
		}
	}
	freeWillDirectResponseDecisionPrompt := getEnvOrDefault("FREE_WILL_DIRECT_RESPONSE_DECISION_PROMPT", "Решай отвечать или нет на прямые обращения")
	freeWillDirectResponsePrompt := getEnvOrDefault("FREE_WILL_DIRECT_RESPONSE_PROMPT", "Отвечай на прямые обращения через Free Will decision")

	// --- Загрузка настроек лимитов для прямых обращений ---
	freeWillDirectResponseMaxPerHour := parseIntOrDefault(getEnvOrDefault("FREE_WILL_DIRECT_RESPONSE_MAX_PER_HOUR", "30"), 30)
	freeWillDirectResponseMinIntervalSeconds := parseFloatOrDefault(getEnvOrDefault("FREE_WILL_DIRECT_RESPONSE_MIN_INTERVAL_SECONDS", "5"), 5)
	freeWillDirectResponseIndependentLimits := parseBoolOrDefault(getEnvOrDefault("FREE_WILL_DIRECT_RESPONSE_INDEPENDENT_LIMITS", "true"), true)

	// --- Загрузка настроек лимитов для генерации изображений ---
	freeWillImageGenerationMaxDecisionsPerInterval := parseIntOrDefault(getEnvOrDefault("FREE_WILL_IMAGE_GENERATION_MAX_DECISIONS_PER_INTERVAL", "3"), 3)
	freeWillImageGenerationIntervalHours := parseIntOrDefault(getEnvOrDefault("FREE_WILL_IMAGE_GENERATION_INTERVAL_HOURS", "6"), 6)
	freeWillImageGenerationMinDecisionIntervalMinutes := parseIntOrDefault(getEnvOrDefault("FREE_WILL_IMAGE_GENERATION_MIN_DECISION_INTERVAL_MINUTES", "30"), 30)
	freeWillImageGenerationIndependentLimits := parseBoolOrDefault(getEnvOrDefault("FREE_WILL_IMAGE_GENERATION_INDEPENDENT_LIMITS", "true"), true)
	imageGenFrequencyHours := parseIntOrDefault(getEnvOrDefault("IMAGE_GEN_FREQUENCY_HOURS", "12"), 12)

	// --- Загрузка переменных для срача ---
	srachWarningPrompt := getEnvOrDefault("SRACH_WARNING_PROMPT", "Зафиксирован повышенный градус дискуссии! Делайте ваши ставки, господа!")
	srachAnalysisPrompt := getEnvOrDefault("SRACH_ANALYSIS_PROMPT", "Проанализируй аргументы сторон и подведи итог.")
	srachConfirmPrompt := getEnvOrDefault("SRACH_CONFIRM_PROMPT", "Является ли следующее сообщение частью спора? Ответь только 'true' или 'false'.")
	srachAnalysisEnabled := parseBoolOrDefault(getEnvOrDefault("SRACH_ANALYSIS_ENABLED", "true"), true)

	// --- Загрузка переменных PostgreSQL ---
	dbHost := getEnvOrDefault("POSTGRESQL_HOST", "")
	dbPort := getEnvOrDefault("POSTGRESQL_PORT", "5432")
	dbUser := getEnvOrDefault("POSTGRESQL_USER", "")
	dbPassword := getEnvOrDefault("POSTGRESQL_PASSWORD", "")
	dbName := getEnvOrDefault("POSTGRESQL_DBNAME", "")

	// --- Загрузка переменных MongoDB ---
	mongoURI := getEnvOrDefault("MONGODB_URI", "")
	mongoDbName := getEnvOrDefault("MONGODB_DBNAME", "rofloslav")
	mongoMessagesCollection := getEnvOrDefault("MONGODB_MESSAGES_COLLECTION", "messages")
	mongoUserProfilesCollection := getEnvOrDefault("MONGODB_USER_PROFILES_COLLECTION", "user_profiles")
	mongoSettingsCollection := getEnvOrDefault("MONGODB_SETTINGS_COLLECTION", "chat_settings")

	// --- Загрузка прочих переменных ---
	storageTypeStr := strings.ToLower(getEnvOrDefault("STORAGE_TYPE", string(StorageTypePostgres)))
	welcomePrompt := getEnvOrDefault("WELCOME_PROMPT", "Привет, чат! Я ваш новый спутник в беседе. Погнали!")
	voiceFormatPrompt := getEnvOrDefault("VOICE_FORMAT_PROMPT", "Расставь знаки препинания и разбей на абзацы")

	// --- Загрузка настроек для анализа фотографий ---
	photoAnalysisEnabled := parseBoolOrDefault(getEnvOrDefault("PHOTO_ANALYSIS_ENABLED", "false"), false)

	// --- Загрузка переменных MIN_MESSAGES и MAX_MESSAGES ---
	minMessages := parseIntOrDefault(getEnvOrDefault("MIN_MESSAGES", "20"), 20)
	maxMessages := parseIntOrDefault(getEnvOrDefault("MAX_MESSAGES", "100"), 100)
	dailyTakeTime := parseIntOrDefault(getEnvOrDefault("DAILY_TAKE_TIME", "20"), 20)
	timeZone := getEnvOrDefault("TIME_ZONE", "")
	summaryIntervalHours := parseIntOrDefault(getEnvOrDefault("SUMMARY_INTERVAL_HOURS", "0"), 0)
	// --- Загрузка настроек еженедельного саммари ---
	weeklySummaryEnabled := parseBoolOrDefault(getEnvOrDefault("WEEKLY_SUMMARY_ENABLED", "false"), false)
	weeklySummaryDay := parseIntOrDefault(getEnvOrDefault("WEEKLY_SUMMARY_DAY", "0"), 0)
	weeklySummaryHour := parseIntOrDefault(getEnvOrDefault("WEEKLY_SUMMARY_HOUR", "18"), 18)
	weeklySummaryMinute := parseIntOrDefault(getEnvOrDefault("WEEKLY_SUMMARY_MINUTE", "0"), 0)
	weeklySummaryMaxParts := parseIntOrDefault(getEnvOrDefault("WEEKLY_SUMMARY_MAX_PARTS", "5"), 5)
	summaryMaxParts := parseIntOrDefault(getEnvOrDefault("SUMMARY_MAX_PARTS", "5"), 5)

	weeklySummaryPrompt := getEnvOrDefault("WEEKLY_SUMMARY_PROMPT", "Создай еженедельное саммари на основе дневных саммари")
	// --- Конец загрузки настроек еженедельного саммари ---
	contextWindow := parseIntOrDefault(getEnvOrDefault("CONTEXT_WINDOW", "10"), 10)
	imageGenerationContextWindow := parseIntOrDefault(getEnvOrDefault("IMAGE_GENERATION_CONTEXT_WINDOW", "50"), 50)
	debug := parseBoolOrDefault(getEnvOrDefault("DEBUG", "true"), true)

	// --- Загрузка переменных для промптов ---
	dailyTakePrompt := getEnvOrDefault("DAILY_TAKE_PROMPT", "Сгенерируй провокационную тему для обсуждения")
	summaryPrompt := getEnvOrDefault("SUMMARY_PROMPT", "Создай краткое саммари")
	donatePrompt := getEnvOrDefault("DONATE_PROMPT", "Поддержи разработку!")
	rateLimitStaticText := getEnvOrDefault("RATE_LIMIT_STATIC_TEXT", "Превышен лимит сообщений.")
	rateLimitPrompt := getEnvOrDefault("RATE_LIMIT_PROMPT", "Слишком часто запрашиваешь!")
	promptEnterMinMessages := getEnvOrDefault("PROMPT_ENTER_MIN_MESSAGES", "Введите минимальное количество сообщений:")
	promptEnterMaxMessages := getEnvOrDefault("PROMPT_ENTER_MAX_MESSAGES", "Введите максимальное количество сообщений:")
	promptEnterDailyTime := getEnvOrDefault("PROMPT_ENTER_DAILY_TIME", "Введите время ежедневного сообщения:")
	promptEnterSummaryInterval := getEnvOrDefault("PROMPT_ENTER_SUMMARY_INTERVAL", "Введите интервал саммари:")

	// --- Загрузка переменных для лимитов прямых ответов ---
	directReplyLimitEnabledDefault := parseBoolOrDefault(getEnvOrDefault("DIRECT_REPLY_LIMIT_ENABLED_DEFAULT", "false"), false)
	directReplyLimitCountDefault := parseIntOrDefault(getEnvOrDefault("DIRECT_REPLY_LIMIT_COUNT_DEFAULT", "3"), 3)
	directReplyLimitDurationDefaultMinutes := parseIntOrDefault(getEnvOrDefault("DIRECT_REPLY_LIMIT_DURATION_DEFAULT", "60"), 60)
	directReplyLimitDurationDefault := time.Duration(directReplyLimitDurationDefaultMinutes) * time.Minute
	directReplyLimitPrompt := getEnvOrDefault("DIRECT_REPLY_LIMIT_PROMPT", "Превышен лимит прямых обращений.")
	promptEnterDirectLimitCount := getEnvOrDefault("PROMPT_ENTER_DIRECT_LIMIT_COUNT", "Введите лимит прямых обращений:")
	promptEnterDirectLimitDuration := getEnvOrDefault("PROMPT_ENTER_DIRECT_LIMIT_DURATION", "Введите длительность лимита:")

	// --- Загрузка переменных админов ---
	adminUsernamesStr := getEnvOrDefault("ADMIN_USERNAMES", "")
	var adminUsernames []string
	if adminUsernamesStr != "" {
		adminUsernames = strings.Split(adminUsernamesStr, ",")
		for i := range adminUsernames {
			adminUsernames[i] = strings.TrimSpace(adminUsernames[i])
		}
	}
	adminID := int64(parseIntOrDefault(getEnvOrDefault("ADMIN_ID", "0"), 0))

	// --- Загрузка настройки автоудаления сообщений об ошибках ---
	errorMessageAutoDeleteSeconds := parseIntOrDefault(getEnvOrDefault("ERROR_MESSAGE_AUTO_DELETE_SECONDS", "5"), 5)

	// --- Загрузка переменных AutoBio ---
	autoBioEnabled := parseBoolOrDefault(getEnvOrDefault("AUTO_BIO_ENABLED", "false"), false)
	autoBioIntervalHours := parseIntOrDefault(getEnvOrDefault("AUTO_BIO_INTERVAL_HOURS", "24"), 24)
	// ИСПРАВЛЕНО: Убрали плейсхолдеры личности бота из промптов анализа профилей пользователей
	autoBioInitialAnalysisPrompt := getEnvOrDefault("AUTO_BIO_INITIAL_ANALYSIS_PROMPT", "Проанализируй следующие сообщения пользователя %s и составь краткое резюме его стиля общения, характера и интересов на основе ТОЛЬКО этих сообщений:\n\n%s\n\nСоздай краткую биографию этого пользователя основываясь исключительно на его сообщениях.")
	autoBioUpdatePrompt := getEnvOrDefault("AUTO_BIO_UPDATE_PROMPT", "Обнови резюме пользователя %s на основе новых сообщений.\n\nТекущее резюме:\n%s\n\nНовые сообщения:\n%s\n\nОбнови резюме учитывая новую информацию, но сохраняя предыдущие факты.")
	autoBioMessagesLookbackDays := parseIntOrDefault(getEnvOrDefault("AUTO_BIO_MESSAGES_LOOKBACK_DAYS", "30"), 30)
	autoBioMinMessagesForAnalysis := parseIntOrDefault(getEnvOrDefault("AUTO_BIO_MIN_MESSAGES_FOR_ANALYSIS", "10"), 10)
	autoBioMaxMessagesForAnalysis := parseIntOrDefault(getEnvOrDefault("AUTO_BIO_MAX_MESSAGES_FOR_ANALYSIS", "1000"), 1000)

	// --- Загрузка настроек реакций ---
	reactionsEnabled := parseBoolOrDefault(getEnvOrDefault("REACTIONS_ENABLED", "true"), true)
	clownReactionPrompt := getEnvOrDefault("CLOWN_REACTION_PROMPT", "")
	reactionAnalysisPrompt := getEnvOrDefault("REACTION_ANALYSIS_PROMPT", "")
	// Новые настройки для предотвращения бесконечного цикла
	clownResponseProbability := parseIntOrDefault(getEnvOrDefault("CLOWN_RESPONSE_PROBABILITY", "40"), 40)
	clownCooldownSeconds := parseIntOrDefault(getEnvOrDefault("CLOWN_COOLDOWN_SECONDS", "30"), 30)
	maxClownResponsesPerHour := parseIntOrDefault(getEnvOrDefault("MAX_CLOWN_RESPONSES_PER_HOUR", "10"), 10)

	// --- Загрузка настроек веб-поиска ---
	webSearchEnabled := parseBoolOrDefault(getEnvOrDefault("WEB_SEARCH_ENABLED", "true"), true)
	googleSearchAPIKey := getEnvOrDefault("GOOGLE_SEARCH_API_KEY", "")
	googleSearchEngineID := getEnvOrDefault("GOOGLE_SEARCH_ENGINE_ID", "")
	webSearchMaxResults := parseIntOrDefault(getEnvOrDefault("WEB_SEARCH_MAX_RESULTS", "3"), 3)
	webSearchTriggerPrompt := getEnvOrDefault("WEB_SEARCH_TRIGGER_PROMPT", "Нужен ли веб-поиск для ответа на этот вопрос? Ответь только 'yes' или 'no'.")

	// --- Загрузка настроек кэширования веб-поиска ---
	webSearchCacheTTLString := getEnvOrDefault("WEB_SEARCH_CACHE_TTL", "5m")
	webSearchCacheTTL, cacheTTLErr := time.ParseDuration(webSearchCacheTTLString)
	if cacheTTLErr != nil {
		log.Printf("[WARN] Ошибка парсинга WEB_SEARCH_CACHE_TTL: %v, используется значение по умолчанию 5m", cacheTTLErr)
		webSearchCacheTTL = 5 * time.Minute
	}
	webSearchCacheMaxSize := parseIntOrDefault(getEnvOrDefault("WEB_SEARCH_CACHE_MAX_SIZE", "100"), 100)

	// --- Загрузка настроек ElevenLabs ---
	elevenLabsAPIKey := getEnvOrDefault("ELEVENLABS_API_KEY", "")
	elevenLabsVoiceID := getEnvOrDefault("ELEVENLABS_VOICE_ID", "Obuyk6KKzg9olSLPaCbl")
	elevenLabsModel := getEnvOrDefault("ELEVENLABS_MODEL", "eleven_multilingual_v2")
	elevenLabsPlan := getEnvOrDefault("ELEVENLABS_PLAN", "starter")

	// --- Загрузка расширенных настроек голоса ---
	elevenLabsStability := parseFloatOrDefault(getEnvOrDefault("ELEVENLABS_STABILITY", "0.5"), 0.5)
	elevenLabsSimilarityBoost := parseFloatOrDefault(getEnvOrDefault("ELEVENLABS_SIMILARITY_BOOST", "0.8"), 0.8)
	elevenLabsStyle := parseFloatOrDefault(getEnvOrDefault("ELEVENLABS_STYLE", "0.0"), 0.0)
	elevenLabsUseSpeakerBoost := parseBoolOrDefault(getEnvOrDefault("ELEVENLABS_USE_SPEAKER_BOOST", "true"), true)
	elevenLabsSpeed := parseFloatOrDefault(getEnvOrDefault("ELEVENLABS_SPEED", "1.0"), 1.0)

	// --- Загрузка промпт-настроек ---
	elevenLabsStylePrompt := getEnvOrDefault("ELEVENLABS_STYLE_PROMPT", "")
	elevenLabsEmotionPrompt := getEnvOrDefault("ELEVENLABS_EMOTION_PROMPT", "")
	elevenLabsPacePrompt := getEnvOrDefault("ELEVENLABS_PACE_PROMPT", "")

	// --- Загрузка дополнительных настроек ---
	elevenLabsRandomVoice := parseBoolOrDefault(getEnvOrDefault("ELEVENLABS_RANDOM_VOICE", "false"), false)

	// --- Загрузка настроек голосовых сообщений ---
	voiceMessagesEnabled := parseBoolOrDefault(getEnvOrDefault("VOICE_MESSAGES_ENABLED", "true"), true)
	minVoiceMessages := parseIntOrDefault(getEnvOrDefault("MIN_VOICE_MESSAGES", "50"), 50)
	maxVoiceMessages := parseIntOrDefault(getEnvOrDefault("MAX_VOICE_MESSAGES", "100"), 100)
	voiceMessageTempDir := getEnvOrDefault("VOICE_MESSAGE_TEMP_DIR", "/tmp/voice_messages")
	voiceMessagesPrompt := getEnvOrDefault("VOICE_MESSAGE_PROMPT", "Сгенерируй голосовое сообщение")

	// Логируем загрузку настроек ElevenLabs (маскируем API ключ)
	if elevenLabsAPIKey != "" {
		log.Printf("ElevenLabs API Key загружен: %s", maskSecret(elevenLabsAPIKey))
		log.Printf("ElevenLabs Voice ID: %s", elevenLabsVoiceID)
		log.Printf("ElevenLabs Plan: %s", elevenLabsPlan)
		log.Printf("Voice Messages Interval: %d-%d", 50, 100)
	} else {
		log.Printf("ВНИМАНИЕ: ELEVENLABS_API_KEY не установлен - голосовые сообщения отключены")
	}

	// --- Загрузка настроек автоочистки MongoDB ---
	mongoCleanupEnabled := parseBoolOrDefault(getEnvOrDefault("MONGO_CLEANUP_ENABLED", "false"), false)
	mongoCleanupSizeLimitMB := parseIntOrDefault(getEnvOrDefault("MONGO_CLEANUP_SIZE_LIMIT_MB", "450"), 450)
	mongoCleanupIntervalMinutes := parseIntOrDefault(getEnvOrDefault("MONGO_CLEANUP_INTERVAL_MINUTES", "60"), 60)
	mongoCleanupChunkDurationHours := parseIntOrDefault(getEnvOrDefault("MONGO_CLEANUP_CHUNK_DURATION_HOURS", "24"), 24)

	// --- Загрузка настроек модерации ---
	modEnabled := parseBoolOrDefault(getEnvOrDefault("MOD_ENABLED", "false"), false)
	modInterval := parseIntOrDefault(getEnvOrDefault("MOD_INTERVAL", "1"), 1)
	modMuteTimeMin := parseIntOrDefault(getEnvOrDefault("MOD_MUTE_TIME_MIN", "5"), 5)
	modBanTimeMin := parseIntOrDefault(getEnvOrDefault("MOD_BAN_TIME_MIN", "60"), 60)
	modPurgeDurationStr := getEnvOrDefault("MOD_PURGE_WINDOW_DURATION", "1h")
	modPurgeDuration, err := time.ParseDuration(modPurgeDurationStr)
	if err != nil {
		log.Printf("Ошибка парсинга MOD_PURGE_WINDOW_DURATION ('%s'): %v. Используется значение по умолчанию 1h.", modPurgeDurationStr, err)
		modPurgeDuration = time.Hour
	}

	modPurgeDelayStr := getEnvOrDefault("MOD_PURGE_DELAY_DURATION", "0s")
	modPurgeDelay, err := time.ParseDuration(modPurgeDelayStr)
	if err != nil {
		log.Printf("Ошибка парсинга MOD_PURGE_DELAY_DURATION ('%s'): %v. Используется значение по умолчанию 0s.", modPurgeDelayStr, err)
		modPurgeDelay = 0
	}
	modCheckAdminRights := parseBoolOrDefault(getEnvOrDefault("MOD_CHECK_ADMIN_RIGHTS", "true"), true)
	modDefaultNotify := parseBoolOrDefault(getEnvOrDefault("MOD_DEFAULT_NOTIFY", "false"), false)

	// --- Загрузка флага дисамбигуации пользователей ---
	disambiguationEnabled := parseBoolOrDefault(getEnvOrDefault("DISAMBIGUATION_ENABLED", "true"), true)

	// Загрузка правил модерации из JSON
	var modRules []ModerationRule
	modRulesStr := getEnvOrDefault("MOD_RULES", "[]")
	if err := json.Unmarshal([]byte(modRulesStr), &modRules); err != nil {
		log.Printf("Ошибка парсинга MOD_RULES: %v", err)
		modRules = []ModerationRule{}
	}

	// Парсим ChatID и UserID из строк в int64 для каждого правила
	for i := range modRules {
		if modRules[i].ChatID != "" && strings.ToLower(modRules[i].ChatID) != "none" {
			parsedID, err := strconv.ParseInt(modRules[i].ChatID, 10, 64)
			if err != nil {
				log.Printf("Ошибка парсинга ChatID ('%s') для правила '%s': %v. ChatID будет считаться как 'для всех'.", modRules[i].ChatID, modRules[i].RuleName, err)
				modRules[i].ParsedChatID = -1 // Используем -1 как индикатор "для всех", если парсинг не удался
			} else {
				modRules[i].ParsedChatID = parsedID
			}
		} else {
			// "none" или пустая строка означает "для всех чатов"
			modRules[i].ParsedChatID = -1 // Используем -1 как маркер "все чаты"
		}

		if modRules[i].UserID != "" && strings.ToLower(modRules[i].UserID) != "none" {
			parsedID, err := strconv.ParseInt(modRules[i].UserID, 10, 64)
			if err != nil {
				log.Printf("Ошибка парсинга UserID ('%s') для правила '%s': %v. UserID будет считаться как 'для всех'.", modRules[i].UserID, modRules[i].RuleName, err)
				modRules[i].ParsedUserID = -1 // Используем -1 как индикатор "для всех", если парсинг не удался
			} else {
				modRules[i].ParsedUserID = parsedID
			}
		} else {
			// "none" или пустая строка означает "для всех пользователей"
			modRules[i].ParsedUserID = -1 // Используем -1 как маркер "все пользователи"
		}

		if debug { // Используем существующую переменную debug из Load()
			log.Printf("[DEBUG Config] Правило '%s': ChatID='%s' -> ParsedChatID=%d, UserID='%s' -> ParsedUserID=%d",
				modRules[i].RuleName, modRules[i].ChatID, modRules[i].ParsedChatID, modRules[i].UserID, modRules[i].ParsedUserID)
		}
	}

	// --- Загрузка настроек памяти личности ---
	personalityUpdateIntervalHours := parseIntOrDefault(getEnvOrDefault("PERSONALITY_UPDATE_INTERVAL_HOURS", "1"), 1)
	personalityMessagesLookback := parseIntOrDefault(getEnvOrDefault("PERSONALITY_MESSAGES_LOOKBACK", "50"), 50)
	personalityAnalysisPrompt := getEnvOrDefault("PERSONALITY_ANALYSIS_PROMPT", "")
	personalityNameAnalysisPrompt := getEnvOrDefault("PERSONALITY_NAME_ANALYSIS_PROMPT", "")
	personalityTopicAnalysisPrompt := getEnvOrDefault("PERSONALITY_TOPIC_ANALYSIS_PROMPT", "")
	personalitySelfUpdatePrompt := getEnvOrDefault("PERSONALITY_SELF_UPDATE_PROMPT", "")
	maxNameMentions := parseIntOrDefault(getEnvOrDefault("MAX_NAME_MENTIONS", "10"), 10)
	maxRecentTopics := parseIntOrDefault(getEnvOrDefault("MAX_RECENT_TOPICS", "10"), 10)
	maxSelfPerceptions := parseIntOrDefault(getEnvOrDefault("MAX_SELF_PERCEPTIONS", "5"), 5)
	maxDiscussionContexts := parseIntOrDefault(getEnvOrDefault("MAX_DISCUSSION_CONTEXTS", "3"), 3)

	// --- Загрузка настроек долгосрочной памяти ---
	longTermMemoryEnabled := parseBoolOrDefault(getEnvOrDefault("LONG_TERM_MEMORY_ENABLED", "false"), false)
	mongoVectorIndexName := getEnvOrDefault("MONGO_VECTOR_INDEX_NAME", "vector_index")
	longTermMemoryFetchK := parseIntOrDefault(getEnvOrDefault("LONG_TERM_MEMORY_FETCH_K", "5"), 5)
	backfillBatchSize := parseIntOrDefault(getEnvOrDefault("BACKFILL_BATCH_SIZE", "100"), 100)
	backfillBatchDelaySeconds := parseIntOrDefault(getEnvOrDefault("BACKFILL_BATCH_DELAY", "1"), 1)
	backfillBatchDelay := time.Duration(backfillBatchDelaySeconds) * time.Second

	// --- Загрузка настроек Free Will ---
	freeWillEnabled := parseBoolOrDefault(getEnvOrDefault("FREE_WILL_ENABLED", "false"), false)
	freeWillMinIntervalMinutes := parseFloatOrDefault(getEnvOrDefault("FREE_WILL_MIN_INTERVAL_MINUTES", "15.0"), 15.0)
	freeWillMaxIntervalMinutes := parseFloatOrDefault(getEnvOrDefault("FREE_WILL_MAX_INTERVAL_MINUTES", "60.0"), 60.0)
	freeWillContextWindow := parseIntOrDefault(getEnvOrDefault("FREE_WILL_CONTEXT_WINDOW", "50"), 50)
	freeWillMoodUpdateProbability := parseFloatOrDefault(getEnvOrDefault("FREE_WILL_MOOD_UPDATE_PROBABILITY", "0.1"), 0.1)
	freeWillMaxDecisionsPerHour := parseIntOrDefault(getEnvOrDefault("FREE_WILL_MAX_DECISIONS_PER_HOUR", "10"), 10)
	freeWillVoiceProbability := parseFloatOrDefault(getEnvOrDefault("FREE_WILL_VOICE_PROBABILITY", "0.3"), 0.3)

	// Новые параметры для реакции на тишину
	freeWillSilenceMinMinutes := parseFloatOrDefault(getEnvOrDefault("FREE_WILL_SILENCE_MIN_MINUTES", "3.0"), 3.0)
	freeWillSilenceMaxMinutes := parseFloatOrDefault(getEnvOrDefault("FREE_WILL_SILENCE_MAX_MINUTES", "5.0"), 5.0)

	// Промпты Free Will
	freeWillShouldReplyPrompt := getEnvOrDefault("FREE_WILL_SHOULD_REPLY_PROMPT", "")
	freeWillResponseTypePrompt := getEnvOrDefault("FREE_WILL_RESPONSE_TYPE_PROMPT", "")
	freeWillDirectPrompt := getEnvOrDefault("FREE_WILL_DIRECT_PROMPT", "")
	freeWillGeneralPrompt := getEnvOrDefault("FREE_WILL_GENERAL_PROMPT", "")
	freeWillContextPrompt := getEnvOrDefault("FREE_WILL_CONTEXT_PROMPT", "")
	freeWillSilencePrompt := getEnvOrDefault("FREE_WILL_SILENCE_PROMPT", "")
	freeWillMoodAnalysisPrompt := getEnvOrDefault("FREE_WILL_MOOD_ANALYSIS_PROMPT", "")
	freeWillTakeResponsePrompt := getEnvOrDefault("FREE_WILL_TAKE_RESPONSE_PROMPT", "")

	// --- Настройки реакций Free Will ---
	freeWillReactionsEnabled := parseBoolOrDefault(getEnvOrDefault("FREE_WILL_REACTIONS_ENABLED", "true"), true)
	freeWillReactionsProbability := parseFloatOrDefault(getEnvOrDefault("FREE_WILL_REACTIONS_PROBABILITY", "0.2"), 0.2)
	freeWillReactionsCooldownMinutes := parseIntOrDefault(getEnvOrDefault("FREE_WILL_REACTIONS_COOLDOWN_MINUTES", "5"), 5)
	freeWillReactionsMaxPerHour := parseIntOrDefault(getEnvOrDefault("FREE_WILL_REACTIONS_MAX_PER_HOUR", "15"), 15)
	freeWillReactionPrompt := getEnvOrDefault("FREE_WILL_REACTION_PROMPT", "")

	// Контроль старого интервального механизма
	intervalMessagesEnabled := parseBoolOrDefault(getEnvOrDefault("INTERVAL_MESSAGES_ENABLED", "true"), true)

	// --- Загрузка настроек системы анти-повторений ---
	antiRepetitionEnabled := parseBoolOrDefault(getEnvOrDefault("ANTI_REPETITION_ENABLED", "true"), true)
	antiRepetitionMaxResponsesPerChat := parseIntOrDefault(getEnvOrDefault("ANTI_REPETITION_MAX_RESPONSES_PER_CHAT", "20"), 20)
	antiRepetitionSimilarityThreshold := parseFloatOrDefault(getEnvOrDefault("ANTI_REPETITION_SIMILARITY_THRESHOLD", "0.75"), 0.75)
	antiRepetitionTimeWindowHours := parseIntOrDefault(getEnvOrDefault("ANTI_REPETITION_TIME_WINDOW_HOURS", "24"), 24)
	antiRepetitionCleanupIntervalHours := parseIntOrDefault(getEnvOrDefault("ANTI_REPETITION_CLEANUP_INTERVAL_HOURS", "1"), 1)

	// --- Настройки переработки повторений ---
	antiRepetitionReworkEnabled := parseBoolOrDefault(getEnvOrDefault("ANTI_REPETITION_REWORK_ENABLED", "true"), true)
	antiRepetitionMaxReworkAttempts := parseIntOrDefault(getEnvOrDefault("ANTI_REPETITION_MAX_REWORK_ATTEMPTS", "2"), 2)
	antiRepetitionReworkTemperature := parseFloatOrDefault(getEnvOrDefault("ANTI_REPETITION_REWORK_TEMPERATURE", "0.8"), 0.8)
	antiRepetitionReworkPrompt := getEnvOrDefault("ANTI_REPETITION_REWORK_PROMPT", "")
	antiRepetitionLocalReworkEnabled := parseBoolOrDefault(getEnvOrDefault("ANTI_REPETITION_LOCAL_REWORK_ENABLED", "true"), true)
	antiRepetitionLocalReworkMaxLength := parseIntOrDefault(getEnvOrDefault("ANTI_REPETITION_LOCAL_REWORK_MAX_LENGTH", "50"), 50)

	// --- Загрузка настроек каузального обучения (Этап 1) ---
	causalLearningEnabled := parseBoolOrDefault(getEnvOrDefault("CAUSAL_LEARNING_ENABLED", "false"), false)

	// Association Cloud flags
	associationCloudEnabled := parseBoolOrDefault(getEnvOrDefault("ASSOCIATION_CLOUD_ENABLED", "false"), false)
	associationCloudMaxNodes := parseIntOrDefault(getEnvOrDefault("ASSOCIATION_CLOUD_MAX_NODES", "5000"), 5000)
	associationCloudMaxEdges := parseIntOrDefault(getEnvOrDefault("ASSOCIATION_CLOUD_MAX_EDGES", "50000"), 50000)
	associationCloudDecayDays := parseIntOrDefault(getEnvOrDefault("ASSOCIATION_CLOUD_DECAY_DAYS", "30"), 30)
	causalAnalysisIntervalHours := parseIntOrDefault(getEnvOrDefault("CAUSAL_ANALYSIS_INTERVAL_HOURS", "4"), 4)
	causalMinConfidence := parseFloatOrDefault(getEnvOrDefault("CAUSAL_MIN_CONFIDENCE", "0.3"), 0.3)
	causalTemporalWindowMinutes := parseIntOrDefault(getEnvOrDefault("CAUSAL_TEMPORAL_WINDOW_MINUTES", "60"), 60)
	causalMaxEntriesPerChat := parseIntOrDefault(getEnvOrDefault("CAUSAL_MAX_ENTRIES_PER_CHAT", "500"), 500)
	causalAnalysisLookbackMessages := parseIntOrDefault(getEnvOrDefault("CAUSAL_ANALYSIS_LOOKBACK_MESSAGES", "100"), 100)
	causalAnalysisPrompt := getEnvOrDefault("CAUSAL_ANALYSIS_PROMPT", "")
	causalAnalysisPromptProvider := getEnvOrDefault("CAUSAL_ANALYSIS_PROMPT_PROVIDER", "gemini")
	causalAnalysisPromptModel := getEnvOrDefault("CAUSAL_ANALYSIS_PROMPT_MODEL", "gemini-2.5-flash")
	causalAnalysisPromptTemperature := parseFloatOrDefault(getEnvOrDefault("CAUSAL_ANALYSIS_PROMPT_TEMPERATURE", "0.7"), 0.7)
	causalAnalysisPromptEnabled := parseBoolOrDefault(getEnvOrDefault("CAUSAL_ANALYSIS_PROMPT_ENABLED", "true"), true)
	causalInfluencePrompt := getEnvOrDefault("CAUSAL_INFLUENCE_PROMPT", "")
	causalInfluencePromptProvider := getEnvOrDefault("CAUSAL_INFLUENCE_PROMPT_PROVIDER", "gemini")
	causalInfluencePromptModel := getEnvOrDefault("CAUSAL_INFLUENCE_PROMPT_MODEL", "gemini-2.0-flash")
	causalInfluencePromptTemperature := parseFloatOrDefault(getEnvOrDefault("CAUSAL_INFLUENCE_PROMPT_TEMPERATURE", "0.6"), 0.6)
	causalInfluencePromptEnabled := parseBoolOrDefault(getEnvOrDefault("CAUSAL_INFLUENCE_PROMPT_ENABLED", "true"), true)

	// --- Загрузка настроек эмоциональной системы (Этап 2) ---
	emotionalLearningEnabled := parseBoolOrDefault(getEnvOrDefault("EMOTIONAL_LEARNING_ENABLED", "true"), true)
	emotionalAnalysisIntervalHours := parseIntOrDefault(getEnvOrDefault("EMOTIONAL_ANALYSIS_INTERVAL_HOURS", "2"), 2)
	emotionalAnalysisLookbackMessages := parseIntOrDefault(getEnvOrDefault("EMOTIONAL_ANALYSIS_LOOKBACK_MESSAGES", "100"), 100)
	emotionalMemoryRetentionDays := parseIntOrDefault(getEnvOrDefault("EMOTIONAL_MEMORY_RETENTION_DAYS", "30"), 30)
	emotionalMinMessagesForAnalysis := parseIntOrDefault(getEnvOrDefault("EMOTIONAL_MIN_MESSAGES_FOR_ANALYSIS", "20"), 20)
	emotionalAnalysisDebounceHours := parseIntOrDefault(getEnvOrDefault("EMOTIONAL_ANALYSIS_DEBOUNCE_HOURS", "6"), 6)

	emotionalAnalysisPrompt := getEnvOrDefault("EMOTIONAL_ANALYSIS_PROMPT", "")
	emotionalAnalysisPromptProvider := getEnvOrDefault("EMOTIONAL_ANALYSIS_PROMPT_PROVIDER", "gemini")
	emotionalAnalysisPromptModel := getEnvOrDefault("EMOTIONAL_ANALYSIS_PROMPT_MODEL", "gemini-2.0-flash")
	emotionalAnalysisPromptTemperature := parseFloatOrDefault(getEnvOrDefault("EMOTIONAL_ANALYSIS_PROMPT_TEMPERATURE", "0.8"), 0.8)
	emotionalAnalysisPromptEnabled := parseBoolOrDefault(getEnvOrDefault("EMOTIONAL_ANALYSIS_PROMPT_ENABLED", "true"), true)

	emotionalAdaptationPrompt := getEnvOrDefault("EMOTIONAL_ADAPTATION_PROMPT", "")
	emotionalAdaptationPromptProvider := getEnvOrDefault("EMOTIONAL_ADAPTATION_PROMPT_PROVIDER", "gemini")
	emotionalAdaptationPromptModel := getEnvOrDefault("EMOTIONAL_ADAPTATION_PROMPT_MODEL", "gemini-2.0-flash")
	emotionalAdaptationPromptTemperature := parseFloatOrDefault(getEnvOrDefault("EMOTIONAL_ADAPTATION_PROMPT_TEMPERATURE", "0.7"), 0.7)
	emotionalAdaptationPromptEnabled := parseBoolOrDefault(getEnvOrDefault("EMOTIONAL_ADAPTATION_PROMPT_ENABLED", "true"), true)

	emotionalFeedbackPrompt := getEnvOrDefault("EMOTIONAL_FEEDBACK_PROMPT", "")
	emotionalFeedbackPromptProvider := getEnvOrDefault("EMOTIONAL_FEEDBACK_PROMPT_PROVIDER", "gemini")
	emotionalFeedbackPromptModel := getEnvOrDefault("EMOTIONAL_FEEDBACK_PROMPT_MODEL", "gemini-2.0-flash")
	emotionalFeedbackPromptTemperature := parseFloatOrDefault(getEnvOrDefault("EMOTIONAL_FEEDBACK_PROMPT_TEMPERATURE", "0.6"), 0.6)
	emotionalFeedbackPromptEnabled := parseBoolOrDefault(getEnvOrDefault("EMOTIONAL_FEEDBACK_PROMPT_ENABLED", "true"), true)

	// --- Загрузка настроек системы убеждений (Этап 3 scaffolding) ---
	beliefLearningEnabled := parseBoolOrDefault(getEnvOrDefault("BELIEF_LEARNING_ENABLED", "false"), false)
	beliefAnalysisIntervalHours := parseIntOrDefault(getEnvOrDefault("BELIEF_ANALYSIS_INTERVAL_HOURS", "6"), 6)
	beliefAnalysisLookbackMessages := parseIntOrDefault(getEnvOrDefault("BELIEF_ANALYSIS_LOOKBACK_MESSAGES", "150"), 150)
	beliefAnalysisPrompt := getEnvOrDefault("BELIEF_ANALYSIS_PROMPT", "")
	beliefAnalysisPromptProvider := getEnvOrDefault("BELIEF_ANALYSIS_PROMPT_PROVIDER", "gemini")
	beliefAnalysisPromptModel := getEnvOrDefault("BELIEF_ANALYSIS_PROMPT_MODEL", "gemini-2.0-flash")
	beliefAnalysisPromptTemperature := parseFloatOrDefault(getEnvOrDefault("BELIEF_ANALYSIS_PROMPT_TEMPERATURE", "0.6"), 0.6)
	beliefAnalysisPromptEnabled := parseBoolOrDefault(getEnvOrDefault("BELIEF_ANALYSIS_PROMPT_ENABLED", "true"), true)

	// --- Загрузка настроек когнитивной архитектуры (Этап 3) ---
	internalMonologueEnabled := parseBoolOrDefault(getEnvOrDefault("INTERNAL_MONOLOGUE_ENABLED", "false"), false)
	selfReflectionEnabled := parseBoolOrDefault(getEnvOrDefault("SELF_REFLECTION_ENABLED", "false"), false)
	confidenceCalibrationEnabled := parseBoolOrDefault(getEnvOrDefault("CONFIDENCE_CALIBRATION_ENABLED", "false"), false)
	internalMonologuePrompt := getEnvOrDefault("INTERNAL_MONOLOGUE_PROMPT", "")
	internalMonologuePromptModel := getEnvOrDefault("INTERNAL_MONOLOGUE_PROMPT_MODEL", "gemini-2.0-flash")
	internalMonologuePromptProvider := getEnvOrDefault("INTERNAL_MONOLOGUE_PROMPT_PROVIDER", "gemini")
	internalMonologuePromptEnabled := parseBoolOrDefault(getEnvOrDefault("INTERNAL_MONOLOGUE_PROMPT_ENABLED", "true"), true)
	internalMonologueTemperature := parseFloatOrDefault(getEnvOrDefault("INTERNAL_MONOLOGUE_TEMPERATURE", "0.4"), 0.4)

	// --- Загрузка настроек саморефлексии ---
	selfReflectionPrompt := getEnvOrDefault("SELF_REFLECTION_PROMPT", "")
	selfReflectionPromptModel := getEnvOrDefault("SELF_REFLECTION_PROMPT_MODEL", "gemini-2.0-flash")
	selfReflectionPromptProvider := getEnvOrDefault("SELF_REFLECTION_PROMPT_PROVIDER", "gemini")
	selfReflectionPromptEnabled := parseBoolOrDefault(getEnvOrDefault("SELF_REFLECTION_PROMPT_ENABLED", "true"), true)
	selfReflectionTemperature := parseFloatOrDefault(getEnvOrDefault("SELF_REFLECTION_TEMPERATURE", "0.5"), 0.5)

	// --- Загрузка настроек социальной архитектуры (Этап 4) ---
	relationshipTrackingEnabled := parseBoolOrDefault(getEnvOrDefault("RELATIONSHIP_TRACKING_ENABLED", "false"), false)
	socialLearningEnabled := parseBoolOrDefault(getEnvOrDefault("SOCIAL_LEARNING_ENABLED", "false"), false)
	relationshipAnalysisPrompt := getEnvOrDefault("RELATIONSHIP_ANALYSIS_PROMPT", "")
	relationshipAnalysisEnabled := parseBoolOrDefault(getEnvOrDefault("RELATIONSHIP_ANALYSIS_PROMPT_ENABLED", "true"), true)
	relationshipAnalysisModel := getEnvOrDefault("RELATIONSHIP_ANALYSIS_PROMPT_MODEL", "gemini-2.0-flash")
	relationshipAnalysisProvider := getEnvOrDefault("RELATIONSHIP_ANALYSIS_PROMPT_PROVIDER", "gemini")
	relationshipAnalysisTemp := parseFloatOrDefault(getEnvOrDefault("RELATIONSHIP_ANALYSIS_PROMPT_TEMPERATURE", "0.6"), 0.6)
	intimacyGrowthRate := parseFloatOrDefault(getEnvOrDefault("INTIMACY_GROWTH_RATE", "0.02"), 0.02)
	trustDecayRate := parseFloatOrDefault(getEnvOrDefault("TRUST_DECAY_RATE", "0.01"), 0.01)

	// --- Загрузка настроек отказоустойчивости LLM ---
	llmFallbackEnabled := parseBoolOrDefault(getEnvOrDefault("LLM_FALLBACK_ENABLED", "true"), true)
	llmFallbackCriticalTypesStr := getEnvOrDefault("LLM_FALLBACK_CRITICAL_TYPES", "free_will_silence,free_will_reaction")
	llmFallbackCriticalTypes := []string{}
	if llmFallbackCriticalTypesStr != "" {
		for _, t := range strings.Split(llmFallbackCriticalTypesStr, ",") {
			v := strings.TrimSpace(strings.ToLower(t))
			if v != "" {
				llmFallbackCriticalTypes = append(llmFallbackCriticalTypes, v)
			}
		}
	}
	llmFallbackProviderOrderStr := getEnvOrDefault("LLM_FALLBACK_PROVIDER_ORDER", "gemini,deepseek,openrouter")
	llmFallbackProviderOrder := []string{}
	if llmFallbackProviderOrderStr != "" {
		for _, p := range strings.Split(llmFallbackProviderOrderStr, ",") {
			v := strings.TrimSpace(strings.ToLower(p))
			if v != "" {
				llmFallbackProviderOrder = append(llmFallbackProviderOrder, v)
			}
		}
	}

	// Создаем конфигурацию
	cfg := &Config{
		TelegramToken:                   telegramToken,
		LLMProvider:                     LLMProvider(llmProviderStr),
		DefaultPrompt:                   defaultPrompt,
		DefaultTemperature:              defaultTemperature,
		GeminiAPIKey:                    geminiAPIKey,
		GeminiModelName:                 geminiModelName,
		GeminiTemperatureNormal:         geminiTemperatureNormal,
		GeminiTemperatureSerious:        geminiTemperatureSerious,
		AudioTranscriptionModel:         audioTranscriptionModel,
		AudioTranscriptionTemperature:   audioTranscriptionTemperature,
		ImageGenerationModel:            imageGenerationModel,
		ImageGenerationTemperature:      imageGenerationTemperature,
		ImageGenFrequencyHours:          imageGenFrequencyHours,
		GeminiAPIKeyReserve:             geminiAPIKeyReserve,
		GeminiKeyRotationTimeHours:      geminiKeyRotationTimeHours,
		GeminiEmbeddingModelName:        geminiEmbeddingModelName,
		DeepSeekAPIKey:                  deepSeekAPIKey,
		DeepSeekModelName:               deepSeekModelName,
		DeepSeekBaseURL:                 deepSeekBaseURL,
		OpenRouterAPIKey:                openRouterAPIKey,
		OpenRouterModelName:             openRouterModelName,
		OpenRouterSiteURL:               openRouterSiteURL,
		OpenRouterSiteTitle:             openRouterSiteTitle,
		DonateTimeHours:                 donateTimeHours,
		ClassifyDirectMessagePrompt:     classifyDirectMessagePrompt,
		SeriousDirectPrompt:             seriousDirectPrompt,
		SRACH_WARNING_PROMPT:            srachWarningPrompt,
		SRACH_ANALYSIS_PROMPT:           srachAnalysisPrompt,
		SRACH_CONFIRM_PROMPT:            srachConfirmPrompt,
		SrachAnalysisEnabled:            srachAnalysisEnabled,
		PostgresqlHost:                  dbHost,
		PostgresqlPort:                  dbPort,
		PostgresqlUser:                  dbUser,
		PostgresqlPassword:              dbPassword,
		PostgresqlDbname:                dbName,
		MongoDbURI:                      mongoURI,
		MongoDbName:                     mongoDbName,
		MongoDbMessagesCollection:       mongoMessagesCollection,
		MongoDbUserProfilesCollection:   mongoUserProfilesCollection,
		MongoDbSettingsCollection:       mongoSettingsCollection,
		StorageType:                     StorageType(storageTypeStr),
		WelcomePrompt:                   welcomePrompt,
		VoiceFormatPrompt:               voiceFormatPrompt,
		PhotoAnalysisEnabled:            photoAnalysisEnabled,
		MinMessages:                     minMessages,
		MaxMessages:                     maxMessages,
		DailyTakeTime:                   dailyTakeTime,
		TimeZone:                        timeZone,
		SummaryIntervalHours:            summaryIntervalHours,
		ContextWindow:                   contextWindow,
		ImageGenerationContextWindow:    imageGenerationContextWindow,
		Debug:                           debug,
		DailyTakePrompt:                 dailyTakePrompt,
		SummaryPrompt:                   summaryPrompt,
		DonatePrompt:                    donatePrompt,
		RateLimitStaticText:             rateLimitStaticText,
		RateLimitPrompt:                 rateLimitPrompt,
		PromptEnterMinMessages:          promptEnterMinMessages,
		PromptEnterMaxMessages:          promptEnterMaxMessages,
		PromptEnterDailyTime:            promptEnterDailyTime,
		PromptEnterSummaryInterval:      promptEnterSummaryInterval,
		DirectReplyLimitEnabledDefault:  directReplyLimitEnabledDefault,
		DirectReplyLimitCountDefault:    directReplyLimitCountDefault,
		DirectReplyLimitDurationDefault: directReplyLimitDurationDefault,
		DirectReplyLimitPrompt:          directReplyLimitPrompt,
		PromptEnterDirectLimitCount:     promptEnterDirectLimitCount,
		PromptEnterDirectLimitDuration:  promptEnterDirectLimitDuration,
		AdminUsernames:                  adminUsernames,
		AdminID:                         adminID,
		AutoBioEnabled:                  autoBioEnabled,
		AutoBioIntervalHours:            autoBioIntervalHours,
		AutoBioInitialAnalysisPrompt:    autoBioInitialAnalysisPrompt,
		AutoBioUpdatePrompt:             autoBioUpdatePrompt,
		AutoBioMessagesLookbackDays:     autoBioMessagesLookbackDays,
		AutoBioMinMessagesForAnalysis:   autoBioMinMessagesForAnalysis,
		AutoBioMaxMessagesForAnalysis:   autoBioMaxMessagesForAnalysis,
		ErrorMessageAutoDeleteSeconds:   errorMessageAutoDeleteSeconds,
		ReactionsEnabled:                reactionsEnabled,
		ClownReactionPrompt:             clownReactionPrompt,
		ReactionAnalysisPrompt:          reactionAnalysisPrompt,
		WebSearchEnabled:                webSearchEnabled,
		GoogleSearchAPIKey:              googleSearchAPIKey,
		GoogleSearchEngineID:            googleSearchEngineID,
		WebSearchMaxResults:             webSearchMaxResults,
		WebSearchTriggerPrompt:          webSearchTriggerPrompt,
		ElevenLabsAPIKey:                elevenLabsAPIKey,
		ElevenLabsVoiceID:               elevenLabsVoiceID,
		ElevenLabsModel:                 elevenLabsModel,
		ElevenLabsPlan:                  elevenLabsPlan,
		ElevenLabsStability:             elevenLabsStability,
		ElevenLabsSimilarityBoost:       elevenLabsSimilarityBoost,
		ElevenLabsStyle:                 elevenLabsStyle,
		ElevenLabsUseSpeakerBoost:       elevenLabsUseSpeakerBoost,
		ElevenLabsSpeed:                 elevenLabsSpeed,
		ElevenLabsStylePrompt:           elevenLabsStylePrompt,
		ElevenLabsEmotionPrompt:         elevenLabsEmotionPrompt,
		ElevenLabsPacePrompt:            elevenLabsPacePrompt,
		ElevenLabsRandomVoice:           elevenLabsRandomVoice,
		VoiceMessagesEnabled:            voiceMessagesEnabled,
		MinVoiceMessages:                minVoiceMessages,
		MaxVoiceMessages:                maxVoiceMessages,
		VoiceMessageTempDir:             voiceMessageTempDir,
		VoiceMessagesPrompt:             voiceMessagesPrompt,
		MongoCleanupEnabled:             mongoCleanupEnabled,
		MongoCleanupSizeLimitMB:         mongoCleanupSizeLimitMB,
		MongoCleanupIntervalMinutes:     mongoCleanupIntervalMinutes,
		MongoCleanupChunkDurationHours:  mongoCleanupChunkDurationHours,
		ModEnabled:                      modEnabled,
		ModInterval:                     modInterval,
		ModMuteTimeMin:                  modMuteTimeMin,
		ModBanTimeMin:                   modBanTimeMin,
		ModPurgeDuration:                modPurgeDuration,
		ModPurgeDelay:                   modPurgeDelay,
		ModCheckAdminRights:             modCheckAdminRights,
		ModDefaultNotify:                modDefaultNotify,
		ModRules:                        modRules,
		PersonalityUpdateIntervalHours:  personalityUpdateIntervalHours,
		PersonalityMessagesLookback:     personalityMessagesLookback,
		PersonalityAnalysisPrompt:       personalityAnalysisPrompt,
		PersonalityNameAnalysisPrompt:   personalityNameAnalysisPrompt,
		PersonalityTopicAnalysisPrompt:  personalityTopicAnalysisPrompt,
		PersonalitySelfUpdatePrompt:     personalitySelfUpdatePrompt,
		MaxNameMentions:                 maxNameMentions,
		MaxRecentTopics:                 maxRecentTopics,
		MaxSelfPerceptions:              maxSelfPerceptions,
		MaxDiscussionContexts:           maxDiscussionContexts,
		LongTermMemoryEnabled:           longTermMemoryEnabled,
		MongoVectorIndexName:            mongoVectorIndexName,
		LongTermMemoryFetchK:            longTermMemoryFetchK,
		BackfillBatchSize:               backfillBatchSize,
		BackfillBatchDelay:              backfillBatchDelay,
		ClownResponseProbability:        clownResponseProbability,
		ClownCooldownSeconds:            clownCooldownSeconds,
		MaxClownResponsesPerHour:        maxClownResponsesPerHour,
		WebSearchCacheTTL:               webSearchCacheTTL,
		WebSearchCacheMaxSize:           webSearchCacheMaxSize,
		DisambiguationEnabled:           disambiguationEnabled,
		// Free Will настройки
		FreeWillEnabled:                                   freeWillEnabled,
		FreeWillMinIntervalMinutes:                        freeWillMinIntervalMinutes,
		FreeWillMaxIntervalMinutes:                        freeWillMaxIntervalMinutes,
		FreeWillContextWindow:                             freeWillContextWindow,
		FreeWillMoodUpdateProbability:                     freeWillMoodUpdateProbability,
		FreeWillMaxDecisionsPerHour:                       freeWillMaxDecisionsPerHour,
		FreeWillVoiceProbability:                          freeWillVoiceProbability,
		FreeWillSilenceMinMinutes:                         freeWillSilenceMinMinutes,
		FreeWillSilenceMaxMinutes:                         freeWillSilenceMaxMinutes,
		FreeWillShouldReplyPrompt:                         freeWillShouldReplyPrompt,
		FreeWillResponseTypePrompt:                        freeWillResponseTypePrompt,
		FreeWillDirectPrompt:                              freeWillDirectPrompt,
		FreeWillGeneralPrompt:                             freeWillGeneralPrompt,
		FreeWillContextPrompt:                             freeWillContextPrompt,
		FreeWillSilencePrompt:                             freeWillSilencePrompt,
		FreeWillMoodAnalysisPrompt:                        freeWillMoodAnalysisPrompt,
		IntervalMessagesEnabled:                           intervalMessagesEnabled,
		FreeWillTakeResponsePrompt:                        freeWillTakeResponsePrompt,
		BotNames:                                          botNames,
		FreeWillDirectResponseDecisionPrompt:              freeWillDirectResponseDecisionPrompt,
		FreeWillDirectResponsePrompt:                      freeWillDirectResponsePrompt,
		FreeWillDirectResponseMaxPerHour:                  freeWillDirectResponseMaxPerHour,
		FreeWillDirectResponseMinIntervalSeconds:          freeWillDirectResponseMinIntervalSeconds,
		FreeWillDirectResponseIndependentLimits:           freeWillDirectResponseIndependentLimits,
		FreeWillImageGenerationMaxDecisionsPerInterval:    freeWillImageGenerationMaxDecisionsPerInterval,
		FreeWillImageGenerationIntervalHours:              freeWillImageGenerationIntervalHours,
		FreeWillImageGenerationMinDecisionIntervalMinutes: freeWillImageGenerationMinDecisionIntervalMinutes,
		FreeWillImageGenerationIndependentLimits:          freeWillImageGenerationIndependentLimits,
		FreeWillReactionsEnabled:                          freeWillReactionsEnabled,
		FreeWillReactionsProbability:                      freeWillReactionsProbability,
		FreeWillReactionsCooldownMinutes:                  freeWillReactionsCooldownMinutes,
		FreeWillReactionsMaxPerHour:                       freeWillReactionsMaxPerHour,
		FreeWillReactionPrompt:                            freeWillReactionPrompt,
		AntiRepetitionEnabled:                             antiRepetitionEnabled,
		AntiRepetitionMaxResponsesPerChat:                 antiRepetitionMaxResponsesPerChat,
		AntiRepetitionSimilarityThreshold:                 antiRepetitionSimilarityThreshold,
		AntiRepetitionTimeWindowHours:                     antiRepetitionTimeWindowHours,
		AntiRepetitionCleanupIntervalHours:                antiRepetitionCleanupIntervalHours,
		AntiRepetitionReworkEnabled:                       antiRepetitionReworkEnabled,
		AntiRepetitionMaxReworkAttempts:                   antiRepetitionMaxReworkAttempts,
		AntiRepetitionReworkTemperature:                   antiRepetitionReworkTemperature,
		AntiRepetitionReworkPrompt:                        antiRepetitionReworkPrompt,
		AntiRepetitionLocalReworkEnabled:                  antiRepetitionLocalReworkEnabled,
		AntiRepetitionLocalReworkMaxLength:                antiRepetitionLocalReworkMaxLength,
		// --- Настройки еженедельного саммари ---
		WeeklySummaryEnabled:  weeklySummaryEnabled,
		WeeklySummaryDay:      weeklySummaryDay,
		WeeklySummaryHour:     weeklySummaryHour,
		WeeklySummaryMinute:   weeklySummaryMinute,
		WeeklySummaryMaxParts: weeklySummaryMaxParts,
		SummaryMaxParts:       summaryMaxParts,
		WeeklySummaryPrompt:   weeklySummaryPrompt,
		// --- Конец настроек еженедельного саммари ---

		// === НОВАЯ ОПЦИЯ СТРУКТУРИРОВАННОГО ФОРМАТИРОВАНИЯ ===
		// Включает новый формат сообщений с тегами [MSG_START]/[MSG_END]
		// для лучшего понимания LLM структуры сообщений и метаданных
		UseStructuredMessageFormat: parseBoolOrDefault(getEnvOrDefault("USE_STRUCTURED_MESSAGE_FORMAT", "false"), false),

		// --- Настройки каузального обучения (Этап 1) ---
		CausalLearningEnabled:            causalLearningEnabled,
		AssociationCloudEnabled:          associationCloudEnabled,
		AssociationCloudMaxNodes:         associationCloudMaxNodes,
		AssociationCloudMaxEdges:         associationCloudMaxEdges,
		AssociationCloudDecayDays:        associationCloudDecayDays,
		CausalAnalysisIntervalHours:      causalAnalysisIntervalHours,
		CausalMinConfidence:              causalMinConfidence,
		CausalTemporalWindowMinutes:      causalTemporalWindowMinutes,
		CausalMaxEntriesPerChat:          causalMaxEntriesPerChat,
		CausalAnalysisLookbackMessages:   causalAnalysisLookbackMessages,
		CausalAnalysisPrompt:             causalAnalysisPrompt,
		CausalAnalysisPromptProvider:     causalAnalysisPromptProvider,
		CausalAnalysisPromptModel:        causalAnalysisPromptModel,
		CausalAnalysisPromptTemperature:  causalAnalysisPromptTemperature,
		CausalAnalysisPromptEnabled:      causalAnalysisPromptEnabled,
		CausalInfluencePrompt:            causalInfluencePrompt,
		CausalInfluencePromptProvider:    causalInfluencePromptProvider,
		CausalInfluencePromptModel:       causalInfluencePromptModel,
		CausalInfluencePromptTemperature: causalInfluencePromptTemperature,
		CausalInfluencePromptEnabled:     causalInfluencePromptEnabled,

		// --- Настройки эмоциональной системы (Этап 2) ---
		EmotionalLearningEnabled:          emotionalLearningEnabled,
		EmotionalAnalysisIntervalHours:    emotionalAnalysisIntervalHours,
		EmotionalAnalysisLookbackMessages: emotionalAnalysisLookbackMessages,
		EmotionalMemoryRetentionDays:      emotionalMemoryRetentionDays,
		EmotionalMinMessagesForAnalysis:   emotionalMinMessagesForAnalysis,
		EmotionalAnalysisDebounceHours:    emotionalAnalysisDebounceHours,

		EmotionalAnalysisPrompt:            emotionalAnalysisPrompt,
		EmotionalAnalysisPromptProvider:    emotionalAnalysisPromptProvider,
		EmotionalAnalysisPromptModel:       emotionalAnalysisPromptModel,
		EmotionalAnalysisPromptTemperature: emotionalAnalysisPromptTemperature,
		EmotionalAnalysisPromptEnabled:     emotionalAnalysisPromptEnabled,

		EmotionalAdaptationPrompt:            emotionalAdaptationPrompt,
		EmotionalAdaptationPromptProvider:    emotionalAdaptationPromptProvider,
		EmotionalAdaptationPromptModel:       emotionalAdaptationPromptModel,
		EmotionalAdaptationPromptTemperature: emotionalAdaptationPromptTemperature,
		EmotionalAdaptationPromptEnabled:     emotionalAdaptationPromptEnabled,

		EmotionalFeedbackPrompt:            emotionalFeedbackPrompt,
		EmotionalFeedbackPromptProvider:    emotionalFeedbackPromptProvider,
		EmotionalFeedbackPromptModel:       emotionalFeedbackPromptModel,
		EmotionalFeedbackPromptTemperature: emotionalFeedbackPromptTemperature,
		EmotionalFeedbackPromptEnabled:     emotionalFeedbackPromptEnabled,

		ResponseTypeConfigs: loadResponseTypeConfigs(LLMProvider(llmProviderStr), geminiModelName, deepSeekModelName, openRouterModelName),

		// Настройки отказоустойчивости LLM
		LLMFallbackEnabled:       llmFallbackEnabled,
		LLMFallbackCriticalTypes: llmFallbackCriticalTypes,
		LLMFallbackProviderOrder: llmFallbackProviderOrder,

		// Система убеждений
		BeliefLearningEnabled:           beliefLearningEnabled,
		BeliefAnalysisIntervalHours:     beliefAnalysisIntervalHours,
		BeliefAnalysisLookbackMessages:  beliefAnalysisLookbackMessages,
		BeliefAnalysisPrompt:            beliefAnalysisPrompt,
		BeliefAnalysisPromptProvider:    beliefAnalysisPromptProvider,
		BeliefAnalysisPromptModel:       beliefAnalysisPromptModel,
		BeliefAnalysisPromptTemperature: beliefAnalysisPromptTemperature,
		BeliefAnalysisPromptEnabled:     beliefAnalysisPromptEnabled,

		// === Когнитивная архитектура (Этап 3) ===
		InternalMonologueEnabled:        internalMonologueEnabled,
		SelfReflectionEnabled:           selfReflectionEnabled,
		ConfidenceCalibrationEnabled:    confidenceCalibrationEnabled,
		InternalMonologuePrompt:         internalMonologuePrompt,
		InternalMonologuePromptModel:    internalMonologuePromptModel,
		InternalMonologuePromptProvider: internalMonologuePromptProvider,
		InternalMonologuePromptEnabled:  internalMonologuePromptEnabled,
		InternalMonologueTemperature:    internalMonologueTemperature,

		// === Саморефлексия ===
		SelfReflectionPrompt:         selfReflectionPrompt,
		SelfReflectionPromptModel:    selfReflectionPromptModel,
		SelfReflectionPromptProvider: selfReflectionPromptProvider,
		SelfReflectionPromptEnabled:  selfReflectionPromptEnabled,
		SelfReflectionTemperature:    selfReflectionTemperature,

		// === Социальная архитектура (Этап 4) ===
		RelationshipTrackingEnabled:  relationshipTrackingEnabled,
		SocialLearningEnabled:        socialLearningEnabled,
		RelationshipAnalysisPrompt:   relationshipAnalysisPrompt,
		RelationshipAnalysisEnabled:  relationshipAnalysisEnabled,
		RelationshipAnalysisModel:    relationshipAnalysisModel,
		RelationshipAnalysisProvider: relationshipAnalysisProvider,
		RelationshipAnalysisTemp:     relationshipAnalysisTemp,
		IntimacyGrowthRate:           intimacyGrowthRate,
		TrustDecayRate:               trustDecayRate,
	}

	// Шаг 4: Загружаем промпты из файлов (с приоритетом над env)
	loadPromptsFromFiles(cfg)

	return cfg, nil
}

// loadPromptsFromFiles загружает промпты из файлов internal/bot/prompts/*.txt,
// loadPromptsFromFiles загружает промпты из txt-файлов в internal/bot/prompts/.
// Файлы имеют приоритет над env-значениями (перезаписывают их).
func loadPromptsFromFiles(cfg *Config) {
	// Карта: имя файла → указатель на строковое поле в Config
	promptFields := map[string]*string{
		"default":                                &cfg.DefaultPrompt,
		"daily_take":                             &cfg.DailyTakePrompt,
		"summary":                                &cfg.SummaryPrompt,
		"weekly_summary":                         &cfg.WeeklySummaryPrompt,
		"voice_message":                          &cfg.VoiceMessagesPrompt,
		"srach_warning":                          &cfg.SRACH_WARNING_PROMPT,
		"srach_analysis":                         &cfg.SRACH_ANALYSIS_PROMPT,
		"srach_confirm":                          &cfg.SRACH_CONFIRM_PROMPT,
		"rate_limit":                             &cfg.RateLimitPrompt,
		"welcome":                                &cfg.WelcomePrompt,
		"startup_greeting":                       &cfg.StartupGreetingPrompt,
		"voice_format":                           &cfg.VoiceFormatPrompt,
		"classify_direct_message":                &cfg.ClassifyDirectMessagePrompt,
		"serious_direct":                         &cfg.SeriousDirectPrompt,
		"direct_reply_limit":                     &cfg.DirectReplyLimitPrompt,
		"photo_analysis":                         &cfg.PhotoAnalysisPrompt,
		"auto_bio_initial_analysis":              &cfg.AutoBioInitialAnalysisPrompt,
		"auto_bio_update":                        &cfg.AutoBioUpdatePrompt,
		"free_will_should_reply":                 &cfg.FreeWillShouldReplyPrompt,
		"free_will_response_type":                &cfg.FreeWillResponseTypePrompt,
		"free_will_reaction":                     &cfg.FreeWillReactionPrompt,
		"free_will_direct":                       &cfg.FreeWillDirectPrompt,
		"free_will_general":                      &cfg.FreeWillGeneralPrompt,
		"free_will_context":                      &cfg.FreeWillContextPrompt,
		"free_will_silence":                      &cfg.FreeWillSilencePrompt,
		"free_will_mood_analysis":                &cfg.FreeWillMoodAnalysisPrompt,
		"free_will_take_response":                &cfg.FreeWillTakeResponsePrompt,
		"free_will_direct_response_decision":     &cfg.FreeWillDirectResponseDecisionPrompt,
		"free_will_direct_response":              &cfg.FreeWillDirectResponsePrompt,
		"donate":                                 &cfg.DonatePrompt,
		"personality_analysis":                   &cfg.PersonalityAnalysisPrompt,
		"personality_name_analysis":              &cfg.PersonalityNameAnalysisPrompt,
		"personality_topic_analysis":             &cfg.PersonalityTopicAnalysisPrompt,
		"personality_self_update":                &cfg.PersonalitySelfUpdatePrompt,
		"clown_reaction":                         &cfg.ClownReactionPrompt,
		"reaction_analysis":                      &cfg.ReactionAnalysisPrompt,
		"web_search_trigger":                     &cfg.WebSearchTriggerPrompt,
		"anti_repetition_rework":                 &cfg.AntiRepetitionReworkPrompt,
		"causal_analysis":                        &cfg.CausalAnalysisPrompt,
		"causal_influence":                       &cfg.CausalInfluencePrompt,
		"emotional_analysis":                     &cfg.EmotionalAnalysisPrompt,
		"emotional_adaptation":                   &cfg.EmotionalAdaptationPrompt,
		"emotional_feedback":                     &cfg.EmotionalFeedbackPrompt,
		"internal_monologue":                     &cfg.InternalMonologuePrompt,
		"self_reflection":                        &cfg.SelfReflectionPrompt,
		"relationship_analysis":                  &cfg.RelationshipAnalysisPrompt,
		"belief_analysis":                        &cfg.BeliefAnalysisPrompt,
		"image_gen_pre_prompt":                   &cfg.ImageGenPrePrompt,
		"free_will_imagegen":                     &cfg.FreeWillImageGenPrompt,
	}

	for fileName, field := range promptFields {
		if field == nil {
			continue // пропускаем поля без соответствующей структуры
		}
		content, err := prompts.LoadPrompt(fileName)
		if err != nil {
			log.Printf("[WARN][Config] Ошибка загрузки промпта из файла %s.txt: %v", fileName, err)
			continue
		}
		if content != "" {
			*field = content
			log.Printf("[INFO][Config] Загружен промпт из файла: %s.txt", fileName)
		}
	}
}

// loadResponseTypeConfigs загружает конфигурации для типов ответов из переменных окружения
func loadResponseTypeConfigs(defaultProvider LLMProvider, geminiModel, deepSeekModel, openRouterModel string) map[string]ResponseTypeConfig {
	configs := make(map[string]ResponseTypeConfig)

	// Функция для получения подходящей модели по умолчанию
	getModelForProvider := func(provider LLMProvider) string {
		switch provider {
		case ProviderGemini:
			return geminiModel
		case ProviderDeepSeek:
			return deepSeekModel
		case ProviderOpenRouter:
			return openRouterModel
		default:
			return geminiModel
		}
	}

	// Функция загрузки конфигурации для типа ответа
	loadConfig := func(responseType string, defaultProviderName string, defaultModelName string, defaultTemp float64, defaultEnabled bool) {
		providerName := strings.ToLower(getEnvOrDefault(strings.ToUpper(responseType)+"_PROMPT_PROVIDER", defaultProviderName))
		modelName := getEnvOrDefault(strings.ToUpper(responseType)+"_PROMPT_MODEL", defaultModelName)
		temperatureStr := getEnvOrDefault(strings.ToUpper(responseType)+"_PROMPT_TEMPERATURE", "")
		enabledStr := getEnvOrDefault(strings.ToUpper(responseType)+"_PROMPT_ENABLED", "")

		// Парсим температуру
		var temperature float64 = defaultTemp
		if temperatureStr != "" {
			temperature = parseFloatOrDefault(temperatureStr, defaultTemp)
		}

		// Парсим enabled
		var enabled bool = defaultEnabled
		if enabledStr != "" {
			enabled = parseBoolOrDefault(enabledStr, defaultEnabled)
		}

		// Конвертируем строку провайдера в LLMProvider
		var provider LLMProvider
		switch providerName {
		case "gemini":
			provider = ProviderGemini
		case "deepseek":
			provider = ProviderDeepSeek
		case "openrouter":
			provider = ProviderOpenRouter
		default:
			provider = LLMProvider(providerName)
		}

		configs[responseType] = ResponseTypeConfig{
			Provider:    provider,
			ModelName:   modelName,
			Temperature: float32(temperature),
			Enabled:     enabled,
		}
	}

	// Загружаем все основные типы ответов
	loadConfig("default", "gemini", geminiModel, 1.0, true)
	loadConfig("direct", "gemini", geminiModel, 1.0, true)
	loadConfig("serious", "gemini", geminiModel, 0.8, true)
	loadConfig("daily_take", "deepseek", deepSeekModel, 1.2, true)
	loadConfig("summary", "gemini", geminiModel, 0.7, true)
	loadConfig("voice_message", "gemini", geminiModel, 1.0, true)
	loadConfig("srach_warning", "gemini", geminiModel, 0.5, true)
	loadConfig("srach_analysis", "gemini", geminiModel, 0.5, true)
	loadConfig("srach_confirm", "gemini", geminiModel, 0.3, true)
	loadConfig("rate_limit", "gemini", geminiModel, 1.0, true)
	loadConfig("welcome", "gemini", geminiModel, 1.0, true)
	loadConfig("voice_format", "gemini", geminiModel, 0.3, true)
	loadConfig("classify_direct_message", "gemini", geminiModel, 0.3, true)
	loadConfig("direct_reply_limit", "gemini", geminiModel, 1.0, true)
	loadConfig("photo_analysis", "gemini", geminiModel, 0.7, true)

	// Остальные типы с дефолтными значениями
	configs["classify"] = ResponseTypeConfig{Provider: ProviderGemini, ModelName: geminiModel, Temperature: 0.3, Enabled: true}
	configs["moderation"] = ResponseTypeConfig{Provider: ProviderGemini, ModelName: geminiModel, Temperature: 0.3, Enabled: true}
	// Для donate используем универсальную загрузку, чтобы читать DONATE_PROMPT_* из окружения (в т.ч. DONATE_PROMPT_ENABLED)
	loadConfig("donate", string(defaultProvider), getModelForProvider(defaultProvider), 1.0, false)
	configs["auto_bio"] = ResponseTypeConfig{Provider: ProviderGemini, ModelName: geminiModel, Temperature: 0.6, Enabled: true}
	configs["auto_bio_update"] = ResponseTypeConfig{Provider: ProviderGemini, ModelName: geminiModel, Temperature: 0.6, Enabled: true}
	configs["personality_analysis"] = ResponseTypeConfig{Provider: ProviderGemini, ModelName: geminiModel, Temperature: 0.5, Enabled: true}
	configs["personality_name"] = ResponseTypeConfig{Provider: ProviderGemini, ModelName: geminiModel, Temperature: 0.3, Enabled: true}
	configs["personality_topic"] = ResponseTypeConfig{Provider: ProviderGemini, ModelName: geminiModel, Temperature: 0.4, Enabled: true}
	configs["personality_self"] = ResponseTypeConfig{Provider: ProviderGemini, ModelName: geminiModel, Temperature: 0.6, Enabled: true}
	configs["photo"] = ResponseTypeConfig{Provider: ProviderGemini, ModelName: geminiModel, Temperature: 0.7, Enabled: true}
	configs["web_search"] = ResponseTypeConfig{Provider: ProviderGemini, ModelName: geminiModel, Temperature: 0.4, Enabled: true}
	configs["clown_reaction"] = ResponseTypeConfig{Provider: defaultProvider, ModelName: getModelForProvider(defaultProvider), Temperature: 1.0, Enabled: false}
	configs["reaction_analysis"] = ResponseTypeConfig{Provider: ProviderGemini, ModelName: geminiModel, Temperature: 0.5, Enabled: true}
	configs["anti_repetition"] = ResponseTypeConfig{Provider: ProviderDeepSeek, ModelName: deepSeekModel, Temperature: 1.1, Enabled: true}
	configs["free_will_should_reply"] = ResponseTypeConfig{Provider: ProviderGemini, ModelName: geminiModel, Temperature: 0.6, Enabled: true}
	configs["free_will_response_type"] = ResponseTypeConfig{Provider: ProviderGemini, ModelName: geminiModel, Temperature: 0.8, Enabled: true}
	configs["free_will_direct"] = ResponseTypeConfig{Provider: defaultProvider, ModelName: getModelForProvider(defaultProvider), Temperature: 1.0, Enabled: false}
	configs["free_will_general"] = ResponseTypeConfig{Provider: defaultProvider, ModelName: getModelForProvider(defaultProvider), Temperature: 1.0, Enabled: false}
	configs["free_will_context"] = ResponseTypeConfig{Provider: defaultProvider, ModelName: getModelForProvider(defaultProvider), Temperature: 1.0, Enabled: false}
	configs["free_will_silence"] = ResponseTypeConfig{Provider: ProviderDeepSeek, ModelName: deepSeekModel, Temperature: 1.2, Enabled: true}
	configs["free_will_mood"] = ResponseTypeConfig{Provider: ProviderGemini, ModelName: geminiModel, Temperature: 0.5, Enabled: true}
	configs["free_will_take"] = ResponseTypeConfig{Provider: ProviderDeepSeek, ModelName: deepSeekModel, Temperature: 1.0, Enabled: true}
	configs["free_will_reaction"] = ResponseTypeConfig{Provider: ProviderGemini, ModelName: geminiModel, Temperature: 0.4, Enabled: true}
	// Система убеждений
	configs["belief_analysis"] = ResponseTypeConfig{Provider: ProviderGemini, ModelName: geminiModel, Temperature: 0.5, Enabled: true}

	return configs
}

// getEnvOrDefault читает переменную окружения или возвращает значение по умолчанию
func getEnvOrDefault(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// maskSecret заменяет большую часть строки звездочками для безопасного логирования
func maskSecret(s string) string {
	if len(s) < 4 {
		return "****"
	}
	visiblePart := 2 // Сколько символов оставить видимыми с каждого конца
	if len(s) < visiblePart*2 {
		visiblePart = 1
	}
	if len(s) < visiblePart*2 {
		return "****"
	}
	return s[:visiblePart] + "****" + s[len(s)-visiblePart:]
}

// parseBoolOrDefault возвращает значение bool по умолчанию, если переменная не установлена
func parseBoolOrDefault(value string, defaultValue bool) bool {
	if parsed, err := strconv.ParseBool(value); err == nil {
		return parsed
	}
	return defaultValue
}

// parseIntOrDefault возвращает значение int по умолчанию, если переменная не установлена
func parseIntOrDefault(value string, defaultValue int) int {
	if parsed, err := strconv.Atoi(value); err == nil {
		return parsed
	}
	return defaultValue
}

// parseFloatOrDefault возвращает значение float64 по умолчанию, если переменная не установлена
func parseFloatOrDefault(value string, defaultValue float64) float64 {
	if parsed, err := strconv.ParseFloat(value, 64); err == nil {
		return parsed
	}
	return defaultValue
}
