package local

import (
	"github.com/Henry-Case-dev/luna_bot/internal/llm"
)

// Sampling parameter constants for Gemma 4 12B Uncensored via Ollama.
// These are critical for Russian morphology and preventing infinite loops.
const (
	// Stage 2: Response Generation (creative text)
	DefaultResponseTemperature     = 1.15
	DefaultResponseTopP            = 1.00
	DefaultResponsePresencePenalty = 0.0
	DefaultResponseMinP            = 0.06
	DefaultRepetitionPenalty       = 1.10

	// Stage 1: Decision Making (JSON output)
	DefaultDecisionTemperature = 0.2 // CRITICAL: low temp prevents JSON breakage
)

// SamplingParams holds all sampling parameters for an LLM request.
type SamplingParams struct {
	Temperature      float64
	TopP             float64
	MinP             float64
	RepetitionPenalty float64
	PresencePenalty   float64
}

// DecisionSamplingParams returns params for Stage 1 (JSON decision making).
func DecisionSamplingParams() SamplingParams {
	return SamplingParams{
		Temperature:      DefaultDecisionTemperature, // 0.2
		TopP:             0.0,                        // disabled for structured output
		MinP:             0.0,                        // not used for JSON decisions
		RepetitionPenalty: 1.0,                       // neutral, no penalty
		PresencePenalty:   0.0,                       // not needed for JSON structure
	}
}

// ResponseSamplingParams returns params for Stage 2 (text generation).
func ResponseSamplingParams() SamplingParams {
	return SamplingParams{
		Temperature:      DefaultResponseTemperature,     // 1.15
		TopP:             DefaultResponseTopP,            // 1.00
		MinP:             DefaultResponseMinP,            // 0.06
		RepetitionPenalty: DefaultRepetitionPenalty,       // 1.10
		PresencePenalty:   DefaultResponsePresencePenalty, // 0.0
	}
}

// isDecisionResponseType returns true if the response type is a Stage 1
// structured JSON decision that should use DecisionSamplingParams.
func isDecisionResponseType(rt string) bool {
	_, ok := decisionResponseTypes[rt]
	return ok
}

// decisionResponseTypes maps response type string values that are Stage 1
// structured JSON decision outputs (require low temp, no TopP, no PresencePenalty).
var decisionResponseTypes = map[string]bool{
	"free_will_should_reply":              true,
	"free_will_response_type":             true,
	"free_will_direct_response_decision": true,
	"free_will_mood_analysis":             true,
	"free_will_reaction":                  true,
	"reaction_analysis":                   true,
	"web_search_trigger":                  true,
	"classify":                            true,
	"moderation":                          true,
	"srach":                               true,
	"causal_analysis":                     true,
	"causal_influence":                    true,
	"belief_analysis":                     true,
}

func NewProvider(cfg llm.ProviderConfig) (llm.Provider, error) {
	modelName := "hf.co/mradermacher/gemma-4-12B-it-abliterated-uncensored-GGUF:Q6_K"
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434/v1"
	}
	if cfg.Extra != nil {
		if v, ok := cfg.Extra["model_name"].(string); ok && v != "" {
			modelName = v
		}
	}
	return New(baseURL, modelName, cfg.APIKey, cfg.Debug)
}
