// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// CommandEncoder implements hal.CommandEncoder for Metal.
//
// Recording state is determined by the presence of cmdBuffer (cmdBuffer != 0).
// This follows the wgpu-rs pattern where Option<CommandBuffer> presence
// indicates recording state, rather than a separate boolean flag.
type CommandEncoder struct {
	device    *Device
	cmdBuffer ID
	label     string
}

// IsRecording returns true if the encoder has an active command buffer.
// This is the canonical way to check recording state.
func (e *CommandEncoder) IsRecording() bool {
	return e.cmdBuffer != 0
}

// BeginEncoding begins command recording with an optional label.
// After successful call, IsRecording() returns true.
func (e *CommandEncoder) BeginEncoding(label string) error {
	if e.cmdBuffer != 0 {
		return fmt.Errorf("metal: encoder is already recording")
	}
	e.label = label

	// Scoped autorelease pool — drain immediately after creating the command buffer.
	// The command buffer is Retained so it survives the pool drain.
	// This prevents LIFO violations when pools from different frames overlap
	// on the ObjC autorelease pool stack (macOS Tahoe SIGABRT fix).
	pool := NewAutoreleasePool()
	e.cmdBuffer = MsgSend(e.device.commandQueue, Sel("commandBuffer"))
	if e.cmdBuffer == 0 {
		pool.Drain()
		return fmt.Errorf("metal: failed to create command buffer")
	}
	Retain(e.cmdBuffer)
	if label != "" {
		nsLabel := NSString(label)
		_ = MsgSend(e.cmdBuffer, Sel("setLabel:"), uintptr(nsLabel))
		Release(nsLabel)
	}
	pool.Drain()
	hal.Logger().Debug("metal: encoding started", "label", label)
	return nil
}

// EndEncoding finishes command recording and returns a command buffer.
// After successful call, IsRecording() returns false.
func (e *CommandEncoder) EndEncoding() (hal.CommandBuffer, error) {
	if e.cmdBuffer == 0 {
		return nil, fmt.Errorf("metal: command encoder is not recording")
	}
	cb := &CommandBuffer{raw: e.cmdBuffer, device: e.device}
	e.cmdBuffer = 0 // Recording state becomes false
	hal.Logger().Debug("metal: encoding ended")
	return cb, nil
}

// DiscardEncoding discards the encoder without creating a command buffer.
// After call, IsRecording() returns false.
func (e *CommandEncoder) DiscardEncoding() {
	if e.cmdBuffer != 0 {
		hal.Logger().Debug("metal: encoding discarded")
		Release(e.cmdBuffer)
		e.cmdBuffer = 0 // Recording state becomes false
	}
}

// ResetAll resets command buffers for reuse.
func (e *CommandEncoder) ResetAll(_ []hal.CommandBuffer) {}

// TransitionBuffers transitions buffer states for synchronization.
func (e *CommandEncoder) TransitionBuffers(_ []hal.BufferBarrier) {}

// TransitionTextures transitions texture states for synchronization.
func (e *CommandEncoder) TransitionTextures(_ []hal.TextureBarrier) {}

// ClearBuffer clears a buffer region to zero.
func (e *CommandEncoder) ClearBuffer(buffer hal.Buffer, offset, size uint64) {
	if e.cmdBuffer == 0 {
		return
	}
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil {
		return
	}
	pool := NewAutoreleasePool()
	defer pool.Drain()
	blitEncoder := MsgSend(e.cmdBuffer, Sel("blitCommandEncoder"))
	if blitEncoder == 0 {
		return
	}
	_ = MsgSend(blitEncoder, Sel("fillBuffer:range:value:"), uintptr(buf.raw), uintptr(offset), uintptr(size), uintptr(0))
	_ = MsgSend(blitEncoder, Sel("endEncoding"))
}

