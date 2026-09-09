package app

import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cody/internal/editor"
	"cody/internal/filetree"
	"cody/internal/statusbar"
	"cody/internal/terminal"
)

type focusArea int

const (
	focusTree focusArea = iota
	focusEditor
	focusTerminal
)

const (
	defaultTreeWidth      = 30
	statusBarHeight       = 1
	menuBarHeight         = 1
	defaultTerminalHeight = 8
	borderSize            = 2 // lipgloss.NormalBorder adds 1 cell on each side

	// mouseWheelLines is how many rows a single wheel notch scrolls.
	mouseWheelLines = 3

	// tabBarHeight is the height of the tab strip shown above the editor
	// pane once at least one file is open. Note: on a terminal already too
	// small to fit tree/editor/terminal without overflow, opening a first
	// tab consumes one more row from the editor pane, making that
	// pre-existing overflow exactly one row worse — this is a known,
	// pre-existing overflow class and not something tabBarHeight itself
	// can fix.
	tabBarHeight = 1

	// Pane-resize clamps: how small a dragged pane may shrink to, and how
	// much room the pane on the other side of the drag must always keep.
	minTreeWidth      = 15
	minEditorWidth    = 20
	minEditorHeight   = 3
	minTerminalHeight = 3
)

var (
	focusedBorderColor   = lipgloss.Color("205")
	unfocusedBorderColor = lipgloss.Color("240")
)

// resizeKind identifies which pane boundary a mouse drag is currently
// resizing, if any.
type resizeKind int

const (
	resizeNone resizeKind = iota
	resizeTree
	resizeTerminal
	resizeEditorSplit
)

// tab is one open file: its absolute path (the key used to detect an
// already-open file and to avoid duplicate tabs) and its own independent
// editor state (cursor, undo history, scroll position, folds, ...).
type tab struct {
	path   string
	editor editor.Model
}

// editorPane is one editor group: its own open tabs and which one is
// active. len(app.Model.panes) == 1 means a single unsplit editor area;
// == 2 means the editor area is split left/right, panes[0] on the left.
type editorPane struct {
	tabs      []tab
	activeTab int // -1 when this pane has no tabs open
}

type Model struct {
	tree          filetree.Model
	panes         []editorPane // len 1 (unsplit) or 2 (split); never 0
	activePane    int          // which pane index keyboard/mouse edits target
	terminal      terminal.Model
	focus         focusArea
	projectName   string
	recentCommand string
	width, height int
	commands      []Command
	rootPath      string
	openMenu      string
	tabMenu       *tabContextMenu // nil when no tab-menu is open

	// treeWidth and terminalHeight are the tree and terminal panes' current
	// on-screen sizes (border included) — mutable, unlike their
	// defaultTreeWidth/defaultTerminalHeight starting values, because the
	// user can drag either pane's boundary to resize it. resizeDrag tracks
	// an in-progress drag between the mouse-down that started it and the
	// button-release that ends it.
	treeWidth      int
	terminalHeight int
	resizeDrag     resizeKind
	activeDialog   dialogKind
	fileOpenInput  textinput.Model
	fileOpenError  string
	paletteFilter  textinput.Model
	paletteCursor  int
	searchInput    textinput.Model

	pendingConfirm     confirmAction
	pendingConfirmPane int // meaningful only when pendingConfirm == confirmCloseTab
	pendingConfirmTab  int // meaningful only when pendingConfirm == confirmCloseTab
	confirmCursor      int // 0 = "anyway", 1 = "Cancel"

	pathPromptAction pathPromptAction
	pathDirInput     textinput.Model
	pathNameInput    textinput.Model
	pathPromptFocus  int
	pathPromptError  string

	splitCol int // on-screen column of the boundary between panes[0] and panes[1], meaningful only when len(panes) == 2
}

func New(rootPath string, nerdFont bool) (Model, error) {
	absPath, err := filepath.Abs(rootPath)
	if err != nil {
		return Model{}, err
	}
	tree, err := filetree.New(absPath, nerdFont)
	if err != nil {
		return Model{}, err
	}
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
}

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

// paneWidths returns each pane's on-screen width (border included):
// unsplit, the one pane spans the full editor column; split, each spans
// its own share either side of m.splitCol. Shared by View()'s rendering,
// newTabEditorSize, and Update's WindowSizeMsg resize loop so they can't
// drift out of sync with each other.
func (m Model) paneWidths() []int {
	widths := make([]int, len(m.panes))
	if len(m.panes) == 1 {
		widths[0] = m.width - m.treeWidth
	} else {
		widths[0] = m.splitCol - m.treeWidth
		widths[1] = m.width - m.splitCol
	}
	return widths
}

