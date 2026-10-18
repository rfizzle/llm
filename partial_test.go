package llm

// What a broken stream keeps. The question every case here asks is
// the same one: is this call whole enough to run, or is it a fragment of a
// decision the model never finished making.

import (
	"testing"
)

func TestCompletedToolCalls_KeepsOnlyWholeCalls(t *testing.T) {
	got := CompletedToolCalls([]ToolCall{
		{ID: "1", Name: "read_file", Arguments: `{"path":"a.go"}`},
		{ID: "2", Name: "edit_file", Arguments: `{"path":"b.go","old`}, // cut mid-write
		{ID: "", Name: "read_file", Arguments: `{}`},                   // no id to answer
		{ID: "4", Name: "", Arguments: `{}`},                           // no name to dispatch
		{ID: "5", Name: "list_dir", Arguments: ""},                     // no arguments is whole
	})
	if len(got) != 2 {
		t.Fatalf("kept %d calls, want the two that were finished: %+v", len(got), got)
	}
	if got[0].ID != "1" || got[1].ID != "5" {
		t.Errorf("kept the wrong calls: %+v", got)
	}
}

func TestCompletedToolCalls_EmptyIsNil(t *testing.T) {
	if got := CompletedToolCalls(nil); got != nil {
		t.Errorf("nothing in, nothing out, got %+v", got)
	}
}
