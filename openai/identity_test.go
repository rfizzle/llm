package openai

import (
	"encoding/json"
	"testing"
)

// TestAppIdentityIsPinned pins every string the backends write for
// the caller. The expected values were captured on the commit before the app
// became a setting, by running the same calls against the code that had them
// written in, so a change here is a change in what a person's cache, headers
// or environment see.
//
// The chat-completions part: its prompt-cache key. The Responses route's
// part is in llm/openai/responses.
func TestAppIdentityIsPinned(t *testing.T) {
	for _, c := range []struct{ name, got, want string }{
		{"chat cache key", chatPromptCacheKey("m", json.RawMessage(`[{"role":"system","content":"s"}]`), json.RawMessage(`[]`)), "acme-5520eb3a07d223c41253f016483d2424934f753c5ed41f9cffdc7c4b84cbd7ef"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}
