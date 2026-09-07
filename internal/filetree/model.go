package filetree

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cody/internal/scrollbar"
)

type FileOpenedMsg struct {
	Path string
}

// FileTreeErrorMsg reports a failed create/rename to the app, which shows
// it in the status bar the same way it already shows other transient
// status messages.
type FileTreeErrorMsg struct {
	Message string
}

type editMode int

const (
	editNone editMode = iota
	editCreatingFile
	editCreatingDir
	editRenaming
)

// contextMenuState describes an open create/rename menu: where new items
// would be created, and (if a specific row was targeted) which one, since
// that's what enables the Rename option. Opened either by right-click
// (HandleRightClick) or by keyboard (OpenContextMenu, for terminals that
// don't forward right-click reliably — some report it as a left click at
// the wire-protocol level, outside this app's control).
type contextMenuState struct {
	targetDir  string
	targetPath string // "" if no specific row was targeted
	selected   int    // which item is highlighted, for keyboard navigation
}

// items lists this menu's actions in render/click order — Rename only
// appears when a specific row was right-clicked.
func (cm *contextMenuState) items() []string {
	items := []string{"New File", "New Folder"}
	if cm.targetPath != "" {
		items = append(items, "Rename")
	}
	return items
}

// scrollbarGutterWidth reserves one space plus one rune for the scrollbar
// column appended to each rendered row.
const scrollbarGutterWidth = 2

var scrollbarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
var dirtyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

type flatItem struct {
	node  *Node
	depth int
}

type Model struct {
	root         *Node
	nerdFont     bool
	flat         []flatItem
	cursor       int
	width        int
	height       int
	scrollOffset int
	dirty        map[string]bool

	mode        editMode
	editInput   textinput.Model
	editTarget  string // create: target directory; rename: the node's current path
	contextMenu *contextMenuState
}

