//go:build !(js && wasm)

package software

import (
	"encoding/binary"
	"log/slog"
	"math"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/software/raster"
	"github.com/gogpu/wgpu/hal/software/shader"
)

// executeDraw is the core draw implementation.
// It selects between fullscreen texture blit and vertex-buffer-based rasterization.
// instanceCount > 1 enables instanced rendering; firstInstance offsets the instance ID.
func (r *RenderPassEncoder) executeDraw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	if r.pipeline == nil {
		return
	}
	if instanceCount == 0 {
		instanceCount = 1
	}

	target := r.getTargetTexture()
	if target == nil {
		return
	}

	// No vertex buffer bound — try SPIR-V shader path or fullscreen blit.
	if r.vertexBufs[0].buffer == nil {
		// SPIR-V path: when the pipeline has no vertex buffer layouts but has
		// a shader module with SPIR-V (e.g. @builtin(vertex_index) triangle),
		// execute the shader via the interpreter.
		if r.executeSPIRVDraw(target, vertexCount, instanceCount, firstVertex, firstInstance) {
			return
		}

		// Fullscreen blit path: blit bound texture to target.
		// The blit overwrites every destination pixel, so applyClear is redundant.
		// Skipping clear saves ~18% CPU (8 MB memset at 1920x1080).
		if r.executeFullscreenBlit(target) {
			r.cleared = true
			return
		}
		// No source texture found — apply clear only (clear-only pass).
		if !r.cleared {
			r.applyClear()
		}
		return
	}

	// Vertex draw: triangles may not cover all pixels → clear first.
	if !r.cleared {
		r.applyClear()
	}
	r.executeVertexDraw(target, vertexCount, instanceCount, firstVertex, firstInstance)
}

// getTargetTexture returns the texture backing the first color attachment.
func (r *RenderPassEncoder) getTargetTexture() *Texture {
	if len(r.desc.ColorAttachments) == 0 {
		return nil
	}
	view, ok := r.desc.ColorAttachments[0].View.(*TextureView)
	if !ok || view.texture == nil {
		return nil
	}
	return view.texture
}

// executeFullscreenBlit blits the first bound texture to the target.
// This is the fast path for gogpu's renderTexturedQuad (6 vertices, no vertex buffer,
// texture in bind group). Returns true if blit was performed, false if no source
// texture was found (caller should apply clear instead).
func (r *RenderPassEncoder) executeFullscreenBlit(target *Texture) bool {
	srcView := r.findBoundTexture()
	if srcView == nil || srcView.texture == nil {
		return false
	}

	src := srcView.texture
	src.mu.RLock()
	srcData := src.data
	srcW := int(src.width)
	srcH := int(src.height)
	srcFmt := src.format
	src.mu.RUnlock()

	dstW := int(target.width)
	dstH := int(target.height)
	dstFmt := target.format

	target.mu.Lock()
	defer target.mu.Unlock()

	// Hoist format check before all loops — constant for entire blit operation.
	swizzle := needsSwizzle(srcFmt, dstFmt)

	// Pre-validate buffer sizes to eliminate per-pixel bounds checks.
	srcSize := srcW * srcH * 4
	dstSize := dstW * dstH * 4
	if len(srcData) < srcSize || len(target.data) < dstSize {
		return false // Buffers too small — skip entire blit
	}

	// Fast path: same-size blit (no scaling needed).
	if srcW == dstW && srcH == dstH {
		if swizzle {
			for i := 0; i < srcSize; i += 4 {
				target.data[i+0] = srcData[i+2] // B←R
				target.data[i+1] = srcData[i+1] // G
				target.data[i+2] = srcData[i+0] // R←B
				target.data[i+3] = srcData[i+3] // A
			}
		} else {
			copy(target.data[:dstSize], srcData[:srcSize])
		}
		return true
	}

	// Scaled blit: nearest-neighbor sampling with pre-computed column map.
	// Optimizations vs naive per-pixel:
	// 1. Column map eliminates multiply+divide from inner loop (table lookup instead)
	// 2. Row deduplication: when upscaling, many dst rows map to the same src row —
	//    just memcpy the previous dst row (~50% fewer per-pixel iterations at 2x scale)

	// Pre-compute column mapping: colMap[dx] = source byte offset within row.
	dstRowBytes := dstW * 4
	colMap := make([]int, dstW)
	for dx := 0; dx < dstW; dx++ {
		sx := dx * srcW / dstW
		if sx >= srcW {
			sx = srcW - 1
		}
		colMap[dx] = sx * 4
	}

	prevSY := -1
	for dy := 0; dy < dstH; dy++ {
		sy := dy * srcH / dstH
		if sy >= srcH {
			sy = srcH - 1
		}
		dstRowOff := dy * dstRowBytes

		// Row deduplication: if this dst row maps to same src row as previous,
		// copy the already-computed dst row (memcpy ≫ per-pixel loop).
		if sy == prevSY && dy > 0 {
			prevRowOff := (dy - 1) * dstRowBytes
			copy(target.data[dstRowOff:dstRowOff+dstRowBytes],
				target.data[prevRowOff:prevRowOff+dstRowBytes])
			continue
		}
		prevSY = sy

		srcRowOff := sy * srcW * 4
		if swizzle {
			for dx := 0; dx < dstW; dx++ {
				srcIdx := srcRowOff + colMap[dx]
				dstIdx := dstRowOff + dx*4
				target.data[dstIdx+0] = srcData[srcIdx+2]
				target.data[dstIdx+1] = srcData[srcIdx+1]
				target.data[dstIdx+2] = srcData[srcIdx+0]
				target.data[dstIdx+3] = srcData[srcIdx+3]
			}
		} else {
			for dx := 0; dx < dstW; dx++ {
				srcIdx := srcRowOff + colMap[dx]
				dstIdx := dstRowOff + dx*4
				target.data[dstIdx+0] = srcData[srcIdx+0]
				target.data[dstIdx+1] = srcData[srcIdx+1]
				target.data[dstIdx+2] = srcData[srcIdx+2]
				target.data[dstIdx+3] = srcData[srcIdx+3]
			}
		}
	}
	return true
}

