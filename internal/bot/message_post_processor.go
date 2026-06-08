package bot

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MessageType определяет тип сообщения для постобработки
type MessageType string

const (
	MessageTypeDefault       MessageType = "default"
	MessageTypeDirect        MessageType = "direct"
	MessageTypeSummary       MessageType = "summary"
	MessageTypeWeeklySummary MessageType = "weekly_summary"
	MessageTypeVoice         MessageType = "voice"
	MessageTypeTake          MessageType = "take"
	MessageTypeFreeWill      MessageType = "free_will"
	MessageTypeDirectSerious MessageType = "direct_serious"
	MessageTypeSystem        MessageType = "system"
	MessageTypeError         MessageType = "error"
	MessageTypeAdmin         MessageType = "admin"
)

// ProcessingType представляет тип постобработки
type ProcessingType string

const (
	ProcessingTypeSingleWord     ProcessingType = "single_word"
	ProcessingTypeShortSentences ProcessingType = "short_sentences"
	ProcessingTypeLongMessages   ProcessingType = "long_messages"
	ProcessingTypeIntelligent    ProcessingType = "intelligent"
	ProcessingTypeSummary        ProcessingType = "summary"
	ProcessingTypeNone           ProcessingType = "none"
)

// MessageCacheEntry представляет запись в кэше постобработки
type MessageCacheEntry struct {
	ProcessedText  string
	Timestamp      time.Time
	ProcessingType ProcessingType
}

// ReplacementEntry представляет одну замену имени пользователя
type ReplacementEntry struct {
	From string
	To   string
}

// ChatReplacementCache представляет кэш замен для одного чата
type ChatReplacementCache struct {
	ChatID       int64
	Replacements []ReplacementEntry
	Timestamp    time.Time
	UserCount    int // Количество пользователей при создании кэша
}

// MessagePostProcessor представляет систему постобработки сообщений
type MessagePostProcessor struct {
	bot              *Bot
	mutex            sync.RWMutex
	cache            map[string]MessageCacheEntry
	replacementCache map[int64]*ChatReplacementCache // Кэш замен по chatID
	replacementMutex sync.RWMutex                    // Отдельный мьютекс для кэша замен
	stats            struct {
		TotalProcessed        int64
		ProcessedByType       map[ProcessingType]int64
		CacheHits             int64
		CacheMisses           int64
		ProcessingTimeTotal   time.Duration
		AverageProcessingTime time.Duration

		// Расширенная статистика для отладки
		SkippedByDisabled   int64
		SkippedByExcluded   int64
		SkippedByTooShort   int64
		SkippedByTooLong    int64
		SkippedByNoneType   int64
		LLMErrors           int64
		EmptyLLMResponses   int64
		CacheExpiredEntries int64
		LastStatsLogTime    time.Time

		// Статистика кэша замен
		ReplacementCacheHits     int64
		ReplacementCacheMisses   int64
		ReplacementCacheBuilds   int64
		ReplacementCacheCleanups int64
	}
	config struct {
		enabled                      bool
		randomizationEnabled         bool
		singleWordProbability        float64
		shortSentencesProbability    float64
		longMessagesProbability      float64
		minLength                    int
		maxLength                    int
		longMessageThreshold         int
		forceLongProcessingThreshold int
		timeoutSeconds               int
		temperature                  float64
		cacheEnabled                 bool
		cacheTTLMinutes              int
		excludeTypes                 []string
		weeklySummaryExclude         bool
		debugLogging                 bool
		logOriginalMessages          bool

		// Настройки кэша замен
		replacementCacheEnabled    bool
		replacementCacheTTLMinutes int
	}
}

// NewMessagePostProcessor создает новый экземпляр MessagePostProcessor
func NewMessagePostProcessor(bot *Bot) *MessagePostProcessor {
	mpp := &MessagePostProcessor{
		bot:              bot,
		cache:            make(map[string]MessageCacheEntry),
		replacementCache: make(map[int64]*ChatReplacementCache),
	}

	// Загружаем конфигурацию
	config := bot.config
	mpp.config.enabled = config.MessagePostProcessorEnabled
	mpp.config.randomizationEnabled = config.MessagePostProcessorRandomizationEnabled
	mpp.config.singleWordProbability = config.MessagePostProcessorSingleWordProbability
	mpp.config.shortSentencesProbability = config.MessagePostProcessorShortSentencesProbability
	mpp.config.longMessagesProbability = config.MessagePostProcessorLongMessagesProbability
	mpp.config.minLength = config.MessagePostProcessorMinLength
	mpp.config.maxLength = config.MessagePostProcessorMaxLength
	mpp.config.longMessageThreshold = config.MessagePostProcessorLongMessageThreshold
	mpp.config.forceLongProcessingThreshold = config.MessagePostProcessorForceLongProcessingThreshold
	mpp.config.timeoutSeconds = config.MessagePostProcessorTimeoutSeconds
	mpp.config.temperature = config.MessagePostProcessorTemperature
	mpp.config.cacheEnabled = config.MessagePostProcessorCacheEnabled
	mpp.config.cacheTTLMinutes = config.MessagePostProcessorCacheTTLMinutes
	mpp.config.excludeTypes = config.MessagePostProcessorExcludeTypes
	mpp.config.weeklySummaryExclude = config.MessagePostProcessorWeeklySummaryExclude
	mpp.config.debugLogging = config.MessagePostProcessorDebugLogging
	mpp.config.logOriginalMessages = config.MessagePostProcessorLogOriginalMessages

	// Настройки кэша замен
	mpp.config.replacementCacheEnabled = config.MessagePostProcessorReplacementCacheEnabled
	mpp.config.replacementCacheTTLMinutes = config.MessagePostProcessorReplacementCacheTTLMinutes

	// Инициализируем статистику
	mpp.stats.ProcessedByType = make(map[ProcessingType]int64)

	// Запускаем очистку кэша
	if mpp.config.cacheEnabled {
		go mpp.startCacheCleanup()
	}

	// Запускаем очистку кэша замен
	if mpp.config.replacementCacheEnabled {
		go mpp.startReplacementCacheCleanup()
	}

	if mpp.config.debugLogging {
		log.Printf("[MessagePostProcessor] Инициализирован. Включен: %v, Рандомизация: %v, Кэш замен: %v",
			mpp.config.enabled, mpp.config.randomizationEnabled, mpp.config.replacementCacheEnabled)
	}

	return mpp
}

