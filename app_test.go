package llm

import (
	"os"
	"testing"
)

var acmeApp = App{
	Name:       "acme",
	EnvPrefix:  "ACME",
	SignInHint: "acme login chatgpt",
	URL:        "https://example.com/acme",
}

// TestMain sets the app the way the client does, because the rest of this package's
// tests were written against ACME_API_KEY and the acme- keys.
func TestMain(m *testing.M) {
	SetApp(acmeApp)
	os.Exit(m.Run())
}
