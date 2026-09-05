package app

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"cody/internal/editor"
	"cody/internal/filetree"
)

func TestTabTogglesFocus(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if m.focus != focusTree {
		t.Fatal("expected initial focus on tree")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.focus != focusEditor {
		t.Fatal("expected focus to move to editor")
	}
}

func TestFileOpenedMsgLoadsEditorAndSwitchesFocus(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)
	if m.focus != focusEditor {
		t.Fatal("expected focus to move to editor")
	}
	if !m.editor.HasBuffer() {
		t.Fatal("expected editor to have a loaded buffer")
	}
}

func TestCtrlSWorksRegardlessOfFocus(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)
	// Focus is now on the editor after opening; switch back to the tree.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.focus != focusTree {
		t.Fatal("expected focus back on tree")
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected ctrl+s to produce a save command even with the tree focused")
	}
	msg, ok := cmd().(editor.CommandExecutedMsg)
	if !ok {
		t.Fatalf("got %T, want editor.CommandExecutedMsg", msg)
	}
}

func TestCtrlQQuits(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if cmd == nil {
		t.Fatal("expected a quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected the command to produce tea.QuitMsg")
	}
}

func TestFullFlowOpenTypeSaveUpdatesStatusBar(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	tree, cmd := m.tree.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.tree = tree
	if cmd == nil {
		t.Fatal("expected a command opening a.go")
	}

	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.focus != focusEditor || !m.editor.HasBuffer() {
		t.Fatal("expected file opened and focus moved to the editor")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	m = updated.(Model)

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected ctrl+s to produce a save command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	if m.recentCommand != "Saved a.go" {
		t.Fatalf("got recentCommand=%q, want %q", m.recentCommand, "Saved a.go")
	}
}

func TestViewRendersAtSmallSize(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 5})
	m = updated.(Model)
	if view := m.View(); view == "" {
		t.Fatal("expected a non-empty rendered view at a small terminal size")
	}
}
