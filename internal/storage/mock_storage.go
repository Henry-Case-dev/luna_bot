package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MockStorage — in-memory mock реализация ChatHistoryStorage для тестирования.
type MockStorage struct {
	mu       sync.RWMutex
	messages map[int64][]*tgbotapi.Message
	profiles map[string]*UserProfile // key: "chatID:userID"
	settings map[int64]*ChatSettings
}

// NewMockStorage создает новый MockStorage.
func NewMockStorage() *MockStorage {
	return &MockStorage{
		messages: make(map[int64][]*tgbotapi.Message),
		profiles: make(map[string]*UserProfile),
		settings: make(map[int64]*ChatSettings),
	}
}

// Убедимся, что MockStorage реализует интерфейс ChatHistoryStorage
var _ ChatHistoryStorage = (*MockStorage)(nil)

// Close — заглушка.
func (m *MockStorage) Close() error { return nil }

// AddMessage добавляет сообщение в память.
func (m *MockStorage) AddMessage(chatID int64, message *tgbotapi.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if message == nil {
		return
	}
	m.messages[chatID] = append(m.messages[chatID], message)
}

// AddVoiceTranscriptionMessage — заглушка, вызывает AddMessage.
func (m *MockStorage) AddVoiceTranscriptionMessage(chatID int64, transcriptionMessage *tgbotapi.Message, originalVoiceUserID int64) {
	m.AddMessage(chatID, transcriptionMessage)
}

// GetMessages возвращает последние N сообщений.
func (m *MockStorage) GetMessages(chatID int64, limit int) ([]*tgbotapi.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	msgs := m.messages[chatID]
	if len(msgs) == 0 {
		return []*tgbotapi.Message{}, nil
	}
	if limit <= 0 || limit >= len(msgs) {
		result := make([]*tgbotapi.Message, len(msgs))
		copy(result, msgs)
		return result, nil
	}
	result := make([]*tgbotapi.Message, limit)
	copy(result, msgs[len(msgs)-limit:])
	return result, nil
}

// GetMessagesSince — упрощенная реализация.
func (m *MockStorage) GetMessagesSince(ctx context.Context, chatID int64, userID int64, since time.Time, limit int) ([]*tgbotapi.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	msgs := m.messages[chatID]
	var filtered []*tgbotapi.Message
	for _, msg := range msgs {
		if userID != 0 && msg.From != nil && msg.From.ID != userID {
			continue
		}
		if msg.Date < int(since.Unix()) {
			continue
		}
		filtered = append(filtered, msg)
	}
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	return filtered, nil
}

// LoadChatHistory — заглушка.
func (m *MockStorage) LoadChatHistory(chatID int64) ([]*tgbotapi.Message, error) {
	return m.GetMessages(chatID, 0)
}

// SaveChatHistory — заглушка.
func (m *MockStorage) SaveChatHistory(chatID int64) error { return nil }

// ClearChatHistory очищает сообщения чата.
func (m *MockStorage) ClearChatHistory(chatID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.messages, chatID)
	return nil
}

// AddMessagesToContext — заглушка.
func (m *MockStorage) AddMessagesToContext(chatID int64, messages []*tgbotapi.Message) {}

// GetAllChatIDs возвращает все известные chatID.
func (m *MockStorage) GetAllChatIDs() ([]int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]int64, 0, len(m.messages))
	for id := range m.messages {
		ids = append(ids, id)
	}
	return ids, nil
}

// GetStatus возвращает статус.
func (m *MockStorage) GetStatus(chatID int64) string {
	return "Хранилище: Mock (in-memory)."
}

// GetUserProfile возвращает профиль пользователя.
func (m *MockStorage) GetUserProfile(chatID int64, userID int64) (*UserProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%d:%d", chatID, userID)
	profile, ok := m.profiles[key]
	if !ok {
		return nil, nil
	}
	return profile, nil
}

// SetUserProfile сохраняет профиль пользователя.
func (m *MockStorage) SetUserProfile(profile *UserProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%d:%d", profile.ChatID, profile.UserID)
	m.profiles[key] = profile
	return nil
}

// GetAllUserProfiles возвращает все профили чата.
func (m *MockStorage) GetAllUserProfiles(chatID int64) ([]*UserProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*UserProfile
	for _, profile := range m.profiles {
		if profile.ChatID == chatID {
			result = append(result, profile)
		}
	}
	return result, nil
}

// GetChatSettings возвращает настройки чата.
func (m *MockStorage) GetChatSettings(chatID int64) (*ChatSettings, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	settings, ok := m.settings[chatID]
	if !ok {
		return &ChatSettings{ChatID: chatID}, nil
	}
	return settings, nil
}

// SetChatSettings сохраняет настройки чата.
func (m *MockStorage) SetChatSettings(settings *ChatSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings[settings.ChatID] = settings
	return nil
}

// UpdateDirectLimitEnabled обновляет настройку.
func (m *MockStorage) UpdateDirectLimitEnabled(chatID int64, enabled bool) error { return nil }

