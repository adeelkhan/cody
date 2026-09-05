# Cody Phase 2: Command Registry + Core Menu Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a command registry, a mouse-driven File/Edit/About menu bar, Cut/Copy/Paste with an internal clipboard, and undo/redo — all dispatched through one registry, per the design spec's phase-2 scope. The Commands menu (command palette) stays a stub — that's Phase 3.

**Architecture:** A new `[]Command` registry in package `app` (`{Name, Shortcut string; Handler func(Model) (Model, tea.Cmd)}`) becomes the single dispatch path for every global shortcut. The root model's key switch, the File/Edit dropdown click handlers, and (later, Phase 3) the command palette all call the same registry entries — nothing duplicates shortcut-to-action logic in more than one place. Editor-level features (selection, clipboard, undo/redo) live in `internal/editor` exactly like Phase 1's editing/save did, reached via synthetic `tea.KeyMsg`s the registry's handlers construct — the same pattern Phase 1's final fix wave already established for `ctrl+s`.

**Tech Stack:** Go, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss` (both already dependencies), plus `github.com/charmbracelet/bubbles` (new, for the File > Open path input).

**Spec:** `docs/superpowers/specs/2026-09-05-cody-tui-editor-design.md` (sections 6 and 8 are the ones this plan implements)

## Global Constraints

- Module `cody`, packages import as `cody/internal/...` (unchanged from Phase 1).
- Canonical keybindings, `Ctrl`-based on every platform (spec §8): `ctrl+o` open, `ctrl+s` save, `ctrl+x`/`ctrl+c`/`ctrl+v` cut/copy/paste, `ctrl+z`/`ctrl+y` undo/redo, `ctrl+q` quit (all pre-existing or added by this plan). No Cmd-key handling — spec's macOS caveat is documented in the About dialog text, not implemented as a keybinding.
- Menu bar is mouse-click-only per spec §8 (no F10/Alt keyboard activation) — clicking "File"/"Edit" toggles that dropdown; clicking an item runs it via the registry; clicking elsewhere or pressing `Esc` closes the dropdown without action.
- **Commands** menu is an accepted no-op stub this phase (clicking it sets a "coming in a later phase" status message) — the command palette itself is Phase 3, explicitly listed as a separate item in the spec's phased build order.
- Clipboard is an internal buffer on `editor.Model`, not the OS clipboard (spec §6) — there's only one editor pane in this app, so no cross-package clipboard plumbing is needed yet.
- **Design decisions filling two spec gaps** (spec §6 describes selection/cut/copy at a high level without pinning down exact keys or the no-selection case):
  - Selection is extended with `Shift+Left/Right/Up/Down`; any plain (non-shift) movement key collapses/clears it. This depends on the terminal reporting shift-modified arrow keys, which most modern terminal emulators do (iTerm2, Alacritty, Kitty, WezTerm) — terminals that don't will simply be unable to extend a selection, a known limitation worth documenting in the README, not a bug to work around in this plan.
  - Cut/Copy with no active selection operates on the whole current line (matches common terminal-editor convention, e.g. nano's line-kill) rather than doing nothing.
- Undo/redo is **snapshot-based** (a stack of full-line-slice + cursor snapshots taken before each mutating edit), not literal command-inverse operations — simpler to implement correctly than hand-rolled inverses for every edit type (especially `DeleteBefore`'s line-join case), and still delivers the spec's user-facing `Ctrl+Z`/`Ctrl+Y` behavior exactly. This is a deliberate implementation-detail choice, not a shortcut around a hard requirement — the spec's "invertible insert/delete operation" phrasing describes the intended UX, not a mandated internal mechanism.
- The menu dropdown is rendered as a **pushed block** between the menu bar and the body (not a floating/z-ordered overlay) — lipgloss has no compositing primitive for absolute-position overlays, and a pushed block keeps click-hit-testing exactly consistent with what's rendered, at the cost of the body visibly shifting down while a menu is open. Dropdown click hit-testing checks the click falls within the open label's column band (its start column to `+20`) and within the item rows below the menu bar — it does not track a fully arbitrary rendered box position.
- File > Open and the About dialog are **modal**: while either is active, they fully replace the body (tree/editor/terminal) in `View()` and capture all `tea.KeyMsg` input in `Update()` before the registry or focused-pane dispatch ever sees it. Mouse clicks are ignored while a dialog is open.
- Every executed command, regardless of dispatch path (shortcut, File/Edit dropdown click), writes a short description to the status bar's recent-command segment — this already works via `editor.CommandExecutedMsg`; this plan extends it to the new actions.

---

### Task 1: Command registry, project root path, and migrating `ctrl+q`/`ctrl+s` onto it

**Files:**
- Create: `internal/app/commands.go`
- Test: `internal/app/commands_test.go`
- Modify: `internal/app/model.go`

**Interfaces:**
- Produces (package `cody/internal/app`):
  - `type Command struct { Name, Shortcut string; Handler func(Model) (Model, tea.Cmd) }`
  - `func buildCommands() []Command`
  - `func commandForShortcut(commands []Command, shortcut string) (Command, bool)`
  - `Model` gains a `commands []Command` field (populated in `New`) and a `rootPath string` field (the absolute project root, needed by Task 5's File > Open to resolve relative paths).

- [ ] **Step 1: Write the failing tests**

Create `internal/app/commands_test.go`:

```go
package app

import "testing"

func TestCommandForShortcutFound(t *testing.T) {
	commands := []Command{
		{Name: "Save", Shortcut: "ctrl+s"},
		{Name: "Quit", Shortcut: "ctrl+q"},
	}
	cmd, ok := commandForShortcut(commands, "ctrl+q")
	if !ok || cmd.Name != "Quit" {
		t.Fatalf("got %+v, ok=%v, want Quit, true", cmd, ok)
	}
}

