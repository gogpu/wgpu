// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/metal/mslmap"
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
	if !e.validateBufferTextureCopies("CopyBufferToTexture", srcBuf, dstTex, regions) {
		return
	}
	pool := NewAutoreleasePool()
	defer pool.Drain()
	blitEncoder := MsgSend(e.cmdBuffer, Sel("blitCommandEncoder"))
	if blitEncoder == 0 {
		return
	}
	for i := range regions {
		region := &regions[i]
		blit, _, _ := validateMetalBufferTextureCopy(srcBuf.size, dstTex.format, dstTex.dimension, region)
		for operation := uint32(0); operation < blit.plan.operationCount; operation++ {
			destination, _ := blit.plan.textureRegion(dstTex.dimension, region.TextureBase.Origin, region.Size, operation)
			sourceOffset, _ := blit.bufferOffset(region, operation)
			msgSendVoid(blitEncoder, Sel("copyFromBuffer:sourceOffset:sourceBytesPerRow:sourceBytesPerImage:sourceSize:toTexture:destinationSlice:destinationLevel:destinationOrigin:options:"),
				argPointer(uintptr(srcBuf.raw)),
				argUint64(sourceOffset),
				argUint64(blit.strides.bytesPerRow),
				argUint64(blit.strides.bytesPerImage),
				argStruct(destination.size, mtlSizeType),
				argPointer(uintptr(dstTex.raw)),
				argUint64(uint64(destination.slice)),
				argUint64(uint64(region.TextureBase.MipLevel)),
				argStruct(destination.origin, mtlOriginType),
				argUint64(uint64(blit.option)),
			)
		}
	}
	_ = MsgSend(blitEncoder, Sel("endEncoding"))
}

