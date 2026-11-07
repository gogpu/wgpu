//go:build !(js && wasm)

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

const (
	// clipGuardBand is how far past the render target's edges, in pixels,
	// the x and y clip planes lie. Primitives are clipped in x and y only to
	// keep their window coordinates in the range the rasterizer snaps to
	// (±2^21 pixels, incremental.go) and precise in float32 (a coordinate
	// under 2^15 keeps better than 1/256 of a pixel): the viewport and the
	// scissor bound the pixels that are drawn, so a primitive that crosses
	// the viewport's edge needs no new vertices there.
	clipGuardBand = 8192

	// minClipW is the least w a clipped vertex keeps. The near and far
	// planes (0 <= z <= w) already remove w < 0, but leave w = 0 on their
	// boundary, which has no window position.
	minClipW = 0x1p-40

	// clipPlaneCount is the number of planes a Clipper clips against: w,
	// near, far, and the four guard band planes.
	clipPlaneCount = 7

	// maxClipVertices bounds a clipped polygon. A convex polygon gains at
	// most one vertex per plane; the rest is room for rounding that makes a
	// nearly degenerate polygon cross a plane twice more.
	maxClipVertices = 3 + 2*clipPlaneCount

	// clipArenaChunk is the number of float32s the attribute arena
	// allocates at once for the vertices clipping creates.
	clipArenaChunk = 4096
)

// clipPlane is a plane of a Clipper: it keeps the points whose ClipPlane
// distance is at least min.
type clipPlane struct {
	ClipPlane
	min float32
}

func (p *clipPlane) distance(v *ClipSpaceVertex) float32 {
	return p.A*v.Position[0] + p.B*v.Position[1] + p.C*v.Position[2] + p.D*v.Position[3] - p.min
}

// Clipper clips triangles in clip space, before the perspective divide
// (WebGPU spec, "Primitive Clipping"): against 0 <= z <= w, against w > 0,
// and against x and y planes a guard band past the render target. Clipping
// in clip space is what keeps a vertex behind the eye (w < 0), whose divided
// position is mirrored, or one far outside the viewport, beyond the range
// the rasterizer can snap to, from changing which pixels a triangle covers.
//
// The vertices clipping creates interpolate the position and every
// attribute linearly in clip space, which is perspective-correct once they
// are divided by w; flat attributes are copied from the provoking vertex.
// Their attributes live in an arena owned by the Clipper and stay valid as
// long as the triangles that use them; a triangle that needs no clipping
// keeps its vertices' attribute slices and costs no allocation.
//
// A Clipper is not safe for concurrent use.
type Clipper struct {
	planes [clipPlaneCount]clipPlane
	// reject is set for a viewport without area: it covers no pixel.
	reject bool
	// flat marks the attribute lanes copied from the provoking vertex, as
	// Triangle.Flat.
	flat uint64
	// prov is the provoking vertex of the triangle being clipped.
	prov  *ClipSpaceVertex
	poly  [2][maxClipVertices]ClipSpaceVertex
	dist  [maxClipVertices]float32
	arena []float32
}

// NewClipper returns a Clipper for primitives that vt maps onto a width x
// height render target, with the attribute lanes in flat copied from the
// provoking vertex.
func NewClipper(vt *ViewportTransform, width, height int, flat uint64) Clipper {
	c := Clipper{flat: flat}
	hx, hy := vt.Width/2, vt.Height/2
	if !(hx > 0 && hy > 0) {
		c.reject = true
		return c
	}
	// Window x = cx + hx*ndc.x and y = cy - hy*ndc.y; the guard band is the
	// window rectangle [-g, width+g] x [-g, height+g] in NDC.
	cx, cy := vt.X+hx, vt.Y+hy
	g := float32(clipGuardBand)
	xMin, xMax := (-g-cx)/hx, (float32(width)+g-cx)/hx
	yMin, yMax := (cy-float32(height)-g)/hy, (cy+g)/hy
	c.planes = [clipPlaneCount]clipPlane{
		{ClipPlane{0, 0, 0, 1}, minClipW},
		{ClipPlaneNear, 0},
		{ClipPlaneFar, 0},
		{ClipPlane{1, 0, 0, -xMin}, 0}, // x >= xMin*w
		{ClipPlane{-1, 0, 0, xMax}, 0}, // x <= xMax*w
		{ClipPlane{0, 1, 0, -yMin}, 0}, // y >= yMin*w
		{ClipPlane{0, -1, 0, yMax}, 0}, // y <= yMax*w
	}
	return c
}

