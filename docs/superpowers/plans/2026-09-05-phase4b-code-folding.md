# Cody Phase 4b: Code Folding Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user fold (collapse) function/block bodies and Markdown sections/code blocks to a single summary line, toggled with `Ctrl+K`, for all five languages Phase 4a already highlights.

**Architecture:** A new `Folder` interface in `internal/highlight`, deliberately separate from Phase 4a's `Highlighter` interface (a real architecture fork decided before this phase started — see Global Constraints). Fold detection runs its own independent tree-sitter parse per language, using a per-language table of foldable node-type strings walked generically. `editor.Model` gets fold state (detected ranges + which ones the user has toggled closed) that's recomputed alongside highlighting on load and after every edit; folded lines are skipped by both rendering and cursor navigation.

**Tech Stack:** Go, `github.com/smacker/go-tree-sitter` (already a dependency from Phase 4a) — no new external dependencies.

**Spec:** `docs/superpowers/specs/2026-09-05-cody-tui-editor-design.md` (§6 "Folding", updated ahead of this plan with the architecture decisions below)

## Global Constraints

- **Folder is a separate capability from Highlighter, not a widened interface.** Phase 4a's final review recommended merging highlighting and folding into one parse for performance; this plan deliberately does NOT do that, favoring lower regression risk against Phase 4a's already-shipped, reviewed `Highlighter`/`Highlight`/`highlightSpans` code over the CPU cost of a second parse (which only matters on very large files — an already-documented, unrelated scaling limit). Every fact below about fold node types was independently verified by actually building and running code against the library during this plan's research phase.
- **Verified per-language foldable node types**, each confirmed by parsing real source and checking the exact resulting byte/line ranges:
  - Go and Python: `block` (a function's own body block starts at its first statement and ends at its last — e.g. for `def add(a, b):\n    if a > 0:\n        return a + b\n    return b\n`, the function's `block` spans lines 1-3, not just the `if`'s nested one-line block, which is correctly a separate, shorter `block`).
  - JavaScript and TypeScript: `statement_block`.
  - Markdown: `section` (a heading plus everything under it until the next heading of equal or higher level — sections nest, so an H1's section contains its H2 subsections) and `fenced_code_block` (reused from Phase 4a's highlighter scope).
- **Single-line "folds" are filtered out** — a block/section that starts and ends on the same line has nothing meaningful to collapse. `FoldLines` (Task 2) drops any range where the computed end line isn't strictly after the start line.
- **Nested folds are allowed and not specially resolved.** If two foldable ranges happen to start on the exact same line (an edge case: a single-line-declared nested block, e.g. `func f() { if x { ... } }` all on one source line), the toggle command matches whichever one appears first in the detected list — deterministic but arbitrary. This is an accepted v1 limitation, not something to add resolution logic for.
- **`Ctrl+K` is the toggle key**, not the spec's original illustrative `za` (a vim-style two-key chord this codebase has no precedent for, and no bare letter key can be repurposed since it's already text input). Registered through the command registry (`internal/app/commands.go`) like every other editor command, so it works regardless of focus and appears in the Phase 3 command palette automatically with zero palette-specific code.
- **Any buffer edit clears all fold-toggle state** (not just structural edits) — simpler and safer than remapping fold-line indices after lines shift. This falls out for free: fold detection and fold-toggle-state both reset in the same `refold()` function that already runs on every load/debounced-edit/synchronous-command cycle Phase 4a established for highlighting.
- Folding does not change `Buffer`'s actual content or `Cut`/`Copy`/`Paste`/`DeleteRange`/`TextRange` — it's purely a rendering + cursor-navigation layer on top of the existing line-based buffer. No changes to `internal/editor/buffer.go` in this entire plan.
- Toggling a fold does not push an undo snapshot or trigger a re-highlight/re-parse — it's a view-state change, not a content edit (matches how `Ctrl+C` Copy already works: no undo, no rehighlight).

---

### Task 1: `internal/highlight` — `Fold` type, `Folder` interface, and the four tree-sitter-backed folders