// validateBufferTextureCopies checks every region of a buffer<->texture blit
// before the first Metal call, so an invalid region records an error and
// leaves the command buffer without a partial copy. It reports whether there
// is anything to encode.
func (e *CommandEncoder) validateBufferTextureCopies(op string, buf *Buffer, tex *Texture, regions []hal.BufferTextureCopy) bool {
	encode := false
	for i := range regions {
		_, skip, err := validateMetalBufferTextureCopy(buf.size, tex.format, tex.dimension, &regions[i])
		if err != nil {
			e.failRecording(fmt.Errorf("metal: %s: region %d: %w", op, i, err))
			return false
		}
		encode = encode || !skip
	}
	return encode
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
	if !e.validateBufferTextureCopies("CopyTextureToBuffer", dstBuf, srcTex, regions) {
		return
	}
	pool := NewAutoreleasePool()
	defer pool.Drain()
	blitEncoder := MsgSend(e.cmdBuffer, Sel("blitCommandEncoder"))
	if blitEncoder == 0 {
		return
	}
	for i := range regions {
		region := &regions[i]
		blit, _, _ := validateMetalBufferTextureCopy(dstBuf.size, srcTex.format, srcTex.dimension, region)
		for operation := uint32(0); operation < blit.plan.operationCount; operation++ {
			source, _ := blit.plan.textureRegion(srcTex.dimension, region.TextureBase.Origin, region.Size, operation)
			destinationOffset, _ := blit.bufferOffset(region, operation)
			msgSendVoid(blitEncoder, Sel("copyFromTexture:sourceSlice:sourceLevel:sourceOrigin:sourceSize:toBuffer:destinationOffset:destinationBytesPerRow:destinationBytesPerImage:options:"),
				argPointer(uintptr(srcTex.raw)),
				argUint64(uint64(source.slice)),
				argUint64(uint64(region.TextureBase.MipLevel)),
				argStruct(source.origin, mtlOriginType),
				argStruct(source.size, mtlSizeType),
				argPointer(uintptr(dstBuf.raw)),
				argUint64(destinationOffset),
				argUint64(blit.strides.bytesPerRow),
				argUint64(blit.strides.bytesPerImage),
				argUint64(uint64(blit.option)),
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

// BuildAccelerationStructures builds one or more acceleration structures.
//
// Delegates to buildAccelerationStructures in raytracing.go which creates a
// transient MTLAccelerationStructureCommandEncoder and issues build/refit
// commands for each descriptor.
//
// Reference: Rust wgpu-hal metal/command.rs:1840-1879.
func (e *CommandEncoder) BuildAccelerationStructures(descriptors []hal.BuildAccelerationStructureDescriptor) {
	if e.cmdBuffer == 0 {
		return
	}
	e.buildAccelerationStructures(descriptors)
}

// PlaceAccelerationStructureBarrier is a no-op on Metal.
//
// Metal handles acceleration structure synchronization internally through
// its command buffer scheduling model. Explicit barriers are not required.
//
// Reference: Rust wgpu-hal metal/command.rs:1881-1885 (empty body).
func (e *CommandEncoder) PlaceAccelerationStructureBarrier(_ hal.AccelerationStructureBarrier) {}

// CopyAccelerationStructure copies or compacts an acceleration structure.
//
// Delegates to copyAccelerationStructure in raytracing.go which creates a
// transient MTLAccelerationStructureCommandEncoder and issues copy/compact.
//
// Reference: Rust wgpu-hal metal/command.rs:746-764.
func (e *CommandEncoder) CopyAccelerationStructure(src, dst hal.AccelerationStructure, copyMode gputypes.AccelerationStructureCopyMode) {
	if e.cmdBuffer == 0 {
		return
	}
	e.copyAccelerationStructure(src, dst, copyMode)
}

// ReadAccelerationStructureCompactSize writes the post-compaction size
// of an acceleration structure into a buffer.
//
// Delegates to readAccelerationStructureCompactSize in raytracing.go.
//
// Reference: Rust wgpu-hal metal/command.rs:1887-1898.
func (e *CommandEncoder) ReadAccelerationStructureCompactSize(accelStruct hal.AccelerationStructure, buffer hal.Buffer, offset uint64) {
	if e.cmdBuffer == 0 {
		return
	}
	e.readAccelerationStructureCompactSize(accelStruct, buffer, offset)
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
		// The depth slice of a 3D attachment (Rust wgpu-hal metal/command.rs
		// begin_render_pass: setDepthPlane). The view's level 0 is its base
		// mip level, so the plane indexes that level's depth.
		if ca.DepthSlice != nil {
			_ = MsgSend(attachment, Sel("setDepthPlane:"), uintptr(*ca.DepthSlice))
		}
		_ = MsgSend(attachment, Sel("setLoadAction:"), uintptr(loadOpToMTL(ca.LoadOp)))
		if ca.LoadOp == gputypes.LoadOpClear {
			clearColor := MTLClearColor{Red: ca.ClearValue.R, Green: ca.ClearValue.G, Blue: ca.ClearValue.B, Alpha: ca.ClearValue.A}
			msgSendClearColor(attachment, Sel("setClearColor:"), clearColor)
		}
		storeAction := storeOpToMTL(ca.StoreOp)
		if ca.ResolveTarget != nil {
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
	if desc.DepthStencilAttachment != nil {
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

	// bindingSizes holds the usable byte size of each bound buffer, by group
	// and binding. A draw copies these sizes into naga's `_buffer_sizes`
	// argument of each stage that has one.
	bindingSizes map[[2]uint32]uint64
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

// resourceSlot returns the slot of a bind group resource in stage s: the
// group's first slot of the resource's kind plus the resource's slot in the
// group. ok is false if the stage does not see the resource or the bind group
// holds none for it.
func resourceSlot(res *bindGroupResource, s int, base mslmap.Counts) (slot uintptr, ok bool) {
	rel := res.slots[s]
	if rel < 0 || res.handle == 0 {
		return 0, false
	}
	return uintptr(base.Of(res.kind) + rel), true
}

// resourceOffset returns the buffer offset of a bind group resource with its
// dynamic offset, if it has one.
func resourceOffset(res *bindGroupResource, offsets []uint32) uintptr {
	offset := res.offset
	if res.dynamicIndex >= 0 && res.dynamicIndex < len(offsets) {
		offset += uint64(offsets[res.dynamicIndex])
	}
	return uintptr(offset)
}

// SetBindGroup sets a bind group by binding each resource directly on the encoder.
//
// Metal does not use argument buffers for basic resource binding. Instead, resources
// are set individually via setVertexBuffer/setFragmentBuffer, setVertexTexture/
// setFragmentTexture, and setVertexSamplerState/setFragmentSamplerState, at the
// slots the pipeline layout gives each stage (mslmap.StageLayout).
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

// renderStageSelectors holds the per-kind encoder selectors of the vertex and
// fragment stages.
var renderStageSelectors = [2]struct {
	stage                    int
	buffer, texture, sampler string
}{
	{stageVertex, "setVertexBuffer:offset:atIndex:", "setVertexTexture:atIndex:", "setVertexSamplerState:atIndex:"},
	{stageFragment, "setFragmentBuffer:offset:atIndex:", "setFragmentTexture:atIndex:", "setFragmentSamplerState:atIndex:"},
}

// applyBindGroup binds each resource of bg for the vertex and fragment
// stages that see it, at the slots the current pipeline layout gives each
// stage, with the dynamic offsets in binding order.
//
// Reference: Rust wgpu-hal metal/command.rs set_bind_group and
// update_bind_group_state.
func (e *RenderPassEncoder) applyBindGroup(index uint32, bg *BindGroup, offsets []uint32) {
	for _, sel := range renderStageSelectors {
		base := e.currentLayout.groupBase(sel.stage, index)
		for i := range bg.resources {
			res := &bg.resources[i]
			slot, ok := resourceSlot(res, sel.stage, base)
			if !ok {
				continue
			}
			switch res.kind {
			case mslmap.KindBuffer:
				_ = MsgSend(e.raw, Sel(sel.buffer), res.handle, resourceOffset(res, offsets), slot)
			case mslmap.KindTexture:
				_ = MsgSend(e.raw, Sel(sel.texture), res.handle, slot)
			case mslmap.KindSampler:
				_ = MsgSend(e.raw, Sel(sel.sampler), res.handle, slot)
			}
		}
	}
	e.bindingSizes = recordBindingSizes(e.bindingSizes, index, bg)
}

// recordBindingSizes saves the binding size of each storage buffer of bg, at
// group index, for naga's `_buffer_sizes` argument; it returns sizes, made
// if it was nil.
func recordBindingSizes(sizes map[[2]uint32]uint64, index uint32, bg *BindGroup) map[[2]uint32]uint64 {
	for i := range bg.resources {
		res := &bg.resources[i]
		if !res.sized {
			continue
		}
		if sizes == nil {
			sizes = make(map[[2]uint32]uint64)
		}
		sizes[[2]uint32{index, res.binding}] = res.size
	}
	return sizes
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
func (e *RenderPassEncoder) SetViewport(vp gputypes.Viewport) {
	viewport := MTLViewport{OriginX: float64(vp.X), OriginY: float64(vp.Y), Width: float64(vp.Width), Height: float64(vp.Height), ZNear: float64(vp.MinDepth), ZFar: float64(vp.MaxDepth)}
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
func (e *RenderPassEncoder) SetScissorRect(rect gputypes.ScissorRect) {
	scissor := MTLScissorRect{X: NSUInteger(rect.X), Y: NSUInteger(rect.Y), Width: NSUInteger(rect.Width), Height: NSUInteger(rect.Height)}
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

// beginDraw opens the native encoder and binds naga's `_buffer_sizes`
// argument of each stage that has one. Every draw calls it, as compute binds
// the sizes before every dispatch, so the sizes always match the bind groups
// set at the time of the draw. It reports false if the draw must be skipped.
func (e *RenderPassEncoder) beginDraw() bool {
	if !e.beginNative() {
		return false
	}
	e.bindBufferSizes()
	return true
}

// bindBufferSizes sends the `_buffer_sizes` words of the vertex and fragment
// stages of the current pipeline. Indirect command buffers inherit them: the
// ICB descriptor sets inheritBuffers.
//
// Reference: Rust wgpu-hal metal/command.rs:1325-1350 (set_render_pipeline)
// and 337-398 (set_bind_group).
func (e *RenderPassEncoder) bindBufferSizes() {
	p := e.pipeline
	if p == nil {
		return
	}
	if len(p.vertexSizes) > 0 {
		sizes := bufferSizeWords(p.vertexSizes, e.bindingSizes)
		_ = msgSendSetBytes(e.raw, Sel("setVertexBytes:length:atIndex:"),
			unsafe.Pointer(&sizes[0]), NSUInteger(4*len(sizes)), NSUInteger(p.vertexSizesSlot))
		runtime.KeepAlive(sizes)
	}
	if len(p.fragmentSizes) > 0 {
		sizes := bufferSizeWords(p.fragmentSizes, e.bindingSizes)
		_ = msgSendSetBytes(e.raw, Sel("setFragmentBytes:length:atIndex:"),
			unsafe.Pointer(&sizes[0]), NSUInteger(4*len(sizes)), NSUInteger(p.fragmentSizesSlot))
		runtime.KeepAlive(sizes)
	}
}

// Draw draws primitives.
func (e *RenderPassEncoder) Draw(args gputypes.DrawArgs) {
	if !e.beginDraw() {
		return
	}
	_ = MsgSend(e.raw, Sel("drawPrimitives:vertexStart:vertexCount:instanceCount:baseInstance:"),
		uintptr(MTLPrimitiveTypeTriangle), uintptr(args.FirstVertex), uintptr(args.VertexCount), uintptr(args.InstanceCount), uintptr(args.FirstInstance))
}

// DrawIndexed draws indexed primitives.
func (e *RenderPassEncoder) DrawIndexed(args gputypes.DrawIndexedArgs) {
	if e.indexBuffer == nil || !e.beginDraw() {
		return
	}
	indexType := indexFormatToMTL(e.indexFormat)
	indexSize := uint32(2)
	if e.indexFormat == gputypes.IndexFormatUint32 {
		indexSize = 4
	}
	offset := e.indexOffset + uint64(args.FirstIndex)*uint64(indexSize)
	_ = MsgSend(e.raw, Sel("drawIndexedPrimitives:indexCount:indexType:indexBuffer:indexBufferOffset:instanceCount:baseVertex:baseInstance:"),
		uintptr(MTLPrimitiveTypeTriangle), uintptr(args.IndexCount), uintptr(indexType),
		uintptr(e.indexBuffer.raw), uintptr(offset), uintptr(args.InstanceCount), uintptr(args.BaseVertex), uintptr(args.FirstInstance))
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
	if !e.beginDraw() {
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
	if !e.beginDraw() {
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

// DrawIndirectCount falls back to maxDrawCount sequential indirect draws on Metal.
func (e *RenderPassEncoder) DrawIndirectCount(
	buffer hal.Buffer, offset uint64,
	countBuffer hal.Buffer, countOffset uint64,
	maxDrawCount uint32,
) {
	hal.RecordIndirectCountMax(e.DrawIndirect, buffer, offset, countBuffer, countOffset, maxDrawCount)
}

// DrawIndexedIndirectCount falls back to maxDrawCount sequential indexed indirect draws.
func (e *RenderPassEncoder) DrawIndexedIndirectCount(
	buffer hal.Buffer, offset uint64,
	countBuffer hal.Buffer, countOffset uint64,
	maxDrawCount uint32,
) {
	hal.RecordIndirectCountMax(e.DrawIndexedIndirect, buffer, offset, countBuffer, countOffset, maxDrawCount)
}

func (e *RenderPassEncoder) tryFirstIndexedICB(buffer *Buffer, offset uint64, count uint32) bool {
	commands, ok := e.prepareIndexedICB(buffer, offset, count)
	if !ok {
		return false
	}
	if !e.beginDraw() {
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

	// bindingSizes holds the usable byte size of each bound buffer, by group
	// and binding. A dispatch copies these sizes into naga's `_buffer_sizes`
	// argument.
	bindingSizes map[[2]uint32]uint64
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

	base := e.currentLayout.groupBase(stageCompute, index)
	for i := range bg.resources {
		res := &bg.resources[i]
		slot, ok := resourceSlot(res, stageCompute, base)
		if !ok {
			continue
		}
		switch res.kind {
		case mslmap.KindBuffer:
			_ = MsgSend(e.raw, Sel("setBuffer:offset:atIndex:"), res.handle, resourceOffset(res, offsets), slot)
		case mslmap.KindTexture:
			_ = MsgSend(e.raw, Sel("setTexture:atIndex:"), res.handle, slot)
		case mslmap.KindSampler:
			_ = MsgSend(e.raw, Sel("setSamplerState:atIndex:"), res.handle, slot)
		}
	}
	e.bindingSizes = recordBindingSizes(e.bindingSizes, index, bg)
}

// bindBufferSizes binds naga's `_buffer_sizes` argument. Kernels need it when
// they use a runtime-sized array, either through WGSL arrayLength() or through
// naga's bounds checks on storage buffers.
//
// The MSL kernel takes one extra parameter. It holds one uint32 byte size for
// each runtime-sized array. The pipeline compiled that parameter to the buffer
// slot that follows the buffers of the pipeline layout.
//
// This binding is required. If it is missing, the kernel reads a size of zero,
// and naga's arrayLength() formula, 1 + (size - offset - stride) / stride,
// wraps around to a very large number.
//
// Reference: Rust wgpu-hal metal/command.rs:1736-1747.
func (e *ComputePassEncoder) bindBufferSizes() {
	p := e.pipeline
	if p == nil || len(p.sizesBindings) == 0 {
		return
	}
	sizes := bufferSizeWords(p.sizesBindings, e.bindingSizes)
	_ = msgSendSetBytes(e.raw, Sel("setBytes:length:atIndex:"),
		unsafe.Pointer(&sizes[0]), NSUInteger(4*len(sizes)), NSUInteger(p.sizesSlot))
	runtime.KeepAlive(sizes)
}

// bufferSizeWords returns the contents of naga's `_buffer_sizes` argument:
// the byte size of each binding in bindings, from sizes (keyed by group and
// binding). An unbound binding reads zero, as in Rust wgpu-hal.
func bufferSizeWords(bindings []ir.ResourceBinding, sizes map[[2]uint32]uint64) []uint32 {
	words := make([]uint32, len(bindings))
	for i, rb := range bindings {
		// The fields are 32-bit, so a binding above 4 GiB reports the
		// largest value that fits.
		size := sizes[[2]uint32{rb.Group, rb.Binding}]
		if size > math.MaxUint32 {
			size = math.MaxUint32
		}
		words[i] = uint32(size)
	}
	return words
}

// boundBufferSize returns the usable byte size of a buffer binding. A zero
// entry size means the rest of the buffer after the offset.
func boundBufferSize(res gputypes.BufferBinding) uint64 {
	if res.Size != 0 {
		return res.Size
	}
	if l := uint64(MsgSend(ID(res.Buffer), Sel("length"))); l > res.Offset {
		return l - res.Offset
	}
	return 0
}

// Dispatch dispatches compute workgroups.
func (e *ComputePassEncoder) Dispatch(x, y, z uint32) {
	if e.pipeline == nil {
		return // No pipeline set
	}
	e.bindBufferSizes()

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
	e.bindBufferSizes()
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