// CopyBufferToBuffer copies data between buffers.
func (e *CommandEncoder) CopyBufferToBuffer(src, dst hal.Buffer, regions []hal.BufferCopy) {
	if e.cmdBuffer == 0 || len(regions) == 0 {
		return
	}
	srcBuf, ok := src.(*Buffer)
	if !ok || srcBuf == nil {
		return
	}
	dstBuf, ok := dst.(*Buffer)
	if !ok || dstBuf == nil {
		return
	}
	pool := NewAutoreleasePool()
	defer pool.Drain()
	blitEncoder := MsgSend(e.cmdBuffer, Sel("blitCommandEncoder"))
	if blitEncoder == 0 {
		return
	}
	for _, region := range regions {
		_ = MsgSend(blitEncoder, Sel("copyFromBuffer:sourceOffset:toBuffer:destinationOffset:size:"),
			uintptr(srcBuf.raw), uintptr(region.SrcOffset), uintptr(dstBuf.raw), uintptr(region.DstOffset), uintptr(region.Size))
	}
	_ = MsgSend(blitEncoder, Sel("endEncoding"))
}

// CopyBufferToTexture copies data from a buffer to a texture.
func (e *CommandEncoder) CopyBufferToTexture(src hal.Buffer, dst hal.Texture, regions []hal.BufferTextureCopy) {
	if e.cmdBuffer == 0 || len(regions) == 0 {
		return
	}
	srcBuf, ok := src.(*Buffer)
	if !ok || srcBuf == nil {
		return
	}
	dstTex, ok := dst.(*Texture)
	if !ok || dstTex == nil {
		return
	}
	pool := NewAutoreleasePool()
	defer pool.Drain()
	blitEncoder := MsgSend(e.cmdBuffer, Sel("blitCommandEncoder"))
	if blitEncoder == 0 {
		return
	}
	for _, region := range regions {
		sourceSize := MTLSize{Width: NSUInteger(region.Size.Width), Height: NSUInteger(region.Size.Height), Depth: NSUInteger(region.Size.DepthOrArrayLayers)}
		destOrigin := MTLOrigin{X: NSUInteger(region.TextureBase.Origin.X), Y: NSUInteger(region.TextureBase.Origin.Y), Z: NSUInteger(region.TextureBase.Origin.Z)}
		bytesPerRow := region.BufferLayout.BytesPerRow
		bytesPerImage := region.BufferLayout.RowsPerImage * bytesPerRow
		msgSendVoid(blitEncoder, Sel("copyFromBuffer:sourceOffset:sourceBytesPerRow:sourceBytesPerImage:sourceSize:toTexture:destinationSlice:destinationLevel:destinationOrigin:"),
			argPointer(uintptr(srcBuf.raw)),
			argUint64(uint64(region.BufferLayout.Offset)),
			argUint64(uint64(bytesPerRow)),
			argUint64(uint64(bytesPerImage)),
			argStruct(sourceSize, mtlSizeType),
			argPointer(uintptr(dstTex.raw)),
			argUint64(uint64(region.TextureBase.Origin.Z)),
			argUint64(uint64(region.TextureBase.MipLevel)),
			argStruct(destOrigin, mtlOriginType),
		)
	}
	_ = MsgSend(blitEncoder, Sel("endEncoding"))
}

