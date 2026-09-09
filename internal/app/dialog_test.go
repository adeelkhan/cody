package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"cody/internal/filetree"
)

func TestCmdOpenFilePromptActivatesDialog(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = cmdOpenFilePrompt(m)
	if m.activeDialog != dialogFileOpen {
		t.Fatalf("got activeDialog=%v, want dialogFileOpen", m.activeDialog)
	}
}

func TestFileOpenDialogEnterLoadsRelativePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = cmdOpenFilePrompt(m)
	m.fileOpenInput.SetValue("a.go")
	updated, _ := m.updateFileOpenDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatal("expected the dialog to close after a successful open")
	}
	if !m.activeEditor().HasBuffer() {
		t.Fatal("expected the editor to have a loaded buffer")
	}
	if m.focus != focusEditor {
		t.Fatal("expected focus to move to the editor")
	}
}

func TestFileOpenDialogEnterWithBadPathShowsErrorAndStaysOpen(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = cmdOpenFilePrompt(m)
	m.fileOpenInput.SetValue("does-not-exist.go")
	updated, _ := m.updateFileOpenDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.activeDialog != dialogFileOpen {
		t.Fatal("expected the dialog to stay open after a failed open")
	}
	if m.fileOpenError == "" {
		t.Fatal("expected an error message")
	}
}

func TestFileOpenDialogEscCloses(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = cmdOpenFilePrompt(m)
	updated, _ := m.updateFileOpenDialog(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatal("expected esc to close the dialog")
	}
}

func TestUpdateRoutesToFileOpenDialogWhenActive(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = updated.(Model)
	if m.activeDialog != dialogFileOpen {
		t.Fatal("expected ctrl+o to open the file-open dialog via the root Update")
	}
	// While the dialog is active, a plain rune key must go to the text input,
	// not to the tree/editor focus dispatch or the registry.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(Model)
	if m.fileOpenInput.Value() != "a" {
		t.Fatalf("got input value=%q, want %q", m.fileOpenInput.Value(), "a")
	}
}

func TestClickAboutLabelOpensAboutDialog(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.clickMenuLabel("About")
	if m.activeDialog != dialogAbout {
		t.Fatalf("got activeDialog=%v, want dialogAbout", m.activeDialog)
	}
}

func TestAboutDialogClosesOnAnyKey(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.clickMenuLabel("About")
	updated, _ := m.updateAboutDialog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatal("expected any key to close the About dialog")
	}
}

func TestUpdateRoutesToAboutDialogWhenActive(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.clickMenuLabel("About")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatal("expected the root Update to route to updateAboutDialog and close it")
	}
}

func TestMouseClickDismissesAboutDialog(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.clickMenuLabel("About")

	updated, _ := m.Update(tea.MouseMsg{X: 40, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)

	if m.activeDialog != dialogNone {
		t.Fatal("expected a left click to dismiss the About dialog, same as Esc")
	}
}

func TestMouseClickDismissesSearchDialogAndClearsSearchState(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("foo\nbar\nfoo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = updated.(Model)
	for _, r := range "foo" {
		updated, _ = m.updateSearchDialog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}

	updated, _ = m.Update(tea.MouseMsg{X: 5, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)

	if m.activeDialog != dialogNone {
		t.Fatal("expected a left click to dismiss the search dialog, same as Esc")
	}
	if _, status := m.activeEditor().FindNext(); status != "No matches" {
		t.Fatalf("got %q, want \"No matches\" — the click should clear search state exactly like Esc, not just hide the dialog", status)
	}
}

func TestMouseClickCancelsConfirmDialogWithoutConfirming(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package a"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m = openAndDirtyFile(t, m, file)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m = updated.(Model)
	if m.activeDialog != dialogConfirmDiscard {
		t.Fatal("setup failed: expected ctrl+q on a dirty tab to open the confirm dialog")
	}

	updated, cmd := m.Update(tea.MouseMsg{X: 5, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)

	if m.activeDialog != dialogNone {
		t.Fatal("expected a left click to close the confirm dialog")
	}
	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatal("expected a click to cancel (same as Esc), not confirm — it must never quit the app")
		}
	}
	if len(m.tabs) != 1 || !m.tabs[0].editor.HasUnsavedChanges() {
		t.Fatal("expected the dirty tab to remain open and dirty after the click cancels the dialog")
	}
}

func TestMouseRightClickDoesNotDismissDialog(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.clickMenuLabel("About")

	updated, _ := m.Update(tea.MouseMsg{X: 40, Y: 10, Button: tea.MouseButtonRight, Action: tea.MouseActionPress})
	m = updated.(Model)

	if m.activeDialog != dialogAbout {
		t.Fatal("expected only a left click to dismiss a dialog")
	}
}

func TestMouseMotionDoesNotDismissDialog(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.clickMenuLabel("About")

	updated, _ := m.Update(tea.MouseMsg{X: 40, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m = updated.(Model)

	if m.activeDialog != dialogAbout {
		t.Fatal("expected only a press (not motion) to dismiss a dialog")
	}
}

func TestCtrlFOpensSearchDialogOnlyWithABufferOpen(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatalf("got activeDialog=%v, want dialogNone (no file open)", m.activeDialog)
	}
	if cmd == nil {
		t.Fatal("expected a CommandExecutedMsg command reporting no file open")
	}
}

func TestCtrlFOpensSearchDialogWithABufferOpen(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("hello world\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = updated.(Model)
	if m.activeDialog != dialogSearch {
		t.Fatalf("got activeDialog=%v, want dialogSearch", m.activeDialog)
	}
}

func TestTypingInSearchDialogJumpsToMatchAndEscClosesAndClears(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("foo\nbar\nfoo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = updated.(Model)

	for _, r := range "foo" {
		updated, _ = m.updateSearchDialog(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	if m.recentCommand != "Match 1 of 2" {
		t.Fatalf("got recentCommand=%q, want %q", m.recentCommand, "Match 1 of 2")
	}

	updated, _ = m.updateSearchDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.recentCommand != "Match 2 of 2" {
		t.Fatalf("got recentCommand=%q, want %q after Enter", m.recentCommand, "Match 2 of 2")
	}

	updated, _ = m.updateSearchDialog(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatalf("got activeDialog=%v, want dialogNone after Esc", m.activeDialog)
	}
	// Cursor still emits reverse-video; check for the search-specific yellow
	// background (color 220) which disappears when the search is cleared.
	if strings.Contains(m.activeEditor().View(), "\x1b[48;5;220m") {
		t.Fatal("expected Esc to clear the match highlight (yellow background)")
	}
}
