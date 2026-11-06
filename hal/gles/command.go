// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"fmt"
	"log/slog"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
	"github.com/gogpu/wgpu/internal/indirect"
)

// copySourceIncompleteMsg is logged when a texture-to-buffer copy is dropped
// because its source cannot be attached to a complete read framebuffer.
const copySourceIncompleteMsg = "gles: copy source framebuffer is incomplete, the copy is dropped"

// Command represents a recorded GL command.
type Command interface {
	Execute(ctx *gl.Context)
}

// CommandBuffer holds recorded commands for later execution.
type CommandBuffer struct {
	commands []Command
}

// Destroy releases the command buffer resources.
func (c *CommandBuffer) Destroy() {
	c.commands = nil
}

// CommandEncoder implements hal.CommandEncoder for OpenGL.
// Platform-specific fields are defined in command_<platform>.go files.
type CommandEncoder struct {
	glCtx           *gl.Context
	commands        []Command
	label           string
	vao             uint32 // persistent VAO from Device for Core Profile
	maxTextureUnits int32  // Hardware limit passed from Device
	fullInstancing  bool   // Device.fullInstancing
	// depthStencilTextureMode is Device.depthStencilTextureMode.
	depthStencilTextureMode bool
	// maxColorAttachments is Device.maxColorAttachments.
	maxColorAttachments uint32
	samplers            samplerState
}

// BeginEncoding begins command recording.
func (e *CommandEncoder) BeginEncoding(label string) error {
	e.label = label
	e.commands = nil
	e.samplers = samplerState{}
	return nil
}

// EndEncoding finishes command recording and returns a command buffer.
func (e *CommandEncoder) EndEncoding() (hal.CommandBuffer, error) {
	cmdBuf := &CommandBuffer{
		commands: e.commands,
	}
	e.commands = nil
	return cmdBuf, nil
}

// DiscardEncoding discards the encoder.
func (e *CommandEncoder) DiscardEncoding() {
	e.commands = nil
}

// ResetAll resets command buffers for reuse.
func (e *CommandEncoder) ResetAll(_ []hal.CommandBuffer) {
	// No-op for OpenGL
}

// Destroy is a no-op for OpenGL (no persistent GPU resources owned by encoder).
func (e *CommandEncoder) Destroy() {}

// TransitionBuffers emits memory barriers for buffer state transitions.
// GLES only needs explicit barriers when transitioning FROM storage write
// (compute shader output) to any other usage (vertex, uniform, draw).
// Matches Rust wgpu-hal/gles command.rs:279-298.
func (e *CommandEncoder) TransitionBuffers(barriers []hal.BufferBarrier) {
	var bits uint32
	for _, bar := range barriers {
		if bar.Usage.OldUsage&gputypes.BufferUsageStorage != 0 {
			bits |= gl.SHADER_STORAGE_BARRIER_BIT | gl.BUFFER_UPDATE_BARRIER_BIT |
				gl.VERTEX_ATTRIB_ARRAY_BARRIER_BIT | gl.ELEMENT_ARRAY_BARRIER_BIT |
				gl.UNIFORM_BARRIER_BIT | gl.COMMAND_BARRIER_BIT
		}
	}
	if bits != 0 {
		e.commands = append(e.commands, &MemoryBarrierCommand{barriers: bits})
	}
}

// TransitionTextures emits memory barriers for texture state transitions.
// GLES only needs explicit barriers when transitioning FROM storage write
// to any other usage (texture fetch, framebuffer attachment).
// Matches Rust wgpu-hal/gles command.rs:300-327.
func (e *CommandEncoder) TransitionTextures(barriers []hal.TextureBarrier) {
	var bits uint32
	for _, bar := range barriers {
		if bar.Usage.OldUsage&gputypes.TextureUsageStorageBinding != 0 {
			bits |= gl.TEXTURE_FETCH_BARRIER_BIT | gl.SHADER_IMAGE_ACCESS_BARRIER_BIT |
				gl.FRAMEBUFFER_BARRIER_BIT | gl.TEXTURE_UPDATE_BARRIER_BIT |
				gl.PIXEL_BUFFER_BARRIER_BIT
		}
	}
	if bits != 0 {
		e.commands = append(e.commands, &MemoryBarrierCommand{barriers: bits})
	}
}

// ClearBuffer clears a buffer region to zero.
func (e *CommandEncoder) ClearBuffer(buffer hal.Buffer, offset, size uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok {
		return
	}
	e.commands = append(e.commands, &ClearBufferCommand{
		buffer: buf,
		offset: offset,
		size:   size,
	})
}

// CopyBufferToBuffer copies data between buffers.
func (e *CommandEncoder) CopyBufferToBuffer(src, dst hal.Buffer, regions []hal.BufferCopy) {
	srcBuf, srcOk := src.(*Buffer)
	dstBuf, dstOk := dst.(*Buffer)
	if !srcOk || !dstOk {
		return
	}

	for _, r := range regions {
		e.commands = append(e.commands, &CopyBufferCommand{
			srcID:     srcBuf.id,
			srcOffset: r.SrcOffset,
			dstID:     dstBuf.id,
			dstOffset: r.DstOffset,
			size:      r.Size,
		})
	}
}

// CopyBufferToTexture copies buffer data to a texture via PBO (pixel unpack buffer).
// Binds the source buffer as GL_PIXEL_UNPACK_BUFFER, then calls glTexSubImage2D
// with offset 0 so that GL reads pixel data from the bound PBO.
// Matches Rust wgpu-hal/src/gles/queue.rs CopyBufferToTexture command.
func (e *CommandEncoder) CopyBufferToTexture(src hal.Buffer, dst hal.Texture, regions []hal.BufferTextureCopy) {
	srcBuf, srcOK := src.(*Buffer)
	dstTex, dstOK := dst.(*Texture)
	if !srcOK || !dstOK || srcBuf == nil || dstTex == nil {
		return
	}

	for _, region := range regions {
		e.commands = append(e.commands, &CopyBufferToTextureCommand{
			srcBuffer:    srcBuf,
			dstTex:       dstTex,
			origin:       [3]uint32{region.TextureBase.Origin.X, region.TextureBase.Origin.Y, region.TextureBase.Origin.Z},
			copySize:     [3]uint32{region.Size.Width, region.Size.Height, region.Size.DepthOrArrayLayers},
			mipLevel:     region.TextureBase.MipLevel,
			bufOffset:    region.BufferLayout.Offset,
			bytesPerRow:  region.BufferLayout.BytesPerRow,
			rowsPerImage: region.BufferLayout.RowsPerImage,
		})
	}
}

// CopyTextureToBuffer copies texture data to a buffer via FBO + glReadPixels.
// For each region, it binds the source texture's FBO and reads pixels into the
// destination buffer's CPU-side data slice.
func (e *CommandEncoder) CopyTextureToBuffer(src hal.Texture, dst hal.Buffer, regions []hal.BufferTextureCopy) {
	srcTex, srcOK := src.(*Texture)
	dstBuf, dstOK := dst.(*Buffer)
	if !srcOK || !dstOK {
		return
	}

	for _, region := range regions {
		e.commands = append(e.commands, &CopyTextureToBufferCommand{
			glCtx:        e.glCtx,
			srcTexture:   srcTex,
			dstBuffer:    dstBuf,
			srcOrigin:    [3]uint32{region.TextureBase.Origin.X, region.TextureBase.Origin.Y, region.TextureBase.Origin.Z},
			copySize:     [3]uint32{region.Size.Width, region.Size.Height, region.Size.DepthOrArrayLayers},
			mipLevel:     region.TextureBase.MipLevel,
			aspect:       region.TextureBase.Aspect,
			dstOffset:    region.BufferLayout.Offset,
			bytesPerRow:  region.BufferLayout.BytesPerRow,
			rowsPerImage: region.BufferLayout.RowsPerImage,
		})
	}
}

// CopyTextureToTexture copies between textures using FBO read + glCopyTexSubImage2D.
// Attaches the source texture to the read framebuffer, then copies pixels into the
// destination texture. This works on GL 3.0+ / ES 3.0+ without glCopyImageSubData.
// Matches Rust wgpu-hal/src/gles/queue.rs CopyTextureToTexture command.
func (e *CommandEncoder) CopyTextureToTexture(src, dst hal.Texture, regions []hal.TextureCopy) {
	srcTex, srcOK := src.(*Texture)
	dstTex, dstOK := dst.(*Texture)
	if !srcOK || !dstOK || srcTex == nil || dstTex == nil {
		return
	}

	for _, region := range regions {
		e.commands = append(e.commands, &CopyTextureToTextureCommand{
			srcTex:    srcTex,
			dstTex:    dstTex,
			srcOrigin: [3]uint32{region.SrcBase.Origin.X, region.SrcBase.Origin.Y, region.SrcBase.Origin.Z},
			dstOrigin: [3]uint32{region.DstBase.Origin.X, region.DstBase.Origin.Y, region.DstBase.Origin.Z},
			copySize:  [3]uint32{region.Size.Width, region.Size.Height, region.Size.DepthOrArrayLayers},
			srcMip:    region.SrcBase.MipLevel,
			dstMip:    region.DstBase.MipLevel,
		})
	}
}

// ResolveQuerySet copies query results from a query set into a destination buffer.
// Each result is a uint64 (8 bytes) written starting at destinationOffset.
// Uses glGetQueryObjectui64v to read results, then glBufferSubData to write them.
// Matches Rust wgpu-hal/src/gles/queue.rs CopyQueryResults command (fallback path).
func (e *CommandEncoder) ResolveQuerySet(querySet hal.QuerySet, firstQuery, queryCount uint32, destination hal.Buffer, destinationOffset uint64) {
	qs, qsOK := querySet.(*QuerySet)
	dstBuf, bufOK := destination.(*Buffer)
	if !qsOK || !bufOK || qs == nil || dstBuf == nil {
		return
	}

	e.commands = append(e.commands, &ResolveQuerySetCommand{
		querySet:   qs,
		firstQuery: firstQuery,
		queryCount: queryCount,
		dstBuffer:  dstBuf,
		dstOffset:  destinationOffset,
	})
}

// BuildAccelerationStructures is a no-op (GLES has no ray tracing).
func (e *CommandEncoder) BuildAccelerationStructures(_ []hal.BuildAccelerationStructureDescriptor) {}

// PlaceAccelerationStructureBarrier is a no-op (GLES has no ray tracing).
func (e *CommandEncoder) PlaceAccelerationStructureBarrier(_ hal.AccelerationStructureBarrier) {}

// CopyAccelerationStructure is a no-op (GLES has no ray tracing).
func (e *CommandEncoder) CopyAccelerationStructure(_, _ hal.AccelerationStructure, _ gputypes.AccelerationStructureCopyMode) {
}

// ReadAccelerationStructureCompactSize is a no-op (GLES has no ray tracing).
func (e *CommandEncoder) ReadAccelerationStructureCompactSize(_ hal.AccelerationStructure, _ hal.Buffer, _ uint64) {
}

// BeginRenderPass begins a render pass.
func (e *CommandEncoder) BeginRenderPass(desc *hal.RenderPassDescriptor) hal.RenderPassEncoder {
	rpe := &RenderPassEncoder{
		encoder: e,
		desc:    desc,
	}

	// Bind the persistent VAO. Core Profile requires a VAO to be bound for any
	// vertex attribute or draw call. Re-binding at pass start ensures it is
	// active even if external code (or a previous pass) unbound it.
	if e.vao != 0 {
		e.commands = append(e.commands, &BindVAOCommand{vao: e.vao})
	}

	// Bind the correct framebuffer and set viewport.
	// Reference wgpu sets viewport at render pass start — required for correct rendering.
	if len(desc.ColorAttachments) > 0 {
		e.setupColorAttachment(desc, rpe)
	} else if desc.DepthStencilAttachment != nil {
		e.setupDepthOnlyTarget(desc.DepthStencilAttachment, rpe)
	}

	// Set draw buffers for MRT. Matches Rust wgpu-hal GLES
	// SetDrawColorBuffers (command.rs:643-646, queue.rs:1202-1207).
	if len(desc.ColorAttachments) > 0 {
		e.commands = append(e.commands, &SetDrawColorBuffersCommand{
			count: len(desc.ColorAttachments),
		})
	}

	// Record per-buffer clear commands. Uses glClearBufferfv for per-target
	// clearing instead of global glClearColor+glClear(GL_COLOR_BUFFER_BIT).
	// Matches Rust wgpu-hal GLES ClearColorF (command.rs:648-676, queue.rs:1222).
	for i, ca := range desc.ColorAttachments {
		if ca.LoadOp == gputypes.LoadOpClear {
			e.commands = append(e.commands, newClearColorBufferCommand(int32(i), ca))
		}
	}

	if desc.DepthStencilAttachment != nil {
		dsa := desc.DepthStencilAttachment
		if dsa.DepthLoadOp == gputypes.LoadOpClear {
			e.commands = append(e.commands, &ClearDepthCommand{
				depth: float64(dsa.DepthClearValue),
			})
		}
		if dsa.StencilLoadOp == gputypes.LoadOpClear {
			e.commands = append(e.commands, &ClearStencilCommand{
				stencil: int32(dsa.StencilClearValue),
			})
		}
	}

	// Emit beginning-of-pass timestamp if requested.
	if desc.TimestampWrites != nil {
		e.emitTimestamp(desc.TimestampWrites.QuerySet, desc.TimestampWrites.BeginningOfPassWriteIndex)
		rpe.endTimestampQuerySet = desc.TimestampWrites.QuerySet
		rpe.endTimestampIndex = desc.TimestampWrites.EndOfPassWriteIndex
	}

	return rpe
}

// setupColorAttachment configures framebuffer, viewport, and MSAA resolve for
// all color attachments of a render pass. For surface targets only attachment[0]
// is used (surfaces are always single-target). For offscreen targets, all
// attachments are bound to GL_COLOR_ATTACHMENT0..N.
// Matches Rust wgpu-hal GLES begin_render_pass (command.rs:552-627).
func (e *CommandEncoder) setupColorAttachment(desc *hal.RenderPassDescriptor, rpe *RenderPassEncoder) {
	ca := desc.ColorAttachments[0]
	tv, ok := ca.View.(*TextureView)
	if !ok {
		return
	}

	if tv.isSurface {
		e.setupSurfaceTarget(desc, tv, rpe)
		return
	}

	if tv.texture == nil {
		return
	}

	e.setupOffscreenTarget(desc, tv, rpe)
}

