package app

import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cody/internal/editor"
)

type dialogKind int

const (
	dialogNone dialogKind = iota
	dialogFileOpen
	dialogAbout
	dialogPalette
	dialogSearch
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
		var cmd tea.Cmd
		m.fileOpenInput, cmd = m.fileOpenInput.Update(msg)
		return m, cmd
	}
	switch keyMsg.String() {
	case "esc":
		m.activeDialog = dialogNone
		return m, nil
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

func renderSearchDialog(width, height int, ti textinput.Model, status string) string {
	content := "Find:\n\n" + ti.View()
	if status != "" {
		content += "\n\n" + status
	}
	return lipgloss.NewStyle().Width(width).Height(height).Padding(1, 2).Render(content)
}

func renderAboutDialog(width, height int) string {
	content := "Cody v0.5 (Phase 5) — a terminal code editor\n\n" +
		"Keyboard shortcuts use Ctrl+ on every platform.\n" +
		"macOS Cmd+ shortcuts depend on your terminal emulator's own\n" +
		"keybinding settings and are not guaranteed to reach this app.\n\n" +
		"Press any key to close."
	return lipgloss.NewStyle().Width(width).Height(height).Padding(1, 2).Render(content)
}
