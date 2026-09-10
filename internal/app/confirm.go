package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cody/internal/terminal"
)

// confirmAction identifies what an open dialogConfirmDiscard dialog is
// asking the user to confirm.
type confirmAction int

const (
	confirmNone confirmAction = iota
	confirmQuit
	confirmCloseTab
)

// closeTab removes the tab at (pane, index). If it has unsaved changes,
// this opens a confirmation dialog instead of closing immediately; the
// actual removal then happens from updateConfirmDialog once the user
// confirms. A no-op for an out-of-range pane or index.
func (m Model) closeTab(pane, index int) (Model, tea.Cmd) {
	if pane < 0 || pane >= len(m.panes) {
		return m, nil
	}
	if index < 0 || index >= len(m.panes[pane].tabs) {
		return m, nil
	}
	if m.panes[pane].tabs[index].editor.HasUnsavedChanges() {
		m.activeDialog = dialogConfirmDiscard
		m.pendingConfirm = confirmCloseTab
		m.pendingConfirmPane = pane
		m.pendingConfirmTab = index
		m.confirmCursor = 0
		return m, nil
	}
	return m.removeTab(pane, index), nil
}

// removeTab unconditionally removes the tab at (pane, index) and
// reassigns that pane's activeTab: closing the active tab prefers the tab
// that shifted into its slot (the one that was to its right), falling
// back to the one before it if the closed tab was last; closing a tab is
// otherwise a no-op on activeTab unless the removal shifted it left.
//
// If this leaves a pane empty while a split exists (len(m.panes) == 2),
// the layout collapses back to a single pane — the one place this rule
// lives, covering both directions:
//   - pane 1 emptied: drop panes[1] entirely, activePane = 0.
//   - pane 0 emptied and pane 1 still has tabs: panes[0] = panes[1] (the
//     survivor moves into the slot View() always treats as "the" pane in
//     unsplit mode), then drop the now-duplicated panes[1], activePane = 0.
//   - both empty (defensive only — not reachable via a single removeTab
//     call in normal use): fall back to a single fresh empty pane.
func (m Model) removeTab(pane, index int) Model {
	if pane < 0 || pane >= len(m.panes) {
		return m
	}
	if index < 0 || index >= len(m.panes[pane].tabs) {
		return m
	}
	p := &m.panes[pane]
	p.tabs = append(p.tabs[:index], p.tabs[index+1:]...)
	switch {
	case len(p.tabs) == 0:
		p.activeTab = -1
		if len(m.panes) == 1 {
			m.focus = focusTree
		}
	case index < p.activeTab:
		p.activeTab--
	case index == p.activeTab:
		if p.activeTab >= len(p.tabs) {
			p.activeTab--
		}
	}

	if len(m.panes) == 2 {
		emptied0 := len(m.panes[0].tabs) == 0
		emptied1 := len(m.panes[1].tabs) == 0
		switch {
		case emptied0 && emptied1:
			m.panes = []editorPane{{activeTab: -1}}
			m.activePane = 0
			m.focus = focusTree
		case emptied1:
			m.panes = m.panes[:1]
			m.activePane = 0
		case emptied0:
			m.panes[0] = m.panes[1]
			m.panes = m.panes[:1]
			m.activePane = 0
		}
	}
	return m
}

// removeTerminalTab closes and removes the terminal tab at index,
// reassigning m.activeTerminal the same way removeTab reassigns an
// editorPane's activeTab (prefer the tab that shifted into the closed
// slot, falling back to the tab before it if the closed tab was last).
// Unlike an editorPane's tabs, this slice must never reach zero — the
// terminal pane itself isn't closable — so closing the sole remaining tab
// replaces it with one fresh, unstarted session in the same slot instead.
// A no-op for an out-of-range index.
func (m Model) removeTerminalTab(index int) Model {
	if index < 0 || index >= len(m.terminals) {
		return m
	}
	m.terminals[index].term.Close()
	if len(m.terminals) == 1 {
		m.nextTerminalID++
		w, h := m.newTerminalSize()
		m.terminals[0] = terminalTab{term: terminal.New(m.nextTerminalID).SetSize(w, h)}
		m.activeTerminal = 0
		return m
	}
	m.terminals = append(m.terminals[:index], m.terminals[index+1:]...)
	switch {
	case index < m.activeTerminal:
		m.activeTerminal--
	case index == m.activeTerminal:
		if m.activeTerminal >= len(m.terminals) {
			m.activeTerminal--
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
		paneIdx := m.pendingConfirmPane
		tabIdx := m.pendingConfirmTab
		confirmed := m.confirmCursor == 0
		m.activeDialog = dialogNone
		m.pendingConfirm = confirmNone
		if !confirmed {
			return m, nil
		}
		switch action {
		case confirmQuit:
			for _, t := range m.terminals {
				t.term.Close()
			}
			return m, tea.Quit
		case confirmCloseTab:
			m = m.removeTab(paneIdx, tabIdx)
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
		for _, p := range m.panes {
			for _, t := range p.tabs {
				if t.editor.HasUnsavedChanges() {
					names = append(names, tabDisplayName(t.path))
				}
			}
		}
		message = fmt.Sprintf("Unsaved changes in: %s", strings.Join(names, ", "))
	case confirmCloseTab:
		name := "the tab"
		if m.pendingConfirmPane >= 0 && m.pendingConfirmPane < len(m.panes) {
			p := m.panes[m.pendingConfirmPane]
			if m.pendingConfirmTab >= 0 && m.pendingConfirmTab < len(p.tabs) {
				name = tabDisplayName(p.tabs[m.pendingConfirmTab].path)
			}
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
