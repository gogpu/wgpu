//go:build js && wasm

package resource

import "github.com/gogpu/wgpu/internal/browser"

// BindGroupLayout defines the structure of resource bindings for shaders.
type BindGroupLayout struct {
	browser  *browser.BindGroupLayout
	released bool
}

// NewBindGroupLayout builds a bind group layout over a browser object.
func NewBindGroupLayout(b *browser.BindGroupLayout) *BindGroupLayout {
	return &BindGroupLayout{browser: b}
}

// Browser returns the browser bind group layout object, or nil.
func (l *BindGroupLayout) Browser() *browser.BindGroupLayout {
	if l == nil {
		return nil
	}
	return l.browser
}

// Release destroys the bind group layout.
func (l *BindGroupLayout) Release() {
	if l.released {
		return
	}
	l.released = true
}

// PipelineLayout defines the bind group layout arrangement for a pipeline.
type PipelineLayout struct {
	browser  *browser.PipelineLayout
	released bool
}

// NewPipelineLayout builds a pipeline layout over a browser object.
func NewPipelineLayout(b *browser.PipelineLayout) *PipelineLayout {
	return &PipelineLayout{browser: b}
}

// Browser returns the browser pipeline layout object, or nil.
func (l *PipelineLayout) Browser() *browser.PipelineLayout {
	if l == nil {
		return nil
	}
	return l.browser
}

// Release destroys the pipeline layout.
func (l *PipelineLayout) Release() {
	if l.released {
		return
	}
	l.released = true
}

// LateBufferBindingInfo records the actual buffer binding size for a layout entry
// with MinBindingSize == 0.
type LateBufferBindingInfo struct {
	BindingIndex uint32
	Size         uint64
}

// BindGroup represents bound GPU resources for shader access.
type BindGroup struct {
	browser  *browser.BindGroup
	released bool
}

// NewBindGroup builds a bind group over a browser object.
func NewBindGroup(b *browser.BindGroup) *BindGroup {
	return &BindGroup{browser: b}
}

// Browser returns the browser bind group object, or nil.
func (g *BindGroup) Browser() *browser.BindGroup {
	if g == nil {
		return nil
	}
	return g.browser
}

// Release marks the bind group for destruction.
func (g *BindGroup) Release() {
	if g.released {
		return
	}
	g.released = true
}
