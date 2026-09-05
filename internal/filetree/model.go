package filetree

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type FileOpenedMsg struct {
	Path string
}

type flatItem struct {
	node  *Node
	depth int
}

type Model struct {
	root     *Node
	nerdFont bool
	flat     []flatItem
	cursor   int
	width    int
	height   int
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
	return m
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
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
		return m.activateCurrent()
	}
	return m, nil
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
	var b strings.Builder
	for i, item := range m.flat {
		prefix := strings.Repeat("  ", item.depth)
		icon := IconFor(item.node, m.nerdFont)
		line := fmt.Sprintf("%s%s %s", prefix, icon, item.node.Name)
		if i == m.cursor {
			line = "> " + line
		} else {
			line = "  " + line
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}
