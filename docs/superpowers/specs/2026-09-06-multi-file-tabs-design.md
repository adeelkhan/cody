# Multi-File Tabs, Unsaved-Changes Confirmation, Dirty Indicator — Design

## 1. Overview

Cody's editor pane currently holds exactly one open file (`app.Model.editor
editor.Model`). This spec adds:

1. **Multi-file tabs**: opening a file from the project tree opens it in a new
   tab if it isn't already open, or switches to its existing tab if it is.
   Tabs render in a bar above the editor pane and are switchable and closable
   by mouse.
2. **Unsaved-changes confirmation**: quitting the app (`ctrl+q`) or closing a
   tab with unsaved changes shows a confirmation dialog instead of silently
   discarding edits.
3. **Modified indicator**: the project tree shows an orange `(M)` after the
   name of any file that has an open tab with unsaved changes.

## 2. Global Constraints

- No new third-party dependencies.
- Existing single-file behavior (open, edit, save, search, fold, mouse
  click/scroll) is preserved — this spec only adds the tab layer around it,
  it does not change `editor.Model`'s own internal behavior.
- All existing tests continue to pass; call sites touched by the `m.editor`
  → `m.tabs`/`activeTab` refactor are updated, not deleted.
- Tab close and quit confirmation share one dialog implementation — no
  duplicated confirm/cancel logic.

## 3. Data model

### 3.1 `app.tab`

A new unexported struct in `internal/app`:

```go
type tab struct {
	path   string
	editor editor.Model
}
```

`path` is the absolute path (matches `filetree.Node.Path`, which is already
absolute — see `tree.go`'s `filepath.Join(n.Path, e.Name())` starting from an
absolute root).

### 3.2 `app.Model` changes

- Remove the `editor editor.Model` field.
- Add:
  ```go
  tabs      []tab
  activeTab int // -1 when no tabs are open
  ```
- `New()` initializes `activeTab: -1`.
- Two helper methods replace direct `m.editor` field access everywhere:
  ```go
  // activeEditor returns the active tab's editor, or a zero-value
  // editor.Model (HasBuffer() == false) when no tabs are open.
  func (m Model) activeEditor() editor.Model {
  	if m.activeTab < 0 || m.activeTab >= len(m.tabs) {
  		return editor.Model{}
  	}
  	return m.tabs[m.activeTab].editor
  }

  // setActiveEditor writes e back into the active tab. A no-op when no
  // tabs are open (mirrors activeEditor's zero-value fallback).
  func (m Model) setActiveEditor(e editor.Model) Model {
  	if m.activeTab >= 0 && m.activeTab < len(m.tabs) {
  		m.tabs[m.activeTab].editor = e
  	}
  	return m
  }
  ```
- Every existing call site that reads or writes `m.editor` is rewritten in
  terms of these two helpers. This is mechanical — see the plan for the
  exact call-site list — and changes no behavior for the single-tab case.

### 3.3 `editor.Model` changes

One new exported method (`app` must not reach into `editor`'s unexported
`buf` field):

```go
// HasUnsavedChanges reports whether the current buffer has unsaved edits.
// False when no file is loaded.
func (m Model) HasUnsavedChanges() bool {
	return m.buf != nil && m.buf.Dirty
}
```

### 3.4 Confirmation dialog state

A new `dialogKind` value, `dialogConfirmDiscard`, added alongside the
existing `dialogNone`/`dialogFileOpen`/`dialogAbout`/`dialogPalette`/
`dialogSearch` in `internal/app/dialog.go`.

A new small enum describes what's being confirmed:

```go
type confirmAction int

const (
	confirmNone confirmAction = iota
	confirmQuit
	confirmCloseTab
)
```

`app.Model` gains:

```go
pendingConfirm    confirmAction
pendingConfirmTab int // meaningful only when pendingConfirm == confirmCloseTab
confirmCursor     int // 0 = "anyway", 1 = "Cancel" — which option is selected
```

## 4. Behavior

### 4.1 Opening a file (tab creation / switch, no duplicates)

`FileOpenedMsg` handling (currently in `app/model.go`'s `Update`) changes
from "always load into `m.editor`" to:

1. Search `m.tabs` for a tab whose `path` equals `msg.Path`.
2. If found at index `i`: set `m.activeTab = i` (switch, don't reload —
   preserves that tab's cursor position, scroll, undo history, etc.).
3. If not found: `editorModel, err := editor.New().LoadFile(msg.Path)`; on
   error, set `m.recentCommand` to the failure message exactly as today and
   return, `tabs`/`activeTab` unchanged. On success, append
   `tab{path: msg.Path, editor: editorModel}` to `m.tabs`, set
   `m.activeTab = len(m.tabs) - 1`.
4. Either way, `m.focus = focusEditor` and `m.recentCommand = "Opened
   <basename>"`, matching today's message.

