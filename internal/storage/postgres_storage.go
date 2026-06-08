package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// Убедимся, что PostgresStorage реализует интерфейс ChatHistoryStorage
var _ ChatHistoryStorage = (*PostgresStorage)(nil)

// PostgresStorage реализует ChatHistoryStorage с использованием PostgreSQL.
type PostgresStorage struct {
	db            *sql.DB
	contextWindow int
	debug         bool
}

// NewPostgresStorage создает и инициализирует новый экземпляр PostgresStorage.
func NewPostgresStorage(dbHost, dbPort, dbUser, dbPassword, dbName string, contextWindow int, debug bool) (*PostgresStorage, error) {
	sslmode := os.Getenv("POSTGRESQL_SSLMODE")
	if sslmode == "" {
		sslmode = "require"
	}
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		dbHost, dbPort, dbUser, dbPassword, dbName, sslmode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("ошибка открытия соединения с PostgreSQL: %w", err)
	}

	// Проверяем соединение
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("не удалось подключиться к PostgreSQL: %w", err)
	}

	storage := &PostgresStorage{
		db:            db,
		contextWindow: contextWindow,
		debug:         debug,
	}

	if err := storage.createTablesIfNotExists(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ошибка создания таблиц PostgreSQL: %w", err)
	}

	log.Println("Таблицы PostgreSQL проверены/созданы.")
	return storage, nil
}

