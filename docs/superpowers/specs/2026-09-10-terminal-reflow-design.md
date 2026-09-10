# Terminal Width Reflow — Design

## 1. Overview

The terminal pane's underlying emulator (`charmbracelet/x/vt` over
`charmbracelet/ultraviolet`) implements a width-shrinking resize as
`Lines[i] = Lines[i][:width]` for every row — permanently truncating
whatever was past the new, narrower column count. The library has no
per-row soft-wrap tracking, so it has no way to reflow: it cannot tell
where one logical line ends and its wrapped continuation begins, so a
shrink just slices, and a later grow only pads blank cells back in —
the truncated characters never return.

This was reported via a screen recording: `cat`-ing `README.md`, then
dragging the terminal pane's window narrower and back wider, permanently
lost the tail of every printed line. Two prior fixes shipped in PR #8
(merged) — a live-grid snapshot taken before a shrink, replayed on a
later regrowth — but real-world testing against an actual interactive
zsh session found the snapshot approach fails for two independent,
structural reasons: (1) many shells redraw their own prompt in response
to the very resize that shrank the pane, and that redraw's output has no
way to be told apart from anything else arriving through the pty, and
(2) if that redraw happens while the pane is still narrow, it can
itself scroll the screen, shifting every row's index between the
snapshot and the eventual restore — silently comparing the wrong rows.
Both failure modes stem from the same root cause: the snapshot approach
compares the *live grid* against a *stale copy of an earlier moment*,
and a live shell's own reaction to a resize can invalidate that
comparison in ways no gating heuristic fully closes.

This spec replaces that approach with real reflow: on every
width-changing resize, re-derive the terminal's logical lines from
whatever the live grid *currently* holds, and re-wrap them at the new
width. There is no stale state to invalidate — each resize event
recomputes fresh from the grid as it exists at that moment, which is the
property that makes this robust against the exact failure modes above.

**Out of scope for this spec** (see §8 for the full list): reflowing
real scrollback history, pulling `shrinkOverflow` content back into view
when a grow frees up space, perfect column-accounting for every
mixed-width edge case, and unifying this with the existing
height-shrink capture path (`shrinkOverflow`/`shrinkContinuing`) beyond
the one interaction point this spec defines.

## 2. Constraints

Global Constraints (apply to every task below):

- Go + Bubble Tea/Lip Gloss, matching the existing codebase's
  architecture and style exactly (value-receiver `Model`s).
- The `terminal.Emulator` interface stays narrow and plain-typed — no
  vt/uv-specific types (`uv.Cell`, `uv.Line`) cross it. This spec adds
  **no new methods** to `Emulator`: reflow operates entirely on the
  plain ANSI-styled strings `Render()`/`ScrollbackLine()` already
  return, and writes back through the existing `Write([]byte)` method
  via ANSI escape sequences — the same technique PR #8's width-shrink
  fix already uses for `restoreWidthShrink`.
