//go:build !windows

package terminal

import (
	"testing"

	"github.com/charmbracelet/x/xpty"
	"golang.org/x/sys/unix"
)

// TestSpawnedShellHasAControllingTerminal is a regression test for I1: the
// spawned shell must be a session leader with the pty as its controlling
// terminal, or else it has no foreground process group and tty signals
// (Ctrl+C -> SIGINT, Ctrl+Z, SIGWINCH on resize) are never delivered.
//
// This spawns a real shell through the real Model.Start() path (no fakes)
// and asserts, via the same TIOCGPGRP ioctl the reviewer used empirically,
// that the pty has a non-zero foreground process group once the shell is
// up. TIOCGPGRP is Unix-specific, hence the build tag.
func TestSpawnedShellHasAControllingTerminal(t *testing.T) {
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

	// Wait for the shell's startup output before checking, so the check
	// runs against a fully-up shell rather than racing its own startup —
	// same technique the package's other real-shell test uses.
	msg := cmd()
	out, ok := msg.(OutputMsg)
	if !ok {
		t.Fatalf("got %T as the first message, want OutputMsg (the shell prompt)", msg)
	}
	m, _ = m.Update(out)

	up, ok := m.pty.(*xpty.UnixPty)
	if !ok {
		t.Fatalf("expected the concrete pty type to be *xpty.UnixPty, got %T", m.pty)
	}

	pgrp, err := unix.IoctlGetInt(int(up.Fd()), unix.TIOCGPGRP)
	if err != nil {
		t.Fatalf("TIOCGPGRP ioctl failed: %v", err)
	}
	if pgrp == 0 {
		t.Fatal("pty has no foreground process group (pgrp == 0) — the spawned shell is not a session leader with a controlling terminal, so Ctrl+C/Ctrl+Z/SIGWINCH cannot reach it")
	}
}
