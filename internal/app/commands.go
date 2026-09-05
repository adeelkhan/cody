package app

import tea "github.com/charmbracelet/bubbletea"

type Command struct {
	Name     string
	Shortcut string
	Handler  func(Model) (Model, tea.Cmd)
}

func buildCommands() []Command {
	return []Command{
		{Name: "Save", Shortcut: "ctrl+s", Handler: cmdSave},
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

func cmdQuit(m Model) (Model, tea.Cmd) {
	return m, tea.Quit
}
