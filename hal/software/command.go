//go:build !(js && wasm)

package software

import (
	"context"
	"fmt"
	"image"
	"log/slog"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/software/raster"
	"github.com/gogpu/wgpu/hal/software/shader"
)

// CommandEncoder implements hal.CommandEncoder for the software backend.
// It holds a device reference so that compute passes can resolve bind group
// resources during dispatch.
type CommandEncoder struct {
	device *Device
}

// BeginEncoding is a no-op.
func (c *CommandEncoder) BeginEncoding(_ string) error {
	return nil
}

// EndEncoding returns a placeholder command buffer.
func (c *CommandEncoder) EndEncoding() (hal.CommandBuffer, error) {
	return &Resource{}, nil
}

// DiscardEncoding is a no-op.
func (c *CommandEncoder) DiscardEncoding() {}

// ResetAll is a no-op.
func (c *CommandEncoder) ResetAll(_ []hal.CommandBuffer) {}

// Destroy is a no-op for the software backend.
func (c *CommandEncoder) Destroy() {}

// TransitionBuffers is a no-op (software backend doesn't need explicit transitions).
func (c *CommandEncoder) TransitionBuffers(_ []hal.BufferBarrier) {}

// TransitionTextures is a no-op (software backend doesn't need explicit transitions).
func (c *CommandEncoder) TransitionTextures(_ []hal.TextureBarrier) {}

// ClearBuffer clears a buffer region to zero.
func (c *CommandEncoder) ClearBuffer(buffer hal.Buffer, offset, size uint64) {
	if b, ok := buffer.(*Buffer); ok {
		b.mu.Lock()
		defer b.mu.Unlock()
		// Clear to zero
		for i := offset; i < offset+size && i < uint64(len(b.data)); i++ {
			b.data[i] = 0
		}
	}
}

// CopyBufferToBuffer copies data between buffers.
func (c *CommandEncoder) CopyBufferToBuffer(src, dst hal.Buffer, regions []hal.BufferCopy) {
	srcBuf, srcOK := src.(*Buffer)
	dstBuf, dstOK := dst.(*Buffer)

	if !srcOK || !dstOK {
		return
	}

	for _, region := range regions {
		srcBuf.mu.RLock()
		dstBuf.mu.Lock()

		// Perform copy with bounds checking
		srcEnd := region.SrcOffset + region.Size
		dstEnd := region.DstOffset + region.Size

		if srcEnd <= uint64(len(srcBuf.data)) && dstEnd <= uint64(len(dstBuf.data)) {
			copy(dstBuf.data[region.DstOffset:dstEnd], srcBuf.data[region.SrcOffset:srcEnd])
		}

		dstBuf.mu.Unlock()
		srcBuf.mu.RUnlock()
	}
}

// CopyBufferToTexture copies data from a buffer to a texture.
func (c *CommandEncoder) CopyBufferToTexture(src hal.Buffer, dst hal.Texture, regions []hal.BufferTextureCopy) {
	srcBuf, srcOK := src.(*Buffer)
	dstTex, dstOK := dst.(*Texture)

	if !srcOK || !dstOK {
		return
	}

	for _, region := range regions {
		srcBuf.mu.RLock()
		dstTex.mu.Lock()

		// Simple copy: just copy from buffer to texture data
		// In a real implementation, this would respect image layout and stride
		offset := region.BufferLayout.Offset
		size := uint64(region.Size.Width) * uint64(region.Size.Height) * uint64(region.Size.DepthOrArrayLayers) * 4 // 4 bytes per pixel

		if offset+size <= uint64(len(srcBuf.data)) && size <= uint64(len(dstTex.data)) {
			copy(dstTex.data, srcBuf.data[offset:offset+size])
		}

		dstTex.mu.Unlock()
		srcBuf.mu.RUnlock()
	}
}

