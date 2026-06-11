package bot

import (
	"errors"
	"strings"
	"testing"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ============================================================================
// Mock TextGenerator
// ============================================================================

type mockTextGen struct {
	name      string
	response  string
	err       error
	callCount int
	closed    bool
}

func (m *mockTextGen) Info() llm.ProviderInfo {
	return llm.ProviderInfo{Name: m.name, Capabilities: []llm.Capability{llm.CapTextGeneration}}
}
func (m *mockTextGen) Close() error { m.closed = true; return nil }
func (m *mockTextGen) GenerateResponse(string, []*tgbotapi.Message, *tgbotapi.Message, float32) (string, error) {
	m.callCount++
	return m.response, m.err
}
func (m *mockTextGen) GenerateResponseFromTextContext(string, string, float32) (string, error) {
	m.callCount++
	return m.response, m.err
}
func (m *mockTextGen) GenerateArbitraryResponse(string, string, float32) (string, error) {
	m.callCount++
	return m.response, m.err
}
func (m *mockTextGen) GenerateResponseByType(llm.ResponseType, string, string, float32) (string, error) {
	m.callCount++
	return m.response, m.err
}

// ============================================================================
// Mock Embedder
// ============================================================================

type mockEmbedder struct {
	name   string
	emb    []float32
	err    error
	closed bool
}

func (m *mockEmbedder) Info() llm.ProviderInfo {
	return llm.ProviderInfo{Name: m.name, Capabilities: []llm.Capability{llm.CapEmbedding}}
}
func (m *mockEmbedder) Close() error                       { m.closed = true; return nil }
func (m *mockEmbedder) EmbedContent(text string) ([]float32, error) { return m.emb, m.err }

// ============================================================================
// Helper: build minimal ConfigV2
// ============================================================================

func testConfigV2(providers map[string]config.ProviderConfig, responseTypes map[string]config.RoutingProfile) *config.ConfigV2 {
	return &config.ConfigV2{
		LLM: config.LLMConfig{
			Providers:     providers,
			ResponseTypes: responseTypes,
			CircuitBreaker: config.CircuitBreakerConfig{
				MaxFailures:     5,
				CooldownSeconds: 60,
			},
		},
	}
}

// ============================================================================
// Tests
// ============================================================================

// TestLLMRouterV2_GenerateResponseByType_Success — успешный вызов через routing-профиль.
func TestLLMRouterV2_GenerateResponseByType_Success(t *testing.T) {
	reg := llm.NewProviderRegistry()
	primary := &mockTextGen{name: "primary", response: "hello from primary"}
	reg.Register("primary", func(cfg llm.ProviderConfig) (llm.Provider, error) {
		return primary, nil
	})

	cfg := testConfigV2(
		map[string]config.ProviderConfig{
			"primary": {Enabled: true},
		},
		map[string]config.RoutingProfile{
			"default": {Provider: "primary", Temperature: 0.7},
		},
	)

	router := NewLLMRouterV2(reg, cfg, false)

	result, err := router.GenerateResponseByType(llm.ResponseTypeDefault, "system", "context", 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello from primary" {
		t.Errorf("expected 'hello from primary', got %q", result)
	}
	if primary.callCount != 1 {
		t.Errorf("expected primary called once, got %d", primary.callCount)
	}
}

// TestLLMRouterV2_GenerateResponseByType_Fallback — primary возвращает 429 → fallback срабатывает.
func TestLLMRouterV2_GenerateResponseByType_Fallback(t *testing.T) {
	reg := llm.NewProviderRegistry()
	pri := &mockTextGen{name: "primary", err: errors.New("429 Too Many Requests")}
	fb := &mockTextGen{name: "fallback", response: "fallback success"}
	reg.Register("primary", func(cfg llm.ProviderConfig) (llm.Provider, error) { return pri, nil })
	reg.Register("fallback", func(cfg llm.ProviderConfig) (llm.Provider, error) { return fb, nil })

	cfg := testConfigV2(
		map[string]config.ProviderConfig{
			"primary":  {Enabled: true},
			"fallback": {Enabled: true},
		},
		map[string]config.RoutingProfile{
			"default": {Provider: "primary", FallbackProvider: "fallback", Temperature: 0.7},
		},
	)

	router := NewLLMRouterV2(reg, cfg, false)

	result, err := router.GenerateResponseByType(llm.ResponseTypeDefault, "system", "context", 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "fallback success" {
		t.Errorf("expected 'fallback success', got %q", result)
	}
	if pri.callCount != 1 {
		t.Errorf("expected primary called once, got %d", pri.callCount)
	}
	if fb.callCount != 1 {
		t.Errorf("expected fallback called once, got %d", fb.callCount)
	}
}

// TestLLMRouterV2_GenerateResponseByType_AllFail — все провайдеры недоступны → ошибка.
func TestLLMRouterV2_GenerateResponseByType_AllFail(t *testing.T) {
	reg := llm.NewProviderRegistry()
	pri := &mockTextGen{name: "primary", err: errors.New("429 Too Many Requests")}
	fb := &mockTextGen{name: "fallback", err: errors.New("503 Service Unavailable")}
	reg.Register("primary", func(cfg llm.ProviderConfig) (llm.Provider, error) { return pri, nil })
	reg.Register("fallback", func(cfg llm.ProviderConfig) (llm.Provider, error) { return fb, nil })

	cfg := testConfigV2(
		map[string]config.ProviderConfig{
			"primary":  {Enabled: true},
			"fallback": {Enabled: true},
		},
		map[string]config.RoutingProfile{
			"default": {Provider: "primary", FallbackProvider: "fallback", Temperature: 0.7},
		},
	)

	router := NewLLMRouterV2(reg, cfg, false)

	_, err := router.GenerateResponseByType(llm.ResponseTypeDefault, "system", "context", 0.5)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if pri.callCount != 1 {
		t.Errorf("expected primary called once, got %d", pri.callCount)
	}
	if fb.callCount != 1 {
		t.Errorf("expected fallback called once, got %d", fb.callCount)
	}
}

// TestLLMRouterV2_GenerateResponse — вызов GenerateResponse через первый доступный TextGenerator.
func TestLLMRouterV2_GenerateResponse(t *testing.T) {
	reg := llm.NewProviderRegistry()
	primary := &mockTextGen{name: "primary", response: "gen response"}
	reg.Register("primary", func(cfg llm.ProviderConfig) (llm.Provider, error) {
		return primary, nil
	})

	cfg := testConfigV2(
		map[string]config.ProviderConfig{
			"primary": {Enabled: true},
		},
		nil,
	)

	router := NewLLMRouterV2(reg, cfg, false)

	result, err := router.GenerateResponse("system", nil, nil, 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "gen response" {
		t.Errorf("expected 'gen response', got %q", result)
	}
	if primary.callCount != 1 {
		t.Errorf("expected primary called once, got %d", primary.callCount)
	}
}

// TestLLMRouterV2_TranscribeAudio_NoTranscriber — нет AudioTranscriber → ошибка.
func TestLLMRouterV2_TranscribeAudio_NoTranscriber(t *testing.T) {
	reg := llm.NewProviderRegistry()

	cfg := testConfigV2(nil, nil)

	router := NewLLMRouterV2(reg, cfg, false)

	_, err := router.TranscribeAudio([]byte("fake audio"), "audio/ogg")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no audio transcribers available") {
		t.Errorf("expected 'no audio transcribers available' in error, got: %v", err)
	}
}

// TestLLMRouterV2_EmbedContent — успешный вызов EmbedContent.
func TestLLMRouterV2_EmbedContent(t *testing.T) {
	reg := llm.NewProviderRegistry()
	emb := &mockEmbedder{name: "emb_provider", emb: []float32{0.1, 0.2, 0.3}}
	reg.Register("emb_provider", func(cfg llm.ProviderConfig) (llm.Provider, error) {
		return emb, nil
	})

	cfg := testConfigV2(
		map[string]config.ProviderConfig{
			"emb_provider": {Enabled: true},
		},
		nil,
	)

	router := NewLLMRouterV2(reg, cfg, false)

	result, err := router.EmbedContent("test text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 || result[0] != 0.1 || result[1] != 0.2 || result[2] != 0.3 {
		t.Errorf("expected [0.1, 0.2, 0.3], got %v", result)
	}
}

// TestLLMRouterV2_Close — Close вызывает Shutdown.
func TestLLMRouterV2_Close(t *testing.T) {
	reg := llm.NewProviderRegistry()
	primary := &mockTextGen{name: "primary", response: "ok"}
	reg.Register("primary", func(cfg llm.ProviderConfig) (llm.Provider, error) {
		return primary, nil
	})

	cfg := testConfigV2(
		map[string]config.ProviderConfig{
			"primary": {Enabled: true},
		},
		nil,
	)

	router := NewLLMRouterV2(reg, cfg, false)

	err := router.Close()
	if err != nil {
		t.Fatalf("unexpected Close error: %v", err)
	}
	if !primary.closed {
		t.Error("expected primary.Closed() to be called via Shutdown")
	}
}

// TestLLMRouterV2_FallbackChain_FullCircle — primary 429 → fallback success.
func TestLLMRouterV2_FallbackChain_FullCircle(t *testing.T) {
	reg := llm.NewProviderRegistry()
	pri := &mockTextGen{name: "gemini", err: errors.New("429 Too Many Requests")}
	fb := &mockTextGen{name: "deepseek", response: "deepseek rescued"}
	reg.Register("gemini", func(cfg llm.ProviderConfig) (llm.Provider, error) { return pri, nil })
	reg.Register("deepseek", func(cfg llm.ProviderConfig) (llm.Provider, error) { return fb, nil })

	cfg := testConfigV2(
		map[string]config.ProviderConfig{
			"gemini":   {Enabled: true},
			"deepseek": {Enabled: true},
		},
		map[string]config.RoutingProfile{
			"default": {Provider: "gemini", FallbackProvider: "deepseek", Temperature: 0.7},
		},
	)

	router := NewLLMRouterV2(reg, cfg, false)

	result, err := router.GenerateResponseByType(llm.ResponseTypeDefault, "system", "context", 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "deepseek rescued" {
		t.Errorf("expected 'deepseek rescued', got %q", result)
	}
	if pri.callCount != 1 {
		t.Errorf("expected primary called once, got %d", pri.callCount)
	}
	if fb.callCount != 1 {
		t.Errorf("expected fallback called once, got %d", fb.callCount)
	}
}

// TestLLMRouterV2_FallbackChain_AllExhausted — all providers return 503, no fallback left.
func TestLLMRouterV2_FallbackChain_AllExhausted(t *testing.T) {
	reg := llm.NewProviderRegistry()
	pri := &mockTextGen{name: "gemini", err: errors.New("503 Service Unavailable")}
	fb := &mockTextGen{name: "deepseek", err: errors.New("503 Service Unavailable")}
	reg.Register("gemini", func(cfg llm.ProviderConfig) (llm.Provider, error) { return pri, nil })
	reg.Register("deepseek", func(cfg llm.ProviderConfig) (llm.Provider, error) { return fb, nil })

	cfg := testConfigV2(
		map[string]config.ProviderConfig{
			"gemini":   {Enabled: true},
			"deepseek": {Enabled: true},
		},
		map[string]config.RoutingProfile{
			"default": {Provider: "gemini", FallbackProvider: "deepseek", Temperature: 0.7},
		},
	)

	router := NewLLMRouterV2(reg, cfg, false)

	_, err := router.GenerateResponseByType(llm.ResponseTypeDefault, "system", "context", 0.5)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if pri.callCount != 1 {
		t.Errorf("expected primary called once, got %d", pri.callCount)
	}
	if fb.callCount != 1 {
		t.Errorf("expected fallback called once, got %d", fb.callCount)
	}
}

// TestLLMRouterV2_CircuitBreaker_Isolation — after 5 failures CB opens for one provider,
// but the other provider still works.
func TestLLMRouterV2_CircuitBreaker_Isolation(t *testing.T) {
	reg := llm.NewProviderRegistry()
	pri := &mockTextGen{name: "gemini", err: errors.New("503 Service Unavailable")}
	fb := &mockTextGen{name: "deepseek", response: "fallback wins"}
	reg.Register("gemini", func(cfg llm.ProviderConfig) (llm.Provider, error) { return pri, nil })
	reg.Register("deepseek", func(cfg llm.ProviderConfig) (llm.Provider, error) { return fb, nil })

	cfg := testConfigV2(
		map[string]config.ProviderConfig{
			"gemini":   {Enabled: true},
			"deepseek": {Enabled: true},
		},
		map[string]config.RoutingProfile{
			"default": {Provider: "gemini", FallbackProvider: "deepseek", Temperature: 0.7},
		},
	)

	router := NewLLMRouterV2(reg, cfg, false)

	breaker := router.getBreaker("gemini")
	for i := 0; i < 5; i++ {
		breaker.RecordFailure()
	}

	if breaker.Allow() {
		t.Fatal("expected gemini circuit breaker to be open after 5 failures")
	}

	result, err := router.GenerateResponseByType(llm.ResponseTypeDefault, "system", "context", 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "fallback wins" {
		t.Errorf("expected 'fallback wins', got %q", result)
	}

	if pri.callCount != 0 {
		t.Errorf("expected primary (gemini) skipped due to open CB, got %d calls", pri.callCount)
	}
	if fb.callCount != 1 {
		t.Errorf("expected fallback (deepseek) called once, got %d", fb.callCount)
	}
}
