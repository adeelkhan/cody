// internal/terminal/reflow.go
package terminal

import "github.com/charmbracelet/x/ansi"

// isRowFilledToEdge reports whether row's rendered content occupies the
// terminal's full display width — the wrap-detection heuristic this
// package relies on (see groupIntoLogicalLines): a row that is NOT
// filled to the edge cannot have been soft-wrapped into the row below
// it, since the terminal only wraps when a row is completely full.
// width must be > 0 — callers (SetSize) already guard this.
func isRowFilledToEdge(row string, width int) bool {
	return ansi.StringWidth(row) >= width
}

// groupIntoLogicalLines groups physical rows (at the given width) into
// logical lines: row i+1 continues logical line L iff row i is filled
// to the edge (see isRowFilledToEdge's own doc comment for the known
// false-join limitation this implies). Each returned line is the
// concatenation of its physical rows' content, in order.
// physicalRowCounts[j] holds how many physical rows contributed to
// lines[j], in the same order — cursorOffset (see reflow.go) uses this
// to map a physical row index back to a logical line index.
func groupIntoLogicalLines(rows []string, width int) (lines []string, physicalRowCounts []int) {
	for i, row := range rows {
		if i == 0 || !isRowFilledToEdge(rows[i-1], width) {
			lines = append(lines, row)
			physicalRowCounts = append(physicalRowCounts, 1)
			continue
		}
		lines[len(lines)-1] += row
		physicalRowCounts[len(physicalRowCounts)-1]++
	}
	return lines, physicalRowCounts
}

// rewrapLogicalLine re-wraps a logical line's content at newWidth,
// returning its new physical rows. Built on ansi.Truncate/TruncateLeft
// (already an indirect dependency via lipgloss, which lipgloss.Width
// itself is built on) rather than a hand-rolled grapheme-walking loop:
// Truncate is already ANSI-aware and wide-character-safe, confirmed by
// reading its implementation to drop a grapheme cluster entirely — never
// split it — if including it would push the accumulated width past the
// limit. Advancing by ansi.StringWidth(row) (not newWidth) after each
// row is what makes the non-split rule automatic: in the one case where
// Truncate dropped a trailing wide cluster, row's width is
// newWidth-1, so the next TruncateLeft naturally leaves that cluster as
// the first thing in the next row instead of skipping or duplicating it.
func rewrapLogicalLine(line string, newWidth int) []string {
	if line == "" {
		return []string{""}
	}
	var rows []string
	remaining := line
	for remaining != "" {
		row := ansi.Truncate(remaining, newWidth, "")
		if row == "" {
			// Degenerate case: not even one cluster fits at this width
			// (e.g. newWidth==1 with a double-width character next).
			// Dump the remainder as a single overflowing row rather
			// than looping forever — an extreme edge case, not the
			// target scenario, so "doesn't hang" is the bar, not
			// "doesn't overflow visually."
			rows = append(rows, remaining)
			break
		}
		rows = append(rows, row)
		consumed := ansi.StringWidth(row)
		remaining = ansi.TruncateLeft(remaining, consumed, "")
	}
	return rows
}

// cursorOffset returns the display-column offset of the cursor at
// (cursorRow, cursorCol) — both physical, 0-indexed — within its
// logical line, given rows (the physical rows at the OLD width) and
// physicalRowCounts (from groupIntoLogicalLines against those same
// rows). logicalLineIndex is which of groupIntoLogicalLines' returned
// lines the cursor falls in.
func cursorOffset(rows []string, physicalRowCounts []int, cursorRow, cursorCol int) (logicalLineIndex, offset int) {
	rowIdx := 0
	for li, count := range physicalRowCounts {
		if cursorRow < rowIdx+count {
			within := cursorRow - rowIdx
			off := 0
			for k := 0; k < within; k++ {
				off += ansi.StringWidth(rows[rowIdx+k])
			}
			return li, off + cursorCol
		}
		rowIdx += count
	}
	// cursorRow is beyond every known row — clamp to the end of the
	// last logical line rather than panic; SetSize's own bounds should
	// prevent this in practice, but this keeps the function total.
	last := len(physicalRowCounts) - 1
	if last < 0 {
		return 0, 0
	}
	off := 0
	for k := rowIdx - physicalRowCounts[last]; k < rowIdx; k++ {
		off += ansi.StringWidth(rows[k])
	}
	return last, off
}

// cursorAfterRewrap returns the (row, col) — both 0-indexed — that
// offset (from cursorOffset) lands at within newRows (the output of
// rewrapLogicalLine for the SAME logical line the offset was computed
// against).
func cursorAfterRewrap(newRows []string, offset int) (row, col int) {
	for i, r := range newRows {
		w := ansi.StringWidth(r)
		if offset <= w {
			return i, offset
		}
		offset -= w
	}
	if len(newRows) == 0 {
		return 0, 0
	}
	last := len(newRows) - 1
	return last, ansi.StringWidth(newRows[last])
}