// UpdateDirectLimitCount обновляет настройку.
func (m *MockStorage) UpdateDirectLimitCount(chatID int64, count int) error { return nil }

// UpdateDirectLimitDuration обновляет настройку.
func (m *MockStorage) UpdateDirectLimitDuration(chatID int64, duration time.Duration) error {
	return nil
}

// UpdateVoiceTranscriptionEnabled обновляет настройку.
func (m *MockStorage) UpdateVoiceTranscriptionEnabled(chatID int64, enabled bool) error { return nil }

// UpdateSrachAnalysisEnabled обновляет настройку.
func (m *MockStorage) UpdateSrachAnalysisEnabled(chatID int64, enabled bool) error { return nil }

// SearchRelevantMessages — заглушка.
func (m *MockStorage) SearchRelevantMessages(chatID int64, queryText string, k int) ([]*tgbotapi.Message, error) {
	return []*tgbotapi.Message{}, nil
}

// GetTotalMessagesCount — заглушка.
func (m *MockStorage) GetTotalMessagesCount(chatID int64) (int64, error) { return 0, nil }

// FindMessagesWithoutEmbedding — заглушка.
func (m *MockStorage) FindMessagesWithoutEmbedding(chatID int64, limit int, skipMessageIDs []int) ([]MongoMessage, error) {
	return nil, nil
}

// UpdateMessageEmbedding — заглушка.
func (m *MockStorage) UpdateMessageEmbedding(chatID int64, messageID int, vector []float32) error {
	return nil
}

// UpdateMessageEmbeddingWithContext — заглушка.
func (m *MockStorage) UpdateMessageEmbeddingWithContext(chatID int64, messageID int, embedding []float32, embeddingContext string) error {
	return nil
}

// GetReplyChain — заглушка.
func (m *MockStorage) GetReplyChain(ctx context.Context, chatID int64, messageID int, maxDepth int) ([]*tgbotapi.Message, error) {
	return nil, nil
}

// GetDailySummariesForWeek — заглушка.
func (m *MockStorage) GetDailySummariesForWeek(ctx context.Context, chatID int64, botUserID int64, since time.Time, until time.Time) ([]*tgbotapi.Message, error) {
	return []*tgbotapi.Message{}, nil
}

// ResetAutoBioTimestamps — заглушка.
func (m *MockStorage) ResetAutoBioTimestamps(chatID int64) error { return nil }

// UpdateAutoBio — заглушка.
func (m *MockStorage) UpdateAutoBio(ctx context.Context, chatID int64, userID int64, autoBio string, updateTime time.Time) error {
	return nil
}

// UpdateUserLastSeen — заглушка.
func (m *MockStorage) UpdateUserLastSeen(chatID int64, userID int64, lastSeen time.Time) error {
	return nil
}

// UpdateMessageReactions — заглушка.
func (m *MockStorage) UpdateMessageReactions(chatID int64, messageID int, userID int64, username, firstName string, reactions []string) error {
	return nil
}

// GetMessageReactions — заглушка.
func (m *MockStorage) GetMessageReactions(chatID int64, messageID int) ([]string, error) {
	return nil, nil
}

// GetBotMessagesWithReactions — заглушка.
func (m *MockStorage) GetBotMessagesWithReactions(chatID int64, lookbackHours int) ([]MongoMessage, error) {
	return nil, nil
}

// AddPositiveExample — заглушка.
func (m *MockStorage) AddPositiveExample(chatID int64, message string, timestamp time.Time) error {
	return nil
}

// AddNegativeExample — заглушка.
func (m *MockStorage) AddNegativeExample(chatID int64, message string, timestamp time.Time) error {
	return nil
}

// GetPersonalityMemory — заглушка.
func (m *MockStorage) GetPersonalityMemory(chatID int64) (*PersonalityMemory, error) {
	return &PersonalityMemory{ChatID: chatID}, nil
}

// SavePersonalityMemory — заглушка.
func (m *MockStorage) SavePersonalityMemory(memory *PersonalityMemory) error { return nil }

// UpdatePersonalityField — заглушка.
func (m *MockStorage) UpdatePersonalityField(chatID int64, fieldName string, value interface{}) error {
	return nil
}

// GetMoodState — заглушка.
func (m *MockStorage) GetMoodState(chatID int64) (*MoodState, error) {
	return &MoodState{ChatID: chatID, CurrentMood: "neutral"}, nil
}

// SaveMoodState — заглушка.
func (m *MockStorage) SaveMoodState(mood *MoodState) error { return nil }

// UpdateMoodState — заглушка.
func (m *MockStorage) UpdateMoodState(chatID int64, currentMood string, moodIntensity float64, triggerReason string) error {
	return nil
}

// GetCausalMemory — заглушка.
func (m *MockStorage) GetCausalMemory(chatID int64) (*CausalMemory, error) {
	return nil, fmt.Errorf("CausalMemory не реализована для MockStorage")
}

// SaveCausalMemory — заглушка.
func (m *MockStorage) SaveCausalMemory(memory *CausalMemory) error {
	return fmt.Errorf("SaveCausalMemory не реализован для MockStorage")
}

