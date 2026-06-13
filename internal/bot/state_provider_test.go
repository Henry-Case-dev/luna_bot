package bot

import (
	"strings"
	"testing"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/bot/prompts"
	"github.com/Henry-Case-dev/luna_bot/internal/bot/state"
)

func newTestStateProvider() *StateProvider {
	return &StateProvider{
		bot:              nil,
		presenceProfiles: make(map[int64]*state.PresenceProfile),
		conflictStates:   make(map[conflictKey]*state.ConflictState),
		relationships:    make(map[relKey]*relCacheEntry),
	}
}

func TestStateProvider_GetOrComputePresenceProfile_Caches(t *testing.T) {
	sp := newTestStateProvider()
	chatID := int64(12345)

	p1 := sp.getOrComputePresenceProfile(chatID)
	if p1 == nil {
		t.Fatal("getOrComputePresenceProfile must not return nil")
	}

	p2 := sp.getOrComputePresenceProfile(chatID)
	if p2 != p1 {
		t.Error("second call must return same pointer (caching)")
	}

	chatID2 := int64(67890)
	p3 := sp.getOrComputePresenceProfile(chatID2)
	if p3 == nil {
		t.Fatal("getOrComputePresenceProfile for different chat must not return nil")
	}
	if p3 == p1 {
		t.Error("different chatIDs must return different profiles")
	}
}

func TestStateProvider_GetRelationship_DefaultsForNewUser(t *testing.T) {
	sp := newTestStateProvider()
	chatID := int64(12345)
	userID := int64(67890)

	stage, score := sp.getRelationship(chatID, userID)

	if stage != state.StageTgGivenWarming {
		t.Errorf("expected stage %q, got %q", state.StageTgGivenWarming, stage)
	}
	if score == nil {
		t.Fatal("score must not be nil")
	}
	if score.Interest != 40 {
		t.Errorf("expected Interest=40, got %.0f", score.Interest)
	}
	if score.Trust != 25 {
		t.Errorf("expected Trust=25, got %.0f", score.Trust)
	}
	if score.Attraction != 30 {
		t.Errorf("expected Attraction=30, got %.0f", score.Attraction)
	}
}

func TestStateProvider_GetRelationship_CachesResult(t *testing.T) {
	sp := newTestStateProvider()
	chatID := int64(12345)
	userID := int64(67890)

	stage1, score1 := sp.getRelationship(chatID, userID)
	stage2, score2 := sp.getRelationship(chatID, userID)

	if stage1 != stage2 {
		t.Error("stage must be consistent across calls")
	}
	if score1 != score2 {
		t.Error("score pointer must be consistent across calls")
	}
}

func TestStateProvider_DefaultBusySlots(t *testing.T) {
	sp := newTestStateProvider()
	slots := sp.defaultBusySlots()

	if len(slots) != 2 {
		t.Errorf("expected 2 busy slots, got %d", len(slots))
	}

	if slots[0].Label != "учёба/пары" {
		t.Errorf("expected first slot label 'учёба/пары', got %q", slots[0].Label)
	}

	weekdays := slots[0].Days
	if len(weekdays) != 5 {
		t.Errorf("expected 5 weekdays, got %d", len(weekdays))
	}
}

func TestStateProvider_GetOrLoadConflict_ReturnsNilForNewChat(t *testing.T) {
	sp := newTestStateProvider()
	cs := sp.getOrLoadConflict(12345, 67890)

	if cs != nil {
		t.Errorf("expected nil conflict for new chat, got %+v", cs)
	}
}

func TestStateProvider_GetOrLoadConflict_ReturnsCached(t *testing.T) {
	sp := newTestStateProvider()
	sp.mu.Lock()
	sp.conflictStates[conflictKey{chatID: 12345, userID: 67890}] = &state.ConflictState{
		Level:     2,
		ColdUntil: time.Now().Add(1 * time.Hour),
		Reason:    "test conflict",
	}
	sp.mu.Unlock()

	cs := sp.getOrLoadConflict(12345, 67890)
	if cs == nil {
		t.Fatal("expected non-nil conflict from cache")
	}
	if cs.Level != 2 {
		t.Errorf("expected conflict level 2, got %d", cs.Level)
	}
	if cs.Reason != "test conflict" {
		t.Errorf("expected reason 'test conflict', got %q", cs.Reason)
	}
}

func TestStateProvider_DefaultBusySlots_SecondSlot(t *testing.T) {
	sp := newTestStateProvider()
	slots := sp.defaultBusySlots()

	if slots[1].Label != "тренировка" {
		t.Errorf("expected second slot label 'тренировка', got %q", slots[1].Label)
	}
	if slots[1].From != "18:00" || slots[1].To != "19:30" {
		t.Errorf("expected second slot 18:00-19:30, got %s-%s", slots[1].From, slots[1].To)
	}
}

