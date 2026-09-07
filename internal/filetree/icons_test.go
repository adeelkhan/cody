package filetree

import "testing"

func TestIconForKnownExtension(t *testing.T) {
	n := &Node{Type: NodeFile, Name: "main.go"}
	if IconFor(n, true) != nerdFontIcons[".go"] {
		t.Fatal("expected go icon")
	}
}

func TestIconForUnknownExtension(t *testing.T) {
	n := &Node{Type: NodeFile, Name: "data.bin"}
	if IconFor(n, true) != nerdFontFile {
		t.Fatal("expected generic file icon")
	}
	if IconFor(n, false) != fallbackFile {
		t.Fatal("expected fallback file icon")
	}
}

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

// TestNerdFontGlyphsAreRealCodepointsNotEmpty is a canary against a past
// failure mode: writing the Private Use Area glyphs below as pasted
// (invisible) characters rather than typed backslash-u escapes has
// silently produced empty strings before, and a test that only compares
// IconFor's output against the package's own constants (like the tests
// above) can never catch that -- an empty constant matches an empty
// constant. This test instead compares against literal expected rune
// values written as backslash-u escapes in a rune literal.
func TestNerdFontGlyphsAreRealCodepointsNotEmpty(t *testing.T) {
	cases := map[string]rune{
		"nerdFontDir":     '',
		"nerdFontDirOpen": '',
		"nerdFontFile":    '',
	}
	got := map[string]string{
		"nerdFontDir":     nerdFontDir,
		"nerdFontDirOpen": nerdFontDirOpen,
		"nerdFontFile":    nerdFontFile,
	}
	for name, want := range cases {
		g := got[name]
		if g == "" {
			t.Fatalf("%s is empty -- expected literal codepoint %U", name, want)
		}
		r := []rune(g)
		if len(r) != 1 || r[0] != want {
			t.Fatalf("%s = %U, want %U", name, r, want)
		}
	}
	extCases := map[string]rune{
		".go": '', ".py": '', ".js": '',
		".ts": '', ".json": '', ".md": '',
	}
	for ext, want := range extCases {
		g, ok := nerdFontIcons[ext]
		if !ok || g == "" {
			t.Fatalf("nerdFontIcons[%q] is empty -- expected literal codepoint %U", ext, want)
		}
		r := []rune(g)
		if len(r) != 1 || r[0] != want {
			t.Fatalf("nerdFontIcons[%q] = %U, want %U", ext, r, want)
		}
	}
}
