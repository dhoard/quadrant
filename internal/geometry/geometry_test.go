package geometry

import (
	"math"
	"testing"
)

// The 2×2 grid is the regression oracle for the placements the program had
// before grids existed: the same quadrants and halves, including odd areas.
func TestDefault2x2MatchesFixedPlacements(t *testing.T) {
	area := Rect{X: -100, Y: 22, W: 101, H: 81}
	grid := Grid{Rows: 2, Columns: 2}
	tests := []struct {
		name   string
		region Region
		want   Rect
	}{
		{"top-left", Region{1, 1, 1, 1}, Rect{-100, 22, 50, 40}},
		{"top-right", Region{1, 2, 1, 1}, Rect{-50, 22, 51, 40}},
		{"bottom-left", Region{2, 1, 1, 1}, Rect{-100, 62, 50, 41}},
		{"bottom-right", Region{2, 2, 1, 1}, Rect{-50, 62, 51, 41}},
		{"left-half", Region{1, 1, 2, 1}, Rect{-100, 22, 50, 81}},
		{"right-half", Region{1, 2, 2, 1}, Rect{-50, 22, 51, 81}},
		{"top-half", Region{1, 1, 1, 2}, Rect{-100, 22, 101, 40}},
		{"bottom-half", Region{2, 1, 1, 2}, Rect{-100, 62, 101, 41}},
	}
	for _, tc := range tests {
		got, err := grid.Target(area, tc.region, 0)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %+v, %v; want %+v", tc.name, got, err, tc.want)
		}
	}
}

func TestOneCellGridCoversArea(t *testing.T) {
	area := Rect{X: 10, Y: 20, W: 100, H: 50}
	got, err := (Grid{Rows: 1, Columns: 1}).Target(area, Region{1, 1, 1, 1}, 0)
	if err != nil || got != area {
		t.Errorf("got %+v, %v; want the whole area %+v", got, err, area)
	}
}

func TestThreeByThreeGrid(t *testing.T) {
	area := Rect{X: 0, Y: 0, W: 100, H: 90}
	grid := Grid{Rows: 3, Columns: 3}
	tests := []struct {
		region Region
		want   Rect
	}{
		{Region{2, 2, 1, 1}, Rect{33, 30, 33, 30}},
		{Region{1, 1, 3, 3}, Rect{0, 0, 100, 90}},
	}
	for _, tc := range tests {
		got, err := grid.Target(area, tc.region, 0)
		if err != nil || got != tc.want {
			t.Errorf("%+v: got %+v, %v; want %+v", tc.region, got, err, tc.want)
		}
	}
}

func TestGridSubdivisionAndSpans(t *testing.T) {
	area := Rect{X: 0, Y: 0, W: 101, H: 81}
	grid := Grid{Rows: 4, Columns: 4}
	tests := []struct {
		name   string
		region Region
		want   Rect
	}{
		{"first-cell", Region{1, 1, 1, 1}, Rect{0, 0, 25, 20}},
		{"third-column", Region{1, 3, 1, 1}, Rect{50, 0, 25, 20}},
		{"last-cell", Region{4, 4, 1, 1}, Rect{75, 60, 26, 21}},
		{"two-by-two-span", Region{2, 2, 2, 2}, Rect{25, 20, 50, 40}},
		{"three-by-three-span", Region{1, 1, 3, 3}, Rect{0, 0, 75, 60}},
		{"top-right-cell", Region{1, 4, 1, 1}, Rect{75, 0, 26, 20}},
		{"bottom-left-cell", Region{4, 1, 1, 1}, Rect{0, 60, 25, 21}},
	}
	for _, tc := range tests {
		got, err := grid.Target(area, tc.region, 0)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %+v, %v; want %+v", tc.name, got, err, tc.want)
		}
	}
}

// Fractional usable areas must still end exactly at the outer edge. The
// interior line is floored relative to the area start, so it can be fractional
// only when the area itself is fractional.
func TestFractionalArea(t *testing.T) {
	area := Rect{X: 10, Y: 20, W: 100.5, H: 50.5}
	grid := Grid{Rows: 2, Columns: 2}
	tests := []struct {
		region Region
		want   Rect
	}{
		{Region{1, 1, 1, 1}, Rect{10, 20, 50, 25}},
		{Region{1, 2, 1, 1}, Rect{60, 20, 50.5, 25}},
		{Region{2, 1, 1, 1}, Rect{10, 45, 50, 25.5}},
		{Region{2, 2, 1, 1}, Rect{60, 45, 50.5, 25.5}},
		{Region{1, 1, 1, 2}, Rect{10, 20, 100.5, 25}},
	}
	for _, tc := range tests {
		got, err := grid.Target(area, tc.region, 0)
		if err != nil || got != tc.want {
			t.Errorf("%+v: got %+v, %v; want %+v", tc.region, got, err, tc.want)
		}
	}
}