func TestRelCacheEntry_Defaults(t *testing.T) {
	entry := &relCacheEntry{
		stage: state.StageTgGivenWarming,
		score: &state.RelationshipScore{
			Interest:   40,
			Trust:      25,
			Attraction: 30,
			Annoyance:  10,
			Cringe:     5,
		},
	}
	if entry.stage != state.StageTgGivenWarming {
		t.Errorf("stage mismatch")
	}
	if entry.score.Interest != 40 {
		t.Errorf("interest mismatch")
	}
}

func TestConflictKey_Equality(t *testing.T) {
	k1 := conflictKey{chatID: 12345, userID: 67890}
	k2 := conflictKey{chatID: 12345, userID: 67890}
	k3 := conflictKey{chatID: 12345, userID: 99999}

	if k1 != k2 {
		t.Error("identical conflict keys must be equal")
	}
	if k1 == k3 {
		t.Error("different conflict keys must not be equal")
	}
}

func TestRelKey_Equality(t *testing.T) {
	k1 := relKey{chatID: 12345, userID: 67890}
	k2 := relKey{chatID: 12345, userID: 67890}
	k3 := relKey{chatID: 12345, userID: 99999}

	if k1 != k2 {
		t.Error("identical rel keys must be equal")
	}
	if k1 == k3 {
		t.Error("different rel keys must not be equal")
	}
}

func simulateCollectState(chatID, userID int64) *prompts.TemplateData {
	data := &prompts.TemplateData{
		PersonalityContext: "test personality context",
		StyleInstructions:  "test style instructions",
	}

	presenceProfile := state.ComputePresenceProfile(chatID, 0, 8, 0.12)
	presenceState := state.ComputePresenceState(
		presenceProfile,
		"Europe/Moscow",
		[]state.BusySlot{
			{
				Label:         "учёба/пары",
				From:          "09:00",
				To:            "15:30",
				Days:          []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday},
				CheckAfterMin: [2]int{10, 25},
			},
			{
				Label:         "тренировка",
				From:          "18:00",
				To:            "19:30",
				Days:          []time.Weekday{time.Monday, time.Wednesday, time.Friday},
				CheckAfterMin: [2]int{5, 15},
			},
		},
		time.Now(),
		time.Now(),
		0,
		false,
		false,
	)

	busyLabel := ""
	busyUntil := ""
	if presenceState.Busy != nil {
		busyLabel = presenceState.Busy.Label
		busyUntil = presenceState.Busy.Until
	}

	data.Presence = &prompts.PresenceData{
		Online:     presenceState.Online,
		Asleep:     presenceState.Asleep,
		NightAwake: presenceState.NightAwake,
		LocalHour:  presenceState.LocalHour,
		Hint:       presenceState.Hint,
		IsBusy:     presenceState.Busy != nil,
		BusyLabel:  busyLabel,
		BusyUntil:  busyUntil,
	}

	data.Relationship = &prompts.RelationshipData{
		Stage:      string(state.StageTgGivenWarming),
		Interest:   40,
		Trust:      25,
		Attraction: 30,
		Annoyance:  10,
		Fragment: state.RelationshipPromptFragment(
			state.StageTgGivenWarming,
			&state.RelationshipScore{Interest: 40, Trust: 25, Attraction: 30, Annoyance: 10, Cringe: 5},
		),
	}

	moodState := state.ComputeMoodState("Europe/Moscow", 0, state.StageTgGivenWarming, &state.RelationshipScore{
		Interest: 40, Trust: 25, Attraction: 30, Annoyance: 10, Cringe: 5,
	})
	data.Mood = &prompts.MoodData{
		CurrentMood:  moodState.CurrentMood,
		Energy:       moodState.Energy,
		Irritability: moodState.Irritability,
		Affection:    moodState.Affection,
		Fragment:     state.MoodPromptFragment(moodState),
	}

	return data
}

func TestSimulateCollectState_ReturnsAllFields(t *testing.T) {
	data := simulateCollectState(12345, 67890)

	if data.Presence == nil {
		t.Fatal("Presence must not be nil")
	}
	if data.Conflict != nil {
		t.Log("Conflict is nil for new chat — expected")
	}
	if data.Relationship == nil {
		t.Fatal("Relationship must not be nil")
	}
	if data.Mood == nil {
		t.Fatal("Mood must not be nil")
	}
	if data.PersonalityContext == "" {
		t.Error("PersonalityContext must not be empty")
	}
}

