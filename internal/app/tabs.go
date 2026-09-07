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

// tabDisplayName returns the name shown for a tab: "Untitled" for a
// pathless (never-saved) tab created via Ctrl+N, filepath.Base(path)
// otherwise — filepath.Base("") returns "." which would otherwise show up
// as a nonsensical tab label.
func tabDisplayName(path string) string {
	if path == "" {
		return "Untitled"
	}
	return filepath.Base(path)
}

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
	name := tabDisplayName(t.path)
	suffix := ""
	if t.editor.HasUnsavedChanges() {
		suffix = " (M)"
	}
	return " " + name + suffix + " × "
}

// tabRegions computes each tab's column range in the rendered tab bar, in
// order. Must stay in sync with renderTabBar's own layout. width is the
// rendered tab bar's width in columns: renderTabBar truncates its content to
// this width (via clampBlockWidth's MaxWidth, not Width, to avoid wrapping),
// so once a tab's region would start at or past width it — and every tab
// after it — is not actually visible and must not be returned, or a click
// over that blank truncated area would incorrectly resolve to a tab that
// isn't there.
func tabRegions(tabs []tab, width int) []tabRegion {
	var regions []tabRegion
	col := 0
	closeWidth := lipgloss.Width("× ")
	for i, t := range tabs {
		if col >= width {
			break
		}
		tabWidth := lipgloss.Width(tabPlainLabel(t))
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

// tabAt returns the region a column falls in, if any.
func tabAt(col int, tabs []tab, width int) (tabRegion, bool) {
	for _, r := range tabRegions(tabs, width) {
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
		name := tabDisplayName(t.path)
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
	// Clamp to width via MaxWidth (truncates) rather than letting
	// tabBarStyle's own Width(width) below wrap: Width() only sets a
	// minimum and wraps content wider than it onto additional physical
	// rows, which would silently break every caller that assumes the tab
	// bar is exactly one row tall (paneLayout's tabBarRect, the
	// WindowSizeMsg handler's editorHeight math, tabRegions' hit-testing).
	truncated := lipgloss.NewStyle().MaxWidth(width).Render(b.String())
	return tabBarStyle.Width(width).Render(truncated)
}
