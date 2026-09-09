package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"cody/internal/highlight"
)

// ansiStrip removes ANSI CSI escape sequences from s, returning plain text.
// Use this before content-presence checks in tests so that cursor and
// highlighting ANSI codes don't obscure the underlying text.
func ansiStrip(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !(s[j] >= 0x40 && s[j] <= 0x7E) {
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func setupEditor(t *testing.T, content string) Model {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestTypingInsertsRunes(t *testing.T) {
	m := setupEditor(t, "")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hi")})
	if m.buf.Lines[0] != "hi" {
		t.Fatalf("got %q, want %q", m.buf.Lines[0], "hi")
	}
	line, col := m.Cursor()
	if line != 1 || col != 3 {
		t.Fatalf("got line=%d col=%d, want 1,3", line, col)
	}
}

func TestEnterSplitsLine(t *testing.T) {
	m := setupEditor(t, "helloworld")
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.buf.Lines) != 2 || m.buf.Lines[0] != "hello" || m.buf.Lines[1] != "world" {
		t.Fatalf("got %v", m.buf.Lines)
	}
}

func TestCtrlSSavesAndEmitsMessage(t *testing.T) {
	m := setupEditor(t, "content")
	path := m.buf.Path
	m.buf.Lines[0] = "changed"
	m.buf.Dirty = true
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("expected a command")
	}
	msg, ok := cmd().(CommandExecutedMsg)
	if !ok {
		t.Fatalf("got %T, want CommandExecutedMsg", msg)
	}
	if msg.Description != "Saved "+filepath.Base(path) {
		t.Fatalf("got %q", msg.Description)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "changed" {
		t.Fatalf("got %q", string(data))
	}
}

func TestFiletype(t *testing.T) {
	m := setupEditor(t, "")
	if m.Filetype() != "go" {
		t.Fatalf("got %q, want go", m.Filetype())
	}
}

func TestAltModifiedKeyDoesNotInsert(t *testing.T) {
	m := setupEditor(t, "")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a"), Alt: true})
	if m.buf.Lines[0] != "" {
		t.Fatalf("got %q, want empty — alt+a should not insert", m.buf.Lines[0])
	}
}

func TestMultiRuneBurstWithNewlineSplitsLines(t *testing.T) {
	m := setupEditor(t, "")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ab\ncd")})
	if len(m.buf.Lines) != 2 || m.buf.Lines[0] != "ab" || m.buf.Lines[1] != "cd" {
		t.Fatalf("got %v, want [\"ab\" \"cd\"]", m.buf.Lines)
	}
}

func TestSpaceBarInsertsSpace(t *testing.T) {
	m := setupEditor(t, "ab")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	if m.buf.Lines[0] != "a b" {
		t.Fatalf("got %q, want %q", m.buf.Lines[0], "a b")
	}
	_, col := m.Cursor()
	if col != 3 {
		t.Fatalf("got col=%d, want 3", col)
	}
}

func TestShiftArrowsExtendSelectionAndCutRemovesIt(t *testing.T) {
	m := setupEditor(t, "hello world")
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	}
	for i := 0; i < 6; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	}
	desc := m.Cut()
	if desc != "Cut selection" {
		t.Fatalf("got %q", desc)
	}
	if m.buf.Lines[0] != "hello" {
		t.Fatalf("got %q, want %q", m.buf.Lines[0], "hello")
	}
	if m.clipboard != " world" {
		t.Fatalf("got clipboard=%q", m.clipboard)
	}
}

func TestPlainArrowCollapsesSelection(t *testing.T) {
	m := setupEditor(t, "hello")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if _, _, _, _, ok := m.selectionRange(); ok {
		t.Fatal("expected selection to be cleared by a plain arrow key")
	}
}

func TestCutWithNoSelectionCutsWholeLine(t *testing.T) {
	m := setupEditor(t, "first\nsecond")
	desc := m.Cut()
	if desc != "Cut line" {
		t.Fatalf("got %q", desc)
	}
	if len(m.buf.Lines) != 1 || m.buf.Lines[0] != "second" {
		t.Fatalf("got %v", m.buf.Lines)
	}
	if m.clipboard != "first\n" {
		t.Fatalf("got clipboard=%q", m.clipboard)
	}
}

func TestCopyWithNoSelectionDoesNotMutateBuffer(t *testing.T) {
	m := setupEditor(t, "only line")
	desc := m.Copy()
	if desc != "Copied line" {
		t.Fatalf("got %q", desc)
	}
	if m.buf.Lines[0] != "only line" {
		t.Fatal("copy must not mutate the buffer")
	}
	if m.clipboard != "only line\n" {
		t.Fatalf("got clipboard=%q", m.clipboard)
	}
}

func TestPasteInsertsClipboardAtCursor(t *testing.T) {
	m := setupEditor(t, "ac")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m.clipboard = "b"
	desc := m.Paste()
	if desc != "Pasted" {
		t.Fatalf("got %q", desc)
	}
	if m.buf.Lines[0] != "abc" {
		t.Fatalf("got %q", m.buf.Lines[0])
	}
}

func TestPasteMultiLineClipboardSplitsLines(t *testing.T) {
	m := setupEditor(t, "")
	m.clipboard = "ab\ncd"
	m.Paste()
	if len(m.buf.Lines) != 2 || m.buf.Lines[0] != "ab" || m.buf.Lines[1] != "cd" {
		t.Fatalf("got %v", m.buf.Lines)
	}
}

