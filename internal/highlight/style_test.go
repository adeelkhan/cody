package highlight

import "testing"

func TestStyleForKnownCapture(t *testing.T) {
	if _, ok := StyleFor("keyword"); !ok {
		t.Fatal("expected a style for \"keyword\"")
	}
}

func TestStyleForUnknownCapture(t *testing.T) {
	if _, ok := StyleFor("no-such-capture"); ok {
		t.Fatal("expected no style for an unknown capture name")
	}
}
