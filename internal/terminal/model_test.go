package terminal

import (
	"errors"
	"io"
	"os/exec"
	"strings"
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
	// sbLines is test-controlled scrollback content, oldest first —
	// independent of written/Write, since real scrollback population is
	// the vt library's own internal concern (see Scrollback in
	// charmbracelet/x/vt), not something this package's Write forwards to
	// in the fake.
	sbLines []string
	// pushOnWrite, if set, is appended to sbLines the next time Write is
	// called (then cleared) — simulates a real Write pushing new lines
	// into scrollback as a side effect, so a test can control exactly
	// when that growth is observed relative to Update's own
	// before/after ScrollbackLen() measurement.
	pushOnWrite []string
}

func (f *fakeEmulator) Write(p []byte) (int, error) {
	f.written = append(f.written, p...)
	if len(f.pushOnWrite) > 0 {
		f.sbLines = append(f.sbLines, f.pushOnWrite...)
		f.pushOnWrite = nil
	}
	return len(p), nil
}

func (f *fakeEmulator) Render() string {
	return string(f.written)
}

func (f *fakeEmulator) Resize(w, h int) {
	f.w, f.h = w, h
	f.resizes++
}

func (f *fakeEmulator) ScrollbackLen() int {
	return len(f.sbLines)
}

func (f *fakeEmulator) ScrollbackLine(index int) string {
	if index < 0 || index >= len(f.sbLines) {
		return ""
	}
	return f.sbLines[index]
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

	m := New(1)
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

	m := New(1)
	m, _ = m.Start()

	if got := m.View(); got != "Terminal error: terminal: boom" {
		t.Fatalf("got %q", got)
	}
}

func TestStartSurfacesCommandStartError(t *testing.T) {
	p := &fakePty{startErr: errors.New("no shell")}
	e := &fakeEmulator{}
	withFakes(t, p, e)

	m := New(1)
	m, _ = m.Start()

	if got := m.View(); got != "Terminal error: terminal: no shell" {
		t.Fatalf("got %q", got)
	}
}

func TestOutputMsgWritesIntoEmulatorAndReschedulesRead(t *testing.T) {
	p := &fakePty{toRead: [][]byte{[]byte("more")}}
	e := &fakeEmulator{}
	withFakes(t, p, e)

	m := New(1)
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

	m := New(1)
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

	m := New(1)
	m, _ = m.Start()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ls")})

	if string(p.writes) != "ls" {
		t.Fatalf("got %q, want %q", p.writes, "ls")
	}
}

func TestKeyPressBeforeStartIsANoOp(t *testing.T) {
	m := New(1)
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

	m := New(1)
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

	m := New(1)
	m, _ = m.Start()

	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !p.closed {
		t.Fatal("expected the pty to be closed")
	}
}

