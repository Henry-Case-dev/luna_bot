package bot

import (
	"sync"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/bot/prompts"
	"github.com/Henry-Case-dev/luna_bot/internal/bot/state"
)

type relCacheEntry struct {
	stage state.StageId
	score *state.RelationshipScore
}

type conflictKey struct {
	chatID int64
	userID int64
}

type relKey struct {
	chatID int64
	userID int64
}

// StateProvider collects state from all state machines and forms TemplateData for prompt rendering.
type StateProvider struct {
	bot *Bot

	mu               sync.RWMutex
	presenceProfiles map[int64]*state.PresenceProfile
	conflictStates   map[conflictKey]*state.ConflictState
	relationships    map[relKey]*relCacheEntry
}

func NewStateProvider(bot *Bot) *StateProvider {
	return &StateProvider{
		bot:              bot,
		presenceProfiles: make(map[int64]*state.PresenceProfile),
		conflictStates:   make(map[conflictKey]*state.ConflictState),
		relationships:    make(map[relKey]*relCacheEntry),
	}
}

// CollectState gathers current bot state for the given chat and target user.
// Returns TemplateData ready for prompt rendering.
func (sp *StateProvider) CollectState(chatID int64, targetUserID int64) *prompts.TemplateData {
	data := &prompts.TemplateData{}

	// Base personality context and style
	pc, err := sp.bot.getPersonalityContext(chatID, "state_provider")
	if err != nil {
		pc = ""
	}
	data.PersonalityContext = pc
	data.StyleInstructions = sp.bot.getStyleInstructions()

	// Presence State
	profile := sp.getOrComputePresenceProfile(chatID)

	tz := sp.bot.config.TimeZone
	if tz == "" {
		tz = "Europe/Moscow"
	}

	busySlots := sp.defaultBusySlots()
	conflictCold := false
	if cs := sp.getOrLoadConflict(chatID, targetUserID); cs != nil {
		conflictCold = state.IsConflictCold(cs)
	}

	presenceState := state.ComputePresenceState(
		profile,
		tz,
		busySlots,
		time.Now(),      // lastUserMsgTs
		time.Now(),      // lastHerReplyTs
		0,               // recentExchangeCount
		false,           // forcedWake
		conflictCold,
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

	// Conflict State
	conflictState := sp.getOrLoadConflict(chatID, targetUserID)
	if conflictState != nil && conflictState.Level > 0 {
		data.Conflict = &prompts.ConflictData{
			Active:     true,
			ColdActive: state.IsConflictCold(conflictState),
			Level:      int(conflictState.Level),
			Reason:     conflictState.Reason,
			Fragment:   state.ConflictPromptFragment(conflictState),
		}
	}

	// Relationship State
	stage, score := sp.getRelationship(chatID, targetUserID)
	data.Relationship = &prompts.RelationshipData{
		Stage:      string(stage),
		Interest:   score.Interest,
		Trust:      score.Trust,
		Attraction: score.Attraction,
		Annoyance:  score.Annoyance,
		Fragment:   state.RelationshipPromptFragment(stage, score),
	}

	// Mood State
	conflictLevel := 0
	if conflictState != nil {
		conflictLevel = int(conflictState.Level)
	}
	moodState := state.ComputeMoodState(tz, conflictLevel, stage, score)

	if emoState, err := sp.bot.storage.GetEmotionalState(chatID); err == nil && emoState != nil {
		moodState.EnrichWithEmotionalState(emoState)
	}

	data.Mood = &prompts.MoodData{
		CurrentMood:  moodState.CurrentMood,
		Energy:       moodState.Energy,
		Irritability: moodState.Irritability,
		Affection:    moodState.Affection,
		Fragment:     state.MoodPromptFragment(moodState),
	}

	return data
}

func (sp *StateProvider) getOrComputePresenceProfile(chatID int64) *state.PresenceProfile {
	sp.mu.RLock()
	if p, ok := sp.presenceProfiles[chatID]; ok {
		sp.mu.RUnlock()
		return p
	}
	sp.mu.RUnlock()

	seed := chatID
	if seed < 0 {
		seed = -seed
	}

	profile := state.ComputePresenceProfile(seed, 0, 8, 0.12)

	sp.mu.Lock()
	sp.presenceProfiles[chatID] = profile
	sp.mu.Unlock()

	return profile
}

func (sp *StateProvider) getOrLoadConflict(chatID int64, targetUserID int64) *state.ConflictState {
	key := conflictKey{chatID: chatID, userID: targetUserID}
	sp.mu.RLock()
	cs, ok := sp.conflictStates[key]
	sp.mu.RUnlock()
	if ok {
		return cs
	}
	return nil
}

func (sp *StateProvider) getRelationship(chatID int64, targetUserID int64) (state.StageId, *state.RelationshipScore) {
	key := relKey{chatID: chatID, userID: targetUserID}
	sp.mu.RLock()
	entry, ok := sp.relationships[key]
	sp.mu.RUnlock()
	if ok {
		return entry.stage, entry.score
	}

	defaultStage := state.StageTgGivenWarming
	defaultScore := &state.RelationshipScore{
		Interest:   40,
		Trust:      25,
		Attraction: 30,
		Annoyance:  10,
		Cringe:     5,
	}

	sp.mu.Lock()
	sp.relationships[key] = &relCacheEntry{stage: defaultStage, score: defaultScore}
	sp.mu.Unlock()

	return defaultStage, defaultScore
}

func (sp *StateProvider) defaultBusySlots() []state.BusySlot {
	return []state.BusySlot{
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
	}
}
