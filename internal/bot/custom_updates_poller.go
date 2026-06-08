package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// CustomUpdatesPoller заменяет стандартный GetUpdatesChan и обрабатывает реакции
type CustomUpdatesPoller struct {
	bot         *Bot
	client      *http.Client
	baseURL     string
	offset      int
	timeout     int
	updatesChan chan tgbotapi.Update
	stop        chan struct{}
	running     bool
	mutex       sync.RWMutex
}

// NewCustomUpdatesPoller создает новый кастомный поллер
func NewCustomUpdatesPoller(bot *Bot) *CustomUpdatesPoller {
	return &CustomUpdatesPoller{
		bot:         bot,
		client:      &http.Client{Timeout: 70 * time.Second},
		baseURL:     fmt.Sprintf("https://api.telegram.org/bot%s", bot.config.TelegramToken),
		timeout:     60,
		updatesChan: make(chan tgbotapi.Update, 100),
		stop:        make(chan struct{}),
	}
}

// Start запускает кастомный поллер
func (cup *CustomUpdatesPoller) Start() chan tgbotapi.Update {
	cup.mutex.Lock()
	defer cup.mutex.Unlock()

	if cup.running {
		return cup.updatesChan
	}

	// Очистка pending updates для избежания конфликтов
	log.Println("[CustomUpdatesPoller] Очистка pending updates...")
	clearURL := fmt.Sprintf("%s/getUpdates", cup.baseURL)
	clearPayload := map[string]interface{}{
		"offset": -1, // Получаем последний update_id
		"limit":  1,
	}

	jsonData, _ := json.Marshal(clearPayload)
	resp, err := cup.client.Post(clearURL, "application/json", bytes.NewBuffer(jsonData))
	if err == nil {
		resp.Body.Close()
		log.Println("[CustomUpdatesPoller] Pending updates очищены")
	}

	cup.running = true
	cup.stop = make(chan struct{})

	go cup.pollUpdates()

	return cup.updatesChan
}

// Stop останавливает поллер
func (cup *CustomUpdatesPoller) Stop() {
	cup.mutex.Lock()
	defer cup.mutex.Unlock()

	if !cup.running {
		return
	}

	cup.running = false
	close(cup.stop)
	close(cup.updatesChan)
}

// pollUpdates основной цикл получения обновлений
func (cup *CustomUpdatesPoller) pollUpdates() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[ERROR][CustomUpdatesPoller] Panic в pollUpdates: %v", r)
		}
	}()

	for {
		select {
		case <-cup.stop:
			if cup.bot.config.Debug {
				log.Println("[DEBUG][CustomUpdatesPoller] Получен сигнал остановки")
			}
			return
		default:
			cup.pollOnce()
		}
	}
}

// pollOnce выполняет один цикл получения обновлений
func (cup *CustomUpdatesPoller) pollOnce() {
	updates, err := cup.getUpdatesWithReactions()
	if err != nil {
		// Специальная обработка конфликтов (409 ошибка)
		if strings.Contains(err.Error(), "Conflict: terminated by other getUpdates request") {
			if cup.bot.config.Debug {
				log.Printf("[DEBUG][CustomUpdatesPoller] Конфликт getUpdates, пауза 5 секунд...")
			}
			time.Sleep(5 * time.Second)
			return
		}

		if cup.bot.config.Debug {
			log.Printf("[DEBUG][CustomUpdatesPoller] Ошибка получения обновлений: %v", err)
		}
		time.Sleep(3 * time.Second)
		return
	}

	for _, update := range updates {
		select {
		case <-cup.stop:
			return
		case cup.updatesChan <- update:
			// Обновляем offset
			if update.UpdateID >= cup.offset {
				cup.offset = update.UpdateID + 1
			}
		default:
			// Если канал переполнен, пропускаем обновление (но отдельно логируем реакции)
			if update.Message != nil || update.CallbackQuery != nil {
				log.Printf("[WARN][CustomUpdatesPoller] Канал обновлений переполнен, пропускаем ОБЫЧНОЕ обновление %d", update.UpdateID)
			} else {
				log.Printf("[CRITICAL][CustomUpdatesPoller] Канал обновлений переполнен, пропускаем ВОЗМОЖНУЮ РЕАКЦИЮ в обновлении %d", update.UpdateID)
			}
			// Обновляем offset даже для пропущенных обновлений
			if update.UpdateID >= cup.offset {
				cup.offset = update.UpdateID + 1
			}
		}
	}
}

