package scrollbar

import "testing"

func TestColumnReturnsAllSpacesWhenContentFitsInViewport(t *testing.T) {
	col := Column(5, 10, 0)
	if len(col) != 10 {
		t.Fatalf("got len %d, want 10", len(col))
	}
	for i, r := range col {
		if r != ' ' {
			t.Fatalf("index %d: got %q, want space", i, r)
		}
	}
}

func TestColumnPlacesThumbAtTopWhenScrolledToStart(t *testing.T) {
	col := Column(100, 10, 0)
	if len(col) != 10 {
		t.Fatalf("got len %d, want 10", len(col))
	}
	if col[0] != '█' {
		t.Fatalf("index 0: got %q, want thumb", col[0])
	}
	for i := 1; i < 10; i++ {
		if col[i] != '│' {
			t.Fatalf("index %d: got %q, want track", i, col[i])
		}
	}
}

func TestColumnPlacesThumbAtBottomWhenScrolledToEnd(t *testing.T) {
	col := Column(100, 10, 90) // maxOffset = 100-10 = 90
	if col[9] != '█' {
		t.Fatalf("index 9: got %q, want thumb", col[9])
	}
	for i := 0; i < 9; i++ {
		if col[i] != '│' {
			t.Fatalf("index %d: got %q, want track", i, col[i])
		}
	}
}

func TestColumnPlacesThumbInMiddleWhenScrolledHalfway(t *testing.T) {
	col := Column(100, 10, 45)
	if col[4] != '█' {
		t.Fatalf("index 4: got %q, want thumb; full col=%q", col[4], string(col))
	}
}

func TestColumnThumbCoversProportionalShareOfLargerViewport(t *testing.T) {
	// total=20, viewport=10, offset=10 (fully scrolled): thumbSize=5, thumbStart=5
	col := Column(20, 10, 10)
	for i := 0; i < 5; i++ {
		if col[i] != '│' {
			t.Fatalf("index %d: got %q, want track", i, col[i])
		}
	}
	for i := 5; i < 10; i++ {
		if col[i] != '█' {
			t.Fatalf("index %d: got %q, want thumb", i, col[i])
		}
	}
}

func TestColumnReturnsEmptyForNonPositiveViewport(t *testing.T) {
	if col := Column(100, 0, 0); len(col) != 0 {
		t.Fatalf("got len %d, want 0", len(col))
	}
	if col := Column(100, -3, 0); len(col) != 0 {
		t.Fatalf("got len %d, want 0", len(col))
	}
}
