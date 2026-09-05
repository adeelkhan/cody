package highlight

type Span struct {
	StartByte int
	EndByte   int
	Capture   string
}

type Highlighter interface {
	Highlight(source []byte) ([]Span, error)
}
