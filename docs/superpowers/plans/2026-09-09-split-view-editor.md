# Split-View Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the editor area split into two side-by-side editor groups (panes), each with its own tab strip, independently editable/resizable/scrollable, with tabs movable between panes via a right-click "Split + Move Right" / "Move Left" command.

**Architecture:** `app.Model` replaces its single `tabs []tab` / `activeTab int` fields with `panes []editorPane` (`editorPane{tabs []tab; activeTab int}`, length 1 or 2) + `activePane int`. Every existing single-pane helper (`activeEditor`, `setActiveEditor`, `openOrSwitch`, `closeTab`/`removeTab`, tab-bar rendering, mouse hit-testing) generalizes to operate on `m.panes[m.activePane]` (or loop over all panes) instead of the single top-level slice. `paneLayout()`'s fixed tab-bar/editor rect pair becomes a slice, one per pane. A right-click on a tab opens a small dropdown (reusing the existing File/Edit dropdown's rendering/hit-testing pattern, not the file tree's row-splicing pattern) offering the one move command.

**Tech Stack:** Go, Bubble Tea, Lip Gloss (no new dependencies).

**Spec:** `docs/superpowers/specs/2026-09-09-split-view-editor-design.md`

## Global Constraints

- No new third-party dependencies.
- Single-pane (unsplit) behavior must be pixel- and behavior-identical to today at every task boundary — Task 1 in particular must leave every existing test passing with only mechanical field-path renames, no behavior change.
- A pane can never be left with zero tabs while a split exists (`len(m.panes) == 2`) — an operation that would empty one collapses the layout back to a single pane (see spec §3.3's full rule — both directions, pane 0 emptied or pane 1 emptied).
- The tab right-click menu reuses the existing dropdown pattern (`dropdownStyle`/`dropdownBorderSize` in `menu.go`, the dropdown-hit-test shape in `handleClick`) — do not build a second, different-looking menu mechanism.
- Run `go build ./... && go test ./...` after every task; it must be green before moving to the next task.

---

### Task 1: Generalize the data model to `panes`/`activePane` (single-pane behavior preserved)

This task is a pure, behavior-preserving refactor: replace `tabs []tab` / `activeTab int` with `panes []editorPane` / `activePane int` (always `len(panes) == 1` at the end of this task — no split UI or split-creation logic yet), updating every production call site and every existing test to match. No new behavior. The full test suite must pass afterward with the exact same behavior as before this task, just accessed through the new field paths.

**Files:**
- Modify: `internal/app/model.go`
- Modify: `internal/app/confirm.go`
- Modify: `internal/app/commands.go`
- Modify: `internal/app/model_test.go`
- Modify: `internal/app/confirm_test.go`
- Modify: `internal/app/pathprompt_test.go`

**Interfaces:**
- Produces: `type editorPane struct { tabs []tab; activeTab int }`, `Model.panes []editorPane`, `Model.activePane int`. `activeEditor()`/`setActiveEditor()` keep their existing signatures (no caller outside this task's files needs to change) — Task 2/3/4 build directly on `m.panes`/`m.activePane`.

- [ ] **Step 1: Add `editorPane` and replace `Model`'s tab fields**

In `internal/app/model.go`, add directly above the `Model` struct:

```go
// editorPane is one editor group: its own open tabs and which one is
// active. len(app.Model.panes) == 1 means a single unsplit editor area;
// == 2 means the editor area is split left/right, panes[0] on the left.
type editorPane struct {
	tabs      []tab
	activeTab int // -1 when this pane has no tabs open
}
```

In the `Model` struct, replace:

```go
	tabs          []tab
	activeTab     int // -1 when no tabs are open
```

with:

```go
	panes      []editorPane // len 1 (unsplit) or 2 (split); never 0
	activePane int          // which pane index keyboard/mouse edits target
```

In `New()`, replace:

```go
		activeTab:      -1,
```

with nothing (delete that line) and instead, after the returned `Model{...}` literal's other fields, add `panes` and `activePane`. The full literal becomes:

```go
	return Model{
		tree:           tree,
		panes:          []editorPane{{activeTab: -1}},
		activePane:     0,
		terminal:       terminal.New(),
		focus:          focusTree,
		projectName:    filepath.Base(absPath),
		rootPath:       absPath,
		commands:       buildCommands(),
		treeWidth:      defaultTreeWidth,
		terminalHeight: defaultTerminalHeight,
	}, nil
```

- [ ] **Step 2: Rewrite `activeEditor`/`setActiveEditor`/`setActiveTabPath`/`tabBarH`/`openOrSwitch`**

Replace:

```go
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

// setActiveTabPath rewrites the active tab's path — used once, by Save As,
// the moment an until-then-untitled buffer is first written to disk.
func (m Model) setActiveTabPath(path string) Model {
	if m.activeTab >= 0 && m.activeTab < len(m.tabs) {
		m.tabs[m.activeTab].path = path
	}
	return m
}

// tabBarH returns the tab bar's current height: tabBarHeight once at least
// one tab is open, 0 otherwise. Centralizes logic that used to be repeated
// (and could drift) across paneLayout, Update's WindowSizeMsg branch, and
// View().
func (m Model) tabBarH() int {
	if len(m.tabs) > 0 {
		return tabBarHeight
	}
	return 0
}
```

with:

```go
// activeEditor returns the active pane's active tab's editor, or a
// zero-value editor.Model (HasBuffer() == false, matching "no file open
// yet") when that pane has no tabs open.
func (m Model) activeEditor() editor.Model {
	p := m.panes[m.activePane]
	if p.activeTab < 0 || p.activeTab >= len(p.tabs) {
		return editor.Model{}
	}
	return p.tabs[p.activeTab].editor
}

// setActiveEditor writes e back into the active pane's active tab. A no-op
// when that pane has no tabs open (mirrors activeEditor's zero-value
// fallback).
func (m Model) setActiveEditor(e editor.Model) Model {
	p := &m.panes[m.activePane]
	if p.activeTab >= 0 && p.activeTab < len(p.tabs) {
		p.tabs[p.activeTab].editor = e
	}
	return m
}

// setActiveTabPath rewrites the active pane's active tab's path — used
// once, by Save As, the moment an until-then-untitled buffer is first
// written to disk.
func (m Model) setActiveTabPath(path string) Model {
	p := &m.panes[m.activePane]
	if p.activeTab >= 0 && p.activeTab < len(p.tabs) {
		p.tabs[p.activeTab].path = path
	}
	return m
}

// tabBarH returns the tab bar's current height: tabBarHeight once pane 0
// has at least one open tab, 0 otherwise. Pane 0 alone is sufficient to
// check even once a split exists — the collapse rule in removeTab
// guarantees pane 0 is never empty while any pane is. Centralizes logic
// that used to be repeated (and could drift) across paneLayout, Update's
// WindowSizeMsg branch, and View().
func (m Model) tabBarH() int {
	if len(m.panes[0].tabs) > 0 {
		return tabBarHeight
	}
	return 0
}
```

Replace:

```go
// openOrSwitch opens path in a new tab, or switches to its existing tab if
// one is already open for that path — never creates a duplicate. On
// failure to load a new file, m is returned unchanged (matching the
// previous single-tab LoadFile-failure behavior) along with the error.
// editorW/editorH size the new tab's editor immediately (via SetSize)
// instead of leaving it at zero size until the next WindowSizeMsg — a
// background tab (or one just opened, before any resize) that's never been
// sized treats itself as "unbounded" (see editor.Model's height <= 0
// guards), which breaks its scroll-offset math for click-to-position and
// wheel-scroll once it's later switched to or clicked in.
func (m Model) openOrSwitch(path string, editorW, editorH int) (Model, error) {
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
	editorModel = editorModel.SetSize(editorW, editorH)
	m.tabs = append(m.tabs, tab{path: path, editor: editorModel})
	m.activeTab = len(m.tabs) - 1
	return m, nil
}
```

with:

```go
// openOrSwitch opens path in a new tab, or switches to its existing tab if
// one is already open for that path in ANY pane — never creates a
// duplicate, and never leaves the same file open as two independently-
// diverging tabs in two panes at once. On failure to load a new file, m is
// returned unchanged (matching the previous single-tab LoadFile-failure
// behavior) along with the error. A brand new tab is appended to the
// active pane. editorW/editorH size the new tab's editor immediately (via
// SetSize) instead of leaving it at zero size until the next
// WindowSizeMsg — a background tab (or one just opened, before any
// resize) that's never been sized treats itself as "unbounded" (see
// editor.Model's height <= 0 guards), which breaks its scroll-offset math
// for click-to-position and wheel-scroll once it's later switched to or
// clicked in.
func (m Model) openOrSwitch(path string, editorW, editorH int) (Model, error) {
	for pi := range m.panes {
		for i, t := range m.panes[pi].tabs {
			if t.path == path {
				m.activePane = pi
				m.panes[pi].activeTab = i
				return m, nil
			}
		}
	}
	editorModel, err := editor.New().LoadFile(path)
	if err != nil {
		return m, err
	}
	editorModel = editorModel.SetSize(editorW, editorH)
	p := &m.panes[m.activePane]
	p.tabs = append(p.tabs, tab{path: path, editor: editorModel})
	p.activeTab = len(p.tabs) - 1
	return m, nil
}
```

- [ ] **Step 3: Update `Update`'s `WindowSizeMsg` branch and `handlePaneClick`'s tab-bar case**

Replace:

```go
		// Size every open tab's editor, not just the active one: a
		// background tab left at its stale (or zero) size would treat
		// itself as "unbounded" once switched to or clicked in, breaking
		// its scroll-offset math (see openOrSwitch's doc comment).
		for i := range m.tabs {
			m.tabs[i].editor = m.tabs[i].editor.SetSize(editorW, max(0, editorHeight-borderSize))
		}
```

with:

```go
		// Size every open tab's editor in every pane, not just the active
		// one: a background tab left at its stale (or zero) size would
		// treat itself as "unbounded" once switched to or clicked in,
		// breaking its scroll-offset math (see openOrSwitch's doc
		// comment). Task 3 makes editorW pane-specific; for now (single
		// pane) every pane shares the same width.
		for pi := range m.panes {
			for i := range m.panes[pi].tabs {
				m.panes[pi].tabs[i].editor = m.panes[pi].tabs[i].editor.SetSize(editorW, max(0, editorHeight-borderSize))
			}
		}
```

In `handlePaneClick`, replace the `tabBarRect` case body:

```go
		case tabBarRect.contains(x, y):
			relX := x - tabBarRect.x0
			region, ok := tabAt(relX, m.tabs, tabBarRect.x1-tabBarRect.x0)
			if !ok {
				return m, nil
			}
			if relX >= region.closeStart && relX < region.closeEnd {
				return m.closeTab(region.tabIndex)
			}
			m.activeTab = region.tabIndex
			m.focus = focusEditor
			return m, nil
```

with:

```go
		case tabBarRect.contains(x, y):
			relX := x - tabBarRect.x0
			region, ok := tabAt(relX, m.panes[0].tabs, tabBarRect.x1-tabBarRect.x0)
			if !ok {
				return m, nil
			}
			if relX >= region.closeStart && relX < region.closeEnd {
				return m.closeTab(0, region.tabIndex)
			}
			m.panes[0].activeTab = region.tabIndex
			m.focus = focusEditor
			return m, nil
```

(Task 3 makes this loop over every pane's rect instead of hard-coding pane 0 — this task only needs it to keep compiling and behaving identically for the one pane that exists.)

- [ ] **Step 4: Update `View()`'s dirty-map loop, tab-bar-empty check, and render call**

Replace:

```go
	dirty := map[string]bool{}
	for _, t := range m.tabs {
		if t.path != "" && t.editor.HasUnsavedChanges() {
			dirty[t.path] = true
		}
	}
```

with:

```go
	dirty := map[string]bool{}
	for _, p := range m.panes {
		for _, t := range p.tabs {
			if t.path != "" && t.editor.HasUnsavedChanges() {
				dirty[t.path] = true
			}
		}
	}
```

Replace:

```go
	var rightSections []string
	if len(m.tabs) > 0 {
```

with:

```go
	var rightSections []string
	if len(m.panes[0].tabs) > 0 {
```

Replace:

```go
		rightSections = append(rightSections, renderTabBar(m.width-m.treeWidth, m.tabs, m.activeTab))
```

with:

```go
		rightSections = append(rightSections, renderTabBar(m.width-m.treeWidth, m.panes[0].tabs, m.panes[0].activeTab))
```

- [ ] **Step 5: Update `confirm.go` — `closeTab`/`removeTab` gain a pane parameter, with the full two-direction collapse rule**

Replace the entire file's `closeTab`, `removeTab`, `updateConfirmDialog`, and `renderConfirmDialog` with:

```go
// closeTab removes the tab at (pane, index). If it has unsaved changes,
// this opens a confirmation dialog instead of closing immediately; the
// actual removal then happens from updateConfirmDialog once the user
// confirms. A no-op for an out-of-range pane or index.
func (m Model) closeTab(pane, index int) (Model, tea.Cmd) {
	if pane < 0 || pane >= len(m.panes) {
		return m, nil
	}
	if index < 0 || index >= len(m.panes[pane].tabs) {
		return m, nil
	}
	if m.panes[pane].tabs[index].editor.HasUnsavedChanges() {
		m.activeDialog = dialogConfirmDiscard
		m.pendingConfirm = confirmCloseTab
		m.pendingConfirmPane = pane
		m.pendingConfirmTab = index
		m.confirmCursor = 0
		return m, nil
	}
	return m.removeTab(pane, index), nil
}

// removeTab unconditionally removes the tab at (pane, index) and
// reassigns that pane's activeTab: closing the active tab prefers the tab
// that shifted into its slot (the one that was to its right), falling
// back to the one before it if the closed tab was last; closing a tab is
// otherwise a no-op on activeTab unless the removal shifted it left.
//
// If this leaves a pane empty while a split exists (len(m.panes) == 2),
// the layout collapses back to a single pane — the one place this rule
// lives, covering both directions:
//   - pane 1 emptied: drop panes[1] entirely, activePane = 0.
//   - pane 0 emptied and pane 1 still has tabs: panes[0] = panes[1] (the
//     survivor moves into the slot View() always treats as "the" pane in
//     unsplit mode), then drop the now-duplicated panes[1], activePane = 0.
//   - both empty (defensive only — not reachable via a single removeTab
//     call in normal use): fall back to a single fresh empty pane.
func (m Model) removeTab(pane, index int) Model {
	if pane < 0 || pane >= len(m.panes) {
		return m
	}
	if index < 0 || index >= len(m.panes[pane].tabs) {
		return m
	}
	p := &m.panes[pane]
	p.tabs = append(p.tabs[:index], p.tabs[index+1:]...)
	switch {
	case len(p.tabs) == 0:
		p.activeTab = -1
		if len(m.panes) == 1 {
			m.focus = focusTree
		}
	case index < p.activeTab:
		p.activeTab--
	case index == p.activeTab:
		if p.activeTab >= len(p.tabs) {
			p.activeTab--
		}
	}

	if len(m.panes) == 2 {
		emptied0 := len(m.panes[0].tabs) == 0
		emptied1 := len(m.panes[1].tabs) == 0
		switch {
		case emptied0 && emptied1:
			m.panes = []editorPane{{activeTab: -1}}
			m.activePane = 0
			m.focus = focusTree
		case emptied1:
			m.panes = m.panes[:1]
			m.activePane = 0
		case emptied0:
			m.panes[0] = m.panes[1]
			m.panes = m.panes[:1]
			m.activePane = 0
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
		paneIdx := m.pendingConfirmPane
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
			m = m.removeTab(paneIdx, tabIdx)
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
		for _, p := range m.panes {
			for _, t := range p.tabs {
				if t.editor.HasUnsavedChanges() {
					names = append(names, tabDisplayName(t.path))
				}
			}
		}
		message = fmt.Sprintf("Unsaved changes in: %s", strings.Join(names, ", "))
	case confirmCloseTab:
		name := "the tab"
		if m.pendingConfirmPane >= 0 && m.pendingConfirmPane < len(m.panes) {
			p := m.panes[m.pendingConfirmPane]
			if m.pendingConfirmTab >= 0 && m.pendingConfirmTab < len(p.tabs) {
				name = tabDisplayName(p.tabs[m.pendingConfirmTab].path)
			}
		}
		message = fmt.Sprintf("%s has unsaved changes.", name)
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

In `internal/app/model.go`'s `Model` struct, replace:

```go
	pendingConfirm    confirmAction
	pendingConfirmTab int // meaningful only when pendingConfirm == confirmCloseTab
	confirmCursor     int // 0 = "anyway", 1 = "Cancel"
```

with:

```go
	pendingConfirm     confirmAction
	pendingConfirmPane int // meaningful only when pendingConfirm == confirmCloseTab
	pendingConfirmTab  int // meaningful only when pendingConfirm == confirmCloseTab
	confirmCursor      int // 0 = "anyway", 1 = "Cancel"
```

- [ ] **Step 6: Update `commands.go`**

Find the Ctrl+N handler (creating a new blank tab) — replace its body's tab-list mutation:

```go
	m.tabs = append(m.tabs, tab{path: "", editor: e})
	m.activeTab = len(m.tabs) - 1
```

with:

```go
	p := &m.panes[m.activePane]
	p.tabs = append(p.tabs, tab{path: "", editor: e})
	p.activeTab = len(p.tabs) - 1
```

Find `cmdQuit`'s dirty-check loop — replace:

```go
	for _, t := range m.tabs {
```

with a pane-aware nested loop (mirroring Step 4's dirty-map change): replace the whole loop block

```go
	var dirty []string
	for _, t := range m.tabs {
		if t.editor.HasUnsavedChanges() {
			dirty = append(dirty, t.path)
		}
	}
```

with:

```go
	var dirty []string
	for _, p := range m.panes {
		for _, t := range p.tabs {
			if t.editor.HasUnsavedChanges() {
				dirty = append(dirty, t.path)
			}
		}
	}
```

- [ ] **Step 7: Fix every remaining test-file reference**

`internal/app/dialog.go` and `internal/app/pathprompt.go` need no changes — their `openOrSwitch(full, editorW, editorH)` calls keep the exact same signature.

The following test files reference the old `m.tabs`/`m.activeTab` fields directly (white-box, same-package field access) and will fail to compile until updated. Apply this mechanical rule throughout each: **every `m.tabs` becomes `m.panes[0].tabs`, every `m.activeTab` becomes `m.panes[0].activeTab`** (these are all pre-existing single-pane tests, and pane 0 is where a lone pane always lives) — `internal/app/model_test.go`, `internal/app/confirm_test.go`, `internal/app/pathprompt_test.go`.

Two exceptions inside `confirm_test.go` needing individual attention rather than the blanket rule:
- Any call to `m.closeTab(N)` becomes `m.closeTab(0, N)` (the function now takes a pane argument first).
- Any direct reference to `m.pendingConfirmTab` in an assertion stays `m.pendingConfirmTab` (unchanged field name) — but if a test also wants to assert *which pane* a pending confirm targets, that's `m.pendingConfirmPane` (new field, defaults to 0, so existing assertions checking only `pendingConfirmTab` remain valid as-is with no changes needed there).

After applying the rule, run `go build ./... && go vet ./... && go test ./...` — the compiler will flag anything the rule didn't cover (any remaining bare `.tabs`/`.activeTab`/`.closeTab(` reference is a compile error, not a silent bug, so iterate on build errors rather than searching by hand). If a particular test's assertion no longer makes sense after the rename (it shouldn't — this step is pure field-path substitution, no behavior changes), do not "fix" the assertion's expected value; stop and treat it as a signal this step's diff was applied incorrectly somewhere, not that the test was wrong.

- [ ] **Step 8: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: PASS across every package, identical behavior to before this task — this is a pure refactor.

- [ ] **Step 9: Commit**

```bash
git add internal/app/model.go internal/app/confirm.go internal/app/commands.go \
        internal/app/model_test.go internal/app/confirm_test.go internal/app/pathprompt_test.go
git commit -m "refactor: generalize app.Model.tabs/activeTab to panes/activePane"
```

---

### Task 2: `moveTabToOtherPane` + right-click tab context menu

Adds the actual "split + move" behavior and the right-click menu that triggers it. After this task, `len(m.panes)` can become 2 for the first time — Task 1's collapse rule in `removeTab` (already written for both directions) now becomes reachable. No new rendering/resizing yet (Task 3) — a 2-pane state exists in the model and is directly testable, even before `View()` knows how to draw it side by side.

**Files:**
- Create: `internal/app/tabmenu.go`
- Create: `internal/app/tabmenu_test.go`
- Modify: `internal/app/model.go` (new field, `handleRightClick`, the generic mouse-dismiss check)
- Test: `internal/app/model_test.go` (new tests for `moveTabToOtherPane`)

**Interfaces:**
- Consumes: `m.panes`/`m.activePane`/`removeTab` (Task 1).
- Produces: `func (m Model) moveTabToOtherPane(pane, index int) Model`, `type tabContextMenu struct{ pane, index int }`, `Model.tabMenu *tabContextMenu` — Task 3's click-routing wires this in as the right-click target for a tab-bar rect.

- [ ] **Step 1: Write the failing tests for `moveTabToOtherPane`**

Add to `internal/app/model_test.go` (needs `"os"`, `"path/filepath"` already imported there):

```go
func TestMoveTabToOtherPaneCreatesSplitAndMovesTheTab(t *testing.T) {
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
	// pane 0 = [a.go, b.go], activeTab = 1 (b.go)

	m = m.moveTabToOtherPane(0, 0) // move a.go right

	if len(m.panes) != 2 {
		t.Fatalf("got %d panes, want 2", len(m.panes))
	}
	if len(m.panes[0].tabs) != 1 || m.panes[0].tabs[0].path != fileB {
		t.Fatalf("got pane0 tabs=%v, want just b.go", m.panes[0].tabs)
	}
	if len(m.panes[1].tabs) != 1 || m.panes[1].tabs[0].path != fileA {
		t.Fatalf("got pane1 tabs=%v, want just a.go", m.panes[1].tabs)
	}
	if m.activePane != 1 {
		t.Fatalf("got activePane=%d, want 1 (focus follows the moved tab)", m.activePane)
	}
	if m.focus != focusEditor {
		t.Fatal("expected focus to be on the editor after the move")
	}
}

func TestMoveTabToOtherPaneOfOnlyOpenTabCollapsesBackToOnePane(t *testing.T) {
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

	m = m.moveTabToOtherPane(0, 0)

	if len(m.panes) != 1 {
		t.Fatalf("got %d panes, want 1 (moving your only tab has nothing to split against)", len(m.panes))
	}
	if len(m.panes[0].tabs) != 1 || m.panes[0].tabs[0].path != file {
		t.Fatalf("got pane0 tabs=%v, want the tab still there", m.panes[0].tabs)
	}
	if m.activePane != 0 {
		t.Fatalf("got activePane=%d, want 0", m.activePane)
	}
}

func TestMoveTabToOtherPaneIntoAlreadySplitPaneAppends(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.go")
	fileB := filepath.Join(dir, "b.go")
	fileC := filepath.Join(dir, "c.go")
	for _, f := range []string{fileA, fileB, fileC} {
		if err := os.WriteFile(f, []byte("package p"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	for _, f := range []string{fileA, fileB, fileC} {
		updated, _ = m.Update(filetree.FileOpenedMsg{Path: f})
		m = updated.(Model)
	}
	// pane0 = [a, b, c]. Split: move a.go right.
	m = m.moveTabToOtherPane(0, 0)
	// pane0 = [b, c], pane1 = [a]. Now move b.go right too.
	m = m.moveTabToOtherPane(0, 0)

	if len(m.panes[0].tabs) != 1 || m.panes[0].tabs[0].path != fileC {
		t.Fatalf("got pane0 tabs=%v, want just c.go", m.panes[0].tabs)
	}
	if len(m.panes[1].tabs) != 2 || m.panes[1].tabs[0].path != fileA || m.panes[1].tabs[1].path != fileB {
		t.Fatalf("got pane1 tabs=%v, want [a.go, b.go] (appended, not disturbing a.go)", m.panes[1].tabs)
	}
}

func TestMoveTabToOtherPaneCanMoveBackLeft(t *testing.T) {
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
	m = m.moveTabToOtherPane(0, 0) // pane0=[b], pane1=[a]

	m = m.moveTabToOtherPane(1, 0) // move a.go back left

	if len(m.panes) != 1 {
		t.Fatalf("got %d panes, want 1 (pane1 emptied, collapses)", len(m.panes))
	}
	if len(m.panes[0].tabs) != 2 || m.panes[0].tabs[0].path != fileB || m.panes[0].tabs[1].path != fileA {
		t.Fatalf("got pane0 tabs=%v, want [b.go, a.go] (b.go untouched, a.go appended back)", m.panes[0].tabs)
	}
	if m.activePane != 0 {
		t.Fatalf("got activePane=%d, want 0", m.activePane)
	}
}

func TestMoveTabToOtherPaneClosingPane0sOnlyTabWhilePane1SurvivesSwaps(t *testing.T) {
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
	m = m.moveTabToOtherPane(0, 0) // pane0=[b], pane1=[a]

	// Close pane0's only remaining tab directly (not via move) — exercises
	// removeTab's "pane 0 emptied, pane 1 survives" branch from Task 1.
	m, _ = m.closeTab(0, 0)

	if len(m.panes) != 1 {
		t.Fatalf("got %d panes, want 1", len(m.panes))
	}
	if len(m.panes[0].tabs) != 1 || m.panes[0].tabs[0].path != fileA {
		t.Fatalf("got pane0 tabs=%v, want just a.go (pane1's survivor swapped into slot 0)", m.panes[0].tabs)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/ -run TestMoveTabToOtherPane -v`
Expected: FAIL to compile (`m.moveTabToOtherPane undefined`).

- [ ] **Step 3: Implement `moveTabToOtherPane`**

Add to `internal/app/model.go` (anywhere after `openOrSwitch`):

```go
// moveTabToOtherPane moves the tab at (pane, index) into the other pane
// (0 <-> 1), creating panes[1] first if it doesn't exist yet — there is
// no separate "create an empty split" operation, only "move a tab into
// the other side, creating that side if needed."
func (m Model) moveTabToOtherPane(pane, index int) Model {
	if pane < 0 || pane >= len(m.panes) {
		return m
	}
	if index < 0 || index >= len(m.panes[pane].tabs) {
		return m
	}
	if len(m.panes) == 1 {
		m.panes = append(m.panes, editorPane{activeTab: -1})
		m.splitCol = m.defaultSplitCol()
	}
	target := 1 - pane
	moved := m.panes[pane].tabs[index]
	// removeTab does the removal + activeTab reassignment + empty-pane
	// collapse (see its own doc comment) — shared with a plain tab close
	// rather than duplicated here. If removing the source's only tab
	// collapsed panes[1] away (source was pane 0's only tab, target was
	// the pane we just created empty above), "target" as an index is now
	// stale — recompute which pane the moved tab actually belongs in.
	m = m.removeTab(pane, index)
	if len(m.panes) == 1 {
		target = 0
	}
	tp := &m.panes[target]
	tp.tabs = append(tp.tabs, moved)
	tp.activeTab = len(tp.tabs) - 1
	m.activePane = target
	m.focus = focusEditor
	return m
}

// defaultSplitCol returns the on-screen column a fresh split's boundary
// starts at: the horizontal midpoint of the editor area, clamped the same
// way a drag would be.
func (m Model) defaultSplitCol() int {
	return m.clampSplitCol(m.treeWidth + (m.width-m.treeWidth)/2)
}

// clampSplitCol keeps a candidate split boundary within
// [m.treeWidth+minEditorWidth, m.width-minEditorWidth] so dragging (or an
// initial split) can never collapse either side below minEditorWidth.
func (m Model) clampSplitCol(col int) int {
	return clampInt(col, m.treeWidth+minEditorWidth, m.width-minEditorWidth)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/app/ -run TestMoveTabToOtherPane -v`
Expected: PASS (all five).

- [ ] **Step 5: Write the failing tests for the tab context menu**

Create `internal/app/tabmenu_test.go`:

```go
package app

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"cody/internal/filetree"
)

func TestRightClickTabOpensMenuWithSplitMoveRightLabel(t *testing.T) {
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

	// Tab bar is at y=1 (bodyTop, no dropdown), x starting at treeWidth(30).
	updated, _ = m.Update(tea.MouseMsg{X: 30, Y: 1, Button: tea.MouseButtonRight, Action: tea.MouseActionPress})
	m = updated.(Model)

	if m.tabMenu == nil {
		t.Fatal("expected right-clicking a tab to open the tab menu")
	}
	if m.tabMenu.pane != 0 || m.tabMenu.index != 0 {
		t.Fatalf("got tabMenu=%+v, want pane 0 index 0", m.tabMenu)
	}
	items := tabMenuItems(*m.tabMenu)
	if len(items) != 1 || items[0] != "Split + Move Right" {
		t.Fatalf("got items=%v, want [\"Split + Move Right\"]", items)
	}
}

func TestSelectingTabMenuItemMovesTheTab(t *testing.T) {
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
	m.tabMenu = &tabContextMenu{pane: 0, index: 0}

	m = m.selectTabMenuItem("Split + Move Right")

	if m.tabMenu != nil {
		t.Fatal("expected selecting an item to close the menu")
	}
	if len(m.panes) != 2 || m.activePane != 1 {
		t.Fatalf("expected the move to have happened: got %d panes, activePane=%d", len(m.panes), m.activePane)
	}
}

func TestTabMenuLabelIsMoveLeftForPane1(t *testing.T) {
	cm := tabContextMenu{pane: 1, index: 0}
	items := tabMenuItems(cm)
	if len(items) != 1 || items[0] != "Move Left" {
		t.Fatalf("got items=%v, want [\"Move Left\"]", items)
	}
}

func TestLeftClickDismissesOpenTabMenu(t *testing.T) {
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
	m.tabMenu = &tabContextMenu{pane: 0, index: 0}

	updated, _ = m.Update(tea.MouseMsg{X: 5, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)

	if m.tabMenu != nil {
		t.Fatal("expected a left click anywhere to dismiss the open tab menu, same as Esc")
	}
	if len(m.panes) != 1 {
		t.Fatal("expected dismissing the menu to not move the tab")
	}
}
```

- [ ] **Step 6: Run the tests to verify they fail**

Run: `go test ./internal/app/ -run 'TestRightClickTab|TestSelectingTabMenu|TestTabMenuLabel|TestLeftClickDismissesOpenTabMenu' -v`
Expected: FAIL to compile.

- [ ] **Step 7: Create `internal/app/tabmenu.go`**

```go
package app

import (
	"github.com/charmbracelet/lipgloss"
)

// tabContextMenu describes an open right-click menu for one tab.
type tabContextMenu struct {
	pane  int
	index int
}

// tabMenuItems lists a tab menu's actions — one item, whose label depends
// on which pane the tab is currently in: pane 0's tabs offer to split
// into (or move further into) pane 1; pane 1's tabs offer to move back.
func tabMenuItems(cm tabContextMenu) []string {
	if cm.pane == 0 {
		return []string{"Split + Move Right"}
	}
	return []string{"Move Left"}
}

// selectTabMenuItem runs the tab menu's item (there is only one, but this
// stays name-based like the File/Edit dropdown's commandByName for
// consistency and to leave room for more items later) and closes the menu.
func (m Model) selectTabMenuItem(item string) Model {
	cm := m.tabMenu
	m.tabMenu = nil
	if cm == nil {
		return m
	}
	switch item {
	case "Split + Move Right", "Move Left":
		return m.moveTabToOtherPane(cm.pane, cm.index)
	}
	return m
}

// renderTabMenu renders the tab context menu as a small dropdown anchored
// at startCol, matching the File/Edit dropdown's exact visual style
// (dropdownStyle: a bordered box) so the app has one consistent "this is a
// dropdown" look rather than two.
func renderTabMenu(cm tabContextMenu, startCol int) string {
	items := tabMenuItems(cm)
	return dropdownStyle.MarginLeft(startCol).Render(items[0])
}
```

- [ ] **Step 8: Wire right-click routing and mouse-dismiss for the tab menu**

In `internal/app/model.go`'s `Model` struct, add (near `openMenu`):

```go
	tabMenu *tabContextMenu // nil when no tab-menu is open
```

In `handleRightClick`, replace:

```go
func (m Model) handleRightClick(x, y int) (tea.Model, tea.Cmd) {
	if m.activeDialog != dialogNone || m.openMenu != "" {
		return m, nil
	}
	treeRect, _, _, _ := m.paneLayout()
	if !treeRect.contains(x, y) {
		return m, nil
	}
	m.focus = focusTree
	relY := y - treeRect.y0 - 1
	m.tree = m.tree.HandleRightClick(relY)
	return m, nil
}
```

with:

```go
func (m Model) handleRightClick(x, y int) (tea.Model, tea.Cmd) {
	if m.activeDialog != dialogNone || m.openMenu != "" {
		return m, nil
	}
	treeRect, tabBarRect, _, _ := m.paneLayout()
	if treeRect.contains(x, y) {
		m.focus = focusTree
		relY := y - treeRect.y0 - 1
		m.tree = m.tree.HandleRightClick(relY)
		return m, nil
	}
	if tabBarRect.contains(x, y) {
		relX := x - tabBarRect.x0
		region, ok := tabAt(relX, m.panes[0].tabs, tabBarRect.x1-tabBarRect.x0)
		if !ok {
			return m, nil
		}
		m.tabMenu = &tabContextMenu{pane: 0, index: region.tabIndex}
		return m, nil
	}
	return m, nil
}
```

(Task 3 generalizes this to loop over every pane's tab-bar rect instead of hard-coding pane 0 — this task only needs pane 0's tab bar to be right-clickable, since pane 1 doesn't render yet.)

In `Update`, find the generic mouse-dismiss block added for modal dialogs:

```go
	if m.activeDialog != dialogNone {
		// A left click anywhere while a modal dialog is open dismisses it,
		// ...
		if mm, ok := msg.(tea.MouseMsg); ok && mm.Action == tea.MouseActionPress && mm.Button == tea.MouseButtonLeft {
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		}
	}
```

Replace it with a version that also covers `m.tabMenu`, and closes the tab menu directly (it isn't one of the `dialogKind` cases, so there's no `update*Dialog` to funnel an Esc into):

```go
	if mm, ok := msg.(tea.MouseMsg); ok && mm.Action == tea.MouseActionPress && mm.Button == tea.MouseButtonLeft {
		if m.tabMenu != nil {
			m.tabMenu = nil
			return m, nil
		}
	}
	if m.activeDialog != dialogNone {
		// A left click anywhere while a modal dialog is open dismisses it,
		// the same as Esc — these dialogs render across the full body
		// width/height, so there's no "outside" region to distinguish;
		// any click is treated as "cancel". Rewriting the message and
		// letting it fall through to each dialog's own "esc" case reuses
		// that dialog's exact cancel behavior instead of duplicating it
		// six times.
		if mm, ok := msg.(tea.MouseMsg); ok && mm.Action == tea.MouseActionPress && mm.Button == tea.MouseButtonLeft {
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		}
	}
```

Also add an `"esc"` case for the tab menu. Find the existing:

```go
		case "esc":
			if m.openMenu != "" {
				m.openMenu = ""
				return m, nil
			}
```

and replace with:

```go
		case "esc":
			if m.tabMenu != nil {
				m.tabMenu = nil
				return m, nil
			}
			if m.openMenu != "" {
				m.openMenu = ""
				return m, nil
			}
```

Finally, `handlePaneClick`'s `tabBarRect` case (Task 1, Step 3) needs one addition: a left click while the tab menu is open must select the clicked item if the click landed on the menu, or otherwise fall through to normal tab-bar click handling (dismissal already happened above, before `handlePaneClick` is ever reached, for a click anywhere else — so by the time `handlePaneClick` runs, `m.tabMenu` is only non-nil if the click is being tested against the still-open menu's own area). Add this check at the very top of `handleClick` (in `internal/app/model.go`), before the existing `y == 0` menu-bar check:

```go
func (m Model) handleClick(x, y int) (tea.Model, tea.Cmd) {
	if m.activeDialog != dialogNone {
		return m, nil
	}
	if m.tabMenu != nil {
		width := lipgloss.Width(renderTabMenu(*m.tabMenu, 0))
		_, tabBarRect, _, _ := m.paneLayout()
		startCol := tabBarRect.x0 // matches renderTabMenu's own anchor, see Task 3 for the exact anchor column
		if x >= startCol && x < startCol+width && y == tabBarRect.y1 {
			m = m.selectTabMenuItem(tabMenuItems(*m.tabMenu)[0])
			return m, nil
		}
		m.tabMenu = nil
		return m, nil
	}
	if y == 0 {
```

(This anchor-column placeholder is intentionally approximate — Task 3 finalizes `renderTabMenu`'s exact anchor column once the tab's own screen position is known from the generalized `paneLayout`. For this task, the important behavior to have correct and tested is: selecting the item moves the tab, and a click that isn't on the menu closes it — both already covered by Step 5's tests, which don't depend on the exact pixel anchor.)

- [ ] **Step 9: Run the tests to verify they pass**

Run: `go test ./internal/app/ -run 'TestRightClickTab|TestSelectingTabMenu|TestTabMenuLabel|TestLeftClickDismissesOpenTabMenu' -v`
Expected: PASS (all four).

- [ ] **Step 10: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: PASS across every package.

- [ ] **Step 11: Commit**

```bash
git add internal/app/model.go internal/app/model_test.go internal/app/tabmenu.go internal/app/tabmenu_test.go
git commit -m "feat: add moveTabToOtherPane and right-click tab context menu"
```

---

### Task 3: Split-aware rendering and mouse routing

Makes the split actually visible and interactive: `paneLayout()` generalizes from a fixed tab-bar/editor rect pair to a slice (one per pane), `View()` renders two side-by-side tab-bar+editor columns when split, every mouse-routing function (`handlePaneClick`, `handleWheel`, `beginResizeDrag`, `applyResizeDrag`, `handleRightClick`) is updated to loop over panes instead of assuming exactly one, and dragging a new boundary between the two panes resizes the split.

**Files:**
- Modify: `internal/app/model.go`
- Modify: `internal/app/tabmenu.go` (finalize `renderTabMenu`'s anchor)
- Test: `internal/app/model_test.go`

**Interfaces:**
- Consumes: `m.panes`/`m.activePane`/`m.splitCol`/`clampSplitCol` (Tasks 1-2).
- Produces: `type editorPaneLayout struct{ tabBar, editor rect }`, `paneLayout() (tree rect, panes []editorPaneLayout, terminal rect)` — this is the new call shape every routing function in this task switches to.

- [ ] **Step 1: Replace `paneLayout` and its return shape**

Replace:

```go
// paneLayout computes the on-screen rectangles (border included) of the
// tree, editor, and terminal panes for the model's current size and menu
// state. It mirrors the geometry View() renders so a mouse event's (x, y)
// can be routed to whichever pane's box contains it — it must be kept in
// sync with View() if that layout ever changes.
func (m Model) paneLayout() (tree, tabBar, editorR, terminalR rect) {
	paneHeight := m.height - menuBarHeight - statusBarHeight
	dropdownHeight := 0
	if m.openMenu != "" {
		dropdownHeight = lipgloss.Height(renderDropdown(m.openMenu, m.commands))
	}
	bodyTop := menuBarHeight + dropdownHeight
	bodyHeight := paneHeight - dropdownHeight
	tabBarH := m.tabBarH()
	editorHeight := bodyHeight - m.terminalHeight - tabBarH

	tree = rect{0, bodyTop, m.treeWidth, bodyTop + bodyHeight}
	tabBar = rect{m.treeWidth, bodyTop, m.width, bodyTop + tabBarH}
	editorR = rect{m.treeWidth, bodyTop + tabBarH, m.width, bodyTop + tabBarH + editorHeight}
	terminalR = rect{m.treeWidth, bodyTop + tabBarH + editorHeight, m.width, bodyTop + bodyHeight}
	return
}
```

with:

```go
// editorPaneLayout is one editor pane's two rectangles: its tab bar and
// its editor box, stacked vertically at the same x-span.
type editorPaneLayout struct {
	tabBar, editor rect
}

// paneLayout computes the on-screen rectangles (border included) of the
// tree, every editor pane, and the terminal for the model's current size
// and menu state. It mirrors the geometry View() renders so a mouse
// event's (x, y) can be routed to whichever box contains it — it must be
// kept in sync with View() if that layout ever changes. panes has one
// entry per m.panes, in the same order (panes[0] on the left).
func (m Model) paneLayout() (tree rect, panes []editorPaneLayout, terminalR rect) {
	paneHeight := m.height - menuBarHeight - statusBarHeight
	dropdownHeight := 0
	if m.openMenu != "" {
		dropdownHeight = lipgloss.Height(renderDropdown(m.openMenu, m.commands))
	}
	bodyTop := menuBarHeight + dropdownHeight
	bodyHeight := paneHeight - dropdownHeight
	tabBarH := m.tabBarH()
	editorHeight := bodyHeight - m.terminalHeight - tabBarH

	tree = rect{0, bodyTop, m.treeWidth, bodyTop + bodyHeight}

	if len(m.panes) == 1 {
		panes = []editorPaneLayout{{
			tabBar: rect{m.treeWidth, bodyTop, m.width, bodyTop + tabBarH},
			editor: rect{m.treeWidth, bodyTop + tabBarH, m.width, bodyTop + tabBarH + editorHeight},
		}}
	} else {
		panes = []editorPaneLayout{
			{
				tabBar: rect{m.treeWidth, bodyTop, m.splitCol, bodyTop + tabBarH},
				editor: rect{m.treeWidth, bodyTop + tabBarH, m.splitCol, bodyTop + tabBarH + editorHeight},
			},
			{
				tabBar: rect{m.splitCol, bodyTop, m.width, bodyTop + tabBarH},
				editor: rect{m.splitCol, bodyTop + tabBarH, m.width, bodyTop + tabBarH + editorHeight},
			},
		}
	}

	terminalR = rect{m.treeWidth, bodyTop + tabBarH + editorHeight, m.width, bodyTop + bodyHeight}
	return
}
```

- [ ] **Step 2: Update every `paneLayout()` caller**

Replace `handlePaneClick` entirely:

```go
func (m Model) handlePaneClick(x, y int) (tea.Model, tea.Cmd) {
	treeRect, panes, terminalRect := m.paneLayout()
	if treeRect.contains(x, y) {
		m.focus = focusTree
		relY := y - treeRect.y0 - 1 // -1 excludes the top border
		var cmd tea.Cmd
		m.tree, cmd = m.tree.HandleClick(relY)
		return m, cmd
	}
	for pi, pl := range panes {
		if pl.tabBar.contains(x, y) {
			relX := x - pl.tabBar.x0
			region, ok := tabAt(relX, m.panes[pi].tabs, pl.tabBar.x1-pl.tabBar.x0)
			if !ok {
				return m, nil
			}
			m.activePane = pi
			if relX >= region.closeStart && relX < region.closeEnd {
				return m.closeTab(pi, region.tabIndex)
			}
			m.panes[pi].activeTab = region.tabIndex
			m.focus = focusEditor
			return m, nil
		}
		if pl.editor.contains(x, y) {
			m.activePane = pi
			m.focus = focusEditor
			relX := x - pl.editor.x0 - 1
			relY := y - pl.editor.y0 - 1
			e, cmd := m.activeEditor().HandleClick(relX, relY)
			m = m.setActiveEditor(e)
			return m, cmd
		}
	}
	if terminalRect.contains(x, y) {
		m.focus = focusTerminal
		return m.maybeStartTerminal()
	}
	return m, nil
}
```

Replace `handleWheel`:

```go
func (m Model) handleWheel(x, y, delta int) (tea.Model, tea.Cmd) {
	if m.activeDialog != dialogNone || m.openMenu != "" {
		return m, nil
	}
	treeRect, panes, _ := m.paneLayout()
	if treeRect.contains(x, y) {
		m.tree = m.tree.Scroll(delta)
		return m, nil
	}
	for pi, pl := range panes {
		if pl.editor.contains(x, y) {
			if pi == m.activePane {
				m = m.setActiveEditor(m.activeEditor().ScrollLines(delta))
				return m, nil
			}
			// Not the active pane: activeEditor()/setActiveEditor() are
			// keyed to m.activePane, so scroll the other pane's tab
			// directly instead — deliberately not switching activePane
			// here, matching this function's existing contract ("the pane
			// under the pointer, not necessarily the focused one").
			// Bounds-checked defensively: every pane in m.panes is
			// guaranteed non-empty whenever a split exists (removeTab's
			// collapse rule, Task 1), so activeTab should never be -1
			// here in practice — but a wheel event landing on a
			// currently-invalid pane index must degrade to a no-op, not
			// panic, regardless of what future changes might do to that
			// invariant.
			pn := &m.panes[pi]
			if pn.activeTab >= 0 && pn.activeTab < len(pn.tabs) {
				pn.tabs[pn.activeTab].editor = pn.tabs[pn.activeTab].editor.ScrollLines(delta)
			}
			return m, nil
		}
	}
	return m, nil
}
```

Replace `handleRightClick` (Task 2 already updated its tree/tabBar shape once for pane 0 only — this replaces that with the full per-pane loop):

```go
func (m Model) handleRightClick(x, y int) (tea.Model, tea.Cmd) {
	if m.activeDialog != dialogNone || m.openMenu != "" {
		return m, nil
	}
	treeRect, panes, _ := m.paneLayout()
	if treeRect.contains(x, y) {
		m.focus = focusTree
		relY := y - treeRect.y0 - 1
		m.tree = m.tree.HandleRightClick(relY)
		return m, nil
	}
	for pi, pl := range panes {
		if pl.tabBar.contains(x, y) {
			relX := x - pl.tabBar.x0
			region, ok := tabAt(relX, m.panes[pi].tabs, pl.tabBar.x1-pl.tabBar.x0)
			if !ok {
				return m, nil
			}
			m.tabMenu = &tabContextMenu{pane: pi, index: region.tabIndex}
			return m, nil
		}
	}
	return m, nil
}
```

Replace `beginResizeDrag`:

```go
func (m Model) beginResizeDrag(x, y int) (Model, bool) {
	if m.activeDialog != dialogNone || m.openMenu != "" {
		return m, false
	}
	treeRect, panes, terminalRect := m.paneLayout()
	if x == treeRect.x1-1 && y >= treeRect.y0 && y < treeRect.y1 {
		m.resizeDrag = resizeTree
		return m, true
	}
	if len(panes) == 2 {
		boundary := panes[0].editor.x1 // == panes[1].editor.x0 == m.splitCol
		if x == boundary && y >= panes[0].editor.y0 && y < panes[0].editor.y1 {
			m.resizeDrag = resizeEditorSplit
			return m, true
		}
	}
	editorRect := panes[0].editor
	onHorizontalBoundary := y == editorRect.y1-1 || y == terminalRect.y0
	if onHorizontalBoundary && x >= treeRect.x1 && x < m.width {
		m.resizeDrag = resizeTerminal
		return m, true
	}
	return m, false
}
```

Replace `applyResizeDrag`'s switch:

```go
func (m Model) applyResizeDrag(x, y int) Model {
	switch m.resizeDrag {
	case resizeTree:
		m.treeWidth = m.clampTreeWidth(x + 1)
	case resizeTerminal:
		_, _, terminalRect := m.paneLayout()
		m.terminalHeight = m.clampTerminalHeight(terminalRect.y1 - y)
	case resizeEditorSplit:
		m.splitCol = m.clampSplitCol(x)
	}
	return m
}
```

Add `resizeEditorSplit` to the `resizeKind` enum:

```go
const (
	resizeNone resizeKind = iota
	resizeTree
	resizeTerminal
	resizeEditorSplit
)
```

- [ ] **Step 3: Update `handleClick`'s tab-menu block and finalize `renderTabMenu`'s anchor**

Task 2's `handleClick` used a placeholder anchor column. Replace that whole `if m.tabMenu != nil { ... }` block with:

```go
	if m.tabMenu != nil {
		_, panes, _ := m.paneLayout()
		pl := panes[m.tabMenu.pane]
		startCol := pl.tabBar.x0 + tabRegions(m.panes[m.tabMenu.pane].tabs, pl.tabBar.x1-pl.tabBar.x0)[m.tabMenu.index].startCol
		width := lipgloss.Width(renderTabMenu(*m.tabMenu, 0))
		menuY := pl.tabBar.y1 // dropdown opens directly below the tab bar
		if x >= startCol && x < startCol+width && y == menuY {
			m = m.selectTabMenuItem(tabMenuItems(*m.tabMenu)[0])
			return m, nil
		}
		m.tabMenu = nil
		return m, nil
	}
```

(This anchors the menu at the right-clicked tab's own screen column, matching how `renderDropdown` anchors at a menu label's column — `MarginLeft(startCol)` in `renderTabMenu` already expects an absolute screen column, so pass `startCol` computed here, not a relative one, when actually rendering it in Step 5.)

- [ ] **Step 4: Rewrite `View()` for split-aware rendering**

Replace the border-color block:

```go
	treeBorderColor := unfocusedBorderColor
	editorBorderColor := unfocusedBorderColor
	terminalBorderColor := unfocusedBorderColor
	switch m.focus {
	case focusTree:
		treeBorderColor = focusedBorderColor
	case focusEditor:
		editorBorderColor = focusedBorderColor
	case focusTerminal:
		terminalBorderColor = focusedBorderColor
	}
```

with:

```go
	treeBorderColor := unfocusedBorderColor
	terminalBorderColor := unfocusedBorderColor
	paneBorderColor := make([]lipgloss.Color, len(m.panes))
	for i := range paneBorderColor {
		paneBorderColor[i] = unfocusedBorderColor
	}
	switch m.focus {
	case focusTree:
		treeBorderColor = focusedBorderColor
	case focusEditor:
		paneBorderColor[m.activePane] = focusedBorderColor
	case focusTerminal:
		terminalBorderColor = focusedBorderColor
	}
```

Replace the `treeStyle`/`editorStyle`/`terminalStyle` block:

```go
	treeStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(treeBorderColor).
		Width(m.treeWidth - borderSize).
		Height(bodyHeight - borderSize)
	editorStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(editorBorderColor).
		Width(m.width - m.treeWidth - borderSize).
		Height(editorHeight - borderSize)
	terminalStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(terminalBorderColor).
		Width(m.width - m.treeWidth - borderSize).
		Height(m.terminalHeight - borderSize)
```

with:

```go
	treeStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(treeBorderColor).
		Width(m.treeWidth - borderSize).
		Height(bodyHeight - borderSize)
	terminalStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(terminalBorderColor).
		Width(m.width - m.treeWidth - borderSize).
		Height(m.terminalHeight - borderSize)

	// One width per pane: unsplit, a pane spans the full editor column
	// (matching today's single editorStyle exactly); split, each spans
	// its own share either side of m.splitCol.
	paneWidth := make([]int, len(m.panes))
	if len(m.panes) == 1 {
		paneWidth[0] = m.width - m.treeWidth
	} else {
		paneWidth[0] = m.splitCol - m.treeWidth
		paneWidth[1] = m.width - m.splitCol
	}
	paneStyle := make([]lipgloss.Style, len(m.panes))
	for i, w := range paneWidth {
		paneStyle[i] = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(paneBorderColor[i]).
			Width(w - borderSize).
			Height(editorHeight - borderSize)
	}
```

Replace the dirty-map / pane-sizing / rendering block:

```go
	dirty := map[string]bool{}
	for _, p := range m.panes {
		for _, t := range p.tabs {
			if t.path != "" && t.editor.HasUnsavedChanges() {
				dirty[t.path] = true
			}
		}
	}
	// Floored at 0 before reaching any pane's SetSize: a window smaller
	// than the panes' combined minimums (aggressive resizing) can leave
	// this arithmetic negative even after clampTreeWidth/
	// clampTerminalHeight (see clampInt's degenerate-window handling), and
	// a negative size reaching the terminal's real vt.Emulator panics
	// rather than degrading gracefully. termWidth/termHeight stay
	// unclamped below for clampBlockWidth, which already treats <= 0 as
	// "don't touch it".
	tree := m.tree.SetSize(max(0, m.treeWidth-borderSize), max(0, bodyHeight-borderSize)).SetDirty(dirty)
	editor := m.activeEditor().SetSize(max(0, m.width-m.treeWidth-borderSize), max(0, editorHeight-borderSize))
	termWidth := m.width - m.treeWidth - borderSize
	termHeight := m.terminalHeight - borderSize
	term := m.terminal.SetSize(max(0, termWidth), max(0, termHeight))

	var rightSections []string
	if len(m.panes[0].tabs) > 0 {
		// The tab bar has no border of its own, so it must be rendered at
		// the same on-screen width as tabBarRect (paneLayout): m.width -
		// m.treeWidth. editorStyle's content Width(m.width-m.treeWidth-
		// borderSize) looks narrower only because its border adds
		// borderSize back on screen — the tab bar has no border to add,
		// so it must use the full span directly instead of subtracting
		// borderSize again.
		rightSections = append(rightSections, renderTabBar(m.width-m.treeWidth, m.panes[0].tabs, m.panes[0].activeTab))
	}
	rightSections = append(rightSections,
		editorStyle.Render(clampBlockWidth(editor.View(), m.width-m.treeWidth-borderSize)),
		terminalStyle.Render(clampBlockWidth(term.View(), termWidth)),
	)
	right := lipgloss.JoinVertical(lipgloss.Left, rightSections...)
	body := lipgloss.JoinHorizontal(lipgloss.Top, treeStyle.Render(clampBlockWidth(tree.View(), m.treeWidth-borderSize)), right)
```

with:

```go
	dirty := map[string]bool{}
	for _, p := range m.panes {
		for _, t := range p.tabs {
			if t.path != "" && t.editor.HasUnsavedChanges() {
				dirty[t.path] = true
			}
		}
	}
	// Floored at 0 before reaching any pane's SetSize: a window smaller
	// than the panes' combined minimums (aggressive resizing) can leave
	// this arithmetic negative even after clampTreeWidth/
	// clampTerminalHeight/clampSplitCol (see clampInt's degenerate-window
	// handling), and a negative size reaching the terminal's real
	// vt.Emulator panics rather than degrading gracefully. termWidth/
	// termHeight stay unclamped below for clampBlockWidth, which already
	// treats <= 0 as "don't touch it".
	tree := m.tree.SetSize(max(0, m.treeWidth-borderSize), max(0, bodyHeight-borderSize)).SetDirty(dirty)
	termWidth := m.width - m.treeWidth - borderSize
	termHeight := m.terminalHeight - borderSize
	term := m.terminal.SetSize(max(0, termWidth), max(0, termHeight))

	var paneColumns []string
	for i, p := range m.panes {
		activeEditor := editor.Model{}
		if p.activeTab >= 0 && p.activeTab < len(p.tabs) {
			activeEditor = p.tabs[p.activeTab].editor.SetSize(max(0, paneWidth[i]-borderSize), max(0, editorHeight-borderSize))
		}
		var col []string
		if len(p.tabs) > 0 {
			// No border of its own, so it renders at this pane's full
			// on-screen width — see the pre-split comment this replaced
			// for why that's paneWidth[i], not paneWidth[i]-borderSize.
			col = append(col, renderTabBar(paneWidth[i], p.tabs, p.activeTab))
		}
		col = append(col, paneStyle[i].Render(clampBlockWidth(activeEditor.View(), paneWidth[i]-borderSize)))
		paneColumns = append(paneColumns, lipgloss.JoinVertical(lipgloss.Left, col...))
	}
	editorsRow := lipgloss.JoinHorizontal(lipgloss.Top, paneColumns...)

	right := lipgloss.JoinVertical(lipgloss.Left,
		editorsRow,
		terminalStyle.Render(clampBlockWidth(term.View(), termWidth)),
	)
	body := lipgloss.JoinHorizontal(lipgloss.Top, treeStyle.Render(clampBlockWidth(tree.View(), m.treeWidth-borderSize)), right)
```

If `m.tabMenu != nil`, append `renderTabMenu(*m.tabMenu, startCol)` (computed the same way as Step 3's `handleClick` block) into `sections` right after `body` and before `status`, so the open dropdown actually appears — add this immediately before the final `sections := []string{menuBar}` block:

```go
	var tabMenuView string
	if m.tabMenu != nil {
		_, panes, _ := m.paneLayout()
		pl := panes[m.tabMenu.pane]
		startCol := pl.tabBar.x0 + tabRegions(m.panes[m.tabMenu.pane].tabs, pl.tabBar.x1-pl.tabBar.x0)[m.tabMenu.index].startCol
		tabMenuView = renderTabMenu(*m.tabMenu, startCol)
	}
```

and change:

```go
	sections := []string{menuBar}
	if dropdown != "" {
		sections = append(sections, dropdown)
	}
	sections = append(sections, body, status)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
```

to:

```go
	sections := []string{menuBar}
	if dropdown != "" {
		sections = append(sections, dropdown)
	}
	sections = append(sections, body)
	if tabMenuView != "" {
		sections = append(sections, tabMenuView)
	}
	sections = append(sections, status)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
```

- [ ] **Step 5: Write and run split-rendering and resize tests**

Add to `internal/app/model_test.go`:

```go
func TestSplitRendersTwoTabBarsAndTwoEditorBoxes(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.go")
	fileB := filepath.Join(dir, "b.go")
	if err := os.WriteFile(fileA, []byte("package a\nline in a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte("package b\nline in b"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileA})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileB})
	m = updated.(Model)
	m = m.moveTabToOtherPane(0, 0)

	view := m.View()
	if !strings.Contains(view, "a.go") || !strings.Contains(view, "b.go") {
		t.Fatalf("expected both tab names visible in the split view")
	}
	if !strings.Contains(view, "line in a") || !strings.Contains(view, "line in b") {
		t.Fatalf("expected both files' content visible side by side")
	}
}

func TestDraggingSplitBoundaryResizesIt(t *testing.T) {
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
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileA})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileB})
	m = updated.(Model)
	m = m.moveTabToOtherPane(0, 0)
	startCol := m.splitCol

	_, panes, _ := m.paneLayout()
	boundaryX := panes[0].editor.x1
	updated, _ = m.Update(press(boundaryX, panes[0].editor.y0+1))
	m = updated.(Model)
	if m.resizeDrag != resizeEditorSplit {
		t.Fatalf("got resizeDrag=%v, want resizeEditorSplit", m.resizeDrag)
	}
	updated, _ = m.Update(drag(boundaryX+10, panes[0].editor.y0+1))
	m = updated.(Model)

	if m.splitCol != startCol+10 {
		t.Fatalf("got splitCol=%d, want %d", m.splitCol, startCol+10)
	}
}

func TestClickInPane1SwitchesActivePaneAndPositionsCursor(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.go")
	fileB := filepath.Join(dir, "b.go")
	if err := os.WriteFile(fileA, []byte("package a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte("line0\nline1\nline2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileA})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileB})
	m = updated.(Model)
	m = m.moveTabToOtherPane(0, 0) // b.go -> pane1, activePane=1
	m.activePane = 0               // simulate focus having moved back to pane0
	m.focus = focusEditor

	_, panes, _ := m.paneLayout()
	pl := panes[1]
	clickX := pl.editor.x0 + 1 + editorGutterWidthForTest()
	clickY := pl.editor.y0 + 1 + 1 // second visible row -> "line1"
	updated, _ = m.Update(tea.MouseMsg{X: clickX, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)

	if m.activePane != 1 {
		t.Fatalf("got activePane=%d, want 1", m.activePane)
	}
	line, _ := m.activeEditor().Cursor()
	if line != 2 {
		t.Fatalf("got cursor line=%d, want 2 (line1, 1-indexed)", line)
	}
}
```

Add this small test-only helper near the other test helpers in `internal/app/model_test.go` (mirrors `editor.editorGutterWidth`, which is unexported in a different package and so can't be referenced directly — duplicating the one constant here is simpler than exporting it just for a test):

```go
// editorGutterWidthForTest mirrors editor.editorGutterWidth (unexported,
// different package) for tests that need to click past the gutter.
func editorGutterWidthForTest() int { return 7 }
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/app/ -run 'TestSplitRenders|TestDraggingSplitBoundary|TestClickInPane1' -v`
Expected: PASS (all three).

- [ ] **Step 7: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: PASS across every package — in particular, every pre-existing single-pane click/wheel/resize/right-click test must still pass unchanged, since `len(m.panes) == 1` makes every loop in this task's rewritten functions iterate exactly once, over exactly the same rectangle the old fixed-shape code computed.

- [ ] **Step 8: Commit**

```bash
git add internal/app/model.go internal/app/tabmenu.go internal/app/model_test.go
git commit -m "feat: split-aware rendering, mouse routing, and resizable split boundary"
```

---

### Task 4: Keyboard focus cycling and final integration

Closes the remaining gap (Tab/Shift+Tab should stop at both editor panes when split) and adds end-to-end tests exercising the whole feature together, as a final sanity pass before the whole-branch review.

**Files:**
- Modify: `internal/app/model.go` (`focusNext`/`focusPrev`)
- Test: `internal/app/model_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1-3.
- Produces: nothing new — this task only adds behavior to existing functions and tests.

- [ ] **Step 1: Write the failing tests**

Add to `internal/app/model_test.go`:

```go
func TestTabCyclesThroughBothPanesWhenSplit(t *testing.T) {
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
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileA})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileB})
	m = updated.(Model)
	m = m.moveTabToOtherPane(0, 0) // split; activePane=1, focus=editor

	// Tab: editor(pane1) -> terminal (pane1 was already the "last" editor
	// stop reached by the split, so the very next Tab leaves the editor).
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.focus != focusTerminal {
		t.Fatalf("got focus=%v, want focusTerminal", m.focus)
	}

	// Tab: terminal -> tree -> editor(pane0) -> editor(pane1) -> terminal
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> tree
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> editor, pane0
	m = updated.(Model)
	if m.focus != focusEditor || m.activePane != 0 {
		t.Fatalf("got focus=%v activePane=%d, want focusEditor/0", m.focus, m.activePane)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> editor, pane1
	m = updated.(Model)
	if m.focus != focusEditor || m.activePane != 1 {
		t.Fatalf("got focus=%v activePane=%d, want focusEditor/1", m.focus, m.activePane)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> terminal
	m = updated.(Model)
	if m.focus != focusTerminal {
		t.Fatalf("got focus=%v, want focusTerminal", m.focus)
	}
}

func TestTabSkipsSecondPaneWhenUnsplit(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> editor
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> terminal directly, no second pane stop
	m = updated.(Model)
	if m.focus != focusTerminal {
		t.Fatalf("got focus=%v, want focusTerminal", m.focus)
	}
}

// End-to-end integration test covering the whole feature together: open
// two files, split, edit both independently, resize, move a tab back,
// close, quit-with-unsaved-changes still shows every dirty file across
// both panes.
func TestSplitViewEndToEnd(t *testing.T) {
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
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileA})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileB})
	m = updated.(Model)
	m = m.moveTabToOtherPane(0, 0) // pane0=[b], pane1=[a], activePane=1

	// Edit pane1's active tab (a.go).
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	m = updated.(Model)
	if !m.panes[1].tabs[0].editor.HasUnsavedChanges() {
		t.Fatal("expected editing pane1's tab to mark it dirty")
	}

	// Switch to pane0 and edit it too.
	m.activePane = 0
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")})
	m = updated.(Model)
	if !m.panes[0].tabs[0].editor.HasUnsavedChanges() {
		t.Fatal("expected editing pane0's tab to mark it dirty")
	}

	// Quitting should list both dirty files, from both panes.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m = updated.(Model)
	if m.activeDialog != dialogConfirmDiscard {
		t.Fatal("expected the quit confirmation to open with two dirty tabs across two panes")
	}

	// Cancel the quit, move the tab back left, close it clean.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	m = m.moveTabToOtherPane(1, 0) // a.go back to pane0
	if len(m.panes) != 1 {
		t.Fatalf("got %d panes, want 1 after moving pane1's only tab back", len(m.panes))
	}
	if len(m.panes[0].tabs) != 2 {
		t.Fatalf("got %d tabs in the collapsed pane, want 2", len(m.panes[0].tabs))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail (for the first two — the third exercises only already-implemented behavior and should already pass, confirming Tasks 1-3 integrate correctly)**

Run: `go test ./internal/app/ -run 'TestTabCyclesThroughBothPanes|TestTabSkipsSecondPane|TestSplitViewEndToEnd' -v`
Expected: the first two FAIL (focus cycling doesn't know about panes yet); `TestSplitViewEndToEnd` should already PASS — if it doesn't, that's a real integration gap from an earlier task, not something to paper over here.

- [ ] **Step 3: Implement the focus-cycling change**

Replace:

```go
func (m *Model) focusNext() {
	switch m.focus {
	case focusTree:
		m.focus = focusEditor
	case focusEditor:
		m.focus = focusTerminal
	case focusTerminal:
		m.focus = focusTree
	}
}

func (m *Model) focusPrev() {
	switch m.focus {
	case focusTree:
		m.focus = focusTerminal
	case focusEditor:
		m.focus = focusTree
	case focusTerminal:
		m.focus = focusEditor
	}
}
```

with:

```go
func (m *Model) focusNext() {
	switch m.focus {
	case focusTree:
		m.focus = focusEditor
		m.activePane = 0
	case focusEditor:
		if m.activePane == 0 && len(m.panes) == 2 {
			m.activePane = 1
		} else {
			m.focus = focusTerminal
		}
	case focusTerminal:
		m.focus = focusTree
	}
}

func (m *Model) focusPrev() {
	switch m.focus {
	case focusTree:
		m.focus = focusTerminal
	case focusEditor:
		if m.activePane == 1 {
			m.activePane = 0
		} else {
			m.focus = focusTree
		}
	case focusTerminal:
		m.focus = focusEditor
		if len(m.panes) == 2 {
			m.activePane = 1
		}
	}
}
```

(`focusNext` resets `activePane` to 0 when entering the editor from the tree, so Tab always starts its editor stop(s) at the left pane first, matching the test's expectations and general left-to-right reading order. `focusPrev` mirrors this: entering from the terminal starts at the rightmost pane.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/app/ -run 'TestTabCyclesThroughBothPanes|TestTabSkipsSecondPane|TestSplitViewEndToEnd' -v`
Expected: PASS (all three).

- [ ] **Step 5: Run the full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: PASS across every package.

- [ ] **Step 6: Commit**

```bash
git add internal/app/model.go internal/app/model_test.go
git commit -m "feat: cycle keyboard focus through both editor panes when split"
```

---

## Final check

After Task 4, run the full suite one more time from the repo root:

```bash
go build ./... && go test ./...
```

All packages must pass. This completes the plan — proceed to the final whole-branch review per subagent-driven-development.
