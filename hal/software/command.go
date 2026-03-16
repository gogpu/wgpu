package software

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// CommandEncoder implements hal.CommandEncoder for the software backend.
type CommandEncoder struct{}

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
func (c *CommandEncoder) BeginRenderPass(desc *hal.RenderPassDescriptor) hal.RenderPassEncoder {
	return &RenderPassEncoder{
		desc: desc,
	}
}

// BeginComputePass begins a compute pass and returns an encoder.
func (c *CommandEncoder) BeginComputePass(desc *hal.ComputePassDescriptor) hal.ComputePassEncoder {
	return &ComputePassEncoder{
		desc: desc,
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

	// Viewport and scissor state.
	viewport    [6]float32 // x, y, w, h, minDepth, maxDepth
	scissorRect [4]uint32  // x, y, w, h
	hasViewport bool
	hasScissor  bool

	// Whether the framebuffer has been cleared this pass.
	// WebGPU spec: LoadOp=Clear happens before the first draw, not at End().
	cleared bool
}

// End finishes the render pass.
// If no draw calls were issued and LoadOp is Clear, the clear is applied now.
// MSAA resolve: copies color attachment pixels to resolve target (WebGPU spec).
func (r *RenderPassEncoder) End() {
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
}

// SetBlendConstant is a no-op (blend constants not yet wired to raster pipeline).
func (r *RenderPassEncoder) SetBlendConstant(_ *gputypes.Color) {}

// SetStencilReference is a no-op (stencil not yet wired).
func (r *RenderPassEncoder) SetStencilReference(_ uint32) {}

// Draw executes a non-indexed draw call.
// It performs vertex fetch, viewport transform, and triangle rasterization
// using the raster/ package. If no vertex buffer is bound and a texture is
// available in a bind group, it performs a fullscreen texture blit.
func (r *RenderPassEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	r.executeDraw(vertexCount, firstVertex)
}

// DrawIndexed is a no-op (indexed drawing not yet implemented).
func (r *RenderPassEncoder) DrawIndexed(_, _, _ uint32, _ int32, _ uint32) {}

// DrawIndirect is a no-op.
func (r *RenderPassEncoder) DrawIndirect(_ hal.Buffer, _ uint64) {}

// DrawIndexedIndirect is a no-op.
func (r *RenderPassEncoder) DrawIndexedIndirect(_ hal.Buffer, _ uint64) {}

// ExecuteBundle is a no-op.
func (r *RenderPassEncoder) ExecuteBundle(_ hal.RenderBundle) {}

// ComputePassEncoder implements hal.ComputePassEncoder for the software backend.
type ComputePassEncoder struct {
	desc *hal.ComputePassDescriptor
}

// End is a no-op.
func (c *ComputePassEncoder) End() {}

// SetPipeline is a no-op (compute not supported).
func (c *ComputePassEncoder) SetPipeline(_ hal.ComputePipeline) {}

// SetBindGroup is a no-op.
func (c *ComputePassEncoder) SetBindGroup(_ uint32, _ hal.BindGroup, _ []uint32) {}

// Dispatch is a no-op (compute not supported).
func (c *ComputePassEncoder) Dispatch(_, _, _ uint32) {}

// DispatchIndirect is a no-op.
func (c *ComputePassEncoder) DispatchIndirect(_ hal.Buffer, _ uint64) {}
