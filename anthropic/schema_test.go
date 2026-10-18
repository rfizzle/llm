package anthropic

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/internal/llmtest"
)

// A tool whose arguments are an array of objects has to arrive at the model
// as that array on every dialect. A converter that rebuilds a schema from the
// fields its own SDK struct happens to have describes the array as a
// free-form value, and the model sends whatever it guessed — which the tool
// then refuses, one round later, for the whole call.
func TestToolSchema_NestedArraySurvivesEveryDialect(t *testing.T) {
	raw := llmtest.NestedArraySchema
	var want map[string]any
	if err := json.Unmarshal([]byte(raw), &want); err != nil {
		t.Fatal(err)
	}
	tools := []llm.Tool{{Name: "edit_file", Parameters: json.RawMessage(raw)}}

	for _, tc := range []struct {
		dialect string
		schema  any
	}{
		{"anthropic", toAnthropicTools(tools)[0].OfTool.InputSchema},
	} {
		raw, err := json.Marshal(tc.schema)
		if err != nil {
			t.Fatalf("%s: marshal: %v", tc.dialect, err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("%s: unmarshal: %v", tc.dialect, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s must send the schema as written:\n got %v\nwant %v", tc.dialect, got, want)
		}
	}
}
