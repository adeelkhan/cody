package app

import (
	"fmt"
	"path/filepath"

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
	}, nil
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		paneHeight := m.height - menuBarHeight - statusBarHeight
		editorHeight := paneHeight - terminalHeight
		m.tree = m.tree.SetSize(treeWidth-borderSize, paneHeight-borderSize)
		m.editor = m.editor.SetSize(m.width-treeWidth-borderSize, editorHeight-borderSize)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+q":
			return m, tea.Quit
		case "tab", "shift+tab":
			m.toggleFocus()
			return m, nil
		case "ctrl+s":
			var cmd tea.Cmd
			m.editor, cmd = m.editor.Update(msg)
			return m, cmd
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
	menuBar := lipgloss.NewStyle().Width(m.width).Render("File  Edit  Commands  About")

	paneHeight := m.height - menuBarHeight - statusBarHeight
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

	return lipgloss.JoinVertical(lipgloss.Left, menuBar, body, status)
}
