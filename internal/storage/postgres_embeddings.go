package storage

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// formatVectorForSQL преобразует slice float32 в строку для PostgreSQL vector типа.
func formatVectorForSQL(vector []float32) string {
	if len(vector) == 0 {
		return "[]"
	}

	var result strings.Builder
	result.WriteString("[")

	for i, v := range vector {
		if i > 0 {
			result.WriteString(",")
		}
		result.WriteString(fmt.Sprintf("%f", v))
	}

	result.WriteString("]")
	return result.String()
}

// UpdateMessageEmbedding обновляет эмбеддинг для указанного сообщения.
func (ps *PostgresStorage) UpdateMessageEmbedding(chatID int64, messageID int, embedding []float32) error {
	return ps.UpdateMessageEmbeddingWithContext(chatID, messageID, embedding, "")
}

// UpdateMessageEmbeddingWithContext обновляет эмбеддинг и контекст для указанного сообщения.
func (ps *PostgresStorage) UpdateMessageEmbeddingWithContext(chatID int64, messageID int, embedding []float32, embeddingContext string) error {
	if len(embedding) == 0 {
		return fmt.Errorf("пустой эмбеддинг")
	}

	vectorStr := formatVectorForSQL(embedding)
	query := `
		UPDATE chat_messages 
		SET message_embedding = $1::vector, 
			embedding_context = $2,
			embedding_generated_at = $3
		WHERE chat_id = $4 AND message_id = $5
	`

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := ps.db.ExecContext(ctx, query, vectorStr, embeddingContext, time.Now(), chatID, messageID)
	if err != nil {
		return fmt.Errorf("ошибка обновления эмбеддинга: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("ошибка получения количества обновленных строк: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("сообщение с ID %d в чате %d не найдено", messageID, chatID)
	}

	if ps.debug {
		log.Printf("[UpdateMessageEmbedding] Обновлен эмбеддинг для сообщения %d в чате %d (размер: %d)",
			messageID, chatID, len(embedding))
	}

	return nil
}
