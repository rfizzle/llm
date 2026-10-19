package llmtest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/attachment"
	"github.com/rfizzle/llm/capability"
	"github.com/rfizzle/llm/failure"
	llmflow "github.com/rfizzle/llm/flow"
	"github.com/rfizzle/llm/internal/testhttp"
)

// App is the identity the backends' tests were written against: they read
// ACME_API_KEY and key their caches under acme-.
var App = llm.App{
	Name:       "acme",
	EnvPrefix:  "ACME",
	SignInHint: "acme login chatgpt",
	URL:        "https://example.com/acme",
}

// Main sets App the way the client does and runs the package's tests.
func Main(m *testing.M) {
	llm.SetApp(App)
	os.Exit(m.Run())
}

// WithApp sets a for one test and puts App back after.
func WithApp(t *testing.T, a llm.App) {
	t.Helper()
	llm.SetApp(a)
	t.Cleanup(func() { llm.SetApp(App) })
}

// HTTP keeps provider fixtures in process. A TCP listener proves nothing
// about a dialect's request or response shape and is unavailable to the
// contained quality suite.
var HTTP testhttp.Registry

// Idle is the deadline the quiet streams run under, and Beat the gap the live
// stream writes at. They are short because the thing under test is the timer
// and not the number, and every quiet test spends the whole deadline — but
// the ratio between them is not arbitrary: the live test demands that six
// gaps in a row are each read as "still writing", so a scheduler hiccup has
// to eat five sixths of the deadline before it can turn a live stream into a
// spurious failure.
const (
	Idle = 300 * time.Millisecond
	Beat = 50 * time.Millisecond
)

// Drain reads a stream to its end: the text, the last calls and usage any
// event carried, and the first error.
func Drain(t *testing.T, ch <-chan llm.StreamEvent) (string, []llm.ToolCall, *llm.Usage, error) {
	t.Helper()
	var text string
	var calls []llm.ToolCall
	var usage *llm.Usage
	for ev := range ch {
		if ev.Err != nil {
			return text, calls, usage, ev.Err
		}
		text += ev.Token
		if len(ev.ToolCalls) > 0 {
			calls = ev.ToolCalls
		}
		if ev.Usage != nil {
			usage = ev.Usage
		}
	}
	return text, calls, usage, nil
}

// DrainFragments reads a stream to its end, keeping the argument fragments in
// the order they arrived and the event that closed it. Every parser is read
// through here, because the promise being checked is the same in all four:
// the fragments come before the terminal event and the terminal event still
// carries the finished calls.
func DrainFragments(t *testing.T, ch <-chan llm.StreamEvent) (fragments []llm.ToolCallDelta, final llm.StreamEvent) {
	t.Helper()
	for ev := range ch {
		switch {
		case ev.ToolCallDelta != nil:
			if ev.Done || ev.Err != nil || len(ev.ToolCalls) > 0 {
				t.Errorf("a fragment event carried more than a fragment: %+v", ev)
			}
			if final.Done || final.Err != nil {
				t.Errorf("a fragment arrived after the terminal event: %+v", *ev.ToolCallDelta)
			}
			fragments = append(fragments, *ev.ToolCallDelta)
		case ev.Done || ev.Err != nil:
			final = ev
		}
	}
	return fragments, final
}

// WantFragments demands the fragments arrived as want, in order.
func WantFragments(t *testing.T, got []llm.ToolCallDelta, want []llm.ToolCallDelta) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d fragments, got %d: %+v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("fragment %d: expected %+v, got %+v", i, want[i], got[i])
		}
	}
}

// CaptureChatRequest runs one completion against a server that decodes the
// request body and answers with an empty stream, and hands back what went
// out on the wire. Decoding into a map rather than the SDK's request type is
// the point: a field the struct would fill in with its zero value has to be
// distinguishable from one that was never sent.
func CaptureChatRequest(t *testing.T, run func(baseURL string) (<-chan llm.StreamEvent, error)) map[string]any {
	t.Helper()
	var body map[string]any
	srv := HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	ch, err := run(srv.URL + "/v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range ch {
	}
	return body
}

