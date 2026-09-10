// internal/terminal/reflow_test.go
package terminal

import (
	"testing"
	"time"
)

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

func TestGroupIntoLogicalLinesJoinsAWrappedPair(t *testing.T) {
	lines, counts := groupIntoLogicalLines([]string{"1234567890", "abcde"}, 10)
	wantLines := []string{"1234567890abcde"}
	wantCounts := []int{2}
	if len(lines) != len(wantLines) || lines[0] != wantLines[0] {
		t.Fatalf("got lines=%q, want %q", lines, wantLines)
	}
	if len(counts) != len(wantCounts) || counts[0] != wantCounts[0] {
		t.Fatalf("got counts=%v, want %v", counts, wantCounts)
	}
}

func TestGroupIntoLogicalLinesKeepsShortRowsSeparate(t *testing.T) {
	lines, counts := groupIntoLogicalLines([]string{"short", "next"}, 10)
	wantLines := []string{"short", "next"}
	wantCounts := []int{1, 1}
	if len(lines) != 2 || lines[0] != wantLines[0] || lines[1] != wantLines[1] {
		t.Fatalf("got lines=%q, want %q", lines, wantLines)
	}
	if len(counts) != 2 || counts[0] != wantCounts[0] || counts[1] != wantCounts[1] {
		t.Fatalf("got counts=%v, want %v", counts, wantCounts)
	}
}

func TestGroupIntoLogicalLinesJoinsAThreeRowChain(t *testing.T) {
	lines, counts := groupIntoLogicalLines([]string{"1234567890", "1234567890", "end"}, 10)
	if len(lines) != 1 || lines[0] != "12345678901234567890end" {
		t.Fatalf("got lines=%q, want a single joined logical line", lines)
	}
	if len(counts) != 1 || counts[0] != 3 {
		t.Fatalf("got counts=%v, want [3]", counts)
	}
}

// TestGroupIntoLogicalLinesFalseJoinOnAFullWidthRow pins a KNOWN,
// documented limitation of the "filled to the edge" heuristic (see the
// design spec's §7/§8): a row that happens to fill the terminal's full
// width without actually being a soft-wrapped continuation — e.g. a
// box-drawing border the same width as the pane — gets incorrectly
// joined with the row below it. This is NOT fixed by this plan; the
// test exists so a future change can't silently make it worse without
// this test flagging it.
func TestGroupIntoLogicalLinesFalseJoinOnAFullWidthRow(t *testing.T) {
	lines, counts := groupIntoLogicalLines([]string{"1234567890", "unrelated!"}, 10)
	if len(lines) != 1 || lines[0] != "1234567890unrelated!" {
		t.Fatalf("got lines=%q, want the two unrelated full-width rows incorrectly joined (pinning the known heuristic limitation, not fixing it)", lines)
	}
	if len(counts) != 1 || counts[0] != 2 {
		t.Fatalf("got counts=%v, want [2]", counts)
	}
}

func TestRewrapLogicalLineNarrowsWithoutLoss(t *testing.T) {
	got := rewrapLogicalLine("ABCDEFGHIJKL", 6)
	want := []string{"ABCDEF", "GHIJKL"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewrapLogicalLineWidensBackToOneRow(t *testing.T) {
	got := rewrapLogicalLine("ABCDEF"+"GHIJKL", 40)
	want := []string{"ABCDEFGHIJKL"}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewrapLogicalLineEmptyProducesOneEmptyRow(t *testing.T) {
	got := rewrapLogicalLine("", 10)
	if len(got) != 1 || got[0] != "" {
		t.Fatalf("got %q, want one empty row", got)
	}
}

// TestRewrapLogicalLineNeverSplitsAWideCluster hand-traces
// ansi.Truncate's own documented behavior: a double-width grapheme
// cluster that would land exactly on the last column is dropped
// entirely (not split) and carried over whole to the next row. "AB你好"
// rewrapped at width 3: "AB" fits (width 2); adding "你" (width 2)
// would make width 4 > 3, so it's dropped from row 1 and carried whole
// into row 2, which in turn can't also fit "好" (2+2=4>3), carrying it
// into row 3.
func TestRewrapLogicalLineNeverSplitsAWideCluster(t *testing.T) {
	got := rewrapLogicalLine("AB你好", 3)
	want := []string{"AB", "你", "好"}
	if len(got) != len(want) {
		t.Fatalf("got %d rows %q, want %d rows %q", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got rows %q, want %q", got, want)
		}
	}
}

// TestRewrapLogicalLineAtWidthOneWithWideCharacterDoesNotHang covers a
// degenerate edge case: newWidth=1 can never fit ANY double-width
// cluster. rewrapLogicalLine must still terminate (dumping the
// remainder as one overflowing row) rather than loop forever.
func TestRewrapLogicalLineAtWidthOneWithWideCharacterDoesNotHang(t *testing.T) {
	done := make(chan []string, 1)
	go func() { done <- rewrapLogicalLine("你好", 1) }()
	select {
	case got := <-done:
		if len(got) == 0 {
			t.Fatalf("got no rows, want at least one (even if overflowing)")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rewrapLogicalLine hung — degenerate width guard is missing or broken")
	}
}
