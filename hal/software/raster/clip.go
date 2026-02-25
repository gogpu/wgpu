package raster

// ClipPlane represents a clipping plane in homogeneous clip space.
// The plane equation is: A*x + B*y + C*z + D*w = 0
// Points with A*x + B*y + C*z + D*w >= 0 are considered inside.
type ClipPlane struct {
	A, B, C, D float32
}

// Standard frustum planes in clip space.
// After vertex shader, clip space coordinates satisfy:
// -w <= x <= w, -w <= y <= w, 0 <= z <= w (for depth [0,1] range)
var (
	// ClipPlaneNear clips against z >= 0 (near plane).
	// Equation: z >= 0, so z + 0*w >= 0, meaning (0, 0, 1, 0).
	ClipPlaneNear = ClipPlane{0, 0, 1, 0}

	// ClipPlaneFar clips against z <= w (far plane).
	// Equation: z <= w, so -z + w >= 0, meaning (0, 0, -1, 1).
	ClipPlaneFar = ClipPlane{0, 0, -1, 1}

	// ClipPlaneLeft clips against x >= -w (left plane).
	// Equation: x >= -w, so x + w >= 0, meaning (1, 0, 0, 1).
	ClipPlaneLeft = ClipPlane{1, 0, 0, 1}

	// ClipPlaneRight clips against x <= w (right plane).
	// Equation: x <= w, so -x + w >= 0, meaning (-1, 0, 0, 1).
	ClipPlaneRight = ClipPlane{-1, 0, 0, 1}

	// ClipPlaneBottom clips against y >= -w (bottom plane).
	// Equation: y >= -w, so y + w >= 0, meaning (0, 1, 0, 1).
	ClipPlaneBottom = ClipPlane{0, 1, 0, 1}

	// ClipPlaneTop clips against y <= w (top plane).
	// Equation: y <= w, so -y + w >= 0, meaning (0, -1, 0, 1).
	ClipPlaneTop = ClipPlane{0, -1, 0, 1}
)

// AllFrustumPlanes contains all 6 frustum clipping planes.
var AllFrustumPlanes = []ClipPlane{
	ClipPlaneNear,
	ClipPlaneFar,
	ClipPlaneLeft,
	ClipPlaneRight,
	ClipPlaneBottom,
	ClipPlaneTop,
}

// NearFarPlanes contains only near and far clipping planes.
var NearFarPlanes = []ClipPlane{
	ClipPlaneNear,
	ClipPlaneFar,
}

// Distance returns the signed distance from a vertex to the plane.
// Positive values indicate the vertex is inside (on the positive side).
// Negative values indicate the vertex is outside.
// Zero indicates the vertex is exactly on the plane.
func (p ClipPlane) Distance(v ClipSpaceVertex) float32 {
	return p.A*v.Position[0] + p.B*v.Position[1] + p.C*v.Position[2] + p.D*v.Position[3]
}

// IsInside returns true if the vertex is inside or on the plane.
func (p ClipPlane) IsInside(v ClipSpaceVertex) bool {
	return p.Distance(v) >= 0
}

// Intersect computes the intersection point between an edge and the plane.
// The edge goes from v0 to v1. Returns the interpolated vertex at the
// intersection and the parameter t in [0, 1] where intersection occurs.
// Assumes the edge actually crosses the plane (one vertex inside, one outside).
func (p ClipPlane) Intersect(v0, v1 ClipSpaceVertex) (ClipSpaceVertex, float32) {
	d0 := p.Distance(v0)
	d1 := p.Distance(v1)

	// Compute interpolation parameter
	// t = d0 / (d0 - d1) gives the point where the edge crosses the plane
	t := d0 / (d0 - d1)

	// Clamp t to valid range to handle numerical issues
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}

	// Interpolate position
	result := ClipSpaceVertex{
		Position: [4]float32{
			v0.Position[0] + t*(v1.Position[0]-v0.Position[0]),
			v0.Position[1] + t*(v1.Position[1]-v0.Position[1]),
			v0.Position[2] + t*(v1.Position[2]-v0.Position[2]),
			v0.Position[3] + t*(v1.Position[3]-v0.Position[3]),
		},
	}

	// Interpolate attributes if present
	if len(v0.Attributes) > 0 && len(v1.Attributes) > 0 {
		n := len(v0.Attributes)
		if len(v1.Attributes) < n {
			n = len(v1.Attributes)
		}
		result.Attributes = make([]float32, n)
		for i := 0; i < n; i++ {
			result.Attributes[i] = v0.Attributes[i] + t*(v1.Attributes[i]-v0.Attributes[i])
		}
	}

	return result, t
}

