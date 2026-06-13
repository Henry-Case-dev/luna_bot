package state

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func sampleDailyLife() *DailyLife {
	return &DailyLife{
		DateLocal: "2025-06-12",
		Weather:   "Москва, +22, солнечно",
		Vibe:      "бодрая, выспалась, день обещает быть хорошим",
		Blocks: []DailyLifeBlock{
			{FromHour: 7, ToHour: 9, Activity: "просыпается, завтрак, сборы", Mood: "сонная", Social: "alone", PhoneAvailable: true},
			{FromHour: 9, ToHour: 13, Activity: "на работе", Mood: "рабочая", Social: "with-coworkers", PhoneAvailable: true},
			{FromHour: 13, ToHour: 14, Activity: "обед с коллегами", Mood: "", Social: "with-coworkers", PhoneAvailable: true},
			{FromHour: 14, ToHour: 18, Activity: "работа, совещание", Mood: "устала от Лены", Social: "with-coworkers", PhoneAvailable: false},
			{FromHour: 18, ToHour: 19, Activity: "дорога домой", Mood: "", Social: "in-transit", PhoneAvailable: true},
			{FromHour: 19, ToHour: 21, Activity: "ужин, сериал", Mood: "расслабленная", Social: "alone", PhoneAvailable: true},
			{FromHour: 21, ToHour: 23, Activity: "душ, подготовка ко сну", Mood: "уставшая", Social: "alone", PhoneAvailable: false},
		},
		Events: []string{"на совещании похвалили проект", "в маршрутке встретила Катю"},
		Wants:  []string{"дойти до спортзала", "купить новый кардиган", "посмотреть Вечернего Урганта"},
	}
}

func TestGetCurrentBlock_InRange(t *testing.T) {
	g := NewDailyLifeGenerator()
	dl := sampleDailyLife()

	tests := []struct {
		hour       int
		wantAct    string
		wantSocial string
	}{
		{7, "просыпается, завтрак, сборы", "alone"},
		{8, "просыпается, завтрак, сборы", "alone"},
		{9, "на работе", "with-coworkers"},
		{12, "на работе", "with-coworkers"},
		{13, "обед с коллегами", "with-coworkers"},
		{14, "работа, совещание", "with-coworkers"},
		{17, "работа, совещание", "with-coworkers"},
		{18, "дорога домой", "in-transit"},
		{20, "ужин, сериал", "alone"},
		{21, "душ, подготовка ко сну", "alone"},
		{22, "душ, подготовка ко сну", "alone"},
	}

	for _, tt := range tests {
		b := g.getCurrentBlockHour(dl, tt.hour)
		if b == nil {
			t.Errorf("hour=%d: expected block, got nil", tt.hour)
			continue
		}
		if b.Activity != tt.wantAct {
			t.Errorf("hour=%d: activity = %q, want %q", tt.hour, b.Activity, tt.wantAct)
		}
		if b.Social != tt.wantSocial {
			t.Errorf("hour=%d: social = %q, want %q", tt.hour, b.Social, tt.wantSocial)
		}
	}
}

func TestGetCurrentBlock_OutsideRange(t *testing.T) {
	g := NewDailyLifeGenerator()
	dl := sampleDailyLife()

	// Hours outside all blocks should return the last block
	b := g.getCurrentBlockHour(dl, 23)
	if b == nil {
		t.Fatal("expected last block for hour=23")
	}
	if b.Activity != "душ, подготовка ко сну" {
		t.Errorf("activity = %q, want %q", b.Activity, "душ, подготовка ко сну")
	}

	b = g.getCurrentBlockHour(dl, 5)
	if b == nil {
		t.Fatal("expected last block for hour=5")
	}
	if b.Activity != "душ, подготовка ко сну" {
		t.Errorf("activity = %q, want %q", b.Activity, "душ, подготовка ко сну")
	}

	b = g.getCurrentBlockHour(dl, 0)
	if b == nil {
		t.Fatal("expected last block for hour=0")
	}
}

func TestGetCurrentBlock_NilDailyLife(t *testing.T) {
	g := NewDailyLifeGenerator()
	b := g.GetCurrentBlock(nil, "Europe/Moscow")
	if b != nil {
		t.Errorf("expected nil for nil DailyLife")
	}
}

