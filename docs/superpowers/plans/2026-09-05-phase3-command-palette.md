# Cody Phase 3: Command Palette Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the "Commands" menu label open a real command palette — a modal listing every registry entry with its shortcut, filterable by typed text, navigable with arrow keys, executed with Enter — replacing Phase 2's "coming in a later phase" stub, per the design spec's phase-3 scope.

**Architecture:** The palette is a fourth `dialogKind` (alongside `dialogFileOpen`/`dialogAbout` from Phase 2), following the exact same modal pattern already established: a top-of-`Update` guard routes all messages to it while active, and `View()` replaces the body with it. It reads `m.commands` — the same registry Phase 2 built — through one new pure filtering function; no new registry entries are needed (there's no "open the palette" keybinding, only a mouse click on "Commands", matching how "About" already works).

**Tech Stack:** Go, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/bubbles/textinput` — all already dependencies, no new ones.

**Spec:** `docs/superpowers/specs/2026-09-05-cody-tui-editor-design.md` (section 8's "Commands" bullet is what this phase implements)

## Global Constraints

- Module `cody`, packages import as `cody/internal/...` (unchanged).
- The palette is modal, following Phase 2's established pattern exactly: a guard at the very top of `Update` (before the `switch msg := msg.(type)` opens) intercepts every message type while `m.activeDialog == dialogPalette`; `handleClick`'s existing `if m.activeDialog != dialogNone { return m, nil }` guard already covers this dialog kind with no changes needed there.
- Filtering is case-insensitive substring match on `Command.Name` (spec: "narrow by name"). Filtering resets the selected cursor to index 0, since the previously-selected row may no longer be in the filtered list.
- **This phase changes existing behavior**, not just adds new code: clicking "Commands" currently (Phase 2) sets a stub status message and does nothing else. This phase must REPLACE that with actually opening the palette. The existing test `TestClickCommandsLabelIsANoOpStub` in `internal/app/menu_test.go` asserts the old stub behavior and will fail once this phase lands — it must be replaced (not left broken, not just deleted) with a test asserting the new behavior.
- **A known Go pitfall from Phase 2, worth restating so it isn't repeated:** a bare `return someFunc()` where `someFunc` returns `(Model, tea.Cmd)` does NOT compile inside a function declared to return `(tea.Model, tea.Cmd)`, even though `Model` satisfies the `tea.Model` interface — Go's assignability rule doesn't apply to the single-call bare-forwarding return form, only to explicit multi-value returns. Destructure into named variables first (`updated, cmd := someFunc(); return updated, cmd`), which does compile. This exact mistake was made and caught in Phase 2's Task 4; watch for it anywhere a `Command.Handler(m)` call's result is returned directly from a function whose own signature is `(tea.Model, tea.Cmd)` (e.g. `Update`, `updateFileOpenDialog`, and this phase's new `updatePaletteDialog`).
- No new files needed beyond one: `internal/app/palette.go` (+ its test). `internal/editor` is untouched by this entire phase.

---

### Task 1: Pure command filtering

**Files:**
- Create: `internal/app/palette.go`
- Test: `internal/app/palette_test.go`

**Interfaces:**
- Produces: `func filteredCommands(commands []Command, query string) []Command` in package `cody/internal/app` — case-insensitive substring match against `Command.Name`; an empty `query` returns all commands unchanged (same order).

- [ ] **Step 1: Write the failing tests**

Create `internal/app/palette_test.go`:

```go
package app

import "testing"

func TestFilteredCommandsEmptyQueryReturnsAll(t *testing.T) {
	commands := buildCommands()
	got := filteredCommands(commands, "")
	if len(got) != len(commands) {
		t.Fatalf("got %d, want %d", len(got), len(commands))
	}
}

func TestFilteredCommandsCaseInsensitiveSubstring(t *testing.T) {
	commands := []Command{{Name: "Save"}, {Name: "Save As"}, {Name: "Undo"}}
	got := filteredCommands(commands, "SAV")
	if len(got) != 2 || got[0].Name != "Save" || got[1].Name != "Save As" {
		t.Fatalf("got %v", got)
	}
}

func TestFilteredCommandsNoMatchReturnsEmpty(t *testing.T) {
	commands := buildCommands()
	got := filteredCommands(commands, "zzz-no-such-command")
	if len(got) != 0 {
		t.Fatalf("got %d commands, want 0", len(got))
	}
}

func TestFilteredCommandsPreservesOrder(t *testing.T) {
	commands := buildCommands()
	got := filteredCommands(commands, "")
	for i := range commands {
		if got[i].Name != commands[i].Name {
			t.Fatalf("got order %v, want the same order as buildCommands()", got)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/... -run TestFilteredCommands -v`
Expected: FAIL to compile — `filteredCommands` doesn't exist yet.

- [ ] **Step 3: Write the implementation**

Create `internal/app/palette.go`:

```go
package app

import "strings"

func filteredCommands(commands []Command, query string) []Command {
	if query == "" {
		return commands
	}
	q := strings.ToLower(query)
	var out []Command
	for _, c := range commands {
		if strings.Contains(strings.ToLower(c.Name), q) {
			out = append(out, c)
		}
	}
	return out
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/app/... -run TestFilteredCommands -v`
Expected: PASS (all 4 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/app/palette.go internal/app/palette_test.go
git commit -m "feat: add case-insensitive command filtering for the palette"
```

---

### Task 2: Wire the command palette dialog

**Files:**
- Modify: `internal/app/dialog.go` (add `dialogPalette` to the `dialogKind` enum — one line)
- Modify: `internal/app/palette.go` (add the dialog's open/update/render logic)
- Modify: `internal/app/model.go` (new `Model` fields, `Update`/`View` wiring, `clickMenuLabel`'s `"Commands"` case)
- Test: `internal/app/palette_test.go` (append)
- Modify: `internal/app/menu_test.go` (replace the now-obsolete stub test)

**Interfaces:**
- Consumes: `Command`, `m.commands`, `filteredCommands` (Task 1); `dialogKind`/`dialogNone`/`dialogFileOpen`/`dialogAbout` (Phase 2); `textinput.Model` (already a dependency, used the same way as `m.fileOpenInput`).
- Produces (package `cody/internal/app`):
  - `dialogPalette` added as a fourth `dialogKind` value.
  - `Model` gains `paletteFilter textinput.Model` and `paletteCursor int`.
  - `func openPalette(m Model) (Model, tea.Cmd)` — constructs a fresh `textinput.Model`, focuses it, resets `paletteCursor` to 0, sets `m.activeDialog = dialogPalette`.
  - `func (m Model) updatePaletteDialog(msg tea.Msg) (tea.Model, tea.Cmd)`
  - `func renderPaletteDialog(width, height int, filterInput textinput.Model, matches []Command, cursor int) string`

- [ ] **Step 1: Write the failing tests**

Add to `internal/app/palette_test.go`:

```go
func TestOpenPaletteActivatesDialog(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = openPalette(m)
	if m.activeDialog != dialogPalette {
		t.Fatalf("got activeDialog=%v, want dialogPalette", m.activeDialog)
	}
	if m.paletteCursor != 0 {
		t.Fatalf("got paletteCursor=%d, want 0", m.paletteCursor)
	}
}

func TestPaletteArrowsMoveCursorWithinBounds(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = openPalette(m)
	updated, _ := m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if m.paletteCursor != 0 {
		t.Fatalf("got %d, want 0 (cannot go above the top)", m.paletteCursor)
	}
	updated, _ = m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.paletteCursor != 1 {
		t.Fatalf("got %d, want 1", m.paletteCursor)
	}
}

func TestPaletteDownStopsAtLastMatch(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = openPalette(m)
	last := len(buildCommands()) - 1
	for i := 0; i < last+5; i++ {
		updated, _ := m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
	}
	if m.paletteCursor != last {
		t.Fatalf("got %d, want %d (clamped to the last command)", m.paletteCursor, last)
	}
}

func TestPaletteTypingFiltersAndResetsCursor(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = openPalette(m)
	updated, _ := m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.paletteCursor != 1 {
		t.Fatalf("setup failed, got cursor=%d", m.paletteCursor)
	}
	updated, _ = m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("undo")})
	m = updated.(Model)
	if m.paletteFilter.Value() != "undo" {
		t.Fatalf("got filter value=%q", m.paletteFilter.Value())
	}
	if m.paletteCursor != 0 {
		t.Fatalf("got cursor=%d, want reset to 0 after the filter changed", m.paletteCursor)
	}
	matches := filteredCommands(m.commands, m.paletteFilter.Value())
	if len(matches) != 1 || matches[0].Name != "Undo" {
		t.Fatalf("got matches=%v", matches)
	}
}

func TestPaletteEnterExecutesSelectedCommandAndCloses(t *testing.T) {
	dir := t.TempDir()
	file := dir + "/a.go"
	if err := writeFile(t, file, "package main"); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)
	m, _ = openPalette(m)
	updated, _ = m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Save")})
	m = updated.(Model)
	matches := filteredCommands(m.commands, m.paletteFilter.Value())
	if len(matches) != 1 || matches[0].Name != "Save" {
		t.Fatalf("setup failed, got matches=%v", matches)
	}
	updated, cmd := m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatal("expected the palette to close after executing a command")
	}
	if cmd == nil {
		t.Fatal("expected the Save command's own command to be returned")
	}
	msg, ok := cmd().(editor.CommandExecutedMsg)
	if !ok || msg.Description == "" {
		t.Fatalf("got %+v, ok=%v", msg, ok)
	}
}

func TestPaletteEnterWithNoMatchesJustCloses(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = openPalette(m)
	updated, _ := m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zzz-no-match")})
	m = updated.(Model)
	updated, cmd := m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatal("expected enter with no matches to close the palette rather than do nothing visibly")
	}
	if cmd != nil {
		t.Fatal("expected no command when there was nothing to execute")
	}
}

func TestPaletteEscCloses(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = openPalette(m)
	updated, _ := m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatal("expected esc to close the palette")
	}
}

func TestUpdateRoutesToPaletteDialogWhenActive(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.clickMenuLabel("Commands")
	if m.activeDialog != dialogPalette {
		t.Fatal("expected clicking Commands to open the palette")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = updated.(Model)
	if m.paletteFilter.Value() != "s" {
		t.Fatalf("got filter value=%q, want the root Update to route typed keys into the palette's filter input", m.paletteFilter.Value())
	}
}
```

This file will need `tea "github.com/charmbracelet/bubbletea"` and `"cody/internal/filetree"` and `"cody/internal/editor"` added to its imports (check `internal/app/menu_test.go`'s existing `writeFile` helper — it's already defined there in the same package, reuse it rather than redefining).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/... -run 'TestOpenPalette|TestPalette|TestUpdateRoutesToPaletteDialog' -v`
Expected: FAIL to compile — `dialogPalette`, `openPalette`, `updatePaletteDialog`, `paletteFilter`, `paletteCursor` don't exist yet.

- [ ] **Step 3: Implement**

Modify `internal/app/dialog.go`'s enum:

```go
const (
	dialogNone dialogKind = iota
	dialogFileOpen
	dialogAbout
	dialogPalette
)
```

Add to `internal/app/palette.go` (after `filteredCommands`):

```go
import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)
```

(Merge this with the existing `"strings"`-only import block from Task 1 into one import block at the top of the file.)

```go
func openPalette(m Model) (Model, tea.Cmd) {
	ti := textinput.New()
	ti.Placeholder = "type to filter..."
	ti.Focus()
	m.paletteFilter = ti
	m.paletteCursor = 0
	m.activeDialog = dialogPalette
	return m, textinput.Blink
}

func (m Model) updatePaletteDialog(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.paletteFilter, cmd = m.paletteFilter.Update(msg)
		return m, cmd
	}
	switch keyMsg.String() {
	case "esc":
		m.activeDialog = dialogNone
		return m, nil
	case "up":
		if m.paletteCursor > 0 {
			m.paletteCursor--
		}
		return m, nil
	case "down":
		matches := filteredCommands(m.commands, m.paletteFilter.Value())
		if m.paletteCursor < len(matches)-1 {
			m.paletteCursor++
		}
		return m, nil
	case "enter":
		matches := filteredCommands(m.commands, m.paletteFilter.Value())
		m.activeDialog = dialogNone
		if m.paletteCursor < 0 || m.paletteCursor >= len(matches) {
			return m, nil
		}
		selected := matches[m.paletteCursor]
		updated, cmd := selected.Handler(m)
		return updated, cmd
	}
	var cmd tea.Cmd
	m.paletteFilter, cmd = m.paletteFilter.Update(msg)
	m.paletteCursor = 0
	return m, cmd
}

func renderPaletteDialog(width, height int, filterInput textinput.Model, matches []Command, cursor int) string {
	var lines []string
	for i, cmd := range matches {
		line := fmt.Sprintf("%-12s %s", cmd.Name, cmd.Shortcut)
		if i == cursor {
			line = lipgloss.NewStyle().Reverse(true).Render(line)
		}
		lines = append(lines, line)
	}
	content := "Commands:\n\n" + filterInput.View() + "\n\n" + strings.Join(lines, "\n")
	return lipgloss.NewStyle().Width(width).Height(height).Padding(1, 2).Render(content)
}
```

Note the `"enter"` case destructures `selected.Handler(m)` into `updated, cmd` before returning them separately — do NOT write `return selected.Handler(m)` directly, per this plan's Global Constraints note on this exact Go pitfall (bare-call return type mismatch between `(Model, tea.Cmd)` and this function's `(tea.Model, tea.Cmd)`).

Modify `internal/app/model.go`:

1. Add `paletteFilter textinput.Model` and `paletteCursor int` fields to `Model`.
2. Add a palette guard to `Update`, alongside the existing two:

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.activeDialog == dialogFileOpen {
		return m.updateFileOpenDialog(msg)
	}
	if m.activeDialog == dialogAbout {
		return m.updateAboutDialog(msg)
	}
	if m.activeDialog == dialogPalette {
		return m.updatePaletteDialog(msg)
	}
	switch msg := msg.(type) {
	// ... existing cases unchanged ...
```

3. Change `clickMenuLabel`'s `"Commands"` case from the Phase 2 stub to actually opening the palette:

```go
	case "Commands":
		m.openMenu = ""
		return openPalette(m)
```

(The surrounding `switch`/`case "File", "Edit":`/`case "About":`/final `return m, nil` are unchanged — only the `"Commands"` case's body changes.)

4. Add a palette-rendering branch to `View()`, alongside the existing two dialog branches:

```go
	if m.activeDialog == dialogPalette {
		matches := filteredCommands(m.commands, m.paletteFilter.Value())
		dialog := renderPaletteDialog(m.width, paneHeight, m.paletteFilter, matches, m.paletteCursor)
		return lipgloss.JoinVertical(lipgloss.Left, menuBar, dialog, status)
	}
```

(Place this after the existing `if m.activeDialog == dialogAbout { ... }` block and before the dropdown-rendering code that follows it.)

Modify `internal/app/menu_test.go` — replace the now-obsolete stub test:

```go
func TestClickCommandsLabelOpensPalette(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.clickMenuLabel("Commands")
	if m.openMenu != "" {
		t.Fatal("Commands must not open a dropdown, it opens a modal palette")
	}
	if m.activeDialog != dialogPalette {
		t.Fatalf("got activeDialog=%v, want dialogPalette", m.activeDialog)
	}
}
```

(This replaces `TestClickCommandsLabelIsANoOpStub` entirely — delete that function, it asserted Phase 2's now-superseded stub behavior.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/app/... -v`
Expected: PASS — every new test above, the replaced `TestClickCommandsLabelOpensPalette`, and every other existing test in the package.

Run: `go build ./...` and `go vet ./...`
Expected: both clean.

- [ ] **Step 5: Commit**

```bash
git add internal/app/dialog.go internal/app/palette.go internal/app/palette_test.go internal/app/model.go internal/app/menu_test.go
git commit -m "feat: wire the command palette dialog behind the Commands menu label"
```

---

### Task 3: Final integration test and README update

**Files:**
- Test: `internal/app/model_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: everything from Tasks 1-2 — no new production code, this task only adds one end-to-end test and documentation.

- [ ] **Step 1: Write the integration test**

Add to `internal/app/model_test.go`:

```go
func TestMouseClickCommandsThenFilterThenEnterRunsSave(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "target.go")
	if err := os.WriteFile(file, []byte("package main"), 0644); err != nil {
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

	// Click "Commands" (columns 12-19, row 0 — see menuLabels()).
	updated, _ = m.Update(tea.MouseMsg{X: 12, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)
	if m.activeDialog != dialogPalette {
		t.Fatalf("got activeDialog=%v, want dialogPalette", m.activeDialog)
	}

	// Type "save" to filter down to a single match.
	for _, r := range "save" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	matches := filteredCommands(m.commands, m.paletteFilter.Value())
	if len(matches) != 1 || matches[0].Name != "Save" {
		t.Fatalf("got matches=%v", matches)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.activeDialog != dialogNone {
		t.Fatal("expected the palette to close after running the selected command")
	}
	if m.recentCommand == "" {
		t.Fatal("expected the Save command's status message to reach the status bar")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails, then passes**

Run: `go test ./internal/app/... -run TestMouseClickCommandsThenFilterThenEnterRunsSave -v`

If it fails, cross-check the mouse-click X/Y against `menuLabels()`'s actual column layout (`"File"` [0,4), `"Edit"` [6,10), `"Commands"` [12,20), `"About"` [22,27) — from Phase 2's own hand-verified arithmetic) rather than assuming the test's coordinates are correct; fix the test's coordinates if they're off, not the production code.

Expected once correct: PASS.

- [ ] **Step 3: Run the whole test suite**

Run: `go test ./... -v`
Expected: PASS — every test in every package (`app`, `editor`, `filetree`, `statusbar`).

Run: `go build ./...`, `go vet ./...`, `gofmt -l .`
Expected: all clean, no output.

- [ ] **Step 4: Update the README**

Modify `README.md`'s "Status" section and the `Commands` line in "Keybindings":

```markdown
## Status

Phase 3 (command palette) of a phased build — see
`docs/superpowers/specs/2026-09-05-cody-tui-editor-design.md` for the full
design and `docs/superpowers/plans/` for phase-by-phase implementation plans.

This phase adds: a command palette behind the Commands menu — click it,
type to filter by name, arrows to move the selection, Enter to run it,
Esc to close. It does not yet have: syntax highlighting/folding, an
embedded shell, or in-buffer search — those land in later phases.
```

```markdown
- Menu bar: click `File`/`Edit` to open a dropdown, click an item to run it,
  `Esc` or clicking elsewhere closes it; click `About` for app info; click
  `Commands` to open the command palette (type to filter, arrows to move
  the selection, `Enter` to run the selected command, `Esc` to close)
```

- [ ] **Step 5: Commit**

```bash
git add internal/app/model_test.go README.md
git commit -m "test: add mouse-to-palette integration test; docs: update README for phase 3"
```
