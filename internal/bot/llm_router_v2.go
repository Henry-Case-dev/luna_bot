package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var _ llm.LLMClient = (*LLMRouterV2)(nil)

// LLMRouterV2 — capability-based LLM router with circuit breakers and routing profiles.
type LLMRouterV2 struct {
	registry *llm.ProviderRegistry
	config   *config.ConfigV2

	mu             sync.RWMutex
	textGens       []llm.TextGenerator
	audioTranscs   []llm.AudioTranscriber
	embedders      []llm.Embedder
	imageAnalyzers []llm.ImageAnalyzer
	imageGens      []llm.ImageGenerator
	audioGens      []llm.AudioGenerator

	breakers    map[string]*llm.CircuitBreaker
	keyRotators map[string]*llm.KeyRotator

	debug bool
}

// NewLLMRouterV2 создаёт новый capability-based router.
func NewLLMRouterV2(registry *llm.ProviderRegistry, cfg *config.ConfigV2, debug bool) *LLMRouterV2 {
	r := &LLMRouterV2{
		registry:    registry,
		config:      cfg,
		breakers:    make(map[string]*llm.CircuitBreaker),
		keyRotators: make(map[string]*llm.KeyRotator),
		debug:       debug,
	}

	maxFailures := cfg.LLM.CircuitBreaker.MaxFailures
	if maxFailures <= 0 {
		maxFailures = 5
	}
	cooldownSeconds := cfg.LLM.CircuitBreaker.CooldownSeconds
	if cooldownSeconds <= 0 {
		cooldownSeconds = 60
	}
	cooldown := time.Duration(cooldownSeconds) * time.Second

	for name, pcfg := range cfg.LLM.Providers {
		r.breakers[name] = llm.NewCircuitBreaker(maxFailures, cooldown)

		if pcfg.ReserveAPIKey != "" {
			ttl := time.Duration(pcfg.KeyRotationHours) * time.Hour
			if ttl <= 0 {
				ttl = 1 * time.Hour
			}
			r.keyRotators[name] = llm.NewKeyRotator(pcfg.APIKey, pcfg.ReserveAPIKey, ttl)
		}
	}

	r.refreshCaches()
	return r
}

// refreshCaches обходит всех провайдеров в реестре и заполняет capability-кэши.
func (r *LLMRouterV2) refreshCaches() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.textGens = nil
	r.audioTranscs = nil
	r.embedders = nil
	r.imageAnalyzers = nil
	r.imageGens = nil
	r.audioGens = nil

	for name := range r.config.LLM.Providers {
		provider, err := r.registry.Resolve(name, r.toLLMProviderConfig(name))
		if err != nil {
			if r.debug {
				log.Printf("[LLMRouterV2 DEBUG] refreshCaches: failed to resolve provider %q: %v", name, err)
			}
			continue
		}

		if tg, ok := provider.(llm.TextGenerator); ok {
			r.textGens = append(r.textGens, tg)
		}
		if at, ok := provider.(llm.AudioTranscriber); ok {
			r.audioTranscs = append(r.audioTranscs, at)
		}
		if emb, ok := provider.(llm.Embedder); ok {
			r.embedders = append(r.embedders, emb)
		}
		if ia, ok := provider.(llm.ImageAnalyzer); ok {
			r.imageAnalyzers = append(r.imageAnalyzers, ia)
		}
		if ig, ok := provider.(llm.ImageGenerator); ok {
			r.imageGens = append(r.imageGens, ig)
		}
		if ag, ok := provider.(llm.AudioGenerator); ok {
			r.audioGens = append(r.audioGens, ag)
		}
	}

	if r.debug {
		log.Printf("[LLMRouterV2] Health: textGens=%d audioTranscs=%d embedders=%d imageAnalyzers=%d imageGens=%d audioGens=%d",
			len(r.textGens), len(r.audioTranscs), len(r.embedders), len(r.imageAnalyzers), len(r.imageGens), len(r.audioGens))
	}
}

// ============================================================================
// Helpers
// ============================================================================

