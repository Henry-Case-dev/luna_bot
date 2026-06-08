package bot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// --- Minimal in-memory storage stub implementing ChatHistoryStorage ---
type memStore struct {
	mems map[int64]*storage.PersonalityMemory
}

func newMemStore() *memStore { return &memStore{mems: map[int64]*storage.PersonalityMemory{}} }

// Only used in tests below
func (m *memStore) GetPersonalityMemory(chatID int64) (*storage.PersonalityMemory, error) {
	if mm, ok := m.mems[chatID]; ok {
		return mm, nil
	}
	mm := &storage.PersonalityMemory{ChatID: chatID}
	m.mems[chatID] = mm
	return mm, nil
}
func (m *memStore) SavePersonalityMemory(memory *storage.PersonalityMemory) error {
	m.mems[memory.ChatID] = memory
	return nil
}

// --- No-op implementations to satisfy the interface ---
func (m *memStore) AddMessage(chatID int64, message *tgbotapi.Message) {}
func (m *memStore) AddVoiceTranscriptionMessage(chatID int64, transcriptionMessage *tgbotapi.Message, originalVoiceUserID int64) {
}
func (m *memStore) GetMessages(chatID int64, limit int) ([]*tgbotapi.Message, error) { return nil, nil }
func (m *memStore) GetMessageByID(chatID int64, messageID int) (*tgbotapi.Message, error) {
	return nil, nil
}
func (m *memStore) GetMessagesSince(ctx context.Context, chatID int64, userID int64, since time.Time, limit int) ([]*tgbotapi.Message, error) {
	return nil, nil
}
func (m *memStore) GetMessagesInRange(ctx context.Context, chatID int64, userID int64, since time.Time, until time.Time, limit int) ([]*tgbotapi.Message, error) {
	return nil, nil
}
func (m *memStore) EnsureTotalDBSizeWithinLimit(cfg *config.Config) (bool, error)   { return false, nil }
func (m *memStore) LoadChatHistory(chatID int64) ([]*tgbotapi.Message, error)       { return nil, nil }
func (m *memStore) SaveChatHistory(chatID int64) error                              { return nil }
func (m *memStore) ClearChatHistory(chatID int64) error                             { return nil }
func (m *memStore) AddMessagesToContext(chatID int64, messages []*tgbotapi.Message) {}
func (m *memStore) GetAllChatIDs() ([]int64, error)                                 { return nil, nil }
func (m *memStore) GetUserProfile(chatID int64, userID int64) (*storage.UserProfile, error) {
	return nil, nil
}
func (m *memStore) SetUserProfile(profile *storage.UserProfile) error               { return nil }
func (m *memStore) GetAllUserProfiles(chatID int64) ([]*storage.UserProfile, error) { return nil, nil }
func (m *memStore) UpdatePersonalityField(chatID int64, fieldName string, value interface{}) error {
	return nil
}
func (m *memStore) GetMoodState(chatID int64) (*storage.MoodState, error) { return nil, nil }
func (m *memStore) SaveMoodState(mood *storage.MoodState) error           { return nil }
func (m *memStore) UpdateMoodState(chatID int64, currentMood string, moodIntensity float64, triggerReason string) error {
	return nil
}
func (m *memStore) GetTotalMessagesCount(chatID int64) (int64, error) { return 0, nil }
func (m *memStore) FindMessagesWithoutEmbedding(chatID int64, limit int, skipMessageIDs []int) ([]storage.MongoMessage, error) {
	return nil, nil
}
func (m *memStore) UpdateMessageEmbedding(chatID int64, messageID int, vector []float32) error {
	return nil
}
func (m *memStore) SearchRelevantMessages(chatID int64, queryText string, k int) ([]*tgbotapi.Message, error) {
	return nil, nil
}
func (m *memStore) GetReplyChain(ctx context.Context, chatID int64, messageID int, maxDepth int) ([]*tgbotapi.Message, error) {
	return nil, nil
}
func (m *memStore) GetDailySummariesForWeek(ctx context.Context, chatID int64, botUserID int64, since time.Time, until time.Time) ([]*tgbotapi.Message, error) {
	return nil, nil
}
func (m *memStore) ResetAutoBioTimestamps(chatID int64) error { return nil }
func (m *memStore) UpdateAutoBio(ctx context.Context, chatID int64, userID int64, autoBio string, updateTime time.Time) error {
	return nil
}
func (m *memStore) UpdateUserLastSeen(chatID int64, userID int64, lastSeen time.Time) error {
	return nil
}
func (m *memStore) UpdateMessageReactions(chatID int64, messageID int, userID int64, username, firstName string, reactions []string) error {
	return nil
}
func (m *memStore) GetMessageReactions(chatID int64, messageID int) ([]string, error) {
	return nil, nil
}
func (m *memStore) GetBotMessagesWithReactions(chatID int64, lookbackHours int) ([]storage.MongoMessage, error) {
	return nil, nil
}
func (m *memStore) AddPositiveExample(chatID int64, message string, timestamp time.Time) error {
	return nil
}
func (m *memStore) AddNegativeExample(chatID int64, message string, timestamp time.Time) error {
	return nil
}
func (m *memStore) GetChatSettings(chatID int64) (*storage.ChatSettings, error)          { return nil, nil }
func (m *memStore) SetChatSettings(settings *storage.ChatSettings) error                 { return nil }
func (m *memStore) UpdateDirectLimitEnabled(chatID int64, enabled bool) error            { return nil }
func (m *memStore) UpdateDirectLimitCount(chatID int64, count int) error                 { return nil }
func (m *memStore) UpdateDirectLimitDuration(chatID int64, duration time.Duration) error { return nil }
func (m *memStore) UpdateVoiceTranscriptionEnabled(chatID int64, enabled bool) error     { return nil }
func (m *memStore) UpdateSrachAnalysisEnabled(chatID int64, enabled bool) error          { return nil }
func (m *memStore) GetCausalMemory(chatID int64) (*storage.CausalMemory, error)          { return nil, nil }
func (m *memStore) SaveCausalMemory(memory *storage.CausalMemory) error                  { return nil }
func (m *memStore) AddCausalEntry(entry *storage.CausalMemoryEntry) error                { return nil }
func (m *memStore) GetCausalEntries(chatID int64, options storage.CausalQueryOptions) ([]*storage.CausalMemoryEntry, error) {
	return nil, nil
}
func (m *memStore) UpdateCausalEntry(entry *storage.CausalMemoryEntry) error { return nil }
func (m *memStore) DeleteCausalEntry(chatID int64, entryID int64) error      { return nil }
func (m *memStore) CleanupCausalMemory(chatID int64) error                   { return nil }
func (m *memStore) SearchCausalEntries(chatID int64, keywords []string, limit int) ([]*storage.CausalMemoryEntry, error) {
	return nil, nil
}
func (m *memStore) GetCausalEntriesByCategory(chatID int64, category string, limit int) ([]*storage.CausalMemoryEntry, error) {
	return nil, nil
}
func (m *memStore) UpdateCausalEntryRelevance(chatID int64, entryID int64, newRelevance float64) error {
	return nil
}
func (m *memStore) GetEmotionalState(chatID int64) (*storage.EmotionalState, error) { return nil, nil }
func (m *memStore) SaveEmotionalState(state *storage.EmotionalState) error          { return nil }
func (m *memStore) UpdateEmotionalState(chatID int64, emotions map[string]float64, intensity float64, triggerEvent string) error {
	return nil
}
func (m *memStore) AddEmotionalMemory(memory *storage.EmotionalMemory) error { return nil }
func (m *memStore) GetEmotionalMemories(chatID int64, userID int64, limit int) ([]*storage.EmotionalMemory, error) {
	return nil, nil
}
func (m *memStore) GetEmotionalMemoriesByEmotion(chatID int64, emotion string, limit int) ([]*storage.EmotionalMemory, error) {
	return nil, nil
}
func (m *memStore) UpdateEmotionalMemory(memory *storage.EmotionalMemory) error       { return nil }
func (m *memStore) CleanupEmotionalMemories(chatID int64, maxAge time.Duration) error { return nil }
func (m *memStore) GetEmotionalTrends(chatID int64, since time.Time, limit int) (map[string]float64, error) {
	return nil, nil
}
func (m *memStore) Close() error                  { return nil }
func (m *memStore) GetStatus(chatID int64) string { return "OK" }
func (m *memStore) GetAssocTopForContext(chatID int64, contextKeys []string, limit int, freshnessDays int, types []string) ([]*storage.AssocNode, []*storage.AssocEdge, error) {
	return nil, nil, nil
}
func (m *memStore) UpdateAssocGraph(chatID int64, updates *storage.AssocUpdateBatch) error {
	return nil
}

