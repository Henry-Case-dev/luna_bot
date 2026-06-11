package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// =============================================================================
// Маппинг: .env переменная → YAML-путь
// Полный набор — 95 маппингов, покрывающих все переменные из .env.example.
// =============================================================================

var envToYAMLKey = map[string]string{
	"LLM_PROVIDER":             "llm.default_provider",
	"LLM_FALLBACK_ENABLED":     "llm.fallback_enabled",
	"LLM_FALLBACK_CRITICAL_TYPES": "llm.fallback_critical_types",
	"LLM_FALLBACK_PROVIDER_ORDER": "llm.fallback_provider_order",

	"GEMINI_API_KEY":               "llm.providers.gemini.api_key",
	"GEMINI_API_KEY_RESERVE":       "llm.providers.gemini.reserve_api_key",
	"GEMINI_KEY_ROTATION_TIME_HOURS": "llm.providers.gemini.key_rotation_hours",
	"GEMINI_MODEL_NAME":            "llm.providers.gemini.models.text",
	"GEMINI_TEMPERATURE_NORMAL":    "llm.providers.gemini.temperatures.normal",
	"GEMINI_TEMPERATURE_SERIOUS":   "llm.providers.gemini.temperatures.serious",
	"AUDIO_TRANSCRIPTION_MODEL":    "llm.providers.gemini.models.audio",
	"IMAGE_GENERATION_MODEL":       "llm.providers.gemini.models.image_gen",
	"GEMINI_EMBEDDING_MODEL_NAME":  "llm.providers.gemini.models.embed",
	"GEMINI_BYPASS_SAFETY_FILTERS": "llm.providers.gemini.safety.bypass_filters",
	"GEMINI_OBFUSCATE_PROMPTS":     "llm.providers.gemini.safety.obfuscate",

	"DEEPSEEK_API_KEY":    "llm.providers.deepseek.api_key",
	"DEEPSEEK_MODEL_NAME": "llm.providers.deepseek.models.text",

	"OPENROUTER_API_KEY":    "llm.providers.openrouter.api_key",
	"OPENROUTER_MODEL_NAME": "llm.providers.openrouter.models.text",

	"PHOTO_ANALYSIS_ENABLED": "llm.image_analysis.enabled",

	"TELEGRAM_TOKEN":                  "telegram.token",
	"DEBUG":                           "telegram.debug",
	"TIME_ZONE":                       "telegram.timezone",
	"BOT_USER_ID":                     "telegram.bot_user_id",
	"USE_STRUCTURED_MESSAGE_FORMAT":   "telegram.use_structured_message_format",
	"ERROR_MESSAGE_AUTO_DELETE_SECONDS": "telegram.error_message_auto_delete_seconds",
	"ADMIN_USERNAMES":                 "telegram.admin_usernames",
	"BOT_NAMES":                       "telegram.bot_names",

	"ELEVENLABS_API_KEY":          "tts.elevenlabs.api_key",
	"ELEVENLABS_VOICE_ID":         "tts.elevenlabs.voice_id",
	"ELEVENLABS_MODEL":            "tts.elevenlabs.model",
	"ELEVENLABS_PLAN":             "tts.elevenlabs.plan",
	"ELEVENLABS_STABILITY":        "tts.elevenlabs.voice_settings.stability",
	"ELEVENLABS_SIMILARITY_BOOST": "tts.elevenlabs.voice_settings.similarity_boost",
	"ELEVENLABS_STYLE":            "tts.elevenlabs.voice_settings.style",
	"ELEVENLABS_USE_SPEAKER_BOOST": "tts.elevenlabs.voice_settings.use_speaker_boost",
	"ELEVENLABS_SPEED":            "tts.elevenlabs.voice_settings.speed",
	"ELEVENLABS_RANDOM_VOICE":     "tts.elevenlabs.random_voice",

	"STORAGE_TYPE":                    "storage.type",
	"POSTGRESQL_HOST":                 "storage.postgresql.host",
	"POSTGRESQL_PORT":                 "storage.postgresql.port",
	"POSTGRESQL_USER":                 "storage.postgresql.user",
	"POSTGRESQL_PASSWORD":             "storage.postgresql.password",
	"POSTGRESQL_DBNAME":               "storage.postgresql.dbname",
	"MONGODB_URI":                     "storage.mongodb.uri",
	"MONGODB_DBNAME":                  "storage.mongodb.dbname",
	"MONGODB_MESSAGES_COLLECTION":     "storage.mongodb.messages_collection",
	"MONGODB_USER_PROFILES_COLLECTION": "storage.mongodb.user_profiles_collection",
	"MONGO_VECTOR_INDEX_NAME":         "storage.mongodb.vector_index_name",
	"MONGO_CLEANUP_ENABLED":           "storage.cleanup.enabled",
	"MONGO_CLEANUP_SIZE_LIMIT_MB":     "storage.cleanup.size_limit_mb",
	"MONGO_CLEANUP_INTERVAL_MINUTES":   "storage.cleanup.interval_minutes",
	"MONGO_CLEANUP_CHUNK_DURATION_HOURS": "storage.cleanup.chunk_duration_hours",
	"LONG_TERM_MEMORY_ENABLED":         "storage.long_term_memory.enabled",
	"LONG_TERM_MEMORY_FETCH_K":         "storage.long_term_memory.fetch_k",
	"BACKFILL_BATCH_SIZE":             "storage.long_term_memory.backfill.batch_size",
	"BACKFILL_BATCH_DELAY_SECONDS":    "storage.long_term_memory.backfill.batch_delay",

	"EMBEDDING_REQUESTS_PER_MINUTE":  "embedding.requests_per_minute",
	"EMBEDDING_REQUESTS_PER_DAY":     "embedding.requests_per_day",
	"EMBEDDING_BATCH_SIZE":           "embedding.batch_size",
	"EMBEDDING_REQUEST_DELAY":        "embedding.request_delay",
	"EMBEDDING_BATCH_DELAY":          "embedding.batch_delay",
	"EMBEDDING_ADAPTIVE_THROTTLING":  "embedding.adaptive_throttling",
	"EMBEDDING_SAFETY_MARGIN":        "embedding.safety_margin",
	"EMBEDDING_MAX_RETRIES":          "embedding.max_retries",
	"EMBEDDING_CACHE_ENABLED":        "embedding.cache.enabled",
	"EMBEDDING_CACHE_DIR":            "embedding.cache.dir",
	"MIGRATION_RESUME_ENABLED":       "embedding.migration.resume_enabled",
	"MIGRATION_STATE_FILE":           "embedding.migration.state_file",

	"MIN_MESSAGES":                    "chat.min_messages",
	"MAX_MESSAGES":                    "chat.max_messages",
	"CONTEXT_WINDOW":                  "chat.context_window",
	"IMAGE_GENERATION_CONTEXT_WINDOW": "chat.image_generation_context_window",

	"SUMMARY_INTERVAL_HOURS":    "summary.interval_hours",
	"SUMMARY_MAX_PARTS":         "summary.max_parts",
	"WEEKLY_SUMMARY_ENABLED":    "summary.weekly.enabled",
	"WEEKLY_SUMMARY_DAY":        "summary.weekly.day",
	"WEEKLY_SUMMARY_HOUR":       "summary.weekly.hour",
	"WEEKLY_SUMMARY_MINUTE":     "summary.weekly.minute",
	"WEEKLY_SUMMARY_MAX_PARTS":  "summary.weekly.max_parts",
	"WEEKLY_SUMMARY_SEARCH_METHOD": "summary.weekly.search_method",
	"SUMMARY_FLAGS_ENABLED":     "summary.weekly.flags_enabled",
	"SUMMARY_KEYWORDS_ENABLED":  "summary.weekly.keywords_enabled",

	"RATE_LIMIT_STATIC_TEXT": "rate_limit.static_text",

	"PROMPT_ENTER_MIN_MESSAGES":      "settings_prompts.enter_min_messages",
	"PROMPT_ENTER_MAX_MESSAGES":      "settings_prompts.enter_max_messages",
	"PROMPT_ENTER_DAILY_TIME":        "settings_prompts.enter_daily_time",
	"PROMPT_ENTER_SUMMARY_INTERVAL":  "settings_prompts.enter_summary_interval",
	"PROMPT_ENTER_DIRECT_LIMIT_COUNT": "settings_prompts.enter_direct_limit_count",
	"PROMPT_ENTER_DIRECT_LIMIT_DURATION": "settings_prompts.enter_direct_limit_duration",

	"SRACH_KEYWORDS":       "srach.keywords",
	"SRACH_ANALYSIS_ENABLED": "srach.analysis_enabled",

	"DIRECT_REPLY_LIMIT_ENABLED_DEFAULT":       "direct_reply_limits.enabled_default",
	"DIRECT_REPLY_LIMIT_COUNT_DEFAULT":         "direct_reply_limits.count_default",
	"DIRECT_REPLY_LIMIT_DURATION_MINUTES_DEFAULT": "direct_reply_limits.duration_minutes_default",

	"DONATE_TIME_HOURS":               "donate.time_hours",
	"PAYMENT_REMINDER_INTERVAL_HOURS": "donate.payment_reminder_interval_hours",

	"FREE_WILL_ENABLED":                "free_will.enabled",
	"FREE_WILL_MIN_INTERVAL_MINUTES":   "free_will.intervals.min_minutes",
	"FREE_WILL_MAX_INTERVAL_MINUTES":   "free_will.intervals.max_minutes",
	"FREE_WILL_CONTEXT_WINDOW":         "free_will.context_window",
	"FREE_WILL_MOOD_UPDATE_PROBABILITY": "free_will.mood_update_probability",
	"FREE_WILL_MAX_DECISIONS_PER_HOUR": "free_will.max_decisions_per_hour",
	"FREE_WILL_VOICE_PROBABILITY":      "free_will.voice_probability",
	"FREE_WILL_SILENCE_MIN_MINUTES":    "free_will.silence.min_minutes",
	"FREE_WILL_SILENCE_MAX_MINUTES":    "free_will.silence.max_minutes",
	"INTERVAL_MESSAGES_ENABLED":        "free_will.interval_messages.enabled",
	"FREE_WILL_REACTIONS_ENABLED":             "free_will.reactions.enabled",
	"FREE_WILL_REACTIONS_PROBABILITY":         "free_will.reactions.probability",
	"FREE_WILL_REACTIONS_COOLDOWN_MINUTES":    "free_will.reactions.cooldown_minutes",
	"FREE_WILL_REACTIONS_MAX_PER_HOUR":        "free_will.reactions.max_per_hour",
	"FREE_WILL_IMAGE_GENERATION_INTERVAL_HOURS":           "free_will.image_generation.interval_hours",
	"FREE_WILL_IMAGE_GENERATION_MAX_DECISIONS_PER_INTERVAL": "free_will.image_generation.max_per_interval",
	"FREE_WILL_IMAGE_GENERATION_MIN_DECISION_INTERVAL_MINUTES": "free_will.image_generation.min_decision_interval_minutes",
	"FREE_WILL_IMAGE_GENERATION_INDEPENDENT_LIMITS":        "free_will.image_generation.independent_limits",

	"MOD_ENABLED":           "moderation.enabled",
	"MOD_INTERVAL":          "moderation.interval_minutes",
	"MOD_MUTE_TIME_MIN":     "moderation.mute_time_minutes",
	"MOD_KICK_TIME_MIN":     "moderation.kick_time_minutes",
	"MOD_BAN_TIME_MIN":      "moderation.ban_time_minutes",
	"MOD_PURGE_DELAY_DURATION":  "moderation.purge_delay_duration",
	"MOD_PURGE_WINDOW_DURATION": "moderation.purge_window_duration",
	"MOD_CHECK_ADMIN_RIGHTS":    "moderation.check_admin_rights",
	"MOD_DEFAULT_NOTIFY":        "moderation.default_notify",

	"PERSONALITY_UPDATE_INTERVAL_HOURS": "personality.update_interval_hours",
	"PERSONALITY_MESSAGES_LOOKBACK":     "personality.messages_lookback",
	"MAX_NAME_MENTIONS":                 "personality.max_name_mentions",
	"MAX_RECENT_TOPICS":                 "personality.max_recent_topics",
	"MAX_SELF_PERCEPTIONS":             "personality.max_self_perceptions",
	"MAX_DISCUSSION_CONTEXTS":           "personality.max_discussion_contexts",

	"REACTIONS_ENABLED":           "reactions.enabled",
	"CLOWN_RESPONSE_PROBABILITY":  "reactions.clown.response_probability",
	"CLOWN_COOLDOWN_SECONDS":      "reactions.clown.cooldown_seconds",
	"MAX_CLOWN_RESPONSES_PER_HOUR": "reactions.clown.max_responses_per_hour",

	"WEB_SEARCH_ENABLED":      "web_search.enabled",
	"GOOGLE_SEARCH_API_KEY":   "web_search.google_api_key",
	"GOOGLE_SEARCH_ENGINE_ID": "web_search.search_engine_id",
	"WEB_SEARCH_MAX_RESULTS":  "web_search.max_results",
	"WEB_SEARCH_CACHE_TTL":    "web_search.cache.ttl",
	"WEB_SEARCH_CACHE_MAX_SIZE": "web_search.cache.max_size",

	"DISAMBIGUATION_ENABLED": "disambiguation.enabled",

	"VOICE_MESSAGES_ENABLED": "voice_messages.enabled",
	"MIN_VOICE_MESSAGES":     "voice_messages.interval.min",
	"MAX_VOICE_MESSAGES":     "voice_messages.interval.max",

	"ANTI_REPETITION_ENABLED":                 "anti_repetition.enabled",
	"ANTI_REPETITION_MAX_RESPONSES_PER_CHAT":  "anti_repetition.max_responses_per_chat",
	"ANTI_REPETITION_SIMILARITY_THRESHOLD":    "anti_repetition.similarity_threshold",
	"ANTI_REPETITION_TIME_WINDOW_HOURS":       "anti_repetition.time_window_hours",
	"ANTI_REPETITION_CLEANUP_INTERVAL_HOURS":  "anti_repetition.cleanup_interval_hours",
	"ANTI_REPETITION_REWORK_ENABLED":          "anti_repetition.rework.enabled",
	"ANTI_REPETITION_MAX_REWORK_ATTEMPTS":     "anti_repetition.rework.max_attempts",
	"ANTI_REPETITION_REWORK_TEMPERATURE":      "anti_repetition.rework.temperature",
	"ANTI_REPETITION_LOCAL_REWORK_ENABLED":    "anti_repetition.rework.local_rework.enabled",
	"ANTI_REPETITION_LOCAL_REWORK_MAX_LENGTH": "anti_repetition.rework.local_rework.max_length",

	"MESSAGE_POST_PROCESSOR_ENABLED":                    "message_post_processor.enabled",
	"MESSAGE_POST_PROCESSOR_RANDOMIZATION_ENABLED":       "message_post_processor.randomization_enabled",
	"MESSAGE_POST_PROCESSOR_SINGLE_WORD_PROBABILITY":     "message_post_processor.probabilities.single_word",
	"MESSAGE_POST_PROCESSOR_SHORT_SENTENCES_PROBABILITY": "message_post_processor.probabilities.short_sentences",
	"MESSAGE_POST_PROCESSOR_LONG_MESSAGES_PROBABILITY":   "message_post_processor.probabilities.long_messages",
	"MESSAGE_POST_PROCESSOR_MIN_LENGTH":                  "message_post_processor.length.min",
	"MESSAGE_POST_PROCESSOR_MAX_LENGTH":                  "message_post_processor.length.max",
	"MESSAGE_POST_PROCESSOR_LONG_MESSAGE_THRESHOLD":      "message_post_processor.length.long_message_threshold",
	"MESSAGE_POST_PROCESSOR_FORCE_LONG_PROCESSING_THRESHOLD": "message_post_processor.length.force_long_processing_threshold",
	"MESSAGE_POST_PROCESSOR_TIMEOUT_SECONDS":             "message_post_processor.performance.timeout_seconds",
	"MESSAGE_POST_PROCESSOR_TEMPERATURE":                 "message_post_processor.performance.temperature",
	"MESSAGE_POST_PROCESSOR_CACHE_ENABLED":               "message_post_processor.cache.enabled",
	"MESSAGE_POST_PROCESSOR_CACHE_TTL_MINUTES":           "message_post_processor.cache.ttl_minutes",
	"MESSAGE_POST_PROCESSOR_EXCLUDE_TYPES":               "message_post_processor.exclude_types",
	"MESSAGE_POST_PROCESSOR_WEEKLY_SUMMARY_EXCLUDE":      "message_post_processor.weekly_summary_exclude",
	"MESSAGE_POST_PROCESSOR_DEBUG_LOGGING":               "message_post_processor.debug.logging",
	"MESSAGE_POST_PROCESSOR_LOG_ORIGINAL_MESSAGES":       "message_post_processor.debug.log_original_messages",
	"MESSAGE_POST_PROCESSOR_REPLACEMENT_CACHE_ENABLED":   "message_post_processor.cache.replacement_cache.enabled",
	"MESSAGE_POST_PROCESSOR_REPLACEMENT_CACHE_TTL_MINUTES": "message_post_processor.cache.replacement_cache.ttl_minutes",

	"AUTO_BIO_ENABLED":                "auto_bio.enabled",
	"AUTO_BIO_INTERVAL_HOURS":         "auto_bio.interval_hours",
	"AUTO_BIO_MESSAGES_LOOKBACK_DAYS": "auto_bio.lookback_days",
	"AUTO_BIO_MIN_MESSAGES_FOR_ANALYSIS": "auto_bio.min_messages_for_analysis",
	"AUTO_BIO_MAX_MESSAGES_FOR_ANALYSIS": "auto_bio.max_messages_for_analysis",

	"CAUSAL_LEARNING_ENABLED":           "causal_learning.enabled",
	"CAUSAL_ANALYSIS_INTERVAL_HOURS":    "causal_learning.analysis_interval_hours",
	"CAUSAL_MIN_CONFIDENCE":             "causal_learning.min_confidence",
	"CAUSAL_TEMPORAL_WINDOW_MINUTES":    "causal_learning.temporal_window_minutes",
	"CAUSAL_MAX_ENTRIES_PER_CHAT":       "causal_learning.max_entries_per_chat",
	"CAUSAL_ANALYSIS_LOOKBACK_MESSAGES": "causal_learning.analysis_lookback_messages",

	"EMOTIONAL_LEARNING_ENABLED":            "emotional_learning.enabled",
	"EMOTIONAL_ANALYSIS_INTERVAL_HOURS":     "emotional_learning.analysis_interval_hours",
	"EMOTIONAL_ANALYSIS_LOOKBACK_MESSAGES":  "emotional_learning.analysis_lookback_messages",
	"EMOTIONAL_MEMORY_RETENTION_DAYS":       "emotional_learning.memory_retention_days",

	"BELIEF_LEARNING_ENABLED":           "belief_learning.enabled",
	"BELIEF_ANALYSIS_INTERVAL_HOURS":    "belief_learning.analysis_interval_hours",
	"BELIEF_ANALYSIS_LOOKBACK_MESSAGES": "belief_learning.analysis_lookback_messages",

	"INTERNAL_MONOLOGUE_ENABLED": "cognitive_architecture.internal_monologue.enabled",
	"SELF_REFLECTION_ENABLED":    "cognitive_architecture.self_reflection.enabled",

	"RELATIONSHIP_TRACKING_ENABLED": "social_architecture.relationship_tracking.enabled",

	"ASSOCIATION_CLOUD_ENABLED":  "association_cloud.enabled",
	"ASSOCIATION_CLOUD_MAX_NODES": "association_cloud.max_nodes",
	"ASSOCIATION_CLOUD_MAX_EDGES": "association_cloud.max_edges",
	"ASSOCIATION_CLOUD_DECAY_DAYS": "association_cloud.decay_days",
}

