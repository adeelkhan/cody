// Package scrollbar computes the visual position of a vertical scrollbar
// thumb for a viewport onto a larger list of rows.
package scrollbar

// Column returns a viewport-length slice of runes representing a vertical
// scrollbar for content of `total` rows shown through a window of
// `viewport` rows starting at `offset`. Thumb rows are '█', track rows are
// '│'. When the content fits entirely within the viewport (total <=
// viewport), there is nothing to scroll, so every rune is a space.
func Column(total, viewport, offset int) []rune {
	if viewport <= 0 {
		return nil
	}
	col := make([]rune, viewport)
	if total <= viewport {
		for i := range col {
			col[i] = ' '
		}
		return col
	}

	thumbSize := viewport * viewport / total
	if thumbSize < 1 {
		thumbSize = 1
	}
	if thumbSize > viewport {
		thumbSize = viewport
	}

	maxOffset := total - viewport
	thumbStart := 0
	if maxOffset > 0 {
		thumbStart = offset * (viewport - thumbSize) / maxOffset
	}
	if thumbStart < 0 {
		thumbStart = 0
	}
	if thumbStart+thumbSize > viewport {
		thumbStart = viewport - thumbSize
	}

	for i := range col {
		if i >= thumbStart && i < thumbStart+thumbSize {
			col[i] = '█'
		} else {
			col[i] = '│'
		}
	}
	return col
}
