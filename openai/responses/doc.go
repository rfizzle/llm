// Package responses is the OpenAI Responses API backend and the ChatGPT
// plan route, which is a mode of the same provider. It reads the model
// catalog and the Decisions API wire from llm/openai.
//
// Import it for its side effect to make "openai-responses" and "chatgpt"
// resolvable by llm.Resolve:
//
//	import _ "github.com/rfizzle/llm/openai/responses"
package responses
