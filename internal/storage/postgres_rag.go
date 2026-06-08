package storage

import (
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// SearchRelevantMessages — заглушка для PostgresStorage.
// PostgresStorage не поддерживает векторный поиск.
func (ps *PostgresStorage) SearchRelevantMessages(chatID int64, queryText string, k int) ([]*tgbotapi.Message, error) {
	log.Printf("[WARN][PostgresStorage] SearchRelevantMessages вызван для chatID %d, но PostgresStorage не поддерживает векторный поиск. Возвращен пустой результат.", chatID)
	return []*tgbotapi.Message{}, nil
}
