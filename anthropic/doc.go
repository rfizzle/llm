// Package anthropic is the Anthropic Messages API backend: streaming,
// extended thinking, tool use, the prompt-cache markers the API needs placed
// on the request, and the SDK's error shape for the failure taxonomy.
//
// Import it for its side effect to make "anthropic" resolvable by llm.Resolve:
//
//	import _ "github.com/rfizzle/llm/anthropic"
//
// An app that holds a configured SDK client builds the provider with NewWith
// or NewNamed instead.
package anthropic
