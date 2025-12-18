//go:build software

package raster

// IncrementalEdge optimizes edge function evaluation by using incremental
// stepping instead of per-pixel computation.
//
// The edge function E(x, y) = A*x + B*y + C can be evaluated incrementally:
//   - E(x+1, y) = E(x, y) + A (step right)
//   - E(x, y+1) = E(x, y) + B (step down)
//
// This reduces per-pixel work from 2 multiplications + 2 additions to
// just 1 addition per pixel.
type IncrementalEdge struct {
	// A is the coefficient for x (equals y0 - y1).
	// Used for stepping in the X direction.
	A float32

	// B is the coefficient for y (equals x1 - x0).
	// Used for stepping in the Y direction.
	B float32

	// C is the constant term (equals x0*y1 - x1*y0).
	C float32

	// rowStart stores the edge value at the start of the current row.
	rowStart float32

	// current stores the edge value at the current pixel.
	current float32
}

// NewIncrementalEdge creates an incremental edge from an EdgeFunction.
func NewIncrementalEdge(e EdgeFunction) IncrementalEdge {
	return IncrementalEdge{
		A: e.A,
		B: e.B,
		C: e.C,
	}
}

// NewIncrementalEdgeFromPoints creates an incremental edge from two vertices.
func NewIncrementalEdgeFromPoints(x0, y0, x1, y1 float32) IncrementalEdge {
	return IncrementalEdge{
		A: y0 - y1,
		B: x1 - x0,
		C: x0*y1 - x1*y0,
	}
}

// SetRow initializes the edge for a new scanline starting at (x, y).
// Call this at the beginning of each row.
func (ie *IncrementalEdge) SetRow(x, y float32) {
	ie.rowStart = ie.A*x + ie.B*y + ie.C
	ie.current = ie.rowStart
}

// Value returns the current edge function value.
func (ie *IncrementalEdge) Value() float32 {
	return ie.current
}

// StepX advances to the next pixel in the row (x+1).
// Call this after processing each pixel to move right.
func (ie *IncrementalEdge) StepX() {
	ie.current += ie.A
}

// NextRow advances to the next scanline.
// This moves to (startX, y+1) where startX was the x passed to SetRow.
func (ie *IncrementalEdge) NextRow() {
	ie.rowStart += ie.B
	ie.current = ie.rowStart
}

// IsTopLeft returns true if this edge is a "top" or "left" edge.
// Same logic as EdgeFunction.IsTopLeft().
func (ie *IncrementalEdge) IsTopLeft() bool {
	if ie.A > 0 {
		return true
	}
	if ie.A == 0 && ie.B < 0 {
		return true
	}
	return false
}

// IncrementalTriangle manages three incremental edges for a triangle.
// It provides efficient per-pixel testing and barycentric coordinate computation.
type IncrementalTriangle struct {
	// E01 is the edge from V0 to V1 (opposite to V2).
	E01 IncrementalEdge

	// E12 is the edge from V1 to V2 (opposite to V0).
	E12 IncrementalEdge

	// E20 is the edge from V2 to V0 (opposite to V1).
	E20 IncrementalEdge

	// InvArea is 1 / (2 * triangle area), used for barycentric normalization.
	InvArea float32

	// Bias values for fill rule (0 for top-left edges, small negative otherwise).
	bias0, bias1, bias2 float32

	// Sign is 1 for CCW triangles, -1 for CW triangles.
	sign float32
}

