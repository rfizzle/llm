package gemini

import (
	"context"
	"net/http"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/internal/llmtest"
)

func TestNew_UsesTheGivenHTTPClient(t *testing.T) {
	rec := &llmtest.Recorder{}
	p, err := New(llm.ResolveOpts{
		APIKey:     "test-key",
		BaseURL:    "http://given.test/v1",
		HTTPClient: &http.Client{Transport: rec},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ch, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, llm.CompletionOpts{}); err == nil {
		for range ch {
		}
	}
	if hosts := rec.Hosts(); len(hosts) == 0 {
		t.Fatalf("the given client carried %v, want at least one request (the SDK picks its own host)", hosts)
	}
}
