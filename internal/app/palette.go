package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func filteredCommands(commands []Command, query string) []Command {
	if query == "" {
		return commands
	}
	q := strings.ToLower(query)
	var out []Command
	for _, c := range commands {
		if strings.Contains(strings.ToLower(c.Name), q) {
			out = append(out, c)
		}
	}
	return out
}

func openPalette(m Model) (Model, tea.Cmd) {
	ti := textinput.New()
	ti.Placeholder = "type to filter..."
	ti.Focus()
	m.paletteFilter = ti
	m.paletteCursor = 0
	m.activeDialog = dialogPalette
	return m, textinput.Blink
}

func (m Model) updatePaletteDialog(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.paletteFilter, cmd = m.paletteFilter.Update(msg)
		return m, cmd
	}
	switch keyMsg.String() {
	case "esc":
		m.activeDialog = dialogNone
		return m, nil
	case "up":
		if m.paletteCursor > 0 {
			m.paletteCursor--
		}
		return m, nil
	case "down":
		matches := filteredCommands(m.commands, m.paletteFilter.Value())
		if m.paletteCursor < len(matches)-1 {
			m.paletteCursor++
		}
		return m, nil
	case "enter":
		matches := filteredCommands(m.commands, m.paletteFilter.Value())
		m.activeDialog = dialogNone
		if m.paletteCursor < 0 || m.paletteCursor >= len(matches) {
			return m, nil
		}
		selected := matches[m.paletteCursor]
		updated, cmd := selected.Handler(m)
		return updated, cmd
	}
	var cmd tea.Cmd
	m.paletteFilter, cmd = m.paletteFilter.Update(msg)
	m.paletteCursor = 0
	return m, cmd
}

func renderPaletteDialog(width, height int, filterInput textinput.Model, matches []Command, cursor int) string {
	var lines []string
	for i, cmd := range matches {
		line := fmt.Sprintf("%-12s %s", cmd.Name, cmd.Shortcut)
		if i == cursor {
			line = lipgloss.NewStyle().Reverse(true).Render(line)
		}
		lines = append(lines, line)
	}
	content := "Commands:\n\n" + filterInput.View() + "\n\n" + strings.Join(lines, "\n")
	return lipgloss.NewStyle().Width(width).Height(height).Padding(1, 2).Render(content)
}
