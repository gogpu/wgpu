//go:build software

package raster

import (
	"sync"
)

// Pipeline is a basic software rendering pipeline.
// It manages the color buffer, depth buffer, and rendering state.
type Pipeline struct {
	// Viewport configuration
	viewport Viewport

	// Depth testing configuration
	depthTest    bool
	depthWrite   bool
	depthCompare CompareFunc

	// Face culling configuration
	cullMode  CullMode
	frontFace FrontFace

	// Blending configuration
	blendState BlendState

	// Buffers
	colorBuffer []byte // RGBA8 format (4 bytes per pixel)
	depthBuffer *DepthBuffer
	width       int
	height      int

	// Thread safety
	mu sync.Mutex
}

// NewPipeline creates a new rendering pipeline with the given dimensions.
// The color buffer is initialized to black, and depth buffer to 1.0 (far).
func NewPipeline(width, height int) *Pipeline {
	size := width * height * 4 // RGBA8

	return &Pipeline{
		viewport: Viewport{
			X:        0,
			Y:        0,
			Width:    width,
			Height:   height,
			MinDepth: 0.0,
			MaxDepth: 1.0,
		},
		depthTest:    false,
		depthWrite:   true,
		depthCompare: CompareLess,
		cullMode:     CullNone,
		frontFace:    FrontFaceCCW,
		blendState:   BlendDisabled,
		colorBuffer:  make([]byte, size),
		depthBuffer:  NewDepthBuffer(width, height),
		width:        width,
		height:       height,
	}
}

// Width returns the framebuffer width.
func (p *Pipeline) Width() int {
	return p.width
}

// Height returns the framebuffer height.
func (p *Pipeline) Height() int {
	return p.height
}

// Clear fills the color buffer with the specified RGBA values.
// Color components are in the range [0, 1].
func (p *Pipeline) Clear(r, g, b, a float32) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Convert to bytes
	rb := clampByte(r * 255)
	gb := clampByte(g * 255)
	bb := clampByte(b * 255)
	ab := clampByte(a * 255)

	for i := 0; i < len(p.colorBuffer); i += 4 {
		p.colorBuffer[i+0] = rb
		p.colorBuffer[i+1] = gb
		p.colorBuffer[i+2] = bb
		p.colorBuffer[i+3] = ab
	}
}

// ClearDepth fills the depth buffer with the specified value.
// Typically use 1.0 (far plane) to reset the depth buffer.
func (p *Pipeline) ClearDepth(value float32) {
	p.depthBuffer.Clear(value)
}

// SetViewport sets the rendering viewport.
func (p *Pipeline) SetViewport(v Viewport) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.viewport = v
}

// GetViewport returns the current viewport.
func (p *Pipeline) GetViewport() Viewport {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.viewport
}

// SetDepthTest enables or disables depth testing and sets the compare function.
func (p *Pipeline) SetDepthTest(enabled bool, compare CompareFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.depthTest = enabled
	p.depthCompare = compare
}

// SetDepthWrite enables or disables writing to the depth buffer.
func (p *Pipeline) SetDepthWrite(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.depthWrite = enabled
}

// SetCullMode sets the face culling mode.
func (p *Pipeline) SetCullMode(mode CullMode) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cullMode = mode
}

// SetFrontFace sets which winding order is considered front-facing.
func (p *Pipeline) SetFrontFace(face FrontFace) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.frontFace = face
}

// SetBlendState sets the blending configuration.
func (p *Pipeline) SetBlendState(state BlendState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.blendState = state
}

// GetBlendState returns the current blend state.
func (p *Pipeline) GetBlendState() BlendState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.blendState
}

// DrawTriangles rasterizes the given triangles with a solid color.
// Color is in RGBA format with values in [0, 1].
func (p *Pipeline) DrawTriangles(triangles []Triangle, color [4]float32) {
	p.mu.Lock()
	viewport := p.viewport
	depthTest := p.depthTest
	depthWrite := p.depthWrite
	depthCompare := p.depthCompare
	cullMode := p.cullMode
	frontFace := p.frontFace
	blendState := p.blendState
	p.mu.Unlock()

	for i := range triangles {
		tri := &triangles[i]

		// Face culling
		if ShouldCull(*tri, cullMode, frontFace) {
			continue
		}

		// Rasterize triangle
		Rasterize(*tri, viewport, func(frag Fragment) {
			// Bounds check
			if frag.X < 0 || frag.X >= p.width || frag.Y < 0 || frag.Y >= p.height {
				return
			}

			// Depth test
			if depthTest {
				if !p.depthBuffer.TestAndSet(frag.X, frag.Y, frag.Depth, depthCompare, depthWrite) {
					return
				}
			} else if depthWrite {
				p.depthBuffer.Set(frag.X, frag.Y, frag.Depth)
			}

			// Apply blending if enabled
			idx := (frag.Y*p.width + frag.X) * 4
			p.mu.Lock()
			if blendState.Enabled {
				r, g, b, a := BlendFloatToByte(color,
					p.colorBuffer[idx+0], p.colorBuffer[idx+1],
					p.colorBuffer[idx+2], p.colorBuffer[idx+3],
					blendState)
				p.colorBuffer[idx+0] = r
				p.colorBuffer[idx+1] = g
				p.colorBuffer[idx+2] = b
				p.colorBuffer[idx+3] = a
			} else {
				p.colorBuffer[idx+0] = clampByte(color[0] * 255)
				p.colorBuffer[idx+1] = clampByte(color[1] * 255)
				p.colorBuffer[idx+2] = clampByte(color[2] * 255)
				p.colorBuffer[idx+3] = clampByte(color[3] * 255)
			}
			p.mu.Unlock()
		})
	}
}

