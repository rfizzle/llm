package llm

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/rfizzle/llm/capability"
	llmflow "github.com/rfizzle/llm/flow"
)

// declare registers models under name for the test and withdraws them after.
func declare(t *testing.T, name string, models map[string]capability.Declared) {
	t.Helper()
	capability.RegisterDeclarations(name, models)
	t.Cleanup(func() { capability.RegisterDeclarations(name, nil) })
}

var testSchema = &ResponseSchema{Name: "verdict", Schema: json.RawMessage(`{"type":"object"}`)}

func scopedNoSchema(flow llmflow.Flow) capability.Declared {
	return capability.Declared{Flows: map[llmflow.Flow]map[string]any{flow: {"structured_outputs": false}}}
}

func TestSchemaFor_AFlowScopedDeclarationNarrowsOnlyThatFlow(t *testing.T) {
	declare(t, "gw", map[string]capability.Declared{"claude-opus-5": scopedNoSchema(llmflow.Classifier)})

	classifier := CompletionOpts{Flow: llmflow.Classifier, ResponseSchema: testSchema}
	if classifier.SchemaFor("claude-opus-5") != nil {
		t.Error("the scoped flow should be sent no schema")
	}
	if c := classifier.Capabilities("claude-opus-5"); !c.NoSchema || c.StructuredOutputs {
		t.Errorf("the scoped flow's answer should be narrowed, got %+v", c)
	}
	backlog := CompletionOpts{Flow: llmflow.Backlog, ResponseSchema: testSchema}
	if backlog.SchemaFor("claude-opus-5") == nil {
		t.Error("another flow on the same model keeps its schema")
	}
	if !capability.For("claude-opus-5").StructuredOutputs {
		t.Error("the model-wide answer is not narrowed by a flow scope")
	}
	if (CompletionOpts{ResponseSchema: testSchema}).SchemaFor("claude-opus-5") == nil {
		t.Error("a request naming no flow is not narrowed by a flow scope")
	}
}

func TestSchemaFor_ModelWideStillMeansEveryFlow(t *testing.T) {
	wide := capability.Declared{Model: map[string]any{"structured_outputs": false}}
	declare(t, "gw", map[string]capability.Declared{"claude-opus-5": wide})
	for _, f := range append(llmflow.All(), "") {
		if (CompletionOpts{Flow: f, ResponseSchema: testSchema}).SchemaFor("claude-opus-5") != nil {
			t.Errorf("flow %q: the model-wide key means every flow", f)
		}
	}
	// A flow-scoped line on top of it changes nothing.
	both := wide
	both.Flows = scopedNoSchema(llmflow.Classifier).Flows
	declare(t, "gw", map[string]capability.Declared{"claude-opus-5": both})
	for _, f := range []llmflow.Flow{llmflow.Classifier, llmflow.Backlog} {
		if (CompletionOpts{Flow: f, ResponseSchema: testSchema}).SchemaFor("claude-opus-5") != nil {
			t.Errorf("flow %q: a scope on top of the model-wide key widened it", f)
		}
	}
}

// The narrowing runs after the floor as well as after the table, so a model
// the table lacks is narrowed the same way, and keeps the floor's reasoning.
func TestSchemaFor_AFlowScopeNarrowsAModelTheTableLacks(t *testing.T) {
	capability.SetLookup(func(string) (capability.Capabilities, bool) { return capability.Capabilities{}, false })
	defer capability.SetLookup(nil)
	declare(t, "gw", map[string]capability.Declared{"claude-sonnet-5-5": scopedNoSchema(llmflow.Classifier)})

	got := (CompletionOpts{Flow: llmflow.Classifier}).Capabilities("claude-sonnet-5-5")
	// The lookup answers nothing and the declaration is scoped to a flow, so
	// the model-wide answer is the family floor's.
	want := capability.For("claude-sonnet-5-5")
	want.StructuredOutputs, want.NoSchema = false, true
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want the floor narrowed and nothing else, %+v", got, want)
	}
	if (CompletionOpts{Flow: llmflow.Backlog, ResponseSchema: testSchema}).SchemaFor("claude-sonnet-5-5") == nil {
		t.Error("the unscoped flow keeps the floor's schema")
	}
}

// Two sources declaring the same model both narrow it; and a scope the
// registry does not allow, which the loader refuses, is passed over by the
// resolver too, so no flow with nothing to narrow can be reached.
func TestDeclarations_NarrowingsCombineAndNothingElseIsReached(t *testing.T) {
	declare(t, "one", map[string]capability.Declared{"claude-opus-5": scopedNoSchema(llmflow.Classifier)})
	declare(t, "two", map[string]capability.Declared{"claude-opus-5": scopedNoSchema(llmflow.Backlog)})
	for _, f := range []llmflow.Flow{llmflow.Classifier, llmflow.Backlog} {
		if (CompletionOpts{Flow: f, ResponseSchema: testSchema}).SchemaFor("claude-opus-5") != nil {
			t.Errorf("flow %q: both sources' narrowings apply", f)
		}
	}
	if got := capability.DeclaredOn("claude-opus-5", llmflow.Backlog, "structured_outputs"); !reflect.DeepEqual(got, []string{"two"}) {
		t.Errorf("DeclaredOn backlog = %v", got)
	}

	declare(t, "bad", map[string]capability.Declared{"gpt-5": scopedNoSchema(llmflow.Title)})
	if c := (CompletionOpts{Flow: llmflow.Title}).Capabilities("gpt-5"); !c.StructuredOutputs || c.NoSchema {
		t.Errorf("a flow the key cannot scope must not be narrowed, got %+v", c)
	}
	if got := capability.DeclaredOn("gpt-5", llmflow.Title, "structured_outputs"); got != nil {
		t.Errorf("DeclaredOn a flow the key cannot scope = %v", got)
	}
	declare(t, "bad", map[string]capability.Declared{"gpt-5": {Model: map[string]any{"structured_outputs": true}}})
	if !capability.For("gpt-5").StructuredOutputs {
		t.Error("a value the rule refuses must not narrow anything")
	}
}

// A declared "no schema" outranks the floor and can only narrow it.
func TestSchemaFor_AProfileCanDeclareNoSchema(t *testing.T) {
	capability.SetLookup(func(model string) (capability.Capabilities, bool) {
		switch model {
		case "claude-sonnet-5-5", "claude-opus-5-5":
			return capability.Capabilities{Reasoning: true, Adaptive: true}, true
		}
		return capability.Capabilities{}, false
	})
	defer capability.SetLookup(nil)
	declare(t, "gw", map[string]capability.Declared{"claude-sonnet-5-5": {Model: map[string]any{"structured_outputs": false}}})

	opts := CompletionOpts{ResponseSchema: &ResponseSchema{Name: "verdict", Schema: []byte(`{"type":"object"}`)}}
	if opts.SchemaFor("claude-sonnet-5-5") != nil {
		t.Error("a model declared to take no schema should be sent none")
	}
	if capability.For("claude-sonnet-5-5").StructuredOutputs {
		t.Error("the declaration should outrank the floor")
	}
	if opts.SchemaFor("claude-opus-5-5") == nil {
		t.Error("a model that declared nothing keeps the floor's answer")
	}
	if opts.SchemaFor("llama3") != nil {
		t.Error("nothing is widened: an undescribed model still gets none")
	}
}
