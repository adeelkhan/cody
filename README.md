# Cody

A terminal-based code editor written in Go.

![Cody screenshot](cody.png)

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
- **Resizable panes** — drag the tree's right border to resize it, or the boundary
  between the editor and terminal to resize the terminal
- **Status bar** — project name, most-recent command, cursor position, file type

## Build & run

Requires a C compiler (CGO) for tree-sitter grammar bindings.

```bash
make build                        # builds the cody binary (go build -o cody ./cmd/cody)
make test                         # runs the test suite (go test ./...)
./cody <path-to-project>          # opens the project tree at the given path
./cody                            # defaults to the current directory
./cody --no-nerd-font <path>      # plain Unicode icons (for terminals without Nerd Fonts)
```

## Keybindings

### Global (any pane)

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Cycle focus: tree → editor → terminal → tree (and reverse) |
| `Ctrl+N` | New blank ("Untitled") tab — `Ctrl+S` on it prompts for a save path |
| `Ctrl+O` | Open file by path |
| `Ctrl+F` | Find in current buffer |
| `Ctrl+Q` | Quit (cleanly terminates the spawned shell; prompts if any tab has unsaved changes) |

Click and drag the tree pane's right border to resize it, or the boundary
between the editor and terminal to resize the terminal. Both panes have a
minimum size, and the editor/tree on the other side of the drag always keeps
enough room to stay usable.

### Project tree

| Key | Action |
|-----|--------|
| `↑`/`↓` or `j`/`k` | Move selection |
| `Enter` or `l` | Expand directory / open file (shifts focus to editor) |
| `h` | Collapse directory |
| `n` | Open the New File / New Folder / Rename menu for the selected item² |
| Right-click | Same menu, anchored at the clicked item (or the end of the listing, for empty space, for New only) |

Once the menu is open: `↑`/`↓` to move between items, `Enter` to pick one,
`Esc` to close. Picking New File/Folder or Rename shows an inline text field
in the tree — type the name and `Enter` to confirm, `Esc` to cancel.

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

² Some terminal emulators don't reliably forward right-click to the app (a
few report it as a left click at the wire-protocol level, which is outside
this app's control) — `n` always works regardless of terminal.

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
