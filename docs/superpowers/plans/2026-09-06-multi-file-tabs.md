# Multi-File Tabs, Unsaved-Changes Confirmation, Dirty Indicator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the editor hold multiple open files as mouse-switchable, mouse-closable tabs; warn before discarding unsaved changes on quit or tab-close; show a modified indicator in the project tree.

**Architecture:** `app.Model` replaces its single `editor editor.Model` field with `tabs []tab` (`tab{path string; editor editor.Model}`) + `activeTab int`, accessed everywhere through two new helper methods (`activeEditor()` / `setActiveEditor()`) so the rest of the app's logic barely changes. A shared confirmation dialog (`dialogConfirmDiscard`) gates both quitting and closing a dirty tab. The project tree gets a `dirty map[string]bool` recomputed from the tabs on every render, matching the existing pattern of re-deriving pane sizes fresh each frame.

**Tech Stack:** Go, Bubble Tea, Lip Gloss (no new dependencies).

**Spec:** `docs/superpowers/specs/2026-09-06-multi-file-tabs-design.md`

## Global Constraints

- No new third-party dependencies.
- Single-file behavior (open/edit/save/search/fold/mouse click/scroll) must be unchanged — this plan adds a tab layer around `editor.Model`, it does not modify `editor.Model`'s own internals (except adding one new read-only method, Task 1).
- Every existing test must keep passing; call sites broken by the `m.editor` → `m.tabs`/`activeTab` refactor (Task 3) are fixed, never deleted.
- Tab close and quit confirmation share one dialog implementation (`dialogConfirmDiscard` / `confirm.go`) — no duplicated confirm/cancel logic.
- Run `go build ./... && go test ./...` after every task; it must be green before moving to the next task.

---

### Task 1: `editor.Model.HasUnsavedChanges`

**Files:**
- Modify: `internal/editor/model.go`
- Test: `internal/editor/model_test.go`

**Interfaces:**
- Produces: `func (m Model) HasUnsavedChanges() bool` — later tasks (in `app`) use this to detect dirty tabs without reaching into `editor.Model`'s unexported `buf` field.

- [ ] **Step 1: Write the failing tests**

Add to `internal/editor/model_test.go` (anywhere after `setupEditor`'s definition):

```go
func TestHasUnsavedChangesFalseOnFreshLoad(t *testing.T) {
	m := setupEditor(t, "hello\n")
	if m.HasUnsavedChanges() {
		t.Fatal("expected a freshly loaded file to have no unsaved changes")
	}
}

func TestHasUnsavedChangesTrueAfterEdit(t *testing.T) {
	m := setupEditor(t, "hello\n")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if !m.HasUnsavedChanges() {
		t.Fatal("expected an edit to mark the buffer as having unsaved changes")
	}
}

func TestHasUnsavedChangesFalseAfterSave(t *testing.T) {
	m := setupEditor(t, "hello\n")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.HasUnsavedChanges() {
		t.Fatal("expected saving to clear unsaved changes")
	}
}

func TestHasUnsavedChangesFalseWithNoBuffer(t *testing.T) {
	m := New()
	if m.HasUnsavedChanges() {
		t.Fatal("expected no unsaved changes before any file is loaded")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/editor/ -run TestHasUnsavedChanges -v`
Expected: FAIL with `m.HasUnsavedChanges undefined`.

- [ ] **Step 3: Implement `HasUnsavedChanges`**

In `internal/editor/model.go`, add directly after the existing `HasBuffer` method:

```go
func (m Model) HasBuffer() bool {
	return m.buf != nil
}

// HasUnsavedChanges reports whether the current buffer has unsaved edits.
// False when no file is loaded.
func (m Model) HasUnsavedChanges() bool {
	return m.buf != nil && m.buf.Dirty
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/editor/ -run TestHasUnsavedChanges -v`
Expected: PASS (all 4).

- [ ] **Step 5: Run the full editor package test suite**

Run: `go test ./internal/editor/...`
Expected: PASS, no regressions.

- [ ] **Step 6: Commit**

```bash
git add internal/editor/model.go internal/editor/model_test.go
git commit -m "feat: add editor.Model.HasUnsavedChanges"
```

---

### Task 2: `filetree.Model.SetDirty` + modified indicator in the tree

**Files:**
- Modify: `internal/filetree/model.go`
- Test: `internal/filetree/model_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `func (m Model) SetDirty(dirty map[string]bool) Model` — Task 5 calls this from `app.Model.View()`, keyed by each open tab's absolute path (matching `Node.Path`, which is already absolute).

- [ ] **Step 1: Write the failing tests**

Add to `internal/filetree/model_test.go`:

```go
func TestSetDirtyShowsModifiedIndicator(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.go"), "")
	mustWriteFile(t, filepath.Join(dir, "b.go"), "")
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	dirtyPath := m.flat[0].node.Path
	m = m.SetDirty(map[string]bool{dirtyPath: true})

	view := m.View()
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[0], "(M)") {
		t.Fatalf("expected %q to show a modified indicator, got line %q", dirtyPath, lines[0])
	}
	if strings.Contains(lines[1], "(M)") {
		t.Fatalf("expected the clean file to show no modified indicator, got line %q", lines[1])
	}
}