func New(rootPath string, nerdFont bool) (Model, error) {
	root, err := NewRoot(rootPath)
	if err != nil {
		return Model{}, err
	}
	m := Model{root: root, nerdFont: nerdFont}
	m.rebuildFlat()
	return m, nil
}

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
	// rebuildFlat can shrink the list (e.g. cancelling an in-progress
	// create removes the phantom row m.cursor was parked on) — clamp here,
	// the one place the list length actually changes, rather than at every
	// caller.
	if m.cursor >= len(m.flat) {
		m.cursor = len(m.flat) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

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

// SelectedDir returns the directory context for creating a new file/folder:
// the selected node itself if it's a directory, its parent otherwise. Falls
// back to the tree's root when nothing is selected (e.g. an empty tree, or
// the selection is currently the phantom "typing a new name" row).
func (m Model) SelectedDir() string {
	if len(m.flat) == 0 || m.cursor < 0 || m.cursor >= len(m.flat) {
		return m.root.Path
	}
	n := m.flat[m.cursor].node
	if n == nil {
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
// if dirPath isn't currently tracked (e.g. it was never expanded) — the new
// entry will show correctly the first time the directory is expanded
// anyway, since LoadChildren always does a fresh read on first expansion.
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

func (m Model) startCreate(dir string, isDir bool) Model {
	if n := m.findNode(dir); n != nil {
		n.Expanded = true
		n.LoadChildren()
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

// HandleRightClick opens a context menu targeting row y (relative to the
// pane's content area, same convention as HandleClick) — the directory
// itself if it's a directory, its parent if a file, or the tree's root if y
// lands past the last row (empty space). Replaces any already-open menu,
// and cancels an in-progress create/rename first (a right-click clearly
// signals "do something else now" rather than leaving a stale phantom row
// or edit state behind).
func (m Model) HandleRightClick(y int) Model {
	// Resolve the target against the state that was actually on screen for
	// this click — before any mutation the click itself triggers. If a
	// menu is already open, its rows were drawn above the tree content, so
	// their count must be subtracted from y first. If an edit is in
	// progress, its phantom row is still part of m.flat right now; capture
	// the target *Node pointer before cancelling (which rebuilds m.flat
	// and would invalidate a plain index into it).
	relY := y
	if m.contextMenu != nil {
		relY -= len(m.contextMenu.items())
	}
	idx := m.scrollOffset + relY

	var target *Node
	if idx >= 0 && idx < len(m.flat) {
		target = m.flat[idx].node // nil if idx lands on the phantom row itself
	}

	if m.mode != editNone {
		m.mode = editNone
		m.rebuildFlat()
	}

	if target == nil {
		m.contextMenu = &contextMenuState{targetDir: m.root.Path}
		return m
	}
	targetDir := target.Path
	if target.Type != NodeDir {
		targetDir = filepath.Dir(target.Path)
	}
	m.contextMenu = &contextMenuState{targetDir: targetDir, targetPath: target.Path}
	return m
}

// OpenContextMenu opens the create/rename menu targeting the currently
// selected row — the same menu HandleRightClick opens, reached by keyboard
// instead of a mouse click. A keyboard-accessible fallback for terminals
// that don't reliably forward right-click to the app (some report it using
// the same wire-protocol code as a left click, which this app has no way
// to distinguish or work around).
func (m Model) OpenContextMenu() Model {
	if m.mode != editNone {
		m.mode = editNone
		m.rebuildFlat()
	}
	if len(m.flat) == 0 || m.cursor < 0 || m.cursor >= len(m.flat) {
		m.contextMenu = &contextMenuState{targetDir: m.root.Path}
		return m
	}
	n := m.flat[m.cursor].node
	if n == nil {
		m.contextMenu = &contextMenuState{targetDir: m.root.Path}
		return m
	}
	targetDir := n.Path
	if n.Type != NodeDir {
		targetDir = filepath.Dir(n.Path)
	}
	m.contextMenu = &contextMenuState{targetDir: targetDir, targetPath: n.Path}
	return m
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

// indexOfPhantom returns the flat-list index of the "typing a new name"
// row, or -1 if there isn't one.
func (m Model) indexOfPhantom() int {
	for i, item := range m.flat {
		if item.node == nil {
			return i
		}
	}
	return -1
}

// indexOfNode returns the flat-list index of the node at path, or -1 if
// it isn't currently visible (e.g. its parent directory is collapsed).
func (m Model) indexOfNode(path string) int {
	for i, item := range m.flat {
		if item.node != nil && item.node.Path == path {
			return i
		}
	}
	return -1
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

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.mode != editNone {
		return m.updateEditInput(keyMsg)
	}
	if m.contextMenu != nil {
		items := m.contextMenu.items()
		switch keyMsg.String() {
		case "esc":
			m.contextMenu = nil
		case "up", "k":
			m.contextMenu.selected--
			if m.contextMenu.selected < 0 {
				m.contextMenu.selected = len(items) - 1
			}
		case "down", "j":
			m.contextMenu.selected++
			if m.contextMenu.selected >= len(items) {
				m.contextMenu.selected = 0
			}
		case "enter":
			m = m.selectContextMenuItem(items[m.contextMenu.selected])
		}
		return m, nil
	}
	var cmd tea.Cmd
	switch keyMsg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.flat)-1 {
			m.cursor++
		}
	case "left", "h":
		m.collapseCurrent()
	case "right", "l", "enter":
		m, cmd = m.activateCurrent()
	case "m":
		m = m.OpenContextMenu()
	}
	m.ensureCursorVisible()
	return m, cmd
}

// ensureCursorVisible scrolls the viewport so the selected item stays
// within it. A no-op when no height has ever been set (m.height <= 0),
// which preserves the unbounded rendering every pre-existing caller relies
// on.
func (m *Model) ensureCursorVisible() {
	if m.height <= 0 {
		return
	}
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	if m.cursor >= m.scrollOffset+m.height {
		m.scrollOffset = m.cursor - m.height + 1
	}
	maxOffset := len(m.flat) - m.height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m *Model) collapseCurrent() {
	if len(m.flat) == 0 {
		return
	}
	n := m.flat[m.cursor].node
	if n.Type == NodeDir && n.Expanded {
		n.Expanded = false
		m.rebuildFlat()
	}
}

// HandleClick selects the row at y (relative to the pane's own content
// area, matching what View() rendered — an index into the visible rows, not
// the full flat list) and activates it exactly as Enter would: expanding or
// collapsing a directory, or opening a file. A click past the last item is
// a no-op.
func (m Model) HandleClick(y int) (Model, tea.Cmd) {
	if m.contextMenu != nil {
		items := m.contextMenu.items()
		if m.height > 0 && len(items) > m.height {
			items = items[:m.height]
		}
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
	idx := m.scrollOffset + y
	if idx < 0 || idx >= len(m.flat) {
		return m, nil
	}
	m.cursor = idx
	var cmd tea.Cmd
	m, cmd = m.activateCurrent()
	m.ensureCursorVisible()
	return m, cmd
}

// Scroll moves the selection n rows (negative scrolls up, positive scrolls
// down) and re-clamps the viewport, for mouse-wheel scrolling — mirrors
// repeated up/down key presses rather than introducing independent
// scroll-only state.
func (m Model) Scroll(n int) Model {
	if len(m.flat) == 0 {
		return m
	}
	m.cursor += n
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(m.flat)-1 {
		m.cursor = len(m.flat) - 1
	}
	m.ensureCursorVisible()
	return m
}

func (m Model) activateCurrent() (Model, tea.Cmd) {
	if len(m.flat) == 0 {
		return m, nil
	}
	n := m.flat[m.cursor].node
	if n.Type == NodeDir {
		if !n.Expanded {
			n.Expanded = true
			if err := n.LoadChildren(); err != nil {
				return m, nil
			}
		} else {
			n.Expanded = false
		}
		m.rebuildFlat()
		return m, nil
	}
	path := n.Path
	return m, func() tea.Msg { return FileOpenedMsg{Path: path} }
}

func (m Model) View() string {
	var menuItems []string
	if m.contextMenu != nil {
		menuItems = m.contextMenu.items()
	}
	clip := m.height > 0
	if clip && len(menuItems) > m.height {
		menuItems = menuItems[:m.height]
	}
	realHeight := m.height - len(menuItems)
	if realHeight < 0 {
		realHeight = 0
	}
	viewStart, viewEnd := 0, len(m.flat)
	if clip {
		viewStart = m.scrollOffset
		if viewStart < 0 {
			viewStart = 0
		}
		if viewStart > len(m.flat) {
			viewStart = len(m.flat)
		}
		viewEnd = viewStart + realHeight
		if viewEnd > len(m.flat) {
			viewEnd = len(m.flat)
		}
		if viewEnd < viewStart {
			viewEnd = viewStart
		}
	}

	var bar []rune
	contentWidth := 0
	if clip {
		bar = scrollbar.Column(len(m.flat), viewEnd-viewStart, viewStart)
		contentWidth = m.width - scrollbarGutterWidth
		if contentWidth < 0 {
			contentWidth = 0
		}
	}

	var b strings.Builder
	for i, label := range menuItems {
		prefix := "  "
		if m.contextMenu != nil && i == m.contextMenu.selected {
			prefix = "> "
		}
		b.WriteString(prefix + "[" + label + "]\n")
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
		if clip {
			// Pad (never wrap or truncate) to the pane's known content width
			// so the bar lands in a fixed column at the right edge, forming
			// a straight vertical scrollbar — appending it directly after
			// variable-length text makes it look like a stray character
			// attached to each line. lipgloss.Style.Width().Render() was
			// tried here first, but it hard-wraps rows wider than the
			// target width instead of leaving them alone, which silently
			// reintroduces multi-line overflow per row — padRow only pads.
			line = padRow(line, contentWidth) + " " + scrollbarStyle.Render(string(bar[idx-viewStart]))
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// padRow right-pads row with spaces to reach the given visible (ANSI-aware)
// width. Rows already at or beyond that width are returned unchanged —
// never wrapped, never truncated.
func padRow(row string, width int) string {
	if pad := width - lipgloss.Width(row); pad > 0 {
		return row + strings.Repeat(" ", pad)
	}
	return row
}
