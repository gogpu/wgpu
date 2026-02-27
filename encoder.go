package wgpu

import (
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

// CommandEncoder records GPU commands for later submission.
//
// A command encoder is single-use. After calling Finish(), the encoder
// cannot be used again. Call Device.CreateCommandEncoder() to create a new one.
//
// NOT thread-safe - do not use from multiple goroutines.
type CommandEncoder struct {
	core     *core.CoreCommandEncoder
	device   *Device
	released bool
}

// BeginRenderPass begins a render pass.
// The returned RenderPassEncoder records draw commands.
// Call RenderPassEncoder.End() when done.
func (e *CommandEncoder) BeginRenderPass(desc *RenderPassDescriptor) (*RenderPassEncoder, error) {
	if e.released {
		return nil, ErrReleased
	}

	coreDesc := convertRenderPassDesc(desc)

	corePass, err := e.core.BeginRenderPass(coreDesc)
	if err != nil {
		return nil, err
	}

	return &RenderPassEncoder{core: corePass, encoder: e}, nil
}

// BeginComputePass begins a compute pass.
// The returned ComputePassEncoder records dispatch commands.
// Call ComputePassEncoder.End() when done.
func (e *CommandEncoder) BeginComputePass(desc *ComputePassDescriptor) (*ComputePassEncoder, error) {
	if e.released {
		return nil, ErrReleased
	}

	var coreDesc *core.CoreComputePassDescriptor
	if desc != nil {
		coreDesc = &core.CoreComputePassDescriptor{Label: desc.Label}
	}

	corePass, err := e.core.BeginComputePass(coreDesc)
	if err != nil {
		return nil, err
	}

	return &ComputePassEncoder{core: corePass, encoder: e}, nil
}

// CopyBufferToBuffer copies data between buffers.
func (e *CommandEncoder) CopyBufferToBuffer(src *Buffer, srcOffset uint64, dst *Buffer, dstOffset uint64, size uint64) {
	if e.released || src == nil || dst == nil {
		return
	}
	raw := e.core.RawEncoder()
	if raw == nil {
		return
	}
	halSrc := src.halBuffer()
	halDst := dst.halBuffer()
	if halSrc == nil || halDst == nil {
		return
	}
	raw.CopyBufferToBuffer(halSrc, halDst, []hal.BufferCopy{
		{SrcOffset: srcOffset, DstOffset: dstOffset, Size: size},
	})
}

// Finish completes command recording and returns a CommandBuffer.
// After calling Finish(), the encoder cannot be used again.
func (e *CommandEncoder) Finish() (*CommandBuffer, error) {
	if e.released {
		return nil, ErrReleased
	}
	e.released = true

	coreCmdBuffer, err := e.core.Finish()
	if err != nil {
		return nil, err
	}

	return &CommandBuffer{core: coreCmdBuffer, device: e.device}, nil
}

// convertRenderPassDesc converts a public descriptor to core descriptor.
func convertRenderPassDesc(desc *RenderPassDescriptor) *core.RenderPassDescriptor {
	if desc == nil {
		return &core.RenderPassDescriptor{}
	}

	coreDesc := &core.RenderPassDescriptor{
		Label: desc.Label,
	}

	for _, ca := range desc.ColorAttachments {
		coreCA := core.RenderPassColorAttachment{
			LoadOp:     ca.LoadOp,
			StoreOp:    ca.StoreOp,
			ClearValue: ca.ClearValue,
		}
		// TextureView conversion requires core.TextureView with HAL integration.
		// The core encoder handles nil views gracefully for now.
		coreDesc.ColorAttachments = append(coreDesc.ColorAttachments, coreCA)
	}

	if desc.DepthStencilAttachment != nil {
		ds := desc.DepthStencilAttachment
		coreDesc.DepthStencilAttachment = &core.RenderPassDepthStencilAttachment{
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

	return coreDesc
}

// CommandBuffer holds recorded GPU commands ready for submission.
// Created by CommandEncoder.Finish().
type CommandBuffer struct {
	core   *core.CoreCommandBuffer
	device *Device
}

// halBuffer returns the underlying HAL command buffer.
func (cb *CommandBuffer) halBuffer() hal.CommandBuffer {
	if cb.core == nil {
		return nil
	}
	return cb.core.Raw()
}
