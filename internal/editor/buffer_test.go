package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewBuffer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello\nworld"), 0644); err != nil {
		t.Fatal(err)
	}
	buf, err := NewBuffer(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"hello", "world"}
	if len(buf.Lines) != len(want) || buf.Lines[0] != want[0] || buf.Lines[1] != want[1] {
		t.Fatalf("got %v, want %v", buf.Lines, want)
	}
}

func TestInsertRune(t *testing.T) {
	buf := &Buffer{Lines: []string{"helo"}}
	buf.InsertRune(0, 3, 'l')
	if buf.Lines[0] != "hello" {
		t.Fatalf("got %q, want %q", buf.Lines[0], "hello")
	}
	if !buf.Dirty {
		t.Fatal("expected Dirty to be true")
	}
}

func TestInsertNewline(t *testing.T) {
	buf := &Buffer{Lines: []string{"helloworld"}}
	buf.InsertNewline(0, 5)
	want := []string{"hello", "world"}
	if len(buf.Lines) != 2 || buf.Lines[0] != want[0] || buf.Lines[1] != want[1] {
		t.Fatalf("got %v, want %v", buf.Lines, want)
	}
}

func TestDeleteBeforeSameLine(t *testing.T) {
	buf := &Buffer{Lines: []string{"hello"}}
	line, col := buf.DeleteBefore(0, 5)
	if buf.Lines[0] != "hell" || line != 0 || col != 4 {
		t.Fatalf("got line=%d col=%d text=%q", line, col, buf.Lines[0])
	}
}

func TestDeleteBeforeJoinsLines(t *testing.T) {
	buf := &Buffer{Lines: []string{"hello", "world"}}
	line, col := buf.DeleteBefore(1, 0)
	want := "helloworld"
	if len(buf.Lines) != 1 || buf.Lines[0] != want || line != 0 || col != 5 {
		t.Fatalf("got line=%d col=%d lines=%v", line, col, buf.Lines)
	}
}

func TestTextRangeSameLine(t *testing.T) {
	buf := &Buffer{Lines: []string{"hello world"}}
	got := buf.TextRange(0, 6, 0, 11)
	if got != "world" {
		t.Fatalf("got %q, want %q", got, "world")
	}
}

func TestTextRangeMultiLine(t *testing.T) {
	buf := &Buffer{Lines: []string{"hello", "middle", "world"}}
	got := buf.TextRange(0, 3, 2, 2)
	want := "lo\nmiddle\nwo"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDeleteRangeSameLine(t *testing.T) {
	buf := &Buffer{Lines: []string{"hello world"}}
	removed := buf.DeleteRange(0, 5, 0, 11)
	if removed != " world" || buf.Lines[0] != "hello" {
		t.Fatalf("got removed=%q lines=%v", removed, buf.Lines)
	}
}

func TestDeleteRangeMultiLine(t *testing.T) {
	buf := &Buffer{Lines: []string{"hello", "middle", "world"}}
	removed := buf.DeleteRange(0, 3, 2, 2)
	if removed != "lo\nmiddle\nwo" {
		t.Fatalf("got removed=%q", removed)
	}
	if len(buf.Lines) != 1 || buf.Lines[0] != "helrld" {
		t.Fatalf("got lines=%v", buf.Lines)
	}
}

func TestDeleteLine(t *testing.T) {
	buf := &Buffer{Lines: []string{"a", "b", "c"}}
	removed := buf.DeleteLine(1)
	if removed != "b" || len(buf.Lines) != 2 || buf.Lines[0] != "a" || buf.Lines[1] != "c" {
		t.Fatalf("got removed=%q lines=%v", removed, buf.Lines)
	}
}

func TestDeleteLineLastRemaining(t *testing.T) {
	buf := &Buffer{Lines: []string{"only"}}
	removed := buf.DeleteLine(0)
	if removed != "only" || len(buf.Lines) != 1 || buf.Lines[0] != "" {
		t.Fatalf("got removed=%q lines=%v, want one empty line left", removed, buf.Lines)
	}
}

func TestSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	buf := &Buffer{Lines: []string{"a", "b"}, Path: path, Dirty: true}
	if err := buf.Save(); err != nil {
		t.Fatal(err)
	}
	if buf.Dirty {
		t.Fatal("expected Dirty to be false after save")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "a\nb" {
		t.Fatalf("got %q", string(data))
	}
}
