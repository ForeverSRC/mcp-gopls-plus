package search

import "testing"

func TestTokenizeIdentifierSplitsMixedCase(t *testing.T) {
	got := tokenizeIdentifier("getUserByID")
	want := []string{"get", "user", "by", "id"}
	if len(got) != len(want) {
		t.Fatalf("unexpected token count: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token %d mismatch: got=%q want=%q", i, got[i], want[i])
		}
	}
}
