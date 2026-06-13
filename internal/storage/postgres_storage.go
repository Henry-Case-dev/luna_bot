package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
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
		sslmode = "disable"
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

	// Включаем расширение pgvector (необходимо для типа vector)
	if _, err := tx.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector;"); err != nil {
		return fmt.Errorf("ошибка включения расширения pgvector: %w", err)
	}

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

	chatSettingsQuery := `
	CREATE TABLE IF NOT EXISTS chat_settings (
		chat_id BIGINT NOT NULL,
		conversation_style TEXT DEFAULT '',
		temperature DOUBLE PRECISION DEFAULT 0.5,
		model TEXT DEFAULT '',
		gemini_safety_threshold TEXT DEFAULT '',
		voice_transcription_enabled BOOLEAN DEFAULT false,
		direct_reply_limit_enabled BOOLEAN DEFAULT false,
		direct_reply_limit_count INTEGER DEFAULT 0,
		direct_reply_limit_duration_minutes INTEGER DEFAULT 0,
		srach_analysis_enabled BOOLEAN DEFAULT false,
		photo_analysis_enabled BOOLEAN DEFAULT false,
		PRIMARY KEY (chat_id)
	);`
	if _, err := tx.ExecContext(ctx, chatSettingsQuery); err != nil {
		return fmt.Errorf("ошибка создания таблицы chat_settings: %w", err)
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

	emotionalStatesQuery := `
	CREATE TABLE IF NOT EXISTS emotional_states (
		chat_id BIGINT NOT NULL,
		joy DOUBLE PRECISION DEFAULT 0,
		sadness DOUBLE PRECISION DEFAULT 0,
		anger DOUBLE PRECISION DEFAULT 0,
		fear DOUBLE PRECISION DEFAULT 0,
		trust DOUBLE PRECISION DEFAULT 0,
		disgust DOUBLE PRECISION DEFAULT 0,
		surprise DOUBLE PRECISION DEFAULT 0,
		anticipation DOUBLE PRECISION DEFAULT 0,
		optimism DOUBLE PRECISION DEFAULT 0,
		contempt DOUBLE PRECISION DEFAULT 0,
		nostalgia DOUBLE PRECISION DEFAULT 0,
		anxiety DOUBLE PRECISION DEFAULT 0,
		aggression DOUBLE PRECISION DEFAULT 0,
		sentimentality DOUBLE PRECISION DEFAULT 0,
		curiosity DOUBLE PRECISION DEFAULT 0,
		cynicism DOUBLE PRECISION DEFAULT 0,
		uncertainty DOUBLE PRECISION DEFAULT 0,
		empathy DOUBLE PRECISION DEFAULT 0,
		irritability DOUBLE PRECISION DEFAULT 0,
		vulnerability DOUBLE PRECISION DEFAULT 0,
		confidence DOUBLE PRECISION DEFAULT 0,
		response_tendency JSONB DEFAULT '{}',
		intensity DOUBLE PRECISION DEFAULT 0,
		stability DOUBLE PRECISION DEFAULT 0,
		last_update TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		trigger_event TEXT DEFAULT '',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (chat_id)
	);`
	if _, err := tx.ExecContext(ctx, emotionalStatesQuery); err != nil {
		return fmt.Errorf("ошибка создания таблицы emotional_states: %w", err)
	}

	emotionalMemoriesQuery := `
	CREATE TABLE IF NOT EXISTS emotional_memories (
		id BIGSERIAL PRIMARY KEY,
		chat_id BIGINT NOT NULL,
		user_id BIGINT NOT NULL,
		user_context TEXT DEFAULT '',
		trigger TEXT DEFAULT '',
		primary_emotion TEXT DEFAULT '',
		emotion_intensity DOUBLE PRECISION DEFAULT 0,
		response TEXT DEFAULT '',
		outcome TEXT DEFAULT '',
		success BOOLEAN DEFAULT FALSE,
		reinforcement DOUBLE PRECISION DEFAULT 0,
		frequency INT DEFAULT 0,
		last_accessed TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		topic_context TEXT DEFAULT '',
		keywords JSONB DEFAULT '[]',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := tx.ExecContext(ctx, emotionalMemoriesQuery); err != nil {
		return fmt.Errorf("ошибка создания таблицы emotional_memories: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ошибка коммита транзакции: %w", err)
	}

	_, err = ps.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_user_profiles_chat_user ON user_profiles (chat_id, user_id);`)
	if err != nil {
		return fmt.Errorf("ошибка создания индекса idx_user_profiles_chat_user: %w", err)
	}

	_, err = ps.db.Exec(`CREATE INDEX IF NOT EXISTS idx_emotional_memories_chat_user ON emotional_memories (chat_id, user_id, created_at DESC);`)
	if err != nil {
		return fmt.Errorf("ошибка создания индекса idx_emotional_memories_chat_user: %w", err)
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

func (ps *PostgresStorage) UpdatePhotoAnalysisEnabled(chatID int64, enabled bool) error {
	return ps.updateSingleSetting(chatID, "photo_analysis_enabled", enabled)
}

// === Методы, специфичные для MongoDB (заглушки для PostgresStorage) ===

func (ps *PostgresStorage) GetTotalMessagesCount(chatID int64) (int64, error) {
	var count int64
	err := ps.db.QueryRow("SELECT COUNT(*) FROM chat_messages WHERE chat_id = $1", chatID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("ошибка подсчёта сообщений: %w", err)
	}
	return count, nil
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

// === Методы для работы с эмоциональной системой ===

// emotionalColumnWhitelist содержит допустимые имена колонок эмоций для динамических UPDATE-запросов.
var emotionalColumnWhitelist = map[string]bool{
	"joy": true, "sadness": true, "anger": true, "fear": true,
	"trust": true, "disgust": true, "surprise": true, "anticipation": true,
	"optimism": true, "contempt": true, "nostalgia": true, "anxiety": true,
	"aggression": true, "sentimentality": true, "curiosity": true, "cynicism": true,
	"uncertainty": true, "empathy": true, "irritability": true, "vulnerability": true,
	"confidence": true,
}

func (ps *PostgresStorage) GetEmotionalState(chatID int64) (*EmotionalState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `SELECT chat_id, joy, sadness, anger, fear, trust, disgust, surprise, anticipation,
		optimism, contempt, nostalgia, anxiety, aggression, sentimentality, curiosity, cynicism,
		uncertainty, empathy, irritability, vulnerability, confidence,
		response_tendency, intensity, stability, last_update, trigger_event, created_at, updated_at
		FROM emotional_states WHERE chat_id = $1`

	state := &EmotionalState{}
	var responseTendencyJSON sql.NullString

	err := ps.db.QueryRowContext(ctx, query, chatID).Scan(
		&state.ChatID,
		&state.Joy, &state.Sadness, &state.Anger, &state.Fear,
		&state.Trust, &state.Disgust, &state.Surprise, &state.Anticipation,
		&state.Optimism, &state.Contempt, &state.Nostalgia, &state.Anxiety,
		&state.Aggression, &state.Sentimentality, &state.Curiosity, &state.Cynicism,
		&state.Uncertainty, &state.Empathy, &state.Irritability, &state.Vulnerability,
		&state.Confidence,
		&responseTendencyJSON,
		&state.Intensity, &state.Stability, &state.LastUpdate, &state.TriggerEvent,
		&state.CreatedAt, &state.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		log.Printf("[Postgres GetEmotionalState ERROR] chatID %d: %v", chatID, err)
		return nil, fmt.Errorf("ошибка получения эмоционального состояния для chatID %d: %w", chatID, err)
	}

	if responseTendencyJSON.Valid {
		if err := json.Unmarshal([]byte(responseTendencyJSON.String), &state.ResponseTendency); err != nil {
			log.Printf("[Postgres GetEmotionalState WARN] chatID %d: ошибка десериализации response_tendency: %v", chatID, err)
			state.ResponseTendency = make(map[string]float64)
		}
	} else {
		state.ResponseTendency = make(map[string]float64)
	}

	if ps.debug {
		log.Printf("[Postgres GetEmotionalState DEBUG] chatID %d: состояние получено (intensity=%.2f)", chatID, state.Intensity)
	}
	return state, nil
}

func (ps *PostgresStorage) SaveEmotionalState(state *EmotionalState) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	responseTendencyJSON := jsonify(state.ResponseTendency)
	now := time.Now()
	if state.CreatedAt.IsZero() {
		state.CreatedAt = now
	}
	state.UpdatedAt = now
	if state.LastUpdate.IsZero() {
		state.LastUpdate = now
	}

	query := `INSERT INTO emotional_states (
		chat_id, joy, sadness, anger, fear, trust, disgust, surprise, anticipation,
		optimism, contempt, nostalgia, anxiety, aggression, sentimentality, curiosity, cynicism,
		uncertainty, empathy, irritability, vulnerability, confidence,
		response_tendency, intensity, stability, last_update, trigger_event, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29)
	ON CONFLICT (chat_id) DO UPDATE SET
		joy = EXCLUDED.joy, sadness = EXCLUDED.sadness, anger = EXCLUDED.anger,
		fear = EXCLUDED.fear, trust = EXCLUDED.trust, disgust = EXCLUDED.disgust,
		surprise = EXCLUDED.surprise, anticipation = EXCLUDED.anticipation,
		optimism = EXCLUDED.optimism, contempt = EXCLUDED.contempt,
		nostalgia = EXCLUDED.nostalgia, anxiety = EXCLUDED.anxiety,
		aggression = EXCLUDED.aggression, sentimentality = EXCLUDED.sentimentality,
		curiosity = EXCLUDED.curiosity, cynicism = EXCLUDED.cynicism,
		uncertainty = EXCLUDED.uncertainty, empathy = EXCLUDED.empathy,
		irritability = EXCLUDED.irritability, vulnerability = EXCLUDED.vulnerability,
		confidence = EXCLUDED.confidence,
		response_tendency = EXCLUDED.response_tendency,
		intensity = EXCLUDED.intensity, stability = EXCLUDED.stability,
		last_update = EXCLUDED.last_update, trigger_event = EXCLUDED.trigger_event,
		updated_at = EXCLUDED.updated_at
	`

	_, err := ps.db.ExecContext(ctx, query,
		state.ChatID,
		state.Joy, state.Sadness, state.Anger, state.Fear,
		state.Trust, state.Disgust, state.Surprise, state.Anticipation,
		state.Optimism, state.Contempt, state.Nostalgia, state.Anxiety,
		state.Aggression, state.Sentimentality, state.Curiosity, state.Cynicism,
		state.Uncertainty, state.Empathy, state.Irritability, state.Vulnerability,
		state.Confidence,
		responseTendencyJSON,
		state.Intensity, state.Stability, state.LastUpdate, state.TriggerEvent,
		state.CreatedAt, state.UpdatedAt,
	)
	if err != nil {
		log.Printf("[Postgres SaveEmotionalState ERROR] chatID %d: %v", state.ChatID, err)
		return fmt.Errorf("ошибка сохранения эмоционального состояния для chatID %d: %w", state.ChatID, err)
	}

	if ps.debug {
		log.Printf("[Postgres SaveEmotionalState DEBUG] chatID %d: состояние сохранено (intensity=%.2f)", state.ChatID, state.Intensity)
	}
	return nil
}

func (ps *PostgresStorage) UpdateEmotionalState(chatID int64, emotions map[string]float64, intensity float64, triggerEvent string) error {
	if len(emotions) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := ps.db.ExecContext(ctx, `INSERT INTO emotional_states (chat_id) VALUES ($1) ON CONFLICT (chat_id) DO NOTHING`, chatID)
	if err != nil {
		log.Printf("[Postgres UpdateEmotionalState ERROR] chatID %d: ошибка обеспечения существования строки: %v", chatID, err)
	}

	var setClauses []string
	var args []interface{}
	argIdx := 1

	for col, val := range emotions {
		if !emotionalColumnWhitelist[col] {
			log.Printf("[Postgres UpdateEmotionalState WARN] chatID %d: пропущена неизвестная колонка эмоции '%s'", chatID, col)
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, argIdx))
		args = append(args, val)
		argIdx++
	}

	if len(setClauses) == 0 {
		return nil
	}

	setClauses = append(setClauses, fmt.Sprintf("intensity = $%d", argIdx))
	args = append(args, intensity)
	argIdx++

	setClauses = append(setClauses, fmt.Sprintf("trigger_event = $%d", argIdx))
	args = append(args, triggerEvent)
	argIdx++

	setClauses = append(setClauses, fmt.Sprintf("last_update = $%d", argIdx))
	args = append(args, time.Now())
	argIdx++

	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now())
	argIdx++

	updateQuery := fmt.Sprintf(`UPDATE emotional_states SET %s WHERE chat_id = $%d`,
		strings.Join(setClauses, ", "), argIdx)
	args = append(args, chatID)

	_, err = ps.db.ExecContext(ctx, updateQuery, args...)
	if err != nil {
		log.Printf("[Postgres UpdateEmotionalState ERROR] chatID %d: %v", chatID, err)
		return fmt.Errorf("ошибка обновления эмоционального состояния для chatID %d: %w", chatID, err)
	}

	if ps.debug {
		log.Printf("[Postgres UpdateEmotionalState DEBUG] chatID %d: обновлены эмоции (intensity=%.2f, trigger=%s)", chatID, intensity, triggerEvent)
	}
	return nil
}

