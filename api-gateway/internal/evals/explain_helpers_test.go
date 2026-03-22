package evals

import "testing"

func TestFirstNonEmpty(t *testing.T) {
	got := firstNonEmpty("", "  ", "value", "later")
	if got != "value" {
		t.Fatalf("expected value, got %q", got)
	}
}

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" one, two ,, three ")
	if len(got) != 3 || got[0] != "one" || got[2] != "three" {
		t.Fatalf("unexpected split result: %#v", got)
	}
}

func TestFirstInt(t *testing.T) {
	attrs := map[string]string{"bad": "x", "good": "12"}
	if got := firstInt(attrs, "bad", "good"); got != 12 {
		t.Fatalf("expected 12, got %d", got)
	}
	if got := firstInt(attrs, "missing"); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}
