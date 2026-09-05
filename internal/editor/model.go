package editor

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cody/internal/highlight"
)

type CommandExecutedMsg struct {
	Description string
}

type RehighlightMsg struct {
	generation int
}

const highlightDebounce = 150 * time.Millisecond

func scheduleRehighlight(generation int) tea.Cmd {
	return tea.Tick(highlightDebounce, func(time.Time) tea.Msg {
		return RehighlightMsg{generation: generation}
	})
}

type Model struct {
	buf                 *Buffer
	cursorLine          int
	cursorCol           int
	width               int
	height              int
	selecting           bool
	selAnchorLine       int
	selAnchorCol        int
	clipboard           string
	undoStack           []undoSnapshot
	redoStack           []undoSnapshot
	highlighter         highlight.Highlighter
	highlightSpans      map[int][]highlight.LineSpan
	highlightGeneration int
	folder              highlight.Folder
	folds               []highlight.LineRange
	foldedStartLines    map[int]bool
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
	m.selecting = false
	m.selAnchorLine = 0
	m.selAnchorCol = 0
	m.undoStack = nil
	m.redoStack = nil
	m.highlighter = nil
	m.highlightSpans = nil
	m.folder = nil
	m.folds = nil
	m.foldedStartLines = nil
	if lang, ok := highlight.LanguageForPath(path); ok {
		if h, err := highlight.New(lang); err == nil {
			m.highlighter = h
		}
		if f, err := highlight.NewFolder(lang); err == nil {
			m.folder = f
		}
	}
	m.rehighlight()
	m.refold()
	return m, nil
}

func (m *Model) rehighlight() {
	if m.highlighter == nil || m.buf == nil {
		m.highlightSpans = nil
		return
	}
	source := []byte(strings.Join(m.buf.Lines, "\n"))
	spans, err := m.highlighter.Highlight(source)
	if err != nil {
		m.highlightSpans = nil
		return
	}
	m.highlightSpans = highlight.LineSpans(source, spans)
}

