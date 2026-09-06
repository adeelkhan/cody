# Cody

A terminal-based code editor written in Go.

## Status

Phase 5 (embedded terminal) of a phased build — see
`docs/superpowers/specs/2026-09-05-cody-tui-editor-design.md` for the full
design and `docs/superpowers/plans/` for phase-by-phase implementation plans.

This phase adds a third focus-cyclable pane that runs a real shell (`$SHELL`,
spawned via `charmbracelet/x/xpty` and rendered using `charmbracelet/x/vt`)
in a pseudo-terminal. The terminal pane spawns on first focus and persists
until quit. Typing a command sends keystrokes through the real pty, and the
shell's output is rendered directly in the pane. The pty and emulator are
resized with the pane and the shell is notified via `SIGWINCH`. There is no
scrollback — the pane shows the emulator's current screen grid only.

Previous phases added: phase 4b (code folding for function/block bodies and
Markdown sections/code blocks — `Ctrl+K` toggles), phase 4 (independent
scrolling/clipping for tree and editor panes, scrollbars, auto-scroll to
cursor). The project tree and editor panes each scroll independently and stay
clipped to their box's height — opening a large file no longer pushes the
tree pane out of view.

Building this phase requires a C compiler on your machine (CGO), since
tree-sitter's grammars are C libraries — this was already noted as a
tradeoff in the design doc's tech stack section.

## Build & run

```bash
go build -o cody ./cmd/cody
./cody <path-to-a-project>
```

## Keybindings (phase 5)

- `Tab` / `Shift+Tab` — cycle focus through the three panes (project tree →
  editor → terminal → tree, and reverse)
- **Project tree**: arrows or `hjkl` to navigate, `Enter`/`l` to open a file or
  toggle a directory's expand/collapse state, `h` to collapse
- **Editor**: arrows to move the cursor, typing inserts text, `Enter` for a
  newline, `Backspace` to delete, `Shift+Arrow` to select text (requires a
  terminal that reports shift-modified arrow keys — most modern terminal
  emulators do, e.g. iTerm2, Alacritty, Kitty, WezTerm), `Ctrl+K` to toggle
  code folds (function/block bodies, or Markdown sections/code blocks)
- **Terminal pane**: runs a real shell spawned on first focus. While the
  terminal has focus, typing is sent directly to the shell; `Ctrl+S`/`Ctrl+X`/
  `Ctrl+C`/`Ctrl+V`/`Ctrl+Z`/`Ctrl+Y` pass through to the shell (not the
  editor commands they normally trigger)
- **Global** (work from any pane):
  - `Ctrl+O` open (or click File > Open)
  - `Ctrl+Q` cleanly terminates the spawned shell process and quits the app
  - Menu bar: click `File`/`Edit` to open a dropdown, click an item to run it,
    `Esc` or clicking elsewhere closes it; click `About` for app info; click
    `Commands` to open the command palette (type to filter, arrows to move
    the selection, `Enter` to run the selected command, `Esc` to close)
- Shortcuts are `Ctrl+`-based on every platform — macOS `Cmd+` shortcuts
  depend on your terminal emulator's own keybinding configuration and are not
  guaranteed to reach this app (see the About dialog)

### Terminal pane v1 limitations

- The spawned shell does not restart if it exits; the pane shows the last
  rendered frame.
- Arrow keys use normal (not application) cursor-key mode, so full-screen
  programs that explicitly request application-mode arrow keys (some versions
  of `vim` with certain `.vimrc` settings, `htop`, etc.) may see incorrect
  behavior. Most modern `vim` and `less` configurations work correctly out of
  the box.
- Abrupt termination (closing the terminal window, `kill -9`) may leave the
  spawned shell running orphaned — `Ctrl+Q` cleanly kills it.
- `Tab`/`Shift+Tab` are reserved globally for cycling pane focus, so a literal
  Tab keypress never reaches the shell — no tab-completion in the terminal
  pane.
