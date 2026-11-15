//go:build !(js && wasm)

package raster

import (
	"sync"
)

// Rect defines a rectangular region in screen space.
type Rect struct {
	// X is the left edge of the rectangle in pixels.
	X int

	// Y is the top edge of the rectangle in pixels.
	Y int

	// Width of the rectangle in pixels.
	Width int

	// Height of the rectangle in pixels.
	Height int
}

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

	// Stencil configuration: stencilState applies to front-facing triangles,
	// stencilBack to back-facing ones (WebGPU stencilFront / stencilBack).
	stencilBuffer *StencilBuffer
	stencilState  StencilState
	stencilBack   StencilState

	// Scissor test (nil = disabled)
	scissorRect *Rect

	// Clipping configuration
	clippingEnabled bool

	// Parallel rasterization
	parallelRasterizer *ParallelRasterizer
	useParallel        bool
	parallelConfig     ParallelConfig

	// colorOutput, when set, receives every fragment color that passes the
	// tests instead of the color buffer (SetColorOutput).
	colorOutput ColorOutput

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
		depthTest:       false,
		depthWrite:      true,
		depthCompare:    CompareLess,
		cullMode:        CullNone,
		frontFace:       FrontFaceCCW,
		blendState:      BlendDisabled,
		stencilBuffer:   nil,
		stencilState:    DefaultStencilState(),
		scissorRect:     nil,
		clippingEnabled: false,
		colorBuffer:     make([]byte, size),
		depthBuffer:     NewDepthBuffer(width, height),
		width:           width,
		height:          height,
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
	rb := FloatToUnorm8(r)
	gb := FloatToUnorm8(g)
	bb := FloatToUnorm8(b)
	ab := FloatToUnorm8(a)

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

// ColorOutput receives the color of one fragment at pixel (x, y) that passed
// the scissor, depth and stencil tests, as the shader produced it: not
// blended, clamped or converted.
type ColorOutput func(x, y int, color [4]float32)

// SetColorOutput routes the fragment colors of later draws to out instead of
// the RGBA8 color buffer, whose blend state then does not apply: the caller
// blends and stores in the format of its own target. nil restores the color
// buffer.
func (p *Pipeline) SetColorOutput(out ColorOutput) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.colorOutput = out
}

// writeColor stores the color of the fragment at (x, y): through out when
// set, otherwise blended into the color buffer.
func (p *Pipeline) writeColor(x, y int, color [4]float32, blendState BlendState, out ColorOutput) {
	if out != nil {
		out(x, y, color)
		return
	}
	idx := (y*p.width + x) * 4
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
		p.colorBuffer[idx+0] = FloatToUnorm8(color[0])
		p.colorBuffer[idx+1] = FloatToUnorm8(color[1])
		p.colorBuffer[idx+2] = FloatToUnorm8(color[2])
		p.colorBuffer[idx+3] = FloatToUnorm8(color[3])
	}
	p.mu.Unlock()
}

// SetDepthBuffer replaces the pipeline's own depth buffer with buf, so draws
// test against and write into a depth buffer that outlives the pipeline (a
// render pass's depth attachment). buf must have the pipeline's dimensions.
func (p *Pipeline) SetDepthBuffer(buf *DepthBuffer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.depthBuffer = buf
}

// SetStencilBuffer sets the stencil buffer to use for stencil testing.
// Pass nil to disable stencil testing.
func (p *Pipeline) SetStencilBuffer(buf *StencilBuffer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stencilBuffer = buf
}

// GetStencilBuffer returns the current stencil buffer.
func (p *Pipeline) GetStencilBuffer() *StencilBuffer {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stencilBuffer
}

// SetStencilState sets the stencil testing configuration of both faces.
func (p *Pipeline) SetStencilState(state StencilState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stencilState = state
	p.stencilBack = state
}

// SetStencilFaceStates sets the stencil testing configuration of
// front-facing and back-facing triangles separately, facing as decided by
// the front face (SetFrontFace) and IsBackFacing.
func (p *Pipeline) SetStencilFaceStates(front, back StencilState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stencilState = front
	p.stencilBack = back
}

// faceStencilState returns the stencil state of tri: back for a back-facing
// triangle, front otherwise.
func faceStencilState(tri *Triangle, frontFace FrontFace, front, back *StencilState) StencilState {
	if *front != *back && IsBackFacing(*tri, frontFace) {
		return *back
	}
	return *front
}

