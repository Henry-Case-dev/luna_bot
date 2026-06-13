package state

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/bot/prompts"
)

// LLMCaller — интерфейс для вызова LLM. Реализуется пакетом bot.
type LLMCaller interface {
	Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// DailyLifeBlock represents a single time block in the daily schedule.
type DailyLifeBlock struct {
	FromHour      int    `json:"fromHour"`
	ToHour        int    `json:"toHour"`
	Activity      string `json:"activity"`
	Mood          string `json:"mood,omitempty"`
	Social        string `json:"social"`
	PhoneAvailable bool  `json:"phoneAvailable"`
}

// DailyLife represents a full daily schedule for one local day.
type DailyLife struct {
	DateLocal string           `json:"dateLocal"`
	Weather   string           `json:"weather,omitempty"`
	Vibe      string           `json:"vibe"`
	Blocks    []DailyLifeBlock `json:"blocks"`
	Events    []string         `json:"events"`
	Wants     []string         `json:"wants"`
}

// DailyLifeGenerator manages generation and caching of daily schedules.
type DailyLifeGenerator struct {
	mu    sync.RWMutex
	cache map[string]*DailyLife // ключ: "dateLocal"
}

// NewDailyLifeGenerator creates a new DailyLifeGenerator.
func NewDailyLifeGenerator() *DailyLifeGenerator {
	return &DailyLifeGenerator{
		cache: make(map[string]*DailyLife),
	}
}

func dateStrLocal(tz string) string {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Now().Format("2006-01-02")
	}
	return time.Now().In(loc).Format("2006-01-02")
}

