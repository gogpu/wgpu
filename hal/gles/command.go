// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows || linux

package gles

import (
	"context"
	"log/slog"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

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
}

// BeginEncoding begins command recording.
func (e *CommandEncoder) BeginEncoding(label string) error {
	e.label = label
	e.commands = nil
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

// TransitionBuffers transitions buffer states.
func (e *CommandEncoder) TransitionBuffers(_ []hal.BufferBarrier) {
	// No-op for OpenGL - no explicit barriers needed
}

// TransitionTextures transitions texture states.
func (e *CommandEncoder) TransitionTextures(_ []hal.TextureBarrier) {
	// No-op for OpenGL - no explicit barriers needed
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

// CopyBufferToTexture copies buffer data to a texture.
// Note: Requires glTexSubImage2D with pixel unpack buffer binding.
// Currently a no-op stub - texture uploads should use Queue.WriteTexture.
func (e *CommandEncoder) CopyBufferToTexture(src hal.Buffer, dst hal.Texture, regions []hal.BufferTextureCopy) {
	_ = src
	_ = dst
	_ = regions
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
			glCtx:       e.glCtx,
			srcTexture:  srcTex,
			dstBuffer:   dstBuf,
			srcOrigin:   [3]uint32{region.TextureBase.Origin.X, region.TextureBase.Origin.Y, region.TextureBase.Origin.Z},
			copySize:    [3]uint32{region.Size.Width, region.Size.Height, region.Size.DepthOrArrayLayers},
			dstOffset:   region.BufferLayout.Offset,
			bytesPerRow: region.BufferLayout.BytesPerRow,
		})
	}
}

// CopyTextureToTexture copies between textures.
// Note: Requires glCopyImageSubData (GL 4.3+ / GLES 3.2+).
// For older GL versions, requires framebuffer blit workaround.
func (e *CommandEncoder) CopyTextureToTexture(src, dst hal.Texture, regions []hal.TextureCopy) {
	_ = src
	_ = dst
	_ = regions
}