// DrawTrianglesInterpolated rasterizes triangles using interpolated vertex colors.
// Each vertex should have 4 attributes (RGBA).
func (p *Pipeline) DrawTrianglesInterpolated(triangles []Triangle) {
	p.mu.Lock()
	viewport := p.viewport
	depthTest := p.depthTest
	depthWrite := p.depthWrite
	depthCompare := p.depthCompare
	cullMode := p.cullMode
	frontFace := p.frontFace
	blendState := p.blendState
	p.mu.Unlock()

	for i := range triangles {
		tri := &triangles[i]

		// Face culling
		if ShouldCull(*tri, cullMode, frontFace) {
			continue
		}

		// Rasterize triangle
		Rasterize(*tri, viewport, func(frag Fragment) {
			// Bounds check
			if frag.X < 0 || frag.X >= p.width || frag.Y < 0 || frag.Y >= p.height {
				return
			}

			// Depth test
			if depthTest {
				if !p.depthBuffer.TestAndSet(frag.X, frag.Y, frag.Depth, depthCompare, depthWrite) {
					return
				}
			} else if depthWrite {
				p.depthBuffer.Set(frag.X, frag.Y, frag.Depth)
			}

			// Get interpolated color from attributes
			srcColor := [4]float32{1, 1, 1, 1}
			if len(frag.Attributes) >= 4 {
				srcColor[0] = frag.Attributes[0]
				srcColor[1] = frag.Attributes[1]
				srcColor[2] = frag.Attributes[2]
				srcColor[3] = frag.Attributes[3]
			}

			// Apply blending if enabled
			idx := (frag.Y*p.width + frag.X) * 4
			p.mu.Lock()
			if blendState.Enabled {
				r, g, b, a := BlendFloatToByte(srcColor,
					p.colorBuffer[idx+0], p.colorBuffer[idx+1],
					p.colorBuffer[idx+2], p.colorBuffer[idx+3],
					blendState)
				p.colorBuffer[idx+0] = r
				p.colorBuffer[idx+1] = g
				p.colorBuffer[idx+2] = b
				p.colorBuffer[idx+3] = a
			} else {
				p.colorBuffer[idx+0] = clampByte(srcColor[0] * 255)
				p.colorBuffer[idx+1] = clampByte(srcColor[1] * 255)
				p.colorBuffer[idx+2] = clampByte(srcColor[2] * 255)
				p.colorBuffer[idx+3] = clampByte(srcColor[3] * 255)
			}
			p.mu.Unlock()
		})
	}
}

// GetColorBuffer returns a copy of the RGBA8 color buffer.
// The data is in row-major order with 4 bytes per pixel (RGBA).
func (p *Pipeline) GetColorBuffer() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()

	result := make([]byte, len(p.colorBuffer))
	copy(result, p.colorBuffer)
	return result
}

// GetDepthBuffer returns the depth buffer.
func (p *Pipeline) GetDepthBuffer() *DepthBuffer {
	return p.depthBuffer
}

// GetPixel returns the RGBA color at the specified pixel.
// Returns (0, 0, 0, 0) for out-of-bounds coordinates.
func (p *Pipeline) GetPixel(x, y int) (r, g, b, a byte) {
	if x < 0 || x >= p.width || y < 0 || y >= p.height {
		return 0, 0, 0, 0
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	idx := (y*p.width + x) * 4
	return p.colorBuffer[idx], p.colorBuffer[idx+1], p.colorBuffer[idx+2], p.colorBuffer[idx+3]
}

// SetPixel sets the RGBA color at the specified pixel.
// Out-of-bounds coordinates are silently ignored.
func (p *Pipeline) SetPixel(x, y int, r, g, b, a byte) {
	if x < 0 || x >= p.width || y < 0 || y >= p.height {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	idx := (y*p.width + x) * 4
	p.colorBuffer[idx+0] = r
	p.colorBuffer[idx+1] = g
	p.colorBuffer[idx+2] = b
	p.colorBuffer[idx+3] = a
}

// Resize changes the dimensions of both buffers.
// This clears all existing data.
func (p *Pipeline) Resize(width, height int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.width = width
	p.height = height
	p.colorBuffer = make([]byte, width*height*4)
	p.depthBuffer = NewDepthBuffer(width, height)

	// Update viewport if it was full-screen
	if p.viewport.X == 0 && p.viewport.Y == 0 {
		p.viewport.Width = width
		p.viewport.Height = height
	}
}

// clampByte converts a float to a byte, clamping to [0, 255].
func clampByte(v float32) byte {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return byte(v)
}

// CreateScreenTriangle creates a Triangle from screen coordinates.
// This is a helper for testing - positions should already be in screen space.
func CreateScreenTriangle(x0, y0, z0, x1, y1, z1, x2, y2, z2 float32) Triangle {
	return Triangle{
		V0: ScreenVertex{X: x0, Y: y0, Z: z0, W: 1.0},
		V1: ScreenVertex{X: x1, Y: y1, Z: z1, W: 1.0},
		V2: ScreenVertex{X: x2, Y: y2, Z: z2, W: 1.0},
	}
}

// CreateScreenTriangleWithColor creates a Triangle with vertex colors.
// Colors are in RGBA format with values in [0, 1].
func CreateScreenTriangleWithColor(
	x0, y0, z0 float32, c0 [4]float32,
	x1, y1, z1 float32, c1 [4]float32,
	x2, y2, z2 float32, c2 [4]float32,
) Triangle {
	return Triangle{
		V0: ScreenVertex{X: x0, Y: y0, Z: z0, W: 1.0, Attributes: c0[:]},
		V1: ScreenVertex{X: x1, Y: y1, Z: z1, W: 1.0, Attributes: c1[:]},
		V2: ScreenVertex{X: x2, Y: y2, Z: z2, W: 1.0, Attributes: c2[:]},
	}
}
