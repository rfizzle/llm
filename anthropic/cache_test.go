package anthropic

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/rfizzle/llm/cache"
	"github.com/rfizzle/llm/internal/cachemarks"
)

// paramsWithTurns builds a request whose conversation has n ordinary turns,
// which is all these tests need to see where the markers land.
func paramsWithTurns(system string, n int) anthropic.MessageNewParams {
	params := anthropic.MessageNewParams{}
	if system != "" {
		params.System = []anthropic.TextBlockParam{{Text: system}}
	}
	for i := 0; i < n; i++ {
		params.Messages = append(params.Messages,
			anthropic.NewUserMessage(anthropic.NewTextBlock("ask")),
			anthropic.NewAssistantMessage(anthropic.NewTextBlock("answer")),
		)
	}
	return params
}

// markedMessages is the indexes of params.Messages carrying a marker.
func markedMessages(params anthropic.MessageNewParams) []int {
	var out []int
	for i, msg := range params.Messages {
		for _, block := range msg.Content {
			if cc := block.GetCacheControl(); cc != nil && cc.Type != "" {
				out = append(out, i)
				break
			}
		}
	}
	return out
}

func countMarks(t *testing.T, params anthropic.MessageNewParams) int {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(raw), `"cache_control"`)
}

func TestMarkAnthropicCacheMarksTheFixedHead(t *testing.T) {
	params := paramsWithTurns("be helpful", 1)
	markAnthropicCache(&params, cache.DefaultTTL)

	if params.System[0].CacheControl.Type == "" {
		t.Fatal("the system prompt must carry a marker: it and the tools are the prefix that never changes")
	}
}

func TestMarkAnthropicCacheRollsOverTheLastTurns(t *testing.T) {
	params := paramsWithTurns("be helpful", 4)
	markAnthropicCache(&params, cache.DefaultTTL)

	last := len(params.Messages) - 1
	want := []int{last - 1, last}
	got := markedMessages(params)
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("rolling markers must sit on the last two messages, want %v got %v", want, got)
	}
}

// The round the second rolling marker exists for: a batch of parallel calls
// appends more blocks than the provider searches back through, so the
// previous round's boundary has to be named rather than found.
func TestMarkAnthropicCacheNamesThePreviousRoundAcrossAParallelBatch(t *testing.T) {
	params := paramsWithTurns("be helpful", 1)
	var calls, results []anthropic.ContentBlockParamUnion
	// The loop's own ceiling on parallel calls, spelled out rather than
	// imported: the agent package is downstream of this one.
	const parallel = 8
	for i := 0; i < parallel; i++ {
		calls = append(calls, anthropic.ContentBlockParamUnion{
			OfToolUse: &anthropic.ToolUseBlockParam{ID: "t", Name: "search", Input: map[string]any{}},
		})
		results = append(results, anthropic.NewToolResultBlock("t", "hit", false))
	}
	params.Messages = append(params.Messages,
		anthropic.NewAssistantMessage(calls...),
		anthropic.NewUserMessage(results...),
	)
	markAnthropicCache(&params, cache.DefaultTTL)

	got := markedMessages(params)
	last := len(params.Messages) - 1
	if len(got) != 2 || got[1] != last || got[0] != last-1 {
		t.Fatalf("the batch's own boundary and the one before it must both be marked, got %v", got)
	}
}

func TestMarkAnthropicCacheStaysUnderTheAPILimit(t *testing.T) {
	params := paramsWithTurns("be helpful", 40)
	markAnthropicCache(&params, cache.DefaultTTL)

	if n := countMarks(t, params); n > cachemarks.Max {
		t.Fatalf("a request may carry at most %d markers, got %d", cachemarks.Max, n)
	}
}

func TestMarkAnthropicCacheSkipsABlockThatCannotCarryOne(t *testing.T) {
	params := paramsWithTurns("be helpful", 1)
	// A turn whose reasoning is its last block: thinking takes no marker, so
	// the text in front of it is what must be marked.
	params.Messages = append(params.Messages, anthropic.NewAssistantMessage(
		anthropic.NewTextBlock("thinking out loud"),
		anthropic.NewThinkingBlock("sig", "why"),
	))
	markAnthropicCache(&params, cache.DefaultTTL)

	blocks := params.Messages[len(params.Messages)-1].Content
	if blocks[0].OfText.CacheControl.Type == "" {
		t.Fatal("a marker must fall back to the last block that can carry one")
	}
	if countMarks(t, params) == 0 {
		t.Fatal("the request lost its markers entirely")
	}
}

