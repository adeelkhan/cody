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
