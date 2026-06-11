package gemini

import (
	"fmt"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	"github.com/Henry-Case-dev/luna_bot/internal/llm"
)

// NewProvider — фабрика провайдера Gemini для ProviderRegistry.
// Принимает llm.ProviderConfig и создаёт настроенного клиента Gemini.
func NewProvider(cfg llm.ProviderConfig) (llm.Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("gemini: api_key is required")
	}

	modelName := "gemini-2.0-flash"
	embeddingModel := "embedding-001"
	debug := cfg.Debug

	if cfg.Extra != nil {
		if v, ok := cfg.Extra["model_name"].(string); ok && v != "" {
			modelName = v
		}
		if v, ok := cfg.Extra["embedding_model"].(string); ok && v != "" {
			embeddingModel = v
		}
	}

	// Создаём минимальный Config для New()
	c := &config.Config{
		GeminiAPIKey:                 cfg.APIKey,
		AudioTranscriptionModel:      "gemini-2.0-flash",
		ImageGenerationModel:         "gemini-2.5-flash-image-preview",
		ImageGenerationTemperature:   1.0,
		GeminiBypassSafetyFilters:    true,
		GeminiKeyRotationTimeHours:   1,
	}

	client, err := New(c, modelName, embeddingModel, debug)
	if err != nil {
		return nil, fmt.Errorf("gemini: failed to create client: %w", err)
	}

	return client, nil
}
