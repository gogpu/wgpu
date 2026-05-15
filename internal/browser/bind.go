//go:build js && wasm

package browser

import "syscall/js"

// BindGroupLayout wraps a browser GPUBindGroupLayout.
type BindGroupLayout struct {
	// ref_ is the GPUBindGroupLayout JavaScript object.
	ref_ js.Value
}

// NewBindGroupLayout constructs a BindGroupLayout from a GPUBindGroupLayout js.Value.
func NewBindGroupLayout(ref js.Value) *BindGroupLayout {
	return &BindGroupLayout{ref_: ref}
}

// Ref returns the underlying GPUBindGroupLayout js.Value.
func (l *BindGroupLayout) Ref() js.Value { return l.ref_ }

// BindGroup wraps a browser GPUBindGroup.
type BindGroup struct {
	// ref_ is the GPUBindGroup JavaScript object.
	ref_ js.Value
}

// NewBindGroup constructs a BindGroup from a GPUBindGroup js.Value.
func NewBindGroup(ref js.Value) *BindGroup {
	return &BindGroup{ref_: ref}
}

// Ref returns the underlying GPUBindGroup js.Value.
func (g *BindGroup) Ref() js.Value { return g.ref_ }

// PipelineLayout wraps a browser GPUPipelineLayout.
type PipelineLayout struct {
	// ref_ is the GPUPipelineLayout JavaScript object.
	ref_ js.Value
}

// NewPipelineLayout constructs a PipelineLayout from a GPUPipelineLayout js.Value.
func NewPipelineLayout(ref js.Value) *PipelineLayout {
	return &PipelineLayout{ref_: ref}
}

// Ref returns the underlying GPUPipelineLayout js.Value.
func (l *PipelineLayout) Ref() js.Value { return l.ref_ }
