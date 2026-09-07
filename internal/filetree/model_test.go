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
	// N items rendered, no trailing newline → N-1 newline separators.
	if got := strings.Count(view, "\n"); got != len(m.flat)-1 {
		t.Fatalf("got %d newline separators, want %d (all items, no trailing newline)", got, len(m.flat)-1)
	}
}

func TestViewClipsRenderedLinesToTheSetHeight(t *testing.T) {
	m := setupModelWithNFiles(t, 20)
	m = m.SetSize(40, 5)
	view := m.View()
	// 5 lines clipped, no trailing newline → 4 newline separators.
	if got := strings.Count(view, "\n"); got != 4 {
		t.Fatalf("got %d newline separators, want 4 (5 lines, no trailing newline)", got)
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
	// 1 item rendered, no trailing newline → 0 newline separators.
	if got := strings.Count(view, "\n"); got != 0 {
		t.Fatalf("got %d newline separators, want 0 (1 line, no trailing newline) — a name wider than the pane must not wrap", got)
	}
	if !strings.Contains(view, longName) {
		t.Fatal("expected the long name's full content to still be present, unwrapped")
	}
}

func TestHandleClickSelectsAndActivatesRow(t *testing.T) {
	m := setupModelWithNFiles(t, 5)
	m, cmd := m.HandleClick(2)
	if m.cursor != 2 {
		t.Fatalf("got cursor %d, want 2", m.cursor)
	}
	if cmd == nil {
		t.Fatal("expected clicking a file row to activate it (open the file)")
	}
	msg := cmd()
	if _, ok := msg.(FileOpenedMsg); !ok {
		t.Fatalf("got %T, want FileOpenedMsg", msg)
	}
}

func TestHandleClickPastLastRowIsANoOp(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	m, cmd := m.HandleClick(10)
	if m.cursor != 0 {
		t.Fatalf("got cursor %d, want 0 (unchanged)", m.cursor)
	}
	if cmd != nil {
		t.Fatal("expected no command for a click past the last row")
	}
}

func TestScrollMovesTreeSelection(t *testing.T) {
	m := setupModelWithNFiles(t, 10)
	m = m.Scroll(3)
	if m.cursor != 3 {
		t.Fatalf("got cursor %d, want 3", m.cursor)
	}
	m = m.Scroll(-1)
	if m.cursor != 2 {
		t.Fatalf("got cursor %d, want 2", m.cursor)
	}
}

func TestSetDirtyShowsModifiedIndicator(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.go"), "")
	mustWriteFile(t, filepath.Join(dir, "b.go"), "")
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	dirtyPath := m.flat[0].node.Path
	m = m.SetDirty(map[string]bool{dirtyPath: true})

	view := m.View()
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[0], "(M)") {
		t.Fatalf("expected %q to show a modified indicator, got line %q", dirtyPath, lines[0])
	}
	if strings.Contains(lines[1], "(M)") {
		t.Fatalf("expected the clean file to show no modified indicator, got line %q", lines[1])
	}
}

func TestSetDirtyWithEmptyMapShowsNoIndicators(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	view := m.View()
	if strings.Contains(view, "(M)") {
		t.Fatal("expected no modified indicators when SetDirty was never called")
	}
}

func TestSelectedDirOnADirectoryReturnsItself(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	// "sub" is the only (directory) entry, so it's at flat index 0.
	if got := m.SelectedDir(); got != filepath.Join(dir, "sub") {
		t.Fatalf("got %q, want %q", got, filepath.Join(dir, "sub"))
	}
}

func TestSelectedDirOnAFileReturnsItsParent(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	got := m.SelectedDir()
	want := filepath.Dir(m.flat[0].node.Path)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSelectedDirOnEmptyTreeReturnsRoot(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.SelectedDir(); got != dir {
		t.Fatalf("got %q, want %q (root, empty tree)", got, dir)
	}
}

func TestReloadDirAddsNewEntryAndAutoExpands(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(sub, "new.go"), "")

	m = m.ReloadDir(sub)

	found := false
	for _, item := range m.flat {
		if item.node.Path == filepath.Join(sub, "new.go") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected the new file to appear in the flat list after ReloadDir")
	}
}

