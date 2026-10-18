package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/internal/llmtest"
)

// On one model, the classifier's flow is sent its tool and no format, and
// the backlog's is sent the format and no tools.
func TestAnthropic_ScopedNoSchemaSendsTheToolForThatFlowOnly(t *testing.T) {
	llmtest.Declare(t, "gw", map[string]llm.Declared{"claude-sonnet-5-5": llmtest.ScopedNoSchema(llm.FlowClassifier)})

	var body map[string]any
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = nil
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "text/event-stream")
		sseEvent(w, "message_start", `{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[],"model":"claude-sonnet-5-5","usage":{"input_tokens":1,"output_tokens":1}}}`)
		sseEvent(w, "message_stop", `{"type":"message_stop"}`)
	}))
	defer srv.Close()
	p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test", BaseURL: srv.URL})

	send := func(flow llm.Flow) {
		t.Helper()
		events, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, llm.CompletionOpts{
			Model:          "claude-sonnet-5-5",
			Flow:           flow,
			Effort:         llm.EffortLow,
			Tools:          []llm.Tool{{Name: "answer", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}},
			ToolChoice:     llm.ToolChoiceAuto,
			ResponseSchema: &llm.ResponseSchema{Name: "answer", Schema: json.RawMessage(`{"type":"object","additionalProperties":false}`)},
		})
		if err != nil {
			t.Fatal(err)
		}
		_, _, _, _ = llmtest.Drain(t, events)
	}

	send(llm.FlowClassifier)
	config, _ := body["output_config"].(map[string]any)
	if _, ok := config["format"]; ok {
		t.Errorf("classifier: no format may be sent, got %v", config["format"])
	}
	if tools, _ := body["tools"].([]any); len(tools) != 1 {
		t.Errorf("classifier: the tool must be offered, got %v", body["tools"])
	}
	if choice, _ := body["tool_choice"].(map[string]any); choice["type"] != "auto" {
		t.Errorf("classifier: tool_choice should be auto, got %v", body["tool_choice"])
	}

	send(llm.FlowBacklog)
	config, _ = body["output_config"].(map[string]any)
	if format, _ := config["format"].(map[string]any); format["type"] != "json_schema" {
		t.Errorf("backlog: the format must be sent, got %v", body["output_config"])
	}
	if _, ok := body["tools"]; ok {
		t.Error("backlog: a request carrying a schema must not also offer tools")
	}
}

// prompt_cache = false says the endpoint keeps no cache, and changes nothing
// that goes out: the marks are still sent on the Messages API, and the
// request is byte for byte what it was. The gateway route's half is beside
// the cache-mark transport's tests.
func TestDeclarations_PromptCacheOffChangesNoRequest(t *testing.T) {
	const model = "claude-sonnet-5-5"
	var body []byte
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		sseEvent(w, "message_start", `{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[],"model":"claude-sonnet-5-5","usage":{"input_tokens":1,"output_tokens":1}}}`)
		sseEvent(w, "message_stop", `{"type":"message_stop"}`)
	}))
	defer srv.Close()
	p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test", BaseURL: srv.URL})
	send := func() []byte {
		t.Helper()
		events, err := p.StreamCompletion(context.Background(), []llm.Message{
			{Role: llm.RoleSystem, Content: "sys"},
			{Role: llm.RoleUser, Content: "hi"},
		}, llm.CompletionOpts{Model: model})
		if err != nil {
			t.Fatal(err)
		}
		_, _, _, _ = llmtest.Drain(t, events)
		return body
	}
	before := send()
	llmtest.Declare(t, "gw", map[string]llm.Declared{model: {Model: map[string]any{llm.KeyPromptCache: false}}})
	if !llm.CapabilitiesFor(model).NoPromptCache {
		t.Fatal("the declaration did not narrow the model")
	}
	after := send()
	if !bytes.Equal(before, after) {
		t.Errorf("the request changed:\n%s\n%s", before, after)
	}
	if !bytes.Contains(after, []byte(`"cache_control"`)) {
		t.Errorf("the marks must still be sent: %s", after)
	}
}