// getBreaker возвращает CircuitBreaker для провайдера (создаёт при необходимости).
func (r *LLMRouterV2) getBreaker(name string) *llm.CircuitBreaker {
	r.mu.RLock()
	b, ok := r.breakers[name]
	r.mu.RUnlock()
	if ok {
		return b
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if b, ok := r.breakers[name]; ok {
		return b
	}

	b = llm.NewCircuitBreaker(5, 60*time.Second)
	r.breakers[name] = b
	return b
}

// isProviderAvailable проверяет, доступен ли провайдер (CircuitBreaker закрыт).
func (r *LLMRouterV2) isProviderAvailable(name string) bool {
	breaker := r.getBreaker(name)
	return breaker.Allow()
}

// findTextGenerator ищет TextGenerator по имени провайдера в кэше.
func (r *LLMRouterV2) findTextGenerator(name string) llm.TextGenerator {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, tg := range r.textGens {
		if r.getProviderNameUnsafe(tg) == name {
			return tg
		}
	}
	return nil
}

// getProviderNameUnsafe извлекает имя провайдера без блокировки (вызывать под RLock).
func (r *LLMRouterV2) getProviderNameUnsafe(v interface{}) string {
	if p, ok := v.(llm.Provider); ok {
		return p.Info().Name
	}
	return "unknown"
}

// getProviderName извлекает имя провайдера через type assertion к Provider.
func (r *LLMRouterV2) getProviderName(v interface{}) string {
	return r.getProviderNameUnsafe(v)
}

// toLLMProviderConfig конвертирует config.ProviderConfig в llm.ProviderConfig.
func (r *LLMRouterV2) toLLMProviderConfig(name string) llm.ProviderConfig {
	cfg, ok := r.config.LLM.Providers[name]
	if !ok {
		return llm.ProviderConfig{Name: name, Debug: r.debug}
	}
	return llm.ProviderConfig{
		Name:    name,
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Debug:   cfg.Debug || r.debug,
		Extra:   cfg.Extra,
	}
}

// recordResult записывает успех/неудачу в CircuitBreaker провайдера.
// Только retryable ошибки (429/5xx) влияют на CircuitBreaker.
func (r *LLMRouterV2) recordResult(providerName string, err error) {
	if err == nil {
		r.getBreaker(providerName).RecordSuccess()
		return
	}
	if !r.isRetryableError(err) {
		return // не-retryable ошибки не должны влиять на CB
	}
	if r.getBreaker(providerName).RecordFailure() {
		log.Printf("[LLMRouterV2] Circuit breaker OPEN for provider %q", providerName)
	}
}

// recordFailure записывает неудачу с ротацией ключа при 429/5xx и обновлением CircuitBreaker.
func (r *LLMRouterV2) recordFailure(name string, err error) {
	if kr, ok := r.keyRotators[name]; ok {
		if kr.RotateOnError(err) {
			log.Printf("[LLMRouterV2] Key rotated for provider %q (now using reserve: %v)", name, kr.UsingReserve())
		}
	}
	r.recordResult(name, err)
}

// isRetryableError проверяет, является ли ошибка retryable (429/5xx/circuit breaker open).
func (r *LLMRouterV2) isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "500") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504") ||
		strings.Contains(errStr, "circuit breaker")
}

// tryTextGenerator — обёртка с CircuitBreaker для вызовов TextGenerator.
func (r *LLMRouterV2) tryTextGenerator(gen llm.TextGenerator, breaker *llm.CircuitBreaker, fn func() (string, error)) (string, error) {
	if !breaker.Allow() {
		return "", fmt.Errorf("LLMRouterV2: circuit breaker is open")
	}

	result, err := fn()
	if err != nil {
		if r.isRetryableError(err) {
			breaker.RecordFailure()
		}
	} else {
		breaker.RecordSuccess()
	}
	return result, err
}

