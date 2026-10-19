package openai

import (
	"testing"

	"github.com/rfizzle/llm"
	openai "github.com/sashabaranov/go-openai"
)

func TestToolCallSet_AssemblesEveryCallItOpened(t *testing.T) {
	// A stream that broke mid-accumulation leaves calls in both states: one
	// the model finished, and one opened by an index whose id never arrived.
	// Both are assembled, in the order they were opened, and which of them is
	// whole enough to run is judged once, afterwards.
	set := newToolCallSet()
	first, second := 1, 2
	for _, chunk := range []openai.ToolCall{
		{Index: &first, ID: "a", Function: openai.FunctionCall{Name: "read_file", Arguments: "{}"}},
		{Index: &second, Function: openai.FunctionCall{Arguments: `{"path":`}},
	} {
		if _, err := set.accumulate(chunk); err != nil {
			t.Fatalf("accumulating %+v: %v", chunk, err)
		}
	}

	calls := set.calls()
	if len(calls) != 2 || calls[0].ID != "a" || calls[1].ID != "" {
		t.Fatalf("both calls should survive the assembly in order, got %+v", calls)
	}
	if whole := llm.CompletedToolCalls(calls); len(whole) != 1 || whole[0].ID != "a" {
		t.Fatalf("only the finished call is runnable, got %+v", whole)
	}
}