func TestCommandForShortcutNotFound(t *testing.T) {
	commands := []Command{{Name: "Save", Shortcut: "ctrl+s"}}
	_, ok := commandForShortcut(commands, "ctrl+z")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestBuildCommandsHasSaveAndQuit(t *testing.T) {
	commands := buildCommands()
	if _, ok := commandForShortcut(commands, "ctrl+s"); !ok {
		t.Fatal("expected a ctrl+s command")
	}
	if _, ok := commandForShortcut(commands, "ctrl+q"); !ok {
		t.Fatal("expected a ctrl+q command")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/... -run TestCommandForShortcut -v`
Expected: FAIL to compile — `Command`, `commandForShortcut`, `buildCommands` don't exist yet.

- [ ] **Step 3: Write the implementation**

Create `internal/app/commands.go`:

```go
package app

import tea "github.com/charmbracelet/bubbletea"

type Command struct {
	Name     string
	Shortcut string
	Handler  func(Model) (Model, tea.Cmd)
}

func buildCommands() []Command {
	return []Command{
		{Name: "Save", Shortcut: "ctrl+s", Handler: cmdSave},
		{Name: "Quit", Shortcut: "ctrl+q", Handler: cmdQuit},
	}
}

func commandForShortcut(commands []Command, shortcut string) (Command, bool) {
	for _, c := range commands {
		if c.Shortcut == shortcut {
			return c, true
		}
	}
	return Command{}, false
}

func cmdSave(m Model) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	return m, cmd
}

func cmdQuit(m Model) (Model, tea.Cmd) {
	return m, tea.Quit
}
```

Modify `internal/app/model.go`:

1. Add `commands []Command` and `rootPath string` fields to `Model`.
2. In `New`, populate both:

```go
func New(rootPath string, nerdFont bool) (Model, error) {
	tree, err := filetree.New(rootPath, nerdFont)
	if err != nil {
		return Model{}, err
	}
	absPath, err := filepath.Abs(rootPath)
	if err != nil {
		return Model{}, err
	}
	return Model{
		tree:        tree,
		editor:      editor.New(),
		focus:       focusTree,
		projectName: filepath.Base(absPath),
		rootPath:    absPath,
		commands:    buildCommands(),
	}, nil
}
```

3. Replace the explicit `"ctrl+q"` and `"ctrl+s"` cases in `Update`'s `tea.KeyMsg` switch with a single `default` case that dispatches through the registry:

```go
		switch msg.String() {
		case "tab", "shift+tab":
			m.toggleFocus()
			return m, nil
		default:
			if cmd, ok := commandForShortcut(m.commands, msg.String()); ok {
				return cmd.Handler(m)
			}
		}
```

(This removes the old `case "ctrl+q":`/`case "ctrl+s":` lines entirely — they're superseded by the registry dispatch.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/app/... -v`
Expected: PASS — the 3 new tests, plus every existing Phase 1 test in this package (`TestTabTogglesFocus`, `TestFileOpenedMsgLoadsEditorAndSwitchesFocus`, `TestCtrlQQuits`, `TestCtrlSWorksRegardlessOfFocus`, `TestFullFlowOpenTypeSaveUpdatesStatusBar`, `TestViewRendersAtSmallSize`). Their observable behavior (ctrl+q quits, ctrl+s saves regardless of focus) is unchanged — only the internal dispatch path moved to the registry.

- [ ] **Step 5: Commit**

```bash
git add internal/app/commands.go internal/app/commands_test.go internal/app/model.go
git commit -m "feat: add command registry, migrate ctrl+q/ctrl+s onto it"
```

---

### Task 2: Editor selection, clipboard (Cut/Copy/Paste), and undo/redo

**Files:**
- Modify: `internal/editor/buffer.go`
- Modify: `internal/editor/model.go`
- Test: `internal/editor/buffer_test.go`
- Test: `internal/editor/model_test.go`

**Interfaces:**
- Produces (package `cody/internal/editor`):
  - `func (b *Buffer) TextRange(startLine, startCol, endLine, endCol int) string` (non-destructive read)
  - `func (b *Buffer) DeleteRange(startLine, startCol, endLine, endCol int) string` (destructive, returns removed text)
  - `func (b *Buffer) DeleteLine(line int) string` (destructive, returns removed line)
  - `Model` gains: `selecting bool`, `selAnchorLine`, `selAnchorCol int`, `clipboard string`, `undoStack`, `redoStack []undoSnapshot`
  - `func (m Model) selectionRange() (startLine, startCol, endLine, endCol int, ok bool)` (normalized, `ok=false` if empty/no selection)
  - `func (m *Model) Cut() string`, `func (m *Model) Copy() string`, `func (m *Model) Paste() string` — each returns a status description for `CommandExecutedMsg`, exactly like the existing `save()` method's pattern
  - `func (m *Model) undo() string`, `func (m *Model) redo() string`
  - `Update` gains `shift+left/right/up/down` (extend selection), `ctrl+x`/`ctrl+c`/`ctrl+v` (cut/copy/paste), `ctrl+z`/`ctrl+y` (undo/redo) cases
  - `View()` renders the active selection with reverse-video styling

- [ ] **Step 1: Write the failing buffer tests**

Add to `internal/editor/buffer_test.go`:

```go
func TestTextRangeSameLine(t *testing.T) {
	buf := &Buffer{Lines: []string{"hello world"}}
	got := buf.TextRange(0, 6, 0, 11)
	if got != "world" {
		t.Fatalf("got %q, want %q", got, "world")
	}
}

func TestTextRangeMultiLine(t *testing.T) {
	buf := &Buffer{Lines: []string{"hello", "middle", "world"}}
	got := buf.TextRange(0, 3, 2, 2)
	want := "lo\nmiddle\nwo"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDeleteRangeSameLine(t *testing.T) {
	buf := &Buffer{Lines: []string{"hello world"}}
	removed := buf.DeleteRange(0, 5, 0, 11)
	if removed != " world" || buf.Lines[0] != "hello" {
		t.Fatalf("got removed=%q lines=%v", removed, buf.Lines)
	}
}

func TestDeleteRangeMultiLine(t *testing.T) {
	buf := &Buffer{Lines: []string{"hello", "middle", "world"}}
	removed := buf.DeleteRange(0, 3, 2, 2)
	if removed != "lo\nmiddle\nwo" {
		t.Fatalf("got removed=%q", removed)
	}
	if len(buf.Lines) != 1 || buf.Lines[0] != "helrld" {
		t.Fatalf("got lines=%v", buf.Lines)
	}
}

func TestDeleteLine(t *testing.T) {
	buf := &Buffer{Lines: []string{"a", "b", "c"}}
	removed := buf.DeleteLine(1)
	if removed != "b" || len(buf.Lines) != 2 || buf.Lines[0] != "a" || buf.Lines[1] != "c" {
		t.Fatalf("got removed=%q lines=%v", removed, buf.Lines)
	}
}

func TestDeleteLineLastRemaining(t *testing.T) {
	buf := &Buffer{Lines: []string{"only"}}
	removed := buf.DeleteLine(0)
	if removed != "only" || len(buf.Lines) != 1 || buf.Lines[0] != "" {
		t.Fatalf("got removed=%q lines=%v, want one empty line left", removed, buf.Lines)
	}
}
```

- [ ] **Step 2: Run the buffer tests to verify they fail**

Run: `go test ./internal/editor/... -run 'TestTextRange|TestDeleteRange|TestDeleteLine' -v`
Expected: FAIL to compile — the new `Buffer` methods don't exist yet.

- [ ] **Step 3: Implement the buffer additions**

Add to `internal/editor/buffer.go` (the file already has `"os"` and `"strings"` imported):

```go
func (b *Buffer) TextRange(startLine, startCol, endLine, endCol int) string {
	if startLine == endLine {
		l := []rune(b.Lines[startLine])
		return string(l[startCol:endCol])
	}
	var sb strings.Builder
	startRunes := []rune(b.Lines[startLine])
	sb.WriteString(string(startRunes[startCol:]))
	for i := startLine + 1; i < endLine; i++ {
		sb.WriteString("\n")
		sb.WriteString(b.Lines[i])
	}
	sb.WriteString("\n")
	endRunes := []rune(b.Lines[endLine])
	sb.WriteString(string(endRunes[:endCol]))
	return sb.String()
}

func (b *Buffer) DeleteRange(startLine, startCol, endLine, endCol int) string {
	removed := b.TextRange(startLine, startCol, endLine, endCol)
	startRunes := []rune(b.Lines[startLine])
	endRunes := []rune(b.Lines[endLine])
	merged := string(startRunes[:startCol]) + string(endRunes[endCol:])
	b.Lines = append(b.Lines[:startLine], append([]string{merged}, b.Lines[endLine+1:]...)...)
	b.Dirty = true
	return removed
}

func (b *Buffer) DeleteLine(line int) string {
	removed := b.Lines[line]
	if len(b.Lines) == 1 {
		b.Lines[0] = ""
	} else {
		b.Lines = append(b.Lines[:line], b.Lines[line+1:]...)
	}
	b.Dirty = true
	return removed
}
```

- [ ] **Step 4: Run the buffer tests to verify they pass**

Run: `go test ./internal/editor/... -run 'TestTextRange|TestDeleteRange|TestDeleteLine' -v`
Expected: PASS

- [ ] **Step 5: Write the failing model tests**

Add to `internal/editor/model_test.go`:

```go
func TestShiftArrowsExtendSelectionAndCutRemovesIt(t *testing.T) {
	m := setupEditor(t, "hello world")
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	}
	for i := 0; i < 6; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	}
	desc := m.Cut()
	if desc != "Cut selection" {
		t.Fatalf("got %q", desc)
	}
	if m.buf.Lines[0] != "hello" {
		t.Fatalf("got %q, want %q", m.buf.Lines[0], "hello")
	}
	if m.clipboard != " world" {
		t.Fatalf("got clipboard=%q", m.clipboard)
	}
}

func TestPlainArrowCollapsesSelection(t *testing.T) {
	m := setupEditor(t, "hello")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if _, _, _, _, ok := m.selectionRange(); ok {
		t.Fatal("expected selection to be cleared by a plain arrow key")
	}
}

func TestCutWithNoSelectionCutsWholeLine(t *testing.T) {
	m := setupEditor(t, "first\nsecond")
	desc := m.Cut()
	if desc != "Cut line" {
		t.Fatalf("got %q", desc)
	}
	if len(m.buf.Lines) != 1 || m.buf.Lines[0] != "second" {
		t.Fatalf("got %v", m.buf.Lines)
	}
	if m.clipboard != "first\n" {
		t.Fatalf("got clipboard=%q", m.clipboard)
	}
}

func TestCopyWithNoSelectionDoesNotMutateBuffer(t *testing.T) {
	m := setupEditor(t, "only line")
	desc := m.Copy()
	if desc != "Copied line" {
		t.Fatalf("got %q", desc)
	}
	if m.buf.Lines[0] != "only line" {
		t.Fatal("copy must not mutate the buffer")
	}
	if m.clipboard != "only line\n" {
		t.Fatalf("got clipboard=%q", m.clipboard)
	}
}

func TestPasteInsertsClipboardAtCursor(t *testing.T) {
	m := setupEditor(t, "ac")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m.clipboard = "b"
	desc := m.Paste()
	if desc != "Pasted" {
		t.Fatalf("got %q", desc)
	}
	if m.buf.Lines[0] != "abc" {
		t.Fatalf("got %q", m.buf.Lines[0])
	}
}

func TestPasteMultiLineClipboardSplitsLines(t *testing.T) {
	m := setupEditor(t, "")
	m.clipboard = "ab\ncd"
	m.Paste()
	if len(m.buf.Lines) != 2 || m.buf.Lines[0] != "ab" || m.buf.Lines[1] != "cd" {
		t.Fatalf("got %v", m.buf.Lines)
	}
}

func TestUndoRestoresPreviousLineContent(t *testing.T) {
	m := setupEditor(t, "hello")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	if m.buf.Lines[0] != "Xhello" {
		t.Fatalf("setup failed, got %q", m.buf.Lines[0])
	}
	desc := m.undo()
	if desc != "Undo" {
		t.Fatalf("got %q", desc)
	}
	if m.buf.Lines[0] != "hello" {
		t.Fatalf("got %q, want %q after undo", m.buf.Lines[0], "hello")
	}
}

func TestRedoReappliesUndoneEdit(t *testing.T) {
	m := setupEditor(t, "hello")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	m.undo()
	desc := m.redo()
	if desc != "Redo" {
		t.Fatalf("got %q", desc)
	}
	if m.buf.Lines[0] != "Xhello" {
		t.Fatalf("got %q, want %q after redo", m.buf.Lines[0], "Xhello")
	}
}

func TestUndoWithEmptyStackReportsNothingToUndo(t *testing.T) {
	m := setupEditor(t, "hello")
	if desc := m.undo(); desc != "Nothing to undo" {
		t.Fatalf("got %q", desc)
	}
}

func TestNewEditAfterUndoClearsRedoStack(t *testing.T) {
	m := setupEditor(t, "hello")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	m.undo()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")})
	if desc := m.redo(); desc != "Nothing to redo" {
		t.Fatalf("got %q, want redo stack cleared by the new edit", desc)
	}
}

func TestCtrlXCtrlCCtrlVKeysDispatchThroughUpdate(t *testing.T) {
	m := setupEditor(t, "hello")
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected a command from ctrl+c")
	}
	msg, ok := cmd().(CommandExecutedMsg)
	if !ok || msg.Description != "Copied line" {
		t.Fatalf("got %+v, ok=%v", msg, ok)
	}
}
```

- [ ] **Step 6: Run the model tests to verify they fail**

Run: `go test ./internal/editor/... -v`
Expected: FAIL to compile — `selectionRange`, `Cut`, `Copy`, `Paste`, `undo`, `redo`, and the new key handling don't exist yet.

- [ ] **Step 7: Implement selection, clipboard, and undo/redo in `internal/editor/model.go`**

Add `"github.com/charmbracelet/lipgloss"` to the imports (already has `fmt`, `path/filepath`, `strings`, `tea`).

Extend the `Model` struct:

```go
type Model struct {
	buf           *Buffer
	cursorLine    int
	cursorCol     int
	width         int
	height        int
	selecting     bool
	selAnchorLine int
	selAnchorCol  int
	clipboard     string
	undoStack     []undoSnapshot
	redoStack     []undoSnapshot
}

type undoSnapshot struct {
	lines      []string
	cursorLine int
	cursorCol  int
}
```

Add the selection, clipboard, and undo/redo methods:

```go
func (m Model) selectionRange() (startLine, startCol, endLine, endCol int, ok bool) {
	if !m.selecting {
		return 0, 0, 0, 0, false
	}
	aLine, aCol := m.selAnchorLine, m.selAnchorCol
	cLine, cCol := m.cursorLine, m.cursorCol
	if aLine > cLine || (aLine == cLine && aCol > cCol) {
		aLine, cLine = cLine, aLine
		aCol, cCol = cCol, aCol
	}
	if aLine == cLine && aCol == cCol {
		return 0, 0, 0, 0, false
	}
	return aLine, aCol, cLine, cCol, true
}

func (m *Model) extendSelection() {
	if !m.selecting {
		m.selecting = true
		m.selAnchorLine = m.cursorLine
		m.selAnchorCol = m.cursorCol
	}
}

func (m *Model) Cut() string {
	if startLine, startCol, endLine, endCol, ok := m.selectionRange(); ok {
		m.pushUndo()
		m.clipboard = m.buf.DeleteRange(startLine, startCol, endLine, endCol)
		m.cursorLine, m.cursorCol = startLine, startCol
		m.selecting = false
		return "Cut selection"
	}
	m.pushUndo()
	m.clipboard = m.buf.DeleteLine(m.cursorLine) + "\n"
	if m.cursorLine >= len(m.buf.Lines) {
		m.cursorLine = len(m.buf.Lines) - 1
	}
	m.cursorCol = 0
	return "Cut line"
}

func (m *Model) Copy() string {
	if startLine, startCol, endLine, endCol, ok := m.selectionRange(); ok {
		m.clipboard = m.buf.TextRange(startLine, startCol, endLine, endCol)
		return "Copied selection"
	}
	m.clipboard = m.buf.Lines[m.cursorLine] + "\n"
	return "Copied line"
}

func (m *Model) Paste() string {
	if m.clipboard == "" {
		return "Nothing to paste"
	}
	m.pushUndo()
	m.insertText(m.clipboard)
	return "Pasted"
}

func (m *Model) insertText(text string) {
	for _, r := range text {
		if r == '\n' || r == '\r' {
			m.buf.InsertNewline(m.cursorLine, m.cursorCol)
			m.cursorLine++
			m.cursorCol = 0
			continue
		}
		m.buf.InsertRune(m.cursorLine, m.cursorCol, r)
		m.cursorCol++
	}
}

func snapshotLines(lines []string) []string {
	cp := make([]string, len(lines))
	copy(cp, lines)
	return cp
}

func (m *Model) pushUndo() {
	m.undoStack = append(m.undoStack, undoSnapshot{
		lines:      snapshotLines(m.buf.Lines),
		cursorLine: m.cursorLine,
		cursorCol:  m.cursorCol,
	})
	m.redoStack = nil
}

func (m *Model) undo() string {
	if len(m.undoStack) == 0 {
		return "Nothing to undo"
	}
	current := undoSnapshot{lines: snapshotLines(m.buf.Lines), cursorLine: m.cursorLine, cursorCol: m.cursorCol}
	prev := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]
	m.redoStack = append(m.redoStack, current)
	m.buf.Lines = prev.lines
	m.cursorLine = prev.cursorLine
	m.cursorCol = prev.cursorCol
	m.buf.Dirty = true
	return "Undo"
}

func (m *Model) redo() string {
	if len(m.redoStack) == 0 {
		return "Nothing to redo"
	}
	current := undoSnapshot{lines: snapshotLines(m.buf.Lines), cursorLine: m.cursorLine, cursorCol: m.cursorCol}
	next := m.redoStack[len(m.redoStack)-1]
	m.redoStack = m.redoStack[:len(m.redoStack)-1]
	m.undoStack = append(m.undoStack, current)
	m.buf.Lines = next.lines
	m.cursorLine = next.cursorLine
	m.cursorCol = next.cursorCol
	m.buf.Dirty = true
	return "Redo"
}
```

Modify `Update`'s switch: replace the plain movement cases so they collapse selection, add the shift-movement cases, add cut/copy/paste/undo/redo cases, add `pushUndo()` calls before every other mutating case, and replace the multi-rune `default` branch with `insertText`:

```go
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
		m.pushUndo()
		m.buf.InsertNewline(m.cursorLine, m.cursorCol)
		m.cursorLine++
		m.cursorCol = 0
	case "backspace":
		m.pushUndo()
		m.cursorLine, m.cursorCol = m.buf.DeleteBefore(m.cursorLine, m.cursorCol)
	case "ctrl+s":
		desc := m.save()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case " ":
		m.pushUndo()
		m.buf.InsertRune(m.cursorLine, m.cursorCol, ' ')
		m.cursorCol++
	case "ctrl+x":
		desc := m.Cut()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+c":
		desc := m.Copy()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+v":
		desc := m.Paste()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+z":
		desc := m.undo()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+y":
		desc := m.redo()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	default:
		if keyMsg.Type == tea.KeyRunes && !keyMsg.Alt {
			m.pushUndo()
			m.insertText(string(keyMsg.Runes))
		}
	}
```

Update `View()` to highlight the active selection with reverse video:

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
		}
		b.WriteString(fmt.Sprintf("%s%4d %s\n", cursorMark, i+1, rendered))
	}
	return b.String()
}