// setupSurfaceTarget binds the Surface's swapchain offscreen framebuffer and
// sets viewport to surface dimensions. The swapchain FBO is a persistent GL
// framebuffer owned by the Surface (allocated in Surface.Configure). User
// render passes target this FBO — never the default framebuffer (FBO 0) —
// because the scene is intentionally rendered upside-down via naga's in-shader
// Y-flip (WriterFlagAdjustCoordinateSpace). Queue.Present performs an explicit
// Y-flipping glBlitFramebuffer from this FBO to FBO 0 before SwapBuffers.
// Mirrors Rust wgpu-hal/src/gles/egl.rs Surface::configure/Surface::present.
func (e *CommandEncoder) setupSurfaceTarget(desc *hal.RenderPassDescriptor, tv *TextureView, rpe *RenderPassEncoder) {
	if tv.surfaceTex != nil && tv.surfaceTex.surface != nil {
		surf := tv.surfaceTex.surface
		e.commands = append(e.commands, &BindSurfaceFramebufferCommand{
			surface: surf, maxColorAttachments: e.maxColorAttachments,
		})
		if surf.config != nil {
			cfg := surf.config
			rpe.fbHeight = cfg.Height
			e.commands = append(e.commands, &SetViewportCommand{
				width:    float32(cfg.Width),
				height:   float32(cfg.Height),
				maxDepth: 1,
			})
		}
		// Attach depth/stencil to the swapchain FBO if requested.
		// The swapchain FBO is a real GL FBO (not FBO 0), so glFramebufferTexture2D
		// works. Matches Rust wgpu-hal GLES: BindAttachment for depth/stencil after
		// ResetFramebuffer { is_default: false } (command.rs:575-601).
		if desc.DepthStencilAttachment != nil {
			if dsView, ok := desc.DepthStencilAttachment.View.(*TextureView); ok && dsView.texture != nil {
				e.commands = append(e.commands, &AttachDepthStencilToFBOCommand{
					depthTexture: dsView.texture,
					mipLevel:     dsView.baseMip,
					layer:        dsView.baseLayer,
				})
			}
		}
		return
	}
	// Fallback: no surface texture (shouldn't normally happen for isSurface=true
	// views, but preserves prior behavior for tests / degenerate cases).
	e.commands = append(e.commands, &BindFramebufferCommand{fbo: 0})
}

// setupOffscreenTarget configures an offscreen FBO with all color attachments,
// depth/stencil attachment, and MSAA resolve.
// Matches Rust wgpu-hal GLES begin_render_pass (command.rs:558-595):
// iterates all color_attachments and calls BindAttachment for each.
func (e *CommandEncoder) setupOffscreenTarget(
	desc *hal.RenderPassDescriptor,
	tv *TextureView,
	rpe *RenderPassEncoder,
) {
	e.commands = append(e.commands, &EnsureOffscreenFBOCommand{
		texture:             tv.texture,
		mipLevel:            tv.baseMip,
		layer:               colorAttachmentLayer(tv, desc.ColorAttachments[0].DepthSlice),
		maxColorAttachments: e.maxColorAttachments,
	})

	// Attach additional color textures (attachments 1..N) to the FBO.
	// Attachment 0 is already bound by EnsureOffscreenFBOCommand.
	// Matches Rust BindAttachment for each color_attachments[i] (command.rs:558-595).
	for i := 1; i < len(desc.ColorAttachments); i++ {
		ca := desc.ColorAttachments[i]
		atv, ok := ca.View.(*TextureView)
		if !ok || atv.texture == nil {
			continue
		}
		e.commands = append(e.commands, &AttachColorCommand{
			attachmentIndex: uint32(i),
			texture:         atv.texture,
			mipLevel:        atv.baseMip,
			layer:           colorAttachmentLayer(atv, ca.DepthSlice),
		})
	}

	// Attach depth/stencil texture to the FBO if provided.
	if desc.DepthStencilAttachment != nil {
		if dsView, ok := desc.DepthStencilAttachment.View.(*TextureView); ok && dsView.texture != nil {
			e.commands = append(e.commands, &AttachDepthStencilCommand{
				colorTexture: tv.texture,
				depthTexture: dsView.texture,
				mipLevel:     dsView.baseMip,
				layer:        dsView.baseLayer,
			})
		}
	}

	// The render area is the view's mip level, not the texture's level 0.
	width := max(tv.texture.size.Width>>tv.baseMip, 1)
	height := max(tv.texture.size.Height>>tv.baseMip, 1)
	rpe.fbHeight = height
	e.commands = append(e.commands, &SetViewportCommand{
		width:    float32(width),
		height:   float32(height),
		maxDepth: 1,
	})

	// Record MSAA resolve target if present (attachment 0 only).
	ca := desc.ColorAttachments[0]
	if resolveView, ok := ca.ResolveTarget.(*TextureView); ok && ca.ResolveTarget != nil {
		if resolveView.texture != nil {
			rpe.msaaTexture = tv.texture
			rpe.resolveTexture = resolveView.texture
		} else if resolveView.isSurface {
			rpe.msaaTexture = tv.texture
			rpe.resolveToSurface = true
		}
	}
}

// setupDepthOnlyTarget binds the framebuffer of a pass without color
// attachments, a shadow-map pass: the depth/stencil texture's own FBO with the
// view's mip level and layer attached, and the viewport of that mip level.
// Rust wgpu-hal begin_render_pass binds its draw FBO and attaches the
// depth/stencil view whether or not the pass has color attachments
// (gles/command.rs), and sets the viewport to the pass's extent.
func (e *CommandEncoder) setupDepthOnlyTarget(dsa *hal.RenderPassDepthStencilAttachment, rpe *RenderPassEncoder) {
	view, ok := dsa.View.(*TextureView)
	if !ok || view.texture == nil {
		return
	}
	e.commands = append(e.commands, &BindDepthStencilFBOCommand{
		texture:  view.texture,
		mipLevel: view.baseMip,
		layer:    view.baseLayer,
	})
	width := max(view.texture.size.Width>>view.baseMip, 1)
	height := max(view.texture.size.Height>>view.baseMip, 1)
	rpe.fbHeight = height
	e.commands = append(e.commands, &SetViewportCommand{
		width:    float32(width),
		height:   float32(height),
		maxDepth: 1,
	})
}

// BeginComputePass begins a compute pass.
func (e *CommandEncoder) BeginComputePass(desc *hal.ComputePassDescriptor) hal.ComputePassEncoder {
	cpe := &ComputePassEncoder{
		encoder: e,
	}

	// Emit beginning-of-pass timestamp if requested.
	if desc != nil && desc.TimestampWrites != nil {
		e.emitTimestamp(desc.TimestampWrites.QuerySet, desc.TimestampWrites.BeginningOfPassWriteIndex)
		cpe.endTimestampQuerySet = desc.TimestampWrites.QuerySet
		cpe.endTimestampIndex = desc.TimestampWrites.EndOfPassWriteIndex
	}

	return cpe
}

// emitTimestamp records a glQueryCounter command for a timestamp write index.
// Used for render/compute pass beginning/end timestamp queries.
func (e *CommandEncoder) emitTimestamp(querySet hal.QuerySet, index *uint32) {
	if index == nil {
		return
	}
	qs, ok := querySet.(*QuerySet)
	if !ok || qs == nil {
		return
	}
	idx := *index
	if int(idx) >= len(qs.queries) {
		return
	}
	e.commands = append(e.commands, &TimestampQueryCommand{query: qs.queries[idx]})
}

// RenderPassEncoder implements hal.RenderPassEncoder for OpenGL.
type RenderPassEncoder struct {
	encoder     *CommandEncoder
	desc        *hal.RenderPassDescriptor
	pipeline    *RenderPipeline
	indexBuffer *Buffer
	indexFormat gputypes.IndexFormat
	stencilRef  uint32
	fbHeight    uint32 // Framebuffer height for MSAA resolve blit Y-flip

	// vertexBuffers holds the buffer bound to each vertex buffer slot. GL
	// sets the buffer and its layout in one glVertexAttribPointer call, but
	// the layout belongs to the pipeline and WebGPU allows SetVertexBuffer
	// before SetPipeline. So SetVertexBuffer and SetPipeline only mark slots
	// dirty, and prepareVertexBuffers configures the dirty slots before a
	// draw (Rust wgpu-hal src/gles/command.rs set_vertex_buffer,
	// set_render_pipeline, rebind_vertex_data).
	vertexBuffers      []vertexBufferBinding
	dirtyVertexBuffers uint32 // bit i: slot i needs its attributes set up

	// activeFirstInstance is the emulated first instance the instance-stepped
	// buffers are set up for (0 with fully featured instancing).
	activeFirstInstance uint32
	// indexOffset is the SetIndexBuffer offset, applied to every indexed draw.
	indexOffset uint64

	// MSAA resolve state: set during BeginRenderPass when ResolveTarget is present.
	msaaTexture      *Texture // The MSAA color texture (source for resolve)
	resolveTexture   *Texture // The single-sample resolve target (nil when resolveToSurface)
	resolveToSurface bool     // True when resolve target is the default framebuffer (FBO 0)

	// End-of-pass timestamp write (deferred to End()).
	endTimestampQuerySet hal.QuerySet
	endTimestampIndex    *uint32
}

// resolveTargetSurface extracts the *Surface owning the resolve target (if the
// resolve target is a surface texture view). Returns nil otherwise.
func (e *RenderPassEncoder) resolveTargetSurface() *Surface {
	if len(e.desc.ColorAttachments) == 0 {
		return nil
	}
	rv, ok := e.desc.ColorAttachments[0].ResolveTarget.(*TextureView)
	if !ok || rv == nil || rv.surfaceTex == nil {
		return nil
	}
	return rv.surfaceTex.surface
}

// emitMSAAResolve appends the appropriate MSAAResolveCommand when the pass has
// an MSAA resolve target. Must only be called when e.msaaTexture != nil.
func (e *RenderPassEncoder) emitMSAAResolve() {
	w := int32(e.msaaTexture.size.Width)
	h := int32(e.msaaTexture.size.Height)
	if e.resolveToSurface {
		// Resolve into the Surface's swapchain offscreen FBO (not FBO 0).
		// The Y-flip happens once at Present time (Queue.Present), not here.
		e.encoder.commands = append(e.encoder.commands, &MSAAResolveCommand{
			msaaTexture:      e.msaaTexture,
			resolveToSurface: true,
			surface:          e.resolveTargetSurface(),
			width:            w,
			height:           h,
		})
		return
	}
	if e.resolveTexture != nil {
		// Resolve into an offscreen texture FBO.
		e.encoder.commands = append(e.encoder.commands, &MSAAResolveCommand{
			msaaTexture:    e.msaaTexture,
			resolveTexture: e.resolveTexture,
			width:          w,
			height:         h,
		})
	}
}

// End finishes the render pass.
// If MSAA resolve is needed, blits the MSAA FBO to the resolve target FBO.
// If the pass was rendering to an offscreen FBO, rebinds the default framebuffer
// so subsequent operations do not accidentally target the offscreen texture.
func (e *RenderPassEncoder) End() {
	e.disableVertexAttribs()

	if e.msaaTexture != nil {
		e.emitMSAAResolve()
	}

	// Emit end-of-pass timestamp if requested.
	if e.endTimestampIndex != nil {
		e.encoder.emitTimestamp(e.endTimestampQuerySet, e.endTimestampIndex)
	}

	// Check if we were rendering to an offscreen target.
	if len(e.desc.ColorAttachments) > 0 {
		if tv, ok := e.desc.ColorAttachments[0].View.(*TextureView); ok {
			if !tv.isSurface && tv.texture != nil {
				e.encoder.commands = append(e.encoder.commands, &BindFramebufferCommand{fbo: 0})
			}
		}
	}
}

// SetPipeline sets the render pipeline.
func (e *RenderPassEncoder) SetPipeline(pipeline hal.RenderPipeline) {
	p, ok := pipeline.(*RenderPipeline)
	if !ok {
		return
	}
	// The new pipeline may read the bound buffers with another layout, and
	// attributes the old one enabled must not stay enabled.
	e.disableVertexAttribs()
	e.pipeline = p
	for slot := range p.vertexBuffers {
		if slot < maxVertexBufferSlots {
			e.dirtyVertexBuffers |= 1 << slot
		}
	}
	e.encoder.commands = append(e.encoder.commands,
		&UseProgramCommand{programID: p.programID},
		&SetPipelineStateCommand{
			topology:     p.primitiveTopology,
			cullMode:     p.cullMode,
			frontFace:    p.frontFace,
			depthStencil: p.depthStencil,
			colorTargets: p.colorTargets,
			stencilRef:   e.stencilRef,
		},
	)
	e.encoder.setPipelineSamplers(&p.samplerBindMap)
}

// SetBindGroup sets a bind group.
func (e *RenderPassEncoder) SetBindGroup(index uint32, group hal.BindGroup, offsets []uint32) {
	bg, ok := group.(*BindGroup)
	if !ok {
		return
	}
	var groupInfos []BindGroupLayoutInfo
	if e.pipeline != nil && e.pipeline.layout != nil {
		groupInfos = e.pipeline.layout.groupInfos
	}
	e.encoder.setBindGroup(index, bg, offsets, groupInfos)
}

// setBindGroup records a bind group for a render or compute pass: its
// buffers, textures and images, then the samplers of the texture units it
// changed (Rust wgpu-hal gles set_bind_group).
func (e *CommandEncoder) setBindGroup(index uint32, bg *BindGroup, offsets []uint32, groupInfos []BindGroupLayoutInfo) {
	e.commands = append(e.commands, &SetBindGroupCommand{
		index:                   index,
		group:                   bg,
		dynamicOffsets:          offsets,
		maxTextureUnits:         e.maxTextureUnits,
		depthStencilTextureMode: e.depthStencilTextureMode,
		groupInfos:              groupInfos,
	})
	dirtyTextures, dirtySamplers := e.samplers.bindGroupSamplers(index, bg, groupInfos)
	e.commands = append(e.commands, e.samplers.rebind(dirtyTextures, dirtySamplers)...)
}

// setPipelineSamplers installs a pipeline's sampler map and rebinds the
// samplers of the texture units whose pairing changed (Rust wgpu-hal gles
// set_pipeline_inner): a bind group kept across the pipeline change must
// reach the new shader's combined samplers.
func (e *CommandEncoder) setPipelineSamplers(m *samplerBindMap) {
	if dirty := e.samplers.setPipelineMap(m); dirty != 0 {
		e.commands = append(e.commands, e.samplers.rebind(dirty, 0)...)
	}
}

// maxVertexBufferSlots bounds the vertex buffer slots a render pass tracks,
// the width of dirtyVertexBuffers. GL_MAX_VERTEX_ATTRIB_BINDINGS, which the
// adapter reports as maxVertexBuffers, is 16 on the drivers we know.
const maxVertexBufferSlots = 32

// vertexBufferBinding is the buffer and offset bound to a vertex buffer slot.
type vertexBufferBinding struct {
	buffer *Buffer
	offset uint64
}

// SetVertexBuffer binds a vertex buffer to a slot. Its attributes are set up
// before the next draw, with the layout of the pipeline current then.
func (e *RenderPassEncoder) SetVertexBuffer(slot uint32, buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok || slot >= maxVertexBufferSlots {
		return
	}
	for len(e.vertexBuffers) <= int(slot) {
		e.vertexBuffers = append(e.vertexBuffers, vertexBufferBinding{})
	}
	e.vertexBuffers[slot] = vertexBufferBinding{buffer: buf, offset: offset}
	e.dirtyVertexBuffers |= 1 << slot
}

