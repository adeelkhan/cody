package editor

import (
	"os"
	"strings"
)

type Buffer struct {
	Lines []string
	Path  string
	Dirty bool

	// savedLines is the content as of the last load or save — the baseline
	// SyncDirty compares against. Content-based rather than a monotonic
	// "has been touched" flag, so undo/redo can clear Dirty again when they
	// land back on exactly what's on disk (or, for an unsaved buffer, back
	// on its starting content).
	savedLines []string
}

func NewBuffer(path string) (*Buffer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	return &Buffer{Lines: lines, Path: path, savedLines: snapshotLines(lines)}, nil
}

// SyncDirty recomputes Dirty by comparing the current content against the
// last-saved (or, for a buffer that's never been saved, starting) content.
// Call this after any mutation that doesn't go through the Insert*/Delete*
// methods above — those set Dirty unconditionally, which is correct since
// they only ever move content away from the saved baseline; undo/redo can
// move it back, so they need the real comparison.
func (b *Buffer) SyncDirty() {
	b.Dirty = !linesEqual(b.Lines, b.savedLines)
}

func linesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (b *Buffer) InsertRune(line, col int, r rune) {
	l := []rune(b.Lines[line])
	l = append(l[:col], append([]rune{r}, l[col:]...)...)
	b.Lines[line] = string(l)
	b.Dirty = true
}

func (b *Buffer) InsertNewline(line, col int) {
	l := []rune(b.Lines[line])
	before := string(l[:col])
	after := string(l[col:])
	b.Lines[line] = before
	tail := append([]string{after}, b.Lines[line+1:]...)
	b.Lines = append(b.Lines[:line+1], tail...)
	b.Dirty = true
}

func (b *Buffer) DeleteBefore(line, col int) (int, int) {
	if col > 0 {
		l := []rune(b.Lines[line])
		l = append(l[:col-1], l[col:]...)
		b.Lines[line] = string(l)
		b.Dirty = true
		return line, col - 1
	}
	if line == 0 {
		return 0, 0
	}
	prevLen := len([]rune(b.Lines[line-1]))
	b.Lines[line-1] += b.Lines[line]
	b.Lines = append(b.Lines[:line], b.Lines[line+1:]...)
	b.Dirty = true
	return line - 1, prevLen
}

func (b *Buffer) TextRange(startLine, startCol, endLine, endCol int) string {
	if startLine == endLine {
		l := []rune(b.Lines[startLine])
		return string(l[startCol:endCol])
	}
	var sb strings.Builder
	startRunes := []rune(b.Lines[startLine])
	sb.WriteString(string(startRunes[startCol:]))
	for i := startLine + 1; i < endLine; i++ {
		sb.WriteString("\n")
		sb.WriteString(b.Lines[i])
	}
	sb.WriteString("\n")
	endRunes := []rune(b.Lines[endLine])
	sb.WriteString(string(endRunes[:endCol]))
	return sb.String()
}

func (b *Buffer) DeleteRange(startLine, startCol, endLine, endCol int) string {
	removed := b.TextRange(startLine, startCol, endLine, endCol)
	startRunes := []rune(b.Lines[startLine])
	endRunes := []rune(b.Lines[endLine])
	merged := string(startRunes[:startCol]) + string(endRunes[endCol:])
	b.Lines = append(b.Lines[:startLine], append([]string{merged}, b.Lines[endLine+1:]...)...)
	b.Dirty = true
	return removed
}

func (b *Buffer) DeleteLine(line int) string {
	removed := b.Lines[line]
	if len(b.Lines) == 1 {
		b.Lines[0] = ""
	} else {
		b.Lines = append(b.Lines[:line], b.Lines[line+1:]...)
	}
	b.Dirty = true
	return removed
}

func (b *Buffer) Save() error {
	data := strings.Join(b.Lines, "\n")
	if err := os.WriteFile(b.Path, []byte(data), 0644); err != nil {
		return err
	}
	b.savedLines = snapshotLines(b.Lines)
	b.Dirty = false
	return nil
}
