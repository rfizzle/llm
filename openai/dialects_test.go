package openai_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/compat"
	"github.com/rfizzle/llm/internal/llmtest"
	"github.com/rfizzle/llm/internal/testhttp"
	llmopenai "github.com/rfizzle/llm/openai"
	"github.com/rfizzle/llm/openrouter"
	openai "github.com/sashabaranov/go-openai"
)

// A gateway's ping is an event on the stream: it carries no text and nothing
// else, and the answer around it arrives whole. One case per dialect that
// sends them, each over the in-process transport; the Messages API's case is
// in its own package. Gemini's SDK hands back whole responses and no dialect
// of its own sends a ping, so it has no case.
func TestStream_AKeepaliveIsAnEvent(t *testing.T) {
	chunk := func(delta, finish string) string {
		return fmt.Sprintf(`{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":%s%s}]}`, delta, finish)
	}
	openAIChunks := []string{
		`{"id":"c1","object":"chat.completion.chunk","choices":[]}`,
		chunk(`{"content":"hi"}`, ""),
		chunk(`{}`, `,"finish_reason":"stop"`),
	}
	responses := []string{
		`data: {"type":"response.created","response":{}}`,
		`: keepalive`,
		`data: {"type":"response.output_text.delta","delta":"hi"}`,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":1,"output_tokens":1}}}`,
	}

	cases := []struct {
		name  string
		want  int
		start func(t *testing.T) (<-chan llm.StreamEvent, error)
	}{
		{"openai", 1, func(t *testing.T) (<-chan llm.StreamEvent, error) {
			p := newChat(llmtest.OpenAISSEServer(t, openAIChunks).URL+"/v1", "gpt-4o")
			return p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, llm.CompletionOpts{})
		}},
		{"openai-compatible", 1, func(t *testing.T) (<-chan llm.StreamEvent, error) {
			p := newCompat(llmtest.OpenAISSEServer(t, openAIChunks).URL+"/v1", "llama3")
			return p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, llm.CompletionOpts{})
		}},
		{"openrouter", 1, func(t *testing.T) (<-chan llm.StreamEvent, error) {
			p := newOpenRouter(llmtest.OpenAISSEServer(t, openAIChunks).URL+"/v1", "m")
			return p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, llm.CompletionOpts{})
		}},
		{"openai-responses", 1, func(t *testing.T) (<-chan llm.StreamEvent, error) {
			p := newResponses(responsesServer(t, responses).URL, "gpt-5")
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

func TestOpenAI_ListModelsSatisfiesModelLister(t *testing.T) {
	srv := llmtest.ModelsServer(t, `{"object":"list","data":[{"id":"gpt-4o"},{"id":"tts-1"}]}`)

	for name, p := range map[string]llm.Provider{
		"openai":     newChat(srv.URL+"/v1", "gpt-4o"),
		"openrouter": newOpenRouter(srv.URL+"/v1", "openai/gpt-4o"),
		"compat":     newCompat(srv.URL+"/v1", "llama3"),
	} {
		lister, ok := p.(llm.ModelLister)
		if !ok {
			t.Fatalf("%s should implement ModelLister", name)
		}
		names, err := lister.ListModels(context.Background())
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", name, err)
		}
		if len(names) != 1 || names[0] != "gpt-4o" {
			t.Fatalf("%s: expected the speech model filtered out, got %v", name, names)
		}
	}
}

func openAIDialects() []struct {
	name  string
	build func(baseURL string) llm.Provider
} {
	return []struct {
		name  string
		build func(baseURL string) llm.Provider
	}{
		{"openai", func(u string) llm.Provider { return newChat(u+"/v1", "gpt-4o") }},
		{"openrouter", func(u string) llm.Provider { return newOpenRouter(u+"/v1", "test-model") }},
		{"openai-compatible", func(u string) llm.Provider { return newCompat(u+"/v1", "llama3") }},
	}
}

// A round that called a tool is the round most of a coding session is made
// of, and its tokens arrive after the reason that ended it. Reading the
// stream only as far as the reason billed none of them: the ledger, the
// context rail and the calibration all went by the text rounds alone.
func TestStreamOpenAI_AToolRoundReportsTheUsageThatFollowsIt(t *testing.T) {
	for _, dialect := range openAIDialects() {
		t.Run(dialect.name, func(t *testing.T) {
			srv := llmtest.OpenAISSEServer(t, llmtest.ToolRoundChunks)
			p := dialect.build(srv.URL)

			ch, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "read main.go"}}, llm.CompletionOpts{})
			if err != nil {
				t.Fatal(err)
			}
			fragments, final := llmtest.DrainFragments(t, ch)

			llmtest.WantFragments(t, fragments, []llm.ToolCallDelta{
				{ID: "call_abc", Arguments: `{"path":`},
				{ID: "call_abc", Arguments: `"main.go"}`},
			})
			if final.Stop != llm.StopTool {
				t.Errorf("stop = %q, want %q", final.Stop, llm.StopTool)
			}
			if len(final.ToolCalls) != 1 || final.ToolCalls[0].Arguments != `{"path":"main.go"}` {
				t.Errorf("the terminal event lost the finished call: %+v", final.ToolCalls)
			}
			if final.Usage == nil {
				t.Fatal("a tool round reported no usage, so nothing downstream can bill it")
			}
			want := llm.Usage{PromptTokens: 999, CompletionTokens: 42, CachedTokens: 128, Reports: llm.FieldCachedTokens}
			if *final.Usage != want {
				t.Errorf("usage = %+v, want %+v", *final.Usage, want)
			}
		})
	}
}

