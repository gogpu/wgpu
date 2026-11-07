//go:build !(js && wasm)

package raster

// QuadLanes is the number of fragments in a quad: lane i is the pixel
// (X + i&1, Y + i>>1) of the quad.
const QuadLanes = 4

// Quad is a 2x2 block of fragments whose top-left pixel (X, Y) has even
// coordinates: the unit a GPU runs fragment shaders in, so that derivatives
// are differences between neighboring fragments (Vulkan "Derivative
// Operations", WGSL "Derivative Built-in Functions").
type Quad struct {
	// X and Y are the top-left pixel of the quad, both even.
	X, Y int

	// Covered marks the lanes whose pixel center the triangle covers and
	// that lie in the rasterized bounds: bit i for lane i.
	Covered uint8

	// Frags holds every lane's fragment, the uncovered lanes included: a
	// lane outside the triangle has the barycentrics, depth and attributes
	// the triangle's planes extrapolate to at its pixel center, which is
	// what a helper invocation sees on a GPU. Their Attributes slices are
	// reused by the next quad.
	Frags [QuadLanes]Fragment
}

// QuadCallback receives each quad that has a covered lane. The quad and its
// fragments' attribute slices are reused after the callback returns.
type QuadCallback func(q *Quad)

// QuadRasterizer rasterizes triangles in quads. It keeps one quad and its
// attribute storage, so a draw that reuses one QuadRasterizer for all its
// triangles allocates nothing per fragment. The zero value is ready to use;
// it is not safe for concurrent use.
type QuadRasterizer struct {
	quad  Quad
	attrs []float32
}

// Rasterize calls callback for every quad with a pixel of the viewport that
// tri covers, under the same top-left fill rule as Rasterize, so a pixel is
// covered in exactly the quads Rasterize would produce it in.
func (r *QuadRasterizer) Rasterize(tri *Triangle, viewport Viewport, callback QuadCallback) {
	it := NewIncrementalTriangle(*tri)
	if it.IsDegenerate() {
		return
	}
	startX := maxInt(viewport.X, it.minX)
	endX := minInt(viewport.X+viewport.Width, it.maxX)
	startY := maxInt(viewport.Y, it.minY)
	endY := minInt(viewport.Y+viewport.Height, it.maxY)
	if startX >= endX || startY >= endY {
		return
	}

	n := len(tri.V0.Attributes)
	if cap(r.attrs) < QuadLanes*n {
		r.attrs = make([]float32, QuadLanes*n)
	}
	attrs := r.attrs[:QuadLanes*n]
	q := &r.quad

	// Clearing the low bit rounds down to even, negative values included,
	// so the quads stay aligned to even framebuffer coordinates.
	x0, y0 := startX&^1, startY&^1
	for qy := y0; qy < endY; qy += 2 {
		it.setRowPixel(x0, qy)
		for qx := x0; qx < endX; qx += 2 {
			covered := it.quadCoverage() & laneBounds(qx, qy, startX, endX, startY, endY)
			if covered != 0 {
				q.X, q.Y, q.Covered = qx, qy, covered
				for lane := range QuadLanes {
					var a []float32
					if n > 0 {
						a = attrs[lane*n : (lane+1)*n : (lane+1)*n]
					}
					it.laneFragment(&q.Frags[lane], tri, qx, qy, lane, a)
				}
				callback(q)
			}
			it.StepX()
			it.StepX()
		}
	}
}

// laneBounds marks the lanes of the quad at (qx, qy) whose pixel lies in
// [startX, endX) x [startY, endY).
func laneBounds(qx, qy, startX, endX, startY, endY int) uint8 {
	var m uint8
	for lane := range QuadLanes {
		x, y := qx+lane&1, qy+lane>>1
		if x >= startX && x < endX && y >= startY && y < endY {
			m |= 1 << lane
		}
	}
	return m
}

// laneEdges returns the edge values (w0 of E12, w1 of E20, w2 of E01) at the
// pixel center of lane of the quad whose top-left pixel the edges are at.
// Exact: one step right or down adds A or B.
func (it *IncrementalTriangle) laneEdges(lane int) (w0, w1, w2 int64) {
	dx, dy := int64(lane&1), int64(lane>>1)
	w0 = it.E12.current + dx*it.E12.A + dy*it.E12.B
	w1 = it.E20.current + dx*it.E20.A + dy*it.E20.B
	w2 = it.E01.current + dx*it.E01.A + dy*it.E01.B
	return w0, w1, w2
}

// quadCoverage marks the lanes of the quad whose top-left pixel the edges
// are at that the triangle covers (see IsInside).
func (it *IncrementalTriangle) quadCoverage() uint8 {
	var m uint8
	for lane := range QuadLanes {
		w0, w1, w2 := it.laneEdges(lane)
		if it.E12.covers(w0) && it.E20.covers(w1) && it.E01.covers(w2) {
			m |= 1 << lane
		}
	}
	return m
}

// laneFragment fills frag with lane of the quad at (qx, qy), whose top-left
// pixel the edges are at, interpolating the attributes into attrs.
func (it *IncrementalTriangle) laneFragment(frag *Fragment, tri *Triangle, qx, qy, lane int, attrs []float32) {
	w0, w1, w2 := it.laneEdges(lane)
	b0 := float32(float64(w0) * it.invArea)
	b1 := float32(float64(w1) * it.invArea)
	b2 := float32(float64(w2) * it.invArea)
	oneOverW := b0*tri.V0.W + b1*tri.V1.W + b2*tri.V2.W
	frag.X, frag.Y = qx+lane&1, qy+lane>>1
	frag.Depth = interpolateDepth(b0, b1, b2, tri, oneOverW)
	frag.InvW = oneOverW
	frag.Bary = [3]float32{b0, b1, b2}
	frag.Attributes = attrs
	if attrs != nil {
		fillAttributes(attrs, b0, b1, b2, tri, oneOverW)
	}
}