func TestUndoRestoresPreviousLineContent(t *testing.T) {
	m := setupEditor(t, "hello")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	if m.buf.Lines[0] != "Xhello" {
		t.Fatalf("setup failed, got %q", m.buf.Lines[0])
	}
	desc := m.undo()
	if desc != "Undo" {
		t.Fatalf("got %q", desc)
	}
	if m.buf.Lines[0] != "hello" {
		t.Fatalf("got %q, want %q after undo", m.buf.Lines[0], "hello")
	}
}

func TestUndoBackToSavedContentClearsDirty(t *testing.T) {
	m := setupEditor(t, "hello")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	if !m.HasUnsavedChanges() {
		t.Fatal("expected the buffer to be dirty after an edit")
	}
	m.undo()
	if m.HasUnsavedChanges() {
		t.Fatal("expected undoing back to the saved content to clear the dirty flag")
	}
}

func TestRedoBackToSavedContentClearsDirty(t *testing.T) {
	m := setupEditor(t, "hello")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	m.undo()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")})
	m.undo()
	m.redo() // reapplies "Y" -> dirty again
	if !m.HasUnsavedChanges() {
		t.Fatal("expected redoing away from the saved content to leave the buffer dirty")
	}
	m.undo() // back to the saved "hello"
	if m.HasUnsavedChanges() {
		t.Fatal("expected undoing back to the saved content to clear the dirty flag")
	}
}

func TestUndoAfterSaveNoLongerClearsDirty(t *testing.T) {
	m := setupEditor(t, "hello")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	if err := m.buf.Save(); err != nil {
		t.Fatal(err)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")})
	m.undo() // back to "Xhello", which is now the saved content
	if m.HasUnsavedChanges() {
		t.Fatal("expected undoing back to the post-save content to clear dirty")
	}
}

func TestRedoReappliesUndoneEdit(t *testing.T) {
	m := setupEditor(t, "hello")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	m.undo()
	desc := m.redo()
	if desc != "Redo" {
		t.Fatalf("got %q", desc)
	}
	if m.buf.Lines[0] != "Xhello" {
		t.Fatalf("got %q, want %q after redo", m.buf.Lines[0], "Xhello")
	}
}

func TestUndoWithEmptyStackReportsNothingToUndo(t *testing.T) {
	m := setupEditor(t, "hello")
	if desc := m.undo(); desc != "Nothing to undo" {
		t.Fatalf("got %q", desc)
	}
}

func TestNewEditAfterUndoClearsRedoStack(t *testing.T) {
	m := setupEditor(t, "hello")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	m.undo()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")})
	if desc := m.redo(); desc != "Nothing to redo" {
		t.Fatalf("got %q, want redo stack cleared by the new edit", desc)
	}
}

func TestCtrlXCtrlCCtrlVKeysDispatchThroughUpdate(t *testing.T) {
	m := setupEditor(t, "hello")
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected a command from ctrl+c")
	}
	msg, ok := cmd().(CommandExecutedMsg)
	if !ok || msg.Description != "Copied line" {
		t.Fatalf("got %+v, ok=%v", msg, ok)
	}
}

func TestBackspaceAfterSelectionThenCopyDoesNotPanic(t *testing.T) {
	m := setupEditor(t, "a\nb\nc")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftUp})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.buf.Lines[0] != "ab" || m.buf.Lines[1] != "c" || len(m.buf.Lines) != 2 {
		t.Fatalf("setup failed, got %v", m.buf.Lines)
	}
	if _, _, _, _, ok := m.selectionRange(); ok {
		t.Fatal("expected backspace to clear the stale selection")
	}
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected a command from ctrl+c")
	}
	msg, ok := cmd().(CommandExecutedMsg)
	if !ok || msg.Description != "Copied line" {
		t.Fatalf("got %+v, ok=%v", msg, ok)
	}
	if m.clipboard != "ab\n" {
		t.Fatalf("got clipboard=%q, want %q", m.clipboard, "ab\n")
	}
}

func TestLoadFileResetsSelection(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.txt")
	if err := os.WriteFile(big, []byte("l0\nl1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9"), 0644); err != nil {
		t.Fatal(err)
	}
	small := filepath.Join(dir, "small.txt")
	if err := os.WriteFile(small, []byte("a\nb"), 0644); err != nil {
		t.Fatal(err)
	}

	m := New()
	m, err := m.LoadFile(big)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftDown})

	m, err = m.LoadFile(small)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, ok := m.selectionRange(); ok {
		t.Fatal("expected LoadFile to clear any selection from the previous buffer")
	}
	// Must not panic, and must operate on the new (small) buffer.
	desc := m.Copy()
	if desc != "Copied line" {
		t.Fatalf("got %q", desc)
	}
}

func TestLoadFileResetsUndoRedoStacks(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(a, []byte("AAA"), 0644); err != nil {
		t.Fatal(err)
	}
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(b, []byte("BBB"), 0644); err != nil {
		t.Fatal(err)
	}

	m := New()
	m, err := m.LoadFile(a)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})

	m, err = m.LoadFile(b)
	if err != nil {
		t.Fatal(err)
	}
	if desc := m.undo(); desc != "Nothing to undo" {
		t.Fatalf("got %q, want the undo stack cleared by LoadFile", desc)
	}
	if m.buf.Lines[0] != "BBB" {
		t.Fatalf("got %q, want b.txt's original content untouched", m.buf.Lines[0])
	}
}

