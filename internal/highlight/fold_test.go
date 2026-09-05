package highlight

import "testing"

// lineOfByteForTest is a small, self-contained line-counting helper local
// to this test file — deliberately NOT the real FoldLines/lineForByte
// production helpers (those don't exist until Task 2). This keeps Task 1
// fully independent and testable on its own, at the cost of duplicating
// a few lines of simple counting logic that Task 2 formalizes properly.
func lineOfByteForTest(source []byte, b int) int {
	line := 0
	for i := 0; i < b && i < len(source); i++ {
		if source[i] == '\n' {
			line++
		}
	}
	return line
}

func containsFoldLines(t *testing.T, folds []Fold, source []byte, startLine, endLine int) bool {
	t.Helper()
	for _, f := range folds {
		if lineOfByteForTest(source, f.StartByte) == startLine && lineOfByteForTest(source, f.EndByte) == endLine {
			return true
		}
	}
	return false
}

func TestGoFolderFindsFunctionBody(t *testing.T) {
	f, err := NewFolder(LanguageGo)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("package main\n\nfunc add(a int, b int) int {\n\treturn a + 42\n}\n")
	folds, err := f.Folds(src)
	if err != nil {
		t.Fatal(err)
	}
	// Lines (0-indexed): 0="package main", 1="", 2="func add(...) int {",
	// 3="\treturn a + 42", 4="}". The function body's block node spans
	// from the opening brace (end of line 2) through the closing brace
	// (line 4) — independently verified against the real grammar.
	if !containsFoldLines(t, folds, src, 2, 4) {
		t.Fatalf("got %v, want a fold spanning lines 2-4 (the function body)", folds)
	}
}

func TestPythonFolderFindsFunctionBodyNotTheNestedOneLiner(t *testing.T) {
	f, err := NewFolder(LanguagePython)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("def add(a, b):\n    if a > 0:\n        return a + b\n    return b\n")
	folds, err := f.Folds(src)
	if err != nil {
		t.Fatal(err)
	}
	// The function's own block starts at its first statement (the "if",
	// line 1) and ends at its last ("return b", line 3) — verified
	// against the real grammar. The nested "if" body ("return a + b") is
	// a separate, single-line block (StartByte and EndByte both on line
	// 2) — this test only checks the function-level fold is present;
	// Task 2's FoldLines is what formally drops single-line ranges like
	// that one from what the editor actually offers to fold.
	if !containsFoldLines(t, folds, src, 1, 3) {
		t.Fatalf("got %v, want a fold spanning lines 1-3 (the function body)", folds)
	}
}

func TestJavaScriptFolderFindsNestedBlocks(t *testing.T) {
	f, err := NewFolder(LanguageJavaScript)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("function add(a, b) {\n  if (a > 0) {\n    return a + b;\n  }\n  return b;\n}\n")
	folds, err := f.Folds(src)
	if err != nil {
		t.Fatal(err)
	}
	// Verified against the real grammar: the function's own
	// statement_block spans lines 0-5 (the whole function), and the
	// nested if's statement_block spans lines 1-3 — both multi-line, both
	// expected to survive as separate, nested folds.
	if !containsFoldLines(t, folds, src, 0, 5) {
		t.Fatalf("got %v, want a fold spanning lines 0-5 (the whole function)", folds)
	}
	if !containsFoldLines(t, folds, src, 1, 3) {
		t.Fatalf("got %v, want a fold spanning lines 1-3 (the nested if block)", folds)
	}
}

func TestTypeScriptFolderFindsFunctionBody(t *testing.T) {
	f, err := NewFolder(LanguageTypeScript)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("function add(a: number, b: number): number {\n  return a + b;\n}\n")
	folds, err := f.Folds(src)
	if err != nil {
		t.Fatal(err)
	}
	if !containsFoldLines(t, folds, src, 0, 2) {
		t.Fatalf("got %v, want a fold spanning lines 0-2 (the function body)", folds)
	}
}

func TestNewFolderReturnsErrorForUnknownLanguage(t *testing.T) {
	if _, err := NewFolder(Language("cobol")); err == nil {
		t.Fatal("expected an error for an unsupported language")
	}
}