func (m *Model) refold() {
	m.foldedStartLines = nil
	if m.folder == nil || m.buf == nil {
		m.folds = nil
		return
	}
	source := []byte(strings.Join(m.buf.Lines, "\n"))
	folds, err := m.folder.Folds(source)
	if err != nil {
		m.folds = nil
		return
	}
	m.folds = highlight.FoldLines(source, folds)
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
	switch msg := msg.(type) {
	case RehighlightMsg:
		if m.buf != nil && msg.generation == m.highlightGeneration {
			m.rehighlight()
			m.refold()
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(keyMsg tea.KeyMsg) (Model, tea.Cmd) {
	if m.buf == nil {
		switch keyMsg.String() {
		case "ctrl+s", "ctrl+x", "ctrl+c", "ctrl+v", "ctrl+z", "ctrl+y", "ctrl+k":
			return m, func() tea.Msg { return CommandExecutedMsg{Description: "No file open"} }
		}
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
		m.selecting = false
		m.pushUndo()
		m.buf.InsertNewline(m.cursorLine, m.cursorCol)
		m.cursorLine++
		m.cursorCol = 0
		m.highlightGeneration++
		m.foldedStartLines = nil
		return m, scheduleRehighlight(m.highlightGeneration)
	case "backspace":
		m.selecting = false
		m.pushUndo()
		m.cursorLine, m.cursorCol = m.buf.DeleteBefore(m.cursorLine, m.cursorCol)
		m.highlightGeneration++
		m.foldedStartLines = nil
		return m, scheduleRehighlight(m.highlightGeneration)
	case "ctrl+s":
		desc := m.save()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case " ":
		m.selecting = false
		m.pushUndo()
		m.buf.InsertRune(m.cursorLine, m.cursorCol, ' ')
		m.cursorCol++
		m.highlightGeneration++
		m.foldedStartLines = nil
		return m, scheduleRehighlight(m.highlightGeneration)
	case "ctrl+x":
		desc := m.Cut()
		m.highlightGeneration++
		m.rehighlight()
		m.refold()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+c":
		desc := m.Copy()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+v":
		desc := m.Paste()
		m.highlightGeneration++
		m.rehighlight()
		m.refold()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+z":
		desc := m.undo()
		m.highlightGeneration++
		m.rehighlight()
		m.refold()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+y":
		desc := m.redo()
		m.highlightGeneration++
		m.rehighlight()
		m.refold()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	case "ctrl+k":
		desc := m.toggleFold()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	default:
		if keyMsg.Type == tea.KeyRunes && !keyMsg.Alt {
			m.selecting = false
			m.pushUndo()
			m.insertText(string(keyMsg.Runes))
			m.highlightGeneration++
			m.foldedStartLines = nil
			return m, scheduleRehighlight(m.highlightGeneration)
		}
	}
	return m, nil
}

func (m Model) isLineHidden(line int) bool {
	for _, f := range m.folds {
		if m.foldedStartLines[f.StartLine] && line > f.StartLine && line <= f.EndLine {
			return true
		}
	}
	return false
}

func (m Model) foldAt(line int) (highlight.LineRange, bool) {
	for _, f := range m.folds {
		if f.StartLine == line {
			return f, true
		}
	}
	return highlight.LineRange{}, false
}

func (m *Model) toggleFold() string {
	f, ok := m.foldAt(m.cursorLine)
	if !ok {
		return "Nothing to fold here"
	}
	if m.foldedStartLines == nil {
		m.foldedStartLines = make(map[int]bool)
	}
	m.foldedStartLines[f.StartLine] = !m.foldedStartLines[f.StartLine]
	if m.foldedStartLines[f.StartLine] {
		return "Folded"
	}
	return "Unfolded"
}

func (m *Model) moveUp() {
	next := m.cursorLine - 1
	for next >= 0 && m.isLineHidden(next) {
		next--
	}
	if next >= 0 {
		m.cursorLine = next
		m.clampCol()
	}
}

func (m *Model) moveDown() {
	next := m.cursorLine + 1
	for next < len(m.buf.Lines) && m.isLineHidden(next) {
		next++
	}
	if next < len(m.buf.Lines) {
		m.cursorLine = next
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
	m.selecting = false
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
	m.selecting = false
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
	m.selecting = false
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
	m.selecting = false
	return "Redo"
}

func (m Model) View() string {
	if m.buf == nil {
		return "Select a file to begin"
	}
	startLine, startCol, endLine, endCol, hasSel := m.selectionRange()
	var b strings.Builder
	for i, line := range m.buf.Lines {
		if m.isLineHidden(i) {
			continue
		}
		cursorMark := "  "
		if i == m.cursorLine {
			cursorMark = "> "
		}
		rendered := line
		if hasSel && i >= startLine && i <= endLine {
			rendered = highlightSelection(line, i, startLine, startCol, endLine, endCol)
		} else if spans, ok := m.highlightSpans[i]; ok {
			rendered = renderHighlightedLine(line, spans)
		}
		if m.foldedStartLines[i] {
			rendered += " ⋯"
		}
		b.WriteString(fmt.Sprintf("%s%4d %s\n", cursorMark, i+1, rendered))
	}
	return b.String()
}

func renderHighlightedLine(line string, spans []highlight.LineSpan) string {
	runes := []rune(line)
	sorted := make([]highlight.LineSpan, len(spans))
	copy(sorted, spans)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].StartCol < sorted[j].StartCol })
	var b strings.Builder
	pos := 0
	for _, sp := range sorted {
		start, end := sp.StartCol, sp.EndCol
		if start < pos {
			continue
		}
		if start > len(runes) {
			break
		}
		if end > len(runes) {
			end = len(runes)
		}
		b.WriteString(string(runes[pos:start]))
		if style, ok := highlight.StyleFor(sp.Capture); ok {
			b.WriteString(style.Render(string(runes[start:end])))
		} else {
			b.WriteString(string(runes[start:end]))
		}
		pos = end
	}
	b.WriteString(string(runes[pos:]))
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
	style := lipgloss.NewStyle().Reverse(true).TabWidth(lipgloss.NoTabConversion)
	return string(runes[:from]) + style.Render(string(runes[from:to])) + string(runes[to:])
}
