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

	m := New(1)
	m = m.SetSize(40, 10)
	m, cmd := m.Start()
	t.Cleanup(func() { _ = m.Close() })
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
