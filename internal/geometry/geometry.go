// Package geometry computes placement rectangles in global, top-left-origin
// Accessibility/CoreGraphics coordinates (positive Y points downward).
package geometry

import (
	"fmt"
	"math"
)

type Rect struct{ X, Y, W, H float64 }

// Grid describes a uniform rows×columns subdivision of the usable display
// area. Rows increase downward and columns increase rightward.
type Grid struct {
	Rows    int
	Columns int
}

// Region is a one-indexed rectangular area of a Grid. Row and Column name the
// top-left cell; the spans cover that many cells inclusively.
type Region struct {
	Row        int `json:"row"`
	Column     int `json:"column"`
	RowSpan    int `json:"row_span"`
	ColumnSpan int `json:"column_span"`
}

// Validate reports whether the grid has a usable shape.
func (g Grid) Validate() error {
	if g.Rows < 1 {
		return fmt.Errorf("rows must be at least 1")
	}
	if g.Columns < 1 {
		return fmt.Errorf("columns must be at least 1")
	}
	return nil
}

// ValidateRegion reports whether r lies entirely inside g. Bounds are checked
// before spans are compared so an extreme span cannot overflow an int.
func (g Grid) ValidateRegion(r Region) error {
	if err := g.Validate(); err != nil {
		return err
	}
	if r.Row < 1 || r.Row > g.Rows {
		return fmt.Errorf("row must be between 1 and %d", g.Rows)
	}
	if r.Column < 1 || r.Column > g.Columns {
		return fmt.Errorf("column must be between 1 and %d", g.Columns)
	}
	if r.RowSpan < 1 {
		return fmt.Errorf("row_span must be at least 1")
	}
	if r.ColumnSpan < 1 {
		return fmt.Errorf("column_span must be at least 1")
	}
	if r.RowSpan > g.Rows-r.Row+1 {
		return fmt.Errorf("row %d plus row_span %d exceeds %d rows", r.Row, r.RowSpan, g.Rows)
	}
	if r.ColumnSpan > g.Columns-r.Column+1 {
		return fmt.Errorf("column %d plus column_span %d exceeds %d columns", r.Column, r.ColumnSpan, g.Columns)
	}
	return nil
}

// Target returns the rectangle for region, inset by padding on every side.
// Interior grid lines are split at whole-point boundaries and the outer edge
// is exact, so adjacent regions share an edge and a fractional usable area is
// still covered completely.
func (g Grid) Target(area Rect, region Region, padding float64) (Rect, error) {
	if area.W <= 0 || area.H <= 0 || math.IsNaN(area.W) || math.IsNaN(area.H) ||
		math.IsInf(area.W, 0) || math.IsInf(area.H, 0) {
		return Rect{}, fmt.Errorf("invalid display work area: %+v", area)
	}
	if err := g.ValidateRegion(region); err != nil {
		return Rect{}, err
	}
	if math.IsNaN(padding) || math.IsInf(padding, 0) || padding < 0 {
		return Rect{}, fmt.Errorf("invalid padding %v", padding)
	}

	x0 := boundary(area.X, area.W, g.Columns, region.Column-1)
	x1 := boundary(area.X, area.W, g.Columns, region.Column-1+region.ColumnSpan)
	y0 := boundary(area.Y, area.H, g.Rows, region.Row-1)
	y1 := boundary(area.Y, area.H, g.Rows, region.Row-1+region.RowSpan)

	r := Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
	r.X += padding
	r.Y += padding
	r.W -= 2 * padding
	r.H -= 2 * padding
	if r.W <= 0 || r.H <= 0 {
		return Rect{}, fmt.Errorf("region has no positive area after padding %v", padding)
	}
	return r, nil
}

// boundary returns the absolute coordinate of grid line index out of parts.
// Interior lines are floored relative to start; the final line is exact so a
// fractional usable-area dimension cannot leave an uncovered sliver.
func boundary(start, length float64, parts, index int) float64 {
	if index <= 0 {
		return start
	}
	if index >= parts {
		return start + length
	}
	return start + math.Floor(length*float64(index)/float64(parts))
}
