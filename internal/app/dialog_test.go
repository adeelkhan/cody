package app

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	if !m.editor.HasBuffer() {
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
