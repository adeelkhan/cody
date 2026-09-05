package highlight

import "testing"

func captureNames(spans []Span, source []byte) map[string][]string {
	out := make(map[string][]string)
	for _, s := range spans {
		out[s.Capture] = append(out[s.Capture], string(source[s.StartByte:s.EndByte]))
	}
	return out
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestGoHighlighterCapturesKeywordsStringsCommentsNumbersFunctions(t *testing.T) {
	h, err := New(LanguageGo)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("package main\n\n// add two numbers\nfunc add(a int, b int) int {\n\treturn a + 42\n}\n\nvar greeting = \"hello\"\n")
	spans, err := h.Highlight(src)
	if err != nil {
		t.Fatal(err)
	}
	got := captureNames(spans, src)
	if !contains(got["keyword"], "func") || !contains(got["keyword"], "return") {
		t.Fatalf("got keywords %v", got["keyword"])
	}
	if !contains(got["function"], "add") {
		t.Fatalf("got functions %v", got["function"])
	}
	if !contains(got["comment"], "// add two numbers") {
		t.Fatalf("got comments %v", got["comment"])
	}
	if !contains(got["number"], "42") {
		t.Fatalf("got numbers %v", got["number"])
	}
	if !contains(got["string"], "\"hello\"") {
		t.Fatalf("got strings %v", got["string"])
	}
}

func TestPythonHighlighterCapturesBasics(t *testing.T) {
	h, err := New(LanguagePython)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("# a comment\ndef add(a, b):\n    return a + 42\ns = \"hi\"\n")
	spans, err := h.Highlight(src)
	if err != nil {
		t.Fatal(err)
	}
	got := captureNames(spans, src)
	if !contains(got["keyword"], "def") || !contains(got["keyword"], "return") {
		t.Fatalf("got keywords %v", got["keyword"])
	}
	if !contains(got["function"], "add") {
		t.Fatalf("got functions %v", got["function"])
	}
	if !contains(got["comment"], "# a comment") {
		t.Fatalf("got comments %v", got["comment"])
	}
	if !contains(got["number"], "42") {
		t.Fatalf("got numbers %v", got["number"])
	}
	if !contains(got["string"], "\"hi\"") {
		t.Fatalf("got strings %v", got["string"])
	}
}

func TestJavaScriptHighlighterCapturesBasics(t *testing.T) {
	h, err := New(LanguageJavaScript)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("// a comment\nfunction add(a, b) {\n  return a + 42;\n}\nconst s = \"hi\";\n")
	spans, err := h.Highlight(src)
	if err != nil {
		t.Fatal(err)
	}
	got := captureNames(spans, src)
	if !contains(got["keyword"], "function") || !contains(got["keyword"], "return") || !contains(got["keyword"], "const") {
		t.Fatalf("got keywords %v", got["keyword"])
	}
	if !contains(got["function"], "add") {
		t.Fatalf("got functions %v", got["function"])
	}
	if !contains(got["number"], "42") {
		t.Fatalf("got numbers %v", got["number"])
	}
	if !contains(got["string"], "\"hi\"") {
		t.Fatalf("got strings %v", got["string"])
	}
}

func TestTypeScriptHighlighterCapturesBasics(t *testing.T) {
	h, err := New(LanguageTypeScript)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("// a comment\nfunction add(a: number, b: number): number {\n  return a + 42;\n}\nconst s: string = \"hi\";\n")
	spans, err := h.Highlight(src)
	if err != nil {
		t.Fatal(err)
	}
	got := captureNames(spans, src)
	if !contains(got["keyword"], "function") || !contains(got["keyword"], "const") {
		t.Fatalf("got keywords %v", got["keyword"])
	}
	if !contains(got["function"], "add") {
		t.Fatalf("got functions %v", got["function"])
	}
	if !contains(got["string"], "\"hi\"") {
		t.Fatalf("got strings %v", got["string"])
	}
}

func TestNewReturnsErrorForUnknownLanguage(t *testing.T) {
	if _, err := New(Language("cobol")); err == nil {
		t.Fatal("expected an error for an unsupported language")
	}
}
