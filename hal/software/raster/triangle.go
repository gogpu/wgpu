//go:build !(js && wasm)

package raster

// EdgeFunction represents a linear edge equation: Ax + By + C = 0.
// Points on the left side of the edge (inside for CCW triangles) yield positive values.
type EdgeFunction struct {
	// A is the coefficient for x (equals y0 - y1).
	A float32

	// B is the coefficient for y (equals x1 - x0).
	B float32

	// C is the constant term (equals x0*y1 - x1*y0).
	C float32
}

// NewEdgeFunction creates an edge function from two vertices.
// The edge goes from (x0, y0) to (x1, y1).
// Points on the left side of this directed edge will have positive values.
func NewEdgeFunction(x0, y0, x1, y1 float32) EdgeFunction {
	return EdgeFunction{
		A: y0 - y1,
		B: x1 - x0,
		C: x0*y1 - x1*y0,
	}
}

// Evaluate returns the signed distance from point (x, y) to the edge.
// Positive values indicate the point is on the inside (left) of the edge.
// Zero indicates the point is exactly on the edge.
// Negative values indicate the point is on the outside (right) of the edge.
func (e EdgeFunction) Evaluate(x, y float32) float32 {
	return e.A*x + e.B*y + e.C
}

// RasterCallback is called for each fragment generated during rasterization.
type RasterCallback func(frag Fragment)

// Rasterize generates fragments for all pixels of the viewport that the
// triangle covers under the top-left fill rule (see IncrementalTriangle.IsInside).
// The callback is invoked for each fragment with interpolated attributes.
func Rasterize(tri Triangle, viewport Viewport, callback RasterCallback) {
	rasterizeIncrementalCore(tri, viewport.X, viewport.Y,
		viewport.X+viewport.Width, viewport.Y+viewport.Height, callback)
}

// ComputeTriangleArea returns twice the signed area of a triangle as the cross
// product (v1-v0) x (v2-v0) of its screen-space (framebuffer, y down)
// coordinates. Positive means clockwise as displayed; this is the opposite
// sign of the WebGPU spec area used for facing (see IsBackFacing).
func ComputeTriangleArea(v0, v1, v2 ScreenVertex) float32 {
	e01 := NewEdgeFunction(v0.X, v0.Y, v1.X, v1.Y)
	return e01.Evaluate(v2.X, v2.Y)
}

// IsBackFacing returns true if the triangle is back-facing.
//
// WebGPU spec (§ "Polygon rasterization"): with the vertices in framebuffer
// coordinates (origin top-left, y down),
//
//	area = 0.5 * ((v1.x*v2.y - v2.x*v1.y) + sum(v[i+1].x*v[i].y - v[i].x*v[i+1].y))
//
// "ccw": area > 0 is front-facing, otherwise back-facing; "cw": area < 0 is
// front-facing, otherwise back-facing. For a triangle this area is
// -ComputeTriangleArea/2, so a zero-area triangle is back-facing either way.
// (Before v0.34.6 the sign was taken the other way round — the y-up reading of
// a y-down screen — and culled the spec's front faces; P1-SW-014.)
func IsBackFacing(tri Triangle, frontFace FrontFace) bool {
	specArea := -ComputeTriangleArea(tri.V0, tri.V1, tri.V2)
	switch frontFace {
	case FrontFaceCCW:
		return !(specArea > 0)
	case FrontFaceCW:
		return !(specArea < 0)
	}
	return false
}

// ShouldCull returns true if the triangle should be culled.
func ShouldCull(tri Triangle, cullMode CullMode, frontFace FrontFace) bool {
	if cullMode == CullNone {
		return false
	}

	isBack := IsBackFacing(tri, frontFace)

	switch cullMode {
	case CullBack:
		return isBack
	case CullFront:
		return !isBack
	}
	return false
}

