package llm_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/internal/llmtest"
	llmopenai "github.com/rfizzle/llm/openai"
	openai "github.com/sashabaranov/go-openai"
)

// openAIBody is a chat-completions request in the shape the OpenAI-speaking
// providers build, which is what the gateway's breakpoints have to be added
// to without disturbing.
func openAIBody(t *testing.T, model string, msgs ...llm.Message) []byte {
	t.Helper()
	raw, err := json.Marshal(openai.ChatCompletionRequest{
		Model:    model,
		Messages: llmopenai.ChatMessages(msgs),
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestMarkOpenAICacheBodyMarksTheSamePositionsAsTheNativePath(t *testing.T) {
	body := llm.MarkOpenAICacheBody(openAIBody(t, "anthropic/claude-sonnet-4-6",
		llm.Message{Role: llm.RoleSystem, Content: "be helpful"},
		llm.Message{Role: llm.RoleUser, Content: "one"},
		llm.Message{Role: llm.RoleAssistant, Content: "two"},
		llm.Message{Role: llm.RoleUser, Content: "three"},
		llm.Message{Role: llm.RoleAssistant, Content: "four"},
	), llm.DefaultCacheTTL)

	got := llmtest.BodyMarks(t, body)
	want := map[int]string{0: string(llm.CacheTTL1h), 3: string(llm.CacheTTL1h), 4: string(llm.CacheTTL1h)}
	if len(got) != len(want) {
		t.Fatalf("markers = %v, want %v", got, want)
	}
	for i, ttl := range want {
		if got[i] != ttl {
			t.Errorf("message %d's marker = %q, want %q", i, got[i], ttl)
		}
	}
}

// The head sits inside the message list here and beside it on the native
// path, so a conversation with no system message must still put its rolling
// markers on the last two turns rather than one short.
func TestMarkOpenAICacheBodyWithNoSystemMessage(t *testing.T) {
	body := llm.MarkOpenAICacheBody(openAIBody(t, "anthropic/claude-opus-5",
		llm.Message{Role: llm.RoleUser, Content: "one"},
		llm.Message{Role: llm.RoleAssistant, Content: "two"},
		llm.Message{Role: llm.RoleUser, Content: "three"},
	), llm.DefaultCacheTTL)

	got := llmtest.BodyMarks(t, body)
	if len(got) != 2 || got[1] == "" || got[2] == "" {
		t.Fatalf("markers = %v, want the last two messages", got)
	}
}

// A breakpoint on a dialect that has never heard of one is what makes this
// safe to send to a gateway; a request the rule does not claim is safe
// because it is not touched at all.
func TestMarkOpenAICacheBodyLeavesOtherVendorsAlone(t *testing.T) {
	for _, model := range []string{"openai/gpt-5.2", "google/gemini-3.1-pro-preview", "claude-opus-5"} {
		before := openAIBody(t, model,
			llm.Message{Role: llm.RoleSystem, Content: "be helpful"},
			llm.Message{Role: llm.RoleUser, Content: "one"},
			llm.Message{Role: llm.RoleAssistant, Content: "two"},
		)
		if after := llm.MarkOpenAICacheBody(before, llm.DefaultCacheTTL); string(after) != string(before) {
			t.Errorf("%s: the request was rewritten\n before %s\n  after %s", model, before, after)
		}
	}
}

// The round every agentic turn ends on: the last message is a tool call with
// no content to hang a marker on. The request has to stay well formed and
// keep the markers it could place.
func TestMarkOpenAICacheBodyWhenTheLastMessageCannotHoldAMarker(t *testing.T) {
	before := openAIBody(t, "anthropic/claude-sonnet-4-6",
		llm.Message{Role: llm.RoleSystem, Content: "be helpful"},
		llm.Message{Role: llm.RoleUser, Content: "one"},
		llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "t", Name: "search", Arguments: "{}"}}},
	)
	body := llm.MarkOpenAICacheBody(before, llm.DefaultCacheTTL)

	got := llmtest.BodyMarks(t, body)
	if len(got) != 2 || got[0] != string(llm.CacheTTL1h) || got[1] != string(llm.CacheTTL1h) {
		t.Fatalf("markers = %v, want the head and the one message that can carry one", got)
	}

	var req openai.ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("the marked body is not a request any more: %v", err)
	}
	if len(req.Messages) != 3 {
		t.Fatalf("the marked body has %d messages, want 3", len(req.Messages))
	}
	if calls := req.Messages[2].ToolCalls; len(calls) != 1 || calls[0].Function.Name != "search" {
		t.Errorf("the tool call did not survive the marking: %+v", req.Messages[2])
	}
}