// ProcessMessage - основная функция постобработки сообщения
func (mpp *MessagePostProcessor) ProcessMessage(originalText string, messageType MessageType, chatID int64) (string, error) {
	if !mpp.config.enabled {
		mpp.incrementSkipCounter("disabled")
		if mpp.config.debugLogging {
			log.Printf("[MessagePostProcessor] 🔴 ПРОПУСК: Система отключена")
		}
		return originalText, nil
	}

	// Проверяем исключения
	if mpp.isExcludedType(messageType) {
		mpp.incrementSkipCounter("excluded")
		if mpp.config.debugLogging {
			log.Printf("[MessagePostProcessor] 🔴 ПРОПУСК: Исключенный тип '%s'", messageType)
		}
		return originalText, nil
	}

	// Проверяем длину сообщения
	if len(originalText) < mpp.config.minLength {
		mpp.incrementSkipCounter("too_short")
		if mpp.config.debugLogging {
			log.Printf("[MessagePostProcessor] 🔴 ПРОПУСК: Слишком короткое сообщение (%d < %d символов)",
				len(originalText), mpp.config.minLength)
		}
		return originalText, nil
	}

	if mpp.config.maxLength > 0 && len(originalText) > mpp.config.maxLength {
		mpp.incrementSkipCounter("too_long")
		if mpp.config.debugLogging {
			log.Printf("[MessagePostProcessor] 🔴 ПРОПУСК: Слишком длинное сообщение (%d > %d символов)",
				len(originalText), mpp.config.maxLength)
		}
		return originalText, nil
	}

	startTime := time.Now()

	// Периодически логируем статистику
	mpp.logPeriodicStats()

	// НОВОЕ: Замена имен пользователей по приоритету ПЕРЕД основной постобработкой
	textWithReplacedNames, err := mpp.replaceUserNamesInText(originalText, chatID)
	if err != nil {
		log.Printf("[MessagePostProcessor] ⚠️ ПРЕДУПРЕЖДЕНИЕ: Ошибка замены имен пользователей в чате %d: %v", chatID, err)
		// Продолжаем с оригинальным текстом при ошибке
		textWithReplacedNames = originalText
	} else if textWithReplacedNames != originalText {
		if mpp.config.debugLogging {
			log.Printf("[MessagePostProcessor] 🔄 ЗАМЕНА ИМЕН: '%s' -> '%s'",
				mpp.truncateForLog(originalText), mpp.truncateForLog(textWithReplacedNames))
		}
	}

	// Проверяем кэш
	if mpp.config.cacheEnabled {
		if cachedResult, found := mpp.getFromCache(textWithReplacedNames); found {
			mpp.updateStats(ProcessingTypeNone, time.Since(startTime), true)
			if mpp.config.debugLogging {
				log.Printf("[MessagePostProcessor] 💾 КЭШИРОВАН: %s", mpp.truncateForLog(textWithReplacedNames))
			}
			return cachedResult, nil
		}
	}

	// Определяем тип обработки
	processingType := mpp.determineProcessingType(textWithReplacedNames, messageType)

	if processingType == ProcessingTypeNone {
		mpp.incrementSkipCounter("none_type")
		if mpp.config.debugLogging {
			log.Printf("[MessagePostProcessor] 🔴 ПРОПУСК: Тип обработки 'none' для сообщения длиной %d", len(textWithReplacedNames))
		}
		return textWithReplacedNames, nil
	}

	if mpp.config.debugLogging {
		log.Printf("[MessagePostProcessor] 🔄 ОБРАБОТКА: Тип '%s', длина %d", processingType, len(textWithReplacedNames))
		if mpp.config.logOriginalMessages {
			log.Printf("[MessagePostProcessor] 📝 ИСХОДНЫЙ ТЕКСТ: %s", mpp.truncateForLog(textWithReplacedNames))
		}
	}

	// Выполняем постобработку
	processedText, err := mpp.performProcessing(textWithReplacedNames, processingType, chatID)
	if err != nil {
		mpp.incrementErrorCounter(err)
		log.Printf("[MessagePostProcessor] ❌ ОШИБКА ОБРАБОТКИ: %v", err)
		return textWithReplacedNames, nil
	}

	processingTime := time.Since(startTime)

	// Сохраняем в кэш
	if mpp.config.cacheEnabled {
		mpp.saveToCache(textWithReplacedNames, processedText, processingType)
		if mpp.config.debugLogging {
			log.Printf("[MessagePostProcessor] 💾 СОХРАНЕНО В КЭШ: ключ для текста длиной %d", len(textWithReplacedNames))
		}
	}

	// Обновляем статистику
	mpp.updateStats(processingType, processingTime, false)

	// Логируем результат
	if mpp.config.debugLogging {
		log.Printf("[MessagePostProcessor] ✅ ОБРАБОТАНО (%s): %s -> %s (время: %v)",
			processingType, mpp.truncateForLog(textWithReplacedNames), mpp.truncateForLog(processedText), processingTime)
	}

	return processedText, nil
}

// ProcessSummaryWithContext - специализированная функция постобработки саммари с полным контекстом
// Используется когда первичная генерация саммари не удалась, но нужно передать полную историю для постобработки
func (mpp *MessagePostProcessor) ProcessSummaryWithContext(summaryText string, fullHistoryContext string, chatID int64) (string, error) {
	if !mpp.config.enabled {
		log.Printf("[MessagePostProcessor] 🔴 ПРОПУСК: Система отключена для саммари с контекстом")
		return summaryText, nil
	}

	log.Printf("[MessagePostProcessor] 🔄 СПЕЦИАЛЬНАЯ ОБРАБОТКА САММАРИ: Используется полный контекст длиной %d символов", len(fullHistoryContext))

	// Получаем промпт для саммари
	prompt := mpp.bot.config.MessagePostProcessorSummaryPrompt
	if prompt == "" {
		return summaryText, fmt.Errorf("отсутствует промпт для постобработки саммари")
	}

	// Обогащаем промпт личностью
	enrichedPrompt := mpp.bot.enrichPromptWithPersonality(prompt, chatID, string(ProcessingTypeSummary))

	// ИСПРАВЛЕНИЕ: Заменяем {original_message} на ПОЛНУЮ историю, а не на fallback-текст
	finalPrompt := enrichedPrompt
	finalPrompt = strings.ReplaceAll(finalPrompt, "{ORIGINAL_MESSAGE}", fullHistoryContext)
	finalPrompt = strings.ReplaceAll(finalPrompt, "{original_message}", fullHistoryContext)

	// Добавляем обработку других плейсхолдеров если нужно
	if strings.Contains(finalPrompt, "{FULL_CONTEXT}") {
		finalPrompt = strings.ReplaceAll(finalPrompt, "{FULL_CONTEXT}", fullHistoryContext)
	}

	if strings.Contains(finalPrompt, "{DIRECT_CONTEXT}") {
		directContext, err := mpp.getDirectContext(chatID)
		if err != nil {
			log.Printf("[WARN][MessagePostProcessor] Ошибка получения прямого контекста для саммари чата %d: %v", chatID, err)
			directContext = "Нет доступного прямого контекста"
		}
		finalPrompt = strings.ReplaceAll(finalPrompt, "{DIRECT_CONTEXT}", directContext)
	}

	// Вызываем LLM для постобработки саммари
	result, err := mpp.bot.llm.GenerateResponseByType(llm.ResponseTypePostProcessSummary, finalPrompt, "", float32(mpp.config.temperature))
	if err != nil {
		log.Printf("[MessagePostProcessor] ❌ ОШИБКА LLM при постобработке саммари: %v", err)
		return summaryText, fmt.Errorf("ошибка LLM при постобработке саммари: %w", err)
	}

	// Очищаем результат
	processedText := mpp.cleanResult(result)

	if strings.TrimSpace(processedText) == "" {
		log.Printf("[MessagePostProcessor] ⚠️ LLM вернул пустой результат для постобработки саммари")
		return summaryText, fmt.Errorf("LLM вернул пустой результат для постобработки саммари")
	}

	log.Printf("[MessagePostProcessor] ✅ САММАРИ УСПЕШНО ПОСТОБРАБОТАНО: исходная длина %d -> итоговая длина %d", len(summaryText), len(processedText))
	return processedText, nil
}

