package failure

// The taxonomy's tests that need no backend: the key's tail, the wait a
// provider asked for, the failure's own words and the log line. The tests
// that feed it each SDK's errors are in shapes_test.go, beside the
// backends that read them.

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestKeyTail(t *testing.T) {
	for _, tc := range []struct{ key, want string }{
		{"", ""},
		{"ab", "ab"},
		{"abcd", "abcd"},
		{"sk-proj-longer-4f9c", "4f9c"},
		{"  sk-padded-1234  ", "1234"},
	} {
		if got := keyTail(tc.key); got != tc.want {
			t.Errorf("keyTail(%q) = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestRetryAfter(t *testing.T) {
	for _, tc := range []struct {
		message string
		want    time.Duration
	}{
		{"Rate limit reached. Please try again in 20s.", 20 * time.Second},
		{"retry after 38 seconds", 38 * time.Second},
		{"Please try again in 1.5s", 1500 * time.Millisecond},
		{"try again in 2 minutes", 2 * time.Minute},
		{"Please try again in 480ms", 480 * time.Millisecond},
		{"rate limited", 0},
	} {
		if got := retryAfter(tc.message); got != tc.want {
			t.Errorf("retryAfter(%q) = %v, want %v", tc.message, got, tc.want)
		}
	}
}

func TestFailure_HeadlineAndRecoverable(t *testing.T) {
	withStatus := &Failure{Class: ClassAuth, Status: 401}
	if got := withStatus.Headline(); got != "401 unauthorized" {
		t.Errorf("Headline() = %q", got)
	}
	if withStatus.Recoverable() {
		t.Error("a rejected key is not a stall the session comes back from on its own")
	}
	bare := &Failure{Class: ClassNetwork}
	if got := bare.Headline(); got != "network" {
		t.Errorf("Headline() without a status = %q", got)
	}
	if !bare.Recoverable() {
		t.Error("a network failure is recoverable")
	}
	for _, class := range []Class{ClassRateLimit, ClassOverloaded, ClassNetwork} {
		if !(&Failure{Class: class}).Recoverable() {
			t.Errorf("%q should be recoverable", class)
		}
	}
	for _, class := range []Class{ClassAuth, ClassQuota, ClassContextLength, ClassMalformed, ClassCancelled, ClassUnclassified, ClassUsageLimit, ClassNotEligible, ClassUnsupported} {
		if (&Failure{Class: class}).Recoverable() {
			t.Errorf("%q should not be recoverable", class)
		}
	}
}

func TestFailure_ErrorAndUnwrap(t *testing.T) {
	inner := errors.New("inner")
	f := &Failure{Class: ClassOverloaded, Message: "Overloaded", err: inner}
	if got := f.Error(); got != "overloaded: Overloaded" {
		t.Errorf("Error() = %q", got)
	}
	if !errors.Is(f, inner) {
		t.Error("the provider's own error should stay reachable")
	}
	if got := (&Failure{Class: ClassCancelled}).Error(); got != "cancelled" {
		t.Errorf("Error() without a message = %q", got)
	}
}

func TestClassify_NilStaysNil(t *testing.T) {
	if err := NewClassifier("openai", "", "")(nil); err != nil {
		t.Errorf("classifying nil returned %v", err)
	}
}

// The line is the one the diagnostic log has always held: the same message
// and the same attributes in the same order, whoever the logger belongs to.
func TestClassify_TheRefusalLineIsUnchanged(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})))
	t.Cleanup(func() { SetLogger(nil) })

	record(&Failure{Provider: "openai", Class: ClassRateLimit, Status: 429, Code: "rl", RequestID: "req_1", Message: "slow down"})
	want := `level=ERROR msg="provider request refused" provider=openai class="rate limited" status=429 code=rl request_id=req_1 detail="slow down"` + "\n"
	if got := buf.String(); got != want {
		t.Errorf("the refusal line changed:\n got %q\nwant %q", got, want)
	}
}

func TestSetLogger_NilAndUnsetDiscard(t *testing.T) {
	SetLogger(nil)
	record(&Failure{Provider: "openai", Class: ClassRateLimit})
	if log() == nil {
		t.Fatal("a nil logger must be the discard logger")
	}
}
