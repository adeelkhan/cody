# File/Folder Creation, Rename, and Directory Icons Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user create files/folders and rename entries in the project tree (via a menu dialog, a blank-tab-then-save-as flow, or a VS-Code-style right-click + inline name entry), and show open/closed folder icons.

**Architecture:** `filetree` gains filesystem primitives (`CreateFile`/`CreateDir`/`Rename`), a state-preserving `Node.Reload`, and self-contained inline-editing UI (a "phantom row" spliced into its own flat list, plus a fixed-position right-click context menu) — all internal to the package, exposed only through `HandleClick`/`HandleRightClick`/`Update`/`View` exactly like today. `editor` gains pathless "untitled" buffer support. `app` gains one new shared dialog (path prompt, used for both "New File" and "Save As") built the same way the existing confirm dialog shares one implementation across two actions.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, `bubbles/textinput` (already a dependency).

**Spec:** `docs/superpowers/specs/2026-09-07-file-create-rename-design.md`

## Global Constraints

- No new third-party dependencies.
- Creating/renaming to a path that already exists is an error shown to the user — no overwrite-confirmation flow.
- Renaming a file currently open in a tab does not update that tab's path (accepted limitation).
- The right-click context menu always renders at the top of the tree pane's visible rows, not at the literal click position.
- Every existing test must keep passing.
- Run `go build ./... && go test ./...` after every task; it must be green before moving to the next task.

---

### Task 1: Open/closed directory icons

**Files:**
- Modify: `internal/filetree/icons.go`
- Test: `internal/filetree/icons_test.go` (new file)

**Interfaces:**
- Produces: `IconFor` now depends on `Node.Expanded` for directories — no signature change, existing callers (`View()`) are unaffected since they already pass the real `*Node`.

- [ ] **Step 1: Write the failing tests**

Create `internal/filetree/icons_test.go`:

```go
package filetree

import "testing"

func TestIconForClosedDirUsesClosedGlyph(t *testing.T) {
	n := &Node{Type: NodeDir, Expanded: false}
	if got := IconFor(n, false); got != fallbackDir {
		t.Fatalf("got %q, want %q (closed dir fallback)", got, fallbackDir)
	}
	if got := IconFor(n, true); got != nerdFontDir {
		t.Fatalf("got %q, want %q (closed dir nerd font)", got, nerdFontDir)
	}
}

func TestIconForExpandedDirUsesOpenGlyph(t *testing.T) {
	n := &Node{Type: NodeDir, Expanded: true}
	if got := IconFor(n, false); got != fallbackDirOpen {
		t.Fatalf("got %q, want %q (open dir fallback)", got, fallbackDirOpen)
	}
	if got := IconFor(n, true); got != nerdFontDirOpen {
		t.Fatalf("got %q, want %q (open dir nerd font)", got, nerdFontDirOpen)
	}
}

func TestIconForOpenAndClosedFallbacksAreDistinct(t *testing.T) {
	if fallbackDir == fallbackDirOpen || fallbackDir == fallbackFile || fallbackDirOpen == fallbackFile {
		t.Fatalf("expected fallbackDir=%q, fallbackDirOpen=%q, fallbackFile=%q to all be distinct", fallbackDir, fallbackDirOpen, fallbackFile)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/filetree/ -run TestIconFor -v`
Expected: FAIL to compile (`fallbackDirOpen`/`nerdFontDirOpen` undefined).

- [ ] **Step 3: Implement the open-folder icon**

Replace the whole of `internal/filetree/icons.go` with:

```go
package filetree

import "path/filepath"

var nerdFontIcons = map[string]string{
	".go":   "",
	".py":   "",
	".js":   "",
	".ts":   "",
	".json": "",
	".md":   "",
}

const (
	nerdFontDir     = "" // fa-folder, closed
	nerdFontDirOpen = "" // fa-folder-open
	nerdFontFile    = ""
	fallbackDir     = "+"
	fallbackDirOpen = "~"
	fallbackFile    = "-"
)

func IconFor(n *Node, nerdFont bool) string {
	if n.Type == NodeDir {
		if n.Expanded {
			if nerdFont {
				return nerdFontDirOpen
			}
			return fallbackDirOpen
		}
		if nerdFont {
			return nerdFontDir
		}
		return fallbackDir
	}
	if nerdFont {
		if icon, ok := nerdFontIcons[filepath.Ext(n.Name)]; ok {
			return icon
		}
		return nerdFontFile
	}
	return fallbackFile
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/filetree/ -run TestIconFor -v`
Expected: PASS (all three).

- [ ] **Step 5: Run the full filetree package test suite**

Run: `go test ./internal/filetree/...`
Expected: PASS, no regressions.

- [ ] **Step 6: Commit**

```bash
git add internal/filetree/icons.go internal/filetree/icons_test.go
git commit -m "feat: show open-folder icon for expanded directories"
```

---

### Task 2: `filetree` filesystem primitives — create, rename, state-preserving reload

**Files:**
- Modify: `internal/filetree/tree.go`
- Modify: `internal/filetree/model.go`
- Test: `internal/filetree/tree_test.go`
- Test: `internal/filetree/model_test.go`

**Interfaces:**
- Produces: `func CreateFile(path string) error`, `func CreateDir(path string) error`, `func Rename(oldPath, newPath string) error`, `func (n *Node) Reload() error`, `func (m Model) SelectedDir() string`, `func (m Model) ReloadDir(dirPath string) Model`, `func (m Model) findNode(path string) *Node` — Tasks 4, 5, and 6 (in `app`) call these directly.

- [ ] **Step 1: Write the failing tests**

Add to `internal/filetree/tree_test.go`:

```go
func TestCreateFileMakesAnEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.go")
	if err := CreateFile(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("got %d bytes, want 0 (empty file)", len(data))
	}
}

func TestCreateFileFailsIfAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.go")
	mustWriteFile(t, path, "content")
	if err := CreateFile(path); err == nil {
		t.Fatal("expected an error creating a file that already exists")
	}
}

func TestCreateDirMakesADirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "newdir")
	if err := CreateDir(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("expected a directory")
	}
}

func TestCreateDirFailsIfAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existingdir")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := CreateDir(path); err == nil {
		t.Fatal("expected an error creating a directory that already exists")
	}
}

func TestRenameMovesTheFile(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.go")
	newPath := filepath.Join(dir, "new.go")
	mustWriteFile(t, oldPath, "content")
	if err := Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("expected the old path to no longer exist")
	}
	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "content" {
		t.Fatalf("got %q, want %q", string(data), "content")
	}
}

func TestRenameFailsIfTargetAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "a.go")
	newPath := filepath.Join(dir, "b.go")
	mustWriteFile(t, oldPath, "a")
	mustWriteFile(t, newPath, "b")
	if err := Rename(oldPath, newPath); err == nil {
		t.Fatal("expected an error renaming onto an existing file")
	}
	data, _ := os.ReadFile(oldPath)
	if string(data) != "a" {
		t.Fatal("expected the source file to be untouched after a failed rename")
	}
}

func TestReloadPicksUpNewFilesWithoutLosingSiblingExpandedState(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(sub, "inner.go"), "")
	root, err := NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Expand "sub" so it has loaded children and Expanded == true.
	var subNode *Node
	for _, c := range root.Children {
		if c.Name == "sub" {
			subNode = c
		}
	}
	if subNode == nil {
		t.Fatal("setup failed: expected a 'sub' child")
	}
	subNode.Expanded = true
	if err := subNode.LoadChildren(); err != nil {
		t.Fatal(err)
	}

	// Create a new sibling file at the root, then reload the root.
	mustWriteFile(t, filepath.Join(dir, "new.go"), "")
	if err := root.Reload(); err != nil {
		t.Fatal(err)
	}

	found := false
	for _, c := range root.Children {
		if c.Name == "new.go" {
			found = true
		}
		if c.Name == "sub" && !c.Expanded {
			t.Fatal("expected 'sub' to remain expanded after reloading its unrelated sibling's parent")
		}
	}
	if !found {
		t.Fatal("expected the newly created file to appear after Reload")
	}
}
```