// determineProcessingType определяет тип постобработки
func (mpp *MessagePostProcessor) determineProcessingType(text string, messageType MessageType) ProcessingType {
	if messageType == MessageTypeSummary {
		if mpp.config.debugLogging {
			log.Printf("[MessagePostProcessor] 🎯 ТИП: Summary (по типу сообщения)")
		}
		return ProcessingTypeSummary
	}

	if len(text) >= mpp.config.forceLongProcessingThreshold {
		if mpp.config.debugLogging {
			log.Printf("[MessagePostProcessor] 🎯 ТИП: Long messages (принудительно, длина %d >= %d)",
				len(text), mpp.config.forceLongProcessingThreshold)
		}
		return ProcessingTypeLongMessages
	}

	if !mpp.config.randomizationEnabled {
		if mpp.config.debugLogging {
			log.Printf("[MessagePostProcessor] 🎯 ТИП: Intelligent (рандомизация отключена)")
		}
		return ProcessingTypeIntelligent
	}

	probability := rand.Float64()

	if probability < mpp.config.singleWordProbability {
		if mpp.config.debugLogging {
			log.Printf("[MessagePostProcessor] 🎯 ТИП: Single word (вероятность %.3f < %.3f)",
				probability, mpp.config.singleWordProbability)
		}
		return ProcessingTypeSingleWord
	}

	if probability < mpp.config.singleWordProbability+mpp.config.shortSentencesProbability {
		if mpp.config.debugLogging {
			log.Printf("[MessagePostProcessor] 🎯 ТИП: Short sentences (вероятность %.3f < %.3f)",
				probability, mpp.config.singleWordProbability+mpp.config.shortSentencesProbability)
		}
		return ProcessingTypeShortSentences
	}

	if probability < mpp.config.singleWordProbability+mpp.config.shortSentencesProbability+mpp.config.longMessagesProbability {
		if len(text) >= mpp.config.longMessageThreshold {
			if mpp.config.debugLogging {
				log.Printf("[MessagePostProcessor] 🎯 ТИП: Long messages (вероятность %.3f, длина %d >= %d)",
					probability, len(text), mpp.config.longMessageThreshold)
			}
			return ProcessingTypeLongMessages
		}
		if mpp.config.debugLogging {
			log.Printf("[MessagePostProcessor] 🎯 ТИП: Short sentences (fallback, длина %d < %d)",
				len(text), mpp.config.longMessageThreshold)
		}
		return ProcessingTypeShortSentences
	}

	if mpp.config.debugLogging {
		log.Printf("[MessagePostProcessor] 🎯 ТИП: None (вероятность %.3f превышает все пороги)", probability)
	}
	return ProcessingTypeNone
}

// performProcessing выполняет постобработку согласно типу
func (mpp *MessagePostProcessor) performProcessing(originalText string, processingType ProcessingType, chatID int64) (string, error) {
	prompt := mpp.getPromptForProcessingType(processingType)
	if prompt == "" {
		return originalText, fmt.Errorf("отсутствует промпт для типа: %s", processingType)
	}

	// Обогащаем промпт личностью
	enrichedPrompt := mpp.bot.enrichPromptWithPersonality(prompt, chatID, string(processingType))

	// Заменяем плейсхолдеры
	finalPrompt := enrichedPrompt

	// Исправляем регистр плейсхолдера
	finalPrompt = strings.ReplaceAll(finalPrompt, "{ORIGINAL_MESSAGE}", originalText)
	finalPrompt = strings.ReplaceAll(finalPrompt, "{original_message}", originalText)

	// Добавляем обработку {FULL_CONTEXT} - контекст сообщений в соответствии с CONTEXT_WINDOW
	if strings.Contains(finalPrompt, "{FULL_CONTEXT}") {
		fullContext, err := mpp.getFullContext(chatID)
		if err != nil {
			log.Printf("[WARN][MessagePostProcessor] Ошибка получения полного контекста для чата %d: %v", chatID, err)
			fullContext = "Нет доступного контекста"
		}
		finalPrompt = strings.ReplaceAll(finalPrompt, "{FULL_CONTEXT}", fullContext)
	}

	// Добавляем обработку {DIRECT_CONTEXT} - цепочка сообщений, на которую отвечает бот
	if strings.Contains(finalPrompt, "{DIRECT_CONTEXT}") {
		directContext, err := mpp.getDirectContext(chatID)
		if err != nil {
			log.Printf("[WARN][MessagePostProcessor] Ошибка получения прямого контекста для чата %d: %v", chatID, err)
			directContext = "Нет доступного прямого контекста"
		}
		finalPrompt = strings.ReplaceAll(finalPrompt, "{DIRECT_CONTEXT}", directContext)
	}

	// Вызываем LLM
	// Определяем тип ответа на основе типа обработки
	var responseType llm.ResponseType
	switch processingType {
	case ProcessingTypeSingleWord:
		responseType = llm.ResponseTypePostProcessSingleWord
	case ProcessingTypeShortSentences:
		responseType = llm.ResponseTypePostProcessShort
	case ProcessingTypeLongMessages:
		responseType = llm.ResponseTypePostProcessLong
	case ProcessingTypeIntelligent:
		responseType = llm.ResponseTypePostProcessIntelligent
	case ProcessingTypeSummary:
		responseType = llm.ResponseTypePostProcessSummary
	default:
		responseType = llm.ResponseTypePostProcessIntelligent
	}

	result, err := mpp.bot.llm.GenerateResponseByType(responseType, finalPrompt, "", float32(mpp.config.temperature))
	if err != nil {
		return originalText, fmt.Errorf("ошибка LLM: %w", err)
	}

	// Очищаем результат
	processedText := mpp.cleanResult(result)

	if strings.TrimSpace(processedText) == "" {
		return originalText, fmt.Errorf("LLM вернул пустой результат")
	}

	return processedText, nil
}

