# Phase 6: In-Buffer Search Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `Ctrl+F` incremental find scoped to the current buffer, via a
mini dialog matching the existing file-open dialog's shape.

**Architecture:** The search logic (finding matches, tracking the current
one, jumping the cursor) lives in `internal/editor` as a small set of
`Model` methods and a pure `findMatches` helper, exactly mirroring how
selection and folding already live there. The dialog UI (a `textinput`,
its own `dialogKind`) lives in `internal/app`, mirroring the existing
file-open dialog's structure exactly. No new external dependencies.

**Tech Stack:** No new dependencies — `bubbles/textinput` (already used
for the file-open dialog and command palette) and this codebase's
existing rune-based column-indexing conventions.

**Spec:** `docs/superpowers/specs/2026-09-05-cody-tui-editor-design.md`
(§6 Search — updated ahead of this plan with the concrete UX decisions:
`Enter`/`Shift+Enter` instead of the original draft's `n`/`N`, since the
dialog's text input stays focused and typing a literal `n`/`N` would
otherwise just be inserted into the query; case-insensitive substring
matching; single-match, not all-matches, highlighting for v1).

## Global Constraints

- Matching is **case-insensitive substring search**, computed via a pure
  function operating on `[]string` lines — no regex, no fuzzy matching.
- Only the **current** match is highlighted, not every match in the
  buffer.
- The search dialog's text input **stays focused throughout** — `Enter`
  moves to the next match, `Shift+Enter` to the previous, both without
  closing the dialog (so a user can keep pressing `Enter` to cycle).
  `Esc` closes the dialog and clears all search state (matches, current
  index, highlight) — it does NOT move the cursor back to where the
  search started; the cursor stays at the last-found match.
- Every buffer-mutating operation (typing, enter, backspace, cut, paste,
  undo, redo) clears search state, exactly mirroring how those same
  operations already clear fold-toggle state — a stale match position
  after an edit is a bug, not a feature, matching this codebase's
  established "any edit invalidates X" pattern.
- `LoadFile` resets search state to empty, matching how it already resets
  highlight/fold state.
- `Ctrl+F` requires an open buffer — attempting it with none shows "No
  file open" (mirroring the no-buffer guard already used by
  Cut/Copy/Paste/Undo/Redo/Toggle-Fold), handled directly in the new
  `cmdFind` registry handler (not via `editor.Model`'s key-based no-buffer
  guard, since `cmdFind` never synthesizes a `tea.KeyMsg` into the editor
  — it directly opens the dialog, the same way `cmdOpenFilePrompt` does).