**Files:**
- Create: `internal/highlight/fold.go`
- Test: `internal/highlight/fold_test.go`

**Interfaces:**
- Produces (package `cody/internal/highlight`):
  - `type Fold struct { StartByte, EndByte int }`
  - `type Folder interface { Folds(source []byte) ([]Fold, error) }`
  - `func NewFolder(lang Language) (Folder, error)` — Task 1 wires Go/Python/JavaScript/TypeScript; Task 2 adds the Markdown case to this same function.

- [ ] **Step 1: Write the failing tests**

Create `internal/highlight/fold_test.go`:

```go
package highlight

import "testing"

// lineOfByteForTest is a small, self-contained line-counting helper local
// to this test file — deliberately NOT the real FoldLines/lineForByte
// production helpers (those don't exist until Task 2). This keeps Task 1
// fully independent and testable on its own, at the cost of duplicating
// a few lines of simple counting logic that Task 2 formalizes properly.
func lineOfByteForTest(source []byte, b int) int {
	line := 0
	for i := 0; i < b && i < len(source); i++ {
		if source[i] == '\n' {
			line++
		}
	}
	return line
}

func containsFoldLines(t *testing.T, folds []Fold, source []byte, startLine, endLine int) bool {
	t.Helper()
	for _, f := range folds {
		if lineOfByteForTest(source, f.StartByte) == startLine && lineOfByteForTest(source, f.EndByte) == endLine {
			return true
		}
	}
	return false
}

func TestGoFolderFindsFunctionBody(t *testing.T) {
	f, err := NewFolder(LanguageGo)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("package main\n\nfunc add(a int, b int) int {\n\treturn a + 42\n}\n")
	folds, err := f.Folds(src)
	if err != nil {
		t.Fatal(err)
	}
	// Lines (0-indexed): 0="package main", 1="", 2="func add(...) int {",
	// 3="\treturn a + 42", 4="}". The function body's block node spans
	// from the opening brace (end of line 2) through the closing brace
	// (line 4) — independently verified against the real grammar.
	if !containsFoldLines(t, folds, src, 2, 4) {
		t.Fatalf("got %v, want a fold spanning lines 2-4 (the function body)", folds)
	}
}

func TestPythonFolderFindsFunctionBodyNotTheNestedOneLiner(t *testing.T) {
	f, err := NewFolder(LanguagePython)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("def add(a, b):\n    if a > 0:\n        return a + b\n    return b\n")
	folds, err := f.Folds(src)
	if err != nil {
		t.Fatal(err)
	}
	// The function's own block starts at its first statement (the "if",
	// line 1) and ends at its last ("return b", line 3) — verified
	// against the real grammar. The nested "if" body ("return a + b") is
	// a separate, single-line block (StartByte and EndByte both on line
	// 2) — this test only checks the function-level fold is present;
	// Task 2's FoldLines is what formally drops single-line ranges like
	// that one from what the editor actually offers to fold.
	if !containsFoldLines(t, folds, src, 1, 3) {
		t.Fatalf("got %v, want a fold spanning lines 1-3 (the function body)", folds)
	}
}

func TestJavaScriptFolderFindsNestedBlocks(t *testing.T) {
	f, err := NewFolder(LanguageJavaScript)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("function add(a, b) {\n  if (a > 0) {\n    return a + b;\n  }\n  return b;\n}\n")
	folds, err := f.Folds(src)
	if err != nil {
		t.Fatal(err)
	}
	// Verified against the real grammar: the function's own
	// statement_block spans lines 0-5 (the whole function), and the
	// nested if's statement_block spans lines 1-3 — both multi-line, both
	// expected to survive as separate, nested folds.
	if !containsFoldLines(t, folds, src, 0, 5) {
		t.Fatalf("got %v, want a fold spanning lines 0-5 (the whole function)", folds)
	}
	if !containsFoldLines(t, folds, src, 1, 3) {
		t.Fatalf("got %v, want a fold spanning lines 1-3 (the nested if block)", folds)
	}
}

func TestTypeScriptFolderFindsFunctionBody(t *testing.T) {
	f, err := NewFolder(LanguageTypeScript)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("function add(a: number, b: number): number {\n  return a + b;\n}\n")
	folds, err := f.Folds(src)
	if err != nil {
		t.Fatal(err)
	}
	if !containsFoldLines(t, folds, src, 0, 2) {
		t.Fatalf("got %v, want a fold spanning lines 0-2 (the function body)", folds)
	}
}

func TestNewFolderReturnsErrorForUnknownLanguage(t *testing.T) {
	if _, err := NewFolder(Language("cobol")); err == nil {
		t.Fatal("expected an error for an unsupported language")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/highlight/... -run 'TestGoFolder|TestPythonFolder|TestJavaScriptFolder|TestTypeScriptFolder|TestNewFolderReturnsError' -v`
