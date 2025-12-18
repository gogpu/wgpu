//go:build software

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
// A pixel is inside the triangle if all three edge functions are non-negative
// (for counter-clockwise winding).
//
// # Fill Rule
//
// The rasterizer implements the top-left fill rule to avoid double-drawing pixels
// on shared triangle edges. An edge is "top" if horizontal and above the triangle,
// or "left" if going up.
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