// NewIncrementalTriangle creates an incremental triangle from screen-space vertices.
func NewIncrementalTriangle(tri Triangle) IncrementalTriangle {
	// Create edge functions
	// E12: from V1 to V2 (opposite to V0) - controls b0
	// E20: from V2 to V0 (opposite to V1) - controls b1
	// E01: from V0 to V1 (opposite to V2) - controls b2
	e12 := NewIncrementalEdgeFromPoints(tri.V1.X, tri.V1.Y, tri.V2.X, tri.V2.Y)
	e20 := NewIncrementalEdgeFromPoints(tri.V2.X, tri.V2.Y, tri.V0.X, tri.V0.Y)
	e01 := NewIncrementalEdgeFromPoints(tri.V0.X, tri.V0.Y, tri.V1.X, tri.V1.Y)

	// Compute triangle area using e01 evaluated at V2
	// This gives 2x the signed area
	area := e01.A*tri.V2.X + e01.B*tri.V2.Y + e01.C

	// Determine sign and inverse area
	var sign float32 = 1
	if area < 0 {
		sign = -1
	}

	var invArea float32
	if area != 0 {
		invArea = 1.0 / area
	}

	// Compute fill rule biases
	var bias0, bias1, bias2 float32
	if !e12.IsTopLeft() {
		bias0 = -1e-6
	}
	if !e20.IsTopLeft() {
		bias1 = -1e-6
	}
	if !e01.IsTopLeft() {
		bias2 = -1e-6
	}

	return IncrementalTriangle{
		E01:     e01,
		E12:     e12,
		E20:     e20,
		InvArea: invArea,
		bias0:   bias0,
		bias1:   bias1,
		bias2:   bias2,
		sign:    sign,
	}
}

// SetRow initializes all edges for a scanline starting at (x, y).
func (it *IncrementalTriangle) SetRow(x, y float32) {
	it.E01.SetRow(x, y)
	it.E12.SetRow(x, y)
	it.E20.SetRow(x, y)
}

// StepX advances all edges to the next pixel in the row.
func (it *IncrementalTriangle) StepX() {
	it.E01.StepX()
	it.E12.StepX()
	it.E20.StepX()
}

// NextRow advances all edges to the next scanline.
func (it *IncrementalTriangle) NextRow() {
	it.E01.NextRow()
	it.E12.NextRow()
	it.E20.NextRow()
}

// IsInside returns true if the current point is inside the triangle.
// This applies the top-left fill rule.
func (it *IncrementalTriangle) IsInside() bool {
	w0 := it.E12.Value()
	w1 := it.E20.Value()
	w2 := it.E01.Value()

	if it.sign > 0 {
		// CCW winding
		return w0 >= it.bias0 && w1 >= it.bias1 && w2 >= it.bias2
	}
	// CW winding - signs are flipped
	return w0 <= -it.bias0 && w1 <= -it.bias1 && w2 <= -it.bias2
}

// Barycentric returns the current barycentric coordinates.
// The coordinates (b0, b1, b2) sum to 1.0 and represent weights
// for vertices V0, V1, V2 respectively.
func (it *IncrementalTriangle) Barycentric() (b0, b1, b2 float32) {
	w0 := it.E12.Value()
	w1 := it.E20.Value()
	w2 := it.E01.Value()

	if it.sign > 0 {
		// CCW winding
		b0 = w0 * it.InvArea
		b1 = w1 * it.InvArea
		b2 = w2 * it.InvArea
	} else {
		// CW winding - negate values for consistent barycentric
		b0 = -w0 * it.InvArea
		b1 = -w1 * it.InvArea
		b2 = -w2 * it.InvArea
	}
	return
}

// EdgeValues returns the current edge function values (w0, w1, w2).
// These are not normalized barycentric coordinates.
func (it *IncrementalTriangle) EdgeValues() (w0, w1, w2 float32) {
	return it.E12.Value(), it.E20.Value(), it.E01.Value()
}

// IsDegenerate returns true if the triangle has zero area.
func (it *IncrementalTriangle) IsDegenerate() bool {
	return it.InvArea == 0
}

// Area returns the signed area of the triangle.
// Positive for CCW, negative for CW.
func (it *IncrementalTriangle) Area() float32 {
	if it.InvArea == 0 {
		return 0
	}
	return 1.0 / it.InvArea
}
