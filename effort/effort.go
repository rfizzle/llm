package effort

// Reasoning effort: how much thinking the session asks a model to do
// before it answers.
//
// Every provider the library speaks has the knob and every one of them spells it
// differently — a named effort on the Responses API and on chat completions,
// a named effort under adaptive thinking on the current Anthropic models, a
// token budget on the older ones and on Gemini. The session should not have
// to know which: it picks one level and each provider translates, after
// fitting the level to what the model in front of it accepts.
//
// Off means the field is not sent at all. That matters more than it sounds:
// `reasoning_effort` on a model that has no reasoning (gpt-4o, gpt-4.1) is a
// 400, so "no opinion" has to stay distinguishable from "the lowest opinion".
// It is no longer the default — a session that says nothing starts on medium —
// but it is still a level a session can choose.

import (
	"fmt"
	"strings"

	"github.com/rfizzle/llm/capability"
)

// Level is the reasoning level asked of the model.
type Level int

const (
	// Off sends no reasoning field: the endpoint's own default applies,
	// and a model that has no reasoning knob is not handed one.
	Off Level = iota
	Low
	Medium
	High
	// XHigh and Max are the two rungs above high that only some
	// models have. Fit lowers them to the highest rung the model does have.
	XHigh
	Max
)

// Default is the level a session starts on when nothing chose one.
const Default = Medium

func (e Level) String() string {
	switch e {
	case Low:
		return "low"
	case Medium:
		return "medium"
	case High:
		return "high"
	case XHigh:
		return "xhigh"
	case Max:
		return "max"
	default:
		return "off"
	}
}

// Describe is the one-line explanation shown by /reasoning and /help.
func (e Level) Describe() string {
	switch e {
	case Low:
		return "a little thinking before answering — the cheapest level that still reasons"
	case Medium:
		return "a working budget for multi-step problems — the default"
	case High:
		return "deep thinking for hard problems; slower and dearer"
	case XHigh:
		return "above high, on the models that have it; the best setting for long agentic work"
	case Max:
		return "the most thinking the model will do, on the models that have it; slowest and dearest"
	default:
		return "no reasoning requested — the model's own default"
	}
}

// On reports whether an effort asks for anything.
func (e Level) On() bool { return e != Off }

// Parse maps a config value, a flag, or a /reasoning argument to its
// Level. "none" is accepted as a spelling of off and "minimal" as one of
// low because the APIs use them for the same ideas. Empty is an error's
// absence, not a level: the caller decides what nothing means, and for a
// session that is Default.
func Parse(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return Default, nil
	case "off", "none", "no":
		return Off, nil
	case "low", "minimal":
		return Low, nil
	case "medium", "med":
		return Medium, nil
	case "high":
		return High, nil
	case "xhigh", "x-high", "extra-high", "extrahigh":
		return XHigh, nil
	case "max", "maximum":
		return Max, nil
	}
	return Off, fmt.Errorf("unknown reasoning level %q (valid: off, low, medium, high, xhigh, max)", s)
}

// Cycle is every level, lowest first — what the config editor offers
// and what the cycle key walks on a model whose rungs are not known.
func Cycle() []Level {
	return []Level{Off, Low, Medium, High, XHigh, Max}
}

// Levels is the rungs a model actually has, lowest first. A model whose
// rungs are not known gets all of them: better to offer a level Fit will
// lower than to hide one the model has.
func Levels(c capability.Capabilities) []Level {
	out := make([]Level, 0, 6)
	for _, e := range Cycle() {
		if e.Fit(c) == e || !c.Known {
			out = append(out, e)
		}
	}
	return out
}

// Next returns the level after current within levels, wrapping around.
// A current level that is not in the list restarts the walk.
func Next(current Level, levels []Level) Level {
	if len(levels) == 0 {
		levels = Cycle()
	}
	for i, e := range levels {
		if e == current {
			return levels[(i+1)%len(levels)]
		}
	}
	return levels[0]
}

// Fit lowers the level to one the model accepts. A rung the model lacks
// becomes the highest one it has; a model with no reasoning knob gets off,
// whatever was asked, because the alternative is a request the API refuses.
// A model nobody could describe is left alone — the caller knows its own
// endpoint's tolerance for a field it may not understand.
func (e Level) Fit(c capability.Capabilities) Level {
	if !e.On() {
		return Off
	}
	if c.Known && !c.Reasoning {
		return Off
	}
	if e == Max && c.Known && !c.Max {
		e = XHigh
	}
	if e == XHigh && c.Known && !c.XHigh {
		e = High
	}
	return e
}

// OpenAIEffort is the `reasoning_effort` / `reasoning.effort` value, or ""
// when nothing should be sent. Fit has already removed a rung the chosen
// model does not accept, so the provider can send the remaining value intact.
func (e Level) OpenAIEffort() string {
	switch e {
	case Off:
		return ""
	}
	return e.String()
}

// thinkingBudgets is the token ladder for the providers whose knob is a
// number rather than a name. The ceiling is the smaller of the two Gemini 2.5
// maxima (flash's 24576), so one ladder serves both families; xhigh and max
// have no more room to ask for than high does.
var thinkingBudgets = map[Level]int{
	Low:    4096,
	Medium: 12288,
	High:   24576,
	XHigh:  24576,
	Max:    24576,
}

// MinThinkingBudget is the smallest budget the Messages API accepts; a
// ceiling that cannot fit it means no thinking rather than a rejected
// request.
const MinThinkingBudget = 1024

// ThinkingBudget is the token budget this effort asks for, clamped to
// ceiling. It returns 0 when nothing should be sent — either the effort is
// off, or the ceiling leaves no room for a budget the API would accept.
func (e Level) ThinkingBudget(ceiling int) int {
	budget := thinkingBudgets[e]
	if budget == 0 {
		return 0
	}
	if ceiling > 0 && budget > ceiling {
		budget = ceiling
	}
	if budget < MinThinkingBudget {
		return 0
	}
	return budget
}
