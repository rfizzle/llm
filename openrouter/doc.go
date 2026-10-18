// Package openrouter is the OpenRouter backend: chat completions through
// llm/openai, the attribution headers the gateway asks for, a stable session
// key per conversation, and the cache markers for the models it routes to
// the Messages API.
//
// Import it for its side effect to make "openrouter" resolvable by
// llm.Resolve:
//
//	import _ "github.com/rfizzle/llm/openrouter"
package openrouter
