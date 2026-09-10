package terminal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/xpty"

	"cody/internal/scrollbar"
)

// OutputMsg carries a chunk of bytes read from the pty. Exported so
// internal/app can pass it through to Update unconditionally, the same
// way it already does for editor.RehighlightMsg.
type OutputMsg struct {
	id         int
	data       []byte
	generation int
}

// ID returns the terminal.Model this message belongs to (see New's doc
// comment) — a caller holding several sessions must check this before
// routing the message into any one of them.
func (m OutputMsg) ID() int {
	return m.id
}

// ReadErrMsg signals the pty's read loop stopped (the shell exited, or
// the pty was closed). Exported for the same reason as OutputMsg.
type ReadErrMsg struct {
	id         int
	generation int
}

// ID returns the terminal.Model this message belongs to — see
// OutputMsg.ID's doc comment.
func (m ReadErrMsg) ID() int {
	return m.id
}

type Model struct {
	id            int // caller-assigned; stable for this Model's lifetime, echoed in OutputMsg/ReadErrMsg for multi-instance routing (see New's doc comment)
	width, height int
	pty           Pty
	emu           Emulator
	started       bool
	err           error
	generation    int
	// scrollOffset is how many lines back from the live tail the viewport
	// currently shows: 0 means "following" (View() renders exactly
	// Render()'s live screen, unchanged from before this field existed);
	// >0 means paused, viewing scrollback (see ScrollLines, View,
	// renderScrolledView). Unlike editor/filetree's own scrollOffset,
	// there is no cursor here for it to track — scrolling the terminal
	// never moves anything the shell itself is doing, only the viewport.
	scrollOffset int
}

// New creates a terminal session identified by id — a value the caller
// controls and can use to tell this session's OutputMsg/ReadErrMsg apart
// from any other terminal.Model instance's. A per-instance generation
// counter alone can't do this: it starts at 0 for every instance, so two
// independently-created sessions' messages can carry the same generation
// and be indistinguishable. id is otherwise opaque to this package — never
// interpreted, only stored and echoed back.
func New(id int) Model {
	return Model{id: id}
}

// ID returns the identity this session was constructed with.
func (m Model) ID() int {
	return m.id
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

// ScrollLines shifts the terminal's viewport by n lines: negative scrolls
// up into scrollback, positive scrolls down toward the live tail. Clamped
// to [0, current scrollback length] — 0 always means "following the live
// output" (see View), and the top end tracks whatever scrollback the
// underlying emulator currently holds, so this never over- or
// under-scrolls even as the buffer grows or gets capped. A no-op before
// the session has started (m.emu == nil, nothing to measure a scrollback
// length against).
func (m Model) ScrollLines(n int) Model {
	if m.emu == nil {
		return m
	}
	m.scrollOffset -= n
	if maxOffset := m.emu.ScrollbackLen(); m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	return m
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
	return m, readCmd(m.pty, m.id, m.generation)
}

func readCmd(p Pty, id, generation int) tea.Cmd {
	return func() tea.Msg {
		buf := make([]byte, 4096)
		n, err := p.Read(buf)
		if err != nil {
			return ReadErrMsg{id: id, generation: generation}
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		return OutputMsg{id: id, data: data, generation: generation}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case OutputMsg:
		if msg.generation != m.generation || m.emu == nil {
			return m, nil
		}
		// If paused (viewing scrollback), keep the viewed window pinned to
		// the same absolute content as new output pushes the live bottom
		// down — matching real terminals, which don't yank a paused view
		// back toward the bottom just because more output arrived in the
		// background. beforeLen/afterLen bracket exactly the growth this
		// one Write caused; scrollOffset (lines back from the bottom)
		// grows by the same amount to compensate. A no-op while following
		// (scrollOffset == 0 stays 0 — nothing to pin).
		var beforeLen int
		if m.scrollOffset > 0 {
			beforeLen = m.emu.ScrollbackLen()
		}
		m.emu.Write(msg.data)
		if m.scrollOffset > 0 {
			m.scrollOffset += m.emu.ScrollbackLen() - beforeLen
		}
		return m, readCmd(m.pty, m.id, m.generation)
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
	// Any keystroke resumes following the live output — matches real
	// terminals: there's no point staying paused on scrollback once
	// you've started typing at the (live) shell again.
	m.scrollOffset = 0
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
	if m.scrollOffset == 0 {
		return m.emu.Render()
	}
	return m.renderScrolledView()
}

// scrollbarStyle matches internal/editor and internal/filetree's own
// locally-defined style exactly (same color, same "defined per-package"
// convention) — kept local rather than shared so this package doesn't pick
// up a dependency on either of theirs for a two-line style value.
var scrollbarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

// renderScrolledView renders the viewport m.scrollOffset lines back from
// the live tail, with a scrollbar overlaid on the last column of each row
// (internal/scrollbar's own Column, the same one editor/filetree already
// use). Only ever called when scrollOffset > 0 — the far more common
// scrollOffset == 0 case renders via Render() directly, completely
// unaffected by any of this (see View), so normal terminal use never pays
// for or sees this overlay.
//
// The vt library's Render() only ever renders the live screen — there is
// no "render at an offset" parameter — so a paused/scrolled view is built
// by hand: scrollback lines (oldest first, via ScrollbackLen/
// ScrollbackLine) followed by the live screen's own rows, treated as one
// combined, chronologically-ordered list. The live screen's individual
// rows come from splitting Render()'s own output on "\n" — safe because
// ANSI SGR/CSI escape sequences never contain a raw newline byte.
func (m Model) renderScrolledView() string {
	liveLines := strings.Split(m.emu.Render(), "\n")
	sbLen := m.emu.ScrollbackLen()
	height := m.height
	if height <= 0 || height > len(liveLines) {
		height = len(liveLines)
	}
	total := sbLen + height
	start := total - height - m.scrollOffset
	if start < 0 {
		start = 0
	}
	bar := scrollbar.Column(total, height, start)
	overlayWidth := m.width - 2 // " " + one scrollbar rune, matching editor/filetree's own scrollbarGutterWidth
	lines := make([]string, height)
	for row := range height {
		i := start + row
		var line string
		switch {
		case i < sbLen:
			line = m.emu.ScrollbackLine(i)
		case i-sbLen < len(liveLines):
			line = liveLines[i-sbLen]
		}
		if overlayWidth > 0 {
			line = lipgloss.NewStyle().MaxWidth(overlayWidth).Render(line)
		}
		var barRune rune
		if row < len(bar) {
			barRune = bar[row]
		}
		lines[row] = line + " " + scrollbarStyle.Render(string(barRune))
	}
	return strings.Join(lines, "\n")
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
