package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"cody/internal/editor"
	"cody/internal/filetree"
	"cody/internal/highlight"
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

// TestRehighlightMsgReachesEditorEvenWhenTreeIsFocused is a regression test
// for a bug where app.Model.Update routed any unrecognized message type to
// whichever component currently had focus. Since the debounced rehighlight
// tick wasn't specifically handled, switching focus to the tree within the
// 150ms debounce window caused the tick to be silently delivered to the
// tree instead of the editor, leaving syntax highlighting stale.
//
// editor.RehighlightMsg's generation field is unexported, so this test
// can't construct one with a matching generation from outside the editor
// package. Instead it drives the real pipeline end to end: it captures the
// actual tea.Cmd returned by typing (the real scheduleRehighlight tick,
// unmodified), switches focus to the tree, then invokes that command for
// real — which really sleeps out the 150ms debounce window — and feeds the
// resulting real RehighlightMsg through app.Model.Update while the tree has
// focus. It then inspects the *rendered ANSI styling* (forcing TrueColor,
// since the default test color profile strips all styling) to prove the
// editor actually reparsed: corrupting "package" into "Xpackage" removes
// the tree-sitter keyword match, so if the tick reached the editor, the
// stale span (which would otherwise mis-highlight "Xpackag" as a keyword,
// misaligned by the inserted character) must be gone.
func TestRehighlightMsgReachesEditorEvenWhenTreeIsFocused(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0644); err != nil {
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

	keywordStyle, ok := highlight.StyleFor("keyword")
	if !ok {
		t.Fatal("setup failed: expected a registered keyword style")
	}
	staleStyledText := keywordStyle.Render("Xpackag")

	// Prepend "X" to the first line via a real keypress through the
	// composed app's Update, capturing the real scheduled tea.Cmd.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("setup failed: expected typing to schedule a rehighlight command")
	}

	// Switch focus to the tree before the debounce tick would normally
	// fire — this is exactly the scenario the bug report describes.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.focus != focusTree {
		t.Fatal("setup failed: expected focus on the tree")
	}

	// Invoke the actual scheduled command for real. This really sleeps out
	// the 150ms debounce window and returns the genuine editor.RehighlightMsg
	// (with its unexported generation field set correctly by editor code),
	// rather than fabricating one with a guessed generation value.
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated.(Model)

	view := m.editor.View()
	if strings.Contains(view, staleStyledText) {
		t.Fatal("expected the rehighlight tick to reach the editor and reparse even though the tree was focused, but the stale (pre-edit) keyword span was still applied to the post-edit text")
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

func TestFoldingAGoFunctionThroughTheComposedApp(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc add(a int, b int) int {\n\treturn a + 42\n}\n\nvar x = 1\n"
	if err := os.WriteFile(file, []byte(src), 0644); err != nil {
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

	for i := 0; i < 2; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m = updated.(Model)

	view := m.View()
	if strings.Contains(view, "return a + 42") {
		t.Fatal("expected the folded function body to be hidden from the rendered view")
	}
	if !strings.Contains(view, "var x = 1") {
		t.Fatal("expected content after the fold to still render")
	}
}