// CopyTextureToBuffer copies data from a texture to a buffer.
func (e *CommandEncoder) CopyTextureToBuffer(src hal.Texture, dst hal.Buffer, regions []hal.BufferTextureCopy) {
	if e.cmdBuffer == 0 || len(regions) == 0 {
		return
	}
	srcTex, ok := src.(*Texture)
	if !ok || srcTex == nil {
		return
	}
	dstBuf, ok := dst.(*Buffer)
	if !ok || dstBuf == nil {
		return
	}
	pool := NewAutoreleasePool()
	defer pool.Drain()
	blitEncoder := MsgSend(e.cmdBuffer, Sel("blitCommandEncoder"))
	if blitEncoder == 0 {
		return
	}
	for _, region := range regions {
		sourceSize := MTLSize{Width: NSUInteger(region.Size.Width), Height: NSUInteger(region.Size.Height), Depth: NSUInteger(region.Size.DepthOrArrayLayers)}
		sourceOrigin := MTLOrigin{X: NSUInteger(region.TextureBase.Origin.X), Y: NSUInteger(region.TextureBase.Origin.Y), Z: NSUInteger(region.TextureBase.Origin.Z)}
		bytesPerRow := region.BufferLayout.BytesPerRow
		bytesPerImage := region.BufferLayout.RowsPerImage * bytesPerRow
		msgSendVoid(blitEncoder, Sel("copyFromTexture:sourceSlice:sourceLevel:sourceOrigin:sourceSize:toBuffer:destinationOffset:destinationBytesPerRow:destinationBytesPerImage:"),
			argPointer(uintptr(srcTex.raw)),
			argUint64(uint64(region.TextureBase.Origin.Z)),
			argUint64(uint64(region.TextureBase.MipLevel)),
			argStruct(sourceOrigin, mtlOriginType),
			argStruct(sourceSize, mtlSizeType),
			argPointer(uintptr(dstBuf.raw)),
			argUint64(uint64(region.BufferLayout.Offset)),
			argUint64(uint64(bytesPerRow)),
			argUint64(uint64(bytesPerImage)),
		)
	}
	_ = MsgSend(blitEncoder, Sel("endEncoding"))
}

// CopyTextureToTexture copies data between textures.
func (e *CommandEncoder) CopyTextureToTexture(src, dst hal.Texture, regions []hal.TextureCopy) {
	if e.cmdBuffer == 0 || len(regions) == 0 {
		return
	}
	srcTex, ok := src.(*Texture)
	if !ok || srcTex == nil {
		return
	}
	dstTex, ok := dst.(*Texture)
	if !ok || dstTex == nil {
		return
	}
	pool := NewAutoreleasePool()
	defer pool.Drain()
	blitEncoder := MsgSend(e.cmdBuffer, Sel("blitCommandEncoder"))
	if blitEncoder == 0 {
		return
	}
	for _, region := range regions {
		sourceSize := MTLSize{Width: NSUInteger(region.Size.Width), Height: NSUInteger(region.Size.Height), Depth: NSUInteger(region.Size.DepthOrArrayLayers)}
		sourceOrigin := MTLOrigin{X: NSUInteger(region.SrcBase.Origin.X), Y: NSUInteger(region.SrcBase.Origin.Y), Z: NSUInteger(region.SrcBase.Origin.Z)}
		destOrigin := MTLOrigin{X: NSUInteger(region.DstBase.Origin.X), Y: NSUInteger(region.DstBase.Origin.Y), Z: NSUInteger(region.DstBase.Origin.Z)}
		msgSendVoid(blitEncoder, Sel("copyFromTexture:sourceSlice:sourceLevel:sourceOrigin:sourceSize:toTexture:destinationSlice:destinationLevel:destinationOrigin:"),
			argPointer(uintptr(srcTex.raw)),
			argUint64(uint64(region.SrcBase.Origin.Z)),
			argUint64(uint64(region.SrcBase.MipLevel)),
			argStruct(sourceOrigin, mtlOriginType),
			argStruct(sourceSize, mtlSizeType),
			argPointer(uintptr(dstTex.raw)),
			argUint64(uint64(region.DstBase.Origin.Z)),
			argUint64(uint64(region.DstBase.MipLevel)),
			argStruct(destOrigin, mtlOriginType),
		)
	}
	_ = MsgSend(blitEncoder, Sel("endEncoding"))
}

// ResolveQuerySet copies query results from a query set into a destination buffer.
// TODO: implement using Metal counter sample buffer readback.
func (e *CommandEncoder) ResolveQuerySet(_ hal.QuerySet, _, _ uint32, _ hal.Buffer, _ uint64) {
	// Stub: Metal timestamp query implementation pending.
}

