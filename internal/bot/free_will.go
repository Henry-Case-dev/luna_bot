package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/storage"
	"github.com/Henry-Case-dev/luna_bot/internal/utils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// FreeWillDecision представляет решение ИИ о необходимости действия
// FreeWillShouldReplyDecision - результат первого этапа (решение о необходимости ответа)
type FreeWillShouldReplyDecision struct {
	ShouldReply     bool   `json:"should_reply"`
	ReplyType       string `json:"reply_type"` // "general", "direct_reply", "silence_response", "context_based"
	TargetMessageID int    `json:"target_message_id"`
	Reason          string `json:"reason"`
}

// FreeWillResponseTypeDecision - результат второго этапа (определение типа ответа)
type FreeWillResponseTypeDecision struct {
	Text    string `json:"text"`
	IsVoice bool   `json:"is_voice"`
	Mood    string `json:"mood"` // "sarcastic", "supportive", "neutral", "playful", "serious"
}

// FreeWillDecision - полное решение (комбинация двух этапов)
type FreeWillDecision struct {
	ShouldReply     bool   `json:"should_reply"`
	ReplyType       string `json:"reply_type"` // "general", "direct_reply", "voice", "mood_based", "silence_response", "context_based"
	TargetMessageID int    `json:"target_message_id"`
	Text            string `json:"text"`
	Reason          string `json:"reason"`
	IsVoice         bool   `json:"is_voice"`
	Mood            string `json:"mood"` // "sarcastic", "supportive", "neutral", "playful", "serious"
}

// FreeWillMoodState представляет текущее настроение бота
type FreeWillMoodState struct {
	CurrentMood    string    `json:"current_mood"`
	MoodIntensity  float64   `json:"mood_intensity"` // 0.0 - 1.0
	LastMoodUpdate time.Time `json:"last_mood_update"`
	TriggerReason  string    `json:"trigger_reason"`
}

// FreeWillTakeDetection результат детекции тейка
type FreeWillTakeDetection struct {
	IsTake     bool    `json:"is_take"`
	Title      string  `json:"title"`
	Confidence float64 `json:"confidence"`
}

// FreeWillReactionDecision решение о постановке реакции
type FreeWillReactionDecision struct {
	ShouldReact bool   `json:"should_react"`
	Reaction    string `json:"reaction"`
	Reason      string `json:"reason"`
}

// FreeWillStats статистика работы Free Will
type FreeWillStats struct {
	TotalDecisions    int            `json:"total_decisions"`
	DecisionsByType   map[string]int `json:"decisions_by_type"`
	LastDecisionTime  time.Time      `json:"last_decision_time"`
	DecisionsThisHour int            `json:"decisions_this_hour"`
	HourResetTime     time.Time      `json:"hour_reset_time"`

	// Отдельные счетчики для прямых обращений
	DirectResponsesThisHour     int       `json:"direct_responses_this_hour"`
	DirectResponseHourResetTime time.Time `json:"direct_response_hour_reset_time"`
	LastDirectResponseTime      time.Time `json:"last_direct_response_time"`

	// Отдельные счетчики для генерации изображений
	ImageGenerationDecisionsThisInterval int       `json:"image_generation_decisions_this_interval"`
	ImageGenerationIntervalResetTime     time.Time `json:"image_generation_interval_reset_time"`
	LastImageGenerationDecisionTime      time.Time `json:"last_image_generation_decision_time"`
}

// FreeWillService - основной сервис модуля Free Will
type FreeWillService struct {
	bot             *Bot
	enabled         bool
	lastActivation  map[int64]time.Time      // Время последней активации по чатам
	lastMessage     map[int64]time.Time      // Время последнего сообщения в чате
	targetIntervals map[int64]time.Duration  // Целевые интервалы активации для каждого чата
	stats           map[int64]*FreeWillStats // Статистика по чатам
	activeAnalysis  map[int64]bool           // Флаги активных анализов по чатам
	mutex           sync.RWMutex
	randSource      *rand.Rand
	ticker          *time.Ticker
	stopChan        chan bool
	isRunning       bool

	// Настройки
	minActivationInterval time.Duration // Минимальный интервал между активациями
	maxActivationInterval time.Duration // Максимальный интервал между активациями
	contextWindow         int           // Количество сообщений для анализа контекста
	moodUpdateProbability float64       // Вероятность обновления настроения (0.0-1.0)
	maxDecisionsPerHour   int           // Максимальное количество решений в час
	voiceProbability      float64       // Вероятность голосового сообщения

	// Настройки реакции на тишину
	silenceMinDuration time.Duration // Минимальное время тишины для реакции
	silenceMaxDuration time.Duration // Максимальное время тишины для реакции

	// Промпт для ответа на тейки
	takeResponsePrompt string

	// Настройки реакций
	reactionsEnabled        bool
	reactionsProbability    float64
	reactionsCooldownPeriod time.Duration
	reactionsMaxPerHour     int
	lastReactionTimes       map[int64]time.Time // Время последней реакции по чатам
	reactionCountThisHour   map[int64]int       // Количество реакций за час по чатам
	reactionHourResetTime   map[int64]time.Time // Время сброса счетчика реакций

	// Настройки прямых обращений (отдельные лимиты)
	directResponseMaxPerHour        int           // Максимальное количество прямых ответов в час
	directResponseMinInterval       time.Duration // Минимальный интервал между прямыми ответами
	directResponseIndependentLimits bool          // Независимые лимиты для прямых обращений

	// Настройки генерации изображений (отдельные лимиты)
	imageGenerationMaxDecisionsPerInterval int           // Максимальное количество попыток принятия решения за интервал
	imageGenerationIntervalDuration        time.Duration // Длительность интервала для лимита изображений
	imageGenerationMinDecisionInterval     time.Duration // Минимальный интервал между решениями
	imageGenerationIndependentLimits       bool          // Независимые лимиты для изображений

	// Система предотвращения дублирования решений
	processedMessages map[string]bool // Ключ: "chatID:messageID", значение: обработано ли сообщение
}

// NewFreeWillService создает новый сервис Free Will
func NewFreeWillService(bot *Bot) *FreeWillService {
	log.Printf("[FreeWill] NewFreeWillService: 🔧 === ИНИЦИАЛИЗАЦИЯ СЕРВИСА FREE WILL ===")

	service := &FreeWillService{
		bot:                   bot,
		enabled:               bot.config.FreeWillEnabled,
		lastActivation:        make(map[int64]time.Time),
		lastMessage:           make(map[int64]time.Time),
		targetIntervals:       make(map[int64]time.Duration),
		stats:                 make(map[int64]*FreeWillStats),
		activeAnalysis:        make(map[int64]bool),
		randSource:            rand.New(rand.NewSource(time.Now().UnixNano())),
		minActivationInterval: time.Duration(bot.config.FreeWillMinIntervalMinutes * float64(time.Minute)),
		maxActivationInterval: time.Duration(bot.config.FreeWillMaxIntervalMinutes * float64(time.Minute)),
		contextWindow:         bot.config.FreeWillContextWindow,
		moodUpdateProbability: bot.config.FreeWillMoodUpdateProbability,
		maxDecisionsPerHour:   bot.config.FreeWillMaxDecisionsPerHour,
		voiceProbability:      bot.config.FreeWillVoiceProbability,
		silenceMinDuration:    time.Duration(bot.config.FreeWillSilenceMinMinutes * float64(time.Minute)),
		silenceMaxDuration:    time.Duration(bot.config.FreeWillSilenceMaxMinutes * float64(time.Minute)),
		// Промпт для ответа на тейки
		takeResponsePrompt: bot.config.FreeWillTakeResponsePrompt,
		// Настройки реакций
		reactionsEnabled:        bot.config.FreeWillReactionsEnabled,
		reactionsProbability:    bot.config.FreeWillReactionsProbability,
		reactionsCooldownPeriod: time.Duration(bot.config.FreeWillReactionsCooldownMinutes) * time.Minute,
		reactionsMaxPerHour:     bot.config.FreeWillReactionsMaxPerHour,
		lastReactionTimes:       make(map[int64]time.Time),
		reactionCountThisHour:   make(map[int64]int),
		reactionHourResetTime:   make(map[int64]time.Time),
		processedMessages:       make(map[string]bool),
		ticker:                  time.NewTicker(time.Minute),
		stopChan:                make(chan bool),
		isRunning:               true,
		// Настройки прямых обращений (отдельные лимиты)
		directResponseMaxPerHour:        bot.config.FreeWillDirectResponseMaxPerHour,
		directResponseMinInterval:       time.Duration(bot.config.FreeWillDirectResponseMinIntervalSeconds * float64(time.Second)),
		directResponseIndependentLimits: bot.config.FreeWillDirectResponseIndependentLimits,
		// Настройки генерации изображений (отдельные лимиты)
		imageGenerationMaxDecisionsPerInterval: bot.config.FreeWillImageGenerationMaxDecisionsPerInterval,
		imageGenerationIntervalDuration:        time.Duration(bot.config.FreeWillImageGenerationIntervalHours) * time.Hour,
		imageGenerationMinDecisionInterval:     time.Duration(bot.config.FreeWillImageGenerationMinDecisionIntervalMinutes) * time.Minute,
		imageGenerationIndependentLimits:       bot.config.FreeWillImageGenerationIndependentLimits,
	}

	// === ИНИЦИАЛИЗАЦИЯ lastMessage ИЗ БД ===
	log.Printf("[FreeWill] NewFreeWillService: 📊 Инициализация lastMessage из БД...")
	service.initializeLastMessageFromDatabase()

	log.Printf("[FreeWill] NewFreeWillService: 📋 === КОНФИГУРАЦИЯ FREE WILL ===")
	log.Printf("[FreeWill] NewFreeWillService:   ✅ Включен: %t", service.enabled)
	log.Printf("[FreeWill] NewFreeWillService:   ⏱️  Минимальный интервал активации: %v", service.minActivationInterval)
	log.Printf("[FreeWill] NewFreeWillService:   ⏱️  Максимальный интервал активации: %v", service.maxActivationInterval)
	log.Printf("[FreeWill] NewFreeWillService:   📖 Окно контекста: %d сообщений", service.contextWindow)
	log.Printf("[FreeWill] NewFreeWillService:   🎭 Вероятность обновления настроения: %.2f", service.moodUpdateProbability)
	log.Printf("[FreeWill] NewFreeWillService:   🔢 Максимум решений в час: %d", service.maxDecisionsPerHour)
	log.Printf("[FreeWill] NewFreeWillService:   🎤 Вероятность голосовых сообщений: %.2f", service.voiceProbability)
	log.Printf("[FreeWill] NewFreeWillService:   🔇 Реакция на тишину: %v - %v", service.silenceMinDuration, service.silenceMaxDuration)
	log.Printf("[FreeWill] NewFreeWillService:   📖 Тейки: промпт загружен=%t", service.takeResponsePrompt != "")
	log.Printf("[FreeWill] NewFreeWillService:   🎭 Реакции: включены=%t, вероятность=%.2f, cooldown=%v, макс/час=%d", service.reactionsEnabled, service.reactionsProbability, service.reactionsCooldownPeriod, service.reactionsMaxPerHour)
	log.Printf("[FreeWill] NewFreeWillService:   🎭 Реакции: включены=%t, вероятность=%.2f, cooldown=%v, макс/час=%d", service.reactionsEnabled, service.reactionsProbability, service.reactionsCooldownPeriod, service.reactionsMaxPerHour)
	log.Printf("[FreeWill] NewFreeWillService:   📞 Прямые обращения: макс/час=%d, мин.интервал=%v, независимые лимиты=%t", service.directResponseMaxPerHour, service.directResponseMinInterval, service.directResponseIndependentLimits)
	log.Printf("[FreeWill] NewFreeWillService:   🖼️  Генерация изображений: макс решений/интервал=%d, интервал=%v, мин.интервал решений=%v, независимые лимиты=%t", service.imageGenerationMaxDecisionsPerInterval, service.imageGenerationIntervalDuration, service.imageGenerationMinDecisionInterval, service.imageGenerationIndependentLimits)
	log.Printf("[FreeWill] NewFreeWillService:   �🕐 Тикер создан: %p", service.ticker)
	log.Printf("[FreeWill] NewFreeWillService:   📡 Канал остановки: %p", service.stopChan)
	log.Printf("[FreeWill] NewFreeWillService:   🔄 isRunning: %t", service.isRunning)

	// КРИТИЧЕСКИ ВАЖНО: Очищаем все активные анализы при старте
	// (на случай если бот был прерван в процессе анализа)
	log.Printf("[FreeWill] NewFreeWillService: 🧹 Очищаем activeAnalysis флаги от предыдущих запусков...")
	service.activeAnalysis = make(map[int64]bool)
	log.Printf("[FreeWill] NewFreeWillService: ✅ activeAnalysis очищен")

	log.Printf("[FreeWill] NewFreeWillService: ✅ === СЕРВИС FREE WILL УСПЕШНО ИНИЦИАЛИЗИРОВАН ===")

	return service
}

// initializeLastMessageFromDatabase инициализирует lastMessage временем последнего сообщения из БД для всех чатов
func (fws *FreeWillService) initializeLastMessageFromDatabase() {
	log.Printf("[FreeWill] initializeLastMessageFromDatabase: 🔧 Получаем список всех чатов...")

	// Проверяем что storage доступен (для тестов может быть nil)
	if fws.bot.storage == nil {
		log.Printf("[FreeWill] initializeLastMessageFromDatabase: ⚠️ Storage недоступен, пропускаем инициализацию")
		return
	}

	// Получаем список всех чатов из БД
	chatIDs, err := fws.bot.storage.GetAllChatIDs()
	if err != nil {
		log.Printf("[FreeWill] initializeLastMessageFromDatabase: ❌ Ошибка получения списка чатов: %v", err)
		return
	}

	log.Printf("[FreeWill] initializeLastMessageFromDatabase: 📊 Найдено %d чатов для инициализации", len(chatIDs))

	initializedCount := 0
	for _, chatID := range chatIDs {
		// Получаем последнее сообщение для каждого чата
		messages, err := fws.bot.storage.GetMessages(chatID, 1)
		if err != nil {
			log.Printf("[FreeWill] initializeLastMessageFromDatabase: ❌ Ошибка получения последнего сообщения для чата %d: %v", chatID, err)
			continue
		}

		if len(messages) > 0 {
			lastMessage := messages[0]
			lastMessageTime := time.Unix(int64(lastMessage.Date), 0)
			fws.lastMessage[chatID] = lastMessageTime
			initializedCount++

			log.Printf("[FreeWill] initializeLastMessageFromDatabase: ✅ Чат %d: последнее сообщение %v (ID: %d)",
				chatID, lastMessageTime.Format("15:04:05 02.01.2006"), lastMessage.MessageID)
		} else {
			log.Printf("[FreeWill] initializeLastMessageFromDatabase: ⚠️ Чат %d: нет сообщений в БД", chatID)
		}
	}

	log.Printf("[FreeWill] initializeLastMessageFromDatabase: 🎯 Инициализация завершена: %d/%d чатов", initializedCount, len(chatIDs))
}

// OnMessage вызывается при получении нового сообщения
func (fws *FreeWillService) OnMessage(chatID int64, message *tgbotapi.Message) {
	messageTime := time.Now()
	log.Printf("[FreeWill] OnMessage: 📨 НОВОЕ СООБЩЕНИЕ чат:%d пользователь:%d время:%v",
		chatID, message.From.ID, messageTime.Format("15:04:05"))

	if !fws.enabled {
		log.Printf("[FreeWill] OnMessage: ❌ Free Will отключен (enabled=%t), пропускаем сообщение в чате %d",
			fws.enabled, chatID)
		return
	}

	log.Printf("[FreeWill] OnMessage: ✅ Free Will включен, обрабатываем сообщение...")

	// Проверяем, не было ли сообщение уже обработано
	if fws.isMessageProcessed(chatID, message.MessageID) {
		log.Printf("[FreeWill] OnMessage: ⚠️ Сообщение %d в чате %d уже было обработано, пропускаем", message.MessageID, chatID)
		return
	}

	log.Printf("[FreeWill] OnMessage: 🔒 Получаем Lock для обновления данных...")
	fws.mutex.Lock()

	// Обновляем время последнего сообщения
	oldLastMessage := fws.lastMessage[chatID]
	newLastMessage := time.Now()
	fws.lastMessage[chatID] = newLastMessage
	log.Printf("[FreeWill] OnMessage: ⏰ Время последнего сообщения чат:%d %v -> %v",
		chatID, oldLastMessage.Format("15:04:05"), newLastMessage.Format("15:04:05"))

	log.Printf("[FreeWill] OnMessage: 🧠 Проверяем активацию анализа...")
	shouldActivate := fws.shouldActivateAnalysis(chatID)
	log.Printf("[FreeWill] OnMessage: 🎯 Результат проверки активации: %t", shouldActivate)

	moodRoll := fws.randSource.Float64()
	shouldUpdateMood := moodRoll < fws.moodUpdateProbability
	log.Printf("[FreeWill] OnMessage: 🎭 Проверка обновления настроения: %.3f < %.3f = %t",
		moodRoll, fws.moodUpdateProbability, shouldUpdateMood)

	fws.mutex.Unlock()
	log.Printf("[FreeWill] OnMessage: 🔓 Lock освобожден")

	log.Printf("[FreeWill] OnMessage: Время последнего сообщения для чата %d обновлено: %v -> %v",
		chatID, oldLastMessage.Format("15:04:05"), fws.lastMessage[chatID].Format("15:04:05"))
	log.Printf("[FreeWill] OnMessage: Проверка активации анализа для чата %d: %t", chatID, shouldActivate)
	log.Printf("[FreeWill] OnMessage: Проверка обновления настроения для чата %d: %.3f < %.3f = %t",
		chatID, moodRoll, fws.moodUpdateProbability, shouldUpdateMood)

	// Отмечаем сообщение как обработанное в OnMessage
	fws.markMessageProcessed(chatID, message.MessageID)

	// Проверяем, нужно ли активировать анализ
	if shouldActivate {
		log.Printf("[FreeWill] OnMessage: Запускаем анализ для чата %d", chatID)
		log.Printf("[FreeWill] OnMessage: 🚀 ЗАПУСКАЕМ ГОРУТИНУ analyzeAndAct для чата %d", chatID)

		// Устанавливаем флаг активного анализа
		fws.mutex.Lock()
		fws.activeAnalysis[chatID] = true
		fws.mutex.Unlock()

		go func() {
			defer func() {
				// Сбрасываем флаг активного анализа при завершении
				fws.mutex.Lock()
				fws.activeAnalysis[chatID] = false
				fws.mutex.Unlock()
				log.Printf("[FreeWill] OnMessage: 🏁 Горутина analyzeAndAct ЗАВЕРШЕНА для чата %d", chatID)
			}()

			log.Printf("[FreeWill] OnMessage: 🏁 Горутина analyzeAndAct ЗАПУЩЕНА для чата %d", chatID)
			fws.analyzeAndAct(chatID)
		}()
		log.Printf("[FreeWill] OnMessage: ✅ Горутина для чата %d отправлена в планировщик", chatID)
	}

	// Обновляем настроение с определенной вероятностью
	if shouldUpdateMood {
		log.Printf("[FreeWill] OnMessage: Запускаем обновление настроения для чата %d", chatID)
		go func() {
			context, err := fws.getContextForAnalysis(chatID)
			if err != nil {
				log.Printf("[FreeWill] OnMessage: Ошибка получения контекста для обновления настроения чата %d: %v", chatID, err)
				return
			}
			fws.updateMood(chatID, context)
		}()
	}

	// Постановка реакций (независимо от принятия решений)
	if fws.reactionsEnabled {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[FreeWill] OnMessage: Ошибка в анализе реакций для чата %d: %v", chatID, r)
				}
			}()

			reactionRoll := fws.randSource.Float64()
			if reactionRoll < fws.reactionsProbability {
				log.Printf("[FreeWill] OnMessage: Запускаем анализ реакций для чата %d", chatID)
				fws.analyzeForReaction(chatID, message)
			}
		}()
	}
}

