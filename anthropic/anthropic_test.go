package anthropic

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/attachment"
	"github.com/rfizzle/llm/cache"
	"github.com/rfizzle/llm/capability"
	"github.com/rfizzle/llm/catalog"
	"github.com/rfizzle/llm/effort"
	"github.com/rfizzle/llm/internal/idle"
	"github.com/rfizzle/llm/internal/llmtest"
	"github.com/rfizzle/llm/internal/testhttp"
)

func TestNewAnthropic_RequiresKey(t *testing.T) {
	t.Setenv("ACME_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	if _, err := New(llm.ResolveOpts{}); err == nil {
		t.Fatal("expected error without API key")
	}
}

func TestNewAnthropic_DefaultModel(t *testing.T) {
	p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test"})
	if p.model != "claude-opus-5" {
		t.Errorf("expected default model 'claude-opus-5', got %q", p.model)
	}
	if p.Name() != "anthropic" {
		t.Errorf("expected name 'anthropic', got %q", p.Name())
	}
}

func TestAnthropic_Registered(t *testing.T) {
	if _, err := llm.Resolve("anthropic", llm.ResolveOpts{APIKey: "sk-test"}); err != nil {
		t.Fatalf("anthropic should be registered: %v", err)
	}
	if d := llm.Defaults("anthropic"); d.Model != "claude-opus-5" {
		t.Errorf("expected default model in registry, got %q", d.Model)
	}
}

func TestToAnthropicMessages_SystemExtracted(t *testing.T) {
	system, msgs := toAnthropicMessages([]llm.Message{
		{Role: llm.RoleSystem, Content: "be helpful"},
		{Role: llm.RoleUser, Content: "hi"},
		{Role: llm.RoleAssistant, Content: "hello"},
	})
	if system != "be helpful" {
		t.Errorf("expected system extracted, got %q", system)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (system pulled out), got %d", len(msgs))
	}
}

func TestToAnthropicMessages_ToolResultsMergeIntoOneUserTurn(t *testing.T) {
	_, msgs := toAnthropicMessages([]llm.Message{
		{Role: llm.RoleUser, Content: "check both files"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "t1", Name: "read_file", Arguments: `{"path":"a"}`},
			{ID: "t2", Name: "read_file", Arguments: `{"path":"b"}`},
		}},
		{Role: llm.RoleTool, Content: "content a", ToolCallID: "t1"},
		{Role: llm.RoleTool, Content: "content b", ToolCallID: "t2"},
	})
	// user, assistant(tool_use x2), user(tool_result x2)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages with merged tool results, got %d", len(msgs))
	}
	last := msgs[2]
	if string(last.Role) != "user" {
		t.Fatalf("tool results should be a user turn, got %s", last.Role)
	}
	if len(last.Content) != 2 {
		t.Fatalf("expected both tool results in one turn, got %d blocks", len(last.Content))
	}
	assistant := msgs[1]
	if len(assistant.Content) != 2 {
		t.Fatalf("expected 2 tool_use blocks on assistant turn, got %d", len(assistant.Content))
	}
	if assistant.Content[0].OfToolUse == nil || assistant.Content[0].OfToolUse.ID != "t1" {
		t.Error("expected tool_use block with ID t1")
	}
}

func TestToAnthropicTools_SchemaMapped(t *testing.T) {
	tools := toAnthropicTools([]llm.Tool{{
		Name:        "read_file",
		Description: "Read a file",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
	}})
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tp := tools[0].OfTool
	if tp == nil {
		t.Fatal("expected OfTool set")
	}
	if tp.Name != "read_file" {
		t.Errorf("unexpected name %q", tp.Name)
	}
	schema := marshalled(t, tp.InputSchema)
	if schema["type"] != "object" {
		t.Errorf("input schema type = %v, want object", schema["type"])
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map, got %T", schema["properties"])
	}
	if _, ok := props["path"]; !ok {
		t.Error("expected 'path' property in input schema")
	}
	if req, ok := schema["required"].([]any); !ok || len(req) != 1 || req[0] != "path" {
		t.Errorf("expected required [path], got %v", schema["required"])
	}
}