// ClipTriangleAgainstPlane clips a triangle against a single plane.
// Returns a slice of triangles (0, 1, or 2) after clipping.
//
// Cases:
//   - All 3 vertices inside: returns 1 triangle (original)
//   - All 3 vertices outside: returns 0 triangles
//   - 1 vertex inside: returns 1 triangle
//   - 2 vertices inside: returns 2 triangles (quad split)
func ClipTriangleAgainstPlane(tri [3]ClipSpaceVertex, plane ClipPlane) [][3]ClipSpaceVertex {
	// Compute distances and count vertices inside
	d := [3]float32{
		plane.Distance(tri[0]),
		plane.Distance(tri[1]),
		plane.Distance(tri[2]),
	}

	inside := [3]bool{d[0] >= 0, d[1] >= 0, d[2] >= 0}
	insideCount := 0
	for _, in := range inside {
		if in {
			insideCount++
		}
	}

	switch insideCount {
	case 0:
		// All outside - triangle is completely clipped
		return nil

	case 3:
		// All inside - return original triangle
		return [][3]ClipSpaceVertex{tri}

	case 1:
		// One vertex inside - produces one smaller triangle
		return clipOneInside(tri, inside, plane)

	case 2:
		// Two vertices inside - produces a quad (two triangles)
		return clipTwoInside(tri, inside, plane)
	}

	return nil
}

// clipOneInside handles the case where exactly one vertex is inside the plane.
// Returns one triangle formed by the inside vertex and two intersection points.
func clipOneInside(tri [3]ClipSpaceVertex, inside [3]bool, plane ClipPlane) [][3]ClipSpaceVertex {
	// Find the inside vertex index
	var insideIdx int
	for i, in := range inside {
		if in {
			insideIdx = i
			break
		}
	}

	// Get vertex indices in order (inside vertex first)
	i0 := insideIdx
	i1 := (insideIdx + 1) % 3
	i2 := (insideIdx + 2) % 3

	// Compute intersection points
	intersect1, _ := plane.Intersect(tri[i0], tri[i1])
	intersect2, _ := plane.Intersect(tri[i0], tri[i2])

	// New triangle: inside vertex + two intersection points
	return [][3]ClipSpaceVertex{
		{tri[i0], intersect1, intersect2},
	}
}

// clipTwoInside handles the case where exactly two vertices are inside the plane.
// Returns two triangles forming a quad.
func clipTwoInside(tri [3]ClipSpaceVertex, inside [3]bool, plane ClipPlane) [][3]ClipSpaceVertex {
	// Find the outside vertex index
	var outsideIdx int
	for i, in := range inside {
		if !in {
			outsideIdx = i
			break
		}
	}

	// Get vertex indices with outside vertex first
	i0 := outsideIdx           // Outside
	i1 := (outsideIdx + 1) % 3 // Inside
	i2 := (outsideIdx + 2) % 3 // Inside

	// Compute intersection points
	// intersect1: on edge from i0 to i1
	// intersect2: on edge from i0 to i2
	intersect1, _ := plane.Intersect(tri[i1], tri[i0])
	intersect2, _ := plane.Intersect(tri[i2], tri[i0])

	// Form two triangles from the quad (i1, intersect1, intersect2, i2)
	// Triangle 1: i1, intersect1, i2
	// Triangle 2: intersect1, intersect2, i2
	return [][3]ClipSpaceVertex{
		{tri[i1], intersect1, tri[i2]},
		{intersect1, intersect2, tri[i2]},
	}
}

// ClipTriangle clips a triangle against all 6 frustum planes.
// Returns a list of clipped triangles (may be 0 to many).
func ClipTriangle(tri [3]ClipSpaceVertex) [][3]ClipSpaceVertex {
	return ClipTriangleAgainstPlanes(tri, AllFrustumPlanes)
}

// ClipTriangleNearFar clips a triangle against only the near and far planes.
// This is faster than full frustum clipping and is sufficient for many cases
// where triangles are known to be within the X/Y bounds.
func ClipTriangleNearFar(tri [3]ClipSpaceVertex) [][3]ClipSpaceVertex {
	return ClipTriangleAgainstPlanes(tri, NearFarPlanes)
}

