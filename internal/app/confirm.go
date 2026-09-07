package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// confirmAction identifies what an open dialogConfirmDiscard dialog is
// asking the user to confirm.
type confirmAction int

const (
	confirmNone confirmAction = iota
	confirmQuit
	confirmCloseTab
)

// closeTab removes the tab at index. If it has unsaved changes, this opens
// a confirmation dialog instead of closing immediately; the actual removal
// then happens from updateConfirmDialog once the user confirms. A no-op
// for an out-of-range index.
func (m Model) closeTab(index int) (Model, tea.Cmd) {
	if index < 0 || index >= len(m.tabs) {
		return m, nil
	}
	if m.tabs[index].editor.HasUnsavedChanges() {
		m.activeDialog = dialogConfirmDiscard
		m.pendingConfirm = confirmCloseTab
		m.pendingConfirmTab = index
		m.confirmCursor = 0
		return m, nil
	}
	return m.removeTab(index), nil
}

// removeTab unconditionally removes the tab at index and reassigns
// activeTab: closing the active tab prefers the tab that shifted into its
// slot (the one that was to its right), falling back to the one before it
// if the closed tab was last; closing a tab is otherwise a no-op on
// activeTab unless the removal shifted it left.
func (m Model) removeTab(index int) Model {
	if index < 0 || index >= len(m.tabs) {
		return m
	}
	m.tabs = append(m.tabs[:index], m.tabs[index+1:]...)
	switch {
	case len(m.tabs) == 0:
		m.activeTab = -1
		m.focus = focusTree
	case index < m.activeTab:
		m.activeTab--
	case index == m.activeTab:
		if m.activeTab >= len(m.tabs) {
			m.activeTab--
		}
	}
	return m
}

func (m Model) updateConfirmDialog(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "esc":
		m.activeDialog = dialogNone
		m.pendingConfirm = confirmNone
		return m, nil
	case "up", "down", "left", "right":
		m.confirmCursor = 1 - m.confirmCursor
		return m, nil
	case "enter":
		action := m.pendingConfirm
		tabIdx := m.pendingConfirmTab
		confirmed := m.confirmCursor == 0
		m.activeDialog = dialogNone
		m.pendingConfirm = confirmNone
		if !confirmed {
			return m, nil
		}
		switch action {
		case confirmQuit:
			m.terminal.Close()
			return m, tea.Quit
		case confirmCloseTab:
			m = m.removeTab(tabIdx)
			return m, nil
		}
	}
	return m, nil
}

func renderConfirmDialog(width, height int, m Model) string {
	var message string
	switch m.pendingConfirm {
	case confirmQuit:
		var names []string
		for _, t := range m.tabs {
			if t.editor.HasUnsavedChanges() {
				names = append(names, tabDisplayName(t.path))
			}
		}
		message = fmt.Sprintf("Unsaved changes in: %s", strings.Join(names, ", "))
	case confirmCloseTab:
		name := "the tab"
		if m.pendingConfirmTab >= 0 && m.pendingConfirmTab < len(m.tabs) {
			name = tabDisplayName(m.tabs[m.pendingConfirmTab].path)
		}
		message = fmt.Sprintf("%s has unsaved changes.", name)
	}

	anywayLabel := "Quit anyway"
	if m.pendingConfirm == confirmCloseTab {
		anywayLabel = "Close anyway"
	}
	options := []string{anywayLabel, "Cancel"}
	var opts strings.Builder
	for i, o := range options {
		if i == m.confirmCursor {
			opts.WriteString(lipgloss.NewStyle().Reverse(true).Render(o))
		} else {
			opts.WriteString(o)
		}
		if i < len(options)-1 {
			opts.WriteString("    ")
		}
	}

	content := message + "\n\n" + opts.String()
	return lipgloss.NewStyle().Width(width).Height(height).Padding(1, 2).Render(content)
}
