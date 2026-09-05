package highlight

import "testing"

// containsRange is shared by this file and markdownfold_test.go (both
// part of this same task, same package — a Go test helper defined in one
// _test.go file is visible to every other _test.go file in the package).
func containsRange(ranges []LineRange, start, end int) bool {
	for _, r := range ranges {
		if r.StartLine == start && r.EndLine == end {
			return true
		}
	}
	return false
}

func TestFoldLinesDropsSingleLineRanges(t *testing.T) {
	source := []byte("line0\nline1\nline2\n")
	folds := []Fold{{StartByte: 0, EndByte: 5}} // entirely within line 0
	ranges := FoldLines(source, folds)
	if len(ranges) != 0 {
		t.Fatalf("got %v, want no ranges for a single-line fold", ranges)
	}
}

func TestFoldLinesKeepsMultiLineRanges(t *testing.T) {
	source := []byte("line0\nline1\nline2\n")
	// Spans from inside line 0 through inside line 2.
	folds := []Fold{{StartByte: 2, EndByte: 14}}
	ranges := FoldLines(source, folds)
	if len(ranges) != 1 || ranges[0].StartLine != 0 || ranges[0].EndLine != 2 {
		t.Fatalf("got %v, want a single range {0, 2}", ranges)
	}
}

func TestFoldLinesEmptyInput(t *testing.T) {
	ranges := FoldLines([]byte("hello\n"), nil)
	if len(ranges) != 0 {
		t.Fatalf("got %d ranges, want 0 for no folds", len(ranges))
	}
}
