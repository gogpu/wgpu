//go:build js && wasm

package resource

import "github.com/gogpu/wgpu/internal/browser"

// ShaderModule represents a compiled shader module.
type ShaderModule struct {
	browser  *browser.ShaderModule
	released bool
}

// NewShaderModule builds a shader module over a browser shader module object.
func NewShaderModule(b *browser.ShaderModule) *ShaderModule {
	return &ShaderModule{browser: b}
}

// Browser returns the browser shader module object, or nil.
func (m *ShaderModule) Browser() *browser.ShaderModule {
	if m == nil {
		return nil
	}
	return m.browser
}

// Release destroys the shader module.
func (m *ShaderModule) Release() {
	if m.released {
		return
	}
	m.released = true
}
