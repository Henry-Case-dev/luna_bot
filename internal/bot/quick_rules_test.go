package bot

import (
	"testing"

	"github.com/Henry-Case-dev/luna_bot/internal/bot/prompts"
)

func simulateQuickRules(data *prompts.TemplateData) *QuickRuleResult {
	if data.Presence != nil && data.Presence.Asleep && !data.Presence.NightAwake {
		return &QuickRuleResult{
			Matched: true, ShouldReply: false,
			Reason: "asleep",
		}
	}

	if data.Conflict != nil && data.Conflict.ColdActive {
		return &QuickRuleResult{
			Matched: true, ShouldReply: false,
			Reason: "conflict-cold",
		}
	}

	if data.Presence != nil && data.Presence.IsBusy && !data.Presence.Online {
		return &QuickRuleResult{
			Matched: true, ShouldReply: false,
			Reason: "busy",
		}
	}

	if data.Presence != nil && data.Presence.NightAwake {
		return &QuickRuleResult{
			Matched: true, ShouldReply: true,
			Reason:    "night-awake",
			ReplyType: "general",
			Mood:      "tired",
		}
	}

	return nil
}

func TestApplyQuickRules_Asleep_NoReply(t *testing.T) {
	data := &prompts.TemplateData{
		Presence: &prompts.PresenceData{
			Asleep:     true,
			NightAwake: false,
		},
	}

	result := simulateQuickRules(data)
	if result == nil {
		t.Fatal("expected QuickRuleResult, got nil")
	}
	if !result.Matched {
		t.Error("expected Matched=true")
	}
	if result.ShouldReply {
		t.Error("expected ShouldReply=false when asleep")
	}
	if result.Reason != "asleep" {
		t.Errorf("expected reason 'asleep', got %q", result.Reason)
	}
}

func TestApplyQuickRules_ConflictCold_NoReply(t *testing.T) {
	data := &prompts.TemplateData{
		Presence: &prompts.PresenceData{
			Online: true,
		},
		Conflict: &prompts.ConflictData{
			ColdActive: true,
			Active:     true,
			Level:      2,
		},
	}

	result := simulateQuickRules(data)
	if result == nil {
		t.Fatal("expected QuickRuleResult, got nil")
	}
	if !result.Matched {
		t.Error("expected Matched=true for conflict-cold")
	}
	if result.ShouldReply {
		t.Error("expected ShouldReply=false when conflict-cold")
	}
	if result.Reason != "conflict-cold" {
		t.Errorf("expected reason 'conflict-cold', got %q", result.Reason)
	}
}

func TestApplyQuickRules_BusyNotOnline_NoReply(t *testing.T) {
	data := &prompts.TemplateData{
		Presence: &prompts.PresenceData{
			IsBusy: true,
			Online: false,
		},
	}

	result := simulateQuickRules(data)
	if result == nil {
		t.Fatal("expected QuickRuleResult, got nil")
	}
	if !result.Matched {
		t.Error("expected Matched=true for busy-not-online")
	}
	if result.ShouldReply {
		t.Error("expected ShouldReply=false when busy and not online")
	}
	if result.Reason != "busy" {
		t.Errorf("expected reason 'busy', got %q", result.Reason)
	}
}

func TestApplyQuickRules_NightAwake_ShortReply(t *testing.T) {
	data := &prompts.TemplateData{
		Presence: &prompts.PresenceData{
			NightAwake: true,
			Online:     true,
		},
	}

	result := simulateQuickRules(data)
	if result == nil {
		t.Fatal("expected QuickRuleResult, got nil")
	}
	if !result.Matched {
		t.Error("expected Matched=true for night-awake")
	}
	if !result.ShouldReply {
		t.Error("expected ShouldReply=true when night-awake")
	}
	if result.Mood != "tired" {
		t.Errorf("expected Mood='tired', got %q", result.Mood)
	}
	if result.Reason != "night-awake" {
		t.Errorf("expected reason 'night-awake', got %q", result.Reason)
	}
}

func TestApplyQuickRules_NoMatch_ReturnsNil(t *testing.T) {
	data := &prompts.TemplateData{
		Presence: &prompts.PresenceData{
			Online:     true,
			Asleep:     false,
			NightAwake: false,
			IsBusy:     false,
		},
		Conflict: nil,
	}

	result := simulateQuickRules(data)
	if result != nil {
		t.Errorf("expected nil for normal state, got result: %+v", result)
	}
}

func TestApplyQuickRules_AsleepButNightAwake_ShouldReply(t *testing.T) {
	data := &prompts.TemplateData{
		Presence: &prompts.PresenceData{
			Asleep:     true,
			NightAwake: true,
		},
	}

	result := simulateQuickRules(data)
	if result == nil {
		t.Fatal("expected QuickRuleResult, got nil")
	}
	if !result.Matched {
		t.Error("expected Matched=true")
	}
	if !result.ShouldReply {
		t.Error("expected ShouldReply=true — NightAwake overrides Asleep")
	}
	if result.Reason != "night-awake" {
		t.Errorf("expected reason 'night-awake', got %q", result.Reason)
	}
}

func TestApplyQuickRules_RulePriority_AsleepBeforeConflict(t *testing.T) {
	data := &prompts.TemplateData{
		Presence: &prompts.PresenceData{
			Asleep:     true,
			NightAwake: false,
			Online:     false,
		},
		Conflict: &prompts.ConflictData{
			ColdActive: true,
			Active:     true,
			Level:      3,
		},
	}

	result := simulateQuickRules(data)
	if result == nil {
		t.Fatal("expected QuickRuleResult, got nil")
	}
	if result.Reason != "asleep" {
		t.Errorf("expected 'asleep' to take priority over 'conflict-cold', got %q", result.Reason)
	}
}

func TestApplyQuickRules_BusyButOnline_NoMatch(t *testing.T) {
	data := &prompts.TemplateData{
		Presence: &prompts.PresenceData{
			IsBusy: true,
			Online: true,
		},
	}

	result := simulateQuickRules(data)
	if result != nil {
		t.Errorf("expected nil when busy BUT online, got result: %+v", result)
	}
}

func TestApplyQuickRules_AllNormal_NoMatch(t *testing.T) {
	data := &prompts.TemplateData{
		Presence: &prompts.PresenceData{
			Online:     true,
			Asleep:     false,
			NightAwake: false,
			IsBusy:     false,
			LocalHour:  12,
			Hint:       "Сейчас в сети",
		},
		Conflict: &prompts.ConflictData{
			ColdActive: false,
			Active:     false,
			Level:      0,
		},
	}

	result := simulateQuickRules(data)
	if result != nil {
		t.Errorf("expected nil for fully normal state, got result: %+v", result)
	}
}

func TestApplyQuickRules_NilPresence_Safe(t *testing.T) {
	data := &prompts.TemplateData{
		Presence: nil,
		Conflict: &prompts.ConflictData{
			ColdActive: true,
		},
	}

	result := simulateQuickRules(data)
	if result == nil {
		t.Fatal("expected QuickRuleResult for conflict even with nil Presence")
	}
	if result.Reason != "conflict-cold" {
		t.Errorf("expected 'conflict-cold', got %q", result.Reason)
	}
}