- `Ctrl+F` joins the existing list of shortcuts (`Ctrl+S/X/C/V/Z/Y/K`)
  that stop being globally intercepted while the **terminal pane** has
  focus (Phase 5's established rule) — it should pass through to the
  shell instead, for the same reason those already do.
- `Shift+Enter` has the same terminal-capability caveat this codebase
  already accepts for `Shift+Arrow` selection (README already documents
  that shift-modified arrow keys need a terminal that reports them) —
  document the same caveat for `Shift+Enter` rather than avoiding the
  binding.
- No changes to `Buffer`, `Cut`/`Copy`/`Paste`, undo/redo, or folding
  logic in this plan — search is purely additive.

---

### Task 1: Search core in `internal/editor`

**Files:**
- Modify: `internal/editor/model.go`
- Test: `internal/editor/model_test.go`

**Interfaces:**
- Produces: `Model.StartSearch() Model`, `Model.SetSearchQuery(query string) (Model, string)`, `Model.FindNext() (Model, string)`, `Model.FindPrev() (Model, string)`, `Model.ClearSearch() Model` — each status string is a short human-readable summary ("Match 2 of 5", "No matches", or `""` when the query is empty).
- Consumes: nothing from other tasks in this plan.

- [ ] **Step 1: Write the failing tests**

Add to `internal/editor/model_test.go`:

```go
func TestSetSearchQueryFindsAllCaseInsensitiveMatchesAndJumpsToNearest(t *testing.T) {
	m := setupEditor(t, "foo\nBAR foo\nfoo bar\n")
	m = m.StartSearch()
	m, status := m.SetSearchQuery("foo")
	if status != "Match 1 of 3" {
		t.Fatalf("got status %q, want %q", status, "Match 1 of 3")
	}
	line, col := m.Cursor()
	if line != 1 || col != 1 {
		t.Fatalf("got cursor line=%d col=%d, want 1,1 (first match)", line, col)
	}
}

func TestSetSearchQueryJumpsToNearestMatchAtOrAfterCursorNotAlwaysTheFirst(t *testing.T) {
	m := setupEditor(t, "foo\nfoo\nfoo\n")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // cursor now on line 2 (0-indexed)
	m = m.StartSearch()
	m, status := m.SetSearchQuery("foo")
	if status != "Match 3 of 3" {
		t.Fatalf("got status %q, want %q (the match on the cursor's own line)", status, "Match 3 of 3")
	}
	line, _ := m.Cursor()
	if line != 3 {
		t.Fatalf("got cursor line=%d, want 3 (1-indexed line 3 == 0-indexed line 2)", line)
	}
}

func TestSetSearchQueryWithNoMatchesReportsNoMatches(t *testing.T) {
	m := setupEditor(t, "hello world\n")
	m = m.StartSearch()
	m, status := m.SetSearchQuery("xyz")
	if status != "No matches" {
		t.Fatalf("got status %q, want %q", status, "No matches")
	}
}

func TestSetSearchQueryWithEmptyQueryReportsEmptyStatus(t *testing.T) {
	m := setupEditor(t, "hello world\n")
	m = m.StartSearch()
	m, status := m.SetSearchQuery("")
	if status != "" {
		t.Fatalf("got status %q, want empty", status)
	}
}

func TestFindNextAndFindPrevCycleThroughMatchesAndWrap(t *testing.T) {
	m := setupEditor(t, "foo\nfoo\nfoo\n")
	m = m.StartSearch()
	m, _ = m.SetSearchQuery("foo")

	m, status := m.FindNext()
	if status != "Match 2 of 3" {
		t.Fatalf("got status %q, want %q", status, "Match 2 of 3")
	}
	m, status = m.FindNext()
	if status != "Match 3 of 3" {
		t.Fatalf("got status %q, want %q", status, "Match 3 of 3")
	}
	m, status = m.FindNext()
	if status != "Match 1 of 3" {
		t.Fatalf("got status %q, want %q (wraps forward)", status, "Match 1 of 3")
	}
	m, status = m.FindPrev()
	if status != "Match 3 of 3" {
		t.Fatalf("got status %q, want %q (wraps backward)", status, "Match 3 of 3")
	}
}

func TestClearSearchRemovesAllMatchState(t *testing.T) {
	// Force a real color profile — the default test profile is Ascii,
	// which never emits ANSI codes at all, so checking their absence
	// would pass trivially regardless of whether ClearSearch did anything.
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := setupEditor(t, "foo\n")
	m = m.StartSearch()
	m, _ = m.SetSearchQuery("foo")
	if !strings.Contains(m.View(), "\x1b[7m") {
		t.Fatal("setup failed: expected a reverse-video match highlight before clearing")
	}

	m = m.ClearSearch()

	if strings.Contains(m.View(), "\x1b[7m") {
		t.Fatal("expected no reverse-video match highlight after ClearSearch")
	}
	if _, status := m.FindNext(); status != "No matches" {
		t.Fatalf("got status %q, want %q after clearing", status, "No matches")
	}
}

func TestCurrentMatchIsHighlightedInView(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := setupEditor(t, "hello world\n")
	m = m.StartSearch()
	m, _ = m.SetSearchQuery("world")

	view := m.View()
	if !strings.Contains(view, "\x1b[7m") {
		t.Fatalf("expected a reverse-video escape code highlighting the match, got %q", view)
	}
}

func TestTypingClearsSearchState(t *testing.T) {
	m := setupEditor(t, "foo\n")
	m = m.StartSearch()
	m, _ = m.SetSearchQuery("foo")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})

	if _, status := m.FindNext(); status != "No matches" {
		t.Fatalf("got status %q, want %q — typing must clear search state", status, "No matches")
	}
}

func TestLoadFileResetsSearchState(t *testing.T) {
	m := setupEditor(t, "foo\n")
	m = m.StartSearch()
	m, _ = m.SetSearchQuery("foo")

	dir := t.TempDir()
	path := filepath.Join(dir, "other.go")
	if err := os.WriteFile(path, []byte("bar\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, status := m.FindNext(); status != "No matches" {
		t.Fatalf("got status %q, want %q after LoadFile", status, "No matches")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (RED)**

Run: `go test ./internal/editor/... -run 'TestSetSearchQuery|TestFindNextAndFindPrev|TestClearSearch|TestCurrentMatchIsHighlighted|TestTypingClearsSearchState|TestLoadFileResetsSearchState' -v`
Expected: compile failure — `StartSearch`/`SetSearchQuery`/`FindNext`/`FindPrev`/`ClearSearch` are undefined.

- [ ] **Step 3: Add the `searchMatch` type, `findMatches`, and the five `Model` methods**

Add to `internal/editor/model.go`, near the other small helper types (e.g.
right after the `undoSnapshot` type):

```go
type searchMatch struct {
	line     int
	startCol int
	endCol   int
}

// findMatches returns every case-insensitive occurrence of query across
// lines, in line/column order. Columns are rune offsets, matching this
// package's existing convention (see highlightSelection).
func findMatches(lines []string, query string) []searchMatch {
	if query == "" {
		return nil
	}
	q := []rune(strings.ToLower(query))
	var matches []searchMatch
	for lineIdx, line := range lines {
		l := []rune(strings.ToLower(line))
		for start := 0; start+len(q) <= len(l); start++ {
			found := true
			for i, r := range q {
				if l[start+i] != r {
					found = false
					break
				}
			}
			if found {
				matches = append(matches, searchMatch{line: lineIdx, startCol: start, endCol: start + len(q)})
			}
		}
	}
	return matches
}
```

Add `searchMatches []searchMatch` and `searchIndex int` fields to the
`Model` struct (alongside the existing `folds`/`foldedStartLines`
fields).

In `New()`, initialize `searchIndex: -1`:

```go
func New() Model {
	return Model{searchIndex: -1}
}
```

In `LoadFile`, reset search state alongside the existing highlight/fold
resets (add these two lines near `m.folds = nil` / `m.foldedStartLines = nil`):

```go
	m.searchMatches = nil
	m.searchIndex = -1
```

Add the five search methods, near `toggleFold` (they follow the same
"pointer-receiver helper called from a value-receiver public method"
shape already used throughout this file):

```go
// StartSearch enters search mode: clears any previous match state so a
// fresh query starts from empty.
func (m Model) StartSearch() Model {
	m.searchMatches = nil
	m.searchIndex = -1
	return m
}

// SetSearchQuery recomputes matches for query and jumps to the nearest
// one at or after the current cursor position (wrapping to the first
// match if none qualify). Returns a short status string.
func (m Model) SetSearchQuery(query string) (Model, string) {
	if m.buf == nil {
		return m, "No file open"
	}
	m.searchMatches = findMatches(m.buf.Lines, query)
	if len(m.searchMatches) == 0 {
		m.searchIndex = -1
		if query == "" {
			return m, ""
		}
		return m, "No matches"
	}
	m.searchIndex = m.nearestMatchIndex()
	m.jumpToCurrentMatch()
	return m, m.matchStatus()
}

// FindNext moves to the next match, wrapping to the first.
func (m Model) FindNext() (Model, string) {
	if len(m.searchMatches) == 0 {
		return m, "No matches"
	}
	m.searchIndex = (m.searchIndex + 1) % len(m.searchMatches)
	m.jumpToCurrentMatch()
	return m, m.matchStatus()
}

// FindPrev moves to the previous match, wrapping to the last.
func (m Model) FindPrev() (Model, string) {
	if len(m.searchMatches) == 0 {
		return m, "No matches"
	}
	m.searchIndex = (m.searchIndex - 1 + len(m.searchMatches)) % len(m.searchMatches)
	m.jumpToCurrentMatch()
	return m, m.matchStatus()
}

// ClearSearch exits search mode: all match state and highlighting is
// removed. The cursor is left wherever the last jump put it.
func (m Model) ClearSearch() Model {
	m.searchMatches = nil
	m.searchIndex = -1
	return m
}

func (m Model) nearestMatchIndex() int {
	for i, match := range m.searchMatches {
		if match.line > m.cursorLine || (match.line == m.cursorLine && match.startCol >= m.cursorCol) {
			return i
		}
	}
	return 0
}

func (m *Model) jumpToCurrentMatch() {
	if m.searchIndex < 0 || m.searchIndex >= len(m.searchMatches) {
		return
	}
	match := m.searchMatches[m.searchIndex]
	m.cursorLine = match.line
	m.cursorCol = match.startCol
	m.ensureCursorVisible()
}

func (m Model) matchStatus() string {
	if len(m.searchMatches) == 0 {
		return "No matches"
	}
	return fmt.Sprintf("Match %d of %d", m.searchIndex+1, len(m.searchMatches))
}

func (m Model) currentMatchOnLine(line int) (searchMatch, bool) {
	if m.searchIndex < 0 || m.searchIndex >= len(m.searchMatches) {
		return searchMatch{}, false
	}
	match := m.searchMatches[m.searchIndex]
	if match.line != line {
		return searchMatch{}, false
	}
	return match, true
}
```

- [ ] **Step 4: Clear search state on every buffer-mutating operation**

In `handleKey`, add `m.searchMatches = nil` and `m.searchIndex = -1`
immediately after each existing `m.foldedStartLines = nil` /
`m.folds = nil` pair (the `enter`, `backspace`, `" "`, and default
typed-rune cases — 4 sites total), and after each existing
`m.rehighlight(); m.refold()` pair (the `ctrl+x`, `ctrl+v`, `ctrl+z`,
`ctrl+y` cases — 4 more sites), so search state never survives an edit.

- [ ] **Step 5: Highlight the current match in `View()`**

In `View()`, change the existing rendering branch:

```go
		rendered := line
		if hasSel && i >= startLine && i <= endLine {
			rendered = highlightSelection(line, i, startLine, startCol, endLine, endCol)
		} else if spans, ok := m.highlightSpans[i]; ok {
			rendered = renderHighlightedLine(line, spans)
		}
```

to:

```go
		rendered := line
		if hasSel && i >= startLine && i <= endLine {
			rendered = highlightSelection(line, i, startLine, startCol, endLine, endCol)
		} else if match, ok := m.currentMatchOnLine(i); ok {
			rendered = highlightMatch(line, match.startCol, match.endCol)
		} else if spans, ok := m.highlightSpans[i]; ok {
			rendered = renderHighlightedLine(line, spans)
		}
```

Add `highlightMatch`, near `highlightSelection`:

```go
func highlightMatch(line string, startCol, endCol int) string {
	runes := []rune(line)
	if startCol > len(runes) {
		startCol = len(runes)
	}
	if endCol > len(runes) {
		endCol = len(runes)
	}
	if startCol >= endCol {
		return line
	}
	style := lipgloss.NewStyle().Reverse(true).Background(lipgloss.Color("220")).TabWidth(lipgloss.NoTabConversion)
	return string(runes[:startCol]) + style.Render(string(runes[startCol:endCol])) + string(runes[endCol:])
}
```

- [ ] **Step 6: Run tests to verify they pass (GREEN)**

Run: `go test ./internal/editor/... -v`
Expected: PASS, every new test plus every pre-existing test in the
package (no regressions to selection/highlight/fold rendering, since the
new branch only activates when a current match exists on that line).

Also run `go build ./...`, `go vet ./...`, `gofmt -l .` for the whole
repo — expected: clean.

- [ ] **Step 7: Commit**

```bash
git add internal/editor/model.go internal/editor/model_test.go
git commit -m "feat: add case-insensitive incremental search core to editor.Model"
```

---

### Task 2: Wire the search dialog into `internal/app`

**Files:**
- Modify: `internal/app/dialog.go`
- Modify: `internal/app/model.go`
- Modify: `internal/app/commands.go`
- Test: `internal/app/dialog_test.go`

**Interfaces:**
- Consumes: `editor.Model.{StartSearch,SetSearchQuery,FindNext,FindPrev,ClearSearch}` from Task 1.
- Produces: `dialogSearch` (a new `dialogKind` value), `Model.searchInput textinput.Model` field, `cmdFind`.

- [ ] **Step 1: Write the failing tests**

`internal/app/dialog_test.go` currently imports `"os"`, `"path/filepath"`,
`"testing"`, and `tea "github.com/charmbracelet/bubbletea"` — add
`"strings"` and `"cody/internal/filetree"` to that import block (both are
used by the new tests below but not yet imported in this file). Then add:

```go
func TestCtrlFOpensSearchDialogOnlyWithABufferOpen(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatalf("got activeDialog=%v, want dialogNone (no file open)", m.activeDialog)
	}
	if cmd == nil {
		t.Fatal("expected a CommandExecutedMsg command reporting no file open")
	}
}