// marshalled renders a request fragment the way the SDK will send it. The
// assertions that matter here are about the wire: a schema field the struct
// has no room for is only visible once it has been marshalled.
func marshalled(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

// A tool schema is not the harness's to simplify: an MCP server's schemas
// arrive here as written, and one that factors a nested shape into $defs and
// reaches it with $ref described the tool to the model as an object with two
// free-form fields once the converter had rebuilt it from properties and
// required alone.
func TestToAnthropicTools_NestedDefsSurviveTheRoundTrip(t *testing.T) {
	raw := `{"type":"object","properties":{"where":{"$ref":"#/$defs/point"}},` +
		`"required":["where"],"additionalProperties":false,` +
		`"$defs":{"point":{"type":"object","properties":{"x":{"type":"number"},` +
		`"y":{"type":"number"}},"required":["x","y"]}}}`
	tools := toAnthropicTools([]llm.Tool{{Name: "move", Parameters: json.RawMessage(raw)}})
	schema := marshalled(t, tools[0].OfTool.InputSchema)

	var want map[string]any
	if err := json.Unmarshal([]byte(raw), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(schema, want) {
		t.Errorf("schema must go out as written:\n got %v\nwant %v", schema, want)
	}
}

// The two values the harness has a use for reach the wire, and nothing else
// does: an unset choice sends no field, which is the dialect's own auto.
func TestAnthropic_ToolChoiceOnTheRequest(t *testing.T) {
	var body map[string]any
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = nil
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "text/event-stream")
		sseEvent(w, "message_start", `{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[],"model":"claude-opus-5","usage":{"input_tokens":1,"output_tokens":1}}}`)
		sseEvent(w, "message_stop", `{"type":"message_stop"}`)
	}))
	defer srv.Close()

	p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test", BaseURL: srv.URL})
	msgs := []llm.Message{{Role: llm.RoleUser, Content: "hi"}}
	tools := []llm.Tool{{Name: "read_file", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}}

	for _, tc := range []struct {
		choice string
		want   any
	}{
		{llm.ToolChoiceAuto, map[string]any{"type": "auto"}},
		{llm.ToolChoiceNone, map[string]any{"type": "none"}},
		{"", nil},
	} {
		events, err := p.StreamCompletion(context.Background(), msgs,
			llm.CompletionOpts{Tools: tools, ToolChoice: tc.choice})
		if err != nil {
			t.Fatal(err)
		}
		_, _, _, _ = llmtest.Drain(t, events)
		if !reflect.DeepEqual(body["tool_choice"], tc.want) {
			t.Errorf("choice %q sent tool_choice %v, want %v", tc.choice, body["tool_choice"], tc.want)
		}
		// Whatever the choice, the tools stay on the request: they are the
		// head of the prefix the API caches, and dropping them to stop a
		// tool call would rebuild the whole head.
		if _, ok := body["tools"]; !ok {
			t.Errorf("choice %q must keep the tools on the request", tc.choice)
		}
	}
}

func sseEvent(w http.ResponseWriter, event, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}

func anthropicSSEServer(t *testing.T, write func(w http.ResponseWriter)) *testhttp.Server {
	t.Helper()
	return llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		write(w)
	}))
}

func TestAnthropic_StreamText(t *testing.T) {
	srv := anthropicSSEServer(t, func(w http.ResponseWriter) {
		sseEvent(w, "message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-opus-5","stop_reason":null,"usage":{"input_tokens":12,"output_tokens":1}}}`)
		sseEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ls "}}`)
		sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"-la"}}`)
		sseEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		sseEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`)
		sseEvent(w, "message_stop", `{"type":"message_stop"}`)
	})
	defer srv.Close()

	p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test", BaseURL: srv.URL})
	events, err := p.StreamCompletion(context.Background(), []llm.Message{
		{Role: llm.RoleSystem, Content: "sys"},
		{Role: llm.RoleUser, Content: "list files"},
	}, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}

	text, toolCalls, usage, streamErr := llmtest.Drain(t, events)
	if streamErr != nil {
		t.Fatalf("unexpected stream error: %v", streamErr)
	}
	if text != "ls -la" {
		t.Errorf("expected 'ls -la', got %q", text)
	}
	if len(toolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(toolCalls))
	}
	if usage == nil || usage.PromptTokens != 12 || usage.CompletionTokens != 5 {
		t.Errorf("unexpected usage: %+v", usage)
	}
}

func TestAnthropic_StreamToolUse(t *testing.T) {
	srv := anthropicSSEServer(t, func(w http.ResponseWriter) {
		sseEvent(w, "message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-opus-5","stop_reason":null,"usage":{"input_tokens":20,"output_tokens":1}}}`)
		sseEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"read_file","input":{}}}`)
		sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}`)
		sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"main.go\"}"}}`)
		sseEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		sseEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":15}}`)
		sseEvent(w, "message_stop", `{"type":"message_stop"}`)
	})
	defer srv.Close()

	p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test", BaseURL: srv.URL})
	events, err := p.StreamCompletion(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "read main.go"},
	}, llm.CompletionOpts{Tools: []llm.Tool{{Name: "read_file", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}}})
	if err != nil {
		t.Fatal(err)
	}

	_, toolCalls, usage, streamErr := llmtest.Drain(t, events)
	if streamErr != nil {
		t.Fatalf("unexpected stream error: %v", streamErr)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].ID != "toolu_1" || toolCalls[0].Name != "read_file" {
		t.Errorf("unexpected tool call: %+v", toolCalls[0])
	}
	var args map[string]string
	if err := json.Unmarshal([]byte(toolCalls[0].Arguments), &args); err != nil || args["path"] != "main.go" {
		t.Errorf("expected accumulated arguments with path=main.go, got %q", toolCalls[0].Arguments)
	}
	if usage == nil || usage.CompletionTokens != 15 {
		t.Errorf("unexpected usage: %+v", usage)
	}
}