// needsSwizzle returns true when source and destination formats require R/B channel swap.
func needsSwizzle(src, dst gputypes.TextureFormat) bool {
	srcBGRA := src == gputypes.TextureFormatBGRA8Unorm || src == gputypes.TextureFormatBGRA8UnormSrgb
	dstBGRA := dst == gputypes.TextureFormatBGRA8Unorm || dst == gputypes.TextureFormatBGRA8UnormSrgb
	return srcBGRA != dstBGRA
}

// findBoundTexture searches all bind groups for the first texture view binding.
func (r *RenderPassEncoder) findBoundTexture() *TextureView {
	for i := range r.bindGroups {
		bg := r.bindGroups[i]
		if bg == nil {
			continue
		}
		for _, tv := range bg.textureViews {
			if tv != nil {
				return tv
			}
		}
	}
	return nil
}

// executeVertexDraw performs vertex fetch, viewport transform, and triangle rasterization.
// Supports instanced rendering and TriangleStrip topology.
// When the pipeline has a SPIR-V vertex shader, the shader is executed per-vertex
// with both @builtin and @location inputs, and the output @location attributes
// are used for per-vertex color interpolation.
func (r *RenderPassEncoder) executeVertexDraw(target *Texture, vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	if r.pipeline.desc == nil {
		return
	}

	layouts := r.pipeline.desc.Vertex.Buffers
	if len(layouts) == 0 {
		// No vertex buffer layouts -- this draw was already handled by
		// executeSPIRVDraw in executeDraw. Nothing more to do.
		return
	}

	w := int(target.width)
	h := int(target.height)

	pipe := raster.NewPipeline(w, h)

	// Copy current framebuffer into the raster pipeline so draws composite.
	target.mu.RLock()
	existingData := make([]byte, len(target.data))
	copy(existingData, target.data)
	target.mu.RUnlock()
	pipe.Clear(0, 0, 0, 0)
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			idx := (py*w + px) * 4
			pipe.SetPixel(px, py, existingData[idx], existingData[idx+1], existingData[idx+2], existingData[idx+3])
		}
	}

	// Attempt SPIR-V vertex shader path (handles @builtin + @location inputs
	// and per-vertex output attributes for interpolation).
	triangles := r.fetchTrianglesSPIRV(layouts, vertexCount, instanceCount, firstVertex, firstInstance, w, h)
	if triangles == nil {
		// Fallback: raw vertex buffer fetch (non-SPIR-V path, single instance only).
		triangles = r.fetchTriangles(layouts, vertexCount, firstVertex, w, h)
	}

	// Determine fragment color source: if vertices carry attributes, interpolate.
	hasAttrs := false
	for i := range triangles {
		if len(triangles[i].V0.Attributes) >= 3 {
			hasAttrs = true
			break
		}
	}
	switch {
	case hasAttrs:
		pipe.DrawTrianglesInterpolated(triangles)
	case r.hasVertexColors(layouts):
		pipe.DrawTrianglesInterpolated(triangles)
	default:
		color := r.resolveFragmentColor()
		pipe.DrawTriangles(triangles, color)
	}

	// Write raster result back to texture.
	target.mu.Lock()
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			cr, cg, cb, ca := pipe.GetPixel(px, py)
			idx := (py*w + px) * 4
			target.data[idx+0] = cr
			target.data[idx+1] = cg
			target.data[idx+2] = cb
			target.data[idx+3] = ca
		}
	}
	target.mu.Unlock()
}

