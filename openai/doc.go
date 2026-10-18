// Package openai is the OpenAI backend: chat completions, the Responses API
// and the ChatGPT plan route, which is a mode of the Responses provider. It
// also holds what the other chat-completions dialects share: the message and
// tool converters, the stream reader, the model-catalog reader, the
// Decisions API wire and the SDK's error shape.
//
// Import it for its side effect to make "openai", "openai-responses" and
// "chatgpt" resolvable by llm.Resolve:
//
//	import _ "github.com/rfizzle/llm/openai"
package openai
