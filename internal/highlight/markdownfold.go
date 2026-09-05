package highlight

import (
	"context"

	"github.com/smacker/go-tree-sitter/markdown"
)

type markdownFolder struct{}

func newMarkdownFolder() *markdownFolder {
	return &markdownFolder{}
}

func (f *markdownFolder) Folds(source []byte) ([]Fold, error) {
	tree, err := markdown.ParseCtx(context.Background(), nil, source)
	if err != nil {
		return nil, err
	}
	var folds []Fold
	tree.Iter(func(n *markdown.Node) bool {
		switch n.Node.Type() {
		case "section", "fenced_code_block":
			folds = append(folds, Fold{
				StartByte: int(n.Node.StartByte()),
				EndByte:   int(n.Node.EndByte()),
			})
		}
		return true
	})
	return folds, nil
}
