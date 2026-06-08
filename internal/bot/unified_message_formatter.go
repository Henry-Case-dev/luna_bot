package bot

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// UnifiedMessageFormatter - унифицированный форматтер сообщений для всего проекта
type UnifiedMessageFormatter struct {
	storage  storage.ChatHistoryStorage
	timeZone string
}

// NewUnifiedMessageFormatter создает новый экземпляр унифицированного форматтера
func NewUnifiedMessageFormatter(storage storage.ChatHistoryStorage, timeZone string) *UnifiedMessageFormatter {
	return &UnifiedMessageFormatter{
		storage:  storage,
		timeZone: timeZone,
	}
}

// FormatMessages - основная функция форматирования истории сообщений
// Заменяет все старые форматтеры в проекте
func (f *UnifiedMessageFormatter) FormatMessages(chatID int64, messages []*tgbotapi.Message) string {
	if len(messages) == 0 {
		return ""
	}

	// Загружаем профили пользователей пакетом
	profileMap := f.batchLoadProfiles(chatID, messages)

	// Получаем временную зону
	loc := f.getTimeLocation()

	var formattedMessages []string
	for _, msg := range messages {
		formatted := f.formatSingleMessage(msg, profileMap, loc)
		if formatted != "" {
			formattedMessages = append(formattedMessages, formatted)
		}
	}

	return strings.Join(formattedMessages, "")
}

// formatSingleMessage форматирует одно сообщение в унифицированном формате
func (f *UnifiedMessageFormatter) formatSingleMessage(msg *tgbotapi.Message, profileMap map[int64]*storage.UserProfile, loc *time.Location) string {
	if msg == nil || msg.From == nil {
		return ""
	}

	var builder strings.Builder
	userID := int64(msg.From.ID)
	profile := profileMap[userID]

	// Открывающий тег с ID сообщения
	builder.WriteString(fmt.Sprintf("[MSG_ID:%d]\n", msg.MessageID))

	// Тег пользователя [U123:Иван] - совместимость с существующими промптами
	userTag := f.generateUserTag(userID, msg.From, profile)
	builder.WriteString(fmt.Sprintf("[%s]\n", userTag))

	// Данные пользователя из профиля
	f.appendUserInfo(&builder, msg.From, profile)

	// Временные данные
	f.appendTimeInfo(&builder, msg, loc)

	// Содержимое сообщения
	f.appendMessageContent(&builder, msg)

	// Метаданные сообщения
	f.appendMessageMetadata(&builder, msg)

	// Закрывающий тег
	builder.WriteString(fmt.Sprintf("[/MSG_ID:%d]\n\n", msg.MessageID))

	return builder.String()
}

// generateUserTag создает тег пользователя типа [U123:Иван]
func (f *UnifiedMessageFormatter) generateUserTag(userID int64, from *tgbotapi.User, profile *storage.UserProfile) string {
	var displayName string

	// Приоритет имени: alias -> real_name -> first_name -> username -> fallback
	if profile != nil && profile.Alias != "" {
		displayName = profile.Alias
	} else if profile != nil && profile.RealName != "" {
		displayName = profile.RealName
	} else if from.FirstName != "" {
		displayName = from.FirstName
	} else if from.UserName != "" {
		displayName = from.UserName
	} else {
		displayName = fmt.Sprintf("User_%d", userID)
	}

	return fmt.Sprintf("U%d:%s", userID, displayName)
}

// appendUserInfo добавляет информацию о пользователе
func (f *UnifiedMessageFormatter) appendUserInfo(builder *strings.Builder, from *tgbotapi.User, profile *storage.UserProfile) {
	// username
	if profile != nil && profile.Username != "" {
		builder.WriteString(fmt.Sprintf("username: @%s\n", profile.Username))
	} else if from.UserName != "" {
		builder.WriteString(fmt.Sprintf("username: @%s\n", from.UserName))
	}

	// alias
	if profile != nil && profile.Alias != "" {
		builder.WriteString(fmt.Sprintf("alias: %s\n", profile.Alias))
	} else if from.FirstName != "" {
		builder.WriteString(fmt.Sprintf("alias: %s\n", from.FirstName))
	}

	// real_name
	if profile != nil && profile.RealName != "" {
		builder.WriteString(fmt.Sprintf("real_name: %s\n", profile.RealName))
	}

	// bio
	if profile != nil && profile.Bio != "" {
		builder.WriteString(fmt.Sprintf("bio: %s\n", profile.Bio))
	}
}

// appendTimeInfo добавляет временную информацию
func (f *UnifiedMessageFormatter) appendTimeInfo(builder *strings.Builder, msg *tgbotapi.Message, loc *time.Location) {
	msgTime := time.Unix(int64(msg.Date), 0).In(loc)
	builder.WriteString(fmt.Sprintf("date: %s\n", msgTime.Format("15:04 02.01 (Mon)")))
}

