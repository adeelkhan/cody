package app

import "testing"

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
