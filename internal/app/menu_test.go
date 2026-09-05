package app

import (
	"os"
	"strings"
	"testing"

	"cody/internal/filetree"
)

func writeFile(t *testing.T, path, content string) error {
	t.Helper()
	return os.WriteFile(path, []byte(content), 0644)
}

func TestMenuLabelAtHitsFile(t *testing.T) {
	name, ok := menuLabelAt(0)
	if !ok || name != "File" {
		t.Fatalf("got %q, ok=%v", name, ok)
	}
}

func TestMenuLabelAtMissesGap(t *testing.T) {
	// "File" occupies columns 0-3; column 4 is part of the 2-space gap.
	if _, ok := menuLabelAt(4); ok {
		t.Fatal("expected the gap between labels to miss")
	}
}

func TestMenuLabelAtHitsEdit(t *testing.T) {
	// "File  " is 6 columns (4 + 2-space gap), so "Edit" starts at column 6.
	name, ok := menuLabelAt(6)
	if !ok || name != "Edit" {
		t.Fatalf("got %q, ok=%v", name, ok)
	}
}

func TestMenuItemsForFile(t *testing.T) {
	items := menuItemsFor("File")
	if len(items) != 2 || items[0] != "Open" || items[1] != "Save" {
		t.Fatalf("got %v", items)
	}
}

func TestMenuItemsForEdit(t *testing.T) {
	items := menuItemsFor("Edit")
	want := []string{"Cut", "Paste", "Copy", "Save"}
	if len(items) != len(want) {
		t.Fatalf("got %v", items)
	}
	for i, w := range want {
		if items[i] != w {
			t.Fatalf("got %v, want %v", items, want)
		}
	}
}

func TestCommandByNameFound(t *testing.T) {
	commands := buildCommands()
	cmd, ok := commandByName(commands, "Save")
	if !ok || cmd.Shortcut != "ctrl+s" {
		t.Fatalf("got %+v, ok=%v", cmd, ok)
	}
}

func TestClickFileLabelOpensDropdown(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.clickMenuLabel("File")
	if m.openMenu != "File" {
		t.Fatalf("got openMenu=%q", m.openMenu)
	}
}

func TestClickSameLabelTwiceCloses(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.clickMenuLabel("File")
	m, _ = m.clickMenuLabel("File")
	if m.openMenu != "" {
		t.Fatalf("got openMenu=%q, want closed", m.openMenu)
	}
}

func TestClickCommandsLabelOpensPalette(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.clickMenuLabel("Commands")
	if m.openMenu != "" {
		t.Fatal("Commands must not open a dropdown, it opens a modal palette")
	}
	if m.activeDialog != dialogPalette {
		t.Fatalf("got activeDialog=%v, want dialogPalette", m.activeDialog)
	}
}

func TestClickDropdownItemRunsCommand(t *testing.T) {
	dir := t.TempDir()
	file := dir + "/a.go"
	if err := writeFile(t, file, "hello"); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)
	m, _ = m.clickMenuLabel("Edit")
	// "Edit" starts at column 6 ("File" is 0-3, then a 2-space gap), so the
	// click's x must fall in Edit's column band [6, 6+dropdownWidth) or the
	// hit-test in handleClick will treat it as an outside click and close
	// the dropdown without running anything. Edit's items are Cut, Paste,
	// Copy, Save (rows 1-4 below the menu bar); row 4 ("Save") is at y=4
	// (row = y-1 = 3, items[3] = "Save").
	updated, cmd := m.handleClick(6, 4)
	m = updated.(Model)
	if m.openMenu != "" {
		t.Fatal("expected the dropdown to close after a click")
	}
	if cmd == nil {
		t.Fatal("expected clicking Save to produce a command")
	}
}

func TestRenderDropdownAlignsWithHitTestModel(t *testing.T) {
	commands := buildCommands()
	dropdown := renderDropdown("Edit", commands)
	lines := strings.Split(dropdown, "\n")
	items := menuItemsFor("Edit")
	if len(lines) != len(items) {
		t.Fatalf("got %d rendered lines, want %d (one per item, no border rows)", len(lines), len(items))
	}
	label, ok := findLabel("Edit")
	if !ok {
		t.Fatal("expected to find the Edit label")
	}
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		if indent < label.startCol {
			t.Fatalf("line %d has indent %d, want at least %d (Edit's startCol)", i, indent, label.startCol)
		}
	}
}