// OnDirectMention вызывается при прямом обращении к боту по имени или reply_to
// когда DIRECT_PROMPT отключен - передаем решение в Free Will
func (fws *FreeWillService) OnDirectMention(chatID int64, message *tgbotapi.Message) {
	log.Printf("[FreeWill] OnDirectMention: 📢 ПРЯМОЕ ОБРАЩЕНИЕ чат:%d пользователь:%d",
		chatID, message.From.ID)

	if !fws.enabled {
		log.Printf("[FreeWill] OnDirectMention: ❌ Free Will отключен, пропускаем прямое обращение")
		return
	}

	// Проверяем, не было ли сообщение уже обработано
	if fws.isMessageProcessed(chatID, message.MessageID) {
		log.Printf("[FreeWill] OnDirectMention: ⚠️ Сообщение %d в чате %d уже было обработано, пропускаем", message.MessageID, chatID)
		return
	}

	// Отмечаем сообщение как обработанное в OnDirectMention
	fws.markMessageProcessed(chatID, message.MessageID)

	// Запускаем анализ в отдельной горутине для Free Will Direct Response
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[ERROR][FreeWill] OnDirectMention: Panic в горутине прямого обращения: %v\nStack: %s",
					r, debug.Stack())
			}
		}()

		log.Printf("[FreeWill] OnDirectMention: 🧠 Запускаем анализ Free Will Direct Response для чата %d", chatID)
		fws.analyzeDirectResponse(chatID, message)
	}()
}

// analyzeDirectResponse анализирует прямое обращение и принимает решение о ответе (ДВУХЭТАПНАЯ АРХИТЕКТУРА)
func (fws *FreeWillService) analyzeDirectResponse(chatID int64, message *tgbotapi.Message) {
	log.Printf("[FreeWill] analyzeDirectResponse: 🎯 === АНАЛИЗ ПРЯМОГО ОБРАЩЕНИЯ (ДВУХЭТАПНЫЙ) ===")
	log.Printf("[FreeWill] analyzeDirectResponse: 📍 Чат: %d, Сообщение: ID=%d От=%d",
		chatID, message.MessageID, message.From.ID)

	// Проверяем лимиты прямых обращений
	if !fws.canProcessDirectResponse(chatID) {
		log.Printf("[FreeWill] analyzeDirectResponse: ❌ Прямое обращение заблокировано лимитами для чата %d", chatID)
		return
	}

	// ЭТАП 1: ПРИНЯТИЕ РЕШЕНИЯ
	log.Printf("[FreeWill] analyzeDirectResponse: 🎯 ЭТАП 1: Принятие решения")

	// Получаем общий контекст чата для понимания атмосферы
	generalContext, err := fws.getContextForAnalysis(chatID)
	if err != nil {
		log.Printf("[ERROR][FreeWill] analyzeDirectResponse: Ошибка получения контекста для чата %d: %v", chatID, err)
		return
	}

	// Формируем контекст для ЭТАПА 1: конкретное сообщение + общий контекст
	decisionContext := fmt.Sprintf("=== КОНКРЕТНОЕ ОБРАЩЕНИЕ ===\nОт: %s (ID: %d)\nСообщение: %s\n\n=== ОБЩИЙ КОНТЕКСТ ЧАТА ===\n%s",
		fws.getMessageAuthorAlias(chatID, message, nil),
		message.From.ID,
		message.Text,
		generalContext)

	// Подмешиваем лёгкий ассоциативный контекст (зафичено флагом)
	if assoc := fws.bot.getAssociativeContext(chatID, fws.bot.buildAssociativeKeys(chatID), 3); assoc != "" {
		decisionContext = assoc + "\n\n" + decisionContext
	}

	// Используем промпт принятия решения
	decisionPrompt := fws.bot.enrichPromptWithPersonality(fws.bot.config.FreeWillDirectResponseDecisionPrompt, chatID, "free_will_direct_response_decision")

	log.Printf("[FreeWill] analyzeDirectResponse: 🤖 ЭТАП 1: Отправляем запрос в LLM для принятия решения")
	log.Printf("[FreeWill] analyzeDirectResponse: 📝 Промпт решения длина: %d символов", len(decisionPrompt))
	log.Printf("[FreeWill] analyzeDirectResponse: 📝 Контекст решения длина: %d символов", len(decisionContext))

	// Генерируем решение через LLM
	decisionResponse, err := fws.bot.llm.GenerateResponseByType(
		llm.ResponseTypeFreeWillDirectResponseDecision,
		decisionPrompt,
		decisionContext, // Передаем контекст с конкретным сообщением
		0.7,             // Более низкая температура для принятия решений
	)

	if err != nil {
		log.Printf("[ERROR][FreeWill] analyzeDirectResponse: Ошибка генерации решения LLM для чата %d: %v", chatID, err)
		return
	}

	log.Printf("[FreeWill] analyzeDirectResponse: ✅ ЭТАП 1: Получено решение от LLM: %s", decisionResponse)

	// Парсим решение о том, стоит ли отвечать
	shouldReplyDecision, err := fws.parseDirectResponseShouldReplyDecision(decisionResponse)
	if err != nil {
		log.Printf("[ERROR][FreeWill] analyzeDirectResponse: Ошибка парсинга решения для чата %d: %v", chatID, err)
		return
	}

	log.Printf("[FreeWill] analyzeDirectResponse: 🎲 ЭТАП 1 РЕШЕНИЕ: отвечать=%t причина=%s",
		shouldReplyDecision.ShouldReply, shouldReplyDecision.Reason)

	// Раньше: если решили НЕ отвечать — завершали обработку.
	// Теперь: для упругости UX продолжаем к ЭТАПУ 2 и формируем короткий, безопасный direct reply.
	// Это сохраняет поведение прямых обращений ожидаемым, даже когда ЭТАП 1 склоняется к отказу.
	if !shouldReplyDecision.ShouldReply {
		log.Printf("[FreeWill] analyzeDirectResponse: 🤐 Решили НЕ отвечать (ЭТАП 1) в чате %d: %s — продолжаем по fallback к короткому ответу",
			chatID, shouldReplyDecision.Reason)
		// Не меняем shouldReplyDecision, просто идём дальше. По умолчанию msgType останется 'casual'.
	}

	// ЭТАП 1.5: КЛАССИФИКАЦИЯ СЕРЬЕЗНОСТИ (НОВЫЙ ЭТАП)
	log.Printf("[FreeWill] analyzeDirectResponse: 🔍 ЭТАП 1.5: Классификация серьезности")

	msgType := "casual" // по умолчанию

	// Классифицируем только если есть не-пустой текст
	if message.Text != "" {
		if fws.bot.config.ClassifyDirectMessagePrompt != "" {
			// Формируем входной текст для классификации - только конкретное сообщение
			classifyInput := message.Text

			log.Printf("[FreeWill] analyzeDirectResponse: 🤖 ЭТАП 1.5: Отправляем запрос классификации серьезности")

			// Классифицируем сообщение
			classifyResult, err := fws.bot.llm.GenerateResponseByType(
				llm.ResponseTypeClassify,
				fws.bot.config.ClassifyDirectMessagePrompt,
				classifyInput,
				float32(fws.bot.config.GeminiTemperatureSerious),
			)

			if err != nil {
				log.Printf("[WARN][FreeWill] analyzeDirectResponse: Ошибка при классификации сообщения в чате %d: %v", chatID, err)
			} else {
				// Очищаем результат классификации от возможных метаданных
				classifyResult = cleanupLLMResponse(classifyResult)

				// Проверяем результат классификации
				lower := strings.ToLower(strings.TrimSpace(classifyResult))
				log.Printf("[DEBUG][FreeWill] analyzeDirectResponse: Результат классификации для чата %d: '%s'", chatID, lower)

				// Расширенная проверка с учетом возможных вариантов ответа на русском и английском
				if strings.Contains(lower, "serious") ||
					strings.Contains(lower, "серьезн") ||
					strings.Contains(lower, "серьёзн") ||
					lower == "yes" ||
					lower == "да" {
					msgType = "serious"
					log.Printf("[DEBUG][FreeWill] analyzeDirectResponse: Сообщение в чате %d классифицировано как SERIOUS", chatID)
				} else if strings.Contains(lower, "casual") ||
					strings.Contains(lower, "обычн") ||
					strings.Contains(lower, "несерьезн") ||
					strings.Contains(lower, "несерьёзн") ||
					lower == "no" ||
					lower == "нет" {
					log.Printf("[DEBUG][FreeWill] analyzeDirectResponse: Сообщение в чате %d классифицировано как CASUAL", chatID)
				} else {
					// Если ответ LLM непонятен, используем casual по умолчанию
					log.Printf("[DEBUG][FreeWill] analyzeDirectResponse: Результат классификации в чате %d не распознан, использую CASUAL", chatID)
				}
			}
		}
	}

	// ЭТАП 2: ГЕНЕРАЦИЯ ОТВЕТА
	log.Printf("[FreeWill] analyzeDirectResponse: 🎭 ЭТАП 2: Генерация ответа (тип: %s)", msgType)

	// Получаем контекст для генерации ответа - с фокусом на прямое обращение
	responseContext, err := fws.getDirectReplyContext(chatID, message.MessageID)
	if err != nil || responseContext == "" {
		// Fallback к общему контексту
		responseContext = generalContext
		log.Printf("[WARN][FreeWill] analyzeDirectResponse: Используем общий контекст как fallback для ответа")
	}

	// Подмешиваем лёгкий ассоциативный контекст (зафичено флагом)
	if assoc := fws.bot.getAssociativeContext(chatID, nil, 3); assoc != "" {
		responseContext = assoc + "\n\n" + responseContext
	}

	// === КОГНИТИВНАЯ ИНТЕГРАЦИЯ (ЭТАП 3): Внутренний монолог перед генерацией ===
	if fws.bot.config.InternalMonologueEnabled {
		trigger := message.Text
		thought := fws.bot.InternalMonologue(chatID, trigger, "free_will_direct")
		if thought != nil {
			thought.ActionTaken = true
			fws.bot.RecordInternalThought(chatID, thought)
			log.Printf("[Stage3][FW-DR] Чат %d: injected internal_thought type=%s len=%d", chatID, thought.Type, len(thought.Content))
			responseContext = "[internal_thought]: " + utils.TruncateString(thought.Content, 120) + "\n\n" + responseContext
		}
	}

	// Детерминированная подсказка стиля на основе отношений с автором сообщения
	userID := int64(message.From.ID)
	before := len(responseContext)
	responseContext = fws.bot.ApplyRelationshipStyleToContext(chatID, userID, responseContext)
	if len(responseContext) > before {
		style := fws.bot.GetRelationshipInfluencedCommunicationStyle(chatID, userID)
		log.Printf("[Stage4][FW-DR] Chat %d: tone_hint applied, style=%s", chatID, style)
	}

	// Выбираем промпт и настройки в зависимости от серьезности
	var responsePrompt string
	var responseTemperature float32
	var responseType llm.ResponseType

	if msgType == "serious" && fws.bot.config.SeriousDirectPrompt != "" {
		responsePrompt = fws.bot.enrichPromptWithPersonality(fws.bot.config.SeriousDirectPrompt, chatID, "serious_direct")
		responseTemperature = float32(fws.bot.config.GeminiTemperatureSerious)
		responseType = llm.ResponseTypeSerious

		log.Printf("[INFO][FreeWill] analyzeDirectResponse: Чат %d: Используем SERIOUS_DIRECT_PROMPT", chatID)

		// Для серьезных ответов используем Smart веб‑поиск
		if fws.bot.webSearch != nil && fws.bot.webSearch.IsEnabled() {
			enhancedContext := fws.bot.webSearch.EnhanceContextWithSmartWebSearch(responseContext, message.Text)
			if enhancedContext != responseContext {
				responseContext = enhancedContext
				log.Printf("[INFO][FreeWill] analyzeDirectResponse: Чат %d: Контекст расширен результатами веб-поиска (smart) для серьезного ответа", chatID)
			}
		}
	} else {
		responsePrompt = fws.bot.enrichPromptWithPersonality(fws.bot.config.FreeWillDirectResponsePrompt, chatID, "free_will_direct_response")
		responseTemperature = float32(fws.bot.config.GeminiTemperatureNormal)
		responseType = llm.ResponseTypeFreeWillDirectResponse

		log.Printf("[INFO][FreeWill] analyzeDirectResponse: Чат %d: Используем стандартный FREE_WILL_DIRECT_RESPONSE_PROMPT", chatID)
	}

	log.Printf("[FreeWill] analyzeDirectResponse: 🤖 ЭТАП 2: Отправляем запрос в LLM для генерации ответа")
	log.Printf("[FreeWill] analyzeDirectResponse: 📝 Промпт ответа длина: %d символов", len(responsePrompt))
	log.Printf("[INFO][FreeWill] analyzeDirectResponse: Генерируем ответ. Тип: %s, Температура: %.2f", msgType, responseTemperature)

	// Генерируем ответ через LLM
	responseContent, err := fws.bot.llm.GenerateResponseByType(
		responseType,
		responsePrompt,
		responseContext, // Используем контекст прямого ответа
		responseTemperature,
	)

	if err != nil {
		log.Printf("[ERROR][FreeWill] analyzeDirectResponse: Ошибка генерации ответа LLM для чата %d: %v", chatID, err)
		return
	}

	log.Printf("[FreeWill] analyzeDirectResponse: ✅ ЭТАП 2: Получен ответ от LLM: %s", responseContent)

	// Парсим сгенерированный ответ
	responseDecision, err := fws.parseDirectResponseContentDecision(responseContent)
	if err != nil {
		log.Printf("[ERROR][FreeWill] analyzeDirectResponse: Ошибка парсинга ответа для чата %d: %v", chatID, err)
		return
	}

	// Упрощенная логика target_message_id - всегда отвечаем на исходное сообщение
	targetMessageID := message.MessageID
	log.Printf("[FreeWill] analyzeDirectResponse: 🎯 Отвечаем на исходное сообщение ID=%d", targetMessageID)

	// Объединяем решения в итоговое решение
	finalDecision := &FreeWillDecision{
		ShouldReply:     true,           // Уже решили отвечать на Этапе 1
		ReplyType:       "direct_reply", // Всегда прямой ответ для direct mention
		TargetMessageID: targetMessageID,
		Text:            responseDecision.Text,
		IsVoice:         responseDecision.IsVoice,
		Mood:            responseDecision.Mood,
		Reason:          shouldReplyDecision.Reason,
	}

	log.Printf("[FreeWill] analyzeDirectResponse: 🎉 ИТОГОВОЕ РЕШЕНИЕ: текст='%s' голос=%t настроение=%s",
		finalDecision.Text, finalDecision.IsVoice, finalDecision.Mood)

	// Выполняем решение
	fws.executeDecision(chatID, finalDecision)

	// Обновляем статистику прямых обращений
	fws.updateDirectResponseStats(chatID, finalDecision)
}

// parseDirectResponseShouldReplyDecision парсит ответ LLM для принятия решения (Этап 1)
func (fws *FreeWillService) parseDirectResponseShouldReplyDecision(response string) (*FreeWillShouldReplyDecision, error) {
	// Пытаемся распарсить как JSON
	var decision FreeWillShouldReplyDecision

	// Очищаем ответ от markdown (специально для JSON)
	cleanResponse := cleanJSONFromMarkdown(response)

	log.Printf("[DEBUG][FreeWill] parseDirectResponseShouldReplyDecision: Исходный ответ: %s", response)
	log.Printf("[DEBUG][FreeWill] parseDirectResponseShouldReplyDecision: Очищенный ответ: %s", cleanResponse)

	if err := json.Unmarshal([]byte(cleanResponse), &decision); err != nil {
		// Если JSON не парсится, возвращаем решение не отвечать
		log.Printf("[WARN][FreeWill] parseDirectResponseShouldReplyDecision: Не удалось распарсить JSON решения: %v", err)
		log.Printf("[WARN][FreeWill] parseDirectResponseShouldReplyDecision: Проблемный JSON: %s", cleanResponse)
		return &FreeWillShouldReplyDecision{
			ShouldReply: false,
			ReplyType:   "ignore",
			Reason:      "Не удалось распарсить решение LLM",
		}, nil
	}

	log.Printf("[DEBUG][FreeWill] parseDirectResponseShouldReplyDecision: ✅ JSON успешно распарсен: should_reply=%t, reply_type=%s",
		decision.ShouldReply, decision.ReplyType)
	return &decision, nil
}

// parseDirectResponseContentDecision парсит ответ LLM для генерации контента (Этап 2)
func (fws *FreeWillService) parseDirectResponseContentDecision(response string) (*FreeWillResponseTypeDecision, error) {
	// Пытаемся распарсить как JSON
	var decision FreeWillResponseTypeDecision

	// Очищаем ответ от markdown (специально для JSON)
	cleanResponse := cleanJSONFromMarkdown(response)

	log.Printf("[DEBUG][FreeWill] parseDirectResponseContentDecision: Исходный ответ: %s", response)
	log.Printf("[DEBUG][FreeWill] parseDirectResponseContentDecision: Очищенный ответ: %s", cleanResponse)

	if err := json.Unmarshal([]byte(cleanResponse), &decision); err != nil {
		// Если JSON не парсится, используем cleanupLLMResponse для текста
		log.Printf("[WARN][FreeWill] parseDirectResponseContentDecision: Не удалось распарсить JSON ответа: %v", err)
		log.Printf("[WARN][FreeWill] parseDirectResponseContentDecision: Проблемный JSON: %s", cleanResponse)
		textResponse := cleanupLLMResponse(response)
		return &FreeWillResponseTypeDecision{
			Text:    textResponse,
			IsVoice: false,
			Mood:    "neutral",
		}, nil
	}

	log.Printf("[DEBUG][FreeWill] parseDirectResponseContentDecision: ✅ JSON успешно распарсен: text_length=%d, is_voice=%t, mood=%s",
		len(decision.Text), decision.IsVoice, decision.Mood)
	return &decision, nil
}

