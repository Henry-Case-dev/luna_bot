package bot

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Henry-Case-dev/luna_bot/internal/config"
	"github.com/Henry-Case-dev/luna_bot/internal/gemini"
	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// LLMRouter реализует интерфейс llm.LLMClient и маршрутизирует запросы между провайдерами
// на основе ResponseTypeConfigs из конфигурации
type LLMRouter struct {
	cfg *config.Config

	// Кэш инициализированных клиентов
	clients map[config.LLMProvider]llm.LLMClient

	// Gemini клиент для специализированных операций (аудио, фото, изображения)
	geminiClient *gemini.Client

	debug bool
}

// NewLLMRouter создает новый router для маршрутизации между LLM провайдерами
func NewLLMRouter(cfg *config.Config, clients map[config.LLMProvider]llm.LLMClient, geminiClient *gemini.Client, debug bool) *LLMRouter {
	return &LLMRouter{
		cfg:          cfg,
		clients:      clients,
		geminiClient: geminiClient,
		debug:        debug,
	}
}

// getClientForResponseType возвращает подходящий LLM клиент для типа ответа
// на основе конфигурации ResponseTypeConfigs
func (r *LLMRouter) getClientForResponseType(responseType llm.ResponseType) llm.LLMClient {
	if r.cfg == nil || r.cfg.ResponseTypeConfigs == nil {
		// Fallback на дефолтный клиент, если конфиг недоступен
		return r.getDefaultClient()
	}

	typeConfig, ok := r.cfg.ResponseTypeConfigs[string(responseType)]
	if !ok {
		// Тип не найден в конфиге — используем дефолтный клиент
		if r.debug {
			log.Printf("[LLMRouter DEBUG] ResponseType %s не найден в конфиге, используем дефолтный клиент", responseType)
		}
		return r.getDefaultClient()
	}

	// Ищем клиент для нужного провайдера
	client, ok := r.clients[typeConfig.Provider]
	if !ok {
		// Клиент провайдера не инициализирован — fallback на дефолтный
		if r.debug {
			log.Printf("[LLMRouter DEBUG] Клиент %s не найден для типа %s, используем дефолтный", typeConfig.Provider, responseType)
		}
		return r.getDefaultClient()
	}

	if r.debug {
		log.Printf("[LLMRouter DEBUG] Маршрут %s -> %s (модель: %s, температура: %.2f)", responseType, typeConfig.Provider, typeConfig.ModelName, typeConfig.Temperature)
	}

	return client
}

// getDefaultClient возвращает клиент "по умолчанию" (обычно основной LLM провайдер)
// или первый доступный клиент
func (r *LLMRouter) getDefaultClient() llm.LLMClient {
	// Приоритет: основной провайдер → Gemini → первый доступный
	if r.cfg != nil {
		if client, ok := r.clients[r.cfg.LLMProvider]; ok {
			return client
		}
	}

	// Fallback на Gemini
	if client, ok := r.clients[config.ProviderGemini]; ok {
		return client
	}

	// Последний исход — первый доступный клиент
	for _, client := range r.clients {
		if client != nil {
			return client
		}
	}

	return nil
}

// GenerateResponse маршрутизирует вызов на Gemini (особый случай, не зависит от типа)
func (r *LLMRouter) GenerateResponse(systemPrompt string, history []*tgbotapi.Message, lastMessage *tgbotapi.Message, temperature float32) (string, error) {
	client := r.getDefaultClient()
	if client == nil {
		return "", fmt.Errorf("LLMRouter: нет доступных LLM клиентов")
	}
	return client.GenerateResponse(systemPrompt, history, lastMessage, temperature)
}

// GenerateResponseFromTextContext маршрутизирует вызов на основной клиент
func (r *LLMRouter) GenerateResponseFromTextContext(systemPrompt string, contextText string, temperature float32) (string, error) {
	client := r.getDefaultClient()
	if client == nil {
		return "", fmt.Errorf("LLMRouter: нет доступных LLM клиентов")
	}
	return client.GenerateResponseFromTextContext(systemPrompt, contextText, temperature)
}

