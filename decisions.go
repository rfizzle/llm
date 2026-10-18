package llm

// The Decisions API: typed questions put over one piece of shared evidence,
// answered as probabilities rather than as prose. A bounded call that only
// ever wanted a yes or a no — the permission classifier — can ask it instead
// of a completion, and what comes back is a number to hold against a
// threshold rather than a reply to parse.
//
// The API is a public beta, so its request shape lives here and nowhere else,
// and the test that pins it names the guide it was written against. Which
// models offer it is declared, never inferred: the native OpenAI providers
// answer from a floor of their own below, a gateway profile from a model it
// declares, and a model nothing names does not offer it.

import (
	"context"
)

// Decider is a provider that can put typed questions to a model through the
// Decisions API. It is optional, like ModelLister: a caller asserts it, and
// asks OffersDecisions for the model it means to use before it asks anything,
// because a provider that can speak the API still serves models that cannot.
type Decider interface {
	// OffersDecisions reports whether model is declared to answer the
	// Decisions API on this provider. False is the answer for every model
	// nothing declares.
	OffersDecisions(model string) bool
	// Decide sends one request and returns its typed answers.
	Decide(ctx context.Context, req DecisionRequest) (DecisionResult, error)
}

// QuestionType is the kind of question asked. Only a predicate is modelled:
// the API also has choice and score questions, and nothing in the library asks one,
// so neither is spelled here until something does.
type QuestionType string

const QuestionPredicate QuestionType = "predicate"

// DecisionQuestion is one question. Instructions is the proposition the
// model weighs; the evidence it is weighed against is the request's Input,
// shared by every question, and never part of the instructions.
type DecisionQuestion struct {
	Type         QuestionType
	Name         string
	Instructions string
}

// DecisionRequest is one request: the model, the evidence, the questions.
type DecisionRequest struct {
	Model     string
	Input     string
	Questions []DecisionQuestion
}

// AnswerType is the kind of answer a question came back with: a predicate's
// probability, or a refusal.
type AnswerType string

const (
	AnswerPredicate AnswerType = "predicate"
	// AnswerRefusal is the model declining to answer. It is an answer of its
	// own kind and carries no probability: a refusal read as a zero would be
	// a confident no, and read as anything else a guess.
	AnswerRefusal AnswerType = "refusal"
)

// DecisionAnswer is one question's answer, by the question's name.
// Probability is meaningful only on a predicate answer.
type DecisionAnswer struct {
	Type        AnswerType
	Name        string
	Probability float64
}

// DecisionResult is what one request came back with.
type DecisionResult struct {
	Answers []DecisionAnswer
	// Usage is the input the request is billed for. The API bills input
	// tokens only, so CompletionTokens is always zero.
	Usage Usage
	// Counted reports that Usage is the caller's own count of the input it sent
	// rather than the provider's figure. The guide documents no usage block
	// on a response, so a response without one is billed at the estimate
	// rather than at nothing.
	Counted bool
}

// Answer is the answer to the question called name, if one came back.
func (r DecisionResult) Answer(name string) (DecisionAnswer, bool) {
	for _, a := range r.Answers {
		if a.Name == name {
			return a, true
		}
	}
	return DecisionAnswer{}, false
}

// decisionsDeclared is each registered provider's answer to whether a model
// offers the API, for the surfaces that ask without building a provider —
// the doctor's row.
var decisionsDeclared = map[string]func(model string) bool{}

// RegisterDecisions records how a provider answers OffersDecisions, under its
// name. A profile registers its declared models; the native providers
// register the floor.
func RegisterDecisions(name string, offers func(model string) bool) {
	if offers == nil {
		delete(decisionsDeclared, normalizeName(name))
		return
	}
	decisionsDeclared[normalizeName(name)] = offers
}

// DecisionsDeclared reports whether the provider registered as name offers the
// Decisions API on model, as that provider would answer it.
func DecisionsDeclared(name, model string) bool {
	offers, ok := decisionsDeclared[normalizeName(name)]
	return ok && offers(model)
}