// createTablesIfNotExists создает необходимые таблицы в базе данных, если они не существуют.
func (ps *PostgresStorage) createTablesIfNotExists() error {
	ctx := context.Background()
	tx, err := ps.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ошибка начала транзакции: %w", err)
	}
	defer tx.Rollback()

	chatMessagesQuery := `
	CREATE TABLE IF NOT EXISTS chat_messages (
		chat_id BIGINT NOT NULL,
		message_id INT NOT NULL,
		user_id BIGINT,
		username VARCHAR(255),
		first_name VARCHAR(255),
		last_name VARCHAR(255),
		is_bot BOOLEAN,
		message_text TEXT,
		message_date TIMESTAMP WITH TIME ZONE NOT NULL,
		reply_to_message_id INT,
		entities JSONB,
		raw_message JSONB,
		is_forward BOOLEAN DEFAULT FALSE,
		forwarded_from_user_id BIGINT,
		forwarded_from_chat_id BIGINT,
		forwarded_from_message_id INT,
		forwarded_date TIMESTAMP WITH TIME ZONE,
		message_embedding vector(768),
		embedding_context TEXT,
		embedding_generated_at TIMESTAMP WITH TIME ZONE,
		PRIMARY KEY (chat_id, message_id)
	);
	CREATE INDEX IF NOT EXISTS idx_chat_messages_chat_id_date ON chat_messages (chat_id, message_date DESC);
	CREATE INDEX IF NOT EXISTS idx_chat_messages_user_id ON chat_messages (user_id);`
	if _, err := tx.ExecContext(ctx, chatMessagesQuery); err != nil {
		return fmt.Errorf("ошибка создания таблицы chat_messages: %w", err)
	}

	profilesTableQuery := `
	CREATE TABLE IF NOT EXISTS user_profiles (
		chat_id BIGINT NOT NULL,
		user_id BIGINT NOT NULL,
		username VARCHAR(255),
		alias VARCHAR(255) DEFAULT '',
		gender VARCHAR(50) DEFAULT '',
		real_name TEXT DEFAULT '',
		bio TEXT DEFAULT '',
		last_seen TIMESTAMP WITH TIME ZONE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		auto_bio TEXT,
		last_auto_bio_update TIMESTAMP WITH TIME ZONE,
		PRIMARY KEY (chat_id, user_id)
	);`
	if _, err := tx.ExecContext(ctx, profilesTableQuery); err != nil {
		return fmt.Errorf("ошибка создания таблицы user_profiles: %w", err)
	}

	triggerFunctionQuery := `
	CREATE OR REPLACE FUNCTION update_updated_at_column()
	RETURNS TRIGGER AS $$
	BEGIN
	   NEW.updated_at = NOW();
	   RETURN NEW;
	END;
	$$ language 'plpgsql';`
	if _, err := tx.ExecContext(ctx, triggerFunctionQuery); err != nil {
		return fmt.Errorf("ошибка создания триггерной функции update_updated_at_column: %w", err)
	}

	triggerQuery := `
	DROP TRIGGER IF EXISTS update_user_profiles_updated_at ON user_profiles;
	CREATE TRIGGER update_user_profiles_updated_at
	BEFORE UPDATE ON user_profiles
	FOR EACH ROW
	EXECUTE FUNCTION update_updated_at_column();`
	if _, err := tx.ExecContext(ctx, triggerQuery); err != nil {
		return fmt.Errorf("ошибка создания триггера для user_profiles: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ошибка коммита транзакции: %w", err)
	}

	_, err = ps.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_user_profiles_chat_user ON user_profiles (chat_id, user_id);`)
	if err != nil {
		return fmt.Errorf("ошибка создания индекса idx_user_profiles_chat_user: %w", err)
	}

	vectorIndexQuery := `CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chat_messages_embedding 
	ON chat_messages USING hnsw (message_embedding vector_cosine_ops);`
	if _, err := ps.db.Exec(vectorIndexQuery); err != nil {
		log.Printf("[WARN] Не удалось создать векторный индекс (возможно, pgvector не установлен): %v", err)
	} else {
		log.Println("Векторный индекс для эмбеддингов проверен/создан.")
	}

	return nil
}

// Close закрывает соединение с базой данных.
func (ps *PostgresStorage) Close() error {
	if ps.db != nil {
		log.Println("Закрытие соединения с PostgreSQL...")
		return ps.db.Close()
	}
	return nil
}

// Вспомогательная функция для безопасного получения строкового представления JSON
func jsonify(v interface{}) sql.NullString {
	if v == nil {
		return sql.NullString{}
	}
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("Ошибка JSON маршалинга для БД: %v", err)
		return sql.NullString{}
	}
	return sql.NullString{
		String: string(data),
		Valid:  true,
	}
}

// GetStatus возвращает статус хранилища PostgreSQL.
func (ps *PostgresStorage) GetStatus(chatID int64) string {
	status := "Хранилище: PostgreSQL. "
	var msgCount, profileCount int64

	errMsgs := ps.db.QueryRow("SELECT COUNT(*) FROM chat_messages WHERE chat_id = $1", chatID).Scan(&msgCount)
	if errMsgs != nil && errMsgs != sql.ErrNoRows {
		log.Printf("[Postgres GetStatus WARN] Чат %d: Ошибка получения количества сообщений: %v", chatID, errMsgs)
		status += "Ошибка подсчета сообщений. "
	} else {
		status += fmt.Sprintf("Сообщений в базе: %d. ", msgCount)
	}

	errProfiles := ps.db.QueryRow("SELECT COUNT(*) FROM user_profiles WHERE chat_id = $1", chatID).Scan(&profileCount)
	if errProfiles != nil && errProfiles != sql.ErrNoRows {
		log.Printf("[Postgres GetStatus WARN] Чат %d: Ошибка получения количества профилей: %v", chatID, errProfiles)
		status += "Ошибка подсчета профилей."
	} else {
		status += fmt.Sprintf("Профилей в базе: %d.", profileCount)
	}

	return status
}

// === Associative Memory Graph (stubs) ===
func (ps *PostgresStorage) GetAssocTopForContext(chatID int64, contextKeys []string, limit int, freshnessDays int, types []string) ([]*AssocNode, []*AssocEdge, error) {
	log.Printf("[PostgresStorage WARNING] Associative graph not implemented; returning empty result")
	return []*AssocNode{}, []*AssocEdge{}, nil
}

func (ps *PostgresStorage) UpdateAssocGraph(chatID int64, updates *AssocUpdateBatch) error {
	return nil
}

// === Методы для настроек чата ===

func (ps *PostgresStorage) updateSingleSetting(chatID int64, columnName string, value interface{}) error {
	query := fmt.Sprintf(`
		INSERT INTO chat_settings (chat_id, %s)
		VALUES ($1, $2)
		ON CONFLICT (chat_id) DO UPDATE SET
			%s = EXCLUDED.%s
	`, columnName, columnName, columnName)

	_, err := ps.db.Exec(query, chatID, value)
	if err != nil {
		log.Printf("[Postgres updateSingleSetting ERROR] Ошибка обновления '%s' для chatID %d: %v", columnName, chatID, err)
		return fmt.Errorf("ошибка обновления настройки '%s': %w", columnName, err)
	}
	if ps.debug {
		log.Printf("[Postgres updateSingleSetting DEBUG] Настройка '%s' для chatID %d успешно обновлена.", columnName, chatID)
	}
	return nil
}

// UpdateDirectLimitEnabled обновляет только поле direct_reply_limit_enabled
func (ps *PostgresStorage) UpdateDirectLimitEnabled(chatID int64, enabled bool) error {
	return ps.updateSingleSetting(chatID, "direct_reply_limit_enabled", enabled)
}

// UpdateDirectLimitCount обновляет только поле direct_reply_limit_count
func (ps *PostgresStorage) UpdateDirectLimitCount(chatID int64, count int) error {
	if count < 0 {
		return fmt.Errorf("количество должно быть не отрицательным")
	}
	return ps.updateSingleSetting(chatID, "direct_reply_limit_count", int64(count))
}

// UpdateDirectLimitDuration обновляет только поле direct_reply_limit_duration_minutes
func (ps *PostgresStorage) UpdateDirectLimitDuration(chatID int64, duration time.Duration) error {
	if duration <= 0 {
		return fmt.Errorf("длительность должна быть положительной")
	}
	durationMinutes := int(duration.Minutes())
	return ps.updateSingleSetting(chatID, "direct_reply_limit_duration_minutes", int64(durationMinutes))
}

func (ps *PostgresStorage) UpdateVoiceTranscriptionEnabled(chatID int64, enabled bool) error {
	return ps.updateSingleSetting(chatID, "voice_transcription_enabled", enabled)
}

func (ps *PostgresStorage) UpdateSrachAnalysisEnabled(chatID int64, enabled bool) error {
	return ps.updateSingleSetting(chatID, "srach_analysis_enabled", enabled)
}

// === Методы, специфичные для MongoDB (заглушки для PostgresStorage) ===

func (ps *PostgresStorage) GetTotalMessagesCount(chatID int64) (int64, error) {
	log.Printf("[WARN][PostgresStorage] GetTotalMessagesCount вызван для chatID %d, но PostgresStorage не поддерживает эту операцию.", chatID)
	return 0, fmt.Errorf("GetTotalMessagesCount не поддерживается PostgresStorage")
}

func (ps *PostgresStorage) FindMessagesWithoutEmbedding(chatID int64, limit int, skipMessageIDs []int) ([]MongoMessage, error) {
	log.Printf("[WARN][PostgresStorage] FindMessagesWithoutEmbedding вызван для chatID %d (лимит %d, пропуск %d ID), но PostgresStorage не поддерживает эту операцию.", chatID, limit, len(skipMessageIDs))
	return nil, fmt.Errorf("FindMessagesWithoutEmbedding не поддерживается PostgresStorage")
}

// === Методы для работы с долгосрочной памятью ===

func (ps *PostgresStorage) ResetAutoBioTimestamps(chatID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	query := `UPDATE user_profiles SET last_auto_bio_update = NULL WHERE chat_id = $1`
	result, err := ps.db.ExecContext(ctx, query, chatID)
	if err != nil {
		log.Printf("[ERROR][ResetAutoBio] Chat %d: Ошибка сброса времени AutoBio в PostgreSQL: %v", chatID, err)
		return fmt.Errorf("ошибка сброса времени AutoBio в PostgreSQL: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if ps.debug {
		log.Printf("[DEBUG][ResetAutoBio] Chat %d: Успешно сброшено время AutoBio для %d профилей в PostgreSQL.", chatID, rowsAffected)
	}

	return nil
}

func (ps *PostgresStorage) UpdateAutoBio(ctx context.Context, chatID int64, userID int64, autoBio string, updateTime time.Time) error {
	query := `
		UPDATE user_profiles
		SET auto_bio = $1, last_auto_bio_update = $2, updated_at = $3
		WHERE chat_id = $4 AND user_id = $5`

	ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	_, err := ps.db.ExecContext(ctxTimeout, query, autoBio, updateTime, updateTime, chatID, userID)
	if err != nil {
		log.Printf("[ERROR][UpdateAutoBio] Chat %d, User %d: Ошибка обновления AutoBio в PostgreSQL: %v", chatID, userID, err)
		return fmt.Errorf("ошибка обновления AutoBio в PostgreSQL для chatID %d, userID %d: %w", chatID, userID, err)
	}

	return nil
}

func (ps *PostgresStorage) UpdateUserLastSeen(chatID int64, userID int64, lastSeen time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		UPDATE user_profiles
		SET last_seen = $1, updated_at = $2
		WHERE chat_id = $3 AND user_id = $4`

	_, err := ps.db.ExecContext(ctx, query, lastSeen, time.Now(), chatID, userID)
	if err != nil {
		log.Printf("[ERROR][UpdateUserLastSeen] Chat %d, User %d: Ошибка обновления LastSeen в PostgreSQL: %v", chatID, userID, err)
		return fmt.Errorf("ошибка обновления LastSeen в PostgreSQL для chatID %d, userID %d: %w", chatID, userID, err)
	}

	return nil
}