// getPromptForProcessingType возвращает промпт для конкретного типа обработки
func (mpp *MessagePostProcessor) getPromptForProcessingType(processingType ProcessingType) string {
	switch processingType {
	case ProcessingTypeSingleWord:
		return mpp.bot.config.MessagePostProcessorSingleWordPrompt
	case ProcessingTypeShortSentences:
		return mpp.bot.config.MessagePostProcessorShortSentencesPrompt
	case ProcessingTypeLongMessages:
		return mpp.bot.config.MessagePostProcessorLongMessagesPrompt
	case ProcessingTypeIntelligent:
		return mpp.bot.config.MessagePostProcessorIntelligentPrompt
	case ProcessingTypeSummary:
		return mpp.bot.config.MessagePostProcessorSummaryPrompt
	default:
		return ""
	}
}

// cleanResult очищает результат LLM от лишних символов
func (mpp *MessagePostProcessor) cleanResult(result string) string {
	result = strings.TrimSpace(result)

	if strings.HasPrefix(result, "```") {
		lines := strings.Split(result, "\n")
		if len(lines) > 2 && strings.HasSuffix(lines[len(lines)-1], "```") {
			result = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	result = strings.Trim(result, "`")

	if (strings.HasPrefix(result, "\"") && strings.HasSuffix(result, "\"")) ||
		(strings.HasPrefix(result, "'") && strings.HasSuffix(result, "'")) {
		result = result[1 : len(result)-1]
	}

	return strings.TrimSpace(result)
}

// getFullContext получает полный контекст сообщений в соответствии с CONTEXT_WINDOW
func (mpp *MessagePostProcessor) getFullContext(chatID int64) (string, error) {
	// Получаем последние сообщения в соответствии с CONTEXT_WINDOW
	messages, err := mpp.bot.storage.GetMessages(chatID, mpp.bot.config.ContextWindow)
	if err != nil {
		return "", fmt.Errorf("ошибка получения сообщений: %w", err)
	}

	if len(messages) == 0 {
		return "Нет доступной истории сообщений", nil
	}

	// Ограничиваем количество сообщений до CONTEXT_WINDOW
	if len(messages) > mpp.bot.config.ContextWindow {
		messages = messages[len(messages)-mpp.bot.config.ContextWindow:]
	}

	// Используем новый унифицированный форматтер
	formatter := NewUnifiedMessageFormatter(mpp.bot.storage, mpp.bot.config.TimeZone)
	formattedHistory := formatter.FormatMessages(chatID, messages)

	log.Printf("[PostProcessor] Chat %d: Использован унифицированный форматтер для %d сообщений", chatID, len(messages))
	return formattedHistory, nil
}

// getDirectContext получает цепочку сообщений, на которую отвечает бот
func (mpp *MessagePostProcessor) getDirectContext(chatID int64) (string, error) {
	// Получаем последние сообщения для поиска цепочки ответов
	messages, err := mpp.bot.storage.GetMessages(chatID, 20) // Берем больше сообщений для поиска цепочки
	if err != nil {
		return "", fmt.Errorf("ошибка получения сообщений: %w", err)
	}

	if len(messages) == 0 {
		return "Нет доступной истории сообщений", nil
	}

	// Ищем последнее сообщение с ReplyToMessage
	var replyChain []*tgbotapi.Message
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.ReplyToMessage != nil {
			// Получаем цепочку ответов для этого сообщения
			chain, err := mpp.bot.storage.GetReplyChain(context.Background(), chatID, msg.ReplyToMessage.MessageID, 10)
			if err != nil {
				log.Printf("[WARN][MessagePostProcessor] Ошибка получения цепочки ответов: %v", err)
				continue
			}
			if len(chain) > 0 {
				replyChain = chain
				break
			}
		}
	}

	if len(replyChain) == 0 {
		return "Нет доступной цепочки ответов", nil
	}

	// Форматируем цепочку ответов
	var sb strings.Builder
	sb.WriteString("=== ЦЕПОЧКА ОТВЕТОВ ===\n")

	for i, msg := range replyChain {
		if msg.From != nil {
			author := mpp.getUserDisplayName(chatID, msg.From.ID, msg)
			sb.WriteString(fmt.Sprintf("[%d] %s: %s\n", i+1, author, msg.Text))
		}
	}

	return sb.String(), nil
}

// getUserDisplayName получает отображаемое имя пользователя
func (mpp *MessagePostProcessor) getUserDisplayName(chatID int64, userID int64, msg *tgbotapi.Message) string {
	// Пробуем получить профиль пользователя
	profile, err := mpp.bot.storage.GetUserProfile(chatID, userID)
	if err == nil && profile != nil && profile.Alias != "" {
		return profile.Alias
	}

	// Fallback к данным из сообщения
	if msg.From.FirstName != "" {
		return msg.From.FirstName
	} else if msg.From.UserName != "" {
		return msg.From.UserName
	} else {
		return fmt.Sprintf("User_%d", userID)
	}
}

// isExcludedType проверяет, исключен ли тип сообщения из обработки
func (mpp *MessagePostProcessor) isExcludedType(messageType MessageType) bool {
	// Специальная проверка для еженедельного саммари
	if messageType == MessageTypeWeeklySummary && mpp.config.weeklySummaryExclude {
		return true
	}

	messageTypeStr := string(messageType)
	for _, excludedType := range mpp.config.excludeTypes {
		if strings.EqualFold(excludedType, messageTypeStr) {
			return true
		}
	}
	return false
}

// Кэширование

// generateCacheKey генерирует ключ для кэша
func (mpp *MessagePostProcessor) generateCacheKey(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}

// getFromCache получает результат из кэша
func (mpp *MessagePostProcessor) getFromCache(text string) (string, bool) {
	mpp.mutex.RLock()
	defer mpp.mutex.RUnlock()

	key := mpp.generateCacheKey(text)
	entry, exists := mpp.cache[key]

	if !exists {
		if mpp.config.debugLogging {
			log.Printf("[MessagePostProcessor] 💾 КЭШMISS: Ключ не найден для текста длиной %d", len(text))
		}
		return "", false
	}

	if time.Since(entry.Timestamp) > time.Duration(mpp.config.cacheTTLMinutes)*time.Minute {
		mpp.stats.CacheExpiredEntries++
		if mpp.config.debugLogging {
			log.Printf("[MessagePostProcessor] 💾 КЭШEXPIRED: Запись устарела (возраст: %v)", time.Since(entry.Timestamp))
		}
		return "", false
	}

	if mpp.config.debugLogging {
		log.Printf("[MessagePostProcessor] 💾 КЭШHIT: Найдена запись типа '%s' (возраст: %v)",
			entry.ProcessingType, time.Since(entry.Timestamp))
	}

	return entry.ProcessedText, true
}

// saveToCache сохраняет результат в кэш
func (mpp *MessagePostProcessor) saveToCache(originalText, processedText string, processingType ProcessingType) {
	mpp.mutex.Lock()
	defer mpp.mutex.Unlock()

	key := mpp.generateCacheKey(originalText)
	mpp.cache[key] = MessageCacheEntry{
		ProcessedText:  processedText,
		Timestamp:      time.Now(),
		ProcessingType: processingType,
	}
}

// startCacheCleanup запускает фоновую очистку кэша сообщений
func (mpp *MessagePostProcessor) startCacheCleanup() {
	ticker := time.NewTicker(time.Duration(mpp.config.cacheTTLMinutes) * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		mpp.cleanupCache()
	}
}

// cleanupCache удаляет устаревшие записи из кэша сообщений
func (mpp *MessagePostProcessor) cleanupCache() {
	mpp.mutex.Lock()
	defer mpp.mutex.Unlock()

	now := time.Now()
	ttl := time.Duration(mpp.config.cacheTTLMinutes) * time.Minute
	expiredCount := 0

	for key, entry := range mpp.cache {
		if now.Sub(entry.Timestamp) > ttl {
			delete(mpp.cache, key)
			expiredCount++
		}
	}

	mpp.stats.CacheExpiredEntries += int64(expiredCount)

	if mpp.config.debugLogging && expiredCount > 0 {
		log.Printf("[MessagePostProcessor] 🧹 ОЧИСТКА КЭША: Удалено %d устаревших записей, осталось %d", expiredCount, len(mpp.cache))
	}
}

// startReplacementCacheCleanup запускает фоновую очистку кэша замен
func (mpp *MessagePostProcessor) startReplacementCacheCleanup() {
	ticker := time.NewTicker(time.Duration(mpp.config.replacementCacheTTLMinutes) * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		mpp.cleanupReplacementCache()
	}
}

// cleanupReplacementCache удаляет устаревшие записи из кэша замен
func (mpp *MessagePostProcessor) cleanupReplacementCache() {
	mpp.replacementMutex.Lock()
	defer mpp.replacementMutex.Unlock()

	now := time.Now()
	ttl := time.Duration(mpp.config.replacementCacheTTLMinutes) * time.Minute
	expiredCount := 0

	for chatID, cache := range mpp.replacementCache {
		if now.Sub(cache.Timestamp) > ttl {
			delete(mpp.replacementCache, chatID)
			expiredCount++
		}
	}

	mpp.stats.ReplacementCacheCleanups += int64(expiredCount)

	if mpp.config.debugLogging && expiredCount > 0 {
		log.Printf("[MessagePostProcessor] 🧹 ОЧИСТКА КЭША ЗАМЕН: Удалено %d устаревших записей, осталось %d", expiredCount, len(mpp.replacementCache))
	}
}

// Статистика и управление

// updateStats обновляет статистику
func (mpp *MessagePostProcessor) updateStats(processingType ProcessingType, processingTime time.Duration, fromCache bool) {
	mpp.mutex.Lock()
	defer mpp.mutex.Unlock()

	if fromCache {
		mpp.stats.CacheHits++
	} else {
		mpp.stats.CacheMisses++
		mpp.stats.TotalProcessed++
		mpp.stats.ProcessedByType[processingType]++
		mpp.stats.ProcessingTimeTotal += processingTime

		if mpp.stats.TotalProcessed > 0 {
			mpp.stats.AverageProcessingTime = mpp.stats.ProcessingTimeTotal / time.Duration(mpp.stats.TotalProcessed)
		}
	}
}

// GetStats возвращает подробную статистику работы постобработчика
func (mpp *MessagePostProcessor) GetStats() map[string]interface{} {
	mpp.mutex.RLock()
	defer mpp.mutex.RUnlock()

	// Основная статистика
	totalProcessed := mpp.stats.TotalProcessed
	processedByType := make(map[string]int64)
	for k, v := range mpp.stats.ProcessedByType {
		processedByType[string(k)] = v
	}

	// Статистика кэша сообщений
	cacheHits := mpp.stats.CacheHits
	cacheMisses := mpp.stats.CacheMisses
	cacheHitRate := float64(0)
	if cacheHits+cacheMisses > 0 {
		cacheHitRate = float64(cacheHits) / float64(cacheHits+cacheMisses) * 100
	}

	// Статистика кэша замен
	replacementCacheHits := mpp.stats.ReplacementCacheHits
	replacementCacheMisses := mpp.stats.ReplacementCacheMisses
	replacementCacheBuilds := mpp.stats.ReplacementCacheBuilds
	replacementCacheCleanups := mpp.stats.ReplacementCacheCleanups
	replacementCacheHitRate := float64(0)
	if replacementCacheHits+replacementCacheMisses > 0 {
		replacementCacheHitRate = float64(replacementCacheHits) / float64(replacementCacheHits+replacementCacheMisses) * 100
	}

	// Подсчет текущих размеров кэшей
	mpp.replacementMutex.RLock()
	currentReplacementCacheSize := len(mpp.replacementCache)
	totalReplacements := 0
	for _, cache := range mpp.replacementCache {
		totalReplacements += len(cache.Replacements)
	}
	mpp.replacementMutex.RUnlock()

	// Статистика времени
	avgProcessingTime := mpp.stats.AverageProcessingTime
	totalProcessingTime := mpp.stats.ProcessingTimeTotal

	// Статистика пропусков
	skippedByDisabled := mpp.stats.SkippedByDisabled
	skippedByExcluded := mpp.stats.SkippedByExcluded
	skippedByTooShort := mpp.stats.SkippedByTooShort
	skippedByTooLong := mpp.stats.SkippedByTooLong
	skippedByNoneType := mpp.stats.SkippedByNoneType
	totalSkipped := skippedByDisabled + skippedByExcluded + skippedByTooShort + skippedByTooLong + skippedByNoneType

	// Статистика ошибок
	llmErrors := mpp.stats.LLMErrors
	emptyLLMResponses := mpp.stats.EmptyLLMResponses
	totalErrors := llmErrors + emptyLLMResponses

	// Процент обработки
	totalRequests := totalProcessed + totalSkipped
	processedRate := float64(0)
	if totalRequests > 0 {
		processedRate = float64(totalProcessed) / float64(totalRequests) * 100
	}

	return map[string]interface{}{
		"enabled":       mpp.config.enabled,
		"randomization": mpp.config.randomizationEnabled,

		// Основная статистика
		"total_processed":   totalProcessed,
		"total_skipped":     totalSkipped,
		"total_requests":    totalRequests,
		"processed_rate":    fmt.Sprintf("%.1f%%", processedRate),
		"processed_by_type": processedByType,

		// Статистика времени
		"avg_processing_time":   fmt.Sprintf("%.2fms", float64(avgProcessingTime.Nanoseconds())/1e6),
		"total_processing_time": fmt.Sprintf("%.2fs", totalProcessingTime.Seconds()),

		// Статистика кэша сообщений
		"cache": map[string]interface{}{
			"enabled":         mpp.config.cacheEnabled,
			"size":            len(mpp.cache),
			"hits":            cacheHits,
			"misses":          cacheMisses,
			"hit_rate":        fmt.Sprintf("%.1f%%", cacheHitRate),
			"expired_entries": mpp.stats.CacheExpiredEntries,
			"ttl_minutes":     mpp.config.cacheTTLMinutes,
		},

		// Статистика кэша замен
		"replacement_cache": map[string]interface{}{
			"enabled":            mpp.config.replacementCacheEnabled,
			"size":               currentReplacementCacheSize,
			"total_replacements": totalReplacements,
			"hits":               replacementCacheHits,
			"misses":             replacementCacheMisses,
			"builds":             replacementCacheBuilds,
			"cleanups":           replacementCacheCleanups,
			"hit_rate":           fmt.Sprintf("%.1f%%", replacementCacheHitRate),
			"ttl_minutes":        mpp.config.replacementCacheTTLMinutes,
		},

		// Статистика пропусков
		"skipped": map[string]interface{}{
			"total":        totalSkipped,
			"by_disabled":  skippedByDisabled,
			"by_excluded":  skippedByExcluded,
			"by_too_short": skippedByTooShort,
			"by_too_long":  skippedByTooLong,
			"by_none_type": skippedByNoneType,
		},

		// Статистика ошибок
		"errors": map[string]interface{}{
			"total":               totalErrors,
			"llm_errors":          llmErrors,
			"empty_llm_responses": emptyLLMResponses,
		},

		// Конфигурация
		"config": map[string]interface{}{
			"min_length":                      mpp.config.minLength,
			"max_length":                      mpp.config.maxLength,
			"long_message_threshold":          mpp.config.longMessageThreshold,
			"force_long_processing_threshold": mpp.config.forceLongProcessingThreshold,
			"timeout_seconds":                 mpp.config.timeoutSeconds,
			"temperature":                     mpp.config.temperature,
			"exclude_types":                   mpp.config.excludeTypes,
			"debug_logging":                   mpp.config.debugLogging,
		},
	}
}

// ToggleEnabled переключает включение/выключение системы
func (mpp *MessagePostProcessor) ToggleEnabled() bool {
	mpp.mutex.Lock()
	defer mpp.mutex.Unlock()

	mpp.config.enabled = !mpp.config.enabled

	if mpp.config.debugLogging {
		log.Printf("[MessagePostProcessor] Переключен статус: %v", mpp.config.enabled)
	}

	return mpp.config.enabled
}

// IsEnabled возвращает текущий статус включения
func (mpp *MessagePostProcessor) IsEnabled() bool {
	mpp.mutex.RLock()
	defer mpp.mutex.RUnlock()
	return mpp.config.enabled
}

// ClearCache очищает весь кэш
func (mpp *MessagePostProcessor) ClearCache() int {
	mpp.mutex.Lock()
	defer mpp.mutex.Unlock()

	cacheSize := len(mpp.cache)
	mpp.cache = make(map[string]MessageCacheEntry)

	if mpp.config.debugLogging {
		log.Printf("[MessagePostProcessor] Кэш очищен. Удалено записей: %d", cacheSize)
	}

	return cacheSize
}

// SetRandomizationEnabled включает/выключает рандомизацию
func (mpp *MessagePostProcessor) SetRandomizationEnabled(enabled bool) bool {
	mpp.mutex.Lock()
	defer mpp.mutex.Unlock()

	mpp.config.randomizationEnabled = enabled

	if mpp.config.debugLogging {
		log.Printf("[MessagePostProcessor] Рандомизация: %v", enabled)
	}

	return enabled
}

// IsRandomizationEnabled возвращает текущий статус рандомизации
func (mpp *MessagePostProcessor) IsRandomizationEnabled() bool {
	mpp.mutex.RLock()
	defer mpp.mutex.RUnlock()
	return mpp.config.randomizationEnabled
}

// ClearStats очищает статистику
func (mpp *MessagePostProcessor) ClearStats() {
	mpp.mutex.Lock()
	defer mpp.mutex.Unlock()

	mpp.stats.TotalProcessed = 0
	mpp.stats.CacheHits = 0
	mpp.stats.CacheMisses = 0
	mpp.stats.ProcessingTimeTotal = 0
	mpp.stats.AverageProcessingTime = 0
	mpp.stats.ProcessedByType = make(map[ProcessingType]int64)

	// Очищаем расширенную статистику
	mpp.stats.SkippedByDisabled = 0
	mpp.stats.SkippedByExcluded = 0
	mpp.stats.SkippedByTooShort = 0
	mpp.stats.SkippedByTooLong = 0
	mpp.stats.SkippedByNoneType = 0
	mpp.stats.LLMErrors = 0
	mpp.stats.EmptyLLMResponses = 0
	mpp.stats.CacheExpiredEntries = 0
	mpp.stats.LastStatsLogTime = time.Time{}

	if mpp.config.debugLogging {
		log.Printf("[MessagePostProcessor] 🧹 СТАТИСТИКА ОЧИЩЕНА")
	}
}

// truncateForLog сокращает текст для логирования
func (mpp *MessagePostProcessor) truncateForLog(text string) string {
	const maxLen = 50
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

// incrementSkipCounter увеличивает счетчик пропущенных сообщений
func (mpp *MessagePostProcessor) incrementSkipCounter(reason string) {
	mpp.mutex.Lock()
	defer mpp.mutex.Unlock()

	switch reason {
	case "disabled":
		mpp.stats.SkippedByDisabled++
	case "excluded":
		mpp.stats.SkippedByExcluded++
	case "too_short":
		mpp.stats.SkippedByTooShort++
	case "too_long":
		mpp.stats.SkippedByTooLong++
	case "none_type":
		mpp.stats.SkippedByNoneType++
	}
}

// incrementErrorCounter увеличивает счетчик ошибок
func (mpp *MessagePostProcessor) incrementErrorCounter(err error) {
	mpp.mutex.Lock()
	defer mpp.mutex.Unlock()

	if err != nil {
		if strings.Contains(err.Error(), "LLM вернул пустой результат") {
			mpp.stats.EmptyLLMResponses++
		} else {
			mpp.stats.LLMErrors++
		}
	}
}

// logPeriodicStats периодически логирует статистику постобработки для мониторинга
func (mpp *MessagePostProcessor) logPeriodicStats() {
	now := time.Now()
	if now.Sub(mpp.stats.LastStatsLogTime) >= 30*time.Minute { // Логируем раз в 30 минут
		mpp.mutex.RLock()
		total := mpp.stats.TotalProcessed
		cacheHits := mpp.stats.CacheHits
		cacheMisses := mpp.stats.CacheMisses
		avgTime := mpp.stats.AverageProcessingTime
		replacementCacheHits := mpp.stats.ReplacementCacheHits
		replacementCacheMisses := mpp.stats.ReplacementCacheMisses
		mpp.mutex.RUnlock()

		hitRate := float64(0)
		if cacheHits+cacheMisses > 0 {
			hitRate = float64(cacheHits) / float64(cacheHits+cacheMisses) * 100
		}

		replacementHitRate := float64(0)
		if replacementCacheHits+replacementCacheMisses > 0 {
			replacementHitRate = float64(replacementCacheHits) / float64(replacementCacheHits+replacementCacheMisses) * 100
		}

		log.Printf("[MessagePostProcessor] 📊 СТАТИСТИКА: Обработано: %d, Кэш: %.1f%% попаданий, Замены: %.1f%% попаданий, Среднее время: %v",
			total, hitRate, replacementHitRate, avgTime)

		mpp.mutex.Lock()
		mpp.stats.LastStatsLogTime = now
		mpp.mutex.Unlock()
	}
}

// buildReplacementCache строит кэш замен для указанного чата
func (mpp *MessagePostProcessor) buildReplacementCache(chatID int64) (*ChatReplacementCache, error) {
	if mpp.bot == nil {
		return nil, fmt.Errorf("bot instance не инициализирован")
	}

	// Получаем все профили пользователей для чата
	profiles, err := mpp.bot.storage.GetAllUserProfiles(chatID)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения профилей пользователей: %w", err)
	}

	var replacements []ReplacementEntry
	userCount := len(profiles)

	for _, profile := range profiles {
		if profile == nil {
			continue
		}

		preferredName := mpp.getUserPreferredName(profile)
		if preferredName == "" {
			continue
		}

		// Собираем все возможные варианты имен, которые нужно заменить
		namesToReplace := mpp.getAllNamesToReplace(profile, preferredName)

		for _, nameToReplace := range namesToReplace {
			if nameToReplace != preferredName && nameToReplace != "" {
				replacements = append(replacements, ReplacementEntry{
					From: nameToReplace,
					To:   preferredName,
				})
			}
		}
	}

	// Сортируем по убыванию длины для корректной замены
	for i := 0; i < len(replacements)-1; i++ {
		for j := i + 1; j < len(replacements); j++ {
			if len(replacements[i].From) < len(replacements[j].From) {
				replacements[i], replacements[j] = replacements[j], replacements[i]
			}
		}
	}

	cache := &ChatReplacementCache{
		ChatID:       chatID,
		Replacements: replacements,
		Timestamp:    time.Now(),
		UserCount:    userCount,
	}

	if mpp.config.debugLogging {
		log.Printf("[MessagePostProcessor] 🏗️ ПОСТРОЕН КЭША ЗАМЕН: Чат %d, %d замен, %d пользователей",
			chatID, len(replacements), userCount)
	}

	return cache, nil
}

// getReplacementCache получает кэш замен для чата или создает новый если необходимо
func (mpp *MessagePostProcessor) getReplacementCache(chatID int64) (*ChatReplacementCache, error) {
	if !mpp.config.replacementCacheEnabled {
		// Если кэш отключен, всегда строим новый
		return mpp.buildReplacementCache(chatID)
	}

	mpp.replacementMutex.RLock()
	cache, exists := mpp.replacementCache[chatID]
	mpp.replacementMutex.RUnlock()

	if exists && mpp.isReplacementCacheValid(cache) {
		mpp.stats.ReplacementCacheHits++
		if mpp.config.debugLogging {
			log.Printf("[MessagePostProcessor] 💾 КЭШ ЗАМЕН НАЙДЕН: Чат %d, %d замен", chatID, len(cache.Replacements))
		}
		return cache, nil
	}

	// Кэш не найден или устарел, строим новый
	mpp.stats.ReplacementCacheMisses++
	cache, err := mpp.buildReplacementCache(chatID)
	if err != nil {
		return nil, err
	}

	// Сохраняем в кэш
	mpp.replacementMutex.Lock()
	mpp.replacementCache[chatID] = cache
	mpp.stats.ReplacementCacheBuilds++
	mpp.replacementMutex.Unlock()

	return cache, nil
}

// isReplacementCacheValid проверяет, актуален ли кэш замен
func (mpp *MessagePostProcessor) isReplacementCacheValid(cache *ChatReplacementCache) bool {
	if cache == nil {
		return false
	}

	now := time.Now()
	ttl := time.Duration(mpp.config.replacementCacheTTLMinutes) * time.Minute

	return now.Sub(cache.Timestamp) <= ttl
}

// replaceUserNamesInText заменяет имена пользователей в тексте согласно приоритету: Alias > RealName > Username (без @)
func (mpp *MessagePostProcessor) replaceUserNamesInText(text string, chatID int64) (string, error) {
	// Получаем кэш замен для чата
	cache, err := mpp.getReplacementCache(chatID)
	if err != nil {
		return text, fmt.Errorf("ошибка получения кэша замен: %w", err)
	}

	if len(cache.Replacements) == 0 {
		return text, nil // Нет замен для выполнения
	}

	result := text

	// Выполняем замены с учетом границ слов и регистронезависимо
	for _, repl := range cache.Replacements {
		// Создаем регулярное выражение для замены с границами слов и флагом case-insensitive
		// Это предотвращает замену частей слов и игнорирует регистр
		pattern := fmt.Sprintf(`(?i)\b%s\b`, regexp.QuoteMeta(repl.From))
		re, err := regexp.Compile(pattern)
		if err != nil {
			log.Printf("[MessagePostProcessor] WARN: Ошибка компиляции регекса для '%s': %v", repl.From, err)
			continue
		}

		// Проверяем, есть ли совпадения перед заменой
		if re.MatchString(result) {
			oldResult := result
			result = re.ReplaceAllString(result, repl.To)
			if mpp.config.debugLogging {
				log.Printf("[MessagePostProcessor] 🔄 ЗАМЕНА: '%s' -> '%s' в тексте", repl.From, repl.To)
				log.Printf("[MessagePostProcessor] 📝 ДО: %s", mpp.truncateForLog(oldResult))
				log.Printf("[MessagePostProcessor] 📝 ПОСЛЕ: %s", mpp.truncateForLog(result))
			}
		}
	}

	return result, nil
}

// getUserPreferredName возвращает предпочтительное имя пользователя по приоритету: Alias > RealName > Username (без @)
func (mpp *MessagePostProcessor) getUserPreferredName(profile *storage.UserProfile) string {
	if profile == nil {
		return ""
	}

	// Приоритет 1: Alias
	if profile.Alias != "" {
		return profile.Alias
	}

	// Приоритет 2: RealName
	if profile.RealName != "" {
		return profile.RealName
	}

	// Приоритет 3: Username (без @)
	if profile.Username != "" {
		username := profile.Username
		// Убираем @ в начале, если есть
		username = strings.TrimPrefix(username, "@")
		return username
	}

	// Fallback: используем UserID
	return fmt.Sprintf("User_%d", profile.UserID)
}

// getAllNamesToReplace возвращает все возможные имена, которые нужно заменить на предпочтительное
func (mpp *MessagePostProcessor) getAllNamesToReplace(profile *storage.UserProfile, preferredName string) []string {
	var names []string

	if profile == nil {
		return names
	}

	// Вспомогательная функция для добавления всех вариантов написания
	addNameVariants := func(name string) {
		if name == "" || name == preferredName {
			return
		}

		variants := mpp.generateNameVariants(name)
		for _, variant := range variants {
			if variant != preferredName {
				names = append(names, variant)
			}
		}
	}

	// Добавляем все варианты Alias
	addNameVariants(profile.Alias)

	// Добавляем все варианты RealName
	addNameVariants(profile.RealName)

	// Добавляем все варианты Username
	if profile.Username != "" {
		username := profile.Username

		// Убираем @ в начале для получения чистого username
		if strings.HasPrefix(username, "@") {
			cleanUsername := username[1:]
			addNameVariants(cleanUsername)
			addNameVariants(username) // Также добавляем версию с @
		} else {
			addNameVariants(username)
			addNameVariants("@" + username) // Добавляем версию с @
		}
	}

	return names
}

// generateNameVariants генерирует различные варианты написания имени
func (mpp *MessagePostProcessor) generateNameVariants(name string) []string {
	if name == "" {
		return nil
	}

	variants := make(map[string]bool) // Используем map для избежания дубликатов

	// Добавляем оригинал
	variants[name] = true

	// Генерируем варианты регистра
	variants[strings.ToLower(name)] = true
	variants[strings.ToUpper(name)] = true
	variants[titleCase(name)] = true

	// Если есть подчеркивания, создаем варианты без них и с заглавными буквами
	if strings.Contains(name, "_") {
		// Убираем подчеркивания
		noUnderscore := strings.ReplaceAll(name, "_", "")
		variants[noUnderscore] = true
		variants[strings.ToLower(noUnderscore)] = true
		variants[strings.ToUpper(noUnderscore)] = true
		variants[titleCase(noUnderscore)] = true

		// Создаем CamelCase версию (каждая часть с заглавной буквы)
		parts := strings.Split(name, "_")
		var camelCase string
		for _, part := range parts {
			if part != "" {
				camelCase += titleCase(strings.ToLower(part))
			}
		}
		if camelCase != "" {
			variants[camelCase] = true
			variants[strings.ToLower(camelCase)] = true
		}
	}

	// Если нет подчеркиваний, но есть CamelCase, добавляем версию с подчеркиваниями
	if !strings.Contains(name, "_") && hasUpperCaseInMiddle(name) {
		underscoreVersion := addUnderscoreBeforeUpper(name)
		variants[underscoreVersion] = true
		variants[strings.ToLower(underscoreVersion)] = true
	}

	// Убираем @ если есть
	nameWithoutAt := strings.TrimPrefix(name, "@")
	if nameWithoutAt != name {
		// Рекурсивно генерируем варианты для имени без @
		subVariants := mpp.generateNameVariants(nameWithoutAt)
		for _, variant := range subVariants {
			variants[variant] = true
		}
	}

	// Конвертируем map в slice
	result := make([]string, 0, len(variants))
	for variant := range variants {
		result = append(result, variant)
	}

	return result
}

// hasUpperCaseInMiddle проверяет, есть ли заглавные буквы в середине строки (признак CamelCase)
func hasUpperCaseInMiddle(s string) bool {
	for i := 1; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			return true
		}
	}
	return false
}