func TestSetDirtyWithEmptyMapShowsNoIndicators(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	view := m.View()
	if strings.Contains(view, "(M)") {
		t.Fatal("expected no modified indicators when SetDirty was never called")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/filetree/ -run TestSetDirty -v`
Expected: FAIL with `m.SetDirty undefined`.

- [ ] **Step 3: Implement `SetDirty` and the indicator**

In `internal/filetree/model.go`, add a `dirty` field to `Model`:

```go
type Model struct {
	root         *Node
	nerdFont     bool
	flat         []flatItem
	cursor       int
	width        int
	height       int
	scrollOffset int
	dirty        map[string]bool
}
```

Add a `dirtyStyle` next to the existing `scrollbarStyle`:

```go
var scrollbarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
var dirtyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
```

Add `SetDirty` next to `SetSize`:

```go
func (m Model) SetSize(width, height int) Model {
	m.width, m.height = width, height
	m.ensureCursorVisible()
	return m
}

// SetDirty marks which paths (absolute, matching Node.Path) have unsaved
// changes, so View() can show a modified indicator next to their name.
func (m Model) SetDirty(dirty map[string]bool) Model {
	m.dirty = dirty
	return m
}
```

In `View()`, change the line-building block from:

```go
		item := m.flat[idx]
		prefix := strings.Repeat("  ", item.depth)
		icon := IconFor(item.node, m.nerdFont)
		line := fmt.Sprintf("%s%s %s", prefix, icon, item.node.Name)
		if idx == m.cursor {
```

to:

```go
		item := m.flat[idx]
		prefix := strings.Repeat("  ", item.depth)
		icon := IconFor(item.node, m.nerdFont)
		line := fmt.Sprintf("%s%s %s", prefix, icon, item.node.Name)
		if m.dirty[item.node.Path] {
			line += dirtyStyle.Render(" (M)")
		}
		if idx == m.cursor {
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/filetree/ -run TestSetDirty -v`
Expected: PASS (both).

- [ ] **Step 5: Run the full filetree package test suite**

Run: `go test ./internal/filetree/...`
Expected: PASS, no regressions.

- [ ] **Step 6: Commit**

```bash
git add internal/filetree/model.go internal/filetree/model_test.go
git commit -m "feat: add filetree.Model.SetDirty and modified indicator"
```

---

### Task 3: `app` tabs/activeTab data model refactor + open-or-switch

This task replaces `app.Model`'s single `editor editor.Model` field with
`tabs []tab` + `activeTab int`, and updates every call site that touched
`m.editor` (production code and tests) to use two new helper methods
instead. It also changes file-opening (from the tree, and from the Ctrl+O
prompt) to open-or-switch semantics: opening an already-open path switches
to its tab instead of creating a duplicate. No tab bar UI or closing yet —
this task is purely the data-model refactor; behavior for a single open
file must be unchanged.

**Files:**
- Modify: `internal/app/model.go`
- Modify: `internal/app/commands.go`
- Modify: `internal/app/dialog.go`
- Modify: `internal/app/model_test.go`
- Modify: `internal/app/dialog_test.go`
- Modify: `internal/app/commands_test.go`

**Interfaces:**
- Consumes: `editor.Model.HasUnsavedChanges` (Task 1) — not used yet in this task, but the `tab` struct's `editor` field is what Tasks 4/5 will call it on.
- Produces: `type tab struct { path string; editor editor.Model }`, `func (m Model) activeEditor() editor.Model`, `func (m Model) setActiveEditor(e editor.Model) Model`, `func (m Model) openOrSwitch(path string) (Model, error)` — Tasks 4 and 5 build directly on these.

- [ ] **Step 1: Update the `Model` struct and `New`**

In `internal/app/model.go`, replace:

```go
type Model struct {
	tree          filetree.Model
	editor        editor.Model
	terminal      terminal.Model
	focus         focusArea
	projectName   string
	recentCommand string
	width, height int
	commands      []Command
	rootPath      string
	openMenu      string
	activeDialog  dialogKind
	fileOpenInput textinput.Model
	fileOpenError string
	paletteFilter textinput.Model
	paletteCursor int
	searchInput   textinput.Model
}

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
		terminal:    terminal.New(),
		focus:       focusTree,
		projectName: filepath.Base(absPath),
		rootPath:    absPath,
		commands:    buildCommands(),
	}, nil
}
```

with:

```go
// tab is one open file: its absolute path (the key used to detect an
// already-open file and to avoid duplicate tabs) and its own independent
// editor state (cursor, undo history, scroll position, folds, ...).
type tab struct {
	path   string
	editor editor.Model
}

type Model struct {
	tree          filetree.Model
	tabs          []tab
	activeTab     int // -1 when no tabs are open
	terminal      terminal.Model
	focus         focusArea
	projectName   string
	recentCommand string
	width, height int
	commands      []Command
	rootPath      string
	openMenu      string
	activeDialog  dialogKind
	fileOpenInput textinput.Model
	fileOpenError string
	paletteFilter textinput.Model
	paletteCursor int
	searchInput   textinput.Model
}

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
		activeTab:   -1,
		terminal:    terminal.New(),
		focus:       focusTree,
		projectName: filepath.Base(absPath),
		rootPath:    absPath,
		commands:    buildCommands(),
	}, nil
}

// activeEditor returns the active tab's editor, or a zero-value
// editor.Model (HasBuffer() == false, matching "no file open yet") when no
// tabs are open.
func (m Model) activeEditor() editor.Model {
	if m.activeTab < 0 || m.activeTab >= len(m.tabs) {
		return editor.Model{}
	}
	return m.tabs[m.activeTab].editor
}

// setActiveEditor writes e back into the active tab. A no-op when no tabs
// are open (mirrors activeEditor's zero-value fallback).
func (m Model) setActiveEditor(e editor.Model) Model {
	if m.activeTab >= 0 && m.activeTab < len(m.tabs) {
		m.tabs[m.activeTab].editor = e
	}
	return m
}

// openOrSwitch opens path in a new tab, or switches to its existing tab if
// one is already open for that path — never creates a duplicate. On
// failure to load a new file, m is returned unchanged (matching the
// previous single-tab LoadFile-failure behavior) along with the error.
func (m Model) openOrSwitch(path string) (Model, error) {
	for i, t := range m.tabs {
		if t.path == path {
			m.activeTab = i
			return m, nil
		}
	}
	editorModel, err := editor.New().LoadFile(path)
	if err != nil {
		return m, err
	}
	m.tabs = append(m.tabs, tab{path: path, editor: editorModel})
	m.activeTab = len(m.tabs) - 1
	return m, nil
}
```

- [ ] **Step 2: Update `Update`'s `WindowSizeMsg`, `RehighlightMsg`, `FileOpenedMsg`, and focus-dispatch branches**

Replace:

```go
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		paneHeight := m.height - menuBarHeight - statusBarHeight
		editorHeight := paneHeight - terminalHeight
		m.tree = m.tree.SetSize(treeWidth-borderSize, paneHeight-borderSize)
		m.editor = m.editor.SetSize(m.width-treeWidth-borderSize, editorHeight-borderSize)
		m.terminal = m.terminal.SetSize(m.width-treeWidth-borderSize, terminalHeight-borderSize)
		return m, nil
	}
	if _, ok := msg.(editor.RehighlightMsg); ok {
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
		return m, cmd
	}
```

with:

```go
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		paneHeight := m.height - menuBarHeight - statusBarHeight
		editorHeight := paneHeight - terminalHeight
		m.tree = m.tree.SetSize(treeWidth-borderSize, paneHeight-borderSize)
		m = m.setActiveEditor(m.activeEditor().SetSize(m.width-treeWidth-borderSize, editorHeight-borderSize))
		m.terminal = m.terminal.SetSize(m.width-treeWidth-borderSize, terminalHeight-borderSize)
		return m, nil
	}
	if _, ok := msg.(editor.RehighlightMsg); ok {
		e, cmd := m.activeEditor().Update(msg)
		m = m.setActiveEditor(e)
		return m, cmd
	}
```

(This step leaves the editor-height formula exactly as it was — no tab bar
row yet. Task 5 adds the tab-bar-height deduction.)

Replace the `FileOpenedMsg` case:

```go
	case filetree.FileOpenedMsg:
		editorModel, err := m.editor.LoadFile(msg.Path)
		if err != nil {
			m.recentCommand = fmt.Sprintf("Open failed: %s", err)
			return m, nil
		}
		m.editor = editorModel
		m.focus = focusEditor
		m.recentCommand = fmt.Sprintf("Opened %s", filepath.Base(msg.Path))
		return m, nil
```

with:

```go
	case filetree.FileOpenedMsg:
		updated, err := m.openOrSwitch(msg.Path)
		if err != nil {
			m.recentCommand = fmt.Sprintf("Open failed: %s", err)
			return m, nil
		}
		m = updated
		m.focus = focusEditor
		m.recentCommand = fmt.Sprintf("Opened %s", filepath.Base(msg.Path))
		return m, nil
```

Replace the bottom focus-dispatch switch:

```go
	var cmd tea.Cmd
	switch m.focus {
	case focusTree:
		m.tree, cmd = m.tree.Update(msg)
	case focusEditor:
		m.editor, cmd = m.editor.Update(msg)
	case focusTerminal:
		m.terminal, cmd = m.terminal.Update(msg)
	}
	return m, cmd
```

with:

```go
	var cmd tea.Cmd
	switch m.focus {
	case focusTree:
		m.tree, cmd = m.tree.Update(msg)
	case focusEditor:
		e, c := m.activeEditor().Update(msg)
		m = m.setActiveEditor(e)
		cmd = c
	case focusTerminal:
		m.terminal, cmd = m.terminal.Update(msg)
	}
	return m, cmd
