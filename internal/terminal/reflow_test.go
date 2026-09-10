// internal/terminal/reflow_test.go
package terminal

import "testing"

func TestIsRowFilledToEdgeAtExactWidth(t *testing.T) {
	if !isRowFilledToEdge("1234567890", 10) {
		t.Fatalf("got false, want true — row's rendered width exactly matches the terminal width")
	}
}

func TestIsRowFilledToEdgeWhenShorter(t *testing.T) {
	if isRowFilledToEdge("short", 10) {
		t.Fatalf("got true, want false — row's rendered width is less than the terminal width")
	}
}