// The markers must be the only difference the gateway's request carries, for
// the same reason they are on the native path: they say what a round costs
// and nothing about what it means.
func TestMarkOpenAICacheBodyChangesNothingButTheMarkers(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: "be helpful"},
		{Role: llm.RoleUser, Content: "one"},
		{Role: llm.RoleAssistant, Content: "two"},
	}
	before := openAIBody(t, "anthropic/claude-sonnet-4-6", msgs...)
	after := llm.MarkOpenAICacheBody(before, llm.DefaultCacheTTL)

	var was, is openai.ChatCompletionRequest
	if err := json.Unmarshal(before, &was); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(after, &is); err != nil {
		t.Fatal(err)
	}
	if was.Model != is.Model || len(was.Messages) != len(is.Messages) {
		t.Fatalf("the request changed shape: %+v against %+v", was, is)
	}
	for i, msg := range was.Messages {
		text := is.Messages[i].Content
		if parts := is.Messages[i].MultiContent; len(parts) > 0 {
			text = parts[len(parts)-1].Text
		}
		if msg.Role != is.Messages[i].Role || msg.Content != text {
			t.Errorf("message %d says something else now: %q/%q against %q/%q",
				i, msg.Role, msg.Content, is.Messages[i].Role, text)
		}
	}
}

// The marking is a round tripper because the field it writes exists only on
// the encoded body, and because more than one path sends this dialect to a
// gateway: the built-in provider and every profile route speaking it. What
// each of them shares is the wire, so the wire is where the rule lives.
func TestCacheMarkTransportMarksTheEncodedBody(t *testing.T) {
	for _, tc := range []struct {
		configured string
		want       string
	}{
		{configured: "", want: string(llm.CacheTTL1h)},
		{configured: "5m", want: string(llm.CacheTTL5m)},
	} {
		var got []byte
		srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, _ = io.ReadAll(r.Body)
		}))

		sent := openAIBody(t, "anthropic/claude-sonnet-4-6",
			llm.Message{Role: llm.RoleSystem, Content: "be helpful"},
			llm.Message{Role: llm.RoleUser, Content: "one"},
		)
		req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(sent))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := (&http.Client{Transport: llm.NewCacheMarkTransport(&llmtest.HTTP, tc.configured)}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		srv.Close()

		marks := llmtest.BodyMarks(t, got)
		if len(marks) != 2 || marks[0] != tc.want || marks[1] != tc.want {
			t.Errorf("cache_ttl = %q: markers = %v, want both at %q", tc.configured, marks, tc.want)
		}
	}
}

// The same client asks the gateway for its catalog, and that request has no
// body at all — so a transport that assumed one would break discovery rather
// than mark anything.
func TestCacheMarkTransportLeavesABodylessRequestAlone(t *testing.T) {
	var method string
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
	}))
	defer srv.Close()

	resp, err := (&http.Client{Transport: llm.NewCacheMarkTransport(&llmtest.HTTP, "")}).Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if method != http.MethodGet {
		t.Errorf("the catalog query did not arrive: %q", method)
	}
}

// A request for a model the gateway routes anywhere else leaves byte for byte
// as it arrived, which is what makes the transport safe to wrap anything in.
func TestCacheMarkTransportLeavesAnotherVendorAlone(t *testing.T) {
	var got []byte
	srv := llmtest.HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
	}))
	defer srv.Close()

	sent := openAIBody(t, "openai/gpt-5.2",
		llm.Message{Role: llm.RoleSystem, Content: "be helpful"},
		llm.Message{Role: llm.RoleUser, Content: "one"},
	)
	req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(sent))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&http.Client{Transport: llm.NewCacheMarkTransport(&llmtest.HTTP, "")}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if string(got) != string(sent) {
		t.Errorf("the request was rewritten\n before %s\n  after %s", sent, got)
	}
}

// prompt_cache = false changes nothing a gateway route sends either: the
// marks still go out on a chat-completions body bound for the Messages API.
func TestCacheMarkBodyIgnoresAPromptCacheDeclaration(t *testing.T) {
	gateway := []byte(`{"model":"anthropic/claude-sonnet-5-5","messages":[{"role":"system","content":"sys"},{"role":"user","content":"hi"}]}`)
	llmtest.Declare(t, "gw", map[string]llm.Declared{"anthropic/claude-sonnet-5-5": {Model: map[string]any{llm.KeyPromptCache: false}}})
	if marked := llm.MarkOpenAICacheBody(gateway, llm.CacheTTL1h); !bytes.Contains(marked, []byte(`"cache_control"`)) {
		t.Errorf("a gateway route keeps its marks: %s", marked)
	}
}
