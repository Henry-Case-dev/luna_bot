package openai

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

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

type Client struct {
	httpClient *http.Client
	apiKey     string
	modelName  string
	baseURL    string
	debug      bool
}

func New(apiKey, modelName, baseURL string, debug bool) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("openai: api key is required")
	}
	if modelName == "" {
		return nil, fmt.Errorf("openai: model name is required")
	}
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}

	log.Printf("[INFO] OpenAI client initialized for model: %s (baseURL: %s)", modelName, baseURL)

	return &Client{
		httpClient: &http.Client{Timeout: 120 * time.Second},
		apiKey:     apiKey,
		modelName:  modelName,
		baseURL:    baseURL,
		debug:      debug,
	}, nil
}

func (c *Client) Close() error {
	return nil
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiRequest struct {
	Model       string          `json:"model"`
	Messages    []openaiMessage `json:"messages"`
	Temperature *float64        `json:"temperature,omitempty"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
}

type openaiChoiceMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiChoice struct {
	Index        int                 `json:"index"`
	Message      openaiChoiceMessage `json:"message"`
	FinishReason string              `json:"finish_reason"`
}

type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openaiResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []openaiChoice `json:"choices"`
	Usage   openaiUsage    `json:"usage"`
}

type openaiErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

type openaiErrorResponse struct {
	Error openaiErrorDetail `json:"error"`
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
		log.Printf("[DEBUG] OpenAI (TextContext): SystemPrompt: %s...", utils.TruncateString(systemPrompt, 100))
		log.Printf("[DEBUG] OpenAI (TextContext): ContextText: %s...", utils.TruncateString(contextText, 150))
		log.Printf("[DEBUG] OpenAI (TextContext): Model=%s, Temp=%.2f", c.modelName, temperature)
	}

	messages := buildMessages(systemPrompt, contextText)
	return c.sendRequest(messages, temperature)
}

func (c *Client) GenerateArbitraryResponse(systemPrompt string, contextText string, temperature float32) (string, error) {
	if c.debug {
		log.Printf("[DEBUG] OpenAI (Arbitrary): SystemPrompt: %s...", utils.TruncateString(systemPrompt, 100))
		log.Printf("[DEBUG] OpenAI (Arbitrary): ContextText: %s...", utils.TruncateString(contextText, 150))
		log.Printf("[DEBUG] OpenAI (Arbitrary): Model=%s, Temp=%.2f", c.modelName, temperature)
	}

	messages := buildMessages(systemPrompt, contextText)
	return c.sendRequest(messages, temperature)
}

func (c *Client) GenerateResponseByType(responseType llm.ResponseType, systemPrompt string, contextText string, temperature float32) (string, error) {
	if c.debug {
		log.Printf("[DEBUG] OpenAI: GenerateResponseByType type=%s temp=%.2f", responseType, temperature)
	}
	return c.GenerateArbitraryResponse(systemPrompt, contextText, temperature)
}

func (c *Client) GenerateChatResponse(responseType llm.ResponseType, messages []llm.ChatMessage, temperature float32) (string, error) {
	return c.GenerateArbitraryResponse(llm.ExtractSystemContent(messages), llm.FlattenChatMessages(messages), temperature)
}

func buildMessages(systemPrompt, contextText string) []openaiMessage {
	messages := make([]openaiMessage, 0, 2)
	if systemPrompt != "" {
		messages = append(messages, openaiMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	messages = append(messages, openaiMessage{
		Role:    "user",
		Content: contextText,
	})
	return messages
}

func (c *Client) sendRequest(messages []openaiMessage, temperature float32) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	var tempPtr *float64
	if temperature > 0 {
		t := float64(temperature)
		tempPtr = &t
	}

	payload := openaiRequest{
		Model:       c.modelName,
		Messages:    messages,
		Temperature: tempPtr,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("openai: json marshal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("openai: http request error: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	if c.debug {
		log.Printf("[DEBUG] OpenAI Request: URL=%s, Model=%s, Messages=%d", req.URL.String(), payload.Model, len(payload.Messages))
		if len(payload.Messages) > 0 {
			last := payload.Messages[len(payload.Messages)-1]
			log.Printf("[DEBUG] OpenAI Last Message: Role=%s, Content=%s...", last.Role, utils.TruncateString(last.Content, 150))
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai: http do error: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("openai: read body error: %w", err)
	}

	if c.debug {
		log.Printf("[DEBUG] OpenAI Response: Status=%s", resp.Status)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp openaiErrorResponse
		if json.Unmarshal(bodyBytes, &errResp) == nil && errResp.Error.Message != "" {
			if resp.StatusCode == http.StatusTooManyRequests {
				return "[Limit]", fmt.Errorf("openai API rate limit (429): %s", errResp.Error.Message)
			}
			return "", fmt.Errorf("openai API error (%d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return "", fmt.Errorf("openai API error: status %s, body: %s", resp.Status, string(bodyBytes))
	}

	var successResp openaiResponse
	if err := json.Unmarshal(bodyBytes, &successResp); err != nil {
		return "", fmt.Errorf("openai: parse response error: %w, body: %s", err, string(bodyBytes))
	}

	if len(successResp.Choices) == 0 || successResp.Choices[0].Message.Content == "" {
		finishReason := "unknown"
		if len(successResp.Choices) > 0 {
			finishReason = successResp.Choices[0].FinishReason
		}
		log.Printf("[WARN] OpenAI returned empty response. FinishReason: %s", finishReason)
		return "", fmt.Errorf("openai: empty response (finish_reason=%s)", finishReason)
	}

	finalResponse := successResp.Choices[0].Message.Content
	if c.debug {
		usage := successResp.Usage
		log.Printf("[DEBUG] OpenAI Response: Tokens: Prompt=%d Completion=%d Total=%d Finish=%s",
			usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens, successResp.Choices[0].FinishReason)
	}

	return finalResponse, nil
}

func (c *Client) Info() llm.ProviderInfo {
	return llm.ProviderInfo{
		Name: "openai",
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
