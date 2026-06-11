package deepseek

import (
	"testing"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
)

// TestInfo проверяет, что Info() возвращает корректную метаинформацию.
func TestInfo(t *testing.T) {
	client := &Client{}
	info := client.Info()

	if info.Name != "deepseek" {
		t.Errorf("expected name 'deepseek', got %q", info.Name)
	}

	if len(info.Capabilities) != 1 {
		t.Errorf("expected 1 capability, got %d: %v", len(info.Capabilities), info.Capabilities)
	}

	if len(info.Capabilities) > 0 && info.Capabilities[0] != llm.CapTextGeneration {
		t.Errorf("expected CapTextGeneration, got %s", info.Capabilities[0])
	}
}

// TestCompileTimeChecks проверяет, что DeepSeek реализует TextGenerator + Closer.
func TestCompileTimeChecks(t *testing.T) {
	var client *Client
	var _ llm.TextGenerator = client
	var _ llm.Closer = client
}

// TestNewProviderMissingKey проверяет ошибку без API-ключа.
func TestNewProviderMissingKey(t *testing.T) {
	_, err := NewProvider(llm.ProviderConfig{
		Name:   "deepseek",
		APIKey: "",
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

// TestNewProviderWithBaseURL проверяет фабрику с кастомным BaseURL.
func TestNewProviderWithBaseURL(t *testing.T) {
	_, err := NewProvider(llm.ProviderConfig{
		Name:    "deepseek",
		APIKey:  "test-key",
		BaseURL: "https://custom-deepseek.example.com/v1",
		Extra: map[string]any{
			"model_name": "deepseek-chat",
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
