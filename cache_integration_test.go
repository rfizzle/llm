//go:build integration

package llm_test

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/anthropic"
	"github.com/rfizzle/llm/openrouter"
)

// TestPromptCacheIntegration checks that the markers the anthropic package places are
// honoured by a real Messages API endpoint: the first request writes the
// prefix and the second reads it back.
//
// Everything else about caching is asserted offline — that the markers land in
// the right places, stay under the API's limit, and change nothing else about
// the request. None of that can tell you whether the other end honours them,
// and a marker that is silently ignored looks exactly like one that works: the
// answer is identical and only the bill differs.
//
// It costs real requests, so it runs only when pointed at an endpoint:
//
//	ACME_CACHE_IT_URL=https://api.anthropic.com \
//	ACME_CACHE_IT_KEY=sk-… \
//	go test . -tags integration -run CacheIntegration -v
//
// The prefix is deliberately far above the minimum cacheable size. That
// minimum differs by model — it is larger for the small ones — and a prefix
// near it produces a run that reports no caching and does not say whether the
// cause was the size or the endpoint.
func TestPromptCacheIntegration(t *testing.T) {
	base, key := os.Getenv("ACME_CACHE_IT_URL"), os.Getenv("ACME_CACHE_IT_KEY")
	if base == "" || key == "" {
		t.Skip("set ACME_CACHE_IT_URL and ACME_CACHE_IT_KEY to run")
	}
	model := os.Getenv("ACME_CACHE_IT_MODEL")
	if model == "" {
		model = llm.Defaults("anthropic").Model
	}

	p, err := anthropic.New(llm.ResolveOpts{APIKey: key, BaseURL: base, Model: model})
	if err != nil {
		t.Fatal(err)
	}

	system := "You are terse. Answer in one word.\n" + strings.Repeat(
		"Background: the operator maintains a command-line coding agent with a passive "+
			"loop, a provider registry, a permission classifier and a golden-file suite. ", 800)

	ask := func(msgs []llm.Message) *llm.Usage {
		t.Helper()
		ch, err := p.StreamCompletion(context.Background(), msgs, llm.CompletionOpts{Model: model, MaxTokens: 32})
		if err != nil {
			t.Fatal(err)
		}
		var usage *llm.Usage
		for ev := range ch {
			if ev.Err != nil {
				t.Fatal(ev.Err)
			}
			if ev.Usage != nil {
				usage = ev.Usage
			}
		}
		if usage == nil {
			t.Fatal("no usage reported")
		}
		return usage
	}

	first := []llm.Message{
		{Role: llm.RoleSystem, Content: system},
		{Role: llm.RoleUser, Content: "Say one."},
	}
	u1 := ask(first)
	t.Logf("first:  prompt=%d fresh=%d written=%d read=%d",
		u1.PromptTokens, u1.PromptTokens-u1.CachedTokens-u1.CacheCreationTokens,
		u1.CacheCreationTokens, u1.CachedTokens)

	// The same prefix with two more messages on the end, which is the shape
	// every round after the first has.
	second := append(append([]llm.Message{}, first...),
		llm.Message{Role: llm.RoleAssistant, Content: "One."},
		llm.Message{Role: llm.RoleUser, Content: "Say two."},
	)
	u2 := ask(second)
	t.Logf("second: prompt=%d fresh=%d written=%d read=%d",
		u2.PromptTokens, u2.PromptTokens-u2.CachedTokens-u2.CacheCreationTokens,
		u2.CacheCreationTokens, u2.CachedTokens)

	if u1.CacheCreationTokens == 0 {
		t.Fatal("the first request cached nothing: the endpoint is ignoring the markers")
	}
	if u2.CachedTokens == 0 {
		t.Fatal("the second request read nothing back, so every round pays full price")
	}
	if u2.CachedTokens < u1.CacheCreationTokens {
		t.Errorf("the second request should read back what the first wrote: wrote %d, read %d",
			u1.CacheCreationTokens, u2.CachedTokens)
	}
	// The parts are disjoint subsets of the prompt count, which is the
	// invariant every reader of a ledger depends on (provider.go).
	if fresh := u2.PromptTokens - u2.CachedTokens - u2.CacheCreationTokens; fresh < 0 {
		t.Errorf("the cached parts must be inside the prompt count, not beside it: %+v", u2)
	}
}