// parseDirectResponseDecision парсит ответ LLM для Free Will Direct Response (УСТАРЕЛ - для обратной совместимости)
func (fws *FreeWillService) parseDirectResponseDecision(response string) (*FreeWillDecision, error) {
	// Пытаемся распарсить как JSON
	var decision FreeWillDecision

	// Очищаем ответ от markdown (специально для JSON)
	cleanResponse := cleanJSONFromMarkdown(response)

	log.Printf("[DEBUG][FreeWill] parseDirectResponseDecision: Исходный ответ: %s", response)
	log.Printf("[DEBUG][FreeWill] parseDirectResponseDecision: Очищенный ответ: %s", cleanResponse)

	if err := json.Unmarshal([]byte(cleanResponse), &decision); err != nil {
		// Если JSON не парсится, возвращаем простое решение не отвечать
		log.Printf("[WARN][FreeWill] parseDirectResponseDecision: Не удалось распарсить JSON: %v", err)
		log.Printf("[WARN][FreeWill] parseDirectResponseDecision: Проблемный JSON: %s", cleanResponse)
		return &FreeWillDecision{
			ShouldReply: false,
			ReplyType:   "ignore",
			Reason:      "Не удалось распарсить решение LLM",
		}, nil
	}

	log.Printf("[DEBUG][FreeWill] parseDirectResponseDecision: ✅ JSON успешно распарсен: should_reply=%t, reply_type=%s",
		decision.ShouldReply, decision.ReplyType)
	return &decision, nil
}

// shouldActivateAnalysis определяет, нужно ли активировать анализ (вызывается под мьютексом)
func (fws *FreeWillService) shouldActivateAnalysis(chatID int64) bool {
	log.Printf("[FreeWill] shouldActivateAnalysis: Начинаем проверку активации для чата %d", chatID)

	// 0. Проверяем, не запущен ли уже анализ для этого чата
	if fws.activeAnalysis[chatID] {
		log.Printf("[FreeWill] shouldActivateAnalysis: ❌ Анализ уже активен для чата %d, отклоняем", chatID)
		return false
	}

	// 1. Проверяем лимит решений за час
	stats := fws.getOrCreateStats(chatID)
	if time.Since(stats.HourResetTime) > time.Hour {
		log.Printf("[FreeWill] shouldActivateAnalysis: Сброс часового счетчика решений для чата %d: %d -> 0", chatID, stats.DecisionsThisHour)
		stats.DecisionsThisHour = 0
		stats.HourResetTime = time.Now()
	}

	if stats.DecisionsThisHour >= fws.maxDecisionsPerHour {
		log.Printf("[FreeWill] shouldActivateAnalysis: Превышен лимит решений для чата %d (%d/%d), отклоняем",
			chatID, stats.DecisionsThisHour, fws.maxDecisionsPerHour)
		return false
	}

	lastActivation, exists := fws.lastActivation[chatID]

	// 2. Проверяем временные интервалы
	if exists {
		elapsed := time.Since(lastActivation)

		// 2а. Проверяем, не слишком ли рано
		if elapsed < fws.minActivationInterval {
			log.Printf("[FreeWill] shouldActivateAnalysis: Слишком рано для активации чата %d (прошло %v, минимум %v)",
				chatID, elapsed, fws.minActivationInterval)
			return false
		}

		// 2б. Проверяем, не слишком ли поздно (принудительная активация)
		if elapsed > fws.maxActivationInterval {
			log.Printf("[FreeWill] shouldActivateAnalysis: Превышен максимальный интервал для чата %d (прошло %v, максимум %v), принудительная активация.",
				chatID, elapsed, fws.maxActivationInterval)
			return true
		}

		// 2в. ИСПРАВЛЕНО: Используем сохраненный или генерируем новый случайный интервал активации
		targetInterval, exists := fws.targetIntervals[chatID]
		if !exists {
			// Генерируем новый случайный интервал для этого чата
			intervalRange := fws.maxActivationInterval - fws.minActivationInterval
			randomOffset := time.Duration(fws.randSource.Float64() * float64(intervalRange))
			targetInterval = fws.minActivationInterval + randomOffset
			fws.targetIntervals[chatID] = targetInterval
			log.Printf("[FreeWill] shouldActivateAnalysis: 🎲 Сгенерирован новый целевой интервал для чата %d: %v (диапазон %v-%v)",
				chatID, targetInterval, fws.minActivationInterval, fws.maxActivationInterval)
		}

		if elapsed >= targetInterval {
			log.Printf("[FreeWill] shouldActivateAnalysis: ✅ Достигнут целевой интервал активации для чата %d (прошло %v, целевой интервал %v)",
				chatID, elapsed, targetInterval)
			// Генерируем новый целевой интервал для следующей активации
			intervalRange := fws.maxActivationInterval - fws.minActivationInterval
			randomOffset := time.Duration(fws.randSource.Float64() * float64(intervalRange))
			newTargetInterval := fws.minActivationInterval + randomOffset
			fws.targetIntervals[chatID] = newTargetInterval
			log.Printf("[FreeWill] shouldActivateAnalysis: 🎲 Сгенерирован новый целевой интервал для следующей активации чата %d: %v",
				chatID, newTargetInterval)
			return true
		} else {
			log.Printf("[FreeWill] shouldActivateAnalysis: ❌ Целевой интервал активации еще не достигнут для чата %d (прошло %v, нужно %v)",
				chatID, elapsed, targetInterval)
			return false
		}
	} else {
		// Первая активация для этого чата
		log.Printf("[FreeWill] shouldActivateAnalysis: ✅ Первая активация для чата %d", chatID)
		return true
	}
}

// CheckSilence проверяет тишину в чатах и запускает анализ при необходимости
func (fws *FreeWillService) CheckSilence() {
	startTime := time.Now()
	log.Printf("[FreeWill] CheckSilence: === НАЧАЛО ПРОВЕРКИ ТИШИНЫ === %v", startTime.Format("15:04:05"))

	if !fws.enabled {
		log.Printf("[FreeWill] CheckSilence: ❌ Free Will отключен (enabled=%t), пропускаем проверку тишины", fws.enabled)
		return
	}

	// Периодически очищаем кэш обработанных сообщений
	fws.cleanOldProcessedMessages()

	log.Printf("[FreeWill] CheckSilence: ✅ Free Will включен, продолжаем проверку...")
	now := time.Now()

	// ИСПРАВЛЕНО: Создаем копию данных под RLock, затем работаем с копией без блокировок
	log.Printf("[FreeWill] CheckSilence: 🔒 Получаем RLock для копирования данных...")
	fws.mutex.RLock()

	lastMessages := make(map[int64]time.Time)
	lastActivations := make(map[int64]time.Time)
	for chatID, lastMsg := range fws.lastMessage {
		lastMessages[chatID] = lastMsg
	}
	for chatID, lastAct := range fws.lastActivation {
		lastActivations[chatID] = lastAct
	}

	totalChats := len(lastMessages)
	log.Printf("[FreeWill] CheckSilence: 📊 Всего чатов для проверки: %d", totalChats)

	if totalChats == 0 {
		log.Printf("[FreeWill] CheckSilence: ⚠️ Нет чатов для проверки! lastMessage пуст")
		fws.mutex.RUnlock()
		return
	}

	// Показываем все чаты с последними сообщениями
	log.Printf("[FreeWill] CheckSilence: 📋 Список всех чатов:")
	for chatID, lastMsg := range lastMessages {
		silenceDuration := now.Sub(lastMsg)
		log.Printf("[FreeWill] CheckSilence:   - Чат %d: последнее сообщение %v назад (%v)",
			chatID, silenceDuration, lastMsg.Format("15:04:05"))
	}

	fws.mutex.RUnlock()
	log.Printf("[FreeWill] CheckSilence: 🔓 RLock освобожден, начинаем анализ без блокировок...")

	// Теперь работаем с копией данных без блокировок
	checkedChats := 0
	for chatID, lastMsg := range lastMessages {
		checkedChats++
		silenceDuration := now.Sub(lastMsg)
		log.Printf("[FreeWill] CheckSilence: Чат %d - последнее сообщение %v назад (мин: %v, макс: %v)",
			chatID, silenceDuration, fws.silenceMinDuration, fws.silenceMaxDuration)

		// Проверяем, попадает ли тишина в нужный диапазон
		if silenceDuration >= fws.silenceMinDuration && silenceDuration <= fws.silenceMaxDuration {
			log.Printf("[FreeWill] CheckSilence: Тишина в чате %d попадает в диапазон (%v), проверяем дополнительные условия",
				chatID, silenceDuration)

			// Проверяем, не слишком ли недавно была активация
			if lastActivation, exists := lastActivations[chatID]; exists {
				timeSinceActivation := now.Sub(lastActivation)
				if timeSinceActivation < fws.minActivationInterval {
					log.Printf("[FreeWill] CheckSilence: Слишком рано для активации чата %d (%v < %v)",
						chatID, timeSinceActivation, fws.minActivationInterval)
					continue
				}
				log.Printf("[FreeWill] CheckSilence: Время с последней активации чата %d достаточное (%v >= %v)",
					chatID, timeSinceActivation, fws.minActivationInterval)
			} else {
				log.Printf("[FreeWill] CheckSilence: Нет записи о предыдущей активации для чата %d", chatID)
			}

			// Проверяем лимит решений за час (одна быстрая блокировка)
			log.Printf("[FreeWill] CheckSilence: 🔒 Проверяем лимит решений для чата %d...", chatID)
			canProceed := false
			fws.mutex.Lock()
			stats := fws.getOrCreateStats(chatID)
			log.Printf("[FreeWill] CheckSilence: Проверяем лимит решений для чата %d: %d/%d за час",
				chatID, stats.DecisionsThisHour, fws.maxDecisionsPerHour)

			if now.Sub(stats.HourResetTime) > time.Hour {
				oldDecisions := stats.DecisionsThisHour
				stats.DecisionsThisHour = 0
				stats.HourResetTime = now
				log.Printf("[FreeWill] CheckSilence: Сброс счетчика решений для чата %d: %d -> 0", chatID, oldDecisions)
			}

			if stats.DecisionsThisHour >= fws.maxDecisionsPerHour {
				log.Printf("[FreeWill] CheckSilence: Превышен лимит решений для чата %d (%d/%d), пропускаем",
					chatID, stats.DecisionsThisHour, fws.maxDecisionsPerHour)
				canProceed = false
			} else {
				canProceed = true
			}
			fws.mutex.Unlock()
			log.Printf("[FreeWill] CheckSilence: 🔓 Lock освобожден для чата %d, canProceed=%t", chatID, canProceed)

			if canProceed {
				// Проверяем, не запущен ли уже анализ для этого чата
				fws.mutex.Lock()
				isAnalysisActive := fws.activeAnalysis[chatID]
				if !isAnalysisActive {
					fws.activeAnalysis[chatID] = true
				}
				fws.mutex.Unlock()

				if isAnalysisActive {
					log.Printf("[FreeWill] CheckSilence: ❌ Анализ уже активен для чата %d, пропускаем", chatID)
					continue
				}

				// Запускаем анализ для реакции на тишину
				log.Printf("[FreeWill] CheckSilence: Все условия пройдены для чата %d! Обнаружена тишина длительностью %v, запускаем анализ",
					chatID, silenceDuration)
				log.Printf("[FreeWill] CheckSilence: 🚀 ЗАПУСКАЕМ ГОРУТИНУ analyzeAndAct для чата %d", chatID)
				go func() {
					defer func() {
						// Сбрасываем флаг активного анализа при завершении
						fws.mutex.Lock()
						fws.activeAnalysis[chatID] = false
						fws.mutex.Unlock()
						log.Printf("[FreeWill] CheckSilence: 🏁 Горутина analyzeAndAct ЗАВЕРШЕНА для чата %d", chatID)
					}()

					log.Printf("[FreeWill] CheckSilence: 🏁 Горутина analyzeAndAct ЗАПУЩЕНА для чата %d", chatID)
					fws.analyzeAndAct(chatID)
				}()
				log.Printf("[FreeWill] CheckSilence: ✅ Горутина для чата %d отправлена в планировщик", chatID)
			}
		} else {
			if silenceDuration < fws.silenceMinDuration {
				log.Printf("[FreeWill] CheckSilence: Тишина в чате %d слишком короткая (%v < %v)",
					chatID, silenceDuration, fws.silenceMinDuration)
			} else {
				log.Printf("[FreeWill] CheckSilence: Тишина в чате %d слишком длинная (%v > %v)",
					chatID, silenceDuration, fws.silenceMaxDuration)
			}
		}
	}

	elapsed := time.Since(startTime)
	log.Printf("[FreeWill] CheckSilence: ✅ Завершена проверка тишины в %d/%d чатах за %v", checkedChats, totalChats, elapsed)

	// === ОТДЕЛЬНАЯ ПРОВЕРКА ГЕНЕРАЦИИ ИЗОБРАЖЕНИЙ ===
	// Проверяем возможность генерации изображений независимо от текстовых ответов
	fws.checkImageGenerationForAllChats()

	log.Printf("[FreeWill] CheckSilence: === КОНЕЦ ПРОВЕРКИ ТИШИНЫ === %v", time.Now().Format("15:04:05"))
}

// getOrCreateStats получает или создает статистику для чата (вызывается под мьютексом)
func (fws *FreeWillService) getOrCreateStats(chatID int64) *FreeWillStats {
	stats, exists := fws.stats[chatID]
	if !exists {
		stats = &FreeWillStats{
			DecisionsByType:                  make(map[string]int),
			HourResetTime:                    time.Now(),
			DirectResponseHourResetTime:      time.Now(),
			ImageGenerationIntervalResetTime: time.Now(),
		}
		fws.stats[chatID] = stats
	}
	return stats
}

// analyzeAndAct основной метод анализа и принятия решения
func (fws *FreeWillService) analyzeAndAct(chatID int64) {
	// Добавляем обработку паник и восстановление
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[FreeWill] analyzeAndAct: 🚨 КРИТИЧЕСКАЯ ОШИБКА (PANIC) в чате %d: %v", chatID, r)
			log.Printf("[FreeWill] analyzeAndAct: 🚨 Stack trace: %s", debug.Stack())
		}
	}()

	// Создаем контекст с таймаутом для всего процесса
	analyzeTimeout := 10 * time.Minute // Максимум 10 минут на весь анализ
	ctx, cancel := context.WithTimeout(context.Background(), analyzeTimeout)
	defer cancel()

	// Канал для уведомления о завершении
	done := make(chan bool, 1)

	// ✅ ИСПРАВЛЕНО: Убрали дублирование проверки activeAnalysis
	// Проверка activeAnalysis уже сделана в OnMessage перед запуском горутины
	// Здесь только логируем начало работы

	startTime := time.Now()
	log.Printf("[FreeWill] analyzeAndAct: 🧠 === НАЧИНАЕМ ДВУХЭТАПНЫЙ АНАЛИЗ === чат:%d время:%v таймаут:%v",
		chatID, startTime.Format("15:04:05"), analyzeTimeout)

	defer func() {
		fws.mutex.Lock()
		// КРИТИЧЕСКИ ВАЖНО: Сбрасываем флаг активного анализа
		delete(fws.activeAnalysis, chatID)
		log.Printf("[FreeWill] analyzeAndAct: 🔓 Флаг activeAnalysis сброшен для чата %d", chatID)

		activationTime := time.Now()
		fws.lastActivation[chatID] = activationTime
		fws.mutex.Unlock()
		elapsed := time.Since(startTime)
		log.Printf("[FreeWill] analyzeAndAct: ✅ === ЗАВЕРШЕН АНАЛИЗ === чат:%d время_выполнения:%v активация_записана:%v",
			chatID, elapsed, activationTime.Format("15:04:05"))
	}()

	// Проверяем, что сервис включен
	if !fws.enabled {
		log.Printf("[FreeWill] analyzeAndAct: ❌ Free Will отключен для чата %d, прерываем анализ", chatID)
		return
	}

	// Запускаем основную логику в горутине с timeout
	go func() {
		defer func() {
			done <- true
		}()
		fws.performAnalysis(chatID)
	}()

	// Ждем завершения или таймаута
	select {
	case <-done:
		log.Printf("[FreeWill] analyzeAndAct: ✅ Анализ для чата %d завершен успешно", chatID)
	case <-ctx.Done():
		log.Printf("[FreeWill] analyzeAndAct: ⏰ ТАЙМАУТ анализа для чата %d после %v", chatID, analyzeTimeout)
	}
}

// performAnalysis выполняет основную логику анализа (без timeout logic)
func (fws *FreeWillService) performAnalysis(chatID int64) {
	// ЭТАП 1: Решение о необходимости ответа
	log.Printf("[FreeWill] performAnalysis: ЭТАП 1 - анализ необходимости ответа для чата %d", chatID)
	shouldReplyDecision, err := fws.decideShouldReply(chatID)
	if err != nil {
		log.Printf("[FreeWill] performAnalysis: Ошибка этапа 1 для чата %d: %v", chatID, err)
		return
	}

	if !shouldReplyDecision.ShouldReply {
		log.Printf("[FreeWill] performAnalysis: ЭТАП 1 - решено НЕ отвечать в чате %d (причина: %s)",
			chatID, shouldReplyDecision.Reason)
		return
	}

	log.Printf("[FreeWill] performAnalysis: ЭТАП 1 - решено отвечать в чате %d: type=%s, target_id=%d, причина: %s",
		chatID, shouldReplyDecision.ReplyType, shouldReplyDecision.TargetMessageID, shouldReplyDecision.Reason)

	// ЭТАП 2: Определение типа ответа с учетом voiceProbability
	log.Printf("[FreeWill] performAnalysis: ЭТАП 2 - определение типа ответа для чата %d", chatID)
	responseDecision, err := fws.decideResponseType(chatID, shouldReplyDecision)
	if err != nil {
		log.Printf("[FreeWill] performAnalysis: Ошибка этапа 2 для чата %d: %v", chatID, err)
		return
	}

	// Применяем вероятность голосовых сообщений
	if responseDecision.IsVoice {
		voiceRoll := fws.randSource.Float64()
		log.Printf("[FreeWill] performAnalysis: Проверка вероятности голосового сообщения для чата %d: %.3f <= %.3f",
			chatID, voiceRoll, fws.voiceProbability)
		if voiceRoll > fws.voiceProbability {
			log.Printf("[FreeWill] performAnalysis: Голосовое сообщение отклонено вероятностью: %.3f > %.3f",
				voiceRoll, fws.voiceProbability)
			responseDecision.IsVoice = false
		} else {
			log.Printf("[FreeWill] performAnalysis: Голосовое сообщение одобрено для чата %d", chatID)
		}
	}

	// Объединяем решения в финальное
	finalDecision := &FreeWillDecision{
		ShouldReply:     shouldReplyDecision.ShouldReply,
		ReplyType:       shouldReplyDecision.ReplyType,
		TargetMessageID: shouldReplyDecision.TargetMessageID,
		Reason:          shouldReplyDecision.Reason,
		Text:            responseDecision.Text,
		IsVoice:         responseDecision.IsVoice,
		Mood:            responseDecision.Mood,
	}

	log.Printf("[FreeWill] performAnalysis: ФИНАЛЬНОЕ решение для чата %d: type=%s, is_voice=%t, mood=%s",
		chatID, finalDecision.ReplyType, finalDecision.IsVoice, finalDecision.Mood)

	// === ИНТЕГРАЦИЯ КОГНИТИВНОЙ АРХИТЕКТУРЫ ===
	// Генерируем внутренний монолог о принятом решении
	if fws.bot.config.InternalMonologueEnabled {
		monologueContext := fmt.Sprintf("Принял решение отвечать: %s. Причина: %s", finalDecision.ReplyType, finalDecision.Reason)
		fws.bot.InternalMonologue(chatID, monologueContext, "free_will_decision")
	}

	// Выполняем действие
	fws.updateStats(chatID, finalDecision)

	fws.executeDecision(chatID, finalDecision)
}

