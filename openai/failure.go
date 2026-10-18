package openai

import (
	"errors"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// openAIErrorShape reads the chat-completions SDK's two errors: a refusal the
// endpoint wrote as JSON, and a response it could not decode, whose body is
// the message.
func errorShape(err error) (int, string, bool) {
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		return apiErr.HTTPStatusCode, apiErr.Message, true
	}
	var reqErr *openai.RequestError
	if errors.As(err, &reqErr) {
		message := strings.TrimSpace(string(reqErr.Body))
		if message == "" && reqErr.Err != nil {
			message = reqErr.Err.Error()
		}
		return reqErr.HTTPStatusCode, message, true
	}
	return 0, "", false
}