// cacheConversationRounds is how many rounds the conversation check runs. Five
// is the shortest run that shows the thing worth showing: the first writes,
// the second reads the head back, and the three after it read a prefix that
// grew since the round before — which is the claim a two-request check cannot
// make.
const cacheConversationRounds = 5

// parallelCacheCalls is the batch that answers the middle round, sized at the
// loop's own ceiling on parallel tool calls. Its sixteen blocks are why a request
// marks two round boundaries rather than one, and a live run is the only
// place that can be checked: offline the marker is where it is asked to be,
// and here it either found the previous round's prefix or it did not.
const parallelCacheCalls = 8

// TestPromptCacheIntegrationOverAConversation runs a conversation the length
// of a short turn against a real Messages API endpoint and watches what each
// round reads back from the cache.
//
// TestPromptCacheIntegration above proves the endpoint honours a marker at
// all. This proves the other half — that the marks roll forward: every round
// reads the prefix the round before it left, across a round of parallel tool
// calls, which is the round where the boundary has to be named rather than
// found. Run it with `make cache-check`.
func TestPromptCacheIntegrationOverAConversation(t *testing.T) {
	base, key := os.Getenv("ACME_CACHE_IT_URL"), os.Getenv("ACME_CACHE_IT_KEY")
	if base == "" || key == "" {
		t.Skip("set ACME_CACHE_IT_URL and ACME_CACHE_IT_KEY to run")
	}
	model := cmp.Or(os.Getenv("ACME_CACHE_IT_MODEL"), llm.Defaults("anthropic").Model)

	p, err := anthropic.New(llm.ResolveOpts{APIKey: key, BaseURL: base, Model: model})
	if err != nil {
		t.Fatal(err)
	}
	runCachedConversation(t, p, model)
}

// TestPromptCacheIntegrationThroughAGateway is the same conversation sent in
// the OpenAI shape to a gateway that forwards it to the Messages API.
//
// It is the case that was wrong for longest and the one nothing offline can
// settle: the breakpoints are written in a dialect the gateway is free to
// drop on the way through, and a gateway that drops them answers exactly what
// one that forwards them answers. Only the bill differs, so only a live run
// can tell them apart.
//
//	ACME_CACHE_IT_GATEWAY_URL=https://openrouter.ai/api/v1 \
//	ACME_CACHE_IT_GATEWAY_KEY=sk-… \
//	go test . -tags integration -run CacheIntegration -v
func TestPromptCacheIntegrationThroughAGateway(t *testing.T) {
	base, key := os.Getenv("ACME_CACHE_IT_GATEWAY_URL"), os.Getenv("ACME_CACHE_IT_GATEWAY_KEY")
	if base == "" || key == "" {
		t.Skip("set ACME_CACHE_IT_GATEWAY_URL and ACME_CACHE_IT_GATEWAY_KEY to run")
	}
	// The model has to be one the gateway routes to that API — nothing else
	// is being asked about — so the default is the gateway provider's own.
	model := cmp.Or(os.Getenv("ACME_CACHE_IT_GATEWAY_MODEL"), llm.Defaults("openrouter").Model)
	if !strings.HasPrefix(strings.ToLower(model), "anthropic/") {
		t.Fatalf("%q is not routed to the Messages API, so there is nothing here to cache", model)
	}

	p, err := openrouter.New(llm.ResolveOpts{APIKey: key, BaseURL: base, Model: model})
	if err != nil {
		t.Fatal(err)
	}
	runCachedConversation(t, p, model)
}

