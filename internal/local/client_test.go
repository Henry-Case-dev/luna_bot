package local

import (
	"testing"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
)

func TestInfo(t *testing.T) {
	client := &Client{}
	info := client.Info()

	if info.Name != "local" {
		t.Errorf("expected name 'local', got %q", info.Name)
	}

	if len(info.Capabilities) != 1 {
		t.Errorf("expected 1 capability, got %d: %v", len(info.Capabilities), info.Capabilities)
	}

	if len(info.Capabilities) > 0 && info.Capabilities[0] != llm.CapTextGeneration {
		t.Errorf("expected CapTextGeneration, got %s", info.Capabilities[0])
	}
}

func TestClose(t *testing.T) {
	client := &Client{}
	if err := client.Close(); err != nil {
		t.Errorf("expected nil error from Close, got %v", err)
	}
}

func TestNewProviderMissingKey(t *testing.T) {
	_, err := NewProvider(llm.ProviderConfig{
		Name:   "local",
		APIKey: "",
	})
	if err != nil {
		t.Fatalf("local provider does not require an API key, got: %v", err)
	}
}

func TestNewProviderWithBaseURL(t *testing.T) {
	provider, err := NewProvider(llm.ProviderConfig{
		Name:    "local",
		BaseURL: "http://localhost:11434/v1",
		Extra: map[string]any{
			"model_name": "llama3.1:8b",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error from NewProvider: %v", err)
	}
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
	if err := provider.Close(); err != nil {
		t.Errorf("unexpected error closing provider: %v", err)
	}
}