Add to `internal/filetree/model_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/filetree/ -run 'TestCreateFile|TestCreateDir|TestRename|TestReload|TestSelectedDir' -v`
Expected: FAIL to compile (`CreateFile`, `CreateDir`, `Rename`, `Reload`, `SelectedDir`, `ReloadDir` undefined).

- [ ] **Step 3: Add the filesystem primitives to `tree.go`**

Add near the top of `internal/filetree/tree.go` (after the imports, which need `"fmt"` added — the current import block is `"os"`, `"path/filepath"`, `"sort"`):

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)
```

Add these three functions anywhere in the file (e.g., right after `NewRoot`):

```go
// CreateFile creates an empty file at path, failing if it already exists.
func CreateFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	return f.Close()
}

// CreateDir creates a directory at path, failing if it already exists.
func CreateDir(path string) error {
	return os.Mkdir(path, 0755)
}

// Rename renames oldPath to newPath, failing if newPath already exists.
func Rename(oldPath, newPath string) error {
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("%s already exists", filepath.Base(newPath))
	}
	return os.Rename(oldPath, newPath)
}
```

- [ ] **Step 4: Refactor `LoadChildren` and add `Reload`**

Replace the whole of `(n *Node) LoadChildren`:

```go
func (n *Node) LoadChildren() error {
	if n.Type != NodeDir || n.loaded {
		return nil
	}
	entries, err := os.ReadDir(n.Path)
	if err != nil {
		return err
	}
	var children []*Node
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		childType := NodeFile
		if e.IsDir() {
			childType = NodeDir
		}
		children = append(children, &Node{
			Name: e.Name(),
			Path: filepath.Join(n.Path, e.Name()),
			Type: childType,
		})
	}
	sort.Slice(children, func(i, j int) bool {
		if children[i].Type != children[j].Type {
			return children[i].Type == NodeDir
		}
		return children[i].Name < children[j].Name
	})
	n.Children = children
	n.loaded = true
	return nil
}
```

with:

```go
func (n *Node) LoadChildren() error {
	if n.Type != NodeDir || n.loaded {
		return nil
	}
	return n.reloadChildren()
}

// Reload re-reads this directory's entries from disk unconditionally (even
// if already loaded), adding new entries and dropping deleted ones, while
// preserving the existing *Node — and its Expanded/loaded/Children state —
// for every entry still present. A no-op on a non-directory node.
func (n *Node) Reload() error {
	if n.Type != NodeDir {
		return nil
	}
	return n.reloadChildren()
}

func (n *Node) reloadChildren() error {
	entries, err := os.ReadDir(n.Path)
	if err != nil {
		return err
	}
	existing := make(map[string]*Node, len(n.Children))
	for _, c := range n.Children {
		existing[c.Name] = c
	}
	var children []*Node
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		if c, ok := existing[e.Name()]; ok {
			children = append(children, c)
			continue
		}
		childType := NodeFile
		if e.IsDir() {
			childType = NodeDir
		}
		children = append(children, &Node{
			Name: e.Name(),
			Path: filepath.Join(n.Path, e.Name()),
			Type: childType,
		})
	}
	sort.Slice(children, func(i, j int) bool {
		if children[i].Type != children[j].Type {
			return children[i].Type == NodeDir
		}
		return children[i].Name < children[j].Name
	})
	n.Children = children
	n.loaded = true
	return nil
}
```

- [ ] **Step 5: Add `SelectedDir`, `findNode`, `ReloadDir` to `model.go`**

Add these methods anywhere in `internal/filetree/model.go` (e.g., right after `SetDirty`); this step does not yet need the `editMode`/phantom-row awareness that `SelectedDir` will gain in Task 4 — for now, `m.flat[m.cursor].node` is always non-nil, since Task 4 hasn't introduced phantom rows yet:

```go
// SelectedDir returns the directory context for creating a new file/folder:
// the selected node itself if it's a directory, its parent otherwise. Falls
// back to the tree's root when nothing is selected (e.g. an empty tree).
func (m Model) SelectedDir() string {
	if len(m.flat) == 0 || m.cursor < 0 || m.cursor >= len(m.flat) {
		return m.root.Path
	}
	n := m.flat[m.cursor].node
	if n.Type == NodeDir {
		return n.Path
	}
	return filepath.Dir(n.Path)
}

// findNode returns the tracked *Node at path, or nil if the tree has never
// loaded that far (e.g. an ancestor directory was never expanded).
func (m Model) findNode(path string) *Node {
	if m.root.Path == path {
		return m.root
	}
	var find func(n *Node) *Node
	find = func(n *Node) *Node {
		for _, c := range n.Children {
			if c.Path == path {
				return c
			}
			if c.Type == NodeDir {
				if found := find(c); found != nil {
					return found
				}
			}
		}
		return nil
	}
	return find(m.root)
}