// decideShouldReply - ЭТАП 1: решение о необходимости ответа
func (fws *FreeWillService) decideShouldReply(chatID int64) (*FreeWillShouldReplyDecision, error) {
	log.Printf("[FreeWill] decideShouldReply: Начинаем этап 1 для чата %d", chatID)

	// Получаем контекст чата
	context, err := fws.getContextForAnalysis(chatID)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения контекста: %w", err)
	}
	log.Printf("[FreeWill] decideShouldReply: Контекст получен для чата %d (длина: %d символов)", chatID, len(context))

	// Получаем текущее настроение
	mood := fws.getCurrentMood(chatID)
	log.Printf("[FreeWill] decideShouldReply: Текущее настроение для чата %d: %s (интенсивность: %.2f)",
		chatID, mood.CurrentMood, mood.MoodIntensity)

	// Формируем промпт для первого этапа
	prompt := fws.buildShouldReplyPrompt(context, mood)
	log.Printf("[FreeWill] decideShouldReply: Промпт этапа 1 сформирован для чата %d (длина: %d символов)",
		chatID, len(prompt))
	log.Printf("[FreeWill] decideShouldReply: === ПОЛНЫЙ ПРОМПТ ЭТАПА 1 ДЛЯ ЧАТА %d ===\n%s\n=== КОНЕЦ ПРОМПТА ЭТАПА 1 ===", chatID, prompt)

	// Отправляем запрос к LLM
	log.Printf("[FreeWill] decideShouldReply: Отправляем запрос к LLM для чата %d", chatID)
	llmStartTime := time.Now()
	response, err := fws.bot.llm.GenerateResponseByType(
		llm.ResponseTypeFreeWillShouldReply,
		prompt,
		context, // ИСПРАВЛЕНО: Передаем контекст вместо пустой строки
		float32(fws.bot.config.GeminiTemperatureNormal),
	)
	llmDuration := time.Since(llmStartTime)

	if err != nil {
		return nil, fmt.Errorf("ошибка генерации решения этапа 1: %w", err)
	}
	log.Printf("[FreeWill] decideShouldReply: Получен ответ от LLM для чата %d (время: %v, длина: %d символов)",
		chatID, llmDuration, len(response))
	log.Printf("[FreeWill] decideShouldReply: === ПОЛНЫЙ ОТВЕТ LLM ЭТАПА 1 ДЛЯ ЧАТА %d ===\n%s\n=== КОНЕЦ ОТВЕТА ЭТАПА 1 ===", chatID, response)

	// Парсим решение первого этапа
	decision, err := fws.parseShouldReplyDecision(response)
	if err != nil {
		log.Printf("[FreeWill] decideShouldReply: Сырой ответ для отладки: %s", response)
		return nil, fmt.Errorf("ошибка парсинга решения этапа 1: %w", err)
	}

	// Валидируем target_message_id если это direct_reply
	if decision.ReplyType == "direct_reply" && decision.TargetMessageID != 0 {
		err := fws.validateTargetMessageID(chatID, decision.TargetMessageID)
		if err != nil {
			log.Printf("[FreeWill] decideShouldReply: Невалидный target_message_id %d для чата %d: %v",
				decision.TargetMessageID, chatID, err)
			// Сбрасываем на general вместо ошибки
			// Мягко предпочтем context_based вместо general
			decision.ReplyType = "context_based"
			decision.TargetMessageID = 0
			decision.Reason = "Переключено на context_based из-за невалидного target_message_id"
			log.Printf("[FreeWill] decideShouldReply: Переключено на context_based для чата %d", chatID)
		}
	}

	// Чуть снизим «порог молчания» для общих/контекстных условий:
	// если LLM решил не отвечать, но в чате тишина близка к минимальному порогу —
	// мягко разрешим ответ как context_based.
	if !decision.ShouldReply && decision.TargetMessageID == 0 {
		// Безопасно читаем время последнего сообщения
		fws.mutex.RLock()
		lastMsgTime, ok := fws.lastMessage[chatID]
		fws.mutex.RUnlock()
		if ok && fws.silenceMinDuration > 0 {
			threshold := time.Duration(float64(fws.silenceMinDuration) * 0.8) // 80% от минимального порога
			since := time.Since(lastMsgTime)
			if since >= threshold && (fws.silenceMaxDuration == 0 || since <= fws.silenceMaxDuration) {
				decision.ShouldReply = true
				decision.ReplyType = "context_based"
				if decision.Reason != "" {
					decision.Reason = decision.Reason + "; смягчен порог молчания"
				} else {
					decision.Reason = "Смягчен порог молчания (context_based)"
				}
				log.Printf("[FreeWill] decideShouldReply: 📉 Смягчен порог молчания для чата %d: %v >= %v — переключаем на context_based",
					chatID, since, threshold)
			}
		}
	}

	// Мягкий приоритет на context_based/general, когда нет явной цели для direct_reply
	if decision.ShouldReply && decision.TargetMessageID == 0 {
		// Корректируем только если тип не задан/неподходящий под direct
		switch decision.ReplyType {
		case "", "ignore", "general", "direct_reply":
			roll := fws.randSource.Float64()
			if roll < 0.6 {
				decision.ReplyType = "context_based"
				log.Printf("[FreeWill] decideShouldReply: ✅ Мягкий приоритет: выбран context_based (roll=%.2f)", roll)
			} else {
				decision.ReplyType = "general"
				log.Printf("[FreeWill] decideShouldReply: ✅ Мягкий приоритет: оставлен general (roll=%.2f)", roll)
			}
		}
	}

	// Стандартизованная строка резюме решения Этапа 1
	log.Printf("[INFO][FreeWill] ShouldReplyDecision: chat=%d, should_reply=%t, reply_type=%s, target_id=%d, reason=%q",
		chatID, decision.ShouldReply, decision.ReplyType, decision.TargetMessageID, decision.Reason)

	return decision, nil
}

// decideResponseType - ЭТАП 2: определение типа ответа
func (fws *FreeWillService) decideResponseType(chatID int64, shouldReplyDecision *FreeWillShouldReplyDecision) (*FreeWillResponseTypeDecision, error) {
	log.Printf("[FreeWill] decideResponseType: Начинаем этап 2 для чата %d (reply_type: %s)", chatID, shouldReplyDecision.ReplyType)

	// Получаем контекст для второго этапа (может быть другой в зависимости от типа)
	context, err := fws.getContextForResponseType(chatID, shouldReplyDecision)
	if err != nil {
		log.Printf("[FreeWill] decideResponseType: ОШИБКА получения контекста для чата %d: %v", chatID, err)
		return nil, fmt.Errorf("ошибка получения контекста для этапа 2: %w", err)
	}

	log.Printf("[FreeWill] decideResponseType: Контекст получен для чата %d (длина: %d символов): %.200s...", chatID, len(context), context)

	// Формируем промпт для второго этапа
	prompt := fws.buildResponseTypePrompt(shouldReplyDecision.ReplyType)
	log.Printf("[FreeWill] decideResponseType: Промпт этапа 2 сформирован для чата %d (длина: %d символов)",
		chatID, len(prompt))
	log.Printf("[FreeWill] decideResponseType: === ПОЛНЫЙ ПРОМПТ ЭТАПА 2 ДЛЯ ЧАТА %d ===\n%s\n=== КОНЕЦ ПРОМПТА ЭТАПА 2 ===", chatID, prompt)

	// Отправляем запрос к LLM
	log.Printf("[FreeWill] decideResponseType: Отправляем запрос к LLM для чата %d", chatID)
	llmStartTime := time.Now()
	response, err := fws.bot.llm.GenerateResponseByType(
		llm.ResponseTypeFreeWillResponseType,
		prompt,
		context,
		float32(fws.bot.config.GeminiTemperatureNormal),
	)
	llmDuration := time.Since(llmStartTime)

	if err != nil {
		return nil, fmt.Errorf("ошибка генерации решения этапа 2: %w", err)
	}
	log.Printf("[FreeWill] decideResponseType: Получен ответ от LLM для чата %d (время: %v, длина: %d символов)",
		chatID, llmDuration, len(response))
	log.Printf("[FreeWill] decideResponseType: === ПОЛНЫЙ ОТВЕТ LLM ЭТАПА 2 ДЛЯ ЧАТА %d ===\n%s\n=== КОНЕЦ ОТВЕТА ЭТАПА 2 ===", chatID, response)

	// Парсим решение второго этапа
	decision, err := fws.parseResponseTypeDecision(response)
	if err != nil {
		log.Printf("[FreeWill] decideResponseType: Сырой ответ для отладки: %s", response)
		return nil, fmt.Errorf("ошибка парсинга решения этапа 2: %w", err)
	}

	log.Printf("[INFO][FreeWill] ResponseTypeDecision: chat=%d, is_voice=%t, mood=%s, text_len=%d",
		chatID, decision.IsVoice, decision.Mood, len(decision.Text))

	return decision, nil
}

// parseShouldReplyDecision парсит ответ первого этапа
func (fws *FreeWillService) parseShouldReplyDecision(response string) (*FreeWillShouldReplyDecision, error) {
	response = strings.TrimSpace(response)
	response = cleanJSONFromMarkdown(response)

	startIdx := strings.Index(response, "{")
	endIdx := strings.LastIndex(response, "}")

	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		return nil, fmt.Errorf("JSON не найден в ответе этапа 1: %s", response)
	}

	jsonStr := response[startIdx : endIdx+1]

	var decision FreeWillShouldReplyDecision
	if err := json.Unmarshal([]byte(jsonStr), &decision); err != nil {
		// FALLBACK: Пробуем парсить как generic map для извлечения значений с неправильными полями
		log.Printf("[FreeWill] parseShouldReplyDecision: Обычный парсинг не удался, пробуем fallback: %v", err)

		var rawJSON map[string]interface{}
		if err2 := json.Unmarshal([]byte(jsonStr), &rawJSON); err2 != nil {
			return nil, fmt.Errorf("ошибка парсинга JSON этапа 1: %w (оригинал: %v)", err2, err)
		}

		// Инициализируем с дефолтными значениями
		decision = FreeWillShouldReplyDecision{}

		// Пробуем извлечь should_reply из различных полей
		if val, ok := rawJSON["should_reply"]; ok {
			if b, ok := val.(bool); ok {
				decision.ShouldReply = b
			}
		} else if val, ok := rawJSON["should_respond"]; ok { // FALLBACK для неправильного поля
			if b, ok := val.(bool); ok {
				decision.ShouldReply = b
			}
		} else if val, ok := rawJSON["decision"]; ok { // FALLBACK для поля "decision" (часто возвращается LLM)
			if s, ok := val.(string); ok {
				// Конвертируем строковые значения в bool
				switch strings.ToLower(s) {
				case "reply", "respond", "yes", "true":
					decision.ShouldReply = true
				case "no_reply", "no_respond", "no", "false":
					decision.ShouldReply = false
				}
				log.Printf("[FreeWill] parseShouldReplyDecision: Конвертируем decision '%s' в should_reply=%t", s, decision.ShouldReply)
			}
		}

		// Пробуем извлечь reply_type
		if val, ok := rawJSON["reply_type"]; ok {
			if s, ok := val.(string); ok {
				decision.ReplyType = s
			}
		} else if val, ok := rawJSON["response_type"]; ok { // FALLBACK для неправильного поля
			if s, ok := val.(string); ok {
				decision.ReplyType = s
			}
		} else {
			// Если reply_type не найден, устанавливаем дефолтное значение на основе других данных
			if decision.TargetMessageID > 0 {
				decision.ReplyType = "direct_reply"
				log.Printf("[FreeWill] parseShouldReplyDecision: reply_type не найден, но есть target_message_id, устанавливаем 'direct_reply'")
			} else {
				decision.ReplyType = "general"
				log.Printf("[FreeWill] parseShouldReplyDecision: reply_type не найден, устанавливаем 'general'")
			}
		}

		// Пробуем извлечь target_message_id
		if val, ok := rawJSON["target_message_id"]; ok {
			if f, ok := val.(float64); ok {
				decision.TargetMessageID = int(f)
			}
		}

		// Пробуем извлечь reason
		if val, ok := rawJSON["reason"]; ok {
			if s, ok := val.(string); ok {
				decision.Reason = s
			}
		} else if val, ok := rawJSON["response"]; ok { // FALLBACK: если LLM поместил текст в "response"
			if s, ok := val.(string); ok {
				decision.Reason = "LLM дал готовый ответ: " + s
				log.Printf("[FreeWill] parseShouldReplyDecision: LLM поместил текст ответа в поле 'response', используем как reason")
			}
		} else if val, ok := rawJSON["text"]; ok { // FALLBACK: если LLM поместил готовый текст (частая ошибка)
			if s, ok := val.(string); ok {
				decision.Reason = "LLM дал готовый ответ: " + s
				log.Printf("[FreeWill] parseShouldReplyDecision: LLM поместил готовый текст в поле 'text', используем как reason")
			}
		}

		log.Printf("[FreeWill] parseShouldReplyDecision: Fallback извлечение данных: should_reply=%t, reply_type=%s, target=%d, reason=%s",
			decision.ShouldReply, decision.ReplyType, decision.TargetMessageID, decision.Reason)
	}

	return &decision, nil
}

// parseResponseTypeDecision парсит ответ второго этапа
func (fws *FreeWillService) parseResponseTypeDecision(response string) (*FreeWillResponseTypeDecision, error) {
	response = strings.TrimSpace(response)
	response = cleanJSONFromMarkdown(response)

	startIdx := strings.Index(response, "{")
	endIdx := strings.LastIndex(response, "}")

	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		return nil, fmt.Errorf("JSON не найден в ответе этапа 2: %s", response)
	}

	jsonStr := response[startIdx : endIdx+1]

	var decision FreeWillResponseTypeDecision
	if err := json.Unmarshal([]byte(jsonStr), &decision); err != nil {
		return nil, fmt.Errorf("ошибка парсинга JSON этапа 2: %w", err)
	}

	return &decision, nil
}

// executeDecision выполняет принятое решение
func (fws *FreeWillService) executeDecision(chatID int64, decision *FreeWillDecision) {
	log.Printf("[INFO][FreeWill] FinalDecision: chat=%d, reply_type=%s, is_voice=%t, mood=%s, reason=%q",
		chatID, decision.ReplyType, decision.IsVoice, decision.Mood, decision.Reason)

	executionStart := time.Now()
	switch decision.ReplyType {
	case "direct_reply":
		log.Printf("[FreeWill] executeDecision: Отправляем прямой ответ для чата %d (target_message_id: %d)",
			chatID, decision.TargetMessageID)
		fws.sendDirectReply(chatID, decision)
	case "general":
		log.Printf("[FreeWill] executeDecision: Отправляем общее сообщение для чата %d", chatID)
		fws.sendGeneralMessage(chatID, decision)
	case "context_based":
		log.Printf("[FreeWill] executeDecision: Отправляем контекстное сообщение для чата %d", chatID)
		fws.sendContextBasedMessage(chatID, decision)
	case "silence_response":
		log.Printf("[FreeWill] executeDecision: Отправляем ответ на тишину для чата %d", chatID)
		fws.sendSilenceResponse(chatID, decision)
	case "mood_based":
		log.Printf("[FreeWill] executeDecision: Отправляем сообщение на основе настроения для чата %d", chatID)
		fws.sendMoodBasedMessage(chatID, decision)
	case "voice":
		log.Printf("[FreeWill] executeDecision: Отправляем голосовое сообщение для чата %d", chatID)
		fws.sendVoiceMessage(chatID, decision)
	case "take_response":
		log.Printf("[FreeWill] executeDecision: Отправляем ответ на тейк для чата %d", chatID)
		fws.sendTakeResponse(chatID, decision)
	default:
		log.Printf("[FreeWill] executeDecision: Неизвестный тип ответа для чата %d: %s", chatID, decision.ReplyType)
	}
	log.Printf("[FreeWill] executeDecision: Завершено выполнение решения для чата %d (время: %v)",
		chatID, time.Since(executionStart))
}

