package compat

import (
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/decision"
)

func TestOpenAICompat_IsNotADecider(t *testing.T) {
	var compat llm.Provider = &Provider{}
	if _, ok := compat.(decision.Decider); ok {
		t.Error("a bare compatible endpoint should not claim the capability")
	}
}