// appendMessageContent добавляет содержимое сообщения
func (f *UnifiedMessageFormatter) appendMessageContent(builder *strings.Builder, msg *tgbotapi.Message) {
	messageText := ""

	// Приоритет: Text -> Caption
	if msg.Text != "" {
		messageText = msg.Text
	} else if msg.Caption != "" {
		messageText = msg.Caption
	}

	if messageText != "" {
		builder.WriteString(fmt.Sprintf("message: %s\n", messageText))
	}

	// Специальные типы сообщений
	if msg.Voice != nil {
		builder.WriteString("type: voice_message\n")
	}
	if msg.Audio != nil {
		builder.WriteString("type: audio_message\n")
	}
	if len(msg.Photo) > 0 {
		builder.WriteString("type: photo_message\n")
	}
	if msg.Video != nil {
		builder.WriteString("type: video_message\n")
	}
	if msg.Document != nil {
		builder.WriteString("type: document_message\n")
	}
	if msg.Sticker != nil {
		builder.WriteString("type: sticker_message\n")
	}
}

// appendMessageMetadata добавляет метаданные сообщения
func (f *UnifiedMessageFormatter) appendMessageMetadata(builder *strings.Builder, msg *tgbotapi.Message) {
	// Пересланное сообщение
	if msg.ForwardFrom != nil {
		forwardFromName := f.getForwardFromName(msg.ForwardFrom)
		builder.WriteString(fmt.Sprintf("forwarded_from: %s\n", forwardFromName))
	} else if msg.ForwardFromChat != nil {
		builder.WriteString(fmt.Sprintf("forwarded_from: %s\n", msg.ForwardFromChat.Title))
	}

	// Ответ на сообщение
	if msg.ReplyToMessage != nil {
		builder.WriteString(fmt.Sprintf("reply_to: MSG_ID:%d\n", msg.ReplyToMessage.MessageID))

		// Автор исходного сообщения
		if msg.ReplyToMessage.From != nil {
			replyAuthorName := ""
			if msg.ReplyToMessage.From.FirstName != "" {
				replyAuthorName = msg.ReplyToMessage.From.FirstName
			} else if msg.ReplyToMessage.From.UserName != "" {
				replyAuthorName = msg.ReplyToMessage.From.UserName
			} else {
				replyAuthorName = fmt.Sprintf("User_%d", msg.ReplyToMessage.From.ID)
			}
			builder.WriteString(fmt.Sprintf("reply_to_author: %s\n", replyAuthorName))
		}
	}

	// Редактированное сообщение
	if msg.EditDate != 0 {
		builder.WriteString("edited: true\n")
	}
}

// getForwardFromName получает имя отправителя пересланного сообщения
func (f *UnifiedMessageFormatter) getForwardFromName(from *tgbotapi.User) string {
	if from.FirstName != "" {
		return from.FirstName
	}
	if from.UserName != "" {
		return from.UserName
	}
	return fmt.Sprintf("User_%d", from.ID)
}

// batchLoadProfiles загружает профили пользователей пакетом
func (f *UnifiedMessageFormatter) batchLoadProfiles(chatID int64, messages []*tgbotapi.Message) map[int64]*storage.UserProfile {
	// Собираем уникальные ID пользователей
	userIDs := make(map[int64]bool)
	for _, msg := range messages {
		if msg.From != nil {
			userIDs[int64(msg.From.ID)] = true
		}
		// Также учитываем авторов сообщений, на которые отвечают
		if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil {
			userIDs[int64(msg.ReplyToMessage.From.ID)] = true
		}
		// И авторов пересланных сообщений
		if msg.ForwardFrom != nil {
			userIDs[int64(msg.ForwardFrom.ID)] = true
		}
	}

	// Преобразуем в slice
	var userIDList []int64
	for userID := range userIDs {
		userIDList = append(userIDList, userID)
	}

	// Загружаем профили
	profileMap := make(map[int64]*storage.UserProfile)
	for _, userID := range userIDList {
		if profile, err := f.storage.GetUserProfile(chatID, userID); err == nil && profile != nil {
			profileMap[userID] = profile
		}
	}

	return profileMap
}

// getTimeLocation получает временную зону
func (f *UnifiedMessageFormatter) getTimeLocation() *time.Location {
	if f.timeZone == "" {
		return time.UTC
	}

	loc, err := time.LoadLocation(f.timeZone)
	if err != nil {
		log.Printf("Не удалось загрузить временную зону %s: %v", f.timeZone, err)
		return time.UTC
	}

	return loc
}

// FormatSortedMessages форматирует сообщения с сортировкой по времени
func (f *UnifiedMessageFormatter) FormatSortedMessages(chatID int64, messages []*tgbotapi.Message) string {
	if len(messages) == 0 {
		return ""
	}

	// Сортируем сообщения по времени
	sortedMessages := make([]*tgbotapi.Message, len(messages))
	copy(sortedMessages, messages)
	sort.Slice(sortedMessages, func(i, j int) bool {
		return sortedMessages[i].Date < sortedMessages[j].Date
	})

	return f.FormatMessages(chatID, sortedMessages)
}

// FormatRecentMessages форматирует последние N сообщений
func (f *UnifiedMessageFormatter) FormatRecentMessages(chatID int64, limit int) string {
	messages, err := f.storage.GetMessages(chatID, limit)
	if err != nil {
		log.Printf("Ошибка получения последних сообщений: %v", err)
		return ""
	}

	return f.FormatMessages(chatID, messages)
}