// BeginRenderPass begins a render pass.
// Returns nil if encoder is not recording (cmdBuffer == 0).
func (e *CommandEncoder) BeginRenderPass(desc *hal.RenderPassDescriptor) hal.RenderPassEncoder {
	if e.cmdBuffer == 0 {
		return nil
	}
	// Scoped pool: rpDesc and other autoreleased objects are only needed during
	// encoder creation. The encoder itself is Retained to survive pool drain.
	pool := NewAutoreleasePool()
	rpDesc := MsgSend(ID(GetClass("MTLRenderPassDescriptor")), Sel("renderPassDescriptor"))
	if rpDesc == 0 {
		pool.Drain()
		return nil
	}
	colorAttachments := MsgSend(rpDesc, Sel("colorAttachments"))
	for i, ca := range desc.ColorAttachments {
		attachment := MsgSend(colorAttachments, Sel("objectAtIndexedSubscript:"), uintptr(i))
		if attachment == 0 {
			continue
		}
		if tv, ok := ca.View.(*TextureView); ok && tv != nil {
			_ = MsgSend(attachment, Sel("setTexture:"), uintptr(tv.raw))
		}
		_ = MsgSend(attachment, Sel("setLoadAction:"), uintptr(loadOpToMTL(ca.LoadOp)))
		if ca.LoadOp == gputypes.LoadOpClear {
			clearColor := MTLClearColor{Red: ca.ClearValue.R, Green: ca.ClearValue.G, Blue: ca.ClearValue.B, Alpha: ca.ClearValue.A}
			msgSendClearColor(attachment, Sel("setClearColor:"), clearColor)
		}
		_ = MsgSend(attachment, Sel("setStoreAction:"), uintptr(storeOpToMTL(ca.StoreOp)))
		if ca.ResolveTarget != nil {
			if rtv, ok := ca.ResolveTarget.(*TextureView); ok && rtv != nil {
				_ = MsgSend(attachment, Sel("setResolveTexture:"), uintptr(rtv.raw))
			}
		}
	}
	if desc.DepthStencilAttachment != nil {
		dsa := desc.DepthStencilAttachment
		depthAttachment := MsgSend(rpDesc, Sel("depthAttachment"))
		if tv, ok := dsa.View.(*TextureView); ok && tv != nil {
			_ = MsgSend(depthAttachment, Sel("setTexture:"), uintptr(tv.raw))
		}
		_ = MsgSend(depthAttachment, Sel("setLoadAction:"), uintptr(loadOpToMTL(dsa.DepthLoadOp)))
		_ = MsgSend(depthAttachment, Sel("setStoreAction:"), uintptr(storeOpToMTL(dsa.DepthStoreOp)))
	}
	encoder := MsgSend(e.cmdBuffer, Sel("renderCommandEncoderWithDescriptor:"), uintptr(rpDesc))
	if encoder == 0 {
		pool.Drain()
		return nil
	}
	Retain(encoder)
	pool.Drain() // drain now — encoder is Retained, rpDesc no longer needed
	return &RenderPassEncoder{raw: encoder, device: e.device}
}

// BeginComputePass begins a compute pass.
// Returns nil if encoder is not recording (cmdBuffer == 0).
func (e *CommandEncoder) BeginComputePass(desc *hal.ComputePassDescriptor) hal.ComputePassEncoder {
	if e.cmdBuffer == 0 {
		return nil
	}
	// Scoped pool: encoder is Retained to survive pool drain.
	pool := NewAutoreleasePool()
	encoder := MsgSend(e.cmdBuffer, Sel("computeCommandEncoder"))
	if encoder == 0 {
		pool.Drain()
		return nil
	}
	Retain(encoder)
	if desc != nil && desc.Label != "" {
		nsLabel := NSString(desc.Label)
		_ = MsgSend(encoder, Sel("setLabel:"), uintptr(nsLabel))
		Release(nsLabel)
	}
	pool.Drain()
	return &ComputePassEncoder{raw: encoder, device: e.device}
}

// CommandBuffer implements hal.CommandBuffer for Metal.
type CommandBuffer struct {
	raw      ID
	device   *Device
	drawable ID // Attached drawable for presentation
}