// withFirstTextGenExcept ищет первый доступный TextGenerator, исключая уже опробованные,
// и выполняет fn через него.
func (r *LLMRouterV2) withFirstTextGenExcept(except map[string]bool, fn func(llm.TextGenerator) (string, error)) (string, error) {
	r.mu.RLock()
	gens := make([]llm.TextGenerator, len(r.textGens))
	copy(gens, r.textGens)
	r.mu.RUnlock()

	var lastErr error
	for _, gen := range gens {
		name := r.getProviderName(gen)
		if except[name] {
			continue
		}
		breaker := r.getBreaker(name)

		result, err := r.tryTextGenerator(gen, breaker, func() (string, error) {
			return fn(gen)
		})
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !r.isRetryableError(err) {
			return "", fmt.Errorf("LLMRouterV2: %w", err)
		}
		if r.debug {
			log.Printf("[LLMRouterV2 DEBUG] provider %q failed with retryable error, trying next", name)
		}
	}

	if lastErr != nil {
		return "", fmt.Errorf("LLMRouterV2: all text generators failed: %w", lastErr)
	}
	return "", fmt.Errorf("LLMRouterV2: no text generators available")
}

// withFirstTextGen ищет первый доступный TextGenerator и выполняет fn через него.
func (r *LLMRouterV2) withFirstTextGen(fn func(llm.TextGenerator) (string, error)) (string, error) {
	r.mu.RLock()
	gens := make([]llm.TextGenerator, len(r.textGens))
	copy(gens, r.textGens)
	r.mu.RUnlock()

	var lastErr error
	for _, gen := range gens {
		name := r.getProviderName(gen)
		breaker := r.getBreaker(name)

		result, err := r.tryTextGenerator(gen, breaker, func() (string, error) {
			return fn(gen)
		})
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !r.isRetryableError(err) {
			return "", fmt.Errorf("LLMRouterV2: %w", err)
		}
		if r.debug {
			log.Printf("[LLMRouterV2 DEBUG] provider %q failed with retryable error, trying next", name)
		}
	}

	if lastErr != nil {
		return "", fmt.Errorf("LLMRouterV2: all text generators failed: %w", lastErr)
	}
	return "", fmt.Errorf("LLMRouterV2: no text generators available")
}

// ============================================================================
// TextGenerator implementation
// ============================================================================

// GenerateResponseByType — главный метод с capability-based диспетчеризацией.
// Использует routing-профиль из ConfigV2 для выбора провайдера и параметров.
// Полная fallback-цепочка: primary → fallback → любой доступный TextGenerator.
func (r *LLMRouterV2) GenerateResponseByType(responseType llm.ResponseType, systemPrompt string, contextText string, temperature float32) (string, error) {
	if r.debug {
		log.Printf("[LLMRouterV2 DEBUG] GenerateResponseByType: type=%s", responseType)
	}

	yamlKey := llm.ResponseTypeToYAML(responseType)
	profile := r.config.GetRoutingProfile(yamlKey)
	effectiveTemp := float32(profile.Temperature)
	if effectiveTemp <= 0 {
		effectiveTemp = temperature
	}

	if r.debug {
		log.Printf("[LLMRouterV2 DEBUG] routing: type=%s yamlKey=%s provider=%s model=%s temp=%.2f fallback=%s",
			responseType, yamlKey, profile.Provider, profile.Model, effectiveTemp, profile.FallbackProvider)
	}

	if profile.Provider == "" {
		return r.withFirstTextGen(func(gen llm.TextGenerator) (string, error) {
			return gen.GenerateResponseByType(responseType, systemPrompt, contextText, temperature)
		})
	}

	tried := make(map[string]bool)

	// Primary
	result, err := r.tryProviderForType(profile.Provider, responseType, systemPrompt, contextText, effectiveTemp)
	if err == nil {
		return result, nil
	}
	tried[profile.Provider] = true

	if !r.isRetryableError(err) {
		return "", fmt.Errorf("LLMRouterV2: %w", err)
	}

	// Fallback
	if profile.FallbackProvider != "" && !tried[profile.FallbackProvider] {
		if r.debug {
			log.Printf("[LLMRouterV2 DEBUG] primary provider %q failed, trying fallback %q", profile.Provider, profile.FallbackProvider)
		}
		result, err = r.tryProviderForType(profile.FallbackProvider, responseType, systemPrompt, contextText, effectiveTemp)
		if err == nil {
			return result, nil
		}
		tried[profile.FallbackProvider] = true
		if !r.isRetryableError(err) {
			return "", fmt.Errorf("LLMRouterV2: %w", err)
		}
	}

	// Full fallback chain: try all remaining textGens
	return r.withFirstTextGenExcept(tried, func(gen llm.TextGenerator) (string, error) {
		return gen.GenerateResponseByType(responseType, systemPrompt, contextText, effectiveTemp)
	})
}

