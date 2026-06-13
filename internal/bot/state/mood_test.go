package state

import (
	"strings"
	"testing"
)

func TestComputeMoodState_EnergyByHour(t *testing.T) {
	score := &RelationshipScore{Annoyance: 0}

	tests := []struct {
		hour       int
		wantSign   int // 1=positive, -1=negative, 0=zero
	}{
		{6, 0},   // sin(0) = 0
		{12, 1},  // sin(pi/2) = 1 → peak pos
		{18, 0},  // sin(pi) = 0
		{0, -1},  // sin(-pi/2) = -1 → peak neg
		{3, -1},  // night, negative
		{9, 1},   // morning, climbing positive
		{15, 1},  // afternoon, positive
		{21, -1}, // evening, energy goes negative after 18
	}

	for _, tt := range tests {
		result := computeMoodState(tt.hour, 0, StageConvinced, score)
		switch tt.wantSign {
		case 1:
			if result.Energy <= 0 {
				t.Errorf("hour=%d: expected positive energy, got %.3f", tt.hour, result.Energy)
			}
		case -1:
			if result.Energy >= 0 {
				t.Errorf("hour=%d: expected negative energy, got %.3f", tt.hour, result.Energy)
			}
		case 0:
			if result.Energy > 0.01 || result.Energy < -0.01 {
				t.Errorf("hour=%d: expected near-zero energy, got %.3f", tt.hour, result.Energy)
			}
		}
	}
}

func TestComputeMoodState_EnergyRange(t *testing.T) {
	score := &RelationshipScore{}
	for hour := 0; hour < 24; hour++ {
		result := computeMoodState(hour, 0, StageConvinced, score)
		if result.Energy < -0.5 || result.Energy > 0.5 {
			t.Errorf("hour=%d: energy %.3f out of expected range [-0.5, 0.5]", hour, result.Energy)
		}
	}
}

func TestComputeMoodState_NightTime(t *testing.T) {
	score := &RelationshipScore{}

	nightHours := []int{22, 23, 0, 1, 2, 3, 4, 5}
	for _, h := range nightHours {
		result := computeMoodState(h, 0, StageConvinced, score)
		if !result.IsNightTime {
			t.Errorf("hour=%d: expected IsNightTime=true", h)
		}
	}

	dayHours := []int{6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21}
	for _, h := range dayHours {
		result := computeMoodState(h, 0, StageConvinced, score)
		if result.IsNightTime {
			t.Errorf("hour=%d: expected IsNightTime=false", h)
		}
	}
}

func TestComputeMoodState_IrritabilityGrowsWithConflict(t *testing.T) {
	score := &RelationshipScore{Annoyance: 0}

	var prev float64
	for level := 0; level <= 4; level++ {
		result := computeMoodState(12, level, StageConvinced, score)
		if level > 0 && result.Irritability <= prev {
			t.Errorf("level=%d: irritability %.3f not greater than previous %.3f", level, result.Irritability, prev)
		}
		prev = result.Irritability
	}

	resultNoConflict := computeMoodState(12, 0, StageConvinced, score)
	resultHighConflict := computeMoodState(12, 4, StageConvinced, score)
	if resultHighConflict.Irritability <= resultNoConflict.Irritability+0.4 {
		t.Errorf("irritability gap too small: base=%.3f, conflict4=%.3f", resultNoConflict.Irritability, resultHighConflict.Irritability)
	}
}

func TestComputeMoodState_IrritabilityHighAnnoyance(t *testing.T) {
	lowAnnoyance := &RelationshipScore{Annoyance: 10}
	highAnnoyance := &RelationshipScore{Annoyance: 80}

	low := computeMoodState(12, 0, StageConvinced, lowAnnoyance)
	high := computeMoodState(12, 0, StageConvinced, highAnnoyance)

	if high.Irritability <= low.Irritability {
		t.Errorf("expected higher irritability with high annoyance: low=%.3f, high=%.3f", low.Irritability, high.Irritability)
	}
}

