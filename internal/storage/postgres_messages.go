package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// AddMessage добавляет одно сообщение в базу данных PostgreSQL.
func (ps *PostgresStorage) AddMessage(chatID int64, message *tgbotapi.Message) {
	if message == nil {
		log.Printf("[PostgresStorage AddMessage WARN] Попытка добавить nil сообщение для chatID %d", chatID)
		return
	}

	// Debug logging skipped (ps.debug flag is not implemented)

	// Подготовка данных для вставки
	var userID int64
	var username, firstName, lastName string
	var isBot bool
	if message.From != nil {
		userID = message.From.ID
		username = message.From.UserName
		firstName = message.From.FirstName
		lastName = message.From.LastName
		isBot = message.From.IsBot
	}

	var replyToMessageID sql.NullInt64
	if message.ReplyToMessage != nil {
		replyToMessageID.Int64 = int64(message.ReplyToMessage.MessageID)
		replyToMessageID.Valid = true
	} else {
		replyToMessageID.Valid = false
	}

	// Информация о пересылке
	var isForward sql.NullBool
	var forwardedFromUserID, forwardedFromChatID sql.NullInt64
	var forwardedFromMessageID sql.NullInt32
	var forwardedDate sql.NullTime

	if message.ForwardDate != 0 {
		isForward.Bool = true
		isForward.Valid = true
		forwardedDate.Time = time.Unix(int64(message.ForwardDate), 0)
		forwardedDate.Valid = true
		forwardedFromMessageID.Int32 = int32(message.ForwardFromMessageID)
		forwardedFromMessageID.Valid = true

		if message.ForwardFrom != nil {
			forwardedFromUserID.Int64 = message.ForwardFrom.ID
			forwardedFromUserID.Valid = true
		} else if message.ForwardFromChat != nil {
			forwardedFromChatID.Int64 = message.ForwardFromChat.ID
			forwardedFromChatID.Valid = true
		}
	} else {
		isForward.Bool = false
		isForward.Valid = true
	}

	entitiesJSON := jsonify(message.Entities)
	rawMessageJSON := jsonify(message)

	query := `
	INSERT INTO chat_messages (
		chat_id, message_id, user_id, username, first_name, last_name, is_bot,
		message_text, message_date, reply_to_message_id, entities, raw_message,
		is_forward, forwarded_from_user_id, forwarded_from_chat_id, forwarded_from_message_id, forwarded_date
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	ON CONFLICT (chat_id, message_id) DO UPDATE SET
		user_id = EXCLUDED.user_id,
		username = EXCLUDED.username,
		first_name = EXCLUDED.first_name,
		last_name = EXCLUDED.last_name,
		is_bot = EXCLUDED.is_bot,
		message_text = EXCLUDED.message_text,
		message_date = EXCLUDED.message_date,
		reply_to_message_id = EXCLUDED.reply_to_message_id,
		entities = EXCLUDED.entities,
		raw_message = EXCLUDED.raw_message,
		is_forward = EXCLUDED.is_forward,
		forwarded_from_user_id = EXCLUDED.forwarded_from_user_id,
		forwarded_from_chat_id = EXCLUDED.forwarded_from_chat_id,
		forwarded_from_message_id = EXCLUDED.forwarded_from_message_id,
		forwarded_date = EXCLUDED.forwarded_date;
	`
	ctx := context.Background()

	_, err := ps.db.ExecContext(ctx, query,
		chatID,
		message.MessageID,
		userID,
		username,
		firstName,
		lastName,
		isBot,
		message.Text,
		time.Unix(int64(message.Date), 0),
		replyToMessageID,
		entitiesJSON,
		rawMessageJSON,
		isForward,
		forwardedFromUserID,
		forwardedFromChatID,
		forwardedFromMessageID,
		forwardedDate,
	)

	if err != nil {
		log.Printf("[PostgresStorage AddMessage ERROR] Ошибка добавления/обновления сообщения %d для chatID %d: %v", message.MessageID, chatID, err)
	}
}

// AddVoiceTranscriptionMessage добавляет расшифровку голосового сообщения.
func (ps *PostgresStorage) AddVoiceTranscriptionMessage(chatID int64, transcriptionMessage *tgbotapi.Message, originalVoiceUserID int64) {
	ps.AddMessage(chatID, transcriptionMessage)
}

