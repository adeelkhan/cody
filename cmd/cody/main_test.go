package main

import (
	"testing"
)

func TestParseArgsDefaults(t *testing.T) {
	path, nerdFont, err := parseArgs([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "." {
		t.Errorf("path = %q, want %q", path, ".")
	}
	if !nerdFont {
		t.Error("nerdFont should default to true")
	}
}

func TestParseArgsNoNerdFont(t *testing.T) {
	path, nerdFont, err := parseArgs([]string{"--no-nerd-font", "/tmp"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nerdFont {
		t.Error("nerdFont should be false when --no-nerd-font is passed")
	}
	_ = path // path resolution tested in app package
}

func TestParseArgsNerdFontDefaultWithPath(t *testing.T) {
	_, nerdFont, err := parseArgs([]string{"/tmp"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !nerdFont {
		t.Error("nerdFont should default to true when no flag given")
	}
}
