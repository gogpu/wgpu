//go:build js && wasm

package resource

import (
	"sync/atomic"

	"github.com/gogpu/wgpu/internal/browser"
)

// ShaderModule represents a compiled shader module.
type ShaderModule struct {
	browser *browser.ShaderModule
	// released is the CAS guard that admits exactly one destroy: concurrent
	// Release calls see CompareAndSwap succeed for one caller only. It is nil
	// only on an unguarded zero value, whose Release is a no-op.
	released *atomic.Bool
}

// NewShaderModule builds a shader module over a browser shader module object.
func NewShaderModule(b *browser.ShaderModule) *ShaderModule {
	return &ShaderModule{browser: b, released: new(atomic.Bool)}
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
	if m.released == nil || !m.released.CompareAndSwap(false, true) {
		return
	}
}
