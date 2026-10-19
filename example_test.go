package llm_test

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/internal/testhttp"
	"github.com/rfizzle/llm/openai/responses"
	"github.com/rfizzle/llm/pricing"
)

// Example_setApp names the program once at startup and resolves a provider
// from the environment: ACME_API_KEY and ACME_BASE_URL are read before the
// vendor's own variables.
func Example_setApp() {
	llm.SetApp(llm.App{
		Name:       "acme",
		EnvPrefix:  "ACME",
		SignInHint: "acme login chatgpt",
		URL:        "https://example.com/acme",
	})
	prev, had := os.LookupEnv("ACME_API_KEY")
	_ = os.Setenv("ACME_API_KEY", "sk-demo")
	defer func() {
		if had {
			_ = os.Setenv("ACME_API_KEY", prev)
		} else {
			_ = os.Unsetenv("ACME_API_KEY")
		}
	}()

	p, err := llm.Resolve("openai-responses", llm.ResolveOpts{Model: "gpt-demo"})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(p.Name())
	// Output: openai-responses
}

// Example_streamAToolCall streams one completion that ends in a tool call and
// prices what it cost. The server is in process (internal/testhttp), so the
// example needs no network; a real app calls llm.Resolve and leaves the
// transport alone.
func Example_streamAToolCall() {
	var origins testhttp.Registry
	srv := origins.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"type":"response.output_text.delta","delta":"Reading it."}`+"\n\n")
		fmt.Fprint(w, `data: {"type":"response.completed","response":{"status":"completed","output":[`+
			`{"type":"function_call","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"go.mod\"}"}],`+
			`"usage":{"input_tokens":1000,"output_tokens":50,"input_tokens_details":{"cached_tokens":400}}}}`+"\n\n")
	}))
	defer srv.Close()

	p := responses.NewWith(origins.Client(), "sk-demo", srv.URL+"/v1", "demo-model", "")
	events, err := p.StreamCompletion(context.Background(),
		[]llm.Message{{Role: llm.RoleUser, Content: "What does go.mod say?"}},
		llm.CompletionOpts{
			Model: "demo-model",
			Tools: []llm.Tool{{
				Name:        "read_file",
				Description: "Read a file.",
				Parameters:  []byte(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			}},
		})
	if err != nil {
		fmt.Println(err)
		return
	}
	reply, err := llm.Collect(context.Background(), events)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(reply.Text, reply.Calls[0].Name, reply.Calls[0].Arguments, reply.Stop)

	table := pricing.NewTable(map[string]pricing.ModelPricing{
		"demo-model": {InputCostPerToken: 1e-6, OutputCostPerToken: 4e-6, CacheReadCostPerToken: 1e-7},
	})
	u := reply.Usage
	cost, found := table.Price("demo-model", int64(u.PromptTokens), int64(u.CompletionTokens), int64(u.CachedTokens), int64(u.CacheCreationTokens))
	fmt.Printf("%d in, %d cached, %d out: $%.6f (priced: %v)\n", u.PromptTokens, u.CachedTokens, u.CompletionTokens, cost, found)
	// Output:
	// Reading it. read_file {"path":"go.mod"} tool
	// 1000 in, 400 cached, 50 out: $0.000840 (priced: true)
}
