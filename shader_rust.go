//go:build rust

package wgpu

import rwgpu "github.com/go-webgpu/webgpu/wgpu"

// ShaderModule represents a compiled shader module.
// On Rust backend, this wraps go-webgpu/webgpu ShaderModule.
type ShaderModule struct {
	r        *rwgpu.ShaderModule
	device   *Device
	released bool
}

// Release destroys the shader module.
func (m *ShaderModule) Release() {
	if m.released {
		return
	}
	m.released = true
	if m.r != nil {
		m.r.Release()
	}
}
