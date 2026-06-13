package state

import "fmt"

// StageId идентификатор стадии отношений.
type StageId string

const (
	StageMetIrlGotTg     StageId = "met-irl-got-tg"
	StageTgGivenCold     StageId = "tg-given-cold"
	StageTgGivenWarming  StageId = "tg-given-warming"
	StageConvinced       StageId = "convinced"
	StageFirstDateDone   StageId = "first-date-done"
	StageDatingEarly     StageId = "dating-early"
	StageDatingStable    StageId = "dating-stable"
	StageLongTerm        StageId = "long-term"
	StageDumped          StageId = "dumped"
)

// StageOrder — порядок стадий (без dumped, он терминальный).
var StageOrder = []StageId{
	StageMetIrlGotTg,
	StageTgGivenCold,
	StageTgGivenWarming,
	StageConvinced,
	StageFirstDateDone,
	StageDatingEarly,
	StageDatingStable,
	StageLongTerm,
}

func stageIndex(id StageId) int {
	for i, s := range StageOrder {
		if s == id {
			return i
		}
	}
	return -1
}

// StageTransitionContext — контекст для принятия решения о смене стадии.
type StageTransitionContext struct {
	CurrentStage      StageId
	Score             *RelationshipScore
	HerMessagesInStage  int
	HisMessagesInStage  int
	IgnoresInStage    int
	HasActiveConflict bool
}

// StageTransitionResult — результат решения о переходе.
type StageTransitionResult struct {
	Next      StageId
	Reason    string
	Direction string // "up" или "down"
}

// DecideStageTransition решает, нужно ли передвинуть стадию.
// Возвращает nil если стадия должна остаться той же.
func DecideStageTransition(ctx *StageTransitionContext) *StageTransitionResult {
	if ctx == nil || ctx.Score == nil {
		return nil
	}

	// "dumped" — терминальная стадия, автоматически из неё не выходим
	if ctx.CurrentStage == StageDumped {
		return nil
	}

	idx := stageIndex(ctx.CurrentStage)
	if idx < 0 {
		return nil
	}

	// Сначала проверяем ПОНИЖЕНИЕ
	if reason := wantsDowngrade(ctx); reason != "" && idx > 0 {
		return &StageTransitionResult{
			Next:      StageOrder[idx-1],
			Reason:    reason,
			Direction: "down",
		}
	}

	// Затем ПОВЫШЕНИЕ (не повышаем во время конфликта)
	if ctx.HasActiveConflict {
		return nil
	}

	if reason := wantsUpgrade(ctx); reason != "" && idx < len(StageOrder)-1 {
		return &StageTransitionResult{
			Next:      StageOrder[idx+1],
			Reason:    reason,
			Direction: "up",
		}
	}

	return nil
}

func wantsDowngrade(ctx *StageTransitionContext) string {
	s := ctx.Score

	// Условие: annoyance высокий, interest/trust сильно просели, >= 8 её сообщений
	if s.Annoyance >= 60 && s.Interest <= -10 && s.Trust <= 10 && ctx.HerMessagesInStage >= 8 {
		return fmt.Sprintf("annoyance %.0f, interest %.0f, trust %.0f — отношения регрессируют",
			s.Annoyance, s.Interest, s.Trust)
	}

	// Игноры на тёплой стадии — признак деградации
	warmStages := map[StageId]bool{
		StageConvinced:     true,
		StageFirstDateDone: true,
		StageDatingEarly:   true,
		StageDatingStable:  true,
		StageLongTerm:      true,
	}
	if warmStages[ctx.CurrentStage] &&
		ctx.IgnoresInStage >= 12 &&
		ctx.HisMessagesInStage > 0 &&
		float64(ctx.IgnoresInStage) >= float64(ctx.HisMessagesInStage)*0.7 &&
		s.Interest < 20 {
		return fmt.Sprintf("%d игноров за стадию из %d его сообщений — теряет интерес",
			ctx.IgnoresInStage, ctx.HisMessagesInStage)
	}

	return ""
}

func wantsUpgrade(ctx *StageTransitionContext) string {
	s := ctx.Score

	// Минимум 6 сообщений от неё перед повышением
	const minHer = 6
	if ctx.HerMessagesInStage < minHer {
		return ""
	}

	switch ctx.CurrentStage {
	case StageMetIrlGotTg:
		if s.Interest >= 30 && s.Attraction >= 20 && s.Annoyance < 20 {
			return fmt.Sprintf("interest %.0f, attraction %.0f — оттаяла", s.Interest, s.Attraction)
		}
	case StageTgGivenCold:
		if s.Interest >= 25 && s.Trust >= 10 && s.Annoyance < 25 {
			return fmt.Sprintf("interest %.0f, trust %.0f — стала отвечать осторожно", s.Interest, s.Trust)
		}
	case StageTgGivenWarming:
		if s.Interest >= 40 && s.Trust >= 25 && s.Attraction >= 30 && s.Annoyance < 20 {
			return fmt.Sprintf("interest %.0f, trust %.0f, attraction %.0f — стабильно общается",
				s.Interest, s.Trust, s.Attraction)
		}
	case StageConvinced:
		if ctx.HerMessagesInStage >= 10 && s.Attraction >= 50 && s.Trust >= 35 && s.Interest >= 50 {
			return fmt.Sprintf("attraction %.0f, trust %.0f — пошли на первое свидание", s.Attraction, s.Trust)
		}
	case StageFirstDateDone:
		if ctx.HerMessagesInStage >= 12 && s.Attraction >= 65 && s.Trust >= 50 && s.Interest >= 60 {
			return fmt.Sprintf("attraction %.0f, trust %.0f — отношения завязались", s.Attraction, s.Trust)
		}
	case StageDatingEarly:
		if ctx.HerMessagesInStage >= 25 && s.Trust >= 70 && s.Attraction >= 65 && s.Annoyance < 15 {
			return fmt.Sprintf("trust %.0f, attraction %.0f, %d сообщений — стабильная пара",
				s.Trust, s.Attraction, ctx.HerMessagesInStage)
		}
	case StageDatingStable:
		if ctx.HerMessagesInStage >= 60 && s.Trust >= 80 && s.Interest >= 55 {
			return fmt.Sprintf("trust %.0f, %d сообщений — давно вместе", s.Trust, ctx.HerMessagesInStage)
		}
	}

	return ""
}

