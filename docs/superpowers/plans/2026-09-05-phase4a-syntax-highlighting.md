# Cody Phase 4a: Tree-Sitter Syntax Highlighting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add real syntax highlighting for Go, Python, JavaScript, TypeScript, and Markdown, backed by `github.com/smacker/go-tree-sitter`, with debounced re-highlighting on edit and a graceful plain-text fallback for anything unsupported or that fails to parse.

**Architecture:** A new, fully decoupled package `internal/highlight` owns everything tree-sitter-shaped — parsing, queries, and capture-to-style mapping. It exposes only a `Highlighter` interface and a plain `[]Span{StartByte, EndByte int, Capture string}` return type; nothing tree-sitter-specific (not even the `sitter` import) leaks into `internal/editor`. `editor.Model` asks `internal/highlight` for spans on load and after a debounced edit, converts them from byte offsets to per-line rune columns, and renders them in `View()` — falling back to plain text when a line is selected (selection takes visual precedence) or when highlighting isn't available for that file.

**Tech Stack:** Go, `github.com/smacker/go-tree-sitter` (new — CGO), plus its `golang`/`python`/`javascript`/`typescript/typescript`/`markdown` sub-packages (new). `github.com/charmbracelet/bubbletea`/`lipgloss` (already dependencies).

**Spec:** `docs/superpowers/specs/2026-09-05-cody-tui-editor-design.md` (§2 tech stack, §6 "Tree-sitter highlighting", updated for the 4a/4b split and JSON deferral; §12 phased build order)

## Global Constraints

- Module `cody`, packages import as `cody/internal/...`.
- **Every fact below about the tree-sitter library's API was independently verified by actually building and running code against it during this plan's research phase — these are not assumptions from documentation.** Confirmed: `sitter.NewParser()`, `parser.SetLanguage(lang *sitter.Language)`, `parser.ParseCtx(ctx, oldTree, sourceBytes) (*sitter.Tree, error)`, `tree.RootNode() *sitter.Node`, `sitter.NewQuery(patternBytes []byte, lang *sitter.Language) (*sitter.Query, error)`, `sitter.NewQueryCursor() *sitter.QueryCursor`, `cursor.Exec(query, node)`, `cursor.NextMatch() (*sitter.QueryMatch, bool)`, `QueryMatch{ID uint32, PatternIndex uint16, Captures []QueryCapture}`, `QueryCapture{Index uint32, Node *sitter.Node}`, `query.CaptureNameForId(id uint32) string`, `Node.StartByte()`/`Node.EndByte() uint32`, `Node.Content(source []byte) string`, `Node.Type() string`. A real CGO build importing all five language sub-packages simultaneously succeeded on macOS/arm64 with no special flags.
- **JSON is out of scope for this plan** — `go-tree-sitter` has no ready-made JSON grammar; adding it later needs vendoring `tree-sitter-json`'s C sources (deferred to a future addition, see spec §2).
- **Markdown's parse API is genuinely different** from the other four languages: `markdown.ParseCtx(ctx, oldTree *markdown.MarkdownTree, content []byte) (*markdown.MarkdownTree, error)` returns a `MarkdownTree`, not a `sitter.Tree`, and exposes `(*MarkdownTree).Iter(f func(node *markdown.Node) bool)` to walk a combined block+inline node sequence, where `markdown.Node` is `struct { *sitter.Node; Inline *sitter.Node }`. This is absorbed entirely inside Markdown's own `Highlighter` implementation (Task 2) — no other code needs to know about it.
- **Markdown's highlighting scope is deliberately reduced** for this first pass: headings (`atx_heading` nodes) and fenced code blocks (`fenced_code_block` nodes) only — not inline emphasis/bold/links within paragraph text. Verified by an empirical parse dump that inline node types (emphasis, code spans, links) require walking each paragraph's separate inline subtree, which is meaningfully more work than headings/code-blocks for a feature the spec only broadly requires ("keyword, string, comment, etc." — never mentions markdown richness specifically). This is a documented scope trim, not an oversight.
- **A recurring Go pitfall in this codebase, worth restating for the third time:** a bare `return someFunc()` where `someFunc`'s return type differs from the enclosing function's declared return type (even when every value is individually assignable, e.g. a concrete type satisfying an interface the enclosing function declares) does not compile — Go's assignability rule only applies to explicit multi-value returns, not the single-call bare-forwarding form. Destructure into named variables first. This has bitten Phase 2 twice already; watch for it anywhere `newSitterHighlighter(...)` (returns `(*sitterHighlighter, error)`) is returned directly from `New(lang Language) (Highlighter, error)`.
- Highlight queries are intentionally minimal (keyword, string, comment, number, function-name captures only) — not full upstream `highlights.scm` files with dozens of capture categories. All four non-Markdown query strings in this plan were independently compiled against their real grammars and run against real source during this plan's research phase; their exact captured output is shown in each task so implementers can verify against the same expectations, not just "it compiles."
- When a line has an active text selection, selection highlighting (reverse-video, from Phase 2) takes visual precedence over syntax-highlight coloring for that line — no color/reverse-video composition is attempted.