// =============================================================================
// YAML-дерево и сериализация
// =============================================================================

// yamlNode — узел YAML-дерева.
type yamlNode struct {
	scalar   interface{}            // string, int, float64, bool — конечное значение
	list     []interface{}          // []string для YAML-списков
	children map[string]*yamlNode   // вложенные ключи
}

func (n *yamlNode) isScalar() bool { return n.children == nil && n.list == nil && n.scalar != nil }
func (n *yamlNode) isList() bool   { return n.children == nil && len(n.list) > 0 }
func (n *yamlNode) isMap() bool    { return n.children != nil }

// ensureChild гарантирует существование дочернего узла и возвращает его.
func (n *yamlNode) ensureChild(key string) *yamlNode {
	if n.children == nil {
		n.children = make(map[string]*yamlNode)
	}
	child, ok := n.children[key]
	if !ok {
		child = &yamlNode{}
		n.children[key] = child
	}
	return child
}

// setAtPath устанавливает значение по пути "a.b.c".
func (n *yamlNode) setAtPath(path string, val interface{}) {
	parts := strings.Split(path, ".")
	cur := n
	for i := 0; i < len(parts)-1; i++ {
		cur = cur.ensureChild(parts[i])
	}
	last := parts[len(parts)-1]
	node := cur.ensureChild(last)
	node.scalar = val
}

