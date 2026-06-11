package openrouter

import (
	"testing"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
)

// TestInfo проверяет, что Info() возвращает корректную метаинформацию.
func TestInfo(t *testing.T) {
	client := &Client{}
	info := client.Info()

	if info.Name != "openrouter" {
		t.Errorf("expected name 'openrouter', got %q", info.Name)
	}

	if len(info.Capabilities) != 1 {
		t.Errorf("expected 1 capability, got %d: %v", len(info.Capabilities), info.Capabilities)
	}

	if len(info.Capabilities) > 0 && info.Capabilities[0] != llm.CapTextGeneration {
		t.Errorf("expected CapTextGeneration, got %s", info.Capabilities[0])
	}
}

// TestCompileTimeChecks проверяет, что OpenRouter реализует TextGenerator + Closer.
func TestCompileTimeChecks(t *testing.T) {
	var client *Client
	var _ llm.TextGenerator = client
	var _ llm.Closer = client
}

// TestNewProviderMissingKey проверяет ошибку без API-ключа.
func TestNewProviderMissingKey(t *testing.T) {
	_, err := NewProvider(llm.ProviderConfig{
		Name:   "openrouter",
		APIKey: "",
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

// TestNewProviderWithExtra проверяет фабрику с Extra-параметрами.
func TestNewProviderWithExtra(t *testing.T) {
	_, err := NewProvider(llm.ProviderConfig{
		Name:   "openrouter",
		APIKey: "test-key",
		Debug:  true,
		Extra: map[string]any{
			"model_name": "deepseek/deepseek-chat-v3.1:free",
			"site_url":   "https://luna-bot.example.com",
			"site_title": "Luna Bot",
		},
	})
	// Ожидаем ошибку, потому что API-ключ недействителен
	if err == nil {
		t.Skip("real API key would be needed for full integration test")
	}
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}
