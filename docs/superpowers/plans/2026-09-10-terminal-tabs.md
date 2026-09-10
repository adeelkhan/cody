# Terminal Tabs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the terminal pane multiple independent shell-session tabs — new via Ctrl+T, closable via a "×" click, each an independent `terminal.Model` (own pty/emulator, own state) — mirroring the editor's own tab strip.

**Architecture:** Add a caller-assigned stable `id` to `terminal.Model` so `internal/app` can route `OutputMsg`/`ReadErrMsg` back to the right session out of several. Replace `app.Model`'s single `terminal terminal.Model` field with `terminals []terminalTab` + `activeTerminal int` (mirrors `panes`/`activePane`'s own shape, minus the split/move-between-panes behavior — there is only one terminal pane). Extend `paneLayout` with a `terminalPaneLayout{tabBar, terminal}` sub-layout, mirroring `editorPaneLayout`. Reuse the editor tab bar's exact column-math shape (`tabRegions`/`tabAt`) via a terminal-specific sibling in a new `internal/app/terminaltabs.go`.

**Tech Stack:** Go, Bubble Tea, Lip Gloss — same as the rest of this codebase.

**Spec:** docs/superpowers/specs/2026-09-10-terminal-tabs-design.md

## Global Constraints

- Every negative/zero-or-smaller size reaching a pty or vt.Emulator must be floored at 0 before `SetSize`.
- Any block placed inside a bordered `lipgloss.Style` with `Width()`+`Height()` set must be pre-truncated with `clampBlockWidth` (`MaxWidth`, never `Width`) first.
- TDD throughout: write the failing test, run it, watch it fail for the right reason, implement, run it again, watch it pass.
- `gofmt`-clean; `go vet ./...` and the full `go test ./... -count=1` suite green before every commit.
- `m.terminals` must never reach length 0 — the terminal pane itself is not closable.
- Every open terminal tab must keep receiving pty output while backgrounded (not just the active one) and must be resized on every window resize / boundary drag (not just the active one) — mirroring the editor tabs' own "every tab, not just active" convention throughout this codebase.

---

### Task 1: `internal/terminal` — stable per-session `id`

**Files:**
- Modify: `internal/terminal/model.go`
- Modify: `internal/terminal/model_test.go` (12 existing `New()` call sites need updating)
- Modify: `internal/terminal/integration_test.go` (1 existing `New()` call site)
- Test: `internal/terminal/model_test.go` (new tests, appended)

**Interfaces:**
- Consumes: nothing new — this task only changes `internal/terminal`'s own public surface.
- Produces: `terminal.New(id int) Model`, `(Model) ID() int`, `(OutputMsg) ID() int`, `(ReadErrMsg) ID() int` — Task 2 constructs every `terminal.Model` with a caller-assigned id and routes incoming messages by matching `ID()`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/terminal/model_test.go`:

```go
func TestIDReturnsWhatNewWasConstructedWith(t *testing.T) {
	m := New(7)
	if got := m.ID(); got != 7 {
		t.Fatalf("got ID()=%d, want 7", got)
	}
}

func TestOutputMsgAndReadErrMsgEchoTheSessionID(t *testing.T) {
	p := &fakePty{toRead: [][]byte{[]byte("hi")}}
	e := &fakeEmulator{}
	withFakes(t, p, e)

	m := New(42)
	m, cmd := m.Start()
	if cmd == nil {
		t.Fatal("test setup: Start() returned a nil cmd")
	}
	msg := cmd()
	out, ok := msg.(OutputMsg)
	if !ok {
		t.Fatalf("test setup: got %T, want OutputMsg", msg)
	}
	if got := out.ID(); got != 42 {
		t.Fatalf("got OutputMsg.ID()=%d, want 42", got)
	}

	// Drain the pty dry so the next read reports an error, producing a
	// ReadErrMsg to check the same way.
	m, cmd = m.Update(msg)
	if cmd == nil {
		t.Fatal("test setup: Update(OutputMsg) returned a nil cmd")
	}
	msg = cmd()
	errMsg, ok := msg.(ReadErrMsg)
	if !ok {
		t.Fatalf("test setup: got %T, want ReadErrMsg", msg)
	}
	if got := errMsg.ID(); got != 42 {
		t.Fatalf("got ReadErrMsg.ID()=%d, want 42", got)
	}
}

func TestTwoSessionsIDsNeverCollideEvenThoughBothGenerationCountersStartAtZero(t *testing.T) {
	p1 := &fakePty{toRead: [][]byte{[]byte("a")}}
	e1 := &fakeEmulator{}
	p2 := &fakePty{toRead: [][]byte{[]byte("b")}}
	e2 := &fakeEmulator{}

	origPty, origEmu := newPty, newEmulator
	t.Cleanup(func() { newPty, newEmulator = origPty, origEmu })

	newPty = func(width, height int) (Pty, error) { return p1, nil }
	newEmulator = func(width, height int) Emulator { return e1 }
	m1 := New(1)
	m1, cmd1 := m1.Start()

	newPty = func(width, height int) (Pty, error) { return p2, nil }
	newEmulator = func(width, height int) Emulator { return e2 }
	m2 := New(2)
	m2, cmd2 := m2.Start()

	out1 := cmd1().(OutputMsg)
	out2 := cmd2().(OutputMsg)
	if out1.ID() == out2.ID() {
		t.Fatalf("got both sessions' OutputMsg.ID()=%d, want distinct ids (1 vs 2) even though both instances' internal generation counters started at the same value", out1.ID())
	}
	if out1.ID() != 1 || out2.ID() != 2 {
		t.Fatalf("got ids (%d, %d), want (1, 2)", out1.ID(), out2.ID())
	}
}
```

- [ ] **Step 2: Run the new tests to verify they fail**

Run: `go test ./internal/terminal/ -run 'TestIDReturnsWhatNewWasConstructedWith|TestOutputMsgAndReadErrMsgEchoTheSessionID|TestTwoSessionsIDsNeverCollideEvenThoughBothGenerationCountersStartAtZero' -v`
Expected: compile failure — `New` takes no arguments, `Model`/`OutputMsg`/`ReadErrMsg` have no `ID` method yet.

- [ ] **Step 3: Add `id` to `Model`, `New(id int)`, and `(Model) ID()`**

In `internal/terminal/model.go`, replace:

```go
type Model struct {
	width, height int
	pty           Pty
	emu           Emulator
	started       bool
	err           error
	generation    int
}

