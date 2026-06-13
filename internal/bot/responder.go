package bot

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/bot/prompts"
	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// === Функции для отправки ответов (перенесены из message_handler.go) ===

// sendAIResponse генерирует и отправляет обычный AI ответ в чат
func (b *Bot) sendAIResponse(chatID int64) {
	log.Printf("[INFO][AI] Chat %d: Попытка генерации AI ответа...", chatID)
	startTime := time.Now()
	defer func() {
		log.Printf("[DEBUG][Timing] Генерация AIResponse для чата %d заняла %s",
			chatID, time.Since(startTime))
	}()

	// 1. Получаем последние сообщения
	messagesForContext, err := b.storage.GetMessages(chatID, b.config.MaxMessages)
	if err != nil {
		log.Printf("[ERROR][sendAIResponse] Ошибка получения сообщений для AI ответа в чате %d: %v", chatID, err)
		return
	}
	if len(messagesForContext) == 0 {
		log.Printf("[INFO][sendAIResponse] Нет сообщений для генерации AI ответа в чате %d", chatID)
		return // Нечего обрабатывать
	}

	// 2. Формируем контекст для LLM
	// Используем стандартное окно контекста
	contextMessages := messagesForContext
	if len(contextMessages) > b.config.ContextWindow {
		contextMessages = contextMessages[len(contextMessages)-b.config.ContextWindow:]
	}

	// Build ChatML context
	formatter := NewUnifiedMessageFormatter(b.storage, b.config.TimeZone)
	formatter.SetDisableUserProfiles(b.config.DisableUserProfiles)
	chatHistory := formatter.FormatSortedChatMessages(chatID, contextMessages)

	var stateData *prompts.TemplateData
	if b.stateProvider != nil {
		stateData = b.stateProvider.CollectState(chatID, 0)
	}
	systemMsg := b.BuildChatSystemMessage(chatID, 0, stateData)

	allMsgs := make([]llm.ChatMessage, 0, len(chatHistory)+1)
	allMsgs = append(allMsgs, systemMsg)
	allMsgs = append(allMsgs, chatHistory...)

	// Find last user message and append to ChatML if not already present
	for i := len(contextMessages) - 1; i >= 0; i-- {
		m := contextMessages[i]
		if m.From != nil && m.From.ID != b.api.Self.ID && m.Text != "" {
			// Append as user message (will be last message for positional anchoring)
			allMsgs = append(allMsgs, llm.ChatMessage{Role: "user", Content: m.Text})
			break
		}
	}

	log.Printf("[Responder] Chat %d: Использован ChatML форматтер для %d сообщений", chatID, len(chatHistory))

	// 2.5. НОВОЕ: Проверяем веб-поиск для последнего сообщения пользователя
	if b.webSearch != nil && b.webSearch.IsEnabled() && len(messagesForContext) > 0 {
		// Находим последнее сообщение пользователя (не от бота)
		var lastUserMessage *tgbotapi.Message
		for i := len(messagesForContext) - 1; i >= 0; i-- {
			if messagesForContext[i].From != nil && messagesForContext[i].From.ID != b.api.Self.ID {
				lastUserMessage = messagesForContext[i]
				break
			}
		}

		if lastUserMessage != nil && lastUserMessage.Text != "" {
			if b.webSearch.ShouldPerformSearch(lastUserMessage.Text) {
				flatCtx := llm.FlattenChatMessages(allMsgs)
				enhancedContext := b.webSearch.EnhanceContextWithWebSearch(flatCtx, lastUserMessage.Text)
				if enhancedContext != flatCtx {
					allMsgs[0].Content = allMsgs[0].Content + "\n\n=== РЕЗУЛЬТАТЫ ПОИСКА ===\n" + enhancedContext
					log.Printf("[INFO][AI] Chat %d: Контекст расширен результатами веб-поиска для обычного ответа", chatID)
				}
			}
		}
	}

	// 3. Проверяем, нужно ли отвечать (например, если последнее сообщение от бота)
	//   Эту логику пока убрали, предполагаем, что если функция вызвана, ответ нужен.

	// Use package-level constant 0.7 for creative text responses
	respTemp := float32(responseTemperature)

	// 4. Используем основной промпт и генерируем ответ
	systemPrompt := b.config.DefaultPrompt
	// Встраиваем личность в промпт
	enrichedPrompt := b.enrichPromptWithPersonality(systemPrompt, chatID, "default")

	// === ИНТЕГРАЦИЯ ЭМОЦИОНАЛЬНОЙ АДАПТАЦИИ (ЭТАП 2) (before prompt prepend) ===
	if b.config.EmotionalLearningEnabled && len(contextMessages) > 0 {
		var lastUserID int64
		for i := len(contextMessages) - 1; i >= 0; i-- {
			msg := contextMessages[i]
			if msg.From != nil && msg.From.ID != b.api.Self.ID {
				lastUserID = msg.From.ID
				break
			}
		}
		if lastUserID != 0 {
			enrichedPrompt = b.ApplyEmotionalAdaptationToPrompt(enrichedPrompt, chatID, lastUserID)
			log.Printf("[DEBUG][AI] Чат %d: Применена эмоциональная адаптация для пользователя %d", chatID, lastUserID)
		}
	}

	// Prepend enriched prompt to system message
	allMsgs[0].Content = enrichedPrompt + "\n\n" + allMsgs[0].Content

	// Подмешиваем лёгкий ассоциативный контекст (зафичено флагом)
	if assoc := b.getAssociativeContext(chatID, nil, 3); assoc != "" {
		allMsgs[0].Content = allMsgs[0].Content + "\n\n" + assoc
	}
	log.Printf("[INFO][AI] Chat %d: Генерируем обычный ответ. Температура: %.2f", chatID, respTemp)

	// === ИНТЕГРАЦИЯ КАУЗАЛЬНОГО ВЛИЯНИЯ (ЭТАП 1) ===
	if b.config.CausalLearningEnabled {
		situationDescription := fmt.Sprintf(
			"Генерация обычного AI ответа в чате. Количество сообщений в контексте: %d",
			len(contextMessages),
		)

		causalInfluence, err := b.GetCausalInfluence(chatID, situationDescription)
		if err != nil {
			log.Printf("[WARN][AI] Ошибка получения каузального влияния: %v", err)
		} else if causalInfluence != nil && len(causalInfluence.BehavioralAdjustments) > 0 {
			flattened := llm.FlattenChatMessages(allMsgs)
			flattened = b.applyCausalInfluenceToContext(flattened, causalInfluence)
			allMsgs[0].Content = allMsgs[0].Content + "\n\n" + flattened
			log.Printf("[DEBUG][AI] Чат %d: Применены каузальные корректировки к контексту", chatID)
		}
	}

	b.setTypingAction(chatID)
	response, err := b.llm.GenerateChatResponse(llm.ResponseTypeDefault, allMsgs, respTemp)
	if err != nil {
		log.Printf("[ERROR][sendAIResponse] Ошибка генерации AI ответа для чата %d: %v", chatID, err)
		return
	}

	// 5. Очищаем ответ от возможных метаданных перед отправкой
	log.Printf("🧹 [RESPONDER] Очистка ответа AIResponse для чата %d (исходная длина: %d)", chatID, len(response))
	response = cleanupLLMResponse(response)

	// Проверка на пустой ответ
	if strings.TrimSpace(response) == "" {
		log.Printf("[INFO][sendAIResponse] LLM вернул пустой ответ для чата %d", chatID)
		return
	}

	// 6. Отправляем ответ с проверкой на повторения
	b.sendReplyWithAntiRepetition(chatID, response, 0, "ai_response")
}

// sendErrorReply отправляет стандартизированное сообщение об ошибке в чат.
func (b *Bot) sendErrorReply(chatID int64, replyToMessageID int, errorContext string) {
	// Логируем детальную ошибку перед отправкой общего сообщения
	log.Printf("[ERROR] Подробности ошибки для чата %d (ReplyTo: %d): %s", chatID, replyToMessageID, errorContext)

	// DEBUG информация НЕ должна попадать в чат - только в логи
	// errorMsg остается человечным независимо от режима отладки
	errorMsg := "⚠️ Извините, возникла проблема при генерации ответа."

	// Если включен режим отладки, добавляем контекст ошибки в сообщение
	if b.config.Debug {
		errorMsg = fmt.Sprintf("❌ Ошибка (%s)", errorContext)
	}

	// Используем новую функцию для автоудаления сообщения об ошибке
	b.sendAutoDeleteErrorReply(chatID, replyToMessageID, errorMsg)
}