func highlightSelection(line string, lineIdx, startLine, startCol, endLine, endCol int) string {
	runes := []rune(line)
	from, to := 0, len(runes)
	if lineIdx == startLine {
		from = startCol
	}
	if lineIdx == endLine {
		to = endCol
	}
	if from > len(runes) {
		from = len(runes)
	}
	if to > len(runes) {
		to = len(runes)
	}
	if from >= to {
		return line
	}
	style := lipgloss.NewStyle().Reverse(true)
	return string(runes[:from]) + style.Render(string(runes[from:to])) + string(runes[to:])
}
```

- [ ] **Step 8: Run all editor package tests to verify they pass**

Run: `go test ./internal/editor/... -v`
Expected: PASS — every new test above, plus every existing Phase 1 test in this package (typing, enter-split, ctrl+s save, filetype, space bar, alt-key guard, multi-rune-newline-burst).

- [ ] **Step 9: Commit**

```bash
git add internal/editor/buffer.go internal/editor/buffer_test.go internal/editor/model.go internal/editor/model_test.go
git commit -m "feat: add editor selection, clipboard cut/copy/paste, and undo/redo"
```

---

### Task 3: Wire Cut/Copy/Paste/Undo/Redo into the app-level command registry

**Files:**
- Modify: `internal/app/commands.go`
- Test: `internal/app/commands_test.go`

**Interfaces:**
- Consumes: `editor.Model.Update` from Task 2 (already handles `ctrl+x`/`ctrl+c`/`ctrl+v`/`ctrl+z`/`ctrl+y` when it receives those `tea.KeyMsg`s).
- Produces: `buildCommands()` grows to include `Cut`, `Copy`, `Paste`, `Undo`, `Redo` entries, each delegating to the editor exactly like `cmdSave` already does — this is what makes these shortcuts work **regardless of which pane has focus**, the same property Phase 1's final fix wave established for `ctrl+s`.

- [ ] **Step 1: Write the failing test**

Add to `internal/app/commands_test.go`:

```go
func TestBuildCommandsHasCutCopyPasteUndoRedo(t *testing.T) {
	commands := buildCommands()
	for _, shortcut := range []string{"ctrl+x", "ctrl+c", "ctrl+v", "ctrl+z", "ctrl+y"} {
		if _, ok := commandForShortcut(commands, shortcut); !ok {
			t.Fatalf("expected a %s command", shortcut)
		}
	}
}