// CopyTextureToBuffer copies data from a texture to a buffer.
func (c *CommandEncoder) CopyTextureToBuffer(src hal.Texture, dst hal.Buffer, regions []hal.BufferTextureCopy) {
	srcTex, srcOK := src.(*Texture)
	dstBuf, dstOK := dst.(*Buffer)

	if !srcOK || !dstOK {
		return
	}

	for _, region := range regions {
		srcTex.mu.RLock()
		dstBuf.mu.Lock()

		// Simple copy: just copy from texture to buffer data
		offset := region.BufferLayout.Offset
		size := uint64(region.Size.Width) * uint64(region.Size.Height) * uint64(region.Size.DepthOrArrayLayers) * 4 // 4 bytes per pixel

		if size <= uint64(len(srcTex.data)) && offset+size <= uint64(len(dstBuf.data)) {
			copy(dstBuf.data[offset:offset+size], srcTex.data[:size])
		}

		dstBuf.mu.Unlock()
		srcTex.mu.RUnlock()
	}
}

// CopyTextureToTexture copies data between textures.
func (c *CommandEncoder) CopyTextureToTexture(src, dst hal.Texture, regions []hal.TextureCopy) {
	srcTex, srcOK := src.(*Texture)
	dstTex, dstOK := dst.(*Texture)

	if !srcOK || !dstOK {
		return
	}

	for _, region := range regions {
		srcTex.mu.RLock()
		dstTex.mu.Lock()

		// Simple copy: just copy texture data
		size := uint64(region.Size.Width) * uint64(region.Size.Height) * uint64(region.Size.DepthOrArrayLayers) * 4 // 4 bytes per pixel

		if size <= uint64(len(srcTex.data)) && size <= uint64(len(dstTex.data)) {
			copy(dstTex.data[:size], srcTex.data[:size])
		}

		dstTex.mu.Unlock()
		srcTex.mu.RUnlock()
	}
}

// ResolveQuerySet is a no-op (query sets not supported in software backend).
func (c *CommandEncoder) ResolveQuerySet(_ hal.QuerySet, _, _ uint32, _ hal.Buffer, _ uint64) {}

// BeginRenderPass begins a render pass and returns an encoder.
// If a depth/stencil attachment is present, a persistent stencil buffer is
// created for the entire pass (matching GPU behavior where the stencil buffer
// is the attachment texture, not recreated per draw call).
func (c *CommandEncoder) BeginRenderPass(desc *hal.RenderPassDescriptor) hal.RenderPassEncoder {
	r := &RenderPassEncoder{
		desc: desc,
	}

	if hal.Logger().Enabled(context.Background(), slog.LevelDebug) {
		var w, h uint32
		loadOp := gputypes.LoadOpClear
		if len(desc.ColorAttachments) > 0 {
			loadOp = desc.ColorAttachments[0].LoadOp
			if tv, ok := desc.ColorAttachments[0].View.(*TextureView); ok && tv.texture != nil {
				w, h = tv.texture.width, tv.texture.height
			}
		}
		hal.Logger().Debug("software: BeginRenderPass",
			"width", w, "height", h,
			"colorLoadOp", loadOp,
			"attachments", len(desc.ColorAttachments),
		)
	}

	// Create persistent stencil buffer from depth/stencil attachment.
	if desc.DepthStencilAttachment != nil { //nolint:nestif // sequential attachment init
		if dsView, ok := desc.DepthStencilAttachment.View.(*TextureView); ok && dsView.texture != nil {
			w := int(dsView.texture.width)
			h := int(dsView.texture.height)
			if w > 0 && h > 0 {
				r.passStencilBuffer = raster.NewStencilBuffer(w, h)
				if desc.DepthStencilAttachment.StencilLoadOp == gputypes.LoadOpClear {
					r.passStencilBuffer.Clear(uint8(desc.DepthStencilAttachment.StencilClearValue))
				}
			}
		}
	}

	return r
}