// A gateway that sends no usage at all must still end the round, and end it
// once the body runs out rather than waiting on tokens that are not coming.
func TestStreamOpenAI_AToolRoundStillEndsWithoutAUsageChunk(t *testing.T) {
	srv := llmtest.OpenAISSEServer(t, llmtest.ToolRoundChunks[:len(llmtest.ToolRoundChunks)-1])
	p := newChat(srv.URL+"/v1", "gpt-4o")

	ch, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "read main.go"}}, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}
	_, final := llmtest.DrainFragments(t, ch)

	if !final.Done || final.Err != nil {
		t.Fatalf("the round did not end cleanly: %+v", final)
	}
	if final.Stop != llm.StopTool {
		t.Errorf("stop = %q, want %q", final.Stop, llm.StopTool)
	}
	if len(final.ToolCalls) != 1 || final.ToolCalls[0].Name != "read_file" {
		t.Errorf("the terminal event lost the finished call: %+v", final.ToolCalls)
	}
	if final.Usage != nil {
		t.Errorf("usage came from nowhere: %+v", *final.Usage)
	}
}

// Some gateways fold the usage into the chunk that names the finish reason
// instead of sending one of its own. That round has both in hand at the same
// moment and ends when the body does, still carrying its tokens.
func TestStreamOpenAI_AToolRoundReportsUsageFoldedIntoTheFinishChunk(t *testing.T) {
	chunks := append([]string{}, llmtest.ToolRoundChunks[:len(llmtest.ToolRoundChunks)-1]...)
	chunks[len(chunks)-1] = `{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":999,"completion_tokens":42,"total_tokens":1041}}`
	srv := llmtest.OpenAISSEServer(t, chunks)
	p := newChat(srv.URL+"/v1", "gpt-4o")

	ch, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "read main.go"}}, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}
	_, final := llmtest.DrainFragments(t, ch)

	if final.Stop != llm.StopTool {
		t.Errorf("stop = %q, want %q", final.Stop, llm.StopTool)
	}
	if final.Usage == nil || final.Usage.PromptTokens != 999 {
		t.Fatalf("usage = %+v, want 999 prompt tokens", final.Usage)
	}
}

// A round's calls are addressed by id and not by their position in the
// stream. Two calls numbered 1 and 2 — a gateway that starts at one, or one
// that left a gap where a call was abandoned — used to come back as a single
// call, so the model received a result for one tool it asked for and the turn
// ran on owing an answer for the other.
func TestStreamOpenAI_ParallelCallsAtIndexesThatAreNotADenseRun(t *testing.T) {
	chunks := []string{
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_a","type":"function","function":{"name":"read_file","arguments":""}}]}}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":2,"id":"call_b","type":"function","function":{"name":"list_dir","arguments":""}}]}}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"path\":\"a.go\"}"}}]}}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":2,"function":{"arguments":"{\"path\":\".\"}"}}]}}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
	}
	srv := llmtest.OpenAISSEServer(t, chunks)
	p := newChat(srv.URL+"/v1", "gpt-4o")

	ch, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "read a.go and list ."}}, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}
	fragments, final := llmtest.DrainFragments(t, ch)

	llmtest.WantFragments(t, fragments, []llm.ToolCallDelta{
		{ID: "call_a", Arguments: `{"path":"a.go"}`},
		{ID: "call_b", Arguments: `{"path":"."}`},
	})
	if len(final.ToolCalls) != 2 {
		t.Fatalf("the round asked for two tools and delivered %d: %+v", len(final.ToolCalls), final.ToolCalls)
	}
	want := []llm.ToolCall{
		{ID: "call_a", Name: "read_file", Arguments: `{"path":"a.go"}`},
		{ID: "call_b", Name: "list_dir", Arguments: `{"path":"."}`},
	}
	for i, call := range final.ToolCalls {
		if call != want[i] {
			t.Errorf("call %d = %+v, want %+v", i, call, want[i])
		}
	}
}

