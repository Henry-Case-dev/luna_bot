package bot

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/bot/prompts"
	"github.com/Henry-Case-dev/luna_bot/internal/llm"
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

// QuickRuleResult — результат быстрой проверки
type QuickRuleResult struct {
	Matched     bool
	ShouldReply bool
	Reason      string
	ReplyType   string
	Mood        string
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

	// Fix P12.4: отслеживание провалов анализа для уменьшения кулдауна
	lastAnalysisFailed map[int64]bool
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
		lastAnalysisFailed:      make(map[int64]bool),
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
	log.Printf("[FreeWill] NewFreeWillService:   🕐 Тикер создан: %p", service.ticker)
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

	// Получаем общий контекст чата для понимания атмосферы (fallback для Этапа 2)
	generalContext, err := fws.getContextForAnalysis(chatID)
	if err != nil {
		log.Printf("[ERROR][FreeWill] analyzeDirectResponse: Ошибка получения контекста для чата %d: %v", chatID, err)
		return
	}

	// Build ChatML context for decision
	decisionMsgs := fws.buildResponseChatContext(chatID, fws.bot.config.ContextWindow, int64(message.From.ID))
	if decisionMsgs == nil {
		log.Printf("[ERROR][FreeWill] analyzeDirectResponse: Ошибка построения ChatML контекста для чата %d: %v", chatID, err)
		return
	}

	// Текущее сообщение как последнее user-сообщение в ChatML
	decisionMsgs = append(decisionMsgs, llm.ChatMessage{
		Role:    "user",
		Content: message.Text,
	})

	// Prepend decision prompt to system message
	prompt := fws.bot.enrichPromptWithPersonality(fws.bot.config.FreeWillDirectResponseDecisionPrompt, chatID, "free_will_direct_response_decision")
	decisionMsgs[0].Content = prompt + "\n\n" + decisionMsgs[0].Content

	// Add assoc context to system message
	if assoc := fws.bot.getAssociativeContext(chatID, fws.bot.buildAssociativeKeys(chatID), 3); assoc != "" {
		decisionMsgs[0].Content = assoc + "\n\n" + decisionMsgs[0].Content
	}

	log.Printf("[FreeWill] analyzeDirectResponse: 🤖 ЭТАП 1: Отправляем запрос в LLM для принятия решения")
	log.Printf("[FreeWill] analyzeDirectResponse: 📝 Промпт решения длина: %d символов", len(prompt))
	log.Printf("[FreeWill] analyzeDirectResponse: 📝 ChatML сообщений: %d", len(decisionMsgs))

	// Показываем "печатает..." пока LLM думает
	fws.bot.setTypingAction(chatID)

	// Генерируем решение через LLM (ChatML)
	decisionResponse, err := fws.bot.llm.GenerateChatResponse(
		llm.ResponseTypeFreeWillDirectResponseDecision,
		decisionMsgs,
		0.2, // CRITICAL: low temp (0.2) prevents JSON breakage in decision stage
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

	// ЭТАП 1.5: КЛАССИФИКАЦИЯ СЕРЬЕЗНОСТИ (RULE-BASED, без LLM)
	log.Printf("[FreeWill] analyzeDirectResponse: 🔍 ЭТАП 1.5: Классификация серьезности (rule-based)")

	msgType := "casual" // по умолчанию

	if message.Text != "" {
		msgType = classifyMessageSeriousness(message.Text)
		log.Printf("[DEBUG][FreeWill] analyzeDirectResponse: Сообщение в чате %d классифицировано как %s (rule-based)", chatID, strings.ToUpper(msgType))
	}

	// ЭТАП 2: ГЕНЕРАЦИЯ ОТВЕТА
	log.Printf("[FreeWill] analyzeDirectResponse: 🎭 ЭТАП 2: Генерация ответа (тип: %s)", msgType)

	// Build ChatML context for response
	responseMsgs := fws.buildResponseChatContext(chatID, fws.bot.config.ContextWindow, int64(message.From.ID))
	if responseMsgs == nil {
		log.Printf("[WARN][FreeWill] analyzeDirectResponse: Используем fallback для ответа")
	} else {
		// Append current user message as last ChatMessage
		responseMsgs = append(responseMsgs, llm.ChatMessage{
			Role:    "user",
			Content: message.Text,
		})
	}

	// Подмешиваем лёгкий ассоциативный контекст (зафичено флагом)
	if responseMsgs != nil {
		if assoc := fws.bot.getAssociativeContext(chatID, nil, 3); assoc != "" {
			responseMsgs[0].Content = responseMsgs[0].Content + "\n\n" + assoc
		}
	}

	// === КОГНИТИВНАЯ ИНТЕГРАЦИЯ (ЭТАП 3): Внутренний монолог перед генерацией ===
	if fws.bot.config.InternalMonologueEnabled {
		trigger := message.Text
		thought := fws.bot.InternalMonologue(chatID, trigger, "free_will_direct")
		if thought != nil {
			thought.ActionTaken = true
			fws.bot.RecordInternalThought(chatID, thought)
			log.Printf("[Stage3][FW-DR] Чат %d: injected internal_thought type=%s len=%d", chatID, thought.Type, len(thought.Content))
			if responseMsgs != nil {
				responseMsgs[0].Content = "[internal_thought]: " + utils.TruncateString(thought.Content, 120) + "\n\n" + responseMsgs[0].Content
			}
		}
	}

	// Детерминированная подсказка стиля на основе отношений с автором сообщения
	userID := int64(message.From.ID)
	if responseMsgs != nil {
		before := len(responseMsgs[0].Content)
		responseMsgs[0].Content = fws.bot.ApplyRelationshipStyleToContext(chatID, userID, responseMsgs[0].Content)
		if len(responseMsgs[0].Content) > before {
			style := fws.bot.GetRelationshipInfluencedCommunicationStyle(chatID, userID)
			log.Printf("[Stage4][FW-DR] Chat %d: tone_hint applied, style=%s", chatID, style)
		}
	}

	// Выбираем промпт и настройки в зависимости от серьезности
	var responsePrompt string
	var respTemp float32
	var responseType llm.ResponseType

	if msgType == "serious" && fws.bot.config.SeriousDirectPrompt != "" {
		responsePrompt = fws.bot.config.SeriousDirectPrompt // kept as task instruction
		respTemp = 0.4 // lower temp for factual/serious responses
		responseType = llm.ResponseTypeSerious

		log.Printf("[INFO][FreeWill] analyzeDirectResponse: Чат %d: Используем SERIOUS_DIRECT_PROMPT", chatID)

		// Для серьезных ответов используем веб-поиск
		if fws.bot.webSearch != nil && fws.bot.webSearch.IsEnabled() && responseMsgs != nil {
			if results := fws.bot.webSearch.SearchAndFormat(message.Text); results != "" {
				responseMsgs[0].Content = responseMsgs[0].Content + "\n\n=== РЕЗУЛЬТАТЫ ПОИСКА ===\n" + results
				log.Printf("[INFO][FreeWill] analyzeDirectResponse: Чат %d: Контекст расширен результатами веб-поиска для серьезного ответа", chatID)
			}
		}
	} else {
		// P1 FIX: Используем free_will_direct (СВОБОДНЫЙ ТЕКСТ) вместо free_will_direct_response (JSON)
		responsePrompt = fws.bot.config.FreeWillDirectPrompt
		// Подставляем {mood} и {intensity}
		moodState := fws.getCurrentMood(chatID)
		if moodState != nil {
			responsePrompt = strings.ReplaceAll(responsePrompt, "{mood}", moodState.CurrentMood)
			responsePrompt = strings.ReplaceAll(responsePrompt, "{intensity}", fmt.Sprintf("%.1f", moodState.MoodIntensity))
		} else {
			responsePrompt = strings.ReplaceAll(responsePrompt, "{mood}", "neutral")
			responsePrompt = strings.ReplaceAll(responsePrompt, "{intensity}", "0.5")
		}
		respTemp = float32(responseTemperature) // package-level constant 0.7 for creative text
		responseType = llm.ResponseTypeFreeWillDirect

		log.Printf("[INFO][FreeWill] analyzeDirectResponse: Чат %d: Используем FREE_WILL_DIRECT_PROMPT (свободный текст, без JSON)", chatID)
	}

	// Prepend prompt to system message
	if responseMsgs != nil {
		enrichedPrompt := fws.bot.enrichPromptWithPersonality(responsePrompt, chatID, "free_will_direct")
		responseMsgs[0].Content = enrichedPrompt + "\n\n" + responseMsgs[0].Content
	}

	log.Printf("[FreeWill] analyzeDirectResponse: 🤖 ЭТАП 2: Отправляем запрос в LLM для генерации ответа")
	log.Printf("[FreeWill] analyzeDirectResponse: 📝 Промпт ответа длина: %d символов", len(responsePrompt))
	log.Printf("[INFO][FreeWill] analyzeDirectResponse: Генерируем ответ. Тип: %s, Температура: %.2f", msgType, respTemp)

	// Продлеваем "печатает..." на время генерации
	fws.bot.setTypingAction(chatID)

	// Генерируем ответ через LLM (ChatML)
	var responseContent string
	if responseMsgs != nil {
		responseContent, err = fws.bot.llm.GenerateChatResponse(
			responseType,
			responseMsgs,
			respTemp,
		)
	} else {
		// Fallback: use old string-based approach
		responseContent, err = fws.bot.llm.GenerateResponseByType(
			responseType,
			fws.bot.enrichPromptWithPersonality(responsePrompt, chatID, "free_will_direct"),
			generalContext,
			respTemp,
		)
	}

	if err != nil {
		log.Printf("[ERROR][FreeWill] analyzeDirectResponse: Ошибка генерации ответа LLM для чата %d: %v", chatID, err)
		return
	}

	log.Printf("[FreeWill] analyzeDirectResponse: ✅ ЭТАП 2: Получен ответ от LLM: %s", responseContent)

	// P1 FIX: Оба пути (serious и casual) теперь выдают СВОБОДНЫЙ ТЕКСТ, а не JSON
	// Парсим как plain text — никакого JSON-парсинга для финального ответа!
	var responseText string
	var isVoice bool
	var responseMood string

	if msgType == "serious" {
		// Серьёзный ответ: просто очищаем текст
		responseText = cleanupLLMResponse(responseContent)
		isVoice = false // серьёзные ответы всегда текстовые
		responseMood = "serious"
	} else {
		// Обычный ответ: очищаем текст, is_voice — рандом, mood — из состояния
		responseText = cleanupLLMResponse(responseContent)
		// Проверяем, не вернула ли модель JSON-галлюцинацию
		if looksLikeInternalJSON(responseText) {
			log.Printf("[ERROR][FreeWill] analyzeDirectResponse: Ответ похож на JSON-галлюцинацию после очистки — пропускаем")
			return
		}
		// is_voice: вероятностная проверка (используем конфиг, а не LLM)
		isVoice = fws.randSource.Float64() < fws.voiceProbability
		// mood: из текущего состояния
		responseMood = fws.getCurrentMoodName(chatID)
	}

	// Проверка на пустой ответ
	if strings.TrimSpace(responseText) == "" {
		log.Printf("[WARN][FreeWill] analyzeDirectResponse: Пустой ответ после очистки — пропускаем")
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
		Text:            responseText,
		IsVoice:         isVoice,
		Mood:            responseMood,
		Reason:          shouldReplyDecision.Reason,
	}

	log.Printf("[FreeWill] analyzeDirectResponse: 🎉 ИТОГОВОЕ РЕШЕНИЕ: текст='%s' голос=%t настроение=%s",
		finalDecision.Text, finalDecision.IsVoice, finalDecision.Mood)

	// Выполняем решение
	fws.executeDecision(chatID, finalDecision)

	// Обновляем статистику прямых обращений
	fws.updateDirectResponseStats(chatID, finalDecision)
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
		// Fix P12.4: если прошлый анализ провалился — используем уменьшенный кулдаун (3 мин)
		effectiveMinInterval := fws.minActivationInterval
		if fws.lastAnalysisFailed[chatID] {
			retryInterval := 3 * time.Minute
			if retryInterval < effectiveMinInterval {
				effectiveMinInterval = retryInterval
			}
			log.Printf("[FreeWill] shouldActivateAnalysis: Прошлый анализ провалился, используем уменьшенный кулдаун %v для чата %d", effectiveMinInterval, chatID)
		}
		if elapsed < effectiveMinInterval {
			log.Printf("[FreeWill] shouldActivateAnalysis: Слишком рано для активации чата %d (прошло %v, минимум %v)",
				chatID, elapsed, effectiveMinInterval)
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

	// Fix P12.4: отслеживаем успех/провал анализа
	var analysisErr error

	defer func() {
		fws.mutex.Lock()
		// КРИТИЧЕСКИ ВАЖНО: Сбрасываем флаг активного анализа
		delete(fws.activeAnalysis, chatID)
		log.Printf("[FreeWill] analyzeAndAct: 🔓 Флаг activeAnalysis сброшен для чата %d", chatID)

		activationTime := time.Now()
		fws.lastActivation[chatID] = activationTime
		// Fix P12.4: запоминаем, провалился ли анализ (для уменьшенного кулдауна)
		if analysisErr != nil {
			fws.lastAnalysisFailed[chatID] = true
		} else {
			fws.lastAnalysisFailed[chatID] = false
		}
		fws.mutex.Unlock()
		elapsed := time.Since(startTime)
		if analysisErr != nil {
			log.Printf("[FreeWill] analyzeAndAct: ❌ === ЗАВЕРШЕН АНАЛИЗ С ОШИБКОЙ === чат:%d время_выполнения:%v ошибка:%v",
				chatID, elapsed, analysisErr)
		} else {
			log.Printf("[FreeWill] analyzeAndAct: ✅ === ЗАВЕРШЕН АНАЛИЗ === чат:%d время_выполнения:%v активация_записана:%v",
				chatID, elapsed, activationTime.Format("15:04:05"))
		}
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
		analysisErr = fws.performAnalysis(chatID)
	}()

	// Ждем завершения или таймаута
	select {
	case <-done:
		if analysisErr != nil {
			log.Printf("[FreeWill] analyzeAndAct: ❌ Анализ для чата %d завершен с ошибкой: %v", chatID, analysisErr)
		} else {
			log.Printf("[FreeWill] analyzeAndAct: ✅ Анализ для чата %d завершен успешно", chatID)
		}
	case <-ctx.Done():
		analysisErr = fmt.Errorf("таймаут анализа после %v", analyzeTimeout)
		log.Printf("[FreeWill] analyzeAndAct: ⏰ ТАЙМАУТ анализа для чата %d после %v", chatID, analyzeTimeout)
	}
}

// performAnalysis выполняет основную логику анализа (без timeout logic)
func (fws *FreeWillService) performAnalysis(chatID int64) error {
	// Quick Rules — детерминированные проверки без LLM
	// Эти правила должны покрывать 60-80% всех решений
	if qrResult := fws.applyQuickRules(chatID); qrResult != nil && qrResult.Matched {
		log.Printf("[FreeWill] performAnalysis: ⚡ Quick Rule matched for chat %d: %s (should_reply=%t, type=%s)",
			chatID, qrResult.Reason, qrResult.ShouldReply, qrResult.ReplyType)

		if !qrResult.ShouldReply {
			log.Printf("[FreeWill] performAnalysis: ⚡ Quick Rule решил НЕ отвечать в чате %d", chatID)
			return nil
		}

		// Формируем финальное решение из Quick Rule
		finalDecision := &FreeWillDecision{
			ShouldReply: qrResult.ShouldReply,
			ReplyType:   qrResult.ReplyType,
			Reason:      qrResult.Reason,
			Mood:        qrResult.Mood,
		}
		fws.updateStats(chatID, finalDecision)
		fws.executeDecision(chatID, finalDecision)
		return nil
	}

	// Собираем TemplateData для шаблонизации промптов
	var stateData *prompts.TemplateData
	if fws.bot.stateProvider != nil {
		stateData = fws.bot.stateProvider.CollectState(chatID, 0)
	}

	// ЭТАП 1: Решение о необходимости ответа
	log.Printf("[FreeWill] performAnalysis: ЭТАП 1 - анализ необходимости ответа для чата %d", chatID)
	shouldReplyDecision, err := fws.decideShouldReply(chatID, stateData)
	if err != nil {
		log.Printf("[FreeWill] performAnalysis: Ошибка этапа 1 для чата %d: %v", chatID, err)
		return fmt.Errorf("ошибка этапа 1: %w", err)
	}

	if !shouldReplyDecision.ShouldReply {
		log.Printf("[FreeWill] performAnalysis: ЭТАП 1 - решено НЕ отвечать в чате %d (причина: %s)",
			chatID, shouldReplyDecision.Reason)
		return nil
	}

	log.Printf("[FreeWill] performAnalysis: ЭТАП 1 - решено отвечать в чате %d: type=%s, target_id=%d, причина: %s",
		chatID, shouldReplyDecision.ReplyType, shouldReplyDecision.TargetMessageID, shouldReplyDecision.Reason)

	// ЭТАП 2: Определение типа ответа с учетом voiceProbability
	log.Printf("[FreeWill] performAnalysis: ЭТАП 2 - определение типа ответа для чата %d", chatID)
	responseDecision, err := fws.decideResponseType(chatID, shouldReplyDecision)
	if err != nil {
		log.Printf("[FreeWill] performAnalysis: Ошибка этапа 2 для чата %d: %v", chatID, err)
		return fmt.Errorf("ошибка этапа 2: %w", err)
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
	return nil
}

// decideShouldReply - ЭТАП 1: решение о необходимости ответа
func (fws *FreeWillService) decideShouldReply(chatID int64, stateData *prompts.TemplateData) (*FreeWillShouldReplyDecision, error) {
	log.Printf("[FreeWill] decideShouldReply: Начинаем этап 1 для чата %d", chatID)

	// Получаем текущее настроение
	mood := fws.getCurrentMood(chatID)
	log.Printf("[FreeWill] decideShouldReply: Текущее настроение для чата %d: %s (интенсивность: %.2f)",
		chatID, mood.CurrentMood, mood.MoodIntensity)

	// Build ChatML context instead of string context
	chatMsgs := fws.buildResponseChatContext(chatID, fws.contextWindow, 0)
	if chatMsgs == nil {
		return nil, fmt.Errorf("ошибка получения контекста ChatML")
	}

	// Формируем промпт для первого этапа — используем шаблонизатор
	var prompt string
	if stateData != nil {
		rendered, renderErr := prompts.LoadAndRenderPrompt("free_will_should_reply", stateData)
		if renderErr != nil || rendered == "" {
			log.Printf("[FreeWill] decideShouldReply: ошибка рендеринга шаблона: %v, fallback на buildShouldReplyPrompt", renderErr)
			prompt = fws.buildShouldReplyPrompt("", mood) // empty context — ChatML handles it
		} else {
			prompt = rendered
		}
	} else {
		prompt = fws.buildShouldReplyPrompt("", mood)
	}
	// Prepend task prompt to system message
	chatMsgs[0].Content = prompt + "\n\n" + chatMsgs[0].Content

	log.Printf("[FreeWill] decideShouldReply: Промпт этапа 1 сформирован для чата %d (длина: %d символов)",
		chatID, len(prompt))
	log.Printf("[FreeWill] decideShouldReply: ChatML сообщений: %d", len(chatMsgs))
	log.Printf("[FreeWill] decideShouldReply: === ПОЛНЫЙ ПРОМПТ ЭТАПА 1 ДЛЯ ЧАТА %d ===\n%s\n=== КОНЕЦ ПРОМПТА ЭТАПА 1 ===", chatID, prompt)

	// Отправляем запрос к LLM (ChatML)
	log.Printf("[FreeWill] decideShouldReply: Отправляем запрос к LLM для чата %d", chatID)
	fws.bot.setTypingAction(chatID)
	llmStartTime := time.Now()
	response, err := fws.bot.llm.GenerateChatResponse(
		llm.ResponseTypeFreeWillShouldReply,
		chatMsgs,
		0.2, // CRITICAL: low temp prevents JSON breakage in decision stage
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

	// Стандартизованная строка резюме решения Этапа 1
	log.Printf("[INFO][FreeWill] ShouldReplyDecision: chat=%d, should_reply=%t, reply_type=%s, target_id=%d, reason=%q",
		chatID, decision.ShouldReply, decision.ReplyType, decision.TargetMessageID, decision.Reason)

	return decision, nil
}

// decideResponseType - ЭТАП 2: определение типа ответа
func (fws *FreeWillService) decideResponseType(chatID int64, shouldReplyDecision *FreeWillShouldReplyDecision) (*FreeWillResponseTypeDecision, error) {
	log.Printf("[FreeWill] decideResponseType: Начинаем этап 2 для чата %d (reply_type: %s)", chatID, shouldReplyDecision.ReplyType)

	// Build ChatML context for Stage 2
	chatMsgs := fws.buildResponseChatContext(chatID, fws.contextWindow, 0)
	if chatMsgs == nil {
		return nil, fmt.Errorf("ошибка получения контекста ChatML для этапа 2")
	}

	// Формируем промпт для второго этапа
	prompt := fws.buildResponseTypePrompt(shouldReplyDecision.ReplyType)
	chatMsgs[0].Content = prompt + "\n\n" + chatMsgs[0].Content

	log.Printf("[FreeWill] decideResponseType: Промпт этапа 2 сформирован для чата %d (длина: %d символов)",
		chatID, len(prompt))
	log.Printf("[FreeWill] decideResponseType: ChatML сообщений: %d", len(chatMsgs))
	log.Printf("[FreeWill] decideResponseType: === ПОЛНЫЙ ПРОМПТ ЭТАПА 2 ДЛЯ ЧАТА %d ===\n%s\n=== КОНЕЦ ПРОМПТА ЭТАПА 2 ===", chatID, prompt)

	// Отправляем запрос к LLM (ChatML)
	log.Printf("[FreeWill] decideResponseType: Отправляем запрос к LLM для чата %d", chatID)
	fws.bot.setTypingAction(chatID)
	llmStartTime := time.Now()
	response, err := fws.bot.llm.GenerateChatResponse(
		llm.ResponseTypeFreeWillResponseType,
		chatMsgs,
		0.2, // CRITICAL: low temp prevents JSON breakage in decision stage
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

// applyQuickRules — быстрые детерминированные проверки без LLM
func (fws *FreeWillService) applyQuickRules(chatID int64) *QuickRuleResult {
	if fws.bot.stateProvider == nil {
		return nil
	}

	stateData := fws.bot.stateProvider.CollectState(chatID, 0)

	if stateData.Presence != nil && stateData.Presence.Asleep && !stateData.Presence.NightAwake {
		return &QuickRuleResult{
			Matched: true, ShouldReply: false,
			Reason: "asleep",
		}
	}

	if stateData.Conflict != nil && stateData.Conflict.ColdActive {
		return &QuickRuleResult{
			Matched: true, ShouldReply: false,
			Reason: "conflict-cold",
		}
	}

	if stateData.Presence != nil && stateData.Presence.IsBusy && !stateData.Presence.Online {
		return &QuickRuleResult{
			Matched: true, ShouldReply: false,
			Reason: "busy",
		}
	}

	if stateData.Presence != nil && stateData.Presence.NightAwake {
		return &QuickRuleResult{
			Matched: true, ShouldReply: true,
			Reason:    "night-awake",
			ReplyType: "general",
			Mood:      "tired",
		}
	}

	return nil
}

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

// classifyMessageSeriousness — rule-based классификация сообщения как serious или casual
// Заменяет LLM-вызов в Этапе 1.5. Детерминированная, без сайд-эффектов.
func classifyMessageSeriousness(messageText string) string {
	if strings.TrimSpace(messageText) == "" {
		return "casual"
	}

	lower := strings.ToLower(messageText)
	runes := []rune(messageText)

	// 1. Ключевые маркеры срочности
	urgencyMarkers := []string{
		"серьёзно", "серьезно", "важно", "срочно",
		"серьезный вопрос", "серьёзный вопрос",
		"critical", "urgent", "important",
	}
	for _, marker := range urgencyMarkers {
		if strings.Contains(lower, marker) {
			return "serious"
		}
	}

	// 2. Прямые запросы информации (вопросительные слова + запросы данных)
	// Проверка "как" в значении вопроса — с исключением casual small-talk
	if strings.Contains(lower, "как ") {
		casualKakPhrases := []string{
			"как дела", "как ты", "как сам", "как жизнь",
			"как настроение", "как оно", "как день", "как поживаешь",
		}
		isCasualSmalltalk := false
		for _, cp := range casualKakPhrases {
			if strings.Contains(lower, cp) {
				isCasualSmalltalk = true
				break
			}
		}
		if !isCasualSmalltalk {
			return "serious"
		}
	}

	infoRequestMarkers := []string{
		"почему", "зачем", "что такое", "объясни", "расскажи", "опиши",
		// NOTE: "кто ", "где ", "когда " — trailing space отсекает "никто"/"кое-где",
		// но пропускает конец сообщения ("а это кто"). Компромисс precision/recall для rule-based.
		"сколько", "кто ", "где ", "когда ", "какой курс", "погода",
		"новости", "что значит", "в чем разница", "как работает",
		"what is", "how to", "explain", "describe", "how much",
		"weather", "news", "definition of", "what does", "how do",
		"как сделать", "как найти", "где найти",
		"курс доллара", "курс евро", "курс валют", "сколько стоит",
	}
	for _, marker := range infoRequestMarkers {
		if strings.Contains(lower, marker) {
			return "serious"
		}
	}

	// 3. Просьбы о помощи/совете
	helpMarkers := []string{
		"помоги", "нужна помощь", "посоветуй", "порекомендуй",
		"help", "advice", "recommend", "suggest",
		"can you help", "could you help", "подскажи",
		"как лучше", "что лучше", "что посоветуешь",
	}
	for _, marker := range helpMarkers {
		if strings.Contains(lower, marker) {
			return "serious"
		}
	}

	// 4. Длинный вопрос со знаком вопроса
	if len(runes) > 100 && strings.Contains(messageText, "?") {
		return "serious"
	}

	// 5. Всё остальное — casual
	return "casual"
}