// tryProviderForType пытается выполнить генерацию через конкретного провайдера по имени,
// используя capability-кэш вместо прямого Resolve.
func (r *LLMRouterV2) tryProviderForType(name string, responseType llm.ResponseType, systemPrompt, contextText string, temperature float32) (string, error) {
	tg := r.findTextGenerator(name)
	if tg == nil {
		return "", fmt.Errorf("provider %q does not support text generation or not available", name)
	}

	breaker := r.getBreaker(name)
	if !breaker.Allow() {
		return "", fmt.Errorf("provider %q circuit breaker is open", name)
	}

	result, err := tg.GenerateResponseByType(responseType, systemPrompt, contextText, temperature)
	r.recordFailure(name, err)
	return result, err
}

// GenerateResponse ищет первый доступный TextGenerator и делегирует вызов.
func (r *LLMRouterV2) GenerateResponse(systemPrompt string, history []*tgbotapi.Message, lastMessage *tgbotapi.Message, temperature float32) (string, error) {
	return r.withFirstTextGen(func(gen llm.TextGenerator) (string, error) {
		return gen.GenerateResponse(systemPrompt, history, lastMessage, temperature)
	})
}

// GenerateResponseFromTextContext ищет первый доступный TextGenerator и делегирует вызов.
func (r *LLMRouterV2) GenerateResponseFromTextContext(systemPrompt string, contextText string, temperature float32) (string, error) {
	return r.withFirstTextGen(func(gen llm.TextGenerator) (string, error) {
		return gen.GenerateResponseFromTextContext(systemPrompt, contextText, temperature)
	})
}

// GenerateArbitraryResponse ищет первый доступный TextGenerator и делегирует вызов.
func (r *LLMRouterV2) GenerateArbitraryResponse(systemPrompt string, contextText string, temperature float32) (string, error) {
	return r.withFirstTextGen(func(gen llm.TextGenerator) (string, error) {
		return gen.GenerateArbitraryResponse(systemPrompt, contextText, temperature)
	})
}

// ============================================================================
// AudioTranscriber implementation
// ============================================================================

// TranscribeAudio ищет первый доступный AudioTranscriber и делегирует вызов.
func (r *LLMRouterV2) TranscribeAudio(audioData []byte, mimeType string) (string, error) {
	r.mu.RLock()
	transcs := make([]llm.AudioTranscriber, len(r.audioTranscs))
	copy(transcs, r.audioTranscs)
	r.mu.RUnlock()

	var lastErr error
	for _, at := range transcs {
		name := r.getProviderName(at)
		breaker := r.getBreaker(name)

		if !breaker.Allow() {
			lastErr = fmt.Errorf("LLMRouterV2: provider %q circuit breaker is open", name)
			continue
		}

		result, err := at.TranscribeAudio(audioData, mimeType)
		r.recordFailure(name, err)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !r.isRetryableError(err) {
			return "", fmt.Errorf("LLMRouterV2: %w", err)
		}
	}

	if lastErr != nil {
		return "", fmt.Errorf("LLMRouterV2: all audio transcribers failed: %w", lastErr)
	}
	return "", fmt.Errorf("LLMRouterV2: no audio transcribers available")
}

// ============================================================================
// Embedder implementation
// ============================================================================

