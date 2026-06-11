package anthropic

import (
	"fmt"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
)

func NewProvider(cfg llm.ProviderConfig) (llm.Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("anthropic: api_key is required")
	}

	modelName := "claude-3-5-sonnet-20241022"
	debug := cfg.Debug

	if cfg.Extra != nil {
		if v, ok := cfg.Extra["model_name"].(string); ok && v != "" {
			modelName = v
		}
	}

	client, err := New(cfg.APIKey, modelName, debug)
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to create client: %w", err)
	}

	return client, nil
}
