# File/Folder Creation, Rename, and Open/Closed Directory Icons — Design

## 1. Overview

This spec adds four related capabilities to Cody:

1. **Directory icons** reflect expand/collapse state (open-folder vs. closed-folder glyph).
2. **New File via menu**: File → New... opens a modal dialog (directory + filename fields) that creates an empty file and opens it in a new tab.
3. **Blank tab via Ctrl+N**: opens an empty, pathless ("untitled") tab immediately; saving it (Ctrl+S) prompts for a path via the same dialog component, relabeled "Save As".
4. **Right-click in the project tree**: a small context menu (New File / New Folder / always; Rename / only when a specific row was clicked) that drives inline, VS-Code-style in-tree name entry — no modal dialog for this path.

## 2. Global Constraints

- No new third-party dependencies (reuses `bubbles/textinput`, already a dependency).
- Creating a file/folder whose name already exists at the target path is an **error**, shown to the user — no overwrite-confirmation flow in this pass (kept out of scope deliberately, mirroring how the unsaved-changes feature didn't add an overwrite flow either).
- Renaming a file that is currently open in a tab does **not** update that tab's path — a known, accepted limitation of this pass (the tab keeps editing the file at its old, now-stale path handle; the OS-level file has moved). Not solving this now keeps the change bounded.
- All existing tests must keep passing.

## 3. Directory open/closed icons

`internal/filetree/icons.go`: `IconFor` currently returns one folder glyph regardless of `Node.Expanded`. It now returns an "open folder" glyph when `Expanded` is true, the existing "closed folder" glyph otherwise. New constants:

```go
nerdFontDirOpen = "" // nf-fa-folder-open (nerdFontDir "" is fa-folder, closed)
fallbackDirOpen = "~"      // distinct from fallbackDir "+" and fallbackFile "-"
```

## 4. `filetree` package: creation, rename, reload primitives

### 4.1 Package-level filesystem operations (`tree.go`)

```go
// CreateFile creates an empty file at path, failing if it already exists.
func CreateFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	return f.Close()
}

// CreateDir creates a directory at path, failing if it already exists.
func CreateDir(path string) error {
	return os.Mkdir(path, 0755)
}

// Rename renames oldPath to newPath, failing if newPath already exists.
func Rename(oldPath, newPath string) error {
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("%s already exists", filepath.Base(newPath))
	}
	return os.Rename(oldPath, newPath)
}
```

### 4.2 `Node.Reload` — refresh a directory's children while preserving state

`Node.LoadChildren` currently loads once and caches forever (`loaded` bool). A newly created file must appear without restarting the app, and reloading must **not** discard the `Expanded`/`loaded` state of unrelated already-expanded subdirectories (a naive "wipe and re-read" would collapse every other expanded folder the moment any one file is created anywhere in the tree).

Refactor `tree.go` so both `LoadChildren` and a new `Reload` share one core routine that re-reads directory entries and **reuses the existing `*Node`** for any entry whose name is still present (preserving that node's `Expanded`/`loaded`/`Children`), only constructing fresh `*Node`s for genuinely new entries:

```go
func (n *Node) LoadChildren() error {
	if n.Type != NodeDir || n.loaded {
		return nil
	}
	return n.reloadChildren()
}

// Reload re-reads this directory's entries from disk unconditionally (even
// if already loaded), adding new entries and dropping deleted ones, while
// preserving the existing Node — and its Expanded/loaded/Children state —
// for every entry still present. A no-op on a non-directory node.
func (n *Node) Reload() error {
	if n.Type != NodeDir {
		return nil
	}
	return n.reloadChildren()
}

func (n *Node) reloadChildren() error {
	entries, err := os.ReadDir(n.Path)
	if err != nil {
		return err
	}
	existing := make(map[string]*Node, len(n.Children))
	for _, c := range n.Children {
		existing[c.Name] = c
	}
	var children []*Node
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		if c, ok := existing[e.Name()]; ok {
			children = append(children, c)
			continue
		}
		childType := NodeFile
		if e.IsDir() {
			childType = NodeDir
		}
		children = append(children, &Node{
			Name: e.Name(),
			Path: filepath.Join(n.Path, e.Name()),
			Type: childType,
		})
	}
	sort.Slice(children, func(i, j int) bool {
		if children[i].Type != children[j].Type {
			return children[i].Type == NodeDir
		}
		return children[i].Name < children[j].Name
	})
	n.Children = children
	n.loaded = true
	return nil
}
```

### 4.3 `Model.SelectedDir`, `Model.ReloadDir`, `Model.findNode`

```go
// SelectedDir returns the directory context for creating a new file/folder:
// the selected node itself if it's a directory, its parent otherwise. Falls
// back to the tree's root when nothing is selected (e.g. an empty tree).
func (m Model) SelectedDir() string {
	if len(m.flat) == 0 || m.cursor < 0 || m.cursor >= len(m.flat) {
		return m.root.Path
	}
	n := m.flat[m.cursor].node
	if n == nil { // mid-edit phantom row (§6) — fall back to root
		return m.root.Path
	}
	if n.Type == NodeDir {
		return n.Path
	}
	return filepath.Dir(n.Path)
}

// findNode returns the tracked *Node at path, or nil if the tree has never
// loaded that far (e.g. an ancestor directory was never expanded).
func (m Model) findNode(path string) *Node {
	if m.root.Path == path {
		return m.root
	}
	var find func(n *Node) *Node
	find = func(n *Node) *Node {
		for _, c := range n.Children {
			if c.Path == path {
				return c
			}
			if c.Type == NodeDir {
				if found := find(c); found != nil {
					return found
				}
			}
		}
		return nil
	}
	return find(m.root)
}

// ReloadDir refreshes dirPath's children (auto-expanding it so a newly
// created entry is immediately visible) and rebuilds the flat list. A no-op
// if dirPath isn't currently tracked (e.g. it was never expanded) — in that
// case the new entry will show correctly the first time the directory is
// expanded anyway, since LoadChildren always does a fresh read on first
// expansion.
func (m Model) ReloadDir(dirPath string) Model {
	n := m.findNode(dirPath)
	if n == nil {
		return m
	}
	n.Expanded = true
	n.Reload()
	m.rebuildFlat()
	m.ensureCursorVisible()
	return m
}
```

## 5. `editor` package: untitled (pathless) buffers

### 5.1 New methods (`model.go`)

```go
// NewBlankBuffer starts editing a fresh, empty, pathless buffer — created
// by Ctrl+N, not tied to any file on disk until SaveAs gives it one.
func (m Model) NewBlankBuffer() Model {
	m.buf = &Buffer{Lines: []string{""}}
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
	m.scrollOffset = 0
	m.searchMatches = nil
	m.searchIndex = -1
	return m
}

// IsUntitled reports whether the buffer has never been saved to a path
// (created via NewBlankBuffer rather than LoadFile). Saving it requires a
// path from the user first — see SaveAs. False when no buffer is loaded.
func (m Model) IsUntitled() bool {
	return m.buf != nil && m.buf.Path == ""
}

// SaveAs writes the buffer's current content to path for the first time,
// attaching that path to the buffer, then runs the same language-detection
// LoadFile does at load time (an untitled buffer has no extension to detect
// a language from until now).
func (m Model) SaveAs(path string) (Model, error) {
	m.buf.Path = path
	if err := m.buf.Save(); err != nil {
		return m, err
	}
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
```

`Buffer{Lines: []string{""}}` (in `NewBlankBuffer`) leaves `Path` as its zero value `""`, which is exactly the sentinel `IsUntitled`/downstream code checks for — no change needed to the `Buffer` struct itself.

## 6. `filetree` package: inline create/rename editing + right-click menu

### 6.1 New `Model` fields

```go
type editMode int

const (
	editNone editMode = iota
	editCreatingFile
	editCreatingDir
	editRenaming
)

type contextMenuState struct {
	targetDir  string // directory new items are created in
	targetPath string // "" if the right-click hit empty space; else the clicked node's path (enables Rename)
}

// items lists this menu's actions in render/click order — Rename only
// when a specific row was right-clicked.
func (cm *contextMenuState) items() []string {
	items := []string{"New File", "New Folder"}
	if cm.targetPath != "" {
		items = append(items, "Rename")
	}
	return items
}
```

Added to `Model`:
```go
mode        editMode
editInput   textinput.Model
editTarget  string // create: target directory; rename: the node's current path
contextMenu *contextMenuState // nil when no menu is open
```

### 6.2 `FileTreeErrorMsg`

A new message type alongside the existing `FileOpenedMsg`, for reporting a failed create/rename to the app's status bar (mirrors how `FileOpenedMsg` already flows from `filetree` to `app`):

```go
type FileTreeErrorMsg struct {
	Message string
}
```

### 6.3 The phantom row (inline "type a new name" row)

Rather than a separate rendering overlay, an in-progress create is represented as one extra entry in `m.flat` itself — a `flatItem{node: nil, depth: ...}` — so all existing viewport/scroll/clip logic in `View()` treats it as just another row. `rebuildFlat` places it at the end of its target directory's visible children (root included, matching "new item appears at the bottom of the folder it's being created in"):

```go
func (m *Model) rebuildFlat() {
	m.flat = nil
	var walk func(n *Node, depth int)
	walk = func(n *Node, depth int) {
		for _, c := range n.Children {
			m.flat = append(m.flat, flatItem{node: c, depth: depth})
			if c.Type == NodeDir && c.Expanded {
				walk(c, depth+1)
			}
		}
		if n.Path == m.editTarget && (m.mode == editCreatingFile || m.mode == editCreatingDir) {
			m.flat = append(m.flat, flatItem{node: nil, depth: depth + 1})
		}
	}
	walk(m.root, 0)
}
```

A rename does **not** insert a phantom row — `View()` swaps in the edit input at the existing row for `m.editTarget` instead (§6.5).

Because `flatItem.node` can now be `nil`, every existing site that dereferences `item.node` must be reviewed: `View()`'s icon/name/dirty-marker rendering (guarded, §6.5) is the only one — `activateCurrent`/`collapseCurrent`/keyboard navigation never run while `m.mode != editNone` (Update routes to the edit-input handler first, §6.6), so they never see a phantom row.

### 6.4 Starting and finishing an edit

```go
func (m Model) startCreate(dir string, isDir bool) Model {
	if n := m.findNode(dir); n != nil {
		n.Expanded = true
	}
	m.mode = editCreatingFile
	if isDir {
		m.mode = editCreatingDir
	}
	m.editTarget = dir
	m.editInput = textinput.New()
	m.editInput.Focus()
	m.rebuildFlat()
	if idx := m.indexOfPhantom(); idx >= 0 {
		m.cursor = idx
	}
	m.ensureCursorVisible()
	return m
}

func (m Model) startRename(path string) Model {
	m.mode = editRenaming
	m.editTarget = path
	m.editInput = textinput.New()
	m.editInput.SetValue(filepath.Base(path))
	m.editInput.CursorEnd()
	m.editInput.Focus()
	if idx := m.indexOfNode(path); idx >= 0 {
		m.cursor = idx
	}
	m.ensureCursorVisible()
	return m
}

// indexOfPhantom / indexOfNode scan m.flat for the phantom row / a node's
// path, returning -1 if not found.
func (m Model) indexOfPhantom() int {
	for i, item := range m.flat {
		if item.node == nil {
			return i
		}
	}
	return -1
}

func (m Model) indexOfNode(path string) int {
	for i, item := range m.flat {
		if item.node != nil && item.node.Path == path {
			return i
		}
	}
	return -1
}

func (m Model) submitEdit() (Model, tea.Cmd) {
	name := m.editInput.Value()
	if name == "" {
		return m, func() tea.Msg { return FileTreeErrorMsg{Message: "name is required"} }
	}
	switch m.mode {
	case editCreatingFile:
		full := filepath.Join(m.editTarget, name)
		if err := CreateFile(full); err != nil {
			return m, func() tea.Msg { return FileTreeErrorMsg{Message: err.Error()} }
		}
		m.mode = editNone
		m = m.ReloadDir(m.editTarget)
		return m, func() tea.Msg { return FileOpenedMsg{Path: full} }
	case editCreatingDir:
		full := filepath.Join(m.editTarget, name)
		if err := CreateDir(full); err != nil {
			return m, func() tea.Msg { return FileTreeErrorMsg{Message: err.Error()} }
		}
		m.mode = editNone
		m = m.ReloadDir(m.editTarget)
		return m, nil
	case editRenaming:
		newPath := filepath.Join(filepath.Dir(m.editTarget), name)
		if err := Rename(m.editTarget, newPath); err != nil {
			return m, func() tea.Msg { return FileTreeErrorMsg{Message: err.Error()} }
		}
		m.mode = editNone
		m = m.ReloadDir(filepath.Dir(m.editTarget))
		return m, nil
	}
	return m, nil
}
```

A failed create/rename stays in edit mode (so the user can fix the name and retry) and reports the error via `FileTreeErrorMsg` — there's no room in a one-line tree row for an inline error message, so it surfaces through the app's status bar instead, same channel `recentCommand` already uses for every other transient status.

Creating a **file** additionally emits `FileOpenedMsg{Path: full}` — reusing the exact message the tree already emits for "user opened this file" — so `app.Update()`'s existing handling opens it in a new tab with no new plumbing. Creating a **folder** and **renaming** do not open anything.

### 6.5 Rendering

`View()`'s per-row loop gains: a phantom-row branch, a rename-in-place branch, and (when `m.contextMenu != nil`) `len(items)` menu rows always rendered first, ahead of the normal scrolled content — reducing the real-row budget by that many so the total emitted rows never exceeds `m.height` (the same technique the app package's tab bar already uses to carve height out of the editor pane):

```go
func (m Model) View() string {
	menuItems := []string(nil)
	if m.contextMenu != nil {
		menuItems = m.contextMenu.items()
	}
	realHeight := m.height
	if realHeight > 0 {
		realHeight -= len(menuItems)
	}
	clip := realHeight > 0
	viewStart, viewEnd := 0, len(m.flat)
	if clip {
		viewStart = m.scrollOffset
		// ... existing clamping, but against realHeight instead of m.height ...
		viewEnd = viewStart + realHeight
		// ...
	}
	// ... existing bar/contentWidth setup, unchanged ...

	var b strings.Builder
	for _, label := range menuItems {
		b.WriteString("  [" + label + "]\n")
	}
	for idx := viewStart; idx < viewEnd; idx++ {
		item := m.flat[idx]
		prefix := strings.Repeat("  ", item.depth)
		var line string
		switch {
		case item.node == nil:
			line = prefix + m.editInput.View()
		case m.mode == editRenaming && item.node.Path == m.editTarget:
			line = prefix + m.editInput.View()
		default:
			icon := IconFor(item.node, m.nerdFont)
			line = fmt.Sprintf("%s%s %s", prefix, icon, item.node.Name)
			if m.dirty[item.node.Path] {
				line += dirtyStyle.Render(" (M)")
			}
		}
		if idx == m.cursor {
			line = "> " + line
		} else {
			line = "  " + line
		}
		// ... existing padRow/scrollbar logic, unchanged ...
	}
	return strings.TrimRight(b.String(), "\n")
}
```

(The plan spells out the exact merged diff against the current `View()` — the elisions above are existing code that doesn't change.)

### 6.6 Click and key routing

`HandleClick` (mouse) checks, in order: an open context menu (row `y` selects that action, any other click dismisses it), an active edit mode (any click cancels it — clicking elsewhere while typing a name isn't given more precise semantics than "never mind"), then falls through to today's unchanged row-select/activate behavior:

```go
func (m Model) HandleClick(y int) (Model, tea.Cmd) {
	if m.contextMenu != nil {
		items := m.contextMenu.items()
		if y >= 0 && y < len(items) {
			return m.selectContextMenuItem(items[y]), nil
		}
		m.contextMenu = nil
		return m, nil
	}
	if m.mode != editNone {
		m.mode = editNone
		m.rebuildFlat()
		return m, nil
	}
	// ... existing body, unchanged ...
}

func (m Model) selectContextMenuItem(label string) Model {
	cm := m.contextMenu
	m.contextMenu = nil
	switch label {
	case "New File":
		return m.startCreate(cm.targetDir, false)
	case "New Folder":
		return m.startCreate(cm.targetDir, true)
	case "Rename":
		return m.startRename(cm.targetPath)
	}
	return m
}
```

`Update` (keyboard) routes to the edit-input handler first when `m.mode != editNone`, then to a minimal context-menu handler (Esc closes it; nothing else does anything, since it's mouse-driven) when `m.contextMenu != nil`, then falls through to today's unchanged up/down/left/right/enter switch:

```go
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.mode != editNone {
		return m.updateEditInput(keyMsg)
	}
	if m.contextMenu != nil {
		if keyMsg.String() == "esc" {
			m.contextMenu = nil
		}
		return m, nil
	}
	// ... existing switch, unchanged ...
}

func (m Model) updateEditInput(keyMsg tea.KeyMsg) (Model, tea.Cmd) {
	switch keyMsg.String() {
	case "esc":
		m.mode = editNone
		m.rebuildFlat()
		return m, nil
	case "enter":
		return m.submitEdit()
	}
	var cmd tea.Cmd
	m.editInput, cmd = m.editInput.Update(keyMsg)
	return m, cmd
}
```

Because `app.Model`'s own top-level Esc handling only special-cases the File/Edit dropdown menu and otherwise falls through to the focused pane's `Update` (see `app/model.go`'s `case "esc":`), Esc reaching the tree while focused there requires **no app-level change** — it flows straight into the handling above.

### 6.7 `HandleRightClick` and app-level wiring

```go
// HandleRightClick opens a context menu targeting row y (relative to the
// pane's content area, same convention as HandleClick) — the directory
// itself if it's a directory, its parent if a file, or the tree's root if y
// lands past the last row (empty space). Replaces any already-open menu,
// and cancels an in-progress create/rename first (a right-click clearly
// signals "do something else now" — abandoning it silently rather than
// leaving a stale phantom row or edit state behind).
func (m Model) HandleRightClick(y int) Model {
	if m.mode != editNone {
		m.mode = editNone
		m.rebuildFlat()
	}
	idx := m.scrollOffset + y
	if idx < 0 || idx >= len(m.flat) || m.flat[idx].node == nil {
		m.contextMenu = &contextMenuState{targetDir: m.root.Path}
		return m
	}
	n := m.flat[idx].node
	targetDir := n.Path
	if n.Type != NodeDir {
		targetDir = filepath.Dir(n.Path)
	}
	m.contextMenu = &contextMenuState{targetDir: targetDir, targetPath: n.Path}
	return m
}
```

`app.Model.Update`'s `tea.MouseMsg` switch gains a `tea.MouseButtonRight` case; a new `handleRightClick` (mirroring `handleClick`'s dialog/dropdown guards) hit-tests only the tree pane's rect (via the existing `paneLayout()`) and forwards to `m.tree.HandleRightClick`, focusing the tree. `app.Model.Update` also gains a `case filetree.FileTreeErrorMsg` next to the existing `case filetree.FileOpenedMsg`, setting `m.recentCommand` to the error text.

## 7. `app` package: New-file menu command, Ctrl+N blank tab, shared path-prompt dialog

### 7.1 Menu and shortcuts

`menuItemsFor("File")` gains a leading `"New"` item: `[]string{"New", "Open", "Save"}`. `buildCommands()` gains two entries:

```go
{Name: "New", Handler: cmdNewFilePrompt},               // menu-only, no shortcut
{Name: "New Tab", Shortcut: "ctrl+n", Handler: cmdNewBlankTab},
```

`Update`'s terminal-focus global-shortcut allowlist (`msg.String() == "ctrl+o" || msg.String() == "ctrl+q"`) gains `|| msg.String() == "ctrl+n"` — Ctrl+N is meant to be reachable the same way Open and Quit already are, regardless of which pane currently has focus.

`cmdNewBlankTab`:
```go
func cmdNewBlankTab(m Model) (Model, tea.Cmd) {
	editorW, editorH := m.newTabEditorSize()
	e := editor.New().NewBlankBuffer().SetSize(editorW, editorH)
	m.tabs = append(m.tabs, tab{path: "", editor: e})
	m.activeTab = len(m.tabs) - 1
	m.focus = focusEditor
	m.recentCommand = "New file"
	return m, nil
}
```

### 7.2 Untitled tab display name

`tab.path == ""` for a blank tab; `filepath.Base("")` returns `"."`, which must not leak into the UI. New helper (`tabs.go`):

```go
// tabDisplayName returns the name shown for a tab: "Untitled" for a
// pathless (never-saved) tab created via Ctrl+N, filepath.Base(path)
// otherwise.
func tabDisplayName(path string) string {
	if path == "" {
		return "Untitled"
	}
	return filepath.Base(path)
}
```

Every existing `filepath.Base(t.path)` / `filepath.Base(m.tabs[i].path)` call in `tabs.go` (`tabPlainLabel`, `renderTabBar`) and `confirm.go` (`renderConfirmDialog`, both occurrences) is replaced with `tabDisplayName(...)`.

### 7.3 `cmdSave` gains the untitled check

```go
func cmdSave(m Model) (Model, tea.Cmd) {
	if m.activeEditor().IsUntitled() {
		return openSaveAsPrompt(m)
	}
	e, cmd := m.activeEditor().Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = m.setActiveEditor(e)
	return m, cmd
}
```

### 7.4 The shared path-prompt dialog (new file `pathprompt.go`)

One dialog kind, `dialogPathPrompt`, serves both "New File" (menu) and "Save As" (saving an untitled buffer), distinguished by a small action enum — the same shared-dialog-by-action-enum shape `confirm.go` already established for quit/close-tab:

```go
type pathPromptAction int

const (
	pathPromptNone pathPromptAction = iota
	pathPromptNewFile
	pathPromptSaveAs
)
```

`Model` gains: `pathPromptAction pathPromptAction`, `pathDirInput textinput.Model`, `pathNameInput textinput.Model`, `pathPromptFocus int` (0 = directory field, 1 = filename field), `pathPromptError string`.

```go
func cmdNewFilePrompt(m Model) (Model, tea.Cmd) {
	return openPathPrompt(m, pathPromptNewFile), textinput.Blink
}

func openSaveAsPrompt(m Model) (Model, tea.Cmd) {
	return openPathPrompt(m, pathPromptSaveAs), textinput.Blink
}

func openPathPrompt(m Model, action pathPromptAction) Model {
	dirInput := textinput.New()
	dirInput.SetValue(m.tree.SelectedDir())
	dirInput.Focus()
	nameInput := textinput.New()
	nameInput.Placeholder = "filename.ext"

	m.pathPromptAction = action
	m.pathDirInput = dirInput
	m.pathNameInput = nameInput
	m.pathPromptFocus = 0
	m.pathPromptError = ""
	m.activeDialog = dialogPathPrompt
	return m
}

func (m Model) updatePathPromptDialog(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m.updatePathPromptInputs(msg)
	}
	switch keyMsg.String() {
	case "esc":
		m.activeDialog = dialogNone
		m.pathPromptAction = pathPromptNone
		return m, nil
	case "tab", "shift+tab":
		m.pathPromptFocus = 1 - m.pathPromptFocus
		m.pathDirInput.Blur()
		m.pathNameInput.Blur()
		if m.pathPromptFocus == 0 {
			m.pathDirInput.Focus()
		} else {
			m.pathNameInput.Focus()
		}
		return m, nil
	case "enter":
		return m.submitPathPrompt()
	}
	return m.updatePathPromptInputs(msg)
}

func (m Model) updatePathPromptInputs(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.pathPromptFocus == 0 {
		m.pathDirInput, cmd = m.pathDirInput.Update(msg)
	} else {
		m.pathNameInput, cmd = m.pathNameInput.Update(msg)
	}
	return m, cmd
}

func (m Model) submitPathPrompt() (tea.Model, tea.Cmd) {
	name := m.pathNameInput.Value()
	if name == "" {
		m.pathPromptError = "filename is required"
		return m, nil
	}
	dir := m.pathDirInput.Value()
	if dir == "" {
		dir = m.rootPath
	}
	full := name
	if !filepath.IsAbs(name) {
		full = filepath.Join(dir, name)
	}

	switch m.pathPromptAction {
	case pathPromptNewFile:
		if err := filetree.CreateFile(full); err != nil {
			m.pathPromptError = err.Error()
			return m, nil
		}
		m.tree = m.tree.ReloadDir(filepath.Dir(full))
		editorW, editorH := m.newTabEditorSize()
		updated, err := m.openOrSwitch(full, editorW, editorH)
		if err != nil {
			m.pathPromptError = err.Error()
			return m, nil
		}
		m = updated
		m.recentCommand = fmt.Sprintf("Created %s", filepath.Base(full))
	case pathPromptSaveAs:
		if _, err := os.Stat(full); err == nil {
			m.pathPromptError = fmt.Sprintf("%s already exists", filepath.Base(full))
			return m, nil
		}
		e, err := m.activeEditor().SaveAs(full)
		if err != nil {
			m.pathPromptError = err.Error()
			return m, nil
		}
		m = m.setActiveEditor(e)
		m = m.setActiveTabPath(full)
		m.tree = m.tree.ReloadDir(filepath.Dir(full))
		m.recentCommand = fmt.Sprintf("Saved %s", filepath.Base(full))
	}

	m.activeDialog = dialogNone
	m.pathPromptAction = pathPromptNone
	m.focus = focusEditor
	return m, nil
}

func renderPathPromptDialog(width, height int, m Model) string {
	title := "New File"
	if m.pathPromptAction == pathPromptSaveAs {
		title = "Save As"
	}
	content := title + "\n\n" +
		"Directory: " + m.pathDirInput.View() + "\n" +
		"Filename:  " + m.pathNameInput.View()
	if m.pathPromptError != "" {
		content += "\n\nError: " + m.pathPromptError
	}
	content += "\n\nTab to switch fields · Enter to confirm · Esc to cancel"
	return lipgloss.NewStyle().Width(width).Height(height).Padding(1, 2).Render(content)
}
```

`Model` also gains a small helper next to `setActiveEditor`:

```go
// setActiveTabPath rewrites the active tab's path — used once, by Save As,
// the moment an until-then-untitled buffer is first written to disk.
func (m Model) setActiveTabPath(path string) Model {
	if m.activeTab >= 0 && m.activeTab < len(m.tabs) {
		m.tabs[m.activeTab].path = path
	}
	return m
}
```

Wiring: `dialogKind` gains `dialogPathPrompt`; `Update` gains `if m.activeDialog == dialogPathPrompt { return m.updatePathPromptDialog(msg) }` alongside its sibling dialog checks; `View` gains a matching render branch alongside its siblings.

## 8. Accepted limitations (deliberately out of scope)

- No overwrite-confirmation flow for New File / Save As / rename-to-an-existing-name — all three simply error.
- Renaming a file open in a tab does not update that tab's path.
- Save-As to a path that happens to already be open in another tab does not merge/warn — two tabs could end up pointing at the same file.
- The right-click context menu always renders at the top of the tree pane's visible rows, not literally floating at the mouse position (terminal UIs generally can't do true floating overlays without much more machinery than this app has; this matches how the existing File/Edit dropdown is also a fixed-position element, not a floating one).

## 9. Testing

- `filetree`: icon reflects `Expanded` state; `CreateFile`/`CreateDir`/`Rename` succeed and fail-on-exists correctly; `Reload` preserves an unrelated sibling's `Expanded` state after a new file appears; `SelectedDir` for a selected dir/file/nothing; the full create-file flow (context menu → phantom row → Enter → `FileOpenedMsg` emitted, tree shows the new file); the full rename flow; Esc cancels either; a click anywhere while editing cancels; right-click on a file vs. a directory vs. empty space produces the right menu (`Rename` present/absent, correct `targetDir`).
- `editor`: `NewBlankBuffer` + `IsUntitled` true until `SaveAs`; `SaveAs` clears dirty, sets `Filetype()`, and picks up syntax highlighting for the new extension.
- `app`: File → New creates+opens a file and it appears in the tree; Ctrl+N opens a blank "Untitled" tab; typing then Ctrl+S on it opens the Save-As dialog (not a direct save); confirming Save As writes the file, renames the tab, and clears its dirty marker; canceling either dialog changes nothing; a name that already exists shows an error and stays in the dialog; right-clicking the tree and creating a file opens it in a new tab.
