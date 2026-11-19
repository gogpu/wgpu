//go:build js && wasm

package resource

import (
	"sync/atomic"

	"github.com/gogpu/wgpu/internal/browser"
)

// BindGroupLayout defines the structure of resource bindings for shaders.
type BindGroupLayout struct {
	browser *browser.BindGroupLayout
	// released is the CAS guard that admits exactly one destroy: concurrent
	// Release calls see CompareAndSwap succeed for one caller only. It is nil
	// only on an unguarded zero value, whose Release is a no-op.
	released *atomic.Bool
}

// NewBindGroupLayout builds a bind group layout over a browser object.
func NewBindGroupLayout(b *browser.BindGroupLayout) *BindGroupLayout {
	return &BindGroupLayout{browser: b, released: new(atomic.Bool)}
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
	if l.released == nil || !l.released.CompareAndSwap(false, true) {
		return
	}
}

// PipelineLayout defines the bind group layout arrangement for a pipeline.
type PipelineLayout struct {
	browser *browser.PipelineLayout
	// released is the CAS guard that admits exactly one destroy: concurrent
	// Release calls see CompareAndSwap succeed for one caller only. It is nil
	// only on an unguarded zero value, whose Release is a no-op.
	released *atomic.Bool
}

// NewPipelineLayout builds a pipeline layout over a browser object.
func NewPipelineLayout(b *browser.PipelineLayout) *PipelineLayout {
	return &PipelineLayout{browser: b, released: new(atomic.Bool)}
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
	if l.released == nil || !l.released.CompareAndSwap(false, true) {
		return
	}
}

// LateBufferBindingInfo records the actual buffer binding size for a layout entry
// with MinBindingSize == 0.
type LateBufferBindingInfo struct {
	BindingIndex uint32
	Size         uint64
}

// BindGroup represents bound GPU resources for shader access.
type BindGroup struct {
	browser *browser.BindGroup
	// released is the CAS guard that admits exactly one destroy: concurrent
	// Release calls see CompareAndSwap succeed for one caller only. It is nil
	// only on an unguarded zero value, whose Release is a no-op.
	released *atomic.Bool
}

// NewBindGroup builds a bind group over a browser object.
func NewBindGroup(b *browser.BindGroup) *BindGroup {
	return &BindGroup{browser: b, released: new(atomic.Bool)}
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
	if g.released == nil || !g.released.CompareAndSwap(false, true) {
		return
	}
}
