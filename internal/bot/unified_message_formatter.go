package bot

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// UnifiedMessageFormatter - унифицированный форматтер сообщений для всего проекта
type UnifiedMessageFormatter struct {
	storage             storage.ChatHistoryStorage
	timeZone            string
	DisableUserProfiles bool // диагностический флаг: если true, профили не загружаются
}

// NewUnifiedMessageFormatter создает новый экземпляр унифицированного форматтера
func NewUnifiedMessageFormatter(storage storage.ChatHistoryStorage, timeZone string) *UnifiedMessageFormatter {
	return &UnifiedMessageFormatter{
		storage:  storage,
		timeZone: timeZone,
	}
}

// SetDisableUserProfiles устанавливает флаг отключения загрузки профилей пользователей
func (f *UnifiedMessageFormatter) SetDisableUserProfiles(v bool) {
	f.DisableUserProfiles = v
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

	seenProfiles := make(map[int64]bool)

	var formattedMessages []string
	for _, msg := range messages {
		formatted := f.formatSingleMessage(msg, profileMap, loc, seenProfiles)
		if formatted != "" {
			formattedMessages = append(formattedMessages, formatted)
		}
	}

	return strings.Join(formattedMessages, "")
}

// formatSingleMessage форматирует одно сообщение в унифицированном формате
func (f *UnifiedMessageFormatter) formatSingleMessage(msg *tgbotapi.Message, profileMap map[int64]*storage.UserProfile, loc *time.Location, seenProfiles map[int64]bool) string {
	if msg == nil || msg.From == nil {
		return ""
	}

	var builder strings.Builder
	userID := int64(msg.From.ID)
	profile := profileMap[userID]

	// Открывающий тег с ID сообщения
	builder.WriteString(fmt.Sprintf("[MSG_ID:%d]\n", msg.MessageID))

	// Тег пользователя [U123:Иван] - совместимость с существующими промптами
	if seenProfiles[userID] {
		f.appendShortUserTag(&builder, userID, msg.From, profile)
	} else {
		userTag := f.generateUserTag(userID, msg.From, profile)
		builder.WriteString(fmt.Sprintf("[%s]\n", userTag))
		f.appendUserInfo(&builder, msg.From, profile)
		seenProfiles[userID] = true
	}

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

// appendShortUserTag добавляет только короткий тег пользователя (без полной информации профиля)
func (f *UnifiedMessageFormatter) appendShortUserTag(builder *strings.Builder, userID int64, from *tgbotapi.User, profile *storage.UserProfile) {
	tag := f.generateUserTag(userID, from, profile)
	builder.WriteString(fmt.Sprintf("[%s]\n", tag))
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
		bio := profile.Bio
		runes := []rune(bio)
		if len(runes) > 300 {
			bio = string(runes[:300]) + "…"
		}
		builder.WriteString(fmt.Sprintf("bio: %s\n", bio))
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
	if f.DisableUserProfiles {
		return make(map[int64]*storage.UserProfile)
	}

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

// FormatChatMessages converts tgbotapi messages into a ChatML message array.
// Bot messages map to role "assistant", all other users map to role "user".
// Content is prefixed with author display name for user messages (e.g. "[Иван]: привет").
func (f *UnifiedMessageFormatter) FormatChatMessages(chatID int64, messages []*tgbotapi.Message) []llm.ChatMessage {
	if len(messages) == 0 {
		return nil
	}

	profileMap := f.batchLoadProfiles(chatID, messages)

	chatMessages := make([]llm.ChatMessage, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}

		content := msg.Text
		if content == "" {
			content = msg.Caption
		}
		if content == "" {
			continue
		}

		role := "user"
		if msg.From != nil && msg.From.IsBot {
			role = "assistant"
		}

		if role == "user" && msg.From != nil {
			displayName := f.getDisplayName(int64(msg.From.ID), msg.From, profileMap)
			content = "[" + displayName + "]: " + content
		}

		chatMessages = append(chatMessages, llm.ChatMessage{
			Role:    role,
			Content: content,
		})
	}

	return chatMessages
}

// getDisplayName возвращает чистое имя пользователя (без markdown, без спецсимволов).
func (f *UnifiedMessageFormatter) getDisplayName(userID int64, from *tgbotapi.User, profileMap map[int64]*storage.UserProfile) string {
	profile := profileMap[userID]
	if profile != nil && profile.Alias != "" {
		return profile.Alias
	}
	if profile != nil && profile.RealName != "" {
		return profile.RealName
	}
	if from.FirstName != "" {
		name := from.FirstName
		if from.LastName != "" {
			name += " " + from.LastName
		}
		return name
	}
	if from.UserName != "" {
		return from.UserName
	}
	return fmt.Sprintf("User_%d", userID)
}

// FormatSortedChatMessages converts and sorts messages by time into ChatML format.
func (f *UnifiedMessageFormatter) FormatSortedChatMessages(chatID int64, messages []*tgbotapi.Message) []llm.ChatMessage {
	if len(messages) == 0 {
		return nil
	}

	sorted := make([]*tgbotapi.Message, len(messages))
	copy(sorted, messages)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Date < sorted[j].Date
	})

	return f.FormatChatMessages(chatID, sorted)
}

// FormatRecentChatMessages fetches the last N messages from storage and returns them as ChatML.
func (f *UnifiedMessageFormatter) FormatRecentChatMessages(chatID int64, limit int) []llm.ChatMessage {
	messages, err := f.storage.GetMessages(chatID, limit)
	if err != nil {
		log.Printf("Ошибка получения последних сообщений для ChatML: %v", err)
		return nil
	}

	return f.FormatChatMessages(chatID, messages)
}