func TestReloadDirOnUnknownPathIsANoOp(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	before := len(m.flat)
	m = m.ReloadDir("/some/path/never/tracked")
	if len(m.flat) != before {
		t.Fatalf("got %d flat items, want %d (unchanged)", len(m.flat), before)
	}
}

func TestStartCreateFileInsertsPhantomRowAtEndOfTargetDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(sub, "existing.go"), "")
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m = m.startCreate(sub, false)

	if m.mode != editCreatingFile {
		t.Fatalf("got mode=%v, want editCreatingFile", m.mode)
	}
	// "sub" must now be expanded (auto-expanded so the phantom row is
	// visible), with "existing.go" then the phantom row (node == nil) as
	// its last two children.
	var subIdx, phantomIdx int = -1, -1
	for i, item := range m.flat {
		if item.node != nil && item.node.Path == sub {
			subIdx = i
		}
		if item.node == nil {
			phantomIdx = i
		}
	}
	if subIdx < 0 {
		t.Fatal("expected 'sub' to be present in the flat list")
	}
	if phantomIdx != subIdx+2 { // sub, then existing.go, then the phantom
		t.Fatalf("got phantom at index %d, want %d (right after sub's one real child)", phantomIdx, subIdx+2)
	}
	if m.cursor != phantomIdx {
		t.Fatalf("got cursor=%d, want %d (the phantom row, so it's scrolled into view)", m.cursor, phantomIdx)
	}
}

func TestStartRenamePrefillsInputWithCurrentName(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	target := m.flat[1].node.Path
	m = m.startRename(target)

	if m.mode != editRenaming {
		t.Fatalf("got mode=%v, want editRenaming", m.mode)
	}
	if m.editInput.Value() != filepath.Base(target) {
		t.Fatalf("got input value %q, want %q", m.editInput.Value(), filepath.Base(target))
	}
	if m.cursor != 1 {
		t.Fatalf("got cursor=%d, want 1 (the row being renamed)", m.cursor)
	}
}

func TestSubmitCreateFileMakesFileReloadsTreeAndEmitsFileOpenedMsg(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m = m.startCreate(dir, false)
	m.editInput.SetValue("new.go")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.mode != editNone {
		t.Fatal("expected edit mode to end after a successful create")
	}
	if _, err := os.Stat(filepath.Join(dir, "new.go")); err != nil {
		t.Fatalf("expected new.go to exist on disk: %v", err)
	}
	found := false
	for _, item := range m.flat {
		if item.node != nil && item.node.Name == "new.go" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected new.go to appear in the reloaded flat list")
	}
	if cmd == nil {
		t.Fatal("expected a command")
	}
	msg, ok := cmd().(FileOpenedMsg)
	if !ok || filepath.Base(msg.Path) != "new.go" {
		t.Fatalf("got %+v, ok=%v, want FileOpenedMsg for new.go", msg, ok)
	}
}

func TestSubmitCreateDirDoesNotEmitFileOpenedMsg(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m = m.startCreate(dir, true)
	m.editInput.SetValue("newdir")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.mode != editNone {
		t.Fatal("expected edit mode to end after a successful create")
	}
	info, err := os.Stat(filepath.Join(dir, "newdir"))
	if err != nil || !info.IsDir() {
		t.Fatal("expected newdir to exist as a directory")
	}
	if cmd != nil {
		if _, ok := cmd().(FileOpenedMsg); ok {
			t.Fatal("expected creating a directory not to emit FileOpenedMsg")
		}
	}
}

func TestSubmitCreateWithExistingNameReportsErrorAndStaysInEditMode(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "existing.go"), "")
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m = m.startCreate(dir, false)
	m.editInput.SetValue("existing.go")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.mode != editCreatingFile {
		t.Fatal("expected to stay in edit mode after a failed create")
	}
	if cmd == nil {
		t.Fatal("expected a command reporting the error")
	}
	msg, ok := cmd().(FileTreeErrorMsg)
	if !ok || msg.Message == "" {
		t.Fatalf("got %+v, ok=%v, want a non-empty FileTreeErrorMsg", msg, ok)
	}
}