// getUpdatesWithReactions получает обновления включая реакции
func (cup *CustomUpdatesPoller) getUpdatesWithReactions() ([]tgbotapi.Update, error) {
	url := fmt.Sprintf("%s/getUpdates", cup.baseURL)

	payload := map[string]interface{}{
		"offset":          cup.offset,
		"timeout":         cup.timeout,
		"allowed_updates": []string{"message", "message_reaction", "callback_query", "edited_message", "my_chat_member", "chat_member"},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("ошибка маршалинга JSON: %w", err)
	}

	resp, err := cup.client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("ошибка HTTP-запроса: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	if cup.bot.config.Debug {
		// Логируем только первые 500 символов для избежания спама
		bodyPreview := string(body)
		if len(bodyPreview) > 500 {
			bodyPreview = bodyPreview[:500] + "..."
		}
		log.Printf("[DEBUG][CustomUpdatesPoller] Ответ от Telegram: %s", bodyPreview)
	}

	// Сначала парсим как raw JSON для извлечения реакций
	var rawResponse map[string]interface{}
	if err := json.Unmarshal(body, &rawResponse); err != nil {
		return nil, fmt.Errorf("ошибка парсинга raw JSON: %w", err)
	}

	if !rawResponse["ok"].(bool) {
		errorDesc := ""
		if desc, exists := rawResponse["description"]; exists {
			errorDesc = desc.(string)
		}
		return nil, fmt.Errorf("ошибка Telegram API: %s", errorDesc)
	}

	result, exists := rawResponse["result"]
	if !exists {
		return []tgbotapi.Update{}, nil
	}

	resultArray, ok := result.([]interface{})
	if !ok {
		return []tgbotapi.Update{}, nil
	}

	var updates []tgbotapi.Update

	for _, updateInterface := range resultArray {
		updateMap, ok := updateInterface.(map[string]interface{})
		if !ok {
			continue
		}

		// Проверяем на наличие message_reaction
		if _, hasReaction := updateMap["message_reaction"]; hasReaction {
			updateID := int(0)
			if uid, ok := updateMap["update_id"].(float64); ok {
				updateID = int(uid)
			}

			log.Printf("[CustomUpdatesPoller] ОБНАРУЖЕНА РЕАКЦИЯ в обновлении %d", updateID)

			// Обрабатываем реакцию через наш ReactionHandler
			updateJSON, err := json.Marshal(updateInterface)
			if err != nil {
				log.Printf("[ERROR][CustomUpdatesPoller] Ошибка маршалинга реакции в обновлении %d: %v", updateID, err)
				continue
			}

			// Передаем в ReactionHandler
			if cup.bot.reactionHandler != nil {
				processed := cup.bot.reactionHandler.ProcessRawUpdate(updateJSON)
				if processed {
					log.Printf("[CustomUpdatesPoller] РЕАКЦИЯ УСПЕШНО ОБРАБОТАНА в обновлении %d", updateID)
				} else {
					log.Printf("[WARN][CustomUpdatesPoller] РЕАКЦИЯ НЕ ОБРАБОТАНА в обновлении %d", updateID)
				}
			} else {
				log.Printf("[ERROR][CustomUpdatesPoller] ReactionHandler не инициализирован для обновления %d", updateID)
			}

			// Создаем стандартное обновление для совместимости (пустое, так как реакция уже обработана)
			emptyUpdate := tgbotapi.Update{
				UpdateID: updateID,
			}
			updates = append(updates, emptyUpdate)
		} else {
			// Обычное обновление - парсим как стандартный Update
			updateJSON, err := json.Marshal(updateInterface)
			if err != nil {
				if cup.bot.config.Debug {
					log.Printf("[DEBUG][CustomUpdatesPoller] Ошибка маршалинга обновления: %v", err)
				}
				continue
			}

			var update tgbotapi.Update
			if err := json.Unmarshal(updateJSON, &update); err != nil {
				if cup.bot.config.Debug {
					log.Printf("[DEBUG][CustomUpdatesPoller] Ошибка парсинга обновления: %v", err)
				}
				continue
			}

			updates = append(updates, update)
		}
	}

	return updates, nil
}