func TestCommandKeysReportNoFileOpenWithoutABuffer(t *testing.T) {
	m := New()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("expected a status message even with no buffer loaded")
	}
	msg := cmd().(CommandExecutedMsg)
	if msg.Description != "No file open" {
		t.Fatalf("got %q", msg.Description)
	}
}

func TestLoadFileHighlightsAGoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.highlightSpans) == 0 {
		t.Fatal("expected highlightSpans to be populated for a .go file")
	}
	view := m.View()
	if view == "" {
		t.Fatal("expected a non-empty rendered view")
	}
}

func TestLoadFileUnsupportedExtensionHasNoHighlighter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")
	if err := os.WriteFile(path, []byte("just bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.highlighter != nil {
		t.Fatal("expected no highlighter for an unsupported extension")
	}
	if view := m.View(); view == "" {
		t.Fatal("expected plain-text rendering to still work with no highlighter")
	}
}

func TestLoadFileResetsHighlightStateAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	goPath := filepath.Join(dir, "a.go")
	if err := os.WriteFile(goPath, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	txtPath := filepath.Join(dir, "b.bin")
	if err := os.WriteFile(txtPath, []byte("plain"), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(goPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.highlightSpans) == 0 {
		t.Fatal("setup failed: expected highlight spans after loading a .go file")
	}
	m, err = m.LoadFile(txtPath)
	if err != nil {
		t.Fatal(err)
	}
	if m.highlighter != nil || len(m.highlightSpans) != 0 {
		t.Fatal("expected highlighter and highlightSpans to be cleared after switching to an unsupported file")
	}
}

func TestTypingSchedulesARehighlightCommand(t *testing.T) {
	m := setupEditor(t, "")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if cmd == nil {
		t.Fatal("expected typing to schedule a rehighlight command")
	}
	msg := cmd()
	if _, ok := msg.(RehighlightMsg); !ok {
		t.Fatalf("got %T, want RehighlightMsg", msg)
	}
}

func TestRehighlightMsgWithCurrentGenerationReparses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.highlightSpans) != 0 {
		t.Fatal("setup failed: expected no highlight spans for an empty file")
	}
	// Typing valid Go into the empty buffer only shows up in highlightSpans
	// if a genuine reparse runs — the stale (empty-file) spans are otherwise
	// still what's cached, so this distinguishes a real reparse from a no-op.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("package main")})
	if m.buf.Lines[0] != "package main" {
		t.Fatalf("setup failed, got %q", m.buf.Lines[0])
	}
	m, _ = m.Update(RehighlightMsg{generation: m.highlightGeneration})
	if len(m.highlightSpans) == 0 {
		t.Fatal("expected highlightSpans to be repopulated after a matching-generation RehighlightMsg")
	}
}

func TestStaleRehighlightMsgIsIgnored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	spansBeforeEdit := len(m.highlightSpans[0])
	if spansBeforeEdit == 0 {
		t.Fatal("setup failed: expected at least one highlight span on line 0 before the edit")
	}

	// Prepend "X" to the line, corrupting the "package" keyword token —
	// a genuine reparse of this new content would find zero keyword
	// matches on this line, letting us detect whether a stale message
	// incorrectly triggered one.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	staleGeneration := m.highlightGeneration - 1

	m, _ = m.Update(RehighlightMsg{generation: staleGeneration})

	if len(m.highlightSpans[0]) != spansBeforeEdit {
		t.Fatalf("got %d spans on line 0, want %d (unchanged from before the edit) — a stale-generation message must not trigger a reparse", len(m.highlightSpans[0]), spansBeforeEdit)
	}
}

func TestCtrlXThroughUpdateRefreshesHighlightSpans(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc add() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Move to line 2 (the function declaration) and cut it.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if len(m.highlightSpans[2]) == 0 {
		t.Fatal("setup failed: expected a highlight span on the function-declaration line before cutting it")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if len(m.buf.Lines) != 3 {
		t.Fatalf("setup failed: expected the line to be removed, got %q", m.buf.Lines)
	}
	if _, ok := m.highlightSpans[2]; ok {
		t.Fatal("expected highlightSpans to no longer have an entry for the removed line after ctrl+x")
	}
}

func TestCtrlZThroughUpdateRefreshesHighlightSpans(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc add() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Move to line 2 (the function declaration), cut it (removing its
	// highlight spans — verified by TestCtrlXThroughUpdateRefreshesHighlightSpans),
	// then undo and confirm ctrl+z's synchronous rehighlight restores them.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if _, ok := m.highlightSpans[2]; ok {
		t.Fatal("setup failed: expected no highlightSpans entry for the removed line after ctrl+x")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})
	if len(m.buf.Lines) != 4 || m.buf.Lines[2] != "func add() {}" {
		t.Fatalf("setup failed: expected undo to restore the removed line, got %q", m.buf.Lines)
	}
	if len(m.highlightSpans[2]) == 0 {
		t.Fatal("expected highlightSpans to be refreshed synchronously after ctrl+z, restoring the function-declaration line's spans")
	}
}

// lipgloss strips all ANSI styling under the default non-TTY test color
// profile, which is exactly the blind spot that let the tab-conversion bug
// (item 1) and the selection/syntax-color precedence go unverified by any
// test in this phase: every prior test here only checked that spans exist
// or that the view is non-empty, never that the rendered text actually
// carries styling. These three tests force TrueColor to make that styling
// visible and assert on it directly.

