# Cody

A terminal-based code editor written in Go.

## Status

Phase 6 (in-buffer search) of a phased build — see
`docs/superpowers/specs/2026-09-05-cody-tui-editor-design.md` for the full
design and `docs/superpowers/plans/` for phase-by-phase implementation plans.

This phase adds incremental find-in-buffer search with `Ctrl+F`, supporting
case-insensitive matching in the current buffer only. Type to jump to the nearest
match, use `Enter`/`Shift+Enter` to cycle through next/previous matches, and
`Esc` to close and clear the search. The search is performed live as you type,
with the first match highlighted as the cursor position.

Previous phases added: phase 5 (embedded terminal with real shell pane),
phase 4b (code folding for function/block bodies and Markdown sections/code blocks —
`Ctrl+K` toggles), phase 4 (independent scrolling/clipping for tree and editor
panes, scrollbars, auto-scroll to cursor). The project tree and editor panes each
scroll independently and stay clipped to their box's height — opening a large file
no longer pushes the tree pane out of view.

Building this phase requires a C compiler on your machine (CGO), since
tree-sitter's grammars are C libraries — this was already noted as a
tradeoff in the design doc's tech stack section.

## Build & run

```bash
go build -o cody ./cmd/cody
./cody <path-to-a-project>
```

## Keybindings (phase 6)

- `Tab` / `Shift+Tab` — cycle focus through the three panes (project tree →
  editor → terminal → tree, and reverse)
- `Ctrl+F` — open incremental find (case-insensitive, current buffer
  only); type to jump to the nearest match, `Enter`/`Shift+Enter` for
  next/previous match, `Esc` closes and clears the search (requires a
  terminal that reports `Shift+Enter` distinctly from plain `Enter` —
  the same class of terminal-capability caveat this project already
  documents for `Shift+Arrow` selection; most modern terminal emulators
  handle it, e.g. iTerm2, Alacritty, Kitty, WezTerm)
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
