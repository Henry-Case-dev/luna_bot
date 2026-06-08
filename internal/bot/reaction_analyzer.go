package bot

import (
	"fmt"
	"log"
	"strings"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
)

// ReactionAnalyzer анализирует реакции на сообщения бота
type ReactionAnalyzer struct {
	bot *Bot
}

// NewReactionAnalyzer создает новый анализатор реакций
func NewReactionAnalyzer(bot *Bot) *ReactionAnalyzer {
	return &ReactionAnalyzer{
		bot: bot,
	}
}

// MessageQuality представляет качество сообщения на основе реакций
type MessageQuality string

const (
	QualityGood    MessageQuality = "good"
	QualityBad     MessageQuality = "bad"
	QualityNeutral MessageQuality = "neutral"
)

// AnalyzeMessage анализирует качество сообщения на основе реакций
func (ra *ReactionAnalyzer) AnalyzeMessage(chatID int64, messageID int, messageText string) (MessageQuality, error) {
	// Получаем реакции на сообщение
	reactions, err := ra.bot.storage.GetMessageReactions(chatID, messageID)
	if err != nil {
		return QualityNeutral, fmt.Errorf("ошибка получения реакций: %w", err)
	}

	if len(reactions) == 0 {
		return QualityNeutral, nil // Нет реакций
	}

	// Классифицируем реакции
	positiveCount := 0
	negativeCount := 0

	for _, emoji := range reactions {
		switch emoji {
		case "😂", "🤣", "😁", "😄", "😃", "👍", "❤️", "🔥", "💯", "👏", "🎉":
			positiveCount++
		case "🤡", "👎", "💩", "🤮", "😡", "🙄":
			negativeCount++
			// Нейтральные: "🤔", "😐" и другие не учитываем
		}
	}

	// Анализируем LLM'ом для более точной оценки
	reactionContext := fmt.Sprintf("Реакции на сообщение: %s", strings.Join(reactions, ", "))

	prompt := ra.bot.enrichPromptWithPersonality(ra.bot.config.ReactionAnalysisPrompt, chatID, "reaction_analysis")
	analysis, err := ra.bot.llm.GenerateResponseByType(llm.ResponseTypeReactionAnalysis, prompt, reactionContext, float32(ra.bot.config.GeminiTemperatureNormal))
	if err != nil {
		log.Printf("[ERROR][ReactionAnalyzer] Ошибка анализа LLM: %v", err)
		// Fallback к простому подсчету
		if positiveCount > negativeCount {
			return QualityGood, nil
		} else if negativeCount > positiveCount {
			return QualityBad, nil
		}
		return QualityNeutral, nil
	}

	// Парсим ответ LLM
	analysis = strings.TrimSpace(strings.ToLower(analysis))

	if strings.Contains(analysis, "good") {
		return QualityGood, nil
	} else if strings.Contains(analysis, "bad") {
		return QualityBad, nil
	}

	return QualityNeutral, nil
}

// ProcessMessageForPersonality обрабатывает сообщение для добавления в память личности
func (ra *ReactionAnalyzer) ProcessMessageForPersonality(chatID int64, messageID int, messageText string, quality MessageQuality) error {
	if messageText == "" {
		return nil // Пустое сообщение не обрабатываем
	}

	switch quality {
	case QualityGood:
		// ВРЕМЕННО ОТКЛЮЧЕНО: AddPositiveExample может вызывать ошибки 429 из-за больших документов
		// personalityEntry := fmt.Sprintf("✅ Хорошее сообщение: %s", messageText)
		// err := ra.bot.storage.AddPositiveExample(chatID, personalityEntry, time.Now())
		// if err != nil {
		// 	log.Printf("[ERROR][ReactionAnalyzer] Ошибка добавления позитивного примера: %v", err)
		// }
		log.Printf("[ReactionAnalyzer] Позитивное сообщение определено (но не сохранено): %.50s...", messageText)

	case QualityBad:
		// ВРЕМЕННО ОТКЛЮЧЕНО: AddNegativeExample может вызывать ошибки 429 из-за больших документов
		// personalityEntry := fmt.Sprintf("❌ Плохое сообщение: %s", messageText)
		// err := ra.bot.storage.AddNegativeExample(chatID, personalityEntry, time.Now())
		// if err != nil {
		// 	log.Printf("[ERROR][ReactionAnalyzer] Ошибка добавления негативного примера: %v", err)
		// }
		log.Printf("[ReactionAnalyzer] Негативное сообщение определено (но не сохранено): %.50s...", messageText)
	}

	return nil
}

// AnalyzeAndStore анализирует сообщение и сохраняет результат
func (ra *ReactionAnalyzer) AnalyzeAndStore(chatID int64, messageID int, messageText string) error {
	quality, err := ra.AnalyzeMessage(chatID, messageID, messageText)
	if err != nil {
		return fmt.Errorf("ошибка анализа сообщения: %w", err)
	}

	if quality != QualityNeutral {
		err = ra.ProcessMessageForPersonality(chatID, messageID, messageText, quality)
		if err != nil {
			return fmt.Errorf("ошибка обработки для личности: %w", err)
		}
	}

	return nil
}

// AnalyzeBotMessagesWithReactions анализирует все сообщения бота с реакциями за последний период
func (ra *ReactionAnalyzer) AnalyzeBotMessagesWithReactions(chatID int64, lookbackHours int) error {
	// Получаем недавние сообщения бота
	botMessages, err := ra.bot.storage.GetBotMessagesWithReactions(chatID, lookbackHours)
	if err != nil {
		return fmt.Errorf("ошибка получения сообщений бота: %w", err)
	}

	processedCount := 0
	for _, msg := range botMessages {
		err = ra.AnalyzeAndStore(chatID, msg.MessageID, msg.Text)
		if err != nil {
			log.Printf("[ERROR][ReactionAnalyzer] Ошибка анализа сообщения %d: %v", msg.MessageID, err)
			continue
		}
		processedCount++
	}

	if ra.bot.config.Debug && processedCount > 0 {
		log.Printf("[DEBUG][ReactionAnalyzer] Проанализировано %d сообщений бота в чате %d", processedCount, chatID)
	}

	return nil
}
