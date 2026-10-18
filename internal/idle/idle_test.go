package idle

import (
	"context"
	"testing"
	"time"
)

// The setting as the file spells it: unset is the built-in deadline, a number
// is that many seconds, and a negative is a machine that would rather wait.
func TestIdle_TheSettingIsReadAsWritten(t *testing.T) {
	for _, tc := range []struct {
		name    string
		seconds int
		want    time.Duration
	}{
		{"unset", 0, Default},
		{"seconds", 30, 30 * time.Second},
		{"off", -1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Of(tc.seconds).Idle(); got != tc.want {
				t.Errorf("%d seconds = %s, want %s", tc.seconds, got, tc.want)
			}
		})
	}
}

// The deadline is switched off by a negative, and a stream under it is left
// alone. A stalled endpoint would hang the turn, so the case is checked
// against a live one: what is being demanded is that nothing cancels it.
func TestIdle_ADeadlineTurnedOffCancelsNothing(t *testing.T) {
	ctx, watch := Deadline{after: -1}.Guard(context.Background())
	defer watch.Stop()
	watch.Alive()
	time.Sleep(2 * time.Millisecond)
	if err := ctx.Err(); err != nil {
		t.Errorf("a stream with no deadline should not be cancelled: %v", err)
	}
	if err := watch.Err(nil); err != nil {
		t.Errorf("a stream with no deadline reports no idle failure, got %v", err)
	}
}