// Destroy releases the command buffer.
func (cb *CommandBuffer) Destroy() {
	if cb.raw != 0 {
		Release(cb.raw)
		cb.raw = 0
	}
}

// SetDrawable attaches a drawable for presentation.
// The drawable will be presented when the command buffer is submitted.
func (cb *CommandBuffer) SetDrawable(drawable ID) {
	cb.drawable = drawable
}

// RenderPassEncoder implements hal.RenderPassEncoder for Metal.
type RenderPassEncoder struct {
	raw         ID
	device      *Device
	pipeline    *RenderPipeline
	indexBuffer *Buffer
	indexFormat gputypes.IndexFormat
	indexOffset uint64
}

// End finishes the render pass.
func (e *RenderPassEncoder) End() {
	if e.raw != 0 {
		_ = MsgSend(e.raw, Sel("endEncoding"))
		Release(e.raw)
		e.raw = 0
	}
}

// SetPipeline sets the render pipeline.
func (e *RenderPassEncoder) SetPipeline(pipeline hal.RenderPipeline) {
	p, ok := pipeline.(*RenderPipeline)
	if !ok || p == nil {
		return
	}
	e.pipeline = p
	_ = MsgSend(e.raw, Sel("setRenderPipelineState:"), uintptr(p.raw))
}

// SetBindGroup sets a bind group by binding each resource directly on the encoder.
//
// Metal does not use argument buffers for basic resource binding. Instead, resources
// are set individually via setVertexBuffer/setFragmentBuffer, setVertexTexture/
// setFragmentTexture, and setVertexSamplerState/setFragmentSamplerState.
//
// The Metal binding index matches the WGSL @binding(N) value because naga MSL
// auto-generates [[buffer(N)]], [[texture(N)]], [[sampler(N)]] attributes directly
// from the binding number when PerEntryPointMap is nil.
func (e *RenderPassEncoder) SetBindGroup(index uint32, group hal.BindGroup, offsets []uint32) {
	bg, ok := group.(*BindGroup)
	if !ok || bg == nil {
		return
	}

	var dynamicIdx int
	for _, entry := range bg.entries {
		slot := uintptr(entry.Binding)

		switch res := entry.Resource.(type) {
		case gputypes.BufferBinding:
			offset := uintptr(res.Offset)
			// Apply dynamic offset if the layout entry has HasDynamicOffset.
			if dynamicIdx < len(offsets) && bg.layout != nil {
				for _, le := range bg.layout.entries {
					if le.Binding == entry.Binding && le.Buffer != nil && le.Buffer.HasDynamicOffset {
						offset += uintptr(offsets[dynamicIdx])
						dynamicIdx++
						break
					}
				}
			}
			_ = MsgSend(e.raw, Sel("setVertexBuffer:offset:atIndex:"), res.Buffer, offset, slot)
			_ = MsgSend(e.raw, Sel("setFragmentBuffer:offset:atIndex:"), res.Buffer, offset, slot)

		case gputypes.TextureViewBinding:
			_ = MsgSend(e.raw, Sel("setVertexTexture:atIndex:"), res.TextureView, slot)
			_ = MsgSend(e.raw, Sel("setFragmentTexture:atIndex:"), res.TextureView, slot)

		case gputypes.SamplerBinding:
			_ = MsgSend(e.raw, Sel("setVertexSamplerState:atIndex:"), res.Sampler, slot)
			_ = MsgSend(e.raw, Sel("setFragmentSamplerState:atIndex:"), res.Sampler, slot)
		}
	}
}

// SetVertexBuffer sets a vertex buffer.
func (e *RenderPassEncoder) SetVertexBuffer(slot uint32, buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil {
		return
	}
	_ = MsgSend(e.raw, Sel("setVertexBuffer:offset:atIndex:"), uintptr(buf.raw), uintptr(offset), uintptr(slot))
}

// SetIndexBuffer sets the index buffer.
func (e *RenderPassEncoder) SetIndexBuffer(buffer hal.Buffer, format gputypes.IndexFormat, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil {
		return
	}
	e.indexBuffer = buf
	e.indexFormat = format
	e.indexOffset = offset
}

