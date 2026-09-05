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
  toggle a directory's expand/collapse state, `h` to collapse
- Editor: arrows to move the cursor, typing inserts text, `Enter` for a
  newline, `Backspace` to delete, `Ctrl+S` to save
- `Ctrl+Q` — quit