func TestCtrlFOpensSearchDialogWithABufferOpen(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("hello world\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = updated.(Model)
	if m.activeDialog != dialogSearch {
		t.Fatalf("got activeDialog=%v, want dialogSearch", m.activeDialog)
	}
}

func TestTypingInSearchDialogJumpsToMatchAndEscClosesAndClears(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("foo\nbar\nfoo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = updated.(Model)

	for _, r := range "foo" {
		updated, _ = m.updateSearchDialog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	if m.recentCommand != "Match 1 of 2" {
		t.Fatalf("got recentCommand=%q, want %q", m.recentCommand, "Match 1 of 2")
	}

	updated, _ = m.updateSearchDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.recentCommand != "Match 2 of 2" {
		t.Fatalf("got recentCommand=%q, want %q after Enter", m.recentCommand, "Match 2 of 2")
	}

	updated, _ = m.updateSearchDialog(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatalf("got activeDialog=%v, want dialogNone after Esc", m.activeDialog)
	}
	if strings.Contains(m.editor.View(), "\x1b[7m") {
		t.Fatal("expected Esc to clear the match highlight")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (RED)**

Run: `go test ./internal/app/... -run 'TestCtrlFOpensSearchDialog|TestTypingInSearchDialog' -v`
Expected: compile failure — `dialogSearch`, `cmdFind`, `updateSearchDialog` undefined.

- [ ] **Step 3: Add `dialogSearch`, `cmdFind`, `updateSearchDialog`, `renderSearchDialog` to `internal/app/dialog.go`**

Add `dialogSearch` to the `dialogKind` const block:

```go
const (
	dialogNone dialogKind = iota
	dialogFileOpen
	dialogAbout
	dialogPalette
	dialogSearch
)
```

Add:

```go
func cmdFind(m Model) (Model, tea.Cmd) {
	if !m.editor.HasBuffer() {
		return m, func() tea.Msg { return editor.CommandExecutedMsg{Description: "No file open"} }
	}
	ti := textinput.New()
	ti.Placeholder = "search"
	ti.Focus()
	m.searchInput = ti
	m.editor = m.editor.StartSearch()
	m.activeDialog = dialogSearch
	m.recentCommand = ""
	return m, textinput.Blink
}

func (m Model) updateSearchDialog(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}
	switch keyMsg.String() {
	case "esc":
		m.editor = m.editor.ClearSearch()
		m.activeDialog = dialogNone
		return m, nil
	case "enter":
		var status string
		m.editor, status = m.editor.FindNext()
		m.recentCommand = status
		return m, nil
	case "shift+enter":
		var status string
		m.editor, status = m.editor.FindPrev()
		m.recentCommand = status
		return m, nil
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	var status string
	m.editor, status = m.editor.SetSearchQuery(m.searchInput.Value())
	m.recentCommand = status
	return m, cmd
}

func renderSearchDialog(width, height int, ti textinput.Model, status string) string {
	content := "Find:\n\n" + ti.View()
	if status != "" {
		content += "\n\n" + status
	}
	return lipgloss.NewStyle().Width(width).Height(height).Padding(1, 2).Render(content)
}
```

Add the import for `"cody/internal/editor"` to this file's import block
if it isn't already there.

- [ ] **Step 4: Wire `dialogSearch` into `internal/app/model.go`**

Add `searchInput textinput.Model` to the `Model` struct, alongside
`fileOpenInput`.

In `Update`, add a branch for `dialogSearch` alongside the existing three
(`dialogFileOpen`/`dialogAbout`/`dialogPalette`):

```go
	if m.activeDialog == dialogSearch {
		return m.updateSearchDialog(msg)
	}
```

In `View`, add the matching render branch:

```go
	if m.activeDialog == dialogSearch {
		dialog := renderSearchDialog(m.width, paneHeight, m.searchInput, m.recentCommand)
		return lipgloss.JoinVertical(lipgloss.Left, menuBar, dialog, status)
	}
```

- [ ] **Step 5: Register `Ctrl+F` and add it to the terminal-passthrough exception list, in `internal/app/commands.go` and `internal/app/model.go`**

Add to `buildCommands()`'s slice literal:

```go
		{Name: "Find", Shortcut: "ctrl+f", Handler: cmdFind},
```

In `internal/app/model.go`'s `Update`, find the `tea.KeyMsg` case's
`default:` branch (the one guarding `commandForShortcut` with
`if m.focus != focusTerminal`). Confirm `"ctrl+f"` falls under the same
"only `ctrl+o`/`ctrl+q` stay global while the terminal has focus" rule
already established in Phase 5 — it should, since `ctrl+f` is not in
that allowlist, so no code change is needed here; just verify by reading
the current `default:` branch that `ctrl+f` genuinely reaches
`commandForShortcut` when focus is NOT `focusTerminal`, and genuinely
falls through to `m.terminal.Update(msg)` when focus IS `focusTerminal`
(matching `ctrl+s`/`ctrl+x`/etc.'s existing behavior) — if the branch is
structured any differently than described, adjust it so `ctrl+f`
consistently follows the same rule as those other shortcuts.

- [ ] **Step 6: Run tests to verify they pass (GREEN)**

Run: `go test ./internal/app/... -v`
Expected: PASS, every new test plus every pre-existing test (Phases 1-5's
tests, including the terminal-focus shortcut-passthrough tests, must be
unaffected).

Run `go build ./...`, `go vet ./...`, `gofmt -l .` for the whole repo —
expected: clean.

- [ ] **Step 7: Commit**

```bash
git add internal/app/dialog.go internal/app/model.go internal/app/commands.go internal/app/dialog_test.go
git commit -m "feat: wire ctrl+f incremental search dialog into the app"
```

---

### Task 3: Final integration test and README

**Files:**
- Test: `internal/app/model_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: everything from Tasks 1-2.

- [ ] **Step 1: Write the final integration test**

Add to `internal/app/model_test.go`:

```go
func TestSearchingAndCyclingMatchesThroughTheComposedApp(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc add(a, b int) int {\n\treturn a + b\n}\n\nfunc addTwo(x int) int {\n\treturn add(x, 2)\n}\n"
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

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = updated.(Model)
	if m.activeDialog != dialogSearch {
		t.Fatalf("got activeDialog=%v, want dialogSearch", m.activeDialog)
	}

	for _, r := range "add" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	line, _ := m.editor.Cursor()
	if line != 3 {
		t.Fatalf("got cursor line=%d, want 3 (the first \"add\" match, in \"func add(\")", line)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	line, _ = m.editor.Cursor()
	if line != 7 {
		t.Fatalf("got cursor line=%d, want 7 (the second \"add\" match, in \"func addTwo(\")", line)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatalf("got activeDialog=%v, want dialogNone", m.activeDialog)
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/app/... -run TestSearchingAndCyclingMatchesThroughTheComposedApp -v`
Expected: PASS.

Run `go test ./... -v`, `go build ./...`, `go vet ./...`, `gofmt -l .` for
the whole repo — expected: all clean.

- [ ] **Step 3: Update `README.md`**

Read the current `README.md` first (last updated for Phase 5). Update the
"Status" section to describe Phase 6, and add to the "Keybindings"
section:

```markdown
- `Ctrl+F` — open incremental find (case-insensitive, current buffer
  only); type to jump to the nearest match, `Enter`/`Shift+Enter` for
  next/previous match, `Esc` closes and clears the search (requires a
  terminal that reports `Shift+Enter` distinctly from plain `Enter` —
  the same class of terminal-capability caveat this project already
  documents for `Shift+Arrow` selection; most modern terminal emulators
  handle it, e.g. iTerm2, Alacritty, Kitty, WezTerm)
```

- [ ] **Step 4: Commit**

```bash
git add internal/app/model_test.go README.md
git commit -m "test: add end-to-end search integration test; docs: update README for phase 6"
```
