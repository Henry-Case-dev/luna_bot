package config

import (
	"log"
	"net/url"
	"sort"
	"strings"

	"github.com/Henry-Case-dev/luna_bot/internal/utils"
)

// logLoadedConfig выводит загруженную конфигурацию в лог (маскируя секреты)
func logLoadedConfig(cfg *Config) {
	log.Println("--- Загруженная конфигурация: ---")
	log.Printf("  TelegramToken: %s", maskSecret(cfg.TelegramToken))
	log.Printf("  LLMProvider (fallback): %s", cfg.LLMProvider)
	log.Printf("  DefaultPrompt: %s...", utils.TruncateString(cfg.DefaultPrompt, 100))
	log.Printf("  ClassifyDirectMessagePrompt: %s...", utils.TruncateString(cfg.ClassifyDirectMessagePrompt, 100))
	log.Printf("  SeriousDirectPrompt: %s...", utils.TruncateString(cfg.SeriousDirectPrompt, 100))
	log.Printf("  DailyTakePrompt: %s...", utils.TruncateString(cfg.DailyTakePrompt, 100))
	log.Printf("  SummaryPrompt: %s...", utils.TruncateString(cfg.SummaryPrompt, 100))
	log.Printf("  RateLimitStaticText: %s", cfg.RateLimitStaticText)
	log.Printf("  RateLimitPrompt: %s...", utils.TruncateString(cfg.RateLimitPrompt, 100))
	log.Printf("  SRACH_WARNING_PROMPT: %s...", utils.TruncateString(cfg.SRACH_WARNING_PROMPT, 100))
	log.Printf("  SRACH_ANALYSIS_PROMPT: %s...", utils.TruncateString(cfg.SRACH_ANALYSIS_PROMPT, 100))
	log.Printf("  SRACH_CONFIRM_PROMPT: %s...", utils.TruncateString(cfg.SRACH_CONFIRM_PROMPT, 100))
	log.Printf("  WelcomePrompt: %s...", utils.TruncateString(cfg.WelcomePrompt, 100))
	log.Printf("  VoiceFormatPrompt: %s...", utils.TruncateString(cfg.VoiceFormatPrompt, 100))
	log.Printf("  DirectReplyLimitPrompt: %s...", utils.TruncateString(cfg.DirectReplyLimitPrompt, 100))

	log.Println("  === LLM Провайдеры (доступные): ===")
	if cfg.GeminiAPIKey != "" {
		log.Printf("    ✓ Gemini: %s (эмбеддинги, аудио, фото, изображения)", cfg.GeminiModelName)
	} else {
		log.Println("    ✗ Gemini: не конфигурирован (REQUIRED for embeddings)")
	}
	if cfg.DeepSeekAPIKey != "" {
		log.Printf("    ✓ DeepSeek: %s", cfg.DeepSeekModelName)
	}
	if cfg.OpenRouterAPIKey != "" {
		log.Printf("    ✓ OpenRouter: %s", cfg.OpenRouterModelName)
	}

	// === Логирование маршрутизации ResponseTypeConfigs ===
	LogResponseTypeConfigs(cfg.ResponseTypeConfigs)

	log.Printf("  MinMessages: %d", cfg.MinMessages)
	log.Printf("  MaxMessages: %d", cfg.MaxMessages)
	log.Printf("  ContextWindow: %d", cfg.ContextWindow)
	log.Printf("  DailyTakeTime: %d", cfg.DailyTakeTime)
	log.Printf("  TimeZone: %s", cfg.TimeZone)
	log.Printf("  SummaryIntervalHours: %d", cfg.SummaryIntervalHours)
	log.Printf("  SrachAnalysisEnabled Default: %t", cfg.SrachAnalysisEnabled)
	log.Printf("  VoiceTranscriptionEnabled Default: %t", cfg.VoiceTranscriptionEnabledDefault)
	log.Printf("  DirectReplyLimitEnabled Default: %t", cfg.DirectReplyLimitEnabledDefault)
	log.Printf("  DirectReplyLimitCount Default: %d", cfg.DirectReplyLimitCountDefault)
	log.Printf("  DirectReplyLimitDuration Default: %v", cfg.DirectReplyLimitDurationDefault)
	log.Printf("  StorageType: %s", cfg.StorageType)

	if cfg.StorageType == StorageTypePostgres {
		log.Printf("  PostgresqlHost: %s", cfg.PostgresqlHost)
		log.Printf("  PostgresqlPort: %s", cfg.PostgresqlPort)
		log.Printf("  PostgresqlUser: %s", cfg.PostgresqlUser)
		log.Printf("  PostgresqlPassword: %s", maskSecret(cfg.PostgresqlPassword))
		log.Printf("  PostgresqlDbname: %s", cfg.PostgresqlDbname)
	}
	if cfg.StorageType == StorageTypeMongo {
		log.Printf("  MongoDbURI: %s", maskSecretURI(cfg.MongoDbURI))
		log.Printf("  MongoDbName: %s", cfg.MongoDbName)
		log.Printf("  MongoDbMessagesCollection: %s", cfg.MongoDbMessagesCollection)
		log.Printf("  MongoDbUserProfilesCollection: %s", cfg.MongoDbUserProfilesCollection)
		log.Printf("  MongoDbSettingsCollection: %s", cfg.MongoDbSettingsCollection)
	}

	log.Printf("  AdminUsernames: [%s]", strings.Join(cfg.AdminUsernames, ", "))
	log.Printf("  Debug: %t", cfg.Debug)

	log.Printf("  LongTermMemoryEnabled: %t", cfg.LongTermMemoryEnabled)
	if cfg.LongTermMemoryEnabled {
		log.Printf("    GeminiEmbeddingModelName: %s", cfg.GeminiEmbeddingModelName)
		log.Printf("    MongoVectorIndexName: %s", cfg.MongoVectorIndexName)
		log.Printf("    LongTermMemoryFetchK: %d", cfg.LongTermMemoryFetchK)
	}
	log.Printf("  BackfillBatchSize: %d", cfg.BackfillBatchSize)
	log.Printf("  BackfillBatchDelay: %v", cfg.BackfillBatchDelay)
	log.Printf("  PhotoAnalysisEnabled: %t", cfg.PhotoAnalysisEnabled)
	log.Printf("  PhotoAnalysisPrompt: %s...", utils.TruncateString(cfg.PhotoAnalysisPrompt, 100))

	// Настройки обхода блокировок Gemini
	log.Printf("  GeminiBypassSafetyFilters: %t", cfg.GeminiBypassSafetyFilters)
	log.Printf("  GeminiObfuscatePrompts: %t", cfg.GeminiObfuscatePrompts)

	// === НАСТРОЙКИ СТРУКТУРИРОВАННОГО ФОРМАТИРОВАНИЯ ===
	log.Printf("  UseStructuredMessageFormat: %t", cfg.UseStructuredMessageFormat)
	if cfg.UseStructuredMessageFormat {
		log.Printf("    → Используется новый формат с тегами [MSG_START]/[MSG_END]")
		log.Printf("    → Каждое сообщение структурируется с метаданными:")
		log.Printf("    → [MSG_START]")
		log.Printf("    → Время: 15:04")
		log.Printf("    → Дата: 02.01 (Mon)")
		log.Printf("    → Автор: ИмяПользователя")
		log.Printf("    → Bio: описание пользователя")
		log.Printf("    → Тип: пользователь, текстовое")
		log.Printf("    → Текст: сообщение")
		log.Printf("    → [MSG_END]")
		log.Printf("    → ✅ Это улучшает понимание LLM структуры сообщений")
	} else {
		log.Printf("    → Используется стандартный формат сообщений")
		log.Printf("    → Формат: 15:04(Mon,02.01) Пользователь (Bio:описание) текст")
		log.Printf("    → ⚠️  Метаданные могут путаться LLM в сложных случаях")
	}

	// --- Логгирование настроек автоочистки MongoDB ---
	log.Printf("  MongoCleanupEnabled: %t", cfg.MongoCleanupEnabled)
	if cfg.MongoCleanupEnabled {
		log.Printf("    MongoCleanupSizeLimitMB: %d", cfg.MongoCleanupSizeLimitMB)
		log.Printf("    MongoCleanupIntervalMinutes: %d", cfg.MongoCleanupIntervalMinutes)
		log.Printf("    MongoCleanupChunkDurationHours: %d", cfg.MongoCleanupChunkDurationHours)
	}
	// --- Конец логгирования ---

	// --- Логгирование настроек модерации ---
	log.Printf("  ModInterval: %d", cfg.ModInterval)
	log.Printf("  ModMuteTimeMin: %d", cfg.ModMuteTimeMin)
	log.Printf("  ModBanTimeMin: %d", cfg.ModBanTimeMin)
	log.Printf("  ModPurgeDuration (Window): %v", cfg.ModPurgeDuration)
	log.Printf("  ModPurgeDelay: %v", cfg.ModPurgeDelay)
	log.Printf("  ModCheckAdminRights: %t", cfg.ModCheckAdminRights)
	log.Printf("  ModDefaultNotify: %t", cfg.ModDefaultNotify)
	log.Printf("  ModRules Count: %d", len(cfg.ModRules))
	// Опционально: можно добавить более детальное логгирование правил,
	// но нужно быть осторожным с потенциально длинными llm_instruction.
	/*
		for i, rule := range cfg.ModRules {
			log.Printf("    Rule #%d (%s): ChatID=%s(%d), UserID=%s(%d), Keywords=%d, Punishment=%s, LLM=%t, NotifyUser=%t, NotifyChat=%t",
				i+1, rule.RuleName, rule.ChatID, rule.ParsedChatID, rule.UserID, rule.ParsedUserID, len(rule.Keywords),
				rule.Punishment, rule.LLMInstruction != "none", rule.NotifyUser, rule.NotifyChat)
		}
	*/
	// --- Конец логгирования модерации ---

	log.Println("--- Конфигурация завершена ---")
}