```

- [ ] **Step 3: Update `handlePaneClick` and `handleWheel`'s editor cases**

Replace:

```go
	case editorRect.contains(x, y):
		m.focus = focusEditor
		relX := x - editorRect.x0 - 1
		relY := y - editorRect.y0 - 1
		var cmd tea.Cmd
		m.editor, cmd = m.editor.HandleClick(relX, relY)
		return m, cmd
```

with:

```go
	case editorRect.contains(x, y):
		m.focus = focusEditor
		relX := x - editorRect.x0 - 1
		relY := y - editorRect.y0 - 1
		e, cmd := m.activeEditor().HandleClick(relX, relY)
		m = m.setActiveEditor(e)
		return m, cmd
```

Replace:

```go
	case editorRect.contains(x, y):
		m.editor = m.editor.ScrollLines(delta)
```

with:

```go
	case editorRect.contains(x, y):
		m = m.setActiveEditor(m.activeEditor().ScrollLines(delta))
```

- [ ] **Step 4: Update `View()`'s editor references**

Replace:

```go
	line, col := m.editor.Cursor()
	status := statusbar.Render(m.width, m.projectName, m.recentCommand, m.editor.Filetype(), line, col)
```

with:

```go
	line, col := m.activeEditor().Cursor()
	status := statusbar.Render(m.width, m.projectName, m.recentCommand, m.activeEditor().Filetype(), line, col)
```

Replace:

```go
	editor := m.editor.SetSize(m.width-treeWidth-borderSize, editorHeight-borderSize)
```

with:

```go
	editor := m.activeEditor().SetSize(m.width-treeWidth-borderSize, editorHeight-borderSize)
```

- [ ] **Step 5: Update `internal/app/commands.go`**

Replace the six editor-key-forwarding commands:

```go
func cmdSave(m Model) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	return m, cmd
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

func cmdToggleFold(m Model) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	return m, cmd
}
```

with:

```go
func cmdSave(m Model) (Model, tea.Cmd) {
	e, cmd := m.activeEditor().Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = m.setActiveEditor(e)
	return m, cmd
}

func cmdCut(m Model) (Model, tea.Cmd) {
	e, cmd := m.activeEditor().Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	m = m.setActiveEditor(e)
	return m, cmd
}

func cmdCopy(m Model) (Model, tea.Cmd) {
	e, cmd := m.activeEditor().Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = m.setActiveEditor(e)
	return m, cmd
}

func cmdPaste(m Model) (Model, tea.Cmd) {
	e, cmd := m.activeEditor().Update(tea.KeyMsg{Type: tea.KeyCtrlV})
	m = m.setActiveEditor(e)
	return m, cmd
}

func cmdUndo(m Model) (Model, tea.Cmd) {
	e, cmd := m.activeEditor().Update(tea.KeyMsg{Type: tea.KeyCtrlZ})
	m = m.setActiveEditor(e)
	return m, cmd
}

func cmdRedo(m Model) (Model, tea.Cmd) {
	e, cmd := m.activeEditor().Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	m = m.setActiveEditor(e)
	return m, cmd
}

func cmdToggleFold(m Model) (Model, tea.Cmd) {
	e, cmd := m.activeEditor().Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m = m.setActiveEditor(e)
	return m, cmd
}
```

`cmdQuit` is untouched in this task (it doesn't reference `m.editor`; Task 4
changes it).

- [ ] **Step 6: Update `internal/app/dialog.go`**

Replace the `updateFileOpenDialog`'s `"enter"` case:

```go
	case "enter":
		m.fileOpenError = ""
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
```

with:

```go
	case "enter":
		m.fileOpenError = ""
		path := m.fileOpenInput.Value()
		full := path
		if !filepath.IsAbs(path) {
			full = filepath.Join(m.rootPath, path)
		}
		updated, err := m.openOrSwitch(full)
		if err != nil {
			m.fileOpenError = err.Error()
			return m, nil
		}
		m = updated
		m.activeDialog = dialogNone
		m.focus = focusEditor
		m.recentCommand = fmt.Sprintf("Opened %s", filepath.Base(full))
		return m, nil
```

Replace `cmdFind`:

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
```

with:

```go
func cmdFind(m Model) (Model, tea.Cmd) {
	if !m.activeEditor().HasBuffer() {
		return m, func() tea.Msg { return editor.CommandExecutedMsg{Description: "No file open"} }
	}
	ti := textinput.New()
	ti.Placeholder = "search"
	ti.Focus()
	m.searchInput = ti
	m = m.setActiveEditor(m.activeEditor().StartSearch())
	m.activeDialog = dialogSearch
	m.recentCommand = ""
	return m, textinput.Blink
}
```

Replace `updateSearchDialog`:

```go
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
```

with:

```go
func (m Model) updateSearchDialog(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}
	switch keyMsg.String() {
	case "esc":
		m = m.setActiveEditor(m.activeEditor().ClearSearch())
		m.activeDialog = dialogNone
		return m, nil
	case "enter":
		e, status := m.activeEditor().FindNext()
		m = m.setActiveEditor(e)
		m.recentCommand = status
		return m, nil
	case "shift+enter":
		e, status := m.activeEditor().FindPrev()
		m = m.setActiveEditor(e)
		m.recentCommand = status
		return m, nil
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	e, status := m.activeEditor().SetSearchQuery(m.searchInput.Value())
	m = m.setActiveEditor(e)
	m.recentCommand = status
	return m, cmd
}
```

- [ ] **Step 7: Fix every `.editor` reference in test files**

These are the only test-file lines that reference the removed `m.editor`
field (found via `grep -rn "\.editor\b" internal/app/*_test.go`). Each is a
mechanical rename from a field access to a method call — `X.editor.Y`
becomes `X.activeEditor().Y`:

- `internal/app/dialog_test.go:42`: `if !m.editor.HasBuffer() {` →
  `if !m.activeEditor().HasBuffer() {`
- `internal/app/dialog_test.go:217`: `if strings.Contains(m.editor.View(), "\x1b[48;5;220m") {` →
  `if strings.Contains(m.activeEditor().View(), "\x1b[48;5;220m") {`
- `internal/app/commands_test.go:127`: `if mA.editor.HasBuffer() != mB.editor.HasBuffer() {` →
  `if mA.activeEditor().HasBuffer() != mB.activeEditor().HasBuffer() {`
- `internal/app/model_test.go:52`: `if !m.editor.HasBuffer() {` →
  `if !m.activeEditor().HasBuffer() {`
- `internal/app/model_test.go:125`: `if m.focus != focusEditor || !m.editor.HasBuffer() {` →
  `if m.focus != focusEditor || !m.activeEditor().HasBuffer() {`
- `internal/app/model_test.go:183`: `if !m.editor.HasBuffer() {` →
  `if !m.activeEditor().HasBuffer() {`
- `internal/app/model_test.go:353`: `view := m.editor.View()` →
  `view := m.activeEditor().View()`
- `internal/app/model_test.go:452`: (comment) `// multi-line block (editorStyle.Render(m.editor.View())) — if any single` →
  `// multi-line block (editorStyle.Render(m.activeEditor().View())) — if any single`
- `internal/app/model_test.go:374`: `if !m.editor.HasBuffer() {` →
  `if !m.activeEditor().HasBuffer() {`
