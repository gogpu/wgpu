//go:build !rust && !(js && wasm)

package wgpu

import (
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/resource"
)

// CommandEncoder records GPU commands for later submission.
//
// A command encoder is single-use. After calling Finish(), the encoder
// cannot be used again. Call Device.CreateCommandEncoder() to create a new one.
//
// NOT thread-safe - do not use from multiple goroutines.
//
// The implementation lives in internal/encoder (ADR-070). This declaration is
// the public surface: every method is one delegation, and the descriptors are
// converted to the implementation's handle types at the call boundary.
type CommandEncoder struct {
	impl *encoder.CommandEncoder

	// device is the device that created the encoder. The public pass encoders
	// record the user's commands through it (limits, features, indirect
	// validation) and the encoder reaches it through the internal seam.
	device *Device
}

// BeginRenderPass begins a render pass.
// The returned RenderPassEncoder records draw commands.
// Call RenderPassEncoder.End() when done.
func (e *CommandEncoder) BeginRenderPass(desc *RenderPassDescriptor) (*RenderPassEncoder, error) {
	start, err := e.impl.BeginRenderPass(encoderRenderPassDescriptor(desc))
	if err != nil {
		return nil, err
	}
	return newRenderPassEncoder(start, e), nil
}

// BeginComputePass begins a compute pass.
// The returned ComputePassEncoder records dispatch commands.
// Call ComputePassEncoder.End() when done.
func (e *CommandEncoder) BeginComputePass(desc *ComputePassDescriptor) (*ComputePassEncoder, error) {
	start, err := e.impl.BeginComputePass(encoderComputePassDescriptor(desc))
	if err != nil {
		return nil, err
	}
	return newComputePassEncoder(start, e), nil
}

// CopyBufferToBuffer copies data between buffers.
func (e *CommandEncoder) CopyBufferToBuffer(src *Buffer, srcOffset uint64, dst *Buffer, dstOffset uint64, size uint64) {
	e.impl.CopyBufferToBuffer(encoderBuffer(src), srcOffset, encoderBuffer(dst), dstOffset, size)
}

// CopyTextureToBuffer copies data from a texture to a buffer.
// This is used for GPU-to-CPU readback of rendered content.
func (e *CommandEncoder) CopyTextureToBuffer(src *Texture, dst *Buffer, regions []BufferTextureCopy) {
	e.impl.CopyTextureToBuffer(encoderTexture(src), encoderBuffer(dst), encoderBufferTextureCopies(regions))
}

// CopyTextureToTexture copies data between textures using DMA hardware copy.
// WebGPU spec: GPUCommandEncoder.copyTextureToTexture()
func (e *CommandEncoder) CopyTextureToTexture(src, dst *Texture, regions []TextureCopy) {
	e.impl.CopyTextureToTexture(encoderTexture(src), encoderTexture(dst), encoderTextureCopies(regions))
}

// TransitionTextures records explicit texture barriers.
//
// Copy commands and render passes already transition their textures
// automatically from the state left by earlier commands of the same encoder,
// so explicit barriers are only needed for states the encoder cannot see.
// Each explicit barrier also updates the encoder's tracker: later commands
// transition from the barrier's NewUsage instead of repeating it.
// On Metal, GLES, and software backends the barrier itself is a no-op.
func (e *CommandEncoder) TransitionTextures(barriers []TextureBarrier) {
	e.impl.TransitionTextures(encoderTextureBarriers(barriers))
}

// CopyBufferToTexture copies data from a buffer to a texture.
// WebGPU spec: GPUCommandEncoder.copyBufferToTexture.
func (e *CommandEncoder) CopyBufferToTexture(src *Buffer, dst *Texture, regions []BufferTextureCopy) {
	e.impl.CopyBufferToTexture(encoderBuffer(src), encoderTexture(dst), encoderBufferTextureCopies(regions))
}

// ClearBuffer clears a buffer region to zero.
// WebGPU spec: GPUCommandEncoder.clearBuffer.
//
// The size is explicit: a zero size clears nothing (WebGPU's default of "the
// rest of the buffer" has no Go spelling; pass buffer.Size() - offset).
// Validation follows Rust wgpu-core command/clear.rs clear_buffer and the
// W3C clearBuffer valid usage list; failures are *core.ClearError from Finish.
func (e *CommandEncoder) ClearBuffer(buffer *Buffer, offset, size uint64) {
	e.impl.ClearBuffer(encoderBuffer(buffer), offset, size)
}

