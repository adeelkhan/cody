# Cody

A terminal-based code editor written in Go.

## Status

Phase 4b (code folding) of a phased build — see
`docs/superpowers/specs/2026-09-05-cody-tui-editor-design.md` for the full
design and `docs/superpowers/plans/` for phase-by-phase implementation plans.

This phase adds folding for function/block bodies (Go, Python, JavaScript,
TypeScript) and Markdown sections/code blocks — `Ctrl+K` toggles the fold
at the cursor's line, collapsing it to a single summary line. Editing the
file clears all fold state. JSON folding (and highlighting) remains
deferred — the underlying tree-sitter library has no ready-made JSON
grammar. It does not yet have: an embedded shell or in-buffer search —
those land in later phases.

Building this phase requires a C compiler on your machine (CGO), since
tree-sitter's grammars are C libraries — this was already noted as a
tradeoff in the design doc's tech stack section, and matters even more
now that folding adds a second tree-sitter dependency surface alongside
highlighting.

## Build & run

```bash
go build -o cody ./cmd/cody
./cody <path-to-a-project>
```

## Keybindings (phase 4b)

- `Tab` / `Shift+Tab` — switch focus between the project tree and the editor
- Project tree: arrows or `hjkl` to navigate, `Enter`/`l` to open a file or
  toggle a directory's expand/collapse state, `h` to collapse
- Editor: arrows to move the cursor, typing inserts text, `Enter` for a
  newline, `Backspace` to delete, `Shift+Arrow` to select text (requires a
  terminal that reports shift-modified arrow keys — most modern terminal
  emulators do, e.g. iTerm2, Alacritty, Kitty, WezTerm)
- `Ctrl+O` open (or click File > Open), `Ctrl+S` save
- `Ctrl+X`/`Ctrl+C`/`Ctrl+V` cut/copy/paste (operates on the selection, or the
  whole current line if nothing is selected)
- `Ctrl+Z`/`Ctrl+Y` undo/redo
- `Ctrl+K` — toggle the code fold at the cursor's current line (function/
  block bodies, or Markdown sections/code blocks)
- `Ctrl+Q` — quit
- Menu bar: click `File`/`Edit` to open a dropdown, click an item to run it,
  `Esc` or clicking elsewhere closes it; click `About` for app info; click
  `Commands` to open the command palette (type to filter, arrows to move
  the selection, `Enter` to run the selected command, `Esc` to close)
- Shortcuts are `Ctrl+`-based on every platform — macOS `Cmd+` shortcuts
  depend on your terminal emulator's own keybinding configuration and are not
  guaranteed to reach this app (see the About dialog)
