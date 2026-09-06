package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// dropdownBorderSize is the thickness of renderDropdown's border on each
// edge. handleClick's dropdown hit-test uses it to translate a click's
// screen row into an index into the dropdown's own item list.
const dropdownBorderSize = 1

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

var menuBarStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("236")).
	Foreground(lipgloss.Color("252"))

func renderMenuBar(width int, openMenu string) string {
	var parts []string
	for _, l := range menuLabels() {
		text := l.name
		if l.name == openMenu {
			text = lipgloss.NewStyle().Reverse(true).Render(text)
		}
		parts = append(parts, text)
	}
	return menuBarStyle.Width(width).Render(strings.Join(parts, "  "))
}

// dropdownStyle gives an open dropdown a visible border so it reads as a
// popup dropping out of the menu bar, rather than plain text floating below
// it. The border color matches the focused-pane border for a consistent
// "this is the active thing" visual language.
var dropdownStyle = lipgloss.NewStyle().
	Border(lipgloss.NormalBorder()).
	BorderForeground(focusedBorderColor).
	Padding(0, 1)

func renderDropdown(menu string, commands []Command) string {
	items := menuItemsFor(menu)
	var lines []string
	for _, name := range items {
		shortcut := ""
		if cmd, ok := commandByName(commands, name); ok {
			shortcut = cmd.Shortcut
		}
		lines = append(lines, fmt.Sprintf("%-10s %s", name, shortcut))
	}
	startCol := 0
	if label, ok := findLabel(menu); ok {
		startCol = label.startCol
	}
	return dropdownStyle.MarginLeft(startCol).Render(strings.Join(lines, "\n"))
}
