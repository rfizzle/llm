package compat

import (
	"testing"

	"github.com/rfizzle/llm"
)

// TestAppIdentityIsPinned pins every string the backends write for
// the caller. The expected values were captured on the commit before the app
// became a setting, by running the same calls against the code that had them
// written in, so a change here is a change in what a person's cache, headers
// or environment see.
//
// The compatible endpoint's part: the two variables it reads.
func TestAppIdentityIsPinned(t *testing.T) {
	for _, k := range []string{"ACME_API_KEY", "ACME_BASE_URL"} {
		t.Setenv(k, "")
	}
	t.Setenv("ACME_API_KEY", "envkey")
	t.Setenv("ACME_BASE_URL", "http://env.invalid/v1")
	c, err := New(llm.ResolveOpts{})
	if err != nil || c.apiKey != "envkey" || c.baseURL != "http://env.invalid/v1" {
		t.Errorf("compat read %q %q (%v)", c.apiKey, c.baseURL, err)
	}
}
