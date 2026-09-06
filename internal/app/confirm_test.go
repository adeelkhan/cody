package app

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"cody/internal/filetree"
)

func openAndDirtyFile(t *testing.T, m Model, path string) Model {
	t.Helper()
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: path})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	return updated.(Model)
}

func TestCloseTabOnCleanTabClosesImmediately(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package a"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)

	m, _ = m.closeTab(0)

	if len(m.tabs) != 0 {
		t.Fatalf("got %d tabs, want 0", len(m.tabs))
	}
	if m.activeTab != -1 {
		t.Fatalf("got activeTab=%d, want -1", m.activeTab)
	}
	if m.focus != focusTree {
		t.Fatal("expected focus to fall back to the tree once the last tab closes")
	}
	if m.activeDialog != dialogNone {
		t.Fatal("expected no confirmation dialog for a clean tab")
	}
}

func TestCloseTabReassignsActiveTabCorrectly(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("package p"), 0644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		updated, _ := m.Update(filetree.FileOpenedMsg{Path: f})
		m = updated.(Model)
	}
	// 3 tabs open, activeTab == 2 (c.go).

	// Closing a tab before the active one shifts activeTab left by one.
	m, _ = m.closeTab(0)
	if m.activeTab != 1 {
		t.Fatalf("got activeTab=%d, want 1 after closing a tab before it", m.activeTab)
	}

	// Now 2 tabs remain (b.go, c.go), activeTab == 1 (c.go, the last one).
	// Closing the active (last) tab falls back to the one before it.
	m, _ = m.closeTab(1)
	if m.activeTab != 0 {
		t.Fatalf("got activeTab=%d, want 0 after closing the active last tab", m.activeTab)
	}
}

func TestCloseTabReassignsActiveTabWhenLaterTabExists(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("package p"), 0644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		updated, _ := m.Update(filetree.FileOpenedMsg{Path: f})
		m = updated.(Model)
	}
	// 3 tabs open (a.go, b.go, c.go), activeTab == 2 (c.go).

	// Re-open b.go: openOrSwitch switches to its existing tab rather than
	// duplicating it, moving activeTab to 1 (b.go), which has a tab after
	// it (c.go at index 2).
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: files[1]})
	m = updated.(Model)
	if m.activeTab != 1 {
		t.Fatalf("got activeTab=%d, want 1 after switching back to b.go", m.activeTab)
	}
	if len(m.tabs) != 3 {
		t.Fatalf("got %d tabs, want 3 (no duplicate from re-opening b.go)", len(m.tabs))
	}

	// Closing the active tab (b.go) that has a later tab (c.go) shifts
	// that later tab left into the closed slot, so activeTab stays at 1.
	m, _ = m.closeTab(1)
	if m.activeTab != 1 {
		t.Fatalf("got activeTab=%d, want 1 after closing the active tab with a later tab present", m.activeTab)
	}
	if len(m.tabs) != 2 {
		t.Fatalf("got %d tabs, want 2", len(m.tabs))
	}
	if m.tabs[m.activeTab].path != files[2] {
		t.Fatalf("got active tab path=%q, want %q (c.go)", m.tabs[m.activeTab].path, files[2])
	}
}

func TestCloseTabOnDirtyTabOpensConfirmDialog(t *testing.T) {
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

	m, _ = m.closeTab(0)

	if m.activeDialog != dialogConfirmDiscard {
		t.Fatal("expected closing a dirty tab to open the confirm dialog")
	}
	if m.pendingConfirm != confirmCloseTab {
		t.Fatalf("got pendingConfirm=%v, want confirmCloseTab", m.pendingConfirm)
	}
	if m.pendingConfirmTab != 0 {
		t.Fatalf("got pendingConfirmTab=%d, want 0", m.pendingConfirmTab)
	}
	if len(m.tabs) != 1 {
		t.Fatal("expected the tab to remain open until confirmed")
	}
}

func TestConfirmDialogConfirmingCloseTabRemovesIt(t *testing.T) {
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
	m, _ = m.closeTab(0)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if len(m.tabs) != 0 {
		t.Fatal("expected confirming to remove the tab")
	}
	if m.activeDialog != dialogNone {
		t.Fatal("expected the dialog to close after confirming")
	}
}

func TestConfirmDialogCancelingLeavesTabOpen(t *testing.T) {
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
	m, _ = m.closeTab(0)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if len(m.tabs) != 1 {
		t.Fatal("expected canceling to leave the tab open")
	}
	if m.activeDialog != dialogNone {
		t.Fatal("expected the dialog to close after canceling")
	}
}

func TestQuitWithNoDirtyTabsQuitsImmediately(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected tea.Quit with no dirty tabs")
	}
	if m.activeDialog != dialogNone {
		t.Fatal("expected no confirmation dialog with no dirty tabs")
	}
}

func TestQuitWithADirtyTabOpensConfirmDialog(t *testing.T) {
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

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m = updated.(Model)

	if m.activeDialog != dialogConfirmDiscard {
		t.Fatal("expected ctrl+q with a dirty tab to open the confirm dialog instead of quitting")
	}
	if m.pendingConfirm != confirmQuit {
		t.Fatalf("got pendingConfirm=%v, want confirmQuit", m.pendingConfirm)
	}
	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatal("expected ctrl+q with a dirty tab not to quit yet")
		}
	}
}

func TestConfirmDialogConfirmingQuitProducesTeaQuit(t *testing.T) {
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

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected confirming the quit dialog to produce tea.Quit")
	}
}

func TestConfirmDialogCancelingQuitReturnsToEditingWithTabIntact(t *testing.T) {
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

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.activeDialog != dialogNone {
		t.Fatal("expected canceling to close the dialog")
	}
	if len(m.tabs) != 1 || !m.tabs[0].editor.HasUnsavedChanges() {
		t.Fatal("expected the dirty tab to remain open and dirty after canceling quit")
	}
}
