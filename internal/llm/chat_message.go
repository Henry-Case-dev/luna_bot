package llm

import "strings"

// ChatMessage represents a single message in OpenAI-compatible ChatML format.
// Maps directly to the JSON structure {"role": "system|user|assistant", "content": "..."}
// used by Ollama /v1/chat/completions endpoint.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// BuildSystemPrompt consolidates personality context, state data, and instructions
// into a single system-role ChatMessage suitable as the first message in a ChatML array.
func BuildSystemPrompt(personality, state, instructions string) ChatMessage {
	var content string

	if personality != "" {
		content += "=== ЛИЧНОСТЬ ===\n" + personality + "\n\n"
	}
	if state != "" {
		content += "=== ТЕКУЩЕЕ СОСТОЯНИЕ ===\n" + state + "\n\n"
	}
	if instructions != "" {
		content += "=== ИНСТРУКЦИИ ===\n" + instructions
	}

	return ChatMessage{
		Role:    "system",
		Content: content,
	}
}

// ExtractSystemContent extracts the content of the first system-role message
// from a ChatML array. Falls back to first message if no system role found.
func ExtractSystemContent(messages []ChatMessage) string {
	for _, m := range messages {
		if m.Role == "system" {
			return m.Content
		}
	}
	if len(messages) > 0 {
		return messages[0].Content
	}
	return ""
}

// FlattenChatMessages joins all non-system message contents into a single string
// separated by newlines, for fallback compatibility with clients that expect
// a single contextText string.
func FlattenChatMessages(messages []ChatMessage) string {
	var sb strings.Builder
	for _, m := range messages {
		if m.Role == "system" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(m.Content)
	}
	return sb.String()
}
