package highlight

import "github.com/charmbracelet/lipgloss"

var styles = map[string]lipgloss.Style{
	"keyword":  lipgloss.NewStyle().Foreground(lipgloss.Color("212")),
	"string":   lipgloss.NewStyle().Foreground(lipgloss.Color("114")),
	"comment":  lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Italic(true),
	"number":   lipgloss.NewStyle().Foreground(lipgloss.Color("215")),
	"function": lipgloss.NewStyle().Foreground(lipgloss.Color("81")),
	"heading":  lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true),
	"code":     lipgloss.NewStyle().Foreground(lipgloss.Color("114")),
}

func StyleFor(capture string) (lipgloss.Style, bool) {
	s, ok := styles[capture]
	return s, ok
}