func (ps *PostgresStorage) AddEmotionalMemory(memory *EmotionalMemory) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	keywordsJSON := jsonify(memory.Keywords)
	now := time.Now()
	if memory.CreatedAt.IsZero() {
		memory.CreatedAt = now
	}
	memory.UpdatedAt = now
	if memory.LastAccessed.IsZero() {
		memory.LastAccessed = now
	}

	query := `INSERT INTO emotional_memories (
		chat_id, user_id, user_context, trigger, primary_emotion, emotion_intensity,
		response, outcome, success, reinforcement, frequency, last_accessed,
		topic_context, keywords, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`

	_, err := ps.db.ExecContext(ctx, query,
		memory.ChatID, memory.UserID, memory.UserContext, memory.Trigger,
		memory.PrimaryEmotion, memory.EmotionIntensity, memory.Response,
		memory.Outcome, memory.Success, memory.Reinforcement, memory.Frequency,
		memory.LastAccessed, memory.TopicContext, keywordsJSON,
		memory.CreatedAt, memory.UpdatedAt,
	)
	if err != nil {
		log.Printf("[Postgres AddEmotionalMemory ERROR] chatID %d userID %d: %v", memory.ChatID, memory.UserID, err)
		return fmt.Errorf("ошибка добавления эмоциональной памяти: %w", err)
	}

	if ps.debug {
		log.Printf("[Postgres AddEmotionalMemory DEBUG] chatID %d userID %d: память добавлена (emotion=%s, intensity=%.2f)", memory.ChatID, memory.UserID, memory.PrimaryEmotion, memory.EmotionIntensity)
	}
	return nil
}