func TestStyledLineActuallyContainsAnsiStyling(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	view := m.View()
	if !strings.Contains(view, "\x1b[") {
		t.Fatal("expected the rendered view to contain ANSI escape codes when a highlighter is active")
	}
}

func TestSelectedLineShowsReverseVideoNotSyntaxColor(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	view := m.View()
	if !strings.Contains(view, "\x1b[7m") {
		t.Fatal("expected the selected line to contain a reverse-video escape code")
	}
}

func TestMarkdownFileHighlightsThroughEditorView(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("# Title\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	view := m.View()
	if !strings.Contains(view, "\x1b[") {
		t.Fatal("expected the rendered Markdown view to contain ANSI escape codes for the heading")
	}
}

func TestLoadFileDetectsFoldsForAGoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc add(a int, b int) int {\n\treturn a + 42\n}\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range m.folds {
		if f.StartLine == 2 && f.EndLine == 4 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a fold spanning lines 2-4, got %v", m.folds)
	}
}

func TestCtrlKTogglesFoldAtCursor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc add(a int, b int) int {\n\treturn a + 42\n}\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.cursorLine != 2 {
		t.Fatalf("setup failed: expected cursor on line 2, got %d", m.cursorLine)
	}

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if cmd == nil {
		t.Fatal("expected ctrl+k to produce a status message")
	}
	if !m.foldedStartLines[2] {
		t.Fatal("expected line 2's fold to be collapsed after ctrl+k")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if m.foldedStartLines[2] {
		t.Fatal("expected a second ctrl+k to re-expand the fold")
	}
}

func TestCtrlKWithNothingFoldableAtCursorIsANoOp(t *testing.T) {
	m := setupEditor(t, "package main\n")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if cmd == nil {
		t.Fatal("expected a status message even when nothing is foldable")
	}
	msg := cmd().(CommandExecutedMsg)
	if msg.Description != "Nothing to fold here" {
		t.Fatalf("got %q", msg.Description)
	}
}

func TestFoldedRegionIsHiddenFromViewAndNavigation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc add(a int, b int) int {\n\treturn a + 42\n}\n\nvar x = 1\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if !m.foldedStartLines[2] {
		t.Fatal("setup failed: expected line 2's fold to be collapsed")
	}

	view := m.View()
	if strings.Contains(view, "return a + 42") {
		t.Fatal("expected the folded line's content to be hidden from View()")
	}

	// The fold spans lines 2-4 (the "{" line through the "}" line), so its
	// hidden interior is lines 3-4; line 5 (the blank separator line before
	// "var x = 1") is outside the fold and stays visible. Moving down from
	// the fold's start line must skip straight past the hidden interior to
	// that next visible line, not land on a hidden line.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursorLine != 5 {
		t.Fatalf("got cursorLine=%d, want 5 (skipping the folded lines 3-4)", m.cursorLine)
	}

	// One more down-press should reach "var x = 1" (line 6).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursorLine != 6 {
		t.Fatalf("got cursorLine=%d, want 6", m.cursorLine)
	}
}

func TestEditingClearsAllFoldState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc add(a int, b int) int {\n\treturn a + 42\n}\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if !m.foldedStartLines[2] {
		t.Fatal("setup failed: expected line 2's fold to be collapsed")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	if m.foldedStartLines[2] {
		t.Fatal("expected an edit to clear fold-toggle state")
	}
}

func setupLargeFile(t *testing.T, numLines int) Model {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	lines := make([]string, numLines)
	for i := range lines {
		lines[i] = fmt.Sprintf("line%d", i)
	}
	src := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestViewIsUnboundedWhenHeightIsNeverSet(t *testing.T) {
	m := setupLargeFile(t, 20)
	view := m.View()
	// setupLargeFile's source ends with "\n", so the buffer has 21 lines
	// (20 content + 1 trailing empty). View() trims the trailing newline, so
	// strings.Count gives 20 newline separators = 21 lines.
	if got := strings.Count(view, "\n"); got != 20 {
		t.Fatalf("got %d newline separators, want 20 (all 21 buffer lines, no trailing newline)", got)
	}
}

func TestViewClipsRenderedLinesToTheSetHeight(t *testing.T) {
	m := setupLargeFile(t, 20)
	m = m.SetSize(80, 5)

	view := m.View()
	// 5 lines rendered, no trailing newline → 4 newline separators.
	if got := strings.Count(view, "\n"); got != 4 {
		t.Fatalf("got %d newline separators, want 4 (5 lines, no trailing newline)", got)
	}
	plain := ansiStrip(view)
	if !strings.Contains(plain, "line0") {
		t.Fatal("expected the first line to be visible before any scrolling")
	}
	if strings.Contains(plain, "line5") {
		t.Fatal("expected line5 to be outside the initial 5-line viewport")
	}
}

func TestMovingCursorPastViewportScrollsTheEditor(t *testing.T) {
	m := setupLargeFile(t, 20)
	m = m.SetSize(80, 5)

	for i := 0; i < 7; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	view := m.View()
	plain := ansiStrip(view)
	if strings.Contains(plain, "line0") {
		t.Fatal("expected line0 to have scrolled out of view")
	}
	if !strings.Contains(plain, "line7") {
		t.Fatal("expected line7 (the cursor's line) to be visible")
	}
}

func TestViewIncludesAScrollbarWhenContentOverflowsHeight(t *testing.T) {
	m := setupLargeFile(t, 20)
	m = m.SetSize(80, 5)

	view := m.View()
	if !strings.ContainsRune(view, '█') && !strings.ContainsRune(view, '│') {
		t.Fatal("expected a scrollbar thumb or track when content overflows the viewport")
	}
}

func TestViewHasNoScrollbarMarksWhenContentFitsTheHeight(t *testing.T) {
	m := setupLargeFile(t, 3)
	m = m.SetSize(80, 10)

	view := m.View()
	if strings.ContainsRune(view, '█') || strings.ContainsRune(view, '│') {
		t.Fatal("expected no scrollbar marks when content fits within the viewport")
	}
}

// Regression test: the scrollbar rune must land in the same column on every
// rendered row, regardless of how long that row's own text is. Appending it
// directly after variable-length text (the original bug) makes it drift to
// a different column per line, which reads as a stray character attached to
// the text rather than a scrollbar.
func TestScrollbarColumnStaysAlignedAcrossLinesOfDifferentLengths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "varied.txt")
	src := "a\nbb\nccc\ndddd\neeeee\nffffff\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
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
		// Strip ANSI codes before counting rune positions so that cursor and
		// current-line background sequences don't inflate the column index.
		plain := ansiStrip(l)
		idx := -1
		for i, r := range []rune(plain) {
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
func TestPaddingALongLineDoesNotWrapItIntoMultiplePhysicalLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "long.txt")
	long := strings.Repeat("x", 100)
	src := long + "\nshort\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m = m.SetSize(40, 3)

	view := m.View()
	// 3 lines rendered, no trailing newline → 2 newline separators.
	if got := strings.Count(view, "\n"); got != 2 {
		t.Fatalf("got %d newline separators, want 2 (3 lines, no trailing newline) — a row wider than the pane must not wrap", got)
	}
	if !strings.Contains(ansiStrip(view), long) {
		t.Fatal("expected the long line's full content to still be present, unwrapped")
	}
}