// newTabEditorSize computes the width/height a newly opened tab's editor
// should be sized at, matching what Update's WindowSizeMsg branch and
// View() compute for the active editor. It always uses the "at least one
// tab" editor-height formula (i.e. reserves tabBarHeight), even if m.tabs
// is currently empty: the tab being opened is about to make m.tabs
// non-empty, so the tab bar is about to appear.
//
// The result is floored at 0: on a window smaller than the panes' combined
// minimums, clampTreeWidth/clampTerminalHeight's degenerate handling (see
// clampInt) can still leave this arithmetic negative, and a negative size
// must never reach a pane's SetSize.
func (m Model) newTabEditorSize() (width, height int) {
	paneHeight := m.height - menuBarHeight - statusBarHeight
	editorHeight := paneHeight - m.terminalHeight - tabBarHeight
	widths := m.paneWidths()
	w := m.width - m.treeWidth // fallback if activePane is somehow out of range
	if m.activePane >= 0 && m.activePane < len(widths) {
		w = widths[m.activePane]
	}
	return max(0, w-borderSize), max(0, editorHeight-borderSize)
}

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
	// Land the moved tab in the target pane BEFORE removing it from the
	// source: removeTab's empty-pane collapse rule (see its own doc
	// comment) inspects both panes' tab counts, and if target were still
	// empty at that point (e.g. the pane we just created above, before it
	// has anything in it), removeTab would mistake it for the emptied side
	// and immediately collapse the split back away — undoing the move
	// before it happens. Appending first means both panes are already in
	// their final shape by the time removeTab's collapse check runs.
	tp := &m.panes[target]
	tp.tabs = append(tp.tabs, moved)
	tp.activeTab = len(tp.tabs) - 1
	// removeTab does the removal + activeTab reassignment + empty-pane
	// collapse (see its own doc comment) — shared with a plain tab close
	// rather than duplicated here. If removing the source's only tab
	// collapsed the split away (source was pane 0's only tab, target was
	// the pane we just created above), "target" as an index is now stale —
	// the collapse rule always keeps the survivor in slot 0, so recompute
	// which pane the moved tab actually ended up in.
	m = m.removeTab(pane, index)
	if len(m.panes) == 1 {
		target = 0
	}
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
	result := clampInt(col, m.treeWidth+minEditorWidth, m.width-minEditorWidth)
	// On a window too small even for its own minimums, clampTreeWidth's
	// degenerate handling (see clampInt) can still leave m.treeWidth at
	// minTreeWidth, pushing this clamp's own lo bound (m.treeWidth+
	// minEditorWidth) past m.width-minEditorWidth — clampInt's hi<lo
	// branch then returns that inflated lo verbatim, past m.width itself,
	// which inverts pane 1's rect (width = m.width-splitCol < 0). Capping
	// at m.width here can't invert pane 0's rect in exchange: its width is
	// splitCol-m.treeWidth, and splitCol's lo bound is always
	// m.treeWidth+minEditorWidth, so splitCol never drops below
	// m.treeWidth regardless of which clampInt branch produced it.
	return min(result, m.width)
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		// Re-clamp any pane sizes the user dragged away from their
		// defaults: a resize the user made at the old window size could
		// now overflow (or invert) the new one.
		m.treeWidth = m.clampTreeWidth(m.treeWidth)
		m.terminalHeight = m.clampTerminalHeight(m.terminalHeight)
		if len(m.panes) == 2 {
			m.splitCol = m.clampSplitCol(m.splitCol)
		}
		// A resize can also invalidate an open tab menu's anchor the same
		// way narrowing a pane's tab bar via a drag can (see
		// tabMenuStartCol's doc comment) — check and clear it here, the
		// one place model state actually gets to change in response to a
		// resize (View() cannot mutate m.tabMenu itself).
		if m.tabMenu != nil {
			if _, ok := m.tabMenuStartCol(); !ok {
				m.tabMenu = nil
			}
		}
		paneHeight := m.height - menuBarHeight - statusBarHeight
		editorHeight := paneHeight - m.terminalHeight - m.tabBarH()
		// clampTreeWidth/clampTerminalHeight's degenerate-window handling
		// (see clampInt) can still leave these negative on a window smaller
		// than the panes' combined minimums (aggressive resizing) — floor
		// every value handed to a pane's SetSize at 0, since a negative
		// size reaching the terminal's real vt.Emulator panics rather than
		// degrading gracefully.
		m.tree = m.tree.SetSize(max(0, m.treeWidth-borderSize), max(0, paneHeight-borderSize))
		// Size every open tab's editor in every pane, not just the active
		// one, and each at its OWN pane's width (paneWidths already
		// accounts for the split, if any) — a background tab left at its
		// stale (or zero) size would treat itself as "unbounded" once
		// switched to or clicked in, breaking its scroll-offset math (see
		// openOrSwitch's doc comment).
		widths := m.paneWidths()
		for pi := range m.panes {
			pw := max(0, widths[pi]-borderSize)
			for i := range m.panes[pi].tabs {
				m.panes[pi].tabs[i].editor = m.panes[pi].tabs[i].editor.SetSize(pw, max(0, editorHeight-borderSize))
			}
		}
		termW := max(0, m.width-m.treeWidth-borderSize)
		m.terminal = m.terminal.SetSize(termW, max(0, m.terminalHeight-borderSize))
		return m, nil
	}
	if _, ok := msg.(editor.RehighlightMsg); ok {
		e, cmd := m.activeEditor().Update(msg)
		m = m.setActiveEditor(e)
		return m, cmd
	}
	if _, ok := msg.(terminal.OutputMsg); ok {
		var cmd tea.Cmd
		m.terminal, cmd = m.terminal.Update(msg)
		return m, cmd
	}
	if _, ok := msg.(terminal.ReadErrMsg); ok {
		var cmd tea.Cmd
		m.terminal, cmd = m.terminal.Update(msg)
		return m, cmd
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
	if m.activeDialog == dialogFileOpen {
		return m.updateFileOpenDialog(msg)
	}
	if m.activeDialog == dialogAbout {
		return m.updateAboutDialog(msg)
	}
	if m.activeDialog == dialogPalette {
		return m.updatePaletteDialog(msg)
	}
	if m.activeDialog == dialogSearch {
		return m.updateSearchDialog(msg)
	}
	if m.activeDialog == dialogConfirmDiscard {
		return m.updateConfirmDialog(msg)
	}
	if m.activeDialog == dialogPathPrompt {
		return m.updatePathPromptDialog(msg)
	}
	switch msg := msg.(type) {
	case tea.MouseMsg:
		switch msg.Action {
		case tea.MouseActionMotion:
			if m.resizeDrag != resizeNone {
				return m.applyResizeDrag(msg.X, msg.Y), nil
			}
			return m, nil
		case tea.MouseActionRelease:
			m.resizeDrag = resizeNone
			return m, nil
		case tea.MouseActionPress:
			switch msg.Button {
			case tea.MouseButtonLeft:
				if updated, handled := m.beginResizeDrag(msg.X, msg.Y); handled {
					return updated, nil
				}
				return m.handleClick(msg.X, msg.Y)
			case tea.MouseButtonRight:
				return m.handleRightClick(msg.X, msg.Y)
			case tea.MouseButtonWheelUp:
				return m.handleWheel(msg.X, msg.Y, -mouseWheelLines)
			case tea.MouseButtonWheelDown:
				return m.handleWheel(msg.X, msg.Y, mouseWheelLines)
			}
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			m.focusNext()
			return m.maybeStartTerminal()
		case "shift+tab":
			m.focusPrev()
			return m.maybeStartTerminal()
		case "esc":
			if m.tabMenu != nil {
				m.tabMenu = nil
				return m, nil
			}
			if m.openMenu != "" {
				m.openMenu = ""
				return m, nil
			}
		default:
			if m.focus != focusTerminal {
				if cmd, ok := commandForShortcut(m.commands, msg.String()); ok {
					return cmd.Handler(m)
				}
			} else if msg.String() == "ctrl+o" || msg.String() == "ctrl+q" || msg.String() == "ctrl+n" {
				if cmd, ok := commandForShortcut(m.commands, msg.String()); ok {
					return cmd.Handler(m)
				}
			}
		}
	case filetree.FileOpenedMsg:
		editorW, editorH := m.newTabEditorSize()
		updated, err := m.openOrSwitch(msg.Path, editorW, editorH)
		if err != nil {
			m.recentCommand = fmt.Sprintf("Open failed: %s", err)
			return m, nil
		}
		m = updated
		m.focus = focusEditor
		m.recentCommand = fmt.Sprintf("Opened %s", filepath.Base(msg.Path))
		return m, nil
	case filetree.FileTreeErrorMsg:
		m.recentCommand = msg.Message
		return m, nil
	case editor.CommandExecutedMsg:
		m.recentCommand = msg.Description
		return m, nil
	}

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
}

