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
