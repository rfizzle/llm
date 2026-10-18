package llm_test

// The taxonomy's own tests. Each dialect hands back a different error
// shape for the same failure — a typed *openai.APIError, an Anthropic error
// envelope, a Gemini string with the status written into the prose — and the
// point of the classifier is that all three arrive as the same class. So the
// table below is organised by shape rather than by class: it is the mapping
// the story asked to be covered.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/rfizzle/llm"
	openai "github.com/sashabaranov/go-openai"
	"google.golang.org/genai"
)

// anthropicError builds the Messages API's error the way the SDK does, so the
// envelope the classifier reads is a real one rather than a struct literal
// with the fields it happens to want.
func anthropicError(t *testing.T, status int, body string) *anthropic.Error {
	t.Helper()
	err := &anthropic.Error{}
	if uerr := json.Unmarshal([]byte(body), err); uerr != nil {
		t.Fatalf("building the fixture error: %v", uerr)
	}
	err.StatusCode = status
	return err
}

func TestClassify_ProviderShapes(t *testing.T) {
	classify := llm.NewClassifier("openai", "OPENAI_API_KEY", "sk-live-abcd4f9c")

	cases := []struct {
		name string
		err  error
		want llm.Class
	}{
		// openai chat + compat + openrouter + the gateway profiles over them
		{"openai 401", &openai.APIError{HTTPStatusCode: 401, Message: "Incorrect API key provided"}, llm.ClassAuth},
		{"openai 403", &openai.APIError{HTTPStatusCode: 403, Message: "Country not supported"}, llm.ClassAuth},
		{"openai 429 rate", &openai.APIError{HTTPStatusCode: 429, Message: "Rate limit reached for gpt-4o"}, llm.ClassRateLimit},
		{"openai 429 quota", &openai.APIError{HTTPStatusCode: 429, Message: "You exceeded your current quota, please check your plan and billing details"}, llm.ClassQuota},
		{"openai 400 context", &openai.APIError{HTTPStatusCode: 400, Message: "This model's maximum context length is 128000 tokens"}, llm.ClassContextLength},
		{"openai 400 otherwise", &openai.APIError{HTTPStatusCode: 400, Message: "Unknown parameter: 'reasoning'"}, llm.ClassUnclassified},
		{"openai 404 model", &openai.APIError{HTTPStatusCode: 404, Message: "The model `gpt-9` does not exist or you do not have access to it."}, llm.ClassModelNotFound},
		{"openrouter 404 model", &openai.APIError{HTTPStatusCode: 404, Message: "No endpoints found for anthropic/claude-sonnet-4-6"}, llm.ClassModelNotFound},
		{"ollama 404 model", &openai.RequestError{HTTPStatusCode: 404, Body: []byte(`model "llama3" not found, try pulling it first`)}, llm.ClassModelNotFound},
		// A base URL that is not a completions endpoint answers 404 too, and
		// it says nothing about a model. Naming this one would send the
		// reader after a model id while the address is what is wrong.
		{"404 with no model in it", &openai.APIError{HTTPStatusCode: 404, Message: "404 page not found"}, llm.ClassUnclassified},
		{"openai 500", &openai.APIError{HTTPStatusCode: 500, Message: "The server had an error"}, llm.ClassOverloaded},
		{"openai 503", &openai.APIError{HTTPStatusCode: 503, Message: "Service Unavailable"}, llm.ClassOverloaded},
		{"openai transport 401", &openai.RequestError{HTTPStatusCode: 401, Body: []byte("invalid api key")}, llm.ClassAuth},
		{"openai transport 429", &openai.RequestError{HTTPStatusCode: 429, Body: []byte("slow down")}, llm.ClassRateLimit},

		// anthropic messages, and any gateway profile speaking it
		{"anthropic 401", anthropicError(t, 401, `{"error":{"type":"authentication_error","message":"invalid x-api-key"}}`), llm.ClassAuth},
		{"anthropic 429", anthropicError(t, 429, `{"error":{"type":"rate_limit_error","message":"Number of request tokens has exceeded your per-minute rate limit"}}`), llm.ClassRateLimit},
		{"anthropic 529", anthropicError(t, 529, `{"error":{"type":"overloaded_error","message":"Overloaded"}}`), llm.ClassOverloaded},
		{"anthropic credit", anthropicError(t, 400, `{"error":{"type":"invalid_request_error","message":"Your credit balance is too low to access the API"}}`), llm.ClassQuota},
		{"anthropic context", anthropicError(t, 400, `{"error":{"type":"invalid_request_error","message":"prompt is too long: 210000 tokens > 200000"}}`), llm.ClassContextLength},
		{"anthropic 404 model", anthropicError(t, 404, `{"error":{"type":"not_found_error","message":"model: claude-x"}}`), llm.ClassModelNotFound},

		// gemini, which reports its status in prose
		{"gemini typed 429", genai.APIError{Code: 429, Message: "Resource has been exhausted"}, llm.ClassRateLimit},
		{"gemini 401 in prose", errors.New("googleapi: Error 401: API key not valid"), llm.ClassAuth},
		{"gemini 403 in prose", errors.New("googleapi: Error 403: forbidden"), llm.ClassAuth},
		{"gemini 429 in prose", errors.New("googleapi: Error 429: rate limit"), llm.ClassRateLimit},
		{"gemini 503 in prose", errors.New("googleapi: Error 503: The model is overloaded"), llm.ClassOverloaded},
		{"gemini 404 in prose", errors.New("googleapi: Error 404: models/gemini-x is not found for API version v1beta, or is not supported for generateContent"), llm.ClassModelNotFound},

		// the transport, which belongs to no dialect
		{"cancelled", context.Canceled, llm.ClassCancelled},
		{"wrapped cancellation", &url.Error{Op: "Post", URL: "https://api.openai.com", Err: context.Canceled}, llm.ClassCancelled},
		{"refused", &url.Error{Op: "Post", URL: "https://api.openai.com", Err: syscall.ECONNREFUSED}, llm.ClassNetwork},
		{"dns", &url.Error{Op: "Post", URL: "https://api.example", Err: &net.DNSError{Err: "no such host"}}, llm.ClassNetwork},
		{"deadline", context.DeadlineExceeded, llm.ClassNetwork},
		{"dropped mid-stream", errors.New("unexpected EOF"), llm.ClassNetwork},
		{"malformed body", errors.New("invalid character 'h' looking for beginning of value"), llm.ClassMalformed},

		// and the one that has no name
		{"unrecognised", errors.New("something went sideways"), llm.ClassUnclassified},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, ok := llm.AsFailure(classify(tc.err))
			if !ok {
				t.Fatalf("classify returned an unclassified error: %v", tc.err)
			}
			if f.Class != tc.want {
				t.Errorf("class = %q, want %q (message %q)", f.Class, tc.want, f.Message)
			}
		})
	}
}