func (m Model) handleClick(x, y int) (tea.Model, tea.Cmd) {
	if m.activeDialog != dialogNone {
		return m, nil
	}
	if m.tabMenu != nil {
		startCol, ok := m.tabMenuStartCol()
		if !ok {
			m.tabMenu = nil
			return m, nil
		}
		rendered := renderTabMenu(*m.tabMenu, startCol)
		// Measure width off an unmargined render: renderTabMenu bakes
		// startCol in as MarginLeft, so lipgloss.Width(rendered) would
		// count that margin as part of the box, making the clickable
		// zone startCol-columns too wide and accepting clicks well past
		// the visible menu.
		width := lipgloss.Width(renderTabMenu(*m.tabMenu, 0))
		top := m.tabMenuTop()
		height := lipgloss.Height(rendered)
		if x >= startCol && x < startCol+width && y >= top && y < top+height {
			m = m.selectTabMenuItem(tabMenuItems(*m.tabMenu)[0])
			return m, nil
		}
		m.tabMenu = nil
		return m, nil
	}
	if y == 0 {
		name, ok := menuLabelAt(x)
		if !ok {
			m.openMenu = ""
			return m, nil
		}
		return m.clickMenuLabel(name)
	}
	if m.openMenu != "" {
		label, ok := findLabel(m.openMenu)
		if !ok {
			m.openMenu = ""
			return m, nil
		}
		dropdown := renderDropdown(m.openMenu, m.commands)
		width := lipgloss.Width(dropdown)
		if x < label.startCol || x >= label.startCol+width {
			m.openMenu = ""
			return m, nil
		}
		items := menuItemsFor(m.openMenu)
		// -menuBarHeight skips the menu bar row the dropdown opens below;
		// -dropdownBorderSize skips its own top border row.
		row := y - menuBarHeight - dropdownBorderSize
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
	return m.handlePaneClick(x, y)
}

// rect is a screen-space rectangle, half-open on both axes: it contains x
// in [x0, x1) and y in [y0, y1).
type rect struct{ x0, y0, x1, y1 int }

func (r rect) contains(x, y int) bool {
	return x >= r.x0 && x < r.x1 && y >= r.y0 && y < r.y1
}

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
	tabMenuHeight := 0
	if m.tabMenu != nil {
		// The rendered height doesn't depend on the anchor column (only
		// its horizontal position does), so a placeholder column is fine
		// here — this call exists purely to measure height, never to
		// compute where the menu actually renders.
		tabMenuHeight = lipgloss.Height(renderTabMenu(*m.tabMenu, 0))
	}
	bodyTop := menuBarHeight + dropdownHeight + tabMenuHeight
	bodyHeight := paneHeight - dropdownHeight - tabMenuHeight
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

// tabMenuTop returns the screen row m.tabMenu (if open) starts rendering
// at: directly below the menu bar and any open File/Edit dropdown, above
// the body — the same row the dropdown itself opens at.
func (m Model) tabMenuTop() int {
	dropdownHeight := 0
	if m.openMenu != "" {
		dropdownHeight = lipgloss.Height(renderDropdown(m.openMenu, m.commands))
	}
	return menuBarHeight + dropdownHeight
}

// tabMenuStartCol computes the screen column m.tabMenu should be anchored
// at (matching the right-clicked tab's own column), or ok=false if the tab
// it's anchored to no longer has a visible region in its pane's tab bar —
// e.g. a resize or drag narrowed that bar since the menu opened. Callers
// must treat ok=false as "this tabMenu is stale" and not index into
// anything with m.tabMenu.index.
func (m Model) tabMenuStartCol() (col int, ok bool) {
	if m.tabMenu == nil {
		return 0, false
	}
	_, panes, _ := m.paneLayout()
	if m.tabMenu.pane < 0 || m.tabMenu.pane >= len(panes) {
		return 0, false
	}
	pl := panes[m.tabMenu.pane]
	regions := tabRegions(m.panes[m.tabMenu.pane].tabs, pl.tabBar.x1-pl.tabBar.x0)
	if m.tabMenu.index < 0 || m.tabMenu.index >= len(regions) {
		return 0, false
	}
	return pl.tabBar.x0 + regions[m.tabMenu.index].startCol, true
}

// handlePaneClick routes a click that landed outside the menu bar and any
// open dropdown to whichever pane's rectangle contains it, switching focus
// there and forwarding the click for pane-specific handling (tree row
// selection, editor cursor placement). A click inside no pane (e.g. on a
// border, or before the first WindowSizeMsg) is a no-op.
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
			if relX >= region.closeStart && relX < region.closeEnd {
				// Deliberately does NOT set m.activePane = pi first — a
				// close action on a non-active pane's tab shouldn't
				// redirect where the next keystroke goes, matching
				// handleWheel's existing "only a deliberate select
				// switches panes" precedent below.
				return m.closeTab(pi, region.tabIndex)
			}
			m.activePane = pi
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

// handleRightClick opens the tree's context menu when the click landed in
// the tree pane, or the tab context menu when it landed on any pane's tab
// bar; a no-op everywhere else, or while a dialog/dropdown is open.
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

// handleWheel scrolls whichever pane's rectangle contains (x, y) — the pane
// under the pointer, not necessarily the focused one — by delta lines
// (negative scrolls up). No-op outside any pane, over the terminal (which
// has no independent scroll-only view), or while a dialog or dropdown is
// open.
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

// beginResizeDrag starts a pane-resize drag if (x, y) landed on a draggable
// pane boundary: the tree pane's right border (drag horizontally to resize
// its width) or the row between the editor and terminal panes (drag
// vertically to resize the terminal's height). Checked ahead of the normal
// click routing so grabbing a border resizes instead of also selecting a
// tree row or placing the cursor. Reports handled=false everywhere else, or
// while a dialog/dropdown is open — resizing then would move panes the
// user can't see change.
func (m Model) beginResizeDrag(x, y int) (Model, bool) {
	if m.activeDialog != dialogNone || m.openMenu != "" || m.tabMenu != nil {
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

// applyResizeDrag updates the pane boundary named by m.resizeDrag to track
// the pointer at (x, y), clamped so neither pane can be dragged into (or
// past) collapse. A no-op if no drag is in progress.
func (m Model) applyResizeDrag(x, y int) Model {
	switch m.resizeDrag {
	case resizeTree:
		m.treeWidth = m.clampTreeWidth(x + 1)
		// Dragging the tree border can push m.treeWidth past the split
		// boundary the same way a window resize can (clampTreeWidth only
		// knows about minEditorWidth, not m.splitCol) — re-clamp so the
		// split can't invert.
		if len(m.panes) == 2 {
			m.splitCol = m.clampSplitCol(m.splitCol)
		}
	case resizeTerminal:
		_, _, terminalRect := m.paneLayout()
		m.terminalHeight = m.clampTerminalHeight(terminalRect.y1 - y)
	case resizeEditorSplit:
		m.splitCol = m.clampSplitCol(x)
	}
	return m
}

// clampTreeWidth keeps a candidate tree-pane width within [minTreeWidth,
// m.width-minEditorWidth] so dragging can never collapse either the tree
// pane or the editor/terminal pane beside it.
func (m Model) clampTreeWidth(w int) int {
	return clampInt(w, minTreeWidth, m.width-minEditorWidth)
}

// clampTerminalHeight keeps a candidate terminal-pane height within
// [minTerminalHeight, paneHeight-tabBarH-minEditorHeight] so dragging can
// never collapse either the terminal pane or the editor pane above it.
func (m Model) clampTerminalHeight(h int) int {
	paneHeight := m.height - menuBarHeight - statusBarHeight
	max := paneHeight - m.tabBarH() - minEditorHeight
	return clampInt(h, minTerminalHeight, max)
}

// clampInt restricts v to [lo, hi]. If hi < lo (the available space is
// smaller than the minimum itself — an already-degenerate window), lo wins.
func clampInt(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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
		return openPalette(m)
	case "About":
		m.openMenu = ""
		m.activeDialog = dialogAbout
	}
	return m, nil
}

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

// maybeStartTerminal lazily spawns the shell the first time the terminal
// pane gains focus. A no-op on every subsequent focus change.
func (m Model) maybeStartTerminal() (Model, tea.Cmd) {
	if m.focus != focusTerminal {
		return m, nil
	}
	var cmd tea.Cmd
	m.terminal, cmd = m.terminal.Start()
	return m, cmd
}

func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}
	menuBar := renderMenuBar(m.width, m.openMenu)
	paneHeight := m.height - menuBarHeight - statusBarHeight
	line, col := m.activeEditor().Cursor()
	status := statusbar.Render(m.width, m.projectName, m.recentCommand, m.activeEditor().Filetype(), line, col)

	if m.activeDialog == dialogFileOpen {
		dialog := renderFileOpenDialog(m.width, paneHeight, m.fileOpenInput, m.fileOpenError)
		return lipgloss.JoinVertical(lipgloss.Left, menuBar, dialog, status)
	}
	if m.activeDialog == dialogAbout {
		dialog := renderAboutDialog(m.width, paneHeight)
		return lipgloss.JoinVertical(lipgloss.Left, menuBar, dialog, status)
	}
	if m.activeDialog == dialogPalette {
		matches := filteredCommands(m.commands, m.paletteFilter.Value())
		dialog := renderPaletteDialog(m.width, paneHeight, m.paletteFilter, matches, m.paletteCursor)
		return lipgloss.JoinVertical(lipgloss.Left, menuBar, dialog, status)
	}
	if m.activeDialog == dialogSearch {
		dialog := renderSearchDialog(m.width, paneHeight, m.searchInput, m.recentCommand)
		return lipgloss.JoinVertical(lipgloss.Left, menuBar, dialog, status)
	}
	if m.activeDialog == dialogConfirmDiscard {
		dialog := renderConfirmDialog(m.width, paneHeight, m)
		return lipgloss.JoinVertical(lipgloss.Left, menuBar, dialog, status)
	}
	if m.activeDialog == dialogPathPrompt {
		dialog := renderPathPromptDialog(m.width, paneHeight, m)
		return lipgloss.JoinVertical(lipgloss.Left, menuBar, dialog, status)
	}

	var dropdown string
	dropdownHeight := 0
	if m.openMenu != "" {
		dropdown = renderDropdown(m.openMenu, m.commands)
		dropdownHeight = lipgloss.Height(dropdown)
	}

	var tabMenuView string
	tabMenuHeight := 0
	if m.tabMenu != nil {
		if startCol, ok := m.tabMenuStartCol(); ok {
			tabMenuView = renderTabMenu(*m.tabMenu, startCol)
			tabMenuHeight = lipgloss.Height(tabMenuView)
		}
		// ok == false means m.tabMenu is stale (see tabMenuStartCol's own
		// doc comment) — View() has no way to clear the field itself (it
		// takes a value receiver and only returns a string); Update's
		// WindowSizeMsg branch is what actually clears it going forward
		// (Fix 2 below). For this one frame, rendering nothing for it is
		// enough to avoid the crash — tabMenuHeight stays 0, matching
		// "nothing to reserve space for".
	}

	bodyHeight := paneHeight - dropdownHeight - tabMenuHeight
	tabBarH := m.tabBarH()
	editorHeight := bodyHeight - m.terminalHeight - tabBarH

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
	paneWidth := m.paneWidths()
	paneStyle := make([]lipgloss.Style, len(m.panes))
	for i, w := range paneWidth {
		paneStyle[i] = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(paneBorderColor[i]).
			Width(w - borderSize).
			Height(editorHeight - borderSize)
	}

	// Re-derive each pane's exact interior height right before rendering
	// (on this value-receiver copy of m, so nothing here mutates the real
	// model) instead of trusting whatever was last set on a WindowSizeMsg —
	// that height doesn't account for a transient dropdown's height, and a
	// stale height would let a pane's content silently overflow its box,
	// since Lip Gloss's Height() only sets a minimum, never a max.
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

	sections := []string{menuBar}
	if dropdown != "" {
		sections = append(sections, dropdown)
	}
	if tabMenuView != "" {
		sections = append(sections, tabMenuView)
	}
	sections = append(sections, body, status)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// clampBlockWidth truncates every line of a multi-line block to at most
// width visible columns (ANSI-aware, via lipgloss.Style.MaxWidth — which
// truncates, unlike Style.Width which wraps). A pane's rendered block is
// passed through this before being handed to a Style with both Width() and
// Height() set: without it, a single line wider than that Width would get
// hard-wrapped into multiple physical lines, and since Height() only sets a
// minimum, never a max, those extra wrapped lines would silently overflow
// the pane's box — the same failure this pane was already fixed against
// for too many lines, just triggered by line width instead of line count.
func clampBlockWidth(block string, width int) string {
	if width <= 0 {
		return block
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(block)
}