// BeginComputePass begins a compute pass and returns an encoder.
// The device reference is passed through so Dispatch can resolve bind group
// buffer data.
func (c *CommandEncoder) BeginComputePass(desc *hal.ComputePassDescriptor) hal.ComputePassEncoder {
	return &ComputePassEncoder{
		desc:   desc,
		device: c.device,
	}
}

// vertexBufferBinding holds a vertex buffer and its byte offset.
type vertexBufferBinding struct {
	buffer *Buffer
	offset uint64
}

// RenderPassEncoder implements hal.RenderPassEncoder for the software backend.
// It tracks pipeline state set during encoding and executes draw calls
// using the raster/ package for triangle rasterization.
type RenderPassEncoder struct {
	desc *hal.RenderPassDescriptor

	// Pipeline and resource state set during encoding.
	pipeline    *RenderPipeline
	bindGroups  [4]*BindGroup          // max 4 per WebGPU spec
	vertexBufs  [8]vertexBufferBinding // max 8 vertex buffers
	indexBuffer *Buffer
	indexFormat gputypes.IndexFormat
	indexOffset uint64

	// activeIndices, when non-nil, remaps the sequential draw position to a
	// vertex index for the current DrawIndexed call (already including
	// baseVertex). The vertex fetch paths consult it instead of computing
	// firstVertex+i, so indexed and non-indexed draws share one rasterizer.
	activeIndices []uint32

	// Viewport and scissor state.
	viewport    [6]float32 // x, y, w, h, minDepth, maxDepth
	scissorRect [4]uint32  // x, y, w, h
	hasViewport bool
	hasScissor  bool

	// Stencil reference value set by SetStencilReference.
	stencilRef uint32

	// Persistent stencil buffer for the render pass — created once at
	// BeginRenderPass, reused across all Draw() calls. On GPU backends
	// the stencil buffer is the depth/stencil attachment texture; here
	// we emulate that by keeping a single raster.StencilBuffer alive
	// for the entire pass. Without this, stencil writes from pass 1
	// (clip shape) would be lost before pass 2 (content draw).
	passStencilBuffer *raster.StencilBuffer

	// Whether the framebuffer has been cleared this pass.
	// WebGPU spec: LoadOp=Clear happens before the first draw, not at End().
	cleared bool

	// drawCount tracks total Draw/DrawIndexed calls for Stats().
	drawCount uint32
}

// End finishes the render pass.
// If no draw calls were issued and LoadOp is Clear, the clear is applied now.
// MSAA resolve: copies color attachment pixels to resolve target (WebGPU spec).
func (r *RenderPassEncoder) End() {
	hal.Logger().Debug("software: End",
		"draws", r.drawCount,
		"hasScissor", r.hasScissor,
		"scissor", r.scissorRect,
	)

	// If no draw happened, apply pending clears.
	if !r.cleared {
		r.applyClear()
	}

	// MSAA resolve: copy color attachment to resolve target.
	// In WebGPU, if a color attachment has a ResolveTarget, the GPU resolves
	// MSAA samples to the single-sample target at end of render pass.
	// Software backend has no real MSAA — this is a direct pixel copy.
	for _, attachment := range r.desc.ColorAttachments {
		if attachment.ResolveTarget == nil {
			continue
		}
		srcView, ok := attachment.View.(*TextureView)
		if !ok || srcView.texture == nil {
			continue
		}
		dstView, ok := attachment.ResolveTarget.(*TextureView)
		if !ok || dstView.texture == nil {
			continue
		}
		src := srcView.texture
		dst := dstView.texture
		src.mu.RLock()
		dst.mu.Lock()
		if len(src.data) == len(dst.data) {
			copy(dst.data, src.data)
		}
		dst.mu.Unlock()
		src.mu.RUnlock()
	}

	// Depth/stencil attachment handling (simplified - just clear if needed)
	r.clearDepthStencilAttachment()
}