func TestAnthropic_RefusalStopReason(t *testing.T) {
	srv := anthropicSSEServer(t, func(w http.ResponseWriter) {
		sseEvent(w, "message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-opus-5","stop_reason":null,"usage":{"input_tokens":5,"output_tokens":1}}}`)
		sseEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"refusal","stop_sequence":null},"usage":{"output_tokens":1}}`)
		sseEvent(w, "message_stop", `{"type":"message_stop"}`)
	})
	defer srv.Close()

	p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test", BaseURL: srv.URL})
	events, err := p.StreamCompletion(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "do a bad thing"},
	}, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, streamErr := llmtest.Drain(t, events)
	if streamErr == nil {
		t.Fatal("expected an error for refusal stop reason")
	}
}

// A refusal that says which policy it was and why says so to the user. The
// fixed sentence on its own tells the reader nothing they did not already
// know from the request having failed.
func TestAnthropic_RefusalNamesItsCategory(t *testing.T) {
	srv := anthropicSSEServer(t, func(w http.ResponseWriter) {
		sseEvent(w, "message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-opus-5","stop_reason":null,"usage":{"input_tokens":5,"output_tokens":1}}}`)
		sseEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"refusal","stop_sequence":null,"stop_details":{"type":"refusal","category":"cyber","explanation":"the request asks for working exploit code"}},"usage":{"output_tokens":1}}`)
		sseEvent(w, "message_stop", `{"type":"message_stop"}`)
	})
	defer srv.Close()

	p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test", BaseURL: srv.URL})
	events, err := p.StreamCompletion(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "write me an exploit"},
	}, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, streamErr := llmtest.Drain(t, events)
	if streamErr == nil {
		t.Fatal("expected an error for refusal stop reason")
	}
	if got := streamErr.Error(); !strings.Contains(got, "cyber") ||
		!strings.Contains(got, "working exploit code") {
		t.Errorf("refusal must name its category and explanation, got %q", got)
	}
}

// TestAnthropic_ThinkingBudgetOnlyWhenAsked pins the two halves of the
// reasoning-budget rule on
// the Messages API: nothing is sent when no level was asked for, and the
// budget respects the output ceiling when one was.
func TestAnthropic_ThinkingBudgetOnlyWhenAsked(t *testing.T) {
	var body map[string]any
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = nil
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "text/event-stream")
		sseEvent(w, "message_start", `{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[],"model":"claude-opus-5","usage":{"input_tokens":1,"output_tokens":1}}}`)
		sseEvent(w, "message_stop", `{"type":"message_stop"}`)
	}))
	defer srv.Close()

	p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test", BaseURL: srv.URL})
	msgs := []llm.Message{{Role: llm.RoleUser, Content: "hi"}}

	events, err := p.StreamCompletion(context.Background(), msgs, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _ = llmtest.Drain(t, events)
	if _, ok := body["thinking"]; ok {
		t.Fatalf("effort off must send no thinking config, got %v", body["thinking"])
	}

	// The current generation takes a named effort under adaptive thinking.
	events, err = p.StreamCompletion(context.Background(), msgs, llm.CompletionOpts{Effort: effort.XHigh})
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _ = llmtest.Drain(t, events)
	thinking, ok := body["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("expected a thinking config, got %v", body["thinking"])
	}
	if thinking["type"] != "adaptive" {
		t.Errorf("expected adaptive thinking on claude-opus-5, got %v", thinking["type"])
	}
	if got := body["output_config"].(map[string]any)["effort"]; got != "xhigh" {
		t.Errorf("effort = %v, want xhigh", got)
	}

	// A budget-only model takes the ladder instead, and a rung it lacks
	// becomes the highest it has.
	legacy := []llm.Message{{Role: llm.RoleUser, Content: "hi"}}
	events, err = p.StreamCompletion(context.Background(), legacy, llm.CompletionOpts{Model: "claude-haiku-4-5", Effort: effort.Max})
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _ = llmtest.Drain(t, events)
	thinking = body["thinking"].(map[string]any)
	if thinking["type"] != "enabled" {
		t.Errorf("expected enabled thinking on haiku 4.5, got %v", thinking["type"])
	}
	if got := thinking["budget_tokens"].(float64); got != 24576 {
		t.Errorf("budget = %v, want the high level's 24576", got)
	}
	if _, ok := body["output_config"]; ok {
		t.Errorf("a budgeted model must not be sent output_config, got %v", body["output_config"])
	}

	// A capped output leaves the answer room: the budget clamps to the cap
	// less the answer floor rather than swallowing the whole ceiling.
	events, err = p.StreamCompletion(context.Background(), legacy, llm.CompletionOpts{Model: "claude-haiku-4-5", Effort: effort.High, MaxTokens: 8192})
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _ = llmtest.Drain(t, events)
	thinking = body["thinking"].(map[string]any)
	if got := thinking["budget_tokens"].(float64); got != 8192-anthropicAnswerFloor {
		t.Errorf("clamped budget = %v, want %d", got, 8192-anthropicAnswerFloor)
	}

	// A ceiling with nothing left under the answer floor asks for no
	// thinking at all. Without the guard the subtraction goes negative, the
	// budget reads that as "no ceiling", and a budget larger than max_tokens
	// goes out — which the Messages API refuses outright.
	events, err = p.StreamCompletion(context.Background(), legacy, llm.CompletionOpts{Model: "claude-haiku-4-5", Effort: effort.Low, MaxTokens: 2048})
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _ = llmtest.Drain(t, events)
	if _, ok := body["thinking"]; ok {
		t.Errorf("a ceiling under the answer floor must send no thinking, got %v", body["thinking"])
	}

	// A model that has no thinking is sent none, whatever was asked.
	events, err = p.StreamCompletion(context.Background(), legacy, llm.CompletionOpts{Model: "claude-3-5-sonnet-latest", Effort: effort.High})
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _ = llmtest.Drain(t, events)
	if _, ok := body["thinking"]; ok {
		t.Errorf("a model without thinking must be sent none, got %v", body["thinking"])
	}
}

