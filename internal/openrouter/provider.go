package openrouter

import (
	"fmt"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	"github.com/Henry-Case-dev/luna_bot/internal/llm"
)

// NewProvider — фабрика провайдера OpenRouter для ProviderRegistry.
// Принимает llm.ProviderConfig и создаёт настроенного клиента OpenRouter.
func NewProvider(cfg llm.ProviderConfig) (llm.Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("openrouter: api_key is required")
	}

	modelName := "deepseek/deepseek-chat-v3.1:free"
	siteURL := ""
	siteTitle := ""

	if cfg.Extra != nil {
		if v, ok := cfg.Extra["model_name"].(string); ok && v != "" {
			modelName = v
		}
		if v, ok := cfg.Extra["site_url"].(string); ok {
			siteURL = v
		}
		if v, ok := cfg.Extra["site_title"].(string); ok {
			siteTitle = v
		}
	}

	// Создаём минимальный Config для New()
	c := &config.Config{
		Debug: cfg.Debug,
	}

	client, err := New(cfg.APIKey, modelName, siteURL, siteTitle, c)
	if err != nil {
		return nil, fmt.Errorf("openrouter: failed to create client: %w", err)
	}

	return client, nil
}
