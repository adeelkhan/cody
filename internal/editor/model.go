package editor

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type CommandExecutedMsg struct {
	Description string
}

type Model struct {
	buf        *Buffer
	cursorLine int
	cursorCol  int
	width      int
	height     int
}

func New() Model {
	return Model{}
}

func (m Model) LoadFile(path string) (Model, error) {
	buf, err := NewBuffer(path)
	if err != nil {
		return m, err
	}
	m.buf = buf
	m.cursorLine = 0
	m.cursorCol = 0
	return m, nil
}

func (m Model) SetSize(width, height int) Model {
	m.width, m.height = width, height
	return m
}

func (m Model) HasBuffer() bool {
	return m.buf != nil
}

func (m Model) Cursor() (line, col int) {
	return m.cursorLine + 1, m.cursorCol + 1
}

func (m Model) Filetype() string {
	if m.buf == nil {
		return ""
	}
	return strings.TrimPrefix(filepath.Ext(m.buf.Path), ".")
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if m.buf == nil {
		return m, nil
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "up":
		m.moveUp()
	case "down":
		m.moveDown()
	case "left":
		m.moveLeft()
	case "right":
		m.moveRight()
	case "enter":
		m.buf.InsertNewline(m.cursorLine, m.cursorCol)
		m.cursorLine++
		m.cursorCol = 0
	case "backspace":
		m.cursorLine, m.cursorCol = m.buf.DeleteBefore(m.cursorLine, m.cursorCol)
	case "ctrl+s":
		desc := m.save()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case " ":
		m.buf.InsertRune(m.cursorLine, m.cursorCol, ' ')
		m.cursorCol++
	default:
		if keyMsg.Type == tea.KeyRunes && !keyMsg.Alt {
			for _, r := range keyMsg.Runes {
				if r == '\n' || r == '\r' {
					m.buf.InsertNewline(m.cursorLine, m.cursorCol)
					m.cursorLine++
					m.cursorCol = 0
					continue
				}
				m.buf.InsertRune(m.cursorLine, m.cursorCol, r)
				m.cursorCol++
			}
		}
	}
	return m, nil
}

func (m *Model) moveUp() {
	if m.cursorLine > 0 {
		m.cursorLine--
		m.clampCol()
	}
}

func (m *Model) moveDown() {
	if m.cursorLine < len(m.buf.Lines)-1 {
		m.cursorLine++
		m.clampCol()
	}
}

func (m *Model) moveLeft() {
	if m.cursorCol > 0 {
		m.cursorCol--
	}
}

func (m *Model) moveRight() {
	if m.cursorCol < len([]rune(m.buf.Lines[m.cursorLine])) {
		m.cursorCol++
	}
}

func (m *Model) clampCol() {
	lineLen := len([]rune(m.buf.Lines[m.cursorLine]))
	if m.cursorCol > lineLen {
		m.cursorCol = lineLen
	}
}

func (m *Model) save() string {
	if err := m.buf.Save(); err != nil {
		return fmt.Sprintf("Save failed: %s", err)
	}
	return fmt.Sprintf("Saved %s", filepath.Base(m.buf.Path))
}

func (m Model) View() string {
	if m.buf == nil {
		return "Select a file to begin"
	}
	var b strings.Builder
	for i, line := range m.buf.Lines {
		cursorMark := "  "
		if i == m.cursorLine {
			cursorMark = "> "
		}
		b.WriteString(fmt.Sprintf("%s%4d %s\n", cursorMark, i+1, line))
	}
	return b.String()
}
