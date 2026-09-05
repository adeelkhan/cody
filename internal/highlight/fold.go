package highlight

import (
	"context"
	"fmt"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

type Fold struct {
	StartByte int
	EndByte   int
}

type Folder interface {
	Folds(source []byte) ([]Fold, error)
}

var folderCache = make(map[Language]Folder)

func NewFolder(lang Language) (Folder, error) {
	if f, ok := folderCache[lang]; ok {
		return f, nil
	}
	var f Folder
	switch lang {
	case LanguageGo:
		f = newSitterFolder(golang.GetLanguage(), map[string]bool{"block": true})
	case LanguagePython:
		f = newSitterFolder(python.GetLanguage(), map[string]bool{"block": true})
	case LanguageJavaScript:
		f = newSitterFolder(javascript.GetLanguage(), map[string]bool{"statement_block": true})
	case LanguageTypeScript:
		f = newSitterFolder(typescript.GetLanguage(), map[string]bool{"statement_block": true})
	case LanguageMarkdown:
		f = newMarkdownFolder()
	default:
		return nil, fmt.Errorf("highlight: unsupported language %q", lang)
	}
	folderCache[lang] = f
	return f, nil
}

type sitterFolder struct {
	lang      *sitter.Language
	foldTypes map[string]bool
}

func newSitterFolder(lang *sitter.Language, foldTypes map[string]bool) *sitterFolder {
	return &sitterFolder{lang: lang, foldTypes: foldTypes}
}

func (f *sitterFolder) Folds(source []byte) ([]Fold, error) {
	parser := sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(f.lang)
	tree, err := parser.ParseCtx(context.Background(), nil, source)
	if err != nil {
		return nil, err
	}
	defer tree.Close()
	var folds []Fold
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if f.foldTypes[n.Type()] {
			folds = append(folds, Fold{StartByte: int(n.StartByte()), EndByte: int(n.EndByte())})
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(tree.RootNode())
	return folds, nil
}
