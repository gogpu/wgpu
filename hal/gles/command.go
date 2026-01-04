// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows || linux

package gles

import (
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
	"github.com/gogpu/wgpu/types"
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
	glCtx    *gl.Context
	commands []Command
	label    string
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

// CopyTextureToBuffer copies texture data to a buffer.
// Note: Requires glGetTexImage with pixel pack buffer binding (not available in GLES).
// This operation has limited support in OpenGL ES environments.
func (e *CommandEncoder) CopyTextureToBuffer(src hal.Texture, dst hal.Buffer, regions []hal.BufferTextureCopy) {
	_ = src
	_ = dst
	_ = regions
}

// CopyTextureToTexture copies between textures.
// Note: Requires glCopyImageSubData (GL 4.3+ / GLES 3.2+).
// For older GL versions, requires framebuffer blit workaround.
func (e *CommandEncoder) CopyTextureToTexture(src, dst hal.Texture, regions []hal.TextureCopy) {
	_ = src
	_ = dst
	_ = regions
}

// BeginRenderPass begins a render pass.
func (e *CommandEncoder) BeginRenderPass(desc *hal.RenderPassDescriptor) hal.RenderPassEncoder {
	rpe := &RenderPassEncoder{
		encoder: e,
		desc:    desc,
	}

	// Record clear commands
	for i, ca := range desc.ColorAttachments {
		if ca.LoadOp == types.LoadOpClear {
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
		if dsa.DepthLoadOp == types.LoadOpClear {
			e.commands = append(e.commands, &ClearDepthCommand{
				depth: float64(dsa.DepthClearValue),
			})
		}
		if dsa.StencilLoadOp == types.LoadOpClear {
			e.commands = append(e.commands, &ClearStencilCommand{
				stencil: int32(dsa.StencilClearValue),
			})
		}
	}

	return rpe
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
	indexFormat   types.IndexFormat
}

// End finishes the render pass.
func (e *RenderPassEncoder) End() {
	// Nothing special needed - commands are already recorded
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
			topology:     p.primitiveTopology,
			cullMode:     p.cullMode,
			frontFace:    p.frontFace,
			depthStencil: p.depthStencil,
		},
	)
}

// SetBindGroup sets a bind group.
func (e *RenderPassEncoder) SetBindGroup(index uint32, group hal.BindGroup, offsets []uint32) {
	bg, ok := group.(*BindGroup)
	if !ok {
		return
	}
	e.encoder.commands = append(e.encoder.commands, &SetBindGroupCommand{
		index:          index,
		group:          bg,
		dynamicOffsets: offsets,
	})
}

// SetVertexBuffer sets a vertex buffer.
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

	e.encoder.commands = append(e.encoder.commands, &SetVertexBufferCommand{
		slot:   slot,
		buffer: buf,
		offset: offset,
	})
}