// TestAnthropic_ThinkingBlocksSurviveIntoTheNextRequest is the requirement
// that makes reasoning blocks a data structure rather than a display detail:
// with thinking on, an assistant turn that requested tools must carry the
// thinking that led to them or the follow-up request is rejected.
func TestAnthropic_ThinkingBlocksSurviveIntoTheNextRequest(t *testing.T) {
	srv := anthropicSSEServer(t, func(w http.ResponseWriter) {
		sseEvent(w, "message_start", `{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[],"model":"claude-opus-5","usage":{"input_tokens":1,"output_tokens":1}}}`)
		sseEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`)
		sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"weighing it"}}`)
		sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-abc"}}`)
		sseEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		sseEvent(w, "content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"t1","name":"read_file","input":{}}}`)
		sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"a\"}"}}`)
		sseEvent(w, "content_block_stop", `{"type":"content_block_stop","index":1}`)
		sseEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":9}}`)
		sseEvent(w, "message_stop", `{"type":"message_stop"}`)
	})
	defer srv.Close()

	p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test", BaseURL: srv.URL})
	events, err := p.StreamCompletion(context.Background(),
		[]llm.Message{{Role: llm.RoleUser, Content: "read a"}}, llm.CompletionOpts{Effort: effort.Medium})
	if err != nil {
		t.Fatal(err)
	}

	var final llm.StreamEvent
	for ev := range events {
		if ev.Done {
			final = ev
		}
	}
	if len(final.ToolCalls) != 1 {
		t.Fatalf("expected the tool call, got %v", final.ToolCalls)
	}
	if len(final.Reasoning) != 1 {
		t.Fatalf("expected the thinking block to ride the terminal event, got %v", final.Reasoning)
	}
	if final.Reasoning[0].Text != "weighing it" || final.Reasoning[0].Signature != "sig-abc" {
		t.Fatalf("thinking block lost its text or signature: %+v", final.Reasoning[0])
	}

	// And it goes back on the wire ahead of the tool_use it explains.
	_, msgs := toAnthropicMessages([]llm.Message{
		{Role: llm.RoleUser, Content: "read a"},
		{Role: llm.RoleAssistant, ToolCalls: final.ToolCalls, Reasoning: final.Reasoning},
	})
	assistant := msgs[1]
	if len(assistant.Content) != 2 {
		t.Fatalf("expected thinking + tool_use on the assistant turn, got %d blocks", len(assistant.Content))
	}
	if assistant.Content[0].OfThinking == nil {
		t.Fatal("thinking must lead the assistant turn")
	}
	if assistant.Content[0].OfThinking.Signature != "sig-abc" {
		t.Errorf("signature must go back exactly as received, got %q", assistant.Content[0].OfThinking.Signature)
	}
	if assistant.Content[1].OfToolUse == nil {
		t.Error("expected the tool_use block after the thinking")
	}
}

// TestAnthropicReasoning_DropsUnsignedBlocks: a stream that broke before the
// signature arrived kept a block the API will not take back, and sending it
// fails the request it was kept for.
func TestAnthropicReasoning_DropsUnsignedBlocks(t *testing.T) {
	_, msgs := toAnthropicMessages([]llm.Message{
		{Role: llm.RoleAssistant, Content: "x", Reasoning: []llm.ReasoningBlock{
			{Text: "unsigned"},
			{Redacted: "opaque"},
		}},
	})
	blocks := msgs[0].Content
	if len(blocks) != 2 {
		t.Fatalf("expected the redacted block and the text, got %d blocks", len(blocks))
	}
	if blocks[0].OfRedactedThinking == nil || blocks[0].OfRedactedThinking.Data != "opaque" {
		t.Errorf("redacted thinking must survive unchanged, got %+v", blocks[0])
	}
}

