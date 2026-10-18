package llm_test

import (
	"context"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/pricing"

	// Every backend, the way an app ships them: each registers its names,
	// defaults, catalog and error shape from its init, and the external
	// tests in this directory read the built-ins they make.
	_ "github.com/rfizzle/llm/anthropic"
	_ "github.com/rfizzle/llm/compat"
	_ "github.com/rfizzle/llm/gemini"
	_ "github.com/rfizzle/llm/openai"
	_ "github.com/rfizzle/llm/openrouter"
)

type stubProvider struct{}

func (s *stubProvider) StreamCompletion(_ context.Context, _ []llm.Message, _ llm.CompletionOpts) (<-chan llm.StreamEvent, error) {
	return nil, nil
}

func (s *stubProvider) Name() string { return "stub" }

// The picker's catalog and every provider's defaults are ids typed by hand,
// and a mistyped one fails in two ways at once: the provider answers 404, and
// no price table has heard of it, so the session's own tokens cost nothing
// while the cheap auxiliary calls make the total read as priced. The gateway
// spelling is where this actually happened — OpenRouter writes a generation
// with a dot where Anthropic's own API writes a hyphen.
//
// The snapshot is the right table to ask because it ships in the binary: an
// id it cannot answer for is one a fresh or offline install prices at zero,
// whatever today's download would have said.
func TestHardcodedModelIDsArePriced(t *testing.T) {
	table := pricing.Snapshot()
	if table.Len() == 0 {
		t.Fatal("the built-in snapshot is empty")
	}
	// The openai-compatible default is the name a local runtime answers to,
	// and no public table has ever keyed one. It is the one id here that is
	// deliberately absent, and internal/provider's family floor — not the
	// table — is what prices and sizes it.
	unpriceable := map[string]bool{llm.Defaults("openai-compatible").Model: true}

	check := func(t *testing.T, where, model string) {
		t.Helper()
		if model == "" || unpriceable[model] {
			return
		}
		if _, ok := table.Entry(model); !ok {
			t.Errorf("%s: the snapshot has no entry for %q", where, model)
		}
	}
	for _, name := range llm.Available() {
		for _, model := range llm.KnownModels(name) {
			t.Run(name+"/"+model, func(t *testing.T) {
				check(t, "the "+name+" catalog", model)
			})
		}
	}
	for _, name := range llm.Available() {
		d := llm.Defaults(name)
		t.Run(name+"/defaults", func(t *testing.T) {
			check(t, name+"'s default model", d.Model)
			check(t, name+"'s cheap model", d.CheapModel)
		})
	}
}

func TestRegister_RefusesToReplaceABuiltIn(t *testing.T) {
	for _, name := range []string{"chatgpt", "anthropic", "openai-responses"} {
		if !llm.IsBuiltin(name) {
			t.Fatalf("%s should be a built-in", name)
		}
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Register(%q) should refuse to overwrite", name)
				}
			}()
			llm.Register(name, func(llm.ResolveOpts) (llm.Provider, error) { return &stubProvider{}, nil })
		}()
	}
	if llm.IsBuiltin("a-gateway") {
		t.Fatal("a name nothing ships with is not a built-in")
	}
}

// Every provider whose catalog can be known ahead of time names a small
// model for the bounded calls, and the one whose catalog cannot does not:
// openai-compatible points at whatever endpoint the user runs, and a name
// guessed for it is a request that 404s.
func TestDefaults_CheapModel(t *testing.T) {
	for _, name := range []string{"anthropic", "openai", "openai-responses", "gemini", "openrouter"} {
		d := llm.Defaults(name)
		if d.CheapModel == "" {
			t.Errorf("%s names no cheap model", name)
			continue
		}
		if d.CheapModel == d.Model {
			t.Errorf("%s: the cheap model is the default model", name)
		}
		if !llm.CapabilitiesFor(d.CheapModel).Known {
			t.Errorf("%s: nothing describes %q", name, d.CheapModel)
		}
	}
	if got := llm.Defaults("openai-compatible").CheapModel; got != "" {
		t.Errorf("openai-compatible should name none, got %q", got)
	}
	// The underscore spelling reaches the same entry the model does.
	if llm.Defaults("openai_responses").CheapModel != llm.Defaults("openai-responses").CheapModel {
		t.Error("the name should normalise the same way for both fields")
	}
}

// The compatible endpoint registers no decisions floor, so a model nothing
// declares offers nothing on it. It is asked here, where the backend is
// imported, so a floor it started registering would be caught.
func TestCapabilities_DecisionsAreDeclaredNotGuessed(t *testing.T) {
	if !llm.IsBuiltin("openai-compatible") {
		t.Fatal("openai-compatible should be a built-in here")
	}
	if llm.DecisionsDeclared("openai-compatible", "gpt-6-luna") {
		t.Error("an endpoint nothing declares should offer nothing")
	}
}