// FormatMessagesXML форматирует историю сообщений в структурированный XML.
func (f *UnifiedMessageFormatter) FormatMessagesXML(chatID int64, messages []*tgbotapi.Message) string {
	if len(messages) == 0 {
		return ""
	}

	profileMap := f.batchLoadProfiles(chatID, messages)
	loc := f.getTimeLocation()
	seenProfiles := make(map[int64]bool)

	var sb strings.Builder
	sb.WriteString("<MESSAGE_HISTORY>\n")

	for _, msg := range messages {
		formatted := f.formatXMLMsg(msg, profileMap, loc, seenProfiles)
		if formatted != "" {
			sb.WriteString(formatted)
		}
	}

	sb.WriteString("</MESSAGE_HISTORY>")

	return sb.String()
}

func (f *UnifiedMessageFormatter) formatXMLMsg(msg *tgbotapi.Message, profileMap map[int64]*storage.UserProfile, loc *time.Location, seenProfiles map[int64]bool) string {
	if msg == nil {
		return ""
	}

	msgTime := time.Unix(int64(msg.Date), 0).In(loc)
	dateStr := msgTime.Format("15:04 02.01")
	dayStr := msgTime.Format("Mon")

	role := "user"
	isAssistant := false
	if msg.From != nil && msg.From.IsBot {
		role = "assistant"
		isAssistant = true
	}
	if msg.From == nil && msg.SenderChat != nil {
		role = "user"
	}

	messageText := msg.Text
	if messageText == "" {
		messageText = msg.Caption
	}
	if messageText == "" && !isAssistant {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<MSG ID=\"%d\" DATE=\"%s\" DAY=\"%s\" ROLE=\"%s\">\n", msg.MessageID, dateStr, dayStr, role))

	if isAssistant {
		sb.WriteString("  <ASSISTANT>\n")
		sb.WriteString("    <ALIAS>Luna</ALIAS>\n")
		sb.WriteString("  </ASSISTANT>\n")
	} else if msg.From != nil {
		userID := int64(msg.From.ID)
		profile := profileMap[userID]
		fullProfile := !seenProfiles[userID]
		seenProfiles[userID] = true
		sb.WriteString(f.formatXMLUserBlock(userID, msg.From, profile, fullProfile))
	} else if msg.SenderChat != nil {
		chatID := int64(msg.SenderChat.ID)
		fullProfile := !seenProfiles[chatID]
		seenProfiles[chatID] = true
		sb.WriteString(f.formatXMLSenderChatBlock(msg.SenderChat, fullProfile))
	}

	if messageText != "" {
		sb.WriteString(fmt.Sprintf("  <TEXT>%s</TEXT>\n", xmlEscape(messageText)))
	}

	sb.WriteString("</MSG>\n")
	return sb.String()
}

func (f *UnifiedMessageFormatter) formatXMLUserBlock(userID int64, from *tgbotapi.User, profile *storage.UserProfile, fullProfile bool) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  <USER ID=\"%d\">\n", userID))

	if fullProfile {
		username := ""
		if profile != nil && profile.Username != "" {
			username = "@" + profile.Username
		} else if from.UserName != "" {
			username = "@" + from.UserName
		}
		if username != "" {
			sb.WriteString(fmt.Sprintf("    <USERNAME>%s</USERNAME>\n", xmlEscape(username)))
		}

		alias := ""
		if profile != nil && profile.Alias != "" {
			alias = profile.Alias
		} else if from.FirstName != "" {
			alias = from.FirstName
		} else if from.UserName != "" {
			alias = from.UserName
		}
		if alias != "" {
			sb.WriteString(fmt.Sprintf("    <ALIAS>%s</ALIAS>\n", xmlEscape(alias)))
		}

		if profile != nil && profile.RealName != "" {
			sb.WriteString(fmt.Sprintf("    <REAL_NAME>%s</REAL_NAME>\n", xmlEscape(profile.RealName)))
		}

		if profile != nil && profile.Bio != "" {
			bio := profile.Bio
			runes := []rune(bio)
			if len(runes) > 300 {
				bio = string(runes[:300]) + "…"
			}
			sb.WriteString(fmt.Sprintf("    <BIO>%s</BIO>\n", xmlEscape(bio)))
		}
	} else {
		alias := ""
		if profile != nil && profile.Alias != "" {
			alias = profile.Alias
		} else if from.FirstName != "" {
			alias = from.FirstName
		} else if from.UserName != "" {
			alias = from.UserName
		}
		if alias != "" {
			sb.WriteString(fmt.Sprintf("    <ALIAS>%s</ALIAS>\n", xmlEscape(alias)))
		}
	}

	sb.WriteString("  </USER>\n")
	return sb.String()
}

func (f *UnifiedMessageFormatter) formatXMLSenderChatBlock(senderChat *tgbotapi.Chat, fullProfile bool) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  <USER ID=\"%d\">\n", senderChat.ID))

	title := senderChat.Title
	if title == "" {
		title = fmt.Sprintf("Chat_%d", senderChat.ID)
	}
	sb.WriteString(fmt.Sprintf("    <ALIAS>%s</ALIAS>\n", xmlEscape(title)))

	sb.WriteString("  </USER>\n")
	return sb.String()
}
