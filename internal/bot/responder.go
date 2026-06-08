package bot

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	// Для UserProfile
	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/utils" // Для TruncateString
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// === Функции для отправки ответов (перенесены из message_handler.go) ===

// sendDirectResponse обрабатывает прямое упоминание или ответ на сообщение бота
func (b *Bot) sendDirectResponse(chatID int64, message *tgbotapi.Message) {
	startTime := time.Now()
	defer func() {
		log.Printf("[DEBUG][Timing] Генерация DirectResponse (ReplyID: %d) для чата %d заняла %s",
			message.MessageID, chatID, time.Since(startTime))
	}()

	if message == nil {
		log.Printf("[ERROR][DR] Chat %d: Невозможно отправить прямой ответ, сообщение nil", chatID)
		return
	}

	log.Printf("[INFO][DR] Chat %d: Получен прямой запрос от %s (ID: %d)", chatID, message.From.UserName, message.From.ID)

	// Проверяем наличие фотографии в сообщении
	hasPhoto := len(message.Photo) > 0

	// Если в сообщении есть фотография, сначала обрабатываем её чтобы сохранить описание в хранилище
	photoDescription := ""
	if hasPhoto {
		log.Printf("[INFO][DR] Chat %d: Обнаружена фотография в прямом обращении", chatID)

		// Получаем настройки чата для проверки включения анализа фото
		settings, err := b.storage.GetChatSettings(chatID)
		photoAnalysisEnabled := b.config.PhotoAnalysisEnabled
		if err == nil && settings != nil && settings.PhotoAnalysisEnabled != nil {
			photoAnalysisEnabled = *settings.PhotoAnalysisEnabled
		}

		// Анализируем изображение только если включен PhotoAnalysisEnabled
		if photoAnalysisEnabled && b.embeddingClient != nil && b.config.GeminiAPIKey != "" {
			// Получаем самую большую фотографию (последнюю в массиве)
			photoSize := message.Photo[len(message.Photo)-1]

			// Получаем информацию о файле
			fileConfig := tgbotapi.FileConfig{
				FileID: photoSize.FileID,
			}
			file, err := b.api.GetFile(fileConfig)
			if err != nil {
				log.Printf("[ERROR][DR] Chat %d: Не удалось получить информацию о фото: %v", chatID, err)
			} else {
				// Загружаем файл
				fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.api.Token, file.FilePath)
				client := &http.Client{}
				resp, err := client.Get(fileURL)
				if err != nil {
					log.Printf("[ERROR][DR] Chat %d: Не удалось загрузить фото: %v", chatID, err)
				} else {
					defer resp.Body.Close()

					// Читаем содержимое файла
					photoFileBytes, err := io.ReadAll(resp.Body)
					if err != nil {
						log.Printf("[ERROR][DR] Chat %d: Не удалось прочитать содержимое фото: %v", chatID, err)
					} else {
						// Анализируем фото с помощью Gemini
						photoDescription, err = b.analyzeImageWithGemini(context.Background(), chatID, photoFileBytes, message.Caption)
						if err != nil {
							log.Printf("[ERROR][DR] Chat %d: Ошибка при анализе фото: %v", chatID, err)
						} else {
							log.Printf("[INFO][DR] Chat %d: Успешно проанализирована фотография в прямом обращении", chatID)

							// Сохраняем текстовое описание изображения в хранилище
							// Случайно выбираем один из вариантов названия для изображения
							imageLabels := []string{"[Картинка]", "[Фотка]", "[Изображение]", "[Пикча]", "[Фото]"}
							selectedLabel := imageLabels[b.randSource.Intn(len(imageLabels))]

							textMessage := &tgbotapi.Message{
								MessageID: message.MessageID,
								From:      message.From,
								Chat:      message.Chat,
								Date:      message.Date,
								Text:      selectedLabel + ": " + photoDescription,
							}
							b.storage.AddMessage(chatID, textMessage)
						}
					}
				}
			}
		}
	}

	// Получаем последние сообщения из чата для контекста
	messages, err := b.storage.GetMessages(chatID, b.config.ContextWindow)
	if err != nil {
		log.Printf("[ERROR][DR] Chat %d: Ошибка при получении истории сообщений: %v", chatID, err)
		// ИСПРАВЛЕНО: Используем sendAutoDeleteErrorReply для технических ошибок
		b.sendAutoDeleteErrorReply(chatID, message.MessageID, "⚠️ Извините, произошла ошибка при обработке вашего запроса.")
		return
	}

	// Получаем цепочку сообщений, на которые отвечали
	var replyChain []*tgbotapi.Message
	if message.ReplyToMessage != nil {
		// Увеличиваем глубину цепочки ответов до 10 для лучшего понимания контекста
		replyChain, err = b.storage.GetReplyChain(context.Background(), chatID, message.ReplyToMessage.MessageID, 10)
		if err != nil {
			log.Printf("[WARN][DR] Chat %d: Ошибка при получении цепочки ответов: %v", chatID, err)
			// Продолжаем работу даже если не удалось получить цепочку ответов
		}
	}

	// Получаем релевантные сообщения с использованием долгосрочной памяти, если она включена
	var relevantMessages []*tgbotapi.Message
	if b.config.LongTermMemoryEnabled {
		// Объединяем текст сообщения с описанием фотографии, если оно есть
		queryText := message.Text
		if hasPhoto && photoDescription != "" {
			if queryText != "" {
				queryText += "\n\n" + photoDescription
			} else {
				queryText = photoDescription
			}
		}

		// Если текст пустой (например, только фото без подписи), используем базовый текст
		if queryText == "" {
			queryText = "фотография"
		}

		// Ищем релевантные сообщения
		relevantMsgs, err := b.storage.SearchRelevantMessages(chatID, queryText, b.config.LongTermMemoryFetchK)
		if err != nil {
			log.Printf("[WARN][DR] Chat %d: Ошибка при поиске релевантных сообщений: %v", chatID, err)
		} else {
			relevantMessages = relevantMsgs
			if b.config.Debug {
				log.Printf("[DEBUG][DR] Chat %d: Найдено %d релевантных сообщений", chatID, len(relevantMessages))
			}
		}
	}

	// Форматируем контекст
	formattedContext := formatDirectReplyContext(chatID, message, replyChain, messages, relevantMessages, b.storage, b.config, b.config.TimeZone)

	// Если в сообщении есть фотография, добавляем информацию о ней в контекст
	if hasPhoto && photoDescription != "" {
		// Добавляем описание фотографии в контекст
		userInfo := fmt.Sprintf("Пользователь %s прикрепил к сообщению фотографию", message.From.UserName)
		if message.Caption != "" {
			userInfo += fmt.Sprintf(" с подписью: \"%s\"", message.Caption)
		}
		photoInfo := fmt.Sprintf("%s\nОписание фотографии: %s", userInfo, photoDescription)

		// Добавляем информацию о фотографии в начало контекста
		formattedContext = photoInfo + "\n\n" + formattedContext
	}

	// Начинаем анализ типа сообщения - серьезное или обычное
	msgType := "regular"

	// Классифицируем только если есть не-пустой текст или если есть фотография с описанием
	if message.Text != "" || (hasPhoto && photoDescription != "") {
		// Формируем входной текст для классификации, объединяя текст сообщения и описание фотографии
		classifyInput := message.Text
		if hasPhoto && photoDescription != "" {
			if classifyInput != "" {
				classifyInput += "\n\n[Содержимое фотографии]: " + photoDescription
			} else {
				classifyInput = "[Содержимое фотографии]: " + photoDescription
			}
		}

		if b.config.ClassifyDirectMessagePrompt != "" {
			// Классифицируем сообщение
			classifyResult, err := b.llm.GenerateResponseByType(
				llm.ResponseTypeClassify,
				b.config.ClassifyDirectMessagePrompt,
				classifyInput,
				float32(b.config.GeminiTemperatureSerious),
			)
			if err != nil {
				log.Printf("[WARN][DR] Chat %d: Ошибка при классификации сообщения: %v", chatID, err)
			} else {
				// Очищаем результат классификации от возможных метаданных
				classifyResult = cleanupLLMResponse(classifyResult)

				// Проверяем результат классификации
				lower := strings.ToLower(strings.TrimSpace(classifyResult))
				log.Printf("[DEBUG][DR] Chat %d: Результат классификации: '%s'", chatID, lower)

				// Расширенная проверка с учетом возможных вариантов ответа на русском и английском
				if strings.Contains(lower, "serious") ||
					strings.Contains(lower, "серьезн") ||
					strings.Contains(lower, "серьёзн") ||
					lower == "yes" ||
					lower == "да" {
					msgType = "serious"
					log.Printf("[DEBUG][DR] Chat %d: Сообщение классифицировано как SERIOUS", chatID)
				} else if strings.Contains(lower, "casual") ||
					strings.Contains(lower, "обычн") ||
					strings.Contains(lower, "несерьезн") ||
					strings.Contains(lower, "несерьёзн") ||
					lower == "no" ||
					lower == "нет" {
					log.Printf("[DEBUG][DR] Chat %d: Сообщение классифицировано как CASUAL", chatID)
				} else {
					// Если ответ LLM непонятен, но содержит вопросительный знак, считаем сообщение серьезным
					if strings.Contains(message.Text, "?") || strings.Contains(message.Text, "？") {
						msgType = "serious"
						log.Printf("[DEBUG][DR] Chat %d: Сообщение содержит вопросительный знак, классифицировано как SERIOUS", chatID)
					} else {
						log.Printf("[DEBUG][DR] Chat %d: Результат классификации не распознан, использую REGULAR", chatID)
					}
				}
			}
		}
	}

	// Выбираем промпт в зависимости от типа сообщения
	var responsePrompt string
	if msgType == "serious" && b.config.SeriousDirectPrompt != "" {
		responsePrompt = b.config.SeriousDirectPrompt
		log.Printf("[INFO][DR] Chat %d: Используем SERIOUS_DIRECT_PROMPT", chatID)

		// Для серьезных ответов добавляем веб-поиск
		if b.webSearch.IsEnabled() {
			enhancedContext := b.webSearch.EnhanceContextWithWebSearch(formattedContext, message.Text)
			if enhancedContext != formattedContext {
				formattedContext = enhancedContext
				log.Printf("[INFO][DR] Chat %d: Контекст расширен результатами веб-поиска", chatID)
			}
		}
	} else {
		responsePrompt = b.config.DirectPrompt
		log.Printf("[INFO][DR] Chat %d: Используем стандартный DIRECT_PROMPT", chatID)
	}

	// --- НОВЫЙ КОД: Определяем температуру для ответа ---
	var responseTemperature float32
	if msgType == "serious" {
		responseTemperature = float32(b.config.GeminiTemperatureSerious)
	} else {
		responseTemperature = float32(b.config.GeminiTemperatureNormal)
	}
	// --- КОНЕЦ НОВОГО КОДА ---

	log.Printf("[INFO][DR] Chat %d: Генерируем ответ. Тип: %s, Промпт: %s..., Температура: %.2f", chatID, msgType, utils.TruncateString(responsePrompt, 30), responseTemperature)

	// Встраиваем личность в промпт
	enrichedPrompt := b.enrichPromptWithPersonality(responsePrompt, chatID, "direct")

	// === КОГНИТИВНАЯ ИНТЕГРАЦИЯ (ЭТАП 3): Внутренний монолог перед ответом ===
	if b.config.InternalMonologueEnabled {
		trigger := message.Text
		thought := b.InternalMonologue(chatID, trigger, "direct")
		if thought != nil {
			// Маркируем, что мысль повлияла на ответ
			thought.ActionTaken = true
			b.RecordInternalThought(chatID, thought)
			log.Printf("[Stage3][DR] Чат %d: injected internal_thought type=%s len=%d", chatID, thought.Type, len(thought.Content))
			// Легко подмешиваем краткий след мысли в контекст (не выводится пользователю, только как подсказка модели)
			formattedContext = "[internal_thought]: " + utils.TruncateString(thought.Content, 120) + "\n\n" + formattedContext
		}
	}

	// Подмешиваем лёгкий ассоциативный контекст (зафичено флагом)
	if assoc := b.getAssociativeContext(chatID, nil, 3); assoc != "" {
		formattedContext = assoc + "\n\n" + formattedContext
	}

	// === СОЦИАЛЬНАЯ ИНТЕГРАЦИЯ (ЭТАП 4): Контекст отношений с адресатом ===
	if b.config.RelationshipTrackingEnabled && message.From != nil {
		userID := int64(message.From.ID)
		relCtx := b.AnalyzeRelationshipForPrompt(chatID, userID)
		if relCtx != "" {
			formattedContext = relCtx + "\n" + formattedContext
		}
		// Детерминированная подсказка стиля на основе отношений
		before := len(formattedContext)
		formattedContext = b.ApplyRelationshipStyleToContext(chatID, userID, formattedContext)
		if len(formattedContext) > before {
			style := b.GetRelationshipInfluencedCommunicationStyle(chatID, userID)
			log.Printf("[Stage4][DR] Chat %d: tone_hint applied, style=%s", chatID, style)
		}
	}

	// === ИНТЕГРАЦИЯ КАУЗАЛЬНОГО ВЛИЯНИЯ (ЭТАП 1) ===
	// Получаем влияние каузальной памяти на текущую ситуацию
	if b.config.CausalLearningEnabled {
		// Формируем описание текущей ситуации для каузального анализа
		situationDescription := fmt.Sprintf(
			"Генерация прямого ответа на сообщение: '%s'. Тип сообщения: %s. Пользователь: %d",
			message.Text, msgType, message.From.ID,
		)

		causalInfluence, err := b.GetCausalInfluence(chatID, situationDescription)
		if err != nil {
			log.Printf("[WARN][DR] Ошибка получения каузального влияния: %v", err)
		} else if causalInfluence != nil && len(causalInfluence.BehavioralAdjustments) > 0 {
			// Применяем каузальные корректировки к контексту
			formattedContext = b.applyCausalInfluenceToContext(formattedContext, causalInfluence)
			log.Printf("[DEBUG][DR] Чат %d: Применены каузальные корректировки к контексту", chatID)
		}
	}

	// === ИНТЕГРАЦИЯ ЭМОЦИОНАЛЬНОЙ АДАПТАЦИИ (ЭТАП 2) ===
	// Применяем эмоциональную адаптацию для понимания пользователя
	if b.config.EmotionalLearningEnabled && message.From != nil {
		enrichedPrompt = b.ApplyEmotionalAdaptationToPrompt(enrichedPrompt, chatID, message.From.ID)
		log.Printf("[DEBUG][DR] Чат %d: Применена эмоциональная адаптация для пользователя %d", chatID, message.From.ID)
	}

	// Генерируем ответ
	var responseType llm.ResponseType
	if msgType == "serious" {
		responseType = llm.ResponseTypeSerious
	} else {
		responseType = llm.ResponseTypeDirect
	}
	responseText, err := b.llm.GenerateResponseByType(responseType, enrichedPrompt, formattedContext, responseTemperature)
	if err != nil {
		log.Printf("[ERROR][DR] Chat %d: Ошибка при генерации ответа LLM: %v", chatID, err)
		// ИСПРАВЛЕНО: Используем sendAutoDeleteErrorReply для технических ошибок (не сохраняется в историю)
		b.sendAutoDeleteErrorReply(chatID, message.MessageID, "⚠️ Извините, произошла ошибка при генерации ответа.")
		return
	}

	// Очищаем ответ от возможных метаданных перед отправкой
	log.Printf("🧹 [RESPONDER] Очистка ответа DirectResponse для чата %d (исходная длина: %d)", chatID, len(responseText))
	responseText = cleanupLLMResponse(responseText)

	// ОБНОВЛЯЕМ PERSONALITY MEMORY на основе диалога
	go func() {
		// Обновляем имена (из сообщения пользователя)
		if message.From != nil {
			// Добавляем имя пользователя
			if message.From.FirstName != "" {
				b.AddNameMentionForChat(chatID, message.From.FirstName)
			}
			if message.From.UserName != "" {
				b.AddNameMentionForChat(chatID, message.From.UserName)
			}
		}

		// Анализируем и добавляем темы из сообщения пользователя
		if message.Text != "" {
			// Извлекаем ключевые слова как темы (простая эвристика)
			words := strings.Fields(strings.ToLower(message.Text))
			for _, word := range words {
				if len(word) > 4 && !strings.Contains(word, "@") { // Фильтруем короткие слова и упоминания
					b.AddRecentTopicForChat(chatID, word)
				}
			}
		}

		// Анализируем и обновляем самовосприятие на основе своего ответа
		if responseText != "" {
			// Если в ответе есть самоидентификация, извлекаем её
			lowerResponse := strings.ToLower(responseText)
			if strings.Contains(lowerResponse, "я ") {
				// Примитивная эвристика для извлечения самоидентификации
				sentences := strings.Split(responseText, ".")
				for _, sentence := range sentences {
					sentence = strings.TrimSpace(sentence)
					if len(sentence) > 10 && strings.Contains(strings.ToLower(sentence), "я ") {
						b.AddSelfPerceptionForChat(chatID, sentence)
						break // Добавляем только первое найденное
					}
				}
			}
		}
	}()

	// Отправляем ответ, делая reply на сообщение пользователя с проверкой на повторения
	userID := int64(message.From.ID)
	b.sendReplyToWithAntiRepetition(chatID, message.MessageID, responseText, userID, "direct_response")

	// Логирование времени выполнения
	duration := time.Since(startTime)
	log.Printf("[DEBUG][Timing] Генерация DirectResponse (ReplyID: %d) для чата %d заняла %v", message.MessageID, chatID, duration)
}

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
	// Используем стандартное окно контекста и форматирование с профилями
	contextMessages := messagesForContext
	if len(contextMessages) > b.config.ContextWindow {
		contextMessages = contextMessages[len(contextMessages)-b.config.ContextWindow:]
	}
	// Используем новый унифицированный форматтер для общих ответов AI
	formatter := NewUnifiedMessageFormatter(b.storage, b.config.TimeZone)
	contextText := formatter.FormatMessages(chatID, contextMessages)

	log.Printf("[Responder] Chat %d: Использован унифицированный форматтер для %d сообщений", chatID, len(contextMessages))

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
			// Проверяем, нужен ли веб-поиск для последнего сообщения пользователя
			if b.webSearch.ShouldPerformSearch(lastUserMessage.Text) {
				enhancedContext := b.webSearch.EnhanceContextWithWebSearch(contextText, lastUserMessage.Text)
				if enhancedContext != contextText {
					contextText = enhancedContext
					log.Printf("[INFO][AI] Chat %d: Контекст расширен результатами веб-поиска для обычного ответа", chatID)
				}
			}
		}
	}

	// 3. Проверяем, нужно ли отвечать (например, если последнее сообщение от бота)
	//   Эту логику пока убрали, предполагаем, что если функция вызвана, ответ нужен.

	// --- НОВЫЙ КОД: Используем НОРМАЛЬНУЮ температуру для обычных ответов ---
	responseTemperature := float32(b.config.GeminiTemperatureNormal)
	// --- КОНЕЦ НОВОГО КОДА ---

	// 4. Используем основной промпт и генерируем ответ
	systemPrompt := b.config.DefaultPrompt
	// Встраиваем личность в промпт
	enrichedPrompt := b.enrichPromptWithPersonality(systemPrompt, chatID, "default")

	// Подмешиваем лёгкий ассоциативный контекст (зафичено флагом)
	if assoc := b.getAssociativeContext(chatID, nil, 3); assoc != "" {
		contextText = assoc + "\n\n" + contextText
	}
	log.Printf("[INFO][AI] Chat %d: Генерируем обычный ответ. Температура: %.2f", chatID, responseTemperature)

	// === ИНТЕГРАЦИЯ КАУЗАЛЬНОГО ВЛИЯНИЯ (ЭТАП 1) ===
	// Получаем влияние каузальной памяти на текущую ситуацию
	if b.config.CausalLearningEnabled {
		// Формируем описание текущей ситуации для каузального анализа
		situationDescription := fmt.Sprintf(
			"Генерация обычного AI ответа в чате. Количество сообщений в контексте: %d",
			len(contextMessages),
		)

		causalInfluence, err := b.GetCausalInfluence(chatID, situationDescription)
		if err != nil {
			log.Printf("[WARN][AI] Ошибка получения каузального влияния: %v", err)
		} else if causalInfluence != nil && len(causalInfluence.BehavioralAdjustments) > 0 {
			// Применяем каузальные корректировки к контексту
			contextText = b.applyCausalInfluenceToContext(contextText, causalInfluence)
			log.Printf("[DEBUG][AI] Чат %d: Применены каузальные корректировки к контексту", chatID)
		}
	}

	// === ИНТЕГРАЦИЯ ЭМОЦИОНАЛЬНОЙ АДАПТАЦИИ (ЭТАП 2) ===
	// Применяем эмоциональную адаптацию для понимания последнего пользователя
	if b.config.EmotionalLearningEnabled && len(contextMessages) > 0 {
		// Ищем последнее сообщение от пользователя (не от бота)
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

	response, err := b.llm.GenerateResponseByType(llm.ResponseTypeDefault, enrichedPrompt, contextText, responseTemperature)
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

// getDirectReplyLimitSettings читает настройки лимита из хранилища или конфига
func (b *Bot) getDirectReplyLimitSettings(chatID int64) (enabled bool, count int, duration time.Duration) {
	b.settingsMutex.RLock() // Только читаем
	defer b.settingsMutex.RUnlock()

	dbSettings, errDb := b.storage.GetChatSettings(chatID)
	if errDb != nil {
		log.Printf("[WARN][getDirectReplyLimitSettings] Chat %d: Ошибка получения настроек из БД: %v. Использую дефолтные.", chatID, errDb)
		dbSettings = nil
	}

	enabled = b.config.DirectReplyLimitEnabledDefault
	count = b.config.DirectReplyLimitCountDefault
	duration = b.config.DirectReplyLimitDurationDefault

	if dbSettings != nil {
		if dbSettings.DirectReplyLimitEnabled != nil {
			enabled = *dbSettings.DirectReplyLimitEnabled
		}
		if dbSettings.DirectReplyLimitCount != nil {
			count = *dbSettings.DirectReplyLimitCount
		}
		if dbSettings.DirectReplyLimitDuration != nil {
			duration = time.Duration(*dbSettings.DirectReplyLimitDuration) * time.Minute
		}
	}
	return
}

// checkDirectReplyLimit проверяет, превышен ли лимит прямых обращений к боту для пользователя.
// Возвращает true, если лимит ПРЕВЫШЕН.
// Переписано для корректной и безопасной работы с мьютексом и картой.
func (b *Bot) checkDirectReplyLimit(chatID int64, userID int64) bool {
	// Получаем актуальные настройки из хранилища
	settings, err := b.storage.GetChatSettings(chatID)
	if err != nil {
		log.Printf("[ERROR][checkDirectReplyLimit] Не удалось получить настройки чата %d: %v. Пропускаю проверку.", chatID, err)
		return false // Считаем, что лимит не превышен
	}

	// Проверяем, включен ли лимит
	limitEnabled := b.config.DirectReplyLimitEnabledDefault
	if settings != nil && settings.DirectReplyLimitEnabled != nil {
		limitEnabled = *settings.DirectReplyLimitEnabled
	}
	if !limitEnabled {
		return false // Лимит выключен
	}

	// Получаем значения лимита
	limitCount := b.config.DirectReplyLimitCountDefault
	if settings != nil && settings.DirectReplyLimitCount != nil {
		limitCount = *settings.DirectReplyLimitCount
	}
	limitDuration := b.config.DirectReplyLimitDurationDefault
	if settings != nil && settings.DirectReplyLimitDuration != nil {
		limitDuration = time.Duration(*settings.DirectReplyLimitDuration) * time.Minute
	}

	// Используем полную блокировку для безопасного чтения и возможной записи
	b.settingsMutex.Lock()
	defer b.settingsMutex.Unlock() // Гарантируем разблокировку

	// --- Ensure the map for the chatID exists ---
	if _, ok := b.directReplyTimestamps[chatID]; !ok {
		b.directReplyTimestamps[chatID] = make(map[int64][]time.Time)
		if b.config.Debug {
			log.Printf("[DEBUG][RateLimit] Initialized directReplyTimestamps map for chat %d", chatID)
		}
	}
	// --- End ensure map exists ---

	// Now it's safe to access b.directReplyTimestamps[chatID]
	userTimestamps := b.directReplyTimestamps[chatID][userID] // Read timestamps for the user

	// Clean up timestamps older than the duration
	now := time.Now()
	cutoff := now.Add(-limitDuration)
	cleanedTimestamps := make([]time.Time, 0, len(userTimestamps))
	for _, ts := range userTimestamps {
		if ts.After(cutoff) {
			cleanedTimestamps = append(cleanedTimestamps, ts)
		}
	}
	userTimestamps = cleanedTimestamps // Update userTimestamps with cleaned slice

	// Check limit BEFORE adding the new timestamp
	if len(userTimestamps) >= limitCount {
		log.Printf("[INFO][RateLimit] Chat %d, User %d: Direct reply limit exceeded (%d/%d in %v)", chatID, userID, len(userTimestamps), limitCount, limitDuration)
		// Don't add the timestamp if the limit is already hit
		return false // Limit exceeded
	}

	// Append the new timestamp to the cleaned slice (limit not exceeded)
	b.directReplyTimestamps[chatID][userID] = append(userTimestamps, now)

	if b.config.Debug {
		log.Printf("[DEBUG][RateLimit] Chat %d, User %d: Timestamp added. Count: %d/%d", chatID, userID, len(b.directReplyTimestamps[chatID][userID]), limitCount)
	}
	return true // Limit not exceeded
}

// sendDirectLimitExceededReply отправляет сообщение о превышении лимита прямых обращений.
func (b *Bot) sendDirectLimitExceededReply(chatID int64, replyToMessageID int) {
	if b.config.DirectReplyLimitPrompt == "" {
		b.sendReplyTo(chatID, replyToMessageID, "Слишком много обращений. Попробуйте позже.") // ИСПРАВЛЕНО: sendReplyToMessage -> sendReplyTo
		return
	}

	// --- НОВЫЙ КОД: Используем НОРМАЛЬНУЮ температуру для сообщения о лимите ---
	limitMsgTemperature := float32(b.config.GeminiTemperatureNormal)
	// --- КОНЕЦ НОВОГО КОДА ---

	prompt := b.enrichPromptWithPersonality(b.config.DirectReplyLimitPrompt, chatID, "direct_reply_limit")
	generatedText, err := b.llm.GenerateResponseByType(llm.ResponseTypeRateLimit, prompt, "", limitMsgTemperature)
	if err != nil {
		log.Printf("[WARN] Chat %d: Не удалось сгенерировать сообщение о превышении лимита: %v", chatID, err)
		b.sendReplyTo(chatID, replyToMessageID, "Слишком много обращений. Попробуйте позже.") // ИСПРАВЛЕНО: sendReplyToMessage -> sendReplyTo
		return
	}
	b.sendReplyTo(chatID, replyToMessageID, cleanupLLMResponse(generatedText)) // ИСПРАВЛЕНО: sendReplyToMessage -> sendReplyTo
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