- `internal/app/model_test.go:711`: `line, _ := m.editor.Cursor()` →
  `line, _ := m.activeEditor().Cursor()`
- `internal/app/model_test.go:718`: `line, _ = m.editor.Cursor()` →
  `line, _ = m.activeEditor().Cursor()`
- `internal/app/model_test.go:792`: `line, col := m.editor.Cursor()` →
  `line, col := m.activeEditor().Cursor()`

- [ ] **Step 8: Run the full app package test suite**

Run: `go test ./internal/app/...`
Expected: PASS, no regressions — this confirms the refactor preserved
single-tab behavior exactly.

- [ ] **Step 9: Write and run new tests for open-or-switch semantics**

Add to `internal/app/model_test.go`:

```go
func TestOpeningTwoDifferentFilesCreatesTwoTabs(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.go")
	fileB := filepath.Join(dir, "b.go")
	if err := os.WriteFile(fileA, []byte("package a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte("package b"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: fileA})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileB})
	m = updated.(Model)

	if len(m.tabs) != 2 {
		t.Fatalf("got %d tabs, want 2", len(m.tabs))
	}
	if m.activeTab != 1 {
		t.Fatalf("got activeTab=%d, want 1 (the most recently opened)", m.activeTab)
	}
}

func TestOpeningAnAlreadyOpenFileSwitchesInsteadOfDuplicating(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.go")
	fileB := filepath.Join(dir, "b.go")
	if err := os.WriteFile(fileA, []byte("package a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte("package b"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: fileA})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileB})
	m = updated.(Model)
	// Move the cursor in fileB's tab so we can confirm re-opening fileA and
	// coming back to fileB preserves this, rather than reloading it.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	lineBeforeReopen, _ := m.activeEditor().Cursor()

	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileA})
	m = updated.(Model)
	if len(m.tabs) != 2 {
		t.Fatalf("got %d tabs, want 2 (re-opening fileA must not duplicate it)", len(m.tabs))
	}
	if m.activeTab != 0 {
		t.Fatalf("got activeTab=%d, want 0 (fileA's existing tab)", m.activeTab)
	}

	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileB})
	m = updated.(Model)
	if m.activeTab != 1 {
		t.Fatalf("got activeTab=%d, want 1 (back to fileB's existing tab)", m.activeTab)
	}
	lineAfterReturn, _ := m.activeEditor().Cursor()
	if lineAfterReturn != lineBeforeReopen {
		t.Fatalf("got cursor line=%d, want %d — switching back to an already-open tab must preserve its state, not reload it", lineAfterReturn, lineBeforeReopen)
	}
}
```

Run: `go test ./internal/app/ -run TestOpening -v`
Expected: PASS (both).

- [ ] **Step 10: Run the full test suite**

Run: `go build ./... && go test ./...`
Expected: PASS across every package.

- [ ] **Step 11: Commit**

```bash
git add internal/app/model.go internal/app/commands.go internal/app/dialog.go \
        internal/app/model_test.go internal/app/dialog_test.go internal/app/commands_test.go
git commit -m "refactor: replace app.Model.editor with tabs/activeTab; add open-or-switch"
```

---

### Task 4: Tab closing + unsaved-changes confirmation dialog

Implements closing a tab and quitting the app, both gated by a shared
confirmation dialog when there are unsaved changes. This task has no
mouse/tab-bar UI yet — `closeTab` and the dialog are fully testable by
calling them directly and by driving `Update()` with synthetic messages.
Task 5 wires mouse clicks on the tab bar's close glyph to `closeTab`.

**Files:**
- Modify: `internal/app/dialog.go` (add `dialogConfirmDiscard` to the `dialogKind` enum)
- Modify: `internal/app/model.go` (add dialog-state fields, wire `updateConfirmDialog`/`renderConfirmDialog` into `Update`/`View`)
- Modify: `internal/app/commands.go` (`cmdQuit` gains the dirty check)
- Create: `internal/app/confirm.go`
- Test: `internal/app/confirm_test.go`

**Interfaces:**
- Consumes: `tab.editor.HasUnsavedChanges()` (Task 1), `m.tabs`/`m.activeTab` (Task 3).
- Produces: `func (m Model) closeTab(index int) (Model, tea.Cmd)` — Task 5's tab-bar close-glyph click handler calls this directly.

- [ ] **Step 1: Add `dialogConfirmDiscard` to the dialog-kind enum**

In `internal/app/dialog.go`, replace:

```go
const (
	dialogNone dialogKind = iota
	dialogFileOpen
	dialogAbout
	dialogPalette
	dialogSearch
)
```

with:

```go
const (
	dialogNone dialogKind = iota
	dialogFileOpen
	dialogAbout
	dialogPalette
	dialogSearch
	dialogConfirmDiscard
)
```

- [ ] **Step 2: Add the confirmation-dialog state fields to `Model`**

In `internal/app/model.go`, add three fields to the `Model` struct (after
`searchInput`):

```go
	searchInput   textinput.Model

	pendingConfirm    confirmAction
	pendingConfirmTab int // meaningful only when pendingConfirm == confirmCloseTab
	confirmCursor     int // 0 = "anyway", 1 = "Cancel"
```

- [ ] **Step 3: Write the failing tests**

Create `internal/app/confirm_test.go`:

```go
package app

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"cody/internal/filetree"
)

func openAndDirtyFile(t *testing.T, m Model, path string) Model {
	t.Helper()
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: path})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	return updated.(Model)
}

func TestCloseTabOnCleanTabClosesImmediately(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package a"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)

	m, _ = m.closeTab(0)

	if len(m.tabs) != 0 {
		t.Fatalf("got %d tabs, want 0", len(m.tabs))
	}
	if m.activeTab != -1 {
		t.Fatalf("got activeTab=%d, want -1", m.activeTab)
	}
	if m.focus != focusTree {
		t.Fatal("expected focus to fall back to the tree once the last tab closes")
	}
	if m.activeDialog != dialogNone {
		t.Fatal("expected no confirmation dialog for a clean tab")
	}
}

func TestCloseTabReassignsActiveTabCorrectly(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("package p"), 0644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		updated, _ := m.Update(filetree.FileOpenedMsg{Path: f})
		m = updated.(Model)
	}
	// 3 tabs open, activeTab == 2 (c.go).

	// Closing a tab before the active one shifts activeTab left by one.
	m, _ = m.closeTab(0)
	if m.activeTab != 1 {
		t.Fatalf("got activeTab=%d, want 1 after closing a tab before it", m.activeTab)
	}

	// Now 2 tabs remain (b.go, c.go), activeTab == 1 (c.go, the last one).
	// Closing the active (last) tab falls back to the one before it.
	m, _ = m.closeTab(1)
	if m.activeTab != 0 {
		t.Fatalf("got activeTab=%d, want 0 after closing the active last tab", m.activeTab)
	}
}

func TestCloseTabOnDirtyTabOpensConfirmDialog(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package a"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m = openAndDirtyFile(t, m, file)

	m, _ = m.closeTab(0)

	if m.activeDialog != dialogConfirmDiscard {
		t.Fatal("expected closing a dirty tab to open the confirm dialog")
	}
	if m.pendingConfirm != confirmCloseTab {
		t.Fatalf("got pendingConfirm=%v, want confirmCloseTab", m.pendingConfirm)
	}
	if m.pendingConfirmTab != 0 {
		t.Fatalf("got pendingConfirmTab=%d, want 0", m.pendingConfirmTab)
	}
	if len(m.tabs) != 1 {
		t.Fatal("expected the tab to remain open until confirmed")
	}
}

func TestConfirmDialogConfirmingCloseTabRemovesIt(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package a"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m = openAndDirtyFile(t, m, file)
	m, _ = m.closeTab(0)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if len(m.tabs) != 0 {
		t.Fatal("expected confirming to remove the tab")
	}
	if m.activeDialog != dialogNone {
		t.Fatal("expected the dialog to close after confirming")
	}
}

func TestConfirmDialogCancelingLeavesTabOpen(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package a"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m = openAndDirtyFile(t, m, file)
	m, _ = m.closeTab(0)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if len(m.tabs) != 1 {
		t.Fatal("expected canceling to leave the tab open")
	}
	if m.activeDialog != dialogNone {
		t.Fatal("expected the dialog to close after canceling")
	}
}

func TestQuitWithNoDirtyTabsQuitsImmediately(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected tea.Quit with no dirty tabs")
	}
	if m.activeDialog != dialogNone {
		t.Fatal("expected no confirmation dialog with no dirty tabs")
	}
}

func TestQuitWithADirtyTabOpensConfirmDialog(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package a"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m = openAndDirtyFile(t, m, file)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m = updated.(Model)

	if m.activeDialog != dialogConfirmDiscard {
		t.Fatal("expected ctrl+q with a dirty tab to open the confirm dialog instead of quitting")
	}
	if m.pendingConfirm != confirmQuit {
		t.Fatalf("got pendingConfirm=%v, want confirmQuit", m.pendingConfirm)
	}
	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatal("expected ctrl+q with a dirty tab not to quit yet")
		}
	}
}

func TestConfirmDialogConfirmingQuitProducesTeaQuit(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package a"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m = openAndDirtyFile(t, m, file)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected confirming the quit dialog to produce tea.Quit")
	}
}

func TestConfirmDialogCancelingQuitReturnsToEditingWithTabIntact(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package a"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m = openAndDirtyFile(t, m, file)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.activeDialog != dialogNone {
		t.Fatal("expected canceling to close the dialog")
	}
	if len(m.tabs) != 1 || !m.tabs[0].editor.HasUnsavedChanges() {
		t.Fatal("expected the dirty tab to remain open and dirty after canceling quit")
	}
}
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `go test ./internal/app/ -run 'TestCloseTab|TestConfirmDialog|TestQuit' -v`
Expected: FAIL to compile (`m.closeTab undefined`, `confirmAction undefined`,
`dialogConfirmDiscard undefined`, etc.).

- [ ] **Step 5: Create `internal/app/confirm.go`**

```go
package app

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// confirmAction identifies what an open dialogConfirmDiscard dialog is
// asking the user to confirm.
type confirmAction int

const (
	confirmNone confirmAction = iota
	confirmQuit
	confirmCloseTab
)

// closeTab removes the tab at index. If it has unsaved changes, this opens
// a confirmation dialog instead of closing immediately; the actual removal
// then happens from updateConfirmDialog once the user confirms. A no-op
// for an out-of-range index.
func (m Model) closeTab(index int) (Model, tea.Cmd) {
	if index < 0 || index >= len(m.tabs) {
		return m, nil
	}
	if m.tabs[index].editor.HasUnsavedChanges() {
		m.activeDialog = dialogConfirmDiscard
		m.pendingConfirm = confirmCloseTab
		m.pendingConfirmTab = index
		m.confirmCursor = 0
		return m, nil
	}
	return m.removeTab(index), nil
}

// removeTab unconditionally removes the tab at index and reassigns
// activeTab: closing the active tab prefers the tab that shifted into its
// slot (the one that was to its right), falling back to the one before it
// if the closed tab was last; closing a tab is otherwise a no-op on
// activeTab unless the removal shifted it left.
func (m Model) removeTab(index int) Model {
	m.tabs = append(m.tabs[:index], m.tabs[index+1:]...)
	switch {
	case len(m.tabs) == 0:
		m.activeTab = -1
		m.focus = focusTree
	case index < m.activeTab:
		m.activeTab--
	case index == m.activeTab:
		if m.activeTab >= len(m.tabs) {
			m.activeTab--
		}
	}
	return m
}

func (m Model) updateConfirmDialog(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "esc":
		m.activeDialog = dialogNone
		m.pendingConfirm = confirmNone
		return m, nil
	case "up", "down", "left", "right":
		m.confirmCursor = 1 - m.confirmCursor
		return m, nil
	case "enter":
		action := m.pendingConfirm
		tabIdx := m.pendingConfirmTab
		confirmed := m.confirmCursor == 0
		m.activeDialog = dialogNone
		m.pendingConfirm = confirmNone
		if !confirmed {
			return m, nil
		}
		switch action {
		case confirmQuit:
			m.terminal.Close()
			return m, tea.Quit
		case confirmCloseTab:
			m = m.removeTab(tabIdx)
			return m, nil
		}
	}
	return m, nil
}

func renderConfirmDialog(width, height int, m Model) string {
	var message string
	switch m.pendingConfirm {
	case confirmQuit:
		var names []string
		for _, t := range m.tabs {
			if t.editor.HasUnsavedChanges() {
				names = append(names, filepath.Base(t.path))
			}
		}
		message = fmt.Sprintf("Unsaved changes in: %s", strings.Join(names, ", "))
	case confirmCloseTab:
		message = fmt.Sprintf("%s has unsaved changes.", filepath.Base(m.tabs[m.pendingConfirmTab].path))
	}

	anywayLabel := "Quit anyway"
	if m.pendingConfirm == confirmCloseTab {
		anywayLabel = "Close anyway"
	}
	options := []string{anywayLabel, "Cancel"}
	var opts strings.Builder
	for i, o := range options {
		if i == m.confirmCursor {
			opts.WriteString(lipgloss.NewStyle().Reverse(true).Render(o))
		} else {
			opts.WriteString(o)
		}
		if i < len(options)-1 {
			opts.WriteString("    ")
		}
	}

	content := message + "\n\n" + opts.String()
	return lipgloss.NewStyle().Width(width).Height(height).Padding(1, 2).Render(content)
}
```

- [ ] **Step 6: Wire the dialog into `Update` and `View`**

In `internal/app/model.go`'s `Update`, add a branch alongside the other
`m.activeDialog ==` checks:

```go
	if m.activeDialog == dialogSearch {
		return m.updateSearchDialog(msg)
	}
```

becomes:

```go
	if m.activeDialog == dialogSearch {
		return m.updateSearchDialog(msg)
	}
	if m.activeDialog == dialogConfirmDiscard {
		return m.updateConfirmDialog(msg)
	}
```

In `View()`, add a branch alongside the other dialog renders:

```go
	if m.activeDialog == dialogSearch {
		dialog := renderSearchDialog(m.width, paneHeight, m.searchInput, m.recentCommand)
		return lipgloss.JoinVertical(lipgloss.Left, menuBar, dialog, status)
	}
```

becomes:

```go
	if m.activeDialog == dialogSearch {
		dialog := renderSearchDialog(m.width, paneHeight, m.searchInput, m.recentCommand)
		return lipgloss.JoinVertical(lipgloss.Left, menuBar, dialog, status)
	}
	if m.activeDialog == dialogConfirmDiscard {
		dialog := renderConfirmDialog(m.width, paneHeight, m)
		return lipgloss.JoinVertical(lipgloss.Left, menuBar, dialog, status)
	}
