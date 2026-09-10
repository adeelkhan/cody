package app

// tabContextMenu describes an open right-click menu for one tab.
type tabContextMenu struct {
	pane  int
	index int
}

// tabMenuItems lists a tab menu's actions — one item, whose label depends
// on which pane the tab is currently in: pane 0's tabs offer to split
// into (or move further into) pane 1; pane 1's tabs offer to move back.
func tabMenuItems(cm tabContextMenu) []string {
	if cm.pane == 0 {
		return []string{"Split + Move Right"}
	}
	return []string{"Move Left"}
}

// selectTabMenuItem runs the tab menu's item (there is only one, but this
// stays name-based like the File/Edit dropdown's commandByName for
// consistency and to leave room for more items later) and closes the menu.
func (m Model) selectTabMenuItem(item string) Model {
	cm := m.tabMenu
	m.tabMenu = nil
	if cm == nil {
		return m
	}
	switch item {
	case "Split + Move Right", "Move Left":
		return m.moveTabToOtherPane(cm.pane, cm.index)
	}
	return m
}

// renderTabMenu renders the tab context menu as a small dropdown anchored
// at startCol, matching the File/Edit dropdown's exact visual style
// (dropdownStyle: a bordered box) so the app has one consistent "this is a
// dropdown" look rather than two.
func renderTabMenu(cm tabContextMenu, startCol int) string {
	items := tabMenuItems(cm)
	return dropdownStyle.MarginLeft(startCol).Render(items[0])
}
