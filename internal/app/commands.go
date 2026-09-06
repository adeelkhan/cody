package app

import tea "github.com/charmbracelet/bubbletea"

type Command struct {
	Name     string
	Shortcut string
	Handler  func(Model) (Model, tea.Cmd)
}

func buildCommands() []Command {
	return []Command{
		{Name: "Open", Shortcut: "ctrl+o", Handler: cmdOpenFilePrompt},
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
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	return m, cmd
}

func cmdCut(m Model) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	return m, cmd
}

func cmdCopy(m Model) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	return m, cmd
}

func cmdPaste(m Model) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
	return m, cmd
}

func cmdUndo(m Model) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})
	return m, cmd
}

func cmdRedo(m Model) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	return m, cmd
}

func cmdToggleFold(m Model) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	return m, cmd
}

func cmdQuit(m Model) (Model, tea.Cmd) {
	m.terminal.Close()
	return m, tea.Quit
}