func TestMarkAnthropicCacheOnAnEmptyRequest(t *testing.T) {
	params := anthropic.MessageNewParams{}
	markAnthropicCache(&params, cache.DefaultTTL)

	if n := countMarks(t, params); n != 0 {
		t.Fatalf("nothing to mark, got %d markers", n)
	}
}

// The markers must be the only difference a request carries: they are a
// statement about billing, and a dialect that ignores them has to see the
// same conversation it would have seen.
func TestMarkAnthropicCacheChangesNothingButTheMarkers(t *testing.T) {
	before := paramsWithTurns("be helpful", 3)
	after := paramsWithTurns("be helpful", 3)
	markAnthropicCache(&after, cache.DefaultTTL)

	strip := func(params anthropic.MessageNewParams) string {
		raw, err := json.Marshal(params)
		if err != nil {
			t.Fatal(err)
		}
		out := regexp.MustCompile(`,?"cache_control":\{[^}]*\},?`).ReplaceAllStringFunc(string(raw), func(m string) string {
			if strings.HasPrefix(m, ",") && strings.HasSuffix(m, ",") {
				return ","
			}
			return ""
		})
		return out
	}
	if strip(before) != strip(after) {
		t.Fatalf("marking rewrote the request:\n before %s\n  after %s", strip(before), strip(after))
	}
}

// Every marker in one request gets the same lifetime, the conversation's
// included. A rolling marker on the shorter one expires over any pause past
// five minutes — reading a diff, answering an approval card — and the round
// after that pause rewrites the whole body instead of extending it.
func TestMarkAnthropicCacheGivesEveryMarkerOneLifetime(t *testing.T) {
	params := paramsWithTurns("be helpful", 2)
	markAnthropicCache(&params, cache.DefaultTTL)

	if got := params.System[0].CacheControl.TTL; got != anthropic.CacheControlEphemeralTTL(cache.TTL1h) {
		t.Errorf("the head's lifetime is %q, want an hour by default", got)
	}
	for _, i := range markedMessages(params) {
		for _, block := range params.Messages[i].Content {
			cc := block.GetCacheControl()
			if cc == nil || cc.Type == "" {
				continue
			}
			if cc.TTL != anthropic.CacheControlEphemeralTTL(cache.TTL1h) {
				t.Errorf("message %d's marker is %q, want the head's own lifetime", i, cc.TTL)
			}
		}
	}
}

func TestMarkAnthropicCacheFollowsTheConfiguredLifetime(t *testing.T) {
	params := paramsWithTurns("be helpful", 2)
	markAnthropicCache(&params, cache.TTL5m)

	if got := params.System[0].CacheControl.TTL; got != anthropic.CacheControlEphemeralTTL(cache.TTL5m) {
		t.Errorf("the head's lifetime is %q, want the one that was chosen", got)
	}
	for _, i := range markedMessages(params) {
		for _, block := range params.Messages[i].Content {
			cc := block.GetCacheControl()
			if cc == nil || cc.Type == "" {
				continue
			}
			if cc.TTL != anthropic.CacheControlEphemeralTTL(cache.TTL5m) {
				t.Errorf("message %d's marker is %q, want the one that was chosen", i, cc.TTL)
			}
		}
	}
}

// A session pointed at a gateway profile speaking the Messages API is the
// same session pointed at a different address, so the lifetime it chose has
// to survive the trip.
func TestNewAnthropicNamedHonoursTheConfiguredLifetime(t *testing.T) {
	if got := NewNamed(anthropic.NewClient(), "claude-opus-5", "gateway", "5m").cacheTTL; got != cache.TTL5m {
		t.Errorf("the profile's lifetime is %q, want the one the session chose", got)
	}
	if got := NewNamed(anthropic.NewClient(), "claude-opus-5", "gateway", "").cacheTTL; got != cache.DefaultTTL {
		t.Errorf("an unset lifetime is %q, want the default", got)
	}
}
