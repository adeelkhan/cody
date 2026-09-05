package highlight

type LineRange struct {
	StartLine int
	EndLine   int
}

func FoldLines(source []byte, folds []Fold) []LineRange {
	if len(folds) == 0 {
		return nil
	}
	offsets := lineOffsets(source)
	var ranges []LineRange
	for _, f := range folds {
		startLine := lineForByte(offsets, f.StartByte)
		endLine := lineForByte(offsets, f.EndByte)
		if endLine <= startLine {
			continue
		}
		ranges = append(ranges, LineRange{StartLine: startLine, EndLine: endLine})
	}
	return ranges
}