// GetStencilState returns the stencil state of front-facing triangles.
func (p *Pipeline) GetStencilState() StencilState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stencilState
}

// SetScissor sets the scissor rectangle for clipping fragments.
// Pass nil to disable the scissor test.
func (p *Pipeline) SetScissor(rect *Rect) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if rect == nil {
		p.scissorRect = nil
	} else {
		// Copy the rect to avoid external mutation
		p.scissorRect = &Rect{
			X:      rect.X,
			Y:      rect.Y,
			Width:  rect.Width,
			Height: rect.Height,
		}
	}
}

// GetScissor returns the current scissor rectangle.
// Returns nil if scissor testing is disabled.
func (p *Pipeline) GetScissor() *Rect {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.scissorRect == nil {
		return nil
	}
	return &Rect{
		X:      p.scissorRect.X,
		Y:      p.scissorRect.Y,
		Width:  p.scissorRect.Width,
		Height: p.scissorRect.Height,
	}
}

// SetClipping enables or disables frustum clipping.
// When enabled, triangles are clipped against the view frustum before rasterization.
func (p *Pipeline) SetClipping(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clippingEnabled = enabled
}

// IsClippingEnabled returns whether frustum clipping is enabled.
func (p *Pipeline) IsClippingEnabled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.clippingEnabled
}

// SetParallelConfig sets the parallel rasterization configuration.
// If enabled, the pipeline will use tile-based parallel rasterization.
func (p *Pipeline) SetParallelConfig(config ParallelConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.parallelConfig = config

	// Recreate parallel rasterizer if needed
	if p.parallelRasterizer != nil {
		p.parallelRasterizer.Close()
	}
	p.parallelRasterizer = NewParallelRasterizer(p.width, p.height, config)
}

// GetParallelConfig returns the current parallel configuration.
func (p *Pipeline) GetParallelConfig() ParallelConfig {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.parallelConfig
}

// EnableParallel enables or disables parallel rasterization.
// When enabled, triangles are rasterized using tile-based parallelization.
func (p *Pipeline) EnableParallel(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.useParallel = enabled

	// Initialize parallel rasterizer if not already created
	if enabled && p.parallelRasterizer == nil {
		if p.parallelConfig.Workers <= 0 {
			p.parallelConfig = DefaultParallelConfig()
		}
		p.parallelRasterizer = NewParallelRasterizer(p.width, p.height, p.parallelConfig)
	}
}

// IsParallelEnabled returns whether parallel rasterization is enabled.
func (p *Pipeline) IsParallelEnabled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.useParallel
}

// ClearStencil fills the stencil buffer with the specified value.
func (p *Pipeline) ClearStencil(value uint8) {
	if p.stencilBuffer != nil {
		p.stencilBuffer.Clear(value)
	}
}

// passesScissorTest returns true if the fragment passes the scissor test.
func (p *Pipeline) passesScissorTest(x, y int, scissor *Rect) bool {
	if scissor == nil {
		return true
	}
	return x >= scissor.X && x < scissor.X+scissor.Width &&
		y >= scissor.Y && y < scissor.Y+scissor.Height
}

// depthStencilTestResult holds the result of depth/stencil testing.
type depthStencilTestResult struct {
	passed     bool
	writeDepth bool
}