func TestSetSearchQueryFindsAllCaseInsensitiveMatchesAndJumpsToNearest(t *testing.T) {
	m := setupEditor(t, "foo\nBAR foo\nfoo bar\n")
	m = m.StartSearch()
	m, status := m.SetSearchQuery("foo")
	if status != "Match 1 of 3" {
		t.Fatalf("got status %q, want %q", status, "Match 1 of 3")
	}
	line, col := m.Cursor()
	if line != 1 || col != 1 {
		t.Fatalf("got cursor line=%d col=%d, want 1,1 (first match)", line, col)
	}
}

func TestSetSearchQueryJumpsToNearestMatchAtOrAfterCursorNotAlwaysTheFirst(t *testing.T) {
	m := setupEditor(t, "foo\nfoo\nfoo\n")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // cursor now on line 2 (0-indexed)
	m = m.StartSearch()
	m, status := m.SetSearchQuery("foo")
	if status != "Match 3 of 3" {
		t.Fatalf("got status %q, want %q (the match on the cursor's own line)", status, "Match 3 of 3")
	}
	line, _ := m.Cursor()
	if line != 3 {
		t.Fatalf("got cursor line=%d, want 3 (1-indexed line 3 == 0-indexed line 2)", line)
	}
}

func TestSetSearchQueryWithNoMatchesReportsNoMatches(t *testing.T) {
	m := setupEditor(t, "hello world\n")
	m = m.StartSearch()
	m, status := m.SetSearchQuery("xyz")
	if status != "No matches" {
		t.Fatalf("got status %q, want %q", status, "No matches")
	}
}

func TestSetSearchQueryWithEmptyQueryReportsEmptyStatus(t *testing.T) {
	m := setupEditor(t, "hello world\n")
	m = m.StartSearch()
	m, status := m.SetSearchQuery("")
	if status != "" {
		t.Fatalf("got status %q, want empty", status)
	}
}

func TestFindNextAndFindPrevCycleThroughMatchesAndWrap(t *testing.T) {
	m := setupEditor(t, "foo\nfoo\nfoo\n")
	m = m.StartSearch()
	m, _ = m.SetSearchQuery("foo")

	m, status := m.FindNext()
	if status != "Match 2 of 3" {
		t.Fatalf("got status %q, want %q", status, "Match 2 of 3")
	}
	m, status = m.FindNext()
	if status != "Match 3 of 3" {
		t.Fatalf("got status %q, want %q", status, "Match 3 of 3")
	}
	m, status = m.FindNext()
	if status != "Match 1 of 3" {
		t.Fatalf("got status %q, want %q (wraps forward)", status, "Match 1 of 3")
	}
	m, status = m.FindPrev()
	if status != "Match 3 of 3" {
		t.Fatalf("got status %q, want %q (wraps backward)", status, "Match 3 of 3")
	}
}

func TestClearSearchRemovesAllMatchState(t *testing.T) {
	// Force a real color profile — the default test profile is Ascii,
	// which never emits ANSI codes at all, so checking their absence
	// would pass trivially regardless of whether ClearSearch did anything.
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := setupEditor(t, "foo\n")
	m = m.StartSearch()
	m, _ = m.SetSearchQuery("foo")
	if !strings.Contains(m.View(), "\x1b[7m") {
		t.Fatal("setup failed: expected a reverse-video match highlight before clearing")
	}

	m = m.ClearSearch()

	// The cursor still emits reverse-video (\x1b[7m) — check specifically for
	// the search-match yellow background (color 220) which is absent after clear.
	if strings.Contains(m.View(), "\x1b[48;5;220m") {
		t.Fatal("expected no search-match highlight (yellow background) after ClearSearch")
	}
	if _, status := m.FindNext(); status != "No matches" {
		t.Fatalf("got status %q, want %q after clearing", status, "No matches")
	}
}