// fetchTriangles reads vertex data from bound buffers, applies viewport transform,
// and groups vertices into triangles (TriangleList topology).
// This is the legacy non-SPIR-V path for single-instance draws without a shader module.
func (r *RenderPassEncoder) fetchTriangles(
	layouts []gputypes.VertexBufferLayout,
	vertexCount, firstVertex uint32,
	targetW, targetH int,
) []raster.Triangle {
	if vertexCount < 3 {
		return nil
	}

	layout := layouts[0]
	vb := r.vertexBufs[0]
	if vb.buffer == nil {
		return nil
	}

	vb.buffer.mu.RLock()
	bufData := vb.buffer.data
	vb.buffer.mu.RUnlock()

	stride := layout.ArrayStride
	if stride == 0 {
		return nil
	}

	// Classify attributes: find position (location 0) and others.
	var posAttr *gputypes.VertexAttribute
	var extraAttrs []gputypes.VertexAttribute
	for i := range layout.Attributes {
		attr := &layout.Attributes[i]
		if attr.ShaderLocation == 0 {
			posAttr = attr
		} else {
			extraAttrs = append(extraAttrs, *attr)
		}
	}
	if posAttr == nil {
		return nil
	}

	// Read all vertices.
	vertices := make([]raster.ScreenVertex, 0, vertexCount)
	for i := uint32(0); i < vertexCount; i++ {
		vi := firstVertex + i
		base := vb.offset + uint64(vi)*stride

		// Read position.
		pos := readVertexAttribute(bufData, base+posAttr.Offset, posAttr.Format)

		// NDC to screen transform.
		// Position is expected in NDC: x,y in [-1,1], z in [0,1].
		// Screen: x = (ndcX+1)/2 * width, y = (1-ndcY)/2 * height (Y flipped).
		sx := (pos[0] + 1.0) * 0.5 * float32(targetW)
		sy := (1.0 - pos[1]) * 0.5 * float32(targetH)
		sz := float32(0)
		if len(pos) > 2 {
			sz = pos[2]
		}

		sv := raster.ScreenVertex{
			X: sx,
			Y: sy,
			Z: sz,
			W: 1.0,
		}

		// Read extra attributes (color, UV, etc.).
		for _, attr := range extraAttrs {
			vals := readVertexAttribute(bufData, base+attr.Offset, attr.Format)
			sv.Attributes = append(sv.Attributes, vals...)
		}

		// Ensure at least 4 attribute components (RGBA) for interpolated color.
		// RGB vertex colors (Float32x3) need alpha=1.0 padding.
		for len(sv.Attributes) < 4 {
			sv.Attributes = append(sv.Attributes, 1.0)
		}

		vertices = append(vertices, sv)
	}

	return r.verticesToTriangles(vertices)
}