// === Методы для работы с реакциями (заглушки для PostgresStorage) ===

func (ps *PostgresStorage) UpdateMessageReactions(chatID int64, messageID int, userID int64, username, firstName string, reactions []string) error {
	log.Printf("[WARN][PostgresStorage] UpdateMessageReactions не реализован для PostgreSQL.")
	return fmt.Errorf("UpdateMessageReactions не реализован для PostgresStorage")
}

func (ps *PostgresStorage) GetMessageReactions(chatID int64, messageID int) ([]string, error) {
	log.Printf("[WARN][PostgresStorage] GetMessageReactions не реализован для PostgreSQL.")
	return nil, fmt.Errorf("GetMessageReactions не реализован для PostgresStorage")
}

func (ps *PostgresStorage) GetBotMessagesWithReactions(chatID int64, lookbackHours int) ([]MongoMessage, error) {
	log.Printf("[WARN][PostgresStorage] GetBotMessagesWithReactions не реализован для PostgreSQL.")
	return nil, fmt.Errorf("GetBotMessagesWithReactions не реализован для PostgresStorage")
}

// === Методы для работы с примерами хороших/плохих сообщений ===

func (ps *PostgresStorage) AddPositiveExample(chatID int64, message string, timestamp time.Time) error {
	log.Printf("[WARN][PostgresStorage] AddPositiveExample не реализован для PostgreSQL.")
	return fmt.Errorf("AddPositiveExample не реализован для PostgresStorage")
}

