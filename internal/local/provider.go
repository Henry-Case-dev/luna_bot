package local

import (
	"github.com/Henry-Case-dev/luna_bot/internal/llm"
)

func NewProvider(cfg llm.ProviderConfig) (llm.Provider, error) {
	modelName := "llama3.1:8b"
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434/v1"
	}
	if cfg.Extra != nil {
		if v, ok := cfg.Extra["model_name"].(string); ok && v != "" {
			modelName = v
		}
	}
	return New(baseURL, modelName, cfg.APIKey, cfg.Debug)
}