// setListAtPath устанавливает список по пути "a.b.c".
func (n *yamlNode) setListAtPath(path string, items []string) {
	parts := strings.Split(path, ".")
	cur := n
	for i := 0; i < len(parts)-1; i++ {
		cur = cur.ensureChild(parts[i])
	}
	last := parts[len(parts)-1]
	node := cur.ensureChild(last)
	list := make([]interface{}, len(items))
	for i, item := range items {
		list[i] = item
	}
	node.list = list
}

// writeYAML рекурсивно сериализует узел в YAML.
func (n *yamlNode) writeYAML(sb *strings.Builder, indent int) {
	if n.children == nil {
		return
	}
	keys := make([]string, 0, len(n.children))
	for k := range n.children {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	prefix := strings.Repeat("  ", indent)
	for _, k := range keys {
		child := n.children[k]
		switch {
		case child.isMap():
			sb.WriteString(fmt.Sprintf("%s%s:\n", prefix, k))
			child.writeYAML(sb, indent+1)
		case child.isList():
			sb.WriteString(fmt.Sprintf("%s%s:\n", prefix, k))
			for _, item := range child.list {
				switch v := item.(type) {
				case string:
					sb.WriteString(fmt.Sprintf("%s  - %s\n", prefix, v))
				default:
					sb.WriteString(fmt.Sprintf("%s  - %v\n", prefix, v))
				}
			}
		case child.isScalar():
			sb.WriteString(fmt.Sprintf("%s%s: %v\n", prefix, k, child.scalar))
		}
	}
}

// =============================================================================
// YAML-шаблон: полная структура luna_bot.yaml со значениями по умолчанию
// =============================================================================

func buildDefaultTree() *yamlNode {
	root := &yamlNode{}
	set := func(path, val string) { root.setAtPath(path, val) }
	setList := func(path string, items []string) { root.setListAtPath(path, items) }

	// --- llm ---
	set("llm.default_provider", strconv.Quote("gemini"))
	set("llm.fallback_enabled", "true")
	setList("llm.fallback_critical_types", []string{strconv.Quote("free_will"), strconv.Quote("direct"), strconv.Quote("direct_serious"), strconv.Quote("voice")})
	setList("llm.fallback_provider_order", []string{strconv.Quote("deepseek"), strconv.Quote("openrouter")})
	set("llm.circuit_breaker.max_failures", "5")
	set("llm.circuit_breaker.cooldown_seconds", "60")

	base := func(provider, path, val string) { set("llm.providers."+provider+"."+path, val) }
	providers := []struct {
		name    string
		baseURL string
		models  map[string]string
		extra   map[string]string
	}{
		{"gemini", strconv.Quote(""), map[string]string{
			"text": strconv.Quote("gemini-2.0-flash"), "audio": strconv.Quote("gemini-2.0-flash"),
			"embed": strconv.Quote("embedding-001"), "image": strconv.Quote("gemini-2.0-flash"),
			"image_gen": strconv.Quote("gemini-2.5-flash-image-preview"), "tts": strconv.Quote("gemini-2.5-flash-preview-tts"),
		}, map[string]string{
			"key_rotation_hours": "1", "safety.bypass_filters": "true", "safety.obfuscate": "false",
			"temperatures.normal": "1.0", "temperatures.serious": "0.9",
		}},
		{"deepseek", strconv.Quote("https://api.deepseek.com/v1"), map[string]string{
			"text": strconv.Quote("deepseek-chat"),
		}, map[string]string{
			"site_url": strconv.Quote(""), "site_title": strconv.Quote(""),
		}},
		{"openrouter", strconv.Quote("https://openrouter.ai/api/v1"), map[string]string{
			"text": strconv.Quote("deepseek/deepseek-chat-v3.1:free"),
		}, map[string]string{
			"site_url": strconv.Quote(""), "site_title": strconv.Quote("Luna Bot"),
		}},
	}
	for _, p := range providers {
		base(p.name, "enabled", "true")
		base(p.name, "api_key", strconv.Quote(""))
		base(p.name, "reserve_api_key", strconv.Quote(""))
		base(p.name, "key_rotation_hours", "0")
		base(p.name, "base_url", p.baseURL)
		base(p.name, "debug", "false")
		base(p.name, "safety.bypass_filters", "false")
		base(p.name, "safety.obfuscate", "false")
		for mk, mv := range p.models {
			base(p.name, "models."+mk, mv)
		}
		for ek, ev := range p.extra {
			base(p.name, ek, ev)
		}
	}
	for _, pn := range []string{"chatgpt", "local"} {
		base(pn, "enabled", "false")
		base(pn, "api_key", strconv.Quote(""))
		base(pn, "reserve_api_key", strconv.Quote(""))
		base(pn, "key_rotation_hours", "0")
		base(pn, "debug", "false")
		base(pn, "safety.bypass_filters", "false")
		base(pn, "safety.obfuscate", "false")
	}
	base("chatgpt", "base_url", strconv.Quote("https://api.openai.com/v1"))
	base("chatgpt", "site_url", strconv.Quote(""))
	base("chatgpt", "site_title", strconv.Quote(""))
	base("chatgpt", "models.text", strconv.Quote("gpt-4o"))
	base("chatgpt", "models.image_gen", strconv.Quote("dall-e-3"))
	base("local", "base_url", strconv.Quote("http://localhost:11434/v1"))
	base("local", "site_url", strconv.Quote(""))
	base("local", "site_title", strconv.Quote(""))
	base("local", "models.text", strconv.Quote("llama3.1:8b"))
	base("local", "models.embed", strconv.Quote("nomic-embed-text"))
	set("llm.image_analysis.enabled", "false")

	// --- tts ---
	set("tts.provider", strconv.Quote("elevenlabs"))
	set("tts.fallback", strconv.Quote("gemini_tts"))
	set("tts.elevenlabs.api_key", strconv.Quote(""))
	set("tts.elevenlabs.voice_id", strconv.Quote("Obuyk6KKzg9olSLPaCbl"))
	set("tts.elevenlabs.model", strconv.Quote("eleven_multilingual_v2"))
	set("tts.elevenlabs.plan", strconv.Quote("starter"))
	set("tts.elevenlabs.voice_settings.stability", "0.5")
	set("tts.elevenlabs.voice_settings.similarity_boost", "0.8")
	set("tts.elevenlabs.voice_settings.style", "0.0")
	set("tts.elevenlabs.voice_settings.use_speaker_boost", "true")
	set("tts.elevenlabs.voice_settings.speed", "1.0")
	set("tts.elevenlabs.prompts.style", strconv.Quote(""))
	set("tts.elevenlabs.prompts.emotion", strconv.Quote(""))
	set("tts.elevenlabs.prompts.pace", strconv.Quote(""))
	set("tts.elevenlabs.random_voice", "false")
	set("tts.gemini_tts.model", strconv.Quote("gemini-2.5-flash-preview-tts"))
	set("tts.gemini_tts.voice_name", strconv.Quote("Zephyr"))

	// --- telegram ---
	set("telegram.token", strconv.Quote(""))
	set("telegram.bot_user_id", "0")
	setList("telegram.bot_names", []string{strconv.Quote("Катя"), strconv.Quote("катя"), strconv.Quote("Luna"), strconv.Quote("luna")})
	set("telegram.admin_ids", "[]")
	set("telegram.admin_usernames", "[]")
	set("telegram.debug", "false")
	set("telegram.timezone", strconv.Quote("Asia/Yekaterinburg"))
	set("telegram.error_message_auto_delete_seconds", "5")
	set("telegram.use_structured_message_format", "true")

	// --- chat ---
	set("chat.min_messages", "10")
	set("chat.max_messages", "50")
	set("chat.context_window", "300")
	set("chat.image_generation_context_window", "50")

	// --- summary ---
	set("summary.interval_hours", "0")
	set("summary.max_parts", "5")
	set("summary.weekly.enabled", "false")
	set("summary.weekly.day", "0")
	set("summary.weekly.hour", "18")
	set("summary.weekly.minute", "0")
	set("summary.weekly.max_parts", "5")
	set("summary.weekly.search_method", strconv.Quote("both"))
	set("summary.weekly.flags_enabled", "true")
	set("summary.weekly.keywords_enabled", "true")

	// --- rate_limit ---
	set("rate_limit.static_text", strconv.Quote("Превышен лимит запросов. Попробуй позже."))

	// --- settings_prompts ---
	set("settings_prompts.enter_min_messages", strconv.Quote("Минимальное количество сообщений для ответа:"))
	set("settings_prompts.enter_max_messages", strconv.Quote("Максимальное количество сообщений для ответа:"))
	set("settings_prompts.enter_daily_time", strconv.Quote("В какой час кидать ежедневную тему (0-23):"))
	set("settings_prompts.enter_summary_interval", strconv.Quote("Интервал саммари в часах (0 - выкл):"))
	set("settings_prompts.enter_direct_limit_count", strconv.Quote("Максимальное количество прямых обращений за период:"))
	set("settings_prompts.enter_direct_limit_duration", strconv.Quote("Длительность периода лимита (в минутах):"))

	// --- srach ---
	setList("srach.keywords", []string{strconv.Quote("спор"), strconv.Quote("конфликт"), strconv.Quote("оскорбление")})
	set("srach.analysis_enabled", "true")

	// --- direct_reply_limits ---
	set("direct_reply_limits.enabled_default", "false")
	set("direct_reply_limits.count_default", "3")
	set("direct_reply_limits.duration_minutes_default", "60")

	// --- donate ---
	set("donate.time_hours", "24")
	set("donate.payment_reminder_interval_hours", "6")

	// --- free_will ---
	set("free_will.enabled", "false")
	set("free_will.intervals.min_minutes", "15.0")
	set("free_will.intervals.max_minutes", "60.0")
	set("free_will.context_window", "50")
	set("free_will.mood_update_probability", "0.1")
	set("free_will.max_decisions_per_hour", "10")
	set("free_will.voice_probability", "0.3")
	set("free_will.silence.min_minutes", "3.0")
	set("free_will.silence.max_minutes", "20.0")
	set("free_will.reactions.enabled", "true")
	set("free_will.reactions.probability", "0.2")
	set("free_will.reactions.cooldown_minutes", "5")
	set("free_will.reactions.max_per_hour", "15")
	set("free_will.direct_response.max_per_hour", "30")
	set("free_will.direct_response.min_interval_seconds", "5.0")
	set("free_will.direct_response.independent_limits", "true")
	set("free_will.image_generation.max_per_interval", "3")
	set("free_will.image_generation.interval_hours", "6")
	set("free_will.image_generation.min_decision_interval_minutes", "30")
	set("free_will.image_generation.independent_limits", "true")
	set("free_will.image_generation.frequency_hours", "12")
	set("free_will.interval_messages.enabled", "true")

	// --- voice_messages ---
	set("voice_messages.enabled", "true")
	set("voice_messages.interval.min", "50")
	set("voice_messages.interval.max", "100")
	set("voice_messages.temp_dir", strconv.Quote("/tmp/voice_messages"))

	// --- moderation ---
	set("moderation.enabled", "false")
	set("moderation.interval_minutes", "1")
	set("moderation.mute_time_minutes", "5")
	set("moderation.kick_time_minutes", "1")
	set("moderation.ban_time_minutes", "60")
	set("moderation.purge_window_duration", strconv.Quote("1h"))
	set("moderation.purge_delay_duration", strconv.Quote("0s"))
	set("moderation.check_admin_rights", "true")
	set("moderation.default_notify", "false")
	set("moderation.rules", "[]")

	// --- anti_repetition ---
	set("anti_repetition.enabled", "true")
	set("anti_repetition.max_responses_per_chat", "20")
	set("anti_repetition.similarity_threshold", "0.75")
	set("anti_repetition.time_window_hours", "24")
	set("anti_repetition.cleanup_interval_hours", "1")
	set("anti_repetition.rework.enabled", "true")
	set("anti_repetition.rework.max_attempts", "2")
	set("anti_repetition.rework.temperature", "0.8")
	set("anti_repetition.rework.local_rework.enabled", "true")
	set("anti_repetition.rework.local_rework.max_length", "50")

	// --- disambiguation ---
	set("disambiguation.enabled", "true")

	// --- message_post_processor ---
	set("message_post_processor.enabled", "true")
	set("message_post_processor.randomization_enabled", "true")
	set("message_post_processor.probabilities.single_word", "0.20")
	set("message_post_processor.probabilities.short_sentences", "0.35")
	set("message_post_processor.probabilities.long_messages", "0.25")
	set("message_post_processor.length.min", "10")
	set("message_post_processor.length.max", "2000")
	set("message_post_processor.length.long_message_threshold", "100")
	set("message_post_processor.length.force_long_processing_threshold", "200")
	set("message_post_processor.performance.timeout_seconds", "15")
	set("message_post_processor.performance.temperature", "0.9")
	set("message_post_processor.cache.enabled", "true")
	set("message_post_processor.cache.ttl_minutes", "30")
	set("message_post_processor.cache.replacement_cache.enabled", "true")
	set("message_post_processor.cache.replacement_cache.ttl_minutes", "10")
	setList("message_post_processor.exclude_types", []string{strconv.Quote("system"), strconv.Quote("error"), strconv.Quote("admin")})
	set("message_post_processor.weekly_summary_exclude", "true")
	set("message_post_processor.debug.logging", "false")
	set("message_post_processor.debug.log_original_messages", "false")

	// --- auto_bio ---
	set("auto_bio.enabled", "false")
	set("auto_bio.interval_hours", "24")
	set("auto_bio.lookback_days", "30")
	set("auto_bio.min_messages_for_analysis", "10")
	set("auto_bio.max_messages_for_analysis", "1000")

	// --- personality ---
	set("personality.update_interval_hours", "1")
	set("personality.messages_lookback", "50")
	set("personality.max_name_mentions", "10")
	set("personality.max_recent_topics", "10")
	set("personality.max_self_perceptions", "5")
	set("personality.max_discussion_contexts", "3")

	// --- reactions ---
	set("reactions.enabled", "true")
	set("reactions.clown.response_probability", "40")
	set("reactions.clown.cooldown_seconds", "30")
	set("reactions.clown.max_responses_per_hour", "10")

	// --- web_search ---
	set("web_search.enabled", "true")
	set("web_search.google_api_key", strconv.Quote(""))
	set("web_search.search_engine_id", strconv.Quote(""))
	set("web_search.max_results", "3")
	set("web_search.cache.ttl", strconv.Quote("5m"))
	set("web_search.cache.max_size", "100")

	// --- causal_learning ---
	set("causal_learning.enabled", "false")
	set("causal_learning.analysis_interval_hours", "4")
	set("causal_learning.min_confidence", "0.3")
	set("causal_learning.temporal_window_minutes", "60")
	set("causal_learning.max_entries_per_chat", "500")
	set("causal_learning.analysis_lookback_messages", "100")

	// --- emotional_learning ---
	set("emotional_learning.enabled", "true")
	set("emotional_learning.analysis_interval_hours", "2")
	set("emotional_learning.analysis_lookback_messages", "100")
	set("emotional_learning.memory_retention_days", "30")

	// --- belief_learning ---
	set("belief_learning.enabled", "false")
	set("belief_learning.analysis_interval_hours", "6")
	set("belief_learning.analysis_lookback_messages", "150")

	// --- cognitive_architecture ---
	set("cognitive_architecture.internal_monologue.enabled", "false")
	set("cognitive_architecture.internal_monologue.temperature", "0.4")
	set("cognitive_architecture.self_reflection.enabled", "false")
	set("cognitive_architecture.self_reflection.temperature", "0.5")
	set("cognitive_architecture.confidence_calibration.enabled", "false")

	// --- social_architecture ---
	set("social_architecture.relationship_tracking.enabled", "false")
	set("social_architecture.social_learning.enabled", "false")
	set("social_architecture.intimacy_growth_rate", "0.02")
	set("social_architecture.trust_decay_rate", "0.01")

	// --- association_cloud ---
	set("association_cloud.enabled", "false")
	set("association_cloud.max_nodes", "5000")
	set("association_cloud.max_edges", "50000")
	set("association_cloud.decay_days", "30")

	// --- storage ---
	set("storage.type", strconv.Quote("postgres"))
	set("storage.postgresql.host", strconv.Quote("localhost"))
	set("storage.postgresql.port", "5432")
	set("storage.postgresql.user", strconv.Quote("postgres"))
	set("storage.postgresql.password", strconv.Quote(""))
	set("storage.postgresql.dbname", strconv.Quote("luna_bot"))
	set("storage.mongodb.uri", strconv.Quote(""))
	set("storage.mongodb.dbname", strconv.Quote("luna_bot"))
	set("storage.mongodb.messages_collection", strconv.Quote("chat_messages"))
	set("storage.mongodb.user_profiles_collection", strconv.Quote("user_profiles"))
	set("storage.mongodb.settings_collection", strconv.Quote("settings"))
	set("storage.mongodb.vector_index_name", strconv.Quote("vector_index"))
	set("storage.long_term_memory.enabled", "false")
	set("storage.long_term_memory.embedding_model", strconv.Quote("embedding-001"))
	set("storage.long_term_memory.fetch_k", "5")
	set("storage.long_term_memory.backfill.batch_size", "100")
	set("storage.long_term_memory.backfill.batch_delay", strconv.Quote("1s"))
	set("storage.cleanup.enabled", "false")
	set("storage.cleanup.size_limit_mb", "450")
	set("storage.cleanup.interval_minutes", "60")
	set("storage.cleanup.chunk_duration_hours", "24")

	// --- embedding ---
	set("embedding.requests_per_minute", "240")
	set("embedding.requests_per_day", "24000")
	set("embedding.batch_size", "100")
	set("embedding.request_delay", strconv.Quote("0.3s"))
	set("embedding.batch_delay", strconv.Quote("60s"))
	set("embedding.adaptive_throttling", "true")
	set("embedding.safety_margin", "0.8")
	set("embedding.max_retries", "3")
	set("embedding.cache.enabled", "true")
	set("embedding.cache.dir", strconv.Quote("./cache/embeddings"))
	set("embedding.migration.resume_enabled", "true")
	set("embedding.migration.state_file", strconv.Quote("./migration_state.json"))

	// --- prompts ---
	set("prompts.source", strconv.Quote("files"))
	set("prompts.inline", "{}")

	return root
}

// =============================================================================
// Парсинг .env
// =============================================================================

func readEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	env := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		value = strings.Trim(value, "\"'")
		env[key] = value
	}
	return env, scanner.Err()
}

