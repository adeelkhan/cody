package app

import "testing"

func TestTerminalTabRegionsAndTerminalTabAtMirrorTheEditorTabBarsColumnMath(t *testing.T) {
	tabs := []terminalTab{{}, {}, {}}
	regions := terminalTabRegions(tabs, 200)
	if len(regions) != 3 {
		t.Fatalf("got %d regions, want 3", len(regions))
	}
	for i, r := range regions {
		if r.tabIndex != i {
			t.Fatalf("got regions[%d].tabIndex=%d, want %d", i, r.tabIndex, i)
		}
		if r.startCol >= r.endCol {
			t.Fatalf("got regions[%d] startCol=%d endCol=%d, want startCol < endCol", i, r.startCol, r.endCol)
		}
		if r.closeStart >= r.closeEnd || r.closeStart < r.startCol || r.closeEnd > r.endCol {
			t.Fatalf("got regions[%d] close range [%d,%d) outside its own tab range [%d,%d)", i, r.closeStart, r.closeEnd, r.startCol, r.endCol)
		}
	}

	region, ok := terminalTabAt(regions[1].startCol, tabs, 200)
	if !ok || region.tabIndex != 1 {
		t.Fatalf("got region=%+v ok=%v for a click inside tab 1's range, want tabIndex=1", region, ok)
	}
}

func TestTerminalTabLabelIsPositionalSinceASessionHasNoFilename(t *testing.T) {
	tabs := []terminalTab{{}, {}}
	if got := terminalTabDisplayName(tabs, 0); got != "Shell 1" {
		t.Fatalf("got %q, want %q", got, "Shell 1")
	}
	if got := terminalTabDisplayName(tabs, 1); got != "Shell 2" {
		t.Fatalf("got %q, want %q", got, "Shell 2")
	}
}
