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

var (
	focusedBorderColor   = lipgloss.Color("205")
	unfocusedBorderColor = lipgloss.Color("240")
)

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

	pendingConfirm    confirmAction
	pendingConfirmTab int // meaningful only when pendingConfirm == confirmCloseTab
	confirmCursor     int // 0 = "anyway", 1 = "Cancel"
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

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	switch msg := msg.(type) {
	case tea.MouseMsg:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		switch msg.Button {
		case tea.MouseButtonLeft:
			return m.handleClick(msg.X, msg.Y)
		case tea.MouseButtonWheelUp:
			return m.handleWheel(msg.X, msg.Y, -mouseWheelLines)
		case tea.MouseButtonWheelDown:
			return m.handleWheel(msg.X, msg.Y, mouseWheelLines)
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
			if m.openMenu != "" {
				m.openMenu = ""
				return m, nil
			}
		default:
			if m.focus != focusTerminal {
				if cmd, ok := commandForShortcut(m.commands, msg.String()); ok {
					return cmd.Handler(m)
				}
			} else if msg.String() == "ctrl+o" || msg.String() == "ctrl+q" {
				if cmd, ok := commandForShortcut(m.commands, msg.String()); ok {
					return cmd.Handler(m)
				}
			}
		}
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

// handlePaneClick routes a click that landed outside the menu bar and any
// open dropdown to whichever pane's rectangle contains it, switching focus
// there and forwarding the click for pane-specific handling (tree row
// selection, editor cursor placement). A click inside no pane (e.g. on a
// border, or before the first WindowSizeMsg) is a no-op.
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

// handleWheel scrolls whichever pane's rectangle contains (x, y) — the pane
// under the pointer, not necessarily the focused one — by delta lines
// (negative scrolls up). No-op outside any pane, over the terminal (which
// has no independent scroll-only view), or while a dialog or dropdown is
// open.
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

	var dropdown string
	dropdownHeight := 0
	if m.openMenu != "" {
		dropdown = renderDropdown(m.openMenu, m.commands)
		dropdownHeight = lipgloss.Height(dropdown)
	}

	bodyHeight := paneHeight - dropdownHeight
	tabBarH := 0
	if len(m.tabs) > 0 {
		tabBarH = tabBarHeight
	}
	editorHeight := bodyHeight - terminalHeight - tabBarH

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
	terminalStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(terminalBorderColor).
		Width(m.width - treeWidth - borderSize).
		Height(terminalHeight - borderSize)

	// Re-derive each pane's exact interior height right before rendering
	// (on this value-receiver copy of m, so nothing here mutates the real
	// model) instead of trusting whatever was last set on a WindowSizeMsg —
	// that height doesn't account for a transient dropdown's height, and a
	// stale height would let a pane's content silently overflow its box,
	// since Lip Gloss's Height() only sets a minimum, never a max.
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

	sections := []string{menuBar}
	if dropdown != "" {
		sections = append(sections, dropdown)
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