### 4.2 Tab bar layout

A new fixed-height row, `tabBarHeight = 1`, rendered directly above the
editor's bordered box — in the editor's column only (same x-range as the
editor/terminal panes; the tree pane's column is untouched). Shown only
when `len(m.tabs) > 0`; contributes 0 height otherwise (mirrors how
`dropdownHeight` already conditionally contributes to body layout in
`View()`).

Updated vertical math (both in `Update`'s `WindowSizeMsg` branch and in
`View()`, which already re-derives heights fresh every render — see the
existing comment on why heights are re-derived rather than cached):

```
paneHeight   = height - menuBarHeight - statusBarHeight
bodyHeight   = paneHeight - dropdownHeight
tabBarH      = 0; if len(tabs) > 0 { tabBarH = tabBarHeight }
editorHeight = bodyHeight - terminalHeight - tabBarH
```

The tab bar sits at `y = menuBarHeight + dropdownHeight` (i.e., immediately
below the menu bar and any open dropdown, immediately above the editor
box). `paneLayout()` (in `app/model.go`, used for mouse hit-testing) gains
a fourth rectangle, `tabBarRect`, computed the same way, and the existing
`editorRect`'s `y0` shifts down by `tabBarH`.

### 4.3 Tab bar rendering

New file `internal/app/tabs.go`. For each tab, in order:

```
" " + filepath.Base(tab.path) + dirtySuffix + " × "
```

- `dirtySuffix` is `" (M)"` styled with `lipgloss.Color("214")` (orange)
  when `tab.editor.HasUnsavedChanges()`, else `""`.
- The active tab's whole label (name + dirty suffix, not the close glyph)
  is rendered with `.Reverse(true)`, matching `renderMenuBar`'s convention
  for the open menu label.
- The close glyph `×` is always plain (not reversed), so it stays a
  consistent click target regardless of active/inactive state.
- Tabs are joined with no extra separator (each tab's own leading/trailing
  spaces provide the visual gap).
- The whole bar gets `Background(lipgloss.Color("236"))` (same as
  `menuBarStyle`) so it's visually grouped with the menu bar above it.

```go
var tabBarStyle = lipgloss.NewStyle().Background(lipgloss.Color("236"))
var dirtyTabStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

func renderTabBar(width int, tabs []tab, activeTab int) string { ... }
```

### 4.4 Tab hit-testing (mouse)

New function, mirroring `menuLabelAt`:

```go
// tabRegion describes one tab's clickable column range and, within it,
// the sub-range that is its close glyph.
type tabRegion struct {
	tabIndex           int
	startCol, endCol   int // the whole tab, close glyph included
	closeStart, closeEnd int
}

func tabRegions(tabs []tab) []tabRegion { ... }

// tabAt returns the region a column falls in, if any.
func tabAt(col int, tabs []tab) (tabRegion, bool) { ... }
```

`handlePaneClick` (in `app/model.go`) gains a case for `tabBarRect`: hit-test
via `tabAt`; if the click is within `closeStart..closeEnd`, call
`closeTab(index)` (§4.6); otherwise set `m.activeTab = index` and
`m.focus = focusEditor`.

### 4.5 Quitting with unsaved changes

`cmdQuit` (in `commands.go`) changes from unconditionally returning
`tea.Quit` to:

1. Collect `dirty := []string{}` — the base name of every tab whose
   `editor.HasUnsavedChanges()` is true.
2. If `len(dirty) == 0`: return `m, tea.Quit` (today's behavior, unchanged).
3. Else: `m.activeDialog = dialogConfirmDiscard`, `m.pendingConfirm =
   confirmQuit`, `m.confirmCursor = 0`, return `m, nil`.

### 4.6 Closing a tab

New method `func (m Model) closeTab(index int) (Model, tea.Cmd)`:

1. If `m.tabs[index].editor.HasUnsavedChanges()`: set `m.activeDialog =
   dialogConfirmDiscard`, `m.pendingConfirm = confirmCloseTab`,
   `m.pendingConfirmTab = index`, `m.confirmCursor = 0`; return `m, nil`
   (the actual removal happens on confirmation, §4.7).
2. Else, remove it immediately: `m.tabs = append(m.tabs[:index],
   m.tabs[index+1:]...)`. Reassign `activeTab`, checking in this order:
   - If `m.tabs` is now empty: `m.activeTab = -1`, `m.focus = focusTree`.
   - Else if the closed tab was not the active one and was before it in
     the slice (`index < m.activeTab`): decrement `m.activeTab` by 1 (it
     shifted left to fill the gap).
   - Else if the closed tab *was* the active one: prefer the tab now at
     the same index (the one that was to its right — it shifted left
     into the closed tab's slot); if that index is out of range (the
     closed tab was the last one), select `index - 1` instead.
   - Otherwise (closed tab was after the active one): `activeTab` is
     unchanged.

### 4.7 The confirmation dialog

New file `internal/app/confirm.go`, following the existing dialog files'
shape (`dialog.go` already has one `update*Dialog` + one `render*Dialog`
per dialog kind).

Rendering (`renderConfirmDialog`): a centered box (same sizing convention
as `renderAboutDialog`) showing:
- For `confirmQuit`: `"Unsaved changes in: <comma-joined dirty basenames>"`
  then `"Quit anyway"` / `"Cancel"` as two selectable lines.
- For `confirmCloseTab`: `"<basename> has unsaved changes."` then `"Close
  anyway"` / `"Cancel"`.
- The option at `m.confirmCursor` is rendered with `.Reverse(true)`.

Update (`updateConfirmDialog`):
- `up`/`down`/`left`/`right`: toggle `m.confirmCursor` between 0 and 1.
- `enter`: if `m.confirmCursor == 1` (Cancel) or always for safety-symmetry
  with `esc`, close the dialog without acting (`activeDialog = dialogNone`,
  `pendingConfirm = confirmNone`). If `m.confirmCursor == 0` ("anyway"):
  - `confirmQuit` → `return m, tea.Quit`.
  - `confirmCloseTab` → perform the §4.6-step-2 removal on
    `m.pendingConfirmTab`, then clear `activeDialog`/`pendingConfirm`.
- `esc`: always Cancel (matches every other dialog's Esc convention).

### 4.8 Modified indicator in the tree

`filetree.Model` gains:

```go
dirty map[string]bool // keyed by absolute path

func (m Model) SetDirty(dirty map[string]bool) Model {
	m.dirty = dirty
	return m
}
```

`View()`'s per-row rendering appends `" (M)"` (styled like `dirtyTabStyle`
above — reuse one exported-from-neither, package-local style per package,
same color `214`) after the name when `m.dirty[item.node.Path]` is true.

`app.Model`'s `View()` computes this map fresh every render (same rationale
as the existing per-render size re-derivation): before calling
`m.tree.View()`, build `dirty := map[string]bool{}`, set `dirty[tab.path] =
true` for every tab with `HasUnsavedChanges()`, and call `tree :=
m.tree.SetDirty(dirty)` alongside the existing `tree := m.tree.SetSize(...)`
call.

## 5. Layout summary (ASCII)

```
┌──────────────────────────────────────────────┐
│ File  Edit  Commands  About                   │  menu bar (existing)
├───────────┬────────────────────────────────────┤
│           │ main.go (M)× │ utils.go×          │  tab bar (new, editor column only)
│  project  ├────────────────────────────────────┤
│  tree     │                                    │
│  (M)      │           editor                   │
│  markers  │                                    │
│           ├────────────────────────────────────┤
│           │           terminal                  │
├───────────┴────────────────────────────────────┤
│ status bar                                      │  (existing)
└──────────────────────────────────────────────┘
```

## 6. Testing

- `editor`: `HasUnsavedChanges` true after an edit, false on a fresh load,
  false after save.
- `filetree`: `SetDirty` + `View()` shows `(M)` for a dirty path and not for
  a clean one.
- `app`:
  - Opening a new file appends a tab and activates it; opening an
    already-open file switches to its existing tab without creating a
    second one or resetting its cursor/scroll state.
  - Clicking a tab label in the tab bar switches `activeTab` and focuses
    the editor.
  - Clicking a tab's `×` on a clean tab closes it immediately and
    reassigns `activeTab` correctly (closing the active tab, closing a
    non-active tab before/after the active one, closing the last tab).
  - Clicking `×` on a dirty tab opens the confirm dialog instead of
    closing it; confirming closes it; canceling leaves it open.
  - `ctrl+q` with no dirty tabs quits immediately (existing test,
    unchanged).
  - `ctrl+q` with a dirty tab opens the confirm dialog instead of quitting;
    confirming produces `tea.Quit`; canceling returns to editing with the
    tab intact.
  - The tree shows `(M)` next to a file with a dirty open tab and not
    for other files.
