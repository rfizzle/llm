package compat

import (
	"testing"

	"github.com/rfizzle/llm"
)

func TestOpenAICompat_IsNotADecider(t *testing.T) {
	var compat llm.Provider = &Provider{}
	if _, ok := compat.(llm.Decider); ok {
		t.Error("a bare compatible endpoint should not claim the capability")
	}
}