func (ps *PostgresStorage) GetEmotionalMemories(chatID int64, userID int64, limit int) ([]*EmotionalMemory, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `SELECT id, chat_id, user_id, user_context, trigger, primary_emotion, emotion_intensity,
		response, outcome, success, reinforcement, frequency, last_accessed,
		topic_context, keywords, created_at, updated_at
		FROM emotional_memories
		WHERE chat_id = $1 AND user_id = $2
		ORDER BY created_at DESC LIMIT $3`

	rows, err := ps.db.QueryContext(ctx, query, chatID, userID, limit)
	if err != nil {
		log.Printf("[Postgres GetEmotionalMemories ERROR] chatID %d userID %d: %v", chatID, userID, err)
		return nil, fmt.Errorf("ошибка получения эмоциональных воспоминаний: %w", err)
	}
	defer rows.Close()

	var memories []*EmotionalMemory
	for rows.Next() {
		var mem EmotionalMemory
		var keywordsJSON sql.NullString
		err := rows.Scan(
			&mem.ID, &mem.ChatID, &mem.UserID, &mem.UserContext, &mem.Trigger,
			&mem.PrimaryEmotion, &mem.EmotionIntensity, &mem.Response,
			&mem.Outcome, &mem.Success, &mem.Reinforcement, &mem.Frequency,
			&mem.LastAccessed, &mem.TopicContext, &keywordsJSON,
			&mem.CreatedAt, &mem.UpdatedAt,
		)
		if err != nil {
			log.Printf("[Postgres GetEmotionalMemories ERROR] chatID %d userID %d: ошибка сканирования строки: %v", chatID, userID, err)
			return nil, fmt.Errorf("ошибка сканирования эмоционального воспоминания: %w", err)
		}
		if keywordsJSON.Valid {
			if err := json.Unmarshal([]byte(keywordsJSON.String), &mem.Keywords); err != nil {
				log.Printf("[Postgres GetEmotionalMemories WARN] chatID %d userID %d: ошибка десериализации keywords: %v", chatID, userID, err)
				mem.Keywords = []string{}
			}
		} else {
			mem.Keywords = []string{}
		}
		memories = append(memories, &mem)
	}

	if err := rows.Err(); err != nil {
		log.Printf("[Postgres GetEmotionalMemories ERROR] chatID %d userID %d: ошибка итерации строк: %v", chatID, userID, err)
		return nil, fmt.Errorf("ошибка итерации эмоциональных воспоминаний: %w", err)
	}

	if ps.debug {
		log.Printf("[Postgres GetEmotionalMemories DEBUG] chatID %d userID %d: получено %d воспоминаний", chatID, userID, len(memories))
	}
	return memories, nil
}

