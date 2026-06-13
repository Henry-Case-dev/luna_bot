package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/Henry-Case-dev/luna_bot/internal/llm"
	"github.com/Henry-Case-dev/luna_bot/internal/utils"
)

const defaultBaseURL = "https://api.anthropic.com/v1"

type Client struct {
	httpClient *http.Client
	apiKey     string
	modelName  string
	debug      bool
}

func New(apiKey, modelName string, debug bool) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic: api key is required")
	}
	if modelName == "" {
		return nil, fmt.Errorf("anthropic: model name is required")
	}

	log.Printf("[INFO] Anthropic client initialized for model: %s", modelName)

	return &Client{
		httpClient: &http.Client{Timeout: 120 * time.Second},
		apiKey:     apiKey,
		modelName:  modelName,
		debug:      debug,
	}, nil
}

func (c *Client) Close() error {
	return nil
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float64            `json:"temperature,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicResponse struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Role       string             `json:"role"`
	Content    []anthropicContent `json:"content"`
	Model      string             `json:"model"`
	StopReason string             `json:"stop_reason"`
	Usage      anthropicUsage     `json:"usage"`
}

type anthropicErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type anthropicErrorResponse struct {
	Type  string               `json:"type"`
	Error anthropicErrorDetail `json:"error"`
}

func (c *Client) GenerateResponse(systemPrompt string, history []*tgbotapi.Message, lastMessage *tgbotapi.Message, temperature float32) (string, error) {
	var ctxBuilder strings.Builder
	for _, msg := range history {
		role := "User"
		if msg.From != nil && msg.From.IsBot {
			role = "Bot"
		}
		text := msg.Text
		if text == "" && msg.Caption != "" {
			text = msg.Caption
		}
		if text != "" {
			ctxBuilder.WriteString(fmt.Sprintf("%s: %s\n", role, text))
		}
	}
	if lastMessage != nil {
		role := "User"
		if lastMessage.From != nil && lastMessage.From.IsBot {
			role = "Bot"
		}
		text := lastMessage.Text
		if text == "" && lastMessage.Caption != "" {
			text = lastMessage.Caption
		}
		if text != "" {
			ctxBuilder.WriteString(fmt.Sprintf("%s: %s\n", role, text))
		} else {
			ctxBuilder.WriteString("User: [message without text]\n")
		}
	}
	return c.GenerateResponseFromTextContext(systemPrompt, ctxBuilder.String(), temperature)
}

func (c *Client) GenerateResponseFromTextContext(systemPrompt string, contextText string, temperature float32) (string, error) {
	if c.debug {
		log.Printf("[DEBUG] Anthropic (TextContext): SystemPrompt: %s...", utils.TruncateString(systemPrompt, 100))
		log.Printf("[DEBUG] Anthropic (TextContext): ContextText: %s...", utils.TruncateString(contextText, 150))
		log.Printf("[DEBUG] Anthropic (TextContext): Model=%s, Temp=%.2f", c.modelName, temperature)
	}

	messages := []anthropicMessage{
		{
			Role: "user",
			Content: []anthropicContent{
				{Type: "text", Text: contextText},
			},
		},
	}

	return c.sendRequest(systemPrompt, messages, temperature)
}

func (c *Client) GenerateArbitraryResponse(systemPrompt string, contextText string, temperature float32) (string, error) {
	if c.debug {
		log.Printf("[DEBUG] Anthropic (Arbitrary): SystemPrompt: %s...", utils.TruncateString(systemPrompt, 100))
		log.Printf("[DEBUG] Anthropic (Arbitrary): ContextText: %s...", utils.TruncateString(contextText, 150))
		log.Printf("[DEBUG] Anthropic (Arbitrary): Model=%s, Temp=%.2f", c.modelName, temperature)
	}

	messages := []anthropicMessage{
		{
			Role: "user",
			Content: []anthropicContent{
				{Type: "text", Text: contextText},
			},
		},
	}

	return c.sendRequest(systemPrompt, messages, temperature)
}

func (c *Client) GenerateResponseByType(responseType llm.ResponseType, systemPrompt string, contextText string, temperature float32) (string, error) {
	if c.debug {
		log.Printf("[DEBUG] Anthropic: GenerateResponseByType type=%s temp=%.2f", responseType, temperature)
	}
	return c.GenerateArbitraryResponse(systemPrompt, contextText, temperature)
}

func (c *Client) GenerateChatResponse(responseType llm.ResponseType, messages []llm.ChatMessage, temperature float32) (string, error) {
	return c.GenerateArbitraryResponse(llm.ExtractSystemContent(messages), llm.FlattenChatMessages(messages), temperature)
}

func (c *Client) sendRequest(systemPrompt string, messages []anthropicMessage, temperature float32) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	maxTokens := 8192
	if maxTokensVal, ok := ctx.Value("max_tokens").(int); ok {
		maxTokens = maxTokensVal
	}

	payload := anthropicRequest{
		Model:     c.modelName,
		MaxTokens: maxTokens,
		Messages:  messages,
		System:    systemPrompt,
	}
	if temperature > 0 {
		payload.Temperature = float64(temperature)
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("anthropic: json marshal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", defaultBaseURL+"/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("anthropic: http request error: %w", err)
	}

	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	if c.debug {
		log.Printf("[DEBUG] Anthropic Request: URL=%s, Model=%s, Messages=%d", req.URL.String(), payload.Model, len(payload.Messages))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic: http do error: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("anthropic: read body error: %w", err)
	}

	if c.debug {
		log.Printf("[DEBUG] Anthropic Response: Status=%s", resp.Status)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp anthropicErrorResponse
		if json.Unmarshal(bodyBytes, &errResp) == nil && errResp.Error.Message != "" {
			return "", fmt.Errorf("anthropic API error (%d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return "", fmt.Errorf("anthropic API error: status %s, body: %s", resp.Status, string(bodyBytes))
	}

	var successResp anthropicResponse
	if err := json.Unmarshal(bodyBytes, &successResp); err != nil {
		return "", fmt.Errorf("anthropic: parse response error: %w, body: %s", err, string(bodyBytes))
	}

	if len(successResp.Content) == 0 {
		log.Printf("[WARN] Anthropic returned empty content. StopReason: %s", successResp.StopReason)
		return "", fmt.Errorf("anthropic: empty response content")
	}

	var responseText strings.Builder
	for _, content := range successResp.Content {
		if content.Type == "text" {
			responseText.WriteString(content.Text)
		}
	}

	finalResponse := responseText.String()
	if c.debug {
		usage := successResp.Usage
		log.Printf("[DEBUG] Anthropic Response: Tokens: In=%d Out=%d Stop=%s",
			usage.InputTokens, usage.OutputTokens, successResp.StopReason)
	}

	return finalResponse, nil
}

func (c *Client) Info() llm.ProviderInfo {
	return llm.ProviderInfo{
		Name: "anthropic",
		Capabilities: []llm.Capability{
			llm.CapTextGeneration,
		},
	}
}

var (
	_ llm.Provider      = (*Client)(nil)
	_ llm.TextGenerator = (*Client)(nil)
	_ llm.Closer        = (*Client)(nil)
)