// OpenAISSEServer writes the given chunks as the chat-completions event
// stream, one `data:` line each, and closes with the sentinel the client
// reads as the end.
func OpenAISSEServer(t *testing.T, chunks []string) *testhttp.Server {
	t.Helper()
	srv := HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ToolRoundChunks is a round that called a tool, in the order the API
// documents for stream_options.include_usage: the call in fragments, then a
// chunk carrying only the finish reason, then a chunk carrying only the usage
// for the whole round, with no choices in it at all.
var ToolRoundChunks = []string{
	`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`,
	`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"read_file","arguments":""}}]}}]}`,
	`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]}}]}`,
	`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"main.go\"}"}}]}}]}`,
	`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
	`{"id":"chatcmpl-1","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":999,"completion_tokens":42,"total_tokens":1041,"prompt_tokens_details":{"cached_tokens":128}}}`,
}

// ModelsServer answers the models endpoint with body.
func ModelsServer(t *testing.T, body string) *testhttp.Server {
	t.Helper()
	srv := HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/models") {
			t.Errorf("expected the models endpoint, got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// QuietServer answers with the headers of a stream and then writes nothing at
// all — the endpoint that accepts a request and holds it, which is the whole
// reason there is a deadline. The handler returns when the client goes away,
// so the server closes rather than hanging the test binary.
func QuietServer(t *testing.T) *testhttp.Server {
	t.Helper()
	srv := HTTP.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	return srv
}

// WentQuiet reads a stream to its end and demands the ending a stalled
// endpoint owes: a failure, classified as the network class the retry
// schedule waits out, and naming the deadline rather than the cancellation it
// was enforced with.
func WentQuiet(t *testing.T, ch <-chan llm.StreamEvent, err error) {
	t.Helper()
	if err != nil {
		// Two of the dialects open the stream synchronously, so the deadline
		// can land before there is a channel to read.
		AssertIdleFailure(t, err)
		return
	}
	var last llm.StreamEvent
	for ev := range ch {
		last = ev
	}
	if last.Err == nil {
		t.Fatalf("a stream that never wrote should fail, got %+v", last)
	}
	AssertIdleFailure(t, last.Err)
}

// AssertIdleFailure demands err is the deadline's own failure, classified as
// a network failure the session can come back from.
func AssertIdleFailure(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, llm.ErrStreamIdle) {
		t.Errorf("the failure should name the deadline, got %v", err)
	}
	f, ok := failure.As(err)
	if !ok {
		t.Fatalf("the failure should be classified, got %T", err)
	}
	if f.Class != failure.ClassNetwork {
		t.Errorf("class = %q, want %q", f.Class, failure.ClassNetwork)
	}
	if !f.Recoverable() {
		t.Error("a stream that went quiet is a stall the session can come back from")
	}
}

// PictureOf is a real PNG of the given size, so Figure has a header to read.
func PictureOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Labelled is one attachment a sentence can point at, and the line the
// request must lead its part with.
type Labelled struct {
	Name  string
	Att   attachment.Attachment
	Label string
}

// LabelledCases are one attachment of each kind a label counts differently.
func LabelledCases(t *testing.T) []Labelled {
	return []Labelled{
		{
			Name: "a picture is counted in pixels",
			Att: attachment.Attachment{Kind: attachment.Image, Name: "clipboard.png", Handle: "Image#1",
				MediaType: "image/png", Data: PictureOf(t, 14, 9)},
			Label: "Image#1 (clipboard.png, 14×9):",
		},
		{
			Name: "a paste is counted in lines",
			Att: attachment.Attachment{Kind: attachment.Text, Name: "paste-1.txt", Handle: "Paste#1",
				MediaType: "text/plain", Data: []byte(strings.Repeat("round 26 reached\n", 214))},
			Label: "Paste#1 (paste-1.txt, 214 lines):",
		},
		{
			Name: "a document is counted in bytes",
			Att: attachment.Attachment{Kind: attachment.Document, Name: "spec.pdf", Handle: "File#1",
				MediaType: "application/pdf", Data: bytes.Repeat([]byte("%PDF"), 1024)},
			Label: "File#1 (spec.pdf, 4 KB):",
		},
	}
}

// The fixtures every dialect's attachment tests are fed.
var (
	PNG = attachment.Attachment{
		Kind:      attachment.Image,
		Name:      "shot.png",
		MediaType: "image/png",
		Data:      []byte{0x89, 'P', 'N', 'G'},
	}
	Notes = attachment.Attachment{
		Kind:      attachment.Text,
		Name:      "notes.md",
		MediaType: "text/markdown",
		Data:      []byte("# heading\n"),
	}
)

// The recordings the sniffer can produce, one per format the dialects
// disagree about: an MP3 both OpenAI dialects name, a WAV they name under a
// media type the sniffing rules do not use, and an Ogg only Gemini takes.
var (
	MP3 = recording("memo.mp3", "audio/mpeg", "ID3\x04")
	WAV = recording("clip.wav", "audio/wav", "RIFF\x00\x00\x00\x00WAVE")
	OGG = recording("call.ogg", "audio/ogg", "OggS\x00")
)

func recording(name, mediaType, data string) attachment.Attachment {
	return attachment.Attachment{Kind: attachment.Audio, Name: name, MediaType: mediaType, Data: []byte(data)}
}

// BodyMarks is the lifetime of every marker in a chat-completions body, keyed
// by the index of the message carrying it.
func BodyMarks(t *testing.T, body []byte) map[int]string {
	t.Helper()
	var decoded struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("the marked body no longer decodes: %v\n%s", err, body)
	}
	out := map[int]string{}
	for i, msg := range decoded.Messages {
		// An unmarked message keeps its plain string content, which is the
		// point: the parts array appears only where a marker went.
		var parts []struct {
			CacheControl *struct {
				Type string `json:"type"`
				TTL  string `json:"ttl"`
			} `json:"cache_control"`
		}
		if json.Unmarshal(msg.Content, &parts) != nil {
			continue
		}
		for _, part := range parts {
			if part.CacheControl == nil {
				continue
			}
			if part.CacheControl.Type != "ephemeral" {
				t.Errorf("message %d carries a marker of type %q", i, part.CacheControl.Type)
			}
			out[i] = part.CacheControl.TTL
		}
	}
	return out
}

// Declare registers models under name for the test and withdraws them after.
func Declare(t *testing.T, name string, models map[string]capability.Declared) {
	t.Helper()
	capability.RegisterDeclarations(name, models)
	t.Cleanup(func() { capability.RegisterDeclarations(name, nil) })
}

// ScopedNoSchema declares that a model takes no schema on flow alone.
func ScopedNoSchema(flow llmflow.Flow) capability.Declared {
	return capability.Declared{Flows: map[llmflow.Flow]map[string]any{flow: {"structured_outputs": false}}}
}

// NestedArraySchema is a tool schema with an array of objects inside it, the
// shape a converter that flattens or re-derives a schema loses.
const NestedArraySchema = `{"type":"object","properties":{` +
	`"path":{"type":"string"},` +
	`"edits":{"type":"array","items":{"type":"object","properties":{` +
	`"old_text":{"type":"string"},"new_text":{"type":"string"},` +
	`"replace_all":{"type":"boolean"}},"required":["old_text","new_text"]}}},` +
	`"required":["path"]}`
