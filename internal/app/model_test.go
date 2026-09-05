package app

import (
	"os"
	"path/filepath"
	"strings"
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

func TestMouseClickFileOpenThenTypeThenLoadsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "target.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// Click "File" (columns 0-3, row 0).
	updated, _ = m.Update(tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)
	if m.openMenu != "File" {
		t.Fatalf("got openMenu=%q, want File", m.openMenu)
	}

	// Click "Open" (row 1, first item in the File dropdown).
	updated, _ = m.Update(tea.MouseMsg{X: 0, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)
	if m.activeDialog != dialogFileOpen {
		t.Fatal("expected clicking Open to activate the file-open dialog")
	}

	// Type the path and confirm.
	for _, r := range "target.go" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.activeDialog != dialogNone {
		t.Fatal("expected the dialog to close after a successful open")
	}
	if !m.editor.HasBuffer() {
		t.Fatal("expected the editor to have loaded target.go")
	}
	if m.focus != focusEditor {
		t.Fatal("expected focus on the editor")
	}
}

func TestMouseClickCommandsThenFilterThenEnterRunsSave(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "target.go")
	if err := os.WriteFile(file, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)

	// Click "Commands" (columns 12-19, row 0 — see menuLabels()).
	updated, _ = m.Update(tea.MouseMsg{X: 12, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)
	if m.activeDialog != dialogPalette {
		t.Fatalf("got activeDialog=%v, want dialogPalette", m.activeDialog)
	}

	// Type "save" to filter down to a single match.
	for _, r := range "save" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	matches := filteredCommands(m.commands, m.paletteFilter.Value())
	if len(matches) != 1 || matches[0].Name != "Save" {
		t.Fatalf("got matches=%v", matches)
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.activeDialog != dialogNone {
		t.Fatal("expected the palette to close after running the selected command")
	}
	// Running a command from the palette (like pressing its shortcut key
	// directly) returns a tea.Cmd rather than updating the status bar
	// synchronously — cmdSave's own tea.Cmd produces an
	// editor.CommandExecutedMsg that must flow back through Model.Update
	// for m.recentCommand to be set, exactly as the real Bubble Tea
	// runtime loop would do it. Without this step, m.recentCommand would
	// still hold the stale "Opened target.go" message set by the earlier
	// FileOpenedMsg handler, and the test would pass even if Save's
	// result never reached the status bar at all.
	if cmd == nil {
		t.Fatal("expected running Save from the palette to return a command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.recentCommand != "Saved target.go" {
		t.Fatalf("got recentCommand=%q, want %q", m.recentCommand, "Saved target.go")
	}
}

func TestWindowSizeUpdatesEvenWithDialogOpen(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	m, _ = m.clickMenuLabel("About")
	if m.activeDialog != dialogAbout {
		t.Fatal("setup failed, expected the About dialog to be open")
	}

	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	if m.width != 120 || m.height != 40 {
		t.Fatalf("got width=%d height=%d, want 120x40 (resize must apply even with a dialog open)", m.width, m.height)
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

func TestOpeningAGoFileHighlightsItInTheComposedApp(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)

	if !m.editor.HasBuffer() {
		t.Fatal("expected the file to be loaded")
	}
	view := m.View()
	if !strings.Contains(view, "func") {
		t.Fatalf("expected the rendered view to contain the source text, got %q", view)
	}
}
