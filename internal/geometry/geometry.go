// Package geometry computes placement rectangles in global, top-left-origin
// Accessibility/CoreGraphics coordinates (positive Y points downward).
package geometry

import (
	"fmt"
	"math"
)

type Rect struct{ X, Y, W, H float64 }

// Placement names a region of the usable display area: one of the four
// quadrants or one of the four halves.
type Placement string

const (
	TopLeft     Placement = "top_left"
	TopRight    Placement = "top_right"
	BottomLeft  Placement = "bottom_left"
	BottomRight Placement = "bottom_right"

	LeftHalf   Placement = "left_half"
	RightHalf  Placement = "right_half"
	TopHalf    Placement = "top_half"
	BottomHalf Placement = "bottom_half"
)

// Target returns the rectangle for the requested placement, inset by padding on
// every side. The area is split at the whole-point midpoint, so halves and
// quadrants share edges even on odd dimensions.
func Target(area Rect, placement Placement, padding float64) (Rect, error) {
	if area.W <= 0 || area.H <= 0 {
		return Rect{}, fmt.Errorf("invalid display work area: %+v", area)
	}
	if math.IsNaN(padding) || math.IsInf(padding, 0) || padding < 0 {
		return Rect{}, fmt.Errorf("invalid padding %v", padding)
	}
	// Split at the whole-point boundary to avoid half-point rounding problems.
	midX := area.X + math.Floor(area.W/2)
	midY := area.Y + math.Floor(area.H/2)
	left := Rect{X: area.X, Y: area.Y, W: midX - area.X, H: midY - area.Y}
	right := Rect{X: midX, Y: area.Y, W: area.X + area.W - midX, H: midY - area.Y}
	bottomLeft := Rect{X: area.X, Y: midY, W: left.W, H: area.Y + area.H - midY}
	bottomRight := Rect{X: midX, Y: midY, W: right.W, H: bottomLeft.H}
	bottom := area.Y + area.H - midY
	var r Rect
	switch placement {
	case TopLeft:
		r = left
	case TopRight:
		r = right
	case BottomLeft:
		r = bottomLeft
	case BottomRight:
		r = bottomRight
	case LeftHalf:
		r = Rect{X: area.X, Y: area.Y, W: left.W, H: area.H}
	case RightHalf:
		r = Rect{X: midX, Y: area.Y, W: right.W, H: area.H}
	case TopHalf:
		r = Rect{X: area.X, Y: area.Y, W: area.W, H: midY - area.Y}
	case BottomHalf:
		r = Rect{X: area.X, Y: midY, W: area.W, H: bottom}
	default:
		return Rect{}, fmt.Errorf("invalid placement %q", placement)
	}
	r.X += padding
	r.Y += padding
	r.W -= 2 * padding
	r.H -= 2 * padding
	if r.W <= 0 || r.H <= 0 {
		return Rect{}, fmt.Errorf("padding %.1f is too large for placement", padding)
	}
	return r, nil
}