// =============================================================================
// Парсинг значений .env → YAML-скаляр или список
// =============================================================================

func parseValue(raw string) interface{} {
	raw = strings.TrimSpace(raw)

	lower := strings.ToLower(raw)
	if lower == "true" {
		return "true"
	}
	if lower == "false" {
		return "false"
	}
	if raw == "" || strings.HasPrefix(raw, "your_") {
		return nil
	}

	// Числа
	if _, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return raw
	}
	if _, err := strconv.ParseFloat(raw, 64); err == nil {
		return raw
	}

	// Duration-like (заканчивается на s/m/h с числовым префиксом) — кавычим
	if len(raw) > 1 {
		last := raw[len(raw)-1]
		if last == 's' || last == 'm' || last == 'h' {
			if _, err := strconv.Atoi(raw[:len(raw)-1]); err == nil {
				return strconv.Quote(raw)
			}
		}
	}

	// Массивы через запятую
	if strings.Contains(raw, ",") {
		parts := strings.Split(raw, ",")
		list := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				list = append(list, strconv.Quote(p))
			}
		}
		if len(list) > 0 {
			return list
		}
	}

	return strconv.Quote(raw)
}

// isBareNumber returns true if raw is an integer without any unit suffix.
func isBareNumber(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	_, err := strconv.Atoi(raw)
	return err == nil
}

