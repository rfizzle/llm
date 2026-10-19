package catalog

import (
	"testing"
)

// The family floor for a model the fetched table has no key for. Wrong-low is
// the failure that matters here: it trims a conversation the model had room
// to keep.
func TestContextWindowFor(t *testing.T) {
	for _, tc := range []struct {
		model string
		want  int64
		known bool
	}{
		{"claude-opus-5", 1_000_000, true},
		{"claude-sonnet-5-20260101", 1_000_000, true},
		{"claude-3-5-sonnet", 200_000, true},
		{"claude-opus-4-5", 200_000, true},
		{"claude-opus-4-7", 1_000_000, true},
		{"claude-haiku-4-5", 200_000, true},
		{"anthropic/claude-opus-5-thinking", 1_000_000, true},
		{"llama3.1:70b", 128_000, true},
		{"llama3", 8_192, true},
		{"llama2:13b", 4_096, true},
		{"llama4:scout", 128_000, true},
		{"meta-llama/Meta-Llama-3.3-70B-Instruct", 128_000, true},
		{"qwen2.5-coder:7b", 32_768, true},
		{"deepseek-chat", 65_536, true},
		{"mistral-nemo", 32_768, true},
		{"mixtral:8x7b", 32_768, true},
		{"gemma3:27b", 128_000, true},
		{"gemma2:9b", 8_192, true},
		{"phi4", 16_384, true},
		{"phi3:medium", 4_096, true},
		// The spellings that must not move: a digit that is already
		// separated, and one that never was.
		{"gpt-4o-mini", 128_000, true},
		{"o3-mini", 200_000, true},
		{"my-own-finetune", 0, false},
		{"", 0, false},
	} {
		got, ok := ContextWindowFor(tc.model)
		if ok != tc.known || got != tc.want {
			t.Errorf("ContextWindowFor(%q) = %d, %v; want %d, %v", tc.model, got, ok, tc.want, tc.known)
		}
	}
}
