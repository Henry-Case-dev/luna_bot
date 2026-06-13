package storage

import (
	"database/sql"
	"fmt"
	"log"
)

// GetChatSettings получает настройки чата из PostgreSQL.
func (ps *PostgresStorage) GetChatSettings(chatID int64) (*ChatSettings, error) {
	var settings ChatSettings
	settings.ChatID = chatID

	query := `
		SELECT
			conversation_style, temperature, model, gemini_safety_threshold,
			voice_transcription_enabled, direct_reply_limit_enabled,
			direct_reply_limit_count, direct_reply_limit_duration_minutes,
			srach_analysis_enabled, photo_analysis_enabled
		FROM chat_settings
		WHERE chat_id = $1
	`
	row := ps.db.QueryRow(query, chatID)

	var style sql.NullString
	var temp sql.NullFloat64
	var model sql.NullString
	var safety sql.NullString
	var voiceEnabled sql.NullBool
	var limitEnabled sql.NullBool
	var limitCount sql.NullInt64
	var limitDuration sql.NullInt64
	var srachEnabled sql.NullBool
	var photoEnabled sql.NullBool

	err := row.Scan(
		&style, &temp, &model, &safety,
		&voiceEnabled, &limitEnabled,
		&limitCount, &limitDuration,
		&srachEnabled, &photoEnabled,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("[Postgres GetChatSettings DEBUG] Настройки для chatID %d не найдены. Возвращаю дефолтные.", chatID)
			return &settings, nil
		}
		log.Printf("[Postgres GetChatSettings ERROR] Ошибка получения настроек для chatID %d: %v", chatID, err)
		return nil, err
	}

	if style.Valid {
		settings.ConversationStyle = style.String
	}
	if temp.Valid {
		settings.Temperature = &temp.Float64
	}
	if model.Valid {
		settings.Model = model.String
	}
	if safety.Valid {
		settings.GeminiSafetyThreshold = safety.String
	}
	if voiceEnabled.Valid {
		settings.VoiceTranscriptionEnabled = &voiceEnabled.Bool
	}
	if limitEnabled.Valid {
		settings.DirectReplyLimitEnabled = &limitEnabled.Bool
	}
	if limitCount.Valid {
		count := int(limitCount.Int64)
		settings.DirectReplyLimitCount = &count
	}
	if limitDuration.Valid {
		duration := int(limitDuration.Int64)
		settings.DirectReplyLimitDuration = &duration
	}
	if srachEnabled.Valid {
		settings.SrachAnalysisEnabled = &srachEnabled.Bool
	}
	if photoEnabled.Valid {
		settings.PhotoAnalysisEnabled = &photoEnabled.Bool
	}

	if ps.debug {
		log.Printf("[Postgres GetChatSettings DEBUG] Настройки для chatID %d успешно получены.", chatID)
	}
	return &settings, nil
}

// SetChatSettings сохраняет настройки чата в PostgreSQL (UPSERT).
func (ps *PostgresStorage) SetChatSettings(settings *ChatSettings) error {
	if settings == nil {
		return fmt.Errorf("нельзя сохранить nil настройки")
	}

	query := `
		INSERT INTO chat_settings (
			chat_id, conversation_style, temperature, model, gemini_safety_threshold,
			voice_transcription_enabled, direct_reply_limit_enabled,
			direct_reply_limit_count, direct_reply_limit_duration_minutes,
			srach_analysis_enabled, photo_analysis_enabled
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (chat_id) DO UPDATE SET
			conversation_style = EXCLUDED.conversation_style,
			temperature = EXCLUDED.temperature,
			model = EXCLUDED.model,
			gemini_safety_threshold = EXCLUDED.gemini_safety_threshold,
			voice_transcription_enabled = EXCLUDED.voice_transcription_enabled,
			direct_reply_limit_enabled = EXCLUDED.direct_reply_limit_enabled,
			direct_reply_limit_count = EXCLUDED.direct_reply_limit_count,
			direct_reply_limit_duration_minutes = EXCLUDED.direct_reply_limit_duration_minutes,
			srach_analysis_enabled = EXCLUDED.srach_analysis_enabled,
			photo_analysis_enabled = EXCLUDED.photo_analysis_enabled
	`

	var temp sql.NullFloat64
	if settings.Temperature != nil {
		temp.Float64 = *settings.Temperature
		temp.Valid = true
	}
	var voiceEnabled sql.NullBool
	if settings.VoiceTranscriptionEnabled != nil {
		voiceEnabled.Bool = *settings.VoiceTranscriptionEnabled
		voiceEnabled.Valid = true
	}
	var limitEnabled sql.NullBool
	if settings.DirectReplyLimitEnabled != nil {
		limitEnabled.Bool = *settings.DirectReplyLimitEnabled
		limitEnabled.Valid = true
	}
	var limitCount sql.NullInt64
	if settings.DirectReplyLimitCount != nil {
		limitCount.Int64 = int64(*settings.DirectReplyLimitCount)
		limitCount.Valid = true
	}
	var limitDuration sql.NullInt64
	if settings.DirectReplyLimitDuration != nil {
		limitDuration.Int64 = int64(*settings.DirectReplyLimitDuration)
		limitDuration.Valid = true
	}
	var srachEnabled sql.NullBool
	if settings.SrachAnalysisEnabled != nil {
		srachEnabled.Bool = *settings.SrachAnalysisEnabled
		srachEnabled.Valid = true
	}
	var photoEnabled sql.NullBool
	if settings.PhotoAnalysisEnabled != nil {
		photoEnabled.Bool = *settings.PhotoAnalysisEnabled
		photoEnabled.Valid = true
	}

	_, err := ps.db.Exec(query,
		settings.ChatID,
		settings.ConversationStyle,
		temp,
		settings.Model,
		settings.GeminiSafetyThreshold,
		voiceEnabled,
		limitEnabled,
		limitCount,
		limitDuration,
		srachEnabled,
		photoEnabled,
	)

	if err != nil {
		log.Printf("[Postgres SetChatSettings ERROR] Ошибка сохранения настроек для chatID %d: %v", settings.ChatID, err)
		return err
	}

	if ps.debug {
		log.Printf("[Postgres SetChatSettings DEBUG] Настройки для chatID %d успешно сохранены (UPSERT).", settings.ChatID)
	}
	return nil
}
