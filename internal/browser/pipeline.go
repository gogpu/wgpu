//go:build js && wasm

package browser

import "syscall/js"

// RenderPipeline wraps a browser GPURenderPipeline.
type RenderPipeline struct {
	// ref_ is the GPURenderPipeline JavaScript object.
	ref_ js.Value
}

// NewRenderPipeline constructs a RenderPipeline from a GPURenderPipeline js.Value.
func NewRenderPipeline(ref js.Value) *RenderPipeline {
	return &RenderPipeline{ref_: ref}
}

// Ref returns the underlying GPURenderPipeline js.Value.
func (p *RenderPipeline) Ref() js.Value { return p.ref_ }

// GetBindGroupLayout returns the bind group layout at the given index.
// Wraps GPURenderPipeline.getBindGroupLayout(index).
func (p *RenderPipeline) GetBindGroupLayout(index uint32) *BindGroupLayout {
	jsLayout := p.ref_.Call("getBindGroupLayout", index)
	return NewBindGroupLayout(jsLayout)
}

// ComputePipeline wraps a browser GPUComputePipeline.
type ComputePipeline struct {
	// ref_ is the GPUComputePipeline JavaScript object.
	ref_ js.Value
}

// NewComputePipeline constructs a ComputePipeline from a GPUComputePipeline js.Value.
func NewComputePipeline(ref js.Value) *ComputePipeline {
	return &ComputePipeline{ref_: ref}
}

// Ref returns the underlying GPUComputePipeline js.Value.
func (p *ComputePipeline) Ref() js.Value { return p.ref_ }

// GetBindGroupLayout returns the bind group layout at the given index.
// Wraps GPUComputePipeline.getBindGroupLayout(index).
func (p *ComputePipeline) GetBindGroupLayout(index uint32) *BindGroupLayout {
	jsLayout := p.ref_.Call("getBindGroupLayout", index)
	return NewBindGroupLayout(jsLayout)
}