// prepareVertexBuffers sets up the attributes of every dirty slot that has
// both a buffer and a layout in the current pipeline. A slot missing either
// stays dirty, as in Rust, and is set up once both are there.
func (e *RenderPassEncoder) prepareVertexBuffers() {
	if e.pipeline == nil {
		return
	}
	for slot := range e.vertexBuffers {
		vb := e.vertexBuffers[slot]
		if e.dirtyVertexBuffers&(1<<slot) == 0 || vb.buffer == nil || slot >= len(e.pipeline.vertexBuffers) {
			continue
		}
		layout := &e.pipeline.vertexBuffers[slot]
		offset := vb.offset
		if layout.StepMode == gputypes.VertexStepModeInstance {
			// The emulated first instance (Rust rebind_vertex_data).
			offset += layout.ArrayStride * uint64(e.activeFirstInstance)
		}
		e.encoder.commands = append(e.encoder.commands, &SetVertexBufferCommand{
			slot:   uint32(slot),
			buffer: vb.buffer,
			offset: offset,
			layout: layout,
		})
		e.dirtyVertexBuffers &^= 1 << slot
	}
}

// prepareDraw sets up the vertex buffers for a draw with firstInstance.
// With fully featured instancing GL applies the first instance itself.
// Without it, instance-stepped buffers start at the first instance's element
// and the naga_vs_first_instance uniform adds it to instance_index, so the
// buffers are set up again when it changes (Rust wgpu-hal src/gles/command.rs
// prepare_draw).
func (e *RenderPassEncoder) prepareDraw(firstInstance uint32) {
	emulated := firstInstance
	if e.encoder.fullInstancing {
		emulated = 0
	}
	if emulated != e.activeFirstInstance && e.pipeline != nil {
		for slot := range e.pipeline.vertexBuffers {
			if slot < maxVertexBufferSlots && e.pipeline.vertexBuffers[slot].StepMode == gputypes.VertexStepModeInstance {
				e.dirtyVertexBuffers |= 1 << slot
			}
		}
	}
	e.activeFirstInstance = emulated
	e.prepareVertexBuffers()
}

// firstInstanceLocation is the current pipeline's naga_vs_first_instance
// uniform, or -1.
func (e *RenderPassEncoder) firstInstanceLocation() int32 {
	if e.pipeline == nil {
		return -1
	}
	return e.pipeline.firstInstanceLocation
}

// disableVertexAttribs disables the attribute arrays the current pipeline's
// layout enables, when the pipeline changes or the pass ends, so none stays
// enabled for a later pipeline that does not use it (Rust wgpu-hal
// UnsetVertexAttribute).
func (e *RenderPassEncoder) disableVertexAttribs() {
	if e.pipeline == nil {
		return
	}
	var locations []uint32
	for i := range e.pipeline.vertexBuffers {
		for _, attr := range e.pipeline.vertexBuffers[i].Attributes {
			locations = append(locations, attr.ShaderLocation)
		}
	}
	if len(locations) > 0 {
		e.encoder.commands = append(e.encoder.commands, &DisableVertexAttribsCommand{locations: locations})
	}
}

// SetIndexBuffer sets the index buffer.
func (e *RenderPassEncoder) SetIndexBuffer(buffer hal.Buffer, format gputypes.IndexFormat, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok {
		return
	}
	e.indexBuffer = buf
	e.indexOffset = offset
	e.indexFormat = format

	e.encoder.commands = append(e.encoder.commands, &SetIndexBufferCommand{
		buffer: buf,
		format: format,
		offset: offset,
	})
}

// SetViewport sets the viewport.
func (e *RenderPassEncoder) SetViewport(vp gputypes.Viewport) {
	e.encoder.commands = append(e.encoder.commands, &SetViewportCommand{
		x: vp.X, y: vp.Y, width: vp.Width, height: vp.Height,
		minDepth: vp.MinDepth, maxDepth: vp.MaxDepth,
	})
}

// SetScissorRect sets the scissor rectangle.
// With ADJUST_COORDINATE_SPACE, no Y-flip is needed — coordinates pass through directly.
func (e *RenderPassEncoder) SetScissorRect(rect gputypes.ScissorRect) {
	e.encoder.commands = append(e.encoder.commands, &SetScissorCommand{
		x: rect.X, y: rect.Y, width: rect.Width, height: rect.Height,
	})
}

// SetBlendConstant sets the blend constant.
func (e *RenderPassEncoder) SetBlendConstant(color *gputypes.Color) {
	e.encoder.commands = append(e.encoder.commands, &SetBlendConstantCommand{
		r: float32(color.R),
		g: float32(color.G),
		b: float32(color.B),
		a: float32(color.A),
	})
}

// SetStencilReference sets the stencil reference value.
func (e *RenderPassEncoder) SetStencilReference(ref uint32) {
	e.stencilRef = ref
	var ds *hal.DepthStencilState
	if e.pipeline != nil {
		ds = e.pipeline.depthStencil
	}
	e.encoder.commands = append(e.encoder.commands, &SetStencilRefCommand{
		ref:          ref,
		depthStencil: ds,
	})
}

// topology returns the current pipeline's primitive topology.
func (e *RenderPassEncoder) topology() gputypes.PrimitiveTopology {
	if e.pipeline == nil {
		return gputypes.PrimitiveTopologyTriangleList
	}
	return e.pipeline.primitiveTopology
}

// Draw draws primitives.
func (e *RenderPassEncoder) Draw(args gputypes.DrawArgs) {
	e.prepareDraw(args.FirstInstance)
	e.encoder.commands = append(e.encoder.commands, &DrawCommand{
		vertexCount:           args.VertexCount,
		instanceCount:         args.InstanceCount,
		firstVertex:           args.FirstVertex,
		firstInstance:         args.FirstInstance,
		topology:              e.topology(),
		fullInstancing:        e.encoder.fullInstancing,
		firstInstanceLocation: e.firstInstanceLocation(),
	})
}

// DrawIndexed draws indexed primitives.
func (e *RenderPassEncoder) DrawIndexed(args gputypes.DrawIndexedArgs) {
	e.prepareDraw(args.FirstInstance)
	e.encoder.commands = append(e.encoder.commands, &DrawIndexedCommand{
		indexCount:            args.IndexCount,
		instanceCount:         args.InstanceCount,
		firstIndex:            args.FirstIndex,
		indexOffset:           e.indexOffset,
		baseVertex:            args.BaseVertex,
		firstInstance:         args.FirstInstance,
		indexFormat:           e.indexFormat,
		topology:              e.topology(),
		fullInstancing:        e.encoder.fullInstancing,
		firstInstanceLocation: e.firstInstanceLocation(),
	})
}

// Indirect record sizes: DrawIndirectArgs and DrawIndexedIndirectArgs.
const (
	drawIndirectRecordSize        = 16
	drawIndexedIndirectRecordSize = 20
)

// DrawIndirect draws drawCount records from offset in buffer (Rust wgpu-hal
// gles command.rs draw_indirect, queue.rs DrawIndirect).
func (e *RenderPassEncoder) DrawIndirect(buffer hal.Buffer, offset uint64, drawCount uint32) {
	buf, ok := buffer.(*Buffer)
	if !ok || drawCount == 0 || !indirect.RangeFits(buf.size, offset, drawIndirectRecordSize, drawCount) {
		return
	}
	e.prepareDraw(0)
	e.encoder.commands = append(e.encoder.commands, &DrawIndirectCommand{
		buffer:                buf.id,
		offset:                offset,
		count:                 drawCount,
		topology:              e.topology(),
		firstInstanceLocation: e.firstInstanceLocation(),
	})
}

// DrawIndexedIndirect draws drawCount indexed records from offset in buffer
// (Rust wgpu-hal gles command.rs draw_indexed_indirect, queue.rs
// DrawIndexedIndirect). A nonzero SetIndexBuffer offset is applied as
// DrawIndexedIndirectCommand describes.
func (e *RenderPassEncoder) DrawIndexedIndirect(buffer hal.Buffer, offset uint64, drawCount uint32) {
	buf, ok := buffer.(*Buffer)
	if !ok || drawCount == 0 || !indirect.RangeFits(buf.size, offset, drawIndexedIndirectRecordSize, drawCount) {
		return
	}
	e.prepareDraw(0)
	cmd := &DrawIndexedIndirectCommand{
		buffer:                buf.id,
		offset:                offset,
		count:                 drawCount,
		indexFormat:           e.indexFormat,
		topology:              e.topology(),
		firstInstanceLocation: e.firstInstanceLocation(),
	}
	if e.indexBuffer != nil && e.indexOffset != 0 {
		cmd.indexBuffer = e.indexBuffer.id
		cmd.indexOffset = e.indexOffset
		cmd.indexBufferSize = e.indexBuffer.size
	}
	e.encoder.commands = append(e.encoder.commands, cmd)
}

// DrawIndirectCount is not supported by the GLES backend.
func (e *RenderPassEncoder) DrawIndirectCount(_ hal.Buffer, _ uint64, _ hal.Buffer, _ uint64, _ uint32) {
}

// DrawIndexedIndirectCount is not supported by the GLES backend.
func (e *RenderPassEncoder) DrawIndexedIndirectCount(_ hal.Buffer, _ uint64, _ hal.Buffer, _ uint64, _ uint32) {
}

// ExecuteBundle executes a pre-recorded render bundle.
// Note: Render bundles are not natively supported in OpenGL.
// OpenGL uses display lists (deprecated) or VAO/VBO state caching.
// This is a no-op - bundles are expanded inline in the command stream.
func (e *RenderPassEncoder) ExecuteBundle(bundle hal.RenderBundle) {
	_ = bundle
}

// ComputePassEncoder implements hal.ComputePassEncoder for OpenGL.
type ComputePassEncoder struct {
	encoder  *CommandEncoder
	pipeline *ComputePipeline

	// End-of-pass timestamp write (deferred to End()).
	endTimestampQuerySet hal.QuerySet
	endTimestampIndex    *uint32
}

// End finishes the compute pass.
func (e *ComputePassEncoder) End() {
	// Emit end-of-pass timestamp if requested.
	if e.endTimestampIndex != nil {
		e.encoder.emitTimestamp(e.endTimestampQuerySet, e.endTimestampIndex)
	}
}

// SetPipeline sets the compute pipeline.
func (e *ComputePassEncoder) SetPipeline(pipeline hal.ComputePipeline) {
	p, ok := pipeline.(*ComputePipeline)
	if !ok {
		return
	}
	e.pipeline = p
	e.encoder.commands = append(e.encoder.commands, &UseProgramCommand{
		programID: p.programID,
	})
	e.encoder.setPipelineSamplers(&p.samplerBindMap)
}

// SetBindGroup sets a bind group.
func (e *ComputePassEncoder) SetBindGroup(index uint32, group hal.BindGroup, offsets []uint32) {
	bg, ok := group.(*BindGroup)
	if !ok {
		return
	}
	var groupInfos []BindGroupLayoutInfo
	if e.pipeline != nil && e.pipeline.layout != nil {
		groupInfos = e.pipeline.layout.groupInfos
	}
	e.encoder.setBindGroup(index, bg, offsets, groupInfos)
}

// Dispatch dispatches compute work.
func (e *ComputePassEncoder) Dispatch(x, y, z uint32) {
	e.encoder.commands = append(e.encoder.commands, &DispatchCommand{
		x: x, y: y, z: z,
	})
}

// DispatchIndirect dispatches compute work with GPU-generated parameters.
func (e *ComputePassEncoder) DispatchIndirect(buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok {
		return
	}
	e.encoder.commands = append(e.encoder.commands, &DispatchIndirectCommand{
		buffer: buf,
		offset: offset,
	})
}

// --- GL Command implementations ---

// MemoryBarrierCommand inserts a glMemoryBarrier call.
// Ensures compute shader writes are visible to subsequent draw/dispatch commands.
type MemoryBarrierCommand struct {
	barriers uint32
}

func (c *MemoryBarrierCommand) Execute(ctx *gl.Context) {
	ctx.MemoryBarrier(c.barriers)
}

// ClearBufferCommand clears a buffer region.
type ClearBufferCommand struct {
	buffer *Buffer
	offset uint64
	size   uint64
}

func (c *ClearBufferCommand) Execute(_ *gl.Context) {
	// Note: glClearBufferSubData requires GL 4.3+ / GLES 3.1+.
	// For older versions, map buffer and memset, or use compute shader.
}

// BindVAOCommand binds a vertex array object.
type BindVAOCommand struct {
	vao uint32
}

func (c *BindVAOCommand) Execute(ctx *gl.Context) {
	ctx.BindVertexArray(c.vao)
}

// BindFramebufferCommand binds a framebuffer object.
type BindFramebufferCommand struct {
	fbo uint32
}

func (c *BindFramebufferCommand) Execute(ctx *gl.Context) {
	ctx.BindFramebuffer(gl.FRAMEBUFFER, c.fbo)
}

// BindSurfaceFramebufferCommand binds the Surface's swapchain offscreen
// framebuffer. Reads surface.swapchainFBO at execute time so reconfigure
// (e.g. window resize) between encode and submit is handled correctly.
// Mirrors Rust wgpu-hal/src/gles/egl.rs Surface::configure swapchain FBO.
type BindSurfaceFramebufferCommand struct {
	surface             *Surface
	maxColorAttachments uint32
}

func (c *BindSurfaceFramebufferCommand) Execute(ctx *gl.Context) {
	if c.surface == nil {
		ctx.BindFramebuffer(gl.FRAMEBUFFER, 0)
		return
	}
	ctx.BindFramebuffer(gl.FRAMEBUFFER, c.surface.swapchainFBO)
	if c.surface.swapchainFBO != 0 {
		// COLOR_ATTACHMENT0 is the surface's own renderbuffer and stays.
		resetFramebufferAttachments(ctx, c.maxColorAttachments)
	}
}

// resetFramebufferAttachments detaches the depth/stencil attachment and color
// attachments 1 to maxColor-1 of the bound framebuffer, before a render pass
// attaches its own. A framebuffer here is reused by every pass on the same
// color texture or surface, so without it an attachment of an earlier pass
// stays in place: a pass without depth would keep the old depth texture, and
// GL limits rendering to the area every attachment covers, so a smaller stale
// attachment clips the draw (Mesa does; some Windows drivers do not).
// COLOR_ATTACHMENT0 is left to the caller, which attaches it next.
//
// Reference: Rust wgpu-hal gles/queue.rs C::ResetFramebuffer, which detaches
// DEPTH_STENCIL_ATTACHMENT and every color attachment up to
// max_color_attachments at the start of each pass. Detaching
// DEPTH_STENCIL_ATTACHMENT detaches both the depth and the stencil point.
func resetFramebufferAttachments(ctx *gl.Context, maxColor uint32) {
	ctx.FramebufferTexture2D(gl.FRAMEBUFFER, gl.DEPTH_STENCIL_ATTACHMENT, gl.TEXTURE_2D, 0, 0)
	for i := uint32(1); i < maxColor; i++ {
		ctx.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0+i, gl.TEXTURE_2D, 0, 0)
	}
}

// EnsureOffscreenFBOCommand lazily creates a framebuffer object for an offscreen
// texture, binds it and attaches the pass's mip level as COLOR_ATTACHMENT0.
type EnsureOffscreenFBOCommand struct {
	texture             *Texture
	mipLevel            uint32 // the view's base mip level
	layer               uint32 // the depth slice or array layer of a layered texture
	maxColorAttachments uint32 // Limits.MaxColorAttachments (resetFramebufferAttachments)
}

