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

// maxShrinkOverflow caps how many rows Model.shrinkOverflow will ever hold
// — mirroring the real vt.Emulator's own 10,000-line scrollback cap (see
// vt.DefaultScrollbackSize), so a long session across many shrink events
// (e.g. several drags of the pane boundary) can't grow this unboundedly.
// Oldest entries are dropped first, same eviction order as the library's
// own scrollback.
const maxShrinkOverflow = 10000

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
	// shrinkOverflow holds rows a height-shrinking SetSize captured
	// before they would otherwise have been silently destroyed by the
	// underlying emulator's own resize (see SetSize's doc comment) —
	// oldest first, rendered as a tier BETWEEN m.emu's own scrollback and
	// the live screen (see renderScrolledView, ScrollLines): the rows it
	// holds were on the live screen at capture time, so they are newer
	// than anything already scrolled into the real scrollback by then,
	// even though the real scrollback usually holds far more lines
	// overall (everything that scrolled off before the shrink ever
	// happened).
	shrinkOverflow []string
	// shrinkContinuing is true immediately after a shrink-capture and
	// reset to false the next time the live grid's content is genuinely
	// perturbed since that capture — either real output being written
	// (Update's OutputMsg case) or the height growing back (SetSize),
	// which pads the grid with new, empty rows and so is just as
	// invalidating as output for this purpose. It exists because a
	// single user resize gesture
	// (dragging a real terminal window's edge) typically arrives as MANY
	// separate, small SetSize calls — one per intermediate size the OS
	// reports — not one big jump. Each of those calls captures a row
	// that is OLDER than the row the previous call in the same gesture
	// captured (the grid keeps shrinking from the same unchanged
	// content), so consecutive captures within one gesture must be
	// PREPENDED to stay oldest-first overall; capturing after output has
	// arrived is a genuinely later, newer batch and must be APPENDED
	// instead. Without this distinction, a smooth multi-step drag comes
	// out with its captured rows in exactly reversed order — this is
	// exactly the difference between the tmux `resize-window` calls this
	// feature was originally verified against (which deliver one resize
	// per call, not a smooth multi-step sequence) and a real terminal
	// emulator's own window-drag behavior.
	shrinkContinuing bool
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
//
// A height SHRINK gets one more step first: the real vt.Emulator's own
// Resize (charmbracelet/x/vt's Screen.Resize, delegating to
// ultraviolet's Buffer.Resize) implements a shrink as `Lines =
// Lines[:height]` — keeping the grid's TOP rows and silently discarding
// everything below, with no scrollback push of its own. Since the
// cursor (and therefore the most recently written content — a shell
// prompt, the tail of whatever was just catted) sits near the BOTTOM of
// the grid, that's exactly backwards for a shrinking terminal. Capture
// the rows about to be destroyed ourselves into shrinkOverflow (see its
// own doc comment for the tier position and the prepend/append
// distinction) before delegating to the library's own resize —
// confirmed against the real library, not just reasoned about
// abstractly (see TestSetSizePreservesRowsLostToARealEmulatorsHeightShrink).
func (m Model) SetSize(width, height int) Model {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	changed := width != m.width || height != m.height
	if changed && m.emu != nil && m.height > 0 && height < m.height && !m.emu.IsAltScreen() {
		// The library keeps Lines[:height] (the TOP height rows of the
		// OLD grid) and discards the rest — so what we must capture is
		// everything AFTER that same cutoff, liveLines[height:], not
		// liveLines[:discarded]. Capturing the top instead (an earlier,
		// wrong version of this fix) grabbed rows the library was never
		// going to lose while the real victims — the rows nearest the
		// cursor, typically including the shell prompt — stayed lost;
		// caught by an independent review that verified the actual
		// discard direction against the real library directly.
		//
		// Skipped entirely while the alt screen (vim, less, ...) is
		// active: Render() would be showing that app's own UI, not shell
		// history, and capturing it here would later surface as
		// unrelated content blended into what's supposed to be
		// main-screen scrollback.
		liveLines := strings.Split(m.emu.Render(), "\n")
		from := max(0, min(height, len(liveLines)))
		captured := liveLines[from:]
		// The grid is always full-height, so everything below the
		// cursor is blank padding, not real content — storing it would
		// render as empty rows between the real content and the live
		// screen, and a repeated shrink/grow bounce would spend the
		// maxShrinkOverflow budget on nothing but blank lines.
		for len(captured) > 0 && strings.TrimSpace(captured[len(captured)-1]) == "" {
			captured = captured[:len(captured)-1]
		}
		if m.shrinkContinuing {
			// Still the same resize gesture as the previous capture (no
			// output has arrived since) — this batch is OLDER than that
			// one (see shrinkContinuing's own doc comment), so it goes
			// in front, not at the back.
			m.shrinkOverflow = append(captured, m.shrinkOverflow...)
		} else {
			m.shrinkOverflow = append(m.shrinkOverflow, captured...)
		}
		m.shrinkContinuing = true
		if excess := len(m.shrinkOverflow) - maxShrinkOverflow; excess > 0 {
			// The oldest entries are always at index 0 regardless of
			// which branch above just ran, so trimming the front here is
			// always correct.
			m.shrinkOverflow = m.shrinkOverflow[excess:]
		}
	} else if changed && height > m.height {
		// A height GROWTH also breaks the "same shrink gesture" chain,
		// same as real output does (see shrinkContinuing's own doc
		// comment) — the library pads the grid with new, empty rows to
		// reach the larger height, so the live grid is no longer the
		// same snapshot a later shrink-capture would need to treat as a
		// continuation. Without this, a shrink/grow/shrink bounce within
		// one drag (a realistic pattern — real window-drag resize events
		// aren't always monotonic) could prepend a captured row of blank
		// padding as if it were OLDER than genuinely older content
		// already sitting in shrinkOverflow. A width-only change does
		// NOT reset this — it doesn't touch the grid vertically, so it
		// can't invalidate a shrink chain the same way.
		m.shrinkContinuing = false
	}
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
// length against), and while the alt screen is active (see View's own
// alt-screen note) — there's nothing valid to scroll into.
func (m Model) ScrollLines(n int) Model {
	if m.emu == nil {
		return m
	}
	if m.emu.IsAltScreen() {
		m.scrollOffset = 0
		return m
	}
	m.scrollOffset -= n
	if maxOffset := m.emu.ScrollbackLen() + len(m.shrinkOverflow); m.scrollOffset > maxOffset {
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
		wasAltScreen := m.emu.IsAltScreen()
		var beforeLen int
		if m.scrollOffset > 0 {
			beforeLen = m.emu.ScrollbackLen()
		}
		m.emu.Write(msg.data)
		// Real output arrived — any later shrink-capture is a genuinely
		// newer batch than whatever's already in shrinkOverflow, not a
		// continuation of the same resize gesture (see shrinkContinuing's
		// own doc comment).
		m.shrinkContinuing = false
		switch {
		case wasAltScreen && !m.emu.IsAltScreen():
			// The alt screen (vim, less, ...) just exited as part of this
			// very Write. Any scrollOffset left over refers to
			// main-screen content from before it started — meaningless
			// now that we're back to the live prompt it just returned
			// to, and View's own alt-screen guard no longer masks it
			// (IsAltScreen() is false again) — reset before it can blend
			// stale scrollback into that prompt.
			m.scrollOffset = 0
		case m.scrollOffset > 0:
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
	// The alt screen (vim, htop, less, ...) doesn't share the main
	// screen's scrollback (vt.Emulator.Scrollback() always reports the
	// main screen's, regardless of which is active) — rendering scrolled
	// while it's up would splice unrelated, stale pre-app content in
	// among the app's own live rows. Fall back to the plain live render
	// unconditionally in that case, even if scrollOffset is still
	// sitting >0 from before the app started (ScrollLines resets it back
	// to 0 on the next scroll attempt, but View() can't wait for that —
	// it must render correctly on the very next frame regardless).
	if m.scrollOffset == 0 || m.emu.IsAltScreen() {
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
// by hand from three chronologically-ordered tiers, oldest first: the
// emulator's own scrollback (ScrollbackLen/ScrollbackLine — normally far
// more content than a single shrink ever captures, everything that
// scrolled off before the shrink happened), then m.shrinkOverflow (rows a
// height shrink would otherwise have destroyed — see SetSize — which
// were still on the live screen at capture time, making them newer than
// whatever was already in scrollback then), then the live screen's own
// rows. The live screen's individual rows come from splitting Render()'s
// own output on "\n" — safe because ANSI SGR/CSI escape sequences never
// contain a raw newline byte.
func (m Model) renderScrolledView() string {
	liveLines := strings.Split(m.emu.Render(), "\n")
	overflowLen := len(m.shrinkOverflow)
	sbLen := m.emu.ScrollbackLen()
	height := m.height
	if height <= 0 || height > len(liveLines) {
		height = len(liveLines)
	}
	total := sbLen + overflowLen + height
	start := max(0, total-height-m.scrollOffset)
	bar := scrollbar.Column(total, height, start)
	overlayWidth := m.width - 2 // " " + one scrollbar rune, matching editor/filetree's own scrollbarGutterWidth
	lines := make([]string, height)
	for row := range height {
		i := start + row
		var line string
		switch {
		case i < sbLen:
			// Real scrollback comes first (oldest): it holds everything
			// that scrolled off before this shrink ever happened, which
			// is normally far more content than a single shrink captures.
			line = m.emu.ScrollbackLine(i)
		case i-sbLen < overflowLen:
			// shrinkOverflow sits between scrollback and the live screen
			// — its rows were still on the live screen at capture time,
			// so they're newer than anything already in scrollback then
			// (see shrinkOverflow's own doc comment for the one edge
			// case this doesn't perfectly handle: real output arriving
			// AFTER a shrink and being pushed into scrollback afterward).
			line = m.shrinkOverflow[i-sbLen]
		case i-sbLen-overflowLen < len(liveLines):
			line = liveLines[i-sbLen-overflowLen]
		}
		var barRune rune
		if row < len(bar) {
			barRune = bar[row]
		}
		// A degenerately narrow pane (the same aggressively-downsized-
		// window class this codebase already floors elsewhere) must
		// never render a row wider than m.width itself claims: no room
		// for content or a gutter at 0, room for only the scrollbar cell
		// itself at 1 (no leading space), and the normal content+gutter
		// shape from 2 up.
		switch {
		case m.width <= 0:
			lines[row] = ""
		case m.width == 1:
			lines[row] = scrollbarStyle.Render(string(barRune))
		case overlayWidth <= 0:
			// m.width == 2: overlayWidth (m.width-2) is 0, and Lip Gloss's
			// MaxWidth skips truncation entirely at 0 rather than
			// collapsing the line to empty — falling through to the
			// default branch below would let a non-empty line pass
			// through untruncated and push the row past m.width.
			lines[row] = " " + scrollbarStyle.Render(string(barRune))
		default:
			// MaxWidth truncates a line longer than overlayWidth, but
			// (unlike editor/filetree's own padRow, which this mirrors)
			// never pads a shorter one — without padding, the scrollbar
			// appended right after would land at a different column on
			// every row, drifting with each row's own content length
			// instead of staying fixed at the pane's right edge.
			line = lipgloss.NewStyle().MaxWidth(overlayWidth).Render(line)
			if pad := overlayWidth - lipgloss.Width(line); pad > 0 {
				line += strings.Repeat(" ", pad)
			}
			lines[row] = line + " " + scrollbarStyle.Render(string(barRune))
		}
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