func TestCutCommandDelegatesToEditorRegardlessOfFocus(t *testing.T) {
	dir := t.TempDir()
	file := dir + "/a.go"
	if err := os.WriteFile(file, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // back to the tree
	m = updated.(Model)
	if m.focus != focusTree {
		t.Fatal("expected focus on tree")
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected ctrl+c to produce a command even with the tree focused")
	}
	msg, ok := cmd().(editor.CommandExecutedMsg)
	if !ok || msg.Description != "Copied line" {
		t.Fatalf("got %+v, ok=%v", msg, ok)
	}
}
```

This test file already imports `os`, `path/filepath`, `testing`, `tea`, and `cody/internal/filetree` (from Phase 1's `model_test.go` in the same package) and will need `cody/internal/editor` added — check the existing imports in `internal/app/model_test.go` before adding a duplicate import in `commands_test.go` (Go allows the same import in multiple files of one package, so just add what `commands_test.go` itself needs: `os`, `testing`, `tea`, `cody/internal/editor`, `cody/internal/filetree`).

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/app/... -run 'TestBuildCommandsHasCutCopyPasteUndoRedo|TestCutCommandDelegatesToEditorRegardlessOfFocus' -v`
Expected: FAIL — the new registry entries don't exist yet, so `commandForShortcut` returns not-found for these shortcuts.

- [ ] **Step 3: Implement**

Extend `buildCommands()` in `internal/app/commands.go`:

```go
func buildCommands() []Command {
	return []Command{
		{Name: "Save", Shortcut: "ctrl+s", Handler: cmdSave},
		{Name: "Cut", Shortcut: "ctrl+x", Handler: cmdCut},
		{Name: "Copy", Shortcut: "ctrl+c", Handler: cmdCopy},
		{Name: "Paste", Shortcut: "ctrl+v", Handler: cmdPaste},
		{Name: "Undo", Shortcut: "ctrl+z", Handler: cmdUndo},
		{Name: "Redo", Shortcut: "ctrl+y", Handler: cmdRedo},
		{Name: "Quit", Shortcut: "ctrl+q", Handler: cmdQuit},
	}
}

func cmdCut(m Model) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	return m, cmd
}

func cmdCopy(m Model) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	return m, cmd
}

func cmdPaste(m Model) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
	return m, cmd
}

func cmdUndo(m Model) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})
	return m, cmd
}

func cmdRedo(m Model) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	return m, cmd
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/app/... -v`
Expected: PASS — all tests in the package, including every Phase 1 test and Task 1's tests.

- [ ] **Step 5: Commit**

```bash
git add internal/app/commands.go internal/app/commands_test.go
git commit -m "feat: wire cut/copy/paste/undo/redo into the command registry"
```

---

### Task 4: Mouse-driven File/Edit menu bar

**Files:**
- Create: `internal/app/menu.go`
- Test: `internal/app/menu_test.go`
- Modify: `internal/app/model.go`
- Modify: `cmd/cody/main.go`

**Interfaces:**
- Consumes: `Command`, `buildCommands()`/`m.commands` from Tasks 1-3.
- Produces (package `cody/internal/app`):
  - `type menuLabel struct { name string; startCol, endCol int }`
  - `func menuLabels() []menuLabel`
  - `func menuLabelAt(col int) (string, bool)`
  - `func menuItemsFor(menu string) []string`
  - `func commandByName(commands []Command, name string) (Command, bool)`
  - `func renderMenuBar(width int, openMenu string) string`
  - `func renderDropdown(menu string, commands []Command) string`
  - `Model` gains an `openMenu string` field (empty = no dropdown open) and handles `tea.MouseMsg` in `Update`.

- [ ] **Step 1: Write the failing tests**

Create `internal/app/menu_test.go`:

```go
package app

import "testing"

func TestMenuLabelAtHitsFile(t *testing.T) {
	name, ok := menuLabelAt(0)
	if !ok || name != "File" {
		t.Fatalf("got %q, ok=%v", name, ok)
	}
}

func TestMenuLabelAtMissesGap(t *testing.T) {
	// "File" occupies columns 0-3; column 4 is part of the 2-space gap.
	if _, ok := menuLabelAt(4); ok {
		t.Fatal("expected the gap between labels to miss")
	}
}

func TestMenuLabelAtHitsEdit(t *testing.T) {
	// "File  " is 6 columns (4 + 2-space gap), so "Edit" starts at column 6.
	name, ok := menuLabelAt(6)
	if !ok || name != "Edit" {
		t.Fatalf("got %q, ok=%v", name, ok)
	}
}

func TestMenuItemsForFile(t *testing.T) {
	items := menuItemsFor("File")
	if len(items) != 2 || items[0] != "Open" || items[1] != "Save" {
		t.Fatalf("got %v", items)
	}
}

func TestMenuItemsForEdit(t *testing.T) {
	items := menuItemsFor("Edit")
	want := []string{"Cut", "Paste", "Copy", "Save"}
	if len(items) != len(want) {
		t.Fatalf("got %v", items)
	}
	for i, w := range want {
		if items[i] != w {
			t.Fatalf("got %v, want %v", items, want)
		}
	}
}

func TestCommandByNameFound(t *testing.T) {
	commands := buildCommands()
	cmd, ok := commandByName(commands, "Save")
	if !ok || cmd.Shortcut != "ctrl+s" {
		t.Fatalf("got %+v, ok=%v", cmd, ok)
	}
}

func TestClickFileLabelOpensDropdown(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.clickMenuLabel("File")
	if m.openMenu != "File" {
		t.Fatalf("got openMenu=%q", m.openMenu)
	}
}

func TestClickSameLabelTwiceCloses(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.clickMenuLabel("File")
	m, _ = m.clickMenuLabel("File")
	if m.openMenu != "" {
		t.Fatalf("got openMenu=%q, want closed", m.openMenu)
	}
}

func TestClickCommandsLabelIsANoOpStub(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.clickMenuLabel("Commands")
	if m.openMenu != "" {
		t.Fatal("Commands is a stub in phase 2, it must not open a dropdown")
	}
	if m.recentCommand == "" {
		t.Fatal("expected a status message acknowledging the click")
	}
}

func TestClickDropdownItemRunsCommand(t *testing.T) {
	dir := t.TempDir()
	file := dir + "/a.go"
	if err := writeFile(t, file, "hello"); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)
	m, _ = m.clickMenuLabel("Edit")
	// "Edit" starts at column 6 ("File" is 0-3, then a 2-space gap), so the
	// click's x must fall in Edit's column band [6, 6+dropdownWidth) or the
	// hit-test in handleClick will treat it as an outside click and close
	// the dropdown without running anything. Edit's items are Cut, Paste,
	// Copy, Save (rows 1-4 below the menu bar); row 4 ("Save") is at y=4
	// (row = y-1 = 3, items[3] = "Save").
	updated, cmd := m.handleClick(6, 4)
	m = updated.(Model)
	if m.openMenu != "" {
		t.Fatal("expected the dropdown to close after a click")
	}
	if cmd == nil {
		t.Fatal("expected clicking Save to produce a command")
	}
}
```

Add a tiny local test helper (this package doesn't have one yet for writing a file with error return; existing tests use `os.WriteFile` directly with `t.Fatal` inline — add this only if it's not already effectively duplicated, otherwise just inline `os.WriteFile` the way other tests in this package do):

```go
func writeFile(t *testing.T, path, content string) error {
	t.Helper()
	return os.WriteFile(path, []byte(content), 0644)
}
```
(Place this in `menu_test.go`; `os` is already imported by `model_test.go` in this package, so add `"os"` to `menu_test.go`'s own imports: `os`, `testing`, and `cody/internal/filetree` for `FileOpenedMsg`.)

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/... -run 'TestMenu|TestCommandByName|TestClick' -v`
Expected: FAIL to compile — `menuLabelAt`, `menuItemsFor`, `commandByName`, `clickMenuLabel`, `handleClick`, `openMenu` don't exist yet.

- [ ] **Step 3: Implement**

Create `internal/app/menu.go`:

```go
package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const dropdownWidth = 20

type menuLabel struct {
	name     string
	startCol int
	endCol   int
}

func menuLabels() []menuLabel {
	names := []string{"File", "Edit", "Commands", "About"}
	var out []menuLabel
	col := 0
	for _, name := range names {
		out = append(out, menuLabel{name: name, startCol: col, endCol: col + len(name)})
		col += len(name) + 2
	}
	return out
}

func menuLabelAt(col int) (string, bool) {
	for _, l := range menuLabels() {
		if col >= l.startCol && col < l.endCol {
			return l.name, true
		}
	}
	return "", false
}

func findLabel(name string) (menuLabel, bool) {
	for _, l := range menuLabels() {
		if l.name == name {
			return l, true
		}
	}
	return menuLabel{}, false
}

func menuItemsFor(menu string) []string {
	switch menu {
	case "File":
		return []string{"Open", "Save"}
	case "Edit":
		return []string{"Cut", "Paste", "Copy", "Save"}
	}
	return nil
}

func commandByName(commands []Command, name string) (Command, bool) {
	for _, c := range commands {
		if c.Name == name {
			return c, true
		}
	}
	return Command{}, false
}

func renderMenuBar(width int, openMenu string) string {
	var parts []string
	for _, l := range menuLabels() {
		text := l.name
		if l.name == openMenu {
			text = lipgloss.NewStyle().Reverse(true).Render(text)
		}
		parts = append(parts, text)
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(parts, "  "))
}

func renderDropdown(menu string, commands []Command) string {
	var lines []string
	for _, name := range menuItemsFor(menu) {
		shortcut := ""
		if cmd, ok := commandByName(commands, name); ok {
			shortcut = cmd.Shortcut
		}
		lines = append(lines, fmt.Sprintf("%-10s %s", name, shortcut))
	}
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}
```

Modify `internal/app/model.go`:

1. Add `openMenu string` to `Model`.
2. Add a `tea.MouseMsg` case to `Update` (place it as its own `case` in the outer `switch msg := msg.(type)`, alongside `tea.WindowSizeMsg`/`tea.KeyMsg`/etc.):

```go
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			return m.handleClick(msg.X, msg.Y)
		}
		return m, nil
```

3. Add an `"esc"` case to the `tea.KeyMsg` switch, before the registry `default`, so `Esc` closes an open dropdown (per spec §8: "clicking elsewhere or `Esc` closes without action"):

```go
		case "esc":
			if m.openMenu != "" {
				m.openMenu = ""
				return m, nil
			}
```

4. Add the click-handling methods:

```go
func (m Model) handleClick(x, y int) (Model, tea.Cmd) {
	if y == 0 {
		name, ok := menuLabelAt(x)
		if !ok {
			m.openMenu = ""
			return m, nil
		}
		return m.clickMenuLabel(name)
	}
	if m.openMenu == "" {
		return m, nil
	}
	label, ok := findLabel(m.openMenu)
	if !ok || x < label.startCol || x >= label.startCol+dropdownWidth {
		m.openMenu = ""
		return m, nil
	}
	items := menuItemsFor(m.openMenu)
	row := y - 1
	if row < 0 || row >= len(items) {
		m.openMenu = ""
		return m, nil
	}
	cmd, ok := commandByName(m.commands, items[row])
	m.openMenu = ""
	if !ok {
		return m, nil
	}
	return cmd.Handler(m)
}

func (m Model) clickMenuLabel(name string) (Model, tea.Cmd) {
	switch name {
	case "File", "Edit":
		if m.openMenu == name {
			m.openMenu = ""
		} else {
			m.openMenu = name
		}
	case "Commands":
		m.openMenu = ""
		m.recentCommand = "Command palette coming in a later phase"
	case "About":
		m.openMenu = ""
	}
	return m, nil
}
```

5. Update `View()` to use `renderMenuBar`, insert the dropdown as a pushed block, and shrink `paneHeight` by the dropdown's rendered height when one is open:

```go
func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}
	menuBar := renderMenuBar(m.width, m.openMenu)

	var dropdown string
	dropdownHeight := 0
	if m.openMenu != "" {
		dropdown = renderDropdown(m.openMenu, m.commands)
		dropdownHeight = lipgloss.Height(dropdown)
	}

	paneHeight := m.height - menuBarHeight - statusBarHeight - dropdownHeight
	editorHeight := paneHeight - terminalHeight

	treeBorderColor := unfocusedBorderColor
	editorBorderColor := unfocusedBorderColor
	if m.focus == focusTree {
		treeBorderColor = focusedBorderColor
	} else {
		editorBorderColor = focusedBorderColor
	}

	treeStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(treeBorderColor).
		Width(treeWidth - borderSize).
		Height(paneHeight - borderSize)
	editorStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(editorBorderColor).
		Width(m.width - treeWidth - borderSize).
		Height(editorHeight - borderSize)
	terminalStyle := lipgloss.NewStyle().Width(m.width - treeWidth).Height(terminalHeight)

	right := lipgloss.JoinVertical(lipgloss.Left,
		editorStyle.Render(m.editor.View()),
		terminalStyle.Render("Terminal (coming in a later phase)"),
	)
	body := lipgloss.JoinHorizontal(lipgloss.Top, treeStyle.Render(m.tree.View()), right)

	line, col := m.editor.Cursor()
	status := statusbar.Render(m.width, m.projectName, m.recentCommand, m.editor.Filetype(), line, col)

	sections := []string{menuBar}
	if dropdown != "" {
		sections = append(sections, dropdown)
	}
	sections = append(sections, body, status)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}
