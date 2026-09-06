package app

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	tabBarStyle  = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	dirtyTabText = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

// tabRegion describes one tab's clickable column range within the rendered
// tab bar (startCol/endCol, half-open, close glyph included), and the
// sub-range within it (closeStart/closeEnd) that is its close glyph.
type tabRegion struct {
	tabIndex             int
	startCol, endCol     int
	closeStart, closeEnd int
}

// tabPlainLabel returns the unstyled text for one tab — " name(M)? × " —
// used to compute column widths for hit-testing. Must stay the same
// length as what renderTabBar actually draws for that tab, or clicks will
// land on the wrong tab.
func tabPlainLabel(t tab) string {
	name := filepath.Base(t.path)
	suffix := ""
	if t.editor.HasUnsavedChanges() {
		suffix = " (M)"
	}
	return " " + name + suffix + " × "
}

// tabRegions computes each tab's column range in the rendered tab bar, in
// order. Must stay in sync with renderTabBar's own layout.
func tabRegions(tabs []tab) []tabRegion {
	var regions []tabRegion
	col := 0
	closeWidth := lipgloss.Width("× ")
	for i, t := range tabs {
		width := lipgloss.Width(tabPlainLabel(t))
		regions = append(regions, tabRegion{
			tabIndex:   i,
			startCol:   col,
			endCol:     col + width,
			closeStart: col + width - closeWidth,
			closeEnd:   col + width,
		})
		col += width
	}
	return regions
}

// tabAt returns the region a column falls in, if any.
func tabAt(col int, tabs []tab) (tabRegion, bool) {
	for _, r := range tabRegions(tabs) {
		if col >= r.startCol && col < r.endCol {
			return r, true
		}
	}
	return tabRegion{}, false
}

// renderTabBar renders the tab strip: each tab shows its file's base name,
// an orange " (M)" suffix if it has unsaved changes, and a "×" close
// glyph. The active tab's name/suffix (not its close glyph, which stays a
// consistent click target regardless of active state) is shown in reverse
// video, matching renderMenuBar's convention for the open menu label.
func renderTabBar(width int, tabs []tab, activeTab int) string {
	var b strings.Builder
	for i, t := range tabs {
		name := filepath.Base(t.path)
		dirty := t.editor.HasUnsavedChanges()
		main := " " + name
		if i == activeTab {
			suffix := ""
			if dirty {
				suffix = " (M)"
			}
			main = lipgloss.NewStyle().Reverse(true).Render(main + suffix)
		} else if dirty {
			main += dirtyTabText.Render(" (M)")
		}
		b.WriteString(main)
		b.WriteString(" × ")
	}
	return tabBarStyle.Width(width).Render(b.String())
}
