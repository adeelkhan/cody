package highlight

import "testing"

func TestMarkdownFolderFindsNestedSections(t *testing.T) {
	f, err := NewFolder(LanguageMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("# Title\n\nSome text.\n\n## Sub\n\nMore text.\n")
	folds, err := f.Folds(src)
	if err != nil {
		t.Fatal(err)
	}
	ranges := FoldLines(src, folds)
	// Verified against the real grammar: the top-level section (starting
	// at "# Title") spans the entire document, lines 0-7, and it
	// contains a nested "## Sub" section spanning lines 4-7. Sections
	// nest — both are expected to survive as separate fold ranges.
	if !containsRange(ranges, 0, 7) {
		t.Fatalf("got %v, want a fold spanning lines 0-7 (the whole document under # Title)", ranges)
	}
	if !containsRange(ranges, 4, 7) {
		t.Fatalf("got %v, want a fold spanning lines 4-7 (the ## Sub section)", ranges)
	}
}

func TestMarkdownFolderFindsFencedCodeBlocks(t *testing.T) {
	f, err := NewFolder(LanguageMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("Some text.\n\n```go\nfmt.Println(1)\n```\n")
	folds, err := f.Folds(src)
	if err != nil {
		t.Fatal(err)
	}
	ranges := FoldLines(src, folds)
	// Verified against the real grammar: the fenced_code_block node's
	// EndByte lands exactly at len(source) (the closing "```" is the last
	// thing in the file, followed only by its own trailing newline). Per
	// the same lineOffsets/lineForByte convention that (correctly) puts
	// the whole-document section's end at line 7 in
	// TestMarkdownFolderFindsNestedSections rather than line 6, a byte
	// offset of exactly len(source) maps to the line index one past the
	// last real line — so this fold's endLine is 5, not 4.
	if !containsRange(ranges, 2, 5) {
		t.Fatalf("got %v, want a fold spanning lines 2-5 (the fenced code block)", ranges)
	}
}
