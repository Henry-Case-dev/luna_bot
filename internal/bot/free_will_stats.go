package bot

import (
	"log"
	"time"
)

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
