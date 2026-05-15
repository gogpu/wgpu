//go:build js && wasm

package browser

import "syscall/js"

// RenderPassEncoder wraps a browser GPURenderPassEncoder with pre-bound methods.
//
// Pre-binding JS methods at construction time avoids repeated property lookups
// on the hot path (draw calls). This matches the Ebiten pattern used by Device.
//
// Matches Rust wgpu WebRenderPassEncoder which holds webgpu_sys::GpuRenderPassEncoder.
type RenderPassEncoder struct {
	// ref_ is the GPURenderPassEncoder JavaScript object.
	ref_ js.Value

	// Pre-bound methods for draw calls and state setting.
	fnSetPipeline         js.Value
	fnSetBindGroup        js.Value
	fnSetVertexBuffer     js.Value
	fnSetIndexBuffer      js.Value
	fnDraw                js.Value
	fnDrawIndexed         js.Value
	fnDrawIndirect        js.Value
	fnDrawIndexedIndirect js.Value
	fnSetViewport         js.Value
	fnSetScissorRect      js.Value
	fnSetBlendConstant    js.Value
	fnSetStencilReference js.Value
	fnEnd                 js.Value
}

// NewRenderPassEncoder constructs a RenderPassEncoder from a GPURenderPassEncoder js.Value.
// Pre-binds all draw and state-setting methods.
func NewRenderPassEncoder(ref js.Value) *RenderPassEncoder {
	return &RenderPassEncoder{
		ref_:                  ref,
		fnSetPipeline:         bindMethod(ref, "setPipeline"),
		fnSetBindGroup:        bindMethod(ref, "setBindGroup"),
		fnSetVertexBuffer:     bindMethod(ref, "setVertexBuffer"),
		fnSetIndexBuffer:      bindMethod(ref, "setIndexBuffer"),
		fnDraw:                bindMethod(ref, "draw"),
		fnDrawIndexed:         bindMethod(ref, "drawIndexed"),
		fnDrawIndirect:        bindMethod(ref, "drawIndirect"),
		fnDrawIndexedIndirect: bindMethod(ref, "drawIndexedIndirect"),
		fnSetViewport:         bindMethod(ref, "setViewport"),
		fnSetScissorRect:      bindMethod(ref, "setScissorRect"),
		fnSetBlendConstant:    bindMethod(ref, "setBlendConstant"),
		fnSetStencilReference: bindMethod(ref, "setStencilReference"),
		fnEnd:                 bindMethod(ref, "end"),
	}
}

// SetPipeline sets the active render pipeline.
func (p *RenderPassEncoder) SetPipeline(pipeline js.Value) {
	p.fnSetPipeline.Invoke(pipeline)
}

// SetBindGroup sets a bind group at the given index.
//
// When dynamicOffsets is non-empty, the offsets are passed as a Uint32Array
// using the overload: setBindGroup(index, group, offsetsArray, 0, len).
// This matches Rust wgpu's set_bind_group_with_u32_slice_and_f64_and_dynamic_offsets_data_length.
func (p *RenderPassEncoder) SetBindGroup(index uint32, group js.Value, dynamicOffsets []uint32) {
	if len(dynamicOffsets) == 0 {
		p.fnSetBindGroup.Invoke(index, group)
		return
	}
	// Build a Uint32Array for the dynamic offsets.
	jsArray := js.Global().Get("Uint32Array").New(len(dynamicOffsets))
	for i, offset := range dynamicOffsets {
		jsArray.SetIndex(i, js.ValueOf(offset))
	}
	p.fnSetBindGroup.Invoke(index, group, jsArray, 0, len(dynamicOffsets))
}

// SetVertexBuffer sets a vertex buffer for the given slot.
//
// If size is 0, the size parameter is omitted (meaning "rest of buffer"),
// matching Rust wgpu's set_vertex_buffer_with_f64 (no size variant).
func (p *RenderPassEncoder) SetVertexBuffer(slot uint32, buffer js.Value, offset uint64, size uint64) {
	if size == 0 {
		// Omit size to use the rest of the buffer.
		p.fnSetVertexBuffer.Invoke(slot, buffer, float64(offset))
	} else {
		p.fnSetVertexBuffer.Invoke(slot, buffer, float64(offset), float64(size))
	}
}

// SetIndexBuffer sets the index buffer.
//
// format is a WebGPU string: "uint16" or "uint32".
// If size is 0, the size parameter is omitted (meaning "rest of buffer"),
// matching Rust wgpu's set_index_buffer_with_f64 (no size variant).
func (p *RenderPassEncoder) SetIndexBuffer(buffer js.Value, format string, offset uint64, size uint64) {
	if size == 0 {
		p.fnSetIndexBuffer.Invoke(buffer, format, float64(offset))
	} else {
		p.fnSetIndexBuffer.Invoke(buffer, format, float64(offset), float64(size))
	}
}

// Draw draws primitives.
// Matches Rust: draw_with_instance_count_and_first_vertex_and_first_instance.
func (p *RenderPassEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	p.fnDraw.Invoke(vertexCount, instanceCount, firstVertex, firstInstance)
}

// DrawIndexed draws indexed primitives.
// baseVertex is int32 (can be negative) per WebGPU spec.
// Matches Rust: draw_indexed_with_instance_count_and_first_index_and_base_vertex_and_first_instance.
func (p *RenderPassEncoder) DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	p.fnDrawIndexed.Invoke(indexCount, instanceCount, firstIndex, baseVertex, firstInstance)
}

// DrawIndirect draws primitives with GPU-generated parameters from an indirect buffer.
func (p *RenderPassEncoder) DrawIndirect(buffer js.Value, offset uint64) {
	p.fnDrawIndirect.Invoke(buffer, float64(offset))
}

// DrawIndexedIndirect draws indexed primitives with GPU-generated parameters.
func (p *RenderPassEncoder) DrawIndexedIndirect(buffer js.Value, offset uint64) {
	p.fnDrawIndexedIndirect.Invoke(buffer, float64(offset))
}

// SetViewport sets the viewport transformation.
func (p *RenderPassEncoder) SetViewport(x, y, w, h, minDepth, maxDepth float32) {
	p.fnSetViewport.Invoke(x, y, w, h, minDepth, maxDepth)
}

// SetScissorRect sets the scissor rectangle for clipping.
func (p *RenderPassEncoder) SetScissorRect(x, y, w, h uint32) {
	p.fnSetScissorRect.Invoke(x, y, w, h)
}

// SetBlendConstant sets the blend constant color via a GPUColorDict.
// Matches Rust: set_blend_constant_with_gpu_color_dict.
func (p *RenderPassEncoder) SetBlendConstant(color js.Value) {
	p.fnSetBlendConstant.Invoke(color)
}

// SetStencilReference sets the stencil reference value.
func (p *RenderPassEncoder) SetStencilReference(ref uint32) {
	p.fnSetStencilReference.Invoke(ref)
}

// End ends the render pass.
// Matches Rust WebRenderPassEncoder Drop which calls end().
func (p *RenderPassEncoder) End() {
	p.fnEnd.Invoke()
}

// Ref returns the underlying GPURenderPassEncoder js.Value.
func (p *RenderPassEncoder) Ref() js.Value {
	return p.ref_
}