func TestGetCurrentBlock_EmptyBlocks(t *testing.T) {
	g := NewDailyLifeGenerator()
	dl := &DailyLife{DateLocal: "2025-06-12", Vibe: "ok", Blocks: []DailyLifeBlock{}}
	b := g.GetCurrentBlock(dl, "Europe/Moscow")
	if b != nil {
		t.Errorf("expected nil for empty blocks")
	}
}

func TestDailyLifePromptFragment_ContainsVibe(t *testing.T) {
	dl := sampleDailyLife()
	fragment := DailyLifePromptFragment(dl, "Europe/Moscow")

	if !strings.Contains(fragment, dl.Vibe) {
		t.Errorf("fragment should contain vibe %q", dl.Vibe)
	}
	if !strings.Contains(fragment, dl.DateLocal) {
		t.Errorf("fragment should contain date %q", dl.DateLocal)
	}
	if !strings.Contains(fragment, dl.Weather) {
		t.Errorf("fragment should contain weather %q", dl.Weather)
	}
}

func TestDailyLifePromptFragment_ContainsCurrentActivity(t *testing.T) {
	dl := sampleDailyLife()
	fragment := DailyLifePromptFragment(dl, "Europe/Moscow")

	// Fragment should reference the schedule section
	if !strings.Contains(fragment, "ты:") {
		t.Error("fragment should contain current activity reference")
	}
}

func TestDailyLifePromptFragment_HasSocialContext(t *testing.T) {
	dl := sampleDailyLife()
	fragment := DailyLifePromptFragment(dl, "Europe/Moscow")

	if !strings.Contains(fragment, "Социально:") {
		t.Error("fragment should contain social context")
	}
}

func TestDailyLifePromptFragment_Nil(t *testing.T) {
	fragment := DailyLifePromptFragment(nil, "Europe/Moscow")
	if fragment != "" {
		t.Errorf("expected empty string for nil DailyLife, got %q", fragment)
	}
}

// mockLLMCaller implements LLMCaller for testing.
type mockLLMCaller struct {
	mu       sync.Mutex
	callCount int
	response  string
	err       error
}

func (m *mockLLMCaller) Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestGenerateDailyLife_CacheReturnsCached(t *testing.T) {
	g := NewDailyLifeGenerator()
	mock := &mockLLMCaller{
		response: `{"vibe":"хороший день","weather":"Москва, +20","blocks":[{"fromHour":8,"toHour":9,"activity":"сон","social":"alone","phoneAvailable":false}],"events":[],"wants":[]}`,
	}

	ctx := context.Background()

	dl1, err := g.GenerateDailyLife(ctx, mock, "Europe/Moscow", 23, 7)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if dl1 == nil {
		t.Fatal("expected non-nil DailyLife")
	}

	if mock.callCount != 1 {
		t.Errorf("expected 1 LLM call, got %d", mock.callCount)
	}

	dl2, err := g.GenerateDailyLife(ctx, mock, "Europe/Moscow", 23, 7)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if mock.callCount != 1 {
		t.Errorf("second call should use cache, expected 1 LLM call total, got %d", mock.callCount)
	}

	if dl1 != dl2 {
		t.Error("cached result should be the same pointer")
	}
}

func TestParseDailyLifeJSON_Valid(t *testing.T) {
	raw := `{"vibe":"отличный день","weather":"СПб, +18","blocks":[{"fromHour":8,"toHour":10,"activity":"работа","social":"with-coworkers","phoneAvailable":true}],"events":["кофе пролили"],"wants":["спать"]}`
	dl := parseDailyLifeJSON(raw, "2025-06-12")

	if dl.Vibe != "отличный день" {
		t.Errorf("vibe = %q, want %q", dl.Vibe, "отличный день")
	}
	if len(dl.Blocks) != 1 {
		t.Errorf("blocks count = %d, want 1", len(dl.Blocks))
	}
	if dl.Blocks[0].Activity != "работа" {
		t.Errorf("activity = %q, want %q", dl.Blocks[0].Activity, "работа")
	}
	if len(dl.Events) != 1 {
		t.Errorf("events count = %d, want 1", len(dl.Events))
	}
}

