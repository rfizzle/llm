// Package cachemarks chooses where a request's prompt-cache markers go.
//
// Two kinds of path reach the Messages API and both have to be told: the
// Anthropic backend, which speaks it directly, and the chat-completions body
// bound for a gateway that forwards it (the root package's cache-mark
// transport). The positions have to be the same on both, so they are chosen
// once here and applied twice — a rule written down twice is a rule that
// gets corrected once.
package cachemarks

import (
	"encoding/json"
)

// Max is how many cache_control markers one request may carry; the
// API refuses a fifth. Three are placed, and the spare is the margin that
// keeps a later marker from having to displace an existing one.
const Max = 4

// Rolling is how many of the conversation's trailing messages are
// marked.
//
// One would be enough if the provider always found the previous round's
// marker on its own. It searches back a bounded number of blocks from the
// marker it is given, and a round that ran its tool calls in parallel appends
// more than that bound: eight requests and the eight results answering them
// is already sixteen blocks before the reasoning and the reply are counted.
// The second mark is the previous round's own boundary, named outright so
// the search never has to reach it.
const Rolling = 2

// Marks is where one request's markers go, as positions rather than in
// any dialect's own types: the fixed head — the tools and the system prompt,
// which the API caches as one prefix because the tools precede the system
// prompt in it — and the trailing messages whose ends are the round
// boundaries.
//
// The head takes one marker and not two. A marker on the system prompt covers
// the tools with it, so marking the tools as well would spend a second marker
// on a prefix the first one already contains.
type Marks struct {
	// Head is whether there is a head to mark at all.
	Head bool
	// Messages are indexes into the conversation under the head, ascending.
	Messages []int
}

// Plan chooses a request's marker positions from whether it has a
// head and how many messages sit under it.
//
// It marks positions, never content: every marker lands on something the
// request was already sending. A request the API declines to cache (a prefix
// under its minimum, a dialect that does not know the field) is answered
// exactly as it would have been without the markers, which is what makes this
// safe to send to a gateway too.
func Plan(head bool, messages int) Marks {
	marks := Marks{Head: head}
	spent := 0
	if head {
		spent = 1
	}
	for i := max(messages-Rolling, 0); i < messages && spent < Max; i++ {
		marks.Messages = append(marks.Messages, i)
		spent++
	}
	return marks
}

// MessageRole is one message's role, or the empty string when it is not an
// object carrying one.
func MessageRole(msg json.RawMessage) string {
	var fields struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(msg, &fields); err != nil {
		return ""
	}
	return fields.Role
}