func TestSubmitRenameMovesTheFileAndReloadsTree(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "old.go"), "content")
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	oldPath := m.flat[0].node.Path
	m = m.startRename(oldPath)
	m.editInput.SetValue("new.go")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.mode != editNone {
		t.Fatal("expected edit mode to end after a successful rename")
	}
	if _, err := os.Stat(filepath.Join(dir, "old.go")); !os.IsNotExist(err) {
		t.Fatal("expected old.go to no longer exist")
	}
	found := false
	for _, item := range m.flat {
		if item.node != nil && item.node.Name == "new.go" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected new.go to appear in the reloaded flat list")
	}
}

func TestEscCancelsCreateWithoutTouchingDisk(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	before := len(m.flat)
	m = m.startCreate(dir, false)
	m.editInput.SetValue("would-be-created.go")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.mode != editNone {
		t.Fatal("expected Esc to cancel edit mode")
	}
	if len(m.flat) != before {
		t.Fatalf("got %d flat items, want %d (phantom row removed, nothing created)", len(m.flat), before)
	}
	if _, err := os.Stat(filepath.Join(dir, "would-be-created.go")); !os.IsNotExist(err) {
		t.Fatal("expected Esc not to create anything on disk")
	}
}

func TestClickWhileEditingCancelsWithoutActing(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	m = m.startRename(m.flat[0].node.Path)

	m, cmd := m.HandleClick(1)

	if m.mode != editNone {
		t.Fatal("expected a click during edit mode to cancel it")
	}
	if cmd != nil {
		t.Fatal("expected no command — a click during edit mode only cancels, it doesn't also activate the clicked row")
	}
}

func TestHandleRightClickOnFileTargetsParentDirWithRename(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	target := m.flat[0].node.Path
	m = m.HandleRightClick(0)

	if m.contextMenu == nil {
		t.Fatal("expected a context menu to open")
	}
	if m.contextMenu.targetPath != target {
		t.Fatalf("got targetPath=%q, want %q", m.contextMenu.targetPath, target)
	}
	if m.contextMenu.targetDir != filepath.Dir(target) {
		t.Fatalf("got targetDir=%q, want %q", m.contextMenu.targetDir, filepath.Dir(target))
	}
	items := m.contextMenu.items()
	if len(items) != 3 || items[2] != "Rename" {
		t.Fatalf("got items=%v, want [New File, New Folder, Rename]", items)
	}
}

func TestHandleRightClickOnDirTargetsItself(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m = m.HandleRightClick(0)
	if m.contextMenu.targetDir != filepath.Join(dir, "sub") {
		t.Fatalf("got targetDir=%q, want %q", m.contextMenu.targetDir, filepath.Join(dir, "sub"))
	}
}

func TestHandleRightClickOnEmptySpaceTargetsRootWithNoRename(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	m = m.HandleRightClick(50) // past the last row

	if m.contextMenu.targetPath != "" {
		t.Fatalf("got targetPath=%q, want empty (no specific row clicked)", m.contextMenu.targetPath)
	}
	items := m.contextMenu.items()
	if len(items) != 2 {
		t.Fatalf("got items=%v, want [New File, New Folder] (no Rename)", items)
	}
}

func TestClickingNewFileMenuItemStartsCreate(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	m = m.HandleRightClick(50) // empty space -> targets root, items = [New File, New Folder]

	m, _ = m.HandleClick(0) // row 0 of the menu = "New File"

	if m.mode != editCreatingFile {
		t.Fatalf("got mode=%v, want editCreatingFile", m.mode)
	}
	if m.contextMenu != nil {
		t.Fatal("expected the context menu to close once an item is selected")
	}
}

func TestClickingOutsideMenuClosesItWithoutAction(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	m = m.HandleRightClick(50)

	m, cmd := m.HandleClick(10) // well past the 2 menu rows

	if m.contextMenu != nil {
		t.Fatal("expected clicking outside the menu to close it")
	}
	if m.mode != editNone {
		t.Fatal("expected no edit mode to start from an outside click")
	}
	if cmd != nil {
		t.Fatal("expected no command from dismissing the menu")
	}
}