---

### Task 1: `internal/highlight` core — Language detection, Span type, and the four tree-sitter-backed highlighters

**Files:**
- Create: `internal/highlight/language.go`
- Create: `internal/highlight/span.go`
- Create: `internal/highlight/highlighter.go`
- Test: `internal/highlight/language_test.go`
- Test: `internal/highlight/highlighter_test.go`

**Interfaces:**
- Produces (package `cody/internal/highlight`):
  - `type Language string` with constants `LanguageGo`, `LanguagePython`, `LanguageJavaScript`, `LanguageTypeScript`, `LanguageMarkdown`
  - `func LanguageForPath(path string) (Language, bool)`
  - `type Span struct { StartByte, EndByte int; Capture string }`
  - `type Highlighter interface { Highlight(source []byte) ([]Span, error) }`
  - `func New(lang Language) (Highlighter, error)` — Task 1 wires Go/Python/JavaScript/TypeScript; Task 2 adds the Markdown case to this same function.

- [ ] **Step 1: Fetch the new dependency**

```bash
go get github.com/smacker/go-tree-sitter
go get github.com/smacker/go-tree-sitter/golang
go get github.com/smacker/go-tree-sitter/python
go get github.com/smacker/go-tree-sitter/javascript
go get github.com/smacker/go-tree-sitter/typescript/typescript
```

- [ ] **Step 2: Write the failing tests**

Create `internal/highlight/language_test.go`:

```go
package highlight

import "testing"

func TestLanguageForPathKnownExtensions(t *testing.T) {
	cases := map[string]Language{
		"main.go":       LanguageGo,
		"script.py":     LanguagePython,
		"app.js":        LanguageJavaScript,
		"app.jsx":       LanguageJavaScript,
		"app.mjs":       LanguageJavaScript,
		"app.ts":        LanguageTypeScript,
		"app.tsx":       LanguageTypeScript,
		"README.md":     LanguageMarkdown,
	}
	for path, want := range cases {
		got, ok := LanguageForPath(path)
		if !ok || got != want {
			t.Fatalf("%s: got (%q, %v), want (%q, true)", path, got, ok, want)
		}
	}
}

func TestLanguageForPathUnknownExtension(t *testing.T) {
	if _, ok := LanguageForPath("data.bin"); ok {
		t.Fatal("expected an unsupported extension to return ok=false")
	}
}
```

Create `internal/highlight/highlighter_test.go`:

```go
package highlight

import "testing"

func captureNames(spans []Span, source []byte) map[string][]string {
	out := make(map[string][]string)
	for _, s := range spans {
		out[s.Capture] = append(out[s.Capture], string(source[s.StartByte:s.EndByte]))
	}
	return out
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestGoHighlighterCapturesKeywordsStringsCommentsNumbersFunctions(t *testing.T) {
	h, err := New(LanguageGo)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("package main\n\n// add two numbers\nfunc add(a int, b int) int {\n\treturn a + 42\n}\n\nvar greeting = \"hello\"\n")
	spans, err := h.Highlight(src)
	if err != nil {
		t.Fatal(err)
	}
	got := captureNames(spans, src)
	if !contains(got["keyword"], "func") || !contains(got["keyword"], "return") {
		t.Fatalf("got keywords %v", got["keyword"])
	}
	if !contains(got["function"], "add") {
		t.Fatalf("got functions %v", got["function"])
	}
	if !contains(got["comment"], "// add two numbers") {
		t.Fatalf("got comments %v", got["comment"])
	}
	if !contains(got["number"], "42") {
		t.Fatalf("got numbers %v", got["number"])
	}
	if !contains(got["string"], "\"hello\"") {
		t.Fatalf("got strings %v", got["string"])
	}
}

func TestPythonHighlighterCapturesBasics(t *testing.T) {
	h, err := New(LanguagePython)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("# a comment\ndef add(a, b):\n    return a + 42\ns = \"hi\"\n")
	spans, err := h.Highlight(src)
	if err != nil {
		t.Fatal(err)
	}
	got := captureNames(spans, src)
	if !contains(got["keyword"], "def") || !contains(got["keyword"], "return") {
		t.Fatalf("got keywords %v", got["keyword"])
	}
	if !contains(got["function"], "add") {
		t.Fatalf("got functions %v", got["function"])
	}
	if !contains(got["comment"], "# a comment") {
		t.Fatalf("got comments %v", got["comment"])
	}
	if !contains(got["number"], "42") {
		t.Fatalf("got numbers %v", got["number"])
	}
	if !contains(got["string"], "\"hi\"") {
		t.Fatalf("got strings %v", got["string"])
	}
}

func TestJavaScriptHighlighterCapturesBasics(t *testing.T) {
	h, err := New(LanguageJavaScript)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("// a comment\nfunction add(a, b) {\n  return a + 42;\n}\nconst s = \"hi\";\n")
	spans, err := h.Highlight(src)
	if err != nil {
		t.Fatal(err)
	}
	got := captureNames(spans, src)
	if !contains(got["keyword"], "function") || !contains(got["keyword"], "return") || !contains(got["keyword"], "const") {
		t.Fatalf("got keywords %v", got["keyword"])
	}
	if !contains(got["function"], "add") {
		t.Fatalf("got functions %v", got["function"])
	}
	if !contains(got["number"], "42") {
		t.Fatalf("got numbers %v", got["number"])
	}
	if !contains(got["string"], "\"hi\"") {
		t.Fatalf("got strings %v", got["string"])
	}
}

func TestTypeScriptHighlighterCapturesBasics(t *testing.T) {
	h, err := New(LanguageTypeScript)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("// a comment\nfunction add(a: number, b: number): number {\n  return a + 42;\n}\nconst s: string = \"hi\";\n")
	spans, err := h.Highlight(src)
	if err != nil {
		t.Fatal(err)
	}
	got := captureNames(spans, src)
	if !contains(got["keyword"], "function") || !contains(got["keyword"], "const") {
		t.Fatalf("got keywords %v", got["keyword"])
	}
	if !contains(got["function"], "add") {
		t.Fatalf("got functions %v", got["function"])
	}
	if !contains(got["string"], "\"hi\"") {
		t.Fatalf("got strings %v", got["string"])
	}
}

func TestNewReturnsErrorForUnknownLanguage(t *testing.T) {
	if _, err := New(Language("cobol")); err == nil {
		t.Fatal("expected an error for an unsupported language")
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/highlight/... -v`
Expected: FAIL to compile — none of `Language`, `LanguageForPath`, `Span`, `Highlighter`, `New` exist yet.

- [ ] **Step 4: Write the implementation**

Create `internal/highlight/language.go`:

```go
package highlight

import "path/filepath"

type Language string

const (
	LanguageGo         Language = "go"
	LanguagePython     Language = "python"
	LanguageJavaScript Language = "javascript"
	LanguageTypeScript Language = "typescript"
	LanguageMarkdown   Language = "markdown"
)

func LanguageForPath(path string) (Language, bool) {
	switch filepath.Ext(path) {
	case ".go":
		return LanguageGo, true
	case ".py":
		return LanguagePython, true
	case ".js", ".jsx", ".mjs":
		return LanguageJavaScript, true
	case ".ts", ".tsx":
		return LanguageTypeScript, true
	case ".md":
		return LanguageMarkdown, true
	}
	return "", false
}
```

Create `internal/highlight/span.go`:

```go
package highlight

type Span struct {
	StartByte int
	EndByte   int
	Capture   string
}

type Highlighter interface {
	Highlight(source []byte) ([]Span, error)
}
```

Create `internal/highlight/highlighter.go`:

