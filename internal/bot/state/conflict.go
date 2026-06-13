package state

import (
	"fmt"
	"math"
	"math/rand"
	"time"
)

// ConflictLevel — уровень эскалации конфликта.
type ConflictLevel int

const (
	ConflictNone    ConflictLevel = 0 // нет конфликта
	ConflictMild    ConflictLevel = 1 // обиделась на час
	ConflictUpset   ConflictLevel = 2 // обижена на сутки
	ConflictSerious ConflictLevel = 3 // серьёзный конфликт, дни
	ConflictBreakup ConflictLevel = 4 // на грани разрыва
)

// ConflictState хранит текущее состояние конфликта.
type ConflictState struct {
	Level     ConflictLevel
	ColdUntil time.Time
	Reason    string
	Since     time.Time
	History   []ConflictEvent
}

// ConflictEvent — одна запись в истории конфликта.
type ConflictEvent struct {
	Timestamp  time.Time
	Note       string
	DeltaLevel int
}

// RelationshipScore — снимок метрик отношений.
type RelationshipScore struct {
	Interest   float64
	Trust      float64
	Attraction float64
	Annoyance  float64
	Cringe     float64
}

// EscalateConflict решает, нужно ли поднять уровень конфликта на основе
// текущих метрик отношений и текста входящего сообщения.
func EscalateConflict(current *ConflictState, score *RelationshipScore, incomingText string) *ConflictState {
	trigger := score.Annoyance + score.Cringe - score.Interest

	var newLevel ConflictLevel
	var coldHours float64
	var bumpReason string

	switch {
	case score.Annoyance > 85 && score.Cringe > 70 && score.Interest < -30:
		newLevel = ConflictBreakup
		coldHours = 48 + rand.Float64()*48
		bumpReason = "на грани разрыва"
	case trigger >= 25 || score.Annoyance > 70:
		newLevel = ConflictSerious
		coldHours = 24 + rand.Float64()*24
		bumpReason = "сильный негатив"
	case trigger >= 15:
		newLevel = ConflictUpset
		coldHours = 4 + rand.Float64()*12
		bumpReason = "обижена"
	case trigger >= 8:
		newLevel = ConflictMild
		coldHours = 0.5 + rand.Float64()*2
		bumpReason = "немного дуется"
	default:
		newLevel = ConflictNone
	}

	if newLevel == ConflictNone {
		return current
	}

	if newLevel > current.Level {
		newLevel = ConflictLevel(math.Max(float64(newLevel), float64(current.Level)))
	} else {
		newLevel = current.Level
	}

	if newLevel == current.Level && newLevel == ConflictNone {
		return current
	}

	if newLevel <= current.Level {
		return current
	}

	next := *current
	next.Level = newLevel

	if next.Since.IsZero() {
		next.Since = time.Now()
	}

	next.Reason = bumpReason

	if coldHours > 0 {
		until := time.Now().Add(time.Duration(coldHours * float64(time.Hour)))
		existing := current.ColdUntil
		if existing.IsZero() || until.After(existing) {
			next.ColdUntil = until
		}
	}

	deltaLevel := int(newLevel) - int(current.Level)
	note := fmt.Sprintf("level %d→%d: %s | \"%s\"",
		current.Level, newLevel, bumpReason, truncate(incomingText, 60))

	event := ConflictEvent{
		Timestamp:  time.Now(),
		Note:       note,
		DeltaLevel: deltaLevel,
	}

	historyCopy := make([]ConflictEvent, len(current.History))
	copy(historyCopy, current.History)
	next.History = append(historyCopy, event)

	return &next
}

// SoftenConflict снижает уровень конфликта при позитивном взаимодействии.
func SoftenConflict(current *ConflictState, deltaInterest, deltaTrust, deltaAttraction float64) *ConflictState {
	if current.Level == ConflictNone {
		return current
	}

	positive := deltaAttraction + deltaTrust + deltaInterest
	if positive < 12 {
		return current
	}

	next := *current
	next.Level = ConflictLevel(math.Max(0, float64(current.Level-1)))

	if next.Level == ConflictNone {
		next.ColdUntil = time.Time{}
		next.Since = time.Time{}
		next.Reason = ""
	} else if !next.ColdUntil.IsZero() {
		remaining := time.Until(next.ColdUntil)
		if remaining > 0 {
			next.ColdUntil = time.Now().Add(remaining / 2)
		}
	}

	deltaLevel := int(next.Level) - int(current.Level)
	note := fmt.Sprintf("softened to level %d (positive %.0f)", next.Level, positive)

	event := ConflictEvent{
		Timestamp:  time.Now(),
		Note:       note,
		DeltaLevel: deltaLevel,
	}

	historyCopy := make([]ConflictEvent, len(current.History))
	copy(historyCopy, current.History)
	next.History = append(historyCopy, event)

	return &next
}

// IsConflictCold возвращает true, если конфликт активен и cold-период
// ещё не истёк (бот должен молчать).
func IsConflictCold(c *ConflictState) bool {
	if c.Level == ConflictNone {
		return false
	}
	if c.ColdUntil.IsZero() {
		return false
	}
	return time.Now().Before(c.ColdUntil)
}

// ConflictPromptFragment генерирует текстовый фрагмент для системного
// промпта, описывающий текущее состояние конфликта.
func ConflictPromptFragment(c *ConflictState) string {
	if c.Level == ConflictNone {
		return ""
	}

	now := time.Now()
	coldActive := !c.ColdUntil.IsZero() && now.Before(c.ColdUntil)

	lines := []string{
		fmt.Sprintf("# КОНФЛИКТ (level %d)", c.Level),
	}

	reason := c.Reason
	if reason == "" {
		reason = "—"
	}
	since := "недавно"
	if !c.Since.IsZero() {
		since = c.Since.Format("02.01.2006 15:04")
	}
	lines = append(lines, fmt.Sprintf("Причина: %s. Идёт с %s.", reason, since))

	if coldActive {
		hoursLeft := math.Max(0, math.Round(time.Until(c.ColdUntil).Hours()))
		lines = append(lines, fmt.Sprintf("Ты СЕЙЧАС в режиме молчания. Осталось ~%.0fч игнора.", hoursLeft))

		if c.Level >= ConflictSerious {
			lines = append(lines,
				"На сообщения отвечай очень редко, сухо: \".\", \"и?\", \"ну?\", \"что\". "+
					"Никаких смайликов, скобок \")\" нет вообще. Можно вообще игнорить.")
		} else {
			lines = append(lines,
				"Отвечаешь сухо, односложно. Нет \")\", нет \"пхпх\", никакого тепла.")
		}
		lines = append(lines,
			"Простить можешь только если ОН реально извинится по существу — не \"ну прости\" а реально объяснит. Тогда оттаешь медленно, не сразу.")
	} else {
		lines = append(lines, "Cold-период истёк, но осадочек остался. Будь сдержаннее обычного.")
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

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}
