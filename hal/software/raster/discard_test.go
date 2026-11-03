package raster

import "testing"

// coveringTriangle covers a w x h target at depth z with 1/w = invW.
func coveringTriangle(w, h int, z, invW float32) Triangle {
	fw, fh := float32(w), float32(h)
	return Triangle{
		V0: ScreenVertex{X: 0, Y: 0, Z: z, W: invW},
		V1: ScreenVertex{X: 2 * fw, Y: 0, Z: z, W: invW},
		V2: ScreenVertex{X: 0, Y: 2 * fh, Z: z, W: invW},
	}
}

// TestFragmentShaderDiscardWritesNothing checks that a discarded fragment
// leaves color and depth untouched, also with depth test and depth write on:
// with mayDiscard the shader runs before the depth test, which would
// otherwise have written the depth already (P1-SW-016). It also checks the
// fragment position the shader sees: pixel center and 1/w.
func TestFragmentShaderDiscardWritesNothing(t *testing.T) {
	const w, h = 8, 2
	p := NewPipeline(w, h)
	p.SetViewport(Viewport{X: 0, Y: 0, Width: w, Height: h, MinDepth: 0, MaxDepth: 1})
	p.Clear(1, 0, 0, 1)
	p.ClearDepth(1)
	p.SetDepthTest(true, CompareLess)
	p.SetDepthWrite(true)

	var sawX []float32
	p.DrawTrianglesWithFragmentShader([]Triangle{coveringTriangle(w, h, 0.25, 0.5)}, func(frag *Fragment) ([4]float32, bool) {
		if frag.Y == 0 {
			sawX = append(sawX, float32(frag.X)+0.5)
		}
		if frag.InvW < 0.49 || frag.InvW > 0.51 {
			t.Errorf("fragment (%d,%d): InvW = %v, want 0.5", frag.X, frag.Y, frag.InvW)
		}
		return [4]float32{0, 1, 0, 1}, frag.X < 4 // discard the left half
	}, true)

	color := p.GetColorBuffer()
	depth := p.GetDepthBuffer()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := (y*w + x) * 4
			green := color[idx] < 128 && color[idx+1] > 128
			d := depth.Get(x, y)
			if x < 4 {
				if green || d != 1 {
					t.Errorf("discarded (%d,%d): green=%v depth=%v, want red and depth 1", x, y, green, d)
				}
			} else if !green || d > 0.26 || d < 0.24 {
				t.Errorf("kept (%d,%d): green=%v depth=%v, want green and depth 0.25", x, y, green, d)
			}
		}
	}
	if len(sawX) != w {
		t.Fatalf("row 0 shaded %d fragments, want %d", len(sawX), w)
	}
}
