package state

import (
	"strings"
	"testing"
	"time"
)

func TestEscalateConflict_HighAnnoyance(t *testing.T) {
	current := &ConflictState{
		Level:   ConflictNone,
		History: []ConflictEvent{},
	}

	// annoyance > 70 → level 3, but also check trigger threshold
	score := &RelationshipScore{
		Annoyance: 75,
		Cringe:    10,
		Interest:  20,
	}

	result := EscalateConflict(current, score, "test message")
	if result.Level != ConflictSerious {
		t.Errorf("expected level %d, got %d", ConflictSerious, result.Level)
	}
	if result.Reason != "сильный негатив" {
		t.Errorf("expected reason 'сильный негатив', got '%s'", result.Reason)
	}
	if result.ColdUntil.IsZero() {
		t.Error("expected non-zero ColdUntil")
	}
	if result.Since.IsZero() {
		t.Error("expected non-zero Since")
	}
	if len(result.History) != 1 {
		t.Errorf("expected 1 history event, got %d", len(result.History))
	}
}

func TestEscalateConflict_MediumTrigger(t *testing.T) {
	// trigger = 30 + 10 - 20 = 20 → >= 15 → level 2
	current := &ConflictState{
		Level:   ConflictNone,
		History: []ConflictEvent{},
	}
	score := &RelationshipScore{
		Annoyance: 30,
		Cringe:    10,
		Interest:  20,
	}

	result := EscalateConflict(current, score, "user annoyed me")
	if result.Level != ConflictUpset {
		t.Errorf("expected level %d, got %d", ConflictUpset, result.Level)
	}
	if result.Reason != "обижена" {
		t.Errorf("expected reason 'обижена', got '%s'", result.Reason)
	}
}

func TestEscalateConflict_MildTrigger(t *testing.T) {
	// trigger = 15 + 5 - 10 = 10 → >= 8 → level 1
	current := &ConflictState{
		Level:   ConflictNone,
		History: []ConflictEvent{},
	}
	score := &RelationshipScore{
		Annoyance: 15,
		Cringe:    5,
		Interest:  10,
	}

	result := EscalateConflict(current, score, "slightly annoying")
	if result.Level != ConflictMild {
		t.Errorf("expected level %d, got %d", ConflictMild, result.Level)
	}
	if result.Reason != "немного дуется" {
		t.Errorf("expected reason 'немного дуется', got '%s'", result.Reason)
	}
}

func TestEscalateConflict_Breakup(t *testing.T) {
	// annoyance > 85, cringe > 70, interest < -30 → level 4
	current := &ConflictState{
		Level:   ConflictNone,
		History: []ConflictEvent{},
	}
	score := &RelationshipScore{
		Annoyance: 90,
		Cringe:    80,
		Interest:  -50,
	}

	result := EscalateConflict(current, score, "you are the worst")
	if result.Level != ConflictBreakup {
		t.Errorf("expected level %d, got %d", ConflictBreakup, result.Level)
	}
	if result.Reason != "на грани разрыва" {
		t.Errorf("expected reason 'на грани разрыва', got '%s'", result.Reason)
	}
}

func TestEscalateConflict_LevelDoesNotExceed(t *testing.T) {
	// Already at level 3, trigger for level 1 should not lower it
	current := &ConflictState{
		Level:   ConflictSerious,
		History: []ConflictEvent{},
	}
	score := &RelationshipScore{
		Annoyance: 15,
		Cringe:    5,
		Interest:  10,
	}

	result := EscalateConflict(current, score, "minor thing")
	if result.Level != ConflictSerious {
		t.Errorf("expected level %d (unchanged), got %d", ConflictSerious, result.Level)
	}
}

func TestEscalateConflict_NoEscalation(t *testing.T) {
	current := &ConflictState{
		Level:   ConflictNone,
		History: []ConflictEvent{},
	}
	score := &RelationshipScore{
		Annoyance: 5,
		Cringe:    2,
		Interest:  50,
	}

	result := EscalateConflict(current, score, "nice message")
	if result != current {
		t.Error("expected same pointer returned when no escalation")
	}
	if result.Level != ConflictNone {
		t.Errorf("expected level %d, got %d", ConflictNone, result.Level)
	}
}

func TestSoftenConflict_ReducesLevel(t *testing.T) {
	current := &ConflictState{
		Level:     ConflictSerious,
		ColdUntil: time.Now().Add(24 * time.Hour),
		Reason:    "сильный негатив",
		History:   []ConflictEvent{},
	}

	result := SoftenConflict(current, 5, 5, 5) // total = 15 >= 12
	if result.Level != ConflictUpset {
		t.Errorf("expected level %d, got %d", ConflictUpset, result.Level)
	}
	if len(result.History) != 1 {
		t.Errorf("expected 1 history event, got %d", len(result.History))
	}
	// ColdUntil should be reduced (halved), not fully reset
	if result.ColdUntil.IsZero() {
		t.Error("expected ColdUntil to still be set after partial soften")
	}
}

