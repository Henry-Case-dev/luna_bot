package openai

import (
	"fmt"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
)

func NewProvider(cfg llm.ProviderConfig) (llm.Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("openai: api_key is required")
	}

	modelName := "gpt-4o"
	baseURL := cfg.BaseURL
	debug := cfg.Debug

	if cfg.Extra != nil {
		if v, ok := cfg.Extra["model_name"].(string); ok && v != "" {
			modelName = v
		}
	}

	client, err := New(cfg.APIKey, modelName, baseURL, debug)
	if err != nil {
		return nil, fmt.Errorf("openai: failed to create client: %w", err)
	}

	return client, nil
}