- `CursorPosition() (x, y int)` (already on `Emulator` since PR #8) is
  reused as-is.
- TDD: write the failing test, watch it fail, implement, watch it pass.
- Every fix must be verified against the **real** `vt.Emulator`, not
  just the fake — the fake cannot reproduce the library's destructive
  resize behavior, so a fake-only test proves nothing about the actual
  bug (established precedent throughout `internal/terminal/model_test.go`).
- `gofmt`-clean; `go vet ./...` and the full `go test ./...` suite green
  before every commit.
- Skipped entirely while `IsAltScreen()` is true, matching every other
  resize-time capture in this package (`shrinkOverflow`'s capture,
  `restoreWidthShrink`) — a full-screen app manages its own redraw on
  resize, and reflowing over it would corrupt whatever it's drawing.
- Skipped entirely when height changes in the same `SetSize` call as
  width (see §5) — falls back to today's existing behavior for that
  step; a later width-only step still reflows normally.

## 3. Wrap Detection & Rewrap Algorithm

### 3.1 Reading the grid

`Render()` already returns the full live grid as one `"\n"`-joined,
ANSI-styled string (used throughout this package — see
`shrinkOverflow`'s own capture in `SetSize`). Splitting on `"\n"` gives
one string per physical row. This spec operates entirely on these
per-row strings — no cell-level (`uv.Cell`) access is introduced.

### 3.2 Wrap detection

A physical row is a **continuation** of the row above it if and only if
the row above is "filled to the edge": its rendered content occupies
the full display width, i.e. `ansi.StringWidth(rowAbove) == oldWidth`
(the same function `lipgloss.Width` itself delegates to — see §3.3).
This mirrors the wrap heuristic every reflow-capable terminal without
explicit wrap tracking relies on (verified against the real library in
prior investigation via the equivalent cell-level check,
`lastNonBlankCol == width-1`) — measuring display width via
`ansi.StringWidth` rather than raw cell indexing keeps this
implementation string-based and avoids adding any new `Emulator`
surface, per §2.

Scanning top to bottom, group physical rows into **logical lines**: start
a new logical line whenever the previous row was not filled to the edge
(or this is the first row). A logical line's content is the
concatenation of its physical rows' strings, in order.

### 3.3 Rewrap

`github.com/charmbracelet/x/ansi` (already an indirect dependency via
lipgloss, which `lipgloss.Width` itself is built on) provides
`Truncate`/`TruncateLeft`/`StringWidth` — ANSI-aware, wide-character-
aware string operations, confirmed by reading the implementation to
never split a grapheme cluster across a cut boundary: `Truncate(s,
length, "")` drops a cluster entirely, rather than splitting it, if
including it would push the accumulated width past `length`. This is
exactly the non-split rule this spec needs, already implemented and
already a dependency — no hand-rolled grapheme-walking loop is needed.

To rewrap one logical line's string at the new width: repeatedly take
`row := ansi.Truncate(remaining, newWidth, "")` as the next physical
row, then advance past exactly what was consumed —
`remaining = ansi.TruncateLeft(remaining, ansi.StringWidth(row), "")`
— until `remaining` is empty. Using `ansi.StringWidth(row)` (not
`newWidth`) to advance is what makes the wide-character rule automatic:
in the one case where `Truncate` dropped a trailing wide cluster rather
than split it, `row`'s width is `newWidth - 1`, so advancing by that
amount naturally leaves the un-split cluster as the first thing in the
next row, rather than skipping or duplicating it.

This produces the logical line's new sequence of physical rows at the
new width. Concatenating every logical line's new rows, in order,
produces the full new grid content.

### 3.4 Statelessness

Nothing above reads any field that persists across resize calls (no
snapshot, no "continuing" flag). Each call re-derives entirely from
`Render()`'s current output. This is deliberate: it is the property
that survives a shell redrawing or scrolling mid-resize, since there is
no earlier-moment state for that redraw to invalidate — the *next*
resize event just reflows whatever is on the grid *then*, correctly.

## 4. Cursor Mapping

Before computing the reflow (i.e. before the library's own destructive
`Resize` call), capture the cursor's current `(x, y)` via
`CursorPosition()`. Locate which physical row `y` falls in, then which
logical line that row belongs to (per §3.2's grouping) and the cursor's
offset — in display columns — within that logical line: the sum of
`ansi.StringWidth` of every physical row before it in that logical
line, plus `x` itself (the cursor's column within its own row).

After rewrapping that one logical line (§3.3), find the new `(x, y)` by
applying the same advance used to build the rewrapped rows
(`ansi.TruncateLeft(logicalLine, offset, "")` gives the point the
cursor's offset falls at; which of the new physical rows contains that
point, and its remaining column within that row, are the new `y` and
`x`). If the logical line's cursor-bearing row ends up pushed into
overflow (§5 — only possible with a very tall single logical line
combined with a small height), clamp `y` to the last visible row
defensively; this is an edge case, not a targeted scenario, so
"visually reasonable" is the bar, not "exactly correct."

## 5. Height Interaction

Rewrapping at a new width can require more physical rows (narrower) or
fewer (wider) than the current height. Two cases:

- **Width changes, height does not** (the common case — most resize
  drags move one boundary at a time): this is the case this spec
  handles. If the rewrapped content needs more rows than fit in the
  unchanged height, the excess from the **top** (oldest) is appended
  into the existing `m.shrinkOverflow` field — reusing the render tier
  PR #8 already built for the height-shrink bug, rather than adding a
  second parallel mechanism. This is always an append (never a
  prepend): reflow has no "continuing the same gesture" concept (§3.4),
  so every call's overflow is, by construction, a fresh, freshly-newer
  batch relative to whatever `shrinkOverflow` already holds — the same
  semantics `SetSize`'s own height-shrink capture already uses for its
  non-continuing case. If the rewrapped content needs *fewer* rows
  (widening), the freed rows at the top of the grid are simply left
  blank — no attempt is made to pull rows back out of `shrinkOverflow`
  to fill them (see §8, out of scope for this pass).
- **Width and height change in the same `SetSize` call**: reflow is
  skipped entirely for that call. `SetSize` falls back to its existing
  behavior for that step (today's width-truncation for the width
  component, and the existing `shrinkOverflow`/`shrinkContinuing`
  machinery for the height component, both untouched by this spec). A
  real drag that moves both boundaries together will still fail to
  reflow on those specific steps; a later width-only step (or the drag
  settling on a final width-only adjustment) reflows normally. This
  keeps this spec from having to touch or re-verify PR #8's
  already-shipped, already-hard-won height-shrink logic — a deliberate
  scope boundary, not an oversight (see the design discussion's
  approach B2).

`shrinkContinuing` itself is untouched by this spec: reflow's own
overflow writes never read or set it, and the existing height-shrink
path's own reads/writes of it are unaffected, since reflow never runs
in the same call as a height change (this case is explicitly skipped,
above).

`shrinkOverflow`'s own doc comment (`Model` struct, `model.go`)
currently describes it as holding rows "a height-shrinking SetSize
captured" — broadened by this spec to also hold width-reflow overflow,
so that comment needs a one-line update alongside the implementation to
stay accurate, not just the code.

## 6. Write-Back

Once the new grid content (§3.3, possibly truncated per §5) is computed,
and cursor position mapped (§4):

1. Call the library's own `emu.Resize(newWidth, newHeight)` first — this
   still needs to happen to actually change the grid's dimensions; this
   spec corrects its content afterward rather than replacing the call.
2. For each new row that fits in `newHeight`, write it via
   `\x1b[<row>;1H<content>` through `emu.Write`, mirroring
   `restoreWidthShrink`'s existing technique — each row's rendered
   string is already self-contained (resets any style/hyperlink it
   opened before ending — see `ultraviolet`'s `renderLine`), so writing
   them back to back on separate rows cannot bleed style state between
   them.
3. Reposition the cursor via `\x1b[<y+1>;<x+1>H` to the mapped position
   from §4.

This mirrors PR #8's already-proven write-back mechanism exactly,
deliberately reusing it rather than introducing a cell-level `SetCell`
write path — keeping `Emulator` narrow per §2, at the minor, already-
accepted cost of round-tripping through `Render()`'s ANSI-string form
rather than raw cells (the same tradeoff `shrinkOverflow` already makes).

## 7. Testing Strategy

- **Algorithm unit tests, pure** (no emulator, no pty): table-driven —
  given a set of logical lines' plain strings at width W, rewrap at
  width W2, assert the exact output rows. Fast, exhaustive coverage of
  the wrap/rejoin logic in isolation.
- **Real-emulator round-trip tests**, mirroring this package's existing
  pattern (`TestSetSizeRestoresContentLostToARealEmulatorsWidthShrink`
  and neighbors): write content via the real `vt.Emulator`, shrink,
  assert no truncation; grow back past the original width, assert full
  rejoin into fewer rows.
- **Wide-character non-split test**: a double-width cluster positioned
  exactly at a would-be wrap boundary must start the next row, never
  split.
- **False-join regression test**: a test that deliberately captures the
  heuristic's known weak point — e.g. a box-drawing-style row that
  happens to fill the last column without actually being wrapped —
  incorrectly joins with the row below. This is **not** fixed by this
  spec; the test exists to pin the current (imperfect, but honestly
  documented) behavior so a future change can't silently make it worse
  without the test flagging it.
- **Cursor-correctness tests**: a distinctive marker character at the
  cursor's position before a shrink/grow round trip must be at the
  semantically equivalent position after.
- **Height-interaction tests**: confirm a simultaneous width+height
  `SetSize` call correctly skips reflow (§5) without regressing any
  existing `TestSetSize*` height-shrink test; confirm width-only
  overflow correctly appends (never prepends) into `shrinkOverflow`.
- **Alt-screen skip test**: reflow does not run while `IsAltScreen()` is
  true.
- **Real end-to-end tmux verification against an interactive zsh
  session** — the exact reproduction (`cat README.md`, shrink the real
  Ghostty window, grow it back) that defeated both snapshot-based
  attempts. This is the actual bar this feature has to clear before
  being considered done, not just its unit tests passing.
- **Independent code review** before push, given this is the most
  complex algorithm in the package.

## 8. Out of Scope

Stated explicitly so these are recognized as deliberate decisions, not
gaps discovered later:

- **Reflowing real scrollback** (`vt.Scrollback`, exposed via
  `ScrollbackLen`/`ScrollbackLine`). Scrollback rows are already only
  *cosmetically* clipped at render time (`lipgloss.MaxWidth` in
  `renderScrolledView`) — the underlying data was never destroyed by a
  width change, so they already "restore" on widening with no fix
  needed. Only the live grid is genuinely, permanently destroyed by the
  library's own resize, so that's the only part this spec touches.
- **Pulling `shrinkOverflow` content back into the live view when a
  grow frees up rows.** A real terminal would pull scrollback content
  back down to fill freed space; this spec leaves freed rows blank
  instead (their content remains reachable by scrolling up, same as
  today). Revisit if this proves visually confusing in practice.
- **Re-reflowing `shrinkOverflow`'s own content on a later resize.**
  §5's overflow rows are written into `shrinkOverflow` once, at
  whatever width they were pushed there, and never touched again — a
  second, separate width-changing `SetSize` call only reflows
  `m.emu.Render()` (the current live grid), not anything already
  sitting in `shrinkOverflow`. Across a sequence of several
  width-changing resizes that each produce overflow, this means
  `shrinkOverflow` can hold rows wrapped at different historical
  widths side by side, with no seam marking where one width's rows end
  and another's begin. This is the direct extension of the first two
  bullets' reasoning to a case they didn't originally call out by name:
  `shrinkOverflow` is, from the moment content lands in it, exactly as
  historical and frozen as real scrollback already is — reflowing it
  in place on every subsequent resize would mean re-wrapping
  potentially thousands of historical rows on every resize event, the
  same cost this spec already declined to pay for real scrollback.
  Content is never lost or duplicated by this — only its *wrap width*
  is inconsistent across the historical/live boundary after multiple
  resizes. Not fixed here; pinned as accepted behavior.
- **Fixing either direction of the wrap-detection heuristic's
  misjudgment** (§3.2). The heuristic — "the row above is a wrapped
  continuation iff its *rendered* width fills the pane" — can be wrong
  both ways, and both are accepted, pinned-by-test limitations rather
  than open bugs:
  1. **False join.** A row that fills the full width without actually
     having wrapped (a box-drawing border exactly as wide as the pane)
     is joined with the row below it and rewrapped as one logical line.
     Pinned by `TestGroupIntoLogicalLinesFalseJoinOnAFullWidthRow`.
  2. **Missed join.** A row that genuinely *did* soft-wrap but whose
     last cell is blank measures *narrower* than the full width, because
     the library's rendering strips trailing blank cells — so it reads
     as "not a continuation" and is never rejoined on a later grow.
     Common in prose, where a wrap point often lands right after a
     space: at width 10, `abcde fgh ijk` renders as `abcde fgh` / `ijk`
     (the boundary space already gone from the rendered row), and
     growing back to width 40 leaves the two rows unjoined. Pinned by
     `TestGroupIntoLogicalLinesMissesAWrapBoundaryEndingInBlank`.

  Both stem from the same root cause: a rendered row string cannot
  distinguish "blank cell inside a filled row" from "row that ends
  here". The principled fix needs cell-level grid access (a
  `lastNonBlankCol`-style check, or a real per-row wrap flag), which §2
  rules out — no new `Emulator` methods, no cell-level API surface — and
  which the upstream-fix bullet below is the real home for. A
  heuristic workaround (e.g. treating a row one column short as a
  continuation) would only trade one misjudgment for a more frequent
  one, so it is deliberately not attempted.
- **Perfect column accounting for every mixed-width edge case**
  (combining characters, zero-width joiners, ambiguous-width
  characters under different locale conventions). The one
  correctness-critical rule — never split a wide cluster across rows —
  is guaranteed and tested; beyond that, "matches `lipgloss.Width`'s own
  measurement" is the bar, not a from-scratch Unicode-width
  implementation.
- **Unifying this with `shrinkContinuing`/height-shrink capture beyond
  the one interaction point in §5.** A follow-up could delete
  `shrinkContinuing`'s prepend/append machinery entirely by unifying
  height and width handling into one stateless recompute (since reflow
  proves that pattern works) — deliberately not attempted here, to keep
  this spec from re-touching and re-verifying already-shipped PR #8
  logic (approach B2 from the design discussion; B1 remains available
  as future work).
- **Filing the upstream fix** (a real per-row wrap flag in
  `ultraviolet`/`x/vt` itself, the architecturally "correct" location
  for this). Worth doing separately; not blocking this spec, and not
  in cody's control regardless.
