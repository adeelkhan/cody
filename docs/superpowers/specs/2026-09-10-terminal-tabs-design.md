# Terminal Tabs — Design

## 1. Overview

The terminal pane currently wraps exactly one `terminal.Model`: one pty,
one shell, one vt.Emulator, lazily started the first time the pane gets
focus. This spec gives the terminal pane multiple independent sessions —
"terminal tabs" — mirroring the editor's own tab strip: each tab wraps its
own `terminal.Model` (own pty, own emulator, own generation counter), shown
in a persistent tab strip above the terminal's content area. A new tab
opens with Ctrl+T; a tab closes via a "×" click, mirroring the editor tab
bar's close glyph exactly, with no confirmation (a shell session carries no
"unsaved work" concept the way an editor buffer does). Each session's
output keeps accumulating in the background while a different tab is
active, the same way a real terminal multiplexer works.

Out of scope: splitting the terminal pane itself (recursive nesting, or a
second terminal pane side by side) — only multiple tabs within the single
existing terminal pane, matching the user's request. Terminal tabs are not
movable between panes (there is only one terminal pane) and have no
right-click context menu (no "split" action applies to them).

## 2. Constraints

Global Constraints (apply to every task below):

- Go + Bubble Tea/Lip Gloss, matching the existing codebase's architecture
  and style exactly (value-receiver `Model`s, `tea.Cmd` continuations).
- Every negative or zero-or-smaller size reaching a pty/vt.Emulator must be
  floored at 0 before `SetSize` — `terminal.Model.SetSize` already does
  this defensively; nothing above it may skip that floor.
