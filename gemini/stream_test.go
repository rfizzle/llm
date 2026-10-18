package gemini

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/internal/idle"
	"github.com/rfizzle/llm/internal/llmtest"
	"google.golang.org/genai"
)

// Gemini never sends a call in pieces, so its fragment is the arguments
// entire — reported all the same, and before the terminal event, so a reader
// following fragments does not have to know which dialect it is following.
func TestStream_GeminiReportsTheWholeCallAsOneFragment(t *testing.T) {
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, `data: {"candidates":[{"content":{"parts":[{"functionCall":{"id":"call_abc","name":"write_file","args":{"path":"main.go"}}}]}}]}`+"\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: srv.URL},
		HTTPClient:  llmtest.HTTP.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	g := &Provider{client: client, model: "gemini-2.5-flash", classify: llm.NewClassifier("gemini", "ACME_API_KEY", "test-key")}

	ch, err := g.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "write main.go"}}, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}

	fragments, final := llmtest.DrainFragments(t, ch)
	llmtest.WantFragments(t, fragments, []llm.ToolCallDelta{{ID: "call_abc", Arguments: `{"path":"main.go"}`}})
	if len(final.ToolCalls) != 1 || final.ToolCalls[0].Name != "write_file" {
		t.Errorf("the terminal event lost the finished call: %+v", final.ToolCalls)
	}
	if final.Stop != llm.StopTool {
		t.Errorf("a round that ended in a call stopped for tools, got %q", final.Stop)
	}
}

// This dialect delivers a function call whole or not at all, so a round it
// cut at the ceiling has no half-written call in it: what it did send is
// dispatchable and the ceiling is reported beside it. The other two things
// the reply is checked for are the ones every dialect owes — the words the
// model wrote survive, and the ending is named.
func TestStream_GeminiCeilingNamesTheEndingAndKeepsWhatArrivedWhole(t *testing.T) {
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, `data: {"candidates":[{"content":{"parts":[{"text":"I will write it"}]}}]}`+"\n\n")
		flusher.Flush()
		fmt.Fprint(w, `data: {"candidates":[{"finishReason":"MAX_TOKENS","content":{"parts":[{"functionCall":{"id":"call_abc","name":"write_file","args":{"path":"main.go"}}}]}}]}`+"\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: srv.URL},
		HTTPClient:  llmtest.HTTP.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	g := &Provider{client: client, model: "gemini-2.5-flash", classify: llm.NewClassifier("gemini", "ACME_API_KEY", "test-key")}

	ch, err := g.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "write main.go"}}, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	var final llm.StreamEvent
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("stream error: %v", ev.Err)
		}
		text += ev.Token
		if ev.Done {
			final = ev
		}
	}
	if text != "I will write it" {
		t.Errorf("the words the model wrote are kept, got %q", text)
	}
	if final.Stop != llm.StopLength {
		t.Errorf("stop = %q, want %q", final.Stop, llm.StopLength)
	}
	if len(final.ToolCalls) != 1 || final.ToolCalls[0].ID != "call_abc" {
		t.Errorf("a call this dialect delivered whole is still a call, got %+v", final.ToolCalls)
	}
}

func TestIdle_GeminiStreamThatGoesQuietFails(t *testing.T) {
	srv := llmtest.QuietServer(t)
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: srv.URL},
		HTTPClient:  llmtest.HTTP.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	g := &Provider{
		client:   client,
		model:    "gemini-2.5-flash",
		Deadline: idle.After(llmtest.Idle),
		classify: llm.NewClassifier("gemini", "ACME_API_KEY", "test-key"),
	}

	ch, err := g.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hello"}}, llm.CompletionOpts{})
	llmtest.WentQuiet(t, ch, err)
}