// GenerateDailyLife calls LLM to generate a daily schedule for today.
// If a schedule for today already exists in cache, returns it without calling LLM.
func (g *DailyLifeGenerator) GenerateDailyLife(ctx context.Context, llmCaller LLMCaller, tz string, sleepFrom, sleepTo int) (*DailyLife, error) {
	dateLocal := dateStrLocal(tz)

	g.mu.RLock()
	if cached, ok := g.cache[dateLocal]; ok {
		g.mu.RUnlock()
		return cached, nil
	}
	g.mu.RUnlock()

	sysPrompt, err := prompts.LoadPrompt("daily_life_gen")
	if err != nil || sysPrompt == "" {
		sysPrompt = defaultSysPrompt
	}

	userPrompt := fmt.Sprintf(
		"Часовой пояс: %s. Сегодня: %s. Сон: с %d:00 до %d:00.\n\nСгенерируй STRICT JSON (только JSON, без markdown).",
		tz, dateLocal, sleepFrom, sleepTo,
	)

	raw, err := llmCaller.Chat(ctx, sysPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	dl := parseDailyLifeJSON(raw, dateLocal)

	g.mu.Lock()
	g.cache[dateLocal] = dl
	g.mu.Unlock()

	return dl, nil
}

// LoadOrGenerateDailyLife loads a schedule from cache or generates a new one.
func (g *DailyLifeGenerator) LoadOrGenerateDailyLife(ctx context.Context, llmCaller LLMCaller, tz string, sleepFrom, sleepTo int) (*DailyLife, error) {
	return g.GenerateDailyLife(ctx, llmCaller, tz, sleepFrom, sleepTo)
}

// GetCurrentBlock returns the block matching the current local hour.
// If no block matches, returns the last block in the schedule.
func (g *DailyLifeGenerator) GetCurrentBlock(dl *DailyLife, tz string) *DailyLifeBlock {
	if dl == nil || len(dl.Blocks) == 0 {
		return nil
	}
	h := localHour(tz)
	for i := range dl.Blocks {
		b := &dl.Blocks[i]
		if h >= b.FromHour && h < b.ToHour {
			return b
		}
	}
	return &dl.Blocks[len(dl.Blocks)-1]
}

// getCurrentBlockHour returns the block matching a specific hour (for testing).
func (g *DailyLifeGenerator) getCurrentBlockHour(dl *DailyLife, hour int) *DailyLifeBlock {
	if dl == nil || len(dl.Blocks) == 0 {
		return nil
	}
	for i := range dl.Blocks {
		b := &dl.Blocks[i]
		if hour >= b.FromHour && hour < b.ToHour {
			return b
		}
	}
	return &dl.Blocks[len(dl.Blocks)-1]
}

// DailyLifePromptFragment generates a text fragment describing the current
// daily life context for insertion into the system prompt.
func DailyLifePromptFragment(dl *DailyLife, tz string) string {
	if dl == nil {
		return ""
	}
	gen := &DailyLifeGenerator{}
	b := gen.GetCurrentBlock(dl, tz)

	parts := []string{
		fmt.Sprintf("# Твой сегодняшний день (%s) — это ТВОЯ жизнь, не его", dl.DateLocal),
		fmt.Sprintf("Общий настрой дня: %s", dl.Vibe),
	}

	if dl.Weather != "" {
		parts = append(parts, fmt.Sprintf("Погода: %s", dl.Weather))
	}

	if b != nil {
		line := fmt.Sprintf("Сейчас (%d:00–%d:00) ты: %s. Социально: %s.",
			b.FromHour, b.ToHour, b.Activity, b.Social)
		if !b.PhoneAvailable {
			line += " Телефон сейчас НЕ доступен — отвечать не можешь / не хочешь."
		}
		parts = append(parts, line)
		if b.Mood != "" {
			parts = append(parts, fmt.Sprintf("Текущее микро-настроение: %s", b.Mood))
		}
	}

	if len(dl.Events) > 0 {
		events := dl.Events
		if len(events) > 3 {
			events = events[:3]
		}
		parts = append(parts, fmt.Sprintf("Что произошло/произойдёт сегодня: %s", strings.Join(events, "; ")))
	}

	if len(dl.Wants) > 0 {
		wants := dl.Wants
		if len(wants) > 3 {
			wants = wants[:3]
		}
		parts = append(parts, fmt.Sprintf("Чего ты хочешь сегодня: %s", strings.Join(wants, "; ")))
	}

	parts = append(parts, "Не вываливай это всё на него сразу. Это для ТЕБЯ — фон. Упоминай естественно когда уместно (\"щас на паре\", \"блин с мамой повздорила\", \"ща в маршрутке\"), не разово как лекцию.")

	return strings.Join(parts, "\n")
}

func parseDailyLifeJSON(raw, dateLocal string) *DailyLife {
	jsonStr := extractJSON(raw)

	var parsed struct {
		Vibe    string           `json:"vibe"`
		Weather string           `json:"weather"`
		Blocks  []DailyLifeBlock `json:"blocks"`
		Events  []string         `json:"events"`
		Wants   []string         `json:"wants"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return &DailyLife{
			DateLocal: dateLocal,
			Vibe:      "обычный день",
			Blocks:    []DailyLifeBlock{},
			Events:    []string{},
			Wants:     []string{},
		}
	}

	if parsed.Vibe == "" {
		parsed.Vibe = "обычный день"
	}

	return &DailyLife{
		DateLocal: dateLocal,
		Weather:   parsed.Weather,
		Vibe:      parsed.Vibe,
		Blocks:    parsed.Blocks,
		Events:    parsed.Events,
		Wants:     parsed.Wants,
	}
}

func extractJSON(raw string) string {
	s := raw

	// Try extracting from ```json ... ``` block
	if idx := strings.Index(s, "```json"); idx != -1 {
		start := idx + 7
		if end := strings.Index(s[start:], "```"); end != -1 {
			s = s[start : start+end]
		}
	} else if idx := strings.Index(s, "```"); idx != -1 {
		start := idx + 3
		if nl := strings.IndexByte(s[start:], '\n'); nl != -1 {
			start += nl + 1
		}
		if end := strings.Index(s[start:], "```"); end != -1 {
			s = s[start : start+end]
		}
	}

	// Find first { and last }
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start != -1 && end != -1 && end > start {
		s = s[start : end+1]
	}

	return s
}

const defaultSysPrompt = `Ты — режиссёр повседневной жизни. Сгенерируй ОДИН обычный день для персонажа.

Сгенерируй STRICT JSON:
{
  "vibe": "1 предложение — как она ощущает себя сегодня",
  "weather": "город+погода коротко",
  "blocks": [
    {"fromHour": 8, "toHour": 9, "activity": "просыпается, кофе", "mood": "не выспалась", "social": "alone", "phoneAvailable": true}
  ],
  "events": ["2-3 мини-события дня"],
  "wants": ["2-3 её желания сегодня"]
}

Правила:
- blocks должны покрывать всё время бодрствования (6-9 блоков)
- phoneAvailable=false когда: тренировка, важное занятие, сон
- social: "alone", "with-friends", "with-family", "with-coworkers", "in-transit"
- Только JSON, без комментариев.`
