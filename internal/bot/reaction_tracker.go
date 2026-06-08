package bot

import (
	"fmt"
	"log"
	"sync"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ReactionTracker отслеживает реакции на сообщения
type ReactionTracker struct {
	bot          *Bot
	storage      storage.ChatHistoryStorage
	reactionsAPI *TelegramReactionsAPI
	mutex        sync.RWMutex
}

// NewReactionTracker создает новый экземпляр ReactionTracker
func NewReactionTracker(bot *Bot, storage storage.ChatHistoryStorage) *ReactionTracker {
	// Создаем API для работы с реакциями
	reactionsAPI := NewTelegramReactionsAPI(bot.config.TelegramToken, bot.config.Debug)

	return &ReactionTracker{
		bot:          bot,
		storage:      storage,
		reactionsAPI: reactionsAPI,
	}
}

// HandleReactionUpdate обрабатывает обновления реакций
func (rt *ReactionTracker) HandleReactionUpdate(update tgbotapi.Update) {
	if !rt.bot.config.ReactionsEnabled {
		return
	}

	// Проверяем, является ли это реакцией (закодированной в CallbackQuery)
	if update.CallbackQuery != nil {
		oldReactions, newReactions, isReaction := rt.reactionsAPI.DecodeReactionData(update.CallbackQuery.Data)
		if isReaction {
			rt.processReactionChange(update.CallbackQuery, oldReactions, newReactions)
		}
		return
	}

	// Пока что реакции обрабатываются только через CallbackQuery
	// В будущем можно добавить поддержку нативных реакций
}

// processReactionChange обрабатывает изменение реакций
func (rt *ReactionTracker) processReactionChange(query *tgbotapi.CallbackQuery, oldReactions, newReactions []string) {
	chatID := query.Message.Chat.ID
	messageID := query.Message.MessageID
	userID := query.From.ID
	username := query.From.UserName

	if rt.bot.config.Debug {
		log.Printf("[DEBUG][Reactions] Чат %d, сообщение %d, пользователь %d (@%s): %v -> %v",
			chatID, messageID, userID, username, oldReactions, newReactions)
	}

	rt.mutex.Lock()
	defer rt.mutex.Unlock()

	// Обновляем реакции в БД
	err := rt.updateMessageReactions(chatID, messageID, userID, username, query.From.FirstName, newReactions)
	if err != nil {
		log.Printf("[ERROR][Reactions] Ошибка обновления реакций в БД: %v", err)
		return
	}

	// Проверяем специальные реакции
	rt.handleSpecialReactions(chatID, messageID, userID, username, oldReactions, newReactions)

	// Анализируем качество сообщения бота на основе реакций
	go rt.analyzeMessageQuality(chatID, messageID)
}

// handleSpecialReactions обрабатывает специальные реакции (например, клоуна)
func (rt *ReactionTracker) handleSpecialReactions(chatID int64, messageID int, userID int64, username string, oldReactions, newReactions []string) {
	// Проверяем, добавили ли клоуна
	clownAdded := false
	for _, emoji := range newReactions {
		if emoji == "🤡" {
			// Проверяем, не было ли клоуна в старых реакциях
			clownWasPresent := false
			for _, oldEmoji := range oldReactions {
				if oldEmoji == "🤡" {
					clownWasPresent = true
					break
				}
			}
			if !clownWasPresent {
				clownAdded = true
				break
			}
		}
	}

	if clownAdded {
		rt.handleClownReaction(chatID, userID, username)
	}
}

// handleClownReaction обрабатывает реакцию клоуна
func (rt *ReactionTracker) handleClownReaction(chatID int64, userID int64, username string) {
	if rt.bot.config.ClownReactionPrompt == "" {
		return
	}

	// Создаем контекст с информацией о пользователе
	userMention := ""

	// Пытаемся получить профиль пользователя для правильного обращения
	if profile, err := rt.bot.storage.GetUserProfile(chatID, userID); err == nil && profile != nil && profile.Alias != "" {
		// Используем Alias без символа @
		userMention = profile.Alias
	} else if username != "" {
		// Используем @Username только если Alias недоступен
		userMention = "@" + username
	} else {
		userMention = fmt.Sprintf("пользователь %d", userID)
	}

	contextText := fmt.Sprintf("Пользователь %s поставил реакцию клоуна 🤡 на сообщение бота.", userMention)

	// Генерируем ответ на клоуна с контекстом
	prompt := rt.bot.enrichPromptWithPersonality(rt.bot.config.ClownReactionPrompt, chatID, "clown_reaction")
	response, err := rt.bot.llm.GenerateResponseByType(llm.ResponseTypeClownReaction, prompt, contextText, float32(rt.bot.config.GeminiTemperatureNormal))
	if err != nil {
		log.Printf("[ERROR][Reactions] Ошибка генерации ответа на клоуна: %v", err)
		return
	}

	// Очищаем ответ
	response = cleanupLLMResponse(response)

	// Добавляем обращение к пользователю в начало ответа
	finalResponse := fmt.Sprintf("%s %s", userMention, response)

	// Отправляем ответ
	rt.bot.sendReply(chatID, finalResponse)

	if rt.bot.config.Debug {
		log.Printf("[DEBUG][Reactions] Отправлен ответ на клоуна от пользователя %s в чате %d", username, chatID)
	}
}

// updateMessageReactions обновляет реакции в БД
func (rt *ReactionTracker) updateMessageReactions(chatID int64, messageID int, userID int64, username, firstName string, reactions []string) error {
	// Получаем MongoDB хранилище
	_, ok := rt.storage.(*storage.PostgresStorage)
	if !ok {
		if rt.bot.config.Debug {
			log.Printf("[DEBUG][Reactions] Реакции поддерживаются только для MongoDB хранилища")
		}
		return nil
	}

	// TODO: Добавить метод UpdateMessageReactions в MongoStorage
	// Пока что логируем
	if rt.bot.config.Debug {
		log.Printf("[DEBUG][Reactions] Обновление реакций для сообщения %d в чате %d от пользователя %s (%s): %v",
			messageID, chatID, username, firstName, reactions)
	}

	return nil
}

// analyzeMessageQuality анализирует качество сообщения бота на основе реакций
func (rt *ReactionTracker) analyzeMessageQuality(chatID int64, messageID int) {
	if rt.bot.config.ReactionAnalysisPrompt == "" {
		return
	}

	// TODO: Получить все реакции на сообщение из БД
	// TODO: Проанализировать с помощью LLM
	// TODO: Сохранить результат в personality_memory

	if rt.bot.config.Debug {
		log.Printf("[DEBUG][Reactions] Анализ качества сообщения %d в чате %d", messageID, chatID)
	}
}

// SimulateClownReaction симулирует реакцию клоуна для тестирования
func (rt *ReactionTracker) SimulateClownReaction(chatID int64, userID int64, username string) {
	if !rt.bot.config.ReactionsEnabled {
		return
	}

	rt.handleClownReaction(chatID, userID, username)
}

// SetBotReaction устанавливает реакцию от имени бота
func (rt *ReactionTracker) SetBotReaction(chatID int64, messageID int, emoji string) error {
	if !rt.bot.config.ReactionsEnabled {
		return nil
	}

	return rt.reactionsAPI.SetMessageReaction(chatID, messageID, emoji, false)
}

// GetReactionsAPI возвращает API для работы с реакциями
func (rt *ReactionTracker) GetReactionsAPI() *TelegramReactionsAPI {
	return rt.reactionsAPI
}
