package geometry

import "testing"

func TestFourQuadrantsOddDimensions(t *testing.T) {
	area := Rect{X: -100, Y: 22, W: 101, H: 81}
	tests := []struct {
		placement Placement
		want      Rect
	}{
		{TopLeft, Rect{-100, 22, 50, 40}},
		{TopRight, Rect{-50, 22, 51, 40}},
		{BottomLeft, Rect{-100, 62, 50, 41}},
		{BottomRight, Rect{-50, 62, 51, 41}},
	}
	for _, tc := range tests {
		got, err := Target(area, tc.placement, 0)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %+v, %v; want %+v", tc.placement, got, err, tc.want)
		}
	}
}

func TestFourHalvesOddDimensions(t *testing.T) {
	area := Rect{X: -100, Y: 22, W: 101, H: 81}
	tests := []struct {
		placement Placement
		want      Rect
	}{
		{LeftHalf, Rect{-100, 22, 50, 81}},
		{RightHalf, Rect{-50, 22, 51, 81}},
		{TopHalf, Rect{-100, 22, 101, 40}},
		{BottomHalf, Rect{-100, 62, 101, 41}},
	}
	for _, tc := range tests {
		got, err := Target(area, tc.placement, 0)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %+v, %v; want %+v", tc.placement, got, err, tc.want)
		}
	}
}

func TestPadding(t *testing.T) {
	got, err := Target(Rect{0, 25, 200, 100}, TopLeft, 8)
	if err != nil || got != (Rect{8, 33, 84, 34}) {
		t.Fatalf("got %+v, %v", got, err)
	}
	// Padding insets a half on every side, like a quadrant.
	got, err = Target(Rect{0, 0, 200, 100}, LeftHalf, 8)
	if err != nil || got != (Rect{8, 8, 84, 84}) {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestInvalid(t *testing.T) {
	for _, tc := range []struct {
		area      Rect
		placement Placement
		pad       float64
	}{
		{Rect{0, 0, 0, 100}, TopLeft, 0},
		{Rect{0, 0, 100, 100}, "middle", 0},
		{Rect{0, 0, 100, 100}, TopRight, -1},
		{Rect{0, 0, 100, 100}, TopRight, 100},
		{Rect{0, 0, 100, 100}, LeftHalf, 100},
	} {
		if _, err := Target(tc.area, tc.placement, tc.pad); err == nil {
			t.Errorf("expected error for %+v", tc)
		}
	}
}
