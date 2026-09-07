package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"cody/internal/editor"
)

func TestTabDisplayNameForUntitledTab(t *testing.T) {
	if got := tabDisplayName(""); got != "Untitled" {
		t.Fatalf("got %q, want %q", got, "Untitled")
	}
}

func TestTabDisplayNameForRealPath(t *testing.T) {
	if got := tabDisplayName("/a/b/c.go"); got != "c.go" {
		t.Fatalf("got %q, want %q", got, "c.go")
	}
}

func TestTabRegionsAreContiguousAndOrdered(t *testing.T) {
	tabs := []tab{{path: "/a.go"}, {path: "/bb.go"}, {path: "/ccc.go"}}
	regions := tabRegions(tabs, 200)
	if len(regions) != 3 {
		t.Fatalf("got %d regions, want 3", len(regions))
	}
	if regions[0].startCol != 0 {
		t.Fatalf("got first region startCol=%d, want 0", regions[0].startCol)
	}
	for i := 1; i < len(regions); i++ {
		if regions[i].startCol != regions[i-1].endCol {
			t.Fatalf("region %d starts at %d, want %d (immediately after region %d)", i, regions[i].startCol, regions[i-1].endCol, i-1)
		}
	}
	for i, r := range regions {
		if r.tabIndex != i {
			t.Fatalf("region %d has tabIndex=%d, want %d", i, r.tabIndex, i)
		}
		if r.closeStart < r.startCol || r.closeEnd > r.endCol {
			t.Fatalf("region %d's close range [%d,%d) falls outside its own range [%d,%d)", i, r.closeStart, r.closeEnd, r.startCol, r.endCol)
		}
	}
}

func TestTabAtFindsCorrectRegionIncludingCloseGlyph(t *testing.T) {
	tabs := []tab{{path: "/a.go"}, {path: "/bb.go"}}
	regions := tabRegions(tabs, 200)

	first, ok := tabAt(regions[0].startCol, tabs, 200)
	if !ok || first.tabIndex != 0 {
		t.Fatalf("got %+v, ok=%v, want tab 0", first, ok)
	}
	closeClick, ok := tabAt(regions[0].closeStart, tabs, 200)
	if !ok || closeClick.tabIndex != 0 {
		t.Fatalf("expected clicking tab 0's close glyph to still resolve to tab 0, got %+v, ok=%v", closeClick, ok)
	}
	second, ok := tabAt(regions[1].startCol, tabs, 200)
	if !ok || second.tabIndex != 1 {
		t.Fatalf("got %+v, ok=%v, want tab 1", second, ok)
	}
	_, ok = tabAt(regions[len(regions)-1].endCol, tabs, 200)
	if ok {
		t.Fatal("expected a column past the last tab to miss")
	}
}

func TestTabRegionsTruncatesAtNarrowWidth(t *testing.T) {
	tabs := []tab{
		{path: "/one.go"}, {path: "/two.go"}, {path: "/three.go"},
		{path: "/four.go"}, {path: "/five.go"}, {path: "/six.go"},
	}
	const width = 20
	regions := tabRegions(tabs, width)
	for _, r := range regions {
		if r.startCol >= width {
			t.Fatalf("region %+v has startCol >= width(%d)", r, width)
		}
		if r.endCol > width {
			t.Fatalf("region %+v has endCol > width(%d)", r, width)
		}
	}
}

func TestRenderTabBarShowsNamesCloseGlyphsAndActiveTabReversed(t *testing.T) {
	// Force a color profile that renders SGR attributes: the default test
	// color profile (no real terminal attached) strips all styling,
	// including non-color attributes like Reverse — see the identical note
	// on TestRehighlightMsgReachesEditorEvenWhenTreeIsFocused in
	// model_test.go.
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	tabs := []tab{
		{path: "/project/one.go", editor: editor.New()},
		{path: "/project/two.go", editor: editor.New()},
	}
	view := renderTabBar(60, tabs, 1)

	if !strings.Contains(view, "one.go") || !strings.Contains(view, "two.go") {
		t.Fatalf("expected both tab names present, got %q", view)
	}
	if !strings.Contains(view, "×") {
		t.Fatal("expected a close glyph for each tab")
	}
	if !strings.Contains(view, "\x1b[7m") {
		t.Fatal("expected the active tab to be shown in reverse video")
	}
}

func TestRenderTabBarShowsDirtyMarkerForUnsavedTab(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	e, err := editor.New().LoadFile(mustWriteAndReturn(t, file, "package a"))
	if err != nil {
		t.Fatal(err)
	}
	e, _ = e.Update(keyRune('x'))
	tabs := []tab{{path: file, editor: e}}

	view := renderTabBar(40, tabs, 0)
	if !strings.Contains(view, "(M)") {
		t.Fatalf("expected a modified indicator for the dirty tab, got %q", view)
	}
}
