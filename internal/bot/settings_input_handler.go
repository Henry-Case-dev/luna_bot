package bot

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	// Необходимо импортировать tgbotapi, т.к. хелперы sendReplyAndDeleteAfter и sendReply используют типы из него
	// Это можно будет убрать в будущем при рефакторинге хелперов
	"github.com/Henry-Case-dev/luna_bot/internal/storage" // Импортируем storage
)

// parseProfileArgs разбирает строку с аргументами профиля.
// Формат: @username {пол=М/Ж} {имя=Имя Фамилия} {био=Описание...}
// Или: {пол=М/Ж} {имя=Имя Фамилия} {био=Описание...} @username
// Возвращает username, alias, gender, realName, bio и ошибку.
func parseProfileArgs(args string) (username, alias, gender, realName, bio string, err error) {
	// Ищем @username в начале или конце строки (необязательно)
	reUsername := regexp.MustCompile(`^@(\w+)\s+|\s+@(\w+)$`)
	matchesUsername := reUsername.FindStringSubmatch(args)
	if len(matchesUsername) > 1 {
		if matchesUsername[1] != "" {
			username = matchesUsername[1]
		} else {
			username = matchesUsername[2]
		}
		// Удаляем найденный @username из строки для дальнейшего парсинга
		args = reUsername.ReplaceAllString(args, "")
	}

	// Регулярное выражение для парсинга аргументов вида {key=value}
	// Используем raw string literal, поэтому экранирование не нужно
	reArgs := regexp.MustCompile(`{(\w+)=([^}]+)}`)
	matches := reArgs.FindAllStringSubmatch(args, -1)

	parsedKeys := make(map[string]string)
	for _, match := range matches {
		if len(match) == 3 {
			key := strings.ToLower(match[1])
			value := strings.TrimSpace(match[2])
			parsedKeys[key] = value
		}
	}

	// Извлекаем значения
	if val, ok := parsedKeys["пол"]; ok {
		gender = strings.ToUpper(val)
		if gender != "М" && gender != "Ж" {
			err = errors.New("неверный формат пола (должно быть М или Ж)")
			return
		}
	}

	alias = parsedKeys["alias"]  // Псевдоним
	realName = parsedKeys["имя"] // Реальное имя
	bio = parsedKeys["био"]      // Био

	// Валидация: хотя бы одно поле, кроме username, должно быть заполнено
	if alias == "" && gender == "" && realName == "" && bio == "" {
		err = errors.New("необходимо указать хотя бы одно поле профиля (alias, пол, имя или био)")
		return
	}

	// Валидация: Username обязателен, если его не удалось найти
	if username == "" {
		err = errors.New("не указан @username пользователя")
		return
	}

	return
}