// sendDirectReply отправляет ответ на конкретное сообщение
func (fws *FreeWillService) sendDirectReply(chatID int64, decision *FreeWillDecision) {
	log.Printf("[FreeWill] sendDirectReply: Начинаем отправку прямого ответа для чата %d", chatID)

	if decision.TargetMessageID == 0 {
		log.Printf("[FreeWill] sendDirectReply: Нет target_message_id для прямого ответа в чате %d", chatID)
		return
	}

	// Получаем информацию о целевом сообщении для лучшего логирования
	targetMessageInfo := fws.getTargetMessageInfo(chatID, decision.TargetMessageID)
	log.Printf("[FreeWill] sendDirectReply: Целевое сообщение для ответа: %d (%s)", decision.TargetMessageID, targetMessageInfo)

	// Получаем полный контекст с цепочкой ответов для прямого ответа
	log.Printf("[FreeWill] sendDirectReply: Получаем контекст для прямого ответа в чате %d", chatID)
	context, err := fws.getDirectReplyContext(chatID, decision.TargetMessageID)
	if err != nil {
		log.Printf("[FreeWill] sendDirectReply: Ошибка получения контекста для direct reply в чате %d: %v", chatID, err)
		return
	}
	log.Printf("[FreeWill] sendDirectReply: Контекст получен для чата %d (длина: %d символов)", chatID, len(context))

	// Детерминированная подсказка стиля на основе отношений с автором исходного сообщения
	// Пытаемся получить исходное сообщение из хранилища, чтобы определить userID автора
	targetMsg, _ := fws.bot.storage.GetMessageByID(chatID, decision.TargetMessageID)
	if targetMsg != nil && targetMsg.From != nil {
		uid := int64(targetMsg.From.ID)
		before := len(context)
		context = fws.bot.ApplyRelationshipStyleToContext(chatID, uid, context)
		if len(context) > before {
			style := fws.bot.GetRelationshipInfluencedCommunicationStyle(chatID, uid)
			log.Printf("[INFO][FW-DR] Chat %d: Применен стиль общения на основе отношений (user %d): %s", chatID, uid, style)
		}
	} else {
		// Безопасный фолбэк: добавляем нейтральный подсказчик для детерминизма
		before := len(context)
		context = fws.bot.ApplyRelationshipStyleToContext(chatID, 0, context)
		if len(context) > before {
			log.Printf("[INFO][FW-DR] Chat %d: Применен стиль общения на основе отношений (fallback, без userID)", chatID)
		}
	}

	// Если analyzeDirectResponse уже сгенерировал текст (Этап 2) — используем его,
	// иначе делаем безопасный фолбэк с генерацией на месте.
	var text string
	if strings.TrimSpace(decision.Text) != "" {
		log.Printf("[FreeWill] sendDirectReply: Используем уже сгенерированный текст из ЭТАПА 2 для чата %d", chatID)
		text = decision.Text
	} else {
		// Генерируем текст ответа с учетом контекста (фолбэк)
		log.Printf("[FreeWill] sendDirectReply: Текст отсутствует в решении — выполняем локальную генерацию. Формируем промпт для чата %d", chatID)
		// Строим промпт и обогащаем личностью для консистентного тона
		prompt := fws.buildDirectReplyPrompt(chatID, decision)
		prompt = fws.bot.enrichPromptWithPersonality(prompt, chatID, "free_will_direct")
		log.Printf("[FreeWill] sendDirectReply: Промпт (enriched) сформирован для чата %d (длина: %d символов)", chatID, len(prompt))

		// Добавляем веб-поиск если включен
		fullContext := context
		if fws.bot.webSearch != nil && fws.bot.webSearch.IsEnabled() {
			log.Printf("[FreeWill] sendDirectReply: Smart веб-поиск для чата %d", chatID)
			// Пытаемся использовать целевое сообщение как основной запрос, затем — уже готовый текст (если есть)
			queryCandidate := ""
			if decision.Text != "" {
				queryCandidate = decision.Text
			}
			enhanced := fws.bot.webSearch.EnhanceContextWithSmartWebSearch(fullContext, queryCandidate)
			if enhanced != fullContext {
				fullContext = enhanced
				log.Printf("[FreeWill] sendDirectReply: Контекст расширен smart веб-поиском для direct reply в чате %d", chatID)
			} else {
				log.Printf("[FreeWill] sendDirectReply: Smart веб-поиск не потребовался для чата %d", chatID)
			}
		}

		log.Printf("[FreeWill] sendDirectReply: Генерируем текст ответа (fallback) для чата %d", chatID)
		textGenStart := time.Now()
		genText, err := fws.bot.llm.GenerateResponseByType(
			llm.ResponseTypeFreeWillDirect,
			prompt,
			fullContext,
			float32(fws.bot.config.GeminiTemperatureNormal),
		)
		textGenDuration := time.Since(textGenStart)
		if err != nil {
			log.Printf("[FreeWill] sendDirectReply: Ошибка генерации прямого ответа (fallback) для чата %d: %v", chatID, err)
			return
		}
		log.Printf("[FreeWill] sendDirectReply: Текст ответа (fallback) сгенерирован для чата %d (время: %v, длина: %d символов)",
			chatID, textGenDuration, len(genText))
		text = genText
	}

	originalText := text
	log.Printf("🧹 [FREE_WILL] Очистка DirectReply ответа для чата %d (исходная длина: %d)", chatID, len(text))
	text = cleanupLLMResponse(text)
	if originalText != text {
		log.Printf("[FreeWill] sendDirectReply: Текст очищен от служебных символов для чата %d", chatID)
	}
	log.Printf("[FreeWill] sendDirectReply: Финальный текст для чата %d: %s", chatID, text)

	if decision.IsVoice {
		log.Printf("[FreeWill] sendDirectReply: Отправляем голосовой ответ для чата %d", chatID)
		fws.sendVoiceReply(chatID, decision.TargetMessageID, text)
	} else {
		log.Printf("[FreeWill] sendDirectReply: Отправляем текстовый ответ для чата %d", chatID)
		// Используем обертку с анти-повторениями (userID 0 для Free Will)
		fws.bot.sendReplyToWithAntiRepetition(chatID, decision.TargetMessageID, text, 0, "free_will_direct")
	}

	log.Printf("[FreeWill] sendDirectReply: Завершена отправка прямого ответа для чата %d (reply to %d)",
		chatID, decision.TargetMessageID)
}

// sendGeneralMessage отправляет общее сообщение в чат (используя CONTEXT_WINDOW)
func (fws *FreeWillService) sendGeneralMessage(chatID int64, decision *FreeWillDecision) {
	log.Printf("[FreeWill] sendGeneralMessage: Начинаем отправку общего сообщения для чата %d", chatID)

	// Получаем контекст для general сообщений (CONTEXT_WINDOW)
	log.Printf("[FreeWill] sendGeneralMessage: Получаем общий контекст для чата %d", chatID)
	context, err := fws.getGeneralContext(chatID)
	if err != nil {
		log.Printf("[FreeWill] sendGeneralMessage: Ошибка получения контекста для general в чате %d: %v", chatID, err)
		return
	}
	log.Printf("[FreeWill] sendGeneralMessage: Контекст получен для чата %d (длина: %d символов)", chatID, len(context))

	// Подмешиваем лёгкий ассоциативный контекст (зафичено флагом)
	if assoc := fws.bot.getAssociativeContext(chatID, fws.bot.buildAssociativeKeys(chatID), 3); assoc != "" {
		context = assoc + "\n\n" + context
	}

	log.Printf("[FreeWill] sendGeneralMessage: Формируем промпт для чата %d", chatID)
	prompt := fws.buildGeneralPrompt(chatID, decision)
	log.Printf("[FreeWill] sendGeneralMessage: Промпт сформирован для чата %d (длина: %d символов)", chatID, len(prompt))

	// Добавляем веб-поиск если включен
	fullContext := context
	if fws.bot.webSearch != nil && fws.bot.webSearch.IsEnabled() {
		log.Printf("[FreeWill] sendGeneralMessage: Smart веб-поиск для чата %d", chatID)
		if len(fullContext) > 0 {
			enhanced := fws.bot.webSearch.EnhanceContextWithSmartWebSearch(fullContext, "актуальные события")
			if enhanced != fullContext {
				fullContext = enhanced
				log.Printf("[FreeWill] sendGeneralMessage: Контекст расширен smart веб-поиском для general в чате %d", chatID)
			} else {
				log.Printf("[FreeWill] sendGeneralMessage: Smart веб-поиск не потребовался для чата %d", chatID)
			}
		}
	}

	log.Printf("[FreeWill] sendGeneralMessage: Генерируем текст общего сообщения для чата %d", chatID)
	textGenStart := time.Now()
	text, err := fws.bot.llm.GenerateResponseByType(
		llm.ResponseTypeFreeWillGeneral,
		prompt,
		fullContext,
		float32(fws.bot.config.GeminiTemperatureNormal),
	)
	textGenDuration := time.Since(textGenStart)

	if err != nil {
		log.Printf("[FreeWill] sendGeneralMessage: Ошибка генерации общего сообщения для чата %d: %v", chatID, err)
		return
	}
	log.Printf("[FreeWill] sendGeneralMessage: Текст сгенерирован для чата %d (время: %v, длина: %d символов)",
		chatID, textGenDuration, len(text))

	originalText := text
	log.Printf("🧹 [FREE_WILL] Очистка GeneralMessage ответа для чата %d (исходная длина: %d)", chatID, len(text))
	text = cleanupLLMResponse(text)
	if originalText != text {
		log.Printf("[FreeWill] sendGeneralMessage: Текст очищен от служебных символов для чата %d", chatID)
	}
	log.Printf("[FreeWill] sendGeneralMessage: Финальный текст для чата %d: %s", chatID, text)

	if decision.IsVoice {
		log.Printf("[FreeWill] sendGeneralMessage: Отправляем голосовое общее сообщение для чата %d", chatID)
		fws.sendVoiceWithText(chatID, text)
	} else {
		log.Printf("[FreeWill] sendGeneralMessage: Отправляем текстовое общее сообщение для чата %d", chatID)
		// Используем обертку с анти-повторениями (userID 0 для Free Will general)
		fws.bot.sendReplyWithAntiRepetition(chatID, text, 0, "free_will_general")
	}

	log.Printf("[FreeWill] sendGeneralMessage: Завершена отправка общего сообщения для чата %d", chatID)
}

// sendContextBasedMessage отправляет сообщение на основе контекста (используя FREE_WILL_CONTEXT_WINDOW)
func (fws *FreeWillService) sendContextBasedMessage(chatID int64, decision *FreeWillDecision) {
	// Получаем расширенный контекст для context-based сообщений (FREE_WILL_CONTEXT_WINDOW)
	context, err := fws.getContextBasedContext(chatID)
	if err != nil {
		log.Printf("[FreeWill] Ошибка получения контекста для context-based: %v", err)
		return
	}

	// Подмешиваем лёгкий ассоциативный контекст (зафичено флагом)
	if assoc := fws.bot.getAssociativeContext(chatID, fws.bot.buildAssociativeKeys(chatID), 3); assoc != "" {
		context = assoc + "\n\n" + context
	}

	prompt := fws.buildContextPrompt(chatID, decision)

	// Добавляем веб-поиск если включен
	fullContext := context
	if fws.bot.webSearch != nil && fws.bot.webSearch.IsEnabled() {
		if len(fullContext) > 0 {
			enhanced := fws.bot.webSearch.EnhanceContextWithSmartWebSearch(fullContext, decision.Text)
			if enhanced != fullContext {
				fullContext = enhanced
				log.Printf("[FreeWill] Контекст расширен smart веб-поиском для context-based")
			}
		}
	}

	text, err := fws.bot.llm.GenerateResponseByType(
		llm.ResponseTypeFreeWillContext,
		prompt,
		fullContext,
		float32(fws.bot.config.GeminiTemperatureNormal),
	)
	if err != nil {
		log.Printf("[FreeWill] Ошибка генерации контекстного сообщения: %v", err)
		return
	}

	log.Printf("🧹 [FREE_WILL] Очистка ContextBased ответа для чата %d (исходная длина: %d)", chatID, len(text))
	text = cleanupLLMResponse(text)

	if decision.IsVoice {
		fws.sendVoiceWithText(chatID, text)
	} else {
		// Используем обертку с анти-повторениями
		fws.bot.sendReplyWithAntiRepetition(chatID, text, 0, "free_will_context")
	}

	log.Printf("[FreeWill] Отправлено контекстное сообщение в чат %d", chatID)
}

// sendSilenceResponse отправляет сообщение для оживления тишины
func (fws *FreeWillService) sendSilenceResponse(chatID int64, decision *FreeWillDecision) {
	// Для ответа на тишину используем общий контекст
	context, err := fws.getGeneralContext(chatID)
	if err != nil {
		log.Printf("[FreeWill] Ошибка получения контекста для silence: %v", err)
		return
	}

	// Подмешиваем лёгкий ассоциативный контекст (зафичено флагом)
	if assoc := fws.bot.getAssociativeContext(chatID, fws.bot.buildAssociativeKeys(chatID), 3); assoc != "" {
		context = assoc + "\n\n" + context
	}

	prompt := fws.buildSilencePrompt(chatID, decision)

	// Smart веб‑поиск: используем последние темы как кандидат
	fullContext := context
	if fws.bot.webSearch != nil && fws.bot.webSearch.IsEnabled() {
		fullContext = fws.bot.webSearch.EnhanceContextWithSmartWebSearch(fullContext, "актуальные события")
	}

	text, err := fws.bot.llm.GenerateResponseByType(
		llm.ResponseTypeFreeWillSilence,
		prompt,
		fullContext,
		float32(fws.bot.config.GeminiTemperatureNormal),
	)
	if err != nil {
		log.Printf("[FreeWill] Ошибка генерации ответа на тишину: %v", err)
		return
	}

	log.Printf("🧹 [FREE_WILL] Очистка Silence ответа для чата %d (исходная длина: %d)", chatID, len(text))
	text = cleanupLLMResponse(text)

	if decision.IsVoice {
		fws.sendVoiceWithText(chatID, text)
	} else {
		// Используем обертку с анти-повторениями
		fws.bot.sendReplyWithAntiRepetition(chatID, text, 0, "free_will_silence")
	}

	log.Printf("[FreeWill] Отправлен ответ на тишину в чат %d", chatID)
}

// sendTakeResponse отправляет развернутый ответ на тейк
func (fws *FreeWillService) sendTakeResponse(chatID int64, decision *FreeWillDecision) {
	log.Printf("[FreeWill] sendTakeResponse: Начинаем отправку ответа на тейк для чата %d", chatID)

	if decision.TargetMessageID == 0 {
		log.Printf("[FreeWill] sendTakeResponse: Нет target_message_id для ответа на тейк в чате %d", chatID)
		return
	}

	// Получаем информацию о целевом сообщении
	targetMessageInfo := fws.getTargetMessageInfo(chatID, decision.TargetMessageID)
	log.Printf("[FreeWill] sendTakeResponse: Целевое сообщение (тейк): %d (%s)", decision.TargetMessageID, targetMessageInfo)

	// Получаем контекст с цепочкой ответов
	log.Printf("[FreeWill] sendTakeResponse: Получаем контекст для ответа на тейк в чате %d", chatID)
	context, err := fws.getDirectReplyContext(chatID, decision.TargetMessageID)
	if err != nil {
		log.Printf("[FreeWill] sendTakeResponse: Ошибка получения контекста для take response в чате %d: %v", chatID, err)
		return
	}

	// Подмешиваем лёгкий ассоциативный контекст (зафичено флагом)
	if assoc := fws.bot.getAssociativeContext(chatID, fws.bot.buildAssociativeKeys(chatID), 3); assoc != "" {
		context = assoc + "\n\n" + context
	}

	// Строим промпт для ответа на тейк
	log.Printf("[FreeWill] sendTakeResponse: Формируем промпт для ответа на тейк в чате %d", chatID)
	prompt := fws.buildTakeResponsePrompt(chatID, decision)
	log.Printf("[FreeWill] sendTakeResponse: Промпт сформирован для чата %d (длина: %d символов)", chatID, len(prompt))

	// Добавляем веб-поиск для поддержки аргументов
	fullContext := context
	if fws.bot.webSearch != nil && fws.bot.webSearch.IsEnabled() {
		log.Printf("[FreeWill] sendTakeResponse: Smart веб-поиск для поддержки аргументов в чате %d", chatID)
		enhanced := fws.bot.webSearch.EnhanceContextWithSmartWebSearch(fullContext, decision.Text)
		if enhanced != fullContext {
			fullContext = enhanced
			log.Printf("[FreeWill] sendTakeResponse: Контекст расширен smart веб-поиском для take response в чате %d", chatID)
		}
	}

	log.Printf("[FreeWill] sendTakeResponse: Генерируем развернутый ответ на тейк для чата %d", chatID)
	textGenStart := time.Now()
	text, err := fws.bot.llm.GenerateResponseByType(
		llm.ResponseTypeFreeWillTakeResponse,
		prompt,
		fullContext,
		float32(fws.bot.config.GeminiTemperatureNormal),
	)
	textGenDuration := time.Since(textGenStart)

	if err != nil {
		log.Printf("[FreeWill] sendTakeResponse: Ошибка генерации ответа на тейк для чата %d: %v", chatID, err)
		return
	}
	log.Printf("[FreeWill] sendTakeResponse: Текст ответа на тейк сгенерирован для чата %d (время: %v, длина: %d символов)",
		chatID, textGenDuration, len(text))

	// Очищаем ответ
	originalText := text
	text = cleanupLLMResponse(text)
	if originalText != text {
		log.Printf("[FreeWill] sendTakeResponse: Текст очищен от служебных символов для чата %d", chatID)
	}
	log.Printf("[FreeWill] sendTakeResponse: Финальный ответ на тейк для чата %d: %s", chatID, text)

	if decision.IsVoice {
		log.Printf("[FreeWill] sendTakeResponse: Отправляем голосовой ответ на тейк для чата %d", chatID)
		fws.sendVoiceReply(chatID, decision.TargetMessageID, text)
	} else {
		log.Printf("[FreeWill] sendTakeResponse: Отправляем текстовый ответ на тейк для чата %d", chatID)
		// Используем обертку с анти-повторениями для ответов на тейки
		fws.bot.sendReplyToWithAntiRepetition(chatID, decision.TargetMessageID, text, 0, "free_will_take_response")
	}

	log.Printf("[FreeWill] sendTakeResponse: Завершена отправка ответа на тейк для чата %d (reply to %d)",
		chatID, decision.TargetMessageID)
}

// sendMoodBasedMessage отправляет сообщение на основе настроения
func (fws *FreeWillService) sendMoodBasedMessage(chatID int64, decision *FreeWillDecision) {
	// Для mood-based сообщений используем общий контекст
	context, err := fws.getGeneralContext(chatID)
	if err != nil {
		log.Printf("[FreeWill] Ошибка получения контекста для mood-based: %v", err)
		return
	}

	// Подмешиваем лёгкий ассоциативный контекст (зафичено флагом)
	if assoc := fws.bot.getAssociativeContext(chatID, fws.bot.buildAssociativeKeys(chatID), 3); assoc != "" {
		context = assoc + "\n\n" + context
	}

	prompt := fws.buildMoodBasedPrompt(chatID, decision)

	text, err := fws.bot.llm.GenerateResponseByType(
		llm.ResponseTypeFreeWillMoodBasedMessage,
		prompt,
		context,
		float32(fws.bot.config.GeminiTemperatureNormal),
	)
	if err != nil {
		log.Printf("[FreeWill] Ошибка генерации сообщения по настроению: %v", err)
		return
	}

	text = cleanupLLMResponse(text)

	if decision.IsVoice {
		fws.sendVoiceWithText(chatID, text)
	} else {
		// Используем обертку с анти-повторениями
		fws.bot.sendReplyWithAntiRepetition(chatID, text, 0, "free_will_mood")
	}

	log.Printf("[FreeWill] Отправлено сообщение по настроению в чат %d", chatID)
}

