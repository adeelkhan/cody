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
