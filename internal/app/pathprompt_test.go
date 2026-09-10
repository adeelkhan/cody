package app

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCtrlNOpensBlankUntitledTab(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = updated.(Model)

	if len(m.panes[0].tabs) != 1 {
		t.Fatalf("got %d tabs, want 1", len(m.panes[0].tabs))
	}
	if m.panes[0].tabs[0].path != "" {
		t.Fatalf("got path=%q, want empty (untitled)", m.panes[0].tabs[0].path)
	}
	if m.focus != focusEditor {
		t.Fatal("expected focus to move to the editor")
	}
	if !m.activeEditor().IsUntitled() {
		t.Fatal("expected the new tab's editor to be untitled")
	}
}

func TestCtrlSOnUntitledTabOpensSaveAsDialogInsteadOfSaving(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(Model)

	if m.activeDialog != dialogPathPrompt {
		t.Fatalf("got activeDialog=%v, want dialogPathPrompt", m.activeDialog)
	}
	if m.pathPromptAction != pathPromptSaveAs {
		t.Fatalf("got pathPromptAction=%v, want pathPromptSaveAs", m.pathPromptAction)
	}
}

func TestConfirmingSaveAsWritesFileRenamesTabAndClearsDirty(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("package main")})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(Model)

	target := filepath.Join(dir, "main.go")
	m.pathDirInput.SetValue(dir)
	m.pathNameInput.SetValue("main.go")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.activeDialog != dialogNone {
		t.Fatal("expected the dialog to close after a successful Save As")
	}
	if m.panes[0].tabs[0].path != target {
		t.Fatalf("got tab path=%q, want %q", m.panes[0].tabs[0].path, target)
	}
	if m.activeEditor().HasUnsavedChanges() {
		t.Fatal("expected Save As to clear dirty status")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "package main" {
		t.Fatalf("got %q, want %q", string(data), "package main")
	}
}

func TestSaveAsToExistingPathShowsErrorAndStaysOpen(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "taken.go")
	if err := os.WriteFile(existing, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(Model)

	m.pathDirInput.SetValue(dir)
	m.pathNameInput.SetValue("taken.go")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.activeDialog != dialogPathPrompt {
		t.Fatal("expected the dialog to stay open after a name collision")
	}
	if m.pathPromptError == "" {
		t.Fatal("expected an error message")
	}
}

func TestCancelingPathPromptChangesNothing(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.activeDialog != dialogNone {
		t.Fatal("expected Esc to close the dialog")
	}
	if len(m.panes[0].tabs) != 1 || m.panes[0].tabs[0].path != "" {
		t.Fatal("expected the untitled tab to be unchanged after canceling")
	}
}

func TestFileMenuNewCreatesAndOpensAFile(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	m, cmd := cmdNewFilePrompt(m)
	if m.activeDialog != dialogPathPrompt {
		t.Fatal("expected cmdNewFilePrompt to open the path-prompt dialog")
	}
	if m.pathPromptAction != pathPromptNewFile {
		t.Fatalf("got pathPromptAction=%v, want pathPromptNewFile", m.pathPromptAction)
	}
	if cmd == nil {
		t.Fatal("expected a command (textinput.Blink)")
	}

	m.pathDirInput.SetValue(dir)
	m.pathNameInput.SetValue("fresh.go")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.activeDialog != dialogNone {
		t.Fatal("expected the dialog to close after a successful create")
	}
	if len(m.panes[0].tabs) != 1 || m.panes[0].tabs[0].path != filepath.Join(dir, "fresh.go") {
		t.Fatalf("got tabs=%v, want one tab for fresh.go", m.panes[0].tabs)
	}
	if _, err := os.Stat(filepath.Join(dir, "fresh.go")); err != nil {
		t.Fatal("expected fresh.go to exist on disk")
	}
}
