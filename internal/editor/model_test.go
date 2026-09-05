package editor

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func setupEditor(t *testing.T, content string) Model {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestTypingInsertsRunes(t *testing.T) {
	m := setupEditor(t, "")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hi")})
	if m.buf.Lines[0] != "hi" {
		t.Fatalf("got %q, want %q", m.buf.Lines[0], "hi")
	}
	line, col := m.Cursor()
	if line != 1 || col != 3 {
		t.Fatalf("got line=%d col=%d, want 1,3", line, col)
	}
}

func TestEnterSplitsLine(t *testing.T) {
	m := setupEditor(t, "helloworld")
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.buf.Lines) != 2 || m.buf.Lines[0] != "hello" || m.buf.Lines[1] != "world" {
		t.Fatalf("got %v", m.buf.Lines)
	}
}

func TestCtrlSSavesAndEmitsMessage(t *testing.T) {
	m := setupEditor(t, "content")
	path := m.buf.Path
	m.buf.Lines[0] = "changed"
	m.buf.Dirty = true
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("expected a command")
	}
	msg, ok := cmd().(CommandExecutedMsg)
	if !ok {
		t.Fatalf("got %T, want CommandExecutedMsg", msg)
	}
	if msg.Description != "Saved "+filepath.Base(path) {
		t.Fatalf("got %q", msg.Description)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "changed" {
		t.Fatalf("got %q", string(data))
	}
}

func TestFiletype(t *testing.T) {
	m := setupEditor(t, "")
	if m.Filetype() != "go" {
		t.Fatalf("got %q, want go", m.Filetype())
	}
}
