package state

import (
	"strings"
	"testing"
)

func TestDecideStageTransition_Upgrade_MetIrlToCold(t *testing.T) {
	// met-irl-got-tg → tg-given-cold при хороших скорах (следующая в StageOrder)
	ctx := &StageTransitionContext{
		CurrentStage:      StageMetIrlGotTg,
		HerMessagesInStage: 6,
		Score: &RelationshipScore{
			Interest:   40,
			Trust:      20,
			Attraction: 25,
			Annoyance:  5,
		},
	}

	result := DecideStageTransition(ctx)
	if result == nil {
		t.Fatal("expected upgrade from met-irl-got-tg")
	}
	if result.Direction != "up" {
		t.Errorf("expected direction 'up', got '%s'", result.Direction)
	}
	if result.Next != StageTgGivenCold {
		t.Errorf("expected next stage %s, got %s", StageTgGivenCold, result.Next)
	}
	if !strings.Contains(result.Reason, "оттаяла") {
		t.Errorf("expected reason to contain 'оттаяла', got '%s'", result.Reason)
	}
}

func TestDecideStageTransition_Upgrade_TgGivenCold(t *testing.T) {
	// tg-given-cold → tg-given-warming
	ctx := &StageTransitionContext{
		CurrentStage:      StageTgGivenCold,
		HerMessagesInStage: 7,
		Score: &RelationshipScore{
			Interest:   30,
			Trust:      15,
			Attraction: 10,
			Annoyance:  10,
		},
	}

	result := DecideStageTransition(ctx)
	if result == nil {
		t.Fatal("expected upgrade from tg-given-cold")
	}
	if result.Direction != "up" {
		t.Errorf("expected direction 'up', got '%s'", result.Direction)
	}
	if result.Next != StageTgGivenWarming {
		t.Errorf("expected next stage %s, got %s", StageTgGivenWarming, result.Next)
	}
}

func TestDecideStageTransition_Upgrade_TgGivenWarming(t *testing.T) {
	ctx := &StageTransitionContext{
		CurrentStage:      StageTgGivenWarming,
		HerMessagesInStage: 8,
		Score: &RelationshipScore{
			Interest:   45,
			Trust:      30,
			Attraction: 35,
			Annoyance:  5,
		},
	}

	result := DecideStageTransition(ctx)
	if result == nil {
		t.Fatal("expected upgrade from tg-given-warming")
	}
	if result.Next != StageConvinced {
		t.Errorf("expected next stage %s, got %s", StageConvinced, result.Next)
	}
}

func TestDecideStageTransition_Upgrade_Convinced(t *testing.T) {
	ctx := &StageTransitionContext{
		CurrentStage:      StageConvinced,
		HerMessagesInStage: 12,
		Score: &RelationshipScore{
			Interest:   55,
			Trust:      40,
			Attraction: 55,
			Annoyance:  5,
		},
	}

	result := DecideStageTransition(ctx)
	if result == nil {
		t.Fatal("expected upgrade from convinced")
	}
	if result.Next != StageFirstDateDone {
		t.Errorf("expected next stage %s, got %s", StageFirstDateDone, result.Next)
	}
}

func TestDecideStageTransition_Upgrade_FirstDateDone(t *testing.T) {
	ctx := &StageTransitionContext{
		CurrentStage:      StageFirstDateDone,
		HerMessagesInStage: 15,
		Score: &RelationshipScore{
			Interest:   65,
			Trust:      55,
			Attraction: 70,
			Annoyance:  5,
		},
	}

	result := DecideStageTransition(ctx)
	if result == nil {
		t.Fatal("expected upgrade from first-date-done")
	}
	if result.Next != StageDatingEarly {
		t.Errorf("expected next stage %s, got %s", StageDatingEarly, result.Next)
	}
}

func TestDecideStageTransition_Upgrade_DatingEarly(t *testing.T) {
	ctx := &StageTransitionContext{
		CurrentStage:      StageDatingEarly,
		HerMessagesInStage: 30,
		Score: &RelationshipScore{
			Interest:   80,
			Trust:      75,
			Attraction: 70,
			Annoyance:  5,
		},
	}

	result := DecideStageTransition(ctx)
	if result == nil {
		t.Fatal("expected upgrade from dating-early")
	}
	if result.Next != StageDatingStable {
		t.Errorf("expected next stage %s, got %s", StageDatingStable, result.Next)
	}
}

