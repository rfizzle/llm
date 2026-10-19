package responses

import (
	"errors"
	"io"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/failure"
	"github.com/rfizzle/llm/internal/idle"
)

// errReader fails partway through, standing in for a connection that dropped.
type errReader struct {
	data []byte
	err  error
	sent bool
}

func (r *errReader) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		n := copy(p, r.data)
		return n, nil
	}
	return 0, r.err
}

func (r *errReader) Close() error { return nil }

func TestStreamResponses_BrokenStreamKeepsTheFinishedCalls(t *testing.T) {
	// One completed function-call item, then the connection dies.
	body := &errReader{
		data: []byte(`data: {"type":"response.output_item.done","item":{"type":"function_call","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"a.go\"}"}}` + "\n\n"),
		err:  io.ErrUnexpectedEOF,
	}
	classify := failure.NewClassifier("openai", "OPENAI_API_KEY", "sk-xxxx1234")

	var last llm.StreamEvent
	for ev := range streamResponses(body, classify, &idle.Watch{}, nil) {
		last = ev
	}
	if last.Err == nil {
		t.Fatal("a dropped connection is still a failure")
	}
	if len(last.ToolCalls) != 1 || last.ToolCalls[0].Name != "read_file" {
		t.Fatalf("the finished call should travel with the failure, got %+v", last.ToolCalls)
	}
	f, ok := failure.As(last.Err)
	if !ok {
		t.Fatalf("the failure should still be classified, got %T", last.Err)
	}
	if !errors.Is(f, failure.ErrNetwork) {
		t.Errorf("class = %q, want network", f.Class)
	}
}
