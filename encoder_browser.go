//go:build js && wasm

package wgpu

// CommandEncoder records GPU commands for later submission.
type CommandEncoder struct {
	released bool
}

// BeginRenderPass begins a render pass.
func (e *CommandEncoder) BeginRenderPass(desc *RenderPassDescriptor) (*RenderPassEncoder, error) {
	panic("wgpu: browser backend not yet implemented")
}

// BeginComputePass begins a compute pass.
func (e *CommandEncoder) BeginComputePass(desc *ComputePassDescriptor) (*ComputePassEncoder, error) {
	panic("wgpu: browser backend not yet implemented")
}

// CopyBufferToBuffer copies data between buffers.
func (e *CommandEncoder) CopyBufferToBuffer(src *Buffer, srcOffset uint64, dst *Buffer, dstOffset uint64, size uint64) {
	panic("wgpu: browser backend not yet implemented")
}

// CopyTextureToBuffer copies data from a texture to a buffer.
func (e *CommandEncoder) CopyTextureToBuffer(src *Texture, dst *Buffer, regions []BufferTextureCopy) {
	panic("wgpu: browser backend not yet implemented")
}

// CopyTextureToTexture copies data between textures.
func (e *CommandEncoder) CopyTextureToTexture(src, dst *Texture, regions []TextureCopy) {
	panic("wgpu: browser backend not yet implemented")
}

// TransitionTextures transitions texture states for synchronization.
func (e *CommandEncoder) TransitionTextures(barriers []TextureBarrier) {
	panic("wgpu: browser backend not yet implemented")
}

// DiscardEncoding discards the encoder without producing a command buffer.
func (e *CommandEncoder) DiscardEncoding() {
	if e.released {
		return
	}
	e.released = true
}

// Finish completes command recording and returns a CommandBuffer.
func (e *CommandEncoder) Finish() (*CommandBuffer, error) {
	panic("wgpu: browser backend not yet implemented")
}

// CommandBuffer holds recorded GPU commands ready for submission.
type CommandBuffer struct {
	released bool
}

// Release releases a CommandBuffer that will NOT be submitted to the GPU.
func (cb *CommandBuffer) Release() {
}