// ReloadDir refreshes dirPath's children (auto-expanding it so a newly
// created entry is immediately visible) and rebuilds the flat list. A no-op
// if dirPath isn't currently tracked (e.g. it was never expanded) — the new
// entry will show correctly the first time the directory is expanded
// anyway, since LoadChildren always does a fresh read on first expansion.
func (m Model) ReloadDir(dirPath string) Model {
	n := m.findNode(dirPath)
	if n == nil {
		return m
	}
	n.Expanded = true
	n.Reload()
	m.rebuildFlat()
	m.ensureCursorVisible()
	return m
}
```

`internal/filetree/model.go`'s import block needs `"path/filepath"` added — the current block is `"fmt"`, `"strings"`, `tea "github.com/charmbracelet/bubbletea"`, `"github.com/charmbracelet/lipgloss"`, `"cody/internal/scrollbar"`:

```go
import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cody/internal/scrollbar"
)
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/filetree/ -run 'TestCreateFile|TestCreateDir|TestRename|TestReload|TestSelectedDir' -v`
Expected: PASS (all).

- [ ] **Step 7: Run the full filetree package test suite**

Run: `go test ./internal/filetree/...`
Expected: PASS, no regressions — this confirms the `LoadChildren`/`Reload` refactor preserved existing tree-loading behavior exactly.

- [ ] **Step 8: Commit**

```bash
git add internal/filetree/tree.go internal/filetree/model.go internal/filetree/tree_test.go internal/filetree/model_test.go
git commit -m "feat: add CreateFile/CreateDir/Rename, state-preserving Reload, SelectedDir"
```

---

### Task 3: `editor` untitled (pathless) buffer support

**Files:**
- Modify: `internal/editor/model.go`
- Test: `internal/editor/model_test.go`

**Interfaces:**
- Produces: `func (m Model) NewBlankBuffer() Model`, `func (m Model) IsUntitled() bool`, `func (m Model) SaveAs(path string) (Model, error)` — Task 6 (in `app`) calls all three.

- [ ] **Step 1: Write the failing tests**

Add to `internal/editor/model_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/editor/ -run 'TestNewBlankBuffer|TestIsUntitled|TestSaveAs' -v`
Expected: FAIL to compile (`NewBlankBuffer`, `IsUntitled`, `SaveAs` undefined).

- [ ] **Step 3: Implement the three methods**

Add to `internal/editor/model.go`, directly after the existing `HasUnsavedChanges` method:

```go
// NewBlankBuffer starts editing a fresh, empty, pathless buffer — created
// by Ctrl+N, not tied to any file on disk until SaveAs gives it one.
func (m Model) NewBlankBuffer() Model {
	m.buf = &Buffer{Lines: []string{""}}
	m.cursorLine = 0
	m.cursorCol = 0
	m.selecting = false
	m.selAnchorLine = 0
	m.selAnchorCol = 0
	m.undoStack = nil
	m.redoStack = nil
	m.highlighter = nil
	m.highlightSpans = nil
	m.folder = nil
	m.folds = nil
	m.foldedStartLines = nil
	m.scrollOffset = 0
	m.searchMatches = nil
	m.searchIndex = -1
	return m
}

// IsUntitled reports whether the buffer has never been saved to a path
// (created via NewBlankBuffer rather than LoadFile). Saving it requires a
// path from the user first — see SaveAs. False when no buffer is loaded.
func (m Model) IsUntitled() bool {
	return m.buf != nil && m.buf.Path == ""
}