```go
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
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/highlight/... -v`
Expected: PASS (all tests). If any query fails to compile (a `sitter.NewQuery` error), it will surface as a test failure with a clear "invalid node type" message naming the exact bad token — fix only that line, don't second-guess the rest of the query, since every line here was independently verified to compile and produce the exact captures shown in the tests above.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/highlight/language.go internal/highlight/span.go internal/highlight/highlighter.go internal/highlight/language_test.go internal/highlight/highlighter_test.go
git commit -m "feat: add tree-sitter highlighting for Go, Python, JavaScript, TypeScript"
```

---

### Task 2: Markdown highlighter and capture-to-style mapping

**Files:**
- Create: `internal/highlight/markdown.go`
- Create: `internal/highlight/style.go`
- Modify: `internal/highlight/highlighter.go` (add the `LanguageMarkdown` case to `New`)
- Test: `internal/highlight/markdown_test.go`
- Test: `internal/highlight/style_test.go`

**Interfaces:**
- Consumes: `Highlighter`, `Span` from Task 1.
- Produces (package `cody/internal/highlight`):
  - `func newMarkdownHighlighter() *markdownHighlighter` (unexported constructor; `New(LanguageMarkdown)` is the public entry point)
  - `func StyleFor(capture string) (lipgloss.Style, bool)`

- [ ] **Step 1: Fetch the dependency**

```bash
go get github.com/smacker/go-tree-sitter/markdown
```

- [ ] **Step 2: Write the failing tests**

Create `internal/highlight/markdown_test.go`:

```go
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
```

Create `internal/highlight/style_test.go`:

```go
package highlight

import "testing"

func TestStyleForKnownCapture(t *testing.T) {
	if _, ok := StyleFor("keyword"); !ok {
		t.Fatal("expected a style for \"keyword\"")
	}
}