func TestCurrentMatchIsHighlightedInView(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	m := setupEditor(t, "hello world\n")
	m = m.StartSearch()
	m, _ = m.SetSearchQuery("world")

	view := m.View()
	if !strings.Contains(view, "\x1b[7m") {
		t.Fatalf("expected a reverse-video escape code highlighting the match, got %q", view)
	}
}

func TestTypingClearsSearchState(t *testing.T) {
	m := setupEditor(t, "foo\n")
	m = m.StartSearch()
	m, _ = m.SetSearchQuery("foo")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})

	if _, status := m.FindNext(); status != "No matches" {
		t.Fatalf("got status %q, want %q — typing must clear search state", status, "No matches")
	}
}

func TestLoadFileResetsSearchState(t *testing.T) {
	m := setupEditor(t, "foo\n")
	m = m.StartSearch()
	m, _ = m.SetSearchQuery("foo")

	dir := t.TempDir()
	path := filepath.Join(dir, "other.go")
	if err := os.WriteFile(path, []byte("bar\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, status := m.FindNext(); status != "No matches" {
		t.Fatalf("got status %q, want %q after LoadFile", status, "No matches")
	}
}

func TestFindNextUnfoldsFoldedMatch(t *testing.T) {
	m := New()
	m.buf = &Buffer{Lines: []string{
		"func hello() {",
		"    target here",
		"    more stuff",
		"}",
		"func other() {}",
	}}
	m.folds = []highlight.LineRange{{StartLine: 0, EndLine: 3}}
	m.foldedStartLines = map[int]bool{0: true}

	if !m.isLineHidden(1) {
		t.Fatal("precondition: line 1 should be hidden before search")
	}

	m = m.StartSearch()
	var status string
	m, status = m.SetSearchQuery("target")
	if status == "No matches" {
		t.Fatalf("expected a match, got %q", status)
	}

	m, _ = m.FindNext()

	if m.cursorLine != 1 {
		t.Errorf("cursorLine = %d, want 1", m.cursorLine)
	}
	if m.isLineHidden(m.cursorLine) {
		t.Error("cursor is still on a hidden line after FindNext — fold was not opened")
	}
	if m.foldedStartLines[0] {
		t.Error("fold starting at line 0 should be unfolded after FindNext jumped into it")
	}
}

// foldMarkOnRow extracts the fold-mark gutter column (see foldMarkWidth)
// from the row at the given visible-row index, stripping any ANSI codes
// first — both the cursor and syntax highlighting can add them.
func foldMarkOnRow(view string, row int) string {
	lines := strings.Split(view, "\n")
	plain := ansiStrip(lines[row])
	runes := []rune(plain)
	return string(runes[0:foldMarkWidth])
}

func TestFoldGutterShowsExpandedAndCollapsedGlyphs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc add(a int, b int) int {\n\treturn a + 42\n}\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m = m.SetSize(60, 10)

	// Row 2 is line2 ("func add(...) int {"), the fold's start line — always
	// visible regardless of fold state, since folding only hides the lines
	// strictly after the start line.
	if got := foldMarkOnRow(m.View(), 2); got != "v " {
		t.Fatalf("got fold mark %q, want \"v \" (expanded) before folding", got)
	}

	for i := 0; i < 2; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})

	if got := foldMarkOnRow(m.View(), 2); got != "> " {
		t.Fatalf("got fold mark %q, want \"> \" (collapsed) after folding", got)
	}
}

func TestClickOnFoldGutterTogglesFold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc add(a int, b int) int {\n\treturn a + 42\n}\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m = m.SetSize(60, 10)

	// Row 2 is line2, the fold's start line. x=0 lands on the fold-mark
	// column, which now starts at the very left of the gutter.
	m, cmd := m.HandleClick(0, 2)
	if !m.foldedStartLines[2] {
		t.Fatal("expected clicking the fold gutter to collapse the fold")
	}
	if cmd == nil {
		t.Fatal("expected a status message, matching ctrl+k's own feedback")
	}
	msg := cmd().(CommandExecutedMsg)
	if msg.Description != "Folded" {
		t.Fatalf("got %q, want %q", msg.Description, "Folded")
	}

	m, _ = m.HandleClick(0, 2)
	if m.foldedStartLines[2] {
		t.Fatal("expected a second click on the fold gutter to re-expand it")
	}
}

// Regression test: a syntax-highlighted token's own lipgloss.Render() ends
// with a full reset (\x1b[0m), which — if nothing re-applies the current
// line's background afterward — cuts the highlight short right after the
// first coloured token instead of covering the full (padded) row.
func TestCurrentLineBackgroundSurvivesInlineSyntaxHighlightReset(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m = m.SetSize(60, 10)

	view := m.View()
	line0 := strings.Split(view, "\n")[0]

	resetIdx := strings.Index(line0, "\x1b[0m")
	if resetIdx < 0 {
		t.Fatal("setup failed: expected a highlighted token with a reset on the cursor's line")
	}
	after := line0[resetIdx+len("\x1b[0m"):]
	if !strings.HasPrefix(after, "\x1b[48;5;237m") {
		t.Fatalf("expected the current-line background to be re-applied immediately after the reset, got %q", after)
	}
	if !strings.HasSuffix(line0, "\x1b[0m") {
		t.Fatal("expected the row (including its trailing padding) to end with a final reset")
	}
}

