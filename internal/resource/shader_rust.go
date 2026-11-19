//go:build rust

package resource

import (
	"sync/atomic"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

// ShaderModule represents a compiled shader module.
// On Rust backend, this wraps go-webgpu/webgpu ShaderModule.
type ShaderModule struct {
	r *rwgpu.ShaderModule
	// released is the CAS guard that admits exactly one destroy: concurrent
	// Release calls see CompareAndSwap succeed for one caller only. It is nil
	// only on an unguarded zero value, whose Release is a no-op.
	released *atomic.Bool
}

// NewShaderModule builds a shader module over a wgpu-native shader module
// object.
func NewShaderModule(r *rwgpu.ShaderModule) *ShaderModule {
	return &ShaderModule{r: r, released: new(atomic.Bool)}
}

// Rust returns the wgpu-native shader module object, or nil.
func (m *ShaderModule) Rust() *rwgpu.ShaderModule {
	if m == nil {
		return nil
	}
	return m.r
}

// Release destroys the shader module.
func (m *ShaderModule) Release() {
	if m.released == nil || !m.released.CompareAndSwap(false, true) {
		return
	}
	if m.r != nil {
		m.r.Release()
	}
}