// fetchTrianglesSPIRV executes the SPIR-V vertex shader per-vertex with both
// @builtin inputs (vertex_index, instance_index) and @location inputs from
// vertex buffers, collecting output @location attributes for interpolation.
// Supports instanced rendering and TriangleStrip topology.
// Returns nil if no SPIR-V module is available (caller falls back to fetchTriangles).
//
//nolint:maintidx // Vertex shader dispatch with instancing + attribute binding is inherently complex.
func (r *RenderPassEncoder) fetchTrianglesSPIRV(
	layouts []gputypes.VertexBufferLayout,
	vertexCount, instanceCount, firstVertex, firstInstance uint32,
	targetW, targetH int,
) []raster.Triangle {
	if r.pipeline.desc == nil {
		return nil
	}

	// Get the vertex shader module.
	vsModule, ok := r.pipeline.desc.Vertex.Module.(*ShaderModule)
	if !ok || vsModule == nil {
		return nil
	}
	parsed := vsModule.ParsedModule()
	if parsed == nil {
		return nil
	}

	// Find vertex entry point.
	vsEntry := r.pipeline.desc.Vertex.EntryPoint
	ep, ok := parsed.EntryPoints[vsEntry]
	if !ok || ep.ExecutionModel != shader.ExecutionModelVertex {
		return nil
	}

	// Classify interface variables: identify builtins and location I/O.
	var vertexIndexVarID, instanceIndexVarID, positionVarID uint32
	hasVertexIndex, hasInstanceIndex, hasPosition := false, false, false

	// locationInputVars: shader location -> variable ID for @location inputs.
	type locationVar struct {
		varID    uint32
		location int
	}
	var locationInputs []locationVar
	var locationOutputs []locationVar

	for _, varID := range ep.InterfaceIDs {
		vi, exists := parsed.Variables[varID]
		if !exists {
			continue
		}
		builtIn := parsed.GetBuiltIn(varID)
		loc := parsed.GetLocation(varID)

		switch {
		case vi.StorageClass == shader.StorageClassInput && builtIn == shader.BuiltInVertexIndex:
			vertexIndexVarID = varID
			hasVertexIndex = true
		case vi.StorageClass == shader.StorageClassInput && builtIn == shader.BuiltInInstanceIndex:
			instanceIndexVarID = varID
			hasInstanceIndex = true
		case vi.StorageClass == shader.StorageClassOutput && builtIn == shader.BuiltInPosition:
			positionVarID = varID
			hasPosition = true
		case vi.StorageClass == shader.StorageClassInput && loc >= 0:
			locationInputs = append(locationInputs, locationVar{varID: varID, location: loc})
		case vi.StorageClass == shader.StorageClassOutput && loc >= 0:
			locationOutputs = append(locationOutputs, locationVar{varID: varID, location: loc})
		}
	}

	if !hasPosition {
		return nil
	}

	// Build execution context with bind group resources.
	ctx := r.buildExecutionContext()

	// Pre-snapshot all vertex buffer data under locks.
	type bufSnapshot struct {
		data   []byte
		offset uint64
	}
	var bufSnaps [8]bufSnapshot
	for i := range r.vertexBufs {
		if r.vertexBufs[i].buffer != nil {
			r.vertexBufs[i].buffer.mu.RLock()
			bufSnaps[i] = bufSnapshot{
				data:   r.vertexBufs[i].buffer.data,
				offset: r.vertexBufs[i].offset,
			}
			r.vertexBufs[i].buffer.mu.RUnlock()
		}
	}

	// Map shader locations to vertex buffer layout attributes for vertex fetch.
	// Build a lookup: shaderLocation -> (bufferSlot, attribute, layout).
	type attrBinding struct {
		slot   int
		attr   gputypes.VertexAttribute
		layout gputypes.VertexBufferLayout
	}
	attrMap := make(map[int]attrBinding)
	for slot, layout := range layouts {
		for _, attr := range layout.Attributes {
			attrMap[int(attr.ShaderLocation)] = attrBinding{
				slot:   slot,
				attr:   attr,
				layout: layout,
			}
		}
	}

	// Execute vertex shader for each (instance, vertex) pair.
	var allTriangles []raster.Triangle

	for inst := uint32(0); inst < instanceCount; inst++ {
		instanceID := firstInstance + inst

		vertices := make([]raster.ScreenVertex, 0, vertexCount)
		for vert := uint32(0); vert < vertexCount; vert++ {
			vertexID := firstVertex + vert

			// Build input map for this invocation.
			inputs := make(map[uint32]shader.Value)
			if hasVertexIndex {
				inputs[vertexIndexVarID] = vertexID
			}
			if hasInstanceIndex {
				inputs[instanceIndexVarID] = instanceID
			}

			// Populate @location inputs from vertex buffer data.
			for _, li := range locationInputs {
				ab, found := attrMap[li.location]
				if !found {
					continue
				}
				snap := bufSnaps[ab.slot]
				if snap.data == nil {
					continue
				}
				stride := ab.layout.ArrayStride
				if stride == 0 {
					continue
				}

				// Determine which index drives this buffer based on step mode.
				var idx uint32
				if ab.layout.StepMode == gputypes.VertexStepModeInstance {
					idx = instanceID
				} else {
					idx = vertexID
				}
				base := snap.offset + uint64(idx)*stride
				vals := readVertexAttribute(snap.data, base+ab.attr.Offset, ab.attr.Format)
				if vals == nil {
					continue
				}

				// Convert to the appropriate shader value type.
				inputs[li.varID] = floatsToShaderValue(vals)
			}

			// Execute vertex shader.
			ctx.Inputs = inputs
			outputs, err := parsed.ExecuteWithContext(vsEntry, ctx)
			if err != nil {
				slog.Debug("software: SPIR-V vertex shader failed",
					"vertex", vertexID, "instance", instanceID, "error", err)
				return nil
			}

			// Extract position.
			posVal, posOK := outputs[positionVarID]
			if !posOK {
				return nil
			}
			pos := shader.Vec4ToFloat32(posVal)

			// Clip-space to screen-space transform.
			wClip := pos[3]
			if wClip == 0 {
				wClip = 1
			}
			ndcX := pos[0] / wClip
			ndcY := pos[1] / wClip
			ndcZ := pos[2] / wClip

			sx := (ndcX + 1.0) * 0.5 * float32(targetW)
			sy := (1.0 - ndcY) * 0.5 * float32(targetH)

			sv := raster.ScreenVertex{
				X: sx,
				Y: sy,
				Z: ndcZ,
				W: 1.0,
			}

			// Collect @location outputs as interpolated attributes (sorted by location).
			// These become per-vertex colors/UVs for the rasterizer.
			if len(locationOutputs) > 0 {
				for _, lo := range locationOutputs {
					outVal, outOK := outputs[lo.varID]
					if !outOK {
						continue
					}
					sv.Attributes = append(sv.Attributes, shaderValueToFloats(outVal)...)
				}
				// Pad to at least 4 components (RGBA) for DrawTrianglesInterpolated.
				for len(sv.Attributes) < 4 {
					sv.Attributes = append(sv.Attributes, 1.0)
				}
			}

			vertices = append(vertices, sv)
		}

		// Convert this instance's vertices to triangles and append.
		instanceTris := r.verticesToTriangles(vertices)
		allTriangles = append(allTriangles, instanceTris...)
	}

	return allTriangles
}

