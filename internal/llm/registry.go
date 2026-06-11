package llm

import (
	"fmt"
	"sync"
)

// Capability — перечисление возможностей провайдера.
type Capability string

const (
	CapTextGeneration     Capability = "text_generation"
	CapAudioTranscription Capability = "audio_transcription"
	CapEmbedding          Capability = "embedding"
	CapImageAnalysis      Capability = "image_analysis"
	CapImageGeneration    Capability = "image_generation"
	CapAudioGeneration    Capability = "audio_generation"
)

// ProviderInfo — метаинформация о провайдере.
type ProviderInfo struct {
	Name         string                // Уникальное имя (например, "gemini", "deepseek")
	Capabilities []Capability          // Поддерживаемые возможности
	ModelRouting map[Capability]string // capability → предпочитаемая модель (из конфига)
}

// Provider — интерфейс, который должен реализовать каждый провайдер.
// Содержит только мета-методы; capability-методы — через отдельные интерфейсы.
type Provider interface {
	Info() ProviderInfo
	Closer
}

// ProviderFactory — функция-конструктор провайдера.
// Принимает конфигурацию для конкретного провайдера.
type ProviderFactory func(cfg ProviderConfig) (Provider, error)

// ProviderConfig — конфигурация для создания провайдера.
type ProviderConfig struct {
	Name    string
	APIKey  string
	BaseURL string
	Debug   bool
	Extra   map[string]any // Провайдер-специфичные параметры
}

// ProviderRegistry — потокобезопасный реестр провайдеров.
type ProviderRegistry struct {
	mu        sync.RWMutex
	factories map[string]ProviderFactory // name → factory
	instances map[string]Provider        // name → initialized instance
}

// NewProviderRegistry создаёт новый реестр.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		factories: make(map[string]ProviderFactory),
		instances: make(map[string]Provider),
	}
}

// Register регистрирует фабрику провайдера.
func (r *ProviderRegistry) Register(name string, factory ProviderFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}

// Resolve получает или создаёт инстанс провайдера по имени.
func (r *ProviderRegistry) Resolve(name string, cfg ProviderConfig) (Provider, error) {
	r.mu.RLock()
	inst, ok := r.instances[name]
	r.mu.RUnlock()
	if ok {
		return inst, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check
	if inst, ok := r.instances[name]; ok {
		return inst, nil
	}

	factory, ok := r.factories[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", name)
	}

	provider, err := factory(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider %q: %w", name, err)
	}

	r.instances[name] = provider
	return provider, nil
}

// FindByCapability возвращает список провайдеров, поддерживающих данную capability.
// Список отсортирован по порядку регистрации (первый = приоритетный).
func (r *ProviderRegistry) FindByCapability(cap Capability) []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Provider
	for _, inst := range r.instances {
		info := inst.Info()
		for _, c := range info.Capabilities {
			if c == cap {
				result = append(result, inst)
				break
			}
		}
	}
	return result
}

// Shutdown вызывает Close() для всех зарегистрированных провайдеров.
func (r *ProviderRegistry) Shutdown() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for name, inst := range r.instances {
		if err := inst.Close(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	return nil
}