// RasterizeIncremental is Rasterize: every path steps the exact fixed-point
// edge functions incrementally, so there is no slower per-pixel variant.
func RasterizeIncremental(tri Triangle, viewport Viewport, callback RasterCallback) {
	Rasterize(tri, viewport, callback)
}

// RasterizeTile rasterizes a triangle within a specific tile.
// This is optimized for tile-based parallel rasterization where each tile
// is processed independently. The coverage is the same as Rasterize's, so
// tiles never draw a shared edge twice or leave a crack at a tile boundary.
func RasterizeTile(tri Triangle, tile Tile, callback RasterCallback) {
	rasterizeIncrementalCore(tri, tile.MinX, tile.MinY, tile.MaxX, tile.MaxY, callback)
}

// rasterizeIncrementalCore is the shared implementation for all rasterization
// paths. It rasterizes the triangle within the pixel bounds
// [startX, endX) x [startY, endY).
func rasterizeIncrementalCore(tri Triangle, startX, startY, endX, endY int, callback RasterCallback) {
	incTri := NewIncrementalTriangle(tri)
	if incTri.IsDegenerate() {
		return
	}

	// Only pixels whose center lies in the snapped bounding box can be covered.
	startX = maxInt(startX, incTri.minX)
	endX = minInt(endX, incTri.maxX)
	startY = maxInt(startY, incTri.minY)
	endY = minInt(endY, incTri.maxY)
	if startX >= endX || startY >= endY {
		return
	}

	attrCount := len(tri.V0.Attributes)

	incTri.setRowPixel(startX, startY)
	for y := startY; y < endY; y++ {
		for x := startX; x < endX; x++ {
			if incTri.IsInside() {
				frag := computeFragment(x, y, &tri, &incTri, attrCount)
				callback(frag)
			}
			incTri.StepX()
		}
		incTri.NextRow()
	}
}

// computeFragment computes a fragment with interpolated attributes.
func computeFragment(x, y int, tri *Triangle, incTri *IncrementalTriangle, attrCount int) Fragment {
	b0, b1, b2 := incTri.Barycentric()

	// Perspective-correct interpolation
	oneOverW := b0*tri.V0.W + b1*tri.V1.W + b2*tri.V2.W

	depth := interpolateDepth(b0, b1, b2, tri, oneOverW)
	attrs := interpolateAttributes(b0, b1, b2, tri, oneOverW, attrCount)

	return Fragment{
		X:          x,
		Y:          y,
		Depth:      depth,
		InvW:       oneOverW,
		Bary:       [3]float32{b0, b1, b2},
		Attributes: attrs,
	}
}

// interpolateDepth performs perspective-correct depth interpolation.
func interpolateDepth(b0, b1, b2 float32, tri *Triangle, oneOverW float32) float32 {
	if oneOverW != 0 {
		return (b0*tri.V0.Z*tri.V0.W + b1*tri.V1.Z*tri.V1.W + b2*tri.V2.Z*tri.V2.W) / oneOverW
	}
	return b0*tri.V0.Z + b1*tri.V1.Z + b2*tri.V2.Z
}

// interpolateAttributes performs perspective-correct attribute interpolation.
func interpolateAttributes(b0, b1, b2 float32, tri *Triangle, oneOverW float32, attrCount int) []float32 {
	if attrCount == 0 {
		return nil
	}

	attrs := make([]float32, attrCount)
	if oneOverW != 0 {
		for i := 0; i < attrCount; i++ {
			attrs[i] = (b0*tri.V0.Attributes[i]*tri.V0.W +
				b1*tri.V1.Attributes[i]*tri.V1.W +
				b2*tri.V2.Attributes[i]*tri.V2.W) / oneOverW
		}
	} else {
		for i := 0; i < attrCount; i++ {
			attrs[i] = b0*tri.V0.Attributes[i] + b1*tri.V1.Attributes[i] + b2*tri.V2.Attributes[i]
		}
	}
	return attrs
}
