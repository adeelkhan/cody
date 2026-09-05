package app

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"cody/internal/editor"
	"cody/internal/filetree"
)

func TestCommandForShortcutFound(t *testing.T) {
	commands := []Command{
		{Name: "Save", Shortcut: "ctrl+s"},
		{Name: "Quit", Shortcut: "ctrl+q"},
	}
	cmd, ok := commandForShortcut(commands, "ctrl+q")
	if !ok || cmd.Name != "Quit" {
		t.Fatalf("got %+v, ok=%v, want Quit, true", cmd, ok)
	}
}

func TestCommandForShortcutNotFound(t *testing.T) {
	commands := []Command{{Name: "Save", Shortcut: "ctrl+s"}}
	_, ok := commandForShortcut(commands, "ctrl+z")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestBuildCommandsHasSaveAndQuit(t *testing.T) {
	commands := buildCommands()
	if _, ok := commandForShortcut(commands, "ctrl+s"); !ok {
		t.Fatal("expected a ctrl+s command")
	}
	if _, ok := commandForShortcut(commands, "ctrl+q"); !ok {
		t.Fatal("expected a ctrl+q command")
	}
}

func TestBuildCommandsHasCutCopyPasteUndoRedo(t *testing.T) {
	commands := buildCommands()
	for _, shortcut := range []string{"ctrl+x", "ctrl+c", "ctrl+v", "ctrl+z", "ctrl+y"} {
		if _, ok := commandForShortcut(commands, shortcut); !ok {
			t.Fatalf("expected a %s command", shortcut)
		}
	}
}

func TestCutCommandDelegatesToEditorRegardlessOfFocus(t *testing.T) {
	dir := t.TempDir()
	file := dir + "/a.go"
	if err := os.WriteFile(file, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // back to the tree
	m = updated.(Model)
	if m.focus != focusTree {
		t.Fatal("expected focus on tree")
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected ctrl+c to produce a command even with the tree focused")
	}
	msg, ok := cmd().(editor.CommandExecutedMsg)
	if !ok || msg.Description != "Copied line" {
		t.Fatalf("got %+v, ok=%v", msg, ok)
	}
}
