# Phase 5: Embedded Terminal Pane Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a third, real interactive terminal pane (spawned `$SHELL` in a
pseudo-terminal, rendered via a VT100/ANSI emulator) alongside the existing
project tree and editor panes, focus-cycled via `Tab`/`Shift+Tab`.

**Architecture:** A new `internal/terminal` package mirrors the existing
`editor`/`filetree` package shape (`Model` with `Update`/`View`/`SetSize`).
It spawns `$SHELL` lazily on first focus via `xpty.NewPty`, reads its output
in a background-goroutine-driven `tea.Cmd` loop (the standard Bubble-Tea
external-I/O pattern) into a `vt.Emulator`, and renders `Emulator.Render()`
each frame. Raw keypresses are encoded to real terminal byte sequences and
written to the PTY when the pane has focus. `internal/app` wires this in
as a third focus target, with a small allowlist of keys that stay global
(chrome-level) regardless of focus.

**Tech Stack:** `github.com/charmbracelet/x/xpty` (PTY spawn/resize,
cross-platform), `github.com/charmbracelet/x/vt` (VT100/ANSI emulator).
Both verified working end-to-end via a research spike before this plan was
written (real PTY spawn of `printf`, real byte read-through-emulator,
real resize on both the PTY and the emulator).

**Spec:** `docs/superpowers/specs/2026-09-05-cody-tui-editor-design.md`
(§2 tech stack, §7 terminal pane, §8 keybindings, §10 error handling, §11
testing strategy — all updated ahead of this plan with Phase 5's concrete
architecture decisions).

## Global Constraints

- Shell spawns **lazily on first focus**, never at startup.
- `xpty`/`vt` are the terminal stack (not the spec's original
  `creack/pty`/`hinshun/vt10x` draft — superseded, see spec §2).
- Verified exact versions from the research spike: `xpty v0.1.4` (tagged
  release), `vt v0.0.0-20260902165432-6f6ad8b37b0a` (pre-release
  pseudo-version — the actively-developed package in this family, no
  tagged release yet; accepted tradeoff, same risk category already
  accepted for `go-tree-sitter`).
- **Ctrl+O (open) and Ctrl+Q (quit) always stay global**, intercepted
  regardless of which pane has focus — unchanged from existing behavior.
  **Ctrl+S/X/C/V/Z/Y/K stop being globally intercepted while the terminal
  pane has focus** — they pass through to the shell instead (a real
  terminal must let Ctrl+C send SIGINT, not silently trigger "Copy"). This
  is a deliberate, spec-required narrowing of the existing "shortcuts work
  regardless of focus" contract established in Phase 2 — it does not
  change behavior for the tree or editor panes.
- **`Ctrl+Q` must cleanly terminate the spawned shell process** before the
  app exits — orphaned/zombie shell processes are a correctness bug, not
  cosmetic. Known, documented, and accepted limitation: abrupt termination
  (SIGKILL, a force-closed terminal window) can still leave the shell
  process running — only the graceful `Ctrl+Q` path is guaranteed to clean
  up. This matches the general Unix limitation that a killed parent cannot
  run its own cleanup code, and is out of scope to solve via process groups
  in this phase.
- The terminal pane's rendered block is passed through the existing
  `clampBlockWidth` helper (added in the viewport-scrolling fix work)
  before composition, exactly like the editor/tree panes — defense in
  depth. The `vt.Emulator` is expected to always produce an exact
  `cols`×`rows` grid (verified in the research spike), so this should be a
  no-op in practice, but costs nothing and closes off a fourth possible
  occurrence of the Lip-Gloss-hard-wrap bug class this project has already
  hit three times.
- No restart-the-terminal feature in this phase: if the shell process
  exits (e.g. the user types `exit`), the pane keeps showing its last
  rendered frame with no further input effect. Documented as a known v1
  limitation, not a bug — restart could be a future enhancement.
- Arrow/navigation keys use "normal" (non-application) cursor-key mode
  xterm sequences. A full-screen program that explicitly switches the
  terminal into application cursor-key mode (DECCKM) — rare in practice —
  may see incorrect arrow-key behavior. Documented as a known v1
  limitation.
- Unit tests use a narrow `Pty`/`Emulator` interface seam (defined in this
  package, satisfied structurally by the real `xpty`/`vt` types) so the
  bulk of `terminal.Model`'s logic is tested with fakes — no real shell
  spawned in those tests. One integration test per the spec's testing
  strategy update DOES spawn a real, short-lived command (`printf`) through
  the real `xpty`+`vt` stack, since the research spike proved this is
  reliably automatable (unlike a long-running full-screen program like
  `vim`, which genuinely needs manual verification).
- No changes to `Buffer`/`editor.Model`/`filetree.Model`'s own logic in
  this plan — only `internal/app` wiring changes to compose the new pane.

---

### Task 1: `internal/terminal` core (Pty/Emulator seam, lazy spawn, read loop, render)

**Files:**
- Create: `internal/terminal/pty.go`
- Create: `internal/terminal/emulator.go`
- Create: `internal/terminal/model.go`
- Test: `internal/terminal/model_test.go`