// applyClear clears color attachments that have LoadOp=Clear.
func (r *RenderPassEncoder) applyClear() {
	r.cleared = true
	for _, attachment := range r.desc.ColorAttachments {
		if attachment.LoadOp == gputypes.LoadOpClear {
			if view, ok := attachment.View.(*TextureView); ok {
				if view.texture != nil {
					view.texture.Clear(attachment.ClearValue)
				}
			}
		}
	}
}

// clearDepthStencilAttachment clears the depth/stencil attachment if present and LoadOp is Clear.
func (r *RenderPassEncoder) clearDepthStencilAttachment() {
	ds := r.desc.DepthStencilAttachment
	if ds == nil || ds.DepthLoadOp != gputypes.LoadOpClear {
		return
	}
	view, ok := ds.View.(*TextureView)
	if !ok || view.texture == nil {
		return
	}
	val := ds.DepthClearValue
	view.texture.Clear(gputypes.Color{R: float64(val), G: float64(val), B: float64(val), A: 1.0})
}

// SetPipeline stores the render pipeline for subsequent draw calls.
func (r *RenderPassEncoder) SetPipeline(p hal.RenderPipeline) {
	if rp, ok := p.(*RenderPipeline); ok {
		r.pipeline = rp
	}
}

// SetBindGroup stores a bind group at the given index.
func (r *RenderPassEncoder) SetBindGroup(index uint32, bg hal.BindGroup, _ []uint32) {
	if index < 4 {
		if b, ok := bg.(*BindGroup); ok {
			r.bindGroups[index] = b
		}
	}
}

// SetVertexBuffer stores a vertex buffer binding at the given slot.
func (r *RenderPassEncoder) SetVertexBuffer(slot uint32, buf hal.Buffer, offset uint64) {
	if slot < 8 {
		if b, ok := buf.(*Buffer); ok {
			r.vertexBufs[slot] = vertexBufferBinding{buffer: b, offset: offset}
		}
	}
}

// SetIndexBuffer stores the index buffer for indexed draw calls.
func (r *RenderPassEncoder) SetIndexBuffer(buf hal.Buffer, format gputypes.IndexFormat, offset uint64) {
	if b, ok := buf.(*Buffer); ok {
		r.indexBuffer = b
		r.indexFormat = format
		r.indexOffset = offset
	}
}

// SetViewport stores the viewport transformation.
func (r *RenderPassEncoder) SetViewport(x, y, w, h, minDepth, maxDepth float32) {
	r.viewport = [6]float32{x, y, w, h, minDepth, maxDepth}
	r.hasViewport = true
}

// SetScissorRect stores the scissor rectangle.
func (r *RenderPassEncoder) SetScissorRect(x, y, w, h uint32) {
	r.scissorRect = [4]uint32{x, y, w, h}
	r.hasScissor = true
	hal.Logger().Debug("software: SetScissorRect", "x", x, "y", y, "w", w, "h", h)
}

// SetBlendConstant is a no-op (blend constants not yet wired to raster pipeline).
func (r *RenderPassEncoder) SetBlendConstant(_ *gputypes.Color) {}

// SetStencilReference stores the stencil reference value for subsequent draw calls.
// The reference value is used by stencil comparison and StencilOpReplace.
func (r *RenderPassEncoder) SetStencilReference(ref uint32) {
	r.stencilRef = ref
}

// Draw executes a non-indexed draw call.
// It performs vertex fetch, viewport transform, and triangle rasterization
// using the raster/ package. If no vertex buffer is bound and a texture is
// available in a bind group, it performs a fullscreen texture blit.
// Supports instanced rendering: instanceCount > 1 draws the same vertices
// multiple times, advancing instance-rate vertex buffers per instance.
func (r *RenderPassEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	r.drawCount++
	hal.Logger().Debug("software: Draw", "vertices", vertexCount, "instances", instanceCount, "drawIndex", r.drawCount)
	r.executeDraw(vertexCount, instanceCount, firstVertex, firstInstance)
}

