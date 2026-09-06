package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"cody/internal/editor"
	"cody/internal/filetree"
	"cody/internal/highlight"
	"cody/internal/terminal"
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
	if !m.activeEditor().HasBuffer() {
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
	// Shift+Tab (not Tab) goes editor -> tree in the three-way focus cycle
	// (Tree -> Editor -> Terminal -> Tree forward, so backward from Editor
	// lands on Tree).
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
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
	if m.focus != focusEditor || !m.activeEditor().HasBuffer() {
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

	// Click "Open" (first item in the File dropdown; y=2 skips the menu bar
	// row and the dropdown's own top border row).
	updated, _ = m.Update(tea.MouseMsg{X: 0, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
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
	if !m.activeEditor().HasBuffer() {
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
	// Shift+Tab (not Tab) goes editor -> tree in the three-way focus cycle
	// (Tree -> Editor -> Terminal -> Tree forward, so backward from Editor
	// lands on Tree).
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
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

	view := m.activeEditor().View()
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

	if !m.activeEditor().HasBuffer() {
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

func TestLargeFileDoesNotPushTheTreePaneOutOfViewAndClipsToWindowHeight(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "biglist.txt")
	lines := make([]string, 500)
	for i := range lines {
		lines[i] = fmt.Sprintf("line%d", i)
	}
	src := strings.Join(lines, "\n") + "\n"
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

	for i := 0; i < 400; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
	}

	view := m.View()
	if !strings.Contains(view, "biglist.txt") {
		t.Fatal("expected the tree pane (showing biglist.txt) to remain visible alongside a large open file")
	}
	if got := strings.Count(view, "\n"); got > 24 {
		t.Fatalf("got %d rendered lines, want at most the window height (24) — content must not overflow the terminal", got)
	}
}

// Regression test: a file whose lines are wider than the editor pane must
// not push the tree pane out of view. app/model.go composes each pane by
// applying a Width()+Height() style to the pane's ALREADY-RENDERED
// multi-line block (editorStyle.Render(m.activeEditor().View())) — if any single
// line inside that block is wider than the style's target width, Lip Gloss
// hard-wraps it into multiple physical lines, and since Height() only sets
// a minimum (never truncates), those extra wrapped lines silently overflow
// the pane's box just like the original "too many lines" bug, except
// triggered by line WIDTH instead of line COUNT.
func TestWideLinesDoNotPushTheTreePaneOutOfViewEither(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "wide.go")
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString(fmt.Sprintf("\tresult := someFunction(argumentOne, argumentTwo, argumentThree, argumentFour) // line %d with a trailing comment that makes it long\n", i))
	}
	if err := os.WriteFile(file, []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)

	view := m.View()
	if !strings.Contains(view, "wide.go") {
		t.Fatal("expected the tree pane (showing wide.go) to remain visible alongside a file with wide lines")
	}
	if got := strings.Count(view, "\n"); got > 30 {
		t.Fatalf("got %d rendered lines, want at most the window height (30) — a wide line must not wrap and overflow the terminal", got)
	}
}

func TestClampBlockWidthTruncatesOverlongLinesWithoutWrapping(t *testing.T) {
	block := "short\n" + strings.Repeat("x", 50) + "\nalso short"
	out := clampBlockWidth(block, 20)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d physical lines, want 3 (no wrapping)", len(lines))
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > 20 {
			t.Fatalf("line %d: width %d exceeds cap of 20 (%q)", i, w, l)
		}
	}
}

func TestClampBlockWidthIsANoOpForNonPositiveWidth(t *testing.T) {
	block := "anything\nhere"
	if got := clampBlockWidth(block, 0); got != block {
		t.Fatalf("got %q, want unchanged %q", got, block)
	}
}

func TestTabCyclesThroughAllThreePanesForwardAndBack(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if m.focus != focusTree {
		t.Fatalf("got initial focus %v, want focusTree", m.focus)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.focus != focusEditor {
		t.Fatalf("got focus %v after one Tab, want focusEditor", m.focus)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.focus != focusTerminal {
		t.Fatalf("got focus %v after two Tabs, want focusTerminal", m.focus)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.focus != focusTree {
		t.Fatalf("got focus %v after three Tabs, want focusTree (wrapped)", m.focus)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = updated.(Model)
	if m.focus != focusTerminal {
		t.Fatalf("got focus %v after one Shift+Tab, want focusTerminal (wrapped backward)", m.focus)
	}
}

func TestFocusingTheTerminalStartsItExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> editor
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> terminal
	m = updated.(Model)
	if !m.terminal.Started() {
		t.Fatal("expected focusing the terminal pane to start it")
	}
	if cmd == nil {
		t.Fatal("expected a command (the pty read loop kickoff) when the terminal starts")
	}

	// Tabbing away and back must not restart it.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> tree
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> editor
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> terminal again
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no new start command the second time the terminal gains focus")
	}
}

func TestEditingShortcutsPassThroughToTheTerminalWhenItHasFocus(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // editor -> terminal
	m = updated.(Model)
	if m.focus != focusTerminal {
		t.Fatalf("got focus %v, want focusTerminal", m.focus)
	}

	before := m.recentCommand
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = updated.(Model)
	if m.recentCommand != before {
		t.Fatalf("expected ctrl+c to NOT trigger the Copy command while the terminal has focus, got recentCommand=%q", m.recentCommand)
	}
}

func TestOpenAndQuitStayGlobalEvenWhenTerminalHasFocus(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.focus != focusTerminal {
		t.Fatalf("got focus %v, want focusTerminal", m.focus)
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if cmd == nil {
		t.Fatal("expected ctrl+q to still produce tea.Quit while the terminal has focus")
	}
}

func TestTerminalPaneShowsRealShellOutputThroughTheComposedApp(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a real subprocess; skipped in -short mode")
	}
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.terminal.Close() })
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // tree -> editor
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab}) // editor -> terminal
	m = updated.(Model)
	if m.focus != focusTerminal {
		t.Fatalf("got focus %v, want focusTerminal", m.focus)
	}
	if cmd == nil {
		t.Fatal("expected a read-loop command once the terminal starts")
	}

	// Wait for the shell's startup prompt before typing, so the typed
	// command reaches a live shell rather than being sent before its
	// stdin is being read — same technique as Task 2's package-level
	// integration test.
	msg := cmd()
	out, ok := msg.(terminal.OutputMsg)
	if !ok {
		t.Fatalf("got %T as the first message, want terminal.OutputMsg (the shell prompt)", msg)
	}
	updated, cmd = m.Update(out)
	m = updated.(Model)

	// Type the command through the exact same tea.KeyMsg path a real
	// keystroke takes, one rune at a time, then Enter.
	command := "printf 'cody-app-marker\\n'"
	for _, r := range command {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	deadline := time.Now().Add(3 * time.Second)
	for {
		if cmd == nil {
			t.Fatalf("read loop stopped before the marker appeared; last view:\n%s", m.View())
		}
		msg := cmd()
		out, ok := msg.(terminal.OutputMsg)
		if !ok {
			t.Fatalf("got %T, want terminal.OutputMsg", msg)
		}
		updated, cmd = m.Update(out)
		m = updated.(Model)
		if strings.Contains(m.View(), "cody-app-marker") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for the marker text through the composed app view; last view:\n%s", m.View())
		}
	}
}

