// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// RenderBundle is a pre-recorded set of render commands.
type RenderBundle struct {
	device        *Device
	commandBuffer vk.CommandBuffer
}

// Destroy releases the render bundle resources.
func (b *RenderBundle) Destroy() {
	if b.device != nil {
		b.device.DestroyRenderBundle(b)
	}
}

// RenderBundleEncoder records commands into a render bundle.
type RenderBundleEncoder struct {
	device        *Device
	commandBuffer vk.CommandBuffer
	pipeline      *RenderPipeline
	finished      bool
}

// SetPipeline sets the active render pipeline.
func (e *RenderBundleEncoder) SetPipeline(pipeline hal.RenderPipeline) {
	if e.finished {
		return
	}
	vkPipeline, ok := pipeline.(*RenderPipeline)
	if !ok || vkPipeline == nil {
		return
	}
	e.pipeline = vkPipeline
	e.device.cmds.CmdBindPipeline(e.commandBuffer, vk.PipelineBindPointGraphics, vkPipeline.handle)
}

// SetBindGroup sets a bind group for the given index.
func (e *RenderBundleEncoder) SetBindGroup(index uint32, group hal.BindGroup, offsets []uint32) {
	if e.finished || e.pipeline == nil {
		return
	}
	vkGroup, ok := group.(*BindGroup)
	if !ok || vkGroup == nil {
		return
	}

	var pOffsets *uint32
	if len(offsets) > 0 {
		pOffsets = &offsets[0]
	}

	e.device.cmds.CmdBindDescriptorSets(
		e.commandBuffer,
		vk.PipelineBindPointGraphics,
		e.pipeline.layout,
		index,
		1,
		&vkGroup.handle,
		uint32(len(offsets)),
		pOffsets,
	)
}

// SetVertexBuffer sets a vertex buffer for the given slot.
func (e *RenderBundleEncoder) SetVertexBuffer(slot uint32, buffer hal.Buffer, offset uint64) {
	if e.finished {
		return
	}
	vkBuffer, ok := buffer.(*Buffer)
	if !ok || vkBuffer == nil {
		return
	}

	vkOffset := vk.DeviceSize(offset)
	e.device.cmds.CmdBindVertexBuffers(e.commandBuffer, slot, 1, &vkBuffer.handle, &vkOffset)
}

// SetIndexBuffer sets the index buffer.
func (e *RenderBundleEncoder) SetIndexBuffer(buffer hal.Buffer, format gputypes.IndexFormat, offset uint64) {
	if e.finished {
		return
	}
	vkBuffer, ok := buffer.(*Buffer)
	if !ok || vkBuffer == nil {
		return
	}

	var indexType vk.IndexType
	switch format {
	case gputypes.IndexFormatUint16:
		indexType = vk.IndexTypeUint16
	case gputypes.IndexFormatUint32:
		indexType = vk.IndexTypeUint32
	default:
		indexType = vk.IndexTypeUint16
	}

	e.device.cmds.CmdBindIndexBuffer(e.commandBuffer, vkBuffer.handle, vk.DeviceSize(offset), indexType)
}

// Draw draws primitives.
func (e *RenderBundleEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	if e.finished {
		return
	}
	e.device.cmds.CmdDraw(e.commandBuffer, vertexCount, instanceCount, firstVertex, firstInstance)
}

// DrawIndexed draws indexed primitives.
func (e *RenderBundleEncoder) DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	if e.finished {
		return
	}
	e.device.cmds.CmdDrawIndexed(e.commandBuffer, indexCount, instanceCount, firstIndex, baseVertex, firstInstance)
}

// Finish finalizes the bundle and returns it.
func (e *RenderBundleEncoder) Finish() hal.RenderBundle {
	if e.finished {
		return nil
	}
	e.finished = true

	// End the secondary command buffer
	e.device.cmds.EndCommandBuffer(e.commandBuffer)

	return &RenderBundle{
		device:        e.device,
		commandBuffer: e.commandBuffer,
	}
}

// CreateRenderBundleEncoder creates a render bundle encoder.
func (d *Device) CreateRenderBundleEncoder(_ *hal.RenderBundleEncoderDescriptor) (hal.RenderBundleEncoder, error) {
	// Allocate a secondary command buffer
	allocInfo := vk.CommandBufferAllocateInfo{
		SType:              vk.StructureTypeCommandBufferAllocateInfo,
		CommandPool:        d.commandPool,
		Level:              vk.CommandBufferLevelSecondary,
		CommandBufferCount: 1,
	}

	var cmdBuffer vk.CommandBuffer
	result := d.cmds.AllocateCommandBuffers(d.handle, &allocInfo, &cmdBuffer)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: failed to allocate secondary command buffer: %d", result)
	}

	// Begin the secondary command buffer with inheritance info
	// Note: We use VK_COMMAND_BUFFER_USAGE_RENDER_PASS_CONTINUE_BIT to indicate
	// this command buffer will be executed inside a render pass.
	inheritanceInfo := vk.CommandBufferInheritanceInfo{
		SType: vk.StructureTypeCommandBufferInheritanceInfo,
		// RenderPass and Framebuffer can be VK_NULL_HANDLE when using dynamic rendering
		// or when the secondary command buffer doesn't depend on specific pass/framebuffer
	}

	beginInfo := vk.CommandBufferBeginInfo{
		SType: vk.StructureTypeCommandBufferBeginInfo,
		Flags: vk.CommandBufferUsageFlags(
			vk.CommandBufferUsageRenderPassContinueBit |
				vk.CommandBufferUsageSimultaneousUseBit,
		),
		PInheritanceInfo: &inheritanceInfo,
	}

	result = d.cmds.BeginCommandBuffer(cmdBuffer, &beginInfo)
	if result != vk.Success {
		d.cmds.FreeCommandBuffers(d.handle, d.commandPool, 1, &cmdBuffer)
		return nil, fmt.Errorf("vulkan: failed to begin secondary command buffer: %d", result)
	}

	return &RenderBundleEncoder{
		device:        d,
		commandBuffer: cmdBuffer,
	}, nil
}

// DestroyRenderBundle destroys a render bundle.
func (d *Device) DestroyRenderBundle(bundle hal.RenderBundle) {
	vkBundle, ok := bundle.(*RenderBundle)
	if !ok || vkBundle == nil {
		return
	}

	if vkBundle.commandBuffer != 0 {
		d.cmds.FreeCommandBuffers(d.handle, d.commandPool, 1, &vkBundle.commandBuffer)
		vkBundle.commandBuffer = 0
	}
	vkBundle.device = nil
}
