package llm_test

import (
	"errors"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/internal/llmtest"
)

// TestAppIdentityIsPinned pins every string the backends write for
// the caller. The expected values were captured on the commit before the app
// became a setting, by running the same calls against the code that had them
// written in, so a change here is a change in what a person's cache, headers
// or environment see.
//
// This is the part every backend shares: the variables a missing key is
// looked for in and the sign-in sentence. Each backend pins its own keys and
// headers beside its code.
func TestAppIdentityIsPinned(t *testing.T) {
	for _, k := range []string{"ACME_API_KEY", "ACME_BASE_URL", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY"} {
		t.Setenv(k, "")
	}
	for name, want := range map[string]string{
		"anthropic":        "ACME_API_KEY or ANTHROPIC_API_KEY is not set",
		"gemini":           "ACME_API_KEY or GEMINI_API_KEY is not set",
		"openai":           "ACME_API_KEY or OPENAI_API_KEY is not set",
		"openai-responses": "ACME_API_KEY or OPENAI_API_KEY is not set",
		"openrouter":       "ACME_API_KEY or OPENROUTER_API_KEY is not set",
	} {
		if _, err := llm.Resolve(name, llm.ResolveOpts{}); err == nil || err.Error() != want {
			t.Errorf("Resolve(%s) = %v, want %q", name, err, want)
		}
	}

	for _, c := range []struct{ name, got, want string }{
		{"signed-out error", llm.ErrChatGPTSignedOut.Error(), "not signed in to ChatGPT; run `acme login chatgpt`"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}

	_, err := llm.Resolve("chatgpt", llm.ResolveOpts{})
	if !errors.Is(err, llm.ErrChatGPTSignedOut) || err.Error() != "not signed in to ChatGPT; run `acme login chatgpt`" {
		t.Errorf("Resolve(chatgpt) = %v", err)
	}
}

// TestTheNeutralAppNamesNoApp: an app that sets nothing reads LLM_ variables
// and gives a sign-in sentence that names no command; and the signed-out
// error still matches by identity.
func TestTheNeutralAppNamesNoApp(t *testing.T) {
	llmtest.WithApp(t, llm.App{})
	for _, k := range []string{"LLM_API_KEY", "ACME_API_KEY", "ANTHROPIC_API_KEY"} {
		t.Setenv(k, "")
	}
	_, err := llm.Resolve("anthropic", llm.ResolveOpts{})
	if err == nil || err.Error() != "LLM_API_KEY or ANTHROPIC_API_KEY is not set" {
		t.Errorf("Resolve(anthropic) = %v", err)
	}
	t.Setenv("LLM_API_KEY", "k")
	if _, err := llm.Resolve("anthropic", llm.ResolveOpts{}); err != nil {
		t.Errorf("LLM_API_KEY was not read: %v", err)
	}
	if got := llm.ErrChatGPTSignedOut.Error(); got != "not signed in to ChatGPT" {
		t.Errorf("signed-out = %q", got)
	}
	if !errors.Is(llm.ErrChatGPTSignedOut, llm.ErrChatGPTSignedOut) {
		t.Error("the sentinel does not match itself")
	}
}