// UpdateRelationshipScore добавляет дельту к текущему скору и возвращает обновлённый скор.
func UpdateRelationshipScore(score *RelationshipScore, delta *RelationshipScore) *RelationshipScore {
	if score == nil || delta == nil {
		return score
	}
	return &RelationshipScore{
		Interest:   score.Interest + delta.Interest,
		Trust:      score.Trust + delta.Trust,
		Attraction: score.Attraction + delta.Attraction,
		Annoyance:  score.Annoyance + delta.Annoyance,
		Cringe:     score.Cringe + delta.Cringe,
	}
}

// RelationshipPromptFragment генерирует текстовый фрагмент для системного
// промпта, описывающий текущую стадию и скор отношений.
func RelationshipPromptFragment(stage StageId, score *RelationshipScore) string {
	label := stageLabel(stage)
	desc := stageDescription(stage)

	lines := []string{
		fmt.Sprintf("# СТАДИЯ ОТНОШЕНИЙ: %s", label),
		fmt.Sprintf("Стадия: %s", StageIdName(stage)),
		desc,
	}

	if score != nil {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("Скор: интерес=%.0f, доверие=%.0f, влечение=%.0f, раздражение=%.0f, кринж=%.0f",
			score.Interest, score.Trust, score.Attraction, score.Annoyance, score.Cringe))
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

// StageIdName возвращает строковое имя стадии (для логов и промптов).
func StageIdName(stage StageId) string {
	names := map[StageId]string{
		StageMetIrlGotTg:    "met-irl-got-tg",
		StageTgGivenCold:    "tg-given-cold",
		StageTgGivenWarming: "tg-given-warming",
		StageConvinced:      "convinced",
		StageFirstDateDone:  "first-date-done",
		StageDatingEarly:    "dating-early",
		StageDatingStable:   "dating-stable",
		StageLongTerm:       "long-term",
		StageDumped:         "dumped",
	}
	if n, ok := names[stage]; ok {
		return n
	}
	return fmt.Sprintf("unknown-%s", stage)
}

func stageLabel(stage StageId) string {
	labels := map[StageId]string{
		StageMetIrlGotTg:    "Встретились в реале — дала тг",
		StageTgGivenCold:    "Дала тг, но не убедил отвечать",
		StageTgGivenWarming: "Дала тг, отвечает осторожно",
		StageConvinced:      "Убедил отвечать стабильно",
		StageFirstDateDone:  "Сходили один раз",
		StageDatingEarly:    "Только начали встречаться",
		StageDatingStable:   "Пара, общаетесь свободно",
		StageLongTerm:       "Давно вместе",
		StageDumped:         "Отшила (служебное)",
	}
	if l, ok := labels[stage]; ok {
		return l
	}
	return StageIdName(stage)
}

func stageDescription(stage StageId) string {
	descs := map[StageId]string{
		StageMetIrlGotTg:    "Только что обменялись тг. Помнит лицо, голос. Лёгкий интерес.",
		StageTgGivenCold:    "Сомневается. Часто игнорит, отвечает односложно. Нужно добиваться.",
		StageTgGivenWarming: "Оттаивает. Отвечает, но коротко. Тестит тебя.",
		StageConvinced:      "Общаетесь регулярно, флиртует, ещё не виделись после знакомства.",
		StageFirstDateDone:  "Первое свидание было, в подвешенном состоянии — нравится, но не пара.",
		StageDatingEarly:    "Около месяца вместе. Бабочки, всё внове, но границы ещё хрупкие.",
		StageDatingStable:   "Стабильные отношения, шутки, бытовуха, доверие.",
		StageLongTerm:       "Год+ вместе. Иногда раздражение, рутина, глубокое доверие.",
		StageDumped:         "Не отвечает. Снимается командой :reset.",
	}
	if d, ok := descs[stage]; ok {
		return d
	}
	return ""
}

// ShouldCheckStageTransition возвращает true, если пора проверять переход
// (каждые 5 сообщений после последней проверки).
func ShouldCheckStageTransition(messagesSinceLastCheck int) bool {
	return messagesSinceLastCheck > 0 && messagesSinceLastCheck%5 == 0
}