// ClipTriangleAgainstPlanes clips a triangle against the specified planes.
func ClipTriangleAgainstPlanes(tri [3]ClipSpaceVertex, planes []ClipPlane) [][3]ClipSpaceVertex {
	// Start with the input triangle
	triangles := [][3]ClipSpaceVertex{tri}

	// Clip against each plane
	for _, plane := range planes {
		if len(triangles) == 0 {
			return nil
		}

		var clipped [][3]ClipSpaceVertex
		for _, t := range triangles {
			result := ClipTriangleAgainstPlane(t, plane)
			clipped = append(clipped, result...)
		}
		triangles = clipped
	}

	return triangles
}

// IsCompletelyOutside returns true if all vertices are outside any single plane.
// This is a fast rejection test.
func IsCompletelyOutside(tri [3]ClipSpaceVertex) bool {
	for _, plane := range AllFrustumPlanes {
		allOutside := true
		for i := 0; i < 3; i++ {
			if plane.Distance(tri[i]) >= 0 {
				allOutside = false
				break
			}
		}
		if allOutside {
			return true
		}
	}
	return false
}

// IsCompletelyInside returns true if all vertices are inside all planes.
// This means no clipping is needed.
func IsCompletelyInside(tri [3]ClipSpaceVertex) bool {
	for _, plane := range AllFrustumPlanes {
		for i := 0; i < 3; i++ {
			if plane.Distance(tri[i]) < 0 {
				return false
			}
		}
	}
	return true
}

// OutcodeVertex computes the outcode for a vertex.
// Each bit indicates which plane the vertex is outside of.
// 0 means the vertex is inside all planes.
type Outcode uint8

const (
	// OutcodeNear indicates the vertex is outside the near plane.
	OutcodeNear Outcode = 1 << iota
	// OutcodeFar indicates the vertex is outside the far plane.
	OutcodeFar
	// OutcodeLeft indicates the vertex is outside the left plane.
	OutcodeLeft
	// OutcodeRight indicates the vertex is outside the right plane.
	OutcodeRight
	// OutcodeBottom indicates the vertex is outside the bottom plane.
	OutcodeBottom
	// OutcodeTop indicates the vertex is outside the top plane.
	OutcodeTop
)

// ComputeOutcode computes the outcode for a clip space vertex.
func ComputeOutcode(v ClipSpaceVertex) Outcode {
	var code Outcode

	x, y, z, w := v.Position[0], v.Position[1], v.Position[2], v.Position[3]

	if z < 0 {
		code |= OutcodeNear
	}
	if z > w {
		code |= OutcodeFar
	}
	if x < -w {
		code |= OutcodeLeft
	}
	if x > w {
		code |= OutcodeRight
	}
	if y < -w {
		code |= OutcodeBottom
	}
	if y > w {
		code |= OutcodeTop
	}

	return code
}

// TriangleTrivialReject returns true if the triangle can be trivially rejected.
// Uses Cohen-Sutherland style outcode testing for fast rejection.
func TriangleTrivialReject(tri [3]ClipSpaceVertex) bool {
	o0 := ComputeOutcode(tri[0])
	o1 := ComputeOutcode(tri[1])
	o2 := ComputeOutcode(tri[2])

	// If all vertices share a common outside region, trivially reject
	return (o0 & o1 & o2) != 0
}

// TriangleTrivialAccept returns true if the triangle can be trivially accepted.
// All vertices must be inside the frustum.
func TriangleTrivialAccept(tri [3]ClipSpaceVertex) bool {
	o0 := ComputeOutcode(tri[0])
	o1 := ComputeOutcode(tri[1])
	o2 := ComputeOutcode(tri[2])

	// If all vertices have zero outcode, they're all inside
	return (o0 | o1 | o2) == 0
}

// ClipTriangleFast clips a triangle with optimized fast paths.
// Uses trivial accept/reject tests before full clipping.
func ClipTriangleFast(tri [3]ClipSpaceVertex) [][3]ClipSpaceVertex {
	// Fast rejection test
	if TriangleTrivialReject(tri) {
		return nil
	}

	// Fast accept test
	if TriangleTrivialAccept(tri) {
		return [][3]ClipSpaceVertex{tri}
	}

	// Full clipping required
	return ClipTriangle(tri)
}
