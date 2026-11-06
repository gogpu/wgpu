//go:build !(js && wasm)

package software

import (
	"encoding/binary"
	"errors"
	"log/slog"
	"maps"
	"math"
	"slices"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/software/raster"
	"github.com/gogpu/wgpu/hal/software/shader"
)

// executeDraw is the core draw implementation.
// It selects between fullscreen texture blit and vertex-buffer-based rasterization.
// instanceCount > 1 enables instanced rendering; firstInstance offsets the instance ID.
func (r *RenderPassEncoder) executeDraw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	// Zero instances draw nothing, as on the GPU backends; an indirect record
	// a culling pass zeroed relies on it.
	if r.pipeline == nil || instanceCount == 0 {
		return
	}

	// A pass may have no color attachment at all (a depth-only shadow pass):
	// it still rasterizes into its depth/stencil attachment.
	w, h, ok := r.renderExtent()
	if !ok {
		return
	}
	target := r.getTargetTexture()

	// No vertex buffer bound — try fullscreen blit or SPIR-V shader path.
	if r.vertexBufs[0].buffer == nil {
		if target != nil && r.executeFullscreenBlit(target) {
			hal.Logger().Debug("software: executeDraw → Tier 0 blit (fast memcpy)",
				"width", target.width, "height", target.height)
			r.cleared = true
			return
		}

		if r.executeSPIRVDraw(w, h, vertexCount, instanceCount, firstVertex, firstInstance) {
			hal.Logger().Debug("software: executeDraw → SPIR-V path (no VB)",
				"vertices", vertexCount)
			return
		}
		if !r.cleared {
			r.applyClear()
		}
		hal.Logger().Debug("software: executeDraw → clear only (no VB, no texture)")
		return
	}

	// Vertex draw: triangles may not cover all pixels → clear first.
	if !r.cleared {
		r.applyClear()
	}
	r.executeVertexDraw(target, w, h, vertexCount, instanceCount, firstVertex, firstInstance)
}

// renderExtent returns the size of the pass's render area: the size of its
// attachments, which WebGPU requires to match. It takes the first color
// target, or the depth/stencil attachment's mip level when the pass has no
// color target. ok is false when the pass has neither.
func (r *RenderPassEncoder) renderExtent() (w, h int, ok bool) {
	for _, t := range r.colorTargets {
		if t != nil {
			return int(t.width), int(t.height), true
		}
	}
	tex, level, _, ok := depthAttachmentImage(r.desc.DepthStencilAttachment)
	if !ok {
		return 0, 0, false
	}
	dw, dh, _ := tex.levelExtent(level)
	return int(dw), int(dh), dw > 0 && dh > 0
}

// getTargetTexture returns the texture the first color attachment renders
// into (resolveColorTargets), or nil.
func (r *RenderPassEncoder) getTargetTexture() *Texture {
	if len(r.colorTargets) == 0 {
		return nil
	}
	return r.colorTargets[0]
}

// getTargetTextures returns the textures all color attachments render into;
// an entry is nil when the pass writes nothing for that attachment.
// Index i corresponds to @location(i) in the fragment shader. Callers must
// not modify the slice.
func (r *RenderPassEncoder) getTargetTextures() []*Texture {
	return r.colorTargets
}

// hasMRT returns true if the render pass has more than one color attachment.
func (r *RenderPassEncoder) hasMRT() bool {
	return len(r.desc.ColorAttachments) > 1
}

// executeFullscreenBlit blits the first bound texture to the target.
// This is the fast path for a pipeline without shader stages (no vertex
// buffer, texture in bind group). Returns true if blit was performed, false if
// no source texture was found (caller should apply clear instead).
//
// When scissor is active, only pixels within the scissor rect are written.
// The blit loop bounds are clamped to the scissor rect to maintain the
// fast-path advantage (no per-pixel branch, just narrowed iteration range).
// blitStateValid checks whether the current pipeline render state allows a
// direct memcpy blit (Skia SkSpriteBlitter_Memcpy::Supports pattern). Returns
// false if any active state would produce different results than a raw copy.
//
// A pipeline with a vertex or fragment shader always runs it: nothing proves
// that a shader is an identity copy of the first bound texture, and one that
// positions the quad, samples another coordinate or changes the color drew
// the wrong image when a blit stood in for it.
func (r *RenderPassEncoder) blitStateValid() bool {
	if r.pipeline == nil || r.pipeline.desc == nil {
		return false
	}
	desc := r.pipeline.desc
	if desc.Vertex.Module != nil || (desc.Fragment != nil && desc.Fragment.Module != nil) {
		return false
	}
	if desc.DepthStencil != nil {
		return false
	}
	// The blit copies the quad without per-triangle facing; a culled quad
	// must draw nothing.
	if desc.Primitive.CullMode != gputypes.CullModeNone {
		return false
	}
	if desc.Multisample.Count > 1 {
		return false
	}
	if desc.Fragment == nil {
		return true
	}
	for _, t := range desc.Fragment.Targets {
		if t.WriteMask != gputypes.ColorWriteMaskAll {
			return false
		}
		if !isBlitSafeBlend(t.Blend) {
			return false
		}
	}
	return true
}

// tryFullscreenBlit attempts Tier 1 fast-path: if 2 triangles form a fullscreen
// quad, the bound texture matches target dimensions, and render state is safe
// (which includes a pipeline without shader stages), copy the texture with a
// direct memcpy/swizzle.
func (r *RenderPassEncoder) tryFullscreenBlit(triangles []raster.Triangle, target *Texture, w, h int) bool {
	if target == nil || len(triangles) != 2 || !r.blitStateValid() || !isFullscreenQuad(triangles, w, h) {
		return false
	}
	srcView := r.findBoundTexture()
	if srcView == nil || srcView.texture == nil {
		return false
	}
	srcView.texture.mu.RLock()
	img, ok := srcView.baseImage()
	srcView.texture.mu.RUnlock()
	if !ok || int(img.width) != w || int(img.height) != h {
		return false
	}
	return r.executeFullscreenBlit(target)
}

