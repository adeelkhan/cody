package app

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"cody/internal/filetree"
)

func TestRightClickTabOpensMenuWithSplitMoveRightLabel(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package a"), 0644); err != nil {
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

	// Tab bar is at y=1 (bodyTop, no dropdown), x starting at treeWidth(30).
	updated, _ = m.Update(tea.MouseMsg{X: 30, Y: 1, Button: tea.MouseButtonRight, Action: tea.MouseActionPress})
	m = updated.(Model)

	if m.tabMenu == nil {
		t.Fatal("expected right-clicking a tab to open the tab menu")
	}
	if m.tabMenu.pane != 0 || m.tabMenu.index != 0 {
		t.Fatalf("got tabMenu=%+v, want pane 0 index 0", m.tabMenu)
	}
	items := tabMenuItems(*m.tabMenu)
	if len(items) != 1 || items[0] != "Split + Move Right" {
		t.Fatalf("got items=%v, want [\"Split + Move Right\"]", items)
	}
}

func TestSelectingTabMenuItemMovesTheTab(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.go")
	fileB := filepath.Join(dir, "b.go")
	if err := os.WriteFile(fileA, []byte("package a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte("package b"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	// Two tabs open: moving one leaves the other behind, so this actually
	// exercises a split rather than the degenerate "moving your only tab"
	// collapse case (see TestMoveTabToOtherPaneOfOnlyOpenTabCollapsesBackToOnePane).
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileA})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileB})
	m = updated.(Model)
	m.tabMenu = &tabContextMenu{pane: 0, index: 0}

	m = m.selectTabMenuItem("Split + Move Right")

	if m.tabMenu != nil {
		t.Fatal("expected selecting an item to close the menu")
	}
	if len(m.panes) != 2 || m.activePane != 1 {
		t.Fatalf("expected the move to have happened: got %d panes, activePane=%d", len(m.panes), m.activePane)
	}
}

func TestTabMenuLabelIsMoveLeftForPane1(t *testing.T) {
	cm := tabContextMenu{pane: 1, index: 0}
	items := tabMenuItems(cm)
	if len(items) != 1 || items[0] != "Move Left" {
		t.Fatalf("got items=%v, want [\"Move Left\"]", items)
	}
}

func TestLeftClickDismissesOpenTabMenu(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package a"), 0644); err != nil {
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
	m.tabMenu = &tabContextMenu{pane: 0, index: 0}

	updated, _ = m.Update(tea.MouseMsg{X: 5, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)

	if m.tabMenu != nil {
		t.Fatal("expected a left click anywhere to dismiss the open tab menu, same as Esc")
	}
	if len(m.panes) != 1 {
		t.Fatal("expected dismissing the menu to not move the tab")
	}
}
