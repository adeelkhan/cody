package filetree

import "path/filepath"

var nerdFontIcons = map[string]string{
	".go":   "",
	".py":   "",
	".js":   "",
	".ts":   "",
	".json": "",
	".md":   "",
}

const (
	nerdFontDir     = "" // fa-folder, closed
	nerdFontDirOpen = "" // fa-folder-open
	nerdFontFile    = ""
	fallbackDir     = "+"
	fallbackDirOpen = "~"
	fallbackFile    = "-"
)

func IconFor(n *Node, nerdFont bool) string {
	if n.Type == NodeDir {
		if n.Expanded {
			if nerdFont {
				return nerdFontDirOpen
			}
			return fallbackDirOpen
		}
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
