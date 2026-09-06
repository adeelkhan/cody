//go:build !windows

package terminal

import (
	"os/exec"
	"syscall"
)

// setControllingTerminal marks cmd to become a session leader with the
// pty as its controlling terminal when started. Without this, the shell
// has no foreground process group, so tty signals (Ctrl+C → SIGINT,
// Ctrl+Z, SIGWINCH on resize) are never delivered — verified empirically
// during this plan's final review (bash printed "no job control in this
// shell" and did not interrupt a running command without this).
//
// Ctty is set to 0, matching fd 0 (stdin) in the child process: xpty's
// UnixPty.Start wires the pty's slave end to the child's stdin (as well
// as stdout/stderr) before starting it, so fd 0 in the child is the pty
// slave that should become its controlling terminal.
func setControllingTerminal(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0,
	}
}
