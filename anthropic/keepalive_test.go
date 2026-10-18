package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/rfizzle/llm"
)

// A gateway's ping is an event on the stream: it carries no text and nothing
// else, and the answer around it arrives whole. The Messages API's case; the
// chat and Responses dialects have theirs beside them.
func TestStream_AKeepaliveIsAnEvent(t *testing.T) {
	anthropic := func(w http.ResponseWriter) {
		sseEvent(w, "message_start", `{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","content":[],"model":"claude-opus-5","stop_reason":null,"usage":{"input_tokens":1,"output_tokens":1}}}`)
		fmt.Fprint(w, ": gateway keepalive\n\n")
		sseEvent(w, "ping", `{"type":"ping"}`)
		sseEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`)
		sseEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		sseEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}`)
		sseEvent(w, "message_stop", `{"type":"message_stop"}`)
	}

	cases := []struct {
		name  string
		want  int
		start func(t *testing.T) (<-chan llm.StreamEvent, error)
	}{
		{"anthropic", 2, func(t *testing.T) (<-chan llm.StreamEvent, error) {
			srv := anthropicSSEServer(t, anthropic)
			t.Cleanup(srv.Close)
			p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test", BaseURL: srv.URL})
			return p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, llm.CompletionOpts{})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ch, err := tc.start(t)
			if err != nil {
				t.Fatal(err)
			}
			var text string
			keepalives := 0
			var last llm.StreamEvent
			for ev := range ch {
				if ev.Keepalive {
					keepalives++
					if ev.Token != "" || ev.Thinking != "" || ev.ToolCallDelta != nil || len(ev.ToolCalls) > 0 || ev.Done || ev.Err != nil {
						t.Errorf("a keepalive carries nothing else, got %+v", ev)
					}
					continue
				}
				text += ev.Token
				last = ev
			}
			if keepalives != tc.want {
				t.Errorf("keepalive events = %d, want %d", keepalives, tc.want)
			}
			if text != "hi" || !last.Done || last.Err != nil {
				t.Errorf("the answer around a ping should arrive whole: text %q, last %+v", text, last)
			}
		})
	}
}

// The sniffer reads a ping that is split across reads, and never mistakes a
// line that only starts like one.
func TestStream_ThePingSnifferReadsAcrossReads(t *testing.T) {
	var got int
	var s pingSniffer
	s.onPing = func() { got++ }
	for _, piece := range []string{"event: pi", "ng\r\ndata: {}\n\nevent: pings\n", ": hi", "\n\n", "data: :\n"} {
		s.ReadCloser = readCloser(piece)
		if _, err := s.Read(make([]byte, 64)); err != nil {
			t.Fatal(err)
		}
	}
	if got != 2 {
		t.Errorf("pings = %d, want 2 (the event line and the comment)", got)
	}
}

type readCloser string

func (r readCloser) Read(b []byte) (int, error) { return copy(b, r), nil }
func (r readCloser) Close() error               { return nil }