// --- Tests ---

func TestUpdateRelationshipAndAnalyzePrompt(t *testing.T) {
	cfg := &config.Config{RelationshipTrackingEnabled: true, TrustDecayRate: 0, IntimacyGrowthRate: 0}
	ms := newMemStore()
	b := &Bot{storage: ms, config: cfg}

	chatID := int64(1234)
	userID := int64(42)

	// Simulate multiple interactions to accumulate relationship data
	b.UpdateRelationshipFromInteraction(chatID, userID, "direct_message", "positive", "Привет!")
	b.UpdateRelationshipFromInteraction(chatID, userID, "joke_shared", "positive", "шутка")
	b.UpdateRelationshipFromInteraction(chatID, userID, "question", "neutral", "?")

	mem, _ := ms.GetPersonalityMemory(chatID)
	if mem.Relationships == nil {
		t.Fatalf("Relationships map is nil")
	}
	key := "42"
	rel, ok := mem.Relationships[key]
	if !ok {
		t.Fatalf("Expected relationship for key %s", key)
	}
	if rel.TotalInteractions < 3 {
		t.Fatalf("Expected at least 3 interactions, got %d", rel.TotalInteractions)
	}

	// Analyze relationship context for prompt should be non-empty now
	ctx := b.AnalyzeRelationshipForPrompt(chatID, userID)
	if ctx == "" {
		t.Fatalf("Expected non-empty relationship context")
	}
}

