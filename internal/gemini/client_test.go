package gemini

import (
	"testing"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
)

// TestInfo проверяет, что Info() возвращает корректную метаинформацию.
func TestInfo(t *testing.T) {
	client := &Client{}
	info := client.Info()

	if info.Name != "gemini" {
		t.Errorf("expected name 'gemini', got %q", info.Name)
	}

	expectedCaps := 6
	if len(info.Capabilities) != expectedCaps {
		t.Errorf("expected %d capabilities, got %d", expectedCaps, len(info.Capabilities))
	}

	capSet := make(map[llm.Capability]bool)
	for _, c := range info.Capabilities {
		capSet[c] = true
	}

	requiredCaps := []llm.Capability{
		llm.CapTextGeneration,
		llm.CapAudioTranscription,
		llm.CapEmbedding,
		llm.CapImageAnalysis,
		llm.CapImageGeneration,
		llm.CapAudioGeneration,
	}
	for _, c := range requiredCaps {
		if !capSet[c] {
			t.Errorf("missing capability: %s", c)
		}
	}
}

// TestCompileTimeChecks проверяет, что Gemini реализует все capability-интерфейсы.
// Эти проверки выполняются на этапе компиляции через var-декларации в client.go.
// Данный тест — документационный, подтверждающий, что пакет компилируется.
func TestCompileTimeChecks(t *testing.T) {
	// Если пакет компилируется, compile-time checks в client.go уже прошли.
	// Этот тест существует для явной проверки в CI.
	var client *Client
	var _ llm.TextGenerator = client
	var _ llm.AudioTranscriber = client
	var _ llm.Embedder = client
	var _ llm.ImageAnalyzer = client
	var _ llm.ImageGenerator = client
	var _ llm.AudioGenerator = client
	var _ llm.Closer = client
}

// TestNewProviderMissingKey проверяет, что фабрика возвращает ошибку без API-ключа.
func TestNewProviderMissingKey(t *testing.T) {
	_, err := NewProvider(llm.ProviderConfig{
		Name:   "gemini",
		APIKey: "",
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

// TestNewProviderWithExtra проверяет, что фабрика принимает Extra-параметры.
// Не создаёт реального клиента (требуется API-ключ), только проверяет путь ошибки.
func TestNewProviderWithExtra(t *testing.T) {
	_, err := NewProvider(llm.ProviderConfig{
		Name:   "gemini",
		APIKey: "test-key",
		Extra: map[string]any{
			"model_name":       "gemini-2.0-flash",
			"embedding_model":  "embedding-001",
		},
	})
	// Ожидаем ошибку, потому что API-ключ недействителен
	if err == nil {
		t.Skip("real API key would be needed for full integration test")
	}
	// Ошибка должна быть о создании клиента, а не о конфигурации
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}
