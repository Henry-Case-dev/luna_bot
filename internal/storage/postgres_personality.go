package storage

import (
	"time"
)

// === Методы для работы с PersonalityMemory ===

// GetPersonalityMemory возвращает заглушку PersonalityMemory для PostgresStorage.
func (ps *PostgresStorage) GetPersonalityMemory(chatID int64) (*PersonalityMemory, error) {
	return &PersonalityMemory{
		ChatID:            chatID,
		NameMentions:      map[string]bool{},
		RecentTopics:      []string{},
		SelfPerception:    []string{},
		DiscussionContext: make(map[string]bool),
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}, nil
}

// SavePersonalityMemory — заглушка для PostgresStorage.
func (ps *PostgresStorage) SavePersonalityMemory(memory *PersonalityMemory) error {
	return nil
}

// UpdatePersonalityField — заглушка для PostgresStorage.
func (ps *PostgresStorage) UpdatePersonalityField(chatID int64, fieldName string, value interface{}) error {
	return nil
}

// === Методы для работы с настроением бота (MoodState) ===

// GetMoodState возвращает заглушку MoodState для PostgresStorage.
func (ps *PostgresStorage) GetMoodState(chatID int64) (*MoodState, error) {
	return &MoodState{
		ChatID:         chatID,
		CurrentMood:    "neutral",
		MoodIntensity:  0.5,
		LastMoodUpdate: time.Now(),
		TriggerReason:  "Default mood state for PostgresStorage",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}, nil
}

// SaveMoodState — заглушка для PostgresStorage.
func (ps *PostgresStorage) SaveMoodState(mood *MoodState) error {
	return nil
}

// UpdateMoodState — заглушка для PostgresStorage.
func (ps *PostgresStorage) UpdateMoodState(chatID int64, currentMood string, moodIntensity float64, triggerReason string) error {
	return nil
}