func (ps *PostgresStorage) GetEmotionalMemoriesByEmotion(chatID int64, emotion string, limit int) ([]*EmotionalMemory, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `SELECT id, chat_id, user_id, user_context, trigger, primary_emotion, emotion_intensity,
		response, outcome, success, reinforcement, frequency, last_accessed,
		topic_context, keywords, created_at, updated_at
		FROM emotional_memories
		WHERE chat_id = $1 AND primary_emotion = $2
		ORDER BY created_at DESC LIMIT $3`

	rows, err := ps.db.QueryContext(ctx, query, chatID, emotion, limit)
	if err != nil {
		log.Printf("[Postgres GetEmotionalMemoriesByEmotion ERROR] chatID %d emotion %s: %v", chatID, emotion, err)
		return nil, fmt.Errorf("ошибка получения эмоциональных воспоминаний по эмоции: %w", err)
	}
	defer rows.Close()

	var memories []*EmotionalMemory
	for rows.Next() {
		var mem EmotionalMemory
		var keywordsJSON sql.NullString
		err := rows.Scan(
			&mem.ID, &mem.ChatID, &mem.UserID, &mem.UserContext, &mem.Trigger,
			&mem.PrimaryEmotion, &mem.EmotionIntensity, &mem.Response,
			&mem.Outcome, &mem.Success, &mem.Reinforcement, &mem.Frequency,
			&mem.LastAccessed, &mem.TopicContext, &keywordsJSON,
			&mem.CreatedAt, &mem.UpdatedAt,
		)
		if err != nil {
			log.Printf("[Postgres GetEmotionalMemoriesByEmotion ERROR] chatID %d: ошибка сканирования строки: %v", chatID, err)
			return nil, fmt.Errorf("ошибка сканирования эмоционального воспоминания: %w", err)
		}
		if keywordsJSON.Valid {
			if err := json.Unmarshal([]byte(keywordsJSON.String), &mem.Keywords); err != nil {
				log.Printf("[Postgres GetEmotionalMemoriesByEmotion WARN] chatID %d: ошибка десериализации keywords: %v", chatID, err)
				mem.Keywords = []string{}
			}
		} else {
			mem.Keywords = []string{}
		}
		memories = append(memories, &mem)
	}

	if err := rows.Err(); err != nil {
		log.Printf("[Postgres GetEmotionalMemoriesByEmotion ERROR] chatID %d: ошибка итерации строк: %v", chatID, err)
		return nil, fmt.Errorf("ошибка итерации эмоциональных воспоминаний: %w", err)
	}

	if ps.debug {
		log.Printf("[Postgres GetEmotionalMemoriesByEmotion DEBUG] chatID %d emotion %s: получено %d воспоминаний", chatID, emotion, len(memories))
	}
	return memories, nil
}

