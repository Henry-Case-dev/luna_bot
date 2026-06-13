package llm

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TextGenerator — генерация текстовых ответов.
// Реализуют: Gemini, DeepSeek, OpenRouter, Local (Ollama).
type TextGenerator interface {
	// GenerateResponse генерирует ответ на основе истории сообщений и системного промпта.
	GenerateResponse(systemPrompt string, history []*tgbotapi.Message, lastMessage *tgbotapi.Message, temperature float32) (string, error)

	// GenerateResponseFromTextContext — генерация из предварительно отформатированного контекста.
	GenerateResponseFromTextContext(systemPrompt string, contextText string, temperature float32) (string, error)

	// GenerateArbitraryResponse — генерация без истории чата (анализ, саммари).
	GenerateArbitraryResponse(systemPrompt string, contextText string, temperature float32) (string, error)

	// GenerateResponseByType — генерация с маршрутизацией по ResponseType.
	GenerateResponseByType(responseType ResponseType, systemPrompt string, contextText string, temperature float32) (string, error)

	// GenerateChatResponse — генерация из ChatML message array.
	// Принимает массив сообщений с корректными ролями (system/user/assistant).
	GenerateChatResponse(responseType ResponseType, messages []ChatMessage, temperature float32) (string, error)
}

// AudioTranscriber — транскрибация аудио в текст.
// Реализуют: Gemini.
type AudioTranscriber interface {
	TranscribeAudio(audioData []byte, mimeType string) (string, error)
}

// Embedder — генерация векторных представлений текста.
// Реализуют: Gemini, Local (с embedding-моделями).
type Embedder interface {
	EmbedContent(text string) ([]float32, error)
}

// ImageAnalyzer — анализ изображений (текстовое описание).
// Реализуют: Gemini.
type ImageAnalyzer interface {
	GenerateContentWithImage(ctx context.Context, systemPrompt string, imageData []byte, caption string) (string, error)
}

// ImageGenerator — генерация/редактирование изображений.
// Реализуют: Gemini.
type ImageGenerator interface {
	GenerateImageWithEdit(ctx context.Context, baseImageData []byte, editPrompt string) ([]byte, error)
}

// AudioGenerator — синтез речи (Text-to-Speech).
// Реализуют: Gemini (TTS), ElevenLabs.
type AudioGenerator interface {
	GenerateAudio(text string, params AudioParams) ([]byte, error)
}

// AudioParams — параметры для генерации аудио.
type AudioParams struct {
	VoiceName   string  // Имя голоса (для Gemini TTS)
	Model       string  // Модель TTS
	Temperature float32 // Температура генерации
	// Провайдер-специфичные параметры передаются через Extra
	Extra map[string]any
}

// Closer — освобождение ресурсов клиента.
// Реализуют: все провайдеры.
type Closer interface {
	Close() error
}
