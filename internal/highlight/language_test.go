package highlight

import "testing"

func TestLanguageForPathKnownExtensions(t *testing.T) {
	cases := map[string]Language{
		"main.go":   LanguageGo,
		"script.py": LanguagePython,
		"app.js":    LanguageJavaScript,
		"app.jsx":   LanguageJavaScript,
		"app.mjs":   LanguageJavaScript,
		"app.ts":    LanguageTypeScript,
		"app.tsx":   LanguageTypeScript,
		"README.md": LanguageMarkdown,
	}
	for path, want := range cases {
		got, ok := LanguageForPath(path)
		if !ok || got != want {
			t.Fatalf("%s: got (%q, %v), want (%q, true)", path, got, ok, want)
		}
	}
}

func TestLanguageForPathUnknownExtension(t *testing.T) {
	if _, ok := LanguageForPath("data.bin"); ok {
		t.Fatal("expected an unsupported extension to return ok=false")
	}
}
