package noop

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// CommandEncoder implements hal.CommandEncoder for the noop backend.
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

// TransitionBuffers is a no-op.
func (c *CommandEncoder) TransitionBuffers(_ []hal.BufferBarrier) {}

// TransitionTextures is a no-op.
func (c *CommandEncoder) TransitionTextures(_ []hal.TextureBarrier) {}

// ClearBuffer is a no-op.
func (c *CommandEncoder) ClearBuffer(_ hal.Buffer, _, _ uint64) {}

// CopyBufferToBuffer is a no-op.
func (c *CommandEncoder) CopyBufferToBuffer(_, _ hal.Buffer, _ []hal.BufferCopy) {}

// CopyBufferToTexture is a no-op.
func (c *CommandEncoder) CopyBufferToTexture(_ hal.Buffer, _ hal.Texture, _ []hal.BufferTextureCopy) {
}

// CopyTextureToBuffer is a no-op.
func (c *CommandEncoder) CopyTextureToBuffer(_ hal.Texture, _ hal.Buffer, _ []hal.BufferTextureCopy) {
}

// CopyTextureToTexture is a no-op.
func (c *CommandEncoder) CopyTextureToTexture(_, _ hal.Texture, _ []hal.TextureCopy) {}

// BeginRenderPass returns a noop render pass encoder.
func (c *CommandEncoder) BeginRenderPass(_ *hal.RenderPassDescriptor) hal.RenderPassEncoder {
	return &RenderPassEncoder{}
}

// BeginComputePass returns a noop compute pass encoder.
func (c *CommandEncoder) BeginComputePass(_ *hal.ComputePassDescriptor) hal.ComputePassEncoder {
	return &ComputePassEncoder{}
}

// RenderPassEncoder implements hal.RenderPassEncoder for the noop backend.
type RenderPassEncoder struct{}

// End is a no-op.
func (r *RenderPassEncoder) End() {}

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

// Draw is a no-op.
func (r *RenderPassEncoder) Draw(_, _, _, _ uint32) {}

// DrawIndexed is a no-op.
func (r *RenderPassEncoder) DrawIndexed(_, _, _ uint32, _ int32, _ uint32) {}

// DrawIndirect is a no-op.
func (r *RenderPassEncoder) DrawIndirect(_ hal.Buffer, _ uint64) {}

// DrawIndexedIndirect is a no-op.
func (r *RenderPassEncoder) DrawIndexedIndirect(_ hal.Buffer, _ uint64) {}

// ExecuteBundle is a no-op.
func (r *RenderPassEncoder) ExecuteBundle(_ hal.RenderBundle) {}

// ComputePassEncoder implements hal.ComputePassEncoder for the noop backend.
type ComputePassEncoder struct{}

// End is a no-op.
func (c *ComputePassEncoder) End() {}

// SetPipeline is a no-op.
func (c *ComputePassEncoder) SetPipeline(_ hal.ComputePipeline) {}

// SetBindGroup is a no-op.
func (c *ComputePassEncoder) SetBindGroup(_ uint32, _ hal.BindGroup, _ []uint32) {}

// Dispatch is a no-op.
func (c *ComputePassEncoder) Dispatch(_, _, _ uint32) {}

// DispatchIndirect is a no-op.
func (c *ComputePassEncoder) DispatchIndirect(_ hal.Buffer, _ uint64) {}