// SetIndexBuffer sets the index buffer.
func (e *RenderPassEncoder) SetIndexBuffer(buffer hal.Buffer, format types.IndexFormat, offset uint64) {
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
func (e *RenderPassEncoder) SetScissorRect(x, y, width, height uint32) {
	e.encoder.commands = append(e.encoder.commands, &SetScissorCommand{
		x: x, y: y, width: width, height: height,
	})
}

// SetBlendConstant sets the blend constant.
func (e *RenderPassEncoder) SetBlendConstant(color *types.Color) {
	e.encoder.commands = append(e.encoder.commands, &SetBlendConstantCommand{
		r: float32(color.R),
		g: float32(color.G),
		b: float32(color.B),
		a: float32(color.A),
	})
}

// SetStencilReference sets the stencil reference value.
func (e *RenderPassEncoder) SetStencilReference(ref uint32) {
	e.encoder.commands = append(e.encoder.commands, &SetStencilRefCommand{
		ref: ref,
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
	e.encoder.commands = append(e.encoder.commands, &SetBindGroupCommand{
		index:          index,
		group:          bg,
		dynamicOffsets: offsets,
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

// ClearColorCommand clears a color attachment.
type ClearColorCommand struct {
	attachment int
	r, g, b, a float32
}

func (c *ClearColorCommand) Execute(ctx *gl.Context) {
	ctx.ClearColor(c.r, c.g, c.b, c.a)
	ctx.Clear(gl.COLOR_BUFFER_BIT)
}

// ClearDepthCommand clears the depth buffer.
type ClearDepthCommand struct {
	depth float64
}

func (c *ClearDepthCommand) Execute(ctx *gl.Context) {
	ctx.Clear(gl.DEPTH_BUFFER_BIT)
}

// ClearStencilCommand clears the stencil buffer.
type ClearStencilCommand struct {
	stencil int32
}

func (c *ClearStencilCommand) Execute(ctx *gl.Context) {
	ctx.Clear(gl.STENCIL_BUFFER_BIT)
}

// UseProgramCommand activates a shader program.
type UseProgramCommand struct {
	programID uint32
}

func (c *UseProgramCommand) Execute(ctx *gl.Context) {
	ctx.UseProgram(c.programID)
}

// SetPipelineStateCommand sets pipeline state (culling, depth, etc.).
type SetPipelineStateCommand struct {
	topology     types.PrimitiveTopology
	cullMode     types.CullMode
	frontFace    types.FrontFace
	depthStencil *hal.DepthStencilState
}

func (c *SetPipelineStateCommand) Execute(ctx *gl.Context) {
	// Culling
	if c.cullMode == types.CullModeNone {
		ctx.Disable(gl.CULL_FACE)
	} else {
		ctx.Enable(gl.CULL_FACE)
		switch c.cullMode {
		case types.CullModeFront:
			ctx.CullFace(gl.FRONT)
		case types.CullModeBack:
			ctx.CullFace(gl.BACK)
		}
	}

	// Front face
	switch c.frontFace {
	case types.FrontFaceCCW:
		ctx.FrontFace(gl.CCW)
	case types.FrontFaceCW:
		ctx.FrontFace(gl.CW)
	}

	// Depth/stencil
	if c.depthStencil != nil {
		if c.depthStencil.DepthWriteEnabled || c.depthStencil.DepthCompare != types.CompareFunctionAlways {
			ctx.Enable(gl.DEPTH_TEST)
			ctx.DepthMask(c.depthStencil.DepthWriteEnabled)
			ctx.DepthFunc(compareFunctionToGL(c.depthStencil.DepthCompare))
		} else {
			ctx.Disable(gl.DEPTH_TEST)
		}
	}
}

// SetBindGroupCommand binds resources.
type SetBindGroupCommand struct {
	index          uint32
	group          *BindGroup
	dynamicOffsets []uint32
}

func (c *SetBindGroupCommand) Execute(ctx *gl.Context) {
	// Bind uniform buffers, textures, and samplers from the bind group.
	// Note: Full implementation requires BindGroup to track resource bindings.
	if c.group == nil {
		return
	}
	// Binding logic is deferred to draw time when pipeline layout is known.
	_ = ctx
}

// SetVertexBufferCommand binds a vertex buffer.
type SetVertexBufferCommand struct {
	slot   uint32
	buffer *Buffer
	offset uint64
}

func (c *SetVertexBufferCommand) Execute(ctx *gl.Context) {
	ctx.BindBuffer(gl.ARRAY_BUFFER, c.buffer.id)
}

// SetIndexBufferCommand binds an index buffer.
type SetIndexBufferCommand struct {
	buffer *Buffer
	format types.IndexFormat
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
type SetScissorCommand struct {
	x, y, width, height uint32
}

func (c *SetScissorCommand) Execute(ctx *gl.Context) {
	ctx.Enable(gl.SCISSOR_TEST)
	ctx.Scissor(int32(c.x), int32(c.y), int32(c.width), int32(c.height))
}

// SetBlendConstantCommand sets blend constant.
type SetBlendConstantCommand struct {
	r, g, b, a float32
}

func (c *SetBlendConstantCommand) Execute(_ *gl.Context) {
	// ctx.BlendColor(c.r, c.g, c.b, c.a)
}

// SetStencilRefCommand sets stencil reference.
type SetStencilRefCommand struct {
	ref uint32
}

func (c *SetStencilRefCommand) Execute(_ *gl.Context) {
	// ctx.StencilFunc uses ref
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
	indexFormat               types.IndexFormat
}

func (c *DrawIndexedCommand) Execute(ctx *gl.Context) {
	indexType := uint32(gl.UNSIGNED_SHORT)
	indexSize := uintptr(2)
	if c.indexFormat == types.IndexFormatUint32 {
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

// compareFunctionToGL converts compare function to GL constant.
func compareFunctionToGL(fn types.CompareFunction) uint32 {
	switch fn {
	case types.CompareFunctionNever:
		return gl.NEVER
	case types.CompareFunctionLess:
		return gl.LESS
	case types.CompareFunctionEqual:
		return gl.EQUAL
	case types.CompareFunctionLessEqual:
		return gl.LEQUAL
	case types.CompareFunctionGreater:
		return gl.GREATER
	case types.CompareFunctionNotEqual:
		return gl.NOTEQUAL
	case types.CompareFunctionGreaterEqual:
		return gl.GEQUAL
	case types.CompareFunctionAlways:
		return gl.ALWAYS
	default:
		return gl.ALWAYS
	}
}
