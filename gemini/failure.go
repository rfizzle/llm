package gemini

import (
	"errors"

	"google.golang.org/genai"
)

// geminiErrorShape reads the Gemini SDK's error, which is a value, not a
// pointer.
func errorShape(err error) (int, string, bool) {
	var genErr genai.APIError
	if errors.As(err, &genErr) {
		return genErr.Code, genErr.Message, true
	}
	return 0, "", false
}