func (ps *PostgresStorage) UpdateEmotionalMemory(memory *EmotionalMemory) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	keywordsJSON := jsonify(memory.Keywords)
	memory.UpdatedAt = time.Now()

	query := `UPDATE emotional_memories SET
		chat_id = $1, user_id = $2, user_context = $3, trigger = $4,
		primary_emotion = $5, emotion_intensity = $6, response = $7,
		outcome = $8, success = $9, reinforcement = $10, frequency = $11,
		last_accessed = $12, topic_context = $13, keywords = $14, updated_at = $15
		WHERE id = $16`

	result, err := ps.db.ExecContext(ctx, query,
		memory.ChatID, memory.UserID, memory.UserContext, memory.Trigger,
		memory.PrimaryEmotion, memory.EmotionIntensity, memory.Response,
		memory.Outcome, memory.Success, memory.Reinforcement, memory.Frequency,
		memory.LastAccessed, memory.TopicContext, keywordsJSON,
		memory.UpdatedAt, memory.ID,
	)
	if err != nil {
		log.Printf("[Postgres UpdateEmotionalMemory ERROR] id %d: %v", memory.ID, err)
		return fmt.Errorf("ошибка обновления эмоциональной памяти id %d: %w", memory.ID, err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		log.Printf("[Postgres UpdateEmotionalMemory WARN] id %d: запись не найдена", memory.ID)
		return fmt.Errorf("эмоциональная память с id %d не найдена", memory.ID)
	}

	if ps.debug {
		log.Printf("[Postgres UpdateEmotionalMemory DEBUG] id %d: память обновлена (emotion=%s)", memory.ID, memory.PrimaryEmotion)
	}
	return nil
}

