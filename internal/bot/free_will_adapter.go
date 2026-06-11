package bot

import "github.com/Henry-Case-dev/luna_bot/internal/llm"

// FreeWillLLMAdapter — адаптер для FreeWillService.
type FreeWillLLMAdapter struct {
	client llm.LLMClient
}

// NewFreeWillLLMAdapter создаёт адаптер.
func NewFreeWillLLMAdapter(client llm.LLMClient) *FreeWillLLMAdapter {
	return &FreeWillLLMAdapter{client: client}
}

// GetClient возвращает обёрнутый LLMClient.
func (a *FreeWillLLMAdapter) GetClient() llm.LLMClient {
	return a.client
}

// GenerateResponseByType делегирует вызов в обёрнутый LLMClient.
func (a *FreeWillLLMAdapter) GenerateResponseByType(responseType llm.ResponseType, systemPrompt, contextText string, temperature float32) (string, error) {
	return a.client.GenerateResponseByType(responseType, systemPrompt, contextText, temperature)
}
