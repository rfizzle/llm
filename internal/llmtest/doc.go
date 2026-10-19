// Package llmtest is what the backends' tests share: the app their pinned
// strings were captured under, the in-process endpoints they talk to, the
// readers that drain a stream, and the fixtures every dialect is fed.
//
// It is test support and imports the root package, so the root package's own
// internal tests never import it; they keep their copies.
package llmtest