func TestClickOnFoldGutterOfNonFoldableLineMovesCursorInstead(t *testing.T) {
	m := setupEditor(t, "hello\nworld\n")
	m = m.SetSize(40, 10)

	m, cmd := m.HandleClick(0, 1)
	if cmd != nil {
		t.Fatal("expected an ordinary cursor-placing click, not a fold toggle")
	}
	line, _ := m.Cursor()
	if line != 2 {
		t.Fatalf("got line=%d, want 2 (row 1's buffer line)", line)
	}
}

func TestHandleClickPositionsCursorInEditor(t *testing.T) {
	m := setupEditor(t, "line0\nline1\nline2\n")
	m = m.SetSize(40, 10)

	// x = editorGutterWidth + 2 lands on column 2 of the clicked line.
	m, _ = m.HandleClick(editorGutterWidth+2, 1)

	line, col := m.Cursor()
	if line != 2 || col != 3 {
		t.Fatalf("got line=%d col=%d, want 2,3", line, col)
	}
}

func TestScrollLinesMovesCursorDownThenUp(t *testing.T) {
	m := setupEditor(t, "a\nb\nc\nd\ne\n")
	m = m.SetSize(40, 10)

	m = m.ScrollLines(3)
	if line, _ := m.Cursor(); line != 4 {
		t.Fatalf("got line=%d, want 4 after scrolling down 3", line)
	}

	m = m.ScrollLines(-2)
	if line, _ := m.Cursor(); line != 2 {
		t.Fatalf("got line=%d, want 2 after scrolling up 2", line)
	}
}

func TestHasUnsavedChangesFalseOnFreshLoad(t *testing.T) {
	m := setupEditor(t, "hello\n")
	if m.HasUnsavedChanges() {
		t.Fatal("expected a freshly loaded file to have no unsaved changes")
	}
}

func TestHasUnsavedChangesTrueAfterEdit(t *testing.T) {
	m := setupEditor(t, "hello\n")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if !m.HasUnsavedChanges() {
		t.Fatal("expected an edit to mark the buffer as having unsaved changes")
	}
}

func TestHasUnsavedChangesFalseAfterSave(t *testing.T) {
	m := setupEditor(t, "hello\n")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.HasUnsavedChanges() {
		t.Fatal("expected saving to clear unsaved changes")
	}
}

func TestHasUnsavedChangesFalseWithNoBuffer(t *testing.T) {
	m := New()
	if m.HasUnsavedChanges() {
		t.Fatal("expected no unsaved changes before any file is loaded")
	}
}

func TestNewBlankBufferStartsEmptyAndUntitled(t *testing.T) {
	m := New().NewBlankBuffer()
	if !m.HasBuffer() {
		t.Fatal("expected a buffer to be loaded")
	}
	if !m.IsUntitled() {
		t.Fatal("expected a freshly created blank buffer to be untitled")
	}
	if m.HasUnsavedChanges() {
		t.Fatal("expected a freshly created blank buffer to have no unsaved changes yet")
	}
}

func TestIsUntitledFalseAfterLoadingARealFile(t *testing.T) {
	m := setupEditor(t, "hello\n")
	if m.IsUntitled() {
		t.Fatal("expected a file loaded via LoadFile to not be untitled")
	}
}

func TestIsUntitledFalseWithNoBuffer(t *testing.T) {
	m := New()
	if m.IsUntitled() {
		t.Fatal("expected no buffer loaded to not be untitled")
	}
}

func TestSaveAsWritesContentAttachesPathAndClearsDirty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.go")
	m := New().NewBlankBuffer()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("package main")})

	m, err := m.SaveAs(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.IsUntitled() {
		t.Fatal("expected SaveAs to clear untitled status")
	}
	if m.HasUnsavedChanges() {
		t.Fatal("expected SaveAs to clear dirty status")
	}
	if m.Filetype() != "go" {
		t.Fatalf("got filetype %q, want %q", m.Filetype(), "go")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "package main" {
		t.Fatalf("got %q, want %q", string(data), "package main")
	}
}

func TestSaveAsPicksUpSyntaxHighlighting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.go")
	m := New().NewBlankBuffer()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("package main")})
	m, err := m.SaveAs(path)
	if err != nil {
		t.Fatal(err)
	}
	m = m.SetSize(60, 10)
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	if !strings.Contains(m.View(), "\x1b[") {
		t.Fatal("expected syntax highlighting to be active after SaveAs picks up the .go extension")
	}
}

func TestSaveAsOnWriteFailureLeavesBufferUntitled(t *testing.T) {
	m := New().NewBlankBuffer()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("content")})

	badPath := filepath.Join(t.TempDir(), "nonexistent-dir", "file.go")
	_, err := m.SaveAs(badPath)
	if err == nil {
		t.Fatal("expected an error writing to a nonexistent directory")
	}
	if !m.IsUntitled() {
		t.Fatal("expected the buffer to remain untitled after a failed SaveAs")
	}
	if !m.HasUnsavedChanges() {
		t.Fatal("expected the buffer to remain dirty after a failed SaveAs")
	}
}

