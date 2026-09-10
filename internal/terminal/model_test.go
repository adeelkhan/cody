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
