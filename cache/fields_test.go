package cache

import "testing"

func TestCacheFields_NamesReadBack(t *testing.T) {
	all := FieldRead | FieldCreation | FieldLease | FieldCachedTokens | FieldCachedContent
	if got := FieldsNamed(all.Names()); got != all {
		t.Errorf("round trip: %v", got)
	}
	if got := FieldsNamed([]string{"cached_tokens", "something_else"}); got != FieldCachedTokens {
		t.Errorf("an unknown name: %v", got)
	}
}