// ResolveQuerySet copies query results from a query set into a destination buffer.
// TODO: implement using GL_EXT_disjoint_timer_query when query sets are supported.
func (e *CommandEncoder) ResolveQuerySet(_ hal.QuerySet, _, _ uint32, _ hal.Buffer, _ uint64) {
	// Stub: GLES timestamp query implementation pending.
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
	}

	// Record clear commands
	for i, ca := range desc.ColorAttachments {
		if ca.LoadOp == gputypes.LoadOpClear {
			clearColor := ca.ClearValue
			e.commands = append(e.commands, &ClearColorCommand{
				attachment: i,
				r:          float32(clearColor.R),
				g:          float32(clearColor.G),
				b:          float32(clearColor.B),
				a:          float32(clearColor.A),
			})
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

	return rpe
}

// setupColorAttachment configures framebuffer, viewport, and MSAA resolve for the
// primary color attachment of a render pass.
func (e *CommandEncoder) setupColorAttachment(desc *hal.RenderPassDescriptor, rpe *RenderPassEncoder) {
	ca := desc.ColorAttachments[0]
	tv, ok := ca.View.(*TextureView)
	if !ok {
		return
	}

	if tv.isSurface {
		e.setupSurfaceTarget(tv, rpe)
		return
	}

	if tv.texture == nil {
		return
	}

	e.setupOffscreenTarget(desc, ca, tv, rpe)
}

// setupSurfaceTarget binds the Surface's swapchain offscreen framebuffer and
// sets viewport to surface dimensions. The swapchain FBO is a persistent GL
// framebuffer owned by the Surface (allocated in Surface.Configure). User
// render passes target this FBO — never the default framebuffer (FBO 0) —
// because the scene is intentionally rendered upside-down via naga's in-shader
// Y-flip (WriterFlagAdjustCoordinateSpace). Queue.Present performs an explicit
// Y-flipping glBlitFramebuffer from this FBO to FBO 0 before SwapBuffers.
// Mirrors Rust wgpu-hal/src/gles/egl.rs Surface::configure/Surface::present.
func (e *CommandEncoder) setupSurfaceTarget(tv *TextureView, rpe *RenderPassEncoder) {
	if tv.surfaceTex != nil && tv.surfaceTex.surface != nil {
		surf := tv.surfaceTex.surface
		e.commands = append(e.commands, &BindSurfaceFramebufferCommand{surface: surf})
		if surf.config != nil {
			cfg := surf.config
			rpe.fbHeight = cfg.Height
			e.commands = append(e.commands, &SetViewportCommand{
				width:  float32(cfg.Width),
				height: float32(cfg.Height),
			})
		}
		return
	}
	// Fallback: no surface texture (shouldn't normally happen for isSurface=true
	// views, but preserves prior behavior for tests / degenerate cases).
	e.commands = append(e.commands, &BindFramebufferCommand{fbo: 0})
}

// setupOffscreenTarget configures an offscreen FBO, depth/stencil attachment, and MSAA resolve.
func (e *CommandEncoder) setupOffscreenTarget(
	desc *hal.RenderPassDescriptor,
	ca hal.RenderPassColorAttachment,
	tv *TextureView,
	rpe *RenderPassEncoder,
) {
	e.commands = append(e.commands, &EnsureOffscreenFBOCommand{texture: tv.texture})

	// Attach depth/stencil texture to the FBO if provided.
	if desc.DepthStencilAttachment != nil {
		if dsView, ok := desc.DepthStencilAttachment.View.(*TextureView); ok && dsView.texture != nil {
			e.commands = append(e.commands, &AttachDepthStencilCommand{
				colorTexture: tv.texture,
				depthTexture: dsView.texture,
			})
		}
	}

	rpe.fbHeight = tv.texture.size.Height
	e.commands = append(e.commands, &SetViewportCommand{
		width:  float32(tv.texture.size.Width),
		height: float32(tv.texture.size.Height),
	})

	// Record MSAA resolve target if present.
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

// BeginComputePass begins a compute pass.
func (e *CommandEncoder) BeginComputePass(_ *hal.ComputePassDescriptor) hal.ComputePassEncoder {
	return &ComputePassEncoder{
		encoder: e,
	}
}

// RenderPassEncoder implements hal.RenderPassEncoder for OpenGL.
type RenderPassEncoder struct {
	encoder       *CommandEncoder
	desc          *hal.RenderPassDescriptor
	pipeline      *RenderPipeline
	vertexBuffers []*Buffer
	indexBuffer   *Buffer
	indexFormat   gputypes.IndexFormat
	stencilRef    uint32
	fbHeight      uint32 // Framebuffer height for MSAA resolve blit Y-flip

	// MSAA resolve state: set during BeginRenderPass when ResolveTarget is present.
	msaaTexture      *Texture // The MSAA color texture (source for resolve)
	resolveTexture   *Texture // The single-sample resolve target (nil when resolveToSurface)
	resolveToSurface bool     // True when resolve target is the default framebuffer (FBO 0)
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
	if e.msaaTexture != nil {
		e.emitMSAAResolve()
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
	e.pipeline = p
	e.encoder.commands = append(e.encoder.commands,
		&UseProgramCommand{programID: p.programID},
		&SetPipelineStateCommand{
			topology:       p.primitiveTopology,
			cullMode:       p.cullMode,
			frontFace:      p.frontFace,
			depthStencil:   p.depthStencil,
			blend:          p.blend,
			colorWriteMask: p.colorWriteMask,
			stencilRef:     e.stencilRef,
		},
	)
}

// SetBindGroup sets a bind group.
func (e *RenderPassEncoder) SetBindGroup(index uint32, group hal.BindGroup, offsets []uint32) {
	bg, ok := group.(*BindGroup)
	if !ok {
		return
	}
	var samplerMap *[maxTextureSlots]int8
	var groupInfos []BindGroupLayoutInfo
	if e.pipeline != nil {
		samplerMap = &e.pipeline.samplerBindMap
		if e.pipeline.layout != nil {
			groupInfos = e.pipeline.layout.groupInfos
		}
	}
	e.encoder.commands = append(e.encoder.commands, &SetBindGroupCommand{
		index:           index,
		group:           bg,
		dynamicOffsets:  offsets,
		maxTextureUnits: e.encoder.maxTextureUnits,
		groupInfos:      groupInfos,
		samplerBindMap:  samplerMap,
	})
}

// SetVertexBuffer sets a vertex buffer and configures vertex attributes.
// In OpenGL, vertex attribute configuration (glVertexAttribPointer +
// glEnableVertexAttribArray) must be done explicitly. The layout is taken
// from the currently bound render pipeline's vertex buffer descriptors.
func (e *RenderPassEncoder) SetVertexBuffer(slot uint32, buffer hal.Buffer, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok {
		return
	}

	// Grow slice if needed
	for len(e.vertexBuffers) <= int(slot) {
		e.vertexBuffers = append(e.vertexBuffers, nil)
	}
	e.vertexBuffers[slot] = buf

	// Get vertex layout from the current pipeline for this slot.
	var layout *gputypes.VertexBufferLayout
	if e.pipeline != nil && int(slot) < len(e.pipeline.vertexBuffers) {
		layout = &e.pipeline.vertexBuffers[slot]
	}

	e.encoder.commands = append(e.encoder.commands, &SetVertexBufferCommand{
		slot:   slot,
		buffer: buf,
		offset: offset,
		layout: layout,
	})
}

// SetIndexBuffer sets the index buffer.
func (e *RenderPassEncoder) SetIndexBuffer(buffer hal.Buffer, format gputypes.IndexFormat, offset uint64) {
	buf, ok := buffer.(*Buffer)
	if !ok {
		return
	}
	e.indexBuffer = buf
	e.indexFormat = format

	e.encoder.commands = append(e.encoder.commands, &SetIndexBufferCommand{
		buffer: buf,
		format: format,
		offset: offset,
	})
}

// SetViewport sets the viewport.
func (e *RenderPassEncoder) SetViewport(x, y, width, height, minDepth, maxDepth float32) {
	e.encoder.commands = append(e.encoder.commands, &SetViewportCommand{
		x: x, y: y, width: width, height: height,
		minDepth: minDepth, maxDepth: maxDepth,
	})
}

// SetScissorRect sets the scissor rectangle.
// With ADJUST_COORDINATE_SPACE, no Y-flip is needed — coordinates pass through directly.
func (e *RenderPassEncoder) SetScissorRect(x, y, width, height uint32) {
	e.encoder.commands = append(e.encoder.commands, &SetScissorCommand{
		x: x, y: y, width: width, height: height,
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

// Draw draws primitives.
func (e *RenderPassEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	e.encoder.commands = append(e.encoder.commands, &DrawCommand{
		vertexCount:   vertexCount,
		instanceCount: instanceCount,
		firstVertex:   firstVertex,
		firstInstance: firstInstance,
	})
}

// DrawIndexed draws indexed primitives.
func (e *RenderPassEncoder) DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	e.encoder.commands = append(e.encoder.commands, &DrawIndexedCommand{
		indexCount:    indexCount,
		instanceCount: instanceCount,
		firstIndex:    firstIndex,
		baseVertex:    baseVertex,
		firstInstance: firstInstance,
		indexFormat:   e.indexFormat,
	})
}

// DrawIndirect draws primitives with GPU-generated parameters.
// Note: Requires GL_ARB_draw_indirect (GL 4.0+ / GLES 3.1+).
// Currently not implemented - use direct Draw calls instead.
func (e *RenderPassEncoder) DrawIndirect(buffer hal.Buffer, offset uint64) {
	_ = buffer
	_ = offset
}

// DrawIndexedIndirect draws indexed primitives with GPU-generated parameters.
// Note: Requires GL_ARB_draw_indirect (GL 4.0+ / GLES 3.1+).
// Currently not implemented - use direct DrawIndexed calls instead.
func (e *RenderPassEncoder) DrawIndexedIndirect(buffer hal.Buffer, offset uint64) {
	_ = buffer
	_ = offset
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
}

// End finishes the compute pass.
func (e *ComputePassEncoder) End() {}

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
	e.encoder.commands = append(e.encoder.commands, &SetBindGroupCommand{
		index:           index,
		group:           bg,
		dynamicOffsets:  offsets,
		maxTextureUnits: e.encoder.maxTextureUnits,
		groupInfos:      groupInfos,
	})
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
	surface *Surface
}

func (c *BindSurfaceFramebufferCommand) Execute(ctx *gl.Context) {
	if c.surface == nil {
		ctx.BindFramebuffer(gl.FRAMEBUFFER, 0)
		return
	}
	ctx.BindFramebuffer(gl.FRAMEBUFFER, c.surface.swapchainFBO)
}

// EnsureOffscreenFBOCommand lazily creates a framebuffer object for an offscreen
// texture and binds it. If the texture already has an FBO, it simply binds it.
type EnsureOffscreenFBOCommand struct {
	texture *Texture
}

func (c *EnsureOffscreenFBOCommand) Execute(ctx *gl.Context) {
	if c.texture.fbo == 0 {
		// Create FBO.
		fbo := ctx.GenFramebuffers(1)
		ctx.BindFramebuffer(gl.FRAMEBUFFER, fbo)
		// Attach the color texture as COLOR_ATTACHMENT0.
		// Use the texture's actual target (GL_TEXTURE_2D or GL_TEXTURE_2D_MULTISAMPLE).
		ctx.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, c.texture.target, c.texture.id, 0)
		// Verify completeness.
		status := ctx.CheckFramebufferStatus(gl.FRAMEBUFFER)
		if status != gl.FRAMEBUFFER_COMPLETE {
			// FBO incomplete — delete and fall back to default framebuffer.
			ctx.DeleteFramebuffers(fbo)
			ctx.BindFramebuffer(gl.FRAMEBUFFER, 0)
			return
		}
		c.texture.fbo = fbo
	} else {
		ctx.BindFramebuffer(gl.FRAMEBUFFER, c.texture.fbo)
	}
}

// AttachDepthStencilCommand attaches a depth/stencil texture to the currently
// bound FBO (the one associated with the color texture). This must be recorded
// after EnsureOffscreenFBOCommand so the FBO is already bound.
type AttachDepthStencilCommand struct {
	colorTexture *Texture // used to verify the FBO exists
	depthTexture *Texture
}

func (c *AttachDepthStencilCommand) Execute(ctx *gl.Context) {
	if c.colorTexture.fbo == 0 {
		return // No FBO was created; nothing to attach to.
	}
	// Attach the depth/stencil texture. Using DEPTH_STENCIL_ATTACHMENT covers
	// combined depth+stencil formats (e.g., Depth24PlusStencil8). For
	// depth-only formats the driver silently ignores the stencil part.
	// Use the texture's actual target (GL_TEXTURE_2D or GL_TEXTURE_2D_MULTISAMPLE).
	ctx.FramebufferTexture2D(gl.FRAMEBUFFER, gl.DEPTH_STENCIL_ATTACHMENT, c.depthTexture.target, c.depthTexture.id, 0)
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
func (c *MSAAResolveCommand) ensureResolveFBO(ctx *gl.Context) bool {
	if c.resolveTexture.fbo == 0 {
		fbo := ctx.GenFramebuffers(1)
		ctx.BindFramebuffer(gl.FRAMEBUFFER, fbo)
		ctx.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0,
			c.resolveTexture.target, c.resolveTexture.id, 0)
		if ctx.CheckFramebufferStatus(gl.FRAMEBUFFER) != gl.FRAMEBUFFER_COMPLETE {
			ctx.DeleteFramebuffers(fbo)
			return false
		}
		c.resolveTexture.fbo = fbo
	}
	ctx.BindFramebuffer(gl.DRAW_FRAMEBUFFER, c.resolveTexture.fbo)
	return true
}

// ClearColorCommand clears a color attachment.
type ClearColorCommand struct {
	attachment int
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
	topology       gputypes.PrimitiveTopology
	cullMode       gputypes.CullMode
	frontFace      gputypes.FrontFace
	depthStencil   *hal.DepthStencilState
	blend          *gputypes.BlendState
	colorWriteMask gputypes.ColorWriteMask
	stencilRef     uint32
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

	// Color write mask
	ctx.ColorMask(
		c.colorWriteMask&gputypes.ColorWriteMaskRed != 0,
		c.colorWriteMask&gputypes.ColorWriteMaskGreen != 0,
		c.colorWriteMask&gputypes.ColorWriteMaskBlue != 0,
		c.colorWriteMask&gputypes.ColorWriteMaskAlpha != 0,
	)

	// Blending
	if c.blend != nil {
		ctx.Enable(gl.BLEND)
		ctx.BlendFuncSeparate(
			blendFactorToGL(c.blend.Color.SrcFactor),
			blendFactorToGL(c.blend.Color.DstFactor),
			blendFactorToGL(c.blend.Alpha.SrcFactor),
			blendFactorToGL(c.blend.Alpha.DstFactor),
		)
		ctx.BlendEquationSeparate(
			blendOperationToGL(c.blend.Color.Operation),
			blendOperationToGL(c.blend.Alpha.Operation),
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
	// groupInfos stores per-group binding-to-slot tables from PipelineLayout.
	// Used to look up the GL slot for each binding instead of the old group*16+binding formula.
	groupInfos []BindGroupLayoutInfo
	// samplerBindMap maps texture unit → sampler unit for combined sampler2D.
	// Built from naga GLSL TextureMappings at pipeline creation (Rust SamplerBindMap pattern).
	// When non-nil, sampler is bound to the texture's unit instead of its own WGSL binding.
	samplerBindMap *[maxTextureSlots]int8
}

func (c *SetBindGroupCommand) Execute(ctx *gl.Context) {
	if c.group == nil {
		return
	}

	dynamicIdx := 0
	for _, entry := range c.group.entries {
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

			// Apply dynamic offset if available.
			if c.dynamicOffsets != nil && dynamicIdx < len(c.dynamicOffsets) {
				if c.group.layout != nil {
					for _, le := range c.group.layout.entries {
						if le.Binding == entry.Binding && le.Buffer != nil && le.Buffer.HasDynamicOffset {
							offset += int(c.dynamicOffsets[dynamicIdx])
							dynamicIdx++
							break
						}
					}
				}
			}

			if size > 0 {
				ctx.BindBufferRange(gl.UNIFORM_BUFFER, glBinding, bufID, offset, size)
			} else {
				ctx.BindBufferBase(gl.UNIFORM_BUFFER, glBinding, bufID)
			}

		case gputypes.TextureViewBinding:
			// TextureView handle is the GL texture object ID (from NativeHandle()).
			texID := uint32(res.TextureView)
			if texID == 0 {
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
			ctx.ActiveTexture(gl.TEXTURE0 + glBinding)
			ctx.BindTexture(gl.TEXTURE_2D, texID)

		case gputypes.SamplerBinding:
			// Sampler handle is the GL sampler object ID (from NativeHandle()).
			samplerID := uint32(res.Sampler)
			if samplerID == 0 {
				continue
			}
			// Determine the correct texture unit for this sampler.
			// Naga GLSL generates combined sampler2D on the texture's binding,
			// so the sampler must be bound to the texture's unit — NOT the
			// sampler's own WGSL binding. SamplerBindMap provides this mapping.
			// Matches Rust wgpu-hal GLES SamplerBindMap (command.rs:247).
			bindUnit := c.resolveSamplerUnit(glBinding)
			if hal.Logger().Enabled(context.Background(), slog.LevelDebug) {
				hal.Logger().Debug("gles: binding sampler",
					"samplerBinding", glBinding,
					"textureUnit", bindUnit,
					"samplerID", samplerID,
				)
			}
			ctx.BindSampler(bindUnit, samplerID)
		}
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

// resolveSamplerUnit finds the texture unit for a sampler via SamplerBindMap.
func (c *SetBindGroupCommand) resolveSamplerUnit(glBinding uint32) uint32 {
	if c.samplerBindMap == nil {
		return glBinding
	}
	for texUnit := range c.samplerBindMap {
		if c.samplerBindMap[texUnit] == int8(glBinding) {
			return uint32(texUnit)
		}
	}
	return glBinding
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
	for _, attr := range c.layout.Attributes {
		loc := attr.ShaderLocation
		size, typ, normalized := vertexFormatToGL(attr.Format)
		attrOffset := uintptr(c.offset) + uintptr(attr.Offset)
		ctx.EnableVertexAttribArray(loc)
		ctx.VertexAttribPointer(loc, size, typ, normalized, stride, attrOffset)
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

// DrawCommand executes a non-indexed draw.
type DrawCommand struct {
	vertexCount, instanceCount uint32
	firstVertex, firstInstance uint32
}

func (c *DrawCommand) Execute(ctx *gl.Context) {
	if c.instanceCount <= 1 {
		ctx.DrawArrays(gl.TRIANGLES, int32(c.firstVertex), int32(c.vertexCount))
	} else {
		ctx.DrawArraysInstanced(gl.TRIANGLES, int32(c.firstVertex), int32(c.vertexCount), int32(c.instanceCount))
	}
}

// DrawIndexedCommand executes an indexed draw.
type DrawIndexedCommand struct {
	indexCount, instanceCount uint32
	firstIndex                uint32
	baseVertex                int32
	firstInstance             uint32
	indexFormat               gputypes.IndexFormat
}

func (c *DrawIndexedCommand) Execute(ctx *gl.Context) {
	indexType := uint32(gl.UNSIGNED_SHORT)
	indexSize := uintptr(2)
	if c.indexFormat == gputypes.IndexFormatUint32 {
		indexType = gl.UNSIGNED_INT
		indexSize = 4
	}

	offset := uintptr(c.firstIndex) * indexSize

	if c.instanceCount <= 1 {
		ctx.DrawElements(gl.TRIANGLES, int32(c.indexCount), indexType, offset)
	} else {
		ctx.DrawElementsInstanced(gl.TRIANGLES, int32(c.indexCount), indexType, offset, int32(c.instanceCount))
	}
}

// CopyBufferCommand copies between buffers.
type CopyBufferCommand struct {
	srcID, dstID         uint32
	srcOffset, dstOffset uint64
	size                 uint64
}

func (c *CopyBufferCommand) Execute(ctx *gl.Context) {
	ctx.BindBuffer(gl.COPY_READ_BUFFER, c.srcID)
	ctx.BindBuffer(gl.COPY_WRITE_BUFFER, c.dstID)
	// glCopyBufferSubData would go here
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
	// Insert barrier for storage buffer coherency after compute dispatch.
	// This ensures subsequent reads/writes see the compute shader results.
	ctx.MemoryBarrier(gl.SHADER_STORAGE_BARRIER_BIT | gl.BUFFER_UPDATE_BARRIER_BIT)
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
	// Insert barrier for storage buffer coherency after compute dispatch
	ctx.MemoryBarrier(gl.SHADER_STORAGE_BARRIER_BIT | gl.BUFFER_UPDATE_BARRIER_BIT)
}

// CopyTextureToBufferCommand reads pixels from a texture's FBO into a buffer's
// CPU-side data slice. This is the standard GLES readback path since GLES lacks
// glGetTexImage. The approach: bind texture FBO -> glReadPixels -> copy to buffer.
type CopyTextureToBufferCommand struct {
	glCtx       *gl.Context
	srcTexture  *Texture
	dstBuffer   *Buffer
	srcOrigin   [3]uint32 // x, y, z
	copySize    [3]uint32 // width, height, depthOrArrayLayers
	dstOffset   uint64
	bytesPerRow uint32
}

// Execute reads pixels from the source texture's FBO into the destination buffer.
func (c *CopyTextureToBufferCommand) Execute(ctx *gl.Context) {
	width := int32(c.copySize[0])
	height := int32(c.copySize[1])
	if width == 0 || height == 0 {
		return
	}

	// Calculate byte sizes. Assume RGBA8 (4 bytes per pixel) for readback.
	bpp := uint32(4)
	rowBytes := uint32(width) * bpp
	totalBytes := uint64(rowBytes) * uint64(height)

	// Ensure destination buffer has enough CPU-side storage.
	requiredSize := c.dstOffset + totalBytes
	if uint64(len(c.dstBuffer.data)) < requiredSize {
		newData := make([]byte, requiredSize)
		copy(newData, c.dstBuffer.data)
		c.dstBuffer.data = newData
	}

	// Save the current FBO binding so we can restore it after the read.
	var prevFBO int32
	ctx.GetIntegerv(gl.FRAMEBUFFER_BINDING, &prevFBO)

	// Ensure the source texture has an FBO. Create one lazily if needed.
	if c.srcTexture.fbo == 0 {
		fbo := ctx.GenFramebuffers(1)
		ctx.BindFramebuffer(gl.FRAMEBUFFER, fbo)
		ctx.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, c.srcTexture.target, c.srcTexture.id, 0)
		status := ctx.CheckFramebufferStatus(gl.FRAMEBUFFER)
		if status != gl.FRAMEBUFFER_COMPLETE {
			ctx.DeleteFramebuffers(fbo)
			ctx.BindFramebuffer(gl.FRAMEBUFFER, uint32(prevFBO))
			return
		}
		c.srcTexture.fbo = fbo
	} else {
		ctx.BindFramebuffer(gl.FRAMEBUFFER, c.srcTexture.fbo)
	}

	// Set tight pixel packing (no row alignment padding).
	ctx.PixelStorei(gl.PACK_ALIGNMENT, 1)

	// Read pixels from the bound FBO into a temporary CPU buffer.
	tmpBuf := make([]byte, totalBytes)
	ctx.ReadPixels(
		int32(c.srcOrigin[0]), int32(c.srcOrigin[1]),
		width, height,
		gl.BGRA, gl.UNSIGNED_BYTE,
		unsafe.Pointer(&tmpBuf[0]),
	)

	// Copy the pixel data into the destination buffer's CPU-side storage.
	// OpenGL reads bottom-to-top, but callers expect top-to-bottom order.
	// Flip the rows during copy.
	for row := int32(0); row < height; row++ {
		// OpenGL row 0 = bottom. We want row 0 = top.
		srcRow := (height - 1 - row)
		srcStart := uint64(srcRow) * uint64(rowBytes)
		dstStart := c.dstOffset + uint64(row)*uint64(rowBytes)
		copy(c.dstBuffer.data[dstStart:dstStart+uint64(rowBytes)], tmpBuf[srcStart:srcStart+uint64(rowBytes)])
	}

	// Restore the previous FBO binding.
	ctx.BindFramebuffer(gl.FRAMEBUFFER, uint32(prevFBO))
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
func stencilOpToGL(op hal.StencilOperation) uint32 {
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