func TestComputeMoodState_AffectionGrowsWithStage(t *testing.T) {
	score := &RelationshipScore{}

	early := computeMoodState(12, 0, StageMetIrlGotTg, score)
	convinced := computeMoodState(12, 0, StageConvinced, score)
	later := computeMoodState(12, 0, StageLongTerm, score)

	if convinced.Affection <= early.Affection {
		t.Errorf("convinced affection %.3f should be > early %.3f", convinced.Affection, early.Affection)
	}
	if later.Affection <= convinced.Affection {
		t.Errorf("later affection %.3f should be > convinced %.3f", later.Affection, later.Affection)
	}
}

func TestComputeMoodState_AffectionDropsWithConflict(t *testing.T) {
	score := &RelationshipScore{}

	noConflict := computeMoodState(12, 0, StageConvinced, score)
	withConflict := computeMoodState(12, 4, StageConvinced, score)

	if withConflict.Affection >= noConflict.Affection {
		t.Errorf("affection should drop with conflict: no=%.3f, with=%.3f", noConflict.Affection, withConflict.Affection)
	}
}

func TestComputeMoodState_AffectionEnergyBoost(t *testing.T) {
	score := &RelationshipScore{}

	lowEnergy := computeMoodState(0, 0, StageConvinced, score)
	highEnergy := computeMoodState(12, 0, StageConvinced, score)

	if highEnergy.Affection != lowEnergy.Affection+0.05 {
		t.Errorf("energy boost should be +0.05: low=%.3f, high=%.3f", lowEnergy.Affection, highEnergy.Affection)
	}
}

func TestComputeMoodState_ClampRanges(t *testing.T) {
	score := &RelationshipScore{Annoyance: 100}

	for hour := 0; hour < 24; hour++ {
		for level := 0; level <= 4; level++ {
			for _, stage := range []StageId{StageMetIrlGotTg, StageConvinced, StageLongTerm, StageDumped} {
				result := computeMoodState(hour, level, stage, score)
				if result.Irritability < 0 || result.Irritability > 1 {
					t.Errorf("irritability out of range: %.3f", result.Irritability)
				}
				if result.Affection < 0 || result.Affection > 1 {
					t.Errorf("affection out of range: %.3f", result.Affection)
				}
			}
		}
	}
}

func TestComputeMoodState_CurrentMood(t *testing.T) {
	score := &RelationshipScore{}

	tests := []struct {
		hour     int
		conflict int
		stage    StageId
		wantMood string
	}{
		{12, 3, StageMetIrlGotTg, "irritated"},
		{12, 4, StageMetIrlGotTg, "irritated"},
		{12, 0, StageMetIrlGotTg, "energetic"},
		{8, 0, StageMetIrlGotTg, "playful"},
		{19, 0, StageMetIrlGotTg, "neutral"},
		{3, 0, StageMetIrlGotTg, "tired"},
	}

	for _, tt := range tests {
		result := computeMoodState(tt.hour, tt.conflict, tt.stage, score)
		if result.CurrentMood != tt.wantMood {
			t.Errorf("hour=%d conflict=%d stage=%s: expected mood=%s, got %s (energy=%.3f, affection=%.3f)",
				tt.hour, tt.conflict, tt.stage, tt.wantMood, result.CurrentMood, result.Energy, result.Affection)
		}
	}

	// affectionate requires high energy (>0.3) AND high affection (>0.6)
	affectionateResult := computeMoodState(12, 0, StageLongTerm, score)
	if affectionateResult.Energy > 0.3 && affectionateResult.Affection > 0.6 {
		if affectionateResult.CurrentMood != "affectionate" {
			t.Errorf("expected affectionate when energy>0.3 and affection>0.6, got %s (energy=%.3f, affection=%.3f)",
				affectionateResult.CurrentMood, affectionateResult.Energy, affectionateResult.Affection)
		}
	}
}

func TestComputeMoodState_NilScore(t *testing.T) {
	result := computeMoodState(12, 0, StageConvinced, nil)
	if result.Irritability < 0 || result.Irritability > 1 {
		t.Errorf("irritability with nil score out of range: %.3f", result.Irritability)
	}
	if result.Affection < 0 || result.Affection > 1 {
		t.Errorf("affection with nil score out of range: %.3f", result.Affection)
	}
}