// isFullscreenQuad checks whether 2 triangles form an axis-aligned quad covering
// the full render target. Tolerance of 1.5 pixels handles sub-pixel vertex positions.
// This is the Tier 1 detection from ADR-SOFTWARE-BLIT-OPTIMIZATION.
func isFullscreenQuad(triangles []raster.Triangle, w, h int) bool {
	if len(triangles) != 2 {
		return false
	}
	const eps = 1.5
	fw, fh := float32(w), float32(h)
	minX, minY := float32(fw), float32(fh)
	maxX, maxY := float32(0), float32(0)
	for _, tri := range triangles {
		for _, v := range [3]raster.ScreenVertex{tri.V0, tri.V1, tri.V2} {
			if v.X < minX {
				minX = v.X
			}
			if v.Y < minY {
				minY = v.Y
			}
			if v.X > maxX {
				maxX = v.X
			}
			if v.Y > maxY {
				maxY = v.Y
			}
		}
	}
	return minX < eps && minY < eps && maxX > fw-eps && maxY > fh-eps
}

// isBlitSafeBlend returns true if the blend state allows a direct memcpy blit.
// Safe modes: no blend (replace), Src (One/Zero), SrcOver (One/OneMinusSrcAlpha).
func isBlitSafeBlend(b *gputypes.BlendState) bool {
	if b == nil {
		return true
	}
	srcOver := b.Color.SrcFactor == gputypes.BlendFactorOne &&
		b.Color.DstFactor == gputypes.BlendFactorOneMinusSrcAlpha
	src := b.Color.SrcFactor == gputypes.BlendFactorOne &&
		b.Color.DstFactor == gputypes.BlendFactorZero
	return srcOver || src
}