// =============================================================================
// main
// =============================================================================

func main() {
	inputFile := ".env.example"
	outputFile := "configs/luna_bot.yaml"

	if len(os.Args) > 1 {
		inputFile = os.Args[1]
	}
	if len(os.Args) > 2 {
		outputFile = os.Args[2]
	}

	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("  env_to_yaml — .env → luna_bot.yaml migration")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("  Input:  %s\n", inputFile)
	fmt.Printf("  Output: %s\n", outputFile)
	fmt.Println()

	// 1. Read .env file
	env, err := readEnvFile(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR reading %s: %v\n", inputFile, err)
		os.Exit(1)
	}
	fmt.Printf("  Parsed: %d variables from %s\n", len(env), filepath.Base(inputFile))

	// 2. Build default YAML tree
	root := buildDefaultTree()

	// 3. Apply .env overrides
	durationFields := map[string]bool{
		"storage.long_term_memory.backfill.batch_delay": true,
		"embedding.request_delay":   true,
		"embedding.batch_delay":     true,
		"moderation.purge_delay_duration":  true,
		"moderation.purge_window_duration": true,
		"web_search.cache.ttl": true,
	}
	mapped := 0
	skipped := 0
	for envKey, envVal := range env {
		yamlPath, ok := envToYAMLKey[envKey]
		if !ok {
			continue
		}
		parsed := parseValue(envVal)
		if parsed == nil {
			skipped++
			continue
		}
		switch v := parsed.(type) {
		case string:
			root.setAtPath(yamlPath, v)
		case []string:
			root.setListAtPath(yamlPath, v)
		}
		// Fix bare numbers in duration fields: append "s" suffix
		if durationFields[yamlPath] && isBareNumber(envVal) {
			root.setAtPath(yamlPath, strconv.Quote(envVal+"s"))
		}
		mapped++
	}

	fmt.Printf("  Mapped: %d variables\n", mapped)
	fmt.Printf("  Skipped: %d (no mapping or placeholder value)\n", skipped)
	fmt.Printf("  Mappings defined: %d\n", len(envToYAMLKey))

	// 4. Serialize to YAML
	var sb strings.Builder
	sb.WriteString("# =============================================================================\n")
	sb.WriteString("# Luna Bot — YAML Configuration\n")
	sb.WriteString("# =============================================================================\n")
	sb.WriteString("# Generated by env_to_yaml from " + filepath.Base(inputFile) + "\n")
	sb.WriteString(fmt.Sprintf("# %d variables mapped, %d skipped\n", mapped, skipped))
	sb.WriteString("# =============================================================================\n\n")
	root.writeYAML(&sb, 0)

	// 5. Ensure output directory
	outDir := filepath.Dir(outputFile)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR creating output dir: %v\n", err)
		os.Exit(1)
	}

	// 6. Write output
	if err := os.WriteFile(outputFile, []byte(sb.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR writing %s: %v\n", outputFile, err)
		os.Exit(1)
	}

	fmt.Printf("\n  Wrote: %s (%d bytes)\n", outputFile, sb.Len())
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("  Done.")
}