func TestDecideStageTransition_Upgrade_DatingStable(t *testing.T) {
	ctx := &StageTransitionContext{
		CurrentStage:      StageDatingStable,
		HerMessagesInStage: 65,
		Score: &RelationshipScore{
			Interest:   60,
			Trust:      85,
			Attraction: 70,
			Annoyance:  5,
		},
	}

	result := DecideStageTransition(ctx)
	if result == nil {
		t.Fatal("expected upgrade from dating-stable")
	}
	if result.Next != StageLongTerm {
		t.Errorf("expected next stage %s, got %s", StageLongTerm, result.Next)
	}
}

func TestDecideStageTransition_NoUpgrade_BadScores(t *testing.T) {
	// met-irl-got-tg с плохими скорами — не должно быть перехода
	ctx := &StageTransitionContext{
		CurrentStage:      StageMetIrlGotTg,
		HerMessagesInStage: 10,
		Score: &RelationshipScore{
			Interest:   5,
			Trust:      2,
			Attraction: 5,
			Annoyance:  30,
		},
	}

	result := DecideStageTransition(ctx)
	if result != nil {
		t.Errorf("expected no transition with bad scores, got %+v", result)
	}
}

func TestDecideStageTransition_NoUpgrade_InsufficientMessages(t *testing.T) {
	// met-irl-got-tg с хорошими скорами, но < 6 её сообщений
	ctx := &StageTransitionContext{
		CurrentStage:      StageMetIrlGotTg,
		HerMessagesInStage: 3,
		Score: &RelationshipScore{
			Interest:   50,
			Trust:      30,
			Attraction: 35,
			Annoyance:  5,
		},
	}

	result := DecideStageTransition(ctx)
	if result != nil {
		t.Errorf("expected no transition with < 6 messages, got %+v", result)
	}
}

func TestDecideStageTransition_Downgrade_HighAnnoyance(t *testing.T) {
	// dating-early с высоким annoyance должно понизить
	ctx := &StageTransitionContext{
		CurrentStage:      StageDatingEarly,
		HerMessagesInStage: 10,
		Score: &RelationshipScore{
			Interest:   -20,
			Trust:      5,
			Attraction: 30,
			Annoyance:  70,
		},
	}

	result := DecideStageTransition(ctx)
	if result == nil {
		t.Fatal("expected downgrade with high annoyance")
	}
	if result.Direction != "down" {
		t.Errorf("expected direction 'down', got '%s'", result.Direction)
	}
	if result.Next != StageFirstDateDone {
		t.Errorf("expected downgrade to %s, got %s", StageFirstDateDone, result.Next)
	}
}

func TestDecideStageTransition_Downgrade_Ignores(t *testing.T) {
	// convinced стадия с кучей игноров — понижение
	ctx := &StageTransitionContext{
		CurrentStage:      StageConvinced,
		HerMessagesInStage: 8,
		HisMessagesInStage: 15,
		IgnoresInStage:    12,
		Score: &RelationshipScore{
			Interest:   10,
			Trust:      30,
			Attraction: 40,
			Annoyance:  20,
		},
	}

	result := DecideStageTransition(ctx)
	if result == nil {
		t.Fatal("expected downgrade due to ignores")
	}
	if result.Direction != "down" {
		t.Errorf("expected direction 'down', got '%s'", result.Direction)
	}
}

func TestDecideStageTransition_Dumped_NoChange(t *testing.T) {
	// dumped никогда не меняется автоматически
	ctx := &StageTransitionContext{
		CurrentStage:      StageDumped,
		HerMessagesInStage: 100,
		Score: &RelationshipScore{
			Interest:   100,
			Trust:      100,
			Attraction: 100,
			Annoyance:  0,
		},
	}

	result := DecideStageTransition(ctx)
	if result != nil {
		t.Errorf("dumped should never auto-transition, got %+v", result)
	}
}

func TestDecideStageTransition_Conflict_BlocksUpgrade(t *testing.T) {
	// При активном конфликте не должно быть повышения
	ctx := &StageTransitionContext{
		CurrentStage:      StageMetIrlGotTg,
		HerMessagesInStage: 10,
		HasActiveConflict: true,
		Score: &RelationshipScore{
			Interest:   50,
			Trust:      30,
			Attraction: 40,
			Annoyance:  5,
		},
	}

	result := DecideStageTransition(ctx)
	if result != nil {
		t.Errorf("active conflict should block upgrade, got %+v", result)
	}
}

