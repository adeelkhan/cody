package filetree

import "testing"

func TestIconForDir(t *testing.T) {
	n := &Node{Type: NodeDir}
	if IconFor(n, true) != nerdFontDir {
		t.Fatal("expected nerd font dir icon")
	}
	if IconFor(n, false) != fallbackDir {
		t.Fatal("expected fallback dir icon")
	}
}

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