// verticesToTriangles converts a list of vertices into triangles based on the
// pipeline's primitive topology (TriangleList or TriangleStrip).
func (r *RenderPassEncoder) verticesToTriangles(vertices []raster.ScreenVertex) []raster.Triangle {
	if len(vertices) < 3 {
		return nil
	}

	topology := gputypes.PrimitiveTopologyTriangleList
	if r.pipeline != nil && r.pipeline.desc != nil {
		topology = r.pipeline.desc.Primitive.Topology
	}

	switch topology {
	case gputypes.PrimitiveTopologyTriangleStrip:
		// TriangleStrip: N vertices produce N-2 triangles.
		// Even triangles: (i, i+1, i+2), odd triangles: (i+1, i, i+2) for correct winding.
		triCount := len(vertices) - 2
		triangles := make([]raster.Triangle, 0, triCount)
		for i := 0; i < triCount; i++ {
			if i%2 == 0 {
				triangles = append(triangles, raster.Triangle{
					V0: vertices[i],
					V1: vertices[i+1],
					V2: vertices[i+2],
				})
			} else {
				triangles = append(triangles, raster.Triangle{
					V0: vertices[i+1],
					V1: vertices[i],
					V2: vertices[i+2],
				})
			}
		}
		return triangles

	default: // TriangleList
		triCount := len(vertices) / 3
		triangles := make([]raster.Triangle, 0, triCount)
		for i := 0; i < triCount; i++ {
			triangles = append(triangles, raster.Triangle{
				V0: vertices[i*3+0],
				V1: vertices[i*3+1],
				V2: vertices[i*3+2],
			})
		}
		return triangles
	}
}

// buildExecutionContext creates a shader ExecutionContext populated with all
// bind group resources (buffers, textures, samplers). This follows the same
// pattern as ComputePassEncoder.Dispatch in command.go.
func (r *RenderPassEncoder) buildExecutionContext() *shader.ExecutionContext {
	ctx := &shader.ExecutionContext{
		Buffers:  make(map[shader.BindingKey][]byte),
		Textures: make(map[shader.BindingKey]*shader.Texture2D),
		Samplers: make(map[shader.BindingKey]*shader.Sampler),
	}

	for groupIdx, bg := range r.bindGroups {
		if bg == nil {
			continue
		}
		// Buffers (uniform/storage).
		for bindingIdx, buf := range bg.buffers {
			if buf == nil {
				continue
			}
			buf.mu.RLock()
			ctx.Buffers[shader.BindingKey{
				Group:   uint32(groupIdx),
				Binding: bindingIdx,
			}] = buf.data
			buf.mu.RUnlock()
		}
		// Textures.
		for bindingIdx, tv := range bg.textureViews {
			if tv == nil || tv.texture == nil {
				continue
			}
			tv.texture.mu.RLock()
			ctx.Textures[shader.BindingKey{
				Group:   uint32(groupIdx),
				Binding: bindingIdx,
			}] = &shader.Texture2D{
				Width:  tv.texture.width,
				Height: tv.texture.height,
				Data:   tv.texture.data,
			}
			tv.texture.mu.RUnlock()
		}
		// Samplers are not stored as separate objects in the software backend's
		// BindGroup struct; the interpreter uses defaults when nil.
	}

	return ctx
}