func TestEscClosesContextMenu(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	m = m.HandleRightClick(50)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.contextMenu != nil {
		t.Fatal("expected Esc to close the context menu")
	}
}

func TestViewClipsToZeroTreeRowsWhenMenuFillsThePane(t *testing.T) {
	m := setupModelWithNFiles(t, 20)
	m = m.SetSize(40, 3)
	m = m.HandleRightClick(0) // targets a file row -> menu has 3 items (New File, New Folder, Rename)

	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d rendered lines, want 3 (menu fills the entire 3-row pane, leaving zero tree rows) — got view:\n%s", len(lines), view)
	}
	for _, l := range lines {
		if !strings.Contains(l, "[") {
			t.Fatalf("expected every rendered line to be a menu row when the menu consumes the whole pane, got %q", l)
		}
	}
}

func TestRightClickWhileEditingCancelsTheEditFirst(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	m = m.startRename(m.flat[0].node.Path)

	m = m.HandleRightClick(50)

	if m.mode != editNone {
		t.Fatal("expected the in-progress rename to be cancelled by a right-click elsewhere")
	}
	if m.contextMenu == nil {
		t.Fatal("expected a new context menu to open")
	}
}

func TestViewTruncatesMenuItemsWhenPaneIsShorterThanTheMenu(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	m = m.SetSize(40, 1)
	m = m.HandleRightClick(0) // targets a file row -> 3-item menu

	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d rendered lines, want 1 (pane height is 1, menu must be truncated to fit) — view:\n%s", len(lines), view)
	}
}

func TestCancelledCreateDoesNotLeaveCursorOutOfRangeAfterRebuild(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	m = m.startCreate(m.root.Path, false)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.mode != editNone {
		t.Fatal("expected Esc to cancel edit mode")
	}
	if m.cursor < 0 || m.cursor >= len(m.flat) {
		t.Fatalf("got cursor=%d out of range for %d flat items after cancelling", m.cursor, len(m.flat))
	}
	// A subsequent Enter must not panic.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
}

func TestMKeyOpensContextMenuOnSelectedRow(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	target := m.flat[0].node.Path

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})

	if m.contextMenu == nil {
		t.Fatal("expected 'm' to open the context menu")
	}
	if m.contextMenu.targetPath != target {
		t.Fatalf("got targetPath=%q, want %q (the currently selected row)", m.contextMenu.targetPath, target)
	}
	items := m.contextMenu.items()
	if len(items) != 3 || items[2] != "Rename" {
		t.Fatalf("got items=%v, want [New File, New Folder, Rename] (a specific row is targeted)", items)
	}
}

func TestOpenContextMenuOnDirectoryTargetsItself(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m = m.OpenContextMenu()
	if m.contextMenu.targetDir != filepath.Join(dir, "sub") {
		t.Fatalf("got targetDir=%q, want %q", m.contextMenu.targetDir, filepath.Join(dir, "sub"))
	}
}

func TestArrowKeysNavigateOpenContextMenuAndEnterSelects(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	m = m.OpenContextMenu() // selected starts at 0 ("New File")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.contextMenu.selected != 1 {
		t.Fatalf("got selected=%d, want 1 (New Folder) after one Down", m.contextMenu.selected)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.contextMenu.selected != 2 {
		t.Fatalf("got selected=%d, want 2 (Rename) after two Down", m.contextMenu.selected)
	}
	// Wraps back to 0 past the last item.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.contextMenu.selected != 0 {
		t.Fatalf("got selected=%d, want 0 (wrapped)", m.contextMenu.selected)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.contextMenu.selected != 2 {
		t.Fatalf("got selected=%d, want 2 (wrapped backward)", m.contextMenu.selected)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != editRenaming {
		t.Fatalf("got mode=%v, want editRenaming (Enter on the highlighted 'Rename' item)", m.mode)
	}
	if m.contextMenu != nil {
		t.Fatal("expected the menu to close once an item is selected via Enter")
	}
}

func TestEscClosesKeyboardOpenedContextMenu(t *testing.T) {
	m := setupModelWithNFiles(t, 3)
	m = m.OpenContextMenu()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.contextMenu != nil {
		t.Fatal("expected Esc to close the context menu")
	}
}
