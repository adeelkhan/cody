package terminal

import "github.com/charmbracelet/x/vt"

// Emulator is the narrow slice of vt.Emulator's interface this package
// actually uses. Defined locally so tests can substitute a fake.
//
// ScrollbackLen/ScrollbackLine expose vt.Emulator's own scrollback buffer
// (it already maintains one internally — see vt.Scrollback — this package
// just never read from it before) as plain ints/strings rather than the
// vt/uv package's own Scrollback/Line types, so this interface — and every
// fake substituting for it — stays free of that dependency, matching how
// Render() already returns a plain string rather than a vt-specific type.
type Emulator interface {
	Write(p []byte) (int, error)
	Render() string
	Resize(width, height int)
	// ScrollbackLen returns the number of lines currently held in the
	// scrollback buffer (lines that have scrolled off the top of the live
	// screen), oldest-first.
	ScrollbackLen() int
	// ScrollbackLine returns the rendered (ANSI-styled) text of the
	// scrollback line at index (0 = oldest), or "" if index is out of
	// range.
	ScrollbackLine(index int) string
}

// vtEmulator adapts *vt.Emulator to this package's narrower Emulator
// interface — vt.Emulator itself has no ScrollbackLine(index) string
// method (its Scrollback() returns a *vt.Scrollback of vt/uv-typed Lines,
// not a plain string), so this wraps it rather than returning *vt.Emulator
// directly from newEmulator.
type vtEmulator struct {
	*vt.Emulator
}

func (e vtEmulator) ScrollbackLen() int {
	return e.Emulator.ScrollbackLen()
}

func (e vtEmulator) ScrollbackLine(index int) string {
	line := e.Emulator.Scrollback().Line(index)
	if line == nil {
		return ""
	}
	return line.Render()
}

// newEmulator is a package-level var so tests can substitute a fake
// constructor.
var newEmulator = func(width, height int) Emulator {
	return vtEmulator{vt.NewEmulator(width, height)}
}
