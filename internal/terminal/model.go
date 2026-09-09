package terminal

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/xpty"
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
//
// width/height are clamped to >= 0 here rather than trusted from the
// caller: an aggressive window resize can drive the app's computed pane
// width negative (see app.Model's WindowSizeMsg handling), and the real
// vt.Emulator's Resize panics on a negative slice bound instead of
// degrading gracefully — this package can't rely on every caller getting
// the arithmetic right upstream, the same way editor/filetree already
// floor their own content width before using it.
func (m Model) SetSize(width, height int) Model {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
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
	cmd := exec.Command(shell)
	setControllingTerminal(cmd)
	if err := p.Start(cmd); err != nil {
		m.err = fmt.Errorf("terminal: %w", err)
		return m, nil
	}
	// Reap the child once it exits so it doesn't linger as a zombie for
	// the remaining life of the app. Fire-and-forget: the model doesn't
	// need to know when this completes.
	go func() { _ = xpty.WaitProcess(context.Background(), cmd) }()

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
