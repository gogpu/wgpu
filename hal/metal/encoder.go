// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"fmt"
	"sync"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/indirect"
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
	icbOwners []*indexedICBOwnership
	finished  *CommandBuffer
	passState renderPassPendingState
	recordErr error
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
	e.recordErr = nil

	// Scoped autorelease pool — drain immediately after creating the command buffer.
	// The command buffer is Retained so it survives the pool drain.
	// This prevents LIFO violations when pools from different frames overlap
	// on the ObjC autorelease pool stack (macOS Tahoe SIGABRT fix).
	pool := NewAutoreleasePool()
	defer pool.Drain()
	e.cmdBuffer = MsgSend(e.device.commandQueue, Sel("commandBuffer"))
	if e.cmdBuffer == 0 {
		return fmt.Errorf("metal: failed to create command buffer")
	}
	Retain(e.cmdBuffer)
	if label != "" {
		nsLabel := NSString(label)
		_ = MsgSend(e.cmdBuffer, Sel("setLabel:"), uintptr(nsLabel))
		Release(nsLabel)
	}
	hal.Logger().Debug("metal: encoding started", "label", label)
	return nil
}

// EndEncoding finishes command recording and returns a command buffer.
// After successful call, IsRecording() returns false.
func (e *CommandEncoder) EndEncoding() (hal.CommandBuffer, error) {
	if e.cmdBuffer == 0 {
		return nil, fmt.Errorf("metal: command encoder is not recording")
	}
	if e.recordErr != nil {
		err := e.recordErr
		e.recordErr = nil
		Release(e.cmdBuffer)
		e.cmdBuffer = 0
		e.releaseICBOwners()
		return nil, err
	}
	cb := &CommandBuffer{raw: e.cmdBuffer, device: e.device, icbOwners: e.icbOwners}
	e.cmdBuffer = 0 // Recording state becomes false
	e.icbOwners = nil
	e.finished = cb
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
	e.releaseICBOwners()
	e.recordErr = nil
}

// ResetAll resets command buffers for reuse.
func (e *CommandEncoder) ResetAll(commandBuffers []hal.CommandBuffer) {
	if len(commandBuffers) == 0 {
		if e.finished != nil {
			e.finished.Destroy()
			e.finished = nil
		}
		return
	}
	for _, raw := range commandBuffers {
		if cb, ok := raw.(*CommandBuffer); ok && cb != nil {
			cb.Destroy()
			if e.finished == cb {
				e.finished = nil
			}
		}
	}
}

// Destroy releases recording, finished, and private ICB state.
func (e *CommandEncoder) Destroy() {
	if e == nil {
		return
	}
	if e.cmdBuffer != 0 {
		Release(e.cmdBuffer)
		e.cmdBuffer = 0
	}
	e.releaseICBOwners()
	e.recordErr = nil
	if e.finished != nil {
		e.finished.Destroy()
		e.finished = nil
	}
}

func (e *CommandEncoder) failRecording(err error) {
	if e != nil && err != nil && e.recordErr == nil {
		e.recordErr = err
	}
}

