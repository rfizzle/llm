package flow

import (
	"slices"
	"strings"
)

// Flow names one bounded call outside the main agent and its children. The
// empty Flow is the main agent and every child: nothing may be scoped there.
type Flow string

// The closed set of flows. A word here is what a profile writes under a
// model's `flows` table, and what the call that builds the request sets.
const (
	Classifier       Flow = "classifier"
	Explanation      Flow = "explanation"
	Description      Flow = "description"
	Reading          Flow = "reading"
	Title            Flow = "title"
	Account          Flow = "account"
	Suggestion       Flow = "suggestion"
	StartOffers      Flow = "start_offers"
	Patterns         Flow = "patterns"
	Compaction       Flow = "compaction"
	Backlog          Flow = "backlog"
	ProfileDrafter   Flow = "profile_drafter"
	ToolchainDrafter Flow = "toolchain_drafter"
)

var flows = []Flow{
	Classifier, Explanation, Description, Reading,
	Title, Account, Suggestion, StartOffers, Patterns,
	Compaction, Backlog, ProfileDrafter, ToolchainDrafter,
}

// All is the closed set, in the order a listing reads it.
func All() []Flow { return slices.Clone(flows) }

// Parse reads a flow word, reporting whether it is one of the set.
func Parse(s string) (Flow, bool) {
	f := Flow(s)
	return f, slices.Contains(flows, f)
}

// Words is the closed set as a profile writes it, for a refusal.
func Words() string {
	words := make([]string, len(flows))
	for i, f := range flows {
		words[i] = string(f)
	}
	return strings.Join(words, ", ")
}
