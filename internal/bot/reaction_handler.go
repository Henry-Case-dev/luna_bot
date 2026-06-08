package bot

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
)

// CustomMessageReaction представляет реакцию на сообщение из Telegram API
type CustomMessageReaction struct {
	Chat        CustomChat           `json:"chat"`
	MessageID   int                  `json:"message_id"`
	User        *CustomUser          `json:"user,omitempty"`
	ActorChat   *CustomChat          `json:"actor_chat,omitempty"`
	Date        int64                `json:"date"`
	OldReaction []CustomReactionType `json:"old_reaction"`
	NewReaction []CustomReactionType `json:"new_reaction"`
}

// CustomReactionType представляет тип реакции
type CustomReactionType struct {
	Type        string                         `json:"type"`
	Emoji       string                         `json:"emoji,omitempty"`
	CustomEmoji *CustomReactionTypeCustomEmoji `json:"custom_emoji,omitempty"`
}

// CustomReactionTypeCustomEmoji представляет кастомную эмодзи реакцию
type CustomReactionTypeCustomEmoji struct {
	CustomEmojiID string `json:"custom_emoji_id"`
}

// CustomChat представляет чат
type CustomChat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
}

// CustomUser представляет пользователя
type CustomUser struct {
	ID           int64  `json:"id"`
	IsBot        bool   `json:"is_bot"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

// ReactionHandler обрабатывает реакции из raw JSON
type ReactionHandler struct {
	bot          *Bot
	clownLimiter *ClownReactionLimiter // Лимитер для предотвращения спама реакций клоуна
}

// NewReactionHandler создает новый обработчик реакций
func NewReactionHandler(bot *Bot) *ReactionHandler {
	return &ReactionHandler{
		bot:          bot,
		clownLimiter: NewClownReactionLimiter(),
	}
}

// ProcessRawUpdate обрабатывает raw JSON update и извлекает реакции
func (rh *ReactionHandler) ProcessRawUpdate(rawJSON []byte) bool {
	if !rh.bot.config.ReactionsEnabled {
		return false
	}

	// Парсим raw JSON для поиска message_reaction
	var rawUpdate map[string]interface{}
	if err := json.Unmarshal(rawJSON, &rawUpdate); err != nil {
		return false
	}

	// Проверяем наличие message_reaction
	messageReactionData, exists := rawUpdate["message_reaction"]
	if !exists {
		return false
	}

	// Конвертируем в нашу структуру
	messageReactionJSON, err := json.Marshal(messageReactionData)
	if err != nil {
		if rh.bot.config.Debug {
			log.Printf("[DEBUG][ReactionHandler] Ошибка маршалинга message_reaction: %v", err)
		}
		return false
	}

	var messageReaction CustomMessageReaction
	if err := json.Unmarshal(messageReactionJSON, &messageReaction); err != nil {
		if rh.bot.config.Debug {
			log.Printf("[DEBUG][ReactionHandler] Ошибка парсинга message_reaction: %v", err)
		}
		return false
	}

	// Обрабатываем реакцию
	rh.handleMessageReaction(&messageReaction)
	return true
}

// handleMessageReaction обрабатывает реакцию на сообщение
func (rh *ReactionHandler) handleMessageReaction(reaction *CustomMessageReaction) {
	// Проверяем, включены ли реакции
	if !rh.bot.config.ReactionsEnabled {
		if rh.bot.config.Debug {
			log.Printf("[DEBUG][ReactionHandler] Реакции отключены в конфигурации")
		}
		return
	}

	chatID := reaction.Chat.ID
	messageID := reaction.MessageID
	userID := int64(0)
	username := ""
	firstName := ""

	if reaction.User != nil {
		userID = reaction.User.ID
		username = reaction.User.Username
		firstName = reaction.User.FirstName
	}

	log.Printf("[ReactionHandler] ОБРАБОТКА РЕАКЦИИ: чат %d, сообщение %d, пользователь %d (@%s)",
		chatID, messageID, userID, username)

	// Увеличиваем счетчик всех реакций
	if rh.bot.reactionStats != nil {
		rh.bot.reactionStats.IncrementTotalReactions()
	}

	// Конвертируем реакции в строки
	oldReactions := rh.convertReactionsToStrings(reaction.OldReaction)
	newReactions := rh.convertReactionsToStrings(reaction.NewReaction)

	log.Printf("[ReactionHandler] РЕАКЦИИ: старые=%v, новые=%v", oldReactions, newReactions)

	// НОВЫЙ КОД: Проверяем сразу на клоуна в новых реакциях для быстрого обнаружения
	hasClownInNew := false
	hasClownInOld := false
	for _, emoji := range newReactions {
		if emoji == "🤡" {
			hasClownInNew = true
			break
		}
	}
	for _, emoji := range oldReactions {
		if emoji == "🤡" {
			hasClownInOld = true
			break
		}
	}

	log.Printf("[ReactionHandler] БЫСТРАЯ ПРОВЕРКА КЛОУНА: в новых=%v, в старых=%v", hasClownInNew, hasClownInOld)

	// Увеличиваем счетчик клоунов если клоун добавлен
	if hasClownInNew && !hasClownInOld && rh.bot.reactionStats != nil {
		rh.bot.reactionStats.IncrementClownReactions()
	}

	// Обновляем реакции в БД (ВАЖНО: делаем это в любом случае)
	err := rh.updateMessageReactions(chatID, messageID, userID, username, firstName, newReactions)
	if err != nil {
		log.Printf("[ERROR][ReactionHandler] Ошибка обновления реакций в БД: %v", err)
		if rh.bot.reactionStats != nil {
			rh.bot.reactionStats.IncrementDBErrors()
		}
		// НЕ ВОЗВРАЩАЕМСЯ - продолжаем обработку даже при ошибке БД
	}

	// Проверяем, является ли это сообщением бота (делаем с повторными попытками)
	isBotMessage := rh.isBotMessageWithRetry(chatID, messageID)

	log.Printf("[ReactionHandler] ПРОВЕРКА СООБЩЕНИЯ БОТА: сообщение %d, результат=%v", messageID, isBotMessage)

	// Обрабатываем специальные реакции (клоун на сообщения бота)
	if isBotMessage {
		log.Printf("[ReactionHandler] ЭТО СООБЩЕНИЕ БОТА - обрабатываем специальные реакции")
		rh.handleSpecialReactions(chatID, messageID, userID, username, oldReactions, newReactions)

		// Анализируем качество сообщения бота через ReactionAnalyzer
		if rh.bot.reactionAnalyzer != nil {
			go func() {
				messageText, err := rh.getMessageText(chatID, messageID)
				if err != nil {
					log.Printf("[ERROR][ReactionHandler] Ошибка получения текста для анализа: %v", err)
					return
				}
				err = rh.bot.reactionAnalyzer.AnalyzeAndStore(chatID, messageID, messageText)
				if err != nil {
					log.Printf("[ERROR][ReactionHandler] Ошибка анализа через ReactionAnalyzer: %v", err)
				}
			}()
		}
	} else {
		log.Printf("[ReactionHandler] ЭТО НЕ СООБЩЕНИЕ БОТА - пропускаем специальную обработку")
		// Убрана fallback логика - бот должен отвечать только на клоуна на СВОИХ сообщениях
	}
}

// convertReactionsToStrings конвертирует реакции в массив строк
func (rh *ReactionHandler) convertReactionsToStrings(reactions []CustomReactionType) []string {
	var result []string
	for _, reaction := range reactions {
		if reaction.Type == "emoji" && reaction.Emoji != "" {
			result = append(result, reaction.Emoji)
		}
		// Можно добавить поддержку кастомных эмодзи в будущем
	}
	return result
}

// updateMessageReactions обновляет реакции в БД
func (rh *ReactionHandler) updateMessageReactions(chatID int64, messageID int, userID int64, username, firstName string, reactions []string) error {
	// Получаем MongoDB хранилище
	mongoStorage, ok := rh.bot.storage.(*storage.PostgresStorage)
	if !ok {
		if rh.bot.config.Debug {
			log.Printf("[DEBUG][ReactionHandler] Реакции поддерживаются только для MongoDB хранилища")
		}
		return nil
	}

	// Обновляем реакции в MongoDB
	return mongoStorage.UpdateMessageReactions(chatID, messageID, userID, username, firstName, reactions)
}

// isBotMessage проверяет, является ли сообщение сообщением бота
func (rh *ReactionHandler) isBotMessage(chatID int64, messageID int) bool {
	// ID бота для сравнения
	botID := rh.bot.api.Self.ID

	if rh.bot.config.Debug {
		log.Printf("[DEBUG][ReactionHandler] Проверяем является ли сообщение %d в чате %d сообщением бота (BotID=%d)", messageID, chatID, botID)
	}

	// Получаем MongoDB хранилище для прямого поиска
	mongoStorage, ok := rh.bot.storage.(*storage.PostgresStorage)
	if !ok {
		log.Printf("[ERROR][ReactionHandler] Реакции поддерживаются только для MongoDB")
		return false
	}

	// Ищем сообщение напрямую по message_id в MongoDB
	message, err := mongoStorage.GetMessageByID(chatID, messageID)
	if err != nil {
		log.Printf("[ERROR][ReactionHandler] Ошибка поиска сообщения %d в чате %d: %v", messageID, chatID, err)
		return false
	}

	if message == nil {
		log.Printf("[ERROR][ReactionHandler] Сообщение %d в чате %d не найдено", messageID, chatID)
		return false
	}

	isBotMsg := message.From != nil && message.From.ID == botID
	fromID := int64(0)
	if message.From != nil {
		fromID = message.From.ID
	}

	log.Printf("[ReactionHandler] Сообщение %d найдено: FromID=%d, BotID=%d, IsBotMessage=%v",
		messageID, fromID, botID, isBotMsg)

	return isBotMsg
}

// isBotMessageWithRetry проверяет сообщение бота с повторными попытками
func (rh *ReactionHandler) isBotMessageWithRetry(chatID int64, messageID int) bool {
	// Первая попытка
	if rh.isBotMessage(chatID, messageID) {
		if rh.bot.reactionStats != nil {
			rh.bot.reactionStats.IncrementBotMessagesFound()
		}
		return true
	}

	// Если не найдено, делаем паузу и повторяем (может быть задержка сохранения)
	log.Printf("[ReactionHandler] RETRY: Сообщение %d не найдено сразу, пауза 500мс и повторная попытка", messageID)
	time.Sleep(500 * time.Millisecond)

	if rh.isBotMessage(chatID, messageID) {
		log.Printf("[ReactionHandler] RETRY: Сообщение %d найдено при повторной попытке", messageID)
		if rh.bot.reactionStats != nil {
			rh.bot.reactionStats.IncrementBotMessagesFound()
		}
		return true
	}

	// Третья попытка через более длительную паузу
	log.Printf("[ReactionHandler] RETRY: Сообщение %d не найдено, пауза 2сек и финальная попытка", messageID)
	time.Sleep(2 * time.Second)

	result := rh.isBotMessage(chatID, messageID)
	if result {
		log.Printf("[ReactionHandler] RETRY: Сообщение %d найдено при финальной попытке", messageID)
		if rh.bot.reactionStats != nil {
			rh.bot.reactionStats.IncrementBotMessagesFound()
		}
	} else {
		log.Printf("[ReactionHandler] RETRY: Сообщение %d так и не найдено после всех попыток", messageID)
		if rh.bot.reactionStats != nil {
			rh.bot.reactionStats.IncrementBotMessagesNotFound()
		}
	}

	return result
}

// handleSpecialReactions обрабатывает специальные реакции
func (rh *ReactionHandler) handleSpecialReactions(chatID int64, messageID int, userID int64, username string, oldReactions, newReactions []string) {
	log.Printf("[ReactionHandler] АНАЛИЗ КЛОУНА: проверяем реакции старые=%v, новые=%v", oldReactions, newReactions)

	// Проверяем, добавили ли клоуна
	clownAdded := false
	for _, emoji := range newReactions {
		if emoji == "🤡" {
			log.Printf("[ReactionHandler] НАЙДЕН КЛОУН в новых реакциях: %s", emoji)
			// Проверяем, не было ли клоуна в старых реакциях
			clownWasPresent := false
			for _, oldEmoji := range oldReactions {
				if oldEmoji == "🤡" {
					clownWasPresent = true
					log.Printf("[ReactionHandler] КЛОУН УЖЕ БЫЛ в старых реакциях")
					break
				}
			}
			if !clownWasPresent {
				clownAdded = true
				log.Printf("[ReactionHandler] КЛОУН ДОБАВЛЕН ЗАНОВО!")
				break
			}
		}
	}

	if clownAdded {
		log.Printf("[ReactionHandler] ===== ОБНАРУЖЕНА НОВАЯ РЕАКЦИЯ КЛОУНА 🤡 =====")
		log.Printf("[ReactionHandler] Пользователь: %s, чат: %d", username, chatID)
		log.Printf("[ReactionHandler] Промпт для клоуна: %s", rh.bot.config.ClownReactionPrompt)
		rh.handleClownReaction(chatID, userID, username)
	} else {
		log.Printf("[ReactionHandler] Клоун не добавлен (старые: %v, новые: %v)", oldReactions, newReactions)
	}
}

// handleClownReaction обрабатывает реакцию клоуна
func (rh *ReactionHandler) handleClownReaction(chatID int64, userID int64, username string) {
	log.Printf("[ReactionHandler] ===== ОБРАБОТКА КЛОУНА =====")

	if rh.bot.config.ClownReactionPrompt == "" {
		log.Printf("[ReactionHandler] ОШИБКА: ClownReactionPrompt пустой, отменяем обработку")
		return
	}

	log.Printf("[ReactionHandler] ClownReactionPrompt: %s", rh.bot.config.ClownReactionPrompt)

	// НОВАЯ ПРОВЕРКА: Можем ли мы ответить на клоуна от этого пользователя?
	canRespond, reason := rh.clownLimiter.CanRespond(
		userID,
		rh.bot.config.ClownCooldownSeconds,
		rh.bot.config.MaxClownResponsesPerHour,
		rh.bot.config.ClownResponseProbability,
	)

	if !canRespond {
		log.Printf("[ReactionHandler] ОТКЛОНЕНО: Ответ на клоуна от пользователя %d (@%s) отклонен: %s", userID, username, reason)
		if rh.bot.reactionStats != nil {
			rh.bot.reactionStats.IncrementClownReactions() // Считаем клоуна, но не ответ
		}
		return
	}

	log.Printf("[ReactionHandler] ОДОБРЕНО: Ответ на клоуна разрешен (%s)", reason)

	// Создаем контекст БЕЗ упоминания пользователя (чтобы LLM не дублировал его)
	contextText := "Кто-то поставил реакцию клоуна 🤡 на твое сообщение."

	// Подготавливаем обращение к пользователю для добавления в финальный ответ
	userMention := ""

	// Пытаемся получить профиль пользователя для правильного обращения
	if profile, err := rh.bot.storage.GetUserProfile(chatID, userID); err == nil && profile != nil && profile.Alias != "" {
		// Используем Alias без символа @
		userMention = profile.Alias
	} else if username != "" {
		// Используем @Username только если Alias недоступен
		userMention = "@" + username
	} else {
		userMention = fmt.Sprintf("пользователь %d", userID)
	}

	log.Printf("[ReactionHandler] Контекст для LLM: %s", contextText)
	log.Printf("[ReactionHandler] Генерируем ответ через LLM...")

	// Генерируем ответ на клоуна с контекстом
	prompt := rh.bot.enrichPromptWithPersonality(rh.bot.config.ClownReactionPrompt, chatID, "clown_reaction")
	response, err := rh.bot.llm.GenerateResponseByType(llm.ResponseTypeClownReaction, prompt, contextText, float32(rh.bot.config.GeminiTemperatureNormal))
	if err != nil {
		log.Printf("[ERROR][ReactionHandler] Ошибка генерации ответа на клоуна: %v", err)
		return
	}

	log.Printf("[ReactionHandler] LLM ответ получен: %s", response)

	// Очищаем ответ
	response = cleanupLLMResponse(response)

	// Добавляем обращение к пользователю в начало ответа
	finalResponse := fmt.Sprintf("%s %s", userMention, response)

	log.Printf("[ReactionHandler] Финальный ответ: %s", finalResponse)
	log.Printf("[ReactionHandler] Отправляем ответ в чат %d", chatID)

	// Отправляем ответ
	rh.bot.sendReply(chatID, finalResponse)

	// НОВАЯ ЗАПИСЬ: Фиксируем факт ответа в лимитере
	rh.clownLimiter.RecordResponse(userID)

	// Увеличиваем счетчики статистики
	if rh.bot.reactionStats != nil {
		rh.bot.reactionStats.IncrementClownReactions()
		rh.bot.reactionStats.IncrementClownResponsesSent()
	}

	log.Printf("[ReactionHandler] ===== ОТВЕТ НА КЛОУНА ОТПРАВЛЕН =====")
}

// analyzeMessageQuality анализирует качество сообщения бота на основе реакций
func (rh *ReactionHandler) analyzeMessageQuality(chatID int64, messageID int) {
	if rh.bot.config.ReactionAnalysisPrompt == "" {
		return
	}

	// Получаем все реакции на сообщение из БД
	reactions, err := rh.getMessageReactions(chatID, messageID)
	if err != nil {
		log.Printf("[ERROR][ReactionHandler] Ошибка получения реакций для анализа: %v", err)
		return
	}

	if len(reactions) == 0 {
		return // Нет реакций для анализа
	}

	// Получаем текст сообщения
	messageText, err := rh.getMessageText(chatID, messageID)
	if err != nil {
		log.Printf("[ERROR][ReactionHandler] Ошибка получения текста сообщения для анализа: %v", err)
		return
	}

	// Формируем контекст для анализа
	reactionsList := strings.Join(reactions, ", ")
	contextText := fmt.Sprintf("Сообщение бота: \"%s\"\nПолученные реакции: %s", messageText, reactionsList)

	// Анализируем с помощью LLM
	prompt := rh.bot.enrichPromptWithPersonality(rh.bot.config.ReactionAnalysisPrompt, chatID, "reaction_analysis")
	analysis, err := rh.bot.llm.GenerateResponseByType(llm.ResponseTypeReactionAnalysis, prompt, contextText, float32(rh.bot.config.GeminiTemperatureNormal))
	if err != nil {
		log.Printf("[ERROR][ReactionHandler] Ошибка анализа качества сообщения: %v", err)
		return
	}

	// Сохраняем результат анализа в personality memory
	err = rh.saveQualityAnalysis(chatID, messageText, analysis, reactions)
	if err != nil {
		log.Printf("[ERROR][ReactionHandler] Ошибка сохранения анализа качества: %v", err)
		return
	}

	if rh.bot.config.Debug {
		log.Printf("[DEBUG][ReactionHandler] Анализ качества сообщения %d в чате %d завершен", messageID, chatID)
	}
}

// getMessageReactions получает все реакции на сообщение
func (rh *ReactionHandler) getMessageReactions(chatID int64, messageID int) ([]string, error) {
	// Получаем MongoDB хранилище
	mongoStorage, ok := rh.bot.storage.(*storage.PostgresStorage)
	if !ok {
		return nil, fmt.Errorf("реакции поддерживаются только для MongoDB")
	}

	return mongoStorage.GetMessageReactions(chatID, messageID)
}

// getMessageText получает текст сообщения
func (rh *ReactionHandler) getMessageText(chatID int64, messageID int) (string, error) {
	// Получаем сообщения из БД
	messages, err := rh.bot.storage.GetMessages(chatID, 100)
	if err != nil {
		return "", err
	}

	// Ищем нужное сообщение
	for _, msg := range messages {
		if msg.MessageID == messageID {
			if msg.Text != "" {
				return msg.Text, nil
			}
			if msg.Caption != "" {
				return msg.Caption, nil
			}
			return "[медиа без текста]", nil
		}
	}

	return "", fmt.Errorf("сообщение не найдено")
}

// saveQualityAnalysis сохраняет анализ качества в personality memory
func (rh *ReactionHandler) saveQualityAnalysis(chatID int64, messageText, analysis string, reactions []string) error {
	// Определяем, является ли сообщение качественным на основе анализа
	isQuality := rh.isQualityMessage(analysis, reactions)

	// Формируем запись для personality memory
	var perceptionText string
	if isQuality {
		perceptionText = fmt.Sprintf("ХОРОШЕЕ СООБЩЕНИЕ: \"%s\" (реакции: %s) - %s",
			messageText, strings.Join(reactions, ", "), analysis)
	} else {
		perceptionText = fmt.Sprintf("НЕУДАЧНОЕ СООБЩЕНИЕ: \"%s\" (реакции: %s) - %s",
			messageText, strings.Join(reactions, ", "), analysis)
	}

	// Сохраняем в personality memory
	return rh.bot.AddSelfPerceptionForChat(chatID, perceptionText)
}

// isQualityMessage определяет качество сообщения на основе анализа и реакций
func (rh *ReactionHandler) isQualityMessage(analysis string, reactions []string) bool {
	// Позитивные реакции
	positiveReactions := []string{"😂", "🤣", "😁", "😄", "😃", "👍", "❤️", "🔥", "💯", "👏", "🎉"}
	// Негативные реакции
	negativeReactions := []string{"🤡", "👎", "💩", "🤮", "😡", "🙄"}

	positiveCount := 0
	negativeCount := 0

	for _, reaction := range reactions {
		for _, positive := range positiveReactions {
			if reaction == positive {
				positiveCount++
				break
			}
		}
		for _, negative := range negativeReactions {
			if reaction == negative {
				negativeCount++
				break
			}
		}
	}

	// Анализируем текст анализа на ключевые слова
	analysisLower := strings.ToLower(analysis)
	positiveWords := []string{"хорошо", "удачно", "качественно", "позитивно", "успешно", "отлично"}
	negativeWords := []string{"плохо", "неудачно", "негативно", "провал", "ошибка", "неуместно"}

	analysisPositive := false
	analysisNegative := false

	for _, word := range positiveWords {
		if strings.Contains(analysisLower, word) {
			analysisPositive = true
			break
		}
	}

	for _, word := range negativeWords {
		if strings.Contains(analysisLower, word) {
			analysisNegative = true
			break
		}
	}

	// Принимаем решение на основе реакций и анализа
	if positiveCount > negativeCount && !analysisNegative {
		return true
	}
	if analysisPositive && negativeCount == 0 {
		return true
	}

	return false
}

// ClownReactionLimiter отслеживает ограничения на ответы клоуна
type ClownReactionLimiter struct {
	userCooldowns   map[int64]time.Time // UserID -> время последнего ответа
	hourlyResponses []time.Time         // Времена ответов за последний час
	mutex           sync.RWMutex
}

// NewClownReactionLimiter создает новый лимитер
func NewClownReactionLimiter() *ClownReactionLimiter {
	return &ClownReactionLimiter{
		userCooldowns:   make(map[int64]time.Time),
		hourlyResponses: make([]time.Time, 0),
	}
}

// CanRespond проверяет, можно ли ответить на клоуна от данного пользователя
func (crl *ClownReactionLimiter) CanRespond(userID int64, cooldownSeconds int, maxPerHour int, probability int) (bool, string) {
	crl.mutex.Lock()
	defer crl.mutex.Unlock()

	now := time.Now()

	// 1. Проверка вероятности
	if rand.Intn(100) >= probability {
		return false, "не прошел проверку вероятности"
	}

	// 2. Проверка cooldown для пользователя
	if lastResponse, exists := crl.userCooldowns[userID]; exists {
		if now.Sub(lastResponse) < time.Duration(cooldownSeconds)*time.Second {
			remaining := time.Duration(cooldownSeconds)*time.Second - now.Sub(lastResponse)
			return false, fmt.Sprintf("cooldown пользователя (осталось %v)", remaining.Round(time.Second))
		}
	}

	// 3. Очистка старых записей (старше часа)
	cutoff := now.Add(-time.Hour)
	filteredResponses := make([]time.Time, 0)
	for _, responseTime := range crl.hourlyResponses {
		if responseTime.After(cutoff) {
			filteredResponses = append(filteredResponses, responseTime)
		}
	}
	crl.hourlyResponses = filteredResponses

	// 4. Проверка лимита ответов в час
	if len(crl.hourlyResponses) >= maxPerHour {
		return false, "достигнут лимит ответов в час"
	}

	return true, "проверки пройдены"
}

// RecordResponse записывает факт ответа на клоуна
func (crl *ClownReactionLimiter) RecordResponse(userID int64) {
	crl.mutex.Lock()
	defer crl.mutex.Unlock()

	now := time.Now()
	crl.userCooldowns[userID] = now
	crl.hourlyResponses = append(crl.hourlyResponses, now)
}

// GetStats возвращает статистику лимитера
func (crl *ClownReactionLimiter) GetStats() map[string]interface{} {
	crl.mutex.RLock()
	defer crl.mutex.RUnlock()

	// Очистка старых записей для точной статистики
	now := time.Now()
	cutoff := now.Add(-time.Hour)
	currentHourResponses := 0
	for _, responseTime := range crl.hourlyResponses {
		if responseTime.After(cutoff) {
			currentHourResponses++
		}
	}

	return map[string]interface{}{
		"users_on_cooldown":    len(crl.userCooldowns),
		"responses_last_hour":  currentHourResponses,
		"total_recorded_users": len(crl.userCooldowns),
	}
}