// sendVoiceMessage отправляет голосовое сообщение через Free Will
func (fws *FreeWillService) sendVoiceMessage(chatID int64, decision *FreeWillDecision) {
	// Для голосовых сообщений используем общий контекст
	context, err := fws.getGeneralContext(chatID)
	if err != nil {
		log.Printf("[FreeWill] Ошибка получения контекста для voice: %v", err)
		return
	}

	// Подмешиваем лёгкий ассоциативный контекст (зафичено флагом)
	if assoc := fws.bot.getAssociativeContext(chatID, nil, 3); assoc != "" {
		context = assoc + "\n\n" + context
	}

	prompt := fws.buildVoicePrompt(chatID, decision)

	text, err := fws.bot.llm.GenerateResponseByType(
		llm.ResponseTypeFreeWillVoiceMessage,
		prompt,
		context,
		float32(fws.bot.config.GeminiTemperatureNormal),
	)
	if err != nil {
		log.Printf("[FreeWill] Ошибка генерации текста для голоса: %v", err)
		return
	}

	text = cleanupLLMResponse(text)
	fws.sendVoiceWithText(chatID, text)

	log.Printf("[FreeWill] Инициировано голосовое сообщение в чат %d", chatID)
}

// sendVoiceWithText отправляет голосовое сообщение с заданным текстом
func (fws *FreeWillService) sendVoiceWithText(chatID int64, text string) {
	log.Printf("[FreeWill] sendVoiceWithText: Отправляем голосовое сообщение для чата %d (текст: %s)", chatID, text)

	if fws.bot.voiceMessageService == nil {
		log.Printf("[FreeWill] sendVoiceWithText: VoiceMessageService недоступен для чата %d", chatID)
		return
	}

	log.Printf("[FreeWill] sendVoiceWithText: Запускаем генерацию голосового сообщения для чата %d", chatID)
	// Используем специальный метод для Free Will, который не зависит от VOICE_MESSAGES_ENABLED
	go fws.bot.voiceMessageService.generateAndSendVoiceMessageForFreeWill(chatID, text)
}

// sendVoiceReply отправляет голосовое сообщение как ответ на другое сообщение
func (fws *FreeWillService) sendVoiceReply(chatID int64, replyToMessageID int, text string) {
	log.Printf("[FreeWill] sendVoiceReply: Отправляем голосовой ответ для чата %d (reply to %d, текст: %s)",
		chatID, replyToMessageID, text)

	if fws.bot.voiceMessageService == nil {
		log.Printf("[FreeWill] sendVoiceReply: VoiceMessageService недоступен для чата %d", chatID)
		return
	}

	log.Printf("[FreeWill] sendVoiceReply: Запускаем генерацию голосового ответа для чата %d", chatID)
	// Используем специальный метод для Free Will с reply, который не зависит от VOICE_MESSAGES_ENABLED
	go fws.bot.voiceMessageService.generateAndSendVoiceMessageReplyForFreeWill(chatID, text, replyToMessageID)
}

// updateMood обновляет настроение бота на основе анализа контекста
func (fws *FreeWillService) updateMood(chatID int64, context string) {
	log.Printf("[FreeWill] updateMood: Начинаем обновление настроения для чата %d", chatID)

	moodRoll := fws.randSource.Float64()
	log.Printf("[FreeWill] updateMood: Проверка вероятности обновления настроения для чата %d: %.3f > %.3f",
		chatID, moodRoll, fws.moodUpdateProbability)

	if moodRoll > fws.moodUpdateProbability {
		log.Printf("[FreeWill] updateMood: Обновление настроения пропущено для чата %d (не прошла проверка вероятности)", chatID)
		return // Не каждый раз обновляем настроение
	}

	log.Printf("[FreeWill] updateMood: Приступаем к анализу настроения для чата %d", chatID)

	// Анализируем настроение через LLM
	prompt := fws.bot.enrichPromptWithPersonality(fws.bot.config.FreeWillMoodAnalysisPrompt, chatID, "free_will_mood_analysis")
	fullPrompt := fmt.Sprintf("%s\n\nКонтекст чата:\n%s", prompt, context)
	log.Printf("[FreeWill] updateMood: Промпт сформирован для чата %d (длина: %d символов)", chatID, len(fullPrompt))

	log.Printf("[FreeWill] updateMood: Отправляем запрос к LLM для анализа настроения чата %d", chatID)
	moodAnalysisStart := time.Now()
	response, err := fws.bot.llm.GenerateResponseByType(llm.ResponseTypeFreeWillMoodAnalysis, fullPrompt, "", float32(fws.bot.config.GeminiTemperatureNormal))
	moodAnalysisDuration := time.Since(moodAnalysisStart)

	if err != nil {
		log.Printf("[FreeWill] updateMood: Ошибка анализа настроения для чата %d: %v", chatID, err)
		return
	}
	log.Printf("[FreeWill] updateMood: Получен ответ LLM для анализа настроения чата %d (время: %v, длина: %d символов)",
		chatID, moodAnalysisDuration, len(response))
	log.Printf("[FreeWill] updateMood: Ответ LLM для настроения чата %d: %s", chatID, response)

	// Очищаем ответ от markdown code blocks и backticks перед парсингом JSON
	response = cleanJSONFromMarkdown(response)
	log.Printf("[FreeWill] updateMood: Очищенный ответ для парсинга: %s", response)

	// Парсим JSON ответ
	log.Printf("[FreeWill] updateMood: Парсим JSON ответ для настроения чата %d", chatID)
	var moodData struct {
		CurrentMood   string  `json:"current_mood"`
		MoodIntensity float64 `json:"mood_intensity"`
		TriggerReason string  `json:"trigger_reason"`
	}

	if err := json.Unmarshal([]byte(response), &moodData); err != nil {
		log.Printf("[FreeWill] updateMood: Ошибка парсинга JSON настроения для чата %d: %v", chatID, err)
		log.Printf("[FreeWill] updateMood: Сырой ответ для отладки: %s", response)
		return
	}
	log.Printf("[FreeWill] updateMood: JSON настроения распарсен для чата %d: mood=%s, intensity=%.2f, reason=%s",
		chatID, moodData.CurrentMood, moodData.MoodIntensity, moodData.TriggerReason)

	// Получаем текущее настроение для сравнения
	currentMood := fws.getMood(chatID)
	log.Printf("[FreeWill] updateMood: Текущее настроение для чата %d: %s (интенсивность: %.2f)",
		chatID, currentMood.CurrentMood, currentMood.MoodIntensity)

	// Создаем новое состояние настроения
	newMoodState := &storage.MoodState{
		ChatID:         chatID,
		CurrentMood:    moodData.CurrentMood,
		MoodIntensity:  moodData.MoodIntensity,
		LastMoodUpdate: time.Now(),
		TriggerReason:  moodData.TriggerReason,
		UpdatedAt:      time.Now(),
	}

	// Сохраняем в БД
	log.Printf("[FreeWill] updateMood: Сохраняем новое настроение в БД для чата %d", chatID)
	err = fws.bot.storage.SaveMoodState(newMoodState)
	if err != nil {
		log.Printf("[FreeWill] updateMood: Ошибка сохранения настроения в БД для чата %d: %v", chatID, err)
		return
	}

	log.Printf("[FreeWill] updateMood: Настроение успешно обновлено для чата %d: %s -> %s (интенсивность: %.2f -> %.2f, причина: %s)",
		chatID, currentMood.CurrentMood, moodData.CurrentMood, currentMood.MoodIntensity, moodData.MoodIntensity, moodData.TriggerReason)
}

// getCurrentMood возвращает текущее настроение в старом формате для совместимости
func (fws *FreeWillService) getCurrentMood(chatID int64) *FreeWillMoodState {
	// Получаем настроение из БД
	moodFromDB := fws.getMood(chatID)

	// Конвертируем в старый формат
	return &FreeWillMoodState{
		CurrentMood:    moodFromDB.CurrentMood,
		MoodIntensity:  moodFromDB.MoodIntensity,
		LastMoodUpdate: moodFromDB.LastMoodUpdate,
		TriggerReason:  moodFromDB.TriggerReason,
	}
}

// updateStats обновляет статистику
func (fws *FreeWillService) updateStats(chatID int64, decision *FreeWillDecision) {
	log.Printf("[FreeWill] updateStats: Обновляем статистику для чата %d (тип: %s)", chatID, decision.ReplyType)

	fws.mutex.Lock()
	defer fws.mutex.Unlock()

	stats := fws.getOrCreateStats(chatID)
	oldTotal := stats.TotalDecisions
	oldByType := stats.DecisionsByType[decision.ReplyType]
	oldThisHour := stats.DecisionsThisHour

	stats.TotalDecisions++
	stats.DecisionsByType[decision.ReplyType]++
	stats.LastDecisionTime = time.Now()
	stats.DecisionsThisHour++

	log.Printf("[FreeWill] updateStats: Статистика обновлена для чата %d - всего решений: %d->%d, типа '%s': %d->%d, за час: %d->%d",
		chatID, oldTotal, stats.TotalDecisions, decision.ReplyType, oldByType, stats.DecisionsByType[decision.ReplyType],
		oldThisHour, stats.DecisionsThisHour)
}

// GetStats возвращает статистику работы Free Will
func (fws *FreeWillService) GetStats(chatID int64) *FreeWillStats {
	fws.mutex.RLock()
	defer fws.mutex.RUnlock()

	stats, exists := fws.stats[chatID]
	if !exists {
		return &FreeWillStats{
			DecisionsByType: make(map[string]int),
		}
	}
	return stats
}

// canProcessDirectResponse проверяет, можно ли обработать прямое обращение с учетом лимитов
func (fws *FreeWillService) canProcessDirectResponse(chatID int64) bool {
	log.Printf("[FreeWill] canProcessDirectResponse: Проверяем лимиты прямых обращений для чата %d", chatID)

	// Если независимые лимиты отключены, используем обычную проверку
	if !fws.directResponseIndependentLimits {
		log.Printf("[FreeWill] canProcessDirectResponse: Независимые лимиты отключены, используем общие лимиты")
		return true // Используется общая проверка в shouldActivateAnalysis
	}

	fws.mutex.Lock()
	defer fws.mutex.Unlock()

	stats := fws.getOrCreateStats(chatID)

	// Сброс счетчика если прошел час
	if time.Since(stats.DirectResponseHourResetTime) > time.Hour {
		log.Printf("[FreeWill] canProcessDirectResponse: Сброс счетчика прямых обращений для чата %d: %d -> 0",
			chatID, stats.DirectResponsesThisHour)
		stats.DirectResponsesThisHour = 0
		stats.DirectResponseHourResetTime = time.Now()
	}

	// Проверка лимита за час
	if stats.DirectResponsesThisHour >= fws.directResponseMaxPerHour {
		log.Printf("[FreeWill] canProcessDirectResponse: Превышен лимит прямых обращений для чата %d (%d/%d)",
			chatID, stats.DirectResponsesThisHour, fws.directResponseMaxPerHour)
		return false
	}

	// Проверка минимального интервала
	if !stats.LastDirectResponseTime.IsZero() {
		elapsed := time.Since(stats.LastDirectResponseTime)
		if elapsed < fws.directResponseMinInterval {
			log.Printf("[FreeWill] canProcessDirectResponse: Слишком рано для прямого обращения в чате %d (прошло %v, минимум %v)",
				chatID, elapsed, fws.directResponseMinInterval)
			return false
		}
	}

	log.Printf("[FreeWill] canProcessDirectResponse: Прямое обращение разрешено для чата %d", chatID)
	return true
}

// updateDirectResponseStats обновляет статистику прямых обращений
func (fws *FreeWillService) updateDirectResponseStats(chatID int64, decision *FreeWillDecision) {
	log.Printf("[FreeWill] updateDirectResponseStats: Обновляем статистику прямых обращений для чата %d", chatID)

	fws.mutex.Lock()
	defer fws.mutex.Unlock()

	stats := fws.getOrCreateStats(chatID)

	// Если независимые лимиты включены, обновляем только счетчики прямых обращений
	if fws.directResponseIndependentLimits {
		oldDirectCount := stats.DirectResponsesThisHour
		stats.DirectResponsesThisHour++
		stats.LastDirectResponseTime = time.Now()

		log.Printf("[FreeWill] updateDirectResponseStats: Счетчик прямых обращений для чата %d: %d->%d (независимый от общих лимитов)",
			chatID, oldDirectCount, stats.DirectResponsesThisHour)
	} else {
		// Если независимые лимиты отключены, обновляем общую статистику
		log.Printf("[FreeWill] updateDirectResponseStats: Обновляем общую статистику для чата %d", chatID)
		oldTotal := stats.TotalDecisions
		oldByType := stats.DecisionsByType[decision.ReplyType]
		oldThisHour := stats.DecisionsThisHour

		stats.TotalDecisions++
		stats.DecisionsByType[decision.ReplyType]++
		stats.LastDecisionTime = time.Now()
		stats.DecisionsThisHour++

		log.Printf("[FreeWill] updateDirectResponseStats: Общая статистика обновлена для чата %d - всего решений: %d->%d, типа '%s': %d->%d, за час: %d->%d",
			chatID, oldTotal, stats.TotalDecisions, decision.ReplyType, oldByType, stats.DecisionsByType[decision.ReplyType],
			oldThisHour, stats.DecisionsThisHour)
	}
}

// getContextForAnalysis получает полный контекст чата для анализа Free Will
func (fws *FreeWillService) getContextForAnalysis(chatID int64) (string, error) {
	// Получаем последние сообщения из истории для анализа
	messages, err := fws.bot.storage.GetMessages(chatID, fws.contextWindow)
	if err != nil {
		return "", fmt.Errorf("ошибка получения истории: %w", err)
	}

	if len(messages) == 0 {
		return "Нет доступной истории сообщений", nil
	}

	// Берем последние N сообщений (если их больше чем нужно)
	if len(messages) > fws.contextWindow {
		messages = messages[len(messages)-fws.contextWindow:]
	}

	// Получаем релевантные сообщения из долгосрочной памяти, если включена
	var relevantMessages []*tgbotapi.Message
	if fws.bot.config.LongTermMemoryEnabled && len(messages) > 0 {
		// Используем последнее сообщение как запрос для поиска релевантного контекста
		lastMessage := messages[len(messages)-1]
		if lastMessage.Text != "" {
			relevantMsgs, err := fws.bot.storage.SearchRelevantMessages(chatID, lastMessage.Text, fws.bot.config.LongTermMemoryFetchK)
			if err != nil {
				log.Printf("[FreeWill] Ошибка поиска релевантных сообщений: %v", err)
			} else {
				relevantMessages = relevantMsgs
			}
		}
	}

	// Используем специальное форматирование для принятия решений Free Will
	// Это обеспечивает четкую связь между target_message_id и сообщениями
	return fws.formatFreeWillDecisionContext(chatID, messages, relevantMessages), nil
}

// getGeneralContext получает контекст для общих сообщений (используя CONTEXT_WINDOW)
func (fws *FreeWillService) getGeneralContext(chatID int64) (string, error) {
	log.Printf("[FreeWill] getGeneralContext: Начинаем получение общего контекста для чата %d", chatID)

	// Для general сообщений используем стандартное окно контекста
	messages, err := fws.bot.storage.GetMessages(chatID, fws.bot.config.ContextWindow)
	if err != nil {
		log.Printf("[FreeWill] getGeneralContext: Ошибка получения сообщений: %v", err)
		return "", fmt.Errorf("ошибка получения истории: %w", err)
	}

	log.Printf("[FreeWill] getGeneralContext: Получено %d сообщений из хранилища для чата %d", len(messages), chatID)

	if len(messages) == 0 {
		log.Printf("[FreeWill] getGeneralContext: Нет сообщений в истории для чата %d", chatID)
		return "Нет доступной истории сообщений", nil
	}

	// Берем последние сообщения
	if len(messages) > fws.bot.config.ContextWindow {
		messages = messages[len(messages)-fws.bot.config.ContextWindow:]
		log.Printf("[FreeWill] getGeneralContext: Обрезано до %d последних сообщений для чата %d", len(messages), chatID)
	}

	// Получаем релевантные сообщения из долгосрочной памяти
	var relevantMessages []*tgbotapi.Message
	if fws.bot.config.LongTermMemoryEnabled && len(messages) > 0 {
		lastMessage := messages[len(messages)-1]
		if lastMessage.Text != "" {
			log.Printf("[FreeWill] getGeneralContext: Ищем релевантные сообщения для чата %d", chatID)
			relevantMsgs, err := fws.bot.storage.SearchRelevantMessages(chatID, lastMessage.Text, fws.bot.config.LongTermMemoryFetchK)
			if err != nil {
				log.Printf("[FreeWill] getGeneralContext: Ошибка поиска релевантных сообщений для general: %v", err)
			} else {
				relevantMessages = relevantMsgs
				log.Printf("[FreeWill] getGeneralContext: Найдено %d релевантных сообщений для чата %d", len(relevantMessages), chatID)
			}
		}
	}

	log.Printf("[FreeWill] getGeneralContext: Вызываем formatDirectReplyContext для чата %d", chatID)
	result := formatDirectReplyContext(chatID, nil, nil, messages, relevantMessages, fws.bot.storage, fws.bot.config, fws.bot.config.TimeZone)

	log.Printf("[FreeWill] getGeneralContext: Результат форматирования для чата %d (длина: %d символов): %.200s...", chatID, len(result), result)

	return result, nil
}

// getContextBasedContext получает расширенный контекст для контекстных сообщений (используя FREE_WILL_CONTEXT_WINDOW)
func (fws *FreeWillService) getContextBasedContext(chatID int64) (string, error) {
	// Для context-based сообщений используем расширенное окно контекста
	messages, err := fws.bot.storage.GetMessages(chatID, fws.contextWindow)
	if err != nil {
		return "", fmt.Errorf("ошибка получения истории: %w", err)
	}

	if len(messages) == 0 {
		return "Нет доступной истории сообщений", nil
	}

	// Берем последние сообщения (больше чем для general)
	if len(messages) > fws.contextWindow {
		messages = messages[len(messages)-fws.contextWindow:]
	}

	// Получаем релевантные сообщения из долгосрочной памяти
	var relevantMessages []*tgbotapi.Message
	if fws.bot.config.LongTermMemoryEnabled && len(messages) > 0 {
		lastMessage := messages[len(messages)-1]
		if lastMessage.Text != "" {
			relevantMsgs, err := fws.bot.storage.SearchRelevantMessages(chatID, lastMessage.Text, fws.bot.config.LongTermMemoryFetchK)
			if err != nil {
				log.Printf("[FreeWill] Ошибка поиска релевантных сообщений для context: %v", err)
			} else {
				relevantMessages = relevantMsgs
			}
		}
	}

	return formatDirectReplyContext(chatID, nil, nil, messages, relevantMessages, fws.bot.storage, fws.bot.config, fws.bot.config.TimeZone), nil
}