func (e *CommandEncoder) releaseICBOwners() {
	for _, owner := range e.icbOwners {
		owner.release()
	}
	e.icbOwners = nil
}

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
	for _, region := range regions {
		if _, _, ok := validateMetalBufferTextureCopyPlan(dstTex.format, dstTex.dimension, region.BufferLayout, region.TextureBase.Origin, region.Size); !ok {
			return
		}
	}
	pool := NewAutoreleasePool()
	defer pool.Drain()
	blitEncoder := MsgSend(e.cmdBuffer, Sel("blitCommandEncoder"))
	if blitEncoder == 0 {
		return
	}
	for _, region := range regions {
		plan, bytesPerImage, _ := validateMetalBufferTextureCopyPlan(dstTex.format, dstTex.dimension, region.BufferLayout, region.TextureBase.Origin, region.Size)
		strides := metalBlitStrides(dstTex.dimension, uint64(region.BufferLayout.BytesPerRow), bytesPerImage)
		for operation := uint32(0); operation < plan.operationCount; operation++ {
			destination, _ := plan.textureRegion(dstTex.dimension, region.TextureBase.Origin, region.Size, operation)
			sourceOffset, _ := plan.bufferOffset(region.BufferLayout.Offset, bytesPerImage, operation)
			msgSendVoid(blitEncoder, Sel("copyFromBuffer:sourceOffset:sourceBytesPerRow:sourceBytesPerImage:sourceSize:toTexture:destinationSlice:destinationLevel:destinationOrigin:"),
				argPointer(uintptr(srcBuf.raw)),
				argUint64(sourceOffset),
				argUint64(strides.bytesPerRow),
				argUint64(strides.bytesPerImage),
				argStruct(destination.size, mtlSizeType),
				argPointer(uintptr(dstTex.raw)),
				argUint64(uint64(destination.slice)),
				argUint64(uint64(region.TextureBase.MipLevel)),
				argStruct(destination.origin, mtlOriginType),
			)
		}
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
	for _, region := range regions {
		if _, _, ok := validateMetalBufferTextureCopyPlan(srcTex.format, srcTex.dimension, region.BufferLayout, region.TextureBase.Origin, region.Size); !ok {
			return
		}
	}
	pool := NewAutoreleasePool()
	defer pool.Drain()
	blitEncoder := MsgSend(e.cmdBuffer, Sel("blitCommandEncoder"))
	if blitEncoder == 0 {
		return
	}
	for _, region := range regions {
		plan, bytesPerImage, _ := validateMetalBufferTextureCopyPlan(srcTex.format, srcTex.dimension, region.BufferLayout, region.TextureBase.Origin, region.Size)
		strides := metalBlitStrides(srcTex.dimension, uint64(region.BufferLayout.BytesPerRow), bytesPerImage)
		for operation := uint32(0); operation < plan.operationCount; operation++ {
			source, _ := plan.textureRegion(srcTex.dimension, region.TextureBase.Origin, region.Size, operation)
			destinationOffset, _ := plan.bufferOffset(region.BufferLayout.Offset, bytesPerImage, operation)
			msgSendVoid(blitEncoder, Sel("copyFromTexture:sourceSlice:sourceLevel:sourceOrigin:sourceSize:toBuffer:destinationOffset:destinationBytesPerRow:destinationBytesPerImage:"),
				argPointer(uintptr(srcTex.raw)),
				argUint64(uint64(source.slice)),
				argUint64(uint64(region.TextureBase.MipLevel)),
				argStruct(source.origin, mtlOriginType),
				argStruct(source.size, mtlSizeType),
				argPointer(uintptr(dstBuf.raw)),
				argUint64(destinationOffset),
				argUint64(strides.bytesPerRow),
				argUint64(strides.bytesPerImage),
			)
		}
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
	for _, region := range regions {
		if _, ok := validateMetalTextureCopyPlan(srcTex.dimension, dstTex.dimension, region.SrcBase.Origin, region.DstBase.Origin, region.Size); !ok {
			return
		}
	}
	pool := NewAutoreleasePool()
	defer pool.Drain()
	blitEncoder := MsgSend(e.cmdBuffer, Sel("blitCommandEncoder"))
	if blitEncoder == 0 {
		return
	}
	for _, region := range regions {
		plan, _ := validateMetalTextureCopyPlan(srcTex.dimension, dstTex.dimension, region.SrcBase.Origin, region.DstBase.Origin, region.Size)
		for operation := uint32(0); operation < plan.operationCount; operation++ {
			source, _ := plan.textureRegion(srcTex.dimension, region.SrcBase.Origin, region.Size, operation)
			destination, _ := plan.textureRegion(dstTex.dimension, region.DstBase.Origin, region.Size, operation)
			msgSendVoid(blitEncoder, Sel("copyFromTexture:sourceSlice:sourceLevel:sourceOrigin:sourceSize:toTexture:destinationSlice:destinationLevel:destinationOrigin:"),
				argPointer(uintptr(srcTex.raw)),
				argUint64(uint64(source.slice)),
				argUint64(uint64(region.SrcBase.MipLevel)),
				argStruct(source.origin, mtlOriginType),
				argStruct(source.size, mtlSizeType),
				argPointer(uintptr(dstTex.raw)),
				argUint64(uint64(destination.slice)),
				argUint64(uint64(region.DstBase.MipLevel)),
				argStruct(destination.origin, mtlOriginType),
			)
		}
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
	// Keep any temporary Objective-C objects scoped to descriptor construction.
	// rpDesc is created with new, so it remains owned after this pool drains and
	// can safely outlive BeginRenderPass while native encoder creation is deferred.
	pool := NewAutoreleasePool()
	defer pool.Drain()
	rpDesc := MsgSend(ID(GetClass("MTLRenderPassDescriptor")), Sel("new"))
	if rpDesc == 0 {
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
		storeAction := storeOpToMTL(ca.StoreOp)
		if ca.ResolveTarget != nil { //nolint:nestif // sequential Metal descriptor setup
			if rtv, ok := ca.ResolveTarget.(*TextureView); ok && rtv != nil {
				_ = MsgSend(attachment, Sel("setResolveTexture:"), uintptr(rtv.raw))
				// Metal requires MultisampleResolve store action when a resolve
				// texture is set. Without this, Metal silently skips the MSAA
				// resolve and the surface stays uninitialized (purple screen).
				if storeAction == MTLStoreActionStore {
					storeAction = MTLStoreActionStoreAndMultisampleResolve
				} else {
					storeAction = MTLStoreActionMultisampleResolve
				}
			}
		}
		_ = MsgSend(attachment, Sel("setStoreAction:"), uintptr(storeAction))
	}
	if desc.DepthStencilAttachment != nil { //nolint:nestif // sequential Metal descriptor setup
		dsa := desc.DepthStencilAttachment

		// Depth attachment
		depthAttachment := MsgSend(rpDesc, Sel("depthAttachment"))
		if tv, ok := dsa.View.(*TextureView); ok && tv != nil {
			_ = MsgSend(depthAttachment, Sel("setTexture:"), uintptr(tv.raw))
		}
		_ = MsgSend(depthAttachment, Sel("setLoadAction:"), uintptr(loadOpToMTL(dsa.DepthLoadOp)))
		if dsa.DepthLoadOp == gputypes.LoadOpClear {
			msgSendVoid(depthAttachment, Sel("setClearDepth:"), argFloat64(float64(dsa.DepthClearValue)))
		}
		_ = MsgSend(depthAttachment, Sel("setStoreAction:"), uintptr(storeOpToMTL(dsa.DepthStoreOp)))

		// Stencil attachment — same texture, separate load/store/clear.
		// Metal requires both depth and stencil attachments to be configured
		// independently when using combined depth-stencil formats (e.g.
		// Depth32FloatStencil8). Without this, the stencil load action
		// defaults to MTLLoadActionDontCare, leaving stencil values
		// undefined and causing progressive rendering artifacts on Apple
		// Silicon TBDR GPUs.
		// Reference: Rust wgpu-hal metal/command.rs:705-727.
		stencilAttachment := MsgSend(rpDesc, Sel("stencilAttachment"))
		if tv, ok := dsa.View.(*TextureView); ok && tv != nil {
			_ = MsgSend(stencilAttachment, Sel("setTexture:"), uintptr(tv.raw))
		}
		_ = MsgSend(stencilAttachment, Sel("setLoadAction:"), uintptr(loadOpToMTL(dsa.StencilLoadOp)))
		if dsa.StencilLoadOp == gputypes.LoadOpClear {
			_ = MsgSend(stencilAttachment, Sel("setClearStencil:"), uintptr(dsa.StencilClearValue))
		}
		_ = MsgSend(stencilAttachment, Sel("setStoreAction:"), uintptr(storeOpToMTL(dsa.StencilStoreOp)))
	}
	// Keep the descriptor alive but delay creation of the native render encoder
	// until the first draw. Metal requires the ICB translator to run on a compute
	// encoder before the render encoder exists; retaining the descriptor gives
	// that backend-private lowering a narrow seam without journaling draw calls.
	e.passState = renderPassPendingState{}
	return &RenderPassEncoder{descriptor: rpDesc, commandEncoder: e, device: e.device, pending: &e.passState}
}

// BeginComputePass begins a compute pass.
// Returns nil if encoder is not recording (cmdBuffer == 0).
func (e *CommandEncoder) BeginComputePass(desc *hal.ComputePassDescriptor) hal.ComputePassEncoder {
	if e.cmdBuffer == 0 {
		return nil
	}
	// Scoped pool: encoder is Retained to survive pool drain.
	pool := NewAutoreleasePool()
	defer pool.Drain()
	encoder := MsgSend(e.cmdBuffer, Sel("computeCommandEncoder"))
	if encoder == 0 {
		return nil
	}
	Retain(encoder)
	if desc != nil && desc.Label != "" {
		nsLabel := NSString(desc.Label)
		_ = MsgSend(encoder, Sel("setLabel:"), uintptr(nsLabel))
		Release(nsLabel)
	}
	return &ComputePassEncoder{raw: encoder, device: e.device}
}

// CommandBuffer implements hal.CommandBuffer for Metal.
type CommandBuffer struct {
	raw       ID
	device    *Device
	drawable  ID // Attached drawable for presentation
	icbOwners []*indexedICBOwnership
	destroy   sync.Once
}

// Destroy releases the command buffer.
func (cb *CommandBuffer) Destroy() {
	if cb == nil {
		return
	}
	cb.destroy.Do(func() {
		if cb.raw != 0 {
			Release(cb.raw)
			cb.raw = 0
		}
		for _, owner := range cb.icbOwners {
			owner.release()
		}
		cb.icbOwners = nil
	})
}

// SetDrawable attaches a drawable for presentation.
// The drawable will be presented when the command buffer is submitted.
func (cb *CommandBuffer) SetDrawable(drawable ID) {
	cb.drawable = drawable
}

// RenderPassEncoder implements hal.RenderPassEncoder for Metal.
type RenderPassEncoder struct {
	raw            ID
	descriptor     ID
	commandEncoder *CommandEncoder
	device         *Device
	pipeline       *RenderPipeline
	currentLayout  *PipelineLayout // set by SetPipeline for SetBindGroup slot offsets
	indexBuffer    *Buffer
	indexFormat    gputypes.IndexFormat
	indexOffset    uint64
	pending        *renderPassPendingState
}

const (
	maxRenderBindGroups     = 4
	maxRenderDynamicOffsets = 16
)

type renderBindGroupState struct {
	group       *BindGroup
	offsets     [maxRenderDynamicOffsets]uint32
	offsetCount uint8
	set         bool
}

type renderVertexBufferState struct {
	buffer *Buffer
	offset uint64
	set    bool
}

type renderPassPendingState struct {
	bindGroups    [maxRenderBindGroups]renderBindGroupState
	vertexBuffers [maxVertexBuffers]renderVertexBufferState
	viewport      MTLViewport
	scissor       MTLScissorRect
	blend         gputypes.Color
	stencil       uint32
	viewportSet   bool
	scissorSet    bool
	blendSet      bool
	stencilSet    bool
}

func (e *RenderPassEncoder) beginNative() bool {
	if e == nil {
		return false
	}
	if e.raw != 0 {
		return true
	}
	if e.descriptor == 0 || e.commandEncoder == nil || e.commandEncoder.cmdBuffer == 0 {
		return false
	}

	pool := NewAutoreleasePool()
	encoder := MsgSend(e.commandEncoder.cmdBuffer, Sel("renderCommandEncoderWithDescriptor:"), uintptr(e.descriptor))
	Release(e.descriptor)
	e.descriptor = 0
	if encoder == 0 {
		e.commandEncoder.failRecording(fmt.Errorf("metal: failed to create render command encoder"))
		pool.Drain()
		return false
	}
	Retain(encoder)
	e.raw = encoder
	pool.Drain()
	e.replayPendingState()
	return true
}

func (e *RenderPassEncoder) replayPendingState() {
	if e.pipeline != nil {
		e.applyPipeline(e.pipeline)
	}
	if e.pending == nil {
		return
	}
	for i := range e.pending.bindGroups {
		state := &e.pending.bindGroups[i]
		if state.set {
			e.applyBindGroup(uint32(i), state.group, state.offsets[:state.offsetCount])
		}
	}
	for i := range e.pending.vertexBuffers {
		state := &e.pending.vertexBuffers[i]
		if state.set {
			e.applyVertexBuffer(uint32(i), state.buffer, state.offset)
		}
	}
	if e.pending.viewportSet {
		msgSendVoid(e.raw, Sel("setViewport:"), argStruct(e.pending.viewport, mtlViewportType))
	}
	if e.pending.scissorSet {
		msgSendVoid(e.raw, Sel("setScissorRect:"), argStruct(e.pending.scissor, mtlScissorRectType))
	}
	if e.pending.blendSet {
		e.applyBlendConstant(e.pending.blend)
	}
	if e.pending.stencilSet {
		_ = MsgSend(e.raw, Sel("setStencilReferenceValue:"), uintptr(e.pending.stencil))
	}
}

// End finishes the render pass.
func (e *RenderPassEncoder) End() {
	e.beginNative()
	if e.raw != 0 {
		_ = MsgSend(e.raw, Sel("endEncoding"))
		Release(e.raw)
		e.raw = 0
	}
	if e.descriptor != 0 {
		Release(e.descriptor)
		e.descriptor = 0
	}
}

// SetPipeline sets the render pipeline.
func (e *RenderPassEncoder) SetPipeline(pipeline hal.RenderPipeline) {
	p, ok := pipeline.(*RenderPipeline)
	if !ok || p == nil {
		return
	}
	e.pipeline = p
	e.currentLayout = p.layout // store for SetBindGroup slot offset lookup
	if e.raw == 0 {
		return
	}
	e.applyPipeline(p)
}

func (e *RenderPassEncoder) applyPipeline(p *RenderPipeline) {
	_ = MsgSend(e.raw, Sel("setRenderPipelineState:"), uintptr(p.raw))
	_ = MsgSend(e.raw, Sel("setCullMode:"), uintptr(p.cullMode))
	_ = MsgSend(e.raw, Sel("setFrontFacingWinding:"), uintptr(p.frontFace))
	if p.depthStencil != 0 {
		_ = MsgSend(e.raw, Sel("setDepthStencilState:"), uintptr(p.depthStencil))
		_ = MsgSend(e.raw, Sel("setDepthBias:slopeScale:clamp:"), uintptr(p.depthBias), uintptr(p.depthSlopeScale), uintptr(p.depthClamp))
	}
}

// bindSlotAssignment holds the computed per-type Metal slot index for a bind group entry.
type bindSlotAssignment struct {
	entryIndex int
	slot       uintptr
}

// computeBindSlots calculates per-type sequential Metal slot indices for bind group entries.
//
// Metal uses separate index spaces: [[buffer(N)]], [[texture(M)]], [[sampler(K)]].
// The naga MSL compiler auto-generates these indices sequentially per type,
// so we must count each resource type independently instead of using the
// WGSL @binding(N) number (which is unique across all types in a group).
func computeBindSlots(entries []gputypes.BindGroupEntry) (bufferSlots, textureSlots, samplerSlots []bindSlotAssignment) {
	var bufferIdx, textureIdx, samplerIdx uintptr
	for i, entry := range entries {
		switch entry.Resource.(type) {
		case gputypes.BufferBinding:
			bufferSlots = append(bufferSlots, bindSlotAssignment{entryIndex: i, slot: bufferIdx})
			bufferIdx++
		case gputypes.TextureViewBinding:
			textureSlots = append(textureSlots, bindSlotAssignment{entryIndex: i, slot: textureIdx})
			textureIdx++
		case gputypes.SamplerBinding:
			samplerSlots = append(samplerSlots, bindSlotAssignment{entryIndex: i, slot: samplerIdx})
			samplerIdx++
		}
	}
	return
}

// SetBindGroup sets a bind group by binding each resource directly on the encoder.
//
// Metal does not use argument buffers for basic resource binding. Instead, resources
// are set individually via setVertexBuffer/setFragmentBuffer, setVertexTexture/
// setFragmentTexture, and setVertexSamplerState/setFragmentSamplerState.
//
// The Metal binding index uses per-type sequential indices because naga MSL
// auto-generates [[buffer(N)]], [[texture(M)]], [[sampler(K)]] attributes
// sequentially per type.
func (e *RenderPassEncoder) SetBindGroup(index uint32, group hal.BindGroup, offsets []uint32) {
	bg, ok := group.(*BindGroup)
	if !ok || bg == nil {
		return
	}
	if e.pending != nil && index < maxRenderBindGroups && len(offsets) <= maxRenderDynamicOffsets {
		state := &e.pending.bindGroups[index]
		state.group = bg
		state.offsetCount = uint8(len(offsets))
		state.set = true
		copy(state.offsets[:], offsets)
		clear(state.offsets[len(offsets):])
	}
	if e.raw == 0 {
		return
	}
	e.applyBindGroup(index, bg, offsets)
}

func (e *RenderPassEncoder) applyBindGroup(index uint32, bg *BindGroup, offsets []uint32) {

	// Metal uses per-type sequential indices: [[buffer(N)]], [[texture(M)]], [[sampler(K)]].
	// naga MSL generates these indices sequentially across ALL bind groups in the
	// pipeline layout. Group 0 starts at 0; group 1 starts where group 0 ended, etc.
	// We use the cumulative offsets from the pipeline layout to compute the correct
	// starting slot for this group.
	//
	// Reference: Rust wgpu-hal metal/command.rs:182 (resource_indices.buffers + index).
	var bufferSlot, textureSlot, samplerSlot uintptr
	if e.currentLayout != nil && int(index) < len(e.currentLayout.groupOffsets) {
		off := e.currentLayout.groupOffsets[index]
		bufferSlot = uintptr(off.Buffers)
		textureSlot = uintptr(off.Textures)
		samplerSlot = uintptr(off.Samplers)
	}

	var dynamicIdx int
	for _, entry := range bg.entries {
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
			_ = MsgSend(e.raw, Sel("setVertexBuffer:offset:atIndex:"), res.Buffer, offset, bufferSlot)
			_ = MsgSend(e.raw, Sel("setFragmentBuffer:offset:atIndex:"), res.Buffer, offset, bufferSlot)
			bufferSlot++

		case gputypes.TextureViewBinding:
			_ = MsgSend(e.raw, Sel("setVertexTexture:atIndex:"), res.TextureView, textureSlot)
			_ = MsgSend(e.raw, Sel("setFragmentTexture:atIndex:"), res.TextureView, textureSlot)
			textureSlot++

		case gputypes.SamplerBinding:
			_ = MsgSend(e.raw, Sel("setVertexSamplerState:atIndex:"), res.Sampler, samplerSlot)
			_ = MsgSend(e.raw, Sel("setFragmentSamplerState:atIndex:"), res.Sampler, samplerSlot)
			samplerSlot++
		}
	}
}

// SetVertexBuffer sets a vertex buffer.
func (e *RenderPassEncoder) SetVertexBuffer(slot uint32, buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil || slot >= maxVertexBuffers {
		return
	}
	if e.pending != nil {
		state := &e.pending.vertexBuffers[slot]
		state.buffer = buf
		state.offset = offset
		state.set = true
	}
	if e.raw == 0 {
		return
	}
	e.applyVertexBuffer(slot, buf, offset)
}

func (e *RenderPassEncoder) applyVertexBuffer(slot uint32, buf *Buffer, offset uint64) {
	bufIdx := maxVertexBuffers - 1 - slot
	_ = MsgSend(e.raw, Sel("setVertexBuffer:offset:atIndex:"), uintptr(buf.raw), uintptr(offset), uintptr(bufIdx))
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
	if e.pending != nil {
		e.pending.viewport = viewport
		e.pending.viewportSet = true
	}
	if e.raw == 0 {
		return
	}
	msgSendVoid(e.raw, Sel("setViewport:"), argStruct(viewport, mtlViewportType))
}

// SetScissorRect sets the scissor rectangle.
func (e *RenderPassEncoder) SetScissorRect(x, y, width, height uint32) {
	scissor := MTLScissorRect{X: NSUInteger(x), Y: NSUInteger(y), Width: NSUInteger(width), Height: NSUInteger(height)}
	if e.pending != nil {
		e.pending.scissor = scissor
		e.pending.scissorSet = true
	}
	if e.raw == 0 {
		return
	}
	msgSendVoid(e.raw, Sel("setScissorRect:"), argStruct(scissor, mtlScissorRectType))
}

// SetBlendConstant sets the blend constant color.
func (e *RenderPassEncoder) SetBlendConstant(color *gputypes.Color) {
	if color == nil {
		return
	}
	if e.pending != nil {
		e.pending.blend = *color
		e.pending.blendSet = true
	}
	if e.raw == 0 {
		return
	}
	e.applyBlendConstant(*color)
}

func (e *RenderPassEncoder) applyBlendConstant(color gputypes.Color) {
	msgSendVoid(e.raw, Sel("setBlendColorRed:green:blue:alpha:"),
		argFloat32(float32(color.R)),
		argFloat32(float32(color.G)),
		argFloat32(float32(color.B)),
		argFloat32(float32(color.A)),
	)
}

// SetStencilReference sets the stencil reference value.
func (e *RenderPassEncoder) SetStencilReference(ref uint32) {
	if e.pending != nil {
		e.pending.stencil = ref
		e.pending.stencilSet = true
	}
	if e.raw == 0 {
		return
	}
	_ = MsgSend(e.raw, Sel("setStencilReferenceValue:"), uintptr(ref))
}

// Draw draws primitives.
func (e *RenderPassEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	if !e.beginNative() {
		return
	}
	_ = MsgSend(e.raw, Sel("drawPrimitives:vertexStart:vertexCount:instanceCount:baseInstance:"),
		uintptr(MTLPrimitiveTypeTriangle), uintptr(firstVertex), uintptr(vertexCount), uintptr(instanceCount), uintptr(firstInstance))
}

// DrawIndexed draws indexed primitives.
func (e *RenderPassEncoder) DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	if e.indexBuffer == nil || !e.beginNative() {
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
func (e *RenderPassEncoder) DrawIndirect(buffer hal.Buffer, offset uint64, drawCount uint32) {
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil || drawCount == 0 {
		return
	}
	if !indirect.RangeFits(buf.size, offset, 16, drawCount) {
		return
	}
	if _, ok := indirect.RecordOffset(offset, 16, drawCount-1); !ok {
		return
	}
	if !e.beginNative() {
		return
	}
	for i := uint32(0); i < drawCount; i++ {
		recordOffset, _ := indirect.RecordOffset(offset, 16, i)
		_ = MsgSend(e.raw, Sel("drawPrimitives:indirectBuffer:indirectBufferOffset:"),
			uintptr(MTLPrimitiveTypeTriangle), uintptr(buf.raw), uintptr(recordOffset))
	}
}

// DrawIndexedIndirect draws indexed primitives with GPU-generated parameters.
// Metal exposes only the single-record indirect operation, so count is lowered
// to consecutive 20-byte calls.
func (e *RenderPassEncoder) DrawIndexedIndirect(buffer hal.Buffer, offset uint64, drawCount uint32) {
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil || e.indexBuffer == nil || drawCount == 0 {
		return
	}
	if !indirect.RangeFits(buf.size, offset, 20, drawCount) {
		return
	}
	if _, ok := indirect.RecordOffset(offset, 20, drawCount-1); !ok {
		return
	}
	if e.tryFirstIndexedICB(buf, offset, drawCount) {
		return
	}
	if !e.beginNative() {
		return
	}
	indexType := indexFormatToMTL(e.indexFormat)
	for i := uint32(0); i < drawCount; i++ {
		recordOffset, ok := indirect.RecordOffset(offset, 20, i)
		if !ok {
			return
		}
		_ = MsgSend(e.raw, Sel("drawIndexedPrimitives:indexType:indexBuffer:indexBufferOffset:indirectBuffer:indirectBufferOffset:"),
			uintptr(MTLPrimitiveTypeTriangle), uintptr(indexType), uintptr(e.indexBuffer.raw), uintptr(e.indexOffset), uintptr(buf.raw), uintptr(recordOffset))
	}
}

func (e *RenderPassEncoder) tryFirstIndexedICB(buffer *Buffer, offset uint64, count uint32) bool {
	commands, ok := e.prepareIndexedICB(buffer, offset, count)
	if !ok {
		return false
	}
	if !e.beginNative() {
		return true
	}
	return e.executeIndexedICB(commands, buffer)
}

func indexedIndirectRecordOffset(offset uint64, index uint32) (uint64, bool) {
	return indirect.RecordOffset(offset, 20, index)
}

// ExecuteBundle executes a pre-recorded render bundle.
func (e *RenderPassEncoder) ExecuteBundle(_ hal.RenderBundle) {}

// ComputePassEncoder implements hal.ComputePassEncoder for Metal.
type ComputePassEncoder struct {
	raw           ID
	device        *Device
	pipeline      *ComputePipeline
	currentLayout *PipelineLayout // set by SetPipeline for SetBindGroup slot offsets
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
	e.currentLayout = p.layout // store for SetBindGroup slot offset lookup
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

	// Metal uses per-type sequential indices across all bind groups.
	// See RenderPassEncoder.SetBindGroup for detailed explanation.
	var bufferSlot, textureSlot, samplerSlot uintptr
	if e.currentLayout != nil && int(index) < len(e.currentLayout.groupOffsets) {
		off := e.currentLayout.groupOffsets[index]
		bufferSlot = uintptr(off.Buffers)
		textureSlot = uintptr(off.Textures)
		samplerSlot = uintptr(off.Samplers)
	}

	var dynamicIdx int
	for _, entry := range bg.entries {
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
			_ = MsgSend(e.raw, Sel("setBuffer:offset:atIndex:"), res.Buffer, offset, bufferSlot)
			bufferSlot++

		case gputypes.TextureViewBinding:
			_ = MsgSend(e.raw, Sel("setTexture:atIndex:"), res.TextureView, textureSlot)
			textureSlot++

		case gputypes.SamplerBinding:
			_ = MsgSend(e.raw, Sel("setSamplerState:atIndex:"), res.Sampler, samplerSlot)
			samplerSlot++
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
	e.insertComputeBarrier()
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
	e.insertComputeBarrier()
}

// insertComputeBarrier inserts a buffer+texture memory barrier after a dispatch.
// VAL-008: ensures storage buffer/texture writes from one dispatch are visible
// to subsequent dispatches. Uses memoryBarrierWithScope: on the compute encoder.
func (e *ComputePassEncoder) insertComputeBarrier() {
	if e.raw == 0 {
		return
	}
	scope := MTLBarrierScopeBuffers | MTLBarrierScopeTextures
	_ = MsgSend(e.raw, Sel("memoryBarrierWithScope:"), uintptr(scope))
}

// msgSendClearColor sends an Objective-C message with an MTLClearColor argument.
func msgSendClearColor(obj ID, sel SEL, color MTLClearColor) {
	if obj == 0 {
		return
	}
	msgSendVoid(obj, sel, argStruct(color, mtlClearColorType))
}
