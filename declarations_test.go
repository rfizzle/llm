package llm

import (
	"encoding/json"
	"reflect"
	"testing"
)

// declare registers models under name for the test and withdraws them after.
func declare(t *testing.T, name string, models map[string]Declared) {
	t.Helper()
	RegisterDeclarations(name, models)
	t.Cleanup(func() { RegisterDeclarations(name, nil) })
}

var testSchema = &ResponseSchema{Name: "verdict", Schema: json.RawMessage(`{"type":"object"}`)}

func scopedNoSchema(flow Flow) Declared {
	return Declared{Flows: map[Flow]map[string]any{flow: {"structured_outputs": false}}}
}

func TestSchemaFor_AFlowScopedDeclarationNarrowsOnlyThatFlow(t *testing.T) {
	declare(t, "gw", map[string]Declared{"claude-opus-5": scopedNoSchema(FlowClassifier)})

	classifier := CompletionOpts{Flow: FlowClassifier, ResponseSchema: testSchema}
	if classifier.SchemaFor("claude-opus-5") != nil {
		t.Error("the scoped flow should be sent no schema")
	}
	if c := classifier.Capabilities("claude-opus-5"); !c.NoSchema || c.StructuredOutputs {
		t.Errorf("the scoped flow's answer should be narrowed, got %+v", c)
	}
	backlog := CompletionOpts{Flow: FlowBacklog, ResponseSchema: testSchema}
	if backlog.SchemaFor("claude-opus-5") == nil {
		t.Error("another flow on the same model keeps its schema")
	}
	if !CapabilitiesFor("claude-opus-5").StructuredOutputs {
		t.Error("the model-wide answer is not narrowed by a flow scope")
	}
	if (CompletionOpts{ResponseSchema: testSchema}).SchemaFor("claude-opus-5") == nil {
		t.Error("a request naming no flow is not narrowed by a flow scope")
	}
}

func TestSchemaFor_ModelWideStillMeansEveryFlow(t *testing.T) {
	wide := Declared{Model: map[string]any{"structured_outputs": false}}
	declare(t, "gw", map[string]Declared{"claude-opus-5": wide})
	for _, f := range append(Flows(), "") {
		if (CompletionOpts{Flow: f, ResponseSchema: testSchema}).SchemaFor("claude-opus-5") != nil {
			t.Errorf("flow %q: the model-wide key means every flow", f)
		}
	}
	// A flow-scoped line on top of it changes nothing.
	both := wide
	both.Flows = scopedNoSchema(FlowClassifier).Flows
	declare(t, "gw", map[string]Declared{"claude-opus-5": both})
	for _, f := range []Flow{FlowClassifier, FlowBacklog} {
		if (CompletionOpts{Flow: f, ResponseSchema: testSchema}).SchemaFor("claude-opus-5") != nil {
			t.Errorf("flow %q: a scope on top of the model-wide key widened it", f)
		}
	}
}

// The narrowing runs after the floor as well as after the table, so a model
// the table lacks is narrowed the same way, and keeps the floor's reasoning.
func TestSchemaFor_AFlowScopeNarrowsAModelTheTableLacks(t *testing.T) {
	SetCapabilityLookup(func(string) (Capabilities, bool) { return Capabilities{}, false })
	defer SetCapabilityLookup(nil)
	declare(t, "gw", map[string]Declared{"claude-sonnet-5-5": scopedNoSchema(FlowClassifier)})

	got := (CompletionOpts{Flow: FlowClassifier}).Capabilities("claude-sonnet-5-5")
	want := familyCapabilities("claude-sonnet-5-5")
	want.StructuredOutputs, want.NoSchema = false, true
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want the floor narrowed and nothing else, %+v", got, want)
	}
	if (CompletionOpts{Flow: FlowBacklog, ResponseSchema: testSchema}).SchemaFor("claude-sonnet-5-5") == nil {
		t.Error("the unscoped flow keeps the floor's schema")
	}
}

// Two sources declaring the same model both narrow it; and a scope the
// registry does not allow, which the loader refuses, is passed over by the
// resolver too, so no flow with nothing to narrow can be reached.
func TestDeclarations_NarrowingsCombineAndNothingElseIsReached(t *testing.T) {
	declare(t, "one", map[string]Declared{"claude-opus-5": scopedNoSchema(FlowClassifier)})
	declare(t, "two", map[string]Declared{"claude-opus-5": scopedNoSchema(FlowBacklog)})
	for _, f := range []Flow{FlowClassifier, FlowBacklog} {
		if (CompletionOpts{Flow: f, ResponseSchema: testSchema}).SchemaFor("claude-opus-5") != nil {
			t.Errorf("flow %q: both sources' narrowings apply", f)
		}
	}
	if got := DeclaredOn("claude-opus-5", FlowBacklog, "structured_outputs"); !reflect.DeepEqual(got, []string{"two"}) {
		t.Errorf("DeclaredOn backlog = %v", got)
	}

	declare(t, "bad", map[string]Declared{"gpt-5": scopedNoSchema(FlowTitle)})
	if c := (CompletionOpts{Flow: FlowTitle}).Capabilities("gpt-5"); !c.StructuredOutputs || c.NoSchema {
		t.Errorf("a flow the key cannot scope must not be narrowed, got %+v", c)
	}
	if got := DeclaredOn("gpt-5", FlowTitle, "structured_outputs"); got != nil {
		t.Errorf("DeclaredOn a flow the key cannot scope = %v", got)
	}
	declare(t, "bad", map[string]Declared{"gpt-5": {Model: map[string]any{"structured_outputs": true}}})
	if !CapabilitiesFor("gpt-5").StructuredOutputs {
		t.Error("a value the rule refuses must not narrow anything")
	}
}

func TestDeclarations_TheRegistry(t *testing.T) {
	d, ok := DeclarationFor("structured_outputs")
	if !ok || len(Declarations()) != 2 {
		t.Fatalf("the registry holds two entries today, got %+v", Declarations())
	}
	cache, ok := DeclarationFor(KeyPromptCache)
	if !ok || len(cache.Flows) != 0 || cache.Unscoped != "caching is a property of the endpoint, not of a call" {
		t.Errorf("prompt_cache is scoped by no flow: %+v", cache)
	}
	if cache.Check(false) != nil || cache.Check(true) == nil || cache.Check("false") == nil {
		t.Error("prompt_cache is written only as false")
	}
	if !reflect.DeepEqual(d.Flows, []Flow{FlowClassifier, FlowBacklog}) {
		t.Errorf("structured_outputs scopes %v", d.Flows)
	}
	if d.Check(false) != nil || d.Check(true) == nil || d.Check("false") == nil {
		t.Error("structured_outputs is written only as false")
	}
	for _, f := range Flows() {
		if got, ok := ParseFlow(string(f)); !ok || got != f {
			t.Errorf("ParseFlow(%q)", f)
		}
	}
	if _, ok := ParseFlow("extraction"); ok {
		t.Error("a word outside the set parsed")
	}
}
