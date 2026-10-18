package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/internal/idle"
	"github.com/rfizzle/llm/internal/llmtest"
)

func TestStream_AnthropicReportsArgumentFragmentsInOrder(t *testing.T) {
	srv := anthropicSSEServer(t, func(w http.ResponseWriter) {
		sseEvent(w, "message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-opus-5","stop_reason":null,"usage":{"input_tokens":20,"output_tokens":1}}}`)
		sseEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"write_file","input":{}}}`)
		sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}`)
		sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"main.go\"}"}}`)
		sseEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		sseEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":15}}`)
		sseEvent(w, "message_stop", `{"type":"message_stop"}`)
	})
	defer srv.Close()

	p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test", BaseURL: srv.URL})
	ch, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "write main.go"}}, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}

	fragments, final := llmtest.DrainFragments(t, ch)
	llmtest.WantFragments(t, fragments, []llm.ToolCallDelta{
		{ID: "toolu_1", Arguments: `{"path":`},
		{ID: "toolu_1", Arguments: `"main.go"}`},
	})
	if len(final.ToolCalls) != 1 || final.ToolCalls[0].Name != "write_file" || final.ToolCalls[0].Arguments != `{"path":"main.go"}` {
		t.Errorf("the terminal event lost the finished call: %+v", final.ToolCalls)
	}
	if final.Stop != llm.StopTool {
		t.Errorf("a round that ended in a call stopped for tools, got %q", final.Stop)
	}
}

func TestIdle_AnthropicStreamThatGoesQuietFails(t *testing.T) {
	srv := llmtest.QuietServer(t)
	p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test", BaseURL: srv.URL})
	p.Deadline = idle.After(llmtest.Idle)

	ch, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hello"}}, llm.CompletionOpts{})
	llmtest.WentQuiet(t, ch, err)
}

// A model that thinks for far longer than the deadline is not a model that
// stopped: the thinking deltas are events, they push the deadline forward,
// and the turn ends the way it would have anyway. This is the failure the
// mechanism would cause if it watched for an answer rather than for silence.
func TestIdle_ASlowLiveThinkingStreamIsNotAFailure(t *testing.T) {
	const beats = 8
	srv := anthropicSSEServer(t, func(w http.ResponseWriter) {
		flusher, _ := w.(http.Flusher)
		sseEvent(w, "message_start", `{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[],"model":"claude-opus-5","usage":{"input_tokens":1,"output_tokens":1}}}`)
		sseEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`)
		flusher.Flush()
		for i := range beats {
			time.Sleep(llmtest.Beat)
			sseEvent(w, "content_block_delta", fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"beat %d "}}`, i))
			flusher.Flush()
		}
		sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-abc"}}`)
		sseEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		sseEvent(w, "content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`)
		sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"done"}}`)
		sseEvent(w, "content_block_stop", `{"type":"content_block_stop","index":1}`)
		sseEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":9}}`)
		sseEvent(w, "message_stop", `{"type":"message_stop"}`)
		flusher.Flush()
	})
	defer srv.Close()

	p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test", BaseURL: srv.URL})
	p.Deadline = idle.After(llmtest.Idle)

	ch, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "think"}}, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}
	var thinking, text string
	var last llm.StreamEvent
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("a live stream failed: %v", ev.Err)
		}
		thinking += ev.Thinking
		text += ev.Token
		if ev.Done {
			last = ev
		}
	}
	if !last.Done {
		t.Fatal("the stream never reached its terminal event")
	}
	if text != "done" {
		t.Errorf("the reply = %q, want %q", text, "done")
	}
	if thinking == "" {
		t.Error("the thinking deltas should have arrived")
	}
}
