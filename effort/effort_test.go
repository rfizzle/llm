package effort

import (
	"testing"

	"github.com/rfizzle/llm/capability"
)

func TestParseEffort(t *testing.T) {
	cases := map[string]Level{
		"":        Default,
		"off":     Off,
		"none":    Off,
		"minimal": Low,
		"  LOW ":  Low,
		"med":     Medium,
		"medium":  Medium,
		"High":    High,
		"xhigh":   XHigh,
		"x-high":  XHigh,
		"max":     Max,
	}
	for in, want := range cases {
		got, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("Parse(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := Parse("ultra"); err == nil {
		t.Error("expected an error for an unknown level, not a silent off")
	}
}

func TestNextEffort_Cycles(t *testing.T) {
	want := []Level{Low, Medium, High, XHigh, Max, Off}
	cur := Off
	for i, expect := range want {
		cur = Next(cur, nil)
		if cur != expect {
			t.Fatalf("step %d: got %v, want %v", i, cur, expect)
		}
	}
	// A model's own rungs are the walk; a level off the list restarts it.
	levels := Levels(capability.Capabilities{Known: true, Reasoning: true})
	if got := Next(High, levels); got != Off {
		t.Errorf("after high on a low/medium/high model: got %v, want off", got)
	}
	if got := Next(Max, levels); got != Off {
		t.Errorf("a level the model lacks restarts the walk: got %v", got)
	}
}

func TestEffort_FitLowersToWhatTheModelHas(t *testing.T) {
	cases := []struct {
		name string
		in   Level
		caps capability.Capabilities
		want Level
	}{
		{"unknown model is left alone", Max, capability.Capabilities{}, Max},
		{"off stays off", Off, capability.Capabilities{Known: true, Reasoning: true, Max: true}, Off},
		{"no reasoning knob means nothing sent", High, capability.Capabilities{Known: true}, Off},
		{"max on a model with max", Max, capability.Capabilities{Known: true, Reasoning: true, XHigh: true, Max: true}, Max},
		{"max without max falls to xhigh", Max, capability.Capabilities{Known: true, Reasoning: true, XHigh: true}, XHigh},
		{"max without either falls to high", Max, capability.Capabilities{Known: true, Reasoning: true}, High},
		{"xhigh without xhigh falls to high", XHigh, capability.Capabilities{Known: true, Reasoning: true, Max: true}, High},
		{"medium is everywhere", Medium, capability.Capabilities{Known: true, Reasoning: true}, Medium},
	}
	for _, tc := range cases {
		if got := tc.in.Fit(tc.caps); got != tc.want {
			t.Errorf("%s: Fit(%v) = %v, want %v", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestLevels_AreTheModelsRungs(t *testing.T) {
	got := Levels(capability.Capabilities{Known: true, Reasoning: true, Max: true})
	want := []Level{Off, Low, Medium, High, Max}
	if len(got) != len(want) {
		t.Fatalf("levels = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("levels = %v, want %v", got, want)
		}
	}
	if got := Levels(capability.Capabilities{}); len(got) != len(Cycle()) {
		t.Errorf("an unknown model offers every level, got %v", got)
	}
	if got := Levels(capability.Capabilities{Known: true}); len(got) != 1 || got[0] != Off {
		t.Errorf("a model without reasoning offers only off, got %v", got)
	}
}

func TestEffort_OpenAIEffortPreservesTheFittedRung(t *testing.T) {
	if got := Max.OpenAIEffort(); got != "max" {
		t.Errorf("max = %q, want max", got)
	}
	if got := XHigh.OpenAIEffort(); got != "xhigh" {
		t.Errorf("xhigh = %q", got)
	}
}

func TestEffort_OpenAIEffortOffSendsNothing(t *testing.T) {
	if got := Off.OpenAIEffort(); got != "" {
		t.Errorf("off must send no effort field, got %q", got)
	}
	if got := High.OpenAIEffort(); got != "high" {
		t.Errorf("high = %q", got)
	}
}

func TestEffort_ThinkingBudget(t *testing.T) {
	if got := Off.ThinkingBudget(0); got != 0 {
		t.Errorf("off must ask for no budget, got %d", got)
	}
	if got := High.ThinkingBudget(0); got != 24576 {
		t.Errorf("uncapped high = %d", got)
	}
	// A ceiling clamps rather than overshoots.
	if got := High.ThinkingBudget(8000); got != 8000 {
		t.Errorf("clamped high = %d, want the ceiling", got)
	}
	// A ceiling below the API's minimum means no thinking at all, not a
	// budget the API would refuse.
	if got := High.ThinkingBudget(MinThinkingBudget - 1); got != 0 {
		t.Errorf("ceiling under the minimum = %d, want 0", got)
	}
}
