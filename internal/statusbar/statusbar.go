package statusbar

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var style = lipgloss.NewStyle().
	Background(lipgloss.Color("62")).
	Foreground(lipgloss.Color("230"))

func Render(width int, projectName, recentCommand, filetype string, line, col int) string {
	left := projectName
	right := fmt.Sprintf("%s  Ln %d, Col %d  %s", recentCommand, line, col, filetype)
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return style.Width(width).MaxWidth(width).Render(left + spaces(gap) + right)
}

func spaces(n int) string {
	if n < 0 {
		n = 0
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}