// addUnderscoreBeforeUpper добавляет подчеркивания перед заглавными буквами
func addUnderscoreBeforeUpper(s string) string {
	if len(s) <= 1 {
		return s
	}

	var result strings.Builder
	result.WriteByte(s[0])

	for i := 1; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			result.WriteByte('_')
		}
		result.WriteByte(s[i])
	}

	return result.String()
}

// ClearReplacementCache очищает кэш замен для всех чатов или конкретного чата
func (mpp *MessagePostProcessor) ClearReplacementCache(chatID ...int64) int {
	mpp.replacementMutex.Lock()
	defer mpp.replacementMutex.Unlock()

	cleared := 0
	if len(chatID) == 0 {
		// Очищаем весь кэш
		cleared = len(mpp.replacementCache)
		mpp.replacementCache = make(map[int64]*ChatReplacementCache)
		if mpp.config.debugLogging {
			log.Printf("[MessagePostProcessor] 🧹 ОЧИЩЕН ВЕСЬ КЭШ ЗАМЕН: %d записей", cleared)
		}
	} else {
		// Очищаем кэш для конкретных чатов
		for _, id := range chatID {
			if _, exists := mpp.replacementCache[id]; exists {
				delete(mpp.replacementCache, id)
				cleared++
				if mpp.config.debugLogging {
					log.Printf("[MessagePostProcessor] 🧹 ОЧИЩЕН КЭШ ЗАМЕН для чата: %d", id)
				}
			}
		}
	}

	return cleared
}
