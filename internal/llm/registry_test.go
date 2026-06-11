package llm

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

// mockProvider — минимальная реализация Provider для тестов.
type mockProvider struct {
	name         string
	capabilities []Capability
	closeErr     error
	closed       bool
}

func (m *mockProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:         m.name,
		Capabilities: m.capabilities,
	}
}

func (m *mockProvider) Close() error {
	m.closed = true
	return m.closeErr
}

// newMockProviderFactory создаёт фабрику для mockProvider.
func newMockProviderFactory(name string, capabilities []Capability) ProviderFactory {
	return func(cfg ProviderConfig) (Provider, error) {
		return &mockProvider{
			name:         name,
			capabilities: capabilities,
		}, nil
	}
}

// newFailingFactory создаёт фабрику, которая всегда возвращает ошибку.
func newFailingFactory() ProviderFactory {
	return func(cfg ProviderConfig) (Provider, error) {
		return nil, errors.New("mock creation error")
	}
}

// TestRegisterAndResolve проверяет регистрацию и resolve провайдера.
func TestRegisterAndResolve(t *testing.T) {
	reg := NewProviderRegistry()

	reg.Register("gemini", newMockProviderFactory("gemini", []Capability{
		CapTextGeneration, CapAudioTranscription, CapEmbedding,
	}))

	prov, err := reg.Resolve("gemini", ProviderConfig{Name: "gemini", APIKey: "test-key"})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if prov == nil {
		t.Fatal("expected non-nil provider")
	}

	info := prov.Info()
	if info.Name != "gemini" {
		t.Errorf("expected name 'gemini', got %q", info.Name)
	}
	if len(info.Capabilities) != 3 {
		t.Errorf("expected 3 capabilities, got %d", len(info.Capabilities))
	}
}

// TestResolveDoubleCheckLocking проверяет, что Resolve возвращает один и тот же инстанс.
func TestResolveDoubleCheckLocking(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register("gemini", newMockProviderFactory("gemini", []Capability{CapTextGeneration}))

	var wg sync.WaitGroup
	providers := make([]Provider, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			prov, err := reg.Resolve("gemini", ProviderConfig{Name: "gemini"})
			if err != nil {
				t.Errorf("Resolve failed: %v", err)
				return
			}
			providers[idx] = prov
		}(i)
	}
	wg.Wait()

	first := providers[0]
	for i := 1; i < 100; i++ {
		if providers[i] != first {
			t.Errorf("providers[%d] is a different instance", i)
		}
	}
}

// TestResolveUnregistered проверяет ошибку при запросе незарегистрированного провайдера.
func TestResolveUnregistered(t *testing.T) {
	reg := NewProviderRegistry()
	_, err := reg.Resolve("nonexistent", ProviderConfig{Name: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for unregistered provider")
	}
}

// TestResolveFactoryError проверяет проброс ошибки из фабрики.
func TestResolveFactoryError(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register("broken", newFailingFactory())

	_, err := reg.Resolve("broken", ProviderConfig{Name: "broken"})
	if err == nil {
		t.Fatal("expected error from failing factory")
	}
}

// TestFindByCapability проверяет фильтрацию провайдеров по capability.
func TestFindByCapability(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register("gemini", newMockProviderFactory("gemini", []Capability{
		CapTextGeneration, CapAudioTranscription, CapEmbedding,
	}))
	reg.Register("deepseek", newMockProviderFactory("deepseek", []Capability{
		CapTextGeneration,
	}))
	reg.Register("elevenlabs", newMockProviderFactory("elevenlabs", []Capability{
		CapAudioGeneration,
	}))

	reg.Resolve("gemini", ProviderConfig{Name: "gemini"})
	reg.Resolve("deepseek", ProviderConfig{Name: "deepseek"})
	reg.Resolve("elevenlabs", ProviderConfig{Name: "elevenlabs"})

	textGens := reg.FindByCapability(CapTextGeneration)
	if len(textGens) != 2 {
		t.Errorf("expected 2 TextGenerator providers, got %d", len(textGens))
	}

	audioGens := reg.FindByCapability(CapAudioGeneration)
	if len(audioGens) != 1 {
		t.Errorf("expected 1 AudioGenerator provider, got %d", len(audioGens))
	}

	empty := reg.FindByCapability(Capability("nonexistent"))
	if len(empty) != 0 {
		t.Errorf("expected 0 providers for nonexistent capability, got %d", len(empty))
	}
}

// TestFindByCapabilityEmptyRegistry проверяет пустой реестр.
func TestFindByCapabilityEmptyRegistry(t *testing.T) {
	reg := NewProviderRegistry()
	result := reg.FindByCapability(CapTextGeneration)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d providers", len(result))
	}
}