```

- [ ] **Step 7: Update `cmdQuit`**

In `internal/app/commands.go`, replace:

```go
func cmdQuit(m Model) (Model, tea.Cmd) {
	m.terminal.Close()
	return m, tea.Quit
}
```

with:

```go
func cmdQuit(m Model) (Model, tea.Cmd) {
	var dirty []string
	for _, t := range m.tabs {
		if t.editor.HasUnsavedChanges() {
			dirty = append(dirty, t.path)
		}
	}
	if len(dirty) == 0 {
		m.terminal.Close()
		return m, tea.Quit
	}
	m.activeDialog = dialogConfirmDiscard
	m.pendingConfirm = confirmQuit
	m.confirmCursor = 0
	return m, nil
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test ./internal/app/ -run 'TestCloseTab|TestConfirmDialog|TestQuit' -v`
Expected: PASS (all).

- [ ] **Step 9: Run the full test suite**

Run: `go build ./... && go test ./...`
Expected: PASS across every package (the existing `TestCtrlQQuits` and
`TestOpenAndQuitStayGlobalEvenWhenTerminalHasFocus` tests must still pass —
neither ever dirties a buffer, so `cmdQuit`'s new dirty-check finds nothing
and quits immediately exactly as before).

- [ ] **Step 10: Commit**

```bash
git add internal/app/dialog.go internal/app/model.go internal/app/commands.go \
        internal/app/confirm.go internal/app/confirm_test.go
git commit -m "feat: add tab-close and quit unsaved-changes confirmation dialog"
```

---

### Task 5: Tab bar UI — layout, rendering, mouse routing, dirty-map wiring

**Files:**
- Modify: `internal/app/model.go` (`tabBarHeight` constant, `paneLayout`'s new rect, `Update`'s `WindowSizeMsg` and `handlePaneClick`/`handleWheel`, `View`'s layout and tree-dirty wiring)
- Create: `internal/app/tabs.go`
- Test: `internal/app/tabs_test.go`
- Modify: `internal/app/model_test.go` (one existing test's coordinates shift because the tab bar changes editor-pane geometry)

**Interfaces:**
- Consumes: `tab.editor.HasUnsavedChanges()` (Task 1), `m.closeTab` (Task 4), `filetree.Model.SetDirty` (Task 2).
- Produces: `renderTabBar`, `tabRegions`, `tabAt` — used only within `internal/app`.

- [ ] **Step 1: Write the failing tests**

Create `internal/app/tabs_test.go`:

```go
package app

import (
	"path/filepath"
	"strings"
	"testing"

	"cody/internal/editor"
)

func TestTabRegionsAreContiguousAndOrdered(t *testing.T) {
	tabs := []tab{{path: "/a.go"}, {path: "/bb.go"}, {path: "/ccc.go"}}
	regions := tabRegions(tabs)
	if len(regions) != 3 {
		t.Fatalf("got %d regions, want 3", len(regions))
	}
	if regions[0].startCol != 0 {
		t.Fatalf("got first region startCol=%d, want 0", regions[0].startCol)
	}
	for i := 1; i < len(regions); i++ {
		if regions[i].startCol != regions[i-1].endCol {
			t.Fatalf("region %d starts at %d, want %d (immediately after region %d)", i, regions[i].startCol, regions[i-1].endCol, i-1)
		}
	}
	for i, r := range regions {
		if r.tabIndex != i {
			t.Fatalf("region %d has tabIndex=%d, want %d", i, r.tabIndex, i)
		}
		if r.closeStart < r.startCol || r.closeEnd > r.endCol {
			t.Fatalf("region %d's close range [%d,%d) falls outside its own range [%d,%d)", i, r.closeStart, r.closeEnd, r.startCol, r.endCol)
		}
	}
}

func TestTabAtFindsCorrectRegionIncludingCloseGlyph(t *testing.T) {
	tabs := []tab{{path: "/a.go"}, {path: "/bb.go"}}
	regions := tabRegions(tabs)

	first, ok := tabAt(regions[0].startCol, tabs)
	if !ok || first.tabIndex != 0 {
		t.Fatalf("got %+v, ok=%v, want tab 0", first, ok)
	}
	closeClick, ok := tabAt(regions[0].closeStart, tabs)
	if !ok || closeClick.tabIndex != 0 {
		t.Fatalf("expected clicking tab 0's close glyph to still resolve to tab 0, got %+v, ok=%v", closeClick, ok)
	}
	second, ok := tabAt(regions[1].startCol, tabs)
	if !ok || second.tabIndex != 1 {
		t.Fatalf("got %+v, ok=%v, want tab 1", second, ok)
	}
	_, ok = tabAt(regions[len(regions)-1].endCol, tabs)
	if ok {
		t.Fatal("expected a column past the last tab to miss")
	}
}

func TestRenderTabBarShowsNamesCloseGlyphsAndActiveTabReversed(t *testing.T) {
	tabs := []tab{
		{path: "/project/one.go", editor: editor.New()},
		{path: "/project/two.go", editor: editor.New()},
	}
	view := renderTabBar(60, tabs, 1)

	if !strings.Contains(view, "one.go") || !strings.Contains(view, "two.go") {
		t.Fatalf("expected both tab names present, got %q", view)
	}
	if !strings.Contains(view, "×") {
		t.Fatal("expected a close glyph for each tab")
	}
	if !strings.Contains(view, "\x1b[7m") {
		t.Fatal("expected the active tab to be shown in reverse video")
	}
}

func TestRenderTabBarShowsDirtyMarkerForUnsavedTab(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	e, err := editor.New().LoadFile(mustWriteAndReturn(t, file, "package a"))
	if err != nil {
		t.Fatal(err)
	}
	e, _ = e.Update(keyRune('x'))
	tabs := []tab{{path: file, editor: e}}

	view := renderTabBar(40, tabs, 0)
	if !strings.Contains(view, "(M)") {
		t.Fatalf("expected a modified indicator for the dirty tab, got %q", view)
	}
}
```

Add these two small test helpers to `internal/app/model_test.go` (used
above and available to any future test in the package):

```go
func mustWriteAndReturn(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/ -run 'TestTabRegions|TestTabAt|TestRenderTabBar' -v`
Expected: FAIL to compile (`tabRegions undefined`, etc.).

- [ ] **Step 3: Create `internal/app/tabs.go`**

```go
package app

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	tabBarStyle  = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	dirtyTabText = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

// tabRegion describes one tab's clickable column range within the rendered
// tab bar (startCol/endCol, half-open, close glyph included), and the
// sub-range within it (closeStart/closeEnd) that is its close glyph.
type tabRegion struct {
	tabIndex             int
	startCol, endCol     int
	closeStart, closeEnd int
}

// tabPlainLabel returns the unstyled text for one tab — " name(M)? × " —
// used to compute column widths for hit-testing. Must stay the same
// length as what renderTabBar actually draws for that tab, or clicks will
// land on the wrong tab.
func tabPlainLabel(t tab) string {
	name := filepath.Base(t.path)
	suffix := ""
	if t.editor.HasUnsavedChanges() {
		suffix = " (M)"
	}
	return " " + name + suffix + " × "
}

// tabRegions computes each tab's column range in the rendered tab bar, in
// order. Must stay in sync with renderTabBar's own layout.
func tabRegions(tabs []tab) []tabRegion {
	var regions []tabRegion
	col := 0
	closeWidth := lipgloss.Width("× ")
	for i, t := range tabs {
		width := lipgloss.Width(tabPlainLabel(t))
		regions = append(regions, tabRegion{
			tabIndex:   i,
			startCol:   col,
			endCol:     col + width,
			closeStart: col + width - closeWidth,
			closeEnd:   col + width,
		})
		col += width
	}
	return regions
}

// tabAt returns the region a column falls in, if any.
func tabAt(col int, tabs []tab) (tabRegion, bool) {
	for _, r := range tabRegions(tabs) {
		if col >= r.startCol && col < r.endCol {
			return r, true
		}
	}
	return tabRegion{}, false
}

// renderTabBar renders the tab strip: each tab shows its file's base name,
// an orange " (M)" suffix if it has unsaved changes, and a "×" close
// glyph. The active tab's name/suffix (not its close glyph, which stays a
// consistent click target regardless of active state) is shown in reverse
// video, matching renderMenuBar's convention for the open menu label.
func renderTabBar(width int, tabs []tab, activeTab int) string {
	var b strings.Builder
	for i, t := range tabs {
		name := filepath.Base(t.path)
		dirty := t.editor.HasUnsavedChanges()
		main := " " + name
		if i == activeTab {
			suffix := ""
			if dirty {
				suffix = " (M)"
			}
			main = lipgloss.NewStyle().Reverse(true).Render(main + suffix)
		} else if dirty {
			main += dirtyTabText.Render(" (M)")
		}
		b.WriteString(main)
		b.WriteString(" × ")
	}
	return tabBarStyle.Width(width).Render(b.String())
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/app/ -run 'TestTabRegions|TestTabAt|TestRenderTabBar' -v`
Expected: PASS (all four).

- [ ] **Step 5: Add `tabBarHeight` and wire the fourth rect into `paneLayout`**

In `internal/app/model.go`'s const block, add `tabBarHeight`:

```go
const (
	treeWidth       = 30
	statusBarHeight = 1
	menuBarHeight   = 1
	terminalHeight  = 8
	borderSize      = 2 // lipgloss.NormalBorder adds 1 cell on each side

	// mouseWheelLines is how many rows a single wheel notch scrolls.
	mouseWheelLines = 3

	// tabBarHeight is the height of the tab strip shown above the editor
	// pane once at least one file is open.
	tabBarHeight = 1
)
```

Replace `paneLayout`:

```go
func (m Model) paneLayout() (tree, editorR, terminalR rect) {
	paneHeight := m.height - menuBarHeight - statusBarHeight
	dropdownHeight := 0
	if m.openMenu != "" {
		dropdownHeight = lipgloss.Height(renderDropdown(m.openMenu, m.commands))
	}
	bodyTop := menuBarHeight + dropdownHeight
	bodyHeight := paneHeight - dropdownHeight
	editorHeight := bodyHeight - terminalHeight

	tree = rect{0, bodyTop, treeWidth, bodyTop + bodyHeight}
	editorR = rect{treeWidth, bodyTop, m.width, bodyTop + editorHeight}
	terminalR = rect{treeWidth, bodyTop + editorHeight, m.width, bodyTop + bodyHeight}
	return
}
```

with:

```go
func (m Model) paneLayout() (tree, tabBar, editorR, terminalR rect) {
	paneHeight := m.height - menuBarHeight - statusBarHeight
	dropdownHeight := 0
	if m.openMenu != "" {
		dropdownHeight = lipgloss.Height(renderDropdown(m.openMenu, m.commands))
	}
	bodyTop := menuBarHeight + dropdownHeight
	bodyHeight := paneHeight - dropdownHeight
	tabBarH := 0
	if len(m.tabs) > 0 {
		tabBarH = tabBarHeight
	}
	editorHeight := bodyHeight - terminalHeight - tabBarH

	tree = rect{0, bodyTop, treeWidth, bodyTop + bodyHeight}
	tabBar = rect{treeWidth, bodyTop, m.width, bodyTop + tabBarH}
	editorR = rect{treeWidth, bodyTop + tabBarH, m.width, bodyTop + tabBarH + editorHeight}
	terminalR = rect{treeWidth, bodyTop + tabBarH + editorHeight, m.width, bodyTop + bodyHeight}
	return
}
```

- [ ] **Step 6: Update `handlePaneClick` and `handleWheel` for the new rect**

Replace:

```go
func (m Model) handlePaneClick(x, y int) (tea.Model, tea.Cmd) {
	treeRect, editorRect, terminalRect := m.paneLayout()
	switch {
	case treeRect.contains(x, y):
		m.focus = focusTree
		relY := y - treeRect.y0 - 1 // -1 excludes the top border
		var cmd tea.Cmd
		m.tree, cmd = m.tree.HandleClick(relY)
		return m, cmd
	case editorRect.contains(x, y):
		m.focus = focusEditor
		relX := x - editorRect.x0 - 1
		relY := y - editorRect.y0 - 1
		e, cmd := m.activeEditor().HandleClick(relX, relY)
		m = m.setActiveEditor(e)
		return m, cmd
	case terminalRect.contains(x, y):
		m.focus = focusTerminal
		return m.maybeStartTerminal()
	}
	return m, nil
}
```

with:

```go
func (m Model) handlePaneClick(x, y int) (tea.Model, tea.Cmd) {
	treeRect, tabBarRect, editorRect, terminalRect := m.paneLayout()
	switch {
	case treeRect.contains(x, y):
		m.focus = focusTree
		relY := y - treeRect.y0 - 1 // -1 excludes the top border
		var cmd tea.Cmd
		m.tree, cmd = m.tree.HandleClick(relY)
		return m, cmd
	case tabBarRect.contains(x, y):
		relX := x - tabBarRect.x0
		region, ok := tabAt(relX, m.tabs)
		if !ok {
			return m, nil
		}
		if relX >= region.closeStart && relX < region.closeEnd {
			return m.closeTab(region.tabIndex)
		}
		m.activeTab = region.tabIndex
		m.focus = focusEditor
		return m, nil
	case editorRect.contains(x, y):
		m.focus = focusEditor
		relX := x - editorRect.x0 - 1
		relY := y - editorRect.y0 - 1
		e, cmd := m.activeEditor().HandleClick(relX, relY)
		m = m.setActiveEditor(e)
		return m, cmd
	case terminalRect.contains(x, y):
		m.focus = focusTerminal
		return m.maybeStartTerminal()
	}
	return m, nil
}
```

Replace:

```go
func (m Model) handleWheel(x, y, delta int) (tea.Model, tea.Cmd) {
	if m.activeDialog != dialogNone || m.openMenu != "" {
		return m, nil
	}
	treeRect, editorRect, _ := m.paneLayout()
	switch {
	case treeRect.contains(x, y):
		m.tree = m.tree.Scroll(delta)
	case editorRect.contains(x, y):
		m = m.setActiveEditor(m.activeEditor().ScrollLines(delta))
	}
	return m, nil
}
```

with:

```go
func (m Model) handleWheel(x, y, delta int) (tea.Model, tea.Cmd) {
	if m.activeDialog != dialogNone || m.openMenu != "" {
		return m, nil
	}
	treeRect, _, editorRect, _ := m.paneLayout()
	switch {
	case treeRect.contains(x, y):
		m.tree = m.tree.Scroll(delta)
	case editorRect.contains(x, y):
		m = m.setActiveEditor(m.activeEditor().ScrollLines(delta))
	}
	return m, nil
}
```

- [ ] **Step 7: Update `Update`'s `WindowSizeMsg` to account for the tab bar**

Replace:

```go
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		paneHeight := m.height - menuBarHeight - statusBarHeight
		editorHeight := paneHeight - terminalHeight
		m.tree = m.tree.SetSize(treeWidth-borderSize, paneHeight-borderSize)
		m = m.setActiveEditor(m.activeEditor().SetSize(m.width-treeWidth-borderSize, editorHeight-borderSize))
		m.terminal = m.terminal.SetSize(m.width-treeWidth-borderSize, terminalHeight-borderSize)
		return m, nil
	}
```

with:

```go
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		paneHeight := m.height - menuBarHeight - statusBarHeight
		tabBarH := 0
		if len(m.tabs) > 0 {
			tabBarH = tabBarHeight
		}
		editorHeight := paneHeight - terminalHeight - tabBarH
		m.tree = m.tree.SetSize(treeWidth-borderSize, paneHeight-borderSize)
		m = m.setActiveEditor(m.activeEditor().SetSize(m.width-treeWidth-borderSize, editorHeight-borderSize))
		m.terminal = m.terminal.SetSize(m.width-treeWidth-borderSize, terminalHeight-borderSize)
		return m, nil
	}
```

- [ ] **Step 8: Update `View()`: layout math, tab bar row, and dirty-map wiring**

Replace:

```go
	bodyHeight := paneHeight - dropdownHeight
	editorHeight := bodyHeight - terminalHeight
```

with:

```go
	bodyHeight := paneHeight - dropdownHeight
	tabBarH := 0
	if len(m.tabs) > 0 {
		tabBarH = tabBarHeight
	}
	editorHeight := bodyHeight - terminalHeight - tabBarH
```

Replace:

```go
	tree := m.tree.SetSize(treeWidth-borderSize, bodyHeight-borderSize)
	editor := m.activeEditor().SetSize(m.width-treeWidth-borderSize, editorHeight-borderSize)
	termWidth := m.width - treeWidth - borderSize
	termHeight := terminalHeight - borderSize
	term := m.terminal.SetSize(termWidth, termHeight)

	right := lipgloss.JoinVertical(lipgloss.Left,
		editorStyle.Render(clampBlockWidth(editor.View(), m.width-treeWidth-borderSize)),
		terminalStyle.Render(clampBlockWidth(term.View(), termWidth)),
	)
	body := lipgloss.JoinHorizontal(lipgloss.Top, treeStyle.Render(clampBlockWidth(tree.View(), treeWidth-borderSize)), right)
```

with:

```go
	dirty := map[string]bool{}
	for _, t := range m.tabs {
		if t.editor.HasUnsavedChanges() {
			dirty[t.path] = true
		}
	}
	tree := m.tree.SetSize(treeWidth-borderSize, bodyHeight-borderSize).SetDirty(dirty)
	editor := m.activeEditor().SetSize(m.width-treeWidth-borderSize, editorHeight-borderSize)
	termWidth := m.width - treeWidth - borderSize
	termHeight := terminalHeight - borderSize
	term := m.terminal.SetSize(termWidth, termHeight)

	var rightSections []string
	if len(m.tabs) > 0 {
		rightSections = append(rightSections, renderTabBar(m.width-treeWidth-borderSize, m.tabs, m.activeTab))
	}
	rightSections = append(rightSections,
		editorStyle.Render(clampBlockWidth(editor.View(), m.width-treeWidth-borderSize)),
		terminalStyle.Render(clampBlockWidth(term.View(), termWidth)),
	)
	right := lipgloss.JoinVertical(lipgloss.Left, rightSections...)
	body := lipgloss.JoinHorizontal(lipgloss.Top, treeStyle.Render(clampBlockWidth(tree.View(), treeWidth-borderSize)), right)
```

- [ ] **Step 9: Fix the one existing test whose coordinates assumed no tab bar**

In `internal/app/model_test.go`'s `TestClickInEditorPanePositionsCursor`,
the test opens one file before clicking the editor pane — once the tab bar
exists, that shifts the editor pane's top edge down by 1 row. Replace:

```go
	// x=43 -> relX = 43 - 30(editor x0) - 1(border) = 12 -> col = 12 - editorGutterWidth(7) = 5.
	// y=4  -> relY = 4 - 1(bodyTop) - 1(border) = 2 -> buffer line index 2 ("line2").
	updated, _ = m.Update(tea.MouseMsg{X: 43, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
```

with:

```go
	// x=43 -> relX = 43 - 30(editor x0) - 1(border) = 12 -> col = 12 - editorGutterWidth(7) = 5.
	// y=5  -> relY = 5 - 1(bodyTop) - 1(tab bar, one file is open) - 1(border) = 2 -> buffer line index 2 ("line2").
	updated, _ = m.Update(tea.MouseMsg{X: 43, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
```

- [ ] **Step 10: Write and run tests for tab-bar mouse routing and the tree dirty indicator**

Add to `internal/app/model_test.go`:

```go
func TestClickingATabLabelSwitchesActiveTab(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.go")
	fileB := filepath.Join(dir, "b.go")
	if err := os.WriteFile(fileA, []byte("package a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte("package b"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileA})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileB})
	m = updated.(Model)
	// activeTab == 1 (b.go). Tab bar is at y=1 (bodyTop, no dropdown open).
	// Tab 0's label starts at x=0 within the editor column (x=30 on screen).
	m.focus = focusTree

	updated, _ = m.Update(tea.MouseMsg{X: 30, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)

	if m.activeTab != 0 {
		t.Fatalf("got activeTab=%d, want 0", m.activeTab)
	}
	if m.focus != focusEditor {
		t.Fatal("expected clicking a tab label to focus the editor")
	}
}

func TestClickingATabsCloseGlyphClosesIt(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package a"), 0644); err != nil {
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

	region := tabRegions(m.tabs)[0]
	// closeStart is relative to the tab bar's own x0 (treeWidth); the
	// screen column is treeWidth + closeStart.
	updated, _ = m.Update(tea.MouseMsg{X: treeWidth + region.closeStart, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)

	if len(m.tabs) != 0 {
		t.Fatal("expected clicking the close glyph on a clean tab to close it")
	}
}

func TestTreeShowsModifiedIndicatorForDirtyOpenTab(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package a"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m = openAndDirtyFile(t, m, file)

	view := m.View()
	if !strings.Contains(view, "(M)") {
		t.Fatal("expected the tree to show a modified indicator for the dirty open file")
	}
}
```

Run: `go test ./internal/app/ -run 'TestClickingATab|TestTreeShowsModifiedIndicator' -v`
Expected: PASS (all three).

- [ ] **Step 11: Run the full test suite**

Run: `go build ./... && go test ./...`
Expected: PASS across every package.

- [ ] **Step 12: Commit**

```bash
git add internal/app/model.go internal/app/tabs.go internal/app/tabs_test.go internal/app/model_test.go
git commit -m "feat: add tab bar UI (layout, rendering, mouse switch/close), tree dirty wiring"
```

---

## Final check

After Task 5, run the full suite one more time from the repo root:

```bash
go build ./... && go test ./...
```

All packages must pass. This completes the plan — proceed to the final
whole-branch review per subagent-driven-development.