// floatsToShaderValue converts a float32 slice into the appropriate shader Value type.
func floatsToShaderValue(vals []float32) shader.Value {
	switch len(vals) {
	case 1:
		return shader.Float32(vals[0])
	case 2:
		return shader.Vec2{vals[0], vals[1]}
	case 3:
		return shader.Vec3{vals[0], vals[1], vals[2]}
	case 4:
		return shader.Vec4{vals[0], vals[1], vals[2], vals[3]}
	default:
		if len(vals) == 0 {
			return shader.Float32(0)
		}
		return shader.Vec4{vals[0], vals[1], vals[2], vals[3]}
	}
}

// shaderValueToFloats converts a shader Value into a float32 slice.
func shaderValueToFloats(val shader.Value) []float32 {
	switch v := val.(type) {
	case shader.Float32:
		return []float32{v}
	case shader.Vec2:
		return v[:]
	case shader.Vec3:
		return v[:]
	case shader.Vec4:
		return v[:]
	default:
		return []float32{0}
	}
}

// readVertexAttribute reads float values from buffer data at the given offset.
func readVertexAttribute(data []byte, offset uint64, format gputypes.VertexFormat) []float32 {
	if int(offset) >= len(data) {
		return nil
	}
	d := data[offset:]

	switch format {
	case gputypes.VertexFormatFloat32:
		if len(d) < 4 {
			return nil
		}
		return []float32{math.Float32frombits(binary.LittleEndian.Uint32(d))}

	case gputypes.VertexFormatFloat32x2:
		if len(d) < 8 {
			return nil
		}
		return []float32{
			math.Float32frombits(binary.LittleEndian.Uint32(d[0:])),
			math.Float32frombits(binary.LittleEndian.Uint32(d[4:])),
		}

	case gputypes.VertexFormatFloat32x3:
		if len(d) < 12 {
			return nil
		}
		return []float32{
			math.Float32frombits(binary.LittleEndian.Uint32(d[0:])),
			math.Float32frombits(binary.LittleEndian.Uint32(d[4:])),
			math.Float32frombits(binary.LittleEndian.Uint32(d[8:])),
		}

	case gputypes.VertexFormatFloat32x4:
		if len(d) < 16 {
			return nil
		}
		return []float32{
			math.Float32frombits(binary.LittleEndian.Uint32(d[0:])),
			math.Float32frombits(binary.LittleEndian.Uint32(d[4:])),
			math.Float32frombits(binary.LittleEndian.Uint32(d[8:])),
			math.Float32frombits(binary.LittleEndian.Uint32(d[12:])),
		}

	case gputypes.VertexFormatUnorm8x4:
		if len(d) < 4 {
			return nil
		}
		return []float32{
			float32(d[0]) / 255.0,
			float32(d[1]) / 255.0,
			float32(d[2]) / 255.0,
			float32(d[3]) / 255.0,
		}

	default:
		// Unsupported format, return zeros based on format size.
		n := int(format.Size() / 4)
		if n == 0 {
			n = 1
		}
		return make([]float32, n)
	}
}

// hasVertexColors returns true if the vertex layout has color-like attributes
// (4+ float components) beyond position (location 0).
func (r *RenderPassEncoder) hasVertexColors(layouts []gputypes.VertexBufferLayout) bool {
	if len(layouts) == 0 {
		return false
	}
	for _, attr := range layouts[0].Attributes {
		if attr.ShaderLocation == 0 {
			continue
		}
		// 3 or 4-component attribute is likely RGB/RGBA color.
		switch attr.Format {
		case gputypes.VertexFormatFloat32x3, gputypes.VertexFormatFloat32x4, gputypes.VertexFormatUnorm8x4:
			return true
		}
	}
	return false
}

