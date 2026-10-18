package compat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/internal/idle"
	"github.com/rfizzle/llm/internal/llmtest"
	openai "github.com/sashabaranov/go-openai"
)

func newTestCompat(baseURL, model string) *Provider {
	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = baseURL
	cfg.HTTPClient = llmtest.HTTP.Client()
	return NewWith(openai.NewClientWithConfig(cfg), model, baseURL)
}

func TestOpenAICompat_Name(t *testing.T) {
	p := newTestCompat("http://unused", "llama3")
	if p.Name() != "openai-compatible" {
		t.Errorf("expected 'openai-compatible', got %q", p.Name())
	}
}

func TestOpenAICompat_DefaultModel(t *testing.T) {
	p := NewWith(openai.NewClient("fake"), "", "http://localhost:11434/v1")
	if p.model != "llama3" {
		t.Errorf("expected default model 'llama3', got %q", p.model)
	}
}

func TestNewOpenAICompat_Defaults(t *testing.T) {
	t.Setenv("ACME_API_KEY", "")
	t.Setenv("ACME_BASE_URL", "")

	p, err := New(llm.ResolveOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.baseURL != "http://localhost:11434/v1" {
		t.Errorf("expected default base URL, got %q", p.baseURL)
	}
	if p.model != "llama3" {
		t.Errorf("expected default model 'llama3', got %q", p.model)
	}
}

func TestNewOpenAICompat_EnvOverrides(t *testing.T) {
	t.Setenv("ACME_BASE_URL", "http://myhost:8080/v1")
	t.Setenv("ACME_API_KEY", "my-key")

	p, err := New(llm.ResolveOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.baseURL != "http://myhost:8080/v1" {
		t.Errorf("expected custom base URL, got %q", p.baseURL)
	}
}

func TestNewOpenAICompat_OptsOverrideEnvAndDefaults(t *testing.T) {
	t.Setenv("ACME_BASE_URL", "http://env-host:9999/v1")

	p, err := New(llm.ResolveOpts{
		BaseURL: "http://flag-host:1234/v1",
		Model:   "flag-model",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.baseURL != "http://flag-host:1234/v1" {
		t.Errorf("expected flag base URL, got %q", p.baseURL)
	}
	if p.model != "flag-model" {
		t.Errorf("expected flag model, got %q", p.model)
	}
}

func TestNewOpenAICompat_NoKeyRequired(t *testing.T) {
	t.Setenv("ACME_API_KEY", "")
	_, err := New(llm.ResolveOpts{})
	if err != nil {
		t.Fatalf("compat provider should not require an API key, got: %v", err)
	}
}

func TestOpenAICompat_StreamCompletion(t *testing.T) {
	tokens := []string{"foo", " bar"}
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, tok := range tokens {
			chunk := openai.ChatCompletionStreamResponse{
				Choices: []openai.ChatCompletionStreamChoice{
					{Delta: openai.ChatCompletionStreamChoiceDelta{Content: tok}},
				},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	p := newTestCompat(srv.URL+"/v1", "llama3")
	ch, err := p.StreamCompletion(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "hi"},
	}, llm.CompletionOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got string
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("unexpected stream error: %v", ev.Err)
		}
		got += ev.Token
	}
	if got != "foo bar" {
		t.Errorf("expected 'foo bar', got %q", got)
	}
}

func TestOpenAICompat_StreamCompletion_ModelOverride(t *testing.T) {
	var receivedModel string
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openai.ChatCompletionRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		receivedModel = req.Model
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p := newTestCompat(srv.URL+"/v1", "llama3")
	ch, err := p.StreamCompletion(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "hi"},
	}, llm.CompletionOpts{Model: "codellama"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range ch {
	}
	if receivedModel != "codellama" {
		t.Errorf("expected model 'codellama', got %q", receivedModel)
	}
}

func TestOpenAICompat_StreamCompletion_ServerError(t *testing.T) {
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "Invalid key",
				"type":    "invalid_request_error",
			},
		})
	}))
	defer srv.Close()

	p := newTestCompat(srv.URL+"/v1", "llama3")
	_, err := p.StreamCompletion(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "hi"},
	}, llm.CompletionOpts{})
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !errors.Is(err, llm.ErrAuth) {
		t.Errorf("expected ErrAuth, got: %v", err)
	}
}

func TestOpenAICompat_BaseURLPassedToServer(t *testing.T) {
	var gotPath string
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p := newTestCompat(srv.URL+"/custom/v1", "llama3")
	ch, err := p.StreamCompletion(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "hi"},
	}, llm.CompletionOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range ch {
	}
	if gotPath != "/custom/v1/chat/completions" {
		t.Errorf("expected path '/custom/v1/chat/completions', got %q", gotPath)
	}
}