```

Modify `cmd/cody/main.go` to enable mouse reporting:

```go
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/app/... -v`
Expected: PASS — all new menu tests, plus every existing test in the package.

Run: `go build ./...`
Expected: clean (confirms `main.go`'s change compiles).

- [ ] **Step 5: Commit**

```bash
git add internal/app/menu.go internal/app/menu_test.go internal/app/model.go cmd/cody/main.go
git commit -m "feat: add mouse-driven File/Edit menu bar with dropdowns"
```

---

### Task 5: File > Open dialog

**Files:**
- Modify: `internal/app/model.go`
- Create: `internal/app/dialog.go`
- Test: `internal/app/dialog_test.go`

**Interfaces:**
- Consumes: `Command`, `m.commands`, `m.rootPath` (Task 1); `m.editor.LoadFile` (Phase 1, unchanged).
- Produces (package `cody/internal/app`):
  - `type dialogKind int` with `dialogNone`, `dialogFileOpen`, `dialogAbout`
  - `Model` gains `activeDialog dialogKind`, `fileOpenInput textinput.Model`, `fileOpenError string`
  - `func cmdOpenFilePrompt(m Model) (Model, tea.Cmd)` registered under `ctrl+o` and reachable via File > Open
  - `func (m Model) updateFileOpenDialog(msg tea.Msg) (tea.Model, tea.Cmd)`
  - `func renderFileOpenDialog(width, height int, ti textinput.Model, errMsg string) string`

- [ ] **Step 1: Fetch the new dependency**

```bash
go get github.com/charmbracelet/bubbles@latest
```

- [ ] **Step 2: Write the failing tests**

Create `internal/app/dialog_test.go`:

```go
package app

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCmdOpenFilePromptActivatesDialog(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = cmdOpenFilePrompt(m)
	if m.activeDialog != dialogFileOpen {
		t.Fatalf("got activeDialog=%v, want dialogFileOpen", m.activeDialog)
	}
}

