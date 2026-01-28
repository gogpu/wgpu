//go:build software

package software

import (
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/gputypes"
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

// RenderPassEncoder implements hal.RenderPassEncoder for the software backend.
type RenderPassEncoder struct {
	desc *hal.RenderPassDescriptor
}

// End finishes the render pass and performs load/store operations.
func (r *RenderPassEncoder) End() {
	// Process color attachments
	for _, attachment := range r.desc.ColorAttachments {
		// Handle clear operation
		if attachment.LoadOp == gputypes.LoadOpClear {
			// Get the underlying texture from the view
			if view, ok := attachment.View.(*TextureView); ok {
				if view.texture != nil {
					view.texture.Clear(attachment.ClearValue)
				}
			}
		}
		// Store operation is implicit (data stays in texture)
	}

	// Depth/stencil attachment handling (simplified - just clear if needed)
	if r.desc.DepthStencilAttachment != nil {
		if r.desc.DepthStencilAttachment.DepthLoadOp == gputypes.LoadOpClear {
			if view, ok := r.desc.DepthStencilAttachment.View.(*TextureView); ok {
				if view.texture != nil {
					// Clear depth to clearValue (as grayscale for simplicity)
					val := r.desc.DepthStencilAttachment.DepthClearValue
					color := gputypes.Color{R: float64(val), G: float64(val), B: float64(val), A: 1.0}
					view.texture.Clear(color)
				}
			}
		}
	}
}

// SetPipeline is a no-op.
func (r *RenderPassEncoder) SetPipeline(_ hal.RenderPipeline) {}

// SetBindGroup is a no-op.
func (r *RenderPassEncoder) SetBindGroup(_ uint32, _ hal.BindGroup, _ []uint32) {}

// SetVertexBuffer is a no-op.
func (r *RenderPassEncoder) SetVertexBuffer(_ uint32, _ hal.Buffer, _ uint64) {}

// SetIndexBuffer is a no-op.
func (r *RenderPassEncoder) SetIndexBuffer(_ hal.Buffer, _ gputypes.IndexFormat, _ uint64) {}

// SetViewport is a no-op.
func (r *RenderPassEncoder) SetViewport(_, _, _, _, _, _ float32) {}

// SetScissorRect is a no-op.
func (r *RenderPassEncoder) SetScissorRect(_, _, _, _ uint32) {}

// SetBlendConstant is a no-op.
func (r *RenderPassEncoder) SetBlendConstant(_ *gputypes.Color) {}

// SetStencilReference is a no-op.
func (r *RenderPassEncoder) SetStencilReference(_ uint32) {}

// Draw is a no-op (rasterization not implemented in Phase 1).
func (r *RenderPassEncoder) Draw(_, _, _, _ uint32) {}

// DrawIndexed is a no-op (rasterization not implemented in Phase 1).
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
