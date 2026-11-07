//go:build !(js && wasm)

// Package raster provides CPU-based triangle rasterization for the software backend.
//
// The raster package implements the core rendering pipeline using the Edge Function
// algorithm (Pineda, 1988). This approach is chosen for its simplicity, correctness,
// and potential for parallelization.
//
// # Algorithm Overview
//
// Triangle rasterization uses three edge functions to determine if a pixel is inside
// a triangle. For each candidate pixel, we evaluate:
//
//	E01(x,y) = (y0-y1)*x + (x1-x0)*y + (x0*y1 - x1*y0)
//	E12(x,y) = (y1-y2)*x + (x2-x1)*y + (x1*y2 - x2*y1)
//	E20(x,y) = (y2-y0)*x + (x0-x2)*y + (x2*y0 - x0*y2)
//
// Pixels are sampled at their centers (x+0.5, y+0.5). The edges are oriented so
// that the interior is positive whatever the triangle's winding.
//
// # Fixed Point
//
// Vertex positions are snapped to fixed point with SubPixelBits (8) fractional
// bits, rounding to nearest even, as D3D11+ hardware does (16.8). The edge
// functions are then evaluated exactly in int64 and stepped from pixel to pixel
// by addition, without drift. A triangle with a non-finite vertex or one beyond
// ±2^21 pixels covers nothing: Clipper keeps every primitive the software
// backend draws inside a guard band well within that range.
//
// # Clipping
//
// Primitives are clipped in clip space, before the perspective divide
// (Clipper): against 0 <= z <= w, against w > 0, and against x and y planes a
// guard band past the render target. The viewport and the scissor bound the
// pixels a primitive covers, so it needs no clipping at the viewport's edges.
//
// # Fill Rule
//
// Coverage follows the top-left rule of the D3D11/D3D12 rasterization rules,
// which Vulkan and WebGPU implementations follow: a pixel center is covered if
// it lies strictly inside all three edges, or exactly on an edge that is a top
// edge (exactly horizontal, with the triangle below it) or a left edge (not
// horizontal, with the triangle to its right). For an edge shared by two
// triangles the exact edge values are negations of each other, so in any
// tiling every pixel center is covered by exactly one triangle: shared edges
// and shared vertices are drawn once, with either winding, on every path
// (Rasterize, RasterizeTile and the tile test used for binning).
//
// # Depth Testing
//
// The depth buffer stores float32 values in the range [0, 1], where 0 is the near
// plane and 1 is the far plane. Depth testing compares interpolated fragment depth
// against the stored value using the configured compare function.
//
// # Usage
//
//	pipeline := raster.NewPipeline(800, 600)
//	pipeline.Clear(0.1, 0.1, 0.1, 1.0)
//	pipeline.ClearDepth(1.0)
//	pipeline.SetDepthTest(true, raster.CompareLess)
//	pipeline.DrawTriangles(triangles, [4]float32{1, 0, 0, 1}) // Red triangles
//	pixels := pipeline.GetColorBuffer() // RGBA8 data
package raster
