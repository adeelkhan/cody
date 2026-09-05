package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const dropdownWidth = 20

type menuLabel struct {
	name     string
	startCol int
	endCol   int
}

func menuLabels() []menuLabel {
	names := []string{"File", "Edit", "Commands", "About"}
	var out []menuLabel
	col := 0
	for _, name := range names {
		out = append(out, menuLabel{name: name, startCol: col, endCol: col + len(name)})
		col += len(name) + 2
	}
	return out
}

func menuLabelAt(col int) (string, bool) {
	for _, l := range menuLabels() {
		if col >= l.startCol && col < l.endCol {
			return l.name, true
		}
	}
	return "", false
}

func findLabel(name string) (menuLabel, bool) {
	for _, l := range menuLabels() {
		if l.name == name {
			return l, true
		}
	}
	return menuLabel{}, false
}

func menuItemsFor(menu string) []string {
	switch menu {
	case "File":
		return []string{"Open", "Save"}
	case "Edit":
		return []string{"Cut", "Paste", "Copy", "Save"}
	}
	return nil
}

func commandByName(commands []Command, name string) (Command, bool) {
	for _, c := range commands {
		if c.Name == name {
			return c, true
		}
	}
	return Command{}, false
}

func renderMenuBar(width int, openMenu string) string {
	var parts []string
	for _, l := range menuLabels() {
		text := l.name
		if l.name == openMenu {
			text = lipgloss.NewStyle().Reverse(true).Render(text)
		}
		parts = append(parts, text)
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(parts, "  "))
}

func renderDropdown(menu string, commands []Command) string {
	var lines []string
	for _, name := range menuItemsFor(menu) {
		shortcut := ""
		if cmd, ok := commandByName(commands, name); ok {
			shortcut = cmd.Shortcut
		}
		lines = append(lines, fmt.Sprintf("%-10s %s", name, shortcut))
	}
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}
