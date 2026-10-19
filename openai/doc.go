// Package openai is the OpenAI chat-completions backend. It also holds what
// the other chat-completions dialects share: the message and tool converters,
// the stream reader, the model-catalog reader, the Decisions API wire and the
// SDK's error shape. The Responses API and the ChatGPT plan route are
// llm/openai/responses.
//
// Import it for its side effect to make "openai" resolvable by llm.Resolve:
//
//	import _ "github.com/rfizzle/llm/openai"
package openai
