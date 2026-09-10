package terminal

import (
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	altScreen   bool
	// altScreenAfterWrite, if non-nil, sets altScreen to this value the
	// next time Write is called (then clears itself) — simulates a real
	// Write transitioning in/out of the alternate screen as a side
	// effect of the bytes it contains (e.g. vim's own alt-screen
	// enter/exit escape sequences arriving in that chunk), independent
	// of whatever IsAltScreen() reported before that particular Write.
	altScreenAfterWrite *bool
}

func (f *fakeEmulator) Write(p []byte) (int, error) {
	f.written = append(f.written, p...)
	if len(f.pushOnWrite) > 0 {
		f.sbLines = append(f.sbLines, f.pushOnWrite...)
		f.pushOnWrite = nil
	}
	if f.altScreenAfterWrite != nil {
		f.altScreen = *f.altScreenAfterWrite
		f.altScreenAfterWrite = nil
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

func (f *fakeEmulator) IsAltScreen() bool {
	return f.altScreen
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

// TestScrollLinesIsANoOpDuringAltScreen and
// TestViewIgnoresStaleScrollOffsetWhenAltScreenBecomesActive cover a real
// correctness bug caught by review on the original version of this
// feature: vt.Emulator.Scrollback() always reports the MAIN screen's
// scrollback, even while a full-screen app (vim, htop, less) has switched
// to the alternate screen — so scrolling during one of those apps used to
// splice stale pre-app shell history in among the app's own live rows.

func TestScrollLinesIsANoOpDuringAltScreen(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{sbLines: []string{"old0", "old1", "old2"}, altScreen: true}
	withFakes(t, p, e)

	m := New(1).SetSize(10, 3)
	m, _ = m.Start()

	m = m.ScrollLines(-1)

	if m.scrollOffset != 0 {
		t.Fatalf("got scrollOffset=%d after scrolling during alt screen, want 0 (real terminals don't scroll into stale main-screen history while a full-screen app owns the display)", m.scrollOffset)
	}
}

func TestViewIgnoresStaleScrollOffsetWhenAltScreenBecomesActive(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{sbLines: []string{"old0", "old1", "old2"}}
	withFakes(t, p, e)

	m := New(1).SetSize(10, 3)
	m, _ = m.Start()
	m = m.ScrollLines(-1) // paused, viewing scrollback, still on the main screen
	if m.scrollOffset == 0 {
		t.Fatal("test setup: expected scrollOffset > 0 after scrolling up")
	}

	// Simulate a full-screen app (vim, htop, less) taking over the
	// display without the user scrolling again in between — matches how
	// the real vt.Emulator's IsAltScreen() flips the moment the app's
	// escape sequence arrives, independent of anything this package does.
	e.altScreen = true
	e.written = []byte("vim-line-A\nvim-line-B\nvim-line-C")

	view := m.View()
	want := "vim-line-A\nvim-line-B\nvim-line-C"
	if view != want {
		t.Fatalf("got View()=%q while alt screen is active with a stale scrollOffset, want %q — the live alt-screen content, not blended with stale main-screen scrollback", view, want)
	}
}

// TestRenderScrolledViewPadsShortLinesSoTheScrollbarStaysAtAFixedColumn
// covers a legitimate finding from PR #8's Greptile bot review:
// lipgloss.NewStyle().MaxWidth() only truncates lines longer than the
// target width, it never pads shorter ones — so a scrollbar appended
// directly after each (unpadded) line landed at a different column on
// every row, drifting left/right depending on each row's own content
// length, instead of staying fixed at the pane's right edge.
func TestRenderScrolledViewPadsShortLinesSoTheScrollbarStaysAtAFixedColumn(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{sbLines: []string{"a", "bb", "ccc"}}
	withFakes(t, p, e)

	m := New(1).SetSize(20, 3)
	m, _ = m.Start()
	e.written = []byte("live0\nlive1\nlive2")
	m = m.ScrollLines(-1)

	lines := strings.Split(m.View(), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	col0 := strings.IndexAny(lines[0], "│█")
	if col0 < 0 {
		t.Fatalf("test setup: row 0 has no scrollbar character at all: %q", lines[0])
	}
	for i, line := range lines {
		col := strings.IndexAny(line, "│█")
		if col != col0 {
			t.Fatalf("got scrollbar column=%d on row %d (%q), want %d (same as row 0) — the scrollbar must stay at a fixed column regardless of each row's own content length", col, i, line, col0)
		}
	}
}

// TestScrollOffsetResetsWhenAltScreenExitsDuringAnOutputMsg covers a real
// gap in the earlier alt-screen fix, caught by CodeRabbit's review of PR
// #8: View() correctly falls back to the plain live render WHILE the alt
// screen is active, but scrollOffset itself was never reset — so once an
// alt-screen app (vim, less, ...) exits, IsAltScreen() goes back to false
// and View() would immediately re-enter renderScrolledView() with the
// STALE pre-app scrollOffset, blending old main-screen scrollback into
// the just-returned live prompt for a frame (until the next scroll or
// keypress happened to reset it).
func TestScrollOffsetResetsWhenAltScreenExitsDuringAnOutputMsg(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{sbLines: []string{"old0", "old1", "old2"}}
	withFakes(t, p, e)

	m := New(1).SetSize(10, 3)
	m, _ = m.Start()
	m = m.ScrollLines(-1) // paused, viewing scrollback, on the main screen
	if m.scrollOffset == 0 {
		t.Fatal("test setup: expected scrollOffset > 0 after scrolling up")
	}

	// Simulate: an alt-screen app started at some point after that (the
	// already-covered transition), and is now exiting within this single
	// OutputMsg — IsAltScreen() reports true going in, false coming out.
	e.altScreen = true
	exiting := false
	e.altScreenAfterWrite = &exiting
	e.written = []byte("prompt-returned")

	updated, _ := m.Update(OutputMsg{id: 1, generation: m.generation, data: []byte("more")})
	m = updated

	if m.scrollOffset != 0 {
		t.Fatalf("got scrollOffset=%d after an alt-screen app exited mid-OutputMsg, want 0 (a stale offset from before the app started must not blend into the just-returned live prompt)", m.scrollOffset)
	}
}

// TestRenderScrolledViewAtZeroWidthRendersNoGutter and
// TestRenderScrolledViewAtWidthOneShowsOnlyTheScrollbarCell cover another
// CodeRabbit finding on PR #8: at m.width == 0, renderScrolledView's own
// `if overlayWidth > 0` guard correctly skipped the (now-negative-width)
// content truncation, but still unconditionally appended the 2-cell
// " "+bar gutter afterward — so a degenerately narrow pane (the same
// aggressively-downsized-window class this codebase already floors
// elsewhere) rendered rows wider than its own claimed width instead of
// nothing at all.

func TestRenderScrolledViewAtZeroWidthRendersNoGutter(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{sbLines: []string{"a", "b", "c"}}
	withFakes(t, p, e)

	m := New(1).SetSize(0, 3)
	m, _ = m.Start()
	e.written = []byte("live0\nlive1\nlive2")
	m = m.ScrollLines(-1)

	for i, line := range strings.Split(m.View(), "\n") {
		if got := lipgloss.Width(line); got != 0 {
			t.Fatalf("got row %d width=%d at pane width 0, want 0 (no content, no gutter): %q", i, got, line)
		}
	}
}

func TestRenderScrolledViewAtWidthOneShowsOnlyTheScrollbarCell(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{sbLines: []string{"a", "b", "c"}}
	withFakes(t, p, e)

	m := New(1).SetSize(1, 3)
	m, _ = m.Start()
	e.written = []byte("live0\nlive1\nlive2")
	m = m.ScrollLines(-1)

	for i, line := range strings.Split(m.View(), "\n") {
		if got := lipgloss.Width(line); got != 1 {
			t.Fatalf("got row %d width=%d at pane width 1, want 1 (scrollbar cell only, no content, no leading space): %q", i, got, line)
		}
	}
}

// TestSetSizeCapturesRowsAHeightShrinkWouldOtherwiseDiscard and
// TestSetSizePreservesRowsLostToARealEmulatorsHeightShrink cover a real,
// user-reported bug: cat a big file, then shrink the terminal pane (drag
// the boundary, or resize the whole window), and the live content —
// including the shell prompt — visibly vanished instead of scrolling into
// history.
//
// Root cause, confirmed directly in the vendored library source
// (charmbracelet/x/ultraviolet's Buffer.Resize): a height shrink is
// implemented as `b.Lines = b.Lines[:height]` — keeping the grid's TOP
// rows and discarding everything below, with no scrollback push at all.
// Since the cursor (and therefore the most recently written content, like
// a shell prompt) sits near the BOTTOM of the grid, this is exactly
// backwards: SetSize now captures the rows about to be destroyed itself,
// oldest-first, into m.shrinkOverflow — an older-than-scrollback tier
// renderScrolledView already knows how to render — before delegating to
// the library's own (lossy) resize.

func TestSetSizeCapturesRowsAHeightShrinkWouldOtherwiseDiscard(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{}
	withFakes(t, p, e)

	m := New(1).SetSize(20, 5)
	m, _ = m.Start()
	e.written = []byte("line0\nline1\nline2\nline3\nline4")

	m = m.SetSize(20, 2) // shrink from 5 rows to 2

	// The library keeps Lines[:2] (line0, line1 — the TOP of the old
	// grid) as the new live screen and discards the rest — so the rows
	// that actually need capturing are line2/line3/line4, NOT line0/
	// line1/line2 (an earlier, wrong version of this test/fix captured
	// the top instead, verified against a live-writing example against
	// the real library directly, not just reasoned about abstractly).
	want := []string{"line2", "line3", "line4"}
	if len(m.shrinkOverflow) != len(want) {
		t.Fatalf("got shrinkOverflow=%q, want %q (the rows a real emulator's Resize would otherwise silently drop — the ones nearest the cursor, not the ones it keeps)", m.shrinkOverflow, want)
	}
	for i, w := range want {
		if m.shrinkOverflow[i] != w {
			t.Fatalf("got shrinkOverflow[%d]=%q, want %q", i, m.shrinkOverflow[i], w)
		}
	}
}

func TestSetSizePreservesRowsLostToARealEmulatorsHeightShrink(t *testing.T) {
	// Uses the REAL vt.Emulator (only newPty is faked, to avoid spawning
	// a real shell) — a fake emulator doesn't reproduce the destructive
	// resize behavior this test exists to guard against, so verifying
	// against the fake wouldn't prove anything about the actual bug.
	p := &fakePty{}
	origPty := newPty
	newPty = func(width, height int) (Pty, error) { return p, nil }
	t.Cleanup(func() { newPty = origPty })

	m := New(1).SetSize(20, 5)
	m, _ = m.Start()
	t.Cleanup(func() { _ = m.Close() })

	// Write 5 lines directly through Update — no read loop needed, this
	// just exercises the same m.emu.Write path a real OutputMsg would.
	updated, _ := m.Update(OutputMsg{id: 1, generation: m.generation, data: []byte("line0\r\nline1\r\nline2\r\nline3\r\nline4")})
	m = updated

	if !strings.Contains(m.View(), "line4") {
		t.Fatalf("test setup: expected the live view to show line4 before shrinking; got %q", m.View())
	}

	m = m.SetSize(20, 2) // shrink from 5 rows to 2 — the real library keeps line0/line1, destroys line2-line4 here without the fix

	// The pane only shows 2 rows at once now, so line3 and line4 (2 apart
	// in the combined buffer) can't both be on screen simultaneously —
	// check each at the scroll position that actually reveals it, rather
	// than asserting them together.
	//
	// line4 (the row nearest the cursor — the one the bug report is
	// specifically about: a shell prompt, the tail of whatever was just
	// catted) must be reachable by scrolling up just slightly.
	oneUp := m.ScrollLines(-1)
	if view := oneUp.View(); !strings.Contains(view, "line4") {
		t.Fatalf("got View()=%q after scrolling up 1 line post-shrink, want it to contain line4 (the row nearest the cursor — preserved via shrinkOverflow, not silently destroyed by the real emulator's own lossy resize)", view)
	}
	// line2 (the OLDEST of the captured rows) must be reachable by
	// scrolling all the way up, proving the full captured range survived,
	// not just the row nearest the boundary. line0 is NOT a meaningful
	// check here — the real library keeps it as part of the new live
	// screen regardless of whether this fix works at all, so asserting on
	// it alone (an earlier, wrong version of this test did) would pass
	// even if the fix captured nothing real.
	allUp := m.ScrollLines(-10)
	if view := allUp.View(); !strings.Contains(view, "line2") {
		t.Fatalf("got View()=%q after scrolling all the way up post-shrink, want it to contain line2 (the oldest captured row)", view)
	}
}

func TestSetSizeShrinkCaptureIsSkippedDuringAltScreen(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{altScreen: true}
	withFakes(t, p, e)

	m := New(1).SetSize(20, 5)
	m, _ = m.Start()
	e.written = []byte("vim-a\nvim-b\nvim-c\nvim-d\nvim-e")

	m = m.SetSize(20, 2) // shrink while an alt-screen app is showing

	if len(m.shrinkOverflow) != 0 {
		t.Fatalf("got shrinkOverflow=%q after shrinking during the alt screen, want empty — capturing the alt-screen app's own UI here would surface as unrelated content blended into main-screen scrollback later", m.shrinkOverflow)
	}
}

func TestSetSizeShrinkCaptureIsCappedLikeTheRealScrollbackBuffer(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{}
	withFakes(t, p, e)

	m := New(1).SetSize(20, 2)
	m, _ = m.Start()

	// Repeatedly grow to 3 rows (writing a fresh, distinguishable row
	// each time) then shrink back to 2, so every cycle captures exactly
	// one more row — enough cycles to push shrinkOverflow's length past
	// maxShrinkOverflow if nothing caps it.
	const cycles = maxShrinkOverflow + 5
	for i := 0; i < cycles; i++ {
		m = m.SetSize(20, 3)
		e.written = []byte("a\nb\nrow")
		m = m.SetSize(20, 2)
	}

	if len(m.shrinkOverflow) != maxShrinkOverflow {
		t.Fatalf("got len(shrinkOverflow)=%d after %d shrink cycles, want it capped at maxShrinkOverflow=%d (mirroring the real emulator's own scrollback cap)", len(m.shrinkOverflow), cycles, maxShrinkOverflow)
	}
}

func TestSetSizeDoesNotCaptureOnAWidthOnlyShrink(t *testing.T) {
	p := &fakePty{}
	e := &fakeEmulator{}
	withFakes(t, p, e)

	m := New(1).SetSize(20, 5)
	m, _ = m.Start()
	e.written = []byte("line0\nline1\nline2\nline3\nline4")

	m = m.SetSize(10, 5) // width shrinks, height unchanged

	if len(m.shrinkOverflow) != 0 {
		t.Fatalf("got shrinkOverflow=%q after a width-only shrink, want empty — only a HEIGHT shrink discards rows in the real emulator", m.shrinkOverflow)
	}
}
