package statusbar

import (
	"strings"
	"testing"
)

func TestRenderIncludesAllSegments(t *testing.T) {
	out := Render(80, "myproject", "Saved main.go", "go", 3, 7)
	for _, want := range []string{"myproject", "Saved main.go", "Ln 3, Col 7", "go"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}