func TestFileOpenDialogEnterLoadsRelativePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = cmdOpenFilePrompt(m)
	m.fileOpenInput.SetValue("a.go")
	updated, _ := m.updateFileOpenDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatal("expected the dialog to close after a successful open")
	}
	if !m.editor.HasBuffer() {
		t.Fatal("expected the editor to have a loaded buffer")
	}
	if m.focus != focusEditor {
		t.Fatal("expected focus to move to the editor")
	}
}

func TestFileOpenDialogEnterWithBadPathShowsErrorAndStaysOpen(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = cmdOpenFilePrompt(m)
	m.fileOpenInput.SetValue("does-not-exist.go")
	updated, _ := m.updateFileOpenDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.activeDialog != dialogFileOpen {
		t.Fatal("expected the dialog to stay open after a failed open")
	}
	if m.fileOpenError == "" {
		t.Fatal("expected an error message")
	}
}

func TestFileOpenDialogEscCloses(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = cmdOpenFilePrompt(m)
	updated, _ := m.updateFileOpenDialog(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatal("expected esc to close the dialog")
	}
}

func TestUpdateRoutesToFileOpenDialogWhenActive(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = updated.(Model)
	if m.activeDialog != dialogFileOpen {
		t.Fatal("expected ctrl+o to open the file-open dialog via the root Update")
	}
	// While the dialog is active, a plain rune key must go to the text input,
	// not to the tree/editor focus dispatch or the registry.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(Model)
	if m.fileOpenInput.Value() != "a" {
		t.Fatalf("got input value=%q, want %q", m.fileOpenInput.Value(), "a")
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/app/... -run 'TestCmdOpenFilePrompt|TestFileOpenDialog|TestUpdateRoutesToFileOpenDialog' -v`
Expected: FAIL to compile — `dialogKind`, `dialogFileOpen`, `cmdOpenFilePrompt`, `updateFileOpenDialog`, `fileOpenInput` don't exist yet.

- [ ] **Step 4: Implement**

Create `internal/app/dialog.go`:

```go
package app

import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type dialogKind int

const (
	dialogNone dialogKind = iota
	dialogFileOpen
	dialogAbout
)

func cmdOpenFilePrompt(m Model) (Model, tea.Cmd) {
	ti := textinput.New()
	ti.Placeholder = "path/to/file"
	ti.Focus()
	m.fileOpenInput = ti
	m.fileOpenError = ""
	m.activeDialog = dialogFileOpen
	return m, textinput.Blink
}

func (m Model) updateFileOpenDialog(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "esc":
		m.activeDialog = dialogNone
		return m, nil
	case "enter":
		path := m.fileOpenInput.Value()
		full := path
		if !filepath.IsAbs(path) {
			full = filepath.Join(m.rootPath, path)
		}
		editorModel, err := m.editor.LoadFile(full)
		if err != nil {
			m.fileOpenError = err.Error()
			return m, nil
		}
		m.editor = editorModel
		m.activeDialog = dialogNone
		m.focus = focusEditor
		m.recentCommand = fmt.Sprintf("Opened %s", filepath.Base(full))
		return m, nil
	}
	var cmd tea.Cmd
	m.fileOpenInput, cmd = m.fileOpenInput.Update(msg)
	return m, cmd
}

func (m Model) updateAboutDialog(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		m.activeDialog = dialogNone
	}
	return m, nil
}