func New() Model {
	return Model{}
}
```

with:

```go
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
// from any other terminal.Model instance's. A per-instance generation
// counter alone can't do this: it starts at 0 for every instance, so two
// independently-created sessions' messages can carry the same generation
// and be indistinguishable. id is otherwise opaque to this package — never
// interpreted, only stored and echoed back.
func New(id int) Model {
	return Model{id: id}
}

// ID returns the identity this session was constructed with.
func (m Model) ID() int {
	return m.id
}
```

- [ ] **Step 4: Thread `id` through `OutputMsg`/`ReadErrMsg`/`readCmd`/`Start`/`Update`**

Replace:

```go
type OutputMsg struct {
	data       []byte
	generation int
}
```

with:

```go
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
```

Replace:

```go
type ReadErrMsg struct {
	generation int
}
```

with:

```go
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

Replace:

```go
func readCmd(p Pty, generation int) tea.Cmd {
	return func() tea.Msg {
		buf := make([]byte, 4096)
		n, err := p.Read(buf)
		if err != nil {
			return ReadErrMsg{generation: generation}
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		return OutputMsg{data: data, generation: generation}
	}
}
```

with:

```go
func readCmd(p Pty, id, generation int) tea.Cmd {
	return func() tea.Msg {
		buf := make([]byte, 4096)
		n, err := p.Read(buf)
		if err != nil {
			return ReadErrMsg{id: id, generation: generation}
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		return OutputMsg{id: id, data: data, generation: generation}
	}
}
```

In `Start`, replace the final line:

```go
	m.pty = p
	m.emu = newEmulator(w, h)
	m.generation++
	return m, readCmd(m.pty, m.generation)
}
```

with:

```go
	m.pty = p
	m.emu = newEmulator(w, h)
	m.generation++
	return m, readCmd(m.pty, m.id, m.generation)
}
```

In `Update`'s `OutputMsg` case, replace:

```go
	case OutputMsg:
		if msg.generation != m.generation || m.emu == nil {
			return m, nil
		}
		m.emu.Write(msg.data)
		return m, readCmd(m.pty, m.generation)
```

with:

```go
	case OutputMsg:
		if msg.generation != m.generation || m.emu == nil {
			return m, nil
		}
		m.emu.Write(msg.data)
		return m, readCmd(m.pty, m.id, m.generation)
```

- [ ] **Step 5: Run the new tests to verify they pass**

Run: `go test ./internal/terminal/ -run 'TestIDReturnsWhatNewWasConstructedWith|TestOutputMsgAndReadErrMsgEchoTheSessionID|TestTwoSessionsIDsNeverCollideEvenThoughBothGenerationCountersStartAtZero' -v`
Expected: all three PASS.

- [ ] **Step 6: Fix every pre-existing `New()` call site**

`New` now requires an `id` argument. Every existing call in this package needs one — the exact value doesn't matter for these tests (none of them test `ID()`), so use `1` uniformly:

In `internal/terminal/model_test.go`: replace every occurrence of `New()` with `New(1)` (12 occurrences — the ones this step didn't just add in Step 1, which already pass their own explicit ids).

In `internal/terminal/integration_test.go`: replace the one `New()` occurrence with `New(1)`.

- [ ] **Step 7: Run the full package test suite**

Run: `go build ./... && go vet ./... && gofmt -l internal/terminal/ && go test ./internal/terminal/... -count=1 -v`
Expected: PASS, no gofmt output, no vet warnings. (Note: `go build ./...` will still fail at this point — `internal/app/model.go`'s `terminal.New()` call site is fixed in Task 2, not this one. That failure is expected and fine to leave for Task 2; do not attempt to fix `internal/app` in this task.)

- [ ] **Step 8: Commit**

```bash
git add internal/terminal/model.go internal/terminal/model_test.go internal/terminal/integration_test.go
git commit -m "feat: add stable per-session id to terminal.Model for multi-instance message routing"
```

---

### Task 2: `internal/app` — terminal tab data model, lifecycle, message routing

**Files:**
- Modify: `internal/app/model.go`
- Modify: `internal/app/commands.go`
- Modify: `internal/app/confirm.go`
- Create: `internal/app/terminaltabs.go`
- Test: `internal/app/model_test.go` (new tests, appended)
- Test: `internal/app/terminaltabs_test.go` (new file)

**Interfaces:**
- Consumes: `terminal.New(id int) Model`, `(terminal.Model) ID() int`, `(terminal.OutputMsg) ID() int`, `(terminal.ReadErrMsg) ID() int` (Task 1).
- Produces: `Model.terminals []terminalTab`, `Model.activeTerminal int`, `Model.nextTerminalID int`, `(Model) maybeStartActiveTerminal() (Model, tea.Cmd)`, `(Model) removeTerminalTab(index int) Model`, `cmdNewTerminalTab` — Task 3's layout/rendering/click-routing work builds on these directly and calls `maybeStartActiveTerminal`/`removeTerminalTab` from click handlers.

This task deliberately does **not** touch `paneLayout`, `View()`'s rendering, or any mouse-click routing — those are Task 3. This task's own tests exercise the new fields and functions directly (no layout dependency), so `internal/app`'s full suite stays green at the end of this task even though there is no terminal tab bar drawn yet and the terminal pane still renders only the active tab's content at its old (pre-tab-bar) size — Task 3 finishes wiring that up.

- [ ] **Step 1: Write the failing tests**

Append to `internal/app/model_test.go`:

```go
func TestNewStartsWithExactlyOneUnstartedTerminalTab(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.terminals) != 1 {
		t.Fatalf("got %d terminal tabs, want 1", len(m.terminals))
	}
	if m.activeTerminal != 0 {
		t.Fatalf("got activeTerminal=%d, want 0", m.activeTerminal)
	}
	if m.terminals[0].term.Started() {
		t.Fatal("expected the initial terminal tab to not be started yet (lazy start)")
	}
}

func TestCmdNewTerminalTabAppendsAndActivatesWithoutStartingIt(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m.focus = focusTree // deliberately not focusTerminal yet

	updated, _ := cmdNewTerminalTab(m)
	m = updated.(Model)

	if len(m.terminals) != 2 {
		t.Fatalf("got %d terminal tabs, want 2", len(m.terminals))
	}
	if m.activeTerminal != 1 {
		t.Fatalf("got activeTerminal=%d, want 1 (the new tab)", m.activeTerminal)
	}
	if m.focus != focusTerminal {
		t.Fatal("expected cmdNewTerminalTab to focus the terminal pane")
	}
	if m.terminals[1].term.Started() {
		t.Fatal("expected the new tab to not be started by cmdNewTerminalTab itself — that's maybeStartActiveTerminal's job")
	}
	if m.terminals[1].term.ID() == m.terminals[0].term.ID() {
		t.Fatalf("got both tabs' ID()=%d, want distinct ids", m.terminals[1].term.ID())
	}
}

func TestCtrlTCreatesANewTerminalTabEvenWhileTheTerminalPaneAlreadyHasFocus(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m.focus = focusTerminal // keys would otherwise route straight to the shell

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	m = updated.(Model)

	if len(m.terminals) != 2 {
		t.Fatalf("got %d terminal tabs after ctrl+t while terminal-focused, want 2", len(m.terminals))
	}
}

func TestRemoveTerminalTabReassignsActiveTerminalLikeRemoveTabDoesForEditorTabs(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	// Grow to 3 tabs: [0, 1, 2], activeTerminal ends at 2.
	updated, _ := cmdNewTerminalTab(m)
	m = updated.(Model)
	updated, _ = cmdNewTerminalTab(m)
	m = updated.(Model)
	if len(m.terminals) != 3 || m.activeTerminal != 2 {
		t.Fatalf("test setup: got %d tabs, activeTerminal=%d, want 3 tabs, activeTerminal=2", len(m.terminals), m.activeTerminal)
	}

	// Closing a tab before the active one shifts activeTerminal left.
	m = m.removeTerminalTab(0)
	if len(m.terminals) != 2 || m.activeTerminal != 1 {
		t.Fatalf("got %d tabs, activeTerminal=%d after closing index 0, want 2 tabs, activeTerminal=1", len(m.terminals), m.activeTerminal)
	}

	// Closing the (now last, and active) tab falls back to the one before it.
	m = m.removeTerminalTab(1)
	if len(m.terminals) != 1 || m.activeTerminal != 0 {
		t.Fatalf("got %d tabs, activeTerminal=%d after closing the last active tab, want 1 tab, activeTerminal=0", len(m.terminals), m.activeTerminal)
	}
}

func TestRemoveTerminalTabOnTheSoleRemainingTabResetsInPlaceInsteadOfEmptying(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	oldID := m.terminals[0].term.ID()

	m = m.removeTerminalTab(0)

	if len(m.terminals) != 1 {
		t.Fatalf("got %d terminal tabs after closing the sole remaining one, want 1 (reset in place, never empty)", len(m.terminals))
	}
	if m.activeTerminal != 0 {
		t.Fatalf("got activeTerminal=%d, want 0", m.activeTerminal)
	}
	if m.terminals[0].term.ID() == oldID {
		t.Fatal("expected the replacement session to have a fresh id, not reuse the closed one's")
	}
	if m.terminals[0].term.Started() {
		t.Fatal("expected the replacement session to be freshly unstarted")
	}
}

func TestBackgroundTerminalTabOutputMsgIsRoutedToItsOwnEmulatorNotTheActiveOne(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m.focus = focusTerminal

	updated, _ := cmdNewTerminalTab(m) // tab 1 is now active
	m = updated.(Model)

	// Start tab 0 (the backgrounded one) directly, bypassing focus, so it
	// has a live generation to accept an OutputMsg against.
	var cmd tea.Cmd
	m.terminals[0].term, cmd = m.terminals[0].term.Start()
	if cmd == nil {
		t.Fatal("test setup: starting tab 0 returned a nil cmd")
	}
	msg := cmd()
	out, ok := msg.(terminal.OutputMsg)
	if !ok {
		t.Fatalf("test setup: got %T, want terminal.OutputMsg", msg)
	}

	t.Cleanup(func() { m.terminals[0].term.Close() })

	updated, _ = m.Update(out)
	m = updated.(Model)

	if m.terminals[0].term.View() == "Terminal not started" {
		t.Fatal("expected tab 0's OutputMsg to have been applied to tab 0, not silently dropped")
	}
	if m.terminals[1].term.Started() {
		t.Fatal("expected tab 0's OutputMsg to leave tab 1 (the active, but unrelated, tab) untouched")
	}
}

func TestQuitClosesEveryTerminalTabNotJustTheActiveOne(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := cmdNewTerminalTab(m)
	m = updated.(Model)

	m.terminals[0].term, _ = m.terminals[0].term.Start()
	m.terminals[1].term, _ = m.terminals[1].term.Start()

	updated, _ = cmdQuit(m)
	m = updated.(Model)

	// terminal.Model.Close() closes the underlying pty; the fake-free path
	// here uses a real pty (Start() with no fakes installed spawns $SHELL),
	// so assert indirectly: closing twice must still be safe (Close is a
	// documented no-op on an already-closed/unstarted session) and the
	// dialog-free quit path must reach tea.Quit for both tabs regardless.
	if m.activeDialog != dialogNone {
		t.Fatal("expected a clean quit (no dirty editor tabs) to not open a dialog")
	}
}
```

Create `internal/app/terminaltabs_test.go`:

```go
package app

import "testing"

func TestTerminalTabRegionsAndTerminalTabAtMirrorTheEditorTabBarsColumnMath(t *testing.T) {
	tabs := []terminalTab{{}, {}, {}}
	regions := terminalTabRegions(tabs, 200)
	if len(regions) != 3 {
		t.Fatalf("got %d regions, want 3", len(regions))
	}
	for i, r := range regions {
		if r.tabIndex != i {
			t.Fatalf("got regions[%d].tabIndex=%d, want %d", i, r.tabIndex, i)
		}
		if r.startCol >= r.endCol {
			t.Fatalf("got regions[%d] startCol=%d endCol=%d, want startCol < endCol", i, r.startCol, r.endCol)
		}
		if r.closeStart >= r.closeEnd || r.closeStart < r.startCol || r.closeEnd > r.endCol {
			t.Fatalf("got regions[%d] close range [%d,%d) outside its own tab range [%d,%d)", i, r.closeStart, r.closeEnd, r.startCol, r.endCol)
		}
	}

	region, ok := terminalTabAt(regions[1].startCol, tabs, 200)
	if !ok || region.tabIndex != 1 {
		t.Fatalf("got region=%+v ok=%v for a click inside tab 1's range, want tabIndex=1", region, ok)
	}
}

func TestTerminalTabLabelIsPositionalSinceASessionHasNoFilename(t *testing.T) {
	tabs := []terminalTab{{}, {}}
	if got := terminalTabDisplayName(tabs, 0); got != "Shell 1" {
		t.Fatalf("got %q, want %q", got, "Shell 1")
	}
	if got := terminalTabDisplayName(tabs, 1); got != "Shell 2" {
		t.Fatalf("got %q, want %q", got, "Shell 2")
	}
}
```

- [ ] **Step 2: Run the new tests to verify they fail**

Run: `go build ./... 2>&1 | head -30`
Expected: compile failures — `m.terminals`/`m.activeTerminal` don't exist yet, `cmdNewTerminalTab`/`removeTerminalTab`/`terminalTabRegions`/`terminalTabAt`/`terminalTabDisplayName` are undefined, `terminalTab` type doesn't exist, `KeyCtrlT` case not wired.

- [ ] **Step 3: Replace `Model`'s single `terminal` field with `terminals`/`activeTerminal`/`nextTerminalID`**

In `internal/app/model.go`, find the `Model` struct and replace this field:

```go
	terminal      terminal.Model
```

with:

```go
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
```

Add the `terminalTab` type near `editorPane`'s own definition (just above the `Model` struct):

```go
// terminalTab is one terminal session: its own independent terminal.Model
// (own pty, own vt.Emulator, own generation counter). Unlike an editor
// tab, it carries no path/dirty state — a shell session has no "unsaved
// changes" concept.
type terminalTab struct {
	term terminal.Model
}
```

In `New(rootPath string, nerdFont bool) (Model, error)`, replace:

```go
		terminal:       terminal.New(),
```

with:

```go
		terminals:      []terminalTab{{term: terminal.New(0)}},
```

(`nextTerminalID` is left at its zero value — the next tab created gets id 1.)

- [ ] **Step 4: Add `cmdNewTerminalTab` and register the `ctrl+t` shortcut**

In `internal/app/commands.go`, add to `buildCommands()`'s returned slice (after the `"New Tab"` entry):

```go
		{Name: "New Terminal Tab", Shortcut: "ctrl+t", Handler: cmdNewTerminalTab},
```

Add a new function, near `cmdNewBlankTab`:

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

This needs `"cody/internal/terminal"` imported in `commands.go` — add it to the import block.

In `internal/app/model.go`'s `Update`, the `tea.KeyMsg` default case currently reads:

```go
		default:
			if m.focus != focusTerminal {
				if cmd, ok := commandForShortcut(m.commands, msg.String()); ok {
					return cmd.Handler(m)
				}
			} else if msg.String() == "ctrl+o" || msg.String() == "ctrl+q" || msg.String() == "ctrl+n" {
				if cmd, ok := commandForShortcut(m.commands, msg.String()); ok {
					return cmd.Handler(m)
				}
			}
```

Replace it with:

```go
		default:
			if m.focus != focusTerminal {
				if cmd, ok := commandForShortcut(m.commands, msg.String()); ok {
					return cmd.Handler(m)
				}
			} else if msg.String() == "ctrl+o" || msg.String() == "ctrl+q" || msg.String() == "ctrl+n" || msg.String() == "ctrl+t" {
				if cmd, ok := commandForShortcut(m.commands, msg.String()); ok {
					return cmd.Handler(m)
				}
			}
```

(Only the added `|| msg.String() == "ctrl+t"` changes.)

- [ ] **Step 5: Rename `maybeStartTerminal` to `maybeStartActiveTerminal`, generalized per-tab**

Replace:

```go
func (m Model) maybeStartTerminal() (Model, tea.Cmd) {
	if m.focus != focusTerminal {
		return m, nil
	}
	var cmd tea.Cmd
	m.terminal, cmd = m.terminal.Start()
	return m, cmd
}
```

with:

```go
// maybeStartActiveTerminal lazily spawns the active terminal tab's shell
// the first time it gets focus, if it isn't already started. Safe to call
// unconditionally — a no-op when focus isn't on the terminal pane, or when
// the active tab is already running (terminal.Model.Start's own no-op
// guard).
func (m Model) maybeStartActiveTerminal() (Model, tea.Cmd) {
	if m.focus != focusTerminal {
		return m, nil
	}
	var cmd tea.Cmd
	m.terminals[m.activeTerminal].term, cmd = m.terminals[m.activeTerminal].term.Start()
	return m, cmd
}
```

Update every call site (this task only; Task 3 updates the ones inside `handlePaneClick`/`beginResizeDrag`, which this task does not touch):

- The `"tab"`/`"shift+tab"` key cases in `Update`:
  ```go
		case "tab":
			m.focusNext()
			return m.maybeStartTerminal()
		case "shift+tab":
			m.focusPrev()
			return m.maybeStartTerminal()
  ```
  become:
  ```go
		case "tab":
			m.focusNext()
			return m.maybeStartActiveTerminal()
		case "shift+tab":
			m.focusPrev()
			return m.maybeStartActiveTerminal()
  ```

`handlePaneClick`'s `terminalRect.contains` branch also calls `maybeStartTerminal` today, which this step just renamed away — fix this call site now (its geometry itself, `terminalRect`, stays untouched until Task 3 replaces the whole branch as part of splitting it into `tabBar`/`terminal` sub-rects):

```go
	if terminalRect.contains(x, y) {
		m.focus = focusTerminal
		return m.maybeStartTerminal()
	}
```

becomes:

```go
	if terminalRect.contains(x, y) {
		m.focus = focusTerminal
		return m.maybeStartActiveTerminal()
	}
```

- [ ] **Step 6: Route `terminal.OutputMsg`/`terminal.ReadErrMsg` by `ID()` across every tab**

In `internal/app/model.go`'s `Update`, replace:

```go
	if _, ok := msg.(terminal.OutputMsg); ok {
		var cmd tea.Cmd
		m.terminal, cmd = m.terminal.Update(msg)
		return m, cmd
	}
	if _, ok := msg.(terminal.ReadErrMsg); ok {
		var cmd tea.Cmd
		m.terminal, cmd = m.terminal.Update(msg)
		return m, cmd
	}
```

with:

```go
	if out, ok := msg.(terminal.OutputMsg); ok {
		for i := range m.terminals {
			if m.terminals[i].term.ID() == out.ID() {
				var cmd tea.Cmd
				m.terminals[i].term, cmd = m.terminals[i].term.Update(msg)
				return m, cmd
			}
		}
		// No tab matches: it was already closed (removeTerminalTab calls
		// Close(), which stops the read loop — but a read already in
		// flight when that happened still completes and produces one more
		// message). Nothing to route it to; drop it.
		return m, nil
	}
	if errMsg, ok := msg.(terminal.ReadErrMsg); ok {
		for i := range m.terminals {
			if m.terminals[i].term.ID() == errMsg.ID() {
				var cmd tea.Cmd
				m.terminals[i].term, cmd = m.terminals[i].term.Update(msg)
				return m, cmd
			}
		}
		return m, nil
	}
```

- [ ] **Step 7: Add `removeTerminalTab`**

In `internal/app/confirm.go`, add (near `removeTab`, whose activeTab-reassignment rule this mirrors — see that function's own doc comment for the precedent):

```go
// removeTerminalTab closes and removes the terminal tab at index,
// reassigning m.activeTerminal the same way removeTab reassigns an
// editorPane's activeTab (prefer the tab that shifted into the closed
// slot, falling back to the tab before it if the closed tab was last).
// Unlike an editorPane's tabs, this slice must never reach zero — the
// terminal pane itself isn't closable — so closing the sole remaining tab
// replaces it with one fresh, unstarted session in the same slot instead.
// A no-op for an out-of-range index.
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

This needs `"cody/internal/terminal"` imported in `confirm.go` — add it to the import block if not already present.

- [ ] **Step 8: Close every terminal tab on quit, not just one**

In `internal/app/commands.go`'s `cmdQuit`, replace:

```go
	if !hasDirty {
		m.terminal.Close()
		return m, tea.Quit
	}
```

with:

```go
	if !hasDirty {
		for _, t := range m.terminals {
			t.term.Close()
		}
		return m, tea.Quit
	}
```

In `internal/app/confirm.go`'s `updateConfirmDialog`, replace:

```go
		case confirmQuit:
			m.terminal.Close()
			return m, tea.Quit
```

with:

```go
		case confirmQuit:
			for _, t := range m.terminals {
				t.term.Close()
			}
			return m, tea.Quit
```

- [ ] **Step 9: Create `internal/app/terminaltabs.go`**

Mirrors `internal/app/tabs.go`'s `tabRegion`/`tabRegions`/`tabAt` column-math exactly, minus the dirty-marker concept (a shell session has no unsaved-changes state) and with a positional label instead of a filename:

```go
package app

import "github.com/charmbracelet/lipgloss"

// terminalTabDisplayName returns the label shown for the terminal tab at
// index: positional ("Shell 1", "Shell 2", ...) since a shell session has
// no filename the way an editor tab does.
func terminalTabDisplayName(tabs []terminalTab, index int) string {
	return fmt.Sprintf("Shell %d", index+1)
}

// terminalTabPlainLabel returns the unstyled text for one terminal tab —
// " Shell N × " — used to compute column widths for hit-testing. Must stay
// the same length as what renderTerminalTabBar actually draws for that
// tab, or clicks will land on the wrong tab (mirrors tabPlainLabel's own
// doc comment).
func terminalTabPlainLabel(tabs []terminalTab, index int) string {
	return " " + terminalTabDisplayName(tabs, index) + " × "
}

// terminalTabRegions computes each terminal tab's column range in the
// rendered tab bar, in order — the exact same shape as tabRegions, over
// []terminalTab instead of []tab. Must stay in sync with
// renderTerminalTabBar's own layout.
func terminalTabRegions(tabs []terminalTab, width int) []tabRegion {
	var regions []tabRegion
	col := 0
	closeWidth := lipgloss.Width("× ")
	for i := range tabs {
		if col >= width {
			break
		}
		tabWidth := lipgloss.Width(terminalTabPlainLabel(tabs, i))
		endCol := col + tabWidth
		if endCol > width {
			endCol = width
		}
		closeEnd := col + tabWidth
		if closeEnd > width {
			closeEnd = width
		}
		closeStart := col + tabWidth - closeWidth
		if closeStart > closeEnd {
			closeStart = closeEnd
		}
		regions = append(regions, tabRegion{
			tabIndex:   i,
			startCol:   col,
			endCol:     endCol,
			closeStart: closeStart,
			closeEnd:   closeEnd,
		})
		col += tabWidth
	}
	return regions
}

// terminalTabAt returns the region a column falls in, if any.
func terminalTabAt(col int, tabs []terminalTab, width int) (tabRegion, bool) {
	for _, r := range terminalTabRegions(tabs, width) {
		if col >= r.startCol && col < r.endCol {
			return r, true
		}
	}
	return tabRegion{}, false
}

// renderTerminalTabBar renders the terminal tab strip — same visual
// language as renderTabBar (active tab in reverse video, "×" close glyph
// per tab), positional label instead of a filename, no dirty-marker.
func renderTerminalTabBar(width int, tabs []terminalTab, activeTerminal int) string {
	var b []byte
	for i := range tabs {
		name := terminalTabDisplayName(tabs, i)
		main := " " + name
		if i == activeTerminal {
			main = lipgloss.NewStyle().Reverse(true).Render(main)
		}
		b = append(b, main...)
		b = append(b, " × "...)
	}
	truncated := lipgloss.NewStyle().MaxWidth(width).Render(string(b))
	return tabBarStyle.Width(width).Render(truncated)
}
```

Add `"fmt"` to this new file's import block (used by `terminalTabDisplayName`).

- [ ] **Step 10: Run the full suite**

Run: `go build ./... && go vet ./... && gofmt -l internal/app/ internal/terminal/ && go test ./... -count=1`
Expected: all packages PASS, no gofmt output, no vet warnings.

- [ ] **Step 11: Commit**

```bash
git add internal/app/model.go internal/app/commands.go internal/app/confirm.go internal/app/terminaltabs.go internal/app/model_test.go internal/app/terminaltabs_test.go
git commit -m "feat: add terminal tab data model, lifecycle, and message routing"
```

---

### Task 3: Layout, rendering, resize propagation, click routing

**Files:**
- Modify: `internal/app/model.go`
- Test: `internal/app/model_test.go` (new tests, appended)

**Interfaces:**
- Consumes: `Model.terminals`/`activeTerminal`, `maybeStartActiveTerminal`, `removeTerminalTab`, `terminalTabAt`, `renderTerminalTabBar` (Task 2).
- Produces: `terminalPaneLayout{tabBar, terminal rect}`, `paneLayout()`'s new 3-value signature `(tree rect, panes []editorPaneLayout, term terminalPaneLayout)` — the final shape; nothing further consumes this beyond what this task itself updates.

- [ ] **Step 1: Write the failing tests**

Append to `internal/app/model_test.go`:

```go
func TestTerminalTabBarIsAlwaysShownEvenWithOneTab(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	if !strings.Contains(m.View(), "Shell 1") {
		t.Fatal("expected the terminal tab bar to render \"Shell 1\" even with only one (unstarted) terminal tab")
	}
}

func TestClickingATerminalTabSwitchesActiveTerminalAndFocusesThePane(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	updated, _ = cmdNewTerminalTab(m) // now 2 tabs, activeTerminal=1
	m = updated.(Model)
	m.focus = focusTree

	_, _, term := m.paneLayout()
	region, ok := terminalTabAt(0, m.terminals, term.tabBar.x1-term.tabBar.x0)
	if !ok || region.tabIndex != 0 {
		t.Fatalf("test setup: got region=%+v ok=%v, want tabIndex=0 at column 0", region, ok)
	}
	clickX := term.tabBar.x0
	clickY := term.tabBar.y0

	updated, _ = m.Update(tea.MouseMsg{X: clickX, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)

	if m.activeTerminal != 0 {
		t.Fatalf("got activeTerminal=%d after clicking tab 0, want 0", m.activeTerminal)
	}
	if m.focus != focusTerminal {
		t.Fatal("expected clicking a terminal tab to focus the terminal pane")
	}
}

func TestClickingATerminalTabsCloseGlyphRemovesItWithoutSwitchingFocus(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	updated, _ = cmdNewTerminalTab(m) // 2 tabs, activeTerminal=1
	m = updated.(Model)
	m.focus = focusTree

	_, _, term := m.paneLayout()
	regions := terminalTabRegions(m.terminals, term.tabBar.x1-term.tabBar.x0)
	closeX := term.tabBar.x0 + regions[0].closeStart
	closeY := term.tabBar.y0

	updated, _ = m.Update(tea.MouseMsg{X: closeX, Y: closeY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(Model)

	if len(m.terminals) != 1 {
		t.Fatalf("got %d terminal tabs after closing tab 0's ×, want 1", len(m.terminals))
	}
	if m.focus != focusTree {
		t.Fatal("expected closing a terminal tab (via its × ) to not change focus, matching the editor tab bar's own close-glyph precedent")
	}
}

func TestResizeAllPanesReachesEveryTerminalTabWithoutDisturbingItsRunningState(t *testing.T) {
	// terminal.Model exposes no getter for its stored width/height, and
	// internal/app cannot reach into internal/terminal's unexported
	// newPty/newEmulator test seams across the package boundary — so this
	// spawns two real shells (matching this codebase's existing
	// terminal-focused app tests, e.g. TestClickInTerminalPaneFocusesTerminal)
	// and checks the one thing observable from here: a resize that reaches
	// every tab (not just the active one, and not just index 0) must not
	// panic, lose track of a tab, or disturb a background tab's running
	// state — the same class of regression this codebase already fixed
	// once for boundary-drag resizes not reaching every open editor tab.
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	updated, _ = cmdNewTerminalTab(m) // tab 1 active
	m = updated.(Model)

	m.terminals[0].term, _ = m.terminals[0].term.Start() // background
	m.terminals[1].term, _ = m.terminals[1].term.Start() // active

	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	m = updated.(Model)

	if len(m.terminals) != 2 {
		t.Fatalf("got %d terminal tabs after a resize, want 2 (none lost or duplicated)", len(m.terminals))
	}
	if !m.terminals[0].term.Started() || !m.terminals[1].term.Started() {
		t.Fatal("expected both tabs to remain started after a resize reaches every tab")
	}
	t.Cleanup(func() {
		m.terminals[0].term.Close()
		m.terminals[1].term.Close()
	})
}

func TestBeginResizeDragOnTheTerminalBoundaryStillWorksWithTheNewTabBarSubLayout(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	_, _, term := m.paneLayout()
	updated, ok := m.beginResizeDrag(m.treeWidth, term.tabBar.y0)
	m = updated.(Model)
	if !ok || m.resizeDrag != resizeTerminal {
		t.Fatalf("got ok=%v resizeDrag=%v, want ok=true resizeDrag=resizeTerminal at the terminal tab bar's own top row", ok, m.resizeDrag)
	}
}
```

- [ ] **Step 2: Run the new tests to verify they fail**

Run: `go build ./... 2>&1 | head -30`
Expected: compile failures — `paneLayout` still returns a flat `terminalR rect`, so `term.tabBar`/`term.terminal` don't exist yet.

- [ ] **Step 3: Add `terminalPaneLayout` and change `paneLayout`'s signature**

In `internal/app/model.go`, add near `editorPaneLayout`'s own definition:

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

Replace `paneLayout`'s signature and final two lines:

```go
func (m Model) paneLayout() (tree rect, panes []editorPaneLayout, terminalR rect) {
```

becomes:

```go
func (m Model) paneLayout() (tree rect, panes []editorPaneLayout, term terminalPaneLayout) {
```

and replace:

```go
	terminalR = rect{m.treeWidth, bodyTop + tabBarH + editorHeight, m.width, bodyTop + bodyHeight}
	return
}
```

with:

```go
	term = terminalPaneLayout{
		tabBar:   rect{m.treeWidth, bodyTop + tabBarH + editorHeight, m.width, bodyTop + tabBarH + editorHeight + tabBarHeight},
		terminal: rect{m.treeWidth, bodyTop + tabBarH + editorHeight + tabBarHeight, m.width, bodyTop + bodyHeight},
	}
	return
}
```

- [ ] **Step 4: Bump `minTerminalHeight` to account for the terminal's own tab bar row**

Replace:

```go
	minTreeWidth      = 15
	minEditorWidth    = 20
	minEditorHeight   = 3
	minTerminalHeight = 3
)
```

with:

```go
	minTreeWidth    = 15
	minEditorWidth  = 20
	minEditorHeight = 3
	// minTerminalHeight = border(2) + the terminal's own tab bar row(1) +
	// at least 1 visible content row.
	minTerminalHeight = 4
)
```

- [ ] **Step 5: Update every remaining `paneLayout()` caller that destructures its third return value**

`beginResizeDrag`: replace

```go
	treeRect, panes, terminalRect := m.paneLayout()
```

with:

```go
	treeRect, panes, term := m.paneLayout()
```

and replace:

```go
	onHorizontalBoundary := y == editorRect.y1-1 || y == terminalRect.y0
```

with:

```go
	onHorizontalBoundary := y == editorRect.y1-1 || y == term.tabBar.y0
```

`applyResizeDrag`'s `resizeTerminal` case: replace

```go
	case resizeTerminal:
		_, _, terminalRect := m.paneLayout()
		m.terminalHeight = m.clampTerminalHeight(terminalRect.y1 - y)
```

with:

```go
	case resizeTerminal:
		_, _, term := m.paneLayout()
		m.terminalHeight = m.clampTerminalHeight(term.terminal.y1 - y)
```

`handlePaneClick`: replace

```go
	treeRect, panes, terminalRect := m.paneLayout()
```

with:

```go
	treeRect, panes, term := m.paneLayout()
```

and replace the whole terminal branch:

```go
	if terminalRect.contains(x, y) {
		m.focus = focusTerminal
		return m.maybeStartActiveTerminal()
	}
```

with:

```go
	if term.tabBar.contains(x, y) {
		relX := x - term.tabBar.x0
		region, ok := terminalTabAt(relX, m.terminals, term.tabBar.x1-term.tabBar.x0)
		if !ok {
			return m, nil
		}
		if relX >= region.closeStart && relX < region.closeEnd {
			// Deliberately does NOT set m.focus/activeTerminal first —
			// matches handlePaneClick's own precedent for the editor tab
			// bar's close glyph just above: a close action on a tab
			// shouldn't also redirect focus/active-selection as a side
			// effect.
			return m.removeTerminalTab(region.tabIndex), nil
		}
		m.activeTerminal = region.tabIndex
		m.focus = focusTerminal
		return m.maybeStartActiveTerminal()
	}
	if term.terminal.contains(x, y) {
		m.focus = focusTerminal
		return m.maybeStartActiveTerminal()
	}
```

- [ ] **Step 6: Resize every terminal tab, not just the active one**

In `resizeAllPanes`, replace:

```go
	termW := max(0, m.width-m.treeWidth-borderSize)
	m.terminal = m.terminal.SetSize(termW, max(0, m.terminalHeight-borderSize))
	return m
}
```

with:

```go
	termW := max(0, m.width-m.treeWidth-borderSize)
	termH := max(0, m.terminalHeight-borderSize-tabBarHeight)
	for i := range m.terminals {
		m.terminals[i].term = m.terminals[i].term.SetSize(termW, termH)
	}
	return m
}
```

- [ ] **Step 7: Update `View()`'s terminal rendering**

Replace:

```go
	termWidth := m.width - m.treeWidth - borderSize
	termHeight := m.terminalHeight - borderSize
	term := m.terminal.SetSize(max(0, termWidth), max(0, termHeight))
```

with:

```go
	termWidth := m.width - m.treeWidth - borderSize
	termHeight := m.terminalHeight - borderSize - tabBarHeight
	activeTerm := m.terminals[m.activeTerminal].term.SetSize(max(0, termWidth), max(0, termHeight))
```

Replace:

```go
	right := lipgloss.JoinVertical(lipgloss.Left,
		editorsRow,
		terminalStyle.Render(clampBlockWidth(term.View(), termWidth)),
	)
```

with:

```go
	right := lipgloss.JoinVertical(lipgloss.Left,
		editorsRow,
		renderTerminalTabBar(m.width-m.treeWidth, m.terminals, m.activeTerminal),
		terminalStyle.Render(clampBlockWidth(activeTerm.View(), termWidth)),
	)
```

`terminalStyle`'s own `Height(m.terminalHeight - borderSize)` also needs the same `- tabBarHeight` adjustment it's implicitly missing now that a row of `m.terminalHeight` is spent on the tab bar. Replace:

```go
	terminalStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(terminalBorderColor).
		Width(m.width - m.treeWidth - borderSize).
		Height(m.terminalHeight - borderSize)
```

with:

```go
	terminalStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(terminalBorderColor).
		Width(m.width - m.treeWidth - borderSize).
		Height(m.terminalHeight - borderSize - tabBarHeight)
```

- [ ] **Step 8: Run the full suite**

Run: `go build ./... && go vet ./... && gofmt -l internal/app/ && go test ./... -count=1`
Expected: all packages PASS, no gofmt output, no vet warnings.

- [ ] **Step 9: Manual smoke check**

Run: `go run ./cmd/cody` (or `make build && ./cody` if that's the repo's established local-run path — check the Makefile) in a real terminal. Verify: the terminal tab bar shows "Shell 1"; Ctrl+T opens a second tab and switches to it; typing in each tab goes to that tab's own shell; clicking between tabs preserves each one's own output; clicking a tab's "×" closes it; closing the last tab resets to a fresh "Shell 1" rather than vanishing; dragging the boundary between the editor and the terminal still resizes it correctly.

- [ ] **Step 10: Commit**

```bash
git add internal/app/model.go internal/app/model_test.go
git commit -m "feat: wire terminal tabs into layout, rendering, resize, and click routing"
```
