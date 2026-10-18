// Package llm is a client library for large language models. It holds what
// any application that talks to a model needs: the message, tool-call, stream
// and usage types, the provider backends, failure classification and retry,
// prompt-cache marking and model pricing. It carries no application's policy
// and names no app: each caller passes in its own name, credentials and
// logger.
package llm