func TestCloseBeforeStartIsANoOp(t *testing.T) {
	m := New(1)
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestViewShowsTerminalNotStartedBeforeFirstFocus(t *testing.T) {
	m := New(1)
	if got := m.View(); got != "Terminal not started" {
		t.Fatalf("got %q", got)
	}
}

// Regression test: an aggressive window resize can shrink the app's
// computed pane width below zero before it reaches SetSize (see
// app.Model's WindowSizeMsg handling) — a negative width/height reaching
// the real vt.Emulator's Resize panics with "slice bounds out of range"
// deep inside its buffer resize (not something this package can catch
// after the fact). SetSize must clamp to zero itself rather than trust
// the caller, since a third-party library's Resize has no obligation to
// handle negative input gracefully.
func TestSetSizeClampsNegativeDimensionsToZero(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{}
	withFakes(t, p, e)

	m := New(1)
	m, _ = m.Start()
	m = m.SetSize(-5, 23)

	if e.w < 0 || e.h < 0 {
		t.Fatalf("got emulator resize (%d, %d), want both clamped to >= 0", e.w, e.h)
	}
	if p.resizeW < 0 || p.resizeH < 0 {
		t.Fatalf("got pty resize (%d, %d), want both clamped to >= 0", p.resizeW, p.resizeH)
	}
}

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

func TestScrollLinesClampsToZeroAndToScrollbackLength(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{sbLines: []string{"a", "b", "c"}}
	withFakes(t, p, e)

	m := New(1).SetSize(10, 3)
	m, _ = m.Start()

	m = m.ScrollLines(1) // positive n scrolls down; already at the bottom
	if m.scrollOffset != 0 {
		t.Fatalf("got scrollOffset=%d after scrolling down from the bottom, want 0 (can't scroll past following live output)", m.scrollOffset)
	}

	m = m.ScrollLines(-100) // scroll way up, past all available scrollback
	if m.scrollOffset != 3 {
		t.Fatalf("got scrollOffset=%d after scrolling far past available scrollback, want 3 (clamped to ScrollbackLen())", m.scrollOffset)
	}
}

func TestViewAtZeroOffsetIsUnchangedFromPriorBehavior(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{sbLines: []string{"old0", "old1"}}
	withFakes(t, p, e)

	m := New(1).SetSize(10, 3)
	m, _ = m.Start()
	e.written = []byte("live0\nlive1\nlive2")

	if got, want := m.View(), "live0\nlive1\nlive2"; got != want {
		t.Fatalf("got View()=%q at scrollOffset=0, want %q — exactly Render()'s own output, unchanged from before scrollback existed (no scrollbar overlay while following)", got, want)
	}
}

func TestViewWhenScrolledUpShowsScrollbackContentAndDropsOldestLiveLine(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{sbLines: []string{"old0", "old1", "old2", "old3", "old4"}}
	withFakes(t, p, e)

	m := New(1).SetSize(20, 3)
	m, _ = m.Start()
	e.written = []byte("live0\nlive1\nlive2")

	m = m.ScrollLines(-1) // scroll up by 1 line

	view := m.View()
	if !strings.Contains(view, "old4") {
		t.Fatalf("expected the scrolled view to show old4 (the newest scrollback line), got %q", view)
	}
	if !strings.Contains(view, "live0") || !strings.Contains(view, "live1") {
		t.Fatalf("expected the scrolled view to still show live0 and live1, got %q", view)
	}
	if strings.Contains(view, "live2") {
		t.Fatalf("expected the scrolled view to have dropped live2 (the newest live line) off the bottom, got %q", view)
	}
	if got := strings.Count(view, "\n"); got != 2 {
		t.Fatalf("got %d newlines in the scrolled view, want 2 (3 rows)", got)
	}
}

func TestKeyPressResumesFollowingAfterScrollingUp(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{sbLines: []string{"old0", "old1"}}
	withFakes(t, p, e)

	m := New(1).SetSize(10, 3)
	m, _ = m.Start()
	m = m.ScrollLines(-1)
	if m.scrollOffset == 0 {
		t.Fatal("test setup: expected scrollOffset > 0 after scrolling up")
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated

	if m.scrollOffset != 0 {
		t.Fatalf("got scrollOffset=%d after a keypress, want 0 (any input resumes following live output)", m.scrollOffset)
	}
}

func TestOutputReceivedWhilePausedKeepsViewedWindowPinned(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{sbLines: []string{"old0", "old1", "old2"}, pushOnWrite: []string{"old3", "old4"}}
	withFakes(t, p, e)

	m := New(1).SetSize(10, 3)
	m, _ = m.Start()
	m = m.ScrollLines(-1)
	if m.scrollOffset != 1 {
		t.Fatalf("test setup: got scrollOffset=%d, want 1", m.scrollOffset)
	}

	updated, _ := m.Update(OutputMsg{id: 1, generation: m.generation, data: []byte("more")})
	m = updated

	if m.scrollOffset != 3 {
		t.Fatalf("got scrollOffset=%d after 2 new scrollback lines arrived while paused, want 3 (1 + 2 — the viewed window stays pinned to the same absolute content as new output pushes the live bottom down)", m.scrollOffset)
	}
}

func TestOutputReceivedWhileFollowingStaysAtOffsetZero(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{pushOnWrite: []string{"old0", "old1"}}
	withFakes(t, p, e)

	m := New(1).SetSize(10, 3)
	m, _ = m.Start()

	updated, _ := m.Update(OutputMsg{id: 1, generation: m.generation, data: []byte("more")})
	m = updated

	if m.scrollOffset != 0 {
		t.Fatalf("got scrollOffset=%d after output arrived while following, want 0 (nothing to pin — the live view keeps following automatically)", m.scrollOffset)
	}
}
