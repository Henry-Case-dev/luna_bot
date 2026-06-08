package llm

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ResponseType определяет тип ответа для выбора подходящей модели LLM
type ResponseType string

const (
	// Основные типы ответов
	ResponseTypeDefault             ResponseType = "default"              // Обычный ответ в чат
	ResponseTypeDirect              ResponseType = "direct"               // Прямое обращение к боту
	ResponseTypeDirectSerious       ResponseType = "direct_serious"       // Серьезный прямой ответ
	ResponseTypeSerious             ResponseType = "serious"              // Серьезный ответ на вопрос
	ResponseTypeClassify            ResponseType = "classify"             // Классификация сообщений
	ResponseTypeDailyTake           ResponseType = "daily_take"           // Ежедневная тема
	ResponseTypeSummary             ResponseType = "summary"              // Саммари чата
	ResponseTypeWeeklySummary       ResponseType = "weekly_summary"       // Еженедельное саммари
	ResponseTypeVoiceMessage        ResponseType = "voice_message"        // Голосовые сообщения
	ResponseTypeVoiceFormat         ResponseType = "voice_format"         // Форматирование голоса
	ResponseTypeWelcome             ResponseType = "welcome"              // Приветствие
	ResponseTypePhoto               ResponseType = "photo"                // Анализ фотографий
	ResponseTypeAutoBio             ResponseType = "auto_bio"             // Анализ профилей пользователей
	ResponseTypeAutoBioInitial      ResponseType = "auto_bio_initial"     // Первичный анализ AutoBio
	ResponseTypeAutoBioUpdate       ResponseType = "auto_bio_update"      // Обновление профилей
	ResponseTypeSrach               ResponseType = "srach"                // Анализ срачей
	ResponseTypeRateLimit           ResponseType = "rate_limit"           // Сообщения о превышении лимитов
	ResponseTypeDonate              ResponseType = "donate"               // Просьбы о донатах
	ResponseTypePersonalityAnalysis ResponseType = "personality_analysis" // Анализ личности
	ResponseTypePersonalityName     ResponseType = "personality_name"     // Анализ имен
	ResponseTypePersonalityTopic    ResponseType = "personality_topic"    // Анализ тем
	ResponseTypePersonalitySelf     ResponseType = "personality_self"     // Обновление самовосприятия
	ResponseTypeWebSearch           ResponseType = "web_search"           // Поисковые запросы
	ResponseTypeClownReaction       ResponseType = "clown_reaction"       // Реакции на клоунов
	ResponseTypeReactionAnalysis    ResponseType = "reaction_analysis"    // Анализ реакций
	ResponseTypeAntiRepetition      ResponseType = "anti_repetition"      // Переработка повторений
	ResponseTypeModeration          ResponseType = "moderation"           // Модерация сообщений

	// Free Will типы
	ResponseTypeFreeWillShouldReply            ResponseType = "free_will_should_reply"             // Free Will: решение о необходимости ответа
	ResponseTypeFreeWillResponseType           ResponseType = "free_will_response_type"            // Free Will: определение типа ответа
	ResponseTypeFreeWillDirect                 ResponseType = "free_will_direct"                   // Free Will: прямой ответ
	ResponseTypeFreeWillDirectResponseDecision ResponseType = "free_will_direct_response_decision" // Free Will: решение о прямых обращениях (Этап 1)
	ResponseTypeFreeWillDirectResponse         ResponseType = "free_will_direct_response"          // Free Will: генерация ответа на прямые обращения (Этап 2)
	ResponseTypeFreeWillGeneral                ResponseType = "free_will_general"                  // Free Will: общее сообщение
	ResponseTypeFreeWillContext                ResponseType = "free_will_context"                  // Free Will: контекстный ответ
	ResponseTypeFreeWillSilence                ResponseType = "free_will_silence"                  // Free Will: ответ на тишину
	ResponseTypeFreeWillMoodAnalysis           ResponseType = "free_will_mood_analysis"            // Free Will: анализ настроения
	ResponseTypeFreeWillMoodBasedMessage       ResponseType = "free_will_mood_based"               // Free Will: сообщения по настроению
	ResponseTypeFreeWillVoiceMessage           ResponseType = "free_will_voice"                    // Free Will: голосовые сообщения
	ResponseTypeFreeWillTakeResponse           ResponseType = "free_will_take_response"            // Free Will: ответ на тейк
	ResponseTypeFreeWillReaction               ResponseType = "free_will_reaction"                 // Free Will: выбор реакций

	// Message Post-Processor типы
	ResponseTypePostProcessSingleWord  ResponseType = "post_process_single"      // Постобработка: одно слово
	ResponseTypePostProcessShort       ResponseType = "post_process_short"       // Постобработка: короткие предложения
	ResponseTypePostProcessLong        ResponseType = "post_process_long"        // Постобработка: длинные сообщения
	ResponseTypePostProcessIntelligent ResponseType = "post_process_intelligent" // Постобработка: интеллектуальный режим
	ResponseTypePostProcessSummary     ResponseType = "post_process_summary"     // Постобработка: саммари

	// New constants from the code block
	ResponseTypePhotoAnalysis        ResponseType = "photo_analysis"         // Анализ фотографий
	ResponseTypeWebSearchTrigger     ResponseType = "web_search_trigger"     // Триггер веб-поиска
	ResponseTypeAntiRepetitionRework ResponseType = "anti_repetition_rework" // Переработка повторяющегося сообщения

	// Каузальное обучение (Этап 1)
	ResponseTypeCausalAnalysis  ResponseType = "causal_analysis"  // Анализ причинно-следственных связей
	ResponseTypeCausalInfluence ResponseType = "causal_influence" // Извлечение влияния каузальной памяти на поведение

	// Эмоциональная система (Этап 2)
	ResponseTypeEmotionalAnalysis   ResponseType = "emotional_analysis"   // Анализ эмоций пользователей
	ResponseTypeEmotionalAdaptation ResponseType = "emotional_adaptation" // Адаптация стиля общения
	ResponseTypeEmotionalFeedback   ResponseType = "emotional_feedback"   // Анализ эмоциональной обратной связи

	// Система убеждений (Этап 1 — анализ убеждений)
	ResponseTypeBeliefAnalysis ResponseType = "belief_analysis" // Анализ и обновление системы убеждений
)