**Interfaces:**
- Produces: `terminal.Model` with `New() Model`, `(m Model) SetSize(width, height int) Model`, `(m Model) Update(msg tea.Msg) (Model, tea.Cmd)`, `(m Model) View() string`, `(m Model) Close() error`, `(m Model) Started() bool`; exported message types `terminal.OutputMsg` and `terminal.ReadErrMsg` (exported specifically so `internal/app` can unconditionally pass them through to `terminal.Model.Update` the same way it already does for `editor.RehighlightMsg`).
- Consumes: nothing from other tasks in this plan.

- [ ] **Step 1: Add the two new dependencies**

```bash
go get github.com/charmbracelet/x/xpty@v0.1.4
go get github.com/charmbracelet/x/vt@v0.0.0-20260902165432-6f6ad8b37b0a
go mod tidy
```

Run `go build ./...` — expected: clean (nothing uses the new imports yet,
but the dependency resolution itself must succeed cleanly, matching the
versions already verified in this plan's research spike).

- [ ] **Step 2: Write `internal/terminal/pty.go`**

```go
package terminal

import (
	"io"
	"os/exec"

	"github.com/charmbracelet/x/xpty"
)

// Pty is the narrow slice of xpty.Pty's interface this package actually
// uses. Defined locally (rather than depending on xpty.Pty directly) so
// tests can substitute a fake without spawning a real pseudo-terminal.
type Pty interface {
	io.ReadWriteCloser
	Resize(width, height int) error
	Start(cmd *exec.Cmd) error
}

// newPty is a package-level var so tests can substitute a fake constructor.
var newPty = func(width, height int) (Pty, error) {
	return xpty.NewPty(width, height)
}
```

- [ ] **Step 3: Write `internal/terminal/emulator.go`**

```go
package terminal

import "github.com/charmbracelet/x/vt"

// Emulator is the narrow slice of vt.Emulator's interface this package
// actually uses. Defined locally so tests can substitute a fake.
type Emulator interface {
	Write(p []byte) (int, error)
	Render() string
	Resize(width, height int)
}

// newEmulator is a package-level var so tests can substitute a fake
// constructor.
var newEmulator = func(width, height int) Emulator {
	return vt.NewEmulator(width, height)
}
```

- [ ] **Step 4: Write `internal/terminal/model.go`**

```go
package terminal

import (
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

// OutputMsg carries a chunk of bytes read from the pty. Exported so
// internal/app can pass it through to Update unconditionally, the same
// way it already does for editor.RehighlightMsg.
type OutputMsg struct {
	data       []byte
	generation int
}

// ReadErrMsg signals the pty's read loop stopped (the shell exited, or
// the pty was closed). Exported for the same reason as OutputMsg.
type ReadErrMsg struct {
	generation int
}

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

// SetSize resizes the pane. Resizing the underlying pty/emulator is a real
// syscall/state change, so it only happens when the size actually changed
// — this makes it safe to call every render frame (matching the pattern
// already used for the editor/tree panes) without spamming the child
// process with spurious resize notifications on every keystroke.
func (m Model) SetSize(width, height int) Model {
	changed := width != m.width || height != m.height
	m.width, m.height = width, height
	if changed {
		if m.pty != nil {
			m.pty.Resize(width, height)
		}
		if m.emu != nil {
			m.emu.Resize(width, height)
		}
	}
	return m
}

func (m Model) Started() bool {
	return m.started
}

// Start spawns $SHELL in a pseudo-terminal, if not already started, and
// returns a command that kicks off the continuous pty-read loop. Safe to
// call more than once — a no-op after the first call.
func (m Model) Start() (Model, tea.Cmd) {
	if m.started {
		return m, nil
	}
	m.started = true

	w, h := m.width, m.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}

	p, err := newPty(w, h)
	if err != nil {
		m.err = fmt.Errorf("terminal: %w", err)
		return m, nil
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	if err := p.Start(exec.Command(shell)); err != nil {
		m.err = fmt.Errorf("terminal: %w", err)
		return m, nil
	}

	m.pty = p
	m.emu = newEmulator(w, h)
	m.generation++
	return m, readCmd(m.pty, m.generation)
}

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

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case OutputMsg:
		if msg.generation != m.generation || m.emu == nil {
			return m, nil
		}
		m.emu.Write(msg.data)
		return m, readCmd(m.pty, m.generation)
	case ReadErrMsg:
		// The shell exited (or the pty closed). Stop reading; the last
		// rendered frame stays visible. No restart in this phase (see
		// plan's Global Constraints).
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.pty == nil {
		return m, nil
	}
	seq := encodeKey(msg)
	if seq == nil {
		return m, nil
	}
	m.pty.Write(seq)
	return m, nil
}

func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Terminal error: %s", m.err)
	}
	if m.emu == nil {
		return "Terminal not started"
	}
	return m.emu.Render()
}

// Close terminates the spawned shell process and releases the pty. Safe
// to call even if the terminal was never started.
func (m Model) Close() error {
	if m.pty == nil {
		return nil
	}
	return m.pty.Close()
}
```

Note: `encodeKey` is referenced here but defined in Task 2. This task
will not compile green on its own until Task 2 lands — see Step 5 below,
which accounts for this explicitly in the RED/GREEN sequence.

- [ ] **Step 5: Write `internal/terminal/model_test.go` using fakes**

```go
package terminal

import (
	"errors"
	"io"
	"os/exec"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type fakePty struct {
	writes   []byte
	toRead   [][]byte
	closed   bool
	resizeW  int
	resizeH  int
	resizes  int
	startErr error
	readErr  error
}

func (f *fakePty) Read(p []byte) (int, error) {
	if len(f.toRead) == 0 {
		if f.readErr != nil {
			return 0, f.readErr
		}
		return 0, io.EOF
	}
	chunk := f.toRead[0]
	f.toRead = f.toRead[1:]
	n := copy(p, chunk)
	return n, nil
}

func (f *fakePty) Write(p []byte) (int, error) {
	f.writes = append(f.writes, p...)
	return len(p), nil
}

func (f *fakePty) Close() error {
	f.closed = true
	return nil
}

func (f *fakePty) Resize(w, h int) error {
	f.resizeW, f.resizeH = w, h
	f.resizes++
	return nil
}

func (f *fakePty) Start(cmd *exec.Cmd) error {
	return f.startErr
}

type fakeEmulator struct {
	written []byte
	w, h    int
	resizes int
}

func (f *fakeEmulator) Write(p []byte) (int, error) {
	f.written = append(f.written, p...)
	return len(p), nil
}

func (f *fakeEmulator) Render() string {
	return string(f.written)
}

func (f *fakeEmulator) Resize(w, h int) {
	f.w, f.h = w, h
	f.resizes++
}

func withFakes(t *testing.T, p *fakePty, e *fakeEmulator) {
	t.Helper()
	origPty, origEmu := newPty, newEmulator
	newPty = func(width, height int) (Pty, error) { return p, nil }
	newEmulator = func(width, height int) Emulator { return e }
	t.Cleanup(func() {
		newPty = origPty
		newEmulator = origEmu
	})
}

func TestStartSpawnsOnlyOnce(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{}
	withFakes(t, p, e)

	m := New()
	spawns := 0
	origPty := newPty
	newPty = func(width, height int) (Pty, error) {
		spawns++
		return p, nil
	}
	t.Cleanup(func() { newPty = origPty })

	m, _ = m.Start()
	m, _ = m.Start()

	if spawns != 1 {
		t.Fatalf("got %d spawns, want 1", spawns)
	}
	if !m.Started() {
		t.Fatal("expected Started() to be true")
	}
}

func TestStartSurfacesPtyCreationError(t *testing.T) {
	origPty := newPty
	newPty = func(width, height int) (Pty, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { newPty = origPty })

	m := New()
	m, _ = m.Start()

	if got := m.View(); got != "Terminal error: terminal: boom" {
		t.Fatalf("got %q", got)
	}
}

func TestStartSurfacesCommandStartError(t *testing.T) {
	p := &fakePty{startErr: errors.New("no shell")}
	e := &fakeEmulator{}
	withFakes(t, p, e)

	m := New()
	m, _ = m.Start()

	if got := m.View(); got != "Terminal error: terminal: no shell" {
		t.Fatalf("got %q", got)
	}
}

func TestOutputMsgWritesIntoEmulatorAndReschedulesRead(t *testing.T) {
	p := &fakePty{toRead: [][]byte{[]byte("more")}}
	e := &fakeEmulator{}
	withFakes(t, p, e)

	m := New()
	m, cmd := m.Start()
	if cmd == nil {
		t.Fatal("expected a read command from Start")
	}

	msg := cmd()
	outMsg, ok := msg.(OutputMsg)
	if !ok {
		t.Fatalf("got %T, want OutputMsg", msg)
	}

	m, cmd = m.Update(outMsg)
	if string(e.written) != string(outMsg.data) {
		t.Fatalf("emulator got %q, want %q", e.written, outMsg.data)
	}
	if cmd == nil {
		t.Fatal("expected Update to reschedule another read")
	}
}

func TestStaleOutputMsgFromOldGenerationIsIgnored(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{}
	withFakes(t, p, e)

	m := New()
	m, _ = m.Start()

	stale := OutputMsg{data: []byte("old"), generation: m.generation - 1}
	m, cmd := m.Update(stale)

	if len(e.written) != 0 {
		t.Fatalf("expected stale message to be ignored, got %q", e.written)
	}
	if cmd != nil {
		t.Fatal("expected no command for a stale message")
	}
}

func TestKeyPressWritesEncodedBytesToPty(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{}
	withFakes(t, p, e)

	m := New()
	m, _ = m.Start()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ls")})

	if string(p.writes) != "ls" {
		t.Fatalf("got %q, want %q", p.writes, "ls")
	}
}

func TestKeyPressBeforeStartIsANoOp(t *testing.T) {
	m := New()
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if cmd != nil {
		t.Fatal("expected no command when the terminal was never started")
	}
	_ = m
}

func TestSetSizeOnlyResizesWhenSizeActuallyChanges(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{}
	withFakes(t, p, e)

	m := New()
	m, _ = m.Start()
	m = m.SetSize(80, 24)
	m = m.SetSize(80, 24)
	m = m.SetSize(80, 24)

	if p.resizes != 1 {
		t.Fatalf("got %d pty resizes, want 1 (idempotent for unchanged size)", p.resizes)
	}
	if e.resizes != 1 {
		t.Fatalf("got %d emulator resizes, want 1", e.resizes)
	}

	m = m.SetSize(100, 30)
	if p.resizes != 2 {
		t.Fatalf("got %d pty resizes after a real size change, want 2", p.resizes)
	}
}

func TestCloseClosesThePty(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{}
	withFakes(t, p, e)

	m := New()
	m, _ = m.Start()

	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !p.closed {
		t.Fatal("expected the pty to be closed")
	}
}

func TestCloseBeforeStartIsANoOp(t *testing.T) {
	m := New()
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestViewShowsTerminalNotStartedBeforeFirstFocus(t *testing.T) {
	m := New()
	if got := m.View(); got != "Terminal not started" {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 6: Run tests to verify they fail (RED)**

Run: `go test ./internal/terminal/... -v`
Expected: compile failure — `encodeKey` is undefined (it's Task 2's
deliverable). This is expected and matches the plan's design: Task 1 and
Task 2 are two independently-reviewable slices of the same package, and
Task 1's tests exercise `encodeKey` only through `TestKeyPressWritesEncodedBytesToPty`.

- [ ] **Step 7: Add a minimal `encodeKey` stub so Task 1 can reach GREEN on its own**

Add to `internal/terminal/model.go` (Task 2 will replace this with the
real implementation — this stub only needs to satisfy Task 1's one test
that exercises it):

```go
// encodeKey is completed in Task 2. This minimal version exists so Task 1
// can be independently tested and reviewed before Task 2 lands.
func encodeKey(msg tea.KeyMsg) []byte {
	if msg.Type == tea.KeyRunes {
		return []byte(string(msg.Runes))
	}
	return nil
}
```

- [ ] **Step 8: Run tests to verify they pass (GREEN)**

Run: `go test ./internal/terminal/... -v`
Expected: PASS, all tests from Step 5.

Also run `go build ./...` and `go vet ./...` for the whole repo — expected:
clean (nothing else references this new package yet).

- [ ] **Step 9: Commit**

```bash
git add go.mod go.sum internal/terminal/pty.go internal/terminal/emulator.go internal/terminal/model.go internal/terminal/model_test.go
git commit -m "feat: add internal/terminal core (pty spawn, read loop, render, resize, close)"
```

---

### Task 2: Key encoding (`encodeKey`) and a real-shell integration test

**Files:**
- Modify: `internal/terminal/model.go` (replace Task 1's stub `encodeKey`)
- Create: `internal/terminal/keys_test.go`
- Create: `internal/terminal/integration_test.go`

**Interfaces:**
- Consumes: `terminal.Model`, `terminal.Pty`, `terminal.Emulator`,
  `newPty`/`newEmulator` package vars from Task 1.
- Produces: the real `encodeKey(tea.KeyMsg) []byte`, replacing Task 1's
  stub.

- [ ] **Step 1: Write `internal/terminal/keys_test.go` (the failing tests)**

```go
package terminal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEncodeKeyRunesPassThrough(t *testing.T) {
	got := encodeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hi")})
	if string(got) != "hi" {
		t.Fatalf("got %q, want %q", got, "hi")
	}
}

func TestEncodeKeySpace(t *testing.T) {
	got := encodeKey(tea.KeyMsg{Type: tea.KeySpace})
	if string(got) != " " {
		t.Fatalf("got %q, want %q", got, " ")
	}
}

func TestEncodeKeyEnterSendsCarriageReturn(t *testing.T) {
	got := encodeKey(tea.KeyMsg{Type: tea.KeyEnter})
	if string(got) != "\r" {
		t.Fatalf("got %q, want %q", got, "\r")
	}
}

func TestEncodeKeyTabSendsHorizontalTab(t *testing.T) {
	got := encodeKey(tea.KeyMsg{Type: tea.KeyTab})
	if string(got) != "\t" {
		t.Fatalf("got %q, want %q", got, "\t")
	}
}

func TestEncodeKeyEscSendsEscapeByte(t *testing.T) {
	got := encodeKey(tea.KeyMsg{Type: tea.KeyEsc})
	if len(got) != 1 || got[0] != 0x1b {
		t.Fatalf("got %v, want [0x1b]", got)
	}
}

func TestEncodeKeyBackspaceSendsDEL(t *testing.T) {
	got := encodeKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if len(got) != 1 || got[0] != 0x7f {
		t.Fatalf("got %v, want [0x7f]", got)
	}
}

func TestEncodeKeyCtrlCSendsETX(t *testing.T) {
	got := encodeKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if len(got) != 1 || got[0] != 0x03 {
		t.Fatalf("got %v, want [0x03]", got)
	}
}

func TestEncodeKeyCtrlZSendsSUB(t *testing.T) {
	got := encodeKey(tea.KeyMsg{Type: tea.KeyCtrlZ})
	if len(got) != 1 || got[0] != 0x1a {
		t.Fatalf("got %v, want [0x1a]", got)
	}
}

func TestEncodeKeyArrowsSendStandardXtermSequences(t *testing.T) {
	cases := map[tea.KeyType]string{
		tea.KeyUp:    "\x1b[A",
		tea.KeyDown:  "\x1b[B",
		tea.KeyRight: "\x1b[C",
		tea.KeyLeft:  "\x1b[D",
		tea.KeyHome:  "\x1b[H",
		tea.KeyEnd:   "\x1b[F",
	}
	for kt, want := range cases {
		got := encodeKey(tea.KeyMsg{Type: kt})
		if string(got) != want {
			t.Errorf("key %v: got %q, want %q", kt, got, want)
		}
	}
}

func TestEncodeKeyPageAndDeleteSendStandardSequences(t *testing.T) {
	cases := map[tea.KeyType]string{
		tea.KeyPgUp:   "\x1b[5~",
		tea.KeyPgDown: "\x1b[6~",
		tea.KeyDelete: "\x1b[3~",
		tea.KeyInsert: "\x1b[2~",
	}
	for kt, want := range cases {
		got := encodeKey(tea.KeyMsg{Type: kt})
		if string(got) != want {
			t.Errorf("key %v: got %q, want %q", kt, got, want)
		}
	}
}

func TestEncodeKeyReturnsNilForUnencodableKeys(t *testing.T) {
	if got := encodeKey(tea.KeyMsg{Type: tea.KeyShiftTab}); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (RED)**

Run: `go test ./internal/terminal/... -run TestEncodeKey -v`
Expected: several FAILs against Task 1's stub `encodeKey` (which only
handles `tea.KeyRunes`) — e.g. `TestEncodeKeySpace` gets `nil` instead of
`" "`.

- [ ] **Step 3: Replace the stub in `internal/terminal/model.go`**

Replace the `encodeKey` function added in Task 1's Step 7 with:

```go
// encodeKey translates a Bubble Tea key event into the raw byte sequence a
// real terminal would send for that key, for forwarding to a pty's stdin.
// Returns nil for keys with no defined terminal encoding.
//
// Verified against bubbletea v1.3.10's key.go: KeyType values 0-127 for
// KeyCtrlA..KeyCtrlZ, the Ctrl-punctuation variants, KeyEnter, KeyTab,
// KeyEsc, and KeyBackspace are literally defined as their ASCII
// control-code byte value (e.g. KeyCtrlC = 3, KeyEnter = 13,
// KeyBackspace = 127) — forwarding byte(msg.Type) for that whole range is
// correct by construction, not a coincidence. Named cursor/navigation keys
// (KeyUp, KeyHome, etc.) are separate negative sentinel values needing
// their own standard xterm escape sequences (see this plan's Global
// Constraints for the one documented limitation: normal, not
// application, cursor-key mode).
func encodeKey(msg tea.KeyMsg) []byte {
	switch msg.Type {
	case tea.KeyRunes:
		return []byte(string(msg.Runes))
	case tea.KeySpace:
		return []byte(" ")
	case tea.KeyUp:
		return []byte("\x1b[A")
	case tea.KeyDown:
		return []byte("\x1b[B")
	case tea.KeyRight:
		return []byte("\x1b[C")
	case tea.KeyLeft:
		return []byte("\x1b[D")
	case tea.KeyHome:
		return []byte("\x1b[H")
	case tea.KeyEnd:
		return []byte("\x1b[F")
	case tea.KeyPgUp:
		return []byte("\x1b[5~")
	case tea.KeyPgDown:
		return []byte("\x1b[6~")
	case tea.KeyDelete:
		return []byte("\x1b[3~")
	case tea.KeyInsert:
		return []byte("\x1b[2~")
	}
	if msg.Type >= 0 && msg.Type <= 127 {
		return []byte{byte(msg.Type)}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass (GREEN)**

Run: `go test ./internal/terminal/... -v`
Expected: PASS, every test from Task 1 and Task 2.

- [ ] **Step 5: Write the real-shell integration test**

Create `internal/terminal/integration_test.go`:

```go
package terminal

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// typeLine sends a line of text through m.Update as a real user's
// keystrokes would arrive (one tea.KeyMsg per rune, then Enter), so this
// test exercises the real encodeKey path, not just the pty plumbing.
func typeLine(m Model, line string) Model {
	for _, r := range line {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return m
}

// pumpUntilContains drains OutputMsg/ReadErrMsg from the read loop,
// feeding each OutputMsg back into m, until the rendered view contains
// want or the deadline elapses.
func pumpUntilContains(t *testing.T, m Model, cmd tea.Cmd, want string, timeout time.Duration) Model {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cmd == nil {
			t.Fatalf("read loop stopped before %q appeared; last view:\n%s", want, m.View())
		}
		msg := cmd()
		switch msg := msg.(type) {
		case OutputMsg:
			m, cmd = m.Update(msg)
			if strings.Contains(m.View(), want) {
				return m
			}
		case ReadErrMsg:
			t.Fatalf("pty read loop ended before %q appeared; last view:\n%s", want, m.View())
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %q; last view:\n%s", want, m.View())
		}
	}
}

// TestRealShellSpawnAndOutputRoundTrip spawns a real interactive shell
// through the real xpty+vt stack (no fakes), types a command through the
// same tea.KeyMsg path a real keystroke would take, and confirms the
// shell's real output genuinely reaches the emulator's rendered screen.
// This is the slice of "spawn a real shell" that the research spike
// proved reliably automatable — the spec's testing-strategy note that PTY
// rendering needs manual verification applies to long-running, fully
// interactive programs (vim, htop), not this basic round-trip.
func TestRealShellSpawnAndOutputRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a real subprocess; skipped in -short mode")
	}

	m := New()
	m = m.SetSize(40, 10)
	m, cmd := m.Start()
	if cmd == nil {
		t.Fatal("expected a read command from Start")
	}
	if m.err != nil {
		t.Fatalf("Start failed: %v", m.err)
	}

	// Wait for the shell to be ready for input before typing — the very
	// first OutputMsg (the shell's startup prompt) is a good-enough signal
	// that pty.Write will now reach a live shell rather than being lost
	// before the shell's stdin is being read.
	msg := cmd()
	out, ok := msg.(OutputMsg)
	if !ok {
		t.Fatalf("got %T as the first message, want OutputMsg (the shell prompt)", msg)
	}
	m, cmd = m.Update(out)

	m = typeLine(m, "printf 'cody-spike-marker\\n'")
	m = pumpUntilContains(t, m, cmd, "cody-spike-marker", 3*time.Second)
}
```

- [ ] **Step 6: Run the full package test suite**

Run: `go test ./internal/terminal/... -v`
Expected: PASS, including the real-shell integration test.

Run `go build ./...`, `go vet ./...`, `gofmt -l .` for the whole repo —
expected: all clean.

- [ ] **Step 7: Commit**

```bash
git add internal/terminal/model.go internal/terminal/keys_test.go internal/terminal/integration_test.go
git commit -m "feat: encode key presses to terminal byte sequences; add real-shell round-trip test"
```

---

### Task 3: Wire the terminal pane into `internal/app`

**Files:**
- Modify: `internal/app/model.go`
- Modify: `internal/app/commands.go`
- Test: `internal/app/model_test.go`

**Interfaces:**
- Consumes: `terminal.New()`, `terminal.Model.{SetSize,Start,Update,View,Close,Started}`, `terminal.OutputMsg`, `terminal.ReadErrMsg`, `clampBlockWidth` (already defined in `internal/app/model.go` from the viewport-scrolling fix work).
- Produces: `focusTerminal` as a new `focusArea` value; `app.Model.terminal` field.

- [ ] **Step 1: Write the failing tests**

Add to `internal/app/model_test.go`:

```go
func TestTabCyclesThroughAllThreePanesForwardAndBack(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if m.focus != focusTree {
		t.Fatalf("got initial focus %v, want focusTree", m.focus)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.focus != focusEditor {
		t.Fatalf("got focus %v after one Tab, want focusEditor", m.focus)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.focus != focusTerminal {
		t.Fatalf("got focus %v after two Tabs, want focusTerminal", m.focus)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.focus != focusTree {
		t.Fatalf("got focus %v after three Tabs, want focusTree (wrapped)", m.focus)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = updated.(Model)
	if m.focus != focusTerminal {
		t.Fatalf("got focus %v after one Shift+Tab, want focusTerminal (wrapped backward)", m.focus)
	}
}

func TestFocusingTheTerminalStartsItExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> editor
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> terminal
	m = updated.(Model)
	if !m.terminal.Started() {
		t.Fatal("expected focusing the terminal pane to start it")
	}
	if cmd == nil {
		t.Fatal("expected a command (the pty read loop kickoff) when the terminal starts")
	}

	// Tabbing away and back must not restart it.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> tree
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> editor
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // -> terminal again
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no new start command the second time the terminal gains focus")
	}
}

func TestEditingShortcutsPassThroughToTheTerminalWhenItHasFocus(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(filetree.FileOpenedMsg{Path: file})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // editor -> terminal
	m = updated.(Model)
	if m.focus != focusTerminal {
		t.Fatalf("got focus %v, want focusTerminal", m.focus)
	}

	before := m.recentCommand
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = updated.(Model)
	if m.recentCommand != before {
		t.Fatalf("expected ctrl+c to NOT trigger the Copy command while the terminal has focus, got recentCommand=%q", m.recentCommand)
	}
}

func TestOpenAndQuitStayGlobalEvenWhenTerminalHasFocus(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.focus != focusTerminal {
		t.Fatalf("got focus %v, want focusTerminal", m.focus)
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if cmd == nil {
		t.Fatal("expected ctrl+q to still produce tea.Quit while the terminal has focus")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (RED)**

Run: `go test ./internal/app/... -run 'TestTabCyclesThrough|TestFocusingTheTerminal|TestEditingShortcutsPassThrough|TestOpenAndQuitStayGlobal' -v`
Expected: compile failure (`focusTerminal` undefined) or logic failures
(Tab currently only toggles between two states).

- [ ] **Step 3: Modify `internal/app/model.go`**

Replace the `focusArea` const block:

```go
const (
	focusTree focusArea = iota
	focusEditor
	focusTerminal
)
```

Add the import and the `terminal` field to `Model`:

```go
import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cody/internal/editor"
	"cody/internal/filetree"
	"cody/internal/statusbar"
	"cody/internal/terminal"
)
```

```go
type Model struct {
	tree          filetree.Model
	editor        editor.Model
	terminal      terminal.Model
	focus         focusArea
	projectName   string
	recentCommand string
	width, height int
	commands      []Command
	rootPath      string
	openMenu      string
	activeDialog  dialogKind
	fileOpenInput textinput.Model
	fileOpenError string
	paletteFilter textinput.Model
	paletteCursor int
}
```

In `New`, initialize it:

```go
	return Model{
		tree:        tree,
		editor:      editor.New(),
		terminal:    terminal.New(),
		focus:       focusTree,
		projectName: filepath.Base(absPath),
		rootPath:    absPath,
		commands:    buildCommands(),
	}, nil
```

Replace `toggleFocus` with a forward/backward pair:

```go
func (m *Model) focusNext() {
	switch m.focus {
	case focusTree:
		m.focus = focusEditor
	case focusEditor:
		m.focus = focusTerminal
	case focusTerminal:
		m.focus = focusTree
	}
}

func (m *Model) focusPrev() {
	switch m.focus {
	case focusTree:
		m.focus = focusTerminal
	case focusEditor:
		m.focus = focusTree
	case focusTerminal:
		m.focus = focusEditor
	}
}

// maybeStartTerminal lazily spawns the shell the first time the terminal
// pane gains focus. A no-op on every subsequent focus change.
func (m Model) maybeStartTerminal() (Model, tea.Cmd) {
	if m.focus != focusTerminal {
		return m, nil
	}
	var cmd tea.Cmd
	m.terminal, cmd = m.terminal.Start()
	return m, cmd
}
```

Replace the `WindowSizeMsg` handling at the top of `Update` to also size
the terminal pane:

```go
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		paneHeight := m.height - menuBarHeight - statusBarHeight
		editorHeight := paneHeight - terminalHeight
		m.tree = m.tree.SetSize(treeWidth-borderSize, paneHeight-borderSize)
		m.editor = m.editor.SetSize(m.width-treeWidth-borderSize, editorHeight-borderSize)
		m.terminal = m.terminal.SetSize(m.width-treeWidth-borderSize, terminalHeight-borderSize)
		return m, nil
	}
```

Add the two terminal message pass-throughs right after the existing
`editor.RehighlightMsg` pass-through (same pattern, same place):

```go
	if _, ok := msg.(editor.RehighlightMsg); ok {
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
		return m, cmd
	}
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

Replace the `tea.KeyMsg` case's inner switch:

```go
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			m.focusNext()
			return m.maybeStartTerminal()
		case "shift+tab":
			m.focusPrev()
			return m.maybeStartTerminal()
		case "esc":
			if m.openMenu != "" {
				m.openMenu = ""
				return m, nil
			}
		default:
			if m.focus != focusTerminal {
				if cmd, ok := commandForShortcut(m.commands, msg.String()); ok {
					return cmd.Handler(m)
				}
			} else if msg.String() == "ctrl+o" || msg.String() == "ctrl+q" {
				if cmd, ok := commandForShortcut(m.commands, msg.String()); ok {
					return cmd.Handler(m)
				}
			}
		}
```

Replace the final generic dispatch (currently a two-way `if`) with a
three-way switch:

```go
	var cmd tea.Cmd
	switch m.focus {
	case focusTree:
		m.tree, cmd = m.tree.Update(msg)
	case focusEditor:
		m.editor, cmd = m.editor.Update(msg)
	case focusTerminal:
		m.terminal, cmd = m.terminal.Update(msg)
	}
	return m, cmd
```

In `View`, add a focus-aware border color for the terminal pane, give it
a border matching the tree/editor convention (it currently renders
borderless), locally re-derive its size the same way the editor/tree
panes already do, and route its content through `clampBlockWidth`:

```go
	treeBorderColor := unfocusedBorderColor
	editorBorderColor := unfocusedBorderColor
	terminalBorderColor := unfocusedBorderColor
	switch m.focus {
	case focusTree:
		treeBorderColor = focusedBorderColor
	case focusEditor:
		editorBorderColor = focusedBorderColor
	case focusTerminal:
		terminalBorderColor = focusedBorderColor
	}

	treeStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(treeBorderColor).
		Width(treeWidth - borderSize).
		Height(bodyHeight - borderSize)
	editorStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(editorBorderColor).
		Width(m.width - treeWidth - borderSize).
		Height(editorHeight - borderSize)
	terminalStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(terminalBorderColor).
		Width(m.width - treeWidth - borderSize).
		Height(terminalHeight - borderSize)

	tree := m.tree.SetSize(treeWidth-borderSize, bodyHeight-borderSize)
	editor := m.editor.SetSize(m.width-treeWidth-borderSize, editorHeight-borderSize)
	termWidth := m.width - treeWidth - borderSize
	termHeight := terminalHeight - borderSize
	term := m.terminal.SetSize(termWidth, termHeight)

	right := lipgloss.JoinVertical(lipgloss.Left,
		editorStyle.Render(clampBlockWidth(editor.View(), m.width-treeWidth-borderSize)),
		terminalStyle.Render(clampBlockWidth(term.View(), termWidth)),
	)
	body := lipgloss.JoinHorizontal(lipgloss.Top, treeStyle.Render(clampBlockWidth(tree.View(), treeWidth-borderSize)), right)
```

(This replaces the prior borderless `terminalStyle` and the prior
`right`/`body` construction — keep everything else in `View` as-is.)

- [ ] **Step 4: Wire shell cleanup into quit, in `internal/app/commands.go`**

Replace:

```go
func cmdQuit(m Model) (Model, tea.Cmd) {
	return m, tea.Quit
}
```

with:

```go
func cmdQuit(m Model) (Model, tea.Cmd) {
	m.terminal.Close()
	return m, tea.Quit
}
```

- [ ] **Step 5: Run tests to verify they pass (GREEN)**

Run: `go test ./internal/app/... -v`
Expected: PASS, including every pre-existing Phase 1-4b test (the
three-way focus switch and terminal pass-through must not change tree/
editor behavior when they have focus) and the five new tests from Step 1.

- [ ] **Step 6: Run the whole repo's test suite and static checks**

Run: `go test ./... -v`
Expected: PASS across `app`, `editor`, `filetree`, `highlight`,
`scrollbar`, `statusbar`, `terminal`.

Run `go build ./...`, `go vet ./...`, `gofmt -l .` — expected: all clean.

- [ ] **Step 7: Commit**

```bash
git add internal/app/model.go internal/app/commands.go internal/app/model_test.go
git commit -m "feat: wire the terminal pane into app — 3-way focus cycle, lazy spawn, shortcut passthrough, quit cleanup"
```

---

### Task 4: Final integration test, README, and manual verification checklist

**Files:**
- Test: `internal/app/model_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: everything from Tasks 1-3.

- [ ] **Step 1: Write the final integration test**

Add `"time"` to `internal/app/model_test.go`'s import block if not already
present, then add:

```go
func TestTerminalPaneShowsRealShellOutputThroughTheComposedApp(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a real subprocess; skipped in -short mode")
	}
	dir := t.TempDir()
	m, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // tree -> editor
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab}) // editor -> terminal
	m = updated.(Model)
	if m.focus != focusTerminal {
		t.Fatalf("got focus %v, want focusTerminal", m.focus)
	}
	if cmd == nil {
		t.Fatal("expected a read-loop command once the terminal starts")
	}

	// Wait for the shell's startup prompt before typing, so the typed
	// command reaches a live shell rather than being sent before its
	// stdin is being read — same technique as Task 2's package-level
	// integration test.
	msg := cmd()
	out, ok := msg.(terminal.OutputMsg)
	if !ok {
		t.Fatalf("got %T as the first message, want terminal.OutputMsg (the shell prompt)", msg)
	}
	updated, cmd = m.Update(out)
	m = updated.(Model)

	// Type the command through the exact same tea.KeyMsg path a real
	// keystroke takes, one rune at a time, then Enter.
	command := "printf 'cody-app-marker\\n'"
	for _, r := range command {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	deadline := time.Now().Add(3 * time.Second)
	for {
		if cmd == nil {
			t.Fatalf("read loop stopped before the marker appeared; last view:\n%s", m.View())
		}
		msg := cmd()
		out, ok := msg.(terminal.OutputMsg)
		if !ok {
			t.Fatalf("got %T, want terminal.OutputMsg", msg)
		}
		updated, cmd = m.Update(out)
		m = updated.(Model)
		if strings.Contains(m.View(), "cody-app-marker") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for the marker text through the composed app view; last view:\n%s", m.View())
		}
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/app/... -run TestTerminalPaneShowsRealShellOutputThroughTheComposedApp -v`
Expected: PASS.

Run `go test ./... -v`, `go build ./...`, `go vet ./...`, `gofmt -l .` for
the whole repo — expected: all clean.

- [ ] **Step 3: Update `README.md`**

Modify the "Status" section to describe Phase 5, and add terminal-pane
keybindings/behavior to the "Keybindings" section — read the current
`README.md` first (it was last updated for Phase 4b's fixes) and write
prose consistent with its existing tone, covering:

- The terminal pane now runs a real `$SHELL` in a pseudo-terminal
  (`charmbracelet/x/xpty` + `charmbracelet/x/vt`), spawned the first time
  it gains focus.
- `Tab`/`Shift+Tab` now cycle through all three panes (tree, editor,
  terminal) in both directions.
- While the terminal pane has focus, `Ctrl+S/X/C/V/Z/Y/K` pass through to
  the shell instead of triggering their editor commands — `Ctrl+O`
  (open) and `Ctrl+Q` (quit) remain global.
- `Ctrl+Q` cleanly terminates the spawned shell; abrupt termination
  (closing the terminal window, `kill -9`) may leave it running — a
  known, accepted limitation.
- Known v1 limitations: no terminal-restart after the shell exits; arrow
  keys use normal (not application) cursor-key mode, so a full-screen
  program that explicitly requests application mode may see incorrect
  arrow-key behavior.

- [ ] **Step 4: Manual verification checklist (record results in the task report, not automatable)**

Build the binary (`go build -o cody ./cmd/cody`) and manually verify,
per the spec's testing-strategy note that full-screen interactive PTY
programs need human verification:

- Opening the app, tabbing to the terminal pane, and seeing a real shell
  prompt appear.
- Typing a command (e.g. `ls`) and seeing real output.
- Running a full-screen program (`vim`, or `htop` if available) and
  confirming it renders and responds to input correctly.
- Resizing the terminal window and confirming the shell/full-screen
  program redraws at the new size (`vim`'s status line reflecting the
  new width is a good check).
- Pressing `Ctrl+C` while a long-running command (e.g. `sleep 100`) is
  running in the terminal pane, confirming it's interrupted (not
  intercepted as "Copy").
- Quitting via `Ctrl+Q` and confirming (via `ps` in a separate real
  terminal) that the spawned shell process is gone, not orphaned.

- [ ] **Step 5: Commit**

```bash
git add internal/app/model_test.go README.md
git commit -m "test: add composed-app terminal integration test; docs: update README for phase 5"
```
