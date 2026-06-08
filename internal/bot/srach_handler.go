package bot

import (
	"log"
	"strings"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
)

// toggleSrachAnalysis переключает состояние анализа срачей для чата
func (b *Bot) toggleSrachAnalysis(chatID int64) (bool, error) {
	// 1. Получаем текущие настройки из БД
	dbSettings, err := b.storage.GetChatSettings(chatID)
	if err != nil {
		log.Printf("[ERROR][toggleSrachAnalysis] Chat %d: Не удалось получить настройки: %v", chatID, err)
		return false, err
	}

	// 2. Определяем текущее и новое состояние
	currentEnabled := b.config.SrachAnalysisEnabled // Дефолт из конфига
	if dbSettings.SrachAnalysisEnabled != nil {
		currentEnabled = *dbSettings.SrachAnalysisEnabled
	}
	newEnabled := !currentEnabled

	// 3. Обновляем настройку в хранилище
	errUpdate := b.storage.UpdateSrachAnalysisEnabled(chatID, newEnabled)
	if errUpdate != nil {
		log.Printf("[ERROR][toggleSrachAnalysis] Chat %d: Не удалось обновить настройку: %v", chatID, errUpdate)
		return currentEnabled, errUpdate // Возвращаем старое значение и ошибку
	}

	log.Printf("Чат %d: Анализ срачей переключен на %s", chatID, getEnabledStatusText(newEnabled))

	// 4. Сбрасываем состояние срача в памяти, если настройка была изменена
	b.settingsMutex.Lock()
	if settings, exists := b.chatSettings[chatID]; exists {
		settings.SrachAnalysisEnabled = newEnabled // Обновляем и в памяти для консистентности
		settings.SrachState = "none"
		settings.SrachMessages = nil
	}
	b.settingsMutex.Unlock()

	return newEnabled, nil
}

// GenerateSrachAnalysis генерирует анализ срача для чата
func (b *Bot) GenerateSrachAnalysis(chatID int64) {
	// Получаем настройки чата
	b.settingsMutex.Lock()
	settings, exists := b.chatSettings[chatID]
	var srachMessages []string
	if exists && settings.SrachMessages != nil {
		srachMessages = settings.SrachMessages
	}
	b.settingsMutex.Unlock()

	if len(srachMessages) == 0 {
		log.Printf("[WARN][Srach Analysis] Chat %d: Нет сообщений срача для анализа", chatID)
		return
	}

	// Объединяем все сообщения срача в единый текст
	formattedSrach := strings.Join(srachMessages, "\n")

	// Генерируем анализ с помощью LLM
	analysisPrompt := b.config.SRACH_ANALYSIS_PROMPT
	analysis, err := b.llm.GenerateResponseByType(llm.ResponseTypeSrach, analysisPrompt, formattedSrach, float32(b.config.GeminiTemperatureSerious))

	if err != nil {
		log.Printf("[ERROR][Srach Analysis] Chat %d: Не удалось сгенерировать анализ срача: %v", chatID, err)
		return
	}

	// Очищаем анализ от возможных метаданных
	analysis = cleanupLLMResponse(analysis)

	// Отправляем анализ в чат
	b.sendReply(chatID, analysis)
}
