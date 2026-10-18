package openai

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/internal/cachemarks"
)

// NewPromptCacheTransport gives GPT-5.6 chat-completions requests the
// same explicit cache steering that the Responses path uses. The SDK has no
// fields for these newer wire controls, so the encoded request is the one
// place the legacy dialect can still send them.
func NewPromptCacheTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &openAIPromptCacheTransport{base: base}
}

type openAIPromptCacheTransport struct {
	base http.RoundTripper
}

func (t *openAIPromptCacheTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	if err := t.mark(req); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

func (t *openAIPromptCacheTransport) mark(req *http.Request) error {
	if req.Body == nil {
		return nil
	}
	body, err := io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil {
		return err
	}
	body = markOpenAIChatPromptCacheBody(body)
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	return nil
}

// markOpenAIChatPromptCacheBody adds only the GPT-5.6 chat controls. An
// unreadable or unfamiliar body is returned untouched, so endpoints that
// merely share the SDK's client do not acquire an unsupported field.
func markOpenAIChatPromptCacheBody(body []byte) []byte {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return body
	}
	var model string
	if err := json.Unmarshal(envelope["model"], &model); err != nil || !responsesExplicitCache(model) {
		return body
	}
	if _, exists := envelope["prompt_cache_key"]; exists {
		return body
	}

	key := chatPromptCacheKey(model, envelope["messages"], envelope["tools"])
	encodedKey, _ := json.Marshal(key)
	envelope["prompt_cache_key"] = encodedKey
	envelope["prompt_cache_options"] = json.RawMessage(`{"ttl":"30m"}`)
	marked, err := json.Marshal(envelope)
	if err != nil {
		return body
	}
	return marked
}

// chatPromptCacheKey hashes the system messages and tool definitions that
// remain fixed while a conversation grows. It is a routing hint rather than a
// cache lookup key, so the actual prompt still decides whether it is reused.
func chatPromptCacheKey(model string, messages, tools json.RawMessage) string {
	h := sha256.New()
	_, _ = h.Write([]byte(model))
	_, _ = h.Write([]byte{0})
	var rawMessages []json.RawMessage
	if json.Unmarshal(messages, &rawMessages) == nil {
		for _, message := range rawMessages {
			if cachemarks.MessageRole(message) != string(llm.RoleSystem) {
				continue
			}
			_, _ = h.Write(message)
			_, _ = h.Write([]byte{0})
		}
	}
	_, _ = h.Write(tools)
	return llm.CurrentApp().Name + "-" + fmt.Sprintf("%x", h.Sum(nil))
}