Expected: FAIL to compile — `Fold`, `Folder`, `NewFolder` don't exist yet.

- [ ] **Step 3: Write the implementation**

Create `internal/highlight/fold.go`:

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
```

Note this file does NOT yet handle `LanguageMarkdown` in `NewFolder` — that's Task 2, which adds one case to this same function, exactly like Phase 4a's `New` grew its `LanguageMarkdown` case in a second task.

Note also: `newSitterFolder(...)` returns a single value (`*sitterFolder`), never an error — so `f = newSitterFolder(...)` inside `NewFolder`'s switch is a plain assignment, not a function call whose result is bare-returned across a differing type. No instance of this plan's code hits the "bare `return someFunc()` across a type mismatch" pitfall that bit Phase 2 twice — verify this stays true as you implement; if you introduce any place where a called function's return type differs from its caller's declared return type and you're tempted to write `return theCall()` directly, destructure into named variables first instead.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/highlight/... -v`
Expected: PASS — all 5 tests above. This task is fully self-contained (its tests use a local `lineOfByteForTest` helper, not Task 2's `FoldLines`), so the package builds and every test goes green here, exactly like every other task in this project's plans.

Run: `go build ./...` and `go vet ./...`
Expected: both clean.

- [ ] **Step 5: Commit**

```bash
git add internal/highlight/fold.go internal/highlight/fold_test.go
git commit -m "feat: add tree-sitter fold detection for Go, Python, JavaScript, TypeScript"
```

---

### Task 2: Markdown folder and byte-range-to-line-range conversion

**Files:**
- Create: `internal/highlight/markdownfold.go`
- Modify: `internal/highlight/fold.go` (add the `LanguageMarkdown` case to `NewFolder`)
- Create: `internal/highlight/foldlines.go`
- Test: `internal/highlight/markdownfold_test.go`
- Test: `internal/highlight/foldlines_test.go`

**Interfaces:**
- Consumes: `Fold`, `Folder` from Task 1.
- Produces (package `cody/internal/highlight`):
  - `func newMarkdownFolder() *markdownFolder` (unexported constructor; `NewFolder(LanguageMarkdown)` is the public entry point)
  - `type LineRange struct { StartLine, EndLine int }`
  - `func FoldLines(source []byte, folds []Fold) []LineRange` — converts flat byte-offset folds into line-index ranges, reusing this package's existing `lineOffsets`/`lineForByte` helpers (from Phase 4a's `linespans.go`), and drops any range where the computed end line isn't strictly after the start line.

- [ ] **Step 1: Write the failing tests**

Create `internal/highlight/markdownfold_test.go`:

```go
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
	if !containsRange(ranges, 2, 4) {
		t.Fatalf("got %v, want a fold spanning lines 2-4 (the fenced code block)", ranges)
	}
}
```

Create `internal/highlight/foldlines_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/highlight/... -v`
Expected: FAIL to compile — `LineRange`, `FoldLines`, and the `LanguageMarkdown` case in `NewFolder` don't exist yet (this also resolves Task 1's tests, which were failing for the same missing-`FoldLines` reason).

- [ ] **Step 3: Write the implementation**

