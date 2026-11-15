//go:build !(js && wasm)

package raster

import "math"

// Coverage is decided in fixed point, as hardware rasterizers do: D3D11 and
// later snap vertex positions to 16.8 fixed point, and desktop Vulkan drivers
// report subPixelPrecisionBits = 8. Exact integer edge functions are what make
// the top-left fill rule work. For an edge shared by two triangles, the value
// at a pixel center in one triangle is exactly the negation of the value in
// the other, so a center on the edge is exactly 0 in both and only the edge
// direction decides which triangle owns it. float32 edge functions cannot
// guarantee that: the triangles' bounding boxes differ, so incremental sums
// round differently, and the rounding of x0*y1 - x1*y0 can put a center that
// lies on the edge on the same side of it for both triangles (drawn twice) or
// on neither (a crack), most visibly at a vertex shared by a fan.

// SubPixelBits is the number of fractional bits that screen-space vertex
// positions are snapped to before coverage is computed (D3D 16.8).
const SubPixelBits = 8

const (
	subPixelOne  = 1 << SubPixelBits
	subPixelHalf = subPixelOne / 2

	// maxSnapped bounds a snapped coordinate to ±2^21 pixels. Every pixel
	// center evaluated lies inside the triangle's bounding box, so both
	// factors of each product in a*(x-x0) + b*(y-y0) are at most 2^30 in
	// magnitude and the edge value stays below 2^61: no int64 overflow.
	maxSnapped = 1 << 29
)

// snapCoord converts a screen coordinate in pixels to fixed point, rounding
// to nearest even. ok is false for NaN, infinities and coordinates beyond
// ±2^21 pixels, which a rasterizer can only receive when clipping did not
// bound the triangle.
func snapCoord(v float32) (fixed int64, ok bool) {
	f := math.RoundToEven(float64(v) * subPixelOne)
	if !(f >= -maxSnapped && f <= maxSnapped) {
		return 0, false
	}
	return int64(f), true
}

// snapClamped is snapCoord for positions that are evaluated, not drawn:
// out-of-range values are clamped and NaN maps to 0.
func snapClamped(v float32) int64 {
	if f, ok := snapCoord(v); ok {
		return f
	}
	if v > 0 {
		return maxSnapped
	}
	if v < 0 {
		return -maxSnapped
	}
	return 0
}

// pixelCenter returns the fixed-point position of the center of pixel i
// (i + 0.5 pixels).
func pixelCenter(i int) int64 {
	return int64(i)*subPixelOne + subPixelHalf
}

// floorPixel returns floor(n / 2^SubPixelBits): the whole pixels in the
// fixed-point value n, rounded toward negative infinity.
func floorPixel(n int64) int64 {
	q := n / subPixelOne
	if n%subPixelOne != 0 && n < 0 {
		q--
	}
	return q
}

// IncrementalEdge is one edge function of a triangle in fixed point:
//
//	E(x, y) = a*(x - x0) + b*(y - y0),  a = y0 - y1,  b = x1 - x0,
//
// with positions in units of 1/2^SubPixelBits pixel. E is exact, so it can be
// stepped from pixel to pixel by addition without drift:
//   - E(x+1, y) = E(x, y) + A (step right)
//   - E(x, y+1) = E(x, y) + B (step down)
//
// The edge from (x1, y1) to (x0, y0) has exactly the negated value at every
// point.
type IncrementalEdge struct {
	// A is the change of the edge value for one pixel step right.
	A int64

	// B is the change of the edge value for one pixel step down.
	B int64

	// a and b are the coefficients per fixed-point unit; (x0, y0) is the
	// snapped start vertex.
	a, b, x0, y0 int64

	// rowStart stores the edge value at the start of the current row.
	rowStart int64

	// current stores the edge value at the current pixel.
	current int64
}

// newIncrementalEdge creates the edge from (x0, y0) to (x1, y1), given in
// fixed point.
func newIncrementalEdge(x0, y0, x1, y1 int64) IncrementalEdge {
	a, b := y0-y1, x1-x0
	return IncrementalEdge{A: a * subPixelOne, B: b * subPixelOne, a: a, b: b, x0: x0, y0: y0}
}

