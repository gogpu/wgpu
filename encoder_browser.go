//go:build js && wasm

package wgpu

import (
	"github.com/gogpu/wgpu/internal/encoder"
)

// CommandEncoder records GPU commands for later submission.
// On browser, this wraps a GPUCommandEncoder via internal/encoder.
//
// The implementation lives in internal/encoder (ADR-070); this declaration is
// the public surface.
type CommandEncoder struct {
	impl *encoder.CommandEncoder
}

// BeginRenderPass begins a render pass.
func (e *CommandEncoder) BeginRenderPass(desc *RenderPassDescriptor) (*RenderPassEncoder, error) {
	pass, err := e.impl.BeginRenderPass(encoderRenderPassDescriptor(desc))
	if err != nil {
		return nil, err
	}
	return &RenderPassEncoder{impl: pass}, nil
}

// BeginComputePass begins a compute pass.
func (e *CommandEncoder) BeginComputePass(desc *ComputePassDescriptor) (*ComputePassEncoder, error) {
	bp, err := e.impl.BeginComputePass(encoderComputePassDescriptor(desc))
	if err != nil {
		return nil, err
	}
	return &ComputePassEncoder{browser: bp}, nil
}

// CopyBufferToBuffer copies data between buffers.
func (e *CommandEncoder) CopyBufferToBuffer(src *Buffer, srcOffset uint64, dst *Buffer, dstOffset uint64, size uint64) {
	e.impl.CopyBufferToBuffer(encoderBuffer(src), srcOffset, encoderBuffer(dst), dstOffset, size)
}

// CopyBufferToTexture copies data from a buffer to a texture.
func (e *CommandEncoder) CopyBufferToTexture(src *Buffer, dst *Texture, regions []BufferTextureCopy) {
	e.impl.CopyBufferToTexture(encoderBuffer(src), encoderTexture(dst), encoderBufferTextureCopies(regions))
}

// ClearBuffer clears a buffer region to zero.
func (e *CommandEncoder) ClearBuffer(buffer *Buffer, offset, size uint64) {
	e.impl.ClearBuffer(encoderBuffer(buffer), offset, size)
}

// CopyTextureToBuffer copies data from a texture to a buffer.
func (e *CommandEncoder) CopyTextureToBuffer(src *Texture, dst *Buffer, regions []BufferTextureCopy) {
	e.impl.CopyTextureToBuffer(encoderTexture(src), encoderBuffer(dst), encoderBufferTextureCopies(regions))
}

// CopyTextureToTexture copies data between textures.
func (e *CommandEncoder) CopyTextureToTexture(src, dst *Texture, regions []TextureCopy) {
	e.impl.CopyTextureToTexture(encoderTexture(src), encoderTexture(dst), encoderTextureCopies(regions))
}

// TransitionTextures transitions texture states for synchronization.
// On browser, this is a no-op — the browser WebGPU API handles barriers internally.
func (e *CommandEncoder) TransitionTextures(barriers []TextureBarrier) {
	e.impl.TransitionTextures(encoderTextureBarriers(barriers))
}

// DiscardEncoding discards the encoder without producing a command buffer.
func (e *CommandEncoder) DiscardEncoding() {
	e.impl.DiscardEncoding()
}

// Finish completes command recording and returns a CommandBuffer.
func (e *CommandEncoder) Finish() (*CommandBuffer, error) {
	cb, err := e.impl.Finish()
	if err != nil {
		return nil, err
	}
	return &CommandBuffer{impl: cb}, nil
}

// CommandBuffer holds recorded GPU commands ready for submission.
// On browser, this wraps a GPUCommandBuffer via internal/encoder.
type CommandBuffer struct {
	impl *encoder.CommandBuffer
}

// Release releases a CommandBuffer that will NOT be submitted to the GPU.
func (cb *CommandBuffer) Release() {
	cb.impl.Release()
}

// encoderRenderPassDescriptor converts a public render pass descriptor to the
// encoder's handle types.
func encoderRenderPassDescriptor(desc *RenderPassDescriptor) *encoder.RenderPassDescriptor {
	if desc == nil {
		return nil
	}
	out := &encoder.RenderPassDescriptor{Label: desc.Label}
	for _, ca := range desc.ColorAttachments {
		out.ColorAttachments = append(out.ColorAttachments, encoder.RenderPassColorAttachment{
			View:          encoderTextureView(ca.View),
			ResolveTarget: encoderTextureView(ca.ResolveTarget),
			DepthSlice:    ca.DepthSlice,
			LoadOp:        ca.LoadOp,
			StoreOp:       ca.StoreOp,
			ClearValue:    ca.ClearValue,
		})
	}
	if ds := desc.DepthStencilAttachment; ds != nil {
		out.DepthStencilAttachment = &encoder.RenderPassDepthStencilAttachment{
			View:              encoderTextureView(ds.View),
			DepthLoadOp:       ds.DepthLoadOp,
			DepthStoreOp:      ds.DepthStoreOp,
			DepthClearValue:   ds.DepthClearValue,
			DepthReadOnly:     ds.DepthReadOnly,
			StencilLoadOp:     ds.StencilLoadOp,
			StencilStoreOp:    ds.StencilStoreOp,
			StencilClearValue: ds.StencilClearValue,
			StencilReadOnly:   ds.StencilReadOnly,
		}
	}
	return out
}

// encoderComputePassDescriptor converts a public compute pass descriptor.
func encoderComputePassDescriptor(desc *ComputePassDescriptor) *encoder.ComputePassDescriptor {
	if desc == nil {
		return nil
	}
	return &encoder.ComputePassDescriptor{Label: desc.Label}
}