func (c *EnsureOffscreenFBOCommand) Execute(ctx *gl.Context) {
	if c.texture.fbo != 0 {
		ctx.BindFramebuffer(gl.FRAMEBUFFER, c.texture.fbo)
		// The FBO belongs to the texture and serves every view of it, so each
		// pass attaches its own mip level, as Rust wgpu-hal attaches each
		// pass's views to its draw FBO (gles/queue.rs set_attachment), after
		// dropping what an earlier pass attached.
		resetFramebufferAttachments(ctx, c.maxColorAttachments)
		c.attach(ctx)
		return
	}
	// Create FBO.
	fbo := ctx.GenFramebuffers(1)
	ctx.BindFramebuffer(gl.FRAMEBUFFER, fbo)
	c.attach(ctx)
	// Verify completeness.
	status := ctx.CheckFramebufferStatus(gl.FRAMEBUFFER)
	if status != gl.FRAMEBUFFER_COMPLETE {
		// FBO incomplete — delete and fall back to default framebuffer.
		ctx.DeleteFramebuffers(fbo)
		ctx.BindFramebuffer(gl.FRAMEBUFFER, 0)
		return
	}
	c.texture.fbo = fbo
}

// attach attaches the color texture as COLOR_ATTACHMENT0 of the bound FBO:
// its mip level and, for a 3D or 2D array texture, its layer.
func (c *EnsureOffscreenFBOCommand) attach(ctx *gl.Context) {
	attachColorLayer(ctx, gl.COLOR_ATTACHMENT0, c.texture, c.mipLevel, c.layer)
}

// AttachDepthStencilCommand attaches a depth/stencil texture to the currently
// bound FBO (the one associated with the color texture). This must be recorded
// after EnsureOffscreenFBOCommand so the FBO is already bound.
//
// The view's mip level and layer are attached (Rust wgpu-hal gles/queue.rs
// set_attachment). Attaching level 0 through glFramebufferTexture2D, which
// has no layer and rejects a 2D array target, left a depth view of another
// level or of an array layer unattached.
type AttachDepthStencilCommand struct {
	colorTexture *Texture // used to verify the FBO exists
	depthTexture *Texture
	mipLevel     uint32
	layer        uint32 // array layer or cube face of a layered texture
}

func (c *AttachDepthStencilCommand) Execute(ctx *gl.Context) {
	if c.colorTexture.fbo == 0 {
		return
	}
	attachment := depthStencilAttachmentPoint(c.depthTexture.format)
	attachColorLayer(ctx, attachment, c.depthTexture, c.mipLevel, c.layer)
}

// BindDepthStencilFBOCommand binds the framebuffer of a pass without color
// attachments: the depth/stencil texture's FBO, created lazily, with one mip
// level and layer of the texture at its depth/stencil attachment point. The
// FBO is shared with the texture's readbacks (CopyTextureToBufferCommand
// bindSourceFBO), so the level and layer are attached for every pass.
type BindDepthStencilFBOCommand struct {
	texture  *Texture
	mipLevel uint32
	layer    uint32 // array layer or cube face of a layered texture
}

func (c *BindDepthStencilFBOCommand) Execute(ctx *gl.Context) {
	attachment := depthStencilAttachmentPoint(c.texture.format)
	if c.texture.fbo != 0 {
		ctx.BindFramebuffer(gl.FRAMEBUFFER, c.texture.fbo)
		attachColorLayer(ctx, attachment, c.texture, c.mipLevel, c.layer)
		return
	}
	fbo := ctx.GenFramebuffers(1)
	ctx.BindFramebuffer(gl.FRAMEBUFFER, fbo)
	attachColorLayer(ctx, attachment, c.texture, c.mipLevel, c.layer)
	if status := ctx.CheckFramebufferStatus(gl.FRAMEBUFFER); status != gl.FRAMEBUFFER_COMPLETE {
		hal.Logger().Warn("gles: depth/stencil framebuffer incomplete", "status", status, "format", c.texture.format)
		ctx.DeleteFramebuffers(fbo)
		ctx.BindFramebuffer(gl.FRAMEBUFFER, 0)
		return
	}
	c.texture.fbo = fbo
}

// depthStencilAttachmentPoint returns the GL attachment point for a depth/stencil
// texture format. Matches Rust wgpu-hal GLES (command.rs:577-580).
func depthStencilAttachmentPoint(format gputypes.TextureFormat) uint32 {
	switch format {
	case gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureFormatDepth32FloatStencil8:
		return gl.DEPTH_STENCIL_ATTACHMENT
	case gputypes.TextureFormatStencil8:
		return gl.STENCIL_ATTACHMENT
	default:
		return gl.DEPTH_ATTACHMENT
	}
}

// AttachDepthStencilToFBOCommand attaches a depth/stencil texture to the
// currently bound FBO. Unlike AttachDepthStencilCommand, this does not
// reference a color texture — it operates on whatever FBO is currently bound
// (typically the surface swapchain FBO).
//
// The attachment point is chosen by texture format (Rust wgpu-hal command.rs:577-580):
//   - depth-only → GL_DEPTH_ATTACHMENT
//   - stencil-only → GL_STENCIL_ATTACHMENT
//   - depth+stencil → GL_DEPTH_STENCIL_ATTACHMENT
//
// Like AttachDepthStencilCommand it attaches the view's mip level and layer.
type AttachDepthStencilToFBOCommand struct {
	depthTexture *Texture
	mipLevel     uint32
	layer        uint32
}

func (c *AttachDepthStencilToFBOCommand) Execute(ctx *gl.Context) {
	attachment := depthStencilAttachmentPoint(c.depthTexture.format)
	attachColorLayer(ctx, attachment, c.depthTexture, c.mipLevel, c.layer)
}

// MSAAResolveCommand resolves an MSAA framebuffer to a single-sample framebuffer
// using glBlitFramebuffer. This is recorded at render pass End() when a
// ResolveTarget is specified in the color attachment.
//
// When resolveToSurface is true, the destination is the Surface's swapchain
// offscreen FBO (NOT FBO 0). No Y-flip is applied here — the Y-flip is the
// sole responsibility of Queue.Present, which blits the swapchain FBO to FBO 0
// with srcY0=height, srcY1=0 before SwapBuffers. Mirrors Rust wgpu-hal:
// MSAA resolve lands in an offscreen target, present blit un-flips.
type MSAAResolveCommand struct {
	msaaTexture      *Texture // MSAA source texture (SampleCount > 1)
	resolveTexture   *Texture // Single-sample resolve target (nil when resolveToSurface)
	resolveToSurface bool     // True to resolve into the Surface's swapchain FBO
	surface          *Surface // Surface owning the swapchain FBO (resolveToSurface only)
	width, height    int32
}

func (c *MSAAResolveCommand) Execute(ctx *gl.Context) {
	// Disable scissor test before blit — glBlitFramebuffer respects GL_SCISSOR_TEST
	// on the draw framebuffer. Without this, only the last scissor rect's pixels
	// are copied, leaving the rest of the surface black (gg#226).
	ctx.Disable(gl.SCISSOR_TEST)

	// Bind MSAA FBO as read source.
	ctx.BindFramebuffer(gl.READ_FRAMEBUFFER, c.msaaTexture.fbo)

	// Bind the draw target.
	if c.resolveToSurface {
		// Resolve into the Surface's swapchain offscreen FBO. The Y-flip is
		// performed at Present time, not here, so the resolve is a straight
		// (non-flipped) MSAA resolve.
		var drawFBO uint32
		if c.surface != nil {
			drawFBO = c.surface.swapchainFBO
		}
		ctx.BindFramebuffer(gl.DRAW_FRAMEBUFFER, drawFBO)
	} else if !c.ensureResolveFBO(ctx) {
		return
	}

	// Straight MSAA resolve — no Y-flip. Both source and destination use the
	// same upside-down orientation; Queue.Present un-flips at SwapBuffers time.
	ctx.BlitFramebuffer(
		0, 0, c.width, c.height,
		0, 0, c.width, c.height,
		gl.COLOR_BUFFER_BIT, gl.NEAREST,
	)

	// Restore default framebuffer binding.
	ctx.BindFramebuffer(gl.READ_FRAMEBUFFER, 0)
	ctx.BindFramebuffer(gl.DRAW_FRAMEBUFFER, 0)
}

// ensureResolveFBO lazily creates the resolve FBO and binds it as draw target.
// Returns false if the FBO creation fails.
//
// The FBO is created on GL_DRAW_FRAMEBUFFER only: GL_FRAMEBUFFER would also
// replace the READ binding (the MSAA source bound by Execute), and the first
// resolve into each new texture then blitted the texture onto itself
// (P1-GLES-013). Rust wgpu-hal binds READ and DRAW explicitly for every blit.
func (c *MSAAResolveCommand) ensureResolveFBO(ctx *gl.Context) bool {
	if c.resolveTexture.fbo == 0 {
		fbo := ctx.GenFramebuffers(1)
		ctx.BindFramebuffer(gl.DRAW_FRAMEBUFFER, fbo)
		ctx.FramebufferTexture2D(gl.DRAW_FRAMEBUFFER, gl.COLOR_ATTACHMENT0,
			c.resolveTexture.target, c.resolveTexture.id, 0)
		if ctx.CheckFramebufferStatus(gl.DRAW_FRAMEBUFFER) != gl.FRAMEBUFFER_COMPLETE {
			ctx.DeleteFramebuffers(fbo)
			return false
		}
		c.resolveTexture.fbo = fbo
	}
	ctx.BindFramebuffer(gl.DRAW_FRAMEBUFFER, c.resolveTexture.fbo)
	return true
}

// AttachColorCommand attaches a color texture to a specific FBO attachment point.
// Used for MRT (Multiple Render Targets) to bind attachments 1..N.
// Attachment 0 is bound by EnsureOffscreenFBOCommand.
// Matches Rust wgpu-hal GLES BindAttachment for color targets (command.rs:580-582).
type AttachColorCommand struct {
	attachmentIndex uint32 // 0-based index (attachment point = GL_COLOR_ATTACHMENT0 + index)
	texture         *Texture
	mipLevel        uint32 // the view's base mip level
	layer           uint32 // the depth slice or array layer of a layered texture
}

func (c *AttachColorCommand) Execute(ctx *gl.Context) {
	attachColorLayer(ctx, gl.COLOR_ATTACHMENT0+c.attachmentIndex, c.texture, c.mipLevel, c.layer)
}

// SetDrawColorBuffersCommand configures the list of draw buffers for MRT output.
// Matches Rust wgpu-hal GLES SetDrawColorBuffers (command.rs:643-646, queue.rs:1202-1207).
type SetDrawColorBuffersCommand struct {
	count int // number of color attachments
}

func (c *SetDrawColorBuffersCommand) Execute(ctx *gl.Context) {
	bufs := make([]uint32, c.count)
	for i := range bufs {
		bufs[i] = gl.COLOR_ATTACHMENT0 + uint32(i)
	}
	ctx.DrawBuffers(bufs)
}

// ClearColorBufferCommand clears a specific color draw buffer with
// glClearBufferfv, glClearBufferuiv or glClearBufferiv, by the sample type of
// the attachment's format: Rust wgpu-hal GLES ClearColorF, ClearColorU and
// ClearColorI (command.rs:656-671). glClearBufferfv on an integer buffer
// leaves it undefined, and no integer target held its clear value.
// Per-buffer clears replace the old global glClearColor+glClear so
// each attachment of an MRT pass gets its own clear value.
type ClearColorBufferCommand struct {
	drawBuffer int32                      // 0-based draw buffer index
	sampleType gputypes.TextureSampleType // Uint, Sint, or a float type
	color      [4]float32                 // RGBA clear value of a float target
	ints       [4]int32                   // of a Sint target
	uints      [4]uint32                  // of a Uint target
}

// newClearColorBufferCommand records the clear of color attachment ca at
// draw buffer i. A surface view, which has no texture, is a float target.
func newClearColorBufferCommand(i int32, ca hal.RenderPassColorAttachment) *ClearColorBufferCommand {
	format := gputypes.TextureFormatUndefined
	if view, ok := ca.View.(*TextureView); ok && view.texture != nil {
		format = view.texture.format
	}
	c := ca.ClearValue
	return &ClearColorBufferCommand{
		drawBuffer: i,
		sampleType: hal.ClearColorSampleType(format),
		color:      [4]float32{float32(c.R), float32(c.G), float32(c.B), float32(c.A)},
		ints:       hal.ClearColorInt32(c),
		uints:      hal.ClearColorUint32(c),
	}
}

func (c *ClearColorBufferCommand) Execute(ctx *gl.Context) {
	ctx.Disable(gl.SCISSOR_TEST) // Ensure clear covers full framebuffer (not clipped by stale scissor)
	// Temporarily enable all color writes so the clear takes effect even if a
	// previous pipeline masked some channels. Matches Rust behavior which
	// sets color_mask(true,true,true,true) before clear (queue.rs:1134).
	ctx.ColorMask(true, true, true, true)
	switch c.sampleType {
	case gputypes.TextureSampleTypeUint:
		ctx.ClearBufferuiv(gl.COLOR, c.drawBuffer, &c.uints)
	case gputypes.TextureSampleTypeSint:
		ctx.ClearBufferiv(gl.COLOR, c.drawBuffer, &c.ints)
	default:
		ctx.ClearBufferfv(gl.COLOR, c.drawBuffer, &c.color)
	}
}

// ClearColorCommand clears a color attachment using the legacy global clear path.
// Retained for backward compatibility with tests and single-target code paths.
// For MRT, use ClearColorBufferCommand instead (per-buffer via glClearBufferfv).
type ClearColorCommand struct {
	r, g, b, a float32
}

func (c *ClearColorCommand) Execute(ctx *gl.Context) {
	ctx.Disable(gl.SCISSOR_TEST) // Ensure clear covers full framebuffer (not clipped by stale scissor)
	ctx.ClearColor(c.r, c.g, c.b, c.a)
	ctx.Clear(gl.COLOR_BUFFER_BIT)
}

// ClearDepthCommand clears the depth buffer.
type ClearDepthCommand struct {
	depth float64
}

func (c *ClearDepthCommand) Execute(ctx *gl.Context) {
	ctx.Disable(gl.SCISSOR_TEST)
	ctx.DepthMask(true)
	ctx.ClearDepth(c.depth)
	ctx.Clear(gl.DEPTH_BUFFER_BIT)
}

// ClearStencilCommand clears the stencil buffer.
type ClearStencilCommand struct {
	stencil int32
}

func (c *ClearStencilCommand) Execute(ctx *gl.Context) {
	ctx.Disable(gl.SCISSOR_TEST)
	// Ensure stencil write mask allows the clear to take effect.
	ctx.StencilMaskSeparate(gl.FRONT_AND_BACK, 0xFF)
	// Without glClearStencil every stencil clear wrote the context's
	// default clear value 0, whatever StencilClearValue the pass gave.
	ctx.ClearStencil(c.stencil)
	ctx.Clear(gl.STENCIL_BUFFER_BIT)
}

// UseProgramCommand activates a shader program.
type UseProgramCommand struct {
	programID uint32
}

func (c *UseProgramCommand) Execute(ctx *gl.Context) {
	ctx.UseProgram(c.programID)
}

