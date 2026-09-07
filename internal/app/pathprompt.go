package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cody/internal/filetree"
)

// pathPromptAction identifies what an open dialogPathPrompt dialog is
// asking the user to do — shared the same way confirm.go's confirmAction
// shares one dialog across quit and close-tab.
type pathPromptAction int

const (
	pathPromptNone pathPromptAction = iota
	pathPromptNewFile
	pathPromptSaveAs
)

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
