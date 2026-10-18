package openai

import (
	"context"
	"sort"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// ListModels enumerates an openai-compatible endpoint's models, sorted
// and filtered to the chat-capable ones.
func ListModels(ctx context.Context, c *openai.Client) ([]string, error) {
	list, err := c.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(list.Models))
	for _, mdl := range list.Models {
		if mdl.ID != "" {
			names = append(names, mdl.ID)
		}
	}
	names = chatModels(names)
	sort.Strings(names)
	return names, nil
}

// nonChatMarkers are id fragments belonging to families that can't serve a
// chat completion. The big hosted catalogs list embeddings, speech, and image
// models next to the chat ones; local runtimes list embedding models too.
var nonChatMarkers = []string{
	"embed",
	"whisper",
	"tts",
	"audio",
	"speech",
	"transcribe",
	"dall-e",
	"moderation",
	"image",
	"rerank",
	"stable-diffusion",
	"clip",
}

// chatModels drops the ids that name a non-chat family. A filter that would
// empty the list is discarded instead — an unfamiliar naming scheme should
// leave the user with every model, not none.
func chatModels(names []string) []string {
	kept := make([]string, 0, len(names))
	for _, name := range names {
		lower := strings.ToLower(name)
		drop := false
		for _, marker := range nonChatMarkers {
			if strings.Contains(lower, marker) {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, name)
		}
	}
	if len(kept) == 0 {
		return names
	}
	return kept
}