func TestDecideStageTransition_Downgrade_OverridesDuringConflict(t *testing.T) {
	// Даже при конфликте, если условия понижения выполнены, должно понизить
	ctx := &StageTransitionContext{
		CurrentStage:      StageDatingEarly,
		HerMessagesInStage: 10,
		HasActiveConflict: true,
		Score: &RelationshipScore{
			Interest:   -20,
			Trust:      5,
			Attraction: 30,
			Annoyance:  70,
		},
	}

	result := DecideStageTransition(ctx)
	if result == nil {
		t.Fatal("downgrade should happen even during conflict")
	}
	if result.Direction != "down" {
		t.Errorf("expected direction 'down', got '%s'", result.Direction)
	}
}

func TestDecideStageTransition_NilContext(t *testing.T) {
	result := DecideStageTransition(nil)
	if result != nil {
		t.Error("nil context should return nil")
	}
}

func TestDecideStageTransition_NilScore(t *testing.T) {
	ctx := &StageTransitionContext{
		CurrentStage: StageMetIrlGotTg,
		Score:        nil,
	}
	result := DecideStageTransition(ctx)
	if result != nil {
		t.Error("nil score should return nil")
	}
}

func TestDecideStageTransition_UnknownStage(t *testing.T) {
	ctx := &StageTransitionContext{
		CurrentStage:      StageId("unknown"),
		HerMessagesInStage: 100,
		Score: &RelationshipScore{
			Interest:   100,
			Attraction: 100,
		},
	}
	result := DecideStageTransition(ctx)
	if result != nil {
		t.Error("unknown stage should return nil")
	}
}

func TestUpdateRelationshipScore(t *testing.T) {
	score := &RelationshipScore{
		Interest:   10,
		Trust:      5,
		Attraction: 15,
		Annoyance:  0,
		Cringe:     2,
	}

	delta := &RelationshipScore{
		Interest:   5,
		Trust:      3,
		Attraction: -2,
		Annoyance:  1,
		Cringe:     0,
	}

	result := UpdateRelationshipScore(score, delta)
	if result.Interest != 15 {
		t.Errorf("interest: expected 15, got %.0f", result.Interest)
	}
	if result.Trust != 8 {
		t.Errorf("trust: expected 8, got %.0f", result.Trust)
	}
	if result.Attraction != 13 {
		t.Errorf("attraction: expected 13, got %.0f", result.Attraction)
	}
	if result.Annoyance != 1 {
		t.Errorf("annoyance: expected 1, got %.0f", result.Annoyance)
	}
	if result.Cringe != 2 {
		t.Errorf("cringe: expected 2, got %.0f", result.Cringe)
	}
}

func TestUpdateRelationshipScore_NilScore(t *testing.T) {
	result := UpdateRelationshipScore(nil, &RelationshipScore{Interest: 5})
	if result != nil {
		t.Error("nil score should return nil")
	}
}

func TestUpdateRelationshipScore_NilDelta(t *testing.T) {
	score := &RelationshipScore{Interest: 10}
	result := UpdateRelationshipScore(score, nil)
	if result != score {
		t.Error("nil delta should return original score pointer")
	}
}

func TestRelationshipPromptFragment_WithScore(t *testing.T) {
	score := &RelationshipScore{
		Interest:   45,
		Trust:      30,
		Attraction: 35,
		Annoyance:  5,
		Cringe:     2,
	}

	fragment := RelationshipPromptFragment(StageTgGivenWarming, score)
	if fragment == "" {
		t.Error("expected non-empty prompt fragment")
	}
	if !strings.Contains(fragment, "СТАДИЯ ОТНОШЕНИЙ") {
		t.Error("expected stage header")
	}
	if !strings.Contains(fragment, "Дала тг, отвечает осторожно") {
		t.Error("expected stage label")
	}
	if !strings.Contains(fragment, "Оттаивает") {
		t.Error("expected stage description")
	}
	if !strings.Contains(fragment, "интерес=45") {
		t.Error("expected interest in score")
	}
}

