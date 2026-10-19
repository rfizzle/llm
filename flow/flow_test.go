package flow

import "testing"

func TestParse_RoundTripsTheSet(t *testing.T) {
	for _, f := range All() {
		if got, ok := Parse(string(f)); !ok || got != f {
			t.Errorf("Parse(%q)", f)
		}
	}
	if _, ok := Parse("extraction"); ok {
		t.Error("a word outside the set parsed")
	}
}