// GetMessages извлекает последние N сообщений для указанного чата.
func (ps *PostgresStorage) GetMessages(chatID int64, limit int) ([]*tgbotapi.Message, error) {
	query := `
	SELECT
		message_id, user_id, username, first_name, last_name, is_bot,
		message_text, message_date, reply_to_message_id, entities, raw_message,
		is_forward, forwarded_from_user_id, forwarded_from_chat_id, forwarded_from_message_id, forwarded_date
	FROM chat_messages
	WHERE chat_id = $1
	ORDER BY message_date DESC
	LIMIT $2;
	`
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := ps.db.QueryContext(ctx, query, chatID, limit)
	if err != nil {
		log.Printf("[PostgresStorage GetMessages ERROR] Ошибка запроса сообщений для chatID %d: %v", chatID, err)
		return nil, err
	}
	defer rows.Close()

	messages := make([]*tgbotapi.Message, 0)
	for rows.Next() {
		var msg tgbotapi.Message
		var userID sql.NullInt64
		var username, firstName, lastName sql.NullString
		var isBot sql.NullBool
		var messageText sql.NullString
		var messageDate time.Time
		var replyToMessageID sql.NullInt32
		var entitiesJSON []byte
		var rawMessageJSON []byte
		var isForward sql.NullBool
		var forwardedFromUserID, forwardedFromChatID sql.NullInt64
		var forwardedFromMessageID sql.NullInt32
		var forwardedDate sql.NullTime

		err := rows.Scan(
			&msg.MessageID, &userID, &username, &firstName, &lastName, &isBot,
			&messageText, &messageDate, &replyToMessageID, &entitiesJSON, &rawMessageJSON,
			&isForward, &forwardedFromUserID, &forwardedFromChatID, &forwardedFromMessageID, &forwardedDate,
		)
		if err != nil {
			log.Printf("[PostgresStorage GetMessages ERROR] Ошибка сканирования строки сообщения для chatID %d: %v", chatID, err)
			continue
		}

		if len(rawMessageJSON) > 0 {
			err = json.Unmarshal(rawMessageJSON, &msg)
			if err == nil {
				msg.Chat = &tgbotapi.Chat{ID: chatID}
				messages = append(messages, &msg)
				continue
			}
			log.Printf("[PostgresStorage GetMessages WARNING] Ошибка десериализации raw_message для сообщения %d chatID %d: %v. Восстанавливаем вручную.", msg.MessageID, chatID, err)
		}

		msg.Chat = &tgbotapi.Chat{ID: chatID}
		msg.Date = int(messageDate.Unix())
		msg.Text = messageText.String

		if userID.Valid {
			msg.From = &tgbotapi.User{
				ID:        userID.Int64,
				UserName:  username.String,
				FirstName: firstName.String,
				LastName:  lastName.String,
				IsBot:     isBot.Bool,
			}
		}

		if replyToMessageID.Valid {
			msg.ReplyToMessage = &tgbotapi.Message{MessageID: int(replyToMessageID.Int32)}
		}

		if len(entitiesJSON) > 0 {
			err = json.Unmarshal(entitiesJSON, &msg.Entities)
			if err != nil {
				log.Printf("[PostgresStorage GetMessages WARNING] Ошибка десериализации entities для сообщения %d chatID %d: %v", msg.MessageID, chatID, err)
			}
		}

		if isForward.Valid && isForward.Bool {
			msg.ForwardDate = int(forwardedDate.Time.Unix())
			msg.ForwardFromMessageID = int(forwardedFromMessageID.Int32)
			if forwardedFromUserID.Valid {
				msg.ForwardFrom = &tgbotapi.User{ID: forwardedFromUserID.Int64}
			} else if forwardedFromChatID.Valid {
				msg.ForwardFromChat = &tgbotapi.Chat{ID: forwardedFromChatID.Int64}
			}
		}

		messages = append(messages, &msg)
	}

	if err = rows.Err(); err != nil {
		log.Printf("[PostgresStorage GetMessages ERROR] Ошибка итерации по строкам сообщений для chatID %d: %v", chatID, err)
		return nil, err
	}

	return messages, nil
}

