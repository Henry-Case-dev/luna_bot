package bot

import (
	"log"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// updateUserProfileIfNeeded проверяет, нужно ли обновить профиль пользователя,
// и создает новый профиль, если он не существует.
// Автоматически обновляет LastSeen при каждом сообщении пользователя.
func (b *Bot) updateUserProfileIfNeeded(chatID int64, user *tgbotapi.User, messageDate int) {
	if user == nil {
		log.Printf("[WARN][UpdateProfileIfNeeded] Chat %d: User is nil. Cannot update profile.", chatID)
		return
	}

	userID := user.ID

	if b.config.Debug {
		log.Printf("[DEBUG][UpdateProfileIfNeeded] Chat %d, User %d (@%s): Checking profile...", chatID, userID, user.UserName)
	}

	// 1. Проверяем, существует ли профиль в БД
	existingProfile, err := b.storage.GetUserProfile(chatID, userID)
	if err != nil {
		log.Printf("[ERROR][UpdateProfileIfNeeded] Chat %d, User %d: Ошибка при получении профиля: %v", chatID, userID, err)
		return
	}

	// 2. Если профиль существует, обновляем LastSeen (это важно для логики приложения)
	if existingProfile != nil {
		if b.config.Debug {
			log.Printf("[DEBUG][UpdateProfileIfNeeded] Chat %d, User %d (@%s): Профиль существует, обновляю LastSeen.", chatID, userID, existingProfile.Username)
		}

		// LastSeen должен автоматически обновляться при каждом сообщении пользователя
		err = b.storage.UpdateUserLastSeen(chatID, userID, time.Unix(int64(messageDate), 0))
		if err != nil {
			log.Printf("[ERROR][UpdateProfileIfNeeded] Chat %d, User %d: Ошибка при обновлении LastSeen: %v", chatID, userID, err)
		}
		return
	}

	// 3. Если профиля нет, создаем новый с правильными автоматическими полями
	if b.config.Debug {
		log.Printf("[DEBUG][UpdateProfileIfNeeded] Chat %d, User %d (@%s): Профиль не найден, создаю новый.", chatID, userID, user.UserName)
	}

	now := time.Unix(int64(messageDate), 0)
	newProfile := &storage.UserProfile{
		ChatID:   chatID,
		UserID:   userID,
		Username: user.UserName,
		Alias:    user.FirstName, // Используем FirstName как Alias по умолчанию
		// Автоматические поля устанавливаются ботом:
		LastSeen:  now, // Время последнего сообщения
		CreatedAt: now, // Время создания профиля
		UpdatedAt: now, // Время последнего изменения профиля
		// Остальные поля остаются пустыми до ручного заполнения
	}

	err = b.storage.SetUserProfile(newProfile)
	if err != nil {
		log.Printf("[ERROR][UpdateProfileIfNeeded] Chat %d, User %d: Ошибка при создании нового профиля: %v", chatID, userID, err)
		return
	}

	if b.config.Debug {
		log.Printf("[DEBUG][UpdateProfileIfNeeded] Chat %d, User %d (@%s): Новый профиль успешно создан.", chatID, userID, user.UserName)
	}
}
