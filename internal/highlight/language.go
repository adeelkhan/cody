package highlight

import "path/filepath"

type Language string

const (
	LanguageGo         Language = "go"
	LanguagePython     Language = "python"
	LanguageJavaScript Language = "javascript"
	LanguageTypeScript Language = "typescript"
	LanguageMarkdown   Language = "markdown"
)

func LanguageForPath(path string) (Language, bool) {
	switch filepath.Ext(path) {
	case ".go":
		return LanguageGo, true
	case ".py":
		return LanguagePython, true
	case ".js", ".jsx", ".mjs":
		return LanguageJavaScript, true
	case ".ts", ".tsx":
		return LanguageTypeScript, true
	case ".md":
		return LanguageMarkdown, true
	}
	return "", false
}