// NewIncrementalEdgeFromPoints creates the edge from (x0, y0) to (x1, y1),
// given in pixels. The points are snapped to fixed point; coordinates beyond
// ±2^21 pixels are clamped.
func NewIncrementalEdgeFromPoints(x0, y0, x1, y1 float32) IncrementalEdge {
	return newIncrementalEdge(snapClamped(x0), snapClamped(y0), snapClamped(x1), snapClamped(y1))
}

// negated returns the same line with the opposite direction: its value is
// exactly -E at every point.
func (ie IncrementalEdge) negated() IncrementalEdge {
	return IncrementalEdge{A: -ie.A, B: -ie.B, a: -ie.a, b: -ie.b, x0: ie.x0, y0: ie.y0}
}

// evaluate returns the edge value at the fixed-point position (x, y).
func (ie *IncrementalEdge) evaluate(x, y int64) int64 {
	return ie.a*(x-ie.x0) + ie.b*(y-ie.y0)
}

// setRowFixed initializes the edge for a scanline starting at the fixed-point
// position (x, y).
func (ie *IncrementalEdge) setRowFixed(x, y int64) {
	ie.rowStart = ie.evaluate(x, y)
	ie.current = ie.rowStart
}

// SetRow initializes the edge for a new scanline starting at (x, y) in pixels
// (pixel centers are at i + 0.5). Call this at the beginning of each row.
func (ie *IncrementalEdge) SetRow(x, y float32) {
	ie.setRowFixed(snapClamped(x), snapClamped(y))
}

