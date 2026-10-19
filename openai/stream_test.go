package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/internal/idle"
	"github.com/rfizzle/llm/internal/llmtest"
	"github.com/rfizzle/llm/internal/testhttp"
	openai "github.com/sashabaranov/go-openai"
)

func TestStream_OpenAIReportsArgumentFragmentsInOrder(t *testing.T) {
	idx0 := 0
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		chunks := []openai.ChatCompletionStreamResponse{
			{Choices: []openai.ChatCompletionStreamChoice{{Delta: openai.ChatCompletionStreamChoiceDelta{ToolCalls: []openai.ToolCall{
				{Index: &idx0, ID: "call_abc", Function: openai.FunctionCall{Name: "write_file", Arguments: `{"pa`}},
			}}}}},
			{Choices: []openai.ChatCompletionStreamChoice{{Delta: openai.ChatCompletionStreamChoiceDelta{ToolCalls: []openai.ToolCall{
				{Index: &idx0, Function: openai.FunctionCall{Arguments: `th":"main.go"}`}},
			}}}}},
			{Choices: []openai.ChatCompletionStreamChoice{{FinishReason: "tool_calls"}}},
		}
		for _, chunk := range chunks {
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	p := newTestOpenAI(srv.URL+"/v1", "gpt-4o")
	ch, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "write main.go"}}, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}

	fragments, final := llmtest.DrainFragments(t, ch)
	llmtest.WantFragments(t, fragments, []llm.ToolCallDelta{
		{ID: "call_abc", Arguments: `{"pa`},
		{ID: "call_abc", Arguments: `th":"main.go"}`},
	})
	if len(final.ToolCalls) != 1 || final.ToolCalls[0].Arguments != `{"path":"main.go"}` {
		t.Errorf("the terminal event lost the finished call: %+v", final.ToolCalls)
	}
	if final.Stop != llm.StopTool {
		t.Errorf("a round that ended in a call stopped for tools, got %q", final.Stop)
	}
}

// The boundary a broken stream is read at is unchanged by the fragments: what
// travels with the failure is the calls that are whole, and a call that was
// still being written is a fragment, not a call — however many fragments of
// it were reported on the way.
func TestStream_ABrokenStreamKeepsOnlyTheCallsThatAreWhole(t *testing.T) {
	idx0, idx1 := 0, 1
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		chunks := []openai.ChatCompletionStreamResponse{
			{Choices: []openai.ChatCompletionStreamChoice{{Delta: openai.ChatCompletionStreamChoiceDelta{ToolCalls: []openai.ToolCall{
				{Index: &idx0, ID: "call_done", Function: openai.FunctionCall{Name: "read_file", Arguments: `{"path":"main.go"}`}},
			}}}}},
			{Choices: []openai.ChatCompletionStreamChoice{{Delta: openai.ChatCompletionStreamChoiceDelta{ToolCalls: []openai.ToolCall{
				{Index: &idx1, ID: "call_half", Function: openai.FunctionCall{Name: "write_file", Arguments: `{"path":"ma`}},
			}}}}},
		}
		for _, chunk := range chunks {
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
		// The wire ends after the fragments, which is the recovery case: a
		// clean end would make the half-written call look complete.
		testhttp.Abort(w)
	}))
	defer srv.Close()

	p := newTestOpenAI(srv.URL+"/v1", "gpt-4o")
	ch, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "write main.go"}}, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}

	fragments, final := llmtest.DrainFragments(t, ch)
	llmtest.WantFragments(t, fragments, []llm.ToolCallDelta{
		{ID: "call_done", Arguments: `{"path":"main.go"}`},
		{ID: "call_half", Arguments: `{"path":"ma`},
	})
	if final.Err == nil {
		t.Fatalf("expected the stream to fail, got %+v", final)
	}
	if len(final.ToolCalls) != 1 || final.ToolCalls[0].ID != "call_done" {
		t.Errorf("expected only the finished call to survive, got %+v", final.ToolCalls)
	}
}

func TestIdle_OpenAIStreamThatGoesQuietFails(t *testing.T) {
	srv := llmtest.QuietServer(t)
	p := newTestOpenAI(srv.URL+"/v1", "gpt-4o")
	p.Deadline = idle.After(llmtest.Idle)

	ch, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hello"}}, llm.CompletionOpts{})
	llmtest.WentQuiet(t, ch, err)
}
