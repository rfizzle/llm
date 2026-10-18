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

func TestStream_OpenAIResponsesReportsArgumentFragmentsInOrder(t *testing.T) {
	srv := responsesServer(t, []string{
		`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1","call_id":"call_abc","name":"write_file"}}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"pa"}`,
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"th\":\"main.go\"}"}`,
		`data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"function_call","id":"fc_1","call_id":"call_abc","name":"write_file","arguments":"{\"path\":\"main.go\"}"}]}}`,
	}, nil)

	p := newTestResponses(srv.URL, "gpt-5")
	ch, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "write main.go"}}, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}

	fragments, final := llmtest.DrainFragments(t, ch)
	llmtest.WantFragments(t, fragments, []llm.ToolCallDelta{
		{ID: "call_abc", Arguments: `{"pa`},
		{ID: "call_abc", Arguments: `th":"main.go"}`},
	})
	if len(final.ToolCalls) != 1 || final.ToolCalls[0].ID != "call_abc" {
		t.Errorf("the terminal event lost the finished call: %+v", final.ToolCalls)
	}
	if final.Stop != llm.StopTool {
		t.Errorf("a round that ended in a call stopped for tools, got %q", final.Stop)
	}
}

// A fragment addressed to an item nobody opened is dropped rather than
// reported under a guessed id: a gateway that replays the argument deltas
// without the item event that names the call has nothing to address them to.
func TestStream_OpenAIResponsesDropsFragmentsItCannotAddress(t *testing.T) {
	srv := responsesServer(t, []string{
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"path\":\"main.go\"}"}`,
		`data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"function_call","id":"fc_1","call_id":"call_abc","name":"write_file","arguments":"{\"path\":\"main.go\"}"}]}}`,
	}, nil)

	p := newTestResponses(srv.URL, "gpt-5")
	ch, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "write main.go"}}, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}

	fragments, final := llmtest.DrainFragments(t, ch)
	if len(fragments) != 0 {
		t.Errorf("expected no fragments, got %+v", fragments)
	}
	if len(final.ToolCalls) != 1 {
		t.Errorf("the terminal event lost the finished call: %+v", final.ToolCalls)
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

func TestIdle_OpenAIResponsesStreamThatGoesQuietFails(t *testing.T) {
	srv := llmtest.QuietServer(t)
	p := newTestResponses(srv.URL, "gpt-5")
	p.Deadline = idle.After(llmtest.Idle)

	ch, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hello"}}, llm.CompletionOpts{})
	llmtest.WentQuiet(t, ch, err)
}

// A provider built from a finished client — every gateway profile's — starts
// with no deadline written down, and that has to read as the built-in one
// rather than as none.
func TestIdle_AProviderBuiltFromAClientStillHasTheDeadline(t *testing.T) {
	p := NewResponsesWith(nil, "k", "http://unused", "gpt-5", "gw")
	if got := p.Idle(); got != idle.Default {
		t.Errorf("deadline = %s, want %s", got, idle.Default)
	}
	p.SetStreamIdle(-1)
	if got := p.Idle(); got != 0 {
		t.Errorf("after being turned off, deadline = %s, want none", got)
	}
}