// ResolveQuerySet copies query results into a buffer.
func (e *CommandEncoder) ResolveQuerySet(querySet *QuerySet, firstQuery, queryCount uint32, dst *Buffer, dstOffset uint64) {
	e.impl.ResolveQuerySet(encoderQuerySet(querySet), firstQuery, queryCount, encoderBuffer(dst), dstOffset)
}

// DiscardEncoding discards the encoder without producing a command buffer.
// Use this to abandon an in-progress encoding when an error occurs.
// If the encoder was acquired from the pool, it is returned for reuse.
func (e *CommandEncoder) DiscardEncoding() {
	e.impl.DiscardEncoding()
}

// Finish completes command recording and returns a CommandBuffer.
// After calling Finish(), the encoder cannot be used again.
//
// The HAL encoder ownership transfers from the CommandEncoder to the
// CommandBuffer. After GPU completion, Submit() schedules the encoder
// to be reset via ResetAll and returned to the Device's encoder pool.
func (e *CommandEncoder) Finish() (*CommandBuffer, error) {
	cb, err := e.impl.Finish()
	if err != nil {
		return nil, err
	}
	return &CommandBuffer{impl: cb}, nil
}

// CommandBuffer holds recorded GPU commands ready for submission.
// Created by CommandEncoder.Finish().
//
// The implementation lives in internal/encoder (ADR-070); this declaration is
// the public handle.
type CommandBuffer struct {
	impl *encoder.CommandBuffer
}

// Release releases a CommandBuffer that will NOT be submitted to the GPU.
// This returns the HAL encoder to the device pool and drops tracked resource refs.
//
// In normal flow, Submit() takes ownership of the encoder and handles recycling
// after GPU completion. Release() is for error paths and canceled operations
// where the CommandBuffer is discarded without submitting.
//
// Matches Rust wgpu-core InnerCommandEncoder::Drop (command/mod.rs:726-738)
// which always calls reset_all + release_encoder regardless of whether the
// command buffer was submitted.
//
// A CommandBuffer MUST be either Submit()'d or Release()'d. Failing to do
// either leaks the HAL encoder (DX12 ~64KB allocator, Vulkan VkCommandPool).
func (cb *CommandBuffer) Release() {
	if cb == nil {
		return
	}
	cb.impl.Release()
}

// encoderQuerySet maps a public query set wrapper to its resource handle. A
// nil implementation is an absent set: the encoder treats it as one, as the
// pre-move code did.
func encoderQuerySet(qs *QuerySet) *resource.QuerySet {
	if qs == nil {
		return nil
	}
	return qs.impl
}

// encoderRenderPassDescriptor converts a public render pass descriptor to the
// encoder's handle types, field by field.
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
	if writes := desc.TimestampWrites; writes != nil {
		out.TimestampWrites = &encoder.RenderPassTimestampWrites{
			QuerySet:                  encoderQuerySet(writes.QuerySet),
			BeginningOfPassWriteIndex: writes.BeginningOfPassWriteIndex,
			EndOfPassWriteIndex:       writes.EndOfPassWriteIndex,
		}
	}
	return out
}

// encoderComputePassDescriptor converts a public compute pass descriptor to
// the encoder's handle types.
func encoderComputePassDescriptor(desc *ComputePassDescriptor) *encoder.ComputePassDescriptor {
	if desc == nil {
		return nil
	}
	out := &encoder.ComputePassDescriptor{Label: desc.Label}
	if writes := desc.TimestampWrites; writes != nil {
		out.TimestampWrites = &encoder.ComputePassTimestampWrites{
			QuerySet:                  encoderQuerySet(writes.QuerySet),
			BeginningOfPassWriteIndex: writes.BeginningOfPassWriteIndex,
			EndOfPassWriteIndex:       writes.EndOfPassWriteIndex,
		}
	}
	return out
}

// setError records a deferred error on the encoder. The public pass encoders
// call it for the errors they detect while recording; the error surfaces at
// Finish.
func (e *CommandEncoder) setError(err error) { e.impl.SetError(err) }