// The Messages API reports the cached parts of the prompt beside input_tokens
// rather than inside it. Usage promises a prompt count that already contains
// them, so the sum has to happen in the converter.
func TestAnthropic_UsageFoldsTheCachedPartsIntoThePromptCount(t *testing.T) {
	srv := anthropicSSEServer(t, func(w http.ResponseWriter) {
		sseEvent(w, "message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-opus-5","stop_reason":null,"usage":{"input_tokens":12,"output_tokens":1,"cache_read_input_tokens":900,"cache_creation_input_tokens":88}}}`)
		sseEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`)
		sseEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		sseEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`)
		sseEvent(w, "message_stop", `{"type":"message_stop"}`)
	})
	defer srv.Close()

	p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test", BaseURL: srv.URL})
	events, err := p.StreamCompletion(context.Background(), []llm.Message{
		{Role: llm.RoleSystem, Content: "sys"},
		{Role: llm.RoleUser, Content: "hi"},
	}, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}

	_, _, usage, streamErr := llmtest.Drain(t, events)
	if streamErr != nil {
		t.Fatalf("unexpected stream error: %v", streamErr)
	}
	if usage == nil {
		t.Fatal("no usage reported")
	}
	if want := 12 + 900 + 88; usage.PromptTokens != want {
		t.Errorf("PromptTokens must count every billed input token: want %d got %d", want, usage.PromptTokens)
	}
	if usage.CachedTokens != 900 {
		t.Errorf("CachedTokens: want 900 got %d", usage.CachedTokens)
	}
	if usage.CacheCreationTokens != 88 {
		t.Errorf("CacheCreationTokens: want 88 got %d", usage.CacheCreationTokens)
	}
	// The parts are subsets, so what was read fresh is what is left.
	if fresh := usage.PromptTokens - usage.CachedTokens - usage.CacheCreationTokens; fresh != 12 {
		t.Errorf("the parts must not overlap: fresh should be 12, got %d", fresh)
	}
}

// The lifetime the session was resolved with is the one the head marker
// carries on the wire, and a hand-edited value the parser cannot read leaves
// the default standing rather than failing the session.
func TestAnthropic_CacheLifetimeOnTheRequest(t *testing.T) {
	var body map[string]any
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = nil
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "text/event-stream")
		sseEvent(w, "message_start", `{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[],"model":"claude-opus-5","usage":{"input_tokens":1,"output_tokens":1}}}`)
		sseEvent(w, "message_stop", `{"type":"message_stop"}`)
	}))
	defer srv.Close()

	for _, tc := range []struct{ configured, want string }{
		{configured: "", want: string(cache.DefaultTTL)},
		{configured: "5m", want: string(cache.TTL5m)},
		{configured: "a fortnight", want: string(cache.DefaultTTL)},
	} {
		p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test", BaseURL: srv.URL, CacheTTL: tc.configured})
		events, err := p.StreamCompletion(context.Background(), []llm.Message{
			{Role: llm.RoleSystem, Content: "be helpful"},
			{Role: llm.RoleUser, Content: "hi"},
		}, llm.CompletionOpts{})
		if err != nil {
			t.Fatal(err)
		}
		_, _, _, _ = llmtest.Drain(t, events)

		system, _ := body["system"].([]any)
		if len(system) != 1 {
			t.Fatalf("%q: the request has no head to mark: %v", tc.configured, body["system"])
		}
		block, _ := system[0].(map[string]any)
		control, _ := block["cache_control"].(map[string]any)
		if control["ttl"] != tc.want {
			t.Errorf("%q: the head's marker is %v, want %q", tc.configured, control["ttl"], tc.want)
		}
	}
}

func TestToAnthropicMessages_ToolResultCarriesAnImage(t *testing.T) {
	_, msgs := toAnthropicMessages([]llm.Message{
		{Role: llm.RoleUser, Content: "read the logo"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "t1", Name: "read_file", Arguments: `{"path":"logo.png"}`}}},
		{Role: llm.RoleTool, Content: "logo.png is an image", ToolCallID: "t1", Attachments: []attachment.Attachment{
			{Kind: attachment.Image, Name: "logo.png", MediaType: "image/png", Data: []byte{1, 2, 3}},
			{Kind: attachment.Text, Name: "notes.txt", MediaType: "text/plain", Data: []byte("hi")},
		}},
	})
	result := msgs[len(msgs)-1].Content[0].OfToolResult
	if result == nil {
		t.Fatal("expected a tool_result block")
	}
	if len(result.Content) != 2 {
		t.Fatalf("expected the text and the image and nothing else, got %d blocks", len(result.Content))
	}
	if result.Content[0].OfText == nil || result.Content[0].OfText.Text != "logo.png is an image" {
		t.Error("the notice should lead the block")
	}
	img := result.Content[1].OfImage
	if img == nil || img.Source.OfBase64 == nil {
		t.Fatal("expected the image as an inline base64 source")
	}
	if got := string(img.Source.OfBase64.MediaType); got != "image/png" {
		t.Errorf("expected image/png, got %q", got)
	}
}

func TestToAnthropicMessages_ToolResultWithoutAnImageIsTextAlone(t *testing.T) {
	_, msgs := toAnthropicMessages([]llm.Message{
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "t1", Name: "read_file"}}},
		{Role: llm.RoleTool, Content: "error: no such file", ToolCallID: "t1"},
	})
	result := msgs[len(msgs)-1].Content[0].OfToolResult
	if result == nil || len(result.Content) != 1 {
		t.Fatalf("expected one text block, got %+v", result)
	}
	if result.IsError.Value != true {
		t.Error("an error result should still be marked as one")
	}
}

// The Messages API carries the schema under the output configuration that
// already carries the thinking effort, so the two have to survive each other.
func TestAnthropic_SchemaRidesTheOutputConfigBesideTheEffort(t *testing.T) {
	var body map[string]any
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = nil
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "text/event-stream")
		sseEvent(w, "message_start", `{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[],"model":"claude-opus-5","usage":{"input_tokens":1,"output_tokens":1}}}`)
		sseEvent(w, "message_stop", `{"type":"message_stop"}`)
	}))
	defer srv.Close()

	p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test", BaseURL: srv.URL})
	msgs := []llm.Message{{Role: llm.RoleUser, Content: "hi"}}
	opts := llm.CompletionOpts{
		Effort:         effort.Low,
		Tools:          []llm.Tool{{Name: "decide", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}},
		ToolChoice:     llm.ToolChoiceAuto,
		ResponseSchema: &llm.ResponseSchema{Name: "verdict", Schema: json.RawMessage(`{"type":"object","additionalProperties":false}`)},
	}

	events, err := p.StreamCompletion(context.Background(), msgs, opts)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _ = llmtest.Drain(t, events)
	config, _ := body["output_config"].(map[string]any)
	if config == nil || config["effort"] != "low" {
		t.Fatalf("the effort must survive the schema, got %v", body["output_config"])
	}
	format, _ := config["format"].(map[string]any)
	if format == nil || format["type"] != "json_schema" {
		t.Fatalf("format = %v", config["format"])
	}
	if schema, _ := format["schema"].(map[string]any); schema["additionalProperties"] != false {
		t.Errorf("the schema must travel whole, got %v", format["schema"])
	}
	if _, ok := body["tools"]; ok {
		t.Error("a request carrying a schema must not also offer tools")
	}

	// A generation that predates the field keeps the tools and the choice.
	opts.Model = "claude-3-5-sonnet-20241022"
	events, err = p.StreamCompletion(context.Background(), msgs, opts)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _ = llmtest.Drain(t, events)
	if config, _ := body["output_config"].(map[string]any); config["format"] != nil {
		t.Errorf("a model without structured output must be sent no format, got %v", body["output_config"])
	}
	if _, ok := body["tools"]; !ok {
		t.Error("the tool path must be untouched")
	}
}

// A model declared to take no schema is sent the tool the classifier already
// offers, with tool_choice auto, and no format; the effort stays in
// output_config.
func TestAnthropic_NoSchemaSendsTheToolAndNoOutputFormat(t *testing.T) {
	capability.SetLookup(func(model string) (capability.Capabilities, bool) {
		return capability.Capabilities{Reasoning: true, Adaptive: true, NoSchema: true}, model == "claude-sonnet-5-5"
	})
	defer capability.SetLookup(nil)

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
	events, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, llm.CompletionOpts{
		Model:          "claude-sonnet-5-5",
		Effort:         effort.Low,
		Tools:          []llm.Tool{{Name: "permission_decision", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}},
		ToolChoice:     llm.ToolChoiceAuto,
		ResponseSchema: &llm.ResponseSchema{Name: "verdict", Schema: json.RawMessage(`{"type":"object","additionalProperties":false}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _ = llmtest.Drain(t, events)
	config, _ := body["output_config"].(map[string]any)
	if config == nil || config["effort"] != "low" {
		t.Fatalf("the effort must stay in output_config, got %v", body["output_config"])
	}
	if _, ok := config["format"]; ok {
		t.Errorf("no format may be sent, got %v", config["format"])
	}
	tools, _ := body["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("the tool must be offered, got %v", body["tools"])
	}
	if choice, _ := body["tool_choice"].(map[string]any); choice["type"] != "auto" {
		t.Errorf("tool_choice should be auto, got %v", body["tool_choice"])
	}
}