// DrawIndexed executes an indexed draw call. It resolves the index buffer into
// a list of vertex indices (applying baseVertex) and runs the same vertex-fetch
// and rasterization path as Draw, with vertex positions remapped through the
// resolved indices. Previously this was a no-op, so every indexed draw (glyph
// mask text, MSDF text) rendered nothing on the software backend.
func (r *RenderPassEncoder) DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	r.drawCount++
	if indexCount == 0 || r.indexBuffer == nil {
		return
	}
	indices := r.resolveIndices(indexCount, firstIndex, baseVertex)
	if indices == nil {
		return
	}
	hal.Logger().Debug("software: DrawIndexed", "indices", indexCount, "instances", instanceCount, "drawIndex", r.drawCount)

	r.activeIndices = indices
	defer func() { r.activeIndices = nil }()
	// vertexCount == indexCount; firstVertex is unused because activeIndices
	// supplies the per-position vertex index directly.
	r.executeDraw(indexCount, instanceCount, 0, firstInstance)
}

// resolveIndices reads indexCount entries from the bound index buffer starting
// at firstIndex, honoring the index format (Uint16/Uint32) and the buffer
// offset set by SetIndexBuffer, and adds baseVertex to each. Returns nil if the
// requested range lies outside the buffer.
func (r *RenderPassEncoder) resolveIndices(indexCount, firstIndex uint32, baseVertex int32) []uint32 {
	indexSize := uint64(2)
	if r.indexFormat == gputypes.IndexFormatUint32 {
		indexSize = 4
	}

	r.indexBuffer.mu.RLock()
	data := r.indexBuffer.data
	r.indexBuffer.mu.RUnlock()

	start := r.indexOffset + uint64(firstIndex)*indexSize
	end := start + uint64(indexCount)*indexSize
	if end > uint64(len(data)) {
		return nil
	}

	out := make([]uint32, indexCount)
	for i := uint32(0); i < indexCount; i++ {
		off := start + uint64(i)*indexSize
		var idx uint32
		if indexSize == 4 {
			idx = uint32(data[off]) | uint32(data[off+1])<<8 | uint32(data[off+2])<<16 | uint32(data[off+3])<<24
		} else {
			idx = uint32(data[off]) | uint32(data[off+1])<<8
		}
		out[i] = uint32(int32(idx) + baseVertex) //nolint:gosec // index + baseVertex is a valid vertex index
	}
	return out
}

// drawVertexIndex maps a sequential draw position to the vertex index to fetch.
// For non-indexed draws it is firstVertex+pos; for indexed draws it reads the
// resolved index list set by DrawIndexed.
func (r *RenderPassEncoder) drawVertexIndex(firstVertex, pos uint32) uint32 {
	if r.activeIndices != nil {
		if pos < uint32(len(r.activeIndices)) {
			return r.activeIndices[pos]
		}
		return 0
	}
	return firstVertex + pos
}

// DrawIndirect is a no-op.
func (r *RenderPassEncoder) DrawIndirect(_ hal.Buffer, _ uint64) {}

// DrawIndexedIndirect is a no-op.
func (r *RenderPassEncoder) DrawIndexedIndirect(_ hal.Buffer, _ uint64) {}

// ExecuteBundle is a no-op.
func (r *RenderPassEncoder) ExecuteBundle(_ hal.RenderBundle) {}

// Stats returns render pass statistics after End(). Designed for CI e2e
// test assertions — zero overhead (fields already tracked during encoding).
func (r *RenderPassEncoder) Stats() RenderPassStats {
	s := RenderPassStats{
		DrawCount:  r.drawCount,
		HasScissor: r.hasScissor,
	}
	if r.hasScissor {
		s.ScissorRect = image.Rect(
			int(r.scissorRect[0]), int(r.scissorRect[1]),
			int(r.scissorRect[0]+r.scissorRect[2]), int(r.scissorRect[1]+r.scissorRect[3]),
		)
	}
	if r.desc != nil && len(r.desc.ColorAttachments) > 0 {
		s.ColorLoadOp = r.desc.ColorAttachments[0].LoadOp
		if tv, ok := r.desc.ColorAttachments[0].View.(*TextureView); ok && tv.texture != nil {
			s.Width = tv.texture.width
			s.Height = tv.texture.height
		}
	}
	return s
}

