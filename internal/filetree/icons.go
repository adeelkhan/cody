package filetree

import "path/filepath"

var nerdFontIcons = map[string]string{
	".go":   "",
	".py":   "",
	".js":   "",
	".ts":   "",
	".json": "",
	".md":   "",
}

const (
	nerdFontDir  = ""
	nerdFontFile = ""
	fallbackDir  = "+"
	fallbackFile = "-"
)

func IconFor(n *Node, nerdFont bool) string {
	if n.Type == NodeDir {
		if nerdFont {
			return nerdFontDir
		}
		return fallbackDir
	}
	if nerdFont {
		if icon, ok := nerdFontIcons[filepath.Ext(n.Name)]; ok {
			return icon
		}
		return nerdFontFile
	}
	return fallbackFile
}
