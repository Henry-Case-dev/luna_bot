package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ReactionType представляет один элемент реакции (тип и emoji).
type ReactionType struct {
	Type  string `json:"type"`
	Emoji string `json:"emoji"`
}

// TelegramReactionsAPI предоставляет методы для работы с реакциями через прямой вызов Telegram API.
type TelegramReactionsAPI struct {
	token  string
	debug  bool
	apiURL string
	client *http.Client
}

// ReactionUpdate представляет обновление с реакцией от Telegram API.
type ReactionUpdate struct {
	UpdateID        int                      `json:"update_id"`
	CallbackQuery   *ReactionCallbackQuery   `json:"callback_query,omitempty"`
	MessageReaction *ReactionMessageReaction `json:"message_reaction,omitempty"`
}

// ReactionCallbackQuery представляет callback query с реакцией.
type ReactionCallbackQuery struct {
	ID           string           `json:"id"`
	From         *ReactionUser    `json:"from"`
	Message      *ReactionMessage `json:"message"`
	ChatInstance string           `json:"chat_instance"`
	Data         string           `json:"data"`
}

// ReactionUser представляет пользователя в реакции.
type ReactionUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

// ReactionMessage представляет сообщение для реакции.
type ReactionMessage struct {
	MessageID int           `json:"message_id"`
	Chat      *ReactionChat `json:"chat"`
	Date      int64         `json:"date"`
	Text      string        `json:"text,omitempty"`
}

// ReactionChat представляет чат для реакции.
type ReactionChat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
}

// ReactionMessageReaction представляет нативную реакцию Telegram на сообщение.
type ReactionMessageReaction struct {
	Chat        *ReactionChat   `json:"chat"`
	MessageID   int             `json:"message_id"`
	Date        int64           `json:"date"`
	User        *ReactionUser   `json:"user"`
	OldReaction []ReactionEmoji `json:"old_reaction"`
	NewReaction []ReactionEmoji `json:"new_reaction"`
}

// ReactionEmoji представляет один emoji в реакции.
type ReactionEmoji struct {
	Type  string `json:"type"`
	Emoji string `json:"emoji"`
}

// NewTelegramReactionsAPI создает новый экземпляр TelegramReactionsAPI.
func NewTelegramReactionsAPI(token string, debug bool) *TelegramReactionsAPI {
	return &TelegramReactionsAPI{
		token:  token,
		debug:  debug,
		apiURL: fmt.Sprintf("https://api.telegram.org/bot%s", token),
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// encodeReactions кодирует список реакций в строку.
func (api *TelegramReactionsAPI) encodeReactions(reactions []ReactionType) string {
	if len(reactions) == 0 {
		return "none"
	}
	var parts []string
	for _, r := range reactions {
		parts = append(parts, r.Emoji)
	}
	return strings.Join(parts, ",")
}

// SetMessageReaction устанавливает реакцию на сообщение.
func (api *TelegramReactionsAPI) SetMessageReaction(chatID int64, messageID int, emoji string, isBig bool) error {
	url := fmt.Sprintf("%s/setMessageReaction", api.apiURL)

	payload := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
		"reaction": []map[string]interface{}{
			{
				"type":  "emoji",
				"emoji": emoji,
			},
		},
		"is_big": isBig,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("ошибка сериализации запроса реакции: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("ошибка создания запроса реакции: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := api.client.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка отправки запроса реакции: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ошибка API Telegram (status=%d): %s", resp.StatusCode, string(respBody))
	}

	if api.debug {
		log.Printf("[ReactionsAPI] Реакция %s установлена на сообщение %d в чате %d", emoji, messageID, chatID)
	}

	return nil
}

// DecodeReactionData декодирует данные callback query для реакций.
// Поддерживает формат "reaction:oldReactions:newReactions".
// Возвращает старые реакции, новые реакции и флаг, является ли это реакцией.
func (api *TelegramReactionsAPI) DecodeReactionData(data string) ([]string, []string, bool) {
	if data == "" {
		return nil, nil, false
	}

	// Проверяем формат "reaction:..."
	if strings.HasPrefix(data, "reaction:") {
		parts := strings.SplitN(data, ":", 3)
		if len(parts) < 3 {
			return nil, nil, false
		}

		oldStr := parts[1]
		newStr := parts[2]

		var oldReactions, newReactions []string
		if oldStr != "none" && oldStr != "" {
			oldReactions = strings.Split(oldStr, ",")
		}
		if newStr != "none" && newStr != "" {
			newReactions = strings.Split(newStr, ",")
		}

		return oldReactions, newReactions, true
	}

	// Пробуем JSON-формат для обратной совместимости
	var reactionData struct {
		OldReaction string `json:"old"`
		NewReaction string `json:"new"`
	}

	if err := json.Unmarshal([]byte(data), &reactionData); err != nil {
		return nil, nil, false
	}

	var oldReactions, newReactions []string
	if reactionData.OldReaction != "" {
		oldReactions = []string{reactionData.OldReaction}
	}
	if reactionData.NewReaction != "" {
		newReactions = []string{reactionData.NewReaction}
	}

	return oldReactions, newReactions, true
}

// GetUpdatesWithReactions получает обновления с реакциями через getUpdates.
func (api *TelegramReactionsAPI) GetUpdatesWithReactions(offset int, timeout int) ([]ReactionUpdate, error) {
	url := fmt.Sprintf("%s/getUpdates", api.apiURL)

	payload := map[string]interface{}{
		"offset":  offset,
		"timeout": timeout,
		"allowed_updates": []string{
			"callback_query",
			"message_reaction",
			"message_reaction_count",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("ошибка сериализации запроса getUpdates: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса getUpdates: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := api.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка выполнения запроса getUpdates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ошибка API Telegram getUpdates (status=%d): %s", resp.StatusCode, string(respBody))
	}

	var apiResponse struct {
		OK     bool             `json:"ok"`
		Result []ReactionUpdate `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа getUpdates: %w", err)
	}

	if !apiResponse.OK {
		return nil, fmt.Errorf("API Telegram вернул ошибку: result=false")
	}

	return apiResponse.Result, nil
}

// ConvertToStandardUpdate конвертирует ReactionUpdate в стандартный tgbotapi.Update.
// Поддерживается конвертация только через CallbackQuery (нативные реакции не поддерживаются текущей версией библиотеки).
func (api *TelegramReactionsAPI) ConvertToStandardUpdate(reactionUpdate ReactionUpdate) *tgbotapi.Update {
	if reactionUpdate.CallbackQuery == nil {
		return nil
	}

	cb := reactionUpdate.CallbackQuery
	if cb.Message == nil || cb.Message.Chat == nil {
		return nil
	}

	return &tgbotapi.Update{
		UpdateID: reactionUpdate.UpdateID,
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID: cb.ID,
			From: &tgbotapi.User{
				ID:        cb.From.ID,
				IsBot:     cb.From.IsBot,
				FirstName: cb.From.FirstName,
				UserName:  cb.From.Username,
			},
			Message: &tgbotapi.Message{
				MessageID: cb.Message.MessageID,
				Chat: &tgbotapi.Chat{
					ID:    cb.Message.Chat.ID,
					Type:  cb.Message.Chat.Type,
					Title: cb.Message.Chat.Title,
				},
				Date: int(cb.Message.Date),
				Text: cb.Message.Text,
			},
			ChatInstance: cb.ChatInstance,
			Data:         cb.Data,
		},
	}
}