// TestShutdown проверяет корректный shutdown всех провайдеров.
func TestShutdown(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register("gemini", newMockProviderFactory("gemini", []Capability{CapTextGeneration}))
	reg.Register("deepseek", newMockProviderFactory("deepseek", []Capability{CapTextGeneration}))

	prov1, _ := reg.Resolve("gemini", ProviderConfig{Name: "gemini"})
	prov2, _ := reg.Resolve("deepseek", ProviderConfig{Name: "deepseek"})

	err := reg.Shutdown()
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	mp1 := prov1.(*mockProvider)
	mp2 := prov2.(*mockProvider)
	if !mp1.closed {
		t.Error("gemini was not closed")
	}
	if !mp2.closed {
		t.Error("deepseek was not closed")
	}
}

// TestShutdownWithErrors проверяет shutdown при частичных ошибках Close().
func TestShutdownWithErrors(t *testing.T) {
	reg := NewProviderRegistry()

	factoryWithCloseErr := func(cfg ProviderConfig) (Provider, error) {
		return &mockProvider{
			name:     "broken",
			closeErr: errors.New("close failed"),
		}, nil
	}
	reg.Register("broken", factoryWithCloseErr)
	reg.Register("ok", newMockProviderFactory("ok", []Capability{CapTextGeneration}))

	reg.Resolve("broken", ProviderConfig{Name: "broken"})
	reg.Resolve("ok", ProviderConfig{Name: "ok"})

	err := reg.Shutdown()
	if err == nil {
		t.Fatal("expected error from Shutdown with failing Close()")
	}
}

// TestRegisterDuplicate проверяет перезапись фабрики при повторной регистрации.
func TestRegisterDuplicate(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register("gemini", newMockProviderFactory("first", []Capability{CapTextGeneration}))
	reg.Register("gemini", newMockProviderFactory("second", []Capability{CapTextGeneration, CapEmbedding}))

	prov, err := reg.Resolve("gemini", ProviderConfig{Name: "gemini"})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	info := prov.Info()
	if info.Name != "second" {
		t.Errorf("expected name 'second' (last registered), got %q", info.Name)
	}
	if len(info.Capabilities) != 2 {
		t.Errorf("expected 2 capabilities from second factory, got %d", len(info.Capabilities))
	}
}

// TestRegistryConcurrency проверяет конкурентные Register + Resolve.
func TestRegistryConcurrency(t *testing.T) {
	reg := NewProviderRegistry()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("provider-%d", idx)
			reg.Register(name, newMockProviderFactory(name, []Capability{CapTextGeneration}))
			prov, err := reg.Resolve(name, ProviderConfig{Name: name})
			if err != nil {
				t.Errorf("Resolve %s failed: %v", name, err)
				return
			}
			info := prov.Info()
			if info.Name != name {
				t.Errorf("expected name %q, got %q", name, info.Name)
			}
		}(i)
	}
	wg.Wait()

	textGens := reg.FindByCapability(CapTextGeneration)
	if len(textGens) != 10 {
		t.Errorf("expected 10 TextGenerator providers, got %d", len(textGens))
	}
}