// A gateway that names the call on every chunk and the index on none of them
// still assembles one call: the id is the address, and the index is only the
// fallback for the chunks that carry no id.
func TestStreamOpenAI_AContinuationWithoutAnIndexReachesItsCallByID(t *testing.T) {
	chunks := []string{
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_abc","type":"function","function":{"name":"read_file","arguments":""}}]}}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_abc","function":{"arguments":"{\"path\":"}}]}}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_abc","function":{"arguments":"\"main.go\"}"}}]}}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
	}
	srv := llmtest.OpenAISSEServer(t, chunks)
	p := newChat(srv.URL+"/v1", "gpt-4o")

	ch, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "read main.go"}}, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}
	_, final := llmtest.DrainFragments(t, ch)

	if final.Err != nil {
		t.Fatalf("the round failed: %v", final.Err)
	}
	want := llm.ToolCall{ID: "call_abc", Name: "read_file", Arguments: `{"path":"main.go"}`}
	if len(final.ToolCalls) != 1 || final.ToolCalls[0] != want {
		t.Fatalf("calls = %+v, want the one call %+v", final.ToolCalls, want)
	}
}

// A chunk carrying neither address ends the round. Read as call 0 — which is
// what an index-keyed accumulator did with it — its arguments land inside a
// call the model wrote separately, and a tool runs on input nobody asked for.
func TestStreamOpenAI_AChunkWithNoIDAndNoIndexIsAFailure(t *testing.T) {
	chunks := []string{
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"main.go\"}"}}]}}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"function":{"arguments":"{\"path\":\"other.go\"}"}}]}}]}`,
	}
	srv := llmtest.OpenAISSEServer(t, chunks)
	p := newChat(srv.URL+"/v1", "gpt-4o")

	ch, err := p.StreamCompletion(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "read main.go"}}, llm.CompletionOpts{})
	if err != nil {
		t.Fatal(err)
	}
	_, final := llmtest.DrainFragments(t, ch)

	if final.Err == nil {
		t.Fatal("a fragment addressed to nothing was folded into a call instead of failing the round")
	}
	if len(final.ToolCalls) != 1 || final.ToolCalls[0].Arguments != `{"path":"main.go"}` {
		t.Errorf("the failure should carry the call that was whole: %+v", final.ToolCalls)
	}
}

// The three chat-completions dialects and the Responses one, built the way
// each package's own tests build them, from outside: the dialects share the
// chat plumbing, so their tests live with it, and only the exported surface is
// reachable from here.

func newChat(baseURL string, model string) *llmopenai.Chat {
	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = baseURL
	cfg.HTTPClient = &http.Client{Transport: llmopenai.NewPromptCacheTransport(llmtest.HTTP.Client().Transport)}
	return llmopenai.NewChatWith(openai.NewClientWithConfig(cfg), model)
}

func newCompat(baseURL, model string) *compat.Provider {
	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = baseURL
	cfg.HTTPClient = llmtest.HTTP.Client()
	return compat.NewWith(openai.NewClientWithConfig(cfg), model, baseURL)
}

// dialectOpenRouter goes out through the cache-mark transport as the
// gateway's own client does; the attribution headers are the gateway
// package's and are pinned there.
func newOpenRouter(baseURL string, model string) *openrouter.Provider {
	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = baseURL
	cfg.HTTPClient = &http.Client{Transport: llm.NewCacheMarkTransport(&llmtest.HTTP, "")}
	return openrouter.NewWith(openai.NewClientWithConfig(cfg), model)
}

func newResponses(baseURL, model string) *llmopenai.Responses {
	return llmopenai.NewResponsesWith(llmtest.HTTP.Client(), "test-key", baseURL+"/v1", model, "")
}

// dialectResponsesServer stands in for the /v1/responses endpoint, replaying
// a scripted event stream.
func responsesServer(t *testing.T, events []string) *testhttp.Server {
	t.Helper()
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("expected the responses endpoint, got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, ev := range events {
			fmt.Fprintf(w, "%s\n\n", ev)
			flusher.Flush()
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}
