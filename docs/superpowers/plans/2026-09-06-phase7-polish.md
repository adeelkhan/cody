# Phase 7 (Polish) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the `--no-nerd-font` CLI flag and ship a final user-facing README that reflects all six prior phases.

**Architecture:** Two independent tasks — flag plumbing (one file change), then README + version string polish (prose + one line change). No new packages, no new interfaces.

**Tech Stack:** Go standard `flag` package; Lip Gloss; Bubble Tea. All internal API already accepts `nerdFont bool`.

**Spec:** docs/superpowers/specs/2026-09-05-cody-tui-editor-design.md (§5 icons/flag, §8 macOS caveat, §12 Phase 7)

## Global Constraints

- Go module path: `cody`
- All existing tests must continue to pass (`go test ./...`)
- `gofmt` clean; `go vet` clean
- No new dependencies
- Do not change any package API signatures
- The `--no-nerd-font` flag must use Go's standard `flag` package — no third-party CLI libraries
- README must not mention "Phase N of a phased build" in its opening — it should read as a released product
- Commit each task separately

---

### Task 1: Wire `--no-nerd-font` CLI flag

**Files:**
- Modify: `cmd/cody/main.go`

**Context:**
- `internal/app.New(path string, nerdFont bool)` already accepts the flag and passes it to `filetree.New`.
- `internal/app.ResolveProjectPath(args []string)` takes positional args (no flags).
- `main.go` currently hard-codes `app.New(path, true)`.

**Interfaces:**
- Consumes: `app.New(path, nerdFont)` — no change to this signature
- Produces: binary now accepts `--no-nerd-font` boolean flag

- [ ] **Step 1: Write the failing test**

`cmd/cody` has no test file. Create `cmd/cody/main_test.go` with a flag-parsing test. Because `main()` calls `os.Exit`, test the flag value extraction in isolation — extract a helper `parseArgs(args []string) (path string, nerdFont bool, err error)` and test that.

```go
package main

import (
	"testing"
)

func TestParseArgsDefaults(t *testing.T) {
	path, nerdFont, err := parseArgs([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "." {
		t.Errorf("path = %q, want %q", path, ".")
	}
	if !nerdFont {
		t.Error("nerdFont should default to true")
	}
}

func TestParseArgsNoNerdFont(t *testing.T) {
	path, nerdFont, err := parseArgs([]string{"--no-nerd-font", "/tmp"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nerdFont {
		t.Error("nerdFont should be false when --no-nerd-font is passed")
	}
	_ = path // path resolution tested in app package
}

func TestParseArgsNerdFontDefaultWithPath(t *testing.T) {
	_, nerdFont, err := parseArgs([]string{"/tmp"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !nerdFont {
		t.Error("nerdFont should default to true when no flag given")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./cmd/cody/... -v
```
Expected: compile error (parseArgs not defined yet).

- [ ] **Step 3: Implement `parseArgs` and wire into `main`**

Replace `cmd/cody/main.go` with:

```go
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"cody/internal/app"
)

func parseArgs(args []string) (path string, nerdFont bool, err error) {
	fs := flag.NewFlagSet("cody", flag.ContinueOnError)
	noNerdFont := fs.Bool("no-nerd-font", false, "use plain ASCII/Unicode icons instead of Nerd Font glyphs")
	if err = fs.Parse(args); err != nil {
		return "", false, err
	}
	path = "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}
	return path, !*noNerdFont, nil
}

func main() {
	rawPath, nerdFont, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	path, err := app.ResolveProjectPath([]string{rawPath})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	model, err := app.New(path, nerdFont)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

Note: `parseArgs([]string{})` passes `"."` to `ResolveProjectPath` — the same default `ResolveProjectPath` applies when given a zero-length slice. No change to `ResolveProjectPath`.

- [ ] **Step 4: Run tests to verify they pass**

```
go test ./cmd/cody/... -v
go test ./... -count=1
go build ./...
go vet ./...
```
All must pass.

- [ ] **Step 5: Commit**

```bash
git add cmd/cody/main.go cmd/cody/main_test.go
git commit -m "feat: add --no-nerd-font CLI flag to force ASCII icon fallback"
```

---

### Task 2: README polish and About dialog version bump

**Files:**
- Modify: `README.md`
- Modify: `internal/app/dialog.go` (version string only)

**Context:**
The current README opens with "Phase 6 (in-buffer search) of a phased build" which is WIP framing. Phase 7 is the last phase — the README should now read as a finished product description. The About dialog still says "v0.5 (Phase 5)".

The macOS Cmd+ caveat is already correctly documented in the About dialog. The README already has a keybindings section; it needs reorganizing to be comprehensive without being a phase-by-phase changelog.

**Interfaces:**
- Produces: polished end-user README; About dialog version = "v0.7"

- [ ] **Step 1: Rewrite `README.md`**

Replace the entire file with the content below. Do not add content not listed here; do not keep any "Phase N of a phased build" framing.

```markdown
# Cody

A terminal-based code editor written in Go.

## Features

- **Project file tree** — lazy directory walker with icons (Nerd Font by default,
  plain Unicode fallback via `--no-nerd-font`); navigate with arrow keys or `hjkl`
- **Syntax highlighting** — tree-sitter-backed, supporting Go, Python, JavaScript,
  TypeScript, and Markdown; debounced re-highlight on edit
