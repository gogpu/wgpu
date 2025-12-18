//go:build software

package raster

import (
	"math"
)

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

// IsTopLeft returns true if this edge is a "top" or "left" edge.
// Used for the top-left fill rule to avoid double-drawing shared edges.
//
// In screen coordinates (Y increases downward):
// - Left edge: edge going upward (A > 0), meaning Y decreases along the edge
// - Top edge: horizontal edge going leftward (A == 0 && B < 0)
//
// Note: B < 0 means the edge goes from right to left (x decreases along edge).
func (e EdgeFunction) IsTopLeft() bool {
	// Left edge: goes upward in screen space (Y decreases, so A > 0)
	if e.A > 0 {
		return true
	}
	// Top edge: horizontal (A == 0) and going leftward (B < 0)
	if e.A == 0 && e.B < 0 {
		return true
	}
	return false
}

// RasterCallback is called for each fragment generated during rasterization.
type RasterCallback func(frag Fragment)

// Rasterize generates fragments for all pixels inside the triangle.
// It uses the edge function algorithm with top-left fill rule.
// The callback is invoked for each fragment with interpolated attributes.
func Rasterize(tri Triangle, viewport Viewport, callback RasterCallback) {
	// Compute bounding box of the triangle
	minX := min3(tri.V0.X, tri.V1.X, tri.V2.X)
	maxX := max3(tri.V0.X, tri.V1.X, tri.V2.X)
	minY := min3(tri.V0.Y, tri.V1.Y, tri.V2.Y)
	maxY := max3(tri.V0.Y, tri.V1.Y, tri.V2.Y)

	// Convert to integer pixel coordinates
	// We add 0.5 to maxX/maxY because pixel centers are at (x+0.5, y+0.5)
	startX := int(math.Floor(float64(minX)))
	endX := int(math.Ceil(float64(maxX)))
	startY := int(math.Floor(float64(minY)))
	endY := int(math.Ceil(float64(maxY)))

	// Clip to viewport
	startX = maxInt(startX, viewport.X)
	endX = minInt(endX, viewport.X+viewport.Width)
	startY = maxInt(startY, viewport.Y)
	endY = minInt(endY, viewport.Y+viewport.Height)

	// Early exit if nothing to draw
	if startX >= endX || startY >= endY {
		return
	}

	// Create edge functions
	// Edge 12: from V1 to V2 (opposite to V0)
	// Edge 20: from V2 to V0 (opposite to V1)
	// Edge 01: from V0 to V1 (opposite to V2)
	e12 := NewEdgeFunction(tri.V1.X, tri.V1.Y, tri.V2.X, tri.V2.Y)
	e20 := NewEdgeFunction(tri.V2.X, tri.V2.Y, tri.V0.X, tri.V0.Y)
	e01 := NewEdgeFunction(tri.V0.X, tri.V0.Y, tri.V1.X, tri.V1.Y)

	// Compute triangle area (2x actual area, but sign matters for winding)
	// This is the same as e01.Evaluate(tri.V2.X, tri.V2.Y)
	area := e01.Evaluate(tri.V2.X, tri.V2.Y)

	// Degenerate triangle (zero area)
	if area == 0 {
		return
	}

	// Inverse area for barycentric normalization
	invArea := 1.0 / area

	// Determine top-left bias for fill rule
	// If the edge is NOT top-left, we need the value to be strictly positive
	bias0 := float32(0)
	bias1 := float32(0)
	bias2 := float32(0)
	if !e12.IsTopLeft() {
		bias0 = -1e-6 // Small negative bias means we need strictly positive
	}
	if !e20.IsTopLeft() {
		bias1 = -1e-6
	}
	if !e01.IsTopLeft() {
		bias2 = -1e-6
	}

	// Precompute attribute count
	attrCount := len(tri.V0.Attributes)

	// Rasterize pixels
	for y := startY; y < endY; y++ {
		for x := startX; x < endX; x++ {
			// Sample at pixel center
			px := float32(x) + 0.5
			py := float32(y) + 0.5

			// Evaluate edge functions
			w0 := e12.Evaluate(px, py)
			w1 := e20.Evaluate(px, py)
			w2 := e01.Evaluate(px, py)

			// Apply fill rule bias
			// For CCW triangles with positive area, inside pixels have all positive values
			// For CW triangles with negative area, we need to flip the sign
			if area > 0 {
				// CCW winding
				if w0 < bias0 || w1 < bias1 || w2 < bias2 {
					continue
				}
			} else {
				// CW winding - flip signs
				if w0 > -bias0 || w1 > -bias1 || w2 > -bias2 {
					continue
				}
				// Negate for consistent barycentric calculation
				w0 = -w0
				w1 = -w1
				w2 = -w2
			}

			// Compute barycentric coordinates
			b0 := w0 * invArea
			b1 := w1 * invArea
			b2 := w2 * invArea

			// For CW triangles, invArea is negative, so we need abs
			if area < 0 {
				b0 = -b0
				b1 = -b1
				b2 = -b2
			}

			// Perspective-correct interpolation for depth and attributes
			// We need to interpolate 1/w and then divide
			oneOverW := b0*tri.V0.W + b1*tri.V1.W + b2*tri.V2.W

			// Interpolate depth (already in screen space, so no perspective correction needed)
			// Actually, depth DOES need perspective correction
			var depth float32
			if oneOverW != 0 {
				depth = (b0*tri.V0.Z*tri.V0.W + b1*tri.V1.Z*tri.V1.W + b2*tri.V2.Z*tri.V2.W) / oneOverW
			} else {
				depth = b0*tri.V0.Z + b1*tri.V1.Z + b2*tri.V2.Z
			}

			// Interpolate attributes with perspective correction
			var attrs []float32
			if attrCount > 0 {
				attrs = make([]float32, attrCount)
				if oneOverW != 0 {
					for i := 0; i < attrCount; i++ {
						// Perspective-correct interpolation:
						// attr = (b0*attr0/w0 + b1*attr1/w1 + b2*attr2/w2) / (b0/w0 + b1/w1 + b2/w2)
						// Since W stores 1/w, this becomes:
						// attr = (b0*attr0*W0 + b1*attr1*W1 + b2*attr2*W2) / (b0*W0 + b1*W1 + b2*W2)
						attrs[i] = (b0*tri.V0.Attributes[i]*tri.V0.W +
							b1*tri.V1.Attributes[i]*tri.V1.W +
							b2*tri.V2.Attributes[i]*tri.V2.W) / oneOverW
					}
				} else {
					// No perspective correction (orthographic or W=1)
					for i := 0; i < attrCount; i++ {
						attrs[i] = b0*tri.V0.Attributes[i] + b1*tri.V1.Attributes[i] + b2*tri.V2.Attributes[i]
					}
				}
			}

			// Create fragment
			frag := Fragment{
				X:          x,
				Y:          y,
				Depth:      depth,
				Bary:       [3]float32{b0, b1, b2},
				Attributes: attrs,
			}

			callback(frag)
		}
	}
}

// ComputeTriangleArea returns the signed area of a triangle.
// Positive for CCW winding, negative for CW winding in screen space.
func ComputeTriangleArea(v0, v1, v2 ScreenVertex) float32 {
	e01 := NewEdgeFunction(v0.X, v0.Y, v1.X, v1.Y)
	return e01.Evaluate(v2.X, v2.Y)
}

// IsBackFacing returns true if the triangle is back-facing.
// This depends on the front face definition and the triangle's winding.
func IsBackFacing(tri Triangle, frontFace FrontFace) bool {
	area := ComputeTriangleArea(tri.V0, tri.V1, tri.V2)
	switch frontFace {
	case FrontFaceCCW:
		// CCW is front, so negative area (CW) means back-facing
		return area < 0
	case FrontFaceCW:
		// CW is front, so positive area (CCW) means back-facing
		return area > 0
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