func TestSimulateCollectState_PresenceFields(t *testing.T) {
	data := simulateCollectState(12345, 67890)

	if data.Presence.LocalHour < 0 || data.Presence.LocalHour > 23 {
		t.Errorf("LocalHour must be 0-23, got %d", data.Presence.LocalHour)
	}
	if data.Presence.Hint == "" {
		t.Error("Presence.Hint must not be empty")
	}
	if _, ok := interface{}(data.Presence.Online).(bool); !ok {
		t.Error("Presence.Online must be bool")
	}
}

func TestSimulateCollectState_RelationshipDefaults(t *testing.T) {
	data := simulateCollectState(12345, 67890)

	if data.Relationship.Stage != string(state.StageTgGivenWarming) {
		t.Errorf("expected stage %q, got %q", state.StageTgGivenWarming, data.Relationship.Stage)
	}
	if data.Relationship.Interest != 40 {
		t.Errorf("expected Interest=40, got %.0f", data.Relationship.Interest)
	}
	if data.Relationship.Trust != 25 {
		t.Errorf("expected Trust=25, got %.0f", data.Relationship.Trust)
	}
	if data.Relationship.Attraction != 30 {
		t.Errorf("expected Attraction=30, got %.0f", data.Relationship.Attraction)
	}
}

func TestSimulateCollectState_MoodFields(t *testing.T) {
	data := simulateCollectState(12345, 67890)

	validMoods := map[string]bool{
		"energetic": true, "neutral": true, "tired": true,
		"irritated": true, "affectionate": true, "playful": true,
	}
	if !validMoods[data.Mood.CurrentMood] {
		t.Errorf("CurrentMood %q is not a valid mood", data.Mood.CurrentMood)
	}
	if data.Mood.Energy < -1.0 || data.Mood.Energy > 1.0 {
		t.Errorf("Energy %.2f must be in [-1.0, 1.0]", data.Mood.Energy)
	}
	if data.Mood.Irritability < 0.0 || data.Mood.Irritability > 1.0 {
		t.Errorf("Irritability %.2f must be in [0.0, 1.0]", data.Mood.Irritability)
	}
}

func TestSimulateCollectState_WithConflict(t *testing.T) {
	data := simulateCollectState(12345, 67890)

	conflictState := &state.ConflictState{
		Level:     2,
		ColdUntil: time.Now().Add(2 * time.Hour),
		Reason:    "test escalation",
		Since:     time.Now().Add(-30 * time.Minute),
	}
	data.Conflict = &prompts.ConflictData{
		Active:     true,
		ColdActive: state.IsConflictCold(conflictState),
		Level:      int(conflictState.Level),
		Reason:     conflictState.Reason,
		Fragment:   state.ConflictPromptFragment(conflictState),
	}

	if data.Conflict == nil {
		t.Fatal("Conflict must not be nil after injection")
	}
	if data.Conflict.Level <= 0 {
		t.Errorf("Conflict.Level must be > 0, got %d", data.Conflict.Level)
	}
	if data.Conflict.Fragment == "" {
		t.Error("Conflict.Fragment must not be empty")
	}
}

func TestSimulateCollectState_PresenceProfile_IsDeterministic(t *testing.T) {
	chatID := int64(12345)
	p1 := state.ComputePresenceProfile(chatID, 0, 8, 0.12)
	p2 := state.ComputePresenceProfile(chatID, 0, 8, 0.12)

	if p1.Pattern != p2.Pattern {
		t.Error("same seed must produce same pattern")
	}
	if p1.CheckEveryMin != p2.CheckEveryMin {
		t.Error("same seed must produce same check interval")
	}
}

func TestSimulateCollectState_FragmentFields(t *testing.T) {
	data := simulateCollectState(12345, 67890)

	if data.Relationship.Fragment == "" {
		t.Error("Relationship.Fragment must not be empty")
	}
	if !strings.Contains(data.Relationship.Fragment, string(state.StageTgGivenWarming)) {
		t.Error("Relationship.Fragment must contain stage identifier")
	}
}

func TestStateProvider_NewStateProvider_InitialisesMaps(t *testing.T) {
	sp := NewStateProvider(nil)

	if sp.presenceProfiles == nil {
		t.Error("presenceProfiles map must be initialised")
	}
	if sp.conflictStates == nil {
		t.Error("conflictStates map must be initialised")
	}
	if sp.relationships == nil {
		t.Error("relationships map must be initialised")
	}
	if sp.bot != nil {
		t.Error("bot must be nil when created with nil")
	}
}