Create `internal/highlight/foldlines.go`:

```go
package highlight

type LineRange struct {
	StartLine int
	EndLine   int
}

func FoldLines(source []byte, folds []Fold) []LineRange {
	if len(folds) == 0 {
		return nil
	}
	offsets := lineOffsets(source)
	var ranges []LineRange
	for _, f := range folds {
		startLine := lineForByte(offsets, f.StartByte)
		endLine := lineForByte(offsets, f.EndByte)
		if endLine <= startLine {
			continue
		}
		ranges = append(ranges, LineRange{StartLine: startLine, EndLine: endLine})
	}
	return ranges
}
```

Create `internal/highlight/markdownfold.go`:

```go
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
```

Modify `internal/highlight/fold.go` — add a `case LanguageMarkdown:` to `NewFolder`, right after the `LanguageTypeScript` case and before the final `default: return nil, fmt.Errorf(...)`:

```go
	case LanguageMarkdown:
		f = newMarkdownFolder()
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/highlight/... -v`
Expected: PASS — every test in the package, including all of Task 1's tests (which needed `FoldLines` to be meaningful) and this task's own.

- [ ] **Step 5: Commit**

```bash
git add internal/highlight/foldlines.go internal/highlight/markdownfold.go internal/highlight/fold.go internal/highlight/foldlines_test.go internal/highlight/markdownfold_test.go
git commit -m "feat: add Markdown section/code-block folding and byte-to-line-range conversion"
```

---

### Task 3: Wire folding into the editor — detection, toggle, and fold-aware rendering/navigation

**Files:**
- Modify: `internal/editor/model.go`
- Test: `internal/editor/model_test.go`

**Interfaces:**
- Consumes: `highlight.Fold`, `highlight.Folder`, `highlight.LineRange`, `highlight.NewFolder`, `highlight.FoldLines` (Tasks 1-2).
- Produces: `editor.Model` gains `folder highlight.Folder`, `folds []highlight.LineRange`, `foldedStartLines map[int]bool` fields; a `Ctrl+K`-triggered toggle; fold-aware `View()` and cursor navigation.

- [ ] **Step 1: Write the failing tests**

Add to `internal/editor/model_test.go`:

```go
func TestLoadFileDetectsFoldsForAGoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc add(a int, b int) int {\n\treturn a + 42\n}\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range m.folds {
		if f.StartLine == 2 && f.EndLine == 4 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a fold spanning lines 2-4, got %v", m.folds)
	}
}

func TestCtrlKTogglesFoldAtCursor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc add(a int, b int) int {\n\treturn a + 42\n}\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.cursorLine != 2 {
		t.Fatalf("setup failed: expected cursor on line 2, got %d", m.cursorLine)
	}

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if cmd == nil {
		t.Fatal("expected ctrl+k to produce a status message")
	}
	if !m.foldedStartLines[2] {
		t.Fatal("expected line 2's fold to be collapsed after ctrl+k")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if m.foldedStartLines[2] {
		t.Fatal("expected a second ctrl+k to re-expand the fold")
	}
}

func TestCtrlKWithNothingFoldableAtCursorIsANoOp(t *testing.T) {
	m := setupEditor(t, "package main\n")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if cmd == nil {
		t.Fatal("expected a status message even when nothing is foldable")
	}
	msg := cmd().(CommandExecutedMsg)
	if msg.Description != "Nothing to fold here" {
		t.Fatalf("got %q", msg.Description)
	}
}

func TestFoldedRegionIsHiddenFromViewAndNavigation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc add(a int, b int) int {\n\treturn a + 42\n}\n\nvar x = 1\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if !m.foldedStartLines[2] {
		t.Fatal("setup failed: expected line 2's fold to be collapsed")
	}

	view := m.View()
	if strings.Contains(view, "return a + 42") {
		t.Fatal("expected the folded line's content to be hidden from View()")
	}

	// Moving down from the fold's start line must skip straight past the
	// hidden interior to the next visible line ("var x = 1", line 6),
	// not land on a hidden line.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursorLine != 6 {
		t.Fatalf("got cursorLine=%d, want 6 (skipping the folded lines 3-4)", m.cursorLine)
	}
}

func TestEditingClearsAllFoldState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc add(a int, b int) int {\n\treturn a + 42\n}\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if !m.foldedStartLines[2] {
		t.Fatal("setup failed: expected line 2's fold to be collapsed")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	if m.foldedStartLines[2] {
		t.Fatal("expected an edit to clear fold-toggle state")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/editor/... -run 'TestLoadFileDetectsFolds|TestCtrlK|TestFoldedRegion|TestEditingClearsAllFoldState' -v`
