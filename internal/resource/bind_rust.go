//go:build rust

package resource

import rwgpu "github.com/go-webgpu/webgpu/wgpu"

// BindGroupLayout defines the structure of resource bindings for shaders.
// On Rust backend, this wraps go-webgpu/webgpu BindGroupLayout.
type BindGroupLayout struct {
	r        *rwgpu.BindGroupLayout
	released bool
}

// NewBindGroupLayout builds a bind group layout over a wgpu-native object.
func NewBindGroupLayout(r *rwgpu.BindGroupLayout) *BindGroupLayout {
	return &BindGroupLayout{r: r}
}

// Rust returns the wgpu-native bind group layout object, or nil.
func (l *BindGroupLayout) Rust() *rwgpu.BindGroupLayout {
	if l == nil {
		return nil
	}
	return l.r
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
	released bool
}

// NewPipelineLayout builds a pipeline layout over a wgpu-native object.
func NewPipelineLayout(r *rwgpu.PipelineLayout) *PipelineLayout {
	return &PipelineLayout{r: r}
}

// Rust returns the wgpu-native pipeline layout object, or nil.
func (l *PipelineLayout) Rust() *rwgpu.PipelineLayout {
	if l == nil {
		return nil
	}
	return l.r
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
	released bool
}

// NewBindGroup builds a bind group over a wgpu-native object.
func NewBindGroup(r *rwgpu.BindGroup) *BindGroup {
	return &BindGroup{r: r}
}

// Rust returns the wgpu-native bind group object, or nil.
func (g *BindGroup) Rust() *rwgpu.BindGroup {
	if g == nil {
		return nil
	}
	return g.r
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