// performDepthStencilTest runs depth and stencil tests for a fragment.
// Returns whether the fragment passed all tests and whether to write depth.
func (p *Pipeline) performDepthStencilTest(
	x, y int, depth float32,
	depthTest, depthWrite bool, depthCompare CompareFunc,
	stencilBuffer *StencilBuffer, stencilState StencilState,
) depthStencilTestResult {
	// With stencil test
	if stencilBuffer != nil && stencilState.Enabled {
		// Perform depth test first to know if we need DepthFailOp
		depthPassed := !depthTest || p.depthBuffer.Test(x, y, depth, depthCompare)

		// Stencil test and apply operation
		if !stencilBuffer.TestAndApply(x, y, depthPassed, stencilState) {
			return depthStencilTestResult{passed: false}
		}

		// Stencil passed but depth failed
		if !depthPassed {
			return depthStencilTestResult{passed: false}
		}

		return depthStencilTestResult{passed: true, writeDepth: depthWrite}
	}

	// Without stencil test - original depth test path
	if depthTest {
		if !p.depthBuffer.TestAndSet(x, y, depth, depthCompare, depthWrite) {
			return depthStencilTestResult{passed: false}
		}
		return depthStencilTestResult{passed: true, writeDepth: false} // Already written
	}

	return depthStencilTestResult{passed: true, writeDepth: depthWrite}
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
	out := p.colorOutput
	stencilBuffer := p.stencilBuffer
	stencilFront, stencilBack := p.stencilState, p.stencilBack
	var scissor *Rect
	if p.scissorRect != nil {
		scissor = &Rect{
			X:      p.scissorRect.X,
			Y:      p.scissorRect.Y,
			Width:  p.scissorRect.Width,
			Height: p.scissorRect.Height,
		}
	}
	p.mu.Unlock()

	for i := range triangles {
		tri := &triangles[i]

		// Face culling
		if ShouldCull(*tri, cullMode, frontFace) {
			continue
		}
		stencilState := faceStencilState(tri, frontFace, &stencilFront, &stencilBack)

		// Rasterize triangle
		Rasterize(*tri, viewport, func(frag Fragment) {
			// Bounds check
			if frag.X < 0 || frag.X >= p.width || frag.Y < 0 || frag.Y >= p.height {
				return
			}

			// Scissor test
			if !p.passesScissorTest(frag.X, frag.Y, scissor) {
				return
			}

			// Depth and stencil tests
			result := p.performDepthStencilTest(
				frag.X, frag.Y, frag.Depth,
				depthTest, depthWrite, depthCompare,
				stencilBuffer, stencilState,
			)
			if !result.passed {
				return
			}
			if result.writeDepth {
				p.depthBuffer.Set(frag.X, frag.Y, frag.Depth)
			}

			p.writeColor(frag.X, frag.Y, color, blendState, out)
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
	out := p.colorOutput
	stencilBuffer := p.stencilBuffer
	stencilFront, stencilBack := p.stencilState, p.stencilBack
	var scissor *Rect
	if p.scissorRect != nil {
		scissor = &Rect{
			X:      p.scissorRect.X,
			Y:      p.scissorRect.Y,
			Width:  p.scissorRect.Width,
			Height: p.scissorRect.Height,
		}
	}
	p.mu.Unlock()

	for i := range triangles {
		tri := &triangles[i]

		// Face culling
		if ShouldCull(*tri, cullMode, frontFace) {
			continue
		}
		stencilState := faceStencilState(tri, frontFace, &stencilFront, &stencilBack)

		// Rasterize triangle
		Rasterize(*tri, viewport, func(frag Fragment) {
			// Bounds check
			if frag.X < 0 || frag.X >= p.width || frag.Y < 0 || frag.Y >= p.height {
				return
			}

			// Scissor test
			if !p.passesScissorTest(frag.X, frag.Y, scissor) {
				return
			}

			// Depth and stencil tests
			result := p.performDepthStencilTest(
				frag.X, frag.Y, frag.Depth,
				depthTest, depthWrite, depthCompare,
				stencilBuffer, stencilState,
			)
			if !result.passed {
				return
			}
			if result.writeDepth {
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

			p.writeColor(frag.X, frag.Y, srcColor, blendState, out)
		})
	}
}

// FragmentShaderFunc computes the output RGBA color, in [0,1], of one
// fragment: its pixel position, depth, 1/w and interpolated attributes. It
// reports discard = true when the shader discarded the fragment. Used by
// DrawTrianglesWithFragmentShader to execute a SPIR-V fragment shader per pixel.
type FragmentShaderFunc func(frag *Fragment) (color [4]float32, discard bool)

// DrawTrianglesWithFragmentShader rasterizes triangles and invokes fragFunc
// per pixel. A discarded fragment writes no color, depth or stencil. When
// mayDiscard is set the shader runs before the depth and stencil tests, whose
// writes would otherwise happen first (late tests); without discard the tests
// run first so occluded fragments are never shaded.
func (p *Pipeline) DrawTrianglesWithFragmentShader(triangles []Triangle, fragFunc FragmentShaderFunc, mayDiscard bool) {
	p.mu.Lock()
	viewport := p.viewport
	depthTest := p.depthTest
	depthWrite := p.depthWrite
	depthCompare := p.depthCompare
	cullMode := p.cullMode
	frontFace := p.frontFace
	blendState := p.blendState
	out := p.colorOutput
	stencilBuffer := p.stencilBuffer
	stencilFront, stencilBack := p.stencilState, p.stencilBack
	var scissor *Rect
	if p.scissorRect != nil {
		scissor = &Rect{
			X:      p.scissorRect.X,
			Y:      p.scissorRect.Y,
			Width:  p.scissorRect.Width,
			Height: p.scissorRect.Height,
		}
	}
	p.mu.Unlock()

	for i := range triangles {
		tri := &triangles[i]

		// Face culling
		if ShouldCull(*tri, cullMode, frontFace) {
			continue
		}
		stencilState := faceStencilState(tri, frontFace, &stencilFront, &stencilBack)

		// Rasterize triangle
		Rasterize(*tri, viewport, func(frag Fragment) {
			// Bounds check
			if frag.X < 0 || frag.X >= p.width || frag.Y < 0 || frag.Y >= p.height {
				return
			}

			// Scissor test
			if !p.passesScissorTest(frag.X, frag.Y, scissor) {
				return
			}

			var srcColor [4]float32
			if mayDiscard {
				var discard bool
				if srcColor, discard = fragFunc(&frag); discard {
					return
				}
			}

			// Depth and stencil tests
			result := p.performDepthStencilTest(
				frag.X, frag.Y, frag.Depth,
				depthTest, depthWrite, depthCompare,
				stencilBuffer, stencilState,
			)
			if !result.passed {
				return
			}
			if result.writeDepth {
				p.depthBuffer.Set(frag.X, frag.Y, frag.Depth)
			}

			if !mayDiscard {
				srcColor, _ = fragFunc(&frag)
			}

			p.writeColor(frag.X, frag.Y, srcColor, blendState, out)
		})
	}
}

// MRTFragmentShaderFunc computes output colors for multiple render targets.
// It receives the fragment and returns a slice of RGBA colors, one per render
// target (indexed by @location), or discard = true. The primary target
// (location 0) is written to the pipeline's color buffer; additional targets
// are written by the caller via the MRTOutput callback.
type MRTFragmentShaderFunc func(frag *Fragment) (colors [][4]float32, discard bool)

// MRTOutput is called by DrawTrianglesWithMRTShader for each fragment that
// passes depth/stencil/scissor tests. It receives the pixel coordinates and
// per-location output colors (index 0 = location 0, etc.). The caller uses
// this to write colors beyond location 0 to their respective target textures.
type MRTOutput func(x, y int, colors [][4]float32)

// DrawTrianglesWithMRTShader rasterizes triangles and invokes fragFunc per
// pixel for Multiple Render Targets. Location 0 is written to the pipeline's
// color buffer (with blending), or to the color output when one is set
// (SetColorOutput). The mrtOut callback receives all per-location
// colors so the caller can write locations 1+ to their respective textures.
// Discard and mayDiscard behave as in DrawTrianglesWithFragmentShader.
func (p *Pipeline) DrawTrianglesWithMRTShader(triangles []Triangle, fragFunc MRTFragmentShaderFunc, mrtOut MRTOutput, mayDiscard bool) {
	p.mu.Lock()
	viewport := p.viewport
	depthTest := p.depthTest
	depthWrite := p.depthWrite
	depthCompare := p.depthCompare
	cullMode := p.cullMode
	frontFace := p.frontFace
	blendState := p.blendState
	out := p.colorOutput
	stencilBuffer := p.stencilBuffer
	stencilFront, stencilBack := p.stencilState, p.stencilBack
	var scissor *Rect
	if p.scissorRect != nil {
		scissor = &Rect{
			X:      p.scissorRect.X,
			Y:      p.scissorRect.Y,
			Width:  p.scissorRect.Width,
			Height: p.scissorRect.Height,
		}
	}
	p.mu.Unlock()

	for i := range triangles {
		tri := &triangles[i]

		if ShouldCull(*tri, cullMode, frontFace) {
			continue
		}
		stencilState := faceStencilState(tri, frontFace, &stencilFront, &stencilBack)

		Rasterize(*tri, viewport, func(frag Fragment) {
			if frag.X < 0 || frag.X >= p.width || frag.Y < 0 || frag.Y >= p.height {
				return
			}

			if !p.passesScissorTest(frag.X, frag.Y, scissor) {
				return
			}

			var colors [][4]float32
			if mayDiscard {
				var discard bool
				if colors, discard = fragFunc(&frag); discard {
					return
				}
			}

			result := p.performDepthStencilTest(
				frag.X, frag.Y, frag.Depth,
				depthTest, depthWrite, depthCompare,
				stencilBuffer, stencilState,
			)
			if !result.passed {
				return
			}
			if result.writeDepth {
				p.depthBuffer.Set(frag.X, frag.Y, frag.Depth)
			}

			if !mayDiscard {
				colors, _ = fragFunc(&frag)
			}

			// Write location 0 to the color buffer or the color output.
			if len(colors) > 0 {
				p.writeColor(frag.X, frag.Y, colors[0], blendState, out)
			}

			// Notify caller about all per-location outputs for targets 1+.
			if mrtOut != nil && len(colors) > 1 {
				mrtOut(frag.X, frag.Y, colors)
			}
		})
	}
}

// QuadShaderFunc runs a fragment shader for the four lanes of q. The lanes
// in helpers run only so the others can take derivatives (Quad.Frags holds a
// fragment for each). It returns, for every lane that is not a helper and did
// not discard, the colors per @location in colors[lane], and the lanes that
// discarded. The colors are read before the next call, so the function may
// reuse their storage.
type QuadShaderFunc func(q *Quad, helpers uint8) (colors *[QuadLanes][][4]float32, discarded uint8)

// DrawTrianglesWithQuadShader rasterizes triangles in 2x2 quads and runs
// quadFunc once per quad that has a covered fragment, for shaders whose
// derivatives or implicit-LOD samples need a fragment's neighbors.
//
// A lane is live when the triangle covers its pixel and the pixel passes the
// bounds and scissor tests; the other lanes are helpers. Without mayDiscard
// the depth and stencil tests run first, and a lane that fails them becomes
// a helper too: it is not shaded for output, but its neighbors still take
// differences from it. With mayDiscard the tests run after the shader, for
// the lanes that did not discard. Location 0 is written as in
// DrawTrianglesWithMRTShader, and mrtOut (if set) receives every location of
// a fragment that has more than one.
func (p *Pipeline) DrawTrianglesWithQuadShader(triangles []Triangle, quadFunc QuadShaderFunc, mrtOut MRTOutput, mayDiscard bool) {
	p.mu.Lock()
	viewport := p.viewport
	depthTest := p.depthTest
	depthWrite := p.depthWrite
	depthCompare := p.depthCompare
	cullMode := p.cullMode
	frontFace := p.frontFace
	blendState := p.blendState
	out := p.colorOutput
	stencilBuffer := p.stencilBuffer
	stencilFront, stencilBack := p.stencilState, p.stencilBack
	var scissor *Rect
	if p.scissorRect != nil {
		scissor = &Rect{
			X:      p.scissorRect.X,
			Y:      p.scissorRect.Y,
			Width:  p.scissorRect.Width,
			Height: p.scissorRect.Height,
		}
	}
	p.mu.Unlock()

	// test runs the depth and stencil tests of frag and writes its depth;
	// it reports whether the fragment passed.
	var stencilState StencilState
	test := func(frag *Fragment) bool {
		result := p.performDepthStencilTest(
			frag.X, frag.Y, frag.Depth,
			depthTest, depthWrite, depthCompare,
			stencilBuffer, stencilState,
		)
		if result.passed && result.writeDepth {
			p.depthBuffer.Set(frag.X, frag.Y, frag.Depth)
		}
		return result.passed
	}
	shadeQuad := func(q *Quad) {
		var live uint8
		for lane := range QuadLanes {
			frag := &q.Frags[lane]
			if q.Covered&(1<<lane) == 0 ||
				frag.X < 0 || frag.X >= p.width || frag.Y < 0 || frag.Y >= p.height ||
				!p.passesScissorTest(frag.X, frag.Y, scissor) {
				continue
			}
			if mayDiscard || test(frag) {
				live |= 1 << lane
			}
		}
		if live == 0 {
			return
		}
		colors, discarded := quadFunc(q, ^live&(1<<QuadLanes-1))
		for lane := range QuadLanes {
			frag := &q.Frags[lane]
			if live&(1<<lane) == 0 || discarded&(1<<lane) != 0 || (mayDiscard && !test(frag)) {
				continue
			}
			c := colors[lane]
			if len(c) > 0 {
				p.writeColor(frag.X, frag.Y, c[0], blendState, out)
			}
			if mrtOut != nil && len(c) > 1 {
				mrtOut(frag.X, frag.Y, c)
			}
		}
	}

	var rast QuadRasterizer
	for i := range triangles {
		tri := &triangles[i]
		if ShouldCull(*tri, cullMode, frontFace) {
			continue
		}
		stencilState = faceStencilState(tri, frontFace, &stencilFront, &stencilBack)
		rast.Rasterize(tri, viewport, shadeQuad)
	}
}

// DrawTrianglesParallel uses tile-based parallel rasterization.
// This can significantly speed up rendering for large numbers of triangles
// by distributing work across multiple CPU cores.
//
// Note: Parallel rendering requires EnableParallel(true) to be called first.
// If parallel is not enabled, this falls back to DrawTriangles.
func (p *Pipeline) DrawTrianglesParallel(triangles []Triangle, color [4]float32) {
	p.mu.Lock()
	useParallel := p.useParallel
	parallelRasterizer := p.parallelRasterizer
	viewport := p.viewport
	depthTest := p.depthTest
	depthWrite := p.depthWrite
	depthCompare := p.depthCompare
	cullMode := p.cullMode
	frontFace := p.frontFace
	blendState := p.blendState
	out := p.colorOutput
	stencilBuffer := p.stencilBuffer
	stencilFront, stencilBack := p.stencilState, p.stencilBack
	var scissor *Rect
	if p.scissorRect != nil {
		scissor = &Rect{
			X:      p.scissorRect.X,
			Y:      p.scissorRect.Y,
			Width:  p.scissorRect.Width,
			Height: p.scissorRect.Height,
		}
	}
	p.mu.Unlock()

	// Fall back to sequential if parallel not enabled
	if !useParallel || parallelRasterizer == nil {
		p.DrawTriangles(triangles, color)
		return
	}

	// Filter and cull triangles before binning
	validTriangles := make([]Triangle, 0, len(triangles))
	for i := range triangles {
		tri := &triangles[i]
		if !ShouldCull(*tri, cullMode, frontFace) {
			validTriangles = append(validTriangles, *tri)
		}
	}

	if len(validTriangles) == 0 {
		return
	}

	// Use parallel rasterizer
	parallelRasterizer.RasterizeParallel(validTriangles, func(tile Tile, tileTriangles []Triangle) {
		// Process all triangles in this tile
		for i := range tileTriangles {
			tri := &tileTriangles[i]
			stencilState := faceStencilState(tri, frontFace, &stencilFront, &stencilBack)

			RasterizeTile(*tri, tile, func(frag Fragment) {
				// Bounds check (should always pass for properly clipped tiles)
				if frag.X < viewport.X || frag.X >= viewport.X+viewport.Width ||
					frag.Y < viewport.Y || frag.Y >= viewport.Y+viewport.Height {
					return
				}

				// Scissor test
				if !p.passesScissorTest(frag.X, frag.Y, scissor) {
					return
				}

				// Depth and stencil tests
				result := p.performDepthStencilTest(
					frag.X, frag.Y, frag.Depth,
					depthTest, depthWrite, depthCompare,
					stencilBuffer, stencilState,
				)
				if !result.passed {
					return
				}
				if result.writeDepth {
					p.depthBuffer.Set(frag.X, frag.Y, frag.Depth)
				}

				p.writeColor(frag.X, frag.Y, color, blendState, out)
			})
		}
	})
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

	// Update parallel rasterizer if enabled
	if p.parallelRasterizer != nil {
		p.parallelRasterizer.Resize(width, height)
	}
}

// Close releases resources used by the pipeline.
// This should be called when the pipeline is no longer needed.
func (p *Pipeline) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.parallelRasterizer != nil {
		p.parallelRasterizer.Close()
		p.parallelRasterizer = nil
	}
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
