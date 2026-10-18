package llm

import (
	"testing"
)

func TestParseCacheTTL(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want CacheTTL
		bad  bool
	}{
		{in: "", want: DefaultCacheTTL},
		{in: "5m", want: CacheTTL5m},
		{in: " 1H ", want: CacheTTL1h},
		{in: "10m", want: DefaultCacheTTL, bad: true},
		{in: "forever", want: DefaultCacheTTL, bad: true},
	} {
		got, err := ParseCacheTTL(tc.in)
		if (err != nil) != tc.bad {
			t.Errorf("ParseCacheTTL(%q) error = %v, want bad = %v", tc.in, err, tc.bad)
		}
		if got != tc.want {
			t.Errorf("ParseCacheTTL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
