// Package compat is the backend for any OpenAI-compatible endpoint: a local
// runtime such as Ollama, vLLM or LM Studio, or a gateway that speaks chat
// completions. It speaks that dialect through llm/openai and reads the
// context windows the endpoint reports.
//
// Import it for its side effect to make "openai-compatible" resolvable by
// llm.Resolve:
//
//	import _ "github.com/rfizzle/llm/compat"
//
// A gateway profile over its own client builds the provider with NewNamed.
package compat
