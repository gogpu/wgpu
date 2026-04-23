//go:build js && wasm

package wgpu

// ShaderModule represents a compiled shader module.
type ShaderModule struct {
	released bool
}

// Release destroys the shader module.
func (m *ShaderModule) Release() {
	if m.released {
		return
	}
	m.released = true
}
