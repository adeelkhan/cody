package highlight

import "github.com/charmbracelet/lipgloss"

var styles = map[string]lipgloss.Style{
	"keyword":  lipgloss.NewStyle().Foreground(lipgloss.Color("212")).TabWidth(lipgloss.NoTabConversion),
	"string":   lipgloss.NewStyle().Foreground(lipgloss.Color("114")).TabWidth(lipgloss.NoTabConversion),
	"comment":  lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Italic(true).TabWidth(lipgloss.NoTabConversion),
	"number":   lipgloss.NewStyle().Foreground(lipgloss.Color("215")).TabWidth(lipgloss.NoTabConversion),
	"function": lipgloss.NewStyle().Foreground(lipgloss.Color("81")).TabWidth(lipgloss.NoTabConversion),
	"heading":  lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true).TabWidth(lipgloss.NoTabConversion),
	"code":     lipgloss.NewStyle().Foreground(lipgloss.Color("114")).TabWidth(lipgloss.NoTabConversion),
}

func StyleFor(capture string) (lipgloss.Style, bool) {
	s, ok := styles[capture]
	return s, ok
}