func (ps *PostgresStorage) AddNegativeExample(chatID int64, message string, timestamp time.Time) error {
	log.Printf("[WARN][PostgresStorage] AddNegativeExample не реализован для PostgreSQL.")
	return fmt.Errorf("AddNegativeExample не реализован для PostgresStorage")
}

// === Методы для работы с эмоциональной системой (заглушки для PostgreSQL) ===

func (ps *PostgresStorage) GetEmotionalState(chatID int64) (*EmotionalState, error) {
	log.Printf("[PostgresStorage WARN] GetEmotionalState не реализован для PostgreSQL")
	return nil, nil
}

func (ps *PostgresStorage) SaveEmotionalState(state *EmotionalState) error {
	log.Printf("[PostgresStorage WARN] SaveEmotionalState не реализован для PostgreSQL")
	return nil
}

func (ps *PostgresStorage) UpdateEmotionalState(chatID int64, emotions map[string]float64, intensity float64, triggerEvent string) error {
	log.Printf("[PostgresStorage WARN] UpdateEmotionalState не реализован для PostgreSQL")
	return nil
}

func (ps *PostgresStorage) AddEmotionalMemory(memory *EmotionalMemory) error {
	log.Printf("[PostgresStorage WARN] AddEmotionalMemory не реализован для PostgreSQL")
	return nil
}

func (ps *PostgresStorage) GetEmotionalMemories(chatID int64, userID int64, limit int) ([]*EmotionalMemory, error) {
	log.Printf("[PostgresStorage WARN] GetEmotionalMemories не реализован для PostgreSQL")
	return nil, nil
}

func (ps *PostgresStorage) GetEmotionalMemoriesByEmotion(chatID int64, emotion string, limit int) ([]*EmotionalMemory, error) {
	log.Printf("[PostgresStorage WARN] GetEmotionalMemoriesByEmotion не реализован для PostgreSQL")
	return nil, nil
}

func (ps *PostgresStorage) UpdateEmotionalMemory(memory *EmotionalMemory) error {
	log.Printf("[PostgresStorage WARN] UpdateEmotionalMemory не реализован для PostgreSQL")
	return nil
}

func (ps *PostgresStorage) CleanupEmotionalMemories(chatID int64, maxAge time.Duration) error {
	log.Printf("[PostgresStorage WARN] CleanupEmotionalMemories не реализован для PostgreSQL")
	return nil
}

func (ps *PostgresStorage) GetEmotionalTrends(chatID int64, since time.Time, limit int) (map[string]float64, error) {
	log.Printf("[PostgresStorage WARN] GetEmotionalTrends не реализован для PostgreSQL")
	return nil, nil
}

// === Заглушки для MongoDB-специфичных методов (совместимость) ===

// GetMongoMessageByID — заглушка для совместимости.
func (ps *PostgresStorage) GetMongoMessageByID(chatID int64, messageID int) (*MongoMessage, error) {
	return nil, fmt.Errorf("GetMongoMessageByID не реализован для PostgresStorage")
}

// MarkMessageAsSummary — заглушка для совместимости.
func (ps *PostgresStorage) MarkMessageAsSummary(chatID int64, messageID int, summary bool, weeklySummary bool) error {
	return nil
}
