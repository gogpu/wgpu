package wgpu

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// BindGroupLayout defines the structure of resource bindings for shaders.
type BindGroupLayout struct {
	hal      hal.BindGroupLayout
	device   *Device
	released bool
	// entries stores the layout entries for entry-by-entry compatibility checks.
	// This matches Rust wgpu-core's pattern where binder.check_compatibility()
	// compares layouts by their entries, not by pointer identity.
	entries []gputypes.BindGroupLayoutEntry
}

// isCompatibleWith returns true if two layouts have identical entries.
// This matches Rust wgpu-core's entry-by-entry compatibility check in
// binder.check_compatibility(), allowing equivalent layouts created via
// separate CreateBindGroupLayout calls to be considered compatible.
func (l *BindGroupLayout) isCompatibleWith(other *BindGroupLayout) bool {
	if l == other {
		return true // pointer equality fast path
	}
	if len(l.entries) != len(other.entries) {
		return false
	}
	for i := range l.entries {
		if !bindGroupLayoutEntriesEqual(&l.entries[i], &other.entries[i]) {
			return false
		}
	}
	return true
}

// bindGroupLayoutEntriesEqual compares two BindGroupLayoutEntry values,
// dereferencing pointer fields (Buffer, Sampler, Texture, StorageTexture)
// to compare by value rather than by pointer identity.
func bindGroupLayoutEntriesEqual(a, b *gputypes.BindGroupLayoutEntry) bool {
	if a.Binding != b.Binding || a.Visibility != b.Visibility {
		return false
	}

	// Compare Buffer pointer fields by value.
	if !optionalEqual(a.Buffer, b.Buffer) {
		return false
	}
	// Compare Sampler pointer fields by value.
	if !optionalEqual(a.Sampler, b.Sampler) {
		return false
	}
	// Compare Texture pointer fields by value.
	if !optionalEqual(a.Texture, b.Texture) {
		return false
	}
	// Compare StorageTexture pointer fields by value.
	if !optionalEqual(a.StorageTexture, b.StorageTexture) {
		return false
	}
	return true
}

// optionalEqual compares two optional (pointer) values by dereferenced content.
// Both nil → equal; one nil → not equal; both non-nil → compare dereferenced values.
func optionalEqual[T comparable](a, b *T) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// Release destroys the bind group layout.
func (l *BindGroupLayout) Release() {
	if l.released {
		return
	}
	l.released = true
	halDevice := l.device.halDevice()
	if halDevice != nil {
		halDevice.DestroyBindGroupLayout(l.hal)
	}
}

// PipelineLayout defines the bind group layout arrangement for a pipeline.
type PipelineLayout struct {
	hal      hal.PipelineLayout
	device   *Device
	released bool
	// bindGroupCount is the number of bind group layouts in this layout.
	// Used for validation in SetBindGroup.
	bindGroupCount uint32
	// bindGroupLayouts stores the layouts used to create this pipeline layout.
	// Used by the binder for draw-time compatibility validation.
	bindGroupLayouts []*BindGroupLayout
}

// Release destroys the pipeline layout.
func (l *PipelineLayout) Release() {
	if l.released {
		return
	}
	l.released = true
	halDevice := l.device.halDevice()
	if halDevice != nil {
		halDevice.DestroyPipelineLayout(l.hal)
	}
}

// BindGroup represents bound GPU resources for shader access.
type BindGroup struct {
	hal      hal.BindGroup
	device   *Device
	released bool
	// layout is the bind group layout used to create this bind group.
	// Stored for draw-time compatibility validation via the binder.
	layout *BindGroupLayout
}

// Release destroys the bind group.
func (g *BindGroup) Release() {
	if g.released {
		return
	}
	g.released = true
	halDevice := g.device.halDevice()
	if halDevice != nil {
		halDevice.DestroyBindGroup(g.hal)
	}
}
