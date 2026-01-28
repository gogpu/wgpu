package hal

import "github.com/gogpu/gputypes"

// CommandEncoder records GPU commands.
// Command encoders are single-use - after EndEncoding, they cannot be reused.
type CommandEncoder interface {
	// BeginEncoding begins command recording with an optional label.
	BeginEncoding(label string) error

	// EndEncoding finishes command recording and returns a command buffer.
	// After this call, the encoder cannot be used again.
	EndEncoding() (CommandBuffer, error)

	// DiscardEncoding discards the encoder without creating a command buffer.
	// Use this to cancel encoding that encountered errors.
	DiscardEncoding()

	// ResetAll resets command buffers for reuse.
	// This is an optimization to avoid allocating new command buffers.
	// Not all backends support this.
	ResetAll(commandBuffers []CommandBuffer)

	// TransitionBuffers transitions buffer states for synchronization.
	// This is required on some backends (Vulkan, DX12) but no-op on others (Metal).
	TransitionBuffers(barriers []BufferBarrier)

	// TransitionTextures transitions texture states for synchronization.
	// This is required on some backends (Vulkan, DX12) but no-op on others (Metal).
	TransitionTextures(barriers []TextureBarrier)

	// ClearBuffer clears a buffer region to zero.
	ClearBuffer(buffer Buffer, offset, size uint64)

	// CopyBufferToBuffer copies data between buffers.
	CopyBufferToBuffer(src, dst Buffer, regions []BufferCopy)

	// CopyBufferToTexture copies data from a buffer to a texture.
	CopyBufferToTexture(src Buffer, dst Texture, regions []BufferTextureCopy)

	// CopyTextureToBuffer copies data from a texture to a buffer.
	CopyTextureToBuffer(src Texture, dst Buffer, regions []BufferTextureCopy)

	// CopyTextureToTexture copies data between textures.
	CopyTextureToTexture(src, dst Texture, regions []TextureCopy)

	// BeginRenderPass begins a render pass.
	// Returns a render pass encoder for recording draw commands.
	BeginRenderPass(desc *RenderPassDescriptor) RenderPassEncoder

	// BeginComputePass begins a compute pass.
	// Returns a compute pass encoder for recording dispatch commands.
	BeginComputePass(desc *ComputePassDescriptor) ComputePassEncoder
}

// RenderPassEncoder records render commands within a render pass.
// Render passes define rendering targets and operations.
type RenderPassEncoder interface {
	// End finishes the render pass.
	// After this call, the encoder cannot be used again.
	End()

	// SetPipeline sets the active render pipeline.
	SetPipeline(pipeline RenderPipeline)

	// SetBindGroup sets a bind group for the given index.
	// offsets are dynamic offsets for dynamic uniform/storage buffers.
	SetBindGroup(index uint32, group BindGroup, offsets []uint32)

	// SetVertexBuffer sets a vertex buffer for the given slot.
	SetVertexBuffer(slot uint32, buffer Buffer, offset uint64)

	// SetIndexBuffer sets the index buffer.
	SetIndexBuffer(buffer Buffer, format gputypes.IndexFormat, offset uint64)

	// SetViewport sets the viewport transformation.
	SetViewport(x, y, width, height, minDepth, maxDepth float32)

	// SetScissorRect sets the scissor rectangle for clipping.
	SetScissorRect(x, y, width, height uint32)

	// SetBlendConstant sets the blend constant color.
	SetBlendConstant(color *gputypes.Color)

	// SetStencilReference sets the stencil reference value.
	SetStencilReference(reference uint32)

	// Draw draws primitives.
	// vertexCount is the number of vertices to draw.
	// instanceCount is the number of instances to draw.
	// firstVertex is the offset into the vertex buffer.
	// firstInstance is the offset into the instance data.
	Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32)

	// DrawIndexed draws indexed primitives.
	// indexCount is the number of indices to draw.
	// instanceCount is the number of instances to draw.
	// firstIndex is the offset into the index buffer.
	// baseVertex is added to each index before fetching vertex data.
	// firstInstance is the offset into the instance data.
	DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32)

	// DrawIndirect draws primitives with GPU-generated parameters.
	// buffer contains DrawIndirectArgs at the given offset.
	DrawIndirect(buffer Buffer, offset uint64)

	// DrawIndexedIndirect draws indexed primitives with GPU-generated parameters.
	// buffer contains DrawIndexedIndirectArgs at the given offset.
	DrawIndexedIndirect(buffer Buffer, offset uint64)

	// ExecuteBundle executes a pre-recorded render bundle.
	// Bundles are an optimization for repeated draw calls.
	ExecuteBundle(bundle RenderBundle)
}

