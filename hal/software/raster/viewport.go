//go:build !(js && wasm)

package raster

import "math"

// ViewportTransform maps clip-space positions to framebuffer coordinates
// (WebGPU spec, "Rasterization"):
//
//	ndc         = position.xyz / position.w
//	framebuffer = (X + 0.5*(ndc.x+1)*Width, Y + 0.5*(1-ndc.y)*Height)
//	depth       = MinDepth + ndc.z*(MaxDepth-MinDepth)
//
// X, Y, Width and Height are the GPURenderPassEncoder viewport in pixels and
// may be fractional or extend past the render target.
type ViewportTransform struct {
	X, Y, Width, Height float32
	MinDepth, MaxDepth  float32
}

// FullViewport is the transform of the default viewport: the whole width x
// height target with depth range 0..1.
func FullViewport(width, height int) ViewportTransform {
	return ViewportTransform{Width: float32(width), Height: float32(height), MaxDepth: 1}
}

// Project divides v's position by its w and maps it through the viewport,
// keeping 1/w for perspective-correct interpolation. v must have w > 0, as
// every vertex a Clipper returns has. The attributes are shared with v, not
// copied.
func (vt *ViewportTransform) Project(v *ClipSpaceVertex) ScreenVertex {
	inv := 1 / v.Position[3]
	nx, ny, nz := v.Position[0]*inv, v.Position[1]*inv, v.Position[2]*inv
	return ScreenVertex{
		X:          vt.X + 0.5*(nx+1)*vt.Width,
		Y:          vt.Y + 0.5*(1-ny)*vt.Height,
		Z:          vt.MinDepth + nz*(vt.MaxDepth-vt.MinDepth),
		W:          inv,
		Attributes: v.Attributes,
	}
}

// PixelBounds returns the pixels of a width x height target whose centers
// lie in the viewport rectangle [X, X+Width) x [Y, Y+Height), with the
// viewport's depth range. Primitives are rasterized only there: the clip
// volume ends at the viewport's edges, and a center exactly on the left or
// top edge is inside under the top-left rule, one on the right or bottom
// edge is not.
func (vt *ViewportTransform) PixelBounds(width, height int) Viewport {
	x0 := pixelEdge(vt.X, width)
	x1 := pixelEdge(vt.X+vt.Width, width)
	y0 := pixelEdge(vt.Y, height)
	y1 := pixelEdge(vt.Y+vt.Height, height)
	return Viewport{
		X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0,
		MinDepth: vt.MinDepth, MaxDepth: vt.MaxDepth,
	}
}

// pixelEdge returns the first pixel whose center c = i + 0.5 satisfies
// c >= edge, clamped to [0, size].
func pixelEdge(edge float32, size int) int {
	i := math.Ceil(float64(edge) - 0.5)
	if i < 0 {
		return 0
	}
	if i > float64(size) {
		return size
	}
	return int(i)
}
