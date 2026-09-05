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

func New(lang Language) (Highlighter, error) {
	switch lang {
	case LanguageGo:
		h, err := newSitterHighlighter(golang.GetLanguage(), goQuery)
		if err != nil {
			return nil, err
		}
		return h, nil
	case LanguagePython:
		h, err := newSitterHighlighter(python.GetLanguage(), pythonQuery)
		if err != nil {
			return nil, err
		}
		return h, nil
	case LanguageJavaScript:
		h, err := newSitterHighlighter(javascript.GetLanguage(), javascriptQuery)
		if err != nil {
			return nil, err
		}
		return h, nil
	case LanguageTypeScript:
		h, err := newSitterHighlighter(typescript.GetLanguage(), typescriptQuery)
		if err != nil {
			return nil, err
		}
		return h, nil
	}
	return nil, fmt.Errorf("highlight: unsupported language %q", lang)
}

type sitterHighlighter struct {
	lang  *sitter.Language
	query *sitter.Query
}

func newSitterHighlighter(lang *sitter.Language, queryText string) (*sitterHighlighter, error) {
	q, err := sitter.NewQuery([]byte(queryText), lang)
	if err != nil {
		return nil, err
	}
	return &sitterHighlighter{lang: lang, query: q}, nil
}

func (h *sitterHighlighter) Highlight(source []byte) ([]Span, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(h.lang)
	tree, err := parser.ParseCtx(context.Background(), nil, source)
	if err != nil {
		return nil, err
	}
	cursor := sitter.NewQueryCursor()
	cursor.Exec(h.query, tree.RootNode())
	var spans []Span
	for {
		m, ok := cursor.NextMatch()
		if !ok {
			break
		}
		for _, c := range m.Captures {
			spans = append(spans, Span{
				StartByte: int(c.Node.StartByte()),
				EndByte:   int(c.Node.EndByte()),
				Capture:   h.query.CaptureNameForId(c.Index),
			})
		}
	}
	return spans, nil
}

const goQuery = `
(comment) @comment
(interpreted_string_literal) @string
(raw_string_literal) @string
(int_literal) @number
(float_literal) @number
(function_declaration name: (identifier) @function)
(method_declaration name: (field_identifier) @function)
"func" @keyword
"package" @keyword
"import" @keyword
"var" @keyword
"const" @keyword
"type" @keyword
"struct" @keyword
"interface" @keyword
"return" @keyword
"if" @keyword
"else" @keyword
"for" @keyword
"range" @keyword
"switch" @keyword
"case" @keyword
"default" @keyword
"break" @keyword
"continue" @keyword
"go" @keyword
"defer" @keyword
"select" @keyword
"chan" @keyword
"map" @keyword
`

const pythonQuery = `
(comment) @comment
(string) @string
(integer) @number
(float) @number
(function_definition name: (identifier) @function)
"def" @keyword
"class" @keyword
"return" @keyword
"if" @keyword
"elif" @keyword
"else" @keyword
"for" @keyword
"while" @keyword
"in" @keyword
"import" @keyword
"from" @keyword
"as" @keyword
"with" @keyword
"try" @keyword
"except" @keyword
"finally" @keyword
"raise" @keyword
"pass" @keyword
"break" @keyword
"continue" @keyword
"lambda" @keyword
(none) @keyword
(true) @keyword
(false) @keyword
"and" @keyword
"or" @keyword
"not" @keyword
`

const javascriptQuery = `
(comment) @comment
(string) @string
(template_string) @string
(number) @number
(function_declaration name: (identifier) @function)
(method_definition name: (property_identifier) @function)
"function" @keyword
"return" @keyword
"const" @keyword
"let" @keyword
"var" @keyword
"if" @keyword
"else" @keyword
"for" @keyword
"while" @keyword
"do" @keyword
"switch" @keyword
"case" @keyword
"default" @keyword
"break" @keyword
"continue" @keyword
"class" @keyword
"extends" @keyword
"new" @keyword
"try" @keyword
"catch" @keyword
"finally" @keyword
"throw" @keyword
"async" @keyword
"await" @keyword
"import" @keyword
"export" @keyword
"from" @keyword
"typeof" @keyword
"instanceof" @keyword
(null) @keyword
(undefined) @keyword
(true) @keyword
(false) @keyword
`

const typescriptQuery = `
(comment) @comment
(string) @string
(template_string) @string
(number) @number
(function_declaration name: (identifier) @function)
(method_definition name: (property_identifier) @function)
"function" @keyword
"return" @keyword
"const" @keyword
"let" @keyword
"var" @keyword
"if" @keyword
"else" @keyword
"for" @keyword
"while" @keyword
"do" @keyword
"switch" @keyword
"case" @keyword
"default" @keyword
"break" @keyword
"continue" @keyword
"class" @keyword
"extends" @keyword
"implements" @keyword
"new" @keyword
"try" @keyword
"catch" @keyword
"finally" @keyword
"throw" @keyword
"async" @keyword
"await" @keyword
"import" @keyword
"export" @keyword
"from" @keyword
"typeof" @keyword
"instanceof" @keyword
"interface" @keyword
"type" @keyword
"enum" @keyword
"public" @keyword
"private" @keyword
"protected" @keyword
"readonly" @keyword
(null) @keyword
(undefined) @keyword
(true) @keyword
(false) @keyword
`
