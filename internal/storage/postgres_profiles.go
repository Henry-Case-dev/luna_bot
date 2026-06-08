package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// GetUserProfile возвращает профиль пользователя из PostgreSQL.
func (ps *PostgresStorage) GetUserProfile(chatID int64, userID int64) (*UserProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `SELECT username, alias, gender, real_name, bio, last_seen, created_at, updated_at, auto_bio, last_auto_bio_update
             FROM user_profiles WHERE chat_id = $1 AND user_id = $2`

	row := ps.db.QueryRowContext(ctx, query, chatID, userID)

	var profile UserProfile
	profile.ChatID = chatID
	profile.UserID = userID

	var username, alias, gender, realName, bio, autoBio sql.NullString
	var lastSeen, createdAt, updatedAt, lastAutoBioUpdate sql.NullTime

	err := row.Scan(
		&username, &alias, &gender, &realName, &bio, &lastSeen, &createdAt, &updatedAt, &autoBio, &lastAutoBioUpdate,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			if ps.debug {
				log.Printf("DEBUG: Профиль пользователя userID %d в чате %d не найден в PostgreSQL.", userID, chatID)
			}
			return nil, nil
		}
		log.Printf("ERROR: Ошибка получения профиля пользователя из PostgreSQL (чат %d, user %d): %v", chatID, userID, err)
		return nil, fmt.Errorf("ошибка запроса профиля пользователя: %w", err)
	}

	if username.Valid {
		profile.Username = username.String
	}
	if alias.Valid {
		profile.Alias = alias.String
	}
	if gender.Valid {
		profile.Gender = gender.String
	}
	if realName.Valid {
		profile.RealName = realName.String
	}
	if bio.Valid {
		profile.Bio = bio.String
	}
	if lastSeen.Valid {
		profile.LastSeen = lastSeen.Time
	}
	if createdAt.Valid {
		profile.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		profile.UpdatedAt = updatedAt.Time
	}
	if autoBio.Valid {
		profile.AutoBio = autoBio.String
	}
	if lastAutoBioUpdate.Valid {
		profile.LastAutoBioUpdate = lastAutoBioUpdate.Time
	} else {
		profile.LastAutoBioUpdate = time.Time{}
	}

	if ps.debug {
		log.Printf("DEBUG: Профиль пользователя userID %d в чате %d успешно получен из PostgreSQL.", userID, chatID)
	}
	return &profile, nil
}

// SetUserProfile создает или обновляет профиль пользователя в PostgreSQL (UPSERT).
func (ps *PostgresStorage) SetUserProfile(profile *UserProfile) error {
	if profile == nil {
		return fmt.Errorf("нельзя сохранить nil профиль")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
        INSERT INTO user_profiles (chat_id, user_id, username, alias, gender, real_name, bio, last_seen, auto_bio, last_auto_bio_update, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
        ON CONFLICT (chat_id, user_id)
        DO UPDATE SET
            username = EXCLUDED.username,
            alias = EXCLUDED.alias,
            gender = EXCLUDED.gender,
            real_name = EXCLUDED.real_name,
            bio = EXCLUDED.bio,
            last_seen = EXCLUDED.last_seen,
            auto_bio = EXCLUDED.auto_bio,
            last_auto_bio_update = EXCLUDED.last_auto_bio_update,
            updated_at = NOW();
    `

	var lastAutoBioUpdateArg sql.NullTime
	if !profile.LastAutoBioUpdate.IsZero() {
		lastAutoBioUpdateArg = sql.NullTime{Time: profile.LastAutoBioUpdate, Valid: true}
	} else {
		lastAutoBioUpdateArg = sql.NullTime{Valid: false}
	}

	_, err := ps.db.ExecContext(ctx, query,
		profile.ChatID,
		profile.UserID,
		profile.Username,
		profile.Alias,
		profile.Gender,
		profile.RealName,
		profile.Bio,
		profile.LastSeen,
		profile.AutoBio,
		lastAutoBioUpdateArg,
	)

	if err != nil {
		log.Printf("ERROR: Ошибка сохранения/обновления профиля пользователя в PostgreSQL (чат %d, user %d): %v", profile.ChatID, profile.UserID, err)
		return fmt.Errorf("ошибка сохранения профиля: %w", err)
	}

	if ps.debug {
		log.Printf("DEBUG: Профиль пользователя userID %d в чате %d успешно сохранен/обновлен в PostgreSQL.", profile.UserID, profile.ChatID)
	}
	return nil
}

// GetAllUserProfiles возвращает все профили пользователей для указанного чата из PostgreSQL.
func (ps *PostgresStorage) GetAllUserProfiles(chatID int64) ([]*UserProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	query := `SELECT user_id, username, alias, gender, real_name, bio, last_seen, created_at, updated_at, auto_bio, last_auto_bio_update
             FROM user_profiles WHERE chat_id = $1`

	rows, err := ps.db.QueryContext(ctx, query, chatID)
	if err != nil {
		log.Printf("ERROR: Ошибка получения всех профилей пользователей из PostgreSQL (чат %d): %v", chatID, err)
		return nil, fmt.Errorf("ошибка запроса профилей: %w", err)
	}
	defer rows.Close()

	var profiles []*UserProfile
	for rows.Next() {
		var profile UserProfile
		profile.ChatID = chatID

		var userID int64
		var username, alias, gender, realName, bio, autoBio sql.NullString
		var lastSeen, createdAt, updatedAt, lastAutoBioUpdate sql.NullTime

		if err := rows.Scan(
			&userID, &username, &alias, &gender, &realName, &bio, &lastSeen, &createdAt, &updatedAt, &autoBio, &lastAutoBioUpdate,
		); err != nil {
			log.Printf("ERROR: Ошибка сканирования строки профиля для чата %d: %v", chatID, err)
			continue
		}

		profile.UserID = userID
		if username.Valid {
			profile.Username = username.String
		}
		if alias.Valid {
			profile.Alias = alias.String
		}
		if gender.Valid {
			profile.Gender = gender.String
		}
		if realName.Valid {
			profile.RealName = realName.String
		}
		if bio.Valid {
			profile.Bio = bio.String
		}
		if lastSeen.Valid {
			profile.LastSeen = lastSeen.Time
		}
		if createdAt.Valid {
			profile.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			profile.UpdatedAt = updatedAt.Time
		}
		if autoBio.Valid {
			profile.AutoBio = autoBio.String
		}
		if lastAutoBioUpdate.Valid {
			profile.LastAutoBioUpdate = lastAutoBioUpdate.Time
		} else {
			profile.LastAutoBioUpdate = time.Time{}
		}

		profiles = append(profiles, &profile)
	}

	if ps.debug {
		log.Printf("DEBUG: Получено %d профилей пользователей из PostgreSQL для чата %d.", len(profiles), chatID)
	}
	return profiles, nil
}
