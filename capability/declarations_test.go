package capability

import (
	"reflect"
	"testing"

	"github.com/rfizzle/llm/flow"
)

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
	if !reflect.DeepEqual(d.Flows, []flow.Flow{flow.Classifier, flow.Backlog}) {
		t.Errorf("structured_outputs scopes %v", d.Flows)
	}
	if d.Check(false) != nil || d.Check(true) == nil || d.Check("false") == nil {
		t.Error("structured_outputs is written only as false")
	}
}