// ComputePassEncoder implements hal.ComputePassEncoder for the software backend.
// It collects pipeline and bind group state, then executes the SPIR-V interpreter
// on Dispatch. Buffer writes from storage buffer bindings are reflected in-place.
type ComputePassEncoder struct {
	desc   *hal.ComputePassDescriptor
	device *Device

	// Pipeline and resource state set during encoding.
	pipeline   *ComputePipeline
	bindGroups [4]*BindGroup // max 4 per WebGPU spec
}

// End finishes the compute pass. Currently a no-op since all work is done
// synchronously in Dispatch.
func (c *ComputePassEncoder) End() {}

// SetPipeline stores the compute pipeline for subsequent Dispatch calls.
func (c *ComputePassEncoder) SetPipeline(p hal.ComputePipeline) {
	if cp, ok := p.(*ComputePipeline); ok {
		c.pipeline = cp
	}
}

// SetBindGroup stores a bind group at the given index for compute dispatch.
func (c *ComputePassEncoder) SetBindGroup(index uint32, bg hal.BindGroup, _ []uint32) {
	if index < 4 {
		if b, ok := bg.(*BindGroup); ok {
			c.bindGroups[index] = b
		}
	}
}

// Dispatch executes the compute shader for x*y*z workgroups.
//
// The implementation:
//  1. Gets the parsed SPIR-V module from the pipeline's shader module.
//  2. Reads the workgroup size from the entry point's OpExecutionMode LocalSize.
//  3. Builds an ExecutionContext with Buffers populated from bound bind groups.
//  4. Delegates to Module.DispatchCompute which iterates over all workgroups
//     and invocations, calling ExecuteCompute for each.
//  5. Storage buffer writes are reflected in-place through shared []byte slices.
func (c *ComputePassEncoder) Dispatch(x, y, z uint32) {
	if c.pipeline == nil {
		slog.Warn("software: ComputePassEncoder.Dispatch called without a pipeline set")
		return
	}

	parsedModule := c.pipeline.module.ParsedModule()
	if parsedModule == nil {
		slog.Warn("software: ComputePassEncoder.Dispatch: pipeline has no parsed SPIR-V module")
		return
	}

	entryPoint := c.pipeline.entryPoint

	// Build the execution context with buffer bindings from all bind groups.
	ctx := &shader.ExecutionContext{
		Buffers: make(map[shader.BindingKey][]byte),
	}

	for groupIdx, bg := range c.bindGroups {
		if bg == nil {
			continue
		}
		for bindingIdx, buf := range bg.buffers {
			if buf == nil {
				continue
			}
			// Share the buffer's data slice directly so storage buffer writes
			// from the interpreter are reflected in the HAL buffer.
			buf.mu.Lock()
			ctx.Buffers[shader.BindingKey{
				Group:   uint32(groupIdx),
				Binding: bindingIdx,
			}] = buf.data
			buf.mu.Unlock()
		}
	}

	slog.Debug("software: ComputePassEncoder.Dispatch",
		"entryPoint", entryPoint,
		"workgroups", fmt.Sprintf("(%d,%d,%d)", x, y, z),
	)

	if err := parsedModule.DispatchCompute(entryPoint, ctx, x, y, z); err != nil {
		slog.Error("software: compute dispatch failed", "error", err)
	}
}

// DispatchIndirect is not yet implemented in the software backend.
// Indirect dispatch requires reading the dispatch parameters from a GPU buffer,
// which is straightforward but not needed for the current use cases.
func (c *ComputePassEncoder) DispatchIndirect(_ hal.Buffer, _ uint64) {
	slog.Warn("software: DispatchIndirect not implemented")
}