// LLMClient определяет общий интерфейс для взаимодействия с различными LLM.
type LLMClient interface {
	// GenerateResponse генерирует ответ на основе истории сообщений и системного промпта.
	// history - это сообщения ДО текущего lastMessage.
	// lastMessage - это последнее сообщение пользователя, на которое нужно сгенерировать ответ.
	// temperature - температура для генерации.
	// DEPRECATED: Используйте GenerateResponseFromTextContext для включения профилей.
	GenerateResponse(systemPrompt string, history []*tgbotapi.Message, lastMessage *tgbotapi.Message, temperature float32) (string, error)

	// GenerateResponseFromTextContext генерирует ответ на основе системного промпта и предварительно отформатированного текстового контекста.
	// contextText должен содержать всю необходимую информацию, включая историю сообщений и данные профилей.
	// temperature - температура для генерации.
	GenerateResponseFromTextContext(systemPrompt string, contextText string, temperature float32) (string, error)

	// GenerateArbitraryResponse генерирует ответ на основе системного промпта и произвольного текстового контекста.
	// Используется для задач, не требующих истории чата (например, анализ срача, саммари без профилей).
	// temperature - температура для генерации.
	GenerateArbitraryResponse(systemPrompt string, contextText string, temperature float32) (string, error)

	// GenerateResponseByType генерирует ответ используя оптимальную модель для указанного типа ответа.
	// Автоматически выбирает провайдера и модель на основе конфигурации для данного типа.
	// responseType - тип ответа для выбора подходящей модели.
	// systemPrompt - системный промпт.
	// contextText - контекст или произвольный текст.
	// temperature - температура для генерации (может быть переопределена в конфигурации типа).
	GenerateResponseByType(responseType ResponseType, systemPrompt string, contextText string, temperature float32) (string, error)

	// TranscribeAudio транскрибирует аудиоданные.
	// Возвращает распознанный текст и ошибку.
	TranscribeAudio(audioData []byte, mimeType string) (string, error)

	// EmbedContent генерирует векторное представление (эмбеддинг) для заданного текста.
	EmbedContent(text string) ([]float32, error)

	// GenerateContentWithImage генерирует ответ на основе изображения и текстового промпта.
	// Возвращает текстовое описание изображения и ошибку.
	GenerateContentWithImage(ctx context.Context, systemPrompt string, imageData []byte, caption string) (string, error)

	// GenerateImageWithEdit генерирует изображение на основе базового изображения и промпта для редактирования.
	// Возвращает данные сгенерированного изображения и ошибку.
	GenerateImageWithEdit(ctx context.Context, baseImageData []byte, editPrompt string) ([]byte, error)

	// Close освобождает ресурсы, связанные с клиентом (если необходимо).
	Close() error
}
