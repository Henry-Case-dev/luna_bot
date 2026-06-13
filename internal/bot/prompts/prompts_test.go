package prompts

import (
	"strings"
	"testing"
	"text/template"
	"bytes"
)

// TestLoadAndRenderPrompt_NilData and TestLoadAndRenderPrompt_WithData removed —
// prompts now use {PLACEHOLDER} string replacement via enrichPromptWithPersonality,
// not go-template {{.Field}} directives. These tests no longer match architecture.

func TestLoadAndRenderPromptWithDefault_Found(t *testing.T) {
	data := &TemplateData{
		PersonalityContext: "TEST",
	}
	result := LoadAndRenderPromptWithDefault("free_will_should_reply", "DEFAULT", data)
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if result == "DEFAULT" {
		t.Error("should return rendered prompt, not default")
	}
}

func TestLoadAndRenderPromptWithDefault_NotFound(t *testing.T) {
	result := LoadAndRenderPromptWithDefault("nonexistent_prompt_xyz", "FALLBACK", nil)
	if result != "FALLBACK" {
		t.Errorf("expected FALLBACK, got %s", result)
	}
}

func TestTemplateSubstitution(t *testing.T) {
	// Проверяем что text/template корректно подставляет поля TemplateData
	tmplText := `Personality: {{.PersonalityContext}}
Style: {{.StyleInstructions}}
ReplyType: {{.ReplyType}}
Mood: {{.MoodName}} ({{.MoodIntensity}})
TargetMsg: {{.TargetMessageID}}
{{if .Presence}}Presence: Online={{.Presence.Online}}, Hour={{.Presence.LocalHour}}, Hint={{.Presence.Hint}}{{end}}
{{if .Conflict}}Conflict: Active={{.Conflict.Active}}, Level={{.Conflict.Level}}, Reason={{.Conflict.Reason}}{{end}}
{{if .Relationship}}Rel: Stage={{.Relationship.Stage}}, Interest={{.Relationship.Interest}}, Trust={{.Relationship.Trust}}{{end}}
{{if .Mood}}MoodObj: {{.Mood.CurrentMood}}, Energy={{.Mood.Energy}}{{end}}
{{if .DailyLife}}Day: {{.DailyLife.DateLocal}}, Vibe={{.DailyLife.Vibe}}, Weather={{.DailyLife.Weather}}{{end}}`

	data := &TemplateData{
		PersonalityContext: "Я — Луна, андроид-философ",
		StyleInstructions:  "Отвечай остроумно",
		ReplyType:          "direct_reply",
		MoodName:           "sarcastic",
		MoodIntensity:      0.75,
		TargetMessageID:    42,
		Presence: &PresenceData{
			Online:    true,
			LocalHour: 15,
			Hint:      "на работе",
			IsBusy:    false,
		},
		Conflict: &ConflictData{
			Active: true,
			Level:  3,
			Reason: "спор о философии",
		},
		Relationship: &RelationshipData{
			Stage:    "знакомство",
			Interest: 0.6,
			Trust:    0.4,
		},
		Mood: &MoodData{
			CurrentMood: "playful",
			Energy:      0.9,
		},
		DailyLife: &DailyLifeData{
			DateLocal: "12.06.2026",
			Vibe:      "вечерняя",
			Weather:   "ясно",
		},
	}

	tmpl, err := template.New("test").Parse(tmplText)
	if err != nil {
		t.Fatalf("template parse failed: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute failed: %v", err)
	}

	result := buf.String()

	checks := []string{
		"Я — Луна, андроид-философ",
		"Отвечай остроумно",
		"direct_reply",
		"sarcastic",
		"0.75",
		"42",
		"Online=true",
		"Hour=15",
		"на работе",
		"Active=true",
		"Level=3",
		"спор о философии",
		"Stage=знакомство",
		"Interest=0.6",
		"Trust=0.4",
		"Energy=0.9",
		"12.06.2026",
		"вечерняя",
		"ясно",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("expected result to contain %q", check)
		}
	}
}

func TestTemplateSubstitution_NilOptional(t *testing.T) {
	// Проверяем что nil-поля (Presence, Conflict и т.д.) не ломают рендеринг
	tmplText := `{{.PersonalityContext}}
{{if .Presence}}Presence: {{.Presence.Hint}}{{else}}No presence data{{end}}
{{if .Conflict}}Conflict: {{.Conflict.Reason}}{{else}}No conflict{{end}}`

	data := &TemplateData{
		PersonalityContext: "TEST_BASE",
		// Presence, Conflict, etc. — nil
	}

	tmpl, err := template.New("test_nil").Parse(tmplText)
	if err != nil {
		t.Fatalf("template parse failed: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute with nil optionals failed: %v", err)
	}

	result := buf.String()
	if !strings.Contains(result, "No presence data") {
		t.Error("expected 'No presence data' for nil Presence")
	}
	if !strings.Contains(result, "No conflict") {
		t.Error("expected 'No conflict' for nil Conflict")
	}
	if !strings.Contains(result, "TEST_BASE") {
		t.Error("expected TEST_BASE in result")
	}
}
