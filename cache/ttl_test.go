package cache

import (
	"testing"
)

func TestParseCacheTTL(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want TTL
		bad  bool
	}{
		{in: "", want: DefaultTTL},
		{in: "5m", want: TTL5m},
		{in: " 1H ", want: TTL1h},
		{in: "10m", want: DefaultTTL, bad: true},
		{in: "forever", want: DefaultTTL, bad: true},
	} {
		got, err := ParseTTL(tc.in)
		if (err != nil) != tc.bad {
			t.Errorf("ParseTTL(%q) error = %v, want bad = %v", tc.in, err, tc.bad)
		}
		if got != tc.want {
			t.Errorf("ParseTTL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
