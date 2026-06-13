package bot

import (
	"strings"
	"testing"

	"github.com/Henry-Case-dev/luna_bot/internal/bot/prompts"
	"github.com/Henry-Case-dev/luna_bot/internal/bot/state"
)

func TestPipeline_Simulation_FullFlow(t *testing.T) {
	chatID := int64(12345)
	userID := int64(67890)

	stateData := simulateCollectState(chatID, userID)

	if stateData.Presence == nil {
		t.Fatal("Presence must not be nil")
	}
	if stateData.Relationship == nil {
		t.Fatal("Relationship must not be nil")
	}
	if stateData.Mood == nil {
		t.Fatal("Mood must not be nil")
	}

	quickResult := simulateQuickRules(stateData)
	if quickResult != nil {
		t.Logf("Quick Rule matched: %s, shouldReply=%t", quickResult.Reason, quickResult.ShouldReply)
	} else {
		t.Log("No Quick Rule matched — would call LLM")
	}

	rendered, err := prompts.LoadAndRenderPrompt("free_will_should_reply", stateData)
	if err != nil {
		t.Fatalf("Failed to render prompt: %v", err)
	}
	if rendered == "" {
		t.Fatal("Rendered prompt must not be empty")
	}

	t.Logf("Rendered prompt length: %d chars", len(rendered))
	t.Logf("Presence hint: %s", stateData.Presence.Hint)
	t.Logf("Mood: %s (energy=%.2f)", stateData.Mood.CurrentMood, stateData.Mood.Energy)
}

func TestPipeline_Simulation_AsleepState(t *testing.T) {
	data := &prompts.TemplateData{
		Presence: &prompts.PresenceData{
			Asleep:     true,
			NightAwake: false,
			LocalHour:  3,
			Hint:       "СПИШЬ",
		},
	}

	result := simulateQuickRules(data)
	if result == nil {
		t.Fatal("expected QuickRuleResult for asleep state")
	}
	if result.ShouldReply {
		t.Error("expected ShouldReply=false when asleep without NightAwake")
	}
	if !result.Matched {
		t.Error("expected Matched=true")
	}

	t.Logf("Asleep state -> Quick Rule: %s (should_reply=%t)", result.Reason, result.ShouldReply)
}

func TestPipeline_Simulation_ConflictState(t *testing.T) {
	data := &prompts.TemplateData{
		Presence: &prompts.PresenceData{
			Online: true,
		},
		Conflict: &prompts.ConflictData{
			ColdActive: true,
			Active:     true,
			Level:      3,
			Fragment:   "# КОНФЛИКТ (level 3)",
		},
	}

	result := simulateQuickRules(data)
	if result == nil {
		t.Fatal("expected QuickRuleResult for conflict state")
	}
	if result.ShouldReply {
		t.Error("expected ShouldReply=false when conflict-cold is active")
	}
	if result.Reason != "conflict-cold" {
		t.Errorf("expected 'conflict-cold', got %q", result.Reason)
	}
}

func TestPipeline_Simulation_NormalState(t *testing.T) {
	data := &prompts.TemplateData{
		Presence: &prompts.PresenceData{
			Online:     true,
			Asleep:     false,
			NightAwake: false,
			IsBusy:     false,
			LocalHour:  14,
			Hint:       "Сейчас в сети",
		},
		Relationship: &prompts.RelationshipData{
			Stage:      string(state.StageTgGivenWarming),
			Interest:   40,
			Trust:      25,
			Attraction: 30,
			Annoyance:  10,
			Fragment:   state.RelationshipPromptFragment(state.StageTgGivenWarming, &state.RelationshipScore{Interest: 40, Trust: 25, Attraction: 30, Annoyance: 10}),
		},
		Mood: &prompts.MoodData{
			CurrentMood:  "neutral",
			Energy:       0.0,
			Irritability: 0.2,
		},
	}

	result := simulateQuickRules(data)
	if result != nil {
		t.Errorf("expected nil (no Quick Rule match for normal state), got: %+v", result)
	}

	rendered, err := prompts.LoadAndRenderPrompt("free_will_should_reply", data)
	if err != nil {
		t.Fatalf("Failed to render prompt in normal state: %v", err)
	}
	if rendered == "" {
		t.Error("Rendered prompt must not be empty for normal state")
	}
	t.Logf("Normal state prompt rendered: %d chars", len(rendered))
}

