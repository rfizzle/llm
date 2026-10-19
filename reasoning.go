package llm

// Reasoning blocks: what the model thought, carried back verbatim. The
// level a request asks the model to think at is llm/effort.

// ReasoningBlock is one provider-native reasoning block the model produced,
// carried back into the conversation exactly as it arrived.
//
// It exists for one requirement: with extended thinking on, the Messages API
// rejects a follow-up request whose last assistant turn contains tool_use but
// not the thinking blocks that led to it. So the blocks are not decoration —
// dropping them turns the second round of every thinking turn into a 400.
// Text and Signature are the two halves of a thinking block; Redacted holds
// the opaque payload of a safety-redacted one, which travels the same way and
// has no readable content. The Responses API has no readable half at all:
// there Signature carries the reasoning item's id and Redacted its sealed
// content, and both go back verbatim because nothing is stored server-side.
type ReasoningBlock struct {
	Text      string
	Signature string
	Redacted  string
}

// ReplayFrom is the index of the first message whose reasoning still goes on
// the wire. Everything before it keeps its blocks in the conversation and
// sends none of them.
//
// Replayed thinking is not free. Anthropic's own pricing page counts
// "thinking blocks from prior assistant turns that remain in context" as
// input tokens, and every model this product would put a session on keeps
// all of them by default, so an hour-long session pays again, every round,
// for every round it has ever thought — in the one category a trim cannot
// reach, because a trim rewrites tool results and never an assistant turn.
// The Responses API does not even use them: outside its newest models it
// carries an earlier turn's tokens forward without rendering that turn's
// reasoning into the next sample, and its own guidance is to pass back the
// reasoning items since the last user message and no more.
//
// So the cut is the last user turn, which is where the current tool chain
// starts — with one turn of slack. A round boundary can append a user
// message of its own (a tree notice, an intervention) after an assistant
// turn that asked for a tool, and cutting at that message would send the
// tool_use with no thinking behind it: the follow-up the Messages API
// refuses, which is the whole reason the blocks are carried. Keeping the
// last assistant turn too costs one turn of thinking and closes that.
//
// The cut only moves forward, and that is what makes it safe on the dialect
// that checks: a thinking block records which one came before it, so
// removing blocks from the front of a history, oldest first, leaves every
// later block valid, while removing one from the middle invalidates all of
// them.
//
// Moving it costs the prefix cache from that position on, which is why it is
// the user turn and not the round: within a turn it does not move, and a
// turn is where the rounds are.
func ReplayFrom(messages []Message) int {
	lastUser, lastAssistant := -1, -1
	for i, m := range messages {
		switch m.Role {
		case RoleUser:
			lastUser = i
		case RoleAssistant:
			lastAssistant = i
		}
	}
	// A conversation with no user turn is all current chain, and one with no
	// assistant turn carries no reasoning to cut.
	if lastUser < 0 || lastAssistant < 0 {
		return 0
	}
	return min(lastUser, lastAssistant)
}
