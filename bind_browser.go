//go:build js && wasm

package wgpu

// BindGroupLayout defines the structure of resource bindings for shaders.
type BindGroupLayout struct {
	released bool
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
	released bool
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
	released bool
}

// Release marks the bind group for destruction.
func (g *BindGroup) Release() {
	if g.released {
		return
	}
	g.released = true
}