// Value returns the current edge function value, in fixed-point units
// squared (1/2^(2*SubPixelBits) pixel²).
func (ie *IncrementalEdge) Value() int64 {
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

// IsTopLeft reports whether the edge is a top or a left edge of a triangle
// whose interior lies where the edge value is positive (NewIncrementalTriangle
// orients all three edges that way). In screen space, y down:
//   - a top edge is exactly horizontal with the interior below it: a == 0 and
//     the value grows with y, b > 0;
//   - a left edge is not horizontal and has the interior to its right: the
//     value grows with x, a > 0.
func (ie *IncrementalEdge) IsTopLeft() bool {
	return ie.a > 0 || (ie.a == 0 && ie.b > 0)
}

// covers applies the top-left rule to an edge value w at a pixel center: the
// center is inside the edge if w > 0, or if it lies exactly on the edge and the
// edge is a top or a left edge.
func (ie *IncrementalEdge) covers(w int64) bool {
	return w > 0 || (w == 0 && ie.IsTopLeft())
}

// IncrementalTriangle manages three incremental edges for a triangle.
// It provides efficient per-pixel testing and barycentric coordinate computation.
//
// The edges are oriented so that the interior has positive values whatever
// the winding of the input, which makes coverage independent of the winding.
type IncrementalTriangle struct {
	// E01 is the edge from V0 to V1 (opposite to V2).
	E01 IncrementalEdge

	// E12 is the edge from V1 to V2 (opposite to V0).
	E12 IncrementalEdge

	// E20 is the edge from V2 to V0 (opposite to V1).
	E20 IncrementalEdge

	// area is twice the area of the snapped triangle in fixed-point units
	// squared: positive, or 0 when the triangle covers nothing.
	area int64

	// invArea is 1/area, used for barycentric normalization.
	invArea float64

	// sign is 1 when V0, V1, V2 give a positive E01(V2) (clockwise as
	// displayed on a y-down screen), -1 otherwise.
	sign int64

	// minX, minY, maxX, maxY bound the pixels whose center lies in the
	// snapped bounding box; max is exclusive.
	minX, minY, maxX, maxY int
}

// NewIncrementalTriangle creates an incremental triangle from screen-space
// vertices. The vertices are snapped to fixed point (SubPixelBits). A
// triangle with zero snapped area, or with a vertex that cannot be snapped
// (NaN, infinite, or beyond ±2^21 pixels), is degenerate and covers nothing.
func NewIncrementalTriangle(tri Triangle) IncrementalTriangle {
	x0, okX0 := snapCoord(tri.V0.X)
	y0, okY0 := snapCoord(tri.V0.Y)
	x1, okX1 := snapCoord(tri.V1.X)
	y1, okY1 := snapCoord(tri.V1.Y)
	x2, okX2 := snapCoord(tri.V2.X)
	y2, okY2 := snapCoord(tri.V2.Y)
	if !okX0 || !okY0 || !okX1 || !okY1 || !okX2 || !okY2 {
		return IncrementalTriangle{}
	}

	// E12 controls b0, E20 controls b1, E01 controls b2.
	it := IncrementalTriangle{
		E12:  newIncrementalEdge(x1, y1, x2, y2),
		E20:  newIncrementalEdge(x2, y2, x0, y0),
		E01:  newIncrementalEdge(x0, y0, x1, y1),
		sign: 1,
	}
	area := it.E01.evaluate(x2, y2)
	switch {
	case area == 0:
		return IncrementalTriangle{}
	case area < 0:
		// Orient the edges so the interior is positive. The top-left
		// classification below then sees the same edge direction for both
		// windings, so both windings cover the same pixels.
		it.E12, it.E20, it.E01 = it.E12.negated(), it.E20.negated(), it.E01.negated()
		it.sign = -1
		area = -area
	}
	it.area = area
	it.invArea = 1 / float64(area)

	minFX, maxFX := min(x0, x1, x2), max(x0, x1, x2)
	minFY, maxFY := min(y0, y1, y2), max(y0, y1, y2)
	// First pixel whose center is >= min, last one whose center is <= max.
	it.minX = int(-floorPixel(subPixelHalf - minFX))
	it.maxX = int(floorPixel(maxFX-subPixelHalf)) + 1
	it.minY = int(-floorPixel(subPixelHalf - minFY))
	it.maxY = int(floorPixel(maxFY-subPixelHalf)) + 1
	return it
}

// setRowPixel initializes all edges at the center of pixel (x, y).
func (it *IncrementalTriangle) setRowPixel(x, y int) {
	px, py := pixelCenter(x), pixelCenter(y)
	it.E01.setRowFixed(px, py)
	it.E12.setRowFixed(px, py)
	it.E20.setRowFixed(px, py)
}

// SetRow initializes all edges for a scanline starting at (x, y) in pixels.
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

// IsInside reports whether the current point is covered by the triangle under
// the top-left fill rule (D3D11/D3D12 rasterization rules, which Vulkan and
// WebGPU implementations follow): inside all three edges, where a point
// exactly on an edge counts as inside only for a top or a left edge. In a
// tiling, every pixel center is therefore covered by exactly one triangle.
func (it *IncrementalTriangle) IsInside() bool {
	return it.area != 0 &&
		it.E12.covers(it.E12.current) &&
		it.E20.covers(it.E20.current) &&
		it.E01.covers(it.E01.current)
}

// Barycentric returns the current barycentric coordinates.
// The coordinates (b0, b1, b2) sum to 1.0 and represent weights
// for vertices V0, V1, V2 respectively.
func (it *IncrementalTriangle) Barycentric() (b0, b1, b2 float32) {
	b0 = float32(float64(it.E12.current) * it.invArea)
	b1 = float32(float64(it.E20.current) * it.invArea)
	b2 = float32(float64(it.E01.current) * it.invArea)
	return
}

// EdgeValues returns the current edge function values (w0, w1, w2), oriented
// so the interior is positive. These are not normalized barycentric
// coordinates.
func (it *IncrementalTriangle) EdgeValues() (w0, w1, w2 int64) {
	return it.E12.current, it.E20.current, it.E01.current
}

// IsDegenerate reports whether the triangle covers nothing: its snapped area
// is zero, or a vertex could not be snapped.
func (it *IncrementalTriangle) IsDegenerate() bool {
	return it.area == 0
}

// Area returns twice the signed area of the snapped triangle in pixels²,
// positive when E01(V2) > 0 (clockwise as displayed on a y-down screen).
func (it *IncrementalTriangle) Area() float32 {
	return float32(float64(it.sign*it.area) / (subPixelOne * subPixelOne))
}
