package bot

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MessageChainMetadata содержит метаданные цепочки сообщений
type MessageChainMetadata struct {
	ChainID        string         // Уникальный ID цепочки
	StartTime      time.Time      // Время начала цепочки
	EndTime        time.Time      // Время последнего сообщения
	ParticipantIDs []int64        // ID участников цепочки
	Depth          int            // Глубина цепочки
	ForwardedFrom  map[int]string // Информация о пересланных сообщениях: messageID -> источник
}

// formatMessageChain форматирует цепочку связанных сообщений
func formatMessageChain(
	chatID int64,
	messages []*tgbotapi.Message,
	profilesCache map[int64]*storage.UserProfile,
	store storage.ChatHistoryStorage,
	loc *time.Location,
	contextType string) string {

	if len(messages) == 0 {
		return ""
	}

	// Инициализируем метаданные цепочки
	metadata := &MessageChainMetadata{
		ChainID:        fmt.Sprintf("chain_%d", messages[len(messages)-1].MessageID),
		StartTime:      time.Unix(int64(messages[0].Date), 0),
		EndTime:        time.Unix(int64(messages[len(messages)-1].Date), 0),
		ParticipantIDs: make([]int64, 0),
		Depth:          len(messages),
		ForwardedFrom:  make(map[int]string),
	}

	// Собираем участников и информацию о пересылках
	participantsMap := make(map[int64]bool)
	for _, msg := range messages {
		if msg.From != nil {
			participantsMap[msg.From.ID] = true
		}
		if msg.ForwardFrom != nil {
			metadata.ForwardedFrom[msg.MessageID] = fmt.Sprintf("user:%d", msg.ForwardFrom.ID)
		}
		if msg.ForwardFromChat != nil {
			metadata.ForwardedFrom[msg.MessageID] = fmt.Sprintf("chat:%s", msg.ForwardFromChat.Title)
		}
	}
	for id := range participantsMap {
		metadata.ParticipantIDs = append(metadata.ParticipantIDs, id)
	}

	// Начинаем форматирование
	var sb strings.Builder

	// Открываем цепочку с метаданными
	sb.WriteString(fmt.Sprintf("[MSG_CHAIN:%s]\n", metadata.ChainID))
	sb.WriteString(fmt.Sprintf("Start: %s\n", metadata.StartTime.In(loc).Format("15:04 02.01.2006")))
	sb.WriteString(fmt.Sprintf("End: %s\n", metadata.EndTime.In(loc).Format("15:04 02.01.2006")))
	sb.WriteString(fmt.Sprintf("Depth: %d\n", metadata.Depth))

	// Информация об участниках
	sb.WriteString("Participants:\n")
	for _, userID := range metadata.ParticipantIDs {
		var displayName string
		if profile, exists := profilesCache[userID]; exists && profile != nil {
			if profile.Alias != "" {
				displayName = profile.Alias
			} else if profile.RealName != "" {
				displayName = profile.RealName
			} else {
				displayName = profile.Username
			}
		} else {
			displayName = fmt.Sprintf("User_%d", userID)
		}
		sb.WriteString(fmt.Sprintf("  - [U%d:%s]\n", userID, displayName))
	}

	// Если есть пересланные сообщения
	if len(metadata.ForwardedFrom) > 0 {
		sb.WriteString("Forwarded Messages:\n")
		for msgID, source := range metadata.ForwardedFrom {
			sb.WriteString(fmt.Sprintf("  - Message #%d from %s\n", msgID, source))
		}
	}
	sb.WriteString("\n")

	// Форматируем каждое сообщение в цепочке
	// Используем новый унифицированный форматтер для каждого сообщения в цепочке
	formatter := NewUnifiedMessageFormatter(store, "UTC") // TODO: передавать правильную временную зону

	for i, msg := range messages {
		// Добавляем отступы для визуализации структуры
		indent := strings.Repeat("  ", i)

		// Форматируем одно сообщение через унифицированный форматтер
		formatted := formatter.FormatMessagesXML(chatID, []*tgbotapi.Message{msg})
		// Добавляем отступы к каждой строке сообщения
		formattedLines := strings.Split(formatted, "\n")
		for _, line := range formattedLines {
			sb.WriteString(indent + line + "\n")
		}
	}

	// Закрываем цепочку
	sb.WriteString(fmt.Sprintf("[/MSG_CHAIN:%s]\n", metadata.ChainID))

	result := sb.String()
	log.Printf("✅ [MSG_CHAIN] Цепочка %s отформатирована (длина: %d символов)",
		metadata.ChainID, len(result))

	return result
}