func TestCtrlRightMovesToEndOfNextWord(t *testing.T) {
	m := setupEditor(t, "hello world")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlRight})
	if m.cursorCol != 5 {
		t.Fatalf("got cursorCol=%d, want 5 (end of \"hello\")", m.cursorCol)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlRight})
	if m.cursorCol != 11 {
		t.Fatalf("got cursorCol=%d, want 11 (end of \"world\")", m.cursorCol)
	}
	// Already at end of line: stays put, does not wrap to the next line.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlRight})
	if m.cursorCol != 11 || m.cursorLine != 0 {
		t.Fatalf("got line=%d col=%d, want to stay at end of line 0", m.cursorLine, m.cursorCol)
	}
}

func TestCtrlLeftMovesToStartOfPreviousWord(t *testing.T) {
	m := setupEditor(t, "hello world")
	m.cursorCol = 11 // end of line
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlLeft})
	if m.cursorCol != 6 {
		t.Fatalf("got cursorCol=%d, want 6 (start of \"world\")", m.cursorCol)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlLeft})
	if m.cursorCol != 0 {
		t.Fatalf("got cursorCol=%d, want 0 (start of \"hello\")", m.cursorCol)
	}
	// Already at start of line: stays put, does not wrap to the previous line.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlLeft})
	if m.cursorCol != 0 || m.cursorLine != 0 {
		t.Fatalf("got line=%d col=%d, want to stay at start of line 0", m.cursorLine, m.cursorCol)
	}
}

func TestCtrlLeftRightSkipPunctuationAsNonWordRuns(t *testing.T) {
	m := setupEditor(t, "foo.bar()")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlRight})
	if m.cursorCol != 3 {
		t.Fatalf("got cursorCol=%d, want 3 (end of \"foo\")", m.cursorCol)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlRight})
	if m.cursorCol != 7 {
		t.Fatalf("got cursorCol=%d, want 7 (end of \"bar\", skipping the '.')", m.cursorCol)
	}
}

func TestCtrlShiftRightExtendsSelectionByWord(t *testing.T) {
	m := setupEditor(t, "hello world")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlShiftRight})
	startLine, startCol, endLine, endCol, ok := m.selectionRange()
	if !ok {
		t.Fatal("expected ctrl+shift+right to start a selection")
	}
	if startLine != 0 || startCol != 0 || endLine != 0 || endCol != 5 {
		t.Fatalf("got selection [%d:%d, %d:%d), want [0:0, 0:5)", startLine, startCol, endLine, endCol)
	}
}

func TestCtrlShiftLeftExtendsSelectionByWord(t *testing.T) {
	m := setupEditor(t, "hello world")
	m.cursorCol = 11
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlShiftLeft})
	startLine, startCol, endLine, endCol, ok := m.selectionRange()
	if !ok {
		t.Fatal("expected ctrl+shift+left to start a selection")
	}
	if startLine != 0 || startCol != 6 || endLine != 0 || endCol != 11 {
		t.Fatalf("got selection [%d:%d, %d:%d), want [0:6, 0:11)", startLine, startCol, endLine, endCol)
	}
}

func TestPlainCtrlArrowCollapsesSelection(t *testing.T) {
	m := setupEditor(t, "hello world")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlShiftRight})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlRight})
	if _, _, _, _, ok := m.selectionRange(); ok {
		t.Fatal("expected a plain ctrl+arrow to collapse the selection, matching plain arrow's behavior")
	}
}

// Regression test (found by automated PR review): clicking a fold-toggle
// gutter moves the cursor to that line without clamping cursorCol against
// the new line's length (HandleClick's fold-toggle branch only sets
// m.cursorLine, unlike its own fall-through cursor-placing branch, which
// does clamp). A cursorCol left over from a longer line then indexes past
// the end of a shorter line — moveWordRight silently produced a stale
// value, but moveWordLeft's runes[i-1] indexing panicked outright.
func TestCtrlLeftAfterFoldClickLeavesStaleColumnDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc add(a int, b int) int {\n\treturn a + 42\n}\n\nfunc f() int {\n\treturn 1\n}\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m, err := m.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m = m.SetSize(60, 20)

	// Line 2 ("func add(a int, b int) int {") is 29 runes; put the cursor
	// at its end.
	m.cursorLine = 2
	m.cursorCol = 29

	// Click the fold gutter of line 6 ("func f() int {", only 14 runes) —
	// no other lines are folded, so visible row index == buffer line index.
	m, _ = m.HandleClick(0, 6)
	if m.cursorLine != 6 {
		t.Fatalf("setup failed: got cursorLine=%d, want 6", m.cursorLine)
	}
	if m.cursorCol <= len([]rune(m.buf.Lines[6])) {
		t.Fatalf("setup failed: expected a stale cursorCol=%d past line 6's length, want it to still be 29", m.cursorCol)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlLeft}) // must not panic

	if m.cursorCol > len([]rune(m.buf.Lines[6])) {
		t.Fatalf("got cursorCol=%d, want it clamped to line 6's length (%d)", m.cursorCol, len([]rune(m.buf.Lines[6])))
	}
}

func TestCtrlRightOnEmptyLineIsANoOp(t *testing.T) {
	m := setupEditor(t, "")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlRight})
	if m.cursorCol != 0 {
		t.Fatalf("got cursorCol=%d, want 0", m.cursorCol)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlLeft})
	if m.cursorCol != 0 {
		t.Fatalf("got cursorCol=%d, want 0", m.cursorCol)
	}
}
