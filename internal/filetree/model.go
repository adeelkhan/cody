package filetree

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cody/internal/scrollbar"
)

type FileOpenedMsg struct {
	Path string
}

// scrollbarGutterWidth reserves one space plus one rune for the scrollbar
// column appended to each rendered row.
const scrollbarGutterWidth = 2

var scrollbarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

type flatItem struct {
	node  *Node
	depth int
}

type Model struct {
	root         *Node
	nerdFont     bool
	flat         []flatItem
	cursor       int
	width        int
	height       int
	scrollOffset int
}

func New(rootPath string, nerdFont bool) (Model, error) {
	root, err := NewRoot(rootPath)
	if err != nil {
		return Model{}, err
	}
	m := Model{root: root, nerdFont: nerdFont}
	m.rebuildFlat()
	return m, nil
}

func (m *Model) rebuildFlat() {
	m.flat = nil
	var walk func(n *Node, depth int)
	walk = func(n *Node, depth int) {
		for _, c := range n.Children {
			m.flat = append(m.flat, flatItem{node: c, depth: depth})
			if c.Type == NodeDir && c.Expanded {
				walk(c, depth+1)
			}
		}
	}
	walk(m.root, 0)
}

func (m Model) SetSize(width, height int) Model {
	m.width, m.height = width, height
	m.ensureCursorVisible()
	return m
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	var cmd tea.Cmd
	switch keyMsg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.flat)-1 {
			m.cursor++
		}
	case "left", "h":
		m.collapseCurrent()
	case "right", "l", "enter":
		m, cmd = m.activateCurrent()
	}
	m.ensureCursorVisible()
	return m, cmd
}

// ensureCursorVisible scrolls the viewport so the selected item stays
// within it. A no-op when no height has ever been set (m.height <= 0),
// which preserves the unbounded rendering every pre-existing caller relies
// on.
func (m *Model) ensureCursorVisible() {
	if m.height <= 0 {
		return
	}
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	if m.cursor >= m.scrollOffset+m.height {
		m.scrollOffset = m.cursor - m.height + 1
	}
	maxOffset := len(m.flat) - m.height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m *Model) collapseCurrent() {
	if len(m.flat) == 0 {
		return
	}
	n := m.flat[m.cursor].node
	if n.Type == NodeDir && n.Expanded {
		n.Expanded = false
		m.rebuildFlat()
	}
}

func (m Model) activateCurrent() (Model, tea.Cmd) {
	if len(m.flat) == 0 {
		return m, nil
	}
	n := m.flat[m.cursor].node
	if n.Type == NodeDir {
		if !n.Expanded {
			n.Expanded = true
			if err := n.LoadChildren(); err != nil {
				return m, nil
			}
		} else {
			n.Expanded = false
		}
		m.rebuildFlat()
		return m, nil
	}
	path := n.Path
	return m, func() tea.Msg { return FileOpenedMsg{Path: path} }
}

func (m Model) View() string {
	clip := m.height > 0
	viewStart, viewEnd := 0, len(m.flat)
	if clip {
		viewStart = m.scrollOffset
		if viewStart < 0 {
			viewStart = 0
		}
		if viewStart > len(m.flat) {
			viewStart = len(m.flat)
		}
		viewEnd = viewStart + m.height
		if viewEnd > len(m.flat) {
			viewEnd = len(m.flat)
		}
	}

	var bar []rune
	contentWidth := 0
	if clip {
		bar = scrollbar.Column(len(m.flat), viewEnd-viewStart, viewStart)
		contentWidth = m.width - scrollbarGutterWidth
		if contentWidth < 0 {
			contentWidth = 0
		}
	}

	var b strings.Builder
	for idx := viewStart; idx < viewEnd; idx++ {
		item := m.flat[idx]
		prefix := strings.Repeat("  ", item.depth)
		icon := IconFor(item.node, m.nerdFont)
		line := fmt.Sprintf("%s%s %s", prefix, icon, item.node.Name)
		if idx == m.cursor {
			line = "> " + line
		} else {
			line = "  " + line
		}
		if clip {
			// Pad to the pane's known content width so the bar lands in a
			// fixed column at the right edge, forming a straight vertical
			// scrollbar — appending it directly after variable-length text
			// makes it look like a stray character attached to each line.
			padded := lipgloss.NewStyle().Width(contentWidth).Render(line)
			line = padded + " " + scrollbarStyle.Render(string(bar[idx-viewStart]))
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}
