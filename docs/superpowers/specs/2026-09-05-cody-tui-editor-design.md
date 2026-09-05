# Cody — TUI Code Editor — Design

## 1. Overview

Cody is a terminal-based code editor written in Go, invoked as:

```
cody <project-path>
```

`<project-path>` is the root of the codebase to browse and edit. If omitted, it
defaults to the current working directory. If the path doesn't exist or isn't
a directory, the app exits immediately with an error message (no TUI is
started).

The app presents a split-pane TUI: a project file tree, a code editor with
tree-sitter-backed syntax highlighting and folding, an embedded interactive
terminal, a menu bar, and a status bar.

## 2. Tech stack

- **Language**: Go.
- **TUI framework**: [Bubble Tea](https://github.com/charmbracelet/bubbletea)
  (Elm-architecture TUI framework) + [Lip Gloss](https://github.com/charmbracelet/lipgloss)
  for styling, using [Bubbles](https://github.com/charmbracelet/bubbles)
  component primitives (viewport, textinput) where they fit.
- **Syntax parsing**: [`github.com/smacker/go-tree-sitter`](https://github.com/smacker/go-tree-sitter)
  (CGO bindings to the real tree-sitter C library), with grammars for the
  initial language set: **Go, Python, JavaScript, TypeScript, Markdown** —
  all five are ready-made sub-packages of `go-tree-sitter` itself (verified
  by a build spike before Phase 4a: `golang`, `python`, `javascript`,
  `typescript/typescript`, `markdown`), so no grammar vendoring is needed for
  this set. **JSON is deferred** — `go-tree-sitter` has no ready-made JSON
  sub-package; adding it later means vendoring `tree-sitter-json`'s C sources
  and writing a small binding file mirroring the library's own pattern for
  its other languages (confirmed low-risk, just not done yet). Additional
  grammars can be added later without changing the architecture — each
  grammar is a Go package registered against a set of file extensions.
  Markdown's parse API returns a `MarkdownTree` with separate block/inline
  trees rather than a single `sitter.Tree` like the other languages; this is
  absorbed inside Markdown's own highlighter implementation, not exposed to
  the rest of the system.
- **Embedded terminal**: [`github.com/creack/pty`](https://github.com/creack/pty)
  to spawn a real shell in a pseudo-terminal, with a VT100/ANSI emulation
  library (e.g. [`hinshun/vt10x`](https://github.com/hinshun/vt10x)) to
  interpret the PTY's output stream into a virtual screen grid for rendering.
- **Build**: CGO is required (for tree-sitter). This constrains cross-compilation
  (a C toolchain must be available for the target) but is accepted as a
  tradeoff for using the mainline tree-sitter grammars.

## 3. Layout

Menu bar fixed on top, status bar fixed on bottom — both always visible. The
middle area splits with the project tree spanning the full height on the
left, and the right side splitting into the editor (top) and terminal
(bottom):

```
┌─────────────────────────────────────┐
│ File  Edit  Commands  About          │  menu bar
├───────────┬───────────────────────────┤
│           │        editor             │
│  project  ├───────────────────────────┤
│   tree    │       terminal            │
│           │                           │
├───────────┴───────────────────────────┤
│ project-name   recent-cmd  Ln:Col  ft │  status bar
└─────────────────────────────────────────┘
```

Focus moves between the three interactive panes (tree, editor, terminal) via
`Tab` / `Shift+Tab`. The focused pane is drawn with a visibly highlighted
border. Root-level key/mouse messages are routed to whichever pane has focus,
except for global shortcuts (menu actions, focus-switch, quit), which the
root model intercepts first.

## 4. Component architecture

One root Bubble Tea model owns four sub-models — `FileTree`, `Editor`,
`Terminal`, `MenuBar` — plus a `StatusBar` view and shared app state:
focused-pane enum, an internal clipboard buffer, and per-buffer dirty
tracking.

### Command registry

A central registry is the single source of truth for every user-invokable
action:

```go
type Command struct {
    Name     string   // e.g. "Save File"
    Shortcut string   // e.g. "ctrl+s"
    Handler  func(*AppState) tea.Cmd
}
```

Every action (Open, Save, Cut, Copy, Paste, Undo, Redo, Find, Quit, etc.) is
registered once. Three separate UI surfaces all dispatch through it rather
than duplicating logic:

1. **Direct keyboard shortcuts** — root model matches incoming key messages
   against registered shortcuts.
2. **Menu dropdowns** (File/Edit) — items are views over registry entries;
   clicking one calls that entry's handler.
3. **Command palette** (Commands menu) — lists all registry entries, filtered
   by a text query, executes the selected one on `Enter`.

This means adding a new command (new menu item, new shortcut, new palette
entry) is one registration, not three.

## 5. Project tree (file explorer)

- Lazily walks the given root directory, expanding subdirectories on demand
  rather than eagerly walking the whole tree — keeps large repos responsive.
- Skips `.git` by default. No other ignore rules in phase 1 (no `.gitignore`
  parsing yet — candidate for a later phase).
- Icons: Nerd Font glyphs keyed by extension/directory type, with a plain
  ASCII/unicode fallback set. A `--no-nerd-font` CLI flag forces the fallback
  (terminal font support can't be reliably auto-detected).
- Navigation: arrow keys / `j`/`k` to move, `Enter`/`l` to expand a directory
  or open a file (opening shifts focus to the editor), `h` to collapse.

## 6. Editor

- **Buffer model**: line-based (`[]string`) — sufficient for source files at
  this scale; not a rope or piece-table.
- **Undo/redo**: command-style undo stack. Each edit is recorded as an
  invertible insert/delete operation; `Ctrl+Z` undoes, `Ctrl+Y` redoes.
- **Editing**: arrows, Home/End/PageUp/PageDown, insert, backspace/delete,
  newline — standard line-buffer editing.
- **Selection**: anchor-to-cursor range, used by Cut/Copy.
- **Cut/Copy/Paste**: internal clipboard buffer only (not OS clipboard) —
  fully portable across SSH/tmux with no extra configuration, at the cost of
  not interoperating with other apps.
- **Tree-sitter highlighting** (Phase 4a): the buffer is (re)parsed on load
  and after debounced edits, using the grammar matched to the file's
  extension (`.go`, `.py`, `.js`/`.jsx`/`.mjs`, `.ts`/`.tsx`, `.md` — JSON
  deferred, see §2). Highlight queries per language map syntax captures
  (keyword, string, comment, number, function name) to Lip Gloss styles. A
  parse or query failure, or an unsupported extension, falls back to
  plain-text rendering for that buffer rather than surfacing an error.
- **Folding** (Phase 4b, built on top of 4a's parse trees): derived from
  tree-sitter node ranges (e.g. function/block bodies), using a
  per-language table of foldable node-type strings — these names are not
  consistent across grammars (e.g. Go/Python use `block`, JS/TS use
  `statement_block`), so the table is per-language even though the
  tree-walk that consumes it is shared. A folded region collapses to a
  single summary line, toggled with a dedicated key (e.g. `za`).
- **Search**: `Ctrl+F` opens a mini dialog for incremental find scoped to the
  **current buffer only** (project-wide search is out of scope for this
  design). `Enter`/`n` jumps to the next match, `N` the previous, `Esc`
  closes the dialog.
- **Save**: `Ctrl+S` writes buffer lines back to disk, clears the dirty flag,
  and reports the result to the status bar.

## 7. Terminal pane

- `$SHELL` is spawned via `creack/pty` on first focus (or at startup),
  attached to a pseudo-terminal sized to match the pane's dimensions.
- Output is run through a VT100/ANSI emulator maintaining a virtual screen
  grid, rendered each frame via Lip Gloss.
- When the pane is focused, raw keypresses (other than the app's own global
  shortcuts) are forwarded directly to the PTY's stdin — full interactivity
  (vim, htop, ssh, Ctrl-C, etc. all work as in a real terminal).
- The PTY is resized whenever the pane's on-screen size changes.

## 8. Menu bar, keybindings, command palette

- Top bar: `File  Edit  Commands  About`, always visible.
- Clicking a menu label opens its dropdown as an overlay; clicking an item
  runs that command and closes the dropdown; clicking elsewhere or `Esc`
  closes without action.
- **File**: Open (prompts for a file path — relative to the project root or
  absolute — and loads it into the editor, same as opening a file from the
  tree; useful for files not easily reached by browsing), Save.
- **Edit**: Cut, Paste, Copy, Save (duplicated from File menu, per original
  spec).
- **Commands**: opens the **command palette** — a modal listing every
  registry entry with its shortcut, a text filter (type to narrow by name),
  arrow keys to move selection, `Enter` to execute the selected command,
  `Esc` to close.
- **About**: a static modal with app name/version and the macOS shortcut
  caveat (below).
- Keybindings (canonical, `Ctrl`-based on every platform): `Ctrl+O` open,
  `Ctrl+S` save, `Ctrl+X`/`Ctrl+C`/`Ctrl+V` cut/copy/paste, `Ctrl+Z`/`Ctrl+Y`
  undo/redo, `Ctrl+F` find, `Tab`/`Shift+Tab` focus switch, `Ctrl+Q` quit.
- **macOS `Cmd+` caveat**: standard terminal emulators (Terminal.app, iTerm2,
  etc.) intercept `Cmd+C`/`Cmd+V`/`Cmd+X`/`Cmd+Z` for their own copy/paste/undo
  before the keypress reaches any program running inside them via stdin. Cody
  cannot force these through — a user who wants `Cmd+`-style shortcuts must
  reconfigure their terminal emulator's own keybindings to send a matching
  escape sequence, which is outside the app's control. The About dialog
  documents this; `Ctrl+` is the one binding scheme guaranteed to work
  everywhere.
- Every executed command, from any of the three dispatch paths, writes a
  short description to the status bar's "recent command" segment.

## 9. Status bar

- Left side: project name/root path.
- Right side, in order: `[recent command]  [Ln X, Col Y]  [filetype]`.
- Rendered on a highlighted background block spanning the bar's full width.

## 10. Error handling

- File I/O errors (permission denied, read-only) surface as a styled
  status-bar error rather than crashing the app.
- PTY spawn failure shows an inline error message inside the terminal pane;
  the rest of the app remains usable.
- Tree-sitter parse/query failures fall back to plain-text rendering for that
  buffer.

## 11. Testing strategy

- Unit tests: buffer edit operations and undo/redo correctness; tree-sitter
  highlight-span extraction (golden-file tests per language sample);
  file-tree walking and icon selection; command-registry dispatch (shortcut →
  handler, palette filter/execute).
- Terminal PTY rendering and full-TUI layout are verified manually per phase
  — raw terminal rendering doesn't lend itself to meaningful unit tests.

## 12. Phased build order

Each phase is independently functional/demoable:

1. **Skeleton** — CLI parsing, root Bubble Tea model, static Layout B panes,
   project tree (walk/icons/nav/open), plain-text editor (load/edit/save, no
   highlighting), status bar, focus switching.
2. **Command registry + core menu** — File/Edit menus, About dialog,
   Cut/Copy/Paste (internal clipboard), undo/redo — all wired through the
   registry.
3. **Command palette** (Commands menu).
4a. **Tree-sitter syntax highlighting** — grammar loading for Go/Python/
   JS/TS/Markdown (JSON deferred, see §2), highlight queries, capture-to-style
   mapping, debounced re-highlight on edit. Split out from the original
   single "tree-sitter integration" phase once a pre-implementation research
   spike found JSON needs custom grammar vendoring and the combined
   highlighting+folding scope was larger than any phase so far — each half
   is independently demoable on its own.
4b. **Code folding** — built on top of 4a's parse trees: per-language
   foldable-node-type tables, fold/unfold toggle, collapsed-region
   rendering.
5. **Embedded terminal pane** (PTY + VT100 emulation).
6. **In-buffer search** mini dialog.
7. **Polish** — nerd-font fallback flag, README, docs for the macOS `Cmd+`
   caveat.
