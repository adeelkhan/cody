package highlight

import "testing"

func TestLineSpansSingleLineSpan(t *testing.T) {
	source := []byte("func add() {}\n")
	spans := []Span{{StartByte: 0, EndByte: 4, Capture: "keyword"}}
	got := LineSpans(source, spans)
	want := []LineSpan{{StartCol: 0, EndCol: 4, Capture: "keyword"}}
	if len(got[0]) != 1 || got[0][0] != want[0] {
		t.Fatalf("got %v, want %v", got[0], want)
	}
}

func TestLineSpansSecondLine(t *testing.T) {
	source := []byte("line0\nkeyword here\n")
	// "keyword" starts at byte 6 (after "line0\n"), ends at byte 13.
	spans := []Span{{StartByte: 6, EndByte: 13, Capture: "keyword"}}
	got := LineSpans(source, spans)
	if len(got[0]) != 0 {
		t.Fatalf("expected no spans on line 0, got %v", got[0])
	}
	want := LineSpan{StartCol: 0, EndCol: 7, Capture: "keyword"}
	if len(got[1]) != 1 || got[1][0] != want {
		t.Fatalf("got %v, want %v", got[1], want)
	}
}

func TestLineSpansMultiLineSpanSplitsAcrossLines(t *testing.T) {
	// A span covering bytes 0-11 of "hello\nworld\n" (the word "hello" on
	// line 0, a newline, and "world" on line 1) should split into one
	// LineSpan per line it touches, clamped to that line's own bounds.
	source := []byte("hello\nworld\n")
	spans := []Span{{StartByte: 0, EndByte: 11, Capture: "string"}}
	got := LineSpans(source, spans)
	if len(got[0]) != 1 || got[0][0].StartCol != 0 || got[0][0].EndCol != 5 {
		t.Fatalf("got line 0 spans %v", got[0])
	}
	if len(got[1]) != 1 || got[1][0].StartCol != 0 || got[1][0].EndCol != 5 {
		t.Fatalf("got line 1 spans %v", got[1])
	}
}

func TestLineSpansMultiByteRunesConvertToRuneColumnsNotByteOffsets(t *testing.T) {
	// "héllo" is 6 bytes (é is 2 bytes in UTF-8) but 5 runes. A span
	// covering the whole word (bytes 0-6) must report EndCol=5 (rune
	// count), not 6 (byte count).
	source := []byte("héllo world\n")
	spans := []Span{{StartByte: 0, EndByte: 6, Capture: "string"}}
	got := LineSpans(source, spans)
	want := LineSpan{StartCol: 0, EndCol: 5, Capture: "string"}
	if len(got[0]) != 1 || got[0][0] != want {
		t.Fatalf("got %v, want %v (rune columns, not byte offsets)", got[0], want)
	}
}

func TestLineSpansEmptyInput(t *testing.T) {
	got := LineSpans([]byte("hello\n"), nil)
	if len(got) != 0 {
		t.Fatalf("got %d entries, want 0 for no spans", len(got))
	}
}
