//go:build !(js && wasm)

package raster

import "testing"

// TestViewportTransformProject checks the WebGPU viewport mapping of NDC to
// framebuffer coordinates and depth, including the perspective divide.
func TestViewportTransformProject(t *testing.T) {
	vt := ViewportTransform{X: 2, Y: 1, Width: 4, Height: 6, MinDepth: 0.25, MaxDepth: 0.75}
	tests := []struct {
		name    string
		pos     [4]float32
		x, y, z float32
	}{
		{"top-left corner", [4]float32{-1, 1, 0, 1}, 2, 1, 0.25},
		{"bottom-right corner", [4]float32{1, -1, 1, 1}, 6, 7, 0.75},
		{"center", [4]float32{0, 0, 0.5, 1}, 4, 4, 0.5},
		{"divided by w", [4]float32{2, -2, 1, 2}, 6, 7, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := []float32{1, 2}
			sv := vt.Project(&ClipSpaceVertex{Position: tt.pos, Attributes: attrs})
			if sv.X != tt.x || sv.Y != tt.y || sv.Z != tt.z {
				t.Errorf("Project(%v) = (%g, %g, %g), want (%g, %g, %g)", tt.pos, sv.X, sv.Y, sv.Z, tt.x, tt.y, tt.z)
			}
			if want := 1 / tt.pos[3]; sv.W != want {
				t.Errorf("Project(%v).W = %g, want 1/w = %g", tt.pos, sv.W, want)
			}
			if &sv.Attributes[0] != &attrs[0] {
				t.Error("Project copied the attributes; it should share them")
			}
		})
	}
}

// TestViewportTransformPixelBounds checks which pixels a viewport covers:
// those whose centers lie in it, clamped to the target.
func TestViewportTransformPixelBounds(t *testing.T) {
	tests := []struct {
		name string
		vt   ViewportTransform
		want Viewport
	}{
		{"full", FullViewport(8, 8), Viewport{X: 0, Y: 0, Width: 8, Height: 8, MaxDepth: 1}},
		{"offset", ViewportTransform{X: 2, Y: 1, Width: 4, Height: 6, MinDepth: 0.25, MaxDepth: 0.75},
			Viewport{X: 2, Y: 1, Width: 4, Height: 6, MinDepth: 0.25, MaxDepth: 0.75}},
		{"past the target", ViewportTransform{X: -2, Y: 3, Width: 8, Height: 8, MaxDepth: 0.5},
			Viewport{X: 0, Y: 3, Width: 6, Height: 5, MaxDepth: 0.5}},
		// Centers 1.5 and 2.5 lie in [1.5, 3.4); 3.5 does not.
		{"fractional", ViewportTransform{X: 1.5, Y: 0.6, Width: 1.9, Height: 1}, Viewport{X: 1, Y: 1, Width: 2, Height: 1}},
		{"outside", ViewportTransform{X: 10, Y: 10, Width: 4, Height: 4}, Viewport{X: 8, Y: 8}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.vt.PixelBounds(8, 8); got != tt.want {
				t.Errorf("PixelBounds = %+v, want %+v", got, tt.want)
			}
		})
	}
}