- **Code folding** — fold/unfold function bodies, block statements, and Markdown
  sections (`Ctrl+K`); search automatically unfolds a folded match
- **Incremental search** — `Ctrl+F` opens an in-buffer find dialog; case-insensitive
  substring matching, live match-jump as you type, `Enter`/`Shift+Enter` cycle
  next/previous, `Esc` closes
- **Embedded terminal** — real shell pane (spawned on first focus); full
  interactivity: `vim`, `htop`, `ssh`, `Ctrl+C`, etc.
- **Edit operations** — `Ctrl+X`/`Ctrl+C`/`Ctrl+V` cut/copy/paste (internal
  clipboard), `Ctrl+Z`/`Ctrl+Y` undo/redo, `Shift+Arrow` selection
- **Command palette** — click `Commands` in the menu bar (or use the palette
  shortcut) to search and run any registered command
- **File open dialog** — `Ctrl+O` to open a file by path (relative to project root
  or absolute)
- **Status bar** — project name, most-recent command, cursor position, file type

## Build & run

Requires a C compiler (CGO) for tree-sitter grammar bindings.

```bash
go build -o cody ./cmd/cody
./cody <path-to-project>          # opens the project tree at the given path
./cody                            # defaults to the current directory
./cody --no-nerd-font <path>      # plain Unicode icons (for terminals without Nerd Fonts)
```

## Keybindings

### Global (any pane)

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Cycle focus: tree → editor → terminal → tree (and reverse) |
| `Ctrl+O` | Open file by path |
| `Ctrl+F` | Find in current buffer |
| `Ctrl+Q` | Quit (cleanly terminates the spawned shell) |

### Project tree

| Key | Action |
|-----|--------|
| `↑`/`↓` or `j`/`k` | Move selection |
| `Enter` or `l` | Expand directory / open file (shifts focus to editor) |
| `h` | Collapse directory |

### Editor

| Key | Action |
|-----|--------|
| Arrow keys | Move cursor |
| `Home` / `End` | Start / end of line |
| `Page Up` / `Page Down` | Scroll by page |
| `Shift+Arrow` | Extend selection¹ |
| `Backspace` / `Delete` | Delete character |
| `Enter` | Insert newline |
| `Ctrl+S` | Save file |
| `Ctrl+X` / `Ctrl+C` / `Ctrl+V` | Cut / Copy / Paste (internal clipboard) |
| `Ctrl+Z` / `Ctrl+Y` | Undo / Redo |
| `Ctrl+K` | Toggle code fold at cursor line |

### Find dialog (`Ctrl+F`)

| Key | Action |
|-----|--------|
| Type | Jump to nearest match (case-insensitive) |
| `Enter` | Next match |
| `Shift+Enter` | Previous match¹ |
| `Esc` | Close dialog and clear search highlight |

### Terminal pane

While the terminal pane has focus, keystrokes go directly to the shell.
`Ctrl+S`/`Ctrl+X`/`Ctrl+C`/`Ctrl+V`/`Ctrl+Z`/`Ctrl+Y`/`Ctrl+K`/`Ctrl+F` pass
through to the shell. Only `Ctrl+O` and `Ctrl+Q` stay global.

### Menu bar

Click `File` or `Edit` to open a dropdown; click an item to run it; `Esc` or
click elsewhere to close. Click `Commands` to open the command palette — type to
filter, arrows to select, `Enter` to run, `Esc` to close. Click `About` for
version info and the macOS shortcut note.

---

¹ Requires a terminal emulator that reports shift-modified keys as distinct escape
sequences (iTerm2, Alacritty, Kitty, WezTerm, and most other modern emulators).

## macOS note

macOS terminal emulators intercept `Cmd+C`/`Cmd+V`/`Cmd+X`/`Cmd+Z` before they
reach any app running inside them. Use `Ctrl+` bindings — they work on every
platform. If you want `Cmd+` bindings, reconfigure your terminal emulator to
send the matching escape sequence.

## Terminal pane limitations

- Tab completion is unavailable — `Tab` / `Shift+Tab` are reserved globally for
  pane-focus cycling and never reach the shell.
- Arrow keys use normal cursor-key mode; full-screen programs that require
  application-mode arrows may see incorrect behavior (most `vim` / `less`
  configs work fine out of the box).
- The shell does not restart if it exits; the pane shows the last rendered frame.
- Abrupt process termination (e.g. `kill -9` on the parent) may leave the spawned
  shell orphaned. `Ctrl+Q` always cleans up correctly.
```

- [ ] **Step 2: Update About dialog version**

In `internal/app/dialog.go`, change the version string in `renderAboutDialog`:

Old line:
```go
content := "Cody v0.5 (Phase 5) — a terminal code editor\n\n" +
```

New line:
```go
content := "Cody v0.7 — a terminal code editor\n\n" +
```

- [ ] **Step 3: Verify build and tests**

```
go build ./...
go vet ./...
go test ./... -count=1
```

All must pass. No new tests needed — the About dialog version string is not currently tested, and README is prose.

- [ ] **Step 4: Commit**

```bash
git add README.md internal/app/dialog.go
git commit -m "docs: polish README to final user-facing form; bump About version to v0.7"
```