// The table above is written in the phrases the classifier looks for, which
// is why it never caught this: `contextPhrases` carried a bare "max_tokens"
// and every one of the strings below that names that field read as a window
// that had filled up. Two of them are not about the window at all — an
// output ceiling set above the model's, and a field name the model does not
// take — and the second is a request the client makes itself, since the
// `openai-compatible` path sends the deprecated field on purpose for any
// model the table does not call reasoning. Both offered "compact now",
// which spends a summary request and comes back to the identical 400.
//
// So this table is the literal strings the four dialects emit, copied
// rather than paraphrased, and it asserts the three cases apart.
func TestClassify_TheLiteralStringsTheDialectsEmit(t *testing.T) {
	classify := llm.NewClassifier("openai", "OPENAI_API_KEY", "")

	cases := []struct {
		name string
		err  error
		want llm.Class
	}{
		// The window is genuinely full. Every dialect says so in its own
		// words and none of them says it with a field name.
		{
			"openai context",
			&openai.APIError{HTTPStatusCode: 400, Code: "context_length_exceeded", Message: "This model's maximum context length is 128000 tokens. However, your messages resulted in 130524 tokens. Please reduce the length of the messages."},
			llm.ClassContextLength,
		},
		{
			"openrouter context",
			&openai.APIError{HTTPStatusCode: 400, Message: "This endpoint's maximum context length is 163840 tokens. However, you requested about 200019 tokens (196019 of text input, 4000 in the output). Please reduce the length of either one."},
			llm.ClassContextLength,
		},
		{
			"anthropic context",
			anthropicError(t, 400, `{"error":{"type":"invalid_request_error","message":"prompt is too long: 210410 tokens > 200000 maximum"}}`),
			llm.ClassContextLength,
		},
		{
			"gemini context",
			genai.APIError{Code: 400, Message: "The input token count (1305678) exceeds the maximum number of tokens allowed (1048575)."},
			llm.ClassContextLength,
		},

		// The output ceiling is above what the model will write. The
		// conversation is whatever size it was; compacting it changes
		// nothing, and the fix is a smaller ceiling.
		{
			"openai output ceiling",
			&openai.APIError{HTTPStatusCode: 400, Message: "max_tokens is too large: 200000. This model supports at most 100000 completion tokens, whereas you provided 200000."},
			llm.ClassUnclassified,
		},
		{
			"anthropic output ceiling",
			anthropicError(t, 400, `{"error":{"type":"invalid_request_error","message":"max_tokens: 200000 > 64000, which is the maximum allowed number of output tokens for claude-sonnet-4-5-20250929"}}`),
			llm.ClassUnclassified,
		},

		// The request named a field this model does not take. The first is
		// the one the client sends itself.
		{
			"openai deprecated field",
			&openai.APIError{HTTPStatusCode: 400, Message: "Unsupported parameter: 'max_tokens' is not supported with this model. Use 'max_completion_tokens' instead."},
			llm.ClassUnclassified,
		},
		{
			"openai unsupported value",
			&openai.APIError{HTTPStatusCode: 400, Message: "Unsupported value: 'temperature' does not support 0.5 with this model. Only the default (1) value is supported."},
			llm.ClassUnclassified,
		},
		{
			"ollama unknown field",
			&openai.RequestError{HTTPStatusCode: 400, Body: []byte(`{"error":{"message":"unknown field max_completion_tokens","type":"invalid_request_error"}}`)},
			llm.ClassUnclassified,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, ok := llm.AsFailure(classify(tc.err))
			if !ok {
				t.Fatalf("classify returned an unclassified error: %v", tc.err)
			}
			if f.Class != tc.want {
				t.Errorf("class = %q, want %q (message %q)", f.Class, tc.want, f.Message)
			}
		})
	}
}