// SetPipelineStateCommand sets pipeline state (culling, depth, stencil, blending, color mask).
type SetPipelineStateCommand struct {
	topology     gputypes.PrimitiveTopology
	cullMode     gputypes.CullMode
	frontFace    gputypes.FrontFace
	depthStencil *hal.DepthStencilState
	colorTargets []ColorTargetDesc // per-target blend/write-mask for MRT
	stencilRef   uint32
}

func (c *SetPipelineStateCommand) Execute(ctx *gl.Context) {
	// Culling
	if c.cullMode == gputypes.CullModeNone {
		ctx.Disable(gl.CULL_FACE)
	} else {
		ctx.Enable(gl.CULL_FACE)
		switch c.cullMode {
		case gputypes.CullModeFront:
			ctx.CullFace(gl.FRONT)
		case gputypes.CullModeBack:
			ctx.CullFace(gl.BACK)
		}
	}

	// Front face — swapped CW↔CCW to compensate for the Y-flip from
	// ADJUST_COORDINATE_SPACE. The negation of gl_Position.y reverses triangle
	// winding order, so we swap the front face to keep the same visibility.
	// Matches Rust wgpu-hal GLES (conv.rs:298-303).
	switch c.frontFace {
	case gputypes.FrontFaceCCW:
		ctx.FrontFace(gl.CW)
	case gputypes.FrontFaceCW:
		ctx.FrontFace(gl.CCW)
	}

	// Depth and stencil
	c.applyDepthStencilState(ctx)

	// Color targets (blend + write mask).
	// Matches Rust wgpu-hal GLES SetColorTarget (queue.rs:1483-1559):
	//   - If all targets are identical, use global (non-indexed) calls.
	//   - If targets differ, use per-draw-buffer indexed calls (GLES 3.2 / GL 4.0).
	//     Fallback: apply target[0] globally when indexed functions unavailable.
	c.applyColorTargets(ctx)
}

// applyColorTargets sets blend and write-mask state per render target.
// Matches Rust wgpu-hal GLES SetColorTarget command (queue.rs:1483-1559).
func (c *SetPipelineStateCommand) applyColorTargets(ctx *gl.Context) {
	if len(c.colorTargets) == 0 {
		// No color targets — disable blending, allow all color writes.
		ctx.Disable(gl.BLEND)
		ctx.ColorMask(true, true, true, true)
		return
	}

	// Single target or all targets identical: use global (non-indexed) calls.
	// Matches Rust path: draw_buffer_index == None (queue.rs:1527-1558).
	ct := c.colorTargets[0]
	ctx.ColorMask(
		ct.WriteMask&gputypes.ColorWriteMaskRed != 0,
		ct.WriteMask&gputypes.ColorWriteMaskGreen != 0,
		ct.WriteMask&gputypes.ColorWriteMaskBlue != 0,
		ct.WriteMask&gputypes.ColorWriteMaskAlpha != 0,
	)
	if ct.Blend != nil {
		ctx.Enable(gl.BLEND)
		ctx.BlendFuncSeparate(
			blendFactorToGL(ct.Blend.Color.SrcFactor),
			blendFactorToGL(ct.Blend.Color.DstFactor),
			blendFactorToGL(ct.Blend.Alpha.SrcFactor),
			blendFactorToGL(ct.Blend.Alpha.DstFactor),
		)
		ctx.BlendEquationSeparate(
			blendOperationToGL(ct.Blend.Color.Operation),
			blendOperationToGL(ct.Blend.Alpha.Operation),
		)
	} else {
		ctx.Disable(gl.BLEND)
	}
}

// applyDepthStencilState configures GL depth test and stencil test from pipeline state.
func (c *SetPipelineStateCommand) applyDepthStencilState(ctx *gl.Context) {
	if c.depthStencil == nil {
		ctx.Disable(gl.DEPTH_TEST)
		ctx.Disable(gl.STENCIL_TEST)
		return
	}

	// Depth test
	if c.depthStencil.DepthWriteEnabled || c.depthStencil.DepthCompare != gputypes.CompareFunctionAlways {
		ctx.Enable(gl.DEPTH_TEST)
		ctx.DepthMask(c.depthStencil.DepthWriteEnabled)
		ctx.DepthFunc(compareFunctionToGL(c.depthStencil.DepthCompare))
	} else {
		ctx.Disable(gl.DEPTH_TEST)
	}

	// Stencil test
	hasStencilOps := c.depthStencil.StencilFront.PassOp != hal.StencilOperationKeep ||
		c.depthStencil.StencilFront.FailOp != hal.StencilOperationKeep ||
		c.depthStencil.StencilBack.PassOp != hal.StencilOperationKeep ||
		c.depthStencil.StencilBack.FailOp != hal.StencilOperationKeep ||
		c.depthStencil.StencilFront.Compare != gputypes.CompareFunctionAlways ||
		c.depthStencil.StencilBack.Compare != gputypes.CompareFunctionAlways

	if !hasStencilOps && c.depthStencil.StencilWriteMask == 0 {
		ctx.Disable(gl.STENCIL_TEST)
		return
	}

	ctx.Enable(gl.STENCIL_TEST)
	ref := int32(c.stencilRef)

	ctx.StencilFuncSeparate(gl.FRONT,
		compareFunctionToGL(c.depthStencil.StencilFront.Compare),
		ref, c.depthStencil.StencilReadMask)
	ctx.StencilFuncSeparate(gl.BACK,
		compareFunctionToGL(c.depthStencil.StencilBack.Compare),
		ref, c.depthStencil.StencilReadMask)

	ctx.StencilOpSeparate(gl.FRONT,
		stencilOpToGL(c.depthStencil.StencilFront.FailOp),
		stencilOpToGL(c.depthStencil.StencilFront.DepthFailOp),
		stencilOpToGL(c.depthStencil.StencilFront.PassOp))
	ctx.StencilOpSeparate(gl.BACK,
		stencilOpToGL(c.depthStencil.StencilBack.FailOp),
		stencilOpToGL(c.depthStencil.StencilBack.DepthFailOp),
		stencilOpToGL(c.depthStencil.StencilBack.PassOp))

	ctx.StencilMaskSeparate(gl.FRONT, c.depthStencil.StencilWriteMask)
	ctx.StencilMaskSeparate(gl.BACK, c.depthStencil.StencilWriteMask)
}

// SetBindGroupCommand binds resources.
type SetBindGroupCommand struct {
	index           uint32
	group           *BindGroup
	dynamicOffsets  []uint32
	maxTextureUnits int32 // Hardware limit for validation
	// depthStencilTextureMode: sampled depth or stencil views select their
	// aspect with DEPTH_STENCIL_TEXTURE_MODE (Device.depthStencilTextureMode).
	depthStencilTextureMode bool
	// groupInfos stores per-group binding-to-slot tables from PipelineLayout.
	// Used to look up the GL slot for each binding instead of the old group*16+binding formula.
	groupInfos []BindGroupLayoutInfo
}

func (c *SetBindGroupCommand) Execute(ctx *gl.Context) {
	if c.group == nil {
		return
	}

	for i, entry := range c.group.entries {
		// Look up the GL slot index from the pre-computed per-type sequential
		// binding table (computed in CreatePipelineLayout). This replaces the old
		// group*16+binding formula and matches Rust wgpu-hal command.rs:720-783.
		glBinding := c.lookupSlot(entry.Binding)
		if glBinding == 0xFF {
			continue // binding not mapped in pipeline layout
		}

		switch res := entry.Resource.(type) {
		case gputypes.BufferBinding:
			// Buffer handle is the GL buffer object ID (from NativeHandle()).
			bufID := uint32(res.Buffer)
			if bufID == 0 {
				continue
			}
			offset := int(res.Offset)
			size := int(res.Size)

			// Determine GL buffer target and apply dynamic offset from layout entry.
			// Storage buffers use GL_SHADER_STORAGE_BUFFER, uniform buffers use GL_UNIFORM_BUFFER.
			// Matches Rust wgpu-hal/src/gles/command.rs:731-746 (set_bind_group).
			target, dynOff := c.resolveBufferTarget(entry.Binding)
			offset += dynOff

			if size > 0 {
				ctx.BindBufferRange(target, glBinding, bufID, offset, size)
			} else {
				ctx.BindBufferBase(target, glBinding, bufID)
			}

		case gputypes.TextureViewBinding:
			// The view was resolved from its handle at CreateBindGroup.
			view := c.group.views[i]
			if view == nil {
				continue
			}
			if st := c.group.layout.storageTexture(entry.Binding); st != nil {
				// glBinding is the image unit (the layout's image counter).
				bindStorageTexture(ctx, glBinding, view, st)
				continue
			}
			// Validate texture unit index against hardware limit.
			// Without this check, textures silently fail to bind when
			// glBinding >= GL_MAX_TEXTURE_IMAGE_UNITS (typically 8 on Intel).
			if c.maxTextureUnits > 0 && int32(glBinding) >= c.maxTextureUnits {
				if l := hal.Logger(); l != nil {
					l.Warn("GLES texture unit overflow: binding exceeds hardware limit",
						slog.Uint64("glBinding", uint64(glBinding)),
						slog.Int64("maxTextureUnits", int64(c.maxTextureUnits)),
						slog.Uint64("group", uint64(c.index)),
						slog.Uint64("binding", uint64(entry.Binding)),
					)
				}
				continue
			}
			bindSampledTexture(ctx, glBinding, view, c.depthStencilTextureMode)
		}
		// Samplers are bound by BindSamplerCommand to the texture units the
		// pipeline combines them with (CommandEncoder.setBindGroup).
	}
}

// bindStorageTexture binds a storage texture view to an image unit, as Rust
// wgpu-hal gles does (device.rs create_bind_group builds the ImageBinding,
// queue.rs C::BindImage calls glBindImageTexture): the view's first mip
// level, all layers for 2D-array and cube-array views, otherwise the view's
// first layer, the layout's access, and the layout format's internal format.
func bindStorageTexture(ctx *gl.Context, unit uint32, view *TextureView, st *gputypes.StorageTextureBindingLayout) {
	layered := st.ViewDimension == gputypes.TextureViewDimension2DArray ||
		st.ViewDimension == gputypes.TextureViewDimensionCubeArray
	desc, err := describeTextureFormat(st.Format)
	if err != nil {
		// Unreachable through core: the bound view's format is the layout's
		// format, and CreateTexture refuses formats without GL storage.
		hal.Logger().Error("gles: storage texture binding of an unsupported format", "err", err)
		return
	}
	ctx.BindImageTexture(unit, view.texture.id, int32(view.baseMip), layered, int32(view.baseLayer),
		storageAccessToGL(st.Access), desc.internal)
}

// bindSampledTexture binds a sampled texture view to a texture unit, as Rust
// wgpu-hal gles queue.rs C::BindTexture does. GL has no texture views: a
// texture binds only through the target it was created with
// (checkSampledView refuses views that need another target), and the view's
// mip range is set on the texture itself as TEXTURE_BASE_LEVEL and
// TEXTURE_MAX_LEVEL at every bind, so a view of mip N samples level N. The
// texture parameters are per texture, not per view: two views of one texture
// with different mip ranges bound to one draw sample the range bound last,
// as in Rust. A depth-only or stencil-only view of a combined depth-stencil
// texture selects its aspect with DEPTH_STENCIL_TEXTURE_MODE where the
// context has it (ES 3.1, GL 4.3); before that GL samples the depth.
func bindSampledTexture(ctx *gl.Context, unit uint32, view *TextureView, depthStencilTextureMode bool) {
	tex := view.texture
	ctx.ActiveTexture(gl.TEXTURE0 + unit)
	ctx.BindTexture(tex.target, tex.id)
	if tex.target == gl.TEXTURE_2D_MULTISAMPLE {
		// A multisampled texture has one level and no level parameters.
		return
	}
	base, count := view.mipRange()
	ctx.TexParameteri(tex.target, gl.TEXTURE_BASE_LEVEL, int32(base))
	ctx.TexParameteri(tex.target, gl.TEXTURE_MAX_LEVEL, int32(base+count-1))
	if !depthStencilTextureMode || !tex.format.HasDepth() || !tex.format.HasStencil() {
		return
	}
	// Set at every bind, depth unless the view is stencil-only: Rust sets
	// nothing for a view of both aspects, which leaves the mode an earlier
	// stencil-only bind set.
	mode := int32(gl.DEPTH_COMPONENT)
	if view.aspect == gputypes.TextureAspectStencilOnly {
		mode = gl.STENCIL_INDEX
	}
	ctx.TexParameteri(tex.target, gl.DEPTH_STENCIL_TEXTURE_MODE, mode)
}

// storageAccessToGL maps a storage texture access to the glBindImageTexture
// access (Rust wgpu-hal gles/conv.rs map_storage_access).
func storageAccessToGL(access gputypes.StorageTextureAccess) uint32 {
	switch access {
	case gputypes.StorageTextureAccessReadOnly:
		return gl.READ_ONLY
	case gputypes.StorageTextureAccessWriteOnly:
		return gl.WRITE_ONLY
	default: // ReadWrite
		return gl.READ_WRITE
	}
}

// lookupSlot returns the GL slot index for a binding number using the
// pre-computed GroupInfos from PipelineLayout. Returns 0xFF if not mapped.
func (c *SetBindGroupCommand) lookupSlot(binding uint32) uint32 {
	if int(c.index) < len(c.groupInfos) {
		info := c.groupInfos[c.index]
		if int(binding) < len(info.BindingToSlot) {
			return uint32(info.BindingToSlot[binding])
		}
	}
	return 0xFF
}

// resolveBufferTarget determines the GL buffer target (UNIFORM_BUFFER or SHADER_STORAGE_BUFFER)
// and dynamic offset for a binding number by looking up the bind group layout entry.
// Returns the GL target and the dynamic offset to apply (0 if none).
// Matches Rust wgpu-hal/src/gles/command.rs set_bind_group buffer target
// selection.
func (c *SetBindGroupCommand) resolveBufferTarget(binding uint32) (uint32, int) {
	target := uint32(gl.UNIFORM_BUFFER)
	dynOffset := 0
	if c.group.layout == nil {
		return target, dynOffset
	}
	for _, le := range c.group.layout.entries {
		if le.Binding != binding || le.Buffer == nil {
			continue
		}
		if le.Buffer.Type == gputypes.BufferBindingTypeStorage ||
			le.Buffer.Type == gputypes.BufferBindingTypeReadOnlyStorage {
			target = gl.SHADER_STORAGE_BUFFER
		}
		if le.Buffer.HasDynamicOffset {
			if i := c.group.layout.dynamicOffsetIndex(binding); i < len(c.dynamicOffsets) {
				dynOffset = int(c.dynamicOffsets[i])
			}
		}
		break
	}
	return target, dynOffset
}

// SetVertexBufferCommand binds a vertex buffer and configures vertex attributes.
// In OpenGL, vertex attributes must be configured explicitly via
// glVertexAttribPointer + glEnableVertexAttribArray. The layout describes
// how vertex data is interpreted (attribute locations, formats, strides).
type SetVertexBufferCommand struct {
	slot   uint32
	buffer *Buffer
	offset uint64
	layout *gputypes.VertexBufferLayout // from the render pipeline descriptor
}