func TestSearchingAndCyclingMatchesThroughTheComposedApp(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc add(a, b int) int {\n\treturn a + b\n}\n\nfunc addTwo(x int) int {\n\treturn add(x, 2)\n}\n"
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

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = updated.(Model)
	if m.activeDialog != dialogSearch {
		t.Fatalf("got activeDialog=%v, want dialogSearch", m.activeDialog)
	}

	for _, r := range "add" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	line, _ := m.activeEditor().Cursor()
	if line != 3 {
		t.Fatalf("got cursor line=%d, want 3 (the first \"add\" match, in \"func add(\")", line)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	line, _ = m.activeEditor().Cursor()
	if line != 7 {
		t.Fatalf("got cursor line=%d, want 7 (the second \"add\" match, in \"func addTwo(\")", line)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.activeDialog != dialogNone {
		t.Fatalf("got activeDialog=%v, want dialogNone", m.activeDialog)
	}
}

// Pane geometry for an 80x24 window (see paneLayout): menu bar at y=0, body
// starting at y=1 with height 22 (no dropdown open). Tree occupies x[0,30),
// editor x[30,80) y[1,15), terminal x[30,80) y[15,23).

func TestClickInTreePaneFocusesTreeAndSelectsRow(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"0.go", "1.go", "2.go", "3.go", "4.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(""), 0644); err != nil {
			t.Fatal(err)
		}
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.focus = focusEditor

	// row = y(5) - bodyTop(1) - border(1) = 3 -> "3.go".
	updated, cmd := m.Update(tea.MouseMsg{X: 5, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)
	if m.focus != focusTree {
		t.Fatal("expected clicking the tree pane to focus it")
	}
	if cmd == nil {
		t.Fatal("expected clicking a file row to activate it (open the file)")
	}
	msg := cmd()
	opened, ok := msg.(filetree.FileOpenedMsg)
	if !ok {
		t.Fatalf("got %T, want FileOpenedMsg", msg)
	}
	if filepath.Base(opened.Path) != "3.go" {
		t.Fatalf("got %q, want 3.go (row 3)", filepath.Base(opened.Path))
	}
}

func TestClickInEditorPanePositionsCursor(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.go")
	src := "line0\nline1\nline2\nline3\nline4\n"
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
	m.focus = focusTree

	// x=43 -> relX = 43 - 30(editor x0) - 1(border) = 12 -> col = 12 - editorGutterWidth(7) = 5.
	// y=4  -> relY = 4 - 1(bodyTop) - 1(border) = 2 -> buffer line index 2 ("line2").
	updated, _ = m.Update(tea.MouseMsg{X: 43, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)
	if m.focus != focusEditor {
		t.Fatal("expected clicking the editor pane to focus it")
	}
	line, col := m.activeEditor().Cursor()
	if line != 3 || col != 6 {
		t.Fatalf("got line=%d col=%d, want 3,6 (line2, zero-indexed col 5)", line, col)
	}
}

func TestClickInTerminalPaneFocusesTerminal(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	// y=18 lands inside the terminal rect (y in [15, 23)).
	updated, _ = m.Update(tea.MouseMsg{X: 45, Y: 18, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)
	if m.focus != focusTerminal {
		t.Fatal("expected clicking the terminal pane to focus it")
	}
	if !m.terminal.Started() {
		t.Fatal("expected the terminal to lazily start on first focus")
	}
}

func TestWheelOverTreeScrollsWithoutChangingFocus(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 30; i++ {
		name := fmt.Sprintf("%02d.go", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(""), 0644); err != nil {
			t.Fatal(err)
		}
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.focus = focusEditor

	before := m.tree.View()
	if !strings.Contains(before, "00.go") {
		t.Fatal("setup failed: expected 00.go visible initially")
	}

	// 8 notches * mouseWheelLines(3) = 24 rows, past the ~20-row viewport,
	// forcing the tree to scroll.
	for i := 0; i < 8; i++ {
		updated, _ = m.Update(tea.MouseMsg{X: 5, Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
		m = updated.(Model)
	}
	if m.focus != focusEditor {
		t.Fatal("expected wheel scrolling not to change focus")
	}
	after := m.tree.View()
	if strings.Contains(after, "00.go") {
		t.Fatal("expected 00.go to have scrolled out of view")
	}
}

func TestOpeningTwoDifferentFilesCreatesTwoTabs(t *testing.T) {
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
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: fileA})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileB})
	m = updated.(Model)

	if len(m.tabs) != 2 {
		t.Fatalf("got %d tabs, want 2", len(m.tabs))
	}
	if m.activeTab != 1 {
		t.Fatalf("got activeTab=%d, want 1 (the most recently opened)", m.activeTab)
	}
}

func TestOpeningAnAlreadyOpenFileSwitchesInsteadOfDuplicating(t *testing.T) {
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
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: fileA})
	m = updated.(Model)
	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileB})
	m = updated.(Model)
	// Move the cursor in fileB's tab so we can confirm re-opening fileA and
	// coming back to fileB preserves this, rather than reloading it.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	lineBeforeReopen, _ := m.activeEditor().Cursor()

	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileA})
	m = updated.(Model)
	if len(m.tabs) != 2 {
		t.Fatalf("got %d tabs, want 2 (re-opening fileA must not duplicate it)", len(m.tabs))
	}
	if m.activeTab != 0 {
		t.Fatalf("got activeTab=%d, want 0 (fileA's existing tab)", m.activeTab)
	}

	updated, _ = m.Update(filetree.FileOpenedMsg{Path: fileB})
	m = updated.(Model)
	if m.activeTab != 1 {
		t.Fatalf("got activeTab=%d, want 1 (back to fileB's existing tab)", m.activeTab)
	}
	lineAfterReturn, _ := m.activeEditor().Cursor()
	if lineAfterReturn != lineBeforeReopen {
		t.Fatalf("got cursor line=%d, want %d — switching back to an already-open tab must preserve its state, not reload it", lineAfterReturn, lineBeforeReopen)
	}
}
