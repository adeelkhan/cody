package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"cody/internal/editor"
	"cody/internal/filetree"
)

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

func TestOpenPaletteActivatesDialog(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = openPalette(m)
	if m.activeDialog != dialogPalette {
		t.Fatalf("got activeDialog=%v, want dialogPalette", m.activeDialog)
	}
	if m.paletteCursor != 0 {
		t.Fatalf("got paletteCursor=%d, want 0", m.paletteCursor)
	}
}

func TestPaletteArrowsMoveCursorWithinBounds(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = openPalette(m)
	updated, _ := m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if m.paletteCursor != 0 {
		t.Fatalf("got %d, want 0 (cannot go above the top)", m.paletteCursor)
	}
	updated, _ = m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.paletteCursor != 1 {
		t.Fatalf("got %d, want 1", m.paletteCursor)
	}
}

func TestPaletteDownStopsAtLastMatch(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = openPalette(m)
	last := len(buildCommands()) - 1
	for i := 0; i < last+5; i++ {
		updated, _ := m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
	}
	if m.paletteCursor != last {
		t.Fatalf("got %d, want %d (clamped to the last command)", m.paletteCursor, last)
	}
}

func TestPaletteTypingFiltersAndResetsCursor(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = openPalette(m)
	updated, _ := m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.paletteCursor != 1 {
		t.Fatalf("setup failed, got cursor=%d", m.paletteCursor)
	}
	updated, _ = m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("undo")})
	m = updated.(Model)
	if m.paletteFilter.Value() != "undo" {
		t.Fatalf("got filter value=%q", m.paletteFilter.Value())
	}
	if m.paletteCursor != 0 {
		t.Fatalf("got cursor=%d, want reset to 0 after the filter changed", m.paletteCursor)
	}
	matches := filteredCommands(m.commands, m.paletteFilter.Value())
	if len(matches) != 1 || matches[0].Name != "Undo" {
		t.Fatalf("got matches=%v", matches)
	}
}

func TestPaletteEnterExecutesSelectedCommandAndCloses(t *testing.T) {
	dir := t.TempDir()
	file := dir + "/a.go"
	if err := writeFile(t, file, "package main"); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)
	m, _ = openPalette(m)
	updated, _ = m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Save")})
	m = updated.(Model)
	matches := filteredCommands(m.commands, m.paletteFilter.Value())
	if len(matches) != 1 || matches[0].Name != "Save" {
		t.Fatalf("setup failed, got matches=%v", matches)
	}
	updated, cmd := m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatal("expected the palette to close after executing a command")
	}
	if cmd == nil {
		t.Fatal("expected the Save command's own command to be returned")
	}
	msg, ok := cmd().(editor.CommandExecutedMsg)
	if !ok || msg.Description == "" {
		t.Fatalf("got %+v, ok=%v", msg, ok)
	}
}

func TestPaletteEnterWithNoMatchesJustCloses(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = openPalette(m)
	updated, _ := m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zzz-no-match")})
	m = updated.(Model)
	updated, cmd := m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatal("expected enter with no matches to close the palette rather than do nothing visibly")
	}
	if cmd != nil {
		t.Fatal("expected no command when there was nothing to execute")
	}
}

func TestPaletteEscCloses(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = openPalette(m)
	updated, _ := m.updatePaletteDialog(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatal("expected esc to close the palette")
	}
}

func TestUpdateRoutesToPaletteDialogWhenActive(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.clickMenuLabel("Commands")
	if m.activeDialog != dialogPalette {
		t.Fatal("expected clicking Commands to open the palette")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = updated.(Model)
	if m.paletteFilter.Value() != "s" {
		t.Fatalf("got filter value=%q, want the root Update to route typed keys into the palette's filter input", m.paletteFilter.Value())
	}
}