Expected: FAIL to compile — `m.folds`, `m.foldedStartLines` don't exist yet, `tea.KeyCtrlK` case isn't handled.

- [ ] **Step 3: Implement**

Modify `internal/editor/model.go`.

Add fields to `Model`:

```go
type Model struct {
	buf                 *Buffer
	cursorLine          int
	cursorCol           int
	width               int
	height              int
	selecting           bool
	selAnchorLine       int
	selAnchorCol        int
	clipboard           string
	undoStack           []undoSnapshot
	redoStack           []undoSnapshot
	highlighter         highlight.Highlighter
	highlightSpans      map[int][]highlight.LineSpan
	highlightGeneration int
	folder              highlight.Folder
	folds               []highlight.LineRange
	foldedStartLines    map[int]bool
}
```

Replace `LoadFile` and add a `refold` helper alongside the existing `rehighlight`:

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
	m.folder = nil
	m.folds = nil
	m.foldedStartLines = nil
	if lang, ok := highlight.LanguageForPath(path); ok {
		if h, err := highlight.New(lang); err == nil {
			m.highlighter = h
		}
		if f, err := highlight.NewFolder(lang); err == nil {
			m.folder = f
		}
	}
	m.rehighlight()
	m.refold()
	return m, nil
}

func (m *Model) refold() {
	m.foldedStartLines = nil
	if m.folder == nil || m.buf == nil {
		m.folds = nil
		return
	}
	source := []byte(strings.Join(m.buf.Lines, "\n"))
	folds, err := m.folder.Folds(source)
	if err != nil {
		m.folds = nil
		return
	}
	m.folds = highlight.FoldLines(source, folds)
}
```

Call `m.refold()` immediately after every existing `m.rehighlight()` call —
there are exactly 5 such call sites today: the `RehighlightMsg` case in
`Update`, and the `ctrl+x`/`ctrl+v`/`ctrl+z`/`ctrl+y` cases in `handleKey`.
Also add the `"ctrl+k"` case (toggle fold) and add `"ctrl+k"` to the
no-buffer-loaded guard. Rather than five separate small edits, here are
both functions in full, as they should look after this task — replace
`Update` and `handleKey` in their entirety with this:

```go
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case RehighlightMsg:
		if m.buf != nil && msg.generation == m.highlightGeneration {
			m.rehighlight()
			m.refold()
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
		case "ctrl+s", "ctrl+x", "ctrl+c", "ctrl+v", "ctrl+z", "ctrl+y", "ctrl+k":
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
		m.rehighlight()
		m.refold()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+c":
		desc := m.Copy()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+v":
		desc := m.Paste()
		m.highlightGeneration++
		m.rehighlight()
		m.refold()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+z":
		desc := m.undo()
		m.highlightGeneration++
		m.rehighlight()
		m.refold()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+y":
		desc := m.redo()
		m.highlightGeneration++
		m.rehighlight()
		m.refold()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+k":
		desc := m.toggleFold()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
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

Every case's logic is byte-for-byte identical to what it replaces except: the `RehighlightMsg` case and the `ctrl+x`/`ctrl+v`/`ctrl+z`/`ctrl+y` cases each gain one new `m.refold()` line, the no-buffer guard gains `"ctrl+k"` in its case list, and there's one entirely new `case "ctrl+k":` block. Nothing else changes.

Add the fold-query and toggle helpers:

```go
func (m Model) isLineHidden(line int) bool {
	for _, f := range m.folds {
		if m.foldedStartLines[f.StartLine] && line > f.StartLine && line <= f.EndLine {
			return true
		}
	}
	return false
}

func (m Model) foldAt(line int) (highlight.LineRange, bool) {
	for _, f := range m.folds {
		if f.StartLine == line {
			return f, true
		}
	}
	return highlight.LineRange{}, false
}

func (m *Model) toggleFold() string {
	f, ok := m.foldAt(m.cursorLine)
	if !ok {
		return "Nothing to fold here"
	}
	if m.foldedStartLines == nil {
		m.foldedStartLines = make(map[int]bool)
	}
	m.foldedStartLines[f.StartLine] = !m.foldedStartLines[f.StartLine]
	if m.foldedStartLines[f.StartLine] {
		return "Folded"
	}
	return "Unfolded"
}
```

Add `"ctrl+k"` to the no-buffer-loaded guard in `handleKey`:

```go
	if m.buf == nil {
		switch keyMsg.String() {
		case "ctrl+s", "ctrl+x", "ctrl+c", "ctrl+v", "ctrl+z", "ctrl+y", "ctrl+k":
			return m, func() tea.Msg { return CommandExecutedMsg{Description: "No file open"} }
		}
		return m, nil
	}
```

Add a `case "ctrl+k":` to `handleKey`'s main switch (placement doesn't matter relative to the other cases; put it near `ctrl+c` since, like Copy, it doesn't mutate the buffer and needs no undo/rehighlight):

```go
	case "ctrl+k":
		desc := m.toggleFold()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
```

Replace `moveUp`/`moveDown` to skip hidden lines:

```go
func (m *Model) moveUp() {
	next := m.cursorLine - 1
	for next >= 0 && m.isLineHidden(next) {
		next--
	}
	if next >= 0 {
		m.cursorLine = next
		m.clampCol()
	}
}

func (m *Model) moveDown() {
	next := m.cursorLine + 1
	for next < len(m.buf.Lines) && m.isLineHidden(next) {
		next++
	}
	if next < len(m.buf.Lines) {
		m.cursorLine = next
		m.clampCol()
	}
}
```

Modify `View()` to skip hidden lines and mark folded lines:

```go
func (m Model) View() string {
	if m.buf == nil {
		return "Select a file to begin"
	}
	startLine, startCol, endLine, endCol, hasSel := m.selectionRange()
	var b strings.Builder
	for i, line := range m.buf.Lines {
		if m.isLineHidden(i) {
			continue
		}
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
		if m.foldedStartLines[i] {
			rendered += " ⋯"
		}
		b.WriteString(fmt.Sprintf("%s%4d %s\n", cursorMark, i+1, rendered))
	}
	return b.String()
}
```

(`m.foldedStartLines[i]` on a `nil` map safely reads as `false` — no nil-check needed, matching Go's normal zero-value map-read semantics already relied on elsewhere in this file.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/editor/... -v`
Expected: PASS — every new test above, plus every existing test in the package (typing, selection, cut/copy/paste, undo/redo, save, space bar, alt-key guard, multi-rune-newline-burst, no-buffer command feedback, highlighting, debounced rehighlight).

Run: `go build ./...` and `go vet ./...`
Expected: both clean.

- [ ] **Step 5: Commit**

```bash
git add internal/editor/model.go internal/editor/model_test.go
git commit -m "feat: detect fold ranges on load/edit, add ctrl+k toggle, hide folded lines from view and navigation"
```

---

### Task 4: Wire `Ctrl+K` into the command registry, final integration test, and README update

**Files:**
- Modify: `internal/app/commands.go`
- Test: `internal/app/model_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: everything from Tasks 1-3.
- Produces: `buildCommands()` gains a `"Toggle Fold"` entry (shortcut `ctrl+k`), delegating to the editor exactly like `cmdSave`/`cmdCut`/etc. already do — this is what makes `Ctrl+K` work regardless of which pane has focus, the same property established for every other editor command since Phase 2.

- [ ] **Step 1: Write the failing test**

Add to `internal/app/commands_test.go` (check its existing imports first — it should already have `os`, `tea`, `cody/internal/editor`, `cody/internal/filetree` from Phase 2):

```go
func TestToggleFoldCommandDelegatesToEditorRegardlessOfFocus(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	src := "package main\n\nfunc add(a int, b int) int {\n\treturn a + 42\n}\n"
	if err := os.WriteFile(file, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)
	for i := 0; i < 2; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // back to the tree
	m = updated.(Model)
	if m.focus != focusTree {
		t.Fatal("expected focus on tree")
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected ctrl+k to produce a command even with the tree focused")
	}
	msg, ok := cmd().(editor.CommandExecutedMsg)
	if !ok || msg.Description != "Folded" {
		t.Fatalf("got %+v, ok=%v", msg, ok)
	}
}
```

Add `"path/filepath"` to this test file's imports if not already present.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/app/... -run TestToggleFoldCommandDelegatesToEditorRegardlessOfFocus -v`
Expected: FAIL — no `"ctrl+k"` entry exists in `buildCommands()` yet.

- [ ] **Step 3: Implement**

Add to `buildCommands()` in `internal/app/commands.go` (append to the existing slice literal):

```go
		{Name: "Toggle Fold", Shortcut: "ctrl+k", Handler: cmdToggleFold},
```

Add the handler alongside the other `cmd*` functions:

```go
func cmdToggleFold(m Model) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	return m, cmd
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/app/... -v`
Expected: PASS — all tests in the package, including every existing Phase 2/3/4a test.

- [ ] **Step 5: Write the final integration test**

Add to `internal/app/model_test.go`:

```go
func TestFoldingAGoFunctionThroughTheComposedApp(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc add(a int, b int) int {\n\treturn a + 42\n}\n\nvar x = 1\n"
	if err := os.WriteFile(file, []byte(src), 0644); err != nil {
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

	for i := 0; i < 2; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m = updated.(Model)

	view := m.View()
	if strings.Contains(view, "return a + 42") {
		t.Fatal("expected the folded function body to be hidden from the rendered view")
	}
	if !strings.Contains(view, "var x = 1") {
		t.Fatal("expected content after the fold to still render")
	}
}
```

- [ ] **Step 6: Run the test, then the whole suite**

Run: `go test ./internal/app/... -run TestFoldingAGoFunctionThroughTheComposedApp -v`
Expected: PASS.

Run: `go test ./... -v`
Expected: PASS — every test in every package (`app`, `editor`, `filetree`, `highlight`, `statusbar`).

Run: `go build ./...`, `go vet ./...`, `gofmt -l .`
Expected: all clean, no output.

- [ ] **Step 7: Update the README**

Modify `README.md`'s "Status" section:

```markdown
## Status

Phase 4b (code folding) of a phased build — see
`docs/superpowers/specs/2026-09-05-cody-tui-editor-design.md` for the full
design and `docs/superpowers/plans/` for phase-by-phase implementation plans.

This phase adds folding for function/block bodies (Go, Python, JavaScript,
TypeScript) and Markdown sections/code blocks — `Ctrl+K` toggles the fold
at the cursor's line, collapsing it to a single summary line. Editing the
file clears all fold state. It does not yet have: an embedded shell or
in-buffer search — those land in later phases.
```

Add a keybinding line to the "Keybindings" section, alongside the existing `Ctrl+Z`/`Ctrl+Y` line:

```markdown
- `Ctrl+K` — toggle the code fold at the cursor's current line (function/
  block bodies, or Markdown sections/code blocks)
```

- [ ] **Step 8: Commit**

```bash
git add internal/app/commands.go internal/app/commands_test.go internal/app/model_test.go README.md
git commit -m "feat: wire ctrl+k fold toggle into the command registry; test: add end-to-end folding integration test; docs: update README for phase 4b"
```
