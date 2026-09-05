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
)

type focusArea int

const (
	focusTree focusArea = iota
	focusEditor
)

const (
	treeWidth       = 30
	statusBarHeight = 1
	menuBarHeight   = 1
	terminalHeight  = 8
	borderSize      = 2 // lipgloss.NormalBorder adds 1 cell on each side
)

var (
	focusedBorderColor   = lipgloss.Color("205")
	unfocusedBorderColor = lipgloss.Color("240")
)

type Model struct {
	tree          filetree.Model
	editor        editor.Model
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
		focus:       focusTree,
		projectName: filepath.Base(absPath),
		rootPath:    absPath,
		commands:    buildCommands(),
	}, nil
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		paneHeight := m.height - menuBarHeight - statusBarHeight
		editorHeight := paneHeight - terminalHeight
		m.tree = m.tree.SetSize(treeWidth-borderSize, paneHeight-borderSize)
		m.editor = m.editor.SetSize(m.width-treeWidth-borderSize, editorHeight-borderSize)
		return m, nil
	}
	if _, ok := msg.(editor.RehighlightMsg); ok {
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
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
	switch msg := msg.(type) {
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			return m.handleClick(msg.X, msg.Y)
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab":
			m.toggleFocus()
			return m, nil
		case "esc":
			if m.openMenu != "" {
				m.openMenu = ""
				return m, nil
			}
		default:
			if cmd, ok := commandForShortcut(m.commands, msg.String()); ok {
				return cmd.Handler(m)
			}
		}
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
	case editor.CommandExecutedMsg:
		m.recentCommand = msg.Description
		return m, nil
	}

	var cmd tea.Cmd
	if m.focus == focusTree {
		m.tree, cmd = m.tree.Update(msg)
	} else {
		m.editor, cmd = m.editor.Update(msg)
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
		return openPalette(m)
	case "About":
		m.openMenu = ""
		m.activeDialog = dialogAbout
	}
	return m, nil
}

func (m *Model) toggleFocus() {
	if m.focus == focusTree {
		m.focus = focusEditor
	} else {
		m.focus = focusTree
	}
}

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
	if m.activeDialog == dialogPalette {
		matches := filteredCommands(m.commands, m.paletteFilter.Value())
		dialog := renderPaletteDialog(m.width, paneHeight, m.paletteFilter, matches, m.paletteCursor)
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

	// Re-derive each pane's exact interior height right before rendering
	// (on this value-receiver copy of m, so nothing here mutates the real
	// model) instead of trusting whatever was last set on a WindowSizeMsg —
	// that height doesn't account for a transient dropdown's height, and a
	// stale height would let a pane's content silently overflow its box,
	// since Lip Gloss's Height() only sets a minimum, never a max.
	tree := m.tree.SetSize(treeWidth-borderSize, bodyHeight-borderSize)
	editor := m.editor.SetSize(m.width-treeWidth-borderSize, editorHeight-borderSize)

	right := lipgloss.JoinVertical(lipgloss.Left,
		editorStyle.Render(clampBlockWidth(editor.View(), m.width-treeWidth-borderSize)),
		terminalStyle.Render("Terminal (coming in a later phase)"),
	)
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
