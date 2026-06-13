package bot

import (
	"regexp"
	"strings"
)

var (
	imStartTag  = "<|im_start|>"
	imEndTag    = "<|im_end|>"
	endOfText   = "<|endoftext|>"
)

// SanitizeUserInput strips LLM special tokens from user text to prevent
// prompt injection and unexpected model behavior.
func SanitizeUserInput(text string) string {
	text = strings.ReplaceAll(text, imStartTag, "")
	text = strings.ReplaceAll(text, imEndTag, "")
	text = strings.ReplaceAll(text, endOfText, "")
	return text
}

// thinkTagRegex matches complete <think>...</think> blocks including content (case-insensitive).
var thinkTagRegex = regexp.MustCompile(`(?is)<think>.*?</think>`)

// orphanCloseRegex matches orphaned </think> tags (case-insensitive).
var orphanCloseRegex = regexp.MustCompile(`(?is)</think>`)

// orphanOpenRegex matches orphaned <think> tags (case-insensitive).
var orphanOpenRegex = regexp.MustCompile(`(?i)<think>`)

// SanitizeThinkTags removes <think>...</think> blocks (chain-of-thought output)
// and orphaned fragments from LLM output before sending to Telegram.
func SanitizeThinkTags(text string) string {
	text = thinkTagRegex.ReplaceAllString(text, "")

	text = orphanCloseRegex.ReplaceAllString(text, "")
	text = orphanOpenRegex.ReplaceAllString(text, "")

	return strings.TrimSpace(text)
}
