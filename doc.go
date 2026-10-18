// Package llm is a client library for large language models. It holds what
// any application that talks to a model needs: the message, tool-call, stream
// and usage types, the provider registry, failure classification and retry,
// prompt-cache marking and model pricing. It carries no application's policy
// and names no app: each caller passes in its own name, credentials and
// logger.
//
// The backends are packages of their own: llm/anthropic, llm/openai (chat
// completions, the Responses API and the ChatGPT plan route), llm/gemini,
// llm/openrouter and llm/compat (any OpenAI-compatible endpoint). Each
// registers its provider names from its init, the way a database/sql driver
// does, so an app imports the backends it ships, for their side effect where
// it only resolves them by name:
//
//	import _ "github.com/rfizzle/llm/anthropic"
//
// and Resolve answers for those names and no others.
package llm