func TestStyleForUnknownCapture(t *testing.T) {
	if _, ok := StyleFor("no-such-capture"); ok {
		t.Fatal("expected no style for an unknown capture name")
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/highlight/... -run 'TestMarkdown|TestStyleFor' -v`
Expected: FAIL — `New(LanguageMarkdown)` currently returns the "unsupported language" error from Task 1; `StyleFor` doesn't exist yet.

- [ ] **Step 4: Write the implementation**

Create `internal/highlight/markdown.go`:

```go
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
```

Create `internal/highlight/style.go`:

```go
package highlight

import "github.com/charmbracelet/lipgloss"

var styles = map[string]lipgloss.Style{
	"keyword":  lipgloss.NewStyle().Foreground(lipgloss.Color("212")),
	"string":   lipgloss.NewStyle().Foreground(lipgloss.Color("114")),
	"comment":  lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Italic(true),
	"number":   lipgloss.NewStyle().Foreground(lipgloss.Color("215")),
	"function": lipgloss.NewStyle().Foreground(lipgloss.Color("81")),
	"heading":  lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true),
	"code":     lipgloss.NewStyle().Foreground(lipgloss.Color("114")),
}

func StyleFor(capture string) (lipgloss.Style, bool) {
	s, ok := styles[capture]
	return s, ok
}
```

Modify `internal/highlight/highlighter.go` — add a `case LanguageMarkdown:` to `New`, right after the `LanguageTypeScript` case and before the final `return nil, fmt.Errorf(...)`:

```go
	case LanguageMarkdown:
		return newMarkdownHighlighter(), nil
```

(No destructuring needed here, unlike the other four cases — `newMarkdownHighlighter()` returns `*markdownHighlighter` directly with no error, and `New`'s bare `return newMarkdownHighlighter(), nil` is a plain two-expression return, not a bare-forwarded multi-value call, so it's unaffected by this plan's Go-pitfall note.)

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/highlight/... -v`
Expected: PASS — every test in the package, including all of Task 1's.

- [ ] **Step 6: Commit**

```bash
git add internal/highlight/markdown.go internal/highlight/style.go internal/highlight/highlighter.go internal/highlight/markdown_test.go internal/highlight/style_test.go go.mod go.sum
git commit -m "feat: add Markdown highlighting (headings, code blocks) and capture-to-style mapping"
```

---

### Task 3: Byte-offset to per-line rune-column conversion

**Files:**
- Create: `internal/highlight/linespans.go`
- Test: `internal/highlight/linespans_test.go`

**Interfaces:**
- Consumes: `Span` from Task 1.
- Produces (package `cody/internal/highlight`):
  - `type LineSpan struct { StartCol, EndCol int; Capture string }`
  - `func LineSpans(source []byte, spans []Span) map[int][]LineSpan` — converts flat byte-offset spans (which may cross line boundaries) into per-line-index, rune-column-based spans, clamped to each line's own bounds.

This is the correctness-critical piece: spans are byte offsets into UTF-8 source, but `editor.Model`'s cursor/selection logic is entirely rune-based (established in Phase 1/2). Converting incorrectly here would misalign highlighting for any line containing multi-byte characters.

- [ ] **Step 1: Write the failing tests**

Create `internal/highlight/linespans_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/highlight/... -run TestLineSpans -v`
Expected: FAIL to compile — `LineSpan`, `LineSpans` don't exist yet.

- [ ] **Step 3: Write the implementation**

Create `internal/highlight/linespans.go`:

```go
package highlight

import "unicode/utf8"

type LineSpan struct {
	StartCol int
	EndCol   int
	Capture  string
}

func LineSpans(source []byte, spans []Span) map[int][]LineSpan {
	if len(spans) == 0 {
		return map[int][]LineSpan{}
	}
	lineOffsets := lineOffsets(source)
	result := make(map[int][]LineSpan)
	for _, sp := range spans {
		startLine := lineForByte(lineOffsets, sp.StartByte)
		endLine := lineForByte(lineOffsets, sp.EndByte)
		for line := startLine; line <= endLine; line++ {
			lineStartByte := lineOffsets[line]
			lineEndByte := len(source)
			if line+1 < len(lineOffsets) {
				lineEndByte = lineOffsets[line+1] - 1 // exclude the trailing "\n"
			}
			spanStart := sp.StartByte
			if spanStart < lineStartByte {
				spanStart = lineStartByte
			}
			spanEnd := sp.EndByte
			if spanEnd > lineEndByte {
				spanEnd = lineEndByte
			}
			if spanStart >= spanEnd {
				continue
			}
			startCol := utf8.RuneCount(source[lineStartByte:spanStart])
			endCol := utf8.RuneCount(source[lineStartByte:spanEnd])
			result[line] = append(result[line], LineSpan{StartCol: startCol, EndCol: endCol, Capture: sp.Capture})
		}
	}
	return result
}

func lineOffsets(source []byte) []int {
	offsets := []int{0}
	for i, b := range source {
		if b == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}

func lineForByte(offsets []int, b int) int {
	line := 0
	for i, off := range offsets {
		if off <= b {
			line = i
		} else {
			break
		}
	}
	return line
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/highlight/... -v`
Expected: PASS — every test in the package.

- [ ] **Step 5: Commit**

```bash
git add internal/highlight/linespans.go internal/highlight/linespans_test.go
git commit -m "feat: add byte-offset-to-rune-column span conversion for per-line rendering"
```

---

### Task 4: Wire highlighting into the editor — load-time parse and styled rendering

**Files:**
- Modify: `internal/editor/model.go`
- Test: `internal/editor/model_test.go`

**Interfaces:**
- Consumes: `highlight.LanguageForPath`, `highlight.New`, `highlight.Highlighter`, `highlight.LineSpans`, `highlight.StyleFor` (Tasks 1-3).
- Produces: `editor.Model` gains `highlighter highlight.Highlighter` and `highlightSpans map[int][]highlight.LineSpan` fields; `LoadFile` selects and runs the highlighter; `View()` renders syntax-highlighted lines when there's no active selection on that line.

- [ ] **Step 1: Write the failing tests**

Add to `internal/editor/model_test.go`:

```go
func TestLoadFileHighlightsAGoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.highlightSpans) == 0 {
		t.Fatal("expected highlightSpans to be populated for a .go file")
	}
	view := m.View()
	if view == "" {
		t.Fatal("expected a non-empty rendered view")
	}
}

func TestLoadFileUnsupportedExtensionHasNoHighlighter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")
	if err := os.WriteFile(path, []byte("just bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.highlighter != nil {
		t.Fatal("expected no highlighter for an unsupported extension")
	}
	if view := m.View(); view == "" {
		t.Fatal("expected plain-text rendering to still work with no highlighter")
	}
}

func TestLoadFileResetsHighlightStateAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	goPath := filepath.Join(dir, "a.go")
	if err := os.WriteFile(goPath, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	txtPath := filepath.Join(dir, "b.bin")
	if err := os.WriteFile(txtPath, []byte("plain"), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(goPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.highlightSpans) == 0 {
		t.Fatal("setup failed: expected highlight spans after loading a .go file")
	}
	m, err = m.LoadFile(txtPath)
	if err != nil {
		t.Fatal(err)
	}
	if m.highlighter != nil || len(m.highlightSpans) != 0 {
		t.Fatal("expected highlighter and highlightSpans to be cleared after switching to an unsupported file")
	}
}
```

Add `"path/filepath"` to this test file's imports if not already present (check first — other tests in this file already use `filepath.Join`).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/editor/... -run 'TestLoadFileHighlights|TestLoadFileUnsupported|TestLoadFileResetsHighlight' -v`
Expected: FAIL to compile — `m.highlightSpans`/`m.highlighter` don't exist yet.

- [ ] **Step 3: Implement**

Modify `internal/editor/model.go`. Add `"sort"` and `"cody/internal/highlight"` to the imports:

```go
import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cody/internal/highlight"
)
```

Add fields to `Model`:

```go
type Model struct {
	buf            *Buffer
	cursorLine     int
	cursorCol      int
	width          int
	height         int
	selecting      bool
	selAnchorLine  int
	selAnchorCol   int
	clipboard      string
	undoStack      []undoSnapshot
	redoStack      []undoSnapshot
	highlighter    highlight.Highlighter
	highlightSpans map[int][]highlight.LineSpan
}
```

Replace `LoadFile` and add a `rehighlight` helper:

```go
func (m Model) LoadFile(path string) (Model, error) {
	buf, err := NewBuffer(path)
	if err != nil {
		return m, err
	}
	m.buf = buf
	m.cursorLine = 0
	m.cursorCol = 0
	m.selecting = false
	m.selAnchorLine = 0
	m.selAnchorCol = 0
	m.undoStack = nil
	m.redoStack = nil
	m.highlighter = nil
	m.highlightSpans = nil
	if lang, ok := highlight.LanguageForPath(path); ok {
		if h, err := highlight.New(lang); err == nil {
			m.highlighter = h
		}
	}
	m.rehighlight()
	return m, nil
}

func (m *Model) rehighlight() {
	if m.highlighter == nil || m.buf == nil {
		m.highlightSpans = nil
		return
	}
	source := []byte(strings.Join(m.buf.Lines, "\n"))
	spans, err := m.highlighter.Highlight(source)
	if err != nil {
		m.highlightSpans = nil
		return
	}
	m.highlightSpans = highlight.LineSpans(source, spans)
}
```

Replace `View()` to apply syntax highlighting when a line has no active selection:

```go
func (m Model) View() string {
	if m.buf == nil {
		return "Select a file to begin"
	}
	startLine, startCol, endLine, endCol, hasSel := m.selectionRange()
	var b strings.Builder
	for i, line := range m.buf.Lines {
		cursorMark := "  "
		if i == m.cursorLine {
			cursorMark = "> "
		}
		rendered := line
		if hasSel && i >= startLine && i <= endLine {
			rendered = highlightSelection(line, i, startLine, startCol, endLine, endCol)
		} else if spans, ok := m.highlightSpans[i]; ok {
			rendered = renderHighlightedLine(line, spans)
		}
		b.WriteString(fmt.Sprintf("%s%4d %s\n", cursorMark, i+1, rendered))
	}
	return b.String()
}

func renderHighlightedLine(line string, spans []highlight.LineSpan) string {
	runes := []rune(line)
	sorted := make([]highlight.LineSpan, len(spans))
	copy(sorted, spans)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].StartCol < sorted[j].StartCol })
	var b strings.Builder
	pos := 0
	for _, sp := range sorted {
		start, end := sp.StartCol, sp.EndCol
		if start < pos {
			continue
		}
		if start > len(runes) {
			break
		}
		if end > len(runes) {
			end = len(runes)
		}
		b.WriteString(string(runes[pos:start]))
		if style, ok := highlight.StyleFor(sp.Capture); ok {
			b.WriteString(style.Render(string(runes[start:end])))
		} else {
			b.WriteString(string(runes[start:end]))
		}
		pos = end
	}
	b.WriteString(string(runes[pos:]))
	return b.String()
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/editor/... -v`
Expected: PASS — every new test above, plus every existing test in the package (typing, selection, cut/copy/paste, undo/redo, save, space bar, alt-key guard, multi-rune-newline-burst, no-buffer command feedback).

Run: `go build ./...` and `go vet ./...`
Expected: both clean.

- [ ] **Step 5: Commit**

```bash
git add internal/editor/model.go internal/editor/model_test.go
git commit -m "feat: highlight files on load and render syntax colors in the editor view"
```

---

### Task 5: Debounced re-highlight after edits

**Files:**
- Modify: `internal/editor/model.go`
- Test: `internal/editor/model_test.go`

**Interfaces:**
- Produces: a `rehighlightMsg{generation int}` internal message type, `editor.Model` gains a `highlightGeneration int` field, `Update` is refactored to a top-level type switch (splitting key handling into a new `handleKey` method) so it can also handle `rehighlightMsg`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/editor/model_test.go`:

```go
func TestTypingSchedulesARehighlightCommand(t *testing.T) {
	m := setupEditor(t, "")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if cmd == nil {
		t.Fatal("expected typing to schedule a rehighlight command")
	}
	msg := cmd()
	if _, ok := msg.(rehighlightMsg); !ok {
		t.Fatalf("got %T, want rehighlightMsg", msg)
	}
}

func TestRehighlightMsgWithCurrentGenerationReparses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	if m.buf.Lines[0] != "Xpackage main" {
		t.Fatalf("setup failed, got %q", m.buf.Lines[0])
	}
	m, _ = m.Update(rehighlightMsg{generation: m.highlightGeneration})
	if len(m.highlightSpans) == 0 {
		t.Fatal("expected highlightSpans to be repopulated after a matching-generation rehighlightMsg")
	}
}