// AddCausalEntry — заглушка.
func (m *MockStorage) AddCausalEntry(entry *CausalMemoryEntry) error {
	return fmt.Errorf("AddCausalEntry не реализован для MockStorage")
}

// GetCausalEntries — заглушка.
func (m *MockStorage) GetCausalEntries(chatID int64, options CausalQueryOptions) ([]*CausalMemoryEntry, error) {
	return nil, fmt.Errorf("GetCausalEntries не реализован для MockStorage")
}

// UpdateCausalEntry — заглушка.
func (m *MockStorage) UpdateCausalEntry(entry *CausalMemoryEntry) error {
	return fmt.Errorf("UpdateCausalEntry не реализован для MockStorage")
}

// DeleteCausalEntry — заглушка.
func (m *MockStorage) DeleteCausalEntry(chatID int64, entryID int64) error {
	return fmt.Errorf("DeleteCausalEntry не реализован для MockStorage")
}

// CleanupCausalMemory — заглушка.
func (m *MockStorage) CleanupCausalMemory(chatID int64) error {
	return fmt.Errorf("CleanupCausalMemory не реализован для MockStorage")
}

// SearchCausalEntries — заглушка.
func (m *MockStorage) SearchCausalEntries(chatID int64, keywords []string, limit int) ([]*CausalMemoryEntry, error) {
	return nil, fmt.Errorf("SearchCausalEntries не реализован для MockStorage")
}

// GetCausalEntriesByCategory — заглушка.
func (m *MockStorage) GetCausalEntriesByCategory(chatID int64, category string, limit int) ([]*CausalMemoryEntry, error) {
	return nil, fmt.Errorf("GetCausalEntriesByCategory не реализован для MockStorage")
}

// UpdateCausalEntryRelevance — заглушка.
func (m *MockStorage) UpdateCausalEntryRelevance(chatID int64, entryID int64, newRelevance float64) error {
	return fmt.Errorf("UpdateCausalEntryRelevance не реализован для MockStorage")
}

// GetAssocTopForContext — заглушка.
func (m *MockStorage) GetAssocTopForContext(chatID int64, contextKeys []string, limit int, freshnessDays int, types []string) ([]*AssocNode, []*AssocEdge, error) {
	return []*AssocNode{}, []*AssocEdge{}, nil
}

// UpdateAssocGraph — заглушка.
func (m *MockStorage) UpdateAssocGraph(chatID int64, updates *AssocUpdateBatch) error { return nil }

// GetEmotionalState — заглушка.
func (m *MockStorage) GetEmotionalState(chatID int64) (*EmotionalState, error) { return nil, nil }

// SaveEmotionalState — заглушка.
func (m *MockStorage) SaveEmotionalState(state *EmotionalState) error { return nil }

// UpdateEmotionalState — заглушка.
func (m *MockStorage) UpdateEmotionalState(chatID int64, emotions map[string]float64, intensity float64, triggerEvent string) error {
	return nil
}

// AddEmotionalMemory — заглушка.
func (m *MockStorage) AddEmotionalMemory(memory *EmotionalMemory) error { return nil }

// GetEmotionalMemories — заглушка.
func (m *MockStorage) GetEmotionalMemories(chatID int64, userID int64, limit int) ([]*EmotionalMemory, error) {
	return nil, nil
}

// GetEmotionalMemoriesByEmotion — заглушка.
func (m *MockStorage) GetEmotionalMemoriesByEmotion(chatID int64, emotion string, limit int) ([]*EmotionalMemory, error) {
	return nil, nil
}

// UpdateEmotionalMemory — заглушка.
func (m *MockStorage) UpdateEmotionalMemory(memory *EmotionalMemory) error { return nil }

// CleanupEmotionalMemories — заглушка.
func (m *MockStorage) CleanupEmotionalMemories(chatID int64, maxAge time.Duration) error { return nil }

// GetEmotionalTrends — заглушка.
func (m *MockStorage) GetEmotionalTrends(chatID int64, since time.Time, limit int) (map[string]float64, error) {
	return nil, nil
}

// GetMessageByID получает сообщение по ID.
func (m *MockStorage) GetMessageByID(chatID int64, messageID int) (*tgbotapi.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	msgs := m.messages[chatID]
	for _, msg := range msgs {
		if msg.MessageID == messageID {
			return msg, nil
		}
	}
	return nil, fmt.Errorf("сообщение %d не найдено в чате %d", messageID, chatID)
}

// GetMessagesInRange — заглушка.
func (m *MockStorage) GetMessagesInRange(ctx context.Context, chatID int64, userID int64, since time.Time, until time.Time, limit int) ([]*tgbotapi.Message, error) {
	return []*tgbotapi.Message{}, nil
}

// EnsureTotalDBSizeWithinLimit — заглушка.
func (m *MockStorage) EnsureTotalDBSizeWithinLimit(cfg *config.Config) (bool, error) {
	return true, nil
}