func TestPaddingInsetsTheAssignedRegion(t *testing.T) {
	tests := []struct {
		name   string
		grid   Grid
		area   Rect
		region Region
		want   Rect
	}{
		{"quadrant", Grid{2, 2}, Rect{0, 25, 200, 100}, Region{1, 1, 1, 1}, Rect{8, 33, 84, 34}},
		{"half", Grid{2, 2}, Rect{0, 25, 200, 100}, Region{1, 1, 2, 1}, Rect{8, 33, 84, 84}},
		{"span", Grid{4, 4}, Rect{0, 0, 200, 100}, Region{1, 1, 2, 2}, Rect{8, 8, 84, 34}},
	}
	for _, tc := range tests {
		got, err := tc.grid.Target(tc.area, tc.region, 8)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %+v, %v; want %+v", tc.name, got, err, tc.want)
		}
	}
}

// Padding is measured from each window's own region, so two adjacent windows
// end up twice the padding apart. A spanning region gets no internal gutter.
func TestAdjacentPaddingIsPerWindow(t *testing.T) {
	grid := Grid{Rows: 4, Columns: 4}
	area := Rect{X: 0, Y: 0, W: 200, H: 100}
	left, err := grid.Target(area, Region{1, 1, 2, 2}, 8)
	if err != nil {
		t.Fatal(err)
	}
	right, err := grid.Target(area, Region{1, 3, 2, 2}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if right.X-(left.X+left.W) != 16 {
		t.Errorf("gap between adjacent windows = %v; want 16", right.X-(left.X+left.W))
	}
	span, err := grid.Target(area, Region{1, 1, 2, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if span != (Rect{0, 0, 100, 50}) {
		t.Errorf("span with no padding = %+v; want the union of its cells", span)
	}
}

func TestInvalid(t *testing.T) {
	validArea := Rect{0, 0, 100, 100}
	tests := []struct {
		name   string
		grid   Grid
		area   Rect
		region Region
		pad    float64
	}{
		{"zero-area", Grid{2, 2}, Rect{0, 0, 0, 100}, Region{1, 1, 1, 1}, 0},
		{"NaN-area", Grid{2, 2}, Rect{0, 0, math.NaN(), 100}, Region{1, 1, 1, 1}, 0},
		{"zero-rows", Grid{0, 2}, validArea, Region{1, 1, 1, 1}, 0},
		{"zero-columns", Grid{2, 0}, validArea, Region{1, 1, 1, 1}, 0},
		{"row-zero", Grid{2, 2}, validArea, Region{0, 1, 1, 1}, 0},
		{"row-too-large", Grid{2, 2}, validArea, Region{3, 1, 1, 1}, 0},
		{"column-zero", Grid{2, 2}, validArea, Region{1, 0, 1, 1}, 0},
		{"column-too-large", Grid{2, 2}, validArea, Region{1, 3, 1, 1}, 0},
		{"row-span-zero", Grid{2, 2}, validArea, Region{1, 1, 0, 1}, 0},
		{"column-span-zero", Grid{2, 2}, validArea, Region{1, 1, 1, 0}, 0},
		{"row-span-too-large", Grid{2, 2}, validArea, Region{2, 1, 2, 1}, 0},
		{"column-span-too-large", Grid{2, 2}, validArea, Region{1, 2, 1, 2}, 0},
		{"negative-padding", Grid{2, 2}, validArea, Region{1, 1, 1, 1}, -1},
		{"padding-too-large", Grid{2, 2}, validArea, Region{1, 1, 1, 1}, 100},
		{"zero-width-cell", Grid{Rows: 1, Columns: 200}, validArea, Region{1, 1, 1, 1}, 0},
	}
	for _, tc := range tests {
		if _, err := tc.grid.Target(tc.area, tc.region, tc.pad); err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}
	}
	if _, err := (Grid{Rows: 2, Columns: 2}).Target(validArea, Region{1, 1, 1, 1}, math.NaN()); err == nil {
		t.Error("NaN padding was accepted")
	}
	if _, err := (Grid{Rows: 2, Columns: 2}).Target(validArea, Region{1, 1, 1, 1}, math.Inf(1)); err == nil {
		t.Error("infinite padding was accepted")
	}
}