// SetViewport sets the viewport.
func (e *RenderPassEncoder) SetViewport(x, y, width, height, minDepth, maxDepth float32) {
	viewport := MTLViewport{OriginX: float64(x), OriginY: float64(y), Width: float64(width), Height: float64(height), ZNear: float64(minDepth), ZFar: float64(maxDepth)}
	msgSendVoid(e.raw, Sel("setViewport:"), argStruct(viewport, mtlViewportType))
}

// SetScissorRect sets the scissor rectangle.
func (e *RenderPassEncoder) SetScissorRect(x, y, width, height uint32) {
	scissor := MTLScissorRect{X: NSUInteger(x), Y: NSUInteger(y), Width: NSUInteger(width), Height: NSUInteger(height)}
	msgSendVoid(e.raw, Sel("setScissorRect:"), argStruct(scissor, mtlScissorRectType))
}

// SetBlendConstant sets the blend constant color.
func (e *RenderPassEncoder) SetBlendConstant(color *gputypes.Color) {
	if color == nil {
		return
	}
	msgSendVoid(e.raw, Sel("setBlendColorRed:green:blue:alpha:"),
		argFloat32(float32(color.R)),
		argFloat32(float32(color.G)),
		argFloat32(float32(color.B)),
		argFloat32(float32(color.A)),
	)
}

// SetStencilReference sets the stencil reference value.
func (e *RenderPassEncoder) SetStencilReference(ref uint32) {
	_ = MsgSend(e.raw, Sel("setStencilReferenceValue:"), uintptr(ref))
}

// Draw draws primitives.
func (e *RenderPassEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	_ = MsgSend(e.raw, Sel("drawPrimitives:vertexStart:vertexCount:instanceCount:baseInstance:"),
		uintptr(MTLPrimitiveTypeTriangle), uintptr(firstVertex), uintptr(vertexCount), uintptr(instanceCount), uintptr(firstInstance))
}

// DrawIndexed draws indexed primitives.
func (e *RenderPassEncoder) DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	if e.indexBuffer == nil {
		return
	}
	indexType := indexFormatToMTL(e.indexFormat)
	indexSize := uint32(2)
	if e.indexFormat == gputypes.IndexFormatUint32 {
		indexSize = 4
	}
	offset := e.indexOffset + uint64(firstIndex)*uint64(indexSize)
	_ = MsgSend(e.raw, Sel("drawIndexedPrimitives:indexCount:indexType:indexBuffer:indexBufferOffset:instanceCount:baseVertex:baseInstance:"),
		uintptr(MTLPrimitiveTypeTriangle), uintptr(indexCount), uintptr(indexType),
		uintptr(e.indexBuffer.raw), uintptr(offset), uintptr(instanceCount), uintptr(baseVertex), uintptr(firstInstance))
}

// DrawIndirect draws primitives with GPU-generated parameters.
func (e *RenderPassEncoder) DrawIndirect(buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil {
		return
	}
	_ = MsgSend(e.raw, Sel("drawPrimitives:indirectBuffer:indirectBufferOffset:"),
		uintptr(MTLPrimitiveTypeTriangle), uintptr(buf.raw), uintptr(offset))
}

// DrawIndexedIndirect draws indexed primitives with GPU-generated parameters.
func (e *RenderPassEncoder) DrawIndexedIndirect(buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil || e.indexBuffer == nil {
		return
	}
	indexType := indexFormatToMTL(e.indexFormat)
	_ = MsgSend(e.raw, Sel("drawIndexedPrimitives:indexType:indexBuffer:indexBufferOffset:indirectBuffer:indirectBufferOffset:"),
		uintptr(MTLPrimitiveTypeTriangle), uintptr(indexType), uintptr(e.indexBuffer.raw), uintptr(e.indexOffset), uintptr(buf.raw), uintptr(offset))
}