func TestInternalMonologueRecording(t *testing.T) {
	cfg := &config.Config{InternalMonologueEnabled: true, InternalMonologuePromptEnabled: false}
	ms := newMemStore()
	b := &Bot{storage: ms, config: cfg}
	chatID := int64(777)

	thought := b.InternalMonologue(chatID, "Почему небо голубое?", "direct")
	if thought == nil {
		t.Fatalf("Expected a thought to be generated")
	}
	if thought.Content == "" {
		t.Errorf("Thought content should not be empty")
	}

	mem, _ := ms.GetPersonalityMemory(chatID)
	if len(mem.InternalThoughts) == 0 {
		t.Fatalf("Expected thought to be saved in memory")
	}

	// Ensure RecordInternalThought caps history to 100
	for i := 0; i < 120; i++ {
		b.RecordInternalThought(chatID, &storage.InternalThought{Type: "reflection", Content: "x", Timestamp: time.Now()})
	}
	mem2, _ := ms.GetPersonalityMemory(chatID)
	if len(mem2.InternalThoughts) > 100 {
		t.Fatalf("Expected thoughts capped to 100, got %d", len(mem2.InternalThoughts))
	}
}

func TestApplyRelationshipStyleToContext_PrependsToneHint(t *testing.T) {
	cfg := &config.Config{RelationshipTrackingEnabled: true}
	ms := newMemStore()
	b := &Bot{storage: ms, config: cfg}

	chatID := int64(999)
	userID := int64(7)

	mem, _ := ms.GetPersonalityMemory(chatID)
	if mem.Relationships == nil {
		mem.Relationships = make(map[string]*storage.Relationship)
	}
	mem.Relationships["7"] = &storage.Relationship{
		UserID:             userID,
		ChatID:             chatID,
		Intimacy:           0.85,
		Affection:          0.7,
		Trust:              0.6,
		Respect:            0.5,
		Familiarity:        0.9,
		SharedExperiences:  []storage.SharedMemory{},
		CommunicationStyle: "",
		TotalInteractions:  10,
		LastInteraction:    time.Now(),
	}
	_ = ms.SavePersonalityMemory(mem)

	input := "Original Context"
	out := b.ApplyRelationshipStyleToContext(chatID, userID, input)

	if !strings.HasPrefix(out, "[tone_hint]:") {
		t.Fatalf("expected tone hint prefix, got: %q", out)
	}
	if !strings.Contains(out, "[style=warm_casual]") {
		t.Fatalf("expected warm_casual style tag, got: %q", out)
	}
	if !strings.HasSuffix(strings.TrimSpace(out), input) {
		t.Fatalf("expected original context preserved at end, got: %q", out)
	}
}

func TestApplyRelationshipStyleToContext_NeutralWhenNoRelationship(t *testing.T) {
	cfg := &config.Config{RelationshipTrackingEnabled: true}
	ms := newMemStore()
	b := &Bot{storage: ms, config: cfg}

	chatID := int64(1001)
	userID := int64(0) // no relationship recorded

	input := "X"
	out := b.ApplyRelationshipStyleToContext(chatID, userID, input)
	if !strings.HasPrefix(out, "[tone_hint]:") {
		t.Fatalf("expected tone hint prefix, got: %q", out)
	}
	if !strings.Contains(out, "[style=neutral]") {
		t.Fatalf("expected neutral style tag, got: %q", out)
	}
}

func TestBuildStyleCueMapping(t *testing.T) {
	cases := map[string]string{
		"warm_casual":             "[style=warm_casual]",
		"respectful_professional": "[style=respectful_professional]",
		"familiar_friendly":       "[style=familiar_friendly]",
		"cautious_distant":        "[style=cautious_distant]",
		"neutral":                 "[style=neutral]",
		"unknown":                 "[style=neutral]",
	}
	for in, want := range cases {
		cue := BuildStyleCue(in)
		if !strings.Contains(cue, want) {
			t.Fatalf("style %q: expected cue to contain %q, got: %q", in, want, cue)
		}
	}
}