// LogResponseTypeConfigs логирует таблицу маршрутизации типов ответов
func LogResponseTypeConfigs(configs map[string]ResponseTypeConfig) {
	if len(configs) == 0 {
		log.Println("⚠ ResponseTypeConfigs пусты!")
		return
	}

	log.Println("\n=== Таблица маршрутизации ResponseTypes ===")

	// Сортируем ключи для читаемого вывода
	var keys []string
	for k := range configs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Группируем по провайдерам для наглядности
	byProvider := make(map[string][]string)
	for _, key := range keys {
		cfg := configs[key]
		providerStr := string(cfg.Provider)
		byProvider[providerStr] = append(byProvider[providerStr], key)
	}

	// Выводим сгруппированные результаты
	for _, provider := range []string{"gemini", "openrouter", "deepseek"} {
		if types, ok := byProvider[provider]; ok && len(types) > 0 {
			log.Printf("  [%s] — %d типов ответов:", provider, len(types))
			for _, responseType := range types {
				cfg := configs[responseType]
				temp := cfg.Temperature
				if temp == 0 {
					temp = 1.0 // Дефолт
				}
				enabled := "✓"
				if !cfg.Enabled {
					enabled = "✗"
				}
				log.Printf("    %s %s → %s (модель: %s, temp: %.2f)", enabled, responseType, cfg.Provider, cfg.ModelName, temp)
			}
		}
	}
	log.Println("=== Конец таблицы маршрутизации ===")
}

// maskSecretURI маскирует пароль в URI для безопасного логирования
func maskSecretURI(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return maskSecret(uri) // Если парсинг не удался, маскируем как обычную строку
	}
	if u.User != nil {
		username := u.User.Username()
		// Пароль маскируем полностью
		u.User = url.UserPassword(username, "********")
		return u.String()
	}
	return uri // Возвращаем как есть, если нет UserInfo
}