func TestAnthropicStop_MapsEveryReasonTheDialectNames(t *testing.T) {
	cases := []struct {
		reason anthropic.StopReason
		want   llm.StopReason
	}{
		{anthropic.StopReasonEndTurn, llm.StopEnd},
		{anthropic.StopReasonToolUse, llm.StopTool},
		{anthropic.StopReasonMaxTokens, llm.StopLength},
		{anthropic.StopReasonRefusal, llm.StopRefusal},
		{anthropic.StopReasonStopSequence, llm.StopOther},
		{anthropic.StopReasonPauseTurn, llm.StopOther},
		{anthropic.StopReasonModelContextWindowExceeded, llm.StopOther},
		// A reason this dialect has not invented yet, and a stream that
		// ended before the delta that carries one.
		{anthropic.StopReason("something_new"), llm.StopOther},
		{"", llm.StopEnd},
	}
	for _, tc := range cases {
		if got := anthropicStop(tc.reason); got != tc.want {
			t.Errorf("anthropicStop(%q) = %q, want %q", tc.reason, got, tc.want)
		}
	}
}

func TestAnthropic_CeilingKeepsTheTextAndDropsTheUnfinishedCall(t *testing.T) {
	srv := anthropicSSEServer(t, func(w http.ResponseWriter) {
		sseEvent(w, "message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-opus-5","stop_reason":null,"usage":{"input_tokens":12,"output_tokens":1}}}`)
		sseEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"I will rewrite the file"}}`)
		sseEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		// One call the model finished writing. Its arguments arrive in
		// pieces and the pieces interleave with the next call's, so the
		// judgement below rests on each block's own bytes and not on
		// whatever the stream wrote last.
		sseEvent(w, "content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call_1","name":"read_file","input":{}}}`)
		sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}`)
		// And one the ceiling landed inside of: the block opened, the
		// arguments never closed.
		sseEvent(w, "content_block_start", `{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"call_2","name":"write_file","input":{}}}`)
		sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"a.go\"}"}}`)
		sseEvent(w, "content_block_stop", `{"type":"content_block_stop","index":1}`)
		sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"a.go\","}}`)
		sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"\"content\":\"package"}}`)
		sseEvent(w, "content_block_stop", `{"type":"content_block_stop","index":2}`)
		sseEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"max_tokens","stop_sequence":null},"usage":{"output_tokens":4096}}`)
		sseEvent(w, "message_stop", `{"type":"message_stop"}`)
	})
	defer srv.Close()

	p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test", BaseURL: srv.URL})
	events, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "rewrite it"}}, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	var final llm.StreamEvent
	for ev := range events {
		text += ev.Token
		if ev.Err != nil {
			t.Fatalf("stream error: %v", ev.Err)
		}
		if ev.Done {
			final = ev
		}
	}
	if text != "I will rewrite the file" {
		t.Errorf("the words the model wrote are kept, got %q", text)
	}
	if final.Stop != llm.StopLength {
		t.Errorf("stop = %q, want %q", final.Stop, llm.StopLength)
	}
	if len(final.ToolCalls) != 1 || final.ToolCalls[0].ID != "call_1" {
		t.Errorf("the finished call survives and half a JSON object does not, got %+v", final.ToolCalls)
	}
}

