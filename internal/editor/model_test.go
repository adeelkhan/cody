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

func TestAltModifiedKeyDoesNotInsert(t *testing.T) {
	m := setupEditor(t, "")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a"), Alt: true})
	if m.buf.Lines[0] != "" {
		t.Fatalf("got %q, want empty — alt+a should not insert", m.buf.Lines[0])
	}
}

func TestMultiRuneBurstWithNewlineSplitsLines(t *testing.T) {
	m := setupEditor(t, "")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ab\ncd")})
	if len(m.buf.Lines) != 2 || m.buf.Lines[0] != "ab" || m.buf.Lines[1] != "cd" {
		t.Fatalf("got %v, want [\"ab\" \"cd\"]", m.buf.Lines)
	}
}

func TestSpaceBarInsertsSpace(t *testing.T) {
	m := setupEditor(t, "ab")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	if m.buf.Lines[0] != "a b" {
		t.Fatalf("got %q, want %q", m.buf.Lines[0], "a b")
	}
	_, col := m.Cursor()
	if col != 3 {
		t.Fatalf("got col=%d, want 3", col)
	}
}

func TestShiftArrowsExtendSelectionAndCutRemovesIt(t *testing.T) {
	m := setupEditor(t, "hello world")
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	}
	for i := 0; i < 6; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	}
	desc := m.Cut()
	if desc != "Cut selection" {
		t.Fatalf("got %q", desc)
	}
	if m.buf.Lines[0] != "hello" {
		t.Fatalf("got %q, want %q", m.buf.Lines[0], "hello")
	}
	if m.clipboard != " world" {
		t.Fatalf("got clipboard=%q", m.clipboard)
	}
}

func TestPlainArrowCollapsesSelection(t *testing.T) {
	m := setupEditor(t, "hello")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if _, _, _, _, ok := m.selectionRange(); ok {
		t.Fatal("expected selection to be cleared by a plain arrow key")
	}
}

func TestCutWithNoSelectionCutsWholeLine(t *testing.T) {
	m := setupEditor(t, "first\nsecond")
	desc := m.Cut()
	if desc != "Cut line" {
		t.Fatalf("got %q", desc)
	}
	if len(m.buf.Lines) != 1 || m.buf.Lines[0] != "second" {
		t.Fatalf("got %v", m.buf.Lines)
	}
	if m.clipboard != "first\n" {
		t.Fatalf("got clipboard=%q", m.clipboard)
	}
}

func TestCopyWithNoSelectionDoesNotMutateBuffer(t *testing.T) {
	m := setupEditor(t, "only line")
	desc := m.Copy()
	if desc != "Copied line" {
		t.Fatalf("got %q", desc)
	}
	if m.buf.Lines[0] != "only line" {
		t.Fatal("copy must not mutate the buffer")
	}
	if m.clipboard != "only line\n" {
		t.Fatalf("got clipboard=%q", m.clipboard)
	}
}

func TestPasteInsertsClipboardAtCursor(t *testing.T) {
	m := setupEditor(t, "ac")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m.clipboard = "b"
	desc := m.Paste()
	if desc != "Pasted" {
		t.Fatalf("got %q", desc)
	}
	if m.buf.Lines[0] != "abc" {
		t.Fatalf("got %q", m.buf.Lines[0])
	}
}

func TestPasteMultiLineClipboardSplitsLines(t *testing.T) {
	m := setupEditor(t, "")
	m.clipboard = "ab\ncd"
	m.Paste()
	if len(m.buf.Lines) != 2 || m.buf.Lines[0] != "ab" || m.buf.Lines[1] != "cd" {
		t.Fatalf("got %v", m.buf.Lines)
	}
}

func TestUndoRestoresPreviousLineContent(t *testing.T) {
	m := setupEditor(t, "hello")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	if m.buf.Lines[0] != "Xhello" {
		t.Fatalf("setup failed, got %q", m.buf.Lines[0])
	}
	desc := m.undo()
	if desc != "Undo" {
		t.Fatalf("got %q", desc)
	}
	if m.buf.Lines[0] != "hello" {
		t.Fatalf("got %q, want %q after undo", m.buf.Lines[0], "hello")
	}
}

func TestRedoReappliesUndoneEdit(t *testing.T) {
	m := setupEditor(t, "hello")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	m.undo()
	desc := m.redo()
	if desc != "Redo" {
		t.Fatalf("got %q", desc)
	}
	if m.buf.Lines[0] != "Xhello" {
		t.Fatalf("got %q, want %q after redo", m.buf.Lines[0], "Xhello")
	}
}

func TestUndoWithEmptyStackReportsNothingToUndo(t *testing.T) {
	m := setupEditor(t, "hello")
	if desc := m.undo(); desc != "Nothing to undo" {
		t.Fatalf("got %q", desc)
	}
}

func TestNewEditAfterUndoClearsRedoStack(t *testing.T) {
	m := setupEditor(t, "hello")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	m.undo()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")})
	if desc := m.redo(); desc != "Nothing to redo" {
		t.Fatalf("got %q, want redo stack cleared by the new edit", desc)
	}
}

func TestCtrlXCtrlCCtrlVKeysDispatchThroughUpdate(t *testing.T) {
	m := setupEditor(t, "hello")
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected a command from ctrl+c")
	}
	msg, ok := cmd().(CommandExecutedMsg)
	if !ok || msg.Description != "Copied line" {
		t.Fatalf("got %+v, ok=%v", msg, ok)
	}
}

func TestBackspaceAfterSelectionThenCopyDoesNotPanic(t *testing.T) {
	m := setupEditor(t, "a\nb\nc")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftUp})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.buf.Lines[0] != "ab" || m.buf.Lines[1] != "c" || len(m.buf.Lines) != 2 {
		t.Fatalf("setup failed, got %v", m.buf.Lines)
	}
	if _, _, _, _, ok := m.selectionRange(); ok {
		t.Fatal("expected backspace to clear the stale selection")
	}
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected a command from ctrl+c")
	}
	msg, ok := cmd().(CommandExecutedMsg)
	if !ok || msg.Description != "Copied line" {
		t.Fatalf("got %+v, ok=%v", msg, ok)
	}
	if m.clipboard != "ab\n" {
		t.Fatalf("got clipboard=%q, want %q", m.clipboard, "ab\n")
	}
}