// resolveFragmentColor determines the solid color for non-color-interpolated draws.
// Checks uniform buffers in bind groups for color data, falls back to white.
func (r *RenderPassEncoder) resolveFragmentColor() [4]float32 {
	// Try reading a color from the first uniform buffer in any bind group.
	for i := range r.bindGroups {
		bg := r.bindGroups[i]
		if bg == nil {
			continue
		}
		for _, buf := range bg.buffers {
			if buf == nil || len(buf.data) < 16 {
				continue
			}
			// Attempt to read 4 floats as RGBA color.
			buf.mu.RLock()
			d := buf.data
			cr := math.Float32frombits(binary.LittleEndian.Uint32(d[0:]))
			cg := math.Float32frombits(binary.LittleEndian.Uint32(d[4:]))
			cb := math.Float32frombits(binary.LittleEndian.Uint32(d[8:]))
			ca := math.Float32frombits(binary.LittleEndian.Uint32(d[12:]))
			buf.mu.RUnlock()

			// Sanity check: values should be in [0,1] range for normalized color.
			if cr >= 0 && cr <= 1 && cg >= 0 && cg <= 1 && cb >= 0 && cb <= 1 && ca >= 0 && ca <= 1 {
				return [4]float32{cr, cg, cb, ca}
			}
		}
	}

	return [4]float32{1, 1, 1, 1} // Default: white.
}

// executeSPIRVDraw handles draws where vertex positions come from SPIR-V
// shader execution with no vertex buffers bound (e.g. @builtin(vertex_index)
// triangle). Populates the interpreter's ExecutionContext with bind group
// resources (uniform buffers, textures, samplers) so shaders can access them.
// Returns true if the draw was handled, false if no SPIR-V module is available.
//
//nolint:maintidx // SPIR-V draw dispatch with instancing + resource binding is inherently complex.
func (r *RenderPassEncoder) executeSPIRVDraw(target *Texture, vertexCount, instanceCount, firstVertex, firstInstance uint32) bool {
	if r.pipeline == nil || r.pipeline.desc == nil {
		return false
	}

	// Get the vertex shader module.
	vsModule, ok := r.pipeline.desc.Vertex.Module.(*ShaderModule)
	if !ok || vsModule == nil {
		return false
	}

	parsed := vsModule.ParsedModule()
	if parsed == nil {
		return false
	}

	// Find the vertex shader entry point.
	vsEntry := r.pipeline.desc.Vertex.EntryPoint
	ep, ok := parsed.EntryPoints[vsEntry]
	if !ok || ep.ExecutionModel != shader.ExecutionModelVertex {
		return false
	}

	// Identify input/output variable IDs by their BuiltIn/Location decorations.
	var vertexIndexVarID, instanceIndexVarID, positionVarID uint32
	hasVertexIndex, hasInstanceIndex, hasPosition := false, false, false

	type locationVar struct {
		varID    uint32
		location int
	}
	var locationOutputs []locationVar

	for _, varID := range ep.InterfaceIDs {
		vi, exists := parsed.Variables[varID]
		if !exists {
			continue
		}
		builtIn := parsed.GetBuiltIn(varID)
		loc := parsed.GetLocation(varID)

		switch {
		case vi.StorageClass == shader.StorageClassInput && builtIn == shader.BuiltInVertexIndex:
			vertexIndexVarID = varID
			hasVertexIndex = true
		case vi.StorageClass == shader.StorageClassInput && builtIn == shader.BuiltInInstanceIndex:
			instanceIndexVarID = varID
			hasInstanceIndex = true
		case vi.StorageClass == shader.StorageClassOutput && builtIn == shader.BuiltInPosition:
			positionVarID = varID
			hasPosition = true
		case vi.StorageClass == shader.StorageClassOutput && loc >= 0:
			locationOutputs = append(locationOutputs, locationVar{varID: varID, location: loc})
		}
	}

	if !hasVertexIndex || !hasPosition {
		return false
	}

	// Build execution context with bind group resources (uniform buffers,
	// textures, samplers). This removes the old resource guard that rejected
	// shaders using Uniform/UniformConstant/StorageBuffer variables.
	ctx := r.buildExecutionContext()

	// Clear before drawing (triangles may not cover all pixels).
	if !r.cleared {
		r.applyClear()
	}

	w := int(target.width)
	h := int(target.height)

	pipe := raster.NewPipeline(w, h)

	// Copy current framebuffer so draws composite correctly.
	target.mu.RLock()
	existingData := make([]byte, len(target.data))
	copy(existingData, target.data)
	target.mu.RUnlock()
	pipe.Clear(0, 0, 0, 0)
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			idx := (py*w + px) * 4
			pipe.SetPixel(px, py, existingData[idx], existingData[idx+1], existingData[idx+2], existingData[idx+3])
		}
	}

	// Execute vertex shader for each (instance, vertex) pair.
	var allTriangles []raster.Triangle
	hasLocOutputs := len(locationOutputs) > 0

	for inst := uint32(0); inst < instanceCount; inst++ {
		instanceID := firstInstance + inst

		vertices := make([]raster.ScreenVertex, 0, vertexCount)
		for vert := uint32(0); vert < vertexCount; vert++ {
			vertexID := firstVertex + vert

			inputs := map[uint32]shader.Value{
				vertexIndexVarID: vertexID,
			}
			if hasInstanceIndex {
				inputs[instanceIndexVarID] = instanceID
			}

			ctx.Inputs = inputs
			outputs, err := parsed.ExecuteWithContext(vsEntry, ctx)
			if err != nil {
				return false
			}

			posVal, posOK := outputs[positionVarID]
			if !posOK {
				return false
			}
			pos := shader.Vec4ToFloat32(posVal)

			wClip := pos[3]
			if wClip == 0 {
				wClip = 1
			}
			ndcX := pos[0] / wClip
			ndcY := pos[1] / wClip
			ndcZ := pos[2] / wClip

			sx := (ndcX + 1.0) * 0.5 * float32(w)
			sy := (1.0 - ndcY) * 0.5 * float32(h)

			sv := raster.ScreenVertex{
				X: sx,
				Y: sy,
				Z: ndcZ,
				W: 1.0,
			}

			// Collect @location outputs as interpolated attributes.
			if hasLocOutputs {
				for _, lo := range locationOutputs {
					outVal, outOK := outputs[lo.varID]
					if !outOK {
						continue
					}
					sv.Attributes = append(sv.Attributes, shaderValueToFloats(outVal)...)
				}
				for len(sv.Attributes) < 4 {
					sv.Attributes = append(sv.Attributes, 1.0)
				}
			}

			vertices = append(vertices, sv)
		}

		instanceTris := r.verticesToTriangles(vertices)
		allTriangles = append(allTriangles, instanceTris...)
	}

	// Choose between interpolated attributes and flat fragment color.
	if hasLocOutputs {
		pipe.DrawTrianglesInterpolated(allTriangles)
	} else {
		fragColor := r.executeSPIRVFragment()
		pipe.DrawTriangles(allTriangles, fragColor)
	}

	// Write raster result back to texture in BGRA byte order.
	// Raster pipeline operates in RGBA; the surface framebuffer is BGRA
	// (required by GDI BitBlt on Windows and X11 ZPixmap on Linux).
	target.mu.Lock()
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			cr, cg, cb, ca := pipe.GetPixel(px, py)
			idx := (py*w + px) * 4
			target.data[idx+0] = cb // B
			target.data[idx+1] = cg // G
			target.data[idx+2] = cr // R
			target.data[idx+3] = ca // A
		}
	}
	target.mu.Unlock()

	return true
}

