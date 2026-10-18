package anthropic

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// anthropicErrorShape reads the Messages API SDK's error.
func errorShape(err error) (int, string, bool) {
	var anthErr *anthropic.Error
	if errors.As(err, &anthErr) {
		return anthErr.StatusCode, anthropicMessage(anthErr), true
	}
	return 0, "", false
}

// anthropicMessage reads the message out of the Messages API's error
// envelope, falling back to the error type it names — `overloaded_error` is a
// worse sentence than the message but a better one than nothing.
func anthropicMessage(err *anthropic.Error) string {
	var envelope struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if raw := err.RawJSON(); raw != "" {
		if json.Unmarshal([]byte(raw), &envelope) == nil && envelope.Error.Message != "" {
			return envelope.Error.Message
		}
	}
	if t := string(err.Type()); t != "" {
		return t
	}
	return fmt.Sprintf("HTTP %d", err.StatusCode)
}