// handlePendingSettingInput обрабатывает ввод пользователя для ожидаемой настройки.
// messageID - ID исходного сообщения пользователя, которое вызвало этот ввод.
func (b *Bot) handlePendingSettingInput(chatID int64, userID int64, username string, pendingSettingKey string, text string, messageID int) error {
	log.Printf("[DEBUG][InputHandler] Chat %d User %d (%s): Обработка ввода для ключа '%s'. Текст: '%s', MessageID: %d", chatID, userID, username, pendingSettingKey, text, messageID)

	// Сначала проверим на команду /cancel (в нижнем регистре для надежности)
	if strings.ToLower(text) == "/cancel" {
		log.Printf("[DEBUG][InputHandler] Chat %d User %d: Отмена ввода настройки '%s'.", chatID, userID, pendingSettingKey)
		b.settingsMutex.Lock()
		if settings, exists := b.chatSettings[chatID]; exists {
			settings.PendingSetting = "" // Сбрасываем ожидание
			// Поле PendingSettingUserID больше не существует
			// settings.PendingSettingUserID = 0
			// LastInfoMessageID будет удален в handleMessage, здесь не трогаем
		}
		b.settingsMutex.Unlock()

		// Используем sendReply и deleteMessage вместо sendReplyAndDeleteAfter
		b.sendReply(chatID, "Ввод отменен.")

		// Отложенное удаление исходного сообщения пользователя
		go func(msgID int) {
			time.Sleep(5 * time.Second)
			b.deleteMessageSilent(chatID, msgID)
		}(messageID) // Передаем ID исходного сообщения

		// Обновляем клавиатуру настроек после отмены
		go b.updateSettingsKeyboardAfterInput(chatID)
		return nil // Успешно обработали отмену
	}

	// --- Обработка ввода для конкретных настроек ---

	var operationStatus string     // Статус операции для сообщения пользователю
	var updateErr error            // Ошибка при обновлении настройки
	var needsKeyboardUpdate = true // Флаг, нужно ли обновлять клавиатуру настроек

	switch pendingSettingKey {
	case "profile_data":
		// --- Handle 'profile_data' input ---
		needsKeyboardUpdate = false // Не обновляем клавиатуру для профиля
		log.Printf("[DEBUG][InputHandler Profile] Чат %d: Обработка ввода профиля: %s", chatID, text)

		targetUsername, alias, gender, realName, bio, parseErr := parseProfileArgs(text)
		if parseErr != nil {
			log.Printf("[ERROR][InputHandler Profile] Чат %d: Ошибка парсинга данных профиля '%s': %v", chatID, text, parseErr)
			b.sendAutoDeleteErrorReply(chatID, 0, fmt.Sprintf("тупой что ли? вводи нормально: %v", parseErr))
			// Оставляем PendingSetting, чтобы пользователь мог попробовать еще раз
			return errors.New("ошибка парсинга данных профиля") // Возвращаем ошибку, но не сбрасываем PendingSetting
		}

		log.Printf("[DEBUG][InputHandler Profile] Чат %d: Распарсено: User=%s, Alias=%s, Gender=%s, RealName=%s, Bio=%s",
			chatID, targetUsername, alias, gender, realName, bio)

		// Пытаемся найти существующий профиль по username
		existingProfile, findErr := b.findUserProfileByUsername(chatID, targetUsername)
		if findErr != nil {
			log.Printf("[ERROR][InputHandler Profile] Чат %d: Ошибка поиска профиля по username '%s': %v", chatID, targetUsername, findErr)
			b.sendAutoDeleteErrorReply(chatID, 0, "что-то пошло не так")
			// Сбрасываем ожидание, так как произошла ошибка бд
			b.settingsMutex.Lock()
			if settings, exists := b.chatSettings[chatID]; exists {
				settings.PendingSetting = ""
				// settings.PendingSettingUserID = 0 // REMOVED: Field does not exist
			}
			b.settingsMutex.Unlock()
			return fmt.Errorf("ошибка поиска профиля: %w", findErr)
		}

		var profileToSave storage.UserProfile
		if existingProfile != nil {
			log.Printf("[DEBUG][InputHandler Profile] Чат %d: Найден существующий профиль для @%s (UserID: %d). Обновляем.", chatID, targetUsername, existingProfile.UserID)
			profileToSave = *existingProfile // Копируем существующий
			// Обновляем только те поля, которые были введены
			profileToSave.Alias = alias       // Всегда обновляем Alias
			profileToSave.Gender = gender     // Всегда обновляем Gender
			profileToSave.RealName = realName // Всегда обновляем RealName
			profileToSave.Bio = bio           // Всегда обновляем Bio
			// НЕ ОБНОВЛЯЕМ LastSeen при ручном редактировании - это только для автоматического отслеживания активности
		} else {
			log.Printf("[DEBUG][InputHandler Profile] Чат %d: Профиль для @%s не найден. Создаем новый.", chatID, targetUsername)
			// Пытаемся получить ID пользователя по username (может быть неточным, если пользователя нет в чате)
			foundUserID, _ := b.getUserIDByUsername(chatID, targetUsername) // Ошибка здесь игнорируется намеренно
			if foundUserID == 0 {
				log.Printf("[WARN][InputHandler Profile] Чат %d: Не удалось определить UserID для @%s. Профиль будет создан без UserID.", chatID, targetUsername)
			}
			profileToSave = storage.UserProfile{
				ChatID:   chatID,
				UserID:   foundUserID, // Может быть 0
				Username: targetUsername,
				Alias:    alias,
				Gender:   gender,
				RealName: realName,
				Bio:      bio,
				// LastSeen и другие поля будут установлены автоматически при создании или первой активности пользователя
			}
		}

		// НЕ устанавливаем LastSeen при ручном редактировании профиля
		// profileToSave.LastSeen = time.Now() // УДАЛЕНО: Не должно обновляться при ручном редактировании

		// Сохраняем профиль
		updateErr = b.storage.SetUserProfile(&profileToSave)
		if updateErr != nil {
			log.Printf("[ERROR][InputHandler Profile] Чат %d: Ошибка сохранения профиля для @%s: %v", chatID, targetUsername, updateErr)
			operationStatus = "🚫 Произошла ошибка при сохранении профиля."
		} else {
			log.Printf("[INFO][InputHandler Profile] Чат %d: Профиль для @%s успешно сохранен/обновлен.", chatID, targetUsername)
			operationStatus = fmt.Sprintf("✅ Профиль для @%s успешно сохранен/обновлен.", targetUsername)
		}

	case "direct_limit_count":
		valueInt, err := strconv.Atoi(text)
		if err != nil {
			log.Printf("[WARN][InputHandler] Chat %d: Неверный ввод для %s: '%s' - не число.", chatID, pendingSettingKey, text)
			b.sendReply(chatID, "🚫 Введите числовое значение или /cancel для отмены.")
			return errors.New("неверный ввод: ожидалось число") // Возвращаем ошибку, не сбрасываем PendingSetting
		}
		if valueInt >= 0 { // 0 means unlimited
			updateErr = b.storage.UpdateDirectLimitCount(chatID, valueInt)
			if updateErr != nil {
				log.Printf("[ERROR][InputHandler] Chat %d: Не удалось сохранить direct_limit_count %d: %v", chatID, valueInt, updateErr)
				operationStatus = "🚫 Ошибка при сохранении лимита сообщений."
			} else {
				log.Printf("[INFO][InputHandler] Chat %d: Лимит прямых сообщений установлен: %d", chatID, valueInt)
				if valueInt == 0 {
					operationStatus = "✅ Лимит прямых сообщений снят (установлен в 0)."
				} else {
					operationStatus = fmt.Sprintf("✅ Лимит прямых сообщений установлен: %d", valueInt)
				}
			}
		} else {
			b.sendAutoDeleteErrorReply(chatID, 0, "вводи нормальное число, дебил")
			return errors.New("неверный ввод: значение должно быть >= 0") // Возвращаем ошибку, не сбрасываем PendingSetting
		}

	case "direct_limit_duration":
		valueInt, err := strconv.Atoi(text)
		if err != nil {
			log.Printf("[WARN][InputHandler] Chat %d: Неверный ввод для %s: '%s' - не число.", chatID, pendingSettingKey, text)
			b.sendReply(chatID, "🚫 Введите числовое значение (в минутах) или /cancel для отмены.")
			return errors.New("неверный ввод: ожидалось число") // Возвращаем ошибку, не сбрасываем PendingSetting
		}
		if valueInt > 0 {
			duration := time.Duration(valueInt) * time.Minute
			updateErr = b.storage.UpdateDirectLimitDuration(chatID, duration) // Use correct method
			if updateErr != nil {
				log.Printf("[ERROR][InputHandler] Chat %d: Не удалось сохранить direct_limit_duration %d мин: %v", chatID, valueInt, updateErr)
				operationStatus = "🚫 Ошибка при сохранении лимита времени."
			} else {
				log.Printf("[INFO][InputHandler] Chat %d: Лимит времени прямых сообщений установлен: %d минут", chatID, valueInt)
				operationStatus = fmt.Sprintf("✅ Лимит времени прямых сообщений установлен: %d минут", valueInt)
			}
		} else {
			b.sendAutoDeleteErrorReply(chatID, 0, "минуты должны быть больше нуля, гений")
			return errors.New("неверный ввод: значение должно быть > 0") // Возвращаем ошибку, не сбрасываем PendingSetting
		}

	default:
		log.Printf("[WARN][InputHandler] Chat %d: Неизвестный ключ ожидаемой настройки: '%s'. Сбрасываем ожидание.", chatID, pendingSettingKey)
		operationStatus = "⚠️ Неизвестная настройка. Ввод отменен."
		needsKeyboardUpdate = false // Не обновляем клавиатуру, если ключ неизвестен
	}

	// --- Завершение обработки ---

	// Отправляем статус операции пользователю, если он есть
	if operationStatus != "" {
		b.sendReply(chatID, operationStatus)
	}

	// Если была ошибка при обновлении в storage, сбрасываем ожидание
	if updateErr != nil {
		log.Printf("[DEBUG][InputHandler] Chat %d: Сброс PendingSetting из-за ошибки обновления: %v", chatID, updateErr)
		b.settingsMutex.Lock()
		if settings, exists := b.chatSettings[chatID]; exists {
			settings.PendingSetting = ""
			// settings.PendingSettingUserID = 0 // REMOVED: Field does not exist
		}
		b.settingsMutex.Unlock()
		// Обновляем клавиатуру, даже если была ошибка сохранения, чтобы убрать сообщение об ожидании ввода
		go b.updateSettingsKeyboardAfterInput(chatID)
		return fmt.Errorf("ошибка обновления настройки '%s': %w", pendingSettingKey, updateErr)
	}

	// Если обработка прошла успешно (без ошибок парсинга или обновления),
	// сбрасываем состояние ожидания ввода.
	log.Printf("[DEBUG][InputHandler] Chat %d: Успешная обработка ввода для '%s'. Сброс PendingSetting.", chatID, pendingSettingKey)
	b.settingsMutex.Lock()
	if settings, exists := b.chatSettings[chatID]; exists {
		settings.PendingSetting = ""
		// settings.PendingSettingUserID = 0 // REMOVED: Field does not exist
	}
	b.settingsMutex.Unlock()

	// Обновляем клавиатуру настроек, если нужно
	if needsKeyboardUpdate {
		go b.updateSettingsKeyboardAfterInput(chatID)
	}

	// Удаляем исходное сообщение пользователя с введенными данными
	// TODO: Consider if we *always* want to delete the user's input message.
	// Maybe add a setting for this? For now, let's keep it simple and delete.
	// go b.deleteMessage(chatID, messageID) // messageID is not available here

	return nil // Успешная обработка
}
