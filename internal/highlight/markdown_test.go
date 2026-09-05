package highlight

import "testing"

func TestMarkdownHighlighterCapturesHeadingsAndCodeBlocks(t *testing.T) {
	h, err := New(LanguageMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("# Title\n\nSome text.\n\n```go\nfmt.Println(1)\n```\n\n## Sub\n")
	spans, err := h.Highlight(src)
	if err != nil {
		t.Fatal(err)
	}
	got := captureNames(spans, src)
	if !contains(got["heading"], "# Title\n") {
		t.Fatalf("got headings %v", got["heading"])
	}
	if !contains(got["heading"], "## Sub\n") {
		t.Fatalf("got headings %v", got["heading"])
	}
	if !contains(got["code"], "```go\nfmt.Println(1)\n```\n") {
		t.Fatalf("got code %v", got["code"])
	}
}

func TestMarkdownHighlighterNoHeadingsOrCodeIsEmpty(t *testing.T) {
	h, err := New(LanguageMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	spans, err := h.Highlight([]byte("just plain text\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 0 {
		t.Fatalf("got %d spans, want 0", len(spans))
	}
}
