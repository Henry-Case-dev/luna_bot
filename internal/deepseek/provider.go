package deepseek

import (
	"fmt"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
)

// NewProvider — фабрика провайдера DeepSeek для ProviderRegistry.
// Принимает llm.ProviderConfig и создаёт настроенного клиента DeepSeek.
func NewProvider(cfg llm.ProviderConfig) (llm.Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("deepseek: api_key is required")
	}

	modelName := "deepseek-chat"
	baseURL := "https://api.deepseek.com/v1"
	debug := cfg.Debug

	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}
	if cfg.Extra != nil {
		if v, ok := cfg.Extra["model_name"].(string); ok && v != "" {
			modelName = v
		}
	}

	client, err := New(cfg.APIKey, modelName, baseURL, debug)
	if err != nil {
		return nil, fmt.Errorf("deepseek: failed to create client: %w", err)
	}

	return client, nil
}