// runCachedConversation drives cacheConversationRounds rounds through one
// provider and asserts that what each round reads back from the cache climbs
// with the conversation.
//
// The replies are written here rather than taken from the model so that both
// endpoints see the same conversation and the run costs the same every time.
// What is being measured is what the request carries, and a fabricated reply
// carries what a real one does.
func runCachedConversation(t *testing.T, p llm.Provider, model string) {
	t.Helper()

	// Far above the minimum cacheable prefix, which differs by model and is
	// largest for the small ones: a prefix near it produces a run that reports
	// no caching and cannot say whether the cause was the size or the endpoint.
	system := "You are terse. Answer in one word.\n" + strings.Repeat(
		"Background: the operator maintains a command-line coding agent with a passive "+
			"loop, a provider registry, a permission classifier and a golden-file suite. ", 800)

	tools := []llm.Tool{{
		Name:        "search",
		Description: "Search the project.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
	}}

	ask := func(msgs []llm.Message) *llm.Usage {
		t.Helper()
		ch, err := p.StreamCompletion(context.Background(), msgs, llm.CompletionOpts{
			Model:     model,
			MaxTokens: 32,
			Tools:     tools,
		})
		if err != nil {
			t.Fatal(err)
		}
		var usage *llm.Usage
		for ev := range ch {
			if ev.Err != nil {
				t.Fatal(ev.Err)
			}
			if ev.Usage != nil {
				usage = ev.Usage
			}
		}
		if usage == nil {
			t.Fatal("no usage reported")
		}
		return usage
	}

	msgs := []llm.Message{{Role: llm.RoleSystem, Content: system}}
	reads := make([]int, 0, cacheConversationRounds)
	for round := 1; round <= cacheConversationRounds; round++ {
		// Each round's own text, long enough that the prefix it adds is
		// unambiguously more than the round before it cached.
		msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: fmt.Sprintf("Round %d. ", round) + strings.Repeat(
			"Say the next number and nothing else; the notes below are context you already have. ", 20)})

		u := ask(msgs)
		reads = append(reads, u.CachedTokens)
		t.Logf("round %d: prompt=%d fresh=%d written=%d read=%d", round,
			u.PromptTokens, u.PromptTokens-u.CachedTokens-u.CacheCreationTokens,
			u.CacheCreationTokens, u.CachedTokens)
		if fresh := u.PromptTokens - u.CachedTokens - u.CacheCreationTokens; fresh < 0 {
			t.Errorf("round %d: the cached parts must be inside the prompt count, not beside it: %+v", round, u)
		}

		// The middle round is the parallel batch: one assistant turn asking
		// for every call at once, and the results answering them.
		if round == cacheConversationRounds/2 {
			var calls []llm.ToolCall
			var results []llm.Message
			for i := 0; i < parallelCacheCalls; i++ {
				id := fmt.Sprintf("call_%d_%d", round, i)
				calls = append(calls, llm.ToolCall{ID: id, Name: "search", Arguments: fmt.Sprintf(`{"query":"note %d"}`, i)})
				results = append(results, llm.Message{Role: llm.RoleTool, ToolCallID: id, Content: strings.Repeat(
					fmt.Sprintf("note %d: the registry resolves a provider from flags, config and environment. ", i), 6)})
			}
			msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: "Looking.", ToolCalls: calls})
			msgs = append(msgs, results...)
		}
		msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: fmt.Sprintf("%d.", round)})
	}

	if reads[1] == 0 {
		t.Fatal("the second round read nothing back, so every round pays full price")
	}
	for i := 2; i < len(reads); i++ {
		if reads[i] < reads[i-1] {
			t.Errorf("round %d read %d after round %d read %d: the marks are not rolling forward",
				i+1, reads[i], i, reads[i-1])
		}
	}
	if reads[len(reads)-1] <= reads[1] {
		t.Errorf("the last round read %d and the second read %d: what is cached should grow with the conversation",
			reads[len(reads)-1], reads[1])
	}
}
