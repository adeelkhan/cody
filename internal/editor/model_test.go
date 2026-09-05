package editor

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

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
	if _, ok := msg.(rehighlightMsg); !ok {
		t.Fatalf("got %T, want rehighlightMsg", msg)
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
	m, _ = m.Update(rehighlightMsg{generation: m.highlightGeneration})
	if len(m.highlightSpans) == 0 {
		t.Fatal("expected highlightSpans to be repopulated after a matching-generation rehighlightMsg")
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

	m, _ = m.Update(rehighlightMsg{generation: staleGeneration})

	if len(m.highlightSpans[0]) != spansBeforeEdit {
		t.Fatalf("got %d spans on line 0, want %d (unchanged from before the edit) — a stale-generation message must not trigger a reparse", len(m.highlightSpans[0]), spansBeforeEdit)
	}
}
