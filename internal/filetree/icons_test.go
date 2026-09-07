package filetree

import "testing"

func TestIconForClosedDirUsesClosedGlyph(t *testing.T) {
	n := &Node{Type: NodeDir, Expanded: false}
	if got := IconFor(n, false); got != fallbackDir {
		t.Fatalf("got %q, want %q (closed dir fallback)", got, fallbackDir)
	}
	if got := IconFor(n, true); got != nerdFontDir {
		t.Fatalf("got %q, want %q (closed dir nerd font)", got, nerdFontDir)
	}
}

func TestIconForExpandedDirUsesOpenGlyph(t *testing.T) {
	n := &Node{Type: NodeDir, Expanded: true}
	if got := IconFor(n, false); got != fallbackDirOpen {
		t.Fatalf("got %q, want %q (open dir fallback)", got, fallbackDirOpen)
	}
	if got := IconFor(n, true); got != nerdFontDirOpen {
		t.Fatalf("got %q, want %q (open dir nerd font)", got, nerdFontDirOpen)
	}
}

func TestIconForOpenAndClosedFallbacksAreDistinct(t *testing.T) {
	if fallbackDir == fallbackDirOpen || fallbackDir == fallbackFile || fallbackDirOpen == fallbackFile {
		t.Fatalf("expected fallbackDir=%q, fallbackDirOpen=%q, fallbackFile=%q to all be distinct", fallbackDir, fallbackDirOpen, fallbackFile)
	}
}
