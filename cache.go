package llm

// Telling a dialect which part of the request it has already read.
//
// A coding turn re-sends its whole opening every round. The system prompt
// carries the project's context file, the memory block, the skills catalog
// and the toolbox; the tool schemas sit in front of it; and the conversation
// beneath only ever grows at the end. Nothing before the newest round changes,
// and by the fiftieth round that unchanged head is most of the request — read
// and billed fifty times, at full price, for text the provider has already
// seen forty-nine times.
//
// Most dialects handle this themselves by matching the prefix they were sent
// last time. The Messages API does not guess: it caches the prefix ending at
// a marker the request carries, and a request with no marker caches nothing
// at all. So the marker is what this file places, and it is placed on every
// request rather than behind a setting — it changes what a round costs and
// nothing about what it says, so there is no session that wants it off. How
// long what it marks stays cached is a setting; whether it is marked at all
// is not.
//
// Two kinds of path reach that API and both have to be told. One speaks it
// directly; the others send the OpenAI shape to a gateway that forwards it,
// honouring whatever breakpoints the request happened to carry — the built-in
// gateway provider and any profile route speaking that dialect alike, which
// is why the second is a round tripper anything can be wrapped in rather than
// one provider's own. The positions have to be the same on both, so they are
// chosen once, in internal/cachemarks, and applied twice: here for the
// gateway path and in the anthropic package for the direct one.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/rfizzle/llm/internal/cachemarks"
)

// CacheTTL is how long a cached prefix outlives the request that wrote it.
// The API takes these two lifetimes and no others.
type CacheTTL string

const (
	CacheTTL5m CacheTTL = "5m"
	CacheTTL1h CacheTTL = "1h"
)

// DefaultCacheTTL is how long a marked prefix stays cached when nothing
// chose. An hour, because the short lifetime is measured from the last read
// and a session idles past five minutes constantly — the reader is looking at
// a diff, answering an approval card, away from the desk — and an unattended
// run's stop-and-wait makes the pause routine rather than exceptional.
//
// One lifetime covers the whole request: the head and the rolling markers
// alike. The argument for giving the rolling markers the short life is that
// they are replaced every round, so the dearer write buys a prefix the next
// round supersedes — and that argument holds only while the rounds keep
// coming. One pause past five minutes and the conversation under the head has
// expired, so the next round rewrites the whole body at the write premium,
// where the longer lifetime would have paid that premium on one round's delta.
// The pause is what decides, and the pause is systematic. It also leaves one
// rule where there were two: the API takes the longer-lived breakpoints first,
// so two lifetimes in one request had an ordering to keep and one has none.
const DefaultCacheTTL = CacheTTL1h

// ParseCacheTTL maps a config value to the lifetime it names. Empty is the
// default, the way an unset key is everywhere else.
func ParseCacheTTL(s string) (CacheTTL, error) {
	switch CacheTTL(strings.ToLower(strings.TrimSpace(s))) {
	case "":
		return DefaultCacheTTL, nil
	case CacheTTL5m:
		return CacheTTL5m, nil
	case CacheTTL1h:
		return CacheTTL1h, nil
	}
	return DefaultCacheTTL, fmt.Errorf("unknown cache lifetime %q (valid: 5m, 1h)", s)
}

// Describe is the one-line explanation the config screen shows beside each
// lifetime.
func (t CacheTTL) Describe() string {
	if t == CacheTTL1h {
		return "the request survives a long pause; dearer to write, and the default"
	}
	return "the request is written cheaply and expires while you read"
}

// CacheTTLCycle is every lifetime, shortest first — what the config editor
// offers.
func CacheTTLCycle() []CacheTTL {
	return []CacheTTL{CacheTTL5m, CacheTTL1h}
}

// TakesCacheLease reports whether a request for model goes out with a lease
// on its marks: a model the Messages API answers, named directly or through
// a gateway that routes it there. A response that reports the lease it wrote
// says the same thing more surely, and the reading takes that too.
func TakesCacheLease(model string) bool {
	return anthropicRouted(model) || strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "claude-")
}

// CacheTTLOrDefault is the lifetime to go out with, given whatever the
// session was configured with. Nothing it cannot read reaches the wire: an
// unset value is the default, and so is a value that is neither lifetime,
// because every surface that writes the key already refuses anything else
// and what arrives here instead is a hand-edited file — where a typo should
// cost a setting rather than the ability to run.
func CacheTTLOrDefault(s string) CacheTTL {
	ttl, err := ParseCacheTTL(s)
	if err != nil {
		return DefaultCacheTTL
	}
	return ttl
}