func TestStaleRehighlightMsgIsIgnored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	spansBeforeEdit := len(m.highlightSpans[0])
	if spansBeforeEdit == 0 {
		t.Fatal("setup failed: expected at least one highlight span on line 0 before the edit")
	}

	// Prepend "X" to the line, corrupting the "package" keyword token —
	// a genuine reparse of this new content would find zero keyword
	// matches on this line, letting us detect whether a stale message
	// incorrectly triggered one.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	staleGeneration := m.highlightGeneration - 1

	m, _ = m.Update(rehighlightMsg{generation: staleGeneration})

	if len(m.highlightSpans[0]) != spansBeforeEdit {
		t.Fatalf("got %d spans on line 0, want %d (unchanged from before the edit) — a stale-generation message must not trigger a reparse", len(m.highlightSpans[0]), spansBeforeEdit)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/editor/... -run 'TestTypingSchedules|TestRehighlightMsg|TestStaleRehighlight' -v`
Expected: FAIL to compile — `rehighlightMsg`, `m.highlightGeneration` don't exist yet.

- [ ] **Step 3: Implement**

Add `"time"` to `internal/editor/model.go`'s imports.

Add the field to `Model`:

```go
	highlightGeneration int
```

Add the message type and scheduling helper (near the top of the file, alongside `CommandExecutedMsg`):

```go
type rehighlightMsg struct {
	generation int
}

const highlightDebounce = 150 * time.Millisecond

func scheduleRehighlight(generation int) tea.Cmd {
	return tea.Tick(highlightDebounce, func(time.Time) tea.Msg {
		return rehighlightMsg{generation: generation}
	})
}
```

Replace `Update` with a top-level type switch that dispatches to a new `handleKey` method:

```go
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case rehighlightMsg:
		if m.buf != nil && msg.generation == m.highlightGeneration {
			m.rehighlight()
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(keyMsg tea.KeyMsg) (Model, tea.Cmd) {
	if m.buf == nil {
		switch keyMsg.String() {
		case "ctrl+s", "ctrl+x", "ctrl+c", "ctrl+v", "ctrl+z", "ctrl+y":
			return m, func() tea.Msg { return CommandExecutedMsg{Description: "No file open"} }
		}
		return m, nil
	}
	switch keyMsg.String() {
	case "up":
		m.selecting = false
		m.moveUp()
	case "down":
		m.selecting = false
		m.moveDown()
	case "left":
		m.selecting = false
		m.moveLeft()
	case "right":
		m.selecting = false
		m.moveRight()
	case "shift+up":
		m.extendSelection()
		m.moveUp()
	case "shift+down":
		m.extendSelection()
		m.moveDown()
	case "shift+left":
		m.extendSelection()
		m.moveLeft()
	case "shift+right":
		m.extendSelection()
		m.moveRight()
	case "enter":
		m.selecting = false
		m.pushUndo()
		m.buf.InsertNewline(m.cursorLine, m.cursorCol)
		m.cursorLine++
		m.cursorCol = 0
		m.highlightGeneration++
		return m, scheduleRehighlight(m.highlightGeneration)
	case "backspace":
		m.selecting = false
		m.pushUndo()
		m.cursorLine, m.cursorCol = m.buf.DeleteBefore(m.cursorLine, m.cursorCol)
		m.highlightGeneration++
		return m, scheduleRehighlight(m.highlightGeneration)
	case "ctrl+s":
		desc := m.save()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case " ":
		m.selecting = false
		m.pushUndo()
		m.buf.InsertRune(m.cursorLine, m.cursorCol, ' ')
		m.cursorCol++
		m.highlightGeneration++
		return m, scheduleRehighlight(m.highlightGeneration)
	case "ctrl+x":
		desc := m.Cut()
		m.highlightGeneration++
		return m, tea.Batch(
			func() tea.Msg { return CommandExecutedMsg{Description: desc} },
			scheduleRehighlight(m.highlightGeneration),
		)
	case "ctrl+c":
		desc := m.Copy()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+v":
		desc := m.Paste()
		m.highlightGeneration++
		return m, tea.Batch(
			func() tea.Msg { return CommandExecutedMsg{Description: desc} },
			scheduleRehighlight(m.highlightGeneration),
		)
	case "ctrl+z":
		desc := m.undo()
		m.highlightGeneration++
		return m, tea.Batch(
			func() tea.Msg { return CommandExecutedMsg{Description: desc} },
			scheduleRehighlight(m.highlightGeneration),
		)
	case "ctrl+y":
		desc := m.redo()
		m.highlightGeneration++
		return m, tea.Batch(
			func() tea.Msg { return CommandExecutedMsg{Description: desc} },
			scheduleRehighlight(m.highlightGeneration),
		)
	default:
		if keyMsg.Type == tea.KeyRunes && !keyMsg.Alt {
			m.selecting = false
			m.pushUndo()
			m.insertText(string(keyMsg.Runes))
			m.highlightGeneration++
			return m, scheduleRehighlight(m.highlightGeneration)
		}
	}
	return m, nil
}
```

(`ctrl+c`/Copy does not mutate the buffer, so it schedules no rehighlight — unchanged from before. Every other case's logic is byte-for-byte identical to what it replaces; only the rehighlight-scheduling additions and the `handleKey` extraction are new.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/editor/... -v`
Expected: PASS — every new test above, plus every existing test in the package (this is the largest single change to `Update` in the project's history — if anything regresses, it will show up here).

Run: `go build ./...` and `go vet ./...`
Expected: both clean.

- [ ] **Step 5: Commit**

```bash
git add internal/editor/model.go internal/editor/model_test.go
git commit -m "feat: debounce re-highlighting after edits using tea.Tick"
```

---

### Task 6: Final integration test and README update

**Files:**
- Test: `internal/app/model_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: everything from Tasks 1-5 — no new production code, this task only adds one end-to-end test and documentation.

- [ ] **Step 1: Write the integration test**

Add to `internal/app/model_test.go`:

```go
func TestOpeningAGoFileHighlightsItInTheComposedApp(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)

	if !m.editor.HasBuffer() {
		t.Fatal("expected the file to be loaded")
	}
	view := m.View()
	if !strings.Contains(view, "func") {
		t.Fatalf("expected the rendered view to contain the source text, got %q", view)
	}
}
```

Add `"strings"` to this test file's imports if not already present.

- [ ] **Step 2: Run the test, then the whole suite**

Run: `go test ./internal/app/... -run TestOpeningAGoFileHighlightsItInTheComposedApp -v`
Expected: PASS.

Run: `go test ./... -v`
Expected: PASS — every test in every package (`app`, `editor`, `filetree`, `highlight`, `statusbar`).

Run: `go build ./...`, `go vet ./...`, `gofmt -l .`
Expected: all clean, no output.

**Note:** `go build`/`go vet` will take noticeably longer than in prior phases the first time they run after this plan's `go get` steps, since CGO now has to compile the C source for five tree-sitter grammars. This is expected, not a hang.

- [ ] **Step 3: Update the README**

Modify `README.md`'s "Status" section:

```markdown
## Status

Phase 4a (tree-sitter syntax highlighting) of a phased build — see
`docs/superpowers/specs/2026-09-05-cody-tui-editor-design.md` for the full
design and `docs/superpowers/plans/` for phase-by-phase implementation plans.

This phase adds real syntax highlighting for Go, Python, JavaScript,
TypeScript, and Markdown (headings and code blocks), backed by
`github.com/smacker/go-tree-sitter`, re-parsed on load and ~150ms after
you stop typing. JSON highlighting is deferred (the underlying library has
no ready-made JSON grammar). It does not yet have: code folding (Phase
4b), an embedded shell, or in-buffer search — those land in later phases.

Building this phase requires a C compiler on your machine (CGO), since
tree-sitter's grammars are C libraries — this was already noted as a
tradeoff in the design doc's tech stack section.
```

- [ ] **Step 4: Commit**

```bash
git add internal/app/model_test.go README.md
git commit -m "test: add end-to-end syntax-highlighting integration test; docs: update README for phase 4a"
```
