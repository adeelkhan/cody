package app

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// terminalTabDisplayName returns the label shown for the terminal tab at
// index: positional ("Shell 1", "Shell 2", ...) since a shell session has
// no filename the way an editor tab does.
func terminalTabDisplayName(tabs []terminalTab, index int) string {
	return fmt.Sprintf("Shell %d", index+1)
}

// terminalTabPlainLabel returns the unstyled text for one terminal tab —
// " Shell N × " — used to compute column widths for hit-testing. Must stay
// the same length as what renderTerminalTabBar actually draws for that
// tab, or clicks will land on the wrong tab (mirrors tabPlainLabel's own
// doc comment).
func terminalTabPlainLabel(tabs []terminalTab, index int) string {
	return " " + terminalTabDisplayName(tabs, index) + " × "
}

// terminalTabRegions computes each terminal tab's column range in the
// rendered tab bar, in order — the exact same shape as tabRegions, over
// []terminalTab instead of []tab. Must stay in sync with
// renderTerminalTabBar's own layout.
func terminalTabRegions(tabs []terminalTab, width int) []tabRegion {
	var regions []tabRegion
	col := 0
	closeWidth := lipgloss.Width("× ")
	for i := range tabs {
		if col >= width {
			break
		}
		tabWidth := lipgloss.Width(terminalTabPlainLabel(tabs, i))
		endCol := col + tabWidth
		if endCol > width {
			endCol = width
		}
		closeEnd := col + tabWidth
		if closeEnd > width {
			closeEnd = width
		}
		closeStart := col + tabWidth - closeWidth
		if closeStart > closeEnd {
			closeStart = closeEnd
		}
		regions = append(regions, tabRegion{
			tabIndex:   i,
			startCol:   col,
			endCol:     endCol,
			closeStart: closeStart,
			closeEnd:   closeEnd,
		})
		col += tabWidth
	}
	return regions
}

// terminalTabAt returns the region a column falls in, if any.
func terminalTabAt(col int, tabs []terminalTab, width int) (tabRegion, bool) {
	for _, r := range terminalTabRegions(tabs, width) {
		if col >= r.startCol && col < r.endCol {
			return r, true
		}
	}
	return tabRegion{}, false
}

// renderTerminalTabBar renders the terminal tab strip — same visual
// language as renderTabBar (active tab in reverse video, "×" close glyph
// per tab), positional label instead of a filename, no dirty-marker.
func renderTerminalTabBar(width int, tabs []terminalTab, activeTerminal int) string {
	var b []byte
	for i := range tabs {
		name := terminalTabDisplayName(tabs, i)
		main := " " + name
		if i == activeTerminal {
			main = lipgloss.NewStyle().Reverse(true).Render(main)
		}
		b = append(b, main...)
		b = append(b, " × "...)
	}
	truncated := lipgloss.NewStyle().MaxWidth(width).Render(string(b))
	return tabBarStyle.Width(width).Render(truncated)
}
