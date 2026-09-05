package highlight

import (
	"context"

	"github.com/smacker/go-tree-sitter/markdown"
)

type markdownHighlighter struct{}

func newMarkdownHighlighter() *markdownHighlighter {
	return &markdownHighlighter{}
}

func (h *markdownHighlighter) Highlight(source []byte) ([]Span, error) {
	tree, err := markdown.ParseCtx(context.Background(), nil, source)
	if err != nil {
		return nil, err
	}
	var spans []Span
	tree.Iter(func(n *markdown.Node) bool {
		switch n.Node.Type() {
		case "atx_heading":
			spans = append(spans, Span{
				StartByte: int(n.Node.StartByte()),
				EndByte:   int(n.Node.EndByte()),
				Capture:   "heading",
			})
		case "fenced_code_block":
			spans = append(spans, Span{
				StartByte: int(n.Node.StartByte()),
				EndByte:   int(n.Node.EndByte()),
				Capture:   "code",
			})
		}
		return true
	})
	return spans, nil
}