// executeSPIRVFragment runs the fragment shader entry point to get a color.
// Falls back to white if no fragment shader is available.
func (r *RenderPassEncoder) executeSPIRVFragment() [4]float32 {
	if r.pipeline.desc.Fragment == nil {
		return [4]float32{1, 1, 1, 1}
	}

	fsModuleSW, ok := r.pipeline.desc.Fragment.Module.(*ShaderModule)
	if !ok || fsModuleSW == nil {
		return [4]float32{1, 1, 1, 1}
	}

	parsed := fsModuleSW.ParsedModule()
	if parsed == nil {
		return [4]float32{1, 1, 1, 1}
	}

	fsEntry := r.pipeline.desc.Fragment.EntryPoint
	ep, ok := parsed.EntryPoints[fsEntry]
	if !ok || ep.ExecutionModel != shader.ExecutionModelFragment {
		return [4]float32{1, 1, 1, 1}
	}

	// Find the output variable (location 0).
	var colorVarID uint32
	hasColorOut := false
	for _, varID := range ep.InterfaceIDs {
		vi, ok := parsed.Variables[varID]
		if !ok || vi.StorageClass != shader.StorageClassOutput {
			continue
		}
		loc := parsed.GetLocation(varID)
		if loc == 0 {
			colorVarID = varID
			hasColorOut = true
			break
		}
	}

	if !hasColorOut {
		return [4]float32{1, 1, 1, 1}
	}

	outputs, err := parsed.Execute(fsEntry, nil)
	if err != nil {
		return [4]float32{1, 1, 1, 1}
	}

	colorVal, ok := outputs[colorVarID]
	if !ok {
		return [4]float32{1, 1, 1, 1}
	}

	return shader.Vec4ToFloat32(colorVal)
}
