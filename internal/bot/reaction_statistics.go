package bot

import (
	"log"
	"sync"
	"time"
)

// ReactionStatistics собирает статистику обработки реакций
type ReactionStatistics struct {
	mutex               sync.RWMutex
	totalReactions      int64
	clownReactions      int64
	clownResponsesSent  int64
	botMessagesFound    int64
	botMessagesNotFound int64
	dbErrors            int64
	lastClownReaction   time.Time
	lastClownResponse   time.Time
	startTime           time.Time
}

// NewReactionStatistics создает новый счетчик статистики
func NewReactionStatistics() *ReactionStatistics {
	return &ReactionStatistics{
		startTime: time.Now(),
	}
}

// IncrementTotalReactions увеличивает счетчик всех реакций
func (rs *ReactionStatistics) IncrementTotalReactions() {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()
	rs.totalReactions++
}

// IncrementClownReactions увеличивает счетчик реакций клоуна
func (rs *ReactionStatistics) IncrementClownReactions() {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()
	rs.clownReactions++
	rs.lastClownReaction = time.Now()
	log.Printf("[ReactionStats] КЛОУН #%d обнаружен в %s", rs.clownReactions, rs.lastClownReaction.Format("15:04:05"))
}

// IncrementClownResponsesSent увеличивает счетчик отправленных ответов на клоуна
func (rs *ReactionStatistics) IncrementClownResponsesSent() {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()
	rs.clownResponsesSent++
	rs.lastClownResponse = time.Now()
	log.Printf("[ReactionStats] ОТВЕТ НА КЛОУНА #%d отправлен в %s", rs.clownResponsesSent, rs.lastClownResponse.Format("15:04:05"))
}

// IncrementBotMessagesFound увеличивает счетчик найденных сообщений бота
func (rs *ReactionStatistics) IncrementBotMessagesFound() {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()
	rs.botMessagesFound++
}

// IncrementBotMessagesNotFound увеличивает счетчик не найденных сообщений бота
func (rs *ReactionStatistics) IncrementBotMessagesNotFound() {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()
	rs.botMessagesNotFound++
}

// IncrementDBErrors увеличивает счетчик ошибок БД
func (rs *ReactionStatistics) IncrementDBErrors() {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()
	rs.dbErrors++
}

// LogCurrentStats выводит текущую статистику
func (rs *ReactionStatistics) LogCurrentStats() {
	rs.mutex.RLock()
	defer rs.mutex.RUnlock()

	uptime := time.Since(rs.startTime).Round(time.Second)

	log.Printf("[ReactionStats] === СТАТИСТИКА РЕАКЦИЙ ===")
	log.Printf("[ReactionStats] Время работы: %v", uptime)
	log.Printf("[ReactionStats] Всего реакций: %d", rs.totalReactions)
	log.Printf("[ReactionStats] Реакций клоуна: %d", rs.clownReactions)
	log.Printf("[ReactionStats] Ответов на клоуна отправлено: %d", rs.clownResponsesSent)
	log.Printf("[ReactionStats] Сообщений бота найдено: %d", rs.botMessagesFound)
	log.Printf("[ReactionStats] Сообщений бота НЕ найдено: %d", rs.botMessagesNotFound)
	log.Printf("[ReactionStats] Ошибок БД: %d", rs.dbErrors)

	if rs.clownReactions > 0 {
		successRate := float64(rs.clownResponsesSent) / float64(rs.clownReactions) * 100
		log.Printf("[ReactionStats] Успешность ответов на клоуна: %.1f%%", successRate)
	}

	if !rs.lastClownReaction.IsZero() {
		log.Printf("[ReactionStats] Последний клоун: %s", rs.lastClownReaction.Format("15:04:05"))
	}

	if !rs.lastClownResponse.IsZero() {
		log.Printf("[ReactionStats] Последний ответ: %s", rs.lastClownResponse.Format("15:04:05"))
	}

	log.Printf("[ReactionStats] ========================")
}

// CheckForLostClowns проверяет, есть ли потерянные клоуны (клоуны без ответов)
func (rs *ReactionStatistics) CheckForLostClowns() {
	rs.mutex.RLock()
	defer rs.mutex.RUnlock()

	if rs.clownReactions > rs.clownResponsesSent {
		lost := rs.clownReactions - rs.clownResponsesSent
		log.Printf("[ReactionStats] ⚠️ ПОТЕРЯНО %d КЛОУНОВ! (всего: %d, ответов: %d)",
			lost, rs.clownReactions, rs.clownResponsesSent)
	}
}

// StartPeriodicLogging запускает периодическое логирование статистики
func (rs *ReactionStatistics) StartPeriodicLogging(stop <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Minute) // Каждые 5 минут
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rs.LogCurrentStats()
			rs.CheckForLostClowns()
		case <-stop:
			log.Println("[ReactionStats] Остановка периодического логирования статистики")
			return
		}
	}
}
