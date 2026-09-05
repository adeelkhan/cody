# Cody Phase 1: Skeleton Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a working, demoable skeleton of Cody: launch via `cody <path>`, see the full split-pane layout, browse the project tree, open and plain-text-edit a file, save it, and see status-bar feedback — with no menu bar interactivity, no tree-sitter highlighting, and no real terminal yet.

**Architecture:** A root Bubble Tea model (`internal/app`) owns two focusable sub-models — `internal/filetree` (tree walk + navigation) and `internal/editor` (line-buffer + cursor) — plus a pure-function `internal/statusbar` renderer. Sub-models communicate upward via custom `tea.Msg` types (`filetree.FileOpenedMsg`, `editor.CommandExecutedMsg`); the root model routes key/window messages to whichever pane has focus and reacts to those custom messages itself. This mirrors the command-registry-free early phase of the design spec — the registry itself arrives in Phase 2.

**Tech Stack:** Go, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`. No CGO/tree-sitter/PTY dependencies in this phase.

**Spec:** `docs/superpowers/specs/2026-09-05-cody-tui-editor-design.md`

## Global Constraints

- Module name is `cody`; all internal packages import as `cody/internal/...`.
- Keybindings are `Ctrl`-based only in this phase (`ctrl+q` quit, `ctrl+s` save) — no menu bar, no Cmd-key handling, no command registry yet (that's Phase 2 per the spec's phased build order).
- The project tree always skips `.git` and lazily loads directory children on expand (spec §5).
- The editor buffer is line-based (`[]string`), not a rope or piece-table (spec §6). No syntax highlighting, folding, undo/redo, or search in this phase — those are later phases.
- Cut/Copy/Paste, the terminal pane, and the menu bar are explicitly **out of scope** for this plan; the terminal area renders as a static placeholder box only.
- Every new package gets unit tests alongside the code that introduces it; TDD order (failing test → implementation → passing test) applies to every task below.

---

### Task 1: Project scaffolding and CLI path resolution

**Files:**
- Create: `go.mod`
- Create: `internal/app/path.go`
- Test: `internal/app/path_test.go`

**Interfaces:**
- Produces: `func ResolveProjectPath(args []string) (string, error)` in package `cody/internal/app` — returns the directory to open (defaults to `"."` when `args` is empty), or an error if the path doesn't exist or isn't a directory.

- [ ] **Step 1: Initialize the Go module and fetch dependencies**

```bash
go mod init cody
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
```

- [ ] **Step 2: Write the failing test**

Create `internal/app/path_test.go`:

```go
package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProjectPathDefaultsToCwd(t *testing.T) {
	path, err := ResolveProjectPath(nil)
	if err != nil {
		t.Fatal(err)
	}
	if path != "." {
		t.Fatalf("got %q, want \".\"", path)
	}
}

