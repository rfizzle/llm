package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/decision"
	"github.com/rfizzle/llm/failure"
	openai "github.com/sashabaranov/go-openai"
)

// decisionsFloor is the models the native OpenAI providers offer the API on,
// by the floor's own prefix rule. It is a list of its own rather than a field
// of the capability floor because offering it is a claim about an endpoint
// and a model together: the floor is read for a gateway's models as well, and
// a gateway that serves gpt-6-luna's completions need not serve its
// decisions.
var decisionsFloor = []string{"gpt-6-luna"}

// DecisionsOnFloor reports whether the native OpenAI providers offer the
// Decisions API on model.
func DecisionsOnFloor(model string) bool {
	name := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	for _, prefix := range decisionsFloor {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// decisionsWire is the request as the guide spells it; the test that pins
// it names the guide and the date it was read.
type decisionsWire struct {
	Model     string              `json:"model"`
	Input     string              `json:"input"`
	Questions []decisionsQuestion `json:"questions"`
}

type decisionsQuestion struct {
	Type         decision.QuestionType `json:"type"`
	Name         string                `json:"name"`
	Instructions string                `json:"instructions"`
}

// decisionsReply is the response. The probability is a pointer so a
// predicate answer that carries none is told apart from one that said zero.
type decisionsReply struct {
	Answers []struct {
		Type        decision.AnswerType `json:"type"`
		Name        string              `json:"name"`
		Probability *float64            `json:"probability"`
	} `json:"answers"`
	Usage *struct {
		InputTokens int `json:"input_tokens"`
	} `json:"usage"`
}

// maxDecisionsBody bounds the response read. An answer is a few dozen bytes
// per question.
const maxDecisionsBody = 1 << 20

// estimatedBytesPerToken is the rough count a request with no reported usage
// is billed at: the same four bytes a token the context meter estimates with.
const estimatedBytesPerToken = 4

// PostDecisions sends one request to {baseURL}/decisions over client, with
// the key as a bearer token, and names any failure the way every dialect
// names one, under the provider called name.
func PostDecisions(ctx context.Context, client *http.Client, baseURL, apiKey, name string, req decision.Request) (decision.Result, error) {
	classify := failure.NewClassifier(name, llm.CurrentApp().KeyVars("OPENAI_API_KEY"), apiKey)
	res, err := postDecisions(ctx, client, baseURL, apiKey, req)
	if err != nil {
		return decision.Result{}, classify(err)
	}
	return res, nil
}

func postDecisions(ctx context.Context, client *http.Client, baseURL, apiKey string, req decision.Request) (decision.Result, error) {
	if client == nil {
		client = &http.Client{}
	}
	wire := decisionsWire{Model: req.Model, Input: req.Input}
	counted := len(req.Input)
	for _, q := range req.Questions {
		wire.Questions = append(wire.Questions, decisionsQuestion(q))
		counted += len(q.Name) + len(q.Instructions)
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return decision.Result{}, err
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/decisions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return decision.Result{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return decision.Result{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxDecisionsBody))
	if err != nil {
		return decision.Result{}, err
	}
	if resp.StatusCode/100 != 2 {
		// The SDK's own error type, so the failure taxonomy reads the status
		// and the body exactly as it reads a refused completion's.
		return decision.Result{}, &openai.RequestError{
			HTTPStatus:     resp.Status,
			HTTPStatusCode: resp.StatusCode,
			Err:            fmt.Errorf("decisions: %s", resp.Status),
			Body:           raw,
		}
	}
	var reply decisionsReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return decision.Result{}, fmt.Errorf("decisions: unreadable response: %w", err)
	}
	var out decision.Result
	for _, a := range reply.Answers {
		answer := decision.Answer{Type: a.Type, Name: a.Name}
		if a.Type == decision.AnswerPredicate {
			// A predicate with no probability, or one outside the unit
			// interval, is not an answer anybody can hold against a
			// threshold, so the request is a failure rather than a guess.
			if a.Probability == nil || math.IsNaN(*a.Probability) || *a.Probability < 0 || *a.Probability > 1 {
				return decision.Result{}, fmt.Errorf("decisions: answer %q carries no probability", a.Name)
			}
			answer.Probability = *a.Probability
		}
		out.Answers = append(out.Answers, answer)
	}
	if reply.Usage != nil {
		out.Usage = llm.Usage{PromptTokens: reply.Usage.InputTokens}
	} else {
		out.Usage, out.Counted = llm.Usage{PromptTokens: counted / estimatedBytesPerToken}, true
	}
	return out, nil
}