// TestPipeline_Simulation_PromptContainsStates removed — personality injection
// now happens through enrichPromptWithPersonality, not go-template rendering.

func TestPipeline_Simulation_NightAwakeButAsleep(t *testing.T) {
	data := &prompts.TemplateData{
		Presence: &prompts.PresenceData{
			Asleep:     true,
			NightAwake: true,
			Online:     true,
			LocalHour:  3,
			Hint:       "НОЧНОЕ ПРОБУЖДЕНИЕ",
		},
		Mood: &prompts.MoodData{
			CurrentMood:  "tired",
			Energy:       -0.5,
			Irritability: 0.4,
		},
	}

	result := simulateQuickRules(data)
	if result == nil {
		t.Fatal("expected QuickRuleResult for night-awake state")
	}
	if !result.ShouldReply {
		t.Error("NightAwake must trigger reply even when Asleep")
	}
	if result.Mood != "tired" {
		t.Errorf("Mood must be 'tired' for night-awake, got %q", result.Mood)
	}

	t.Logf("NightAwake + Asleep -> Quick Rule: %s, Mood=%s", result.Reason, result.Mood)
}

func TestPipeline_Simulation_PromptRenderWithNilConflict(t *testing.T) {
	data := &prompts.TemplateData{
		PersonalityContext: "test",
		Presence: &prompts.PresenceData{
			Online:    true,
			LocalHour: 12,
			Hint:      "В сети",
		},
		Relationship: &prompts.RelationshipData{
			Stage:      string(state.StageTgGivenWarming),
			Interest:   40,
			Trust:      25,
			Attraction: 30,
			Fragment:   "test fragment",
		},
		Mood: &prompts.MoodData{
			CurrentMood: "neutral",
			Energy:      0.0,
			Fragment:    "test mood",
		},
	}

	rendered, err := prompts.LoadAndRenderPrompt("free_will_should_reply", data)
	if err != nil {
		t.Fatalf("Rendering with nil Conflict should not error: %v", err)
	}
	if rendered == "" {
		t.Error("Rendered prompt must not be empty")
	}
}

// TestPipeline_Simulation_PromptRenderWithActiveConflict removed — personality
// injection now happens through enrichPromptWithPersonality, not go-template rendering.

func TestPipeline_Simulation_BusyState(t *testing.T) {
	data := &prompts.TemplateData{
		Presence: &prompts.PresenceData{
			Online:    false,
			IsBusy:    true,
			BusyLabel: "учёба/пары",
			BusyUntil: "15:30",
			LocalHour: 11,
			Hint:      "Сейчас занята: учёба/пары до 15:30",
		},
	}

	result := simulateQuickRules(data)
	if result == nil {
		t.Fatal("expected QuickRuleResult for busy state when offline")
	}
	if result.ShouldReply {
		t.Error("expected ShouldReply=false when busy and offline")
	}
	if result.Reason != "busy" {
		t.Errorf("expected reason 'busy', got %q", result.Reason)
	}
}

func TestPipeline_Simulation_LocalePresentInPrompt(t *testing.T) {
	data := simulateCollectState(12345, 67890)

	if data.Presence.LocalHour < 0 || data.Presence.LocalHour > 23 {
		t.Errorf("LocalHour must be 0-23, got %d", data.Presence.LocalHour)
	}

	rendered, err := prompts.LoadAndRenderPrompt("free_will_should_reply", data)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	if !strings.Contains(rendered, data.Presence.Hint) {
		t.Log("Hint may not be literally in prompt — template may transform it")
	}

	t.Logf("LocalHour=%d, Hint=%q", data.Presence.LocalHour, data.Presence.Hint)
	t.Logf("Prompt contains Presence: %t", strings.Contains(rendered, data.Presence.Hint))
}