func renderFileOpenDialog(width, height int, ti textinput.Model, errMsg string) string {
	content := "Open file:\n\n" + ti.View()
	if errMsg != "" {
		content += "\n\nError: " + errMsg
	}
	return lipgloss.NewStyle().Width(width).Height(height).Padding(1, 2).Render(content)
}

func renderAboutDialog(width, height int) string {
	content := "Cody — a terminal code editor\n\n" +
		"Keyboard shortcuts use Ctrl+ on every platform.\n" +
		"macOS Cmd+ shortcuts depend on your terminal emulator's own\n" +
		"keybinding settings and are not guaranteed to reach this app.\n\n" +
		"Press any key to close."
	return lipgloss.NewStyle().Width(width).Height(height).Padding(1, 2).Render(content)
}
```

Modify `internal/app/model.go`:

1. Add `activeDialog dialogKind`, `fileOpenInput textinput.Model`, `fileOpenError string` fields to `Model`. Add `"github.com/charmbracelet/bubbles/textinput"` to the imports.
2. Add `{Name: "Open", Shortcut: "ctrl+o", Handler: cmdOpenFilePrompt}` to `buildCommands()` in `commands.go` (this task extends Task 3's list).
3. At the very top of `Update`, right after the `switch msg := msg.(type)` opens and before the `tea.WindowSizeMsg` case — actually place it as a guard *before* the type switch entirely, since it must intercept every message type while active:

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.activeDialog == dialogFileOpen {
		return m.updateFileOpenDialog(msg)
	}
	if m.activeDialog == dialogAbout {
		return m.updateAboutDialog(msg)
	}
	switch msg := msg.(type) {
	// ... existing cases unchanged ...
```

