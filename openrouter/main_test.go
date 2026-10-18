package openrouter

import (
	"testing"

	"github.com/rfizzle/llm/internal/llmtest"
)

// TestMain sets the app the way the client does, because the tests here were
// written against ACME_API_KEY and the acme- keys.
func TestMain(m *testing.M) { llmtest.Main(m) }
