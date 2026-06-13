package state

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/storage"
)

// MoodState хранит текущее настроение, энергию и эмоциональные метрики.
type MoodState struct {
	CurrentMood   string
	Energy        float64
	Irritability  float64
	Affection     float64
	ConflictLevel int
	LocalHour     int
	IsNightTime   bool
}

// ComputeMoodState вычисляет состояние настроения на основе циркадного ритма,
// уровня конфликта, стадии отношений и метрик.
func ComputeMoodState(tz string, conflictLevel int, relationshipStage StageId, relationshipScore *RelationshipScore) *MoodState {
	hour := localHour(tz)
	return computeMoodState(hour, conflictLevel, relationshipStage, relationshipScore)
}

func localHour(tz string) int {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Now().Hour()
	}
	return time.Now().In(loc).Hour()
}

func computeMoodState(hour int, conflictLevel int, relationshipStage StageId, relationshipScore *RelationshipScore) *MoodState {
	isNightTime := hour >= 22 || hour < 6

	energy := math.Sin(float64(hour-6)/24.0*2*math.Pi) * 0.45

	affection := 0.45
	idx := stageIndex(relationshipStage)
	convincedIdx := stageIndex(StageConvinced)
	if idx >= convincedIdx {
		affection += float64(idx-convincedIdx+1) * 0.1
	}
	affection -= float64(conflictLevel) * 0.1
	if energy > 0.3 {
		affection += 0.05
	}
	affection = clampFloat(affection, 0.0, 1.0)

	var irritabilityBase float64
	switch {
	case hour >= 6 && hour < 12:
		irritabilityBase = 0.2
	case hour >= 12 && hour < 18:
		irritabilityBase = 0.1
	case hour >= 18 && hour < 22:
		irritabilityBase = 0.3
	default:
		irritabilityBase = 0.4
	}

	irritability := irritabilityBase
	irritability += float64(conflictLevel) * 0.15
	if relationshipScore != nil && relationshipScore.Annoyance > 50 {
		irritability += 0.1
	}
	if affection > 0.6 {
		irritability -= 0.05
	}
	irritability = clampFloat(irritability, 0.0, 1.0)

	currentMood := resolveMood(conflictLevel, energy, affection)

	return &MoodState{
		CurrentMood:   currentMood,
		Energy:        energy,
		Irritability:  irritability,
		Affection:     affection,
		ConflictLevel: conflictLevel,
		LocalHour:     hour,
		IsNightTime:   isNightTime,
	}
}

func resolveMood(conflictLevel int, energy, affection float64) string {
	if conflictLevel >= 3 {
		return "irritated"
	}
	if energy > 0.3 && affection > 0.6 {
		return "affectionate"
	}
	if energy > 0.3 {
		return "energetic"
	}
	if energy > 0.0 {
		return "playful"
	}
	if energy > -0.3 {
		return "neutral"
	}
	return "tired"
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// EnrichWithEmotionalState обогащает циркадное настроение данными LLM-анализа эмоций.
// Plutchik-баллы влияют на параметры настроения с понижающим коэффициентом,
// чтобы сохранить стабильность циркадного фона.
func (m *MoodState) EnrichWithEmotionalState(emoState *storage.EmotionalState) {
	if emoState == nil {
		return
	}

	const llmWeight = 0.3

	m.Energy += emoState.Joy * llmWeight * 0.5
	m.Affection += emoState.Joy * llmWeight * 0.4
	m.Irritability -= emoState.Joy * llmWeight * 0.3

	m.Irritability += emoState.Anger * llmWeight * 0.6
	m.Affection -= emoState.Anger * llmWeight * 0.3

	m.Energy -= emoState.Sadness * llmWeight * 0.5

	m.Irritability += emoState.Fear * llmWeight * 0.2
	m.Energy -= emoState.Fear * llmWeight * 0.2

	m.Energy += emoState.Surprise * llmWeight * 0.2

	m.Irritability += emoState.Disgust * llmWeight * 0.4

	m.Affection += emoState.Trust * llmWeight * 0.4

	m.Energy += emoState.Anticipation * llmWeight * 0.2

	m.Energy = clampFloat(m.Energy, 0.0, 1.0)
	m.Irritability = clampFloat(m.Irritability, 0.0, 1.0)
	m.Affection = clampFloat(m.Affection, 0.0, 1.0)

	log.Printf("[MoodState] Обогащено эмоциями LLM: joy=%.2f anger=%.2f → energy=%.2f irritability=%.2f",
		emoState.Joy, emoState.Anger, m.Energy, m.Irritability)
}

// MoodPromptFragment генерирует текстовый фрагмент для системного промпта,
// описывающий текущее настроение.
func MoodPromptFragment(m *MoodState) string {
	if m == nil {
		return ""
	}

	energyLabel := "средняя"
	if m.Energy > 0.3 {
		energyLabel = "высокая"
	} else if m.Energy < -0.3 {
		energyLabel = "низкая"
	}

	irritLabel := "средняя"
	if m.Irritability > 0.5 {
		irritLabel = "высокая"
	} else if m.Irritability < 0.2 {
		irritLabel = "низкая"
	}

	lines := []string{
		fmt.Sprintf("Настроение: %s. Энергия: %s. Раздражительность: %s.", m.CurrentMood, energyLabel, irritLabel),
	}

	if m.ConflictLevel >= 3 {
		lines = append(lines, fmt.Sprintf("Ты раздражена из-за конфликта (level %d). Будь холоднее обычного.", m.ConflictLevel))
	}

	if m.IsNightTime {
		lines = append(lines, "Сейчас ночь — ты уставшая. Отвечай коротко.")
	}

	result := ""
	for i, l := range lines {
		if i > 0 {
			result += "\n"
		}
		result += l
	}
	return result
}
