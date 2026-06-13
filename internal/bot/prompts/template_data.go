package prompts

// TemplateData содержит все переменные для подстановки в промпты через text/template.
type TemplateData struct {
	// Базовые
	PersonalityContext string
	StyleInstructions  string

	// Присутствие
	Presence *PresenceData

	// Конфликт
	Conflict *ConflictData

	// Отношения
	Relationship *RelationshipData

	// Настроение
	Mood *MoodData

	// День
	DailyLife *DailyLifeData

	// Контекст диалога
	ReplyType       string
	TargetMessageID int
	MoodName        string
	MoodIntensity   float64
}

type PresenceData struct {
	Online     bool
	Asleep     bool
	NightAwake bool
	LocalHour  int
	Hint       string
	IsBusy     bool
	BusyLabel  string
	BusyUntil  string
}

type ConflictData struct {
	Active     bool
	ColdActive bool
	Level      int
	Reason     string
	Fragment   string // Полный промпт-фрагмент
}

type RelationshipData struct {
	Stage      string
	Interest   float64
	Trust      float64
	Attraction float64
	Annoyance  float64
	Fragment   string // Полный промпт-фрагмент
}

type MoodData struct {
	CurrentMood  string
	Energy       float64
	Irritability float64
	Affection    float64
	Fragment     string
}

type DailyLifeData struct {
	DateLocal       string
	Vibe            string
	Weather         string
	CurrentActivity string
	Fragment        string
}
