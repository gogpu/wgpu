//go:build js && wasm

package browser

import "syscall/js"

// ShaderModule wraps a browser GPUShaderModule.
//
// On browser, shader modules hold WGSL code compiled by the browser's native
// shader compiler. No naga compilation happens -- the WGSL goes directly to
// the browser's createShaderModule.
type ShaderModule struct {
	// ref_ is the GPUShaderModule JavaScript object.
	ref_ js.Value
}

// NewShaderModule constructs a ShaderModule from a GPUShaderModule js.Value.
func NewShaderModule(ref js.Value) *ShaderModule {
	return &ShaderModule{ref_: ref}
}

// Ref returns the underlying GPUShaderModule js.Value.
func (m *ShaderModule) Ref() js.Value { return m.ref_ }
