package responses

import (
	"encoding/json"
	"testing"

	"github.com/rfizzle/llm"
)

// TestAppIdentityIsPinned pins every string the backends write for
// the caller. The expected values were captured on the commit before the app
// became a setting, by running the same calls against the code that had them
// written in, so a change here is a change in what a person's cache, headers
// or environment see.
//
// The Responses route's part: the variables it reads, its prompt-cache key,
// the sign-in hint and the plan route's tool group. The chat-completions key
// is pinned in llm/openai.
func TestAppIdentityIsPinned(t *testing.T) {
	for _, k := range []string{"ACME_API_KEY", "ACME_BASE_URL", "OPENAI_API_KEY"} {
		t.Setenv(k, "")
	}
	t.Setenv("ACME_API_KEY", "envkey")
	t.Setenv("ACME_BASE_URL", "http://env.invalid/v1")
	r, err := New(llm.ResolveOpts{})
	if err != nil || r.apiKey != "envkey" || r.baseURL != "http://env.invalid/v1" {
		t.Errorf("responses read %q %q (%v)", r.apiKey, r.baseURL, err)
	}

	for _, c := range []struct{ name, got, want string }{
		{"responses cache key", responseCacheKey("m", "ins", []llm.Tool{{Name: "t", Description: "d", Parameters: json.RawMessage(`{}`)}}), "acme-6edd4baac2de7dee38778661cb677e8d"},
		{"sign-in hint", chatGPTKeyEnv(), "acme login chatgpt"},
		{"tool group", planToolNamespace(), "acme"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}

	plan := &responsesRequest{Tools: []responsesTool{{Name: "read_file"}}, Input: []responseItem{{Type: "function_call"}}}
	rename := shapeForPlan(plan)
	body, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	const wantPlan = `{"model":"","input":[{"type":"function_call","namespace":"acme"}],"stream":true,"store":false,"tools":[{"type":"namespace","name":"acme","description":"The tools this session can call.","tools":[{"type":"","name":"read_file"}]}]}`
	if string(body) != wantPlan {
		t.Errorf("plan request = %s, want %s", body, wantPlan)
	}
	if got := rename("", "acme.read_file"); got != "read_file" {
		t.Errorf("a group-qualified name mapped to %q", got)
	}
}