// thinkingBlocks counts the thinking and redacted-thinking blocks a converted
// turn carries, which is what a replay policy is a statement about.
func thinkingBlocks(msg anthropic.MessageParam) int {
	var n int
	for _, b := range msg.Content {
		if b.OfThinking != nil || b.OfRedactedThinking != nil {
			n++
		}
	}
	return n
}

// TestAnthropicReasoning_ReplaysOnlyTheCurrentChain: this dialect keeps prior
// turns' thinking in context and bills it as input, so a session that
// replayed all of it would pay again every round for every round it had ever
// thought, in the one category a trim cannot reach.
func TestAnthropicReasoning_ReplaysOnlyTheCurrentChain(t *testing.T) {
	think := func(sig string) []llm.ReasoningBlock {
		return []llm.ReasoningBlock{{Text: "weighing " + sig, Signature: sig}}
	}
	_, msgs := toAnthropicMessages([]llm.Message{
		{Role: llm.RoleUser, Content: "read a"},
		{Role: llm.RoleAssistant, Reasoning: think("old"), ToolCalls: []llm.ToolCall{{ID: "t1", Name: "read"}}},
		{Role: llm.RoleTool, ToolCallID: "t1", Content: "contents"},
		{Role: llm.RoleAssistant, Content: "done", Reasoning: think("older-answer")},
		{Role: llm.RoleUser, Content: "now read b"},
		{Role: llm.RoleAssistant, Reasoning: think("now"), ToolCalls: []llm.ToolCall{{ID: "t2", Name: "read"}}},
		{Role: llm.RoleTool, ToolCallID: "t2", Content: "contents"},
	})
	// user, assistant, tool-result-as-user, assistant, user, assistant, tool
	if n := thinkingBlocks(msgs[1]); n != 0 {
		t.Errorf("the first turn's thinking is history and must not go back, got %d blocks", n)
	}
	if n := thinkingBlocks(msgs[3]); n != 0 {
		t.Errorf("the answer that closed the first turn must not go back, got %d blocks", n)
	}
	last := msgs[len(msgs)-2]
	if n := thinkingBlocks(last); n != 1 {
		t.Fatalf("the current chain's thinking must go back, got %d blocks", n)
	}
	if last.Content[0].OfThinking.Signature != "now" {
		t.Errorf("the wrong turn's thinking survived: %+v", last.Content[0].OfThinking)
	}
}

