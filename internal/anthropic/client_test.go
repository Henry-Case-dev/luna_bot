package anthropic

import (
	"testing"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
)

func TestInfo(t *testing.T) {
	client := &Client{}
	info := client.Info()

	if info.Name != "anthropic" {
		t.Errorf("expected name 'anthropic', got %q", info.Name)
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
		Name:   "anthropic",
		APIKey: "",
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestNewProviderWithKey(t *testing.T) {
	_, err := NewProvider(llm.ProviderConfig{
		Name:   "anthropic",
		APIKey: "test-key",
	})
	if err == nil {
		t.Skip("real API key would be needed for full integration test")
	}
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}