// EmbedContent ищет первый доступный Embedder и делегирует вызов.
func (r *LLMRouterV2) EmbedContent(text string) ([]float32, error) {
	r.mu.RLock()
	embs := make([]llm.Embedder, len(r.embedders))
	copy(embs, r.embedders)
	r.mu.RUnlock()

	var lastErr error
	for _, emb := range embs {
		name := r.getProviderName(emb)
		breaker := r.getBreaker(name)

		if !breaker.Allow() {
			lastErr = fmt.Errorf("LLMRouterV2: provider %q circuit breaker is open", name)
			continue
		}

		result, err := emb.EmbedContent(text)
		r.recordFailure(name, err)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !r.isRetryableError(err) {
			return nil, fmt.Errorf("LLMRouterV2: %w", err)
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("LLMRouterV2: all embedders failed: %w", lastErr)
	}
	return nil, fmt.Errorf("LLMRouterV2: no embedders available")
}

// ============================================================================
// ImageAnalyzer implementation
// ============================================================================

// GenerateContentWithImage ищет первый доступный ImageAnalyzer и делегирует вызов.
func (r *LLMRouterV2) GenerateContentWithImage(ctx context.Context, systemPrompt string, imageData []byte, caption string) (string, error) {
	r.mu.RLock()
	analyzers := make([]llm.ImageAnalyzer, len(r.imageAnalyzers))
	copy(analyzers, r.imageAnalyzers)
	r.mu.RUnlock()

	var lastErr error
	for _, ia := range analyzers {
		name := r.getProviderName(ia)
		breaker := r.getBreaker(name)

		if !breaker.Allow() {
			lastErr = fmt.Errorf("LLMRouterV2: provider %q circuit breaker is open", name)
			continue
		}

		result, err := ia.GenerateContentWithImage(ctx, systemPrompt, imageData, caption)
		r.recordFailure(name, err)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !r.isRetryableError(err) {
			return "", fmt.Errorf("LLMRouterV2: %w", err)
		}
	}

	if lastErr != nil {
		return "", fmt.Errorf("LLMRouterV2: all image analyzers failed: %w", lastErr)
	}
	return "", fmt.Errorf("LLMRouterV2: no image analyzers available")
}

// ============================================================================
// ImageGenerator implementation
// ============================================================================

// GenerateImageWithEdit ищет первый доступный ImageGenerator и делегирует вызов.
func (r *LLMRouterV2) GenerateImageWithEdit(ctx context.Context, baseImageData []byte, editPrompt string) ([]byte, error) {
	r.mu.RLock()
	gens := make([]llm.ImageGenerator, len(r.imageGens))
	copy(gens, r.imageGens)
	r.mu.RUnlock()

	var lastErr error
	for _, ig := range gens {
		name := r.getProviderName(ig)
		breaker := r.getBreaker(name)

		if !breaker.Allow() {
			lastErr = fmt.Errorf("LLMRouterV2: provider %q circuit breaker is open", name)
			continue
		}

		result, err := ig.GenerateImageWithEdit(ctx, baseImageData, editPrompt)
		r.recordFailure(name, err)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !r.isRetryableError(err) {
			return nil, fmt.Errorf("LLMRouterV2: %w", err)
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("LLMRouterV2: all image generators failed: %w", lastErr)
	}
	return nil, fmt.Errorf("LLMRouterV2: no image generators available")
}

// ============================================================================
// AudioGenerator implementation
// ============================================================================

// GenerateAudio ищет первый доступный AudioGenerator и делегирует вызов.
func (r *LLMRouterV2) GenerateAudio(text string, params llm.AudioParams) ([]byte, error) {
	r.mu.RLock()
	gens := make([]llm.AudioGenerator, len(r.audioGens))
	copy(gens, r.audioGens)
	r.mu.RUnlock()

	var lastErr error
	for _, ag := range gens {
		name := r.getProviderName(ag)
		breaker := r.getBreaker(name)

		if !breaker.Allow() {
			lastErr = fmt.Errorf("LLMRouterV2: provider %q circuit breaker is open", name)
			continue
		}

		result, err := ag.GenerateAudio(text, params)
		r.recordFailure(name, err)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !r.isRetryableError(err) {
			return nil, fmt.Errorf("LLMRouterV2: %w", err)
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("LLMRouterV2: all audio generators failed: %w", lastErr)
	}
	return nil, fmt.Errorf("LLMRouterV2: no audio generators available")
}

// ============================================================================
// Closer implementation
// ============================================================================

// Close завершает работу всех провайдеров через реестр.
func (r *LLMRouterV2) Close() error {
	return r.registry.Shutdown()
}
