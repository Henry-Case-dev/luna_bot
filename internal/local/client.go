package local

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/utils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

type Client struct {
	httpClient *http.Client
	baseURL    string
	modelName  string
	apiKey     string
	debug      bool
}

func New(baseURL, modelName, apiKey string, debug bool) (*Client, error) {
	if baseURL == "" {
		baseURL = "http://localhost:11434/v1"
	}
	return &Client{
		httpClient: &http.Client{Timeout: 120 * time.Second},
		baseURL:    baseURL,
		modelName:  modelName,
		apiKey:     apiKey,
		debug:      debug,
	}, nil
}

func (c *Client) Info() llm.ProviderInfo {
	return llm.ProviderInfo{
		Name:         "local",
		Capabilities: []llm.Capability{llm.CapTextGeneration},
	}
}

func (c *Client) Close() error { return nil }

func (c *Client) GenerateResponse(systemPrompt string, history []*tgbotapi.Message, lastMessage *tgbotapi.Message, temperature float32) (string, error) {
	if c.debug {
		log.Printf("[DEBUG] Local Запрос: SystemPrompt: %s...", utils.TruncateString(systemPrompt, 100))
		log.Printf("[DEBUG] Local Запрос: LastMessage: %s...", utils.TruncateString(lastMessage.Text, 50))
		log.Printf("[DEBUG] Local Запрос: Модель %s, Температура %.2f", c.modelName, temperature)
	}

	messages := c.prepareChatHistory(systemPrompt, history)
	messages = append(messages, chatMessage{
		Role:    "user",
		Content: lastMessage.Text,
	})

	return c.doChatCompletion(messages, temperature)
}

func (c *Client) GenerateResponseFromTextContext(systemPrompt string, contextText string, temperature float32) (string, error) {
	if c.debug {
		log.Printf("[DEBUG] Local Запрос (Text Context): SystemPrompt: %s...", utils.TruncateString(systemPrompt, 100))
		log.Printf("[DEBUG] Local Запрос (Text Context): ContextText: %s...", utils.TruncateString(contextText, 150))
		log.Printf("[DEBUG] Local Запрос (Text Context): Модель %s, Температура %.2f", c.modelName, temperature)
	}

	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: contextText},
	}

	return c.doChatCompletion(messages, temperature)
}

func (c *Client) GenerateArbitraryResponse(systemPrompt string, contextText string, temperature float32) (string, error) {
	if c.debug {
		log.Printf("[DEBUG] Local Запрос (Arbitrary): SystemPrompt: %s...", utils.TruncateString(systemPrompt, 100))
		log.Printf("[DEBUG] Local Запрос (Arbitrary): ContextText: %s...", utils.TruncateString(contextText, 150))
		log.Printf("[DEBUG] Local Запрос (Arbitrary): Модель %s, Температура %.2f", c.modelName, temperature)
	}

	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: contextText},
	}

	return c.doChatCompletion(messages, temperature)
}

func (c *Client) GenerateResponseByType(responseType llm.ResponseType, systemPrompt string, contextText string, temperature float32) (string, error) {
	if c.debug {
		log.Printf("[DEBUG] Local: Генерация ответа типа %s с температурой %f", responseType, temperature)
	}
	return c.GenerateArbitraryResponse(systemPrompt, contextText, temperature)
}

func (c *Client) prepareChatHistory(systemPrompt string, messages []*tgbotapi.Message) []chatMessage {
	chatMessages := make([]chatMessage, 0, len(messages)+1)

	if systemPrompt != "" {
		chatMessages = append(chatMessages, chatMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	for _, msg := range messages {
		if msg == nil || msg.Text == "" {
			continue
		}

		role := "user"
		if msg.From != nil && msg.From.IsBot {
			role = "assistant"
		}

		chatMessages = append(chatMessages, chatMessage{
			Role:    role,
			Content: msg.Text,
		})
	}

	return chatMessages
}

func (c *Client) doChatCompletion(messages []chatMessage, temperature float32) (string, error) {
	url := c.baseURL + "/chat/completions"

	reqBody := chatRequest{
		Model:       c.modelName,
		Messages:    messages,
		Temperature: float64(temperature),
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("local: failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("local: failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[ERROR] Local Ошибка API: %v", err)
		return "", fmt.Errorf("local: API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("local: failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[ERROR] Local API вернул статус %d: %s", resp.StatusCode, string(body))
		return "", fmt.Errorf("local: API returned status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("local: failed to parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 || chatResp.Choices[0].Message.Content == "" {
		log.Printf("[ERROR] Local: Пустой ответ от API.")
		return "", fmt.Errorf("local: empty response from API")
	}

	response := chatResp.Choices[0].Message.Content
	if c.debug {
		log.Printf("[DEBUG] Local Ответ: %s...", utils.TruncateString(response, 100))
	}

	return response, nil
}

var (
	_ llm.Provider      = (*Client)(nil)
	_ llm.TextGenerator = (*Client)(nil)
	_ llm.Closer        = (*Client)(nil)
)
