package filetree

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func setupModel(t *testing.T) Model {
	t.Helper()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.go"), "")
	mustWriteFile(t, filepath.Join(dir, "b.go"), "")
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCursorMovesDown(t *testing.T) {
	m := setupModel(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Fatalf("got cursor %d, want 1", m.cursor)
	}
}

func TestCursorStopsAtBottom(t *testing.T) {
	m := setupModel(t)
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.cursor != len(m.flat)-1 {
		t.Fatalf("got cursor %d, want %d", m.cursor, len(m.flat)-1)
	}
}

func TestEnterOnFileEmitsFileOpenedMsg(t *testing.T) {
	m := setupModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command")
	}
	msg := cmd()
	opened, ok := msg.(FileOpenedMsg)
	if !ok {
		t.Fatalf("got %T, want FileOpenedMsg", msg)
	}
	if filepath.Base(opened.Path) != "a.go" {
		t.Fatalf("got %q, want a.go", opened.Path)
	}
}

func TestEnterOnDirExpands(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(sub, "f.txt"), "")
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	before := len(m.flat)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.flat) <= before {
		t.Fatalf("expected flat list to grow after expanding, got %d -> %d", before, len(m.flat))
	}
}