func TestClassify_SentinelsMatchTheClass(t *testing.T) {
	classify := llm.NewClassifier("openai", "OPENAI_API_KEY", "")
	for class, sentinel := range llm.Sentinels {
		f := &llm.Failure{Class: class}
		if !errors.Is(f, sentinel) {
			t.Errorf("errors.Is on %q did not match its sentinel", class)
		}
	}
	err := classify(&openai.APIError{HTTPStatusCode: 401, Message: "nope"})
	if !errors.Is(err, llm.ErrAuth) {
		t.Errorf("a 401 should match ErrAuth, got %v", err)
	}
	if errors.Is(err, llm.ErrRateLimited) {
		t.Errorf("a 401 should not match ErrRateLimited")
	}
}

func TestClassify_KeepsTheProvidersOwnWords(t *testing.T) {
	classify := llm.NewClassifier("openai", "OPENAI_API_KEY", "")
	f, _ := llm.AsFailure(classify(&openai.APIError{HTTPStatusCode: 400, Message: "Unknown parameter: 'reasoning'"}))
	if f.Class != llm.ClassUnclassified {
		t.Fatalf("class = %q, want unclassified", f.Class)
	}
	detail := f.Detail()
	if len(detail) != 1 || detail[0] != "Unknown parameter: 'reasoning'" {
		t.Errorf("the message should reach the detail body, got %q", detail)
	}
}