// markOpenAICacheBody places the same plan on a chat-completions body bound
// for a gateway that forwards it to the Messages API.
//
// The shape is the one that dialect has for a breakpoint: `cache_control` on
// a content part, so a message whose content is a plain string becomes a
// one-element parts array carrying the same text. The annotation is made on
// the encoded body rather than on the request that produced it because the Go
// client's content part is a closed struct with nowhere to put the field, and
// the encoded body is the only place this request exists as JSON.
//
// Anything it cannot read is returned untouched — a body that is not an
// object, a model the gateway does not route to that API, a message with no
// content to hold a marker — so a request that gains nothing here is byte for
// byte the request that was sent before any of this existed.
func markOpenAICacheBody(body []byte, ttl CacheTTL) []byte {
	ttl = CacheTTLOrDefault(string(ttl))
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return body
	}
	var model string
	if err := json.Unmarshal(envelope["model"], &model); err != nil || !anthropicRouted(model) {
		return body
	}
	var msgs []json.RawMessage
	if err := json.Unmarshal(envelope["messages"], &msgs); err != nil {
		return body
	}

	// The head is the leading system message. It travels inside the message
	// list in this shape and beside it in the native one, so it comes off the
	// front before the conversation is counted — otherwise the two paths
	// would mark different rounds of the same conversation.
	head := -1
	if len(msgs) > 0 && cachemarks.MessageRole(msgs[0]) == string(RoleSystem) {
		head = 0
	}
	marks := cachemarks.Plan(head == 0, len(msgs)-(head+1))

	marked := false
	if marks.Head {
		marked = markOpenAIContentPart(&msgs[head], ttl)
	}
	for _, i := range marks.Messages {
		if markOpenAIContentPart(&msgs[head+1+i], ttl) {
			marked = true
		}
	}
	if !marked {
		return body
	}

	raw, err := json.Marshal(msgs)
	if err != nil {
		return body
	}
	envelope["messages"] = raw
	out, err := json.Marshal(envelope)
	if err != nil {
		return body
	}
	return out
}

// markOpenAIContentPart marks the last text part of one message, promoting a
// string content to the parts array the field lives on, and reports whether
// it placed a marker.
//
// A message with nothing to mark is left exactly as it was: an assistant turn
// that is only tool calls sends no content at all, and inventing an empty
// part to hang a marker on would change what the request says.
func markOpenAIContentPart(msg *json.RawMessage, ttl CacheTTL) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(*msg, &fields); err != nil {
		return false
	}
	content, ok := fields["content"]
	if !ok {
		return false
	}

	var parts []map[string]json.RawMessage
	var text string
	switch {
	case json.Unmarshal(content, &text) == nil:
		if text == "" {
			return false
		}
		quoted, err := json.Marshal(text)
		if err != nil {
			return false
		}
		parts = []map[string]json.RawMessage{{
			"type": json.RawMessage(`"text"`),
			"text": quoted,
		}}
	case json.Unmarshal(content, &parts) == nil:
	default:
		return false
	}

	// From the end, for markLastBlock's reason: an attached image is a part
	// that takes no marker, and the text in front of it caches a prefix one
	// attachment shorter rather than none at all.
	at := -1
	for i := len(parts) - 1; i >= 0; i-- {
		if string(parts[i]["type"]) == `"text"` {
			at = i
			break
		}
	}
	if at < 0 {
		return false
	}
	parts[at]["cache_control"] = json.RawMessage(`{"type":"ephemeral","ttl":"` + string(ttl) + `"}`)

	raw, err := json.Marshal(parts)
	if err != nil {
		return false
	}
	fields["content"] = raw
	out, err := json.Marshal(fields)
	if err != nil {
		return false
	}
	*msg = out
	return true
}

// NewCacheMarkTransport wraps base so that every chat-completions request
// leaving through it carries the breakpoints, where the model it names is one
// the gateway forwards to the Messages API.
//
// It is a round tripper because the encoded body is the only place the field
// can still be reached: the Go client's content part is a closed struct with
// nowhere to put it. And it is here rather than in any one provider because
// more than one path sends that shape to a gateway — the built-in one, and
// every profile whose route speaks `openai-chat` — and a marking that lived
// in one of them left the others paying full price for their whole opening on
// every round of every turn. Which of the two it is cannot be read off the
// dialect: what decides is the model.
//
// A body it cannot read, or one for a model routed anywhere else, goes out
// exactly as it arrived, so a wrapped client that never names an Anthropic
// model sends what it always sent.
func NewCacheMarkTransport(base http.RoundTripper, cacheTTL string) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &cacheMarkTransport{base: base, ttl: CacheTTLOrDefault(cacheTTL)}
}

type cacheMarkTransport struct {
	base http.RoundTripper
	ttl  CacheTTL
}

func (t *cacheMarkTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// A round tripper may not modify the request it was handed, and the body
	// is what this one rewrites — so the edit is made on a copy, and the
	// caller's request is still the one it built.
	req = req.Clone(req.Context())
	if err := t.mark(req); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

// mark replaces the request's body with the marked one. A request with no
// body at all — the catalog query the same client makes — has nothing to mark
// and is left alone.
func (t *cacheMarkTransport) mark(req *http.Request) error {
	if req.Body == nil {
		return nil
	}
	body, err := io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil {
		return err
	}
	body = markOpenAICacheBody(body, t.ttl)
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	// GetBody is what a redirect or a retried HTTP/2 request replays the body
	// from; without it the second attempt would send an empty one.
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	return nil
}
