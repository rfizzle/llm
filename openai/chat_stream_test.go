package openai

import (
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

// A large tool argument arrives as a very long run of very small fragments —
// a rewritten file is hundreds of kilobytes in pieces of a few dozen bytes —
// and it is assembled on the goroutine reading the wire, so the cost of
// assembling it is time the stream is not being read. Growing a string per
// fragment made that quadratic.
func BenchmarkToolCallSetAccumulate(b *testing.B) {
	const (
		fragment  = "0123456789abcdefghijklmnopqrstuvwxyz0123" // 40 bytes
		fragments = 300_000 / len(fragment)
	)
	id := "call_abc"
	index := 0
	b.ReportAllocs()
	for b.Loop() {
		set := newToolCallSet()
		if _, err := set.accumulate(openai.ToolCall{
			ID: id, Index: &index, Function: openai.FunctionCall{Name: "write_file"},
		}); err != nil {
			b.Fatal(err)
		}
		for i := 0; i < fragments; i++ {
			if _, err := set.accumulate(openai.ToolCall{
				Index: &index, Function: openai.FunctionCall{Arguments: fragment},
			}); err != nil {
				b.Fatal(err)
			}
		}
		if n := len(set.calls()[0].Arguments); n != fragments*len(fragment) {
			b.Fatalf("assembled %d bytes", n)
		}
	}
}