func (c *SetVertexBufferCommand) Execute(ctx *gl.Context) {
	ctx.BindBuffer(gl.ARRAY_BUFFER, c.buffer.id)

	// Configure vertex attributes from the pipeline's vertex layout.
	if c.layout == nil {
		return
	}
	stride := int32(c.layout.ArrayStride)
	divisor := vertexAttribDivisor(c.layout)
	for _, attr := range c.layout.Attributes {
		loc := attr.ShaderLocation
		size, typ, normalized := vertexFormatToGL(attr.Format)
		attrOffset := uintptr(c.offset) + uintptr(attr.Offset)
		ctx.EnableVertexAttribArray(loc)
		ctx.VertexAttribPointer(loc, size, typ, normalized, stride, attrOffset)
		// Set the instance divisor. Per-vertex attributes get divisor=0 (reset),
		// per-instance attributes get divisor=1. Without this, instanced rendering
		// reads the same instance data for all instances.
		ctx.VertexAttribDivisor(loc, divisor)
	}
}

// broadcastDivisor is the instance divisor that makes every instance read
// element 0 of a vertex attribute.
const broadcastDivisor = 0xFFFFFFFF

// vertexAttribDivisor returns the glVertexAttribDivisor value for a vertex
// buffer layout: 0 per vertex, 1 per instance (Rust wgpu-hal
// src/gles/queue.rs), and broadcastDivisor for arrayStride 0.
//
// arrayStride 0 is WebGPU's broadcast: every vertex and instance reads
// element 0. glVertexAttribPointer reads a stride of 0 as tightly packed, so
// the stride cannot express it. A non-zero divisor makes all vertices of an
// instance share one element, and the largest divisor makes every instance
// share element 0. This is Dawn's emulation (src/dawn/native/opengl/
// RenderPipelineGL.cpp); Rust wgpu-hal passes the 0 through.
func vertexAttribDivisor(layout *gputypes.VertexBufferLayout) uint32 {
	switch {
	case layout.ArrayStride == 0:
		return broadcastDivisor
	case layout.StepMode == gputypes.VertexStepModeInstance:
		return 1
	default:
		return 0
	}
}

// DisableVertexAttribsCommand disables vertex attribute arrays.
type DisableVertexAttribsCommand struct {
	locations []uint32
}

func (c *DisableVertexAttribsCommand) Execute(ctx *gl.Context) {
	for _, loc := range c.locations {
		ctx.DisableVertexAttribArray(loc)
	}
}

// SetIndexBufferCommand binds an index buffer.
type SetIndexBufferCommand struct {
	buffer *Buffer
	format gputypes.IndexFormat
	offset uint64
}

func (c *SetIndexBufferCommand) Execute(ctx *gl.Context) {
	ctx.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, c.buffer.id)
}

// SetViewportCommand sets the viewport.
type SetViewportCommand struct {
	x, y, width, height float32
	minDepth, maxDepth  float32
}

func (c *SetViewportCommand) Execute(ctx *gl.Context) {
	ctx.Viewport(int32(c.x), int32(c.y), int32(c.width), int32(c.height))
	ctx.DepthRange(float64(c.minDepth), float64(c.maxDepth))
}

// SetScissorCommand sets the scissor rectangle.
// With ADJUST_COORDINATE_SPACE enabled, the scene is rendered upside-down in GL.
// The scissor coordinates are passed through directly (no Y-flip needed) because
// the scissor operates in the same flipped coordinate space as the rendered content.
// This matches Rust wgpu-hal GLES which also passes scissor through without Y-flip.
type SetScissorCommand struct {
	x, y, width, height uint32
}

func (c *SetScissorCommand) Execute(ctx *gl.Context) {
	ctx.Enable(gl.SCISSOR_TEST)
	// No Y-flip: ADJUST_COORDINATE_SPACE flips the scene in the vertex shader,
	// so GL pixel Y=0 corresponds to the top of the scene (WebGPU Y=0).
	// The scissor rect in WebGPU coords maps directly to GL coords.
	ctx.Scissor(int32(c.x), int32(c.y), int32(c.width), int32(c.height))
}

// SetBlendConstantCommand sets blend constant.
type SetBlendConstantCommand struct {
	r, g, b, a float32
}

func (c *SetBlendConstantCommand) Execute(ctx *gl.Context) {
	ctx.BlendColor(c.r, c.g, c.b, c.a)
}

// SetStencilRefCommand updates the stencil reference value.
// This re-applies glStencilFuncSeparate with the new reference while
// keeping the compare function and read mask from the current pipeline.
type SetStencilRefCommand struct {
	ref          uint32
	depthStencil *hal.DepthStencilState
}

func (c *SetStencilRefCommand) Execute(ctx *gl.Context) {
	if c.depthStencil == nil {
		return
	}
	ref := int32(c.ref)
	ctx.StencilFuncSeparate(gl.FRONT,
		compareFunctionToGL(c.depthStencil.StencilFront.Compare),
		ref, c.depthStencil.StencilReadMask)
	ctx.StencilFuncSeparate(gl.BACK,
		compareFunctionToGL(c.depthStencil.StencilBack.Compare),
		ref, c.depthStencil.StencilReadMask)
}

// setFirstInstance sets the naga_vs_first_instance uniform, through which an
// emulated first instance reaches instance_index.
func setFirstInstance(ctx *gl.Context, location int32, firstInstance uint32) {
	if location >= 0 {
		ctx.Uniform1ui(location, firstInstance)
	}
}

// DrawCommand executes a non-indexed draw (Rust wgpu-hal gles/queue.rs
// C::Draw). It always draws instanced: ANGLE ignores the instance divisor in
// glDrawArrays (gfx-rs/wgpu#3578).
type DrawCommand struct {
	vertexCount, instanceCount uint32
	firstVertex, firstInstance uint32
	topology                   gputypes.PrimitiveTopology
	fullInstancing             bool
	firstInstanceLocation      int32
}

func (c *DrawCommand) Execute(ctx *gl.Context) {
	mode := primitiveTopologyToGL(c.topology)
	if c.fullInstancing {
		ctx.DrawArraysInstancedBaseInstance(mode, int32(c.firstVertex), int32(c.vertexCount), int32(c.instanceCount), c.firstInstance)
		return
	}
	setFirstInstance(ctx, c.firstInstanceLocation, c.firstInstance)
	ctx.DrawArraysInstanced(mode, int32(c.firstVertex), int32(c.vertexCount), int32(c.instanceCount))
}

// indexTypeAndSize returns the GL index type and its size in bytes.
func indexTypeAndSize(format gputypes.IndexFormat) (uint32, uint64) {
	if format == gputypes.IndexFormatUint32 {
		return gl.UNSIGNED_INT, 4
	}
	return gl.UNSIGNED_SHORT, 2
}

// DrawIndexedCommand executes an indexed draw (Rust wgpu-hal gles/queue.rs
// C::DrawIndexed): the index buffer offset plus firstIndex elements, the base
// vertex, and the first instance as for DrawCommand.
type DrawIndexedCommand struct {
	indexCount, instanceCount uint32
	firstIndex                uint32
	indexOffset               uint64 // SetIndexBuffer offset in bytes
	baseVertex                int32
	firstInstance             uint32
	indexFormat               gputypes.IndexFormat
	topology                  gputypes.PrimitiveTopology
	fullInstancing            bool
	firstInstanceLocation     int32
}

func (c *DrawIndexedCommand) Execute(ctx *gl.Context) {
	indexType, indexSize := indexTypeAndSize(c.indexFormat)
	offset := uintptr(c.indexOffset + uint64(c.firstIndex)*indexSize)
	mode := primitiveTopologyToGL(c.topology)

	switch {
	case c.fullInstancing:
		ctx.DrawElementsInstancedBaseVertexBaseInstance(mode, int32(c.indexCount), indexType, offset,
			int32(c.instanceCount), c.baseVertex, c.firstInstance)
	case c.baseVertex == 0:
		setFirstInstance(ctx, c.firstInstanceLocation, c.firstInstance)
		ctx.DrawElementsInstanced(mode, int32(c.indexCount), indexType, offset, int32(c.instanceCount))
	default:
		// Core offers DownlevelFlagsBaseVertex only where this exists.
		setFirstInstance(ctx, c.firstInstanceLocation, c.firstInstance)
		ctx.DrawElementsInstancedBaseVertex(mode, int32(c.indexCount), indexType, offset,
			int32(c.instanceCount), c.baseVertex)
	}
}

// DrawIndirectCommand draws count DrawIndirectArgs records (Rust wgpu-hal
// gles/queue.rs C::DrawIndirect). The first instance in a record reaches GL
// only with fully featured instancing; the uniform is 0. Several records go
// to one glMultiDrawArraysIndirect where the context has it, so gl_DrawID
// (draw_index) counts them; Rust issues one draw per record, and offers no
// MultiDrawIndirect on GLES.
type DrawIndirectCommand struct {
	buffer                uint32
	offset                uint64
	count                 uint32
	topology              gputypes.PrimitiveTopology
	firstInstanceLocation int32
}

func (c *DrawIndirectCommand) Execute(ctx *gl.Context) {
	mode := primitiveTopologyToGL(c.topology)
	setFirstInstance(ctx, c.firstInstanceLocation, 0)
	ctx.BindBuffer(gl.DRAW_INDIRECT_BUFFER, c.buffer)
	if c.count > 1 && ctx.HasMultiDrawIndirect() {
		ctx.MultiDrawArraysIndirect(mode, uintptr(c.offset), int32(c.count), drawIndirectRecordSize)
		return
	}
	for i := range uint64(c.count) {
		ctx.DrawArraysIndirect(mode, uintptr(c.offset+i*drawIndirectRecordSize))
	}
}

// DrawIndexedIndirectCommand draws count DrawIndexedIndirectArgs records
// (Rust wgpu-hal gles/queue.rs C::DrawIndexedIndirect), as DrawIndirectCommand.
//
// glDrawElementsIndirect has no index buffer offset: a record's firstIndex
// counts from the start of the bound GL_ELEMENT_ARRAY_BUFFER, and the record
// lives on the GPU, so it cannot be patched on the CPU. With a nonzero
// SetIndexBuffer offset the draw therefore binds a GPU copy of the index
// buffer from that offset (glCopyBufferSubData) and restores the original
// binding afterwards. Rust wgpu-hal gles ignores the offset here (P1-GLES-018).
type DrawIndexedIndirectCommand struct {
	buffer                uint32
	offset                uint64
	count                 uint32
	indexFormat           gputypes.IndexFormat
	topology              gputypes.PrimitiveTopology
	firstInstanceLocation int32

	// indexBuffer, indexOffset and indexBufferSize describe the bound index
	// buffer when its SetIndexBuffer offset is nonzero; zero otherwise.
	indexBuffer     uint32
	indexOffset     uint64
	indexBufferSize uint64
}

func (c *DrawIndexedIndirectCommand) Execute(ctx *gl.Context) {
	if c.indexOffset != 0 {
		if c.indexOffset >= c.indexBufferSize {
			return // no index lies in bounds, so every record is out of range
		}
		shifted := shiftedIndexBuffer(ctx, c.indexBuffer, c.indexOffset, c.indexBufferSize)
		defer func() {
			ctx.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, c.indexBuffer)
			// The driver keeps the copy alive until the draws that read it finish.
			ctx.DeleteBuffers(shifted)
		}()
	}
	mode := primitiveTopologyToGL(c.topology)
	indexType, _ := indexTypeAndSize(c.indexFormat)
	setFirstInstance(ctx, c.firstInstanceLocation, 0)
	ctx.BindBuffer(gl.DRAW_INDIRECT_BUFFER, c.buffer)
	if c.count > 1 && ctx.HasMultiDrawIndirect() {
		ctx.MultiDrawElementsIndirect(mode, indexType, uintptr(c.offset), int32(c.count), drawIndexedIndirectRecordSize)
		return
	}
	for i := range uint64(c.count) {
		ctx.DrawElementsIndirect(mode, indexType, uintptr(c.offset+i*drawIndexedIndirectRecordSize))
	}
}

// shiftedIndexBuffer copies bytes [offset, size) of the index buffer into a
// new GL buffer on the GPU, binds it to GL_ELEMENT_ARRAY_BUFFER and returns
// it. The copy goes through the COPY_WRITE target, which leaves the vertex,
// index and uniform bindings alone.
func shiftedIndexBuffer(ctx *gl.Context, indexBuffer uint32, offset, size uint64) uint32 {
	remaining := int(size - offset)
	shifted := ctx.GenBuffers(1)
	ctx.BindBuffer(gl.COPY_WRITE_BUFFER, shifted)
	ctx.AllocateBuffer(gl.COPY_WRITE_BUFFER, remaining, gl.STREAM_COPY)
	ctx.BindBuffer(gl.COPY_READ_BUFFER, indexBuffer)
	ctx.CopyBufferSubData(gl.COPY_READ_BUFFER, gl.COPY_WRITE_BUFFER, int(offset), 0, remaining)
	ctx.BindBuffer(gl.COPY_READ_BUFFER, 0)
	ctx.BindBuffer(gl.COPY_WRITE_BUFFER, 0)
	ctx.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, shifted)
	return shifted
}

// CopyBufferCommand copies a region of one GL buffer object into another.
type CopyBufferCommand struct {
	srcID, dstID         uint32
	srcOffset, dstOffset uint64
	size                 uint64
}

// Execute performs the copy with glCopyBufferSubData through the dedicated
// COPY_READ_BUFFER / COPY_WRITE_BUFFER binding points, so the copy does not
// disturb the buffers bound to the vertex, index or uniform targets.
// Mirrors Rust wgpu-hal gles/queue.rs C::CopyBufferToBuffer (raw → raw case).
// Desktop GL and GLES 3.0 allow any buffer role on the copy targets; the
// ELEMENT_ARRAY_BUFFER special case in Rust exists only for WebGL.
func (c *CopyBufferCommand) Execute(ctx *gl.Context) {
	if c.size == 0 || c.srcID == 0 || c.dstID == 0 {
		return
	}
	ctx.BindBuffer(gl.COPY_READ_BUFFER, c.srcID)
	ctx.BindBuffer(gl.COPY_WRITE_BUFFER, c.dstID)
	ctx.CopyBufferSubData(gl.COPY_READ_BUFFER, gl.COPY_WRITE_BUFFER,
		int(c.srcOffset), int(c.dstOffset), int(c.size))
	ctx.BindBuffer(gl.COPY_READ_BUFFER, 0)
	ctx.BindBuffer(gl.COPY_WRITE_BUFFER, 0)
}

// DispatchCommand dispatches compute work.
type DispatchCommand struct {
	x, y, z uint32
}

// Execute dispatches compute work and inserts a memory barrier.
func (c *DispatchCommand) Execute(ctx *gl.Context) {
	ctx.DispatchCompute(c.x, c.y, c.z)
	// VERTEX_ATTRIB_ARRAY_BARRIER_BIT is required when compute writes an SSBO that
	// is later read as a vertex buffer (e.g. particles ping-pong). Without it,
	// vertex fetch reads stale pre-compute data. Found by @lkmavi (PR #215).
	ctx.MemoryBarrier(gl.SHADER_STORAGE_BARRIER_BIT | gl.VERTEX_ATTRIB_ARRAY_BARRIER_BIT | gl.BUFFER_UPDATE_BARRIER_BIT)
}