// SaveAs writes the buffer's current content to path for the first time,
// attaching that path to the buffer, then runs the same language-detection
// LoadFile does at load time — an untitled buffer has no extension to
// detect a language from until now.
func (m Model) SaveAs(path string) (Model, error) {
	m.buf.Path = path
	if err := m.buf.Save(); err != nil {
		return m, err
	}
	if lang, ok := highlight.LanguageForPath(path); ok {
		if h, err := highlight.New(lang); err == nil {
			m.highlighter = h
		}
		if f, err := highlight.NewFolder(lang); err == nil {
			m.folder = f
		}
	}
	m.rehighlight()
	m.refold()
	return m, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/editor/ -run 'TestNewBlankBuffer|TestIsUntitled|TestSaveAs' -v`
Expected: PASS (all five).

- [ ] **Step 5: Run the full editor package test suite**

Run: `go test ./internal/editor/...`
Expected: PASS, no regressions.

- [ ] **Step 6: Commit**

```bash
git add internal/editor/model.go internal/editor/model_test.go
git commit -m "feat: add untitled-buffer support (NewBlankBuffer, IsUntitled, SaveAs)"
```

---

### Task 4: `filetree` inline create/rename editing

**Files:**
- Modify: `internal/filetree/model.go`
- Test: `internal/filetree/model_test.go`

**Interfaces:**
- Consumes: `CreateFile`/`CreateDir`/`Rename`/`Model.ReloadDir` (Task 2).
- Produces: `func (m Model) startCreate(dir string, isDir bool) Model`, `func (m Model) startRename(path string) Model`, `type FileTreeErrorMsg struct{ Message string }` — Task 5's context-menu selection calls `startCreate`/`startRename`; Task 6 (`app`) handles `FileTreeErrorMsg` the same way it already handles `FileOpenedMsg`.

This task adds no way to *reach* `startCreate`/`startRename` yet (that's the right-click context menu, Task 5) — it's tested by calling them directly, matching how Task 4 of the tabs plan tested `closeTab` before any UI called it.

- [ ] **Step 1: Add the new fields, message type, and `bubbles/textinput` import**

Add `"github.com/charmbracelet/bubbles/textinput"` to `model.go`'s import block (alongside the existing `tea`/`lipgloss` imports).

Add near the top of `model.go`, after `FileOpenedMsg`:

```go
// FileTreeErrorMsg reports a failed create/rename to the app, which shows
// it in the status bar the same way it already shows other transient
// status messages.
type FileTreeErrorMsg struct {
	Message string
}

type editMode int

const (
	editNone editMode = iota
	editCreatingFile
	editCreatingDir
	editRenaming
)
```

Add three fields to `Model`:

```go
type Model struct {
	root         *Node
	nerdFont     bool
	flat         []flatItem
	cursor       int
	width        int
	height       int
	scrollOffset int
	dirty        map[string]bool

	mode       editMode
	editInput  textinput.Model
	editTarget string // create: target directory; rename: the node's current path
}
```

- [ ] **Step 2: Write the failing tests**

Add to `internal/filetree/model_test.go`:

```go
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
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/filetree/ -run 'TestStartCreate|TestStartRename|TestSubmitCreate|TestSubmitRename|TestEscCancels|TestClickWhileEditing' -v`
Expected: FAIL to compile (`startCreate`, `startRename` undefined).

- [ ] **Step 4: Update `SelectedDir` for phantom-row safety**

Replace the `SelectedDir` method (added in Task 2) with:

```go
// SelectedDir returns the directory context for creating a new file/folder:
// the selected node itself if it's a directory, its parent otherwise. Falls
// back to the tree's root when nothing is selected (e.g. an empty tree, or
// the selection is currently the phantom "typing a new name" row).
func (m Model) SelectedDir() string {
	if len(m.flat) == 0 || m.cursor < 0 || m.cursor >= len(m.flat) {
		return m.root.Path
	}
	n := m.flat[m.cursor].node
	if n == nil {
		return m.root.Path
	}
	if n.Type == NodeDir {
		return n.Path
	}
	return filepath.Dir(n.Path)
}
```

- [ ] **Step 5: Update `rebuildFlat` to splice in the phantom row**

Replace:

```go
func (m *Model) rebuildFlat() {
	m.flat = nil
	var walk func(n *Node, depth int)
	walk = func(n *Node, depth int) {
		for _, c := range n.Children {
			m.flat = append(m.flat, flatItem{node: c, depth: depth})
			if c.Type == NodeDir && c.Expanded {
				walk(c, depth+1)
			}
		}
	}
	walk(m.root, 0)
}
```

with:

```go
func (m *Model) rebuildFlat() {
	m.flat = nil
	var walk func(n *Node, depth int)
	walk = func(n *Node, depth int) {
		for _, c := range n.Children {
			m.flat = append(m.flat, flatItem{node: c, depth: depth})
			if c.Type == NodeDir && c.Expanded {
				walk(c, depth+1)
			}
		}
		if n.Path == m.editTarget && (m.mode == editCreatingFile || m.mode == editCreatingDir) {
			m.flat = append(m.flat, flatItem{node: nil, depth: depth + 1})
		}
	}
	walk(m.root, 0)
}
```

- [ ] **Step 6: Add `startCreate`, `startRename`, `indexOfPhantom`, `indexOfNode`, `submitEdit`, `updateEditInput`**

Add these methods anywhere in `model.go` (e.g., after `ReloadDir`):

```go
func (m Model) startCreate(dir string, isDir bool) Model {
	if n := m.findNode(dir); n != nil {
		n.Expanded = true
	}
	m.mode = editCreatingFile
	if isDir {
		m.mode = editCreatingDir
	}
	m.editTarget = dir
	m.editInput = textinput.New()
	m.editInput.Focus()
	m.rebuildFlat()
	if idx := m.indexOfPhantom(); idx >= 0 {
		m.cursor = idx
	}
	m.ensureCursorVisible()
	return m
}

func (m Model) startRename(path string) Model {
	m.mode = editRenaming
	m.editTarget = path
	m.editInput = textinput.New()
	m.editInput.SetValue(filepath.Base(path))
	m.editInput.CursorEnd()
	m.editInput.Focus()
	if idx := m.indexOfNode(path); idx >= 0 {
		m.cursor = idx
	}
	m.ensureCursorVisible()
	return m
}

// indexOfPhantom returns the flat-list index of the "typing a new name"
// row, or -1 if there isn't one.
func (m Model) indexOfPhantom() int {
	for i, item := range m.flat {
		if item.node == nil {
			return i
		}
	}
	return -1
}

// indexOfNode returns the flat-list index of the node at path, or -1 if
// it isn't currently visible (e.g. its parent directory is collapsed).
func (m Model) indexOfNode(path string) int {
	for i, item := range m.flat {
		if item.node != nil && item.node.Path == path {
			return i
		}
	}
	return -1
}

func (m Model) updateEditInput(keyMsg tea.KeyMsg) (Model, tea.Cmd) {
	switch keyMsg.String() {
	case "esc":
		m.mode = editNone
		m.rebuildFlat()
		return m, nil
	case "enter":
		return m.submitEdit()
	}
	var cmd tea.Cmd
	m.editInput, cmd = m.editInput.Update(keyMsg)
	return m, cmd
}

func (m Model) submitEdit() (Model, tea.Cmd) {
	name := m.editInput.Value()
	if name == "" {
		return m, func() tea.Msg { return FileTreeErrorMsg{Message: "name is required"} }
	}
	switch m.mode {
	case editCreatingFile:
		full := filepath.Join(m.editTarget, name)
		if err := CreateFile(full); err != nil {
			return m, func() tea.Msg { return FileTreeErrorMsg{Message: err.Error()} }
		}
		m.mode = editNone
		m = m.ReloadDir(m.editTarget)
		return m, func() tea.Msg { return FileOpenedMsg{Path: full} }
	case editCreatingDir:
		full := filepath.Join(m.editTarget, name)
		if err := CreateDir(full); err != nil {
			return m, func() tea.Msg { return FileTreeErrorMsg{Message: err.Error()} }
		}
		m.mode = editNone
		m = m.ReloadDir(m.editTarget)
		return m, nil
	case editRenaming:
		newPath := filepath.Join(filepath.Dir(m.editTarget), name)
		if err := Rename(m.editTarget, newPath); err != nil {
			return m, func() tea.Msg { return FileTreeErrorMsg{Message: err.Error()} }
		}
		m.mode = editNone
		m = m.ReloadDir(filepath.Dir(m.editTarget))
		return m, nil
	}
	return m, nil
}
```

- [ ] **Step 7: Route `Update` and `HandleClick` through edit mode**

Replace the top of `Update`:

```go
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	var cmd tea.Cmd
	switch keyMsg.String() {
```

with:

```go
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.mode != editNone {
		return m.updateEditInput(keyMsg)
	}
	var cmd tea.Cmd
	switch keyMsg.String() {
```

(The rest of `Update` — the existing up/k/down/j/left/h/right/l/enter switch, `m.ensureCursorVisible()`, `return m, cmd` — is unchanged.)

Replace the top of `HandleClick`:

```go
func (m Model) HandleClick(y int) (Model, tea.Cmd) {
	idx := m.scrollOffset + y
```

with:

```go
func (m Model) HandleClick(y int) (Model, tea.Cmd) {
	if m.mode != editNone {
		m.mode = editNone
		m.rebuildFlat()
		return m, nil
	}
	idx := m.scrollOffset + y
```

- [ ] **Step 8: Render the phantom/rename row in `View()`**

Replace the per-row block:

```go
	var b strings.Builder
	for idx := viewStart; idx < viewEnd; idx++ {
		item := m.flat[idx]
		prefix := strings.Repeat("  ", item.depth)
		icon := IconFor(item.node, m.nerdFont)
		line := fmt.Sprintf("%s%s %s", prefix, icon, item.node.Name)
		if m.dirty[item.node.Path] {
			line += dirtyStyle.Render(" (M)")
		}
		if idx == m.cursor {
			line = "> " + line
		} else {
			line = "  " + line
		}
```

with:

```go
	var b strings.Builder
	for idx := viewStart; idx < viewEnd; idx++ {
		item := m.flat[idx]
		prefix := strings.Repeat("  ", item.depth)
		var line string
		switch {
		case item.node == nil:
			line = prefix + m.editInput.View()
		case m.mode == editRenaming && item.node.Path == m.editTarget:
			line = prefix + m.editInput.View()
		default:
			icon := IconFor(item.node, m.nerdFont)
			line = fmt.Sprintf("%s%s %s", prefix, icon, item.node.Name)
			if m.dirty[item.node.Path] {
				line += dirtyStyle.Render(" (M)")
			}
		}
		if idx == m.cursor {
			line = "> " + line
		} else {
			line = "  " + line
		}
```

(Everything below this — the `padRow`/scrollbar block and the loop's closing brace — is unchanged.)

- [ ] **Step 9: Run the tests to verify they pass**

Run: `go test ./internal/filetree/ -run 'TestStartCreate|TestStartRename|TestSubmitCreate|TestSubmitRename|TestEscCancels|TestClickWhileEditing' -v`
Expected: PASS (all eight).

- [ ] **Step 10: Run the full filetree package test suite**

Run: `go test ./internal/filetree/...`
Expected: PASS, no regressions.

- [ ] **Step 11: Commit**

```bash
git add internal/filetree/model.go internal/filetree/model_test.go
git commit -m "feat: add inline create/rename editing (phantom row) to filetree"
```

---

### Task 5: Right-click context menu (filetree + app wiring)

**Files:**
- Modify: `internal/filetree/model.go`
- Modify: `internal/app/model.go`
- Test: `internal/filetree/model_test.go`
- Test: `internal/app/model_test.go`

**Interfaces:**
- Consumes: `startCreate`/`startRename` (Task 4).
- Produces: `func (m Model) HandleRightClick(y int) Model` (filetree) — `app`'s `handleRightClick` calls it.

- [ ] **Step 1: Add `contextMenuState` and the `contextMenu` field**

Add near the top of `internal/filetree/model.go`, after the `editMode` const block:

```go
// contextMenuState describes an open right-click menu: where new items
// would be created, and (if a specific row was clicked) which one, since
// that's what enables the Rename option.
type contextMenuState struct {
	targetDir  string
	targetPath string // "" if the right-click hit empty space
}

// items lists this menu's actions in render/click order — Rename only
// appears when a specific row was right-clicked.
func (cm *contextMenuState) items() []string {
	items := []string{"New File", "New Folder"}
	if cm.targetPath != "" {
		items = append(items, "Rename")
	}
	return items
}
```

Add one field to `Model` (after `editTarget`):

```go
	mode        editMode
	editInput   textinput.Model
	editTarget  string
	contextMenu *contextMenuState
```

- [ ] **Step 2: Write the failing tests**

Add to `internal/filetree/model_test.go`:

```go
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
```

Add to `internal/app/model_test.go`:

```go
func TestRightClickInTreeOpensContextMenu(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	m.focus = focusEditor

	updated, _ = m.Update(tea.MouseMsg{X: 5, Y: 5, Button: tea.MouseButtonRight, Action: tea.MouseActionPress})
	m = updated.(Model)

	if m.focus != focusTree {
		t.Fatal("expected right-clicking the tree to focus it")
	}
}

func TestFileTreeErrorMsgSetsRecentCommand(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileTreeErrorMsg{Message: "boom"})
	m = updated.(Model)
	if m.recentCommand != "boom" {
		t.Fatalf("got recentCommand=%q, want %q", m.recentCommand, "boom")
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/filetree/ -run 'TestHandleRightClick|TestClickingNewFile|TestClickingOutside|TestEscClosesContext|TestRightClickWhileEditing' -v`
Expected: FAIL to compile (`HandleRightClick` undefined).

- [ ] **Step 4: Add `HandleRightClick` and `selectContextMenuItem` to `filetree`**

Add anywhere in `model.go` (e.g., after `startRename`):

```go
// HandleRightClick opens a context menu targeting row y (relative to the
// pane's content area, same convention as HandleClick) — the directory
// itself if it's a directory, its parent if a file, or the tree's root if y
// lands past the last row (empty space). Replaces any already-open menu,
// and cancels an in-progress create/rename first (a right-click clearly
// signals "do something else now" rather than leaving a stale phantom row
// or edit state behind).
func (m Model) HandleRightClick(y int) Model {
	if m.mode != editNone {
		m.mode = editNone
		m.rebuildFlat()
	}
	idx := m.scrollOffset + y
	if idx < 0 || idx >= len(m.flat) || m.flat[idx].node == nil {
		m.contextMenu = &contextMenuState{targetDir: m.root.Path}
		return m
	}
	n := m.flat[idx].node
	targetDir := n.Path
	if n.Type != NodeDir {
		targetDir = filepath.Dir(n.Path)
	}
	m.contextMenu = &contextMenuState{targetDir: targetDir, targetPath: n.Path}
	return m
}

func (m Model) selectContextMenuItem(label string) Model {
	cm := m.contextMenu
	m.contextMenu = nil
	switch label {
	case "New File":
		return m.startCreate(cm.targetDir, false)
	case "New Folder":
		return m.startCreate(cm.targetDir, true)
	case "Rename":
		return m.startRename(cm.targetPath)
	}
	return m
}
```

- [ ] **Step 5: Route `HandleClick`, `Update`, and `View` through the context menu**

Replace the top of `HandleClick` (from Task 4):

```go
func (m Model) HandleClick(y int) (Model, tea.Cmd) {
	if m.mode != editNone {
		m.mode = editNone
		m.rebuildFlat()
		return m, nil
	}
	idx := m.scrollOffset + y
```

with:

```go
func (m Model) HandleClick(y int) (Model, tea.Cmd) {
	if m.contextMenu != nil {
		items := m.contextMenu.items()
		if y >= 0 && y < len(items) {
			return m.selectContextMenuItem(items[y]), nil
		}
		m.contextMenu = nil
		return m, nil
	}
	if m.mode != editNone {
		m.mode = editNone
		m.rebuildFlat()
		return m, nil
	}
	idx := m.scrollOffset + y
```

Replace the top of `Update` (from Task 4):

```go
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.mode != editNone {
		return m.updateEditInput(keyMsg)
	}
	var cmd tea.Cmd
	switch keyMsg.String() {
```

with:

```go
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.mode != editNone {
		return m.updateEditInput(keyMsg)
	}
	if m.contextMenu != nil {
		if keyMsg.String() == "esc" {
			m.contextMenu = nil
		}
		return m, nil
	}
	var cmd tea.Cmd
	switch keyMsg.String() {
```

Replace the start of `View()`:

```go
func (m Model) View() string {
	clip := m.height > 0
	viewStart, viewEnd := 0, len(m.flat)
	if clip {
		viewStart = m.scrollOffset
		if viewStart < 0 {
			viewStart = 0
		}
		if viewStart > len(m.flat) {
			viewStart = len(m.flat)
		}
		viewEnd = viewStart + m.height
		if viewEnd > len(m.flat) {
			viewEnd = len(m.flat)
		}
	}

	var bar []rune
	contentWidth := 0
	if clip {
		bar = scrollbar.Column(len(m.flat), viewEnd-viewStart, viewStart)
		contentWidth = m.width - scrollbarGutterWidth
		if contentWidth < 0 {
			contentWidth = 0
		}
	}

	var b strings.Builder
	for idx := viewStart; idx < viewEnd; idx++ {
```

with:

```go
func (m Model) View() string {
	var menuItems []string
	if m.contextMenu != nil {
		menuItems = m.contextMenu.items()
	}
	realHeight := m.height
	if realHeight > 0 {
		realHeight -= len(menuItems)
	}
	clip := realHeight > 0
	viewStart, viewEnd := 0, len(m.flat)
	if clip {
		viewStart = m.scrollOffset
		if viewStart < 0 {
			viewStart = 0
		}
		if viewStart > len(m.flat) {
			viewStart = len(m.flat)
		}
		viewEnd = viewStart + realHeight
		if viewEnd > len(m.flat) {
			viewEnd = len(m.flat)
		}
	}

	var bar []rune
	contentWidth := 0
	if clip {
		bar = scrollbar.Column(len(m.flat), viewEnd-viewStart, viewStart)
		contentWidth = m.width - scrollbarGutterWidth
		if contentWidth < 0 {
			contentWidth = 0
		}
	}

	var b strings.Builder
	for _, label := range menuItems {
		b.WriteString("  [" + label + "]\n")
	}
	for idx := viewStart; idx < viewEnd; idx++ {
```

(Everything from the per-row block onward — the phantom/rename rendering added in Task 4, padding, scrollbar — is unchanged.)

- [ ] **Step 6: Run the filetree-side tests**

Run: `go test ./internal/filetree/ -run 'TestHandleRightClick|TestClickingNewFile|TestClickingOutside|TestEscClosesContext|TestRightClickWhileEditing' -v`
Expected: PASS (all six).

- [ ] **Step 7: Wire right-click routing and `FileTreeErrorMsg` into `app`**

In `internal/app/model.go`, replace the `tea.MouseMsg` case body:

```go
	case tea.MouseMsg:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		switch msg.Button {
		case tea.MouseButtonLeft:
			return m.handleClick(msg.X, msg.Y)
		case tea.MouseButtonWheelUp:
			return m.handleWheel(msg.X, msg.Y, -mouseWheelLines)
		case tea.MouseButtonWheelDown:
			return m.handleWheel(msg.X, msg.Y, mouseWheelLines)
		}
		return m, nil
```

with:

```go
	case tea.MouseMsg:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		switch msg.Button {
		case tea.MouseButtonLeft:
			return m.handleClick(msg.X, msg.Y)
		case tea.MouseButtonRight:
			return m.handleRightClick(msg.X, msg.Y)
		case tea.MouseButtonWheelUp:
			return m.handleWheel(msg.X, msg.Y, -mouseWheelLines)
		case tea.MouseButtonWheelDown:
			return m.handleWheel(msg.X, msg.Y, mouseWheelLines)
		}
		return m, nil
```

Add a new case to the `switch msg := msg.(type)` block, right after `case filetree.FileOpenedMsg:`'s block ends (before `case editor.CommandExecutedMsg:`):

```go
	case filetree.FileTreeErrorMsg:
		m.recentCommand = msg.Message
		return m, nil
```

Add `handleRightClick` anywhere in `model.go` (e.g., right after `handlePaneClick`):

```go
// handleRightClick opens the tree's context menu when the click landed in
// the tree pane; a no-op everywhere else (there's nothing to right-click
// in the editor/terminal panes in this pass) or while a dialog/dropdown is
// open.
func (m Model) handleRightClick(x, y int) (tea.Model, tea.Cmd) {
	if m.activeDialog != dialogNone || m.openMenu != "" {
		return m, nil
	}
	treeRect, _, _, _ := m.paneLayout()
	if !treeRect.contains(x, y) {
		return m, nil
	}
	m.focus = focusTree
	relY := y - treeRect.y0 - 1
	m.tree = m.tree.HandleRightClick(relY)
	return m, nil
}
```

- [ ] **Step 8: Run the app-side tests**

Run: `go test ./internal/app/ -run 'TestRightClickInTree|TestFileTreeErrorMsg' -v`
Expected: PASS (both).

- [ ] **Step 9: Run the full test suite**

Run: `go build ./... && go test ./...`
Expected: PASS across every package.

- [ ] **Step 10: Commit**

```bash
git add internal/filetree/model.go internal/filetree/model_test.go internal/app/model.go internal/app/model_test.go
git commit -m "feat: add right-click context menu (New File/Folder, Rename) to project tree"
```

---

### Task 6: `app` — New-file menu command, Ctrl+N blank tab, shared path-prompt dialog

**Files:**
- Modify: `internal/app/dialog.go`
- Modify: `internal/app/menu.go`
- Modify: `internal/app/commands.go`
- Modify: `internal/app/model.go`
- Modify: `internal/app/tabs.go`
- Modify: `internal/app/confirm.go`
- Create: `internal/app/pathprompt.go`
- Test: `internal/app/pathprompt_test.go`
- Test: `internal/app/menu_test.go`
- Test: `internal/app/tabs_test.go`

**Interfaces:**
- Consumes: `filetree.CreateFile`, `Model.ReloadDir` (Task 2), `editor.Model.NewBlankBuffer`/`IsUntitled`/`SaveAs` (Task 3).

- [ ] **Step 1: Add `dialogPathPrompt`**

In `internal/app/dialog.go`, replace:

```go
const (
	dialogNone dialogKind = iota
	dialogFileOpen
	dialogAbout
	dialogPalette
	dialogSearch
	dialogConfirmDiscard
)
```

with:

```go
const (
	dialogNone dialogKind = iota
	dialogFileOpen
	dialogAbout
	dialogPalette
	dialogSearch
	dialogConfirmDiscard
	dialogPathPrompt
)
```

- [ ] **Step 2: Add the `tabDisplayName` helper and fix "Untitled" display**

In `internal/app/tabs.go`, add this function near the top (e.g., right after the `var (...)` style block):

```go
// tabDisplayName returns the name shown for a tab: "Untitled" for a
// pathless (never-saved) tab created via Ctrl+N, filepath.Base(path)
// otherwise — filepath.Base("") returns "." which would otherwise show up
// as a nonsensical tab label.
func tabDisplayName(path string) string {
	if path == "" {
		return "Untitled"
	}
	return filepath.Base(path)
}
```

In `tabPlainLabel`, replace `name := filepath.Base(t.path)` with `name := tabDisplayName(t.path)`.

In `renderTabBar`, replace `name := filepath.Base(t.path)` with `name := tabDisplayName(t.path)`.

In `internal/app/confirm.go`'s `renderConfirmDialog`, replace:

```go
			names = append(names, filepath.Base(t.path))
```

with:

```go
			names = append(names, tabDisplayName(t.path))
```

and replace:

```go
			name = filepath.Base(m.tabs[m.pendingConfirmTab].path)
```

with:

```go
			name = tabDisplayName(m.tabs[m.pendingConfirmTab].path)
```

Those were the only two uses of `filepath` in `confirm.go` — remove `"path/filepath"` from its import block now, or `go build` will fail with "imported and not used".

- [ ] **Step 3: Add the menu item, commands, and `setActiveTabPath` helper**

In `internal/app/menu.go`, replace:

```go
	case "File":
		return []string{"Open", "Save"}
```

with:

```go
	case "File":
		return []string{"New", "Open", "Save"}
```

In `internal/app/commands.go`, replace `buildCommands`:

```go
func buildCommands() []Command {
	return []Command{
		{Name: "Open", Shortcut: "ctrl+o", Handler: cmdOpenFilePrompt},
		{Name: "Find", Shortcut: "ctrl+f", Handler: cmdFind},
		{Name: "Save", Shortcut: "ctrl+s", Handler: cmdSave},
		{Name: "Cut", Shortcut: "ctrl+x", Handler: cmdCut},
		{Name: "Copy", Shortcut: "ctrl+c", Handler: cmdCopy},
		{Name: "Paste", Shortcut: "ctrl+v", Handler: cmdPaste},
		{Name: "Undo", Shortcut: "ctrl+z", Handler: cmdUndo},
		{Name: "Redo", Shortcut: "ctrl+y", Handler: cmdRedo},
		{Name: "Toggle Fold", Shortcut: "ctrl+k", Handler: cmdToggleFold},
		{Name: "Quit", Shortcut: "ctrl+q", Handler: cmdQuit},
	}
}
```

with:

```go
func buildCommands() []Command {
	return []Command{
		{Name: "New", Handler: cmdNewFilePrompt},
		{Name: "New Tab", Shortcut: "ctrl+n", Handler: cmdNewBlankTab},
		{Name: "Open", Shortcut: "ctrl+o", Handler: cmdOpenFilePrompt},
		{Name: "Find", Shortcut: "ctrl+f", Handler: cmdFind},
		{Name: "Save", Shortcut: "ctrl+s", Handler: cmdSave},
		{Name: "Cut", Shortcut: "ctrl+x", Handler: cmdCut},
		{Name: "Copy", Shortcut: "ctrl+c", Handler: cmdCopy},
		{Name: "Paste", Shortcut: "ctrl+v", Handler: cmdPaste},
		{Name: "Undo", Shortcut: "ctrl+z", Handler: cmdUndo},
		{Name: "Redo", Shortcut: "ctrl+y", Handler: cmdRedo},
		{Name: "Toggle Fold", Shortcut: "ctrl+k", Handler: cmdToggleFold},
		{Name: "Quit", Shortcut: "ctrl+q", Handler: cmdQuit},
	}
}
```

Replace `cmdSave`:

```go
func cmdSave(m Model) (Model, tea.Cmd) {
	e, cmd := m.activeEditor().Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = m.setActiveEditor(e)
	return m, cmd
}
```

with:

```go
func cmdSave(m Model) (Model, tea.Cmd) {
	if m.activeEditor().IsUntitled() {
		return openSaveAsPrompt(m)
	}
	e, cmd := m.activeEditor().Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = m.setActiveEditor(e)
	return m, cmd
}
```

Add `cmdNewBlankTab` anywhere in `commands.go` (e.g., right after `cmdSave`):

```go
func cmdNewBlankTab(m Model) (Model, tea.Cmd) {
	editorW, editorH := m.newTabEditorSize()
	e := editor.New().NewBlankBuffer().SetSize(editorW, editorH)
	m.tabs = append(m.tabs, tab{path: "", editor: e})
	m.activeTab = len(m.tabs) - 1
	m.focus = focusEditor
	m.recentCommand = "New file"
	return m, nil
}
```

`commands.go` needs `"cody/internal/editor"` added to its import block (currently just `tea "github.com/charmbracelet/bubbletea"`):

```go
import (
	tea "github.com/charmbracelet/bubbletea"

	"cody/internal/editor"
)
```

In `internal/app/model.go`, add `setActiveTabPath` right after `setActiveEditor`:

```go
// setActiveTabPath rewrites the active tab's path — used once, by Save As,
// the moment an until-then-untitled buffer is first written to disk.
func (m Model) setActiveTabPath(path string) Model {
	if m.activeTab >= 0 && m.activeTab < len(m.tabs) {
		m.tabs[m.activeTab].path = path
	}
	return m
}
```

Replace the terminal-focus global-shortcut check in `Update`:

```go
			} else if msg.String() == "ctrl+o" || msg.String() == "ctrl+q" {
```

with:

```go
			} else if msg.String() == "ctrl+o" || msg.String() == "ctrl+q" || msg.String() == "ctrl+n" {
```

- [ ] **Step 4: Write the failing tests**

Add to `internal/app/menu_test.go`:

```go
func TestMenuItemsForFileIncludesNew(t *testing.T) {
	items := menuItemsFor("File")
	if len(items) != 3 || items[0] != "New" || items[1] != "Open" || items[2] != "Save" {
		t.Fatalf("got %v, want [New, Open, Save]", items)
	}
}
```

Add to `internal/app/tabs_test.go`:

```go
func TestTabDisplayNameForUntitledTab(t *testing.T) {
	if got := tabDisplayName(""); got != "Untitled" {
		t.Fatalf("got %q, want %q", got, "Untitled")
	}
}

func TestTabDisplayNameForRealPath(t *testing.T) {
	if got := tabDisplayName("/a/b/c.go"); got != "c.go" {
		t.Fatalf("got %q, want %q", got, "c.go")
	}
}
```

Create `internal/app/pathprompt_test.go`:

```go
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

	if len(m.tabs) != 1 {
		t.Fatalf("got %d tabs, want 1", len(m.tabs))
	}
	if m.tabs[0].path != "" {
		t.Fatalf("got path=%q, want empty (untitled)", m.tabs[0].path)
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
	if m.tabs[0].path != target {
		t.Fatalf("got tab path=%q, want %q", m.tabs[0].path, target)
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
	if len(m.tabs) != 1 || m.tabs[0].path != "" {
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
	if len(m.tabs) != 1 || m.tabs[0].path != filepath.Join(dir, "fresh.go") {
		t.Fatalf("got tabs=%v, want one tab for fresh.go", m.tabs)
	}
	if _, err := os.Stat(filepath.Join(dir, "fresh.go")); err != nil {
		t.Fatal("expected fresh.go to exist on disk")
	}
}
```

- [ ] **Step 5: Run the tests to verify they fail**

Run: `go test ./internal/app/ -run 'TestCtrlN|TestConfirmingSaveAs|TestSaveAsToExisting|TestCancelingPathPrompt|TestFileMenuNew|TestMenuItemsForFileIncludesNew|TestTabDisplayName' -v`
Expected: FAIL to compile (`dialogPathPrompt`, `pathPromptSaveAs`, `cmdNewFilePrompt`, `tabDisplayName`, etc. undefined, and/or `menuItemsFor("File")` still returning the old 2-item list).

- [ ] **Step 6: Create `internal/app/pathprompt.go`**

```go
package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cody/internal/filetree"
)

// pathPromptAction identifies what an open dialogPathPrompt dialog is
// asking the user to do — shared the same way confirm.go's confirmAction
// shares one dialog across quit and close-tab.
type pathPromptAction int

const (
	pathPromptNone pathPromptAction = iota
	pathPromptNewFile
	pathPromptSaveAs
)

func cmdNewFilePrompt(m Model) (Model, tea.Cmd) {
	return openPathPrompt(m, pathPromptNewFile), textinput.Blink
}

func openSaveAsPrompt(m Model) (Model, tea.Cmd) {
	return openPathPrompt(m, pathPromptSaveAs), textinput.Blink
}

func openPathPrompt(m Model, action pathPromptAction) Model {
	dirInput := textinput.New()
	dirInput.SetValue(m.tree.SelectedDir())
	dirInput.Focus()
	nameInput := textinput.New()
	nameInput.Placeholder = "filename.ext"

	m.pathPromptAction = action
	m.pathDirInput = dirInput
	m.pathNameInput = nameInput
	m.pathPromptFocus = 0
	m.pathPromptError = ""
	m.activeDialog = dialogPathPrompt
	return m
}

func (m Model) updatePathPromptDialog(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m.updatePathPromptInputs(msg)
	}
	switch keyMsg.String() {
	case "esc":
		m.activeDialog = dialogNone
		m.pathPromptAction = pathPromptNone
		return m, nil
	case "tab", "shift+tab":
		m.pathPromptFocus = 1 - m.pathPromptFocus
		m.pathDirInput.Blur()
		m.pathNameInput.Blur()
		if m.pathPromptFocus == 0 {
			m.pathDirInput.Focus()
		} else {
			m.pathNameInput.Focus()
		}
		return m, nil
	case "enter":
		return m.submitPathPrompt()
	}
	return m.updatePathPromptInputs(msg)
}

func (m Model) updatePathPromptInputs(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.pathPromptFocus == 0 {
		m.pathDirInput, cmd = m.pathDirInput.Update(msg)
	} else {
		m.pathNameInput, cmd = m.pathNameInput.Update(msg)
	}
	return m, cmd
}

func (m Model) submitPathPrompt() (tea.Model, tea.Cmd) {
	name := m.pathNameInput.Value()
	if name == "" {
		m.pathPromptError = "filename is required"
		return m, nil
	}
	dir := m.pathDirInput.Value()
	if dir == "" {
		dir = m.rootPath
	}
	full := name
	if !filepath.IsAbs(name) {
		full = filepath.Join(dir, name)
	}

	switch m.pathPromptAction {
	case pathPromptNewFile:
		if err := filetree.CreateFile(full); err != nil {
			m.pathPromptError = err.Error()
			return m, nil
		}
		m.tree = m.tree.ReloadDir(filepath.Dir(full))
		editorW, editorH := m.newTabEditorSize()
		updated, err := m.openOrSwitch(full, editorW, editorH)
		if err != nil {
			m.pathPromptError = err.Error()
			return m, nil
		}
		m = updated
		m.recentCommand = fmt.Sprintf("Created %s", filepath.Base(full))
	case pathPromptSaveAs:
		if _, err := os.Stat(full); err == nil {
			m.pathPromptError = fmt.Sprintf("%s already exists", filepath.Base(full))
			return m, nil
		}
		e, err := m.activeEditor().SaveAs(full)
		if err != nil {
			m.pathPromptError = err.Error()
			return m, nil
		}
		m = m.setActiveEditor(e)
		m = m.setActiveTabPath(full)
		m.tree = m.tree.ReloadDir(filepath.Dir(full))
		m.recentCommand = fmt.Sprintf("Saved %s", filepath.Base(full))
	}

	m.activeDialog = dialogNone
	m.pathPromptAction = pathPromptNone
	m.focus = focusEditor
	return m, nil
}

func renderPathPromptDialog(width, height int, m Model) string {
	title := "New File"
	if m.pathPromptAction == pathPromptSaveAs {
		title = "Save As"
	}
	content := title + "\n\n" +
		"Directory: " + m.pathDirInput.View() + "\n" +
		"Filename:  " + m.pathNameInput.View()
	if m.pathPromptError != "" {
		content += "\n\nError: " + m.pathPromptError
	}
	content += "\n\nTab to switch fields · Enter to confirm · Esc to cancel"
	return lipgloss.NewStyle().Width(width).Height(height).Padding(1, 2).Render(content)
}
```

- [ ] **Step 7: Add the path-prompt fields and wire `Update`/`View`**

In `internal/app/model.go`, add five fields to `Model` (after `confirmCursor`):

```go
	pendingConfirm    confirmAction
	pendingConfirmTab int
	confirmCursor     int

	pathPromptAction pathPromptAction
	pathDirInput     textinput.Model
	pathNameInput    textinput.Model
	pathPromptFocus  int
	pathPromptError  string
```

In `Update`, add a dialog check alongside the existing ones:

```go
	if m.activeDialog == dialogConfirmDiscard {
		return m.updateConfirmDialog(msg)
	}
```

becomes:

```go
	if m.activeDialog == dialogConfirmDiscard {
		return m.updateConfirmDialog(msg)
	}
	if m.activeDialog == dialogPathPrompt {
		return m.updatePathPromptDialog(msg)
	}
```

In `View`, add a render branch alongside the existing ones:

```go
	if m.activeDialog == dialogConfirmDiscard {
		dialog := renderConfirmDialog(m.width, paneHeight, m)
		return lipgloss.JoinVertical(lipgloss.Left, menuBar, dialog, status)
	}
```

becomes:

```go
	if m.activeDialog == dialogConfirmDiscard {
		dialog := renderConfirmDialog(m.width, paneHeight, m)
		return lipgloss.JoinVertical(lipgloss.Left, menuBar, dialog, status)
	}
	if m.activeDialog == dialogPathPrompt {
		dialog := renderPathPromptDialog(m.width, paneHeight, m)
		return lipgloss.JoinVertical(lipgloss.Left, menuBar, dialog, status)
	}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test ./internal/app/ -run 'TestCtrlN|TestConfirmingSaveAs|TestSaveAsToExisting|TestCancelingPathPrompt|TestFileMenuNew|TestMenuItemsForFileIncludesNew|TestTabDisplayName' -v`
Expected: PASS (all).

- [ ] **Step 9: Run the full test suite**

Run: `go build ./... && go test ./...`
Expected: PASS across every package.

- [ ] **Step 10: Commit**

```bash
git add internal/app/dialog.go internal/app/menu.go internal/app/commands.go \
        internal/app/model.go internal/app/tabs.go internal/app/confirm.go \
        internal/app/pathprompt.go internal/app/pathprompt_test.go \
        internal/app/menu_test.go internal/app/tabs_test.go
git commit -m "feat: add File > New dialog, Ctrl+N blank tab, and Save As"
```

---

## Final check

After Task 6, run the full suite one more time from the repo root:

```bash
go build ./... && go test ./...
```

All packages must pass. This completes the plan — proceed to the final
whole-branch review per subagent-driven-development.