// GenerateArbitraryResponse маршрутизирует вызов на основной клиент
func (r *LLMRouter) GenerateArbitraryResponse(systemPrompt string, contextText string, temperature float32) (string, error) {
	client := r.getDefaultClient()
	if client == nil {
		return "", fmt.Errorf("LLMRouter: нет доступных LLM клиентов")
	}
	return client.GenerateArbitraryResponse(systemPrompt, contextText, temperature)
}

// GenerateResponseByType маршрутизирует вызов нужному провайдеру на основе ResponseTypeConfigs
// и переопределяет температуру из конфига если указана
func (r *LLMRouter) GenerateResponseByType(responseType llm.ResponseType, systemPrompt string, contextText string, temperature float32) (string, error) {
	client := r.getClientForResponseType(responseType)
	if client == nil {
		return "", fmt.Errorf("LLMRouter: нет доступного клиента для типа %s", responseType)
	}

	// Получаем конфиг типа для переопределения температуры
	effectiveTemp := temperature
	if r.cfg != nil && r.cfg.ResponseTypeConfigs != nil {
		if typeConfig, ok := r.cfg.ResponseTypeConfigs[string(responseType)]; ok && typeConfig.Temperature > 0 {
			effectiveTemp = typeConfig.Temperature
			if r.debug {
				log.Printf("[LLMRouter DEBUG] Переопределение температуры для %s: %f -> %f", responseType, temperature, effectiveTemp)
			}
		}
	}

	// Передаем запрос выбранному клиенту
	return client.GenerateResponseByType(responseType, systemPrompt, contextText, effectiveTemp)
}

// TranscribeAudio всегда использует Gemini (единственный поддерживаемый вариант)
func (r *LLMRouter) TranscribeAudio(audioData []byte, mimeType string) (string, error) {
	if r.geminiClient == nil {
		return "", fmt.Errorf("LLMRouter: Gemini клиент недоступен для транскрибации")
	}
	return r.geminiClient.TranscribeAudio(audioData, mimeType)
}

// EmbedContent всегда использует Gemini (единственный поддерживаемый вариант)
func (r *LLMRouter) EmbedContent(text string) ([]float32, error) {
	if r.geminiClient == nil {
		return nil, fmt.Errorf("LLMRouter: Gemini клиент недоступен для создания эмбеддингов")
	}
	return r.geminiClient.EmbedContent(text)
}

// GenerateContentWithImage всегда использует Gemini (единственный поддерживаемый вариант)
func (r *LLMRouter) GenerateContentWithImage(ctx context.Context, systemPrompt string, imageData []byte, caption string) (string, error) {
	if r.geminiClient == nil {
		return "", fmt.Errorf("LLMRouter: Gemini клиент недоступен для анализа изображений")
	}
	return r.geminiClient.GenerateContentWithImage(ctx, systemPrompt, imageData, caption)
}

// GenerateImageWithEdit всегда использует Gemini (единственный поддерживаемый вариант)
func (r *LLMRouter) GenerateImageWithEdit(ctx context.Context, baseImageData []byte, editPrompt string) ([]byte, error) {
	if r.geminiClient == nil {
		return nil, fmt.Errorf("LLMRouter: Gemini клиент недоступен для генерации изображений")
	}
	return r.geminiClient.GenerateImageWithEdit(ctx, baseImageData, editPrompt)
}

// Close закрывает все инициализированные клиенты
func (r *LLMRouter) Close() error {
	var errs []string
	for provider, client := range r.clients {
		if client != nil {
			if err := client.Close(); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", provider, err))
			}
		}
	}

	if r.geminiClient != nil {
		if err := r.geminiClient.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("gemini: %v", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("LLMRouter: ошибки при закрытии клиентов: %s", strings.Join(errs, "; "))
	}
	return nil
}
