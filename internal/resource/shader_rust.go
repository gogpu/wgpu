//go:build rust

package resource

import rwgpu "github.com/go-webgpu/webgpu/wgpu"

// ShaderModule represents a compiled shader module.
// On Rust backend, this wraps go-webgpu/webgpu ShaderModule.
type ShaderModule struct {
	r        *rwgpu.ShaderModule
	released bool
}

// NewShaderModule builds a shader module over a wgpu-native shader module
// object.
func NewShaderModule(r *rwgpu.ShaderModule) *ShaderModule {
	return &ShaderModule{r: r}
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
	if m.released {
		return
	}
	m.released = true
	if m.r != nil {
		m.r.Release()
	}
}