// getDirectReplyContext получает полный контекст для прямого ответа на сообщение
func (fws *FreeWillService) getDirectReplyContext(chatID int64, targetMessageID int) (string, error) {
	log.Printf("[FreeWill] getDirectReplyContext: Начинаем получение контекста для чата %d, targetMessageID: %d", chatID, targetMessageID)

	// Получаем цепочку ответов для целевого сообщения
	replyChain, err := fws.bot.storage.GetReplyChain(context.Background(), chatID, targetMessageID, 10)
	if err != nil {
		log.Printf("[FreeWill] Ошибка получения цепочки ответов: %v", err)
		// Продолжаем без цепочки ответов
	} else {
		log.Printf("[FreeWill] getDirectReplyContext: Получена цепочка ответов длиной %d", len(replyChain))
	}

	// Получаем общий контекст чата
	messages, err := fws.bot.storage.GetMessages(chatID, fws.bot.config.ContextWindow)
	if err != nil {
		log.Printf("[FreeWill] getDirectReplyContext: Ошибка получения сообщений: %v", err)
		return "", fmt.Errorf("ошибка получения истории: %w", err)
	}
	log.Printf("[FreeWill] getDirectReplyContext: Получено %d сообщений из хранилища", len(messages))

	// Получаем релевантные сообщения из долгосрочной памяти
	var relevantMessages []*tgbotapi.Message
	if fws.bot.config.LongTermMemoryEnabled && len(messages) > 0 {
		// Ищем сообщение с указанным ID для поиска релевантного контекста
		var targetMessage *tgbotapi.Message
		for _, msg := range messages {
			if msg.MessageID == targetMessageID {
				targetMessage = msg
				break
			}
		}

		if targetMessage != nil && targetMessage.Text != "" {
			log.Printf("[FreeWill] getDirectReplyContext: Найдено целевое сообщение ID:%d, текст: %.50s...", targetMessageID, targetMessage.Text)
			relevantMsgs, err := fws.bot.storage.SearchRelevantMessages(chatID, targetMessage.Text, fws.bot.config.LongTermMemoryFetchK)
			if err != nil {
				log.Printf("[FreeWill] Ошибка поиска релевантных сообщений для direct reply: %v", err)
			} else {
				relevantMessages = relevantMsgs
				log.Printf("[FreeWill] getDirectReplyContext: Найдено %d релевантных сообщений", len(relevantMessages))
			}
		} else {
			log.Printf("[FreeWill] getDirectReplyContext: Целевое сообщение ID:%d не найдено или пустое", targetMessageID)
		}
	}

	// Находим целевое сообщение
	var triggeringMessage *tgbotapi.Message
	for _, msg := range messages {
		if msg.MessageID == targetMessageID {
			triggeringMessage = msg
			break
		}
	}

	if triggeringMessage != nil {
		log.Printf("[FreeWill] getDirectReplyContext: Найдено триггерное сообщение ID:%d от пользователя %d", targetMessageID, triggeringMessage.From.ID)
	} else {
		log.Printf("[FreeWill] getDirectReplyContext: ВНИМАНИЕ! Триггерное сообщение ID:%d НЕ НАЙДЕНО в %d сообщениях", targetMessageID, len(messages))
	}

	log.Printf("[FreeWill] getDirectReplyContext: Вызываем formatDirectReplyContext с параметрами:")
	log.Printf("  - chatID: %d", chatID)
	log.Printf("  - triggeringMessage: %v", triggeringMessage != nil)
	log.Printf("  - replyChain: %d сообщений", len(replyChain))
	log.Printf("  - messages: %d сообщений", len(messages))
	log.Printf("  - relevantMessages: %d сообщений", len(relevantMessages))

	result := formatDirectReplyContext(chatID, triggeringMessage, replyChain, messages, relevantMessages, fws.bot.storage, fws.bot.config, fws.bot.config.TimeZone)

	log.Printf("[FreeWill] getDirectReplyContext: Результат форматирования (длина: %d символов): %.200s...", len(result), result)

	return result, nil
}

// Методы построения промптов

// buildShouldReplyPrompt строит промпт для первого этапа (решение о необходимости ответа)
func (fws *FreeWillService) buildShouldReplyPrompt(context string, mood *FreeWillMoodState) string {
	// Получаем промпт из конфигурации
	prompt := fws.bot.config.FreeWillShouldReplyPrompt
	if prompt == "" {
		// Fallback базовый промпт
		basePersonality := "Ты — участник чата. Часто используй голосовые сообщения - они более живые."
		prompt = basePersonality + "\n\n" + context + "\n\nТебе нужно решить, стоит ли сейчас что-то сказать."
	}

	// Добавляем информацию о настроении
	if mood != nil {
		prompt += fmt.Sprintf("\n\nТвое текущее настроение: %s (интенсивность: %.1f)", mood.CurrentMood, mood.MoodIntensity)
	}

	prompt += "\n\n" + context

	return prompt
}

// buildResponseTypePrompt строит промпт для второго этапа (определение типа ответа)
func (fws *FreeWillService) buildResponseTypePrompt(replyType string) string {
	prompt := fws.bot.config.FreeWillResponseTypePrompt
	if prompt == "" {
		// Fallback базовый промпт
		prompt = "Сгенерируй ответ типа: " + replyType + "\n" +
			"Учитывай:\n" +
			"1. Тип ответа: " + replyType + "\n" +
			"2. Контекст чата\n" +
			"3. Свою личность\n" +
			"4. Возможность голосового сообщения\n" +
			"5. Пиши в стиле Андрея"
	}

	return strings.ReplaceAll(prompt, "{reply_type}", replyType)
}

// getContextForResponseType получает контекст для второго этапа
func (fws *FreeWillService) getContextForResponseType(chatID int64, shouldReplyDecision *FreeWillShouldReplyDecision) (string, error) {
	log.Printf("[FreeWill] getContextForResponseType: Начинаем получение контекста для чата %d, reply_type: %s", chatID, shouldReplyDecision.ReplyType)

	// В зависимости от типа ответа используем разные методы получения контекста
	switch shouldReplyDecision.ReplyType {
	case "direct_reply":
		log.Printf("[FreeWill] getContextForResponseType: Обрабатываем direct_reply для чата %d", chatID)
		if shouldReplyDecision.TargetMessageID != 0 {
			log.Printf("[FreeWill] getContextForResponseType: Используем getDirectReplyContext с targetMessageID: %d", shouldReplyDecision.TargetMessageID)
			return fws.getDirectReplyContext(chatID, shouldReplyDecision.TargetMessageID)
		}
		// Если нет target_message_id, используем общий контекст
		log.Printf("[FreeWill] getContextForResponseType: Нет target_message_id, используем getGeneralContext")
		return fws.getGeneralContext(chatID)
	case "take_response":
		log.Printf("[FreeWill] getContextForResponseType: Обрабатываем take_response для чата %d", chatID)
		// Для ответа на тейк используем контекст с цепочкой ответов
		if shouldReplyDecision.TargetMessageID != 0 {
			log.Printf("[FreeWill] getContextForResponseType: Используем getDirectReplyContext с targetMessageID: %d", shouldReplyDecision.TargetMessageID)
			return fws.getDirectReplyContext(chatID, shouldReplyDecision.TargetMessageID)
		}
		// Если нет target_message_id, используем общий контекст
		log.Printf("[FreeWill] getContextForResponseType: Нет target_message_id, используем getGeneralContext")
		return fws.getGeneralContext(chatID)
	case "context_based":
		log.Printf("[FreeWill] getContextForResponseType: Обрабатываем context_based для чата %d", chatID)
		return fws.getContextBasedContext(chatID)
	case "silence_response":
		log.Printf("[FreeWill] getContextForResponseType: Обрабатываем silence_response для чата %d", chatID)
		// Для ответа на тишину нужен минимальный контекст
		return fws.getGeneralContext(chatID)
	default: // "general"
		log.Printf("[FreeWill] getContextForResponseType: Обрабатываем general (default) для чата %d", chatID)
		return fws.getGeneralContext(chatID)
	}
}

// buildDirectReplyPrompt строит промпт для прямого ответа
func (fws *FreeWillService) buildDirectReplyPrompt(chatID int64, decision *FreeWillDecision) string {
	prompt := fws.bot.config.FreeWillDirectPrompt
	if prompt == "" {
		// Fallback промпт
		prompt = "Дай краткий прямой ответ на исходное сообщение с учетом контекста. Настроение: {mood} (интенсивность {intensity}). Поддерживай текущую личность и стиль."
	}

	// Получаем mood state для подстановки
	moodState := fws.getCurrentMood(chatID)
	currentMood := "neutral"
	intensity := "0.5"
	if moodState != nil {
		currentMood = moodState.CurrentMood
		intensity = fmt.Sprintf("%.1f", moodState.MoodIntensity)
	}

	// Заменяем плейсхолдеры
	prompt = strings.ReplaceAll(prompt, "{mood}", currentMood)
	prompt = strings.ReplaceAll(prompt, "{intensity}", intensity)

	return prompt
}

// buildGeneralPrompt строит промпт для общего сообщения
func (fws *FreeWillService) buildGeneralPrompt(chatID int64, decision *FreeWillDecision) string {
	prompt := fws.bot.config.FreeWillGeneralPrompt
	if prompt == "" {
		// Fallback промпт
		prompt = "Напиши общее сообщение в чат. Настроения: %s. Учти контекст чата и стиль Андрея (1-2 предложения)."
	}

	// Получаем mood state для подстановки
	moodState := fws.getCurrentMood(chatID)
	currentMood := "neutral"
	intensity := "0.5"
	if moodState != nil {
		currentMood = moodState.CurrentMood
		intensity = fmt.Sprintf("%.1f", moodState.MoodIntensity)
	}

	// Заменяем плейсхолдеры
	prompt = strings.ReplaceAll(prompt, "{mood}", currentMood)
	prompt = strings.ReplaceAll(prompt, "{intensity}", intensity)

	return prompt
}

// buildContextPrompt строит промпт для контекстного сообщения
func (fws *FreeWillService) buildContextPrompt(chatID int64, decision *FreeWillDecision) string {
	prompt := fws.bot.config.FreeWillContextPrompt
	if prompt == "" {
		// Fallback промпт
		prompt = "Прокомментируй недавнюю тему обсуждения. Настроения: %s. Учти контекст чата и стиль Андрея (1-2 предложения)."
	}

	// Получаем mood state для подстановки
	moodState := fws.getCurrentMood(chatID)
	currentMood := "neutral"
	intensity := "0.5"
	if moodState != nil {
		currentMood = moodState.CurrentMood
		intensity = fmt.Sprintf("%.1f", moodState.MoodIntensity)
	}

	// Заменяем плейсхолдеры
	prompt = strings.ReplaceAll(prompt, "{mood}", currentMood)
	prompt = strings.ReplaceAll(prompt, "{intensity}", intensity)

	return prompt
}

// buildSilencePrompt строит промпт для ответа на тишину
func (fws *FreeWillService) buildSilencePrompt(chatID int64, decision *FreeWillDecision) string {
	prompt := fws.bot.config.FreeWillSilencePrompt
	if prompt == "" {
		// Fallback промпт
		prompt = "В чате тишина. Оживи беседу. Настроения: %s. Учти контекст чата и стиль Андрея (1-2 предложения)."
	}

	// Получаем mood state для подстановки
	moodState := fws.getCurrentMood(chatID)
	currentMood := "neutral"
	intensity := "0.5"
	if moodState != nil {
		currentMood = moodState.CurrentMood
		intensity = fmt.Sprintf("%.1f", moodState.MoodIntensity)
	}

	// Заменяем плейсхолдеры
	prompt = strings.ReplaceAll(prompt, "{mood}", currentMood)
	prompt = strings.ReplaceAll(prompt, "{intensity}", intensity)

	return prompt
}

// buildMoodBasedPrompt строит промпт для сообщения по настроению
func (fws *FreeWillService) buildMoodBasedPrompt(chatID int64, decision *FreeWillDecision) string {
	// Этот промпт основан на настроении
	prompt := "Напиши сообщение в соответствии с настроением: %s. Учти контекст чата и стиль Андрея (1-2 предложения)."

	// Получаем mood state для подстановки
	moodState := fws.getCurrentMood(chatID)
	currentMood := "neutral"
	intensity := "0.5"
	if moodState != nil {
		currentMood = moodState.CurrentMood
		intensity = fmt.Sprintf("%.1f", moodState.MoodIntensity)
	}

	// Заменяем плейсхолдеры
	prompt = strings.ReplaceAll(prompt, "%s", currentMood)
	prompt = strings.ReplaceAll(prompt, "{mood}", currentMood)
	prompt = strings.ReplaceAll(prompt, "{intensity}", intensity)

	return prompt
}

// buildVoicePrompt строит промпт для голосового сообщения
func (fws *FreeWillService) buildVoicePrompt(chatID int64, decision *FreeWillDecision) string {
	// Используем специальный промпт для голосовых сообщений
	prompt := fws.bot.config.VoiceMessagesPrompt
	if prompt == "" {
		prompt = "Сгенерируй короткое голосовое сообщение (1-2 предложения) для участников чата."
	}

	// Получаем mood state для подстановки
	moodState := fws.getCurrentMood(chatID)
	currentMood := "neutral"
	intensity := "0.5"
	if moodState != nil {
		currentMood = moodState.CurrentMood
		intensity = fmt.Sprintf("%.1f", moodState.MoodIntensity)
	}

	// Встраиваем личность в промпт
	basePrompt := fws.bot.enrichPromptWithPersonality(prompt, chatID, "free_will_voice")

	return fmt.Sprintf(`%s Напиши это сообщение в соответствии с текущим настроением: %s. Интенсивность настроения: %s. Учти контекст чата и стиль Андрея (1-2 предложения).

Это будет голосовое сообщение, поэтому пиши естественно для произношения.
Причина голосового сообщения: %s`,
		basePrompt, currentMood, intensity, decision.Reason)
}

// buildTakeResponsePrompt строит промпт для ответа на тейк
func (fws *FreeWillService) buildTakeResponsePrompt(chatID int64, decision *FreeWillDecision) string {
	mood := fws.getCurrentMood(chatID)
	moodText := "нейтральном"
	intensityText := "0.5"

	if mood != nil {
		moodText = mood.CurrentMood
		intensityText = fmt.Sprintf("%.2f", mood.MoodIntensity)
	}

	// Используем специальный промпт для ответов на тейки
	basePrompt := fws.bot.enrichPromptWithPersonality(fws.takeResponsePrompt, chatID, "free_will_take_response")

	return fmt.Sprintf(`%s 

Текущее настроение: %s (интенсивность: %s).
Настроение должно влиять на стиль и аргументацию ответа.
Причина ответа на тейк: %s`,
		basePrompt, moodText, intensityText, decision.Reason)
}

// IsEnabled проверяет, включен ли сервис Free Will
func (fws *FreeWillService) IsEnabled() bool {
	return fws.enabled
}

// SetEnabled включает/выключает сервис Free Will
func (fws *FreeWillService) SetEnabled(enabled bool) {
	fws.mutex.Lock()
	defer fws.mutex.Unlock()
	fws.enabled = enabled
	log.Printf("[FreeWill] Сервис %s", map[bool]string{true: "включен", false: "выключен"}[enabled])
}

// ForceAnalysis принудительно запускает анализ для чата (для отладки)
func (fws *FreeWillService) ForceAnalysis(chatID int64) {
	if !fws.enabled {
		log.Printf("[FreeWill] Сервис отключен, принудительный анализ невозможен")
		return
	}

	log.Printf("[FreeWill] Принудительный запуск анализа для чата %d", chatID)
	go fws.analyzeAndAct(chatID)
}

// ForceUpdateMood принудительно обновляет настроение для чата
func (fws *FreeWillService) ForceUpdateMood(chatID int64) {
	if !fws.enabled {
		log.Printf("[FreeWill] Сервис отключен, обновление настроения невозможно")
		return
	}

	log.Printf("[FreeWill] Принудительное обновление настроения для чата %d", chatID)
	go func() {
		context, err := fws.getContextForAnalysis(chatID)
		if err != nil {
			log.Printf("[FreeWill] Ошибка получения контекста для принудительного обновления настроения чата %d: %v", chatID, err)
			return
		}
		fws.updateMood(chatID, context)
	}()
}

