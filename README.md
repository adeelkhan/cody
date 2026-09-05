# Cody

A terminal-based code editor written in Go.

## Status

Phase 2 (command registry + core menu) of a phased build — see
`docs/superpowers/specs/2026-09-05-cody-tui-editor-design.md` for the full
design and `docs/superpowers/plans/` for phase-by-phase implementation plans.

This phase adds: a command registry as the single dispatch path for every
shortcut, a mouse-driven File/Edit menu bar with dropdowns, an About dialog,
Cut/Copy/Paste with an internal clipboard and Shift+Arrow selection, and
snapshot-based undo/redo. It does not yet have: the Commands menu's command
palette (clicking it shows a placeholder status message), syntax
highlighting/folding, an embedded shell, or in-buffer search — those land in
later phases.

## Build & run

```bash
go build -o cody ./cmd/cody
./cody <path-to-a-project>
```

## Keybindings (phase 2)

- `Tab` / `Shift+Tab` — switch focus between the project tree and the editor
- Project tree: arrows or `hjkl` to navigate, `Enter`/`l` to open a file or
  toggle a directory's expand/collapse state, `h` to collapse
- Editor: arrows to move the cursor, typing inserts text, `Enter` for a
  newline, `Backspace` to delete, `Shift+Arrow` to select text
- `Ctrl+O` open (or click File > Open), `Ctrl+S` save
- `Ctrl+X`/`Ctrl+C`/`Ctrl+V` cut/copy/paste (operates on the selection, or the
  whole current line if nothing is selected)
- `Ctrl+Z`/`Ctrl+Y` undo/redo
- `Ctrl+Q` — quit
- Menu bar: click `File`/`Edit` to open a dropdown, click an item to run it,
  `Esc` or clicking elsewhere closes it; click `About` for app info; click
  `Commands` — not implemented yet (Phase 3)
- Shortcuts are `Ctrl+`-based on every platform — macOS `Cmd+` shortcuts
  depend on your terminal emulator's own keybinding configuration and are not
  guaranteed to reach this app (see the About dialog)