// ComputePassEncoder records compute commands within a compute pass.
type ComputePassEncoder interface {
	// End finishes the compute pass.
	// After this call, the encoder cannot be used again.
	End()

	// SetPipeline sets the active compute pipeline.
	SetPipeline(pipeline ComputePipeline)

	// SetBindGroup sets a bind group for the given index.
	// offsets are dynamic offsets for dynamic uniform/storage buffers.
	SetBindGroup(index uint32, group BindGroup, offsets []uint32)

	// Dispatch dispatches compute work.
	// x, y, z are the number of workgroups to dispatch in each dimension.
	Dispatch(x, y, z uint32)

	// DispatchIndirect dispatches compute work with GPU-generated parameters.
	// buffer contains DispatchIndirectArgs at the given offset.
	DispatchIndirect(buffer Buffer, offset uint64)
}

// RenderBundle is a pre-recorded set of render commands.
// Bundles can be executed multiple times for better performance.
type RenderBundle interface {
	Resource
}

// BufferBarrier defines a buffer state transition.
type BufferBarrier struct {
	Buffer Buffer
	Usage  BufferUsageTransition
}

// TextureBarrier defines a texture state transition.
type TextureBarrier struct {
	Texture Texture
	Range   TextureRange
	Usage   TextureUsageTransition
}

// BufferUsageTransition defines a buffer usage state transition.
type BufferUsageTransition struct {
	OldUsage gputypes.BufferUsage
	NewUsage gputypes.BufferUsage
}

// TextureUsageTransition defines a texture usage state transition.
type TextureUsageTransition struct {
	OldUsage gputypes.TextureUsage
	NewUsage gputypes.TextureUsage
}

// TextureRange specifies a range of texture subresources.
type TextureRange struct {
	// Aspect specifies which aspect of the texture (color, depth, stencil).
	Aspect gputypes.TextureAspect

	// BaseMipLevel is the first mip level in the range.
	BaseMipLevel uint32

	// MipLevelCount is the number of mip levels (0 means all remaining levels).
	MipLevelCount uint32

	// BaseArrayLayer is the first array layer in the range.
	BaseArrayLayer uint32

	// ArrayLayerCount is the number of array layers (0 means all remaining layers).
	ArrayLayerCount uint32
}

// BufferCopy defines a buffer-to-buffer copy region.
type BufferCopy struct {
	SrcOffset uint64
	DstOffset uint64
	Size      uint64
}

// BufferTextureCopy defines a buffer-texture copy region.
type BufferTextureCopy struct {
	BufferLayout ImageDataLayout
	TextureBase  ImageCopyTexture
	Size         Extent3D
}

// TextureCopy defines a texture-to-texture copy region.
type TextureCopy struct {
	SrcBase ImageCopyTexture
	DstBase ImageCopyTexture
	Size    Extent3D
}

// ImageDataLayout describes the layout of image data in a buffer.
type ImageDataLayout struct {
	// Offset is the offset in bytes from the start of the buffer.
	Offset uint64

	// BytesPerRow is the stride in bytes between rows of the image.
	// Must be a multiple of 256 for texture copies.
	// Can be 0 for single-row images.
	BytesPerRow uint32

	// RowsPerImage is the number of rows per image slice.
	// Only needed for 3D textures.
	// Can be 0 to use the image height.
	RowsPerImage uint32
}

// ImageCopyTexture specifies a texture location for copying.
type ImageCopyTexture struct {
	// Texture is the texture to copy to/from.
	Texture Texture

	// MipLevel is the mip level to copy.
	MipLevel uint32

	// Origin is the starting point of the copy.
	Origin Origin3D

	// Aspect specifies which aspect to copy (color, depth, stencil).
	Aspect gputypes.TextureAspect
}

// Origin3D is a 3D origin point.
type Origin3D struct {
	X uint32
	Y uint32
	Z uint32
}

// Extent3D is a 3D extent.
type Extent3D struct {
	Width              uint32
	Height             uint32
	DepthOrArrayLayers uint32
}
