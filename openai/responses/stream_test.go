package responses

import (
	"context"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/internal/idle"
	"github.com/rfizzle/llm/internal/llmtest"
)

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
	p := NewWith(nil, "k", "http://unused", "gpt-5", "gw")
	if got := p.Idle(); got != idle.Default {
		t.Errorf("deadline = %s, want %s", got, idle.Default)
	}
	p.SetStreamIdle(-1)
	if got := p.Idle(); got != 0 {
		t.Errorf("after being turned off, deadline = %s, want none", got)
	}
}