func (ps *PostgresStorage) CleanupEmotionalMemories(chatID int64, maxAge time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cutoff := time.Now().Add(-maxAge)
	query := `DELETE FROM emotional_memories WHERE chat_id = $1 AND created_at < $2`

	result, err := ps.db.ExecContext(ctx, query, chatID, cutoff)
	if err != nil {
		log.Printf("[Postgres CleanupEmotionalMemories ERROR] chatID %d: %v", chatID, err)
		return fmt.Errorf("ошибка очистки эмоциональных воспоминаний для chatID %d: %w", chatID, err)
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("[Postgres CleanupEmotionalMemories] chatID %d: удалено %d старых воспоминаний (maxAge=%v)", chatID, rowsAffected, maxAge)
	return nil
}

func (ps *PostgresStorage) GetEmotionalTrends(chatID int64, since time.Time, limit int) (map[string]float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `SELECT primary_emotion, AVG(emotion_intensity) as avg_intensity
		FROM emotional_memories
		WHERE chat_id = $1 AND created_at >= $2
		GROUP BY primary_emotion
		ORDER BY avg_intensity DESC LIMIT $3`

	rows, err := ps.db.QueryContext(ctx, query, chatID, since, limit)
	if err != nil {
		log.Printf("[Postgres GetEmotionalTrends ERROR] chatID %d: %v", chatID, err)
		return nil, fmt.Errorf("ошибка получения эмоциональных трендов для chatID %d: %w", chatID, err)
	}
	defer rows.Close()

	trends := make(map[string]float64)
	for rows.Next() {
		var emotion string
		var avgIntensity float64
		if err := rows.Scan(&emotion, &avgIntensity); err != nil {
			log.Printf("[Postgres GetEmotionalTrends ERROR] chatID %d: ошибка сканирования строки: %v", chatID, err)
			return nil, fmt.Errorf("ошибка сканирования тренда: %w", err)
		}
		trends[emotion] = avgIntensity
	}

	if err := rows.Err(); err != nil {
		log.Printf("[Postgres GetEmotionalTrends ERROR] chatID %d: ошибка итерации строк: %v", chatID, err)
		return nil, fmt.Errorf("ошибка итерации трендов: %w", err)
	}

	if ps.debug {
		log.Printf("[Postgres GetEmotionalTrends DEBUG] chatID %d: получено %d трендов с %v", chatID, len(trends), since)
	}
	return trends, nil
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