// A round boundary appends a user message of its own — a tree notice, an
// intervention — after the assistant turn that asked for a tool. Cutting the
// replay at that message would send the tool_use with none of the thinking
// behind it, which is the follow-up this API refuses.
func TestAnthropicReasoning_KeepsTheTurnAUserMessageInterrupted(t *testing.T) {
	_, msgs := toAnthropicMessages([]llm.Message{
		{Role: llm.RoleUser, Content: "read a"},
		{
			Role:      llm.RoleAssistant,
			Reasoning: []llm.ReasoningBlock{{Text: "weighing it", Signature: "sig"}},
			ToolCalls: []llm.ToolCall{{ID: "t1", Name: "read"}},
		},
		{Role: llm.RoleTool, ToolCallID: "t1", Content: "contents"},
		{Role: llm.RoleUser, Content: "another session moved the tree under you"},
	})
	if n := thinkingBlocks(msgs[1]); n != 1 {
		t.Fatalf("the tool call's thinking must survive the notice, got %d blocks", n)
	}
}

func newTestAnthropic(opts llm.ResolveOpts) *Provider {
	clientOpts := []option.RequestOption{
		option.WithAPIKey(cmp.Or(opts.APIKey, "sk-test")),
		option.WithHTTPClient(llmtest.HTTP.Client()),
	}
	if opts.BaseURL != "" {
		clientOpts = append(clientOpts, option.WithBaseURL(opts.BaseURL))
	}
	p := NewNamed(
		anthropic.NewClient(clientOpts...), opts.Model, "anthropic", opts.CacheTTL,
	)
	p.Deadline = idle.Of(opts.StreamIdleSeconds)
	return p
}

// The Messages API says which lifetime it wrote the cache for. A request
// asking for an hour that the provider honoured as five minutes is read here
// and nowhere else.
func TestAnthropic_UsageSplitsTheLeaseConfirmed(t *testing.T) {
	for _, tc := range []struct {
		name, usage string
		want        llm.Usage
	}{
		{"an hour honoured",
			`"usage":{"input_tokens":12,"output_tokens":1,"cache_read_input_tokens":900,"cache_creation_input_tokens":88,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":88}}`,
			llm.Usage{PromptTokens: 1000, CompletionTokens: 5, CachedTokens: 900, CacheCreationTokens: 88, CacheCreation1h: 88,
				Reports: cache.FieldRead | cache.FieldCreation | cache.FieldLease}},
		{"an hour written as five minutes",
			`"usage":{"input_tokens":12,"output_tokens":1,"cache_read_input_tokens":0,"cache_creation_input_tokens":988,"cache_creation":{"ephemeral_5m_input_tokens":988,"ephemeral_1h_input_tokens":0}}`,
			llm.Usage{PromptTokens: 1000, CompletionTokens: 5, CacheCreationTokens: 988, CacheCreation5m: 988,
				Reports: cache.FieldRead | cache.FieldCreation | cache.FieldLease}},
		{"a gateway that strips the fields",
			`"usage":{"input_tokens":1000,"output_tokens":1}`,
			llm.Usage{PromptTokens: 1000, CompletionTokens: 5}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := anthropicSSEServer(t, func(w http.ResponseWriter) {
				sseEvent(w, "message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-opus-5","stop_reason":null,`+tc.usage+`}}`)
				sseEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
				sseEvent(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`)
				sseEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
				sseEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`)
				sseEvent(w, "message_stop", `{"type":"message_stop"}`)
			})
			defer srv.Close()
			p := newTestAnthropic(llm.ResolveOpts{APIKey: "sk-test", BaseURL: srv.URL})
			events, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, llm.CompletionOpts{})
			if err != nil {
				t.Fatal(err)
			}
			_, _, usage, streamErr := llmtest.Drain(t, events)
			if streamErr != nil || usage == nil {
				t.Fatalf("usage %v, err %v", usage, streamErr)
			}
			if *usage != tc.want {
				t.Errorf("usage = %+v, want %+v", *usage, tc.want)
			}
		})
	}
}

func TestAnthropicIsNotAModelLister(t *testing.T) {
	// The Messages API has no models endpoint; the curated catalog covers it.
	var p llm.Provider = &Provider{}
	if _, ok := p.(catalog.Lister); ok {
		t.Fatal("anthropic should not claim live model discovery")
	}
}
