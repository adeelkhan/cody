package editor

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type CommandExecutedMsg struct {
	Description string
}

type Model struct {
	buf           *Buffer
	cursorLine    int
	cursorCol     int
	width         int
	height        int
	selecting     bool
	selAnchorLine int
	selAnchorCol  int
	clipboard     string
	undoStack     []undoSnapshot
	redoStack     []undoSnapshot
}

type undoSnapshot struct {
	lines      []string
	cursorLine int
	cursorCol  int
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
		m.selecting = false
		m.moveUp()
	case "down":
		m.selecting = false
		m.moveDown()
	case "left":
		m.selecting = false
		m.moveLeft()
	case "right":
		m.selecting = false
		m.moveRight()
	case "shift+up":
		m.extendSelection()
		m.moveUp()
	case "shift+down":
		m.extendSelection()
		m.moveDown()
	case "shift+left":
		m.extendSelection()
		m.moveLeft()
	case "shift+right":
		m.extendSelection()
		m.moveRight()
	case "enter":
		m.pushUndo()
		m.buf.InsertNewline(m.cursorLine, m.cursorCol)
		m.cursorLine++
		m.cursorCol = 0
	case "backspace":
		m.pushUndo()
		m.cursorLine, m.cursorCol = m.buf.DeleteBefore(m.cursorLine, m.cursorCol)
	case "ctrl+s":
		desc := m.save()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case " ":
		m.pushUndo()
		m.buf.InsertRune(m.cursorLine, m.cursorCol, ' ')
		m.cursorCol++
	case "ctrl+x":
		desc := m.Cut()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+c":
		desc := m.Copy()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+v":
		desc := m.Paste()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+z":
		desc := m.undo()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+y":
		desc := m.redo()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	default:
		if keyMsg.Type == tea.KeyRunes && !keyMsg.Alt {
			m.pushUndo()
			m.insertText(string(keyMsg.Runes))
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

func (m Model) selectionRange() (startLine, startCol, endLine, endCol int, ok bool) {
	if !m.selecting {
		return 0, 0, 0, 0, false
	}
	aLine, aCol := m.selAnchorLine, m.selAnchorCol
	cLine, cCol := m.cursorLine, m.cursorCol
	if aLine > cLine || (aLine == cLine && aCol > cCol) {
		aLine, cLine = cLine, aLine
		aCol, cCol = cCol, aCol
	}
	if aLine == cLine && aCol == cCol {
		return 0, 0, 0, 0, false
	}
	return aLine, aCol, cLine, cCol, true
}

func (m *Model) extendSelection() {
	if !m.selecting {
		m.selecting = true
		m.selAnchorLine = m.cursorLine
		m.selAnchorCol = m.cursorCol
	}
}

func (m *Model) Cut() string {
	if startLine, startCol, endLine, endCol, ok := m.selectionRange(); ok {
		m.pushUndo()
		m.clipboard = m.buf.DeleteRange(startLine, startCol, endLine, endCol)
		m.cursorLine, m.cursorCol = startLine, startCol
		m.selecting = false
		return "Cut selection"
	}
	m.pushUndo()
	m.clipboard = m.buf.DeleteLine(m.cursorLine) + "\n"
	if m.cursorLine >= len(m.buf.Lines) {
		m.cursorLine = len(m.buf.Lines) - 1
	}
	m.cursorCol = 0
	return "Cut line"
}

func (m *Model) Copy() string {
	if startLine, startCol, endLine, endCol, ok := m.selectionRange(); ok {
		m.clipboard = m.buf.TextRange(startLine, startCol, endLine, endCol)
		return "Copied selection"
	}
	m.clipboard = m.buf.Lines[m.cursorLine] + "\n"
	return "Copied line"
}

func (m *Model) Paste() string {
	if m.clipboard == "" {
		return "Nothing to paste"
	}
	m.pushUndo()
	m.insertText(m.clipboard)
	return "Pasted"
}

func (m *Model) insertText(text string) {
	for _, r := range text {
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

func snapshotLines(lines []string) []string {
	cp := make([]string, len(lines))
	copy(cp, lines)
	return cp
}

func (m *Model) pushUndo() {
	m.undoStack = append(m.undoStack, undoSnapshot{
		lines:      snapshotLines(m.buf.Lines),
		cursorLine: m.cursorLine,
		cursorCol:  m.cursorCol,
	})
	m.redoStack = nil
}

func (m *Model) undo() string {
	if len(m.undoStack) == 0 {
		return "Nothing to undo"
	}
	current := undoSnapshot{lines: snapshotLines(m.buf.Lines), cursorLine: m.cursorLine, cursorCol: m.cursorCol}
	prev := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]
	m.redoStack = append(m.redoStack, current)
	m.buf.Lines = prev.lines
	m.cursorLine = prev.cursorLine
	m.cursorCol = prev.cursorCol
	m.buf.Dirty = true
	return "Undo"
}

func (m *Model) redo() string {
	if len(m.redoStack) == 0 {
		return "Nothing to redo"
	}
	current := undoSnapshot{lines: snapshotLines(m.buf.Lines), cursorLine: m.cursorLine, cursorCol: m.cursorCol}
	next := m.redoStack[len(m.redoStack)-1]
	m.redoStack = m.redoStack[:len(m.redoStack)-1]
	m.undoStack = append(m.undoStack, current)
	m.buf.Lines = next.lines
	m.cursorLine = next.cursorLine
	m.cursorCol = next.cursorCol
	m.buf.Dirty = true
	return "Redo"
}

func (m Model) View() string {
	if m.buf == nil {
		return "Select a file to begin"
	}
	startLine, startCol, endLine, endCol, hasSel := m.selectionRange()
	var b strings.Builder
	for i, line := range m.buf.Lines {
		cursorMark := "  "
		if i == m.cursorLine {
			cursorMark = "> "
		}
		rendered := line
		if hasSel && i >= startLine && i <= endLine {
			rendered = highlightSelection(line, i, startLine, startCol, endLine, endCol)
		}
		b.WriteString(fmt.Sprintf("%s%4d %s\n", cursorMark, i+1, rendered))
	}
	return b.String()
}

func highlightSelection(line string, lineIdx, startLine, startCol, endLine, endCol int) string {
	runes := []rune(line)
	from, to := 0, len(runes)
	if lineIdx == startLine {
		from = startCol
	}
	if lineIdx == endLine {
		to = endCol
	}
	if from > len(runes) {
		from = len(runes)
	}
	if to > len(runes) {
		to = len(runes)
	}
	if from >= to {
		return line
	}
	style := lipgloss.NewStyle().Reverse(true)
	return string(runes[:from]) + style.Render(string(runes[from:to])) + string(runes[to:])
}
