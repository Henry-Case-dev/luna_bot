package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEnvLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env.test")

	content := "# Comment\n  \nKEY=VALUE\nTOKEN=abc123\nMODE=\"quoted\"\nEMPTY=\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	env, err := readEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(env) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(env))
	}

	tests := []struct {
		key   string
		value string
	}{
		{"KEY", "VALUE"},
		{"TOKEN", "abc123"},
		{"MODE", "quoted"},
		{"EMPTY", ""},
	}
	for _, tt := range tests {
		if v, ok := env[tt.key]; !ok {
			t.Errorf("missing key %q", tt.key)
		} else if v != tt.value {
			t.Errorf("key %q: expected %q, got %q", tt.key, tt.value, v)
		}
	}

	if _, ok := env["# Comment"]; ok {
		t.Error("comment line was not skipped")
	}
}

func TestParseEnvLine_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env.empty")

	if err := os.WriteFile(path, []byte("# only comment\n\n   \n"), 0644); err != nil {
		t.Fatal(err)
	}

	env, err := readEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(env) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(env))
	}
}

func TestEnvToYAMLMapping(t *testing.T) {
	tests := []struct {
		envVar   string
		yamlPath string
	}{
		{"TELEGRAM_TOKEN", "telegram.token"},
		{"GEMINI_API_KEY", "llm.providers.gemini.api_key"},
		{"GEMINI_API_KEY_RESERVE", "llm.providers.gemini.reserve_api_key"},
		{"DEEPSEEK_API_KEY", "llm.providers.deepseek.api_key"},
		{"ELEVENLABS_API_KEY", "tts.elevenlabs.api_key"},
		{"DEBUG", "telegram.debug"},
		{"TIME_ZONE", "telegram.timezone"},
		{"LLM_PROVIDER", "llm.default_provider"},
		{"LLM_FALLBACK_ENABLED", "llm.fallback_enabled"},
	}

	for _, tt := range tests {
		path, ok := envToYAMLKey[tt.envVar]
		if !ok {
			t.Errorf("missing mapping for %q", tt.envVar)
			continue
		}
		if path != tt.yamlPath {
			t.Errorf("%q: expected %q, got %q", tt.envVar, tt.yamlPath, path)
		}
	}
}

func TestParseValue(t *testing.T) {
	tests := []struct {
		raw    string
		isNil  bool
	}{
		{"true", false},
		{"false", false},
		{"", true},
		{"your_api_key", true},
		{"42", false},
		{"3.14", false},
		{"hello", false},
		{"a,b,c", false},
	}

	for _, tt := range tests {
		v := parseValue(tt.raw)
		if tt.isNil && v != nil {
			t.Errorf("parseValue(%q) expected nil, got %v", tt.raw, v)
		}
		if !tt.isNil && v == nil {
			t.Errorf("parseValue(%q) expected non-nil, got nil", tt.raw)
		}
	}
}
