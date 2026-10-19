// Package cachemarks chooses where a request's prompt-cache markers go.
//
// Two kinds of path reach the Messages API and both have to be told: the
// Anthropic backend, which speaks it directly, and the chat-completions body
// bound for a gateway that forwards it (the root package's cache-mark
// transport). The positions have to be the same on both, so they are chosen
// once here and applied twice — a rule written down twice is a rule that
// gets corrected once.
package cachemarks