func (r *RenderPassEncoder) executeFullscreenBlit(target *Texture) bool {
	if !r.blitStateValid() {
		return false
	}
	srcView := r.findBoundTexture()
	if srcView == nil || srcView.texture == nil {
		return false
	}

	// The view's base mip level and layer, not the whole texture storage.
	src := srcView.texture
	src.mu.RLock()
	img, ok := srcView.baseImage()
	src.mu.RUnlock()
	// The blit copies bytes, which equals a sampled draw only between 8-bit
	// RGBA or BGRA views of the same encoding: an sRGB view decodes on fetch
	// and encodes on write, a unorm one does neither.
	srcFmt := srcView.viewFormat()
	dstFmt := r.colorFormat(0)
	if !ok || !isBlitFormat(srcFmt) || !isBlitFormat(dstFmt) || srcFmt.IsSrgb() != dstFmt.IsSrgb() {
		return false
	}
	srcData := img.data
	srcW := int(img.width)
	srcH := int(img.height)

	dstW := int(target.width)
	dstH := int(target.height)

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

	// Determine blit bounds. Without scissor, the full destination is written.
	// With scissor, clamp iteration range to the scissor rect so that pixels
	// outside the rect remain untouched — no per-pixel branch needed.
	blitMinX, blitMinY := 0, 0
	blitMaxX, blitMaxY := dstW, dstH
	if r.hasScissor {
		blitMinX, blitMinY, blitMaxX, blitMaxY = clampBlitBounds(
			blitMinX, blitMinY, blitMaxX, blitMaxY, r.scissorRect)
		if blitMinX >= blitMaxX || blitMinY >= blitMaxY {
			return true // Scissor is fully outside — nothing to blit, but blit "succeeded"
		}
	}

	// Fast path: same-size blit without scissor (no scaling, no clipping).
	if srcW == dstW && srcH == dstH && !r.hasScissor {
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

	// General blit: handles scaling and/or scissor clipping.
	// Uses nearest-neighbor sampling with pre-computed column map.
	dstRowBytes := dstW * 4
	colMap := make([]int, dstW)
	for dx := blitMinX; dx < blitMaxX; dx++ {
		sx := dx * srcW / dstW
		if sx >= srcW {
			sx = srcW - 1
		}
		colMap[dx] = sx * 4
	}

	for dy := blitMinY; dy < blitMaxY; dy++ {
		sy := dy * srcH / dstH
		if sy >= srcH {
			sy = srcH - 1
		}

		srcRowOff := sy * srcW * 4
		if swizzle {
			for dx := blitMinX; dx < blitMaxX; dx++ {
				srcIdx := srcRowOff + colMap[dx]
				dstIdx := dy*dstRowBytes + dx*4
				target.data[dstIdx+0] = srcData[srcIdx+2]
				target.data[dstIdx+1] = srcData[srcIdx+1]
				target.data[dstIdx+2] = srcData[srcIdx+0]
				target.data[dstIdx+3] = srcData[srcIdx+3]
			}
		} else {
			for dx := blitMinX; dx < blitMaxX; dx++ {
				srcIdx := srcRowOff + colMap[dx]
				dstIdx := dy*dstRowBytes + dx*4
				target.data[dstIdx+0] = srcData[srcIdx+0]
				target.data[dstIdx+1] = srcData[srcIdx+1]
				target.data[dstIdx+2] = srcData[srcIdx+2]
				target.data[dstIdx+3] = srcData[srcIdx+3]
			}
		}
	}
	return true
}

// isBlitFormat reports whether the fullscreen blit can copy texels of format:
// the 8-bit RGBA and BGRA formats, unorm or sRGB.
func isBlitFormat(f gputypes.TextureFormat) bool {
	switch f {
	case gputypes.TextureFormatRGBA8Unorm, gputypes.TextureFormatRGBA8UnormSrgb,
		gputypes.TextureFormatBGRA8Unorm, gputypes.TextureFormatBGRA8UnormSrgb:
		return true
	}
	return false
}

// needsSwizzle returns true when source and destination formats require R/B channel swap.
func needsSwizzle(src, dst gputypes.TextureFormat) bool {
	srcBGRA := src == gputypes.TextureFormatBGRA8Unorm || src == gputypes.TextureFormatBGRA8UnormSrgb
	dstBGRA := dst == gputypes.TextureFormatBGRA8Unorm || dst == gputypes.TextureFormatBGRA8UnormSrgb
	return srcBGRA != dstBGRA
}

// isBGRA returns true when the texture format stores pixels in BGRA byte order.
// Used to determine whether raster pipeline output (RGBA) needs R↔B swap
// before writing to the target texture.
func isBGRA(fmt gputypes.TextureFormat) bool {
	return fmt == gputypes.TextureFormatBGRA8Unorm || fmt == gputypes.TextureFormatBGRA8UnormSrgb
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
func (r *RenderPassEncoder) executeVertexDraw(target *Texture, w, h int, vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	if r.pipeline.desc == nil {
		return
	}

	layouts := r.pipeline.desc.Vertex.Buffers
	if len(layouts) == 0 {
		// No vertex buffer layouts -- this draw was already handled by
		// executeSPIRVDraw in executeDraw. Nothing more to do.
		return
	}

	pipe := raster.NewPipeline(w, h)
	r.configureRasterPipeline(pipe)
	targets := r.routeColorOutput(pipe)

	// Attempt SPIR-V vertex shader path (handles @builtin + @location inputs
	// and per-vertex output attributes for interpolation).
	triangles := r.fetchTrianglesSPIRV(layouts, vertexCount, instanceCount, firstVertex, firstInstance, w, h)
	if triangles == nil {
		// Fallback: raw vertex buffer fetch (non-SPIR-V path, single instance only).
		triangles = r.fetchTriangles(layouts, vertexCount, firstVertex, w, h)
	}

	// Tier 1 fast-path: fullscreen textured quad → direct memcpy (#241).
	if r.tryFullscreenBlit(triangles, target, w, h) {
		return
	}

	// Determine fragment color source: if vertices carry attributes, interpolate.
	hasAttrs := false
	for i := range triangles {
		if len(triangles[i].V0.Attributes) >= 3 {
			hasAttrs = true
			break
		}
	}

	// MRT path: when multiple color attachments exist, use the MRT fragment
	// shader to write outputs for all @location(N) targets simultaneously.
	if r.hasMRT() && hasAttrs {
		if mrtFunc, mayDiscard := r.buildMRTFragmentShaderFunc(); mrtFunc != nil {
			pipe.DrawTrianglesWithMRTShader(triangles, mrtFunc, writeMRTLocations(targets), mayDiscard)
			return
		}
	}

	switch {
	case hasAttrs:
		// Try per-pixel fragment shader first; fall back to direct attribute interpolation.
		if fragFunc, mayDiscard := r.buildFragmentShaderFunc(); fragFunc != nil {
			pipe.DrawTrianglesWithFragmentShader(triangles, fragFunc, mayDiscard)
		} else {
			pipe.DrawTrianglesInterpolated(triangles)
		}
	case r.hasVertexColors(layouts):
		pipe.DrawTrianglesInterpolated(triangles)
	default:
		color := r.resolveFragmentColor()
		pipe.DrawTriangles(triangles, color)
	}
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

	// arrayStride 0 is WebGPU's broadcast: every vertex reads element 0.
	stride := layout.ArrayStride

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
		vi := r.drawVertexIndex(firstVertex, i)
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
func (r *RenderPassEncoder) fetchTrianglesSPIRV(
	layouts []gputypes.VertexBufferLayout,
	vertexCount, instanceCount, firstVertex, firstInstance uint32,
	targetW, targetH int,
) []raster.Triangle {
	if r.pipeline.desc == nil {
		return nil
	}

	// Get the vertex shader module.
	parsed := r.pipeline.vertex
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
			vertexID := r.drawVertexIndex(firstVertex, vert)

			// Build input map for this invocation.
			inputs := make(map[uint32]shader.Value)
			if hasVertexIndex {
				inputs[vertexIndexVarID] = shader.ValUint(vertexID)
			}
			if hasInstanceIndex {
				inputs[instanceIndexVarID] = shader.ValUint(instanceID)
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
				// Determine which index drives this buffer based on step mode.
				// arrayStride 0 is WebGPU's broadcast: every vertex and
				// instance reads element 0, so the index drops out.
				var idx uint32
				if ab.layout.StepMode == gputypes.VertexStepModeInstance {
					idx = instanceID
				} else {
					idx = vertexID
				}
				base := snap.offset + uint64(idx)*ab.layout.ArrayStride
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
// bind group resources (buffers, textures, samplers).
func (r *RenderPassEncoder) buildExecutionContext() *shader.ExecutionContext {
	return bindGroupExecutionContext(&r.bindGroups)
}

// bindGroupExecutionContext creates a shader ExecutionContext with the
// resources of the bound groups, for draws and dispatches alike.
func bindGroupExecutionContext(groups *[4]*BindGroup) *shader.ExecutionContext {
	ctx := &shader.ExecutionContext{
		Buffers:  make(map[shader.BindingKey][]byte),
		Textures: make(map[shader.BindingKey]*shader.Texture2D),
		Samplers: make(map[shader.BindingKey]*shader.Sampler),
	}
	for groupIdx, bg := range groups {
		if bg == nil {
			continue
		}
		addBufferBindings(ctx, uint32(groupIdx), bg)
		addTextureBindings(ctx, uint32(groupIdx), bg)
	}
	return ctx
}

// addBufferBindings adds the buffer bindings of bind group groupIdx to ctx,
// each the slice of the buffer's storage its offset, dynamic offset and
// size select, so storage buffer writes land in the buffer itself. Dynamic
// offsets are consumed in binding order by the bindings with
// HasDynamicOffset, as WebGPU orders them.
func addBufferBindings(ctx *shader.ExecutionContext, groupIdx uint32, bg *BindGroup) {
	dynIdx := 0
	for _, bindingIdx := range slices.Sorted(maps.Keys(bg.bufferBindings)) {
		bs := bg.bufferBindings[bindingIdx]
		if bs.buf == nil {
			continue
		}
		off := bs.offset
		if bg.hasDynamicOffset[bindingIdx] && dynIdx < len(bg.dynamicOffsets) {
			off += uint64(bg.dynamicOffsets[dynIdx])
			dynIdx++
		}
		bs.buf.mu.RLock()
		data := bs.buf.data
		end := uint64(len(data))
		if bs.size > 0 && off+bs.size <= end {
			end = off + bs.size
		}
		if off < uint64(len(data)) {
			data = data[off:end]
		}
		ctx.Buffers[shader.BindingKey{Group: groupIdx, Binding: bindingIdx}] = data
		bs.buf.mu.RUnlock()
	}
}

// addTextureBindings adds the texture views and samplers of bind group
// groupIdx to ctx. A texture's Data aliases the texture's storage, so a
// storage texture write (OpImageWrite) lands in the texture itself.
func addTextureBindings(ctx *shader.ExecutionContext, groupIdx uint32, bg *BindGroup) {
	for bindingIdx, tv := range bg.textureViews {
		if tv == nil || tv.texture == nil {
			continue
		}
		tv.texture.mu.RLock()
		ctx.Textures[shader.BindingKey{Group: groupIdx, Binding: bindingIdx}] = viewToShader(tv)
		tv.texture.mu.RUnlock()
	}
	for bindingIdx, samp := range bg.samplers {
		if samp == nil {
			continue
		}
		ctx.Samplers[shader.BindingKey{Group: groupIdx, Binding: bindingIdx}] = samplerResourceToShader(samp)
	}
}

// viewToShader describes texture view tv to the interpreter: the view's mip
// levels and array layers (or the depth slices of a 3D view), each level's
// data aliasing the texture's storage. Levels is filled only for a view of
// more than one level, so the common single-level binding allocates nothing
// beyond the Texture2D itself. The caller holds the texture's lock.
func viewToShader(tv *TextureView) *shader.Texture2D {
	t := tv.texture
	base, mips := tv.baseMipLevel, tv.mipLevels()
	is3D := t.is3D()
	first, layers := tv.baseArrayLayer, tv.arrayLayers()
	if is3D {
		first = 0
	}
	images := func(level uint32) uint32 {
		if !is3D {
			return layers
		}
		_, _, depth := t.levelExtent(level)
		return depth
	}
	w, h, _ := t.levelExtent(base)
	st := &shader.Texture2D{
		Width:  w,
		Height: h,
		Data:   t.levelData(base, first, images(base)),
		// The view's format: an sRGB view of a unorm texture decodes on
		// fetch, a unorm view of an sRGB texture reads the raw bytes. The
		// stride is the texture's storage.
		Format:        uint32(tv.viewFormat()),
		Aspect:        uint32(tv.viewAspect()),
		BytesPerPixel: uint32(formatBytesPerPixel(t.format)),
		MipLevels:     mips,
		SampleCount:   t.sampleCount,
		Layers:        layers,
		Dimension:     viewDimensionToShader(tv.viewDimension()),
	}
	if is3D {
		st.Layers, st.Depth = 1, images(base)
	}
	if mips > 1 {
		st.Levels = make([]shader.TextureLevel, mips)
		for i := range mips {
			lw, lh, _ := t.levelExtent(base + i)
			n := images(base + i)
			st.Levels[i] = shader.TextureLevel{Width: lw, Height: lh, Images: n, Data: t.levelData(base+i, first, n)}
		}
	}
	return st
}

// viewDimensionToShader converts a view dimension to the interpreter's
// TexDim constant.
func viewDimensionToShader(d gputypes.TextureViewDimension) uint32 {
	switch d {
	case gputypes.TextureViewDimension1D:
		return shader.TexDim1D
	case gputypes.TextureViewDimension2DArray:
		return shader.TexDim2DArray
	case gputypes.TextureViewDimensionCube:
		return shader.TexDimCube
	case gputypes.TextureViewDimensionCubeArray:
		return shader.TexDimCubeArray
	case gputypes.TextureViewDimension3D:
		return shader.TexDim3D
	default:
		return shader.TexDim2D
	}
}

// samplerResourceToShader converts a SamplerResource to a shader.Sampler
// using the addressing, filtering and LOD clamps from the HAL descriptor.
// The clamps are taken literally, like every backend's: the root package
// resolves the public descriptor's zero LodMaxClamp to 32.
func samplerResourceToShader(s *SamplerResource) *shader.Sampler {
	if s == nil || s.Desc == nil {
		return &shader.Sampler{LodMaxClamp: shader.LodClampNone} // Default: nearest, repeat, no LOD clamp.
	}
	return &shader.Sampler{
		MinFilter:    filterModeToShader(s.Desc.MinFilter),
		MagFilter:    filterModeToShader(s.Desc.MagFilter),
		MipmapFilter: filterModeToShader(s.Desc.MipmapFilter),
		WrapU:        wrapModeToShader(s.Desc.AddressModeU),
		WrapV:        wrapModeToShader(s.Desc.AddressModeV),
		WrapW:        wrapModeToShader(s.Desc.AddressModeW),
		LodMinClamp:  s.Desc.LodMinClamp,
		LodMaxClamp:  s.Desc.LodMaxClamp,
	}
}

// filterModeToShader converts a gputypes.FilterMode to shader filter constant.
func filterModeToShader(mode gputypes.FilterMode) uint32 {
	switch mode {
	case gputypes.FilterModeLinear:
		return shader.FilterLinear
	default:
		return shader.FilterNearest
	}
}

// wrapModeToShader converts a gputypes.AddressMode to shader wrap constant.
func wrapModeToShader(mode gputypes.AddressMode) uint32 {
	switch mode {
	case gputypes.AddressModeClampToEdge:
		return shader.WrapClampToEdge
	case gputypes.AddressModeMirrorRepeat:
		return shader.WrapMirroredRepeat
	default: // AddressModeRepeat or undefined
		return shader.WrapRepeat
	}
}

// floatsToShaderValue converts a float32 slice into the appropriate shader Value type.
func floatsToShaderValue(vals []float32) shader.Value {
	switch len(vals) {
	case 1:
		return shader.ValFloat(vals[0])
	case 2:
		return shader.ValVec2(vals[0], vals[1])
	case 3:
		return shader.ValVec3(vals[0], vals[1], vals[2])
	case 4:
		return shader.ValVec4(vals[0], vals[1], vals[2], vals[3])
	default:
		if len(vals) == 0 {
			return shader.ValFloat(0)
		}
		return shader.ValVec4(vals[0], vals[1], vals[2], vals[3])
	}
}

// shaderValueToFloats converts a shader Value into a float32 slice.
func shaderValueToFloats(val shader.Value) []float32 {
	switch val.Tag {
	case shader.TagFloat32:
		return []float32{val.F[0]}
	case shader.TagVec2:
		v := val.AsVec2()
		return v[:]
	case shader.TagVec3:
		v := val.AsVec3()
		return v[:]
	case shader.TagVec4:
		v := val.AsVec4()
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
		for _, bs := range bg.bufferBindings {
			if bs.buf == nil {
				continue
			}
			bs.buf.mu.RLock()
			d := bs.buf.data
			if bs.offset > 0 && bs.offset < uint64(len(d)) {
				d = d[bs.offset:]
			}
			if len(d) < 16 {
				bs.buf.mu.RUnlock()
				continue
			}
			// Attempt to read 4 floats as RGBA color.
			cr := math.Float32frombits(binary.LittleEndian.Uint32(d[0:]))
			cg := math.Float32frombits(binary.LittleEndian.Uint32(d[4:]))
			cb := math.Float32frombits(binary.LittleEndian.Uint32(d[8:]))
			ca := math.Float32frombits(binary.LittleEndian.Uint32(d[12:]))
			bs.buf.mu.RUnlock()

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
// spirvDrawSetup holds the parsed vertex shader state needed by executeSPIRVDraw.
type spirvDrawSetup struct {
	parsed             *shader.Module
	vsEntry            string
	vertexIndexVarID   uint32
	instanceIndexVarID uint32
	positionVarID      uint32
	hasInstanceIndex   bool
	locationOutputs    []locationVar
}

type locationVar struct {
	varID    uint32
	location int
}

// prepareSPIRVDraw validates the pipeline and resolves vertex shader interface variables.
func (r *RenderPassEncoder) prepareSPIRVDraw() *spirvDrawSetup {
	if r.pipeline == nil || r.pipeline.desc == nil {
		return nil
	}
	parsed := r.pipeline.vertex
	if parsed == nil {
		return nil
	}
	vsEntry := r.pipeline.desc.Vertex.EntryPoint
	ep, ok := parsed.EntryPoints[vsEntry]
	if !ok || ep.ExecutionModel != shader.ExecutionModelVertex {
		return nil
	}

	s := &spirvDrawSetup{parsed: parsed, vsEntry: vsEntry}
	var hasVertexIndex, hasPosition bool

	for _, varID := range ep.InterfaceIDs {
		vi, exists := parsed.Variables[varID]
		if !exists {
			continue
		}
		builtIn := parsed.GetBuiltIn(varID)
		loc := parsed.GetLocation(varID)

		switch {
		case vi.StorageClass == shader.StorageClassInput && builtIn == shader.BuiltInVertexIndex:
			s.vertexIndexVarID = varID
			hasVertexIndex = true
		case vi.StorageClass == shader.StorageClassInput && builtIn == shader.BuiltInInstanceIndex:
			s.instanceIndexVarID = varID
			s.hasInstanceIndex = true
		case vi.StorageClass == shader.StorageClassOutput && builtIn == shader.BuiltInPosition:
			s.positionVarID = varID
			hasPosition = true
		case vi.StorageClass == shader.StorageClassOutput && loc >= 0:
			s.locationOutputs = append(s.locationOutputs, locationVar{varID: varID, location: loc})
		}
	}
	if !hasVertexIndex || !hasPosition {
		return nil
	}
	return s
}

func (r *RenderPassEncoder) executeSPIRVDraw(w, h int, vertexCount, instanceCount, firstVertex, firstInstance uint32) bool {
	s := r.prepareSPIRVDraw()
	if s == nil {
		return false
	}

	ctx := r.buildExecutionContext()

	if !r.cleared {
		r.applyClear()
	}

	pipe := raster.NewPipeline(w, h)
	r.configureRasterPipeline(pipe)
	targets := r.routeColorOutput(pipe)

	var allTriangles []raster.Triangle
	hasLocOutputs := len(s.locationOutputs) > 0

	for inst := uint32(0); inst < instanceCount; inst++ {
		instanceID := firstInstance + inst

		vertices := make([]raster.ScreenVertex, 0, vertexCount)
		for vert := uint32(0); vert < vertexCount; vert++ {
			vertexID := r.drawVertexIndex(firstVertex, vert)

			inputs := map[uint32]shader.Value{
				s.vertexIndexVarID: shader.ValUint(vertexID),
			}
			if s.hasInstanceIndex {
				inputs[s.instanceIndexVarID] = shader.ValUint(instanceID)
			}

			ctx.Inputs = inputs
			outputs, err := s.parsed.ExecuteWithContext(s.vsEntry, ctx)
			if err != nil {
				return false
			}

			posVal, posOK := outputs[s.positionVarID]
			if !posOK {
				return false
			}
			pos := shader.Vec4ToFloat32(posVal)

			wClip := pos[3]
			if wClip == 0 {
				wClip = 1
			}
			sv := raster.ScreenVertex{
				X: (pos[0]/wClip + 1.0) * 0.5 * float32(w),
				Y: (1.0 - pos[1]/wClip) * 0.5 * float32(h),
				Z: pos[2] / wClip,
				W: 1.0,
			}

			if hasLocOutputs {
				for _, lo := range s.locationOutputs {
					if outVal, outOK := outputs[lo.varID]; outOK {
						sv.Attributes = append(sv.Attributes, shaderValueToFloats(outVal)...)
					}
				}
				for len(sv.Attributes) < 4 {
					sv.Attributes = append(sv.Attributes, 1.0)
				}
			}

			vertices = append(vertices, sv)
		}

		allTriangles = append(allTriangles, r.verticesToTriangles(vertices)...)
	}

	// MRT path for SPIR-V draws with multiple color attachments.
	if r.hasMRT() {
		if mrtFunc, mayDiscard := r.buildMRTFragmentShaderFunc(); mrtFunc != nil {
			pipe.DrawTrianglesWithMRTShader(allTriangles, mrtFunc, writeMRTLocations(targets), mayDiscard)
			return true
		}
	}

	// A fragment shader that reads any per-fragment input (an interpolated
	// @location or a builtin such as the position) runs per pixel; one that
	// reads none gives the same result everywhere and runs once.
	switch fragFunc, mayDiscard := r.buildFragmentShaderFunc(); {
	case fragFunc != nil:
		pipe.DrawTrianglesWithFragmentShader(allTriangles, fragFunc, mayDiscard)
	case hasLocOutputs && r.pipeline.fragment == nil:
		pipe.DrawTrianglesInterpolated(allTriangles)
	default:
		if color, discard := r.executeSPIRVFragment(); !discard {
			pipe.DrawTriangles(allTriangles, color)
		}
	}
	return true
}

// executeSPIRVFragment runs the fragment shader entry point to get a color.
// Falls back to white if no fragment shader is available.
// It is used only when the shader reads no per-fragment input (see
// buildFragmentShaderFunc). discard reports that the invocation discarded,
// which then holds for every fragment of the draw.
func (r *RenderPassEncoder) executeSPIRVFragment() (color [4]float32, discard bool) {
	if r.pipeline.desc.Fragment == nil {
		return [4]float32{1, 1, 1, 1}, false
	}

	parsed := r.pipeline.fragment
	if parsed == nil {
		return [4]float32{1, 1, 1, 1}, false
	}

	fsEntry := r.pipeline.desc.Fragment.EntryPoint
	ep, ok := parsed.EntryPoints[fsEntry]
	if !ok || ep.ExecutionModel != shader.ExecutionModelFragment {
		return [4]float32{1, 1, 1, 1}, false
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
		return [4]float32{1, 1, 1, 1}, false
	}

	// No per-fragment input, so one invocation stands for every fragment of
	// the draw; it still reads the bound resources (uniforms, textures).
	outputs, err := parsed.ExecuteWithContext(fsEntry, r.buildExecutionContext())
	if errors.Is(err, shader.ErrDiscarded) {
		return [4]float32{}, true
	}
	if err != nil {
		return [4]float32{1, 1, 1, 1}, false
	}

	colorVal, ok := outputs[colorVarID]
	if !ok {
		return [4]float32{1, 1, 1, 1}, false
	}

	return fragmentColor(colorVal, colorKindOf(r.colorFormat(0))), false
}

// fragmentColor returns a fragment shader color output as the four channels
// the color target of kind stores (colorTarget): float values, or for an
// integer target the bits of the 32-bit integers. A scalar u32 or i32 output
// holds its value in Value.U and stays exact. The interpreter holds integer
// vectors as float32 (TagVec*, P1-SW-019), so their channels are exact only
// up to 2^24.
func fragmentColor(v shader.Value, kind colorKind) [4]float32 {
	if kind == colorKindFloat {
		switch v.Tag {
		case shader.TagUint32:
			return [4]float32{float32(v.U[0]), 0, 0, 1}
		case shader.TagInt32:
			return [4]float32{float32(int32(v.U[0])), 0, 0, 1}
		}
		return shader.Vec4ToFloat32(v)
	}
	if v.Tag == shader.TagUint32 || v.Tag == shader.TagInt32 {
		return intBitsColor([4]uint32{v.U[0], 0, 0, 1})
	}
	f := shader.Vec4ToFloat32(v)
	var bits [4]uint32
	for i, c := range f {
		if kind == colorKindSint {
			bits[i] = uint32(int32(c))
		} else {
			bits[i] = uint32(int64(c))
		}
	}
	return intBitsColor(bits)
}

// fragmentInterface classifies the interface variables of a fragment entry
// point: the @location inputs fed from interpolated attributes, the position
// builtin (FragCoord) and the @location outputs.
type fragmentInterface struct {
	parsed         *shader.Module
	entry          string
	locationInputs []fragmentLocationVar
	outputs        []fragmentLocationVar
	fragCoordVar   uint32 // 0 when the shader does not read @builtin(position)
	hasInputs      bool   // any Input variable, location or builtin
}

type fragmentLocationVar struct {
	varID    uint32
	location int
}

// fragmentInterfaceOf returns the classified interface of the pipeline's
// fragment entry point, or false when there is no SPIR-V fragment stage.
func (r *RenderPassEncoder) fragmentInterfaceOf() (fragmentInterface, bool) {
	if r.pipeline == nil || r.pipeline.desc == nil || r.pipeline.desc.Fragment == nil || r.pipeline.fragment == nil {
		return fragmentInterface{}, false
	}
	parsed := r.pipeline.fragment
	fi := fragmentInterface{parsed: parsed, entry: r.pipeline.desc.Fragment.EntryPoint}
	ep, ok := parsed.EntryPoints[fi.entry]
	if !ok || ep.ExecutionModel != shader.ExecutionModelFragment {
		return fragmentInterface{}, false
	}
	for _, varID := range ep.InterfaceIDs {
		vi, exists := parsed.Variables[varID]
		if !exists {
			continue
		}
		loc := parsed.GetLocation(varID)
		switch vi.StorageClass {
		case shader.StorageClassInput:
			fi.hasInputs = true
			if loc >= 0 {
				fi.locationInputs = append(fi.locationInputs, fragmentLocationVar{varID: varID, location: loc})
			} else if parsed.GetBuiltIn(varID) == shader.BuiltInFragCoord {
				fi.fragCoordVar = varID
			}
		case shader.StorageClassOutput:
			if loc >= 0 {
				fi.outputs = append(fi.outputs, fragmentLocationVar{varID: varID, location: loc})
			}
		}
	}
	return fi, true
}

// inputs builds the input values of one fragment invocation. Interpolated
// attributes are the flat concatenation of the vertex @location outputs in
// location order; they are handed back by component count. The position
// builtin is the pixel center in framebuffer coordinates, the depth and 1/w.
func (fi *fragmentInterface) inputs(frag *raster.Fragment) map[uint32]shader.Value {
	inputs := make(map[uint32]shader.Value, len(fi.locationInputs)+1)
	attrs := frag.Attributes
	offset := 0
	for _, li := range fi.locationInputs {
		if offset >= len(attrs) {
			break
		}
		width := fi.parsed.GetTypeComponentCount(li.varID)
		if width == 0 {
			width = 4
		}
		end := min(offset+width, len(attrs))
		inputs[li.varID] = floatsToShaderValue(attrs[offset:end])
		offset = end
	}
	if fi.fragCoordVar != 0 {
		inputs[fi.fragCoordVar] = floatsToShaderValue([]float32{
			float32(frag.X) + 0.5, float32(frag.Y) + 0.5, frag.Depth, frag.InvW,
		})
	}
	return inputs
}

// buildFragmentShaderFunc creates a raster.FragmentShaderFunc that executes
// the SPIR-V fragment shader per pixel and returns the location 0 color, and
// whether the shader may discard (the raster then runs it before the depth
// and stencil tests).
//
// Returns nil when the shader reads no per-fragment input (no @location input
// and no builtin): every fragment then gets the same result, and the caller
// uses executeSPIRVFragment once for the draw.
func (r *RenderPassEncoder) buildFragmentShaderFunc() (raster.FragmentShaderFunc, bool) {
	fi, ok := r.fragmentInterfaceOf()
	if !ok || !fi.hasInputs {
		return nil, false
	}
	var colorOutputVarID uint32
	for _, o := range fi.outputs {
		if o.location == 0 {
			colorOutputVarID = o.varID
		}
	}
	if colorOutputVarID == 0 {
		return nil, false
	}

	// Build execution context with bind group resources (uniform buffers, textures, etc.).
	ctx := r.buildExecutionContext()
	kind := colorKindOf(r.colorFormat(0))

	return func(frag *raster.Fragment) ([4]float32, bool) {
		ctx.Inputs = fi.inputs(frag)
		outputs, err := fi.parsed.ExecuteWithContext(fi.entry, ctx)
		if errors.Is(err, shader.ErrDiscarded) {
			return [4]float32{}, true
		}
		if err != nil {
			return [4]float32{1, 1, 1, 1}, false
		}

		colorVal, ok := outputs[colorOutputVarID]
		if !ok {
			return [4]float32{1, 1, 1, 1}, false
		}
		return fragmentColor(colorVal, kind), false
	}, r.pipeline.fragmentMayDiscard
}

// buildMRTFragmentShaderFunc creates a raster.MRTFragmentShaderFunc that executes
// the SPIR-V fragment shader per pixel and returns colors for ALL @location(N)
// outputs, supporting Multiple Render Targets. The returned slice is indexed by
// location number (0..N).
//
// Returns nil, as buildFragmentShaderFunc does, when the shader reads no
// per-fragment input or has no @location output. The second result reports
// whether the shader may discard.
func (r *RenderPassEncoder) buildMRTFragmentShaderFunc() (raster.MRTFragmentShaderFunc, bool) {
	fi, ok := r.fragmentInterfaceOf()
	if !ok || !fi.hasInputs || len(fi.outputs) == 0 {
		return nil, false
	}

	// Determine the max output location to size the result slice.
	maxLoc := 0
	for _, lo := range fi.outputs {
		if lo.location > maxLoc {
			maxLoc = lo.location
		}
	}
	numOutputs := maxLoc + 1

	ctx := r.buildExecutionContext()
	kinds := r.colorKinds()

	return func(frag *raster.Fragment) ([][4]float32, bool) {
		ctx.Inputs = fi.inputs(frag)
		outputs, err := fi.parsed.ExecuteWithContext(fi.entry, ctx)
		if errors.Is(err, shader.ErrDiscarded) {
			return nil, true
		}
		if err != nil {
			// On error, return white for all targets.
			result := make([][4]float32, numOutputs)
			for i := range result {
				result[i] = [4]float32{1, 1, 1, 1}
			}
			return result, false
		}

		// Collect outputs by location.
		result := make([][4]float32, numOutputs)
		for _, lo := range fi.outputs {
			colorVal, ok := outputs[lo.varID]
			if !ok {
				result[lo.location] = [4]float32{0, 0, 0, 0}
				continue
			}
			result[lo.location] = fragmentColor(colorVal, kindAt(kinds, lo.location))
		}
		return result, false
	}, r.pipeline.fragmentMayDiscard
}

// clampBlitBounds narrows blit iteration bounds to the scissor rectangle.
// Returns the clamped (minX, minY, maxX, maxY).
func clampBlitBounds(minX, minY, maxX, maxY int, scissor [4]uint32) (int, int, int, int) {
	sx := int(scissor[0])
	sy := int(scissor[1])
	sw := int(scissor[2])
	sh := int(scissor[3])
	if sx > minX {
		minX = sx
	}
	if sy > minY {
		minY = sy
	}
	if sx+sw < maxX {
		maxX = sx + sw
	}
	if sy+sh < maxY {
		maxY = sy + sh
	}
	return minX, minY, maxX, maxY
}

// configureRasterPipeline applies scissor, depth, stencil, and blend state
// from the command encoder to a newly created raster.Pipeline. This is the
// single point where WebGPU render pass state is translated to the raster
// package's config.
func (r *RenderPassEncoder) configureRasterPipeline(pipe *raster.Pipeline) {
	// Scissor: clip fragments to the scissor rectangle set by SetScissorRect.
	if r.hasScissor {
		pipe.SetScissor(&raster.Rect{
			X:      int(r.scissorRect[0]),
			Y:      int(r.scissorRect[1]),
			Width:  int(r.scissorRect[2]),
			Height: int(r.scissorRect[3]),
		})
	}

	if r.pipeline == nil || r.pipeline.desc == nil {
		return
	}

	// Face culling. Set before the DepthStencil early return below: a
	// pipeline without depth/stencil culls too (P1-SW-014 — the raster
	// pipeline stayed at CullNone for every draw).
	pipe.SetCullMode(convertCullMode(r.pipeline.desc.Primitive.CullMode))
	pipe.SetFrontFace(convertFrontFace(r.pipeline.desc.Primitive.FrontFace))

	// Blending happens in the color targets (routeColorOutput), in each
	// attachment's format.

	// Depth and stencil state from the render pipeline descriptor.
	if r.pipeline.desc.DepthStencil == nil {
		return
	}
	ds := r.pipeline.desc.DepthStencil

	// Every draw of the pass tests against and writes into the pass's depth
	// buffer, the depth attachment's contents.
	if d := r.passDepthBuffer; d != nil && d.Width() == pipe.Width() && d.Height() == pipe.Height() {
		pipe.SetDepthBuffer(d)
	}

	// Depth testing.
	depthCompare := convertCompareFunction(ds.DepthCompare)
	depthEnabled := ds.DepthCompare != gputypes.CompareFunctionUndefined &&
		ds.DepthCompare != gputypes.CompareFunctionAlways
	pipe.SetDepthTest(depthEnabled, depthCompare)
	pipe.SetDepthWrite(ds.DepthWriteEnabled)

	// Stencil state: use front-face state (software rasterizer does not
	// distinguish front/back face stencil; use front as the common case).
	sf := ds.StencilFront
	stencilEnabled := isStencilEnabled(sf)
	if stencilEnabled && r.passStencilBuffer != nil {
		// Use the persistent stencil buffer created at BeginRenderPass.
		// This is the same buffer for ALL draw calls in this pass, matching
		// GPU behavior where the depth/stencil attachment texture persists.
		pipe.SetStencilBuffer(r.passStencilBuffer)
		pipe.SetStencilState(raster.StencilState{
			Enabled:     true,
			ReadMask:    uint8(ds.StencilReadMask),
			WriteMask:   uint8(ds.StencilWriteMask),
			Compare:     convertCompareFunction(sf.Compare),
			FailOp:      convertStencilOp(sf.FailOp),
			DepthFailOp: convertStencilOp(sf.DepthFailOp),
			PassOp:      convertStencilOp(sf.PassOp),
			Reference:   uint8(r.stencilRef),
		})
	}
}

// isStencilEnabled returns true if the stencil face state defines any
// non-trivial operations (not all Keep + Always).
func isStencilEnabled(sf hal.StencilFaceState) bool {
	// WebGPU default: Compare=Always, all ops=Keep. This is effectively disabled.
	if sf.Compare == gputypes.CompareFunctionAlways &&
		sf.FailOp == hal.StencilOperationKeep &&
		sf.DepthFailOp == hal.StencilOperationKeep &&
		sf.PassOp == hal.StencilOperationKeep {
		return false
	}
	// Also treat Undefined compare as disabled.
	if sf.Compare == gputypes.CompareFunctionUndefined {
		return false
	}
	return true
}

// convertCullMode maps gputypes.CullMode to raster.CullMode. The values differ
// (raster uses iota), so the mapping is explicit.
func convertCullMode(m gputypes.CullMode) raster.CullMode {
	switch m {
	case gputypes.CullModeFront:
		return raster.CullFront
	case gputypes.CullModeBack:
		return raster.CullBack
	default:
		return raster.CullNone
	}
}

// convertFrontFace maps gputypes.FrontFace to raster.FrontFace.
func convertFrontFace(f gputypes.FrontFace) raster.FrontFace {
	if f == gputypes.FrontFaceCW {
		return raster.FrontFaceCW
	}
	return raster.FrontFaceCCW
}

// convertCompareFunction maps gputypes.CompareFunction to raster.CompareFunc.
func convertCompareFunction(cf gputypes.CompareFunction) raster.CompareFunc {
	switch cf {
	case gputypes.CompareFunctionNever:
		return raster.CompareNever
	case gputypes.CompareFunctionLess:
		return raster.CompareLess
	case gputypes.CompareFunctionEqual:
		return raster.CompareEqual
	case gputypes.CompareFunctionLessEqual:
		return raster.CompareLessEqual
	case gputypes.CompareFunctionGreater:
		return raster.CompareGreater
	case gputypes.CompareFunctionNotEqual:
		return raster.CompareNotEqual
	case gputypes.CompareFunctionGreaterEqual:
		return raster.CompareGreaterEqual
	case gputypes.CompareFunctionAlways:
		return raster.CompareAlways
	default:
		return raster.CompareAlways
	}
}

// convertStencilOp maps gputypes.StencilOperation to raster.StencilOp.
func convertStencilOp(op gputypes.StencilOperation) raster.StencilOp {
	switch op {
	case hal.StencilOperationZero:
		return raster.StencilOpZero
	case hal.StencilOperationReplace:
		return raster.StencilOpReplace
	case hal.StencilOperationInvert:
		return raster.StencilOpInvert
	case hal.StencilOperationIncrementClamp:
		return raster.StencilOpIncrementClamp
	case hal.StencilOperationDecrementClamp:
		return raster.StencilOpDecrementClamp
	case hal.StencilOperationIncrementWrap:
		return raster.StencilOpIncrementWrap
	case hal.StencilOperationDecrementWrap:
		return raster.StencilOpDecrementWrap
	default: // Keep or undefined
		return raster.StencilOpKeep
	}
}

// convertBlendState maps a gputypes.BlendState to raster.BlendState.
func convertBlendState(bs *gputypes.BlendState) raster.BlendState {
	return raster.BlendState{
		Enabled:  true,
		SrcColor: convertBlendFactor(bs.Color.SrcFactor),
		DstColor: convertBlendFactor(bs.Color.DstFactor),
		ColorOp:  convertBlendOp(bs.Color.Operation),
		SrcAlpha: convertBlendFactor(bs.Alpha.SrcFactor),
		DstAlpha: convertBlendFactor(bs.Alpha.DstFactor),
		AlphaOp:  convertBlendOp(bs.Alpha.Operation),
	}
}

// convertBlendFactor maps gputypes.BlendFactor to raster.BlendFactor.
func convertBlendFactor(f gputypes.BlendFactor) raster.BlendFactor {
	switch f {
	case gputypes.BlendFactorZero:
		return raster.BlendFactorZero
	case gputypes.BlendFactorOne:
		return raster.BlendFactorOne
	case gputypes.BlendFactorSrc:
		return raster.BlendFactorSrc
	case gputypes.BlendFactorOneMinusSrc:
		return raster.BlendFactorOneMinusSrc
	case gputypes.BlendFactorSrcAlpha:
		return raster.BlendFactorSrcAlpha
	case gputypes.BlendFactorOneMinusSrcAlpha:
		return raster.BlendFactorOneMinusSrcAlpha
	case gputypes.BlendFactorDst:
		return raster.BlendFactorDst
	case gputypes.BlendFactorOneMinusDst:
		return raster.BlendFactorOneMinusDst
	case gputypes.BlendFactorDstAlpha:
		return raster.BlendFactorDstAlpha
	case gputypes.BlendFactorOneMinusDstAlpha:
		return raster.BlendFactorOneMinusDstAlpha
	case gputypes.BlendFactorSrcAlphaSaturated:
		return raster.BlendFactorSrcAlphaSaturated
	case gputypes.BlendFactorConstant:
		return raster.BlendFactorConstant
	case gputypes.BlendFactorOneMinusConstant:
		return raster.BlendFactorOneMinusConstant
	default:
		return raster.BlendFactorOne
	}
}

// convertBlendOp maps gputypes.BlendOperation to raster.BlendOperation.
func convertBlendOp(op gputypes.BlendOperation) raster.BlendOperation {
	switch op {
	case gputypes.BlendOperationAdd:
		return raster.BlendOpAdd
	case gputypes.BlendOperationSubtract:
		return raster.BlendOpSubtract
	case gputypes.BlendOperationReverseSubtract:
		return raster.BlendOpReverseSubtract
	case gputypes.BlendOperationMin:
		return raster.BlendOpMin
	case gputypes.BlendOperationMax:
		return raster.BlendOpMax
	default:
		return raster.BlendOpAdd
	}
}
