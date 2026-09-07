package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"cody/internal/editor"
)

type Command struct {
	Name     string
	Shortcut string
	Handler  func(Model) (Model, tea.Cmd)
}

func buildCommands() []Command {
	return []Command{
		{Name: "New", Handler: cmdNewFilePrompt},
		{Name: "New Tab", Shortcut: "ctrl+n", Handler: cmdNewBlankTab},
		{Name: "Open", Shortcut: "ctrl+o", Handler: cmdOpenFilePrompt},
		{Name: "Find", Shortcut: "ctrl+f", Handler: cmdFind},
		{Name: "Save", Shortcut: "ctrl+s", Handler: cmdSave},
		{Name: "Cut", Shortcut: "ctrl+x", Handler: cmdCut},
		{Name: "Copy", Shortcut: "ctrl+c", Handler: cmdCopy},
		{Name: "Paste", Shortcut: "ctrl+v", Handler: cmdPaste},
		{Name: "Undo", Shortcut: "ctrl+z", Handler: cmdUndo},
		{Name: "Redo", Shortcut: "ctrl+y", Handler: cmdRedo},
		{Name: "Toggle Fold", Shortcut: "ctrl+k", Handler: cmdToggleFold},
		{Name: "Quit", Shortcut: "ctrl+q", Handler: cmdQuit},
	}
}

func commandForShortcut(commands []Command, shortcut string) (Command, bool) {
	for _, c := range commands {
		if c.Shortcut == shortcut {
			return c, true
		}
	}
	return Command{}, false
}

func cmdSave(m Model) (Model, tea.Cmd) {
	if m.activeEditor().IsUntitled() {
		return openSaveAsPrompt(m)
	}
	e, cmd := m.activeEditor().Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = m.setActiveEditor(e)
	return m, cmd
}

func cmdNewBlankTab(m Model) (Model, tea.Cmd) {
	editorW, editorH := m.newTabEditorSize()
	e := editor.New().NewBlankBuffer().SetSize(editorW, editorH)
	m.tabs = append(m.tabs, tab{path: "", editor: e})
	m.activeTab = len(m.tabs) - 1
	m.focus = focusEditor
	m.recentCommand = "New file"
	return m, nil
}

func cmdCut(m Model) (Model, tea.Cmd) {
	e, cmd := m.activeEditor().Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	m = m.setActiveEditor(e)
	return m, cmd
}

func cmdCopy(m Model) (Model, tea.Cmd) {
	e, cmd := m.activeEditor().Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = m.setActiveEditor(e)
	return m, cmd
}

func cmdPaste(m Model) (Model, tea.Cmd) {
	e, cmd := m.activeEditor().Update(tea.KeyMsg{Type: tea.KeyCtrlV})
	m = m.setActiveEditor(e)
	return m, cmd
}

func cmdUndo(m Model) (Model, tea.Cmd) {
	e, cmd := m.activeEditor().Update(tea.KeyMsg{Type: tea.KeyCtrlZ})
	m = m.setActiveEditor(e)
	return m, cmd
}

func cmdRedo(m Model) (Model, tea.Cmd) {
	e, cmd := m.activeEditor().Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	m = m.setActiveEditor(e)
	return m, cmd
}

func cmdToggleFold(m Model) (Model, tea.Cmd) {
	e, cmd := m.activeEditor().Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m = m.setActiveEditor(e)
	return m, cmd
}

func cmdQuit(m Model) (Model, tea.Cmd) {
	hasDirty := false
	for _, t := range m.tabs {
		if t.editor.HasUnsavedChanges() {
			hasDirty = true
			break
		}
	}
	if !hasDirty {
		m.terminal.Close()
		return m, tea.Quit
	}
	m.activeDialog = dialogConfirmDiscard
	m.pendingConfirm = confirmQuit
	m.confirmCursor = 0
	return m, nil
}