func TestResolveProjectPathValidDir(t *testing.T) {
	dir := t.TempDir()
	path, err := ResolveProjectPath([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if path != dir {
		t.Fatalf("got %q, want %q", path, dir)
	}
}

func TestResolveProjectPathMissing(t *testing.T) {
	_, err := ResolveProjectPath([]string{"/does/not/exist"})
	if err == nil {
		t.Fatal("expected an error for a missing path")
	}
}

func TestResolveProjectPathNotADir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveProjectPath([]string{file})
	if err == nil {
		t.Fatal("expected an error for a non-directory path")
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/app/... -run TestResolveProjectPath -v`
Expected: FAIL — `ResolveProjectPath` is undefined.

- [ ] **Step 4: Write the implementation**

Create `internal/app/path.go`:

```go
package app

import (
	"fmt"
	"os"
)

func ResolveProjectPath(args []string) (string, error) {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("cody: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("cody: %s is not a directory", path)
	}
	return path, nil
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/app/... -run TestResolveProjectPath -v`
Expected: PASS (all 4 subtests)

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/app/path.go internal/app/path_test.go
git commit -m "feat: add module scaffolding and CLI path resolution"
```

---

### Task 2: File tree data model (walk, lazy-load, icons)

**Files:**
- Create: `internal/filetree/tree.go`
- Create: `internal/filetree/icons.go`
- Test: `internal/filetree/tree_test.go`
- Test: `internal/filetree/icons_test.go`

**Interfaces:**
- Produces (package `cody/internal/filetree`):
  - `type NodeType int` with constants `NodeFile`, `NodeDir`
  - `type Node struct { Name, Path string; Type NodeType; Expanded bool; Children []*Node }` (plus an unexported `loaded bool`)
  - `func NewRoot(rootPath string) (*Node, error)` — stats the path, builds the root node, and calls `LoadChildren` once
  - `func (n *Node) LoadChildren() error` — populates `n.Children` for a directory node (idempotent; skips `.git`; directories sorted before files, then alphabetically)
  - `func IconFor(n *Node, nerdFont bool) string`

- [ ] **Step 1: Write the failing tests**

Create `internal/filetree/tree_test.go`:

```go
package filetree

import (
	"os"
	"path/filepath"
	"testing"
)

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestNewRootSkipsGit(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, ".git"))
	mustMkdir(t, filepath.Join(dir, "src"))
	mustWriteFile(t, filepath.Join(dir, "main.go"), "package main")

	root, err := NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range root.Children {
		if c.Name == ".git" {
			t.Fatal(".git should be skipped")
		}
	}
	if len(root.Children) != 2 {
		t.Fatalf("got %d children, want 2", len(root.Children))
	}
}

func TestNewRootSortsDirsFirst(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "b.go"), "")
	mustMkdir(t, filepath.Join(dir, "a-dir"))

	root, err := NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if root.Children[0].Name != "a-dir" || root.Children[0].Type != NodeDir {
		t.Fatalf("expected dir first, got %+v", root.Children[0])
	}
}

func TestLoadChildrenLazy(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	mustMkdir(t, sub)
	mustWriteFile(t, filepath.Join(sub, "f.txt"), "")

	root, err := NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	subNode := root.Children[0]
	if subNode.Children != nil {
		t.Fatal("expected children not loaded until LoadChildren is called")
	}
	if err := subNode.LoadChildren(); err != nil {
		t.Fatal(err)
	}
	if len(subNode.Children) != 1 {
		t.Fatalf("got %d children, want 1", len(subNode.Children))
	}
}
```

Create `internal/filetree/icons_test.go`:

```go
package filetree

import "testing"

func TestIconForDir(t *testing.T) {
	n := &Node{Type: NodeDir}
	if IconFor(n, true) != nerdFontDir {
		t.Fatal("expected nerd font dir icon")
	}
	if IconFor(n, false) != fallbackDir {
		t.Fatal("expected fallback dir icon")
	}
}

func TestIconForKnownExtension(t *testing.T) {
	n := &Node{Type: NodeFile, Name: "main.go"}
	if IconFor(n, true) != nerdFontIcons[".go"] {
		t.Fatal("expected go icon")
	}
}

func TestIconForUnknownExtension(t *testing.T) {
	n := &Node{Type: NodeFile, Name: "data.bin"}
	if IconFor(n, true) != nerdFontFile {
		t.Fatal("expected generic file icon")
	}
	if IconFor(n, false) != fallbackFile {
		t.Fatal("expected fallback file icon")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/filetree/... -v`
Expected: FAIL to compile — none of `Node`, `NewRoot`, `IconFor`, etc. exist yet.

- [ ] **Step 3: Write the implementation**

Create `internal/filetree/tree.go`:

```go
package filetree

import (
	"os"
	"path/filepath"
	"sort"
)

type NodeType int

const (
	NodeFile NodeType = iota
	NodeDir
)

type Node struct {
	Name     string
	Path     string
	Type     NodeType
	Expanded bool
	Children []*Node
	loaded   bool
}

func NewRoot(rootPath string) (*Node, error) {
	info, err := os.Stat(rootPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &os.PathError{Op: "newroot", Path: rootPath, Err: os.ErrInvalid}
	}
	root := &Node{
		Name:     filepath.Base(rootPath),
		Path:     rootPath,
		Type:     NodeDir,
		Expanded: true,
	}
	if err := root.LoadChildren(); err != nil {
		return nil, err
	}
	return root, nil
}

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

Create `internal/filetree/icons.go`:

```go
package filetree

import "path/filepath"

var nerdFontIcons = map[string]string{
	".go":   "",
	".py":   "",
	".js":   "",
	".ts":   "",
	".json": "",
	".md":   "",
}

const (
	nerdFontDir  = ""
	nerdFontFile = ""
	fallbackDir  = "+"
	fallbackFile = "-"
)

func IconFor(n *Node, nerdFont bool) string {
	if n.Type == NodeDir {
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

Run: `go test ./internal/filetree/... -v`
Expected: PASS (all tests)

- [ ] **Step 5: Commit**

```bash
git add internal/filetree/tree.go internal/filetree/icons.go internal/filetree/tree_test.go internal/filetree/icons_test.go
git commit -m "feat: add file tree walking, lazy loading, and icon selection"
```

---

### Task 3: File tree Bubble Tea model

**Files:**
- Create: `internal/filetree/model.go`
- Test: `internal/filetree/model_test.go`

**Interfaces:**
- Consumes: `NewRoot`, `Node`, `IconFor` from Task 2 (same package, no import needed).
- Produces (package `cody/internal/filetree`):
  - `type FileOpenedMsg struct { Path string }`
  - `func New(rootPath string, nerdFont bool) (Model, error)`
  - `func (m Model) SetSize(width, height int) Model`
  - `func (m Model) Update(msg tea.Msg) (Model, tea.Cmd)`
  - `func (m Model) View() string`

- [ ] **Step 1: Write the failing tests**

Create `internal/filetree/model_test.go`:

```go
package filetree

import (
	"os"
	"path/filepath"
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/filetree/... -run 'TestCursor|TestEnter' -v`
Expected: FAIL to compile — `Model`, `New`, `FileOpenedMsg` don't exist yet.

- [ ] **Step 3: Write the implementation**

Create `internal/filetree/model.go`:

```go
package filetree

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type FileOpenedMsg struct {
	Path string
}

type flatItem struct {
	node  *Node
	depth int
}

type Model struct {
	root     *Node
	nerdFont bool
	flat     []flatItem
	cursor   int
	width    int
	height   int
}

func New(rootPath string, nerdFont bool) (Model, error) {
	root, err := NewRoot(rootPath)
	if err != nil {
		return Model{}, err
	}
	m := Model{root: root, nerdFont: nerdFont}
	m.rebuildFlat()
	return m, nil
}

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

func (m Model) SetSize(width, height int) Model {
	m.width, m.height = width, height
	return m
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.flat)-1 {
			m.cursor++
		}
	case "left", "h":
		m.collapseCurrent()
	case "right", "l", "enter":
		return m.activateCurrent()
	}
	return m, nil
}

func (m *Model) collapseCurrent() {
	if len(m.flat) == 0 {
		return
	}
	n := m.flat[m.cursor].node
	if n.Type == NodeDir && n.Expanded {
		n.Expanded = false
		m.rebuildFlat()
	}
}

func (m Model) activateCurrent() (Model, tea.Cmd) {
	if len(m.flat) == 0 {
		return m, nil
	}
	n := m.flat[m.cursor].node
	if n.Type == NodeDir {
		if !n.Expanded {
			n.Expanded = true
			if err := n.LoadChildren(); err != nil {
				return m, nil
			}
		} else {
			n.Expanded = false
		}
		m.rebuildFlat()
		return m, nil
	}
	path := n.Path
	return m, func() tea.Msg { return FileOpenedMsg{Path: path} }
}

func (m Model) View() string {
	var b strings.Builder
	for i, item := range m.flat {
		prefix := strings.Repeat("  ", item.depth)
		icon := IconFor(item.node, m.nerdFont)
		line := fmt.Sprintf("%s%s %s", prefix, icon, item.node.Name)
		if i == m.cursor {
			line = "> " + line
		} else {
			line = "  " + line
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/filetree/... -v`
Expected: PASS (all tests in the package)

- [ ] **Step 5: Commit**

```bash
git add internal/filetree/model.go internal/filetree/model_test.go
git commit -m "feat: add file tree Bubble Tea model with keyboard navigation"
```

---

### Task 4: Editor line buffer

**Files:**
- Create: `internal/editor/buffer.go`
- Test: `internal/editor/buffer_test.go`

**Interfaces:**
- Produces (package `cody/internal/editor`):
  - `type Buffer struct { Lines []string; Path string; Dirty bool }`
  - `func NewBuffer(path string) (*Buffer, error)`
  - `func (b *Buffer) InsertRune(line, col int, r rune)`
  - `func (b *Buffer) InsertNewline(line, col int)`
  - `func (b *Buffer) DeleteBefore(line, col int) (newLine, newCol int)`
  - `func (b *Buffer) Save() error`

- [ ] **Step 1: Write the failing tests**

Create `internal/editor/buffer_test.go`:

```go
package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewBuffer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello\nworld"), 0644); err != nil {
		t.Fatal(err)
	}
	buf, err := NewBuffer(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"hello", "world"}
	if len(buf.Lines) != len(want) || buf.Lines[0] != want[0] || buf.Lines[1] != want[1] {
		t.Fatalf("got %v, want %v", buf.Lines, want)
	}
}

func TestInsertRune(t *testing.T) {
	buf := &Buffer{Lines: []string{"helo"}}
	buf.InsertRune(0, 3, 'l')
	if buf.Lines[0] != "hello" {
		t.Fatalf("got %q, want %q", buf.Lines[0], "hello")
	}
	if !buf.Dirty {
		t.Fatal("expected Dirty to be true")
	}
}

func TestInsertNewline(t *testing.T) {
	buf := &Buffer{Lines: []string{"helloworld"}}
	buf.InsertNewline(0, 5)
	want := []string{"hello", "world"}
	if len(buf.Lines) != 2 || buf.Lines[0] != want[0] || buf.Lines[1] != want[1] {
		t.Fatalf("got %v, want %v", buf.Lines, want)
	}
}

func TestDeleteBeforeSameLine(t *testing.T) {
	buf := &Buffer{Lines: []string{"hello"}}
	line, col := buf.DeleteBefore(0, 5)
	if buf.Lines[0] != "hell" || line != 0 || col != 4 {
		t.Fatalf("got line=%d col=%d text=%q", line, col, buf.Lines[0])
	}
}

func TestDeleteBeforeJoinsLines(t *testing.T) {
	buf := &Buffer{Lines: []string{"hello", "world"}}
	line, col := buf.DeleteBefore(1, 0)
	want := "helloworld"
	if len(buf.Lines) != 1 || buf.Lines[0] != want || line != 0 || col != 5 {
		t.Fatalf("got line=%d col=%d lines=%v", line, col, buf.Lines)
	}
}

func TestSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	buf := &Buffer{Lines: []string{"a", "b"}, Path: path, Dirty: true}
	if err := buf.Save(); err != nil {
		t.Fatal(err)
	}
	if buf.Dirty {
		t.Fatal("expected Dirty to be false after save")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "a\nb" {
		t.Fatalf("got %q", string(data))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/editor/... -v`
Expected: FAIL to compile — `Buffer`, `NewBuffer`, etc. don't exist yet.

- [ ] **Step 3: Write the implementation**

Create `internal/editor/buffer.go`:

```go
package editor

import (
	"os"
	"strings"
)

type Buffer struct {
	Lines []string
	Path  string
	Dirty bool
}

func NewBuffer(path string) (*Buffer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	return &Buffer{Lines: lines, Path: path}, nil
}

func (b *Buffer) InsertRune(line, col int, r rune) {
	l := []rune(b.Lines[line])
	l = append(l[:col], append([]rune{r}, l[col:]...)...)
	b.Lines[line] = string(l)
	b.Dirty = true
}

func (b *Buffer) InsertNewline(line, col int) {
	l := []rune(b.Lines[line])
	before := string(l[:col])
	after := string(l[col:])
	b.Lines[line] = before
	tail := append([]string{after}, b.Lines[line+1:]...)
	b.Lines = append(b.Lines[:line+1], tail...)
	b.Dirty = true
}

func (b *Buffer) DeleteBefore(line, col int) (int, int) {
	if col > 0 {
		l := []rune(b.Lines[line])
		l = append(l[:col-1], l[col:]...)
		b.Lines[line] = string(l)
		b.Dirty = true
		return line, col - 1
	}
	if line == 0 {
		return 0, 0
	}
	prevLen := len([]rune(b.Lines[line-1]))
	b.Lines[line-1] += b.Lines[line]
	b.Lines = append(b.Lines[:line], b.Lines[line+1:]...)
	b.Dirty = true
	return line - 1, prevLen
}

func (b *Buffer) Save() error {
	data := strings.Join(b.Lines, "\n")
	if err := os.WriteFile(b.Path, []byte(data), 0644); err != nil {
		return err
	}
	b.Dirty = false
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/editor/... -v`
Expected: PASS (all tests)

- [ ] **Step 5: Commit**

```bash
git add internal/editor/buffer.go internal/editor/buffer_test.go
git commit -m "feat: add line-based editor buffer with edit and save operations"
```

---

### Task 5: Editor Bubble Tea model

**Files:**
- Create: `internal/editor/model.go`
- Test: `internal/editor/model_test.go`

**Interfaces:**
- Consumes: `Buffer`, `NewBuffer`, `InsertRune`, `InsertNewline`, `DeleteBefore`, `Save` from Task 4 (same package).
- Produces (package `cody/internal/editor`):
  - `type CommandExecutedMsg struct { Description string }`
  - `func New() Model`
  - `func (m Model) LoadFile(path string) (Model, error)`
  - `func (m Model) SetSize(width, height int) Model`
  - `func (m Model) HasBuffer() bool`
  - `func (m Model) Cursor() (line, col int)` — 1-indexed, for direct display in the status bar
  - `func (m Model) Filetype() string` — file extension without the leading dot, or `""` if no buffer is loaded
  - `func (m Model) Update(msg tea.Msg) (Model, tea.Cmd)`
  - `func (m Model) View() string`

- [ ] **Step 1: Write the failing tests**

Create `internal/editor/model_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/editor/... -run 'TestTyping|TestEnterSplits|TestCtrlS|TestFiletype' -v`
Expected: FAIL to compile — `Model`, `New`, `CommandExecutedMsg` don't exist yet.

- [ ] **Step 3: Write the implementation**

Create `internal/editor/model.go`:

```go
package editor

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type CommandExecutedMsg struct {
	Description string
}

type Model struct {
	buf        *Buffer
	cursorLine int
	cursorCol  int
	width      int
	height     int
}

func New() Model {
	return Model{}
}

func (m Model) LoadFile(path string) (Model, error) {
	buf, err := NewBuffer(path)
	if err != nil {
		return m, err
	}
	m.buf = buf
	m.cursorLine = 0
	m.cursorCol = 0
	return m, nil
}

func (m Model) SetSize(width, height int) Model {
	m.width, m.height = width, height
	return m
}

func (m Model) HasBuffer() bool {
	return m.buf != nil
}

func (m Model) Cursor() (line, col int) {
	return m.cursorLine + 1, m.cursorCol + 1
}

func (m Model) Filetype() string {
	if m.buf == nil {
		return ""
	}
	return strings.TrimPrefix(filepath.Ext(m.buf.Path), ".")
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if m.buf == nil {
		return m, nil
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "up":
		m.moveUp()
	case "down":
		m.moveDown()
	case "left":
		m.moveLeft()
	case "right":
		m.moveRight()
	case "enter":
		m.buf.InsertNewline(m.cursorLine, m.cursorCol)
		m.cursorLine++
		m.cursorCol = 0
	case "backspace":
		m.cursorLine, m.cursorCol = m.buf.DeleteBefore(m.cursorLine, m.cursorCol)
	case "ctrl+s":
		desc := m.save()
		return m, func() tea.Msg { return CommandExecutedMsg{Description: desc} }
	default:
		if keyMsg.Type == tea.KeyRunes {
			for _, r := range keyMsg.Runes {
				m.buf.InsertRune(m.cursorLine, m.cursorCol, r)
				m.cursorCol++
			}
		}
	}
	return m, nil
}

func (m *Model) moveUp() {
	if m.cursorLine > 0 {
		m.cursorLine--
		m.clampCol()
	}
}

func (m *Model) moveDown() {
	if m.cursorLine < len(m.buf.Lines)-1 {
		m.cursorLine++
		m.clampCol()
	}
}

func (m *Model) moveLeft() {
	if m.cursorCol > 0 {
		m.cursorCol--
	}
}

func (m *Model) moveRight() {
	if m.cursorCol < len([]rune(m.buf.Lines[m.cursorLine])) {
		m.cursorCol++
	}
}

func (m *Model) clampCol() {
	lineLen := len([]rune(m.buf.Lines[m.cursorLine]))
	if m.cursorCol > lineLen {
		m.cursorCol = lineLen
	}
}

func (m *Model) save() string {
	if err := m.buf.Save(); err != nil {
		return fmt.Sprintf("Save failed: %s", err)
	}
	return fmt.Sprintf("Saved %s", filepath.Base(m.buf.Path))
}

func (m Model) View() string {
	if m.buf == nil {
		return "Select a file to begin"
	}
	var b strings.Builder
	for i, line := range m.buf.Lines {
		cursorMark := "  "
		if i == m.cursorLine {
			cursorMark = "> "
		}
		b.WriteString(fmt.Sprintf("%s%4d %s\n", cursorMark, i+1, line))
	}
	return b.String()
}
```

Note: `moveUp`/`moveDown`/etc. use pointer receivers but are called on the local `m Model` value inside `Update` — this compiles because a local variable is addressable, so Go automatically takes its address. No line-wrapping or vertical scrolling is implemented in this phase; very long files or lines will just extend past the visible pane. That's an accepted skeleton-phase limitation, not a bug to fix here.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/editor/... -v`
Expected: PASS (all tests)

- [ ] **Step 5: Commit**

```bash
git add internal/editor/model.go internal/editor/model_test.go
git commit -m "feat: add editor Bubble Tea model with cursor movement and save"
```

---

### Task 6: Status bar renderer

**Files:**
- Create: `internal/statusbar/statusbar.go`
- Test: `internal/statusbar/statusbar_test.go`

**Interfaces:**
- Produces: `func Render(width int, projectName, recentCommand, filetype string, line, col int) string` in package `cody/internal/statusbar`.

- [ ] **Step 1: Write the failing test**

Create `internal/statusbar/statusbar_test.go`:

```go
package statusbar

import (
	"strings"
	"testing"
)

func TestRenderIncludesAllSegments(t *testing.T) {
	out := Render(80, "myproject", "Saved main.go", "go", 3, 7)
	for _, want := range []string{"myproject", "Saved main.go", "Ln 3, Col 7", "go"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/statusbar/... -v`
Expected: FAIL to compile — `Render` doesn't exist yet.

- [ ] **Step 3: Write the implementation**

Create `internal/statusbar/statusbar.go`:

```go
package statusbar

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var style = lipgloss.NewStyle().
	Background(lipgloss.Color("62")).
	Foreground(lipgloss.Color("230"))

func Render(width int, projectName, recentCommand, filetype string, line, col int) string {
	left := projectName
	right := fmt.Sprintf("%s  Ln %d, Col %d  %s", recentCommand, line, col, filetype)
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return style.Width(width).Render(left + spaces(gap) + right)
}

func spaces(n int) string {
	if n < 0 {
		n = 0
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/statusbar/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/statusbar/statusbar.go internal/statusbar/statusbar_test.go
git commit -m "feat: add status bar renderer"
```

---

### Task 7: Root app model wiring the panes together, plus main.go

**Files:**
- Create: `internal/app/model.go`
- Test: `internal/app/model_test.go`
- Create: `cmd/cody/main.go`

**Interfaces:**
- Consumes:
  - `ResolveProjectPath` from Task 1
  - `filetree.New`, `filetree.Model`, `filetree.FileOpenedMsg` from Tasks 2-3
  - `editor.New`, `editor.Model`, `editor.CommandExecutedMsg` from Tasks 4-5
  - `statusbar.Render` from Task 6
- Produces: `func New(rootPath string, nerdFont bool) (Model, error)` in package `cody/internal/app`, where `Model` implements `tea.Model` (`Init() tea.Cmd`, `Update(tea.Msg) (tea.Model, tea.Cmd)`, `View() string`).

- [ ] **Step 1: Write the failing tests**

Create `internal/app/model_test.go`:

```go
package app

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"cody/internal/filetree"
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/... -run 'TestTab|TestFileOpened|TestCtrlQ' -v`
Expected: FAIL to compile — `Model`, `New`, `focusTree`, `focusEditor` don't exist yet.

- [ ] **Step 3: Write the implementation**

Create `internal/app/model.go`:

```go
package app

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cody/internal/editor"
	"cody/internal/filetree"
	"cody/internal/statusbar"
)

type focusArea int

const (
	focusTree focusArea = iota
	focusEditor
)

const (
	treeWidth       = 30
	statusBarHeight = 1
	menuBarHeight   = 1
	terminalHeight  = 8
)

type Model struct {
	tree          filetree.Model
	editor        editor.Model
	focus         focusArea
	projectName   string
	recentCommand string
	width, height int
}

func New(rootPath string, nerdFont bool) (Model, error) {
	tree, err := filetree.New(rootPath, nerdFont)
	if err != nil {
		return Model{}, err
	}
	absPath, err := filepath.Abs(rootPath)
	if err != nil {
		return Model{}, err
	}
	return Model{
		tree:        tree,
		editor:      editor.New(),
		focus:       focusTree,
		projectName: filepath.Base(absPath),
	}, nil
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		paneHeight := m.height - menuBarHeight - statusBarHeight
		editorHeight := paneHeight - terminalHeight
		m.tree = m.tree.SetSize(treeWidth, paneHeight)
		m.editor = m.editor.SetSize(m.width-treeWidth, editorHeight)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+q":
			return m, tea.Quit
		case "tab", "shift+tab":
			m.toggleFocus()
			return m, nil
		}
	case filetree.FileOpenedMsg:
		editorModel, err := m.editor.LoadFile(msg.Path)
		if err != nil {
			m.recentCommand = fmt.Sprintf("Open failed: %s", err)
			return m, nil
		}
		m.editor = editorModel
		m.focus = focusEditor
		m.recentCommand = fmt.Sprintf("Opened %s", filepath.Base(msg.Path))
		return m, nil
	case editor.CommandExecutedMsg:
		m.recentCommand = msg.Description
		return m, nil
	}

	var cmd tea.Cmd
	if m.focus == focusTree {
		m.tree, cmd = m.tree.Update(msg)
	} else {
		m.editor, cmd = m.editor.Update(msg)
	}
	return m, cmd
}

func (m *Model) toggleFocus() {
	if m.focus == focusTree {
		m.focus = focusEditor
	} else {
		m.focus = focusTree
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}
	menuBar := lipgloss.NewStyle().Width(m.width).Render("File  Edit  Commands  About")

	paneHeight := m.height - menuBarHeight - statusBarHeight
	editorHeight := paneHeight - terminalHeight

	treeStyle := lipgloss.NewStyle().Width(treeWidth).Height(paneHeight)
	editorStyle := lipgloss.NewStyle().Width(m.width - treeWidth).Height(editorHeight)
	terminalStyle := lipgloss.NewStyle().Width(m.width - treeWidth).Height(terminalHeight)

	right := lipgloss.JoinVertical(lipgloss.Left,
		editorStyle.Render(m.editor.View()),
		terminalStyle.Render("Terminal (coming in a later phase)"),
	)
	body := lipgloss.JoinHorizontal(lipgloss.Top, treeStyle.Render(m.tree.View()), right)

	line, col := m.editor.Cursor()
	status := statusbar.Render(m.width, m.projectName, m.recentCommand, m.editor.Filetype(), line, col)

	return lipgloss.JoinVertical(lipgloss.Left, menuBar, body, status)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/app/... -v`
Expected: PASS (all tests)

- [ ] **Step 5: Write `cmd/cody/main.go`**

```go
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"cody/internal/app"
)

func main() {
	path, err := app.ResolveProjectPath(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	model, err := app.New(path, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 6: Build the whole module**

Run: `go build ./...`
Expected: builds with no errors.

- [ ] **Step 7: Commit**

```bash
git add internal/app/model.go internal/app/model_test.go cmd/cody/main.go
git commit -m "feat: wire root app model and add cody CLI entrypoint"
```

---

### Task 8: Smoke-test the running app and add a README

**Files:**
- Create: `README.md`

Bubble Tea's alt-screen program can't be driven by piping stdin through `bash` directly, so this task uses `tmux` to launch the real binary, send it keystrokes, and capture what it rendered — a standard way to smoke-test a full-screen TUI non-interactively.

- [ ] **Step 1: Build the binary**

```bash
go build -o /tmp/cody-smoketest ./cmd/cody
```

- [ ] **Step 2: Launch it in a detached tmux session against this repo**

```bash
tmux kill-session -t cody-smoketest 2>/dev/null
tmux new-session -d -s cody-smoketest -x 100 -y 30 "/tmp/cody-smoketest ."
sleep 1
tmux capture-pane -t cody-smoketest -p
```

Expected: a rendered frame showing the `File  Edit  Commands  About` menu bar, the project's own file/directory listing on the left (e.g. `internal`, `cmd`, `go.mod`), an empty editor pane with "Select a file to begin", the terminal placeholder text, and a status bar at the bottom.

- [ ] **Step 3: Navigate into `cmd`, open `main.go`, and confirm it renders**

```bash
tmux send-keys -t cody-smoketest Down Down Enter
sleep 0.3
tmux capture-pane -t cody-smoketest -p
```

Expected: the tree now shows `cmd` expanded with `cody` under it (adjust arrow-key count to reach `main.go`'s directory/file in the sorted listing), and once a `.go` file is opened the editor pane shows its contents with line numbers and the status bar's filetype segment reads `go`.

- [ ] **Step 4: Confirm typing and save work**

```bash
tmux send-keys -t cody-smoketest "// smoke test"
sleep 0.2
tmux send-keys -t cody-smoketest C-s
sleep 0.2
tmux capture-pane -t cody-smoketest -p
```

Expected: the typed text appears in the editor pane, and the status bar's recent-command segment reads `Saved main.go`.

**Important:** this actually writes to the opened file on disk. Run this step against a scratch copy of the project, not a file you care about keeping unmodified — after confirming, use `git checkout -- <file>` (or just don't commit the change) to discard the smoke-test edit.

- [ ] **Step 5: Confirm quit works and clean up**

```bash
tmux send-keys -t cody-smoketest C-q
sleep 0.3
tmux has-session -t cody-smoketest 2>/dev/null && echo "still running (unexpected)" || echo "exited cleanly"
tmux kill-session -t cody-smoketest 2>/dev/null
rm -f /tmp/cody-smoketest
```

Expected: `exited cleanly`.

- [ ] **Step 6: Discard the smoke-test edit to `main.go`, if any was left uncommitted**

```bash
git status
git checkout -- cmd/cody/main.go
```

- [ ] **Step 7: Write the README**

Create `README.md`:

```markdown
# Cody

A terminal-based code editor written in Go.

## Status

Phase 1 (skeleton) of a phased build — see
`docs/superpowers/specs/2026-09-05-cody-tui-editor-design.md` for the full
design and `docs/superpowers/plans/` for phase-by-phase implementation plans.

This phase has: a split-pane layout (project tree, editor, terminal
placeholder, status bar), file tree browsing, and plain-text file
editing/saving. It does not yet have: the interactive menu bar, syntax
highlighting/folding, an embedded shell, in-buffer search, or undo/redo —
those land in later phases.

## Build & run

```bash
go build -o cody ./cmd/cody
./cody <path-to-a-project>
```

## Keybindings (phase 1)

- `Tab` / `Shift+Tab` — switch focus between the project tree and the editor
- Project tree: arrows or `hjkl` to navigate, `Enter`/`l` to open a file or
  expand a directory, `h` to collapse
- Editor: arrows to move the cursor, typing inserts text, `Enter` for a
  newline, `Backspace` to delete, `Ctrl+S` to save
- `Ctrl+Q` — quit
```

- [ ] **Step 8: Commit**

```bash
git add README.md
git commit -m "docs: add README covering phase 1 build/run/keybindings"
```
