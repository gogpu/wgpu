//go:build rust

package wgpu

import rwgpu "github.com/go-webgpu/webgpu/wgpu"

// BindGroupLayout defines the structure of resource bindings for shaders.
// On Rust backend, this wraps go-webgpu/webgpu BindGroupLayout.
type BindGroupLayout struct {
	r        *rwgpu.BindGroupLayout
	device   *Device
	released bool
}

// Release destroys the bind group layout.
func (l *BindGroupLayout) Release() {
	if l.released {
		return
	}
	l.released = true
	if l.r != nil {
		l.r.Release()
	}
}

// PipelineLayout defines the bind group layout arrangement for a pipeline.
// On Rust backend, this wraps go-webgpu/webgpu PipelineLayout.
type PipelineLayout struct {
	r        *rwgpu.PipelineLayout
	device   *Device
	released bool
}

// Release destroys the pipeline layout.
func (l *PipelineLayout) Release() {
	if l.released {
		return
	}
	l.released = true
	if l.r != nil {
		l.r.Release()
	}
}

// LateBufferBindingInfo records the actual buffer binding size for a layout entry
// with MinBindingSize == 0.
type LateBufferBindingInfo struct {
	BindingIndex uint32
	Size         uint64
}

// BindGroup represents bound GPU resources for shader access.
// On Rust backend, this wraps go-webgpu/webgpu BindGroup.
type BindGroup struct {
	r        *rwgpu.BindGroup
	device   *Device
	released bool
}

// Release marks the bind group for destruction.
func (g *BindGroup) Release() {
	if g.released {
		return
	}
	g.released = true
	if g.r != nil {
		g.r.Release()
	}
}