func TestMoodPromptFragment_Basic(t *testing.T) {
	m := &MoodState{
		CurrentMood:   "playful",
		Energy:        0.15,
		Irritability:  0.15,
		Affection:     0.5,
		ConflictLevel: 0,
		LocalHour:     14,
		IsNightTime:   false,
	}

	fragment := MoodPromptFragment(m)
	if !strings.Contains(fragment, "playful") {
		t.Errorf("expected 'playful' in fragment: %s", fragment)
	}
	if !strings.Contains(fragment, "средняя") {
		t.Errorf("expected 'средняя' energy in fragment: %s", fragment)
	}
}

func TestMoodPromptFragment_HighEnergy(t *testing.T) {
	m := &MoodState{
		CurrentMood:   "energetic",
		Energy:        0.4,
		Irritability:  0.1,
		Affection:     0.5,
		ConflictLevel: 0,
		LocalHour:     14,
		IsNightTime:   false,
	}

	fragment := MoodPromptFragment(m)
	if !strings.Contains(fragment, "высокая") && !strings.Contains(fragment, "высокая") {
		t.Errorf("expected high energy label: %s", fragment)
	}
}

func TestMoodPromptFragment_NightTime(t *testing.T) {
	m := &MoodState{
		CurrentMood:   "tired",
		Energy:        -0.4,
		Irritability:  0.4,
		Affection:     0.4,
		ConflictLevel: 0,
		LocalHour:     2,
		IsNightTime:   true,
	}

	fragment := MoodPromptFragment(m)
	if !strings.Contains(fragment, "Сейчас ночь") {
		t.Errorf("expected night mention: %s", fragment)
	}
	if !strings.Contains(fragment, "уставшая") {
		t.Errorf("expected tired mention: %s", fragment)
	}
}

func TestMoodPromptFragment_Conflict(t *testing.T) {
	m := &MoodState{
		CurrentMood:   "irritated",
		Energy:        -0.1,
		Irritability:  0.8,
		Affection:     0.3,
		ConflictLevel: 3,
		LocalHour:     14,
		IsNightTime:   false,
	}

	fragment := MoodPromptFragment(m)
	if !strings.Contains(fragment, "конфликта") {
		t.Errorf("expected conflict mention: %s", fragment)
	}
	if !strings.Contains(fragment, "level 3") {
		t.Errorf("expected conflict level 3: %s", fragment)
	}
	if !strings.Contains(fragment, "холоднее") {
		t.Errorf("expected cold instruction: %s", fragment)
	}
}

func TestMoodPromptFragment_ConflictAndNight(t *testing.T) {
	m := &MoodState{
		CurrentMood:   "irritated",
		Energy:        -0.5,
		Irritability:  0.9,
		Affection:     0.2,
		ConflictLevel: 4,
		LocalHour:     23,
		IsNightTime:   true,
	}

	fragment := MoodPromptFragment(m)
	if !strings.Contains(fragment, "конфликта") {
		t.Errorf("expected conflict mention: %s", fragment)
	}
	if !strings.Contains(fragment, "Сейчас ночь") {
		t.Errorf("expected night mention: %s", fragment)
	}
}

func TestMoodPromptFragment_Nil(t *testing.T) {
	fragment := MoodPromptFragment(nil)
	if fragment != "" {
		t.Errorf("expected empty fragment for nil, got: %s", fragment)
	}
}

func TestComputeMoodState_LocalHour(t *testing.T) {
	result := ComputeMoodState("UTC", 0, StageConvinced, &RelationshipScore{})
	utcHour := result.LocalHour

	// LocalHour should be set (0-23)
	if utcHour < 0 || utcHour > 23 {
		t.Errorf("LocalHour out of range: %d", utcHour)
	}

	// Energy should be within expected range
	if result.Energy < -1 || result.Energy > 1 {
		t.Errorf("Energy out of range: %.3f", result.Energy)
	}
}

func TestComputeMoodState_InvalidTimezone(t *testing.T) {
	result := ComputeMoodState("Nonsense/Zone", 0, StageConvinced, &RelationshipScore{})
	if result.LocalHour < 0 || result.LocalHour > 23 {
		t.Errorf("LocalHour out of range with invalid tz: %d", result.LocalHour)
	}
}
