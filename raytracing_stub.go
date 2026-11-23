//go:build rust || (js && wasm)

package wgpu

import (
	"fmt"

	"github.com/gogpu/gputypes"
)

// Ray tracing is not available on the Rust FFI backend (go-webgpu/webgpu
// has no acceleration structure bindings) or in the browser (not part of
// WebGPU). These stubs keep the API identical across backends; creating a
// BLAS or TLAS returns ErrUnsupported.

// Blas is a bottom-level acceleration structure. Not supported by this
// backend.
type Blas struct{}

// Handle returns false: this backend has no acceleration structures.
func (b *Blas) Handle() (uint64, bool) { return 0, false }

// Release is a no-op.
func (b *Blas) Release() {}

// Tlas is a top-level acceleration structure. Not supported by this backend.
type Tlas struct {
	instances []*TlasInstance
}

// MaxInstances returns the number of instance slots.
func (t *Tlas) MaxInstances() uint32 { return uint32(len(t.instances)) } //nolint:gosec // always 0

// Instances returns the instance slots.
func (t *Tlas) Instances() []*TlasInstance { return t.instances }

// SetInstance sets instance slot i. It panics if i is out of range.
func (t *Tlas) SetInstance(i int, instance *TlasInstance) { t.instances[i] = instance }

// InstancesMut returns slots [lo, hi), or nil if the range is out of bounds.
func (t *Tlas) InstancesMut(lo, hi int) []*TlasInstance {
	if lo < 0 || hi > len(t.instances) || lo > hi {
		return nil
	}
	return t.instances[lo:hi:hi]
}

// Release is a no-op.
func (t *Tlas) Release() {}

// CreateBlas returns ErrUnsupported.
func (d *Device) CreateBlas(_ *gputypes.CreateBlasDescriptor, _ gputypes.BlasGeometrySizeDescriptors) (*Blas, error) {
	return nil, fmt.Errorf("wgpu: CreateBlas: %w", ErrUnsupported)
}

// CreateTlas returns ErrUnsupported.
func (d *Device) CreateTlas(_ *gputypes.CreateTlasDescriptor) (*Tlas, error) {
	return nil, fmt.Errorf("wgpu: CreateTlas: %w", ErrUnsupported)
}

// BuildAccelerationStructures does nothing: no BLAS or TLAS can be created
// on this backend.
func (e *CommandEncoder) BuildAccelerationStructures(_ []BlasBuildEntry, _ []*Tlas) {}

// rejectAccelerationStructureEntries returns ErrUnsupported if any entry
// binds an acceleration structure.
func rejectAccelerationStructureEntries(entries []BindGroupEntry) error {
	for i := range entries {
		if entries[i].AccelerationStructure != nil {
			return fmt.Errorf("wgpu: acceleration structure bindings: %w", ErrUnsupported)
		}
	}
	return nil
}
