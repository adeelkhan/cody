package filetree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func setupModel(t *testing.T) Model {
	t.Helper()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.go"), "")
	mustWriteFile(t, filepath.Join(dir, "b.go"), "")
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCursorMovesDown(t *testing.T) {
	m := setupModel(t)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Fatalf("got cursor %d, want 1", m.cursor)
	}
}

func TestCursorStopsAtBottom(t *testing.T) {
	m := setupModel(t)
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.cursor != len(m.flat)-1 {
		t.Fatalf("got cursor %d, want %d", m.cursor, len(m.flat)-1)
	}
}

func TestEnterOnFileEmitsFileOpenedMsg(t *testing.T) {
	m := setupModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command")
	}
	msg := cmd()
	opened, ok := msg.(FileOpenedMsg)
	if !ok {
		t.Fatalf("got %T, want FileOpenedMsg", msg)
	}
	if filepath.Base(opened.Path) != "a.go" {
		t.Fatalf("got %q, want a.go", opened.Path)
	}
}

func TestEnterOnDirExpands(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(sub, "f.txt"), "")
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	before := len(m.flat)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.flat) <= before {
		t.Fatalf("expected flat list to grow after expanding, got %d -> %d", before, len(m.flat))
	}
}

func setupModelWithNFiles(t *testing.T, n int) Model {
	t.Helper()
	dir := t.TempDir()
	for i := 0; i < n; i++ {
		mustWriteFile(t, filepath.Join(dir, fmt.Sprintf("file%02d.go", i)), "")
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestViewIsUnboundedWhenHeightIsNeverSet(t *testing.T) {
	m := setupModelWithNFiles(t, 20)
	view := m.View()
	if got := strings.Count(view, "\n"); got != len(m.flat) {
		t.Fatalf("got %d rendered lines, want %d (unbounded)", got, len(m.flat))
	}
}

func TestViewClipsRenderedLinesToTheSetHeight(t *testing.T) {
	m := setupModelWithNFiles(t, 20)
	m = m.SetSize(40, 5)
	view := m.View()
	if got := strings.Count(view, "\n"); got != 5 {
		t.Fatalf("got %d rendered lines, want 5", got)
	}
}

func TestMovingCursorPastViewportScrollsTheTree(t *testing.T) {
	m := setupModelWithNFiles(t, 20)
	m = m.SetSize(40, 5)
	firstName := m.flat[0].node.Name
	for i := 0; i < 7; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	view := m.View()
	if strings.Contains(view, firstName) {
		t.Fatal("expected the first item to have scrolled out of view")
	}
	if !strings.Contains(view, m.flat[m.cursor].node.Name) {
		t.Fatal("expected the selected item to be visible")
	}
}

func TestViewIncludesAScrollbarWhenContentOverflowsHeight(t *testing.T) {
	m := setupModelWithNFiles(t, 20)
	m = m.SetSize(40, 5)
	view := m.View()
	if !strings.ContainsRune(view, '█') && !strings.ContainsRune(view, '│') {
		t.Fatal("expected a scrollbar thumb or track when content overflows the viewport")
	}
}

func TestViewHasNoScrollbarMarksWhenContentFitsTheHeight(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	m = m.SetSize(40, 10)
	view := m.View()
	if strings.ContainsRune(view, '█') || strings.ContainsRune(view, '│') {
		t.Fatal("expected no scrollbar marks when content fits within the viewport")
	}
}

// Regression test: the scrollbar rune must land in the same column on every
// rendered row, regardless of how long that row's name is. Appending it
// directly after variable-length text (the original bug) makes it drift to
// a different column per line, which reads as a stray character attached to
// the text rather than a scrollbar.
func TestScrollbarColumnStaysAlignedAcrossNamesOfDifferentLengths(t *testing.T) {
	dir := t.TempDir()
	names := []string{"a.go", "bb.go", "ccc.go", "dddd.go", "eeeee.go", "ffffff.go"}
	for _, n := range names {
		mustWriteFile(t, filepath.Join(dir, n), "")
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m = m.SetSize(40, 3)

	view := m.View()
	renderedLines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(renderedLines) == 0 {
		t.Fatal("expected at least one rendered line")
	}
	col := -1
	for _, l := range renderedLines {
		idx := -1
		for i, r := range []rune(l) {
			if r == '█' || r == '│' {
				idx = i
			}
		}
		if idx < 0 {
			t.Fatalf("expected a scrollbar rune in line %q", l)
		}
		if col == -1 {
			col = idx
		} else if idx != col {
			t.Fatalf("scrollbar column drifted: got %d, want %d (line %q)", idx, col, l)
		}
	}
}

// Regression test: padding a row to the pane's content width must never
// wrap or truncate a row that's already wider than that width — an earlier
// version of this fix used lipgloss.Style.Width().Render(), which silently
// hard-wraps overlong rows into multiple physical lines, defeating the
// viewport-clipping fix (a pane's rendered output must never exceed its
// set height).
func TestPaddingALongNameDoesNotWrapItIntoMultiplePhysicalLines(t *testing.T) {
	dir := t.TempDir()
	longName := strings.Repeat("x", 100) + ".go"
	mustWriteFile(t, filepath.Join(dir, longName), "")
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m = m.SetSize(40, 3)

	view := m.View()
	if got := strings.Count(view, "\n"); got != 1 {
		t.Fatalf("got %d physical lines, want 1 — a name wider than the pane must not wrap", got)
	}
	if !strings.Contains(view, longName) {
		t.Fatal("expected the long name's full content to still be present, unwrapped")
	}
}