// DispatchIndirectCommand dispatches compute work with GPU-generated parameters.
type DispatchIndirectCommand struct {
	buffer *Buffer
	offset uint64
}

// Execute dispatches compute work from indirect buffer and inserts a memory barrier.
func (c *DispatchIndirectCommand) Execute(ctx *gl.Context) {
	// Bind the buffer containing dispatch parameters
	ctx.BindBuffer(gl.DISPATCH_INDIRECT_BUFFER, c.buffer.id)
	// Dispatch with parameters from the buffer at the given offset
	ctx.DispatchComputeIndirect(uintptr(c.offset))
	// Unbind the indirect buffer
	ctx.BindBuffer(gl.DISPATCH_INDIRECT_BUFFER, 0)
	ctx.MemoryBarrier(gl.SHADER_STORAGE_BARRIER_BIT | gl.VERTEX_ATTRIB_ARRAY_BARRIER_BIT | gl.BUFFER_UPDATE_BARRIER_BIT)
}

// CopyTextureToBufferCommand reads pixels from a texture's FBO into a buffer's
// CPU-side data slice. This is the standard GLES readback path since GLES lacks
// glGetTexImage. The approach: bind texture FBO -> glReadPixels -> copy to buffer.
type CopyTextureToBufferCommand struct {
	glCtx      *gl.Context
	srcTexture *Texture
	dstBuffer  *Buffer
	srcOrigin  [3]uint32 // x, y, z
	copySize   [3]uint32 // width, height, depthOrArrayLayers
	mipLevel   uint32
	// aspect of the copy: the depth or the stencil of a combined format.
	aspect      gputypes.TextureAspect
	dstOffset   uint64
	bytesPerRow uint32
	// rowsPerImage of the destination layout; 0 means the copy's height.
	rowsPerImage uint32
}

// readback is how glReadPixels reads one aspect of a texture: the
// framebuffer attachment the texture goes to, the pixel transfer format and
// type, and the bytes of one texel in the buffer.
type readback struct {
	attachment, dataFormat, dataType, bpp uint32
}

// readbackOf returns how to read aspect of a texture of format, or false for
// a format and aspect no buffer copy may read (core rejects them:
// depth24plus and the depth of depth24plus-stencil8, whose layout WebGPU
// leaves undefined).
//
// Color is read in the texture's own transfer format and type, as Rust
// wgpu-hal does (gles/queue.rs CopyTextureToBuffer: format_desc.external,
// data_type); a hard-coded GL_BGRA swapped R and B for RGBA8 textures.
//
// Depth and stencil are attached at their own attachment point and read as
// DEPTH_COMPONENT or STENCIL_INDEX, in the buffer layout WebGPU gives the
// aspect: depth16unorm as 16-bit unorm, depth32float as float, stencil as
// stencil8. This deviates from Rust wgpu-hal, which attaches every format
// to COLOR_ATTACHMENT0 (gles/queue.rs CopyTextureToBuffer), so a depth
// texture leaves the framebuffer incomplete and its readback fails; Rust
// never reports DEPTH_TEXTURE_AND_BUFFER_COPIES on GLES, so the path is not
// reached there. We report the flag on desktop GL, where glReadPixels reads
// depth and stencil, and leave it off on ES, so core refuses these copies.
func readbackOf(format gputypes.TextureFormat, aspect gputypes.TextureAspect) (readback, bool) {
	stencil := readback{gl.STENCIL_ATTACHMENT, gl.STENCIL_INDEX, gl.UNSIGNED_BYTE, 1}
	switch format {
	case gputypes.TextureFormatStencil8:
		return stencil, true
	case gputypes.TextureFormatDepth16Unorm:
		return readback{gl.DEPTH_ATTACHMENT, gl.DEPTH_COMPONENT, gl.UNSIGNED_SHORT, 2}, true
	case gputypes.TextureFormatDepth32Float:
		return readback{gl.DEPTH_ATTACHMENT, gl.DEPTH_COMPONENT, gl.FLOAT, 4}, true
	case gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureFormatDepth32FloatStencil8:
		stencil.attachment = gl.DEPTH_STENCIL_ATTACHMENT
		switch {
		case aspect == gputypes.TextureAspectStencilOnly:
			return stencil, true
		case aspect == gputypes.TextureAspectDepthOnly && format == gputypes.TextureFormatDepth32FloatStencil8:
			return readback{gl.DEPTH_STENCIL_ATTACHMENT, gl.DEPTH_COMPONENT, gl.FLOAT, 4}, true
		}
		return readback{}, false
	case gputypes.TextureFormatDepth24Plus:
		return readback{}, false
	}
	desc, err := describeTextureFormat(format)
	if err != nil {
		return readback{}, false
	}
	bpp := glPixelBytes(desc.external, desc.dataType)
	return readback{gl.COLOR_ATTACHMENT0, desc.external, desc.dataType, bpp}, bpp != 0
}

// Execute reads pixels from the source texture's FBO into the destination buffer.
func (c *CopyTextureToBufferCommand) Execute(ctx *gl.Context) {
	width := int32(c.copySize[0])
	height := int32(c.copySize[1])
	if width == 0 || height == 0 {
		return
	}

	rb, ok := readbackOf(c.srcTexture.format, c.aspect)
	if !ok {
		hal.Logger().Error("gles: CopyTextureToBuffer of a format and aspect no copy may read",
			"format", c.srcTexture.format, "aspect", c.aspect)
		return
	}
	rowBytes := uint32(width) * rb.bpp

	// Rows land at the caller's bytesPerRow (WebGPU requires a multiple of 256
	// for multi-row copies); writing them tightly packed sheared the image.
	stride := uint64(c.bytesPerRow)
	if stride < uint64(rowBytes) {
		stride = uint64(rowBytes) // bytesPerRow may be omitted for a single row
	}
	// A 3D or 2D array texture or a cube map is read one layer at a time,
	// each layer rowsPerImage rows after the previous one.
	layers := uint32(1)
	if hasLayers(c.srcTexture.target) {
		layers = max(c.copySize[2], 1)
	}
	rowsPerImage := uint64(c.rowsPerImage)
	if rowsPerImage < uint64(height) {
		rowsPerImage = uint64(height)
	}
	imageStride := stride * rowsPerImage

	requiredSize := c.dstOffset + imageStride*uint64(layers-1) + stride*uint64(height-1) + uint64(rowBytes)
	if requiredSize > c.dstBuffer.size {
		return // core validates copy bounds; never write past the buffer
	}
	// The readback lands in the buffer's CPU shadow, and MapBuffer uses the
	// shadow only when it covers the whole buffer (otherwise it reads the GL
	// buffer, which this copy never wrote). Start a full-size shadow from the
	// GL buffer's current contents so bytes outside the copied rows keep
	// their values. Removing the shadows is ADR-076 (glMapBufferRange).
	if uint64(len(c.dstBuffer.data)) != c.dstBuffer.size {
		c.dstBuffer.data = readGLBuffer(ctx, c.dstBuffer)
	}

	// Save the current FBO binding so we can restore it after the read.
	var prevFBO int32
	ctx.GetIntegerv(gl.FRAMEBUFFER_BINDING, &prevFBO)

	if !c.bindSourceFBO(ctx, rb.attachment, c.srcOrigin[2]) {
		ctx.BindFramebuffer(gl.FRAMEBUFFER, uint32(prevFBO))
		return
	}

	// Set tight pixel packing (no row alignment padding).
	ctx.PixelStorei(gl.PACK_ALIGNMENT, 1)

	tmpBuf := make([]byte, uint64(rowBytes)*uint64(height))
	for i := uint32(0); i < layers; i++ {
		if i > 0 {
			attachColorLayer(ctx, rb.attachment, c.srcTexture, c.mipLevel, c.srcOrigin[2]+i)
		}
		ctx.ReadPixels(
			int32(c.srcOrigin[0]), int32(c.srcOrigin[1]),
			width, height,
			rb.dataFormat, rb.dataType,
			unsafe.Pointer(&tmpBuf[0]),
		)
		// Copy the rows in memory order. Shaders are compiled with naga's
		// ADJUST_COORDINATE_SPACE, so texture row 0 is already the top row of
		// the image (Present does the Y flip for the window); flipping here
		// turned every readback upside down. Rust wgpu-hal does not flip either.
		base := c.dstOffset + uint64(i)*imageStride
		for row := uint64(0); row < uint64(height); row++ {
			srcStart := row * uint64(rowBytes)
			dstStart := base + row*stride
			copy(c.dstBuffer.data[dstStart:dstStart+uint64(rowBytes)], tmpBuf[srcStart:srcStart+uint64(rowBytes)])
		}
	}

	// Restore the previous FBO binding.
	ctx.BindFramebuffer(gl.FRAMEBUFFER, uint32(prevFBO))
}

// bindSourceFBO binds the source texture's FBO, created lazily, with the
// copy's mip level (and, for a 3D or 2D array texture, layer) attached at
// attachment: COLOR_ATTACHMENT0, or the depth/stencil point of a depth or
// stencil texture (readbackOf). The FBO serves every view and copy of the
// texture, so the level and layer are attached every time, as Rust wgpu-hal
// attaches them for each copy (gles/queue.rs CopyTextureToBuffer). Reading
// level 0 and layer 0 for every copy returned their texels for a copy of
// another level or layer.
//
// It returns false, and logs the copy as dropped, when a new FBO is
// incomplete: glReadPixels is the only way out of a GL texture, and it needs
// a framebuffer the format can be attached to. RGB9E5Ufloat is not
// color-renderable on Mesa or on ES, so its copies stop here. Rust wgpu-hal
// reads through the same FBO and has the same gap; Dawn copies such formats
// out with a compute blit (UseBlitForRGB9E5UfloatTextureCopy), which is card
// P2-GLES-023. Command.Execute has no error return, so the log is the only
// report the HAL can give.
func (c *CopyTextureToBufferCommand) bindSourceFBO(ctx *gl.Context, attachment, layer uint32) bool {
	if c.srcTexture.fbo != 0 {
		ctx.BindFramebuffer(gl.FRAMEBUFFER, c.srcTexture.fbo)
		attachColorLayer(ctx, attachment, c.srcTexture, c.mipLevel, layer)
		return true
	}
	fbo := ctx.GenFramebuffers(1)
	ctx.BindFramebuffer(gl.FRAMEBUFFER, fbo)
	attachColorLayer(ctx, attachment, c.srcTexture, c.mipLevel, layer)
	if status := ctx.CheckFramebufferStatus(gl.FRAMEBUFFER); status != gl.FRAMEBUFFER_COMPLETE {
		ctx.DeleteFramebuffers(fbo)
		hal.Logger().Error(copySourceIncompleteMsg,
			"format", c.srcTexture.format.String(), "status", fmt.Sprintf("0x%04X", status))
		return false
	}
	c.srcTexture.fbo = fbo
	return true
}

// readGLBuffer returns a copy of buf's full GL contents (zeros if the buffer
// cannot be mapped). Caller holds the context.
func readGLBuffer(ctx *gl.Context, buf *Buffer) []byte {
	data := make([]byte, buf.size)
	if buf.id == 0 || buf.size == 0 {
		return data
	}
	ctx.BindBuffer(gl.COPY_READ_BUFFER, buf.id)
	if ptr := ctx.MapBuffer(gl.COPY_READ_BUFFER, gl.READ_ONLY); ptr != 0 {
		src := unsafe.Slice(*(**byte)(unsafe.Pointer(&ptr)), buf.size)
		copy(data, src)
		ctx.UnmapBuffer(gl.COPY_READ_BUFFER)
	}
	ctx.BindBuffer(gl.COPY_READ_BUFFER, 0)
	return data
}

// glPixelBytes returns the size of one pixel that glReadPixels writes for a
// color transfer format and type, or 0 for formats it cannot read here.
func glPixelBytes(dataFormat, dataType uint32) uint32 {
	switch dataType {
	case gl.UNSIGNED_INT_2_10_10_10_REV, gl.UNSIGNED_INT_10F_11F_11F_REV, gl.UNSIGNED_INT_5_9_9_9_REV:
		// Packed types hold the whole pixel in one 32-bit word.
		if dataFormat == gl.RGB || dataFormat == gl.RGBA || dataFormat == gl.RGBA_INTEGER {
			return 4
		}
		return 0
	}
	var components uint32
	switch dataFormat {
	case gl.RED, gl.RED_INTEGER:
		components = 1
	case gl.RG, gl.RG_INTEGER:
		components = 2
	case gl.RGB, gl.RGB_INTEGER:
		components = 3
	case gl.RGBA, gl.BGRA, gl.RGBA_INTEGER:
		components = 4
	default:
		return 0
	}
	switch dataType {
	case gl.UNSIGNED_BYTE, gl.BYTE:
		return components
	case gl.HALF_FLOAT, gl.UNSIGNED_SHORT, gl.SHORT:
		return components * 2
	case gl.FLOAT, gl.UNSIGNED_INT, gl.INT:
		return components * 4
	}
	return 0
}

// CopyBufferToTextureCommand copies buffer data to a texture using a pixel unpack
// buffer. Binds the source GL buffer as GL_PIXEL_UNPACK_BUFFER, then calls
// glTexSubImage2D (glTexSubImage3D for a 3D or 2D array texture) with the
// buffer offset as the pixels pointer, so GL reads from the bound PBO.
// Matches Rust wgpu-hal/src/gles/queue.rs CopyBufferToTexture.
type CopyBufferToTextureCommand struct {
	srcBuffer *Buffer
	dstTex    *Texture
	origin    [3]uint32 // x, y, z (z is the depth slice or array layer)
	copySize  [3]uint32 // width, height, depthOrArrayLayers
	mipLevel  uint32
	bufOffset uint64
	// bytesPerRow of the source layout; 0 means tightly packed rows.
	bytesPerRow uint32
	// rowsPerImage of the source layout; 0 means the copy's height.
	rowsPerImage uint32
}

// unpackBufferOffset returns a byte offset into the bound PIXEL_UNPACK_BUFFER
// in the pointer argument GL reads it from. The value is never dereferenced
// by Go (the govet unsafeptr exclusion for this file in .golangci.yml).
func unpackBufferOffset(offset uint64) unsafe.Pointer {
	return unsafe.Pointer(uintptr(offset))
}