// GetMessagesSince извлекает сообщения из указанного чата, начиная с определенного времени.
func (ps *PostgresStorage) GetMessagesSince(ctx context.Context, chatID int64, userID int64, since time.Time, limit int) ([]*tgbotapi.Message, error) {
	args := []interface{}{chatID, since}
	query := `
	SELECT
		message_id, user_id, username, first_name, last_name, is_bot,
		message_text, message_date, reply_to_message_id, entities, raw_message,
		is_forward, forwarded_from_user_id, forwarded_from_chat_id, forwarded_from_message_id, forwarded_date
	FROM chat_messages
	WHERE chat_id = $1 AND message_date >= $2`

	if userID != 0 {
		query += fmt.Sprintf(" AND user_id = $%d", len(args)+1)
		args = append(args, userID)
	}

	query += " ORDER BY message_date DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", len(args)+1)
		args = append(args, limit)
	}
	query += ";"

	rows, err := ps.db.QueryContext(ctx, query, args...)
	if err != nil {
		log.Printf("[PostgresStorage GetMessagesSince ERROR] Чат %d, User %d: Ошибка запроса сообщений (since %v, limit %d): %v", chatID, userID, since, limit, err)
		return nil, err
	}
	defer rows.Close()

	messages := make([]*tgbotapi.Message, 0)
	for rows.Next() {
		var msg tgbotapi.Message
		var dbUserID sql.NullInt64
		var username, firstName, lastName sql.NullString
		var isBot sql.NullBool
		var messageText sql.NullString
		var messageDate time.Time
		var replyToMessageID sql.NullInt32
		var entitiesJSON []byte
		var rawMessageJSON []byte
		var isForward sql.NullBool
		var forwardedFromUserID, forwardedFromChatID sql.NullInt64
		var forwardedFromMessageID sql.NullInt32
		var forwardedDate sql.NullTime

		err := rows.Scan(
			&msg.MessageID, &dbUserID, &username, &firstName, &lastName, &isBot,
			&messageText, &messageDate, &replyToMessageID, &entitiesJSON, &rawMessageJSON,
			&isForward, &forwardedFromUserID, &forwardedFromChatID, &forwardedFromMessageID, &forwardedDate,
		)
		if err != nil {
			log.Printf("[PostgresStorage GetMessagesSince ERROR] Ошибка сканирования строки сообщения для chatID %d: %v", chatID, err)
			continue
		}

		if len(rawMessageJSON) > 0 {
			err = json.Unmarshal(rawMessageJSON, &msg)
			if err == nil {
				msg.Chat = &tgbotapi.Chat{ID: chatID}
				messages = append(messages, &msg)
				continue
			}
			log.Printf("[PostgresStorage GetMessagesSince WARNING] Ошибка десериализации raw_message для сообщения %d chatID %d: %v. Восстанавливаем вручную.", msg.MessageID, chatID, err)
		}

		msg.Chat = &tgbotapi.Chat{ID: chatID}
		msg.Date = int(messageDate.Unix())
		msg.Text = messageText.String

		if dbUserID.Valid {
			msg.From = &tgbotapi.User{
				ID:        dbUserID.Int64,
				UserName:  username.String,
				FirstName: firstName.String,
				LastName:  lastName.String,
				IsBot:     isBot.Bool,
			}
		}

		if replyToMessageID.Valid {
			msg.ReplyToMessage = &tgbotapi.Message{MessageID: int(replyToMessageID.Int32)}
		}

		if len(entitiesJSON) > 0 {
			err = json.Unmarshal(entitiesJSON, &msg.Entities)
			if err != nil {
				log.Printf("[PostgresStorage GetMessagesSince WARNING] Ошибка десериализации entities для сообщения %d chatID %d: %v", msg.MessageID, chatID, err)
			}
		}

		if isForward.Valid && isForward.Bool {
			msg.ForwardDate = int(forwardedDate.Time.Unix())
			msg.ForwardFromMessageID = int(forwardedFromMessageID.Int32)
			if forwardedFromUserID.Valid {
				msg.ForwardFrom = &tgbotapi.User{ID: forwardedFromUserID.Int64}
			} else if forwardedFromChatID.Valid {
				msg.ForwardFromChat = &tgbotapi.Chat{ID: forwardedFromChatID.Int64}
			}
		}

		messages = append(messages, &msg)
	}

	if err = rows.Err(); err != nil {
		log.Printf("[PostgresStorage GetMessagesSince ERROR] Ошибка итерации по строкам сообщений для chatID %d: %v", chatID, err)
		return nil, err
	}

	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	if ps.debug {
		log.Printf("[PostgresStorage GetMessagesSince DEBUG] Чат %d, User %d: Успешно получено %d сообщений (since %v, limit %d).", chatID, userID, len(messages), since, limit)
	}

	return messages, nil
}

// LoadChatHistory для PostgresStorage возвращает nil (история всегда в БД).
func (ps *PostgresStorage) LoadChatHistory(chatID int64) ([]*tgbotapi.Message, error) {
	log.Printf("[PostgresStorage STUB] LoadChatHistory вызван для chatID %d (возвращает nil)", chatID)
	return nil, nil
}

