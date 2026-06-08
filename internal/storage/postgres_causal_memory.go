package storage

import (
	"fmt"
)

// === Методы для работы с казуальной памятью (заглушки для PostgresStorage) ===

// GetCausalMemory — заглушка для PostgresStorage.
func (ps *PostgresStorage) GetCausalMemory(chatID int64) (*CausalMemory, error) {
	return nil, fmt.Errorf("CausalMemory не реализована для PostgresStorage")
}

// SaveCausalMemory — заглушка для PostgresStorage.
func (ps *PostgresStorage) SaveCausalMemory(memory *CausalMemory) error {
	return fmt.Errorf("SaveCausalMemory не реализован для PostgresStorage")
}

// AddCausalEntry — заглушка для PostgresStorage.
func (ps *PostgresStorage) AddCausalEntry(entry *CausalMemoryEntry) error {
	return fmt.Errorf("AddCausalEntry не реализован для PostgresStorage")
}

// GetCausalEntries — заглушка для PostgresStorage.
func (ps *PostgresStorage) GetCausalEntries(chatID int64, options CausalQueryOptions) ([]*CausalMemoryEntry, error) {
	return nil, fmt.Errorf("GetCausalEntries не реализован для PostgresStorage")
}

// UpdateCausalEntry — заглушка для PostgresStorage.
func (ps *PostgresStorage) UpdateCausalEntry(entry *CausalMemoryEntry) error {
	return fmt.Errorf("UpdateCausalEntry не реализован для PostgresStorage")
}

// DeleteCausalEntry — заглушка для PostgresStorage.
func (ps *PostgresStorage) DeleteCausalEntry(chatID int64, entryID int64) error {
	return fmt.Errorf("DeleteCausalEntry не реализован для PostgresStorage")
}

// CleanupCausalMemory — заглушка для PostgresStorage.
func (ps *PostgresStorage) CleanupCausalMemory(chatID int64) error {
	return fmt.Errorf("CleanupCausalMemory не реализован для PostgresStorage")
}

// SearchCausalEntries — заглушка для PostgresStorage.
func (ps *PostgresStorage) SearchCausalEntries(chatID int64, keywords []string, limit int) ([]*CausalMemoryEntry, error) {
	return nil, fmt.Errorf("SearchCausalEntries не реализован для PostgresStorage")
}

// GetCausalEntriesByCategory — заглушка для PostgresStorage.
func (ps *PostgresStorage) GetCausalEntriesByCategory(chatID int64, category string, limit int) ([]*CausalMemoryEntry, error) {
	return nil, fmt.Errorf("GetCausalEntriesByCategory не реализован для PostgresStorage")
}

// UpdateCausalEntryRelevance — заглушка для PostgresStorage.
func (ps *PostgresStorage) UpdateCausalEntryRelevance(chatID int64, entryID int64, newRelevance float64) error {
	return fmt.Errorf("UpdateCausalEntryRelevance не реализован для PostgresStorage")
}