func TestRelationshipPromptFragment_NilScore(t *testing.T) {
	fragment := RelationshipPromptFragment(StageDumped, nil)
	if fragment == "" {
		t.Error("expected non-empty fragment even with nil score")
	}
	if !strings.Contains(fragment, "Отшила") {
		t.Error("expected stage label even with nil score")
	}
	if strings.Contains(fragment, "Скор:") {
		t.Error("should not contain score when nil")
	}
}

func TestRelationshipPromptFragment_VariousStages(t *testing.T) {
	stages := []StageId{
		StageMetIrlGotTg,
		StageTgGivenCold,
		StageTgGivenWarming,
		StageConvinced,
		StageFirstDateDone,
		StageDatingEarly,
		StageDatingStable,
		StageLongTerm,
		StageDumped,
	}

	for _, stage := range stages {
		fragment := RelationshipPromptFragment(stage, nil)
		if fragment == "" {
			t.Errorf("expected non-empty fragment for stage %s", stage)
		}
	}
}

func TestShouldCheckStageTransition(t *testing.T) {
	tests := []struct {
		messages int
		expected bool
	}{
		{0, false},
		{1, false},
		{2, false},
		{3, false},
		{4, false},
		{5, true},
		{6, false},
		{7, false},
		{9, false},
		{10, true},
		{15, true},
		{20, true},
		{25, true},
		{100, true},
		{101, false},
		{-1, false},
		{-5, false},
	}

	for _, tt := range tests {
		result := ShouldCheckStageTransition(tt.messages)
		if result != tt.expected {
			t.Errorf("ShouldCheckStageTransition(%d) = %v, want %v", tt.messages, result, tt.expected)
		}
	}
}

func TestStageOrder_ContainsAllStages(t *testing.T) {
	// Все стадии кроме dumped должны быть в StageOrder
	allStages := []StageId{
		StageMetIrlGotTg,
		StageTgGivenCold,
		StageTgGivenWarming,
		StageConvinced,
		StageFirstDateDone,
		StageDatingEarly,
		StageDatingStable,
		StageLongTerm,
	}

	for _, stage := range allStages {
		idx := stageIndex(stage)
		if idx < 0 {
			t.Errorf("stage %s not found in StageOrder", stage)
		}
	}

	// dumped не должен быть в StageOrder
	dumpIdx := stageIndex(StageDumped)
	if dumpIdx >= 0 {
		t.Errorf("dumped should not be in StageOrder, found at index %d", dumpIdx)
	}
}

func TestDecideStageTransition_Downgrade_NotEnoughHerMessages(t *testing.T) {
	// При высоком annoyance но < 8 её сообщений — не должно быть понижения
	ctx := &StageTransitionContext{
		CurrentStage:      StageDatingEarly,
		HerMessagesInStage: 5,
		Score: &RelationshipScore{
			Interest:   -20,
			Trust:      5,
			Attraction: 30,
			Annoyance:  70,
		},
	}

	result := DecideStageTransition(ctx)
	if result != nil {
		t.Errorf("expected no downgrade with < 8 her messages, got %+v", result)
	}
}

func TestDecideStageTransition_Convinced_InsufficientHerMessages(t *testing.T) {
	// convinced требует >= 10 её сообщений
	ctx := &StageTransitionContext{
		CurrentStage:      StageConvinced,
		HerMessagesInStage: 8,
		Score: &RelationshipScore{
			Interest:   60,
			Trust:      40,
			Attraction: 55,
		},
	}

	result := DecideStageTransition(ctx)
	if result != nil {
		t.Errorf("convinced needs >= 10 her messages, got %+v", result)
	}
}

func TestDecideStageTransition_MetIrl_TgGivenWarming_AnnoyanceBlocks(t *testing.T) {
	// met-irl-got-tg с хорошими скорами но высоким annoyance — не должно повысить
	ctx := &StageTransitionContext{
		CurrentStage:      StageMetIrlGotTg,
		HerMessagesInStage: 10,
		Score: &RelationshipScore{
			Interest:   50,
			Attraction: 40,
			Annoyance:  25, // >= 20 — блокирует
		},
	}

	result := DecideStageTransition(ctx)
	if result != nil {
		t.Errorf("annoyance >= 20 should block upgrade from met-irl-got-tg, got %+v", result)
	}
}