// outcode returns the planes v lies outside of, bit i for plane i.
func (c *Clipper) outcode(v *ClipSpaceVertex) uint8 {
	var code uint8
	for i := range c.planes {
		// !(d >= 0) puts a NaN position outside every plane.
		if d := c.planes[i].distance(v); !(d >= 0) {
			code |= 1 << i
		}
	}
	return code
}

// Clip clips the triangle v0, v1, v2, whose provoking vertex is v0, and
// returns the visible part as a convex polygon in the triangle's winding,
// empty when nothing is visible. Its first vertex carries the provoking
// vertex's flat attributes: it is v0, or a vertex clipping created. The
// polygon is valid until the next call.
func (c *Clipper) Clip(v0, v1, v2 *ClipSpaceVertex) []ClipSpaceVertex {
	if c.reject {
		return nil
	}
	o0, o1, o2 := c.outcode(v0), c.outcode(v1), c.outcode(v2)
	if o0&o1&o2 != 0 {
		return nil // all three outside one plane
	}
	poly := c.poly[0][:3]
	poly[0], poly[1], poly[2] = *v0, *v1, *v2
	cut := o0 | o1 | o2
	if cut == 0 {
		return poly
	}
	c.prov = v0
	dst := 1 // the polygon alternates between the two buffers
	for i := range c.planes {
		if cut&(1<<i) == 0 {
			continue // every vertex is inside: the plane cuts nothing
		}
		poly = c.clipPolygon(poly, &c.planes[i], c.poly[dst][:0])
		if len(poly) < 3 {
			return nil
		}
		dst ^= 1
	}
	return poly
}

// clipPolygon clips poly against p into dst (Sutherland-Hodgman). The first
// vertex of the result is poly's first vertex when it is inside, otherwise
// the vertex created where the polygon first enters the plane, so the
// provoking vertex's flat attributes stay at index 0.
func (c *Clipper) clipPolygon(poly []ClipSpaceVertex, p *clipPlane, dst []ClipSpaceVertex) []ClipSpaceVertex {
	n := len(poly)
	for i := range n {
		c.dist[i] = p.distance(&poly[i])
	}
	for i := range n {
		j := i + 1
		if j == n {
			j = 0
		}
		di, dj := c.dist[i], c.dist[j]
		inI, inJ := di >= 0, dj >= 0
		if inI && len(dst) < maxClipVertices {
			dst = append(dst, poly[i])
		}
		if inI != inJ && len(dst) < maxClipVertices {
			// The new vertex is computed from the inside vertex, so the
			// shared edge of two triangles is cut at the same point whichever
			// way they wind.
			if inI {
				dst = append(dst, c.intersect(&poly[i], &poly[j], di, dj))
			} else {
				dst = append(dst, c.intersect(&poly[j], &poly[i], dj, di))
			}
		}
	}
	return dst
}

// intersect returns the point where the edge from in (inside the plane, at
// distance dIn) to out (outside, at dOut) crosses the plane.
func (c *Clipper) intersect(in, out *ClipSpaceVertex, dIn, dOut float32) ClipSpaceVertex {
	t := dIn / (dIn - dOut)
	var v ClipSpaceVertex
	for i := range v.Position {
		v.Position[i] = in.Position[i] + t*(out.Position[i]-in.Position[i])
	}
	n := min(len(in.Attributes), len(out.Attributes), len(c.prov.Attributes))
	if n == 0 {
		return v
	}
	attrs := c.alloc(n)
	for i := range attrs {
		if i < 64 && c.flat&(1<<i) != 0 {
			// A flat lane holds a bit pattern: copied, never interpolated.
			attrs[i] = c.prov.Attributes[i]
			continue
		}
		attrs[i] = in.Attributes[i] + t*(out.Attributes[i]-in.Attributes[i])
	}
	v.Attributes = attrs
	return v
}

// alloc returns n float32s from the arena. A full chunk is left to the
// triangles that use it, never reused.
func (c *Clipper) alloc(n int) []float32 {
	if len(c.arena) < n {
		c.arena = make([]float32, max(clipArenaChunk, n))
	}
	s := c.arena[:n:n]
	c.arena = c.arena[n:]
	return s
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