// ExecuteBundle executes a pre-recorded render bundle.
func (e *RenderPassEncoder) ExecuteBundle(_ hal.RenderBundle) {}

// ComputePassEncoder implements hal.ComputePassEncoder for Metal.
type ComputePassEncoder struct {
	raw      ID
	device   *Device
	pipeline *ComputePipeline
}

// End finishes the compute pass.
func (e *ComputePassEncoder) End() {
	if e.raw != 0 {
		_ = MsgSend(e.raw, Sel("endEncoding"))
		Release(e.raw)
		e.raw = 0
	}
}

// SetPipeline sets the compute pipeline.
func (e *ComputePassEncoder) SetPipeline(pipeline hal.ComputePipeline) {
	p, ok := pipeline.(*ComputePipeline)
	if !ok || p == nil {
		return
	}
	e.pipeline = p
	_ = MsgSend(e.raw, Sel("setComputePipelineState:"), uintptr(p.raw))
}

// SetBindGroup sets a bind group by binding each resource directly on the compute encoder.
//
// See RenderPassEncoder.SetBindGroup for the binding index convention.
func (e *ComputePassEncoder) SetBindGroup(index uint32, group hal.BindGroup, offsets []uint32) {
	bg, ok := group.(*BindGroup)
	if !ok || bg == nil {
		return
	}

	var dynamicIdx int
	for _, entry := range bg.entries {
		slot := uintptr(entry.Binding)

		switch res := entry.Resource.(type) {
		case gputypes.BufferBinding:
			offset := uintptr(res.Offset)
			if dynamicIdx < len(offsets) && bg.layout != nil {
				for _, le := range bg.layout.entries {
					if le.Binding == entry.Binding && le.Buffer != nil && le.Buffer.HasDynamicOffset {
						offset += uintptr(offsets[dynamicIdx])
						dynamicIdx++
						break
					}
				}
			}
			_ = MsgSend(e.raw, Sel("setBuffer:offset:atIndex:"), res.Buffer, offset, slot)

		case gputypes.TextureViewBinding:
			_ = MsgSend(e.raw, Sel("setTexture:atIndex:"), res.TextureView, slot)

		case gputypes.SamplerBinding:
			_ = MsgSend(e.raw, Sel("setSamplerState:atIndex:"), res.Sampler, slot)
		}
	}
}

// Dispatch dispatches compute workgroups.
func (e *ComputePassEncoder) Dispatch(x, y, z uint32) {
	if e.pipeline == nil {
		return // No pipeline set
	}

	threadgroupsPerGrid := MTLSize{Width: NSUInteger(x), Height: NSUInteger(y), Depth: NSUInteger(z)}
	// Use pipeline's workgroup size instead of hardcoded value
	threadsPerThreadgroup := e.pipeline.workgroupSize

	msgSendVoid(e.raw, Sel("dispatchThreadgroups:threadsPerThreadgroup:"),
		argStruct(threadgroupsPerGrid, mtlSizeType),
		argStruct(threadsPerThreadgroup, mtlSizeType),
	)
}

// DispatchIndirect dispatches compute work with GPU-generated parameters.
func (e *ComputePassEncoder) DispatchIndirect(buffer hal.Buffer, offset uint64) {
	if e.pipeline == nil {
		return // No pipeline set
	}
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil {
		return
	}
	// Use pipeline's workgroup size instead of hardcoded value
	threadsPerThreadgroup := e.pipeline.workgroupSize
	msgSendVoid(e.raw, Sel("dispatchThreadgroupsWithIndirectBuffer:indirectBufferOffset:threadsPerThreadgroup:"),
		argPointer(uintptr(buf.raw)),
		argUint64(offset),
		argStruct(threadsPerThreadgroup, mtlSizeType),
	)
}

// msgSendClearColor sends an Objective-C message with an MTLClearColor argument.
func msgSendClearColor(obj ID, sel SEL, color MTLClearColor) {
	if obj == 0 {
		return
	}
	msgSendVoid(obj, sel, argStruct(color, mtlClearColorType))
}
