package bot

import "testing"

// Smoke test for cleanJSONFromMarkdown (ensures file compiles and basic behavior works)
func TestCleanJSONFromMarkdown_Smoke(t *testing.T) {
	b := &Bot{}
	input := "```json\n{\"a\":1}\n```"
	out := b.cleanJSONFromMarkdown(input)
	if out == "" {
		t.Fatalf("expected non-empty output")
	}
}