func TestClassify_BoundsTheMessage(t *testing.T) {
	long := make([]byte, llm.MaxMessage*3)
	for i := range long {
		long[i] = 'x'
	}
	classify := llm.NewClassifier("openai", "OPENAI_API_KEY", "")
	f, _ := llm.AsFailure(classify(&openai.APIError{HTTPStatusCode: 500, Message: string(long)}))
	if got := len([]rune(f.Message)); got != llm.MaxMessage+1 {
		t.Errorf("message length = %d, want %d plus the ellipsis", got, llm.MaxMessage)
	}
}

func TestClassify_PassesAClassifiedFailureThrough(t *testing.T) {
	first := llm.NewClassifier("openai", "OPENAI_API_KEY", "sk-abcd")
	second := llm.NewClassifier("litellm", "LITELLM_KEY", "sk-zzzz")
	once := first(&openai.APIError{HTTPStatusCode: 401, Message: "nope"})
	twice := second(once)
	f, ok := llm.AsFailure(twice)
	if !ok {
		t.Fatal("expected a classified failure")
	}
	if f.Provider != "openai" {
		t.Errorf("provider = %q, want the dialect that first named the failure", f.Provider)
	}
}

func TestClassify_CarriesTheKeyItSent(t *testing.T) {
	classify := llm.NewClassifier("openai", "ACME_API_KEY or OPENAI_API_KEY", "sk-live-abcd4f9c")
	f, _ := llm.AsFailure(classify(&openai.APIError{HTTPStatusCode: 401, Message: "nope"}))
	if f.KeyTail != "4f9c" {
		t.Errorf("KeyTail = %q, want the last four characters", f.KeyTail)
	}
	if f.KeyEnv != "ACME_API_KEY or OPENAI_API_KEY" {
		t.Errorf("KeyEnv = %q", f.KeyEnv)
	}
	if f.Provider != "openai" {
		t.Errorf("Provider = %q", f.Provider)
	}
}

// Every refused request lands in the diagnostic log, because this is the
// only place a failure is named — and a cancellation does not, because it is
// the reader pressing Esc rather than anything going wrong.
func TestClassify_WritesTheRefusalToTheLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acme.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	llm.SetLogger(slog.New(slog.NewTextHandler(f, nil)))
	t.Cleanup(func() { llm.SetLogger(nil) })

	classify := llm.NewClassifier("openai", "OPENAI_API_KEY", "sk-live-abcd4f9c")
	// The classified errors themselves are what other tests here assert; the
	// question this one asks is what reached the file.
	_ = classify(&openai.APIError{HTTPStatusCode: 429, Message: "rate limit reached"})
	_ = classify(context.Canceled)

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("nothing was written to the log: %v", err)
	}
	written := string(body)
	for _, want := range []string{"provider request refused", "provider=openai", `class="rate limited"`, "status=429"} {
		if !strings.Contains(written, want) {
			t.Errorf("the refusal does not say %s:\n%s", want, written)
		}
	}
	if strings.Contains(written, "cancelled") {
		t.Errorf("a cancellation reached the log:\n%s", written)
	}
	// What the recovery row needs to make an honest offer — which key was
	// sent, and where it was looked for — is for the screen. Neither is
	// worth a line in a file that outlives the session.
	for _, secret := range []string{"sk-live-abcd4f9c", "4f9c", "OPENAI_API_KEY"} {
		if strings.Contains(written, secret) {
			t.Errorf("the log carries %q:\n%s", secret, written)
		}
	}
}
