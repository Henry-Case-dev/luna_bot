package llm

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

// LLMClient — композитный интерфейс.
// Объединяет все capability-интерфейсы в один контракт.
type LLMClient interface {
	TextGenerator
	AudioTranscriber
	Embedder
	ImageAnalyzer
	ImageGenerator
	AudioGenerator
	Closer
}
