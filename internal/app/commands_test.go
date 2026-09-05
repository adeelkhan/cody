package app

import (
	"os"
	"path/filepath"
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

func TestEditorCommandsWorkRegardlessOfFocus(t *testing.T) {
	shortcuts := []string{"ctrl+x", "ctrl+c", "ctrl+v", "ctrl+z", "ctrl+y"}
	for _, shortcut := range shortcuts {
		t.Run(shortcut, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "a.go")
			if err := os.WriteFile(file, []byte("hello world"), 0644); err != nil {
				t.Fatal(err)
			}

			mEditorFocused, err := New(dir, false)
			if err != nil {
				t.Fatal(err)
			}
			updated, _ := mEditorFocused.Update(filetree.FileOpenedMsg{Path: file})
			mEditorFocused = updated.(Model)
			if mEditorFocused.focus != focusEditor {
				t.Fatal("expected focus on editor after opening a file")
			}
			updatedA, cmdA := mEditorFocused.Update(tea.KeyMsg{Type: keyTypeFor(shortcut)})

			mTreeFocused, err := New(dir, false)
			if err != nil {
				t.Fatal(err)
			}
			updated, _ = mTreeFocused.Update(filetree.FileOpenedMsg{Path: file})
			mTreeFocused = updated.(Model)
			updated, _ = mTreeFocused.Update(tea.KeyMsg{Type: tea.KeyTab})
			mTreeFocused = updated.(Model)
			if mTreeFocused.focus != focusTree {
				t.Fatal("expected focus back on tree")
			}
			updatedB, cmdB := mTreeFocused.Update(tea.KeyMsg{Type: keyTypeFor(shortcut)})

			if (cmdA == nil) != (cmdB == nil) {
				t.Fatalf("%s: cmd nil-ness differs between focus states (editor-focused=%v, tree-focused=%v)", shortcut, cmdA != nil, cmdB != nil)
			}
			if cmdA == nil {
				return
			}
			descA := cmdA().(editor.CommandExecutedMsg).Description
			descB := cmdB().(editor.CommandExecutedMsg).Description
			if descA != descB {
				t.Fatalf("%s: got description %q (editor-focused) vs %q (tree-focused)", shortcut, descA, descB)
			}
			mA := updatedA.(Model)
			mB := updatedB.(Model)
			if mA.editor.HasBuffer() != mB.editor.HasBuffer() {
				t.Fatalf("%s: HasBuffer differs between focus states", shortcut)
			}
		})
	}
}

func keyTypeFor(shortcut string) tea.KeyType {
	switch shortcut {
	case "ctrl+x":
		return tea.KeyCtrlX
	case "ctrl+c":
		return tea.KeyCtrlC
	case "ctrl+v":
		return tea.KeyCtrlV
	case "ctrl+z":
		return tea.KeyCtrlZ
	case "ctrl+y":
		return tea.KeyCtrlY
	}
	panic("unknown shortcut: " + shortcut)
}