// getMood возвращает текущее настроение бота для чата
func (fws *FreeWillService) getMood(chatID int64) *storage.MoodState {
	// Получаем настроение из БД
	mood, err := fws.bot.storage.GetMoodState(chatID)
	if err != nil {
		log.Printf("[FreeWill] Ошибка получения настроения из БД для чата %d: %v", chatID, err)
		// Возвращаем базовое настроение
		return &storage.MoodState{
			ChatID:         chatID,
			CurrentMood:    "neutral",
			MoodIntensity:  0.5,
			LastMoodUpdate: time.Now(),
			TriggerReason:  "Default fallback mood",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
	}

	return mood
}

// formatFreeWillDecisionContext форматирует контекст специально для принятия решений Free Will
// Теперь использует унифицированное форматирование с акцентом на MessageID для target_message_id
func (fws *FreeWillService) formatFreeWillDecisionContext(chatID int64, messages []*tgbotapi.Message, relevantMessages []*tgbotapi.Message) string {
	// Используем новый унифицированный форматтер
	formatter := NewUnifiedMessageFormatter(fws.bot.storage, fws.bot.config.TimeZone)
	formattedHistory := formatter.FormatMessages(chatID, messages)

	log.Printf("[FreeWill] Chat %d: Использован унифицированный форматтер для %d сообщений", chatID, len(messages))
	return formattedHistory
}

// getMessageAuthorAlias получает алиас автора сообщения с кешированием профилей и дисамбигуацией
func (fws *FreeWillService) getMessageAuthorAlias(chatID int64, msg *tgbotapi.Message, profilesCache map[int64]*storage.UserProfile) string {
	if msg.From == nil {
		if msg.SenderChat != nil {
			if msg.SenderChat.Title != "" {
				return msg.SenderChat.Title
			}
			return fmt.Sprintf("Chat_%d", msg.SenderChat.ID)
		}
		return "Неизвестный"
	}

	userID := msg.From.ID

	// НОВОЕ: Используем систему дисамбигуации для Free Will (decision_making контекст)
	if fws.bot.userValidator != nil {
		return fws.bot.userValidator.FormatUserWithDisambiguation(chatID, userID, "decision_making", msg)
	}

	// Гарантируем, что кеш профилей инициализирован (nil-safe)
	if profilesCache == nil {
		profilesCache = make(map[int64]*storage.UserProfile)
	}

	// Fallback к старой логике если валидатор недоступен
	// Проверяем кеш профилей
	profile, found := profilesCache[userID]
	if !found {
		// Загружаем профиль, если не в кеше
		loadedProfile, err := fws.bot.storage.GetUserProfile(chatID, userID)
		if err != nil {
			log.Printf("[WARN] Chat %d: Ошибка загрузки профиля для userID %d: %v", chatID, userID, err)
		} else if loadedProfile != nil {
			profilesCache[userID] = loadedProfile // Сохраняем в кеш
			profile = loadedProfile
		}
	}

	// Определяем алиас
	if profile != nil && profile.Alias != "" {
		return profile.Alias
	} else if msg.From.FirstName != "" {
		return msg.From.FirstName
	} else if msg.From.UserName != "" {
		return msg.From.UserName
	} else {
		return fmt.Sprintf("User_%d", userID)
	}
}

// validateTargetMessageID проверяет, что target_message_id существует в недавних сообщениях
func (fws *FreeWillService) validateTargetMessageID(chatID int64, targetMessageID int) error {
	// Получаем недавние сообщения из того же контекста, что используется для анализа
	messages, err := fws.bot.storage.GetMessages(chatID, fws.contextWindow)
	if err != nil {
		return fmt.Errorf("ошибка получения сообщений для валидации: %w", err)
	}

	// Ограничиваем до contextWindow как в getContextForAnalysis
	if len(messages) > fws.contextWindow {
		messages = messages[len(messages)-fws.contextWindow:]
	}

	// Проверяем, есть ли сообщение с таким ID
	for _, msg := range messages {
		if msg.MessageID == targetMessageID {
			log.Printf("[FreeWill] validateTargetMessageID: Найдено валидное сообщение %d в чате %d", targetMessageID, chatID)
			return nil
		}
	}

	return fmt.Errorf("сообщение с ID %d не найдено в контексте чата %d", targetMessageID, chatID)
}

// getTargetMessageInfo получает краткую информацию о целевом сообщении для логирования
func (fws *FreeWillService) getTargetMessageInfo(chatID int64, targetMessageID int) string {
	messages, err := fws.bot.storage.GetMessages(chatID, fws.contextWindow)
	if err != nil {
		return "ошибка получения сообщения"
	}

	// Ограничиваем до contextWindow
	if len(messages) > fws.contextWindow {
		messages = messages[len(messages)-fws.contextWindow:]
	}

	// Ищем целевое сообщение
	for _, msg := range messages {
		if msg.MessageID == targetMessageID {
			profiles := make(map[int64]*storage.UserProfile)
			authorAlias := fws.getMessageAuthorAlias(chatID, msg, profiles)

			msgText := msg.Text
			if msgText == "" {
				msgText = msg.Caption
			}

			// Ограничиваем длину для логирования
			if len(msgText) > 50 {
				msgText = msgText[:47] + "..."
			}

			return fmt.Sprintf("от %s: %s", authorAlias, msgText)
		}
	}

	return "сообщение не найдено"
}

// ================================ ДЕТЕКЦИЯ ТЕЙКОВ ================================

// analyzeForTake анализирует сообщение на предмет того, является ли оно "тейком"

// ================================ СИСТЕМА РЕАКЦИЙ ================================

// analyzeForReaction анализирует сообщение для постановки реакции
func (fws *FreeWillService) analyzeForReaction(chatID int64, message *tgbotapi.Message) {
	// Проверяем основные условия ДО обращения к полям message
	if message == nil {
		log.Printf("[FreeWill] analyzeForReaction: Сообщение отсутствует (nil)")
		return
	}
	// Копируем значение, чтобы дальше работать с непустой структурой и избежать предупреждений анализатора
	msg := *message
	if msg.Text == "" {
		log.Printf("[FreeWill] analyzeForReaction: Сообщение без текста")
		return
	}

	log.Printf("[FreeWill] analyzeForReaction: Начинаем анализ реакции для сообщения %d в чате %d", msg.MessageID, chatID)

	// Проверяем cooldown и лимиты
	if !fws.canReact(chatID) {
		log.Printf("[FreeWill] analyzeForReaction: Нельзя реагировать (cooldown или лимит)")
		return
	}

	// Запрашиваем LLM для выбора реакции
	prompt := fws.bot.config.FreeWillReactionPrompt + "\n\n" + msg.Text
	log.Printf("[FreeWill] analyzeForReaction: Отправляем запрос к LLM для выбора реакции")

	response, err := fws.bot.llm.GenerateResponseByType(
		llm.ResponseTypeFreeWillReaction,
		prompt,
		"",
		float32(fws.bot.config.GeminiTemperatureNormal),
	)

	if err != nil {
		log.Printf("[FreeWill] analyzeForReaction: Ошибка получения ответа от LLM: %v", err)
		return
	}

	log.Printf("[FreeWill] analyzeForReaction: Получен ответ от LLM: %s", response)

	// Парсим решение о реакции
	reactionDecision, err := fws.parseReactionDecision(response)
	if err != nil {
		log.Printf("[FreeWill] analyzeForReaction: Ошибка парсинга решения: %v", err)
		return
	}

	if !reactionDecision.ShouldReact {
		log.Printf("[INFO][FreeWill] ReactionDecision: chat=%d, message_id=%d, should_react=%t, reason=%q",
			chatID, msg.MessageID, reactionDecision.ShouldReact, reactionDecision.Reason)
		return
	}
	log.Printf("[INFO][FreeWill] ReactionDecision: chat=%d, message_id=%d, should_react=%t, reaction=%s, reason=%q",
		chatID, msg.MessageID, reactionDecision.ShouldReact, reactionDecision.Reaction, reactionDecision.Reason)

	// Ставим реакцию
	fws.setReaction(chatID, msg.MessageID, reactionDecision.Reaction)
}

// parseReactionDecision парсит решение LLM о реакции
func (fws *FreeWillService) parseReactionDecision(response string) (*FreeWillReactionDecision, error) {
	// Очищаем ответ от markdown
	cleanResponse := cleanJSONFromMarkdown(response)

	var decision FreeWillReactionDecision
	err := json.Unmarshal([]byte(cleanResponse), &decision)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга JSON решения о реакции: %w", err)
	}

	return &decision, nil
}

// canReact проверяет, можно ли поставить реакцию (cooldown + лимиты)
func (fws *FreeWillService) canReact(chatID int64) bool {
	fws.mutex.Lock()
	defer fws.mutex.Unlock()

	now := time.Now()

	// Ленивая инициализация map при необходимости
	if fws.lastReactionTimes == nil {
		fws.lastReactionTimes = make(map[int64]time.Time)
	}
	if fws.reactionHourResetTime == nil {
		fws.reactionHourResetTime = make(map[int64]time.Time)
	}
	if fws.reactionCountThisHour == nil {
		fws.reactionCountThisHour = make(map[int64]int)
	}

	// Проверяем cooldown
	if lastReaction, exists := fws.lastReactionTimes[chatID]; exists {
		if now.Sub(lastReaction) < fws.reactionsCooldownPeriod {
			log.Printf("[FreeWill] canReact: Cooldown не прошел для чата %d", chatID)
			return false
		}
	}

	// Проверяем часовой лимит, используя границы часа
	resetTime, exists := fws.reactionHourResetTime[chatID]
	if !exists || now.Truncate(time.Hour) != resetTime.Truncate(time.Hour) {
		// Наступил новый час - сбрасываем счетчик
		fws.reactionCountThisHour[chatID] = 0
		fws.reactionHourResetTime[chatID] = now.Truncate(time.Hour)
		log.Printf("[FreeWill] canReact: Сброс часового счетчика реакций для чата %d", chatID)
	}

	// Проверяем и инициализируем счетчик если нужно
	count := 0
	if existingCount, exists := fws.reactionCountThisHour[chatID]; exists {
		count = existingCount
	} else {
		fws.reactionCountThisHour[chatID] = 0
	}

	if count >= fws.reactionsMaxPerHour {
		log.Printf("[FreeWill] canReact: Достигнут часовой лимит реакций для чата %d", chatID)
		return false
	}

	// Обновляем счетчики под защитой мьютекса
	fws.lastReactionTimes[chatID] = now
	fws.reactionCountThisHour[chatID]++

	return true
}

// setReaction ставит реакцию на сообщение
func (fws *FreeWillService) setReaction(chatID int64, messageID int, reaction string) {
	log.Printf("[FreeWill] setReaction: Ставим реакцию %s на сообщение %d в чате %d", reaction, messageID, chatID)

	// Ставим реакцию через ReactionTracker
	if fws.bot.reactionTracker != nil {
		err := fws.bot.reactionTracker.SetBotReaction(chatID, messageID, reaction)
		if err != nil {
			log.Printf("[FreeWill] setReaction: Ошибка постановки реакции: %v", err)
			return
		}
	} else {
		log.Printf("[FreeWill] setReaction: ReactionTracker не инициализирован")
		return
	}

	log.Printf("[FreeWill] setReaction: Реакция %s успешно поставлена", reaction)
}

// === СИСТЕМА ПРЕДОТВРАЩЕНИЯ ДУБЛИРОВАНИЯ ===

// markMessageProcessed отмечает сообщение как обработанное
func (fws *FreeWillService) markMessageProcessed(chatID int64, messageID int) {
	fws.mutex.Lock()
	defer fws.mutex.Unlock()

	key := fmt.Sprintf("%d:%d", chatID, messageID)
	fws.processedMessages[key] = true

	log.Printf("[FreeWill] AntiDup: ✅ Сообщение %s отмечено как обработанное", key)
}

// isMessageProcessed проверяет, было ли сообщение уже обработано
func (fws *FreeWillService) isMessageProcessed(chatID int64, messageID int) bool {
	fws.mutex.RLock()
	defer fws.mutex.RUnlock()

	key := fmt.Sprintf("%d:%d", chatID, messageID)
	processed := fws.processedMessages[key]

	log.Printf("[FreeWill] AntiDup: 🔍 Проверка сообщения %s: обработано=%t", key, processed)
	return processed
}

// cleanOldProcessedMessages очищает старые записи (вызывается периодически)
func (fws *FreeWillService) cleanOldProcessedMessages() {
	fws.mutex.Lock()
	defer fws.mutex.Unlock()

	// Очищаем все записи старше 1 часа (простая реализация)
	// В production можно добавить timestamp для каждой записи
	if len(fws.processedMessages) > 1000 {
		fws.processedMessages = make(map[string]bool)
		log.Printf("[FreeWill] AntiDup: 🧹 Очищен кэш обработанных сообщений (превышен лимит 1000)")
	}
}

// checkImageGeneration проверяет и выполняет генерацию изображений в рамках Free Will
func (fws *FreeWillService) checkImageGeneration(chatID int64, decision *FreeWillDecision) {
	// Проверяем, что сервис генерации изображений доступен
	if fws.bot.imageGenerationService == nil || !fws.bot.imageGenerationService.IsEnabled() {
		return
	}

	// Собираем контекстные данные для принятия решения
	contextData := map[string]interface{}{
		"mood":            decision.Mood,
		"reply_type":      decision.ReplyType,
		"is_voice":        decision.IsVoice,
		"response_reason": decision.Reason,
		"decision_time":   time.Now(),
	}

	// Используем механизм принятия решений сервиса изображений
	shouldGenerate := fws.bot.imageGenerationService.DecisionMechanismShouldGenerate(chatID, contextData)

	if !shouldGenerate {
		log.Printf("[FreeWill] checkImageGeneration: Генерация изображения не требуется для чата %d", chatID)
		return
	}

	log.Printf("[FreeWill] checkImageGeneration: 🎨 Принято решение о генерации изображения для чата %d", chatID)

	// Запускаем генерацию изображения в отдельной горутине, чтобы не блокировать основной ответ
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		image, err := fws.bot.imageGenerationService.GenerateImageForChat(ctx, chatID, "personality_based")
		if err != nil {
			log.Printf("[FreeWill] checkImageGeneration: ❌ Ошибка генерации изображения для чата %d: %v", chatID, err)
			return
		}

		err = fws.bot.imageGenerationService.SendGeneratedImage(image)
		if err != nil {
			log.Printf("[FreeWill] checkImageGeneration: ❌ Ошибка отправки изображения для чата %d: %v", chatID, err)
			return
		}

		log.Printf("[FreeWill] checkImageGeneration: ✅ Изображение успешно сгенерировано и отправлено в чат %d", chatID)
	}()
}

// checkImageGenerationForAllChats проверяет возможность генерации изображений для всех активных чатов независимо от текстовых ответов
func (fws *FreeWillService) checkImageGenerationForAllChats() {
	// Проверяем, что сервис генерации изображений доступен
	if fws.bot.imageGenerationService == nil || !fws.bot.imageGenerationService.IsEnabled() {
		return
	}

	fws.mutex.RLock()
	// Копируем данные под RLock для минимизации времени блокировки
	lastMessagesCopy := make(map[int64]time.Time)
	for chatID, lastTime := range fws.lastMessage {
		lastMessagesCopy[chatID] = lastTime
	}
	fws.mutex.RUnlock()

	now := time.Now()
	for chatID, lastMessageTime := range lastMessagesCopy {
		// Проверяем базовые условия для генерации изображений
		timeSinceLastMessage := now.Sub(lastMessageTime)

		// Генерируем изображения реже, чем текстовые ответы (минимальный интервал из конфигурации)
		if timeSinceLastMessage < fws.imageGenerationMinDecisionInterval {
			continue
		}

		// Проверяем лимиты генерации изображений
		if !fws.canGenerateImage(chatID) {
			continue
		}

		// Базовые контекстные данные для принятия решения
		contextData := map[string]interface{}{
			"decision_time":      now,
			"silence_duration":   timeSinceLastMessage,
			"generation_trigger": "silence_based",
		}

		// Используем механизм принятия решений сервиса изображений
		shouldGenerate := fws.bot.imageGenerationService.DecisionMechanismShouldGenerate(chatID, contextData)

		if !shouldGenerate {
			continue
		}

		log.Printf("[FreeWill] checkImageGenerationForAllChats: 🎨 Принято решение о генерации изображения для чата %d (тишина: %v)",
			chatID, timeSinceLastMessage)

		// Обновляем статистику генерации изображений
		fws.updateImageGenerationStats(chatID)

		// Запускаем генерацию изображения в отдельной горутине
		go func(cID int64) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			image, err := fws.bot.imageGenerationService.GenerateImageForChat(ctx, cID, "personality_based")
			if err != nil {
				log.Printf("[FreeWill] checkImageGenerationForAllChats: ❌ Ошибка генерации изображения для чата %d: %v", cID, err)
				return
			}

			err = fws.bot.imageGenerationService.SendGeneratedImage(image)
			if err != nil {
				log.Printf("[FreeWill] checkImageGenerationForAllChats: ❌ Ошибка отправки изображения для чата %d: %v", cID, err)
				return
			}

			log.Printf("[FreeWill] checkImageGenerationForAllChats: ✅ Изображение успешно сгенерировано и отправлено в чат %d", cID)
		}(chatID)
	}
}

// canGenerateImage проверяет, можно ли сгенерировать изображение для чата с учетом лимитов
func (fws *FreeWillService) canGenerateImage(chatID int64) bool {
	fws.mutex.Lock()
	defer fws.mutex.Unlock()

	stats := fws.getOrCreateStats(chatID)
	now := time.Now()

	// Сбрасываем счетчик если прошел интервал
	if now.Sub(stats.ImageGenerationIntervalResetTime) >= fws.imageGenerationIntervalDuration {
		stats.ImageGenerationDecisionsThisInterval = 0
		stats.ImageGenerationIntervalResetTime = now
		log.Printf("[FreeWill] canGenerateImage: Сброшен счетчик изображений для чата %d (интервал %v)", chatID, fws.imageGenerationIntervalDuration)
	}

	// Проверяем лимит генераций за интервал
	if stats.ImageGenerationDecisionsThisInterval >= fws.imageGenerationMaxDecisionsPerInterval {
		log.Printf("[FreeWill] canGenerateImage: Превышен лимит изображений для чата %d (%d/%d за %v)",
			chatID, stats.ImageGenerationDecisionsThisInterval, fws.imageGenerationMaxDecisionsPerInterval, fws.imageGenerationIntervalDuration)
		return false
	}

	// Проверяем минимальный интервал между генерациями
	if !stats.LastImageGenerationDecisionTime.IsZero() {
		timeSinceLastGeneration := now.Sub(stats.LastImageGenerationDecisionTime)
		if timeSinceLastGeneration < fws.imageGenerationMinDecisionInterval {
			log.Printf("[FreeWill] canGenerateImage: Слишком рано для новой генерации в чате %d (%v < %v)",
				chatID, timeSinceLastGeneration, fws.imageGenerationMinDecisionInterval)
			return false
		}
	}

	return true
}

// updateImageGenerationStats обновляет статистику генерации изображений
func (fws *FreeWillService) updateImageGenerationStats(chatID int64) {
	fws.mutex.Lock()
	defer fws.mutex.Unlock()

	stats := fws.getOrCreateStats(chatID)
	now := time.Now()

	stats.ImageGenerationDecisionsThisInterval++
	stats.LastImageGenerationDecisionTime = now

	log.Printf("[FreeWill] updateImageGenerationStats: Обновлена статистика изображений для чата %d: %d/%d за интервал %v",
		chatID, stats.ImageGenerationDecisionsThisInterval, fws.imageGenerationMaxDecisionsPerInterval, fws.imageGenerationIntervalDuration)
}

// ========== ПУБЛИЧНЫЕ МЕТОДЫ ДЛЯ ТЕСТИРОВАНИЯ ==========

// CanGenerateImageForChat проверяет, можно ли генерировать изображение для чата (публичный метод для тестирования)
func (fws *FreeWillService) CanGenerateImageForChat(chatID int64) bool {
	return fws.canGenerateImage(chatID)
}

// UpdateImageGenerationStats обновляет статистику генерации изображений (публичный метод для тестирования)
func (fws *FreeWillService) UpdateImageGenerationStats(chatID int64) {
	fws.updateImageGenerationStats(chatID)
}

// GetStatsForChat возвращает статистику для конкретного чата (публичный метод для тестирования)
func (fws *FreeWillService) GetStatsForChat(chatID int64) *FreeWillStats {
	fws.mutex.RLock()
	defer fws.mutex.RUnlock()
	return fws.stats[chatID]
}

// GetAllStats возвращает всю статистику по чатам (публичный метод для тестирования)
func (fws *FreeWillService) GetAllStats() map[int64]*FreeWillStats {
	fws.mutex.RLock()
	defer fws.mutex.RUnlock()

	result := make(map[int64]*FreeWillStats)
	for chatID, stats := range fws.stats {
		// Создаем копию для безопасности
		statsCopy := *stats
		result[chatID] = &statsCopy
	}
	return result
}