- ANSI-safe rendering: any block placed inside a bordered `lipgloss.Style`
  with both `Width()` and `Height()` set must be pre-truncated with
  `clampBlockWidth` (`MaxWidth`, not `Width`) — `Width()`/`Height()` are
  minimums, not maximums, and a too-wide line silently wraps and overflows
  the box otherwise (the exact class of bug fixed twice already on this
  codebase's split-view-editor branch).
- TDD: write the failing test, watch it fail, implement, watch it pass.
- `gofmt`-clean; `go vet ./...` and the full `go test ./...` suite green
  before every commit.
- The terminal pane itself must never disappear or drop to zero tabs — it
  is a fixed, non-closable pane (unlike an editor pane, which can collapse
  away entirely when both split panes' tabs close). Closing the sole
  remaining terminal tab replaces it with a fresh, unstarted session in the
  same slot rather than leaving the tab strip empty.

## 3. Data Model

### 3.1 `internal/terminal` package — adding a stable session identity

`terminal.Model`'s existing `generation int` field is scoped to a single
`Model` instance (it starts at 0 and increments each time `Start()` is
called, purely to let that instance's own `Update` discard reads issued
before a restart). It is **not** a value a caller holding several
independent `Model` instances can use to tell them apart — every instance's
`generation` starts at 0 the same way, so two different sessions' `OutputMsg`
values can carry the same `generation` and be indistinguishable.

Add a caller-assigned `id int`, set once at construction and echoed back on
every message that instance produces, so `internal/app` — which will hold
several `terminal.Model` instances in a slice — can route an incoming
`OutputMsg`/`ReadErrMsg` back to the specific instance it belongs to,
regardless of which tab is currently active:

```go
// internal/terminal/model.go

type Model struct {
	id            int // caller-assigned; stable for this Model's lifetime, echoed in OutputMsg/ReadErrMsg for multi-instance routing (see New's doc comment)
	width, height int
	pty           Pty
	emu           Emulator
	started       bool
	err           error
	generation    int
}

// New creates a terminal session identified by id — a value the caller
// controls and can use to tell this session's OutputMsg/ReadErrMsg apart
// from any other terminal.Model instance's. id is otherwise opaque to this
// package: it is never interpreted, only stored and echoed back.
func New(id int) Model {
	return Model{id: id}
}

// ID returns the identity this session was constructed with.
func (m Model) ID() int {
	return m.id
}

type OutputMsg struct {
	id         int
	data       []byte
	generation int
}

// ID returns the terminal.Model this message belongs to (see New's doc
// comment) — a caller holding several sessions must check this before
// routing the message into any one of them.
func (m OutputMsg) ID() int {
	return m.id
}

type ReadErrMsg struct {
	id         int
	generation int
}

// ID returns the terminal.Model this message belongs to — see
// OutputMsg.ID's doc comment.
func (m ReadErrMsg) ID() int {
	return m.id
}
```

`readCmd`, `Start`, and `Update` all thread `m.id` through into the
messages they produce (full code in the plan). No other package's call
sites need to know about `id` beyond passing one to `New` and reading it
back off the messages they route.

### 3.2 `internal/app` package

```go
// terminalTab is one terminal session: its own independent terminal.Model
// (own pty, own vt.Emulator, own generation counter). Unlike an editor
// tab, it carries no path/dirty state — a shell session has no "unsaved
// changes" concept.
type terminalTab struct {
	term terminal.Model
}

type Model struct {
	// ... existing fields ...

	// terminals/activeTerminal mirror panes/activePane's own invariants:
	// len(terminals) is never 0 — the terminal pane itself is not
	// closable, so closing the sole remaining tab resets it in place
	// rather than emptying the slice (see removeTerminalTab). activeTerminal
	// indexes the tab currently shown/receiving keyboard input when
	// focus == focusTerminal.
	terminals      []terminalTab
	activeTerminal int

	// nextTerminalID is a monotonic counter handed to terminal.New as each
	// new session's stable id (see that package's New doc comment) —
	// never reused, so a message from a since-closed session can never be
	// misrouted to whatever new session happens to occupy its old slice
	// index.
	nextTerminalID int

	// (the old singular `terminal terminal.Model` field is removed)
}
```

`New(rootPath string, nerdFont bool) (Model, error)` constructs
`terminals: []terminalTab{{term: terminal.New(0)}}`, `nextTerminalID: 0`
(so the very next tab created gets id 1).

### 3.3 Layout: `paneLayout`'s terminal rect becomes a sub-layout

`paneLayout` currently returns a single flat `terminalR rect` for the whole
terminal pane. Splitting that into its own tab bar row (always
`tabBarHeight` tall — unlike the editor's `tabBarH()`, which is `0` when no
tabs are open, the terminal always has at least one tab) and the content
rect below it, mirroring `editorPaneLayout`'s own `{tabBar, editor}` shape:

```go
// terminalPaneLayout describes the terminal pane's own two rows: its tab
// strip (always tabBarHeight tall — the terminal always has at least one
// tab, so unlike editorPaneLayout.tabBar this is never a zero-height rect)
// and the content rect below it where the active session actually renders.
type terminalPaneLayout struct {
	tabBar   rect
	terminal rect
}
```

`paneLayout`'s signature becomes
`func (m Model) paneLayout() (tree rect, panes []editorPaneLayout, term terminalPaneLayout)`.
`m.terminalHeight` keeps meaning exactly what it already means — the
terminal pane's full on-screen height, border included — it is now just
subdivided one row further (tab bar + content) the same way `editorHeight`
already subdivides into `tabBarH` + the editor's own interior height.
`minTerminalHeight` moves from `3` to `4` (border `2` + the new tab bar row
`1` + at least `1` content row) to keep guaranteeing at least one visible
content row exists; every other clamp formula that already treats
`m.terminalHeight` as an opaque outer height is unaffected.

## 4. Behavior

### 4.1 Creating a tab

A new "New Terminal Tab" command, shortcut `ctrl+t`, added to
`buildCommands()` and to the terminal-focus passthrough allowlist in
`Update`'s `tea.KeyMsg` handling (alongside the existing `ctrl+o`/`ctrl+q`/
`ctrl+n` exceptions — without this, a shell that has focus would swallow
`ctrl+t` as raw input instead of it reaching the command). Its handler
appends a new, not-yet-started `terminalTab`, makes it the active terminal
tab, and focuses the terminal pane:

```go
func cmdNewTerminalTab(m Model) (Model, tea.Cmd) {
	m.nextTerminalID++
	m.terminals = append(m.terminals, terminalTab{term: terminal.New(m.nextTerminalID)})
	m.activeTerminal = len(m.terminals) - 1
	m.focus = focusTerminal
	m.recentCommand = "New terminal"
	return m.maybeStartActiveTerminal()
}
```

No dedicated command palette restriction — it appears in the palette (fuzzy
search) the same as every other `Command`, matching `New Tab`'s own
precedent (which also has no File/Edit dropdown entry, only a shortcut and
a palette entry).

### 4.2 Lazy start, generalized per-tab

`maybeStartTerminal` is renamed `maybeStartActiveTerminal` and now starts
whichever tab is active, not a single field:

```go
func (m Model) maybeStartActiveTerminal() (Model, tea.Cmd) {
	if m.focus != focusTerminal {
		return m, nil
	}
	var cmd tea.Cmd
	m.terminals[m.activeTerminal].term, cmd = m.terminals[m.activeTerminal].term.Start()
	return m, cmd
}
```

Every existing call site (`handlePaneClick`'s terminal-rect click,
`focusNext`/`focusPrev`'s `tab`/`shift+tab` key handling) is updated to
call this instead — same lazy-start-on-first-focus behavior, just scoped to
whichever tab is active at the time.

### 4.3 Switching tabs

Clicking a tab in the terminal's tab bar switches `m.activeTerminal` to it,
focuses the terminal pane, and lazily starts it if this is its first time
becoming active — mirroring the editor tab bar's click-to-switch exactly,
via a terminal-specific `terminalTabAt`/`terminalTabRegions` pair (same
column-math shape as `tabAt`/`tabRegions`, but over `[]terminalTab` with no
dirty-marker concept — see `internal/app/terminaltabs.go` in the plan).

### 4.4 Closing a tab

Clicking a tab's "×" closes that specific session — `Close()`s its pty —
and removes it from `m.terminals`, reassigning `m.activeTerminal` with the
exact same rule `removeTab` already uses for editor tabs (prefer the tab
that shifted into the closed one's slot; fall back to the tab before it if
the closed tab was the last one) — except a terminal tab list is never
allowed to reach zero:

```go
// removeTerminalTab closes and removes the tab at index, reassigning
// m.activeTerminal the same way removeTab reassigns an editorPane's
// activeTab (prefer the tab that shifted into the closed slot, falling
// back to the tab before it if the closed tab was last). Unlike an
// editorPane's tabs, this slice must never reach zero — the terminal pane
// itself isn't closable — so closing the sole remaining tab replaces it
// with one fresh, unstarted session in the same slot instead.
func (m Model) removeTerminalTab(index int) Model {
	if index < 0 || index >= len(m.terminals) {
		return m
	}
	m.terminals[index].term.Close()
	if len(m.terminals) == 1 {
		m.nextTerminalID++
		m.terminals[0] = terminalTab{term: terminal.New(m.nextTerminalID)}
		m.activeTerminal = 0
		return m
	}
	m.terminals = append(m.terminals[:index], m.terminals[index+1:]...)
	switch {
	case index < m.activeTerminal:
		m.activeTerminal--
	case index == m.activeTerminal:
		if m.activeTerminal >= len(m.terminals) {
			m.activeTerminal--
		}
	}
	return m
}
```

No confirmation dialog — matches the spec's own framing in §1: a shell
session has no "unsaved changes" concept, so there is nothing to confirm
losing (this deliberately does **not** reuse `confirmCloseTab`/
`dialogConfirmDiscard`, which exist specifically to protect unsaved editor
buffers).

### 4.5 Message routing: background sessions keep running

`Update`'s existing unconditional `terminal.OutputMsg`/`terminal.ReadErrMsg`
handling (today, routed straight to the single `m.terminal` regardless of
`m.focus`) becomes a lookup by `ID()` across `m.terminals`, so a
backgrounded tab's shell keeps producing output — and the app keeps
reading and buffering it into that tab's own emulator — even while a
different tab is the one actually on screen:

```go
if out, ok := msg.(terminal.OutputMsg); ok {
	for i := range m.terminals {
		if m.terminals[i].term.ID() == out.ID() {
			var cmd tea.Cmd
			m.terminals[i].term, cmd = m.terminals[i].term.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}
if _, ok := msg.(terminal.ReadErrMsg); ok {
	// same shape, keyed by ReadErrMsg.ID()
}
```

A message whose `ID()` matches no current tab (the tab was already closed
by the time its last in-flight read completed) is silently dropped — not
an error case, just a race between "user closed the tab" and "one more
read was already in flight," and there is nothing left to route it to.

### 4.6 Resizing

`resizeAllPanes` (already the single place window-resize and boundary-drag
resizes get propagated into every pane's persisted state, per the
split-view-editor branch's own fix) gains a loop over `m.terminals`,
sizing every tab's `terminal.Model` — not just the active one — matching
the exact reasoning already documented for editor tabs: a backgrounded
session left at a stale size would misrender or break its content the
moment it's switched into. `View()`'s independent, render-time-only
recomputation gets the same per-tab loop for the same reason
`resizeAllPanes` exists at all: the persisted state and the rendered state
must not drift apart.

### 4.7 Rendering

The terminal pane's column becomes tab-bar-then-content, exactly mirroring
each editor pane's `renderTabBar` + bordered content column:

```go
right := lipgloss.JoinVertical(lipgloss.Left,
	editorsRow,
	renderTerminalTabBar(m.width-m.treeWidth, m.terminals, m.activeTerminal),
	terminalStyle.Render(clampBlockWidth(activeTerm.View(), termWidth)),
)
```

`renderTerminalTabBar` mirrors `renderTabBar`'s exact visual language
(active tab in reverse video, "×" close glyph per tab) with a numbered
label ("Shell 1", "Shell 2", ... by current position, matching how
`tabDisplayName` falls back to a stable, always-available label) since a
shell session has no filename to show.

### 4.8 Quitting

`cmdQuit` and `updateConfirmDialog`'s `confirmQuit` branch each currently
call `m.terminal.Close()` once; both become a loop closing every tab's
session before `tea.Quit`, so no shell process is left dangling because it
happened to be in a background tab at quit time.

## 5. Testing

Every behavior above gets a TDD-written regression test, following this
codebase's existing terminal-focused test conventions (`internal/terminal`
already has a `fakePty`/`fakeEmulator` test-double pair — reused, not
duplicated, for any new `internal/terminal` test):

- `terminal.Model.ID()` returns the id passed to `New`; `OutputMsg.ID()`/
  `ReadErrMsg.ID()` echo that same id after a real read cycle through the
  fake pty.
- Two independently-constructed `terminal.Model`s given different ids
  never produce messages with colliding `ID()` values even after both have
  been `Start()`ed (their internal `generation` counters both begin at the
  same value — this is the exact scenario `id` exists to disambiguate).
- `cmdNewTerminalTab` appends a tab, makes it active, focuses the
  terminal pane, and does not start it (that's `maybeStartActiveTerminal`'s
  job, invoked separately) until it's actually the active tab under
  terminal focus.
- Ctrl+T while the terminal pane already has focus (so keys would
  otherwise route to the shell) still creates a new tab, matching
  `ctrl+o`/`ctrl+q`/`ctrl+n`'s existing passthrough-allowlist precedent.
- Clicking a terminal tab switches `activeTerminal` and focuses the pane;
  clicking its "×" closes it and reassigns `activeTerminal` per §4.4's
  rule, covering: closing a middle tab, closing the last tab, closing the
  active tab, closing a non-active tab (activeTerminal shifts only when
  the closed index was at or before it), and closing the sole remaining
  tab (replaces it with a fresh, unstarted session rather than emptying
  `m.terminals`).
- A background tab's `OutputMsg` is applied to that tab's own emulator, not
  the active one — verified by starting two tabs, feeding output to the
  non-active one via its fake pty, and asserting only that tab's rendered
  content changed.
- A `WindowSizeMsg`/boundary-drag resize reaches every terminal tab's
  stored size, not just the active one's (mirroring the split-view-editor
  branch's own `resizeAllPanes` regression test for the same class of
  staleness bug, applied to terminal tabs instead of editor tabs).
- Quitting with two terminal tabs open closes both sessions' ptys (a fake
  `Pty`'s `Close()` call is observable in `internal/terminal`'s existing
  test doubles; verified at the `internal/app` level via a closed-count
  assertion on both tabs' underlying fakes).
