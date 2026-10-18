package openai

import (
	"testing"
)

func TestChatModels_KeepsEverythingWhenAllFiltered(t *testing.T) {
	names := []string{"my-embed-model", "text-embedding-3-small"}
	if got := chatModels(names); len(got) != 2 {
		t.Fatalf("an all-filtered list should be kept whole, got %v", got)
	}
}

func TestChatModels_Filters(t *testing.T) {
	names := []string{"gpt-4o", "whisper-1", "dall-e-3", "o3", "text-embedding-3-large", "tts-1-hd"}
	got := chatModels(names)
	if len(got) != 2 || got[0] != "gpt-4o" || got[1] != "o3" {
		t.Fatalf("expected only the chat models, got %v", got)
	}
}
