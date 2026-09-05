package editor

import (
	"os"
	"strings"
)

type Buffer struct {
	Lines []string
	Path  string
	Dirty bool
}

func NewBuffer(path string) (*Buffer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	return &Buffer{Lines: lines, Path: path}, nil
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

func (b *Buffer) Save() error {
	data := strings.Join(b.Lines, "\n")
	if err := os.WriteFile(b.Path, []byte(data), 0644); err != nil {
		return err
	}
	b.Dirty = false
	return nil
}