func TestParseDailyLifeJSON_MarkdownCodeBlock(t *testing.T) {
	raw := "```json\n{\"vibe\":\"норм\",\"weather\":\"дождь\",\"blocks\":[],\"events\":[],\"wants\":[]}\n```"
	dl := parseDailyLifeJSON(raw, "2025-06-12")

	if dl.Vibe != "норм" {
		t.Errorf("vibe = %q, want %q", dl.Vibe, "норм")
	}
}

func TestParseDailyLifeJSON_Invalid(t *testing.T) {
	raw := "not json at all"
	dl := parseDailyLifeJSON(raw, "2025-06-12")

	if dl.Vibe != "обычный день" {
		t.Errorf("expected fallback vibe, got %q", dl.Vibe)
	}
	if len(dl.Blocks) != 0 {
		t.Errorf("expected empty blocks on parse error")
	}
}

func TestParseDailyLifeJSON_MissingFields(t *testing.T) {
	raw := `{"vibe":"ок"}`
	dl := parseDailyLifeJSON(raw, "2025-06-12")

	if dl.Vibe != "ок" {
		t.Errorf("vibe = %q", dl.Vibe)
	}
	if dl.Weather != "" {
		t.Errorf("weather should be empty, got %q", dl.Weather)
	}
	if len(dl.Blocks) != 0 {
		t.Errorf("blocks should be empty, got %d", len(dl.Blocks))
	}
}

func TestExtractJSON_PlainJSON(t *testing.T) {
	raw := `{"key":"value"}`
	result := extractJSON(raw)
	if result != `{"key":"value"}` {
		t.Errorf("got %q", result)
	}
}

func TestExtractJSON_MarkdownCodeBlock(t *testing.T) {
	raw := "```json\n{\"key\":\"value\"}\n```"
	result := extractJSON(raw)
	if result != `{"key":"value"}` {
		t.Errorf("got %q", result)
	}
}

func TestExtractJSON_NoBraces(t *testing.T) {
	raw := "no json here"
	result := extractJSON(raw)
	if result != "no json here" {
		t.Errorf("got %q", result)
	}
}

func TestDailyLifePromptFragment_PhoneUnavailable(t *testing.T) {
	dl := &DailyLife{
		DateLocal: "2025-06-12",
		Vibe:      "уставшая",
		Blocks: []DailyLifeBlock{
			{FromHour: 0, ToHour: 24, Activity: "спит", Social: "alone", PhoneAvailable: false},
		},
	}
	fragment := DailyLifePromptFragment(dl, "Europe/Moscow")
	if !strings.Contains(fragment, "НЕ доступен") {
		t.Error("fragment should mention phone not available")
	}
}

func TestDailyLifePromptFragment_TruncatesEventsAndWants(t *testing.T) {
	dl := &DailyLife{
		DateLocal: "2025-06-12",
		Vibe:      "норм",
		Blocks: []DailyLifeBlock{
			{FromHour: 0, ToHour: 24, Activity: "дома", Social: "alone", PhoneAvailable: true},
		},
		Events: []string{"a", "b", "c", "d", "e"},
		Wants:  []string{"w1", "w2", "w3", "w4"},
	}
	fragment := DailyLifePromptFragment(dl, "Europe/Moscow")

	// Only first 3 events/wants should appear
	if strings.Contains(fragment, "d") && strings.Count(fragment, ";") > 5 {
		t.Error("events/wants should be truncated to 3")
	}
}

func TestGenerateDailyLife_ParseErrorStillReturns(t *testing.T) {
	g := NewDailyLifeGenerator()
	mock := &mockLLMCaller{
		response: "not json",
	}

	ctx := context.Background()
	dl, err := g.GenerateDailyLife(ctx, mock, "Europe/Moscow", 23, 7)
	if err != nil {
		t.Fatalf("should not error on parse failure: %v", err)
	}
	if dl == nil {
		t.Fatal("expected non-nil fallback DailyLife")
	}
	if dl.Vibe != "обычный день" {
		t.Errorf("expected fallback vibe, got %q", dl.Vibe)
	}
}
