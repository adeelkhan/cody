package highlight

import (
	"sort"
	"unicode/utf8"
)

type LineSpan struct {
	StartCol int
	EndCol   int
	Capture  string
}

func LineSpans(source []byte, spans []Span) map[int][]LineSpan {
	if len(spans) == 0 {
		return map[int][]LineSpan{}
	}
	lineOffsets := lineOffsets(source)
	result := make(map[int][]LineSpan)
	for _, sp := range spans {
		startLine := lineForByte(lineOffsets, sp.StartByte)
		endLine := lineForByte(lineOffsets, sp.EndByte)
		for line := startLine; line <= endLine; line++ {
			lineStartByte := lineOffsets[line]
			lineEndByte := len(source)
			if line+1 < len(lineOffsets) {
				lineEndByte = lineOffsets[line+1] - 1 // exclude the trailing "\n"
			}
			spanStart := sp.StartByte
			if spanStart < lineStartByte {
				spanStart = lineStartByte
			}
			spanEnd := sp.EndByte
			if spanEnd > lineEndByte {
				spanEnd = lineEndByte
			}
			if spanStart >= spanEnd {
				continue
			}
			startCol := utf8.RuneCount(source[lineStartByte:spanStart])
			endCol := utf8.RuneCount(source[lineStartByte:spanEnd])
			result[line] = append(result[line], LineSpan{StartCol: startCol, EndCol: endCol, Capture: sp.Capture})
		}
	}
	return result
}

func lineOffsets(source []byte) []int {
	offsets := []int{0}
	for i, b := range source {
		if b == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}

func lineForByte(offsets []int, b int) int {
	i := sort.SearchInts(offsets, b+1) - 1
	if i < 0 {
		i = 0
	}
	return i
}
