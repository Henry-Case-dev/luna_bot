package storage

import (
	"context"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// UserProfile содержит информацию о пользователе чата.
type UserProfile struct {
	ID       int64     `db:"id"`
	ChatID   int64     `db:"chat_id"`   // ID чата, к которому привязан профиль
	UserID   int64     `db:"user_id"`   // ID пользователя Telegram
	Username string    `db:"username"`  // Никнейм Telegram (@username)
	Alias    string    `db:"alias"`     // Прозвище / Короткое имя (ранее FirstName)
	Gender   string    `db:"gender"`    // Пол (ранее LastName)
	RealName string    `db:"real_name"` // Реальное имя (если известно)
	Bio      string    `db:"bio"`       // Редактируемое описание/бэкграунд
	LastSeen time.Time `db:"last_seen"` // Время последнего сообщения (для актуальности)
	// Можно добавить другие поля по необходимости
	CreatedAt time.Time `db:"created_at"` // Время создания записи
	UpdatedAt time.Time `db:"updated_at"` // Время последнего обновления
	// --- Новые поля для Auto Bio ---
	AutoBio           string    `db:"auto_bio"`
	LastAutoBioUpdate time.Time `db:"last_auto_bio_update"`
	// --- Конец новых полей ---
}

// PersonalityMemory представляет "личность" бота для конкретного чата.
type PersonalityMemory struct {
	ID     int64 `db:"id"`
	ChatID int64 `db:"chat_id"` // ID чата, к которому привязана личность

	// === СТАТИЧЕСКАЯ ЛИЧНОСТЬ (НЕ ИЗМЕНЯЕТСЯ АВТОМАТИЧЕСКИ) ===
	StaticPersonality  string    `db:"static_personality"`  // Основа характера
	StyleInstructions  string    `db:"style_instructions"`  // Инструкции поведения
	PersonalityVersion int       `db:"personality_version"` // Версия для отслеживания изменений
	LastManualUpdate   time.Time `db:"last_manual_update"`  // Время последнего ручного изменения

	// === ДИНАМИЧЕСКАЯ ЛИЧНОСТЬ (ИЗМЕНЯЕТСЯ АВТОМАТИЧЕСКИ) ===
	NameMentions      map[string]bool `db:"name_mentions"`      // Отслеживание упоминания имён (бота и других)
	RecentTopics      []string        `db:"recent_topics"`      // Недавние темы обсуждения
	SelfPerception    []string        `db:"self_perception"`    // Как бот видит себя в диалоге
	DiscussionContext map[string]bool `db:"discussion_context"` // Текущие темы обсуждения

	// === АДАПТИВНЫЕ ЭЛЕМЕНТЫ ===
	CurrentViews          []string           `db:"current_views"`          // Текущие взгляды/мнения
	TemporalTraits        map[string]float64 `db:"temporal_traits"`        // Временные черты характера (ключ -> интенсивность)
	ContextualAdaptations []string           `db:"contextual_adaptations"` // Адаптации к контексту

	// === СИСТЕМА УБЕЖДЕНИЙ (ЭТАП 1 - КАУЗАЛЬНОЕ ОБУЧЕНИЕ) ===
	BeliefSystem *BeliefSystem `db:"belief_system"` // Система убеждений бота

	// === ЭМОЦИОНАЛЬНАЯ СИСТЕМА (ЭТАП 2 - ЭМОЦИОНАЛЬНАЯ АРХИТЕКТУРА) ===
	EmotionalState    *EmotionalState    `db:"emotional_state"`    // Текущее эмоциональное состояние
	EmotionalMemories []*EmotionalMemory `db:"emotional_memories"` // Эмоциональная память о пользователях

	// === КОГНИТИВНАЯ АРХИТЕКТУРА (ЭТАП 3) ===
	InternalThoughts []*InternalThought `db:"internal_thoughts"` // Внутренние мысли (история)
	MetaCognition    *MetaCognition     `db:"meta_cognition"`    // Метакогнитивное состояние

	// === СОЦИАЛЬНАЯ АРХИТЕКТУРА (ЭТАП 4) ===
	Relationships map[string]*Relationship `db:"relationships"` // Карта отношений по пользователям

	// === МЕТАДАННЫЕ ===
	CreatedAt time.Time `db:"created_at"` // Время создания записи
	UpdatedAt time.Time `db:"updated_at"` // Время последнего обновления
}

// BeliefSystem представляет систему убеждений бота
type BeliefSystem struct {
	CoreBeliefs      map[string]*BeliefEntry `db:"core_beliefs"`       // Основные убеждения
	BeliefTriggers   []*BeliefTrigger        `db:"belief_triggers"`    // События, изменившие убеждения
	BeliefConflicts  []*BeliefConflict       `db:"belief_conflicts"`   // Обнаруженные противоречия
	LastBeliefUpdate time.Time               `db:"last_belief_update"` // Последнее обновление убеждений
	BeliefVersion    int                     `db:"belief_version"`     // Версия системы убеждений
}

// BeliefEntry представляет одно убеждение с его метаданными
type BeliefEntry struct {
	Topic      string    `db:"topic"`       // Тема убеждения
	Strength   float64   `db:"strength"`    // Сила убеждения (0.0-1.0)
	Confidence float64   `db:"confidence"`  // Уверенность в убеждении (0.0-1.0)
	Content    string    `db:"content"`     // Содержание убеждения
	Evidence   []string  `db:"evidence"`    // Доказательства/основания
	LastUpdate time.Time `db:"last_update"` // Последнее обновление
	Source     string    `db:"source"`      // Источник убеждения
	Stability  float64   `db:"stability"`   // Стабильность убеждения (0.0-1.0)
}

// BeliefTrigger представляет событие, изменившее убеждение
type BeliefTrigger struct {
	Event       string    `db:"event"`        // Что произошло
	Topic       string    `db:"topic"`        // Тема убеждения
	OldStrength float64   `db:"old_strength"` // Старая сила убеждения
	NewStrength float64   `db:"new_strength"` // Новая сила убеждения
	Evidence    []string  `db:"evidence"`     // Доказательства изменения
	Confidence  float64   `db:"confidence"`   // Уверенность в изменении
	Timestamp   time.Time `db:"timestamp"`    // Время события
}

// BeliefConflict представляет противоречие между убеждениями
type BeliefConflict struct {
	Topic1     string    `db:"topic1"`     // Первое убеждение
	Topic2     string    `db:"topic2"`     // Второе убеждение
	Conflict   string    `db:"conflict"`   // Описание конфликта
	Resolution string    `db:"resolution"` // Попытка разрешения
	Severity   float64   `db:"severity"`   // Серьезность конфликта (0.0-1.0)
	Detected   time.Time `db:"detected"`   // Время обнаружения
	Resolved   bool      `db:"resolved"`   // Разрешен ли конфликт
}

// MoodState представляет настроение бота для конкретного чата.
type MoodState struct {
	ID             int64     `db:"id"`
	ChatID         int64     `db:"chat_id"`          // ID чата, к которому привязано настроение
	CurrentMood    string    `db:"current_mood"`     // Текущее настроение
	MoodIntensity  float64   `db:"mood_intensity"`   // Интенсивность настроения (0.0 - 1.0)
	LastMoodUpdate time.Time `db:"last_mood_update"` // Время последнего обновления настроения
	TriggerReason  string    `db:"trigger_reason"`   // Причина настроения
	CreatedAt      time.Time `db:"created_at"`       // Время создания записи
	UpdatedAt      time.Time `db:"updated_at"`       // Время последнего обновления
}

// EmotionalState представляет расширенную модель эмоций (Plutchik's Wheel + мета-эмоции)
type EmotionalState struct {
	ID     int64 `db:"id"`
	ChatID int64 `db:"chat_id"`

	// === БАЗОВЫЕ ЭМОЦИИ (Plutchik's Wheel) ===
	Joy          float64 `db:"joy"`          // Радость (0.0-1.0)
	Sadness      float64 `db:"sadness"`      // Грусть
	Anger        float64 `db:"anger"`        // Гнев
	Fear         float64 `db:"fear"`         // Страх
	Trust        float64 `db:"trust"`        // Доверие
	Disgust      float64 `db:"disgust"`      // Отвращение
	Surprise     float64 `db:"surprise"`     // Удивление
	Anticipation float64 `db:"anticipation"` // Предвкушение

	// === СЛОЖНЫЕ ЭМОЦИИ (комбинации базовых) ===
	Optimism       float64 `db:"optimism"`       // Радость + Предвкушение
	Contempt       float64 `db:"contempt"`       // Гнев + Отвращение
	Nostalgia      float64 `db:"nostalgia"`      // Грусть + Радость
	Anxiety        float64 `db:"anxiety"`        // Страх + Предвкушение
	Aggression     float64 `db:"aggression"`     // Гнев + Страх
	Sentimentality float64 `db:"sentimentality"` // Доверие + Радость
	Curiosity      float64 `db:"curiosity"`      // Удивление + Предвкушение
	Cynicism       float64 `db:"cynicism"`       // Отвращение + Предвкушение

	// === МЕТА-ЭМОЦИИ (эмоции о эмоциях) ===
	Uncertainty   float64 `db:"uncertainty"`   // Неуверенность в своих реакциях
	Empathy       float64 `db:"empathy"`       // Способность к сочувствию
	Irritability  float64 `db:"irritability"`  // Склонность к раздражению
	Vulnerability float64 `db:"vulnerability"` // Уязвимость
	Confidence    float64 `db:"confidence"`    // Уверенность в себе

	// === ВЛИЯНИЕ НА ПОВЕДЕНИЕ ===
	ResponseTendency map[string]float64 `db:"response_tendency"` // "sarcasm" -> 0.8, "support" -> 0.3

	// === МЕТАДАННЫЕ ===
	Intensity    float64   `db:"intensity"`     // Общая интенсивность эмоций
	Stability    float64   `db:"stability"`     // Стабильность эмоционального состояния
	LastUpdate   time.Time `db:"last_update"`   // Время последнего обновления
	TriggerEvent string    `db:"trigger_event"` // Событие, вызвавшее эмоцию
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

// EmotionalMemory представляет память о эмоциональных реакциях с пользователями
type EmotionalMemory struct {
	ID          int64  `db:"id"`
	ChatID      int64  `db:"chat_id"`
	UserID      int64  `db:"user_id"`      // С кем связана эмоция
	UserContext string `db:"user_context"` // Контекст пользователя

	// === ЭМОЦИОНАЛЬНЫЕ ДАННЫЕ ===
	Trigger          string  `db:"trigger"`           // Что вызвало эмоцию
	PrimaryEmotion   string  `db:"primary_emotion"`   // Основная эмоция
	EmotionIntensity float64 `db:"emotion_intensity"` // Интенсивность эмоции
	Response         string  `db:"response"`          // Как отреагировал
	Outcome          string  `db:"outcome"`           // Результат реакции
	Success          bool    `db:"success"`           // Была ли реакция успешной

	// === ОБУЧЕНИЕ ===
	Reinforcement float64   `db:"reinforcement"` // Подкрепление (-1.0 до 1.0)
	Frequency     int       `db:"frequency"`     // Частота возникновения
	LastAccessed  time.Time `db:"last_accessed"` // Когда последний раз использовалась

	// === КОНТЕКСТ ===
	TopicContext string   `db:"topic_context"` // Тема разговора
	Keywords     []string `db:"keywords"`      // Ключевые слова

	// === МЕТАДАННЫЕ ===
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// === КОГНИТИВНАЯ АРХИТЕКТУРА (ЭТАП 3) ===

// InternalThought представляет одну запись внутреннего монолога
type InternalThought struct {
	Type        string    `db:"type"`         // "reflection", "planning", "doubt", "curiosity"
	Content     string    `db:"content"`      // Текст мысли
	Confidence  float64   `db:"confidence"`   // Уверенность в мысли
	Triggered   string    `db:"triggered"`    // Что вызвало мысль
	ActionTaken bool      `db:"action_taken"` // Повлияло ли на ответ
	Private     bool      `db:"private"`      // Приватная мысль
	Context     string    `db:"context"`      // Контекст
	Timestamp   time.Time `db:"timestamp"`
}

// MetaCognition хранит метакогнитивные параметры на уровне чата
type MetaCognition struct {
	SelfAwareness     float64   `db:"self_awareness"`
	ConfidenceLevel   float64   `db:"confidence_level"`
	LearningRate      float64   `db:"learning_rate"`
	AdaptabilityScore float64   `db:"adaptability_score"`
	ResponseQuality   float64   `db:"response_quality"`
	EmotionalControl  float64   `db:"emotional_control"`
	SocialSkills      float64   `db:"social_skills"`
	LastSelfCheck     time.Time `db:"last_self_check"`
}

// === СОЦИАЛЬНАЯ АРХИТЕКТУРА (ЭТАП 4) ===

type Relationship struct {
	UserID int64 `db:"user_id"`
	ChatID int64 `db:"chat_id"`

	// Атрибуты отношений
	Intimacy    float64 `db:"intimacy"`    // Близость (0.0-1.0)
	Trust       float64 `db:"trust"`       // Доверие
	Respect     float64 `db:"respect"`     // Уважение
	Affection   float64 `db:"affection"`   // Симпатия
	Familiarity float64 `db:"familiarity"` // Знакомство

	// История отношений
	KeyMoments        []RelationshipEvent `db:"key_moments"`
	Conflicts         []ConflictMemory    `db:"conflicts"`
	SharedExperiences []SharedMemory      `db:"shared_experiences"`
	InsideJokes       []string            `db:"inside_jokes"`

	// Адаптация поведения
	CommunicationStyle string   `db:"communication_style"` // "formal", "casual", ...
	PreferredTopics    []string `db:"preferred_topics"`
	AvoidedTopics      []string `db:"avoided_topics"`
	Humor              string   `db:"humor"`

	// Временные характеристики
	LastInteraction   time.Time     `db:"last_interaction"`
	TotalInteractions int           `db:"total_interactions"`
	AverageGap        time.Duration `db:"average_gap"`
}

type RelationshipEvent struct {
	Type        string    `db:"type"`        // "bonding", "conflict", ...
	Description string    `db:"description"` // Описание события
	Impact      float64   `db:"impact"`      // Влияние (-1.0 .. 1.0)
	Timestamp   time.Time `db:"timestamp"`
}

type ConflictMemory struct {
	Issue      string    `db:"issue"`
	Resolution string    `db:"resolution"`
	Learned    string    `db:"learned"`
	Resolved   bool      `db:"resolved"`
	Impact     float64   `db:"impact"`
	Timestamp  time.Time `db:"timestamp"`
}

type SharedMemory struct {
	Experience   string    `db:"experience"`
	Significance float64   `db:"significance"`
	References   int       `db:"references"`
	Created      time.Time `db:"created"`
	LastMention  time.Time `db:"last_mention"`
}

// CausalMemoryEntry представляет одну причинно-следственную связь
type CausalMemoryEntry struct {
	ID     int64 `db:"id"`
	ChatID int64 `db:"chat_id"`

	// Основные поля записи
	Event      string  `db:"event"`      // Что произошло
	Cause      string  `db:"cause"`      // Причина
	Effect     string  `db:"effect"`     // Следствие/изменение
	Category   string  `db:"category"`   // Категория: "opinion", "relationship", "worldview", "habit", "preference"
	Confidence float64 `db:"confidence"` // Уверенность в связи (0.0-1.0)

	// Метаданные
	TriggerType string   `db:"trigger_type"` // Тип триггера: "conversation", "reaction", "pattern", "conflict"
	Keywords    []string `db:"keywords"`     // Ключевые слова для поиска

	// Временные метки
	CreatedAt    time.Time `db:"created_at"`
	LastAccessed time.Time `db:"last_accessed"` // Когда последний раз использовалась

	// Важность и актуальность
	Importance float64 `db:"importance"` // Важность записи (0.0-1.0)
	Relevance  float64 `db:"relevance"`  // Текущая актуальность (уменьшается со временем)

	// Связи
	RelatedEntries []int64 `db:"related_entries"` // Связанные записи

	// Контекст
	UserContext  string `db:"user_context"`  // С кем связано (если применимо)
	TopicContext string `db:"topic_context"` // Тема обсуждения
}

// CausalMemory представляет коллекцию причинно-следственных связей для чата
type CausalMemory struct {
	ChatID       int64     `db:"chat_id"`
	TotalEntries int       `db:"total_entries"`
	LastCleanup  time.Time `db:"last_cleanup"`

	// Статистика
	CategoryCounts map[string]int `db:"category_counts"`

	// Настройки
	MaxEntries    int     `db:"max_entries"`    // Максимальное количество записей
	MinConfidence float64 `db:"min_confidence"` // Минимальная уверенность для сохранения
	DecayRate     float64 `db:"decay_rate"`     // Скорость затухания актуальности

	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// ChatHistoryStorage определяет интерфейс для работы с историей сообщений и профилями.
type ChatHistoryStorage interface {
	// === Методы для истории сообщений ===
	AddMessage(chatID int64, message *tgbotapi.Message)
	AddVoiceTranscriptionMessage(chatID int64, transcriptionMessage *tgbotapi.Message, originalVoiceUserID int64)
	GetMessages(chatID int64, limit int) ([]*tgbotapi.Message, error)
	GetMessageByID(chatID int64, messageID int) (*tgbotapi.Message, error)
	GetMessagesSince(ctx context.Context, chatID int64, userID int64, since time.Time, limit int) ([]*tgbotapi.Message, error)
	GetMessagesInRange(ctx context.Context, chatID int64, userID int64, since time.Time, until time.Time, limit int) ([]*tgbotapi.Message, error)
	EnsureTotalDBSizeWithinLimit(cfg *config.Config) (bool, error)
	LoadChatHistory(chatID int64) ([]*tgbotapi.Message, error)
	SaveChatHistory(chatID int64) error
	ClearChatHistory(chatID int64) error
	AddMessagesToContext(chatID int64, messages []*tgbotapi.Message)
	GetAllChatIDs() ([]int64, error)

	// === Методы для профилей пользователей ===
	GetUserProfile(chatID int64, userID int64) (*UserProfile, error)
	SetUserProfile(profile *UserProfile) error
	GetAllUserProfiles(chatID int64) ([]*UserProfile, error)

	// === Методы для работы с PersonalityMemory ===
	GetPersonalityMemory(chatID int64) (*PersonalityMemory, error)
	SavePersonalityMemory(memory *PersonalityMemory) error
	UpdatePersonalityField(chatID int64, fieldName string, value interface{}) error

	// === Методы для работы с настроением бота ===
	GetMoodState(chatID int64) (*MoodState, error)
	SaveMoodState(mood *MoodState) error
	UpdateMoodState(chatID int64, currentMood string, moodIntensity float64, triggerReason string) error

	// === Методы для работы с эмбеддингами ===
	GetTotalMessagesCount(chatID int64) (int64, error)
	FindMessagesWithoutEmbedding(chatID int64, limit int, skipMessageIDs []int) ([]MongoMessage, error)
	UpdateMessageEmbedding(chatID int64, messageID int, vector []float32) error

	// === Методы для долгосрочной памяти ===
	SearchRelevantMessages(chatID int64, queryText string, k int) ([]*tgbotapi.Message, error)

	// === Методы для получения ветки ответов ===
	GetReplyChain(ctx context.Context, chatID int64, messageID int, maxDepth int) ([]*tgbotapi.Message, error)

	// === Методы для получения ежедневных саммари ===
	GetDailySummariesForWeek(ctx context.Context, chatID int64, botUserID int64, since time.Time, until time.Time) ([]*tgbotapi.Message, error)

	// === Методы для сброса времени AutoBio ===
	ResetAutoBioTimestamps(chatID int64) error
	UpdateAutoBio(ctx context.Context, chatID int64, userID int64, autoBio string, updateTime time.Time) error
	UpdateUserLastSeen(chatID int64, userID int64, lastSeen time.Time) error

	// === Методы для работы с реакциями ===
	UpdateMessageReactions(chatID int64, messageID int, userID int64, username, firstName string, reactions []string) error
	GetMessageReactions(chatID int64, messageID int) ([]string, error)
	GetBotMessagesWithReactions(chatID int64, lookbackHours int) ([]MongoMessage, error)

	// === Методы для работы с примерами хороших/плохих сообщений ===
	AddPositiveExample(chatID int64, message string, timestamp time.Time) error
	AddNegativeExample(chatID int64, message string, timestamp time.Time) error

	// === Методы для работы с настройками чатов ===
	GetChatSettings(chatID int64) (*ChatSettings, error)
	SetChatSettings(settings *ChatSettings) error

	// --- Методы для обновления отдельных настроек лимитов ---
	UpdateDirectLimitEnabled(chatID int64, enabled bool) error
	UpdateDirectLimitCount(chatID int64, count int) error
	UpdateDirectLimitDuration(chatID int64, duration time.Duration) error
	UpdateVoiceTranscriptionEnabled(chatID int64, enabled bool) error
	UpdateSrachAnalysisEnabled(chatID int64, enabled bool) error

	// === Методы для работы с казуальной памятью ===
	GetCausalMemory(chatID int64) (*CausalMemory, error)
	SaveCausalMemory(memory *CausalMemory) error
	AddCausalEntry(entry *CausalMemoryEntry) error
	GetCausalEntries(chatID int64, options CausalQueryOptions) ([]*CausalMemoryEntry, error)
	UpdateCausalEntry(entry *CausalMemoryEntry) error
	DeleteCausalEntry(chatID int64, entryID int64) error
	CleanupCausalMemory(chatID int64) error
	SearchCausalEntries(chatID int64, keywords []string, limit int) ([]*CausalMemoryEntry, error)
	GetCausalEntriesByCategory(chatID int64, category string, limit int) ([]*CausalMemoryEntry, error)
	UpdateCausalEntryRelevance(chatID int64, entryID int64, newRelevance float64) error

	// === Методы для работы с эмоциональной системой ===
	GetEmotionalState(chatID int64) (*EmotionalState, error)
	SaveEmotionalState(state *EmotionalState) error
	UpdateEmotionalState(chatID int64, emotions map[string]float64, intensity float64, triggerEvent string) error
	AddEmotionalMemory(memory *EmotionalMemory) error
	GetEmotionalMemories(chatID int64, userID int64, limit int) ([]*EmotionalMemory, error)
	GetEmotionalMemoriesByEmotion(chatID int64, emotion string, limit int) ([]*EmotionalMemory, error)
	UpdateEmotionalMemory(memory *EmotionalMemory) error
	CleanupEmotionalMemories(chatID int64, maxAge time.Duration) error
	GetEmotionalTrends(chatID int64, since time.Time, limit int) (map[string]float64, error)

	// === Общие методы ===
	Close() error
	GetStatus(chatID int64) string
	GetAssocTopForContext(chatID int64, contextKeys []string, limit int, freshnessDays int, types []string) ([]*AssocNode, []*AssocEdge, error)
	UpdateAssocGraph(chatID int64, updates *AssocUpdateBatch) error
}

// FileStorage реализует ChatHistoryStorage с использованием файлов.

// ChatSettings содержит настройки, специфичные для чата, которые сохраняются в БД.
type ChatSettings struct {
	ChatID                    int64    `db:"chat_id"`
	ConversationStyle         string   `db:"conversation_style"`
	Temperature               *float64 `db:"temperature"` // Указатель, чтобы отличить 0 от отсутствия значения
	Model                     string   `db:"model"`
	GeminiSafetyThreshold     string   `db:"gemini_safety_threshold"`
	VoiceTranscriptionEnabled *bool    `db:"voice_transcription_enabled"` // Включена ли транскрипция ГС
	// --- Новые поля для лимита прямых ответов ---
	DirectReplyLimitEnabled  *bool `db:"direct_reply_limit_enabled"`          // Включен ли лимит
	DirectReplyLimitCount    *int  `db:"direct_reply_limit_count"`            // Макс. кол-во обращений
	DirectReplyLimitDuration *int  `db:"direct_reply_limit_duration_minutes"` // Длительность периода (в минутах)
	// --- Настройка анализа срачей ---
	SrachAnalysisEnabled *bool `db:"srach_analysis_enabled"` // Включен ли анализ срачей для чата
	// --- Настройка анализа фотографий ---
	PhotoAnalysisEnabled *bool `db:"photo_analysis_enabled"` // Включен ли анализ фотографий для чата
}

// === Associative Memory Graph Types ===
type AssocNode struct {
	Type     string    `db:"type" json:"type"`
	Key      string    `db:"key" json:"key"`
	Score    float64   `db:"score" json:"score"`
	LastSeen time.Time `db:"last_seen" json:"last_seen"`
}

type AssocEdge struct {
	FromKey  string    `db:"from" json:"from"`
	FromType string    `db:"from_type" json:"from_type"`
	ToKey    string    `db:"to" json:"to"`
	ToType   string    `db:"to_type" json:"to_type"`
	Weight   float64   `db:"weight" json:"weight"`
	LastSeen time.Time `db:"last_seen" json:"last_seen"`
}

type AssocUpdate struct {
	NodeType      string
	NodeKey       string
	Increments    map[string]float64 // neighborKey → delta weight
	NeighborTypes map[string]string  // neighborKey → type
}

type AssocUpdateBatch struct {
	Nodes []*AssocUpdate
	Decay float64 // 0..1 multiplicative decay to apply
}

// MongoMessage - структура для хранения сообщений в MongoDB (устаревшая, только для совместимости).
type MongoMessage struct {
	ID                     int64                     `db:"id"`
	ChatID                 int64                     `db:"chat_id"`
	MessageID              int                       `db:"message_id"`
	UserID                 int64                     `db:"user_id"`
	Username               string                    `db:"username"`
	FirstName              string                    `db:"first_name"`
	LastName               string                    `db:"last_name"`
	IsBot                  bool                      `db:"is_bot"`
	Date                   time.Time                 `db:"date"`
	Text                   string                    `db:"text"`
	ReplyToMessageID       int                       `db:"reply_to_message_id"`
	Entities               []tgbotapi.MessageEntity  `db:"entities"`
	Caption                string                    `db:"caption"`
	CaptionEntities        []tgbotapi.MessageEntity  `db:"caption_entities"`
	HasMedia               bool                      `db:"has_media"`
	IsVoice                bool                      `db:"is_voice"`
	MessageVector          []float32                 `db:"message_vector"`
	IsForward              bool                      `db:"is_forward"`
	ForwardedFromUserID    int64                     `db:"forwarded_from_user_id"`
	ForwardedFromChatID    int64                     `db:"forwarded_from_chat_id"`
	ForwardedFromMessageID int                       `db:"forwarded_from_message_id"`
	ForwardedDate          time.Time                 `db:"forwarded_date"`
	Reactions              map[string][]ReactionInfo `db:"reactions"`
	Summary                bool                      `db:"summary"`
	WeeklySummary          bool                      `db:"weekly_summary"`
	IsVoiceTranscription   bool                      `db:"is_voice_transcription"`
	OriginalVoiceUserID    int64                     `db:"original_voice_user_id"`
}

// ReactionInfo содержит информацию о реакции пользователя
type ReactionInfo struct {
	UserID    int64     `db:"user_id"`
	Username  string    `db:"username"`
	FirstName string    `db:"first_name"`
	Timestamp time.Time `db:"timestamp"`
}

// CausalQueryOptions опции для поиска причинно-следственных связей
type CausalQueryOptions struct {
	Categories     []string `json:"categories"`
	Keywords       []string `json:"keywords"`
	MinConfidence  float64  `json:"min_confidence"`
	MinRelevance   float64  `json:"min_relevance"`
	MinImportance  float64  `json:"min_importance"`
	UserContext    string   `json:"user_context"`
	TopicContext   string   `json:"topic_context"`
	TriggerTypes   []string `json:"trigger_types"`
	SortBy         string   `json:"sort_by"`
	Limit          int      `json:"limit"`
	IncludeExpired bool     `json:"include_expired"`
}

// Убедимся, что все типы хранилищ реализуют интерфейс ChatHistoryStorage.
var _ ChatHistoryStorage = (*FileStorage)(nil)
var _ ChatHistoryStorage = (*PostgresStorage)(nil)
var _ ChatHistoryStorage = (*MockStorage)(nil)
