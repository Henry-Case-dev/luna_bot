package bot

import (
	"log"
	"time"
)

// ReactionPoller получает обновления с реакциями параллельно с основным циклом
type ReactionPoller struct {
	bot          *Bot
	reactionsAPI *TelegramReactionsAPI
	offset       int
	running      bool
	stopChan     chan struct{}
}

// NewReactionPoller создает новый экземпляр ReactionPoller
func NewReactionPoller(bot *Bot) *ReactionPoller {
	reactionsAPI := NewTelegramReactionsAPI(bot.config.TelegramToken, bot.config.Debug)

	return &ReactionPoller{
		bot:          bot,
		reactionsAPI: reactionsAPI,
		offset:       0,
		stopChan:     make(chan struct{}),
	}
}

// Start запускает поллер реакций
func (rp *ReactionPoller) Start() {
	if !rp.bot.config.ReactionsEnabled {
		log.Println("[ReactionPoller] Реакции отключены, поллер не запускается")
		return
	}

	rp.running = true
	log.Println("[ReactionPoller] Запуск поллера реакций...")

	go rp.pollLoop()
}

// Stop останавливает поллер реакций
func (rp *ReactionPoller) Stop() {
	if !rp.running {
		return
	}

	log.Println("[ReactionPoller] Остановка поллера реакций...")
	rp.running = false
	close(rp.stopChan)
}

// pollLoop основной цикл получения обновлений с реакциями
func (rp *ReactionPoller) pollLoop() {
	for rp.running {
		select {
		case <-rp.stopChan:
			log.Println("[ReactionPoller] Получен сигнал остановки")
			return
		default:
			rp.pollOnce()
		}
	}
}

// pollOnce выполняет один цикл получения обновлений
func (rp *ReactionPoller) pollOnce() {
	updates, err := rp.reactionsAPI.GetUpdatesWithReactions(rp.offset, 60)
	if err != nil {
		if rp.bot.config.Debug {
			log.Printf("[ReactionPoller] Ошибка получения обновлений: %v", err)
		}
		// Ждем перед повторной попыткой
		time.Sleep(5 * time.Second)
		return
	}

	for _, reactionUpdate := range updates {
		if reactionUpdate.MessageReaction != nil {
			// Конвертируем в стандартный Update
			standardUpdate := rp.reactionsAPI.ConvertToStandardUpdate(reactionUpdate)
			if standardUpdate != nil {
				// Обрабатываем как обычное обновление
				go rp.bot.handleUpdate(*standardUpdate)

				// Обновляем offset
				// Для реакций используем timestamp как ID
				if reactionUpdate.MessageReaction.Date > int64(rp.offset) {
					rp.offset = int(reactionUpdate.MessageReaction.Date) + 1
				}
			}
		}
	}

	// Небольшая пауза между запросами
	time.Sleep(1 * time.Second)
}

// GetOffset возвращает текущий offset
func (rp *ReactionPoller) GetOffset() int {
	return rp.offset
}

// SetOffset устанавливает offset
func (rp *ReactionPoller) SetOffset(offset int) {
	rp.offset = offset
}