// SaveChatHistory для PostgresStorage ничего не делает (сохранение в AddMessage).
func (ps *PostgresStorage) SaveChatHistory(chatID int64) error {
	return nil
}

// ClearChatHistory удаляет все сообщения для указанного чата.
func (ps *PostgresStorage) ClearChatHistory(chatID int64) error {
	query := `DELETE FROM chat_messages WHERE chat_id = $1;`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := ps.db.ExecContext(ctx, query, chatID)
	if err != nil {
		log.Printf("[PostgresStorage Clear ERROR] Ошибка удаления истории для chatID %d: %v", chatID, err)
		return fmt.Errorf("ошибка удаления истории чата %d: %w", chatID, err)
	}

	rowsAffected, _ := result.RowsAffected()
	if ps.debug {
		log.Printf("[PostgresStorage Clear DEBUG] Удалена история для chatID %d (%d строк).", chatID, rowsAffected)
	}

	return nil
}

// AddMessagesToContext для PostgresStorage ничего не делает.
func (ps *PostgresStorage) AddMessagesToContext(chatID int64, messages []*tgbotapi.Message) {
	// No-op for PostgreSQL
}

// GetAllChatIDs возвращает список уникальных ID чатов.
func (ps *PostgresStorage) GetAllChatIDs() ([]int64, error) {
	query := `SELECT DISTINCT chat_id FROM chat_messages;`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := ps.db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("[PostgresStorage GetAllChatIDs ERROR] Ошибка запроса ID чатов: %v", err)
		return nil, fmt.Errorf("ошибка получения списка chatID: %w", err)
	}
	defer rows.Close()

	var chatIDs []int64
	for rows.Next() {
		var chatID int64
		if err := rows.Scan(&chatID); err != nil {
			log.Printf("[PostgresStorage GetAllChatIDs ERROR] Ошибка сканирования chatID: %v", err)
			continue
		}
		chatIDs = append(chatIDs, chatID)
	}

	if err := rows.Err(); err != nil {
		log.Printf("[PostgresStorage GetAllChatIDs ERROR] Ошибка после итерации по строкам: %v", err)
	}

	if ps.debug {
		log.Printf("[PostgresStorage GetAllChatIDs DEBUG] Найдено %d уникальных ChatID.", len(chatIDs))
	}

	return chatIDs, nil
}

// GetMessageByID получает конкретное сообщение по его ID.
func (ps *PostgresStorage) GetMessageByID(chatID int64, messageID int) (*tgbotapi.Message, error) {
	messages, err := ps.GetMessages(chatID, -1)
	if err != nil {
		return nil, err
	}
	for _, msg := range messages {
		if msg.MessageID == messageID {
			return msg, nil
		}
	}
	return nil, fmt.Errorf("сообщение %d не найдено в чате %d", messageID, chatID)
}

// GetReplyChain - заглушка для PostgresStorage.
func (ps *PostgresStorage) GetReplyChain(ctx context.Context, chatID int64, messageID int, maxDepth int) ([]*tgbotapi.Message, error) {
	log.Printf("[PostgresStorage WARN] GetReplyChain не реализован для PostgreSQL.")
	return nil, nil
}

// GetDailySummariesForWeek - заглушка для PostgresStorage.
func (ps *PostgresStorage) GetDailySummariesForWeek(ctx context.Context, chatID int64, botUserID int64, since time.Time, until time.Time) ([]*tgbotapi.Message, error) {
	log.Printf("[GetDailySummariesForWeek WARN] Чат %d: PostgresStorage не поддерживает получение ежедневных саммари", chatID)
	return []*tgbotapi.Message{}, nil
}

// GetMessagesInRange - заглушка для PostgresStorage.
func (ps *PostgresStorage) GetMessagesInRange(ctx context.Context, chatID int64, userID int64, since time.Time, until time.Time, limit int) ([]*tgbotapi.Message, error) {
	log.Printf("[PostgresStorage WARN] GetMessagesInRange не реализован для PostgresStorage. Будет возвращен пустой список.")
	return []*tgbotapi.Message{}, nil
}

// EnsureTotalDBSizeWithinLimit - заглушка для PostgresStorage.
func (ps *PostgresStorage) EnsureTotalDBSizeWithinLimit(cfg *config.Config) (bool, error) {
	log.Printf("[PostgresStorage WARN] EnsureTotalDBSizeWithinLimit не реализован для PostgresStorage.")
	return false, nil
}