func TestOpenAICompat_Registration(t *testing.T) {
	names := llm.Available()
	found := false
	for _, n := range names {
		if n == "openai-compatible" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'openai-compatible' to be registered")
	}
}

func TestOpenAICompat_CeilingFieldFollowsCapabilities(t *testing.T) {
	cases := []struct {
		model string
		want  string
	}{
		// Nothing describes what a local runtime is serving, so the ceiling
		// keeps the field every one of them has always taken.
		{"llama3", "max_tokens"},
		// A model with no thinking has no quarrel with the old field, and an
		// endpoint old enough to serve it may not know the new one.
		{"gpt-4o", "max_tokens"},
		// A reasoning model is the one case the old field is refused.
		{"o3", "max_completion_tokens"},
	}
	for _, tc := range cases {
		body := llmtest.CaptureChatRequest(t, func(baseURL string) (<-chan llm.StreamEvent, error) {
			return newTestCompat(baseURL, tc.model).StreamCompletion(
				context.Background(),
				[]llm.Message{{Role: llm.RoleUser, Content: "hi"}},
				llm.CompletionOpts{MaxTokens: 8192},
			)
		})
		if got := body[tc.want]; got != float64(8192) {
			t.Errorf("%s: %s = %v, want 8192", tc.model, tc.want, got)
		}
		other := "max_tokens"
		if tc.want == other {
			other = "max_completion_tokens"
		}
		if got, ok := body[other]; ok {
			t.Errorf("%s: request also carries %s = %v", tc.model, other, got)
		}
	}
}

func TestIdle_OpenAICompatStreamThatGoesQuietFails(t *testing.T) {
	srv := llmtest.QuietServer(t)
	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = srv.URL + "/v1"
	cfg.HTTPClient = llmtest.HTTP.Client()
	p := NewWith(openai.NewClientWithConfig(cfg), "llama3", cfg.BaseURL)
	p.Deadline = idle.After(llmtest.Idle)

	ch, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hello"}}, llm.CompletionOpts{})
	llmtest.WentQuiet(t, ch, err)
}

func TestOpenAICompat_ListModels(t *testing.T) {
	srv := llmtest.ModelsServer(t, `{"object":"list","data":[
		{"id":"qwen3:8b"},{"id":"llama3"},{"id":"nomic-embed-text"}]}`)

	p := newTestCompat(srv.URL+"/v1", "llama3")
	names, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 2 || names[0] != "llama3" || names[1] != "qwen3:8b" {
		t.Fatalf("expected the chat models sorted, got %v", names)
	}
}

func TestOpenAICompat_ListModelsError(t *testing.T) {
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer srv.Close()

	p := newTestCompat(srv.URL+"/v1", "llama3")
	if _, err := p.ListModels(context.Background()); err == nil {
		t.Fatal("expected an error from a 401")
	}
}

func TestOpenAICompat_ModelWindows(t *testing.T) {
	srv := llmtest.ModelsServer(t, `{"object":"list","data":[
		{"id":"Qwen/Qwen3-8B","max_model_len":262144},
		{"id":"local.gguf","meta":{"n_ctx":8192,"n_ctx_train":131072}},
		{"id":"lmstudio-model","max_context_length":32768},
		{"id":"llama3"}]}`)

	p := newTestCompat(srv.URL+"/v1", "llama3")
	p.httpc = llmtest.HTTP.Client()
	windows, err := p.ModelWindows(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for id, want := range map[string]int64{
		"qwen/qwen3-8b":  262_144,
		"local.gguf":     8_192,
		"lmstudio-model": 32_768,
	} {
		if got := windows[id]; got != want {
			t.Errorf("window for %q = %d, want %d", id, got, want)
		}
	}
	// A runtime that reports no length is not an answer, and the table and
	// the family floor take it from there.
	if _, ok := windows["llama3"]; ok {
		t.Errorf("a model with no reported length should be absent, got %v", windows)
	}
}

// A provider built over somebody else's client has no transport of its own to
// ask with, and says so rather than guessing.
func TestOpenAICompat_ModelWindowsWithoutATransport(t *testing.T) {
	windows, err := newTestCompat("http://unused", "llama3").ModelWindows(context.Background())
	if err != nil || windows != nil {
		t.Fatalf("expected no answer and no error, got %v, %v", windows, err)
	}
}

func TestOpenAICompat_ModelWindowsError(t *testing.T) {
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := newTestCompat(srv.URL+"/v1", "llama3")
	p.httpc = llmtest.HTTP.Client()
	if _, err := p.ModelWindows(context.Background()); err == nil {
		t.Fatal("expected an error from an endpoint with no catalog")
	}
}

func TestOpenAICompatIsAModelWindower(t *testing.T) {
	var p llm.Provider = &Provider{}
	if _, ok := p.(llm.ModelWindower); !ok {
		t.Fatal("the compat provider should answer for its endpoint's windows")
	}
}
