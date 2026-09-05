package app

import "testing"

func TestFilteredCommandsEmptyQueryReturnsAll(t *testing.T) {
	commands := buildCommands()
	got := filteredCommands(commands, "")
	if len(got) != len(commands) {
		t.Fatalf("got %d, want %d", len(got), len(commands))
	}
}

func TestFilteredCommandsCaseInsensitiveSubstring(t *testing.T) {
	commands := []Command{{Name: "Save"}, {Name: "Save As"}, {Name: "Undo"}}
	got := filteredCommands(commands, "SAV")
	if len(got) != 2 || got[0].Name != "Save" || got[1].Name != "Save As" {
		t.Fatalf("got %v", got)
	}
}

func TestFilteredCommandsNoMatchReturnsEmpty(t *testing.T) {
	commands := buildCommands()
	got := filteredCommands(commands, "zzz-no-such-command")
	if len(got) != 0 {
		t.Fatalf("got %d commands, want 0", len(got))
	}
}

func TestFilteredCommandsPreservesOrder(t *testing.T) {
	commands := buildCommands()
	got := filteredCommands(commands, "")
	for i := range commands {
		if got[i].Name != commands[i].Name {
			t.Fatalf("got order %v, want the same order as buildCommands()", got)
		}
	}
}
