package memory

import "testing"

func TestTokenizeQueryNormalizesNaturalLanguage(t *testing.T) {
	got := tokenizeQuery("authentication handling")
	want := []string{"auth", "handle"}
	if len(got) != len(want) {
		t.Fatalf("unexpected token count: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token %d mismatch: got=%q want=%q", i, got[i], want[i])
		}
	}
}