func (c *CopyBufferToTextureCommand) Execute(ctx *gl.Context) {
	width := int32(c.copySize[0])
	height := int32(c.copySize[1])
	if width == 0 || height == 0 {
		return
	}

	desc, err := describeTextureFormat(c.dstTex.format)
	if err != nil {
		// Unreachable for a texture CreateTexture made: it refuses the format.
		hal.Logger().Error("gles: CopyBufferToTexture into a texture of an unsupported format", "err", err)
		return
	}
	format, dataType := desc.external, desc.dataType

	// Bind source buffer as pixel unpack buffer (PBO).
	ctx.BindBuffer(gl.PIXEL_UNPACK_BUFFER, c.srcBuffer.id)

	// Bind destination texture.
	ctx.BindTexture(c.dstTex.target, c.dstTex.id)

	if c.dstTex.format.IsCompressed() {
		cc := compressedCopy{
			tex: c.dstTex, mipLevel: c.mipLevel, origin: c.origin, size: c.copySize,
			bytesPerRow: c.bytesPerRow, rowsPerImage: c.rowsPerImage,
		}
		cc.upload(ctx, desc.internal, func(offset uint64) unsafe.Pointer {
			return unpackBufferOffset(c.bufOffset + offset)
		})
		ctx.BindBuffer(gl.PIXEL_UNPACK_BUFFER, 0)
		ctx.BindTexture(c.dstTex.target, 0)
		return
	}

	// Set pixel alignment to 1 for formats whose rows may not be 4-byte aligned.
	ctx.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	// Source rows are bytesPerRow apart (a multiple of 256 for multi-row
	// copies), so tell GL the row length in texels, as Rust wgpu-hal does
	// (gles/queue.rs CopyBufferToTexture: row_texels = bytes_per_row / block
	// size). Leaving it at 0 read padded rows as tightly packed; the texel
	// size covers depth and stencil transfers as well as color.
	var rowTexels int32
	if bpp := uploadTexelBytes(format, dataType); bpp != 0 && c.bytesPerRow != 0 {
		rowTexels = int32(uint64(c.bytesPerRow) / bpp)
	}
	ctx.PixelStorei(gl.UNPACK_ROW_LENGTH, rowTexels)

	if isLayeredTarget(c.dstTex.target) {
		// Layers are rowsPerImage rows apart in the buffer; the z offset is
		// the depth slice of a 3D texture or the array layer (Rust
		// gles/queue.rs: UNPACK_IMAGE_HEIGHT = rows_per_image,
		// tex_sub_image_3d at get_z_offset). When a PBO is bound, the pixels
		// pointer is a byte offset into it.
		ctx.PixelStorei(gl.UNPACK_IMAGE_HEIGHT, int32(c.rowsPerImage))
		ctx.TexSubImage3D(c.dstTex.target, int32(c.mipLevel),
			int32(c.origin[0]), int32(c.origin[1]), int32(c.origin[2]),
			width, height, int32(max(c.copySize[2], 1)),
			format, dataType, unpackBufferOffset(c.bufOffset))
		ctx.PixelStorei(gl.UNPACK_IMAGE_HEIGHT, 0)
	} else {
		c.uploadLayers(ctx, format, dataType)
	}

	// Restore defaults.
	ctx.PixelStorei(gl.UNPACK_ALIGNMENT, 4)
	ctx.PixelStorei(gl.UNPACK_ROW_LENGTH, 0)
	ctx.BindBuffer(gl.PIXEL_UNPACK_BUFFER, 0)
	ctx.BindTexture(c.dstTex.target, 0)
}

// uploadLayers uploads each layer of the copy into a 2D texture or a cube
// map face, layer z starting z*bytesPerRow*rowsPerImage bytes after the
// copy's offset in the bound unpack buffer (Rust gles/queue.rs
// CopyBufferToTexture: tex_sub_image_2d at get_2d_target(array_layer), one
// layer per region). TexSubImage2D at TEXTURE_CUBE_MAP itself is an invalid
// enum: no face was written.
func (c *CopyBufferToTextureCommand) uploadLayers(ctx *gl.Context, format, dataType uint32) {
	imageBytes := uint64(c.bytesPerRow) * uint64(max(c.rowsPerImage, c.copySize[1]))
	for z := uint32(0); z < max(c.copySize[2], 1); z++ {
		offset := c.bufOffset + uint64(z)*imageBytes
		ctx.TexSubImage2D(get2DTarget(c.dstTex.target, c.origin[2]+z), int32(c.mipLevel),
			int32(c.origin[0]), int32(c.origin[1]),
			int32(c.copySize[0]), int32(c.copySize[1]),
			format, dataType, unpackBufferOffset(offset))
	}
}

// CopyTextureToTextureCommand copies pixels between textures using an FBO.
// Attaches the source texture to GL_READ_FRAMEBUFFER, then copies into the
// destination via glCopyTexSubImage2D.
// Matches Rust wgpu-hal/src/gles/queue.rs CopyTextureToTexture.
type CopyTextureToTextureCommand struct {
	srcTex    *Texture
	dstTex    *Texture
	srcOrigin [3]uint32 // x, y, z
	dstOrigin [3]uint32 // x, y, z
	copySize  [3]uint32 // width, height, depthOrArrayLayers
	srcMip    uint32
	dstMip    uint32
}

func (c *CopyTextureToTextureCommand) Execute(ctx *gl.Context) {
	width := int32(c.copySize[0])
	height := int32(c.copySize[1])
	if width == 0 || height == 0 {
		return
	}

	// Save current FBO binding.
	var prevFBO int32
	ctx.GetIntegerv(gl.FRAMEBUFFER_BINDING, &prevFBO)

	// Create a temporary FBO for reading from the source texture.
	readFBO := ctx.GenFramebuffers(1)
	ctx.BindFramebuffer(gl.READ_FRAMEBUFFER, readFBO)
	ctx.FramebufferTexture2D(gl.READ_FRAMEBUFFER, gl.COLOR_ATTACHMENT0,
		c.srcTex.target, c.srcTex.id, int32(c.srcMip))

	// Bind destination texture and copy pixels from the read framebuffer.
	ctx.BindTexture(c.dstTex.target, c.dstTex.id)
	ctx.CopyTexSubImage2D(c.dstTex.target, int32(c.dstMip),
		int32(c.dstOrigin[0]), int32(c.dstOrigin[1]),
		int32(c.srcOrigin[0]), int32(c.srcOrigin[1]),
		width, height)
	ctx.BindTexture(c.dstTex.target, 0)

	// Clean up temporary FBO.
	ctx.DeleteFramebuffers(readFBO)
	ctx.BindFramebuffer(gl.FRAMEBUFFER, uint32(prevFBO))
}

// ResolveQuerySetCommand reads query results via glGetQueryObjectui64v and
// writes them into the destination buffer via glBufferSubData.
// Uses the CPU fallback path (no QUERY_BUFFER) to stay compatible with
// GLES 3.0+ / GL 3.3+. Each result is a uint64 (8 bytes).
// Matches Rust wgpu-hal/src/gles/queue.rs CopyQueryResults (fallback path).
type ResolveQuerySetCommand struct {
	querySet   *QuerySet
	firstQuery uint32
	queryCount uint32
	dstBuffer  *Buffer
	dstOffset  uint64
}

func (c *ResolveQuerySetCommand) Execute(ctx *gl.Context) {
	if c.querySet == nil || len(c.querySet.queries) == 0 {
		return
	}

	// Read query results into a temporary slice.
	results := make([]uint64, c.queryCount)
	for i := uint32(0); i < c.queryCount; i++ {
		idx := c.firstQuery + i
		if int(idx) >= len(c.querySet.queries) {
			break
		}
		ctx.GetQueryObjectui64v(c.querySet.queries[idx], gl.QUERY_RESULT, &results[i])
	}

	// Write results into the destination GL buffer via glBufferSubData.
	if c.dstBuffer.id != 0 {
		queryData := unsafe.Slice((*byte)(unsafe.Pointer(&results[0])), len(results)*8)
		ctx.BindBuffer(c.dstBuffer.target, c.dstBuffer.id)
		ctx.BufferSubData(c.dstBuffer.target, int(c.dstOffset), len(queryData),
			unsafe.Pointer(&queryData[0]))
		ctx.BindBuffer(c.dstBuffer.target, 0)
	}
}

// TimestampQueryCommand records a timestamp via glQueryCounter.
// Used internally by render/compute pass timestamp writes.
// Matches Rust wgpu-hal/src/gles/queue.rs TimestampQuery command.
type TimestampQueryCommand struct {
	query uint32 // GL query object ID
}

func (c *TimestampQueryCommand) Execute(ctx *gl.Context) {
	ctx.QueryCounter(c.query, gl.TIMESTAMP)
}

// vertexFormatToGL converts a WebGPU vertex format to GL component count and type.
func vertexFormatToGL(format gputypes.VertexFormat) (size int32, typ uint32, normalized bool) {
	switch format {
	case gputypes.VertexFormatFloat32:
		return 1, gl.FLOAT, false
	case gputypes.VertexFormatFloat32x2:
		return 2, gl.FLOAT, false
	case gputypes.VertexFormatFloat32x3:
		return 3, gl.FLOAT, false
	case gputypes.VertexFormatFloat32x4:
		return 4, gl.FLOAT, false
	case gputypes.VertexFormatUint8x2:
		return 2, gl.UNSIGNED_BYTE, false
	case gputypes.VertexFormatUint8x4:
		return 4, gl.UNSIGNED_BYTE, false
	case gputypes.VertexFormatUnorm8x2:
		return 2, gl.UNSIGNED_BYTE, true
	case gputypes.VertexFormatUnorm8x4:
		return 4, gl.UNSIGNED_BYTE, true
	case gputypes.VertexFormatSint8x2:
		return 2, gl.BYTE, false
	case gputypes.VertexFormatSint8x4:
		return 4, gl.BYTE, false
	case gputypes.VertexFormatSnorm8x2:
		return 2, gl.BYTE, true
	case gputypes.VertexFormatSnorm8x4:
		return 4, gl.BYTE, true
	case gputypes.VertexFormatUint16x2:
		return 2, gl.UNSIGNED_SHORT, false
	case gputypes.VertexFormatUint16x4:
		return 4, gl.UNSIGNED_SHORT, false
	case gputypes.VertexFormatUnorm16x2:
		return 2, gl.UNSIGNED_SHORT, true
	case gputypes.VertexFormatUnorm16x4:
		return 4, gl.UNSIGNED_SHORT, true
	case gputypes.VertexFormatSint16x2:
		return 2, gl.SHORT, false
	case gputypes.VertexFormatSint16x4:
		return 4, gl.SHORT, false
	case gputypes.VertexFormatSnorm16x2:
		return 2, gl.SHORT, true
	case gputypes.VertexFormatSnorm16x4:
		return 4, gl.SHORT, true
	case gputypes.VertexFormatUint32:
		return 1, gl.UNSIGNED_INT, false
	case gputypes.VertexFormatUint32x2:
		return 2, gl.UNSIGNED_INT, false
	case gputypes.VertexFormatUint32x3:
		return 3, gl.UNSIGNED_INT, false
	case gputypes.VertexFormatUint32x4:
		return 4, gl.UNSIGNED_INT, false
	case gputypes.VertexFormatSint32:
		return 1, gl.INT, false
	case gputypes.VertexFormatSint32x2:
		return 2, gl.INT, false
	case gputypes.VertexFormatSint32x3:
		return 3, gl.INT, false
	case gputypes.VertexFormatSint32x4:
		return 4, gl.INT, false
	default:
		return 4, gl.FLOAT, false
	}
}

// stencilOpToGL converts a HAL stencil operation to the corresponding GL constant.
func stencilOpToGL(op gputypes.StencilOperation) uint32 {
	switch op {
	case hal.StencilOperationKeep:
		return gl.KEEP
	case hal.StencilOperationZero:
		return gl.ZERO
	case hal.StencilOperationReplace:
		return gl.REPLACE
	case hal.StencilOperationInvert:
		return gl.INVERT
	case hal.StencilOperationIncrementClamp:
		return gl.INCR
	case hal.StencilOperationDecrementClamp:
		return gl.DECR
	case hal.StencilOperationIncrementWrap:
		return gl.INCR_WRAP
	case hal.StencilOperationDecrementWrap:
		return gl.DECR_WRAP
	default:
		return gl.KEEP
	}
}

// compareFunctionToGL converts compare function to GL constant.
func compareFunctionToGL(fn gputypes.CompareFunction) uint32 {
	switch fn {
	case gputypes.CompareFunctionNever:
		return gl.NEVER
	case gputypes.CompareFunctionLess:
		return gl.LESS
	case gputypes.CompareFunctionEqual:
		return gl.EQUAL
	case gputypes.CompareFunctionLessEqual:
		return gl.LEQUAL
	case gputypes.CompareFunctionGreater:
		return gl.GREATER
	case gputypes.CompareFunctionNotEqual:
		return gl.NOTEQUAL
	case gputypes.CompareFunctionGreaterEqual:
		return gl.GEQUAL
	case gputypes.CompareFunctionAlways:
		return gl.ALWAYS
	default:
		return gl.ALWAYS
	}
}

// blendFactorToGL converts a WebGPU blend factor to the corresponding GL constant.
func blendFactorToGL(f gputypes.BlendFactor) uint32 {
	switch f {
	case gputypes.BlendFactorZero:
		return gl.ZERO
	case gputypes.BlendFactorOne:
		return gl.ONE
	case gputypes.BlendFactorSrc:
		return gl.SRC_COLOR
	case gputypes.BlendFactorOneMinusSrc:
		return gl.ONE_MINUS_SRC_COLOR
	case gputypes.BlendFactorSrcAlpha:
		return gl.SRC_ALPHA
	case gputypes.BlendFactorOneMinusSrcAlpha:
		return gl.ONE_MINUS_SRC_ALPHA
	case gputypes.BlendFactorDst:
		return gl.DST_COLOR
	case gputypes.BlendFactorOneMinusDst:
		return gl.ONE_MINUS_DST_COLOR
	case gputypes.BlendFactorDstAlpha:
		return gl.DST_ALPHA
	case gputypes.BlendFactorOneMinusDstAlpha:
		return gl.ONE_MINUS_DST_ALPHA
	case gputypes.BlendFactorSrcAlphaSaturated:
		return gl.SRC_ALPHA_SATURATE
	case gputypes.BlendFactorConstant:
		return gl.CONSTANT_COLOR
	case gputypes.BlendFactorOneMinusConstant:
		return gl.ONE_MINUS_CONSTANT_COLOR
	default:
		return gl.ONE
	}
}

// blendOperationToGL converts a WebGPU blend operation to the corresponding GL constant.
func blendOperationToGL(op gputypes.BlendOperation) uint32 {
	switch op {
	case gputypes.BlendOperationAdd:
		return gl.FUNC_ADD
	case gputypes.BlendOperationSubtract:
		return gl.FUNC_SUBTRACT
	case gputypes.BlendOperationReverseSubtract:
		return gl.FUNC_REVERSE_SUBTRACT
	case gputypes.BlendOperationMin:
		return gl.MIN
	case gputypes.BlendOperationMax:
		return gl.MAX
	default:
		return gl.FUNC_ADD
	}
}

// primitiveTopologyToGL converts a WebGPU primitive topology to the corresponding GL constant.
// Matches Rust wgpu-hal/src/gles/conv.rs map_primitive_topology.
func primitiveTopologyToGL(topology gputypes.PrimitiveTopology) uint32 {
	switch topology {
	case gputypes.PrimitiveTopologyPointList:
		return gl.POINTS
	case gputypes.PrimitiveTopologyLineList:
		return gl.LINES
	case gputypes.PrimitiveTopologyLineStrip:
		return gl.LINE_STRIP
	case gputypes.PrimitiveTopologyTriangleList:
		return gl.TRIANGLES
	case gputypes.PrimitiveTopologyTriangleStrip:
		return gl.TRIANGLE_STRIP
	default:
		return gl.TRIANGLES
	}
}