(This means a `tea.WindowSizeMsg` arriving while a dialog is open won't update `m.width`/`m.height` — acceptable for this phase since dialogs are short-lived and a resize mid-dialog is a rare edge case, not a scenario the plan needs to handle.)

4. In `handleClick` (Task 4), guard against clicks while a dialog is active — add at the very top of `handleClick`:

```go
func (m Model) handleClick(x, y int) (Model, tea.Cmd) {
	if m.activeDialog != dialogNone {
		return m, nil
	}
	// ... rest unchanged ...
```

5. In `clickMenuLabel`, change the `"About"` case to actually open the dialog:

```go
	case "About":
		m.openMenu = ""
		m.activeDialog = dialogAbout
```

6. Update `View()` to render the active dialog instead of the normal body when one is active:

```go
func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}
	menuBar := renderMenuBar(m.width, m.openMenu)
	paneHeight := m.height - menuBarHeight - statusBarHeight
	line, col := m.editor.Cursor()
	status := statusbar.Render(m.width, m.projectName, m.recentCommand, m.editor.Filetype(), line, col)

	if m.activeDialog == dialogFileOpen {
		dialog := renderFileOpenDialog(m.width, paneHeight, m.fileOpenInput, m.fileOpenError)
		return lipgloss.JoinVertical(lipgloss.Left, menuBar, dialog, status)
	}
	if m.activeDialog == dialogAbout {
		dialog := renderAboutDialog(m.width, paneHeight)
		return lipgloss.JoinVertical(lipgloss.Left, menuBar, dialog, status)
	}

	var dropdown string
	dropdownHeight := 0
	if m.openMenu != "" {
		dropdown = renderDropdown(m.openMenu, m.commands)
		dropdownHeight = lipgloss.Height(dropdown)
	}

	bodyHeight := paneHeight - dropdownHeight
	editorHeight := bodyHeight - terminalHeight

	treeBorderColor := unfocusedBorderColor
	editorBorderColor := unfocusedBorderColor
	if m.focus == focusTree {
		treeBorderColor = focusedBorderColor
	} else {
		editorBorderColor = focusedBorderColor
	}

	treeStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(treeBorderColor).
		Width(treeWidth - borderSize).
		Height(bodyHeight - borderSize)
	editorStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(editorBorderColor).
		Width(m.width - treeWidth - borderSize).
		Height(editorHeight - borderSize)
	terminalStyle := lipgloss.NewStyle().Width(m.width - treeWidth).Height(terminalHeight)

	right := lipgloss.JoinVertical(lipgloss.Left,
		editorStyle.Render(m.editor.View()),
		terminalStyle.Render("Terminal (coming in a later phase)"),
	)
	body := lipgloss.JoinHorizontal(lipgloss.Top, treeStyle.Render(m.tree.View()), right)

	sections := []string{menuBar}
	if dropdown != "" {
		sections = append(sections, dropdown)
	}
	sections = append(sections, body, status)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}
```

(This replaces Task 4's `View()` — `paneHeight` is now computed once up front and reused, renamed to `bodyHeight` after the dropdown subtraction, to serve both the dialog-rendering early-returns and the normal path.)

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/app/... -v`
Expected: PASS — all new dialog tests, plus every existing test in the package.

Run: `go build ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add internal/app/dialog.go internal/app/dialog_test.go internal/app/model.go internal/app/commands.go go.mod go.sum
git commit -m "feat: add File > Open dialog with ctrl+o and menu-click entry points"
```

---

### Task 6: About dialog rendering path (menu-click already wired in Task 5)

**Files:**
- Test: `internal/app/dialog_test.go`

Task 5 already wired the About dialog's state transition (`clickMenuLabel("About")` sets `dialogAbout`) and its `View()`/`Update()` handling (`updateAboutDialog`, `renderAboutDialog`). This task's only job is to add the tests that were deferred out of Task 5 to keep that task's diff focused on File > Open, and to verify the About path end-to-end.

**Interfaces:**
- Consumes: `dialogAbout`, `updateAboutDialog`, `renderAboutDialog`, `clickMenuLabel` from Task 5 — no new production code.

- [ ] **Step 1: Write the tests**

Add to `internal/app/dialog_test.go`:

```go
func TestClickAboutLabelOpensAboutDialog(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.clickMenuLabel("About")
	if m.activeDialog != dialogAbout {
		t.Fatalf("got activeDialog=%v, want dialogAbout", m.activeDialog)
	}
}

func TestAboutDialogClosesOnAnyKey(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.clickMenuLabel("About")
	updated, _ := m.updateAboutDialog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatal("expected any key to close the About dialog")
	}
}

func TestUpdateRoutesToAboutDialogWhenActive(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.clickMenuLabel("About")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatal("expected the root Update to route to updateAboutDialog and close it")
	}
}
```

- [ ] **Step 2: Run the tests**

Run: `go test ./internal/app/... -run 'TestClickAboutLabel|TestAboutDialogClosesOnAnyKey|TestUpdateRoutesToAboutDialog' -v`
Expected: PASS immediately — Task 5 already implemented everything these tests exercise. If any of these fail, that means Task 5's About wiring has a gap; fix it in this task rather than skipping the test (this is TDD-after-the-fact for a path Task 5 built but didn't itself test).

- [ ] **Step 3: Run the whole package's tests**

Run: `go test ./internal/app/... -v`
Expected: PASS — everything so far.

- [ ] **Step 4: Commit**

```bash
git add internal/app/dialog_test.go
git commit -m "test: cover the About dialog's open/close/routing behavior"
```

---

### Task 7: Final integration test and README update

**Files:**
- Test: `internal/app/model_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: everything from Tasks 1-6 — no new production code, this task only adds one end-to-end test and documentation.

- [ ] **Step 1: Write the integration test**

Add to `internal/app/model_test.go`:

```go
func TestMouseClickFileOpenThenTypeThenLoadsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "target.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// Click "File" (columns 0-3, row 0).
	updated, _ = m.Update(tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)
	if m.openMenu != "File" {
		t.Fatalf("got openMenu=%q, want File", m.openMenu)
	}

	// Click "Open" (row 1, first item in the File dropdown).
	updated, _ = m.Update(tea.MouseMsg{X: 0, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)
	if m.activeDialog != dialogFileOpen {
		t.Fatal("expected clicking Open to activate the file-open dialog")
	}

	// Type the path and confirm.
	for _, r := range "target.go" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.activeDialog != dialogNone {
		t.Fatal("expected the dialog to close after a successful open")
	}
	if !m.editor.HasBuffer() {
		t.Fatal("expected the editor to have loaded target.go")
	}
	if m.focus != focusEditor {
		t.Fatal("expected focus on the editor")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails, then passes**

Run: `go test ./internal/app/... -run TestMouseClickFileOpenThenTypeThenLoadsFile -v`

If it fails, read the failure carefully — this test exercises the exact composition of Tasks 4 and 5 (mouse click → dropdown → dialog → text input → load), so a failure here means a real integration gap between those tasks' pieces, not a typo in the test. Fix the production code, not the test, unless the test itself has a mistaken assumption about row/column math (cross-check against `menuLabels()` and `menuItemsFor("File")` if so).

Expected once correct: PASS.

- [ ] **Step 3: Run the whole test suite**

Run: `go test ./... -v`
Expected: PASS — every test in every package (`app`, `editor`, `filetree`, `statusbar`).

Run: `go build ./...` and `go vet ./...`
Expected: both clean.

Run: `gofmt -l .`
Expected: no output.

- [ ] **Step 4: Update the README**

Modify `README.md`'s "Status" and "Keybindings" sections:

```markdown
## Status

Phase 2 (command registry + core menu) of a phased build — see
`docs/superpowers/specs/2026-09-05-cody-tui-editor-design.md` for the full
design and `docs/superpowers/plans/` for phase-by-phase implementation plans.

This phase adds: a command registry as the single dispatch path for every
shortcut, a mouse-driven File/Edit menu bar with dropdowns, an About dialog,
Cut/Copy/Paste with an internal clipboard and Shift+Arrow selection, and
snapshot-based undo/redo. It does not yet have: the Commands menu's command
palette (clicking it shows a placeholder status message), syntax
highlighting/folding, an embedded shell, or in-buffer search — those land in
later phases.

## Keybindings (phase 2)

- `Tab` / `Shift+Tab` — switch focus between the project tree and the editor
- Project tree: arrows or `hjkl` to navigate, `Enter`/`l` to open a file or
  toggle a directory's expand/collapse state, `h` to collapse
- Editor: arrows to move the cursor, typing inserts text, `Enter` for a
  newline, `Backspace` to delete, `Shift+Arrow` to select text
- `Ctrl+O` open (or click File > Open), `Ctrl+S` save
- `Ctrl+X`/`Ctrl+C`/`Ctrl+V` cut/copy/paste (operates on the selection, or the
  whole current line if nothing is selected)
- `Ctrl+Z`/`Ctrl+Y` undo/redo
- `Ctrl+Q` — quit
- Menu bar: click `File`/`Edit` to open a dropdown, click an item to run it,
  `Esc` or clicking elsewhere closes it; click `About` for app info; click
  `Commands` — not implemented yet (Phase 3)
- Shortcuts are `Ctrl+`-based on every platform — macOS `Cmd+` shortcuts
  depend on your terminal emulator's own keybinding configuration and are not
  guaranteed to reach this app (see the About dialog)
```

- [ ] **Step 5: Commit**

```bash
git add internal/app/model_test.go README.md
git commit -m "test: add mouse-to-dialog integration test; docs: update README for phase 2"
```