func TestSoftenConflict_ResetsAtZero(t *testing.T) {
	current := &ConflictState{
		Level:     ConflictMild,
		ColdUntil: time.Now().Add(-1 * time.Hour), // expired
		Reason:    "немного дуется",
		Since:     time.Now().Add(-2 * time.Hour),
		History:   []ConflictEvent{},
	}

	result := SoftenConflict(current, 10, 10, 10) // total = 30 >= 12
	if result.Level != ConflictNone {
		t.Errorf("expected level %d, got %d", ConflictNone, result.Level)
	}
	if !result.ColdUntil.IsZero() {
		t.Error("expected ColdUntil to be zeroed")
	}
	if !result.Since.IsZero() {
		t.Error("expected Since to be zeroed")
	}
	if result.Reason != "" {
		t.Errorf("expected empty reason, got '%s'", result.Reason)
	}
}

func TestSoftenConflict_Insufficient(t *testing.T) {
	current := &ConflictState{
		Level:   ConflictSerious,
		History: []ConflictEvent{},
	}

	result := SoftenConflict(current, 3, 3, 3) // total = 9 < 12
	if result != current {
		t.Error("expected same pointer returned when soften insufficient")
	}
	if result.Level != ConflictSerious {
		t.Errorf("expected level %d, got %d", ConflictSerious, result.Level)
	}
}

func TestSoftenConflict_NoConflict(t *testing.T) {
	current := &ConflictState{
		Level:   ConflictNone,
		History: []ConflictEvent{},
	}

	result := SoftenConflict(current, 10, 10, 10)
	if result != current {
		t.Error("expected same pointer returned when no conflict")
	}
}

func TestIsConflictCold_Active(t *testing.T) {
	c := &ConflictState{
		Level:     ConflictSerious,
		ColdUntil: time.Now().Add(5 * time.Hour),
	}

	if !IsConflictCold(c) {
		t.Error("expected cold to be active")
	}
}

func TestIsConflictCold_Expired(t *testing.T) {
	c := &ConflictState{
		Level:     ConflictSerious,
		ColdUntil: time.Now().Add(-1 * time.Hour),
	}

	if IsConflictCold(c) {
		t.Error("expected cold to be inactive (expired)")
	}
}

func TestIsConflictCold_NoConflict(t *testing.T) {
	c := &ConflictState{
		Level:     ConflictNone,
		ColdUntil: time.Now().Add(5 * time.Hour),
	}

	if IsConflictCold(c) {
		t.Error("expected cold inactive when no conflict")
	}
}

func TestIsConflictCold_ZeroColdUntil(t *testing.T) {
	c := &ConflictState{
		Level: ConflictSerious,
	}

	if IsConflictCold(c) {
		t.Error("expected cold inactive when ColdUntil is zero")
	}
}

func TestConflictPromptFragment_ActiveCold(t *testing.T) {
	c := &ConflictState{
		Level:     ConflictSerious,
		ColdUntil: time.Now().Add(5 * time.Hour),
		Reason:    "сильный негатив",
		Since:     time.Now(),
	}

	fragment := ConflictPromptFragment(c)
	if fragment == "" {
		t.Error("expected non-empty prompt fragment")
	}
	if !strings.Contains(fragment, "# КОНФЛИКТ (level 3)") {
		t.Error("expected conflict header")
	}
	if !strings.Contains(fragment, "режиме молчания") {
		t.Error("expected silence mode mention")
	}
	if !strings.Contains(fragment, "сильный негатив") {
		t.Error("expected reason in fragment")
	}
}

func TestConflictPromptFragment_ExpiredCold(t *testing.T) {
	c := &ConflictState{
		Level:     ConflictMild,
		ColdUntil: time.Now().Add(-1 * time.Hour), // expired
		Reason:    "немного дуется",
	}

	fragment := ConflictPromptFragment(c)
	if fragment == "" {
		t.Error("expected non-empty prompt fragment")
	}
	if !strings.Contains(fragment, "осадочек остался") {
		t.Error("expected 'осадочек' mention for expired cold")
	}
}

func TestConflictPromptFragment_NoConflict(t *testing.T) {
	c := &ConflictState{
		Level: ConflictNone,
	}

	fragment := ConflictPromptFragment(c)
	if fragment != "" {
		t.Error("expected empty fragment when no conflict")
	}
}

func TestConflictPromptFragment_MildInstructions(t *testing.T) {
	c := &ConflictState{
		Level:     ConflictMild,
		ColdUntil: time.Now().Add(1 * time.Hour),
		Reason:    "немного дуется",
	}

	fragment := ConflictPromptFragment(c)
	if strings.Contains(fragment, "очень редко, сухо") {
		t.Error("mild conflict should not have serious instructions")
	}
	if !strings.Contains(fragment, "сухо, односложно") {
		t.Error("expected mild dry response instructions")
	}
}
