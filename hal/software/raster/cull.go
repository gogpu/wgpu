package raster

import (
	"math"
)

// FrustumCull performs coarse frustum culling on a triangle in clip space.
// Returns true if the triangle is definitely outside the frustum and should be culled.
// This uses the bounding box of the triangle for a conservative test.
func FrustumCull(tri [3]ClipSpaceVertex) bool {
	// Use trivial reject test - if all vertices are outside any single plane
	return TriangleTrivialReject(tri)
}

// SmallTriangleCull returns true if the triangle is too small to render.
// This culls subpixel triangles that would produce no fragments.
// The triangle should be in screen space.
func SmallTriangleCull(tri Triangle, minArea float32) bool {
	area := ComputeScreenTriangleArea(tri)

	// Use absolute value since area can be negative for CW triangles
	if area < 0 {
		area = -area
	}

	return area < minArea
}

// DegenerateTriangleCull returns true if the triangle has zero or near-zero area.
// Works in clip space before perspective divide.
func DegenerateTriangleCull(tri [3]ClipSpaceVertex) bool {
	// Compute 2D area using x and y components (ignoring z and w)
	// This gives a rough estimate - actual screen area depends on w
	area := ComputeClipSpaceTriangleArea2D(tri)

	// Use a small epsilon for floating point comparison
	const epsilon = 1e-10
	return math.Abs(float64(area)) < epsilon
}

// ComputeScreenTriangleArea computes the signed area of a triangle in screen space.
// Positive values indicate CCW winding, negative values indicate CW winding.
// The actual area is half this value (this returns 2*area for efficiency).
func ComputeScreenTriangleArea(tri Triangle) float32 {
	// Use the cross product formula: (v1-v0) x (v2-v0)
	// Area = 0.5 * |cross product|, but we return 2*area for the signed value
	e1x := tri.V1.X - tri.V0.X
	e1y := tri.V1.Y - tri.V0.Y
	e2x := tri.V2.X - tri.V0.X
	e2y := tri.V2.Y - tri.V0.Y

	return e1x*e2y - e1y*e2x
}

// ComputeClipSpaceTriangleArea2D computes the signed 2D area of a triangle
// using only the x and y components of clip space coordinates.
// This is useful for quick degenerate triangle detection.
func ComputeClipSpaceTriangleArea2D(tri [3]ClipSpaceVertex) float32 {
	x0, y0 := tri[0].Position[0], tri[0].Position[1]
	x1, y1 := tri[1].Position[0], tri[1].Position[1]
	x2, y2 := tri[2].Position[0], tri[2].Position[1]

	e1x := x1 - x0
	e1y := y1 - y0
	e2x := x2 - x0
	e2y := y2 - y0

	return e1x*e2y - e1y*e2x
}

// ComputeClipSpaceTriangleAreaNDC computes the signed area in NDC space.
// This performs perspective divide first to get accurate area in normalized coordinates.
// Returns 0 if any vertex has w <= 0 (behind camera).
func ComputeClipSpaceTriangleAreaNDC(tri [3]ClipSpaceVertex) float32 {
	// Check for vertices behind the camera
	for i := 0; i < 3; i++ {
		if tri[i].Position[3] <= 0 {
			return 0
		}
	}

	// Perspective divide to get NDC coordinates
	x0 := tri[0].Position[0] / tri[0].Position[3]
	y0 := tri[0].Position[1] / tri[0].Position[3]
	x1 := tri[1].Position[0] / tri[1].Position[3]
	y1 := tri[1].Position[1] / tri[1].Position[3]
	x2 := tri[2].Position[0] / tri[2].Position[3]
	y2 := tri[2].Position[1] / tri[2].Position[3]

	e1x := x1 - x0
	e1y := y1 - y0
	e2x := x2 - x0
	e2y := y2 - y0

	return e1x*e2y - e1y*e2x
}

// IsBackFacingClipSpace returns true if the triangle is back-facing in clip space.
// Uses the NDC area to determine facing.
func IsBackFacingClipSpace(tri [3]ClipSpaceVertex, frontFace FrontFace) bool {
	area := ComputeClipSpaceTriangleAreaNDC(tri)

	switch frontFace {
	case FrontFaceCCW:
		// CCW is front, so negative area (CW in NDC) means back-facing
		return area < 0
	case FrontFaceCW:
		// CW is front, so positive area (CCW in NDC) means back-facing
		return area > 0
	}
	return false
}

// ShouldCullClipSpace returns true if the triangle should be culled in clip space.
func ShouldCullClipSpace(tri [3]ClipSpaceVertex, cullMode CullMode, frontFace FrontFace) bool {
	if cullMode == CullNone {
		return false
	}

	isBack := IsBackFacingClipSpace(tri, frontFace)

	switch cullMode {
	case CullBack:
		return isBack
	case CullFront:
		return !isBack
	}
	return false
}

// GuardBandCull performs guard-band culling for triangles.
// The guard band extends beyond the viewport to allow larger triangles
// to pass without clipping, which can improve performance.
// Returns true if the triangle is completely outside the guard band.
func GuardBandCull(tri [3]ClipSpaceVertex, guardBandX, guardBandY float32) bool {
	// Guard band test: |x| <= guardBandX * w and |y| <= guardBandY * w
	// If all vertices fail any single test, cull the triangle

	// Check left (-x > guardBandX * w for all vertices)
	allOutsideLeft := true
	for i := 0; i < 3; i++ {
		x, w := tri[i].Position[0], tri[i].Position[3]
		if x >= -guardBandX*w {
			allOutsideLeft = false
			break
		}
	}
	if allOutsideLeft {
		return true
	}

	// Check right (x > guardBandX * w for all vertices)
	allOutsideRight := true
	for i := 0; i < 3; i++ {
		x, w := tri[i].Position[0], tri[i].Position[3]
		if x <= guardBandX*w {
			allOutsideRight = false
			break
		}
	}
	if allOutsideRight {
		return true
	}

	// Check bottom (-y > guardBandY * w for all vertices)
	allOutsideBottom := true
	for i := 0; i < 3; i++ {
		y, w := tri[i].Position[1], tri[i].Position[3]
		if y >= -guardBandY*w {
			allOutsideBottom = false
			break
		}
	}
	if allOutsideBottom {
		return true
	}

	// Check top (y > guardBandY * w for all vertices)
	allOutsideTop := true
	for i := 0; i < 3; i++ {
		y, w := tri[i].Position[1], tri[i].Position[3]
		if y <= guardBandY*w {
			allOutsideTop = false
			break
		}
	}

	return allOutsideTop
}

// DefaultGuardBandX is the default guard band multiplier for X coordinates.
// A value of 2.0 means the guard band extends to 2x the viewport width.
const DefaultGuardBandX = 2.0

// DefaultGuardBandY is the default guard band multiplier for Y coordinates.
const DefaultGuardBandY = 2.0
