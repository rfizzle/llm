package openrouter

import (
	"net/http"
	"strings"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/internal/llmtest"
)

type headerRecorder struct{ h http.Header }

func (r *headerRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.h = req.Header.Clone()
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

// TestAppIdentityIsPinned pins every string the backends write for
// the caller. The expected values were captured on the commit before the app
// became a setting, by running the same calls against the code that had them
// written in, so a change here is a change in what a person's cache, headers
// or environment see.
//
// OpenRouter's part: the attribution headers and the session key.
func TestAppIdentityIsPinned(t *testing.T) {
	rec := &headerRecorder{}
	req, _ := http.NewRequest(http.MethodPost, "http://x.invalid", nil)
	if _, err := (&openRouterTransport{base: rec}).RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if got := rec.h.Get("HTTP-Referer"); got != "https://example.com/acme" {
		t.Errorf("HTTP-Referer = %q", got)
	}
	if got := rec.h.Get("X-Title"); got != "acme" {
		t.Errorf("X-Title = %q", got)
	}

	messages := []llm.Message{{Role: llm.RoleSystem, Content: "sys"}, {Role: llm.RoleUser, Content: "hi"}}
	for _, c := range []struct{ name, got, want string }{
		{"openrouter session", openRouterSessionID(messages), "acme-17c15c4576d9a78cd19891ac5979026d"},
		{"openrouter session, no messages", openRouterSessionID(nil), "acme-d6324b34db7702f0caaf1eafeac27841"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

// TestTheNeutralAppNamesNoApp: an app that sets nothing sends no referer,
// titles itself llm and keys its sessions under llm.
func TestTheNeutralAppNamesNoApp(t *testing.T) {
	llmtest.WithApp(t, llm.App{})
	rec := &headerRecorder{}
	req, _ := http.NewRequest(http.MethodPost, "http://x.invalid", nil)
	_, _ = (&openRouterTransport{base: rec}).RoundTrip(req)
	if _, ok := rec.h["Http-Referer"]; ok || rec.h.Get("X-Title") != "llm" {
		t.Errorf("headers = %v", rec.h)
	}
	if got := openRouterSessionID(nil); !strings.HasPrefix(got, "llm-") {
		t.Errorf("session id %q", got)
	}
}
