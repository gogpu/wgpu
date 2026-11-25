//go:build !rust && !(js && wasm)

package device

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/resource"
)

// CreateBlas creates a bottom-level acceleration structure sized for the
// given geometry. Requires FeatureRayQuery.
//
// Matches Rust wgpu's Device::create_blas.
func (d *Device) CreateBlas(desc *gputypes.CreateBlasDescriptor, sizes gputypes.BlasGeometrySizeDescriptors) (*resource.Blas, error) {
	d = orZero(d)
	if d.released.Load() {
		return nil, core.ErrReleased
	}
	coreBlas, err := d.core.CreateBlas(desc, sizes)
	if err != nil {
		return nil, err
	}
	return resource.NewBlas(coreBlas), nil
}

// CreateTlas creates a top-level acceleration structure with
// desc.MaxInstances empty instance slots. Requires FeatureRayQuery.
//
// Matches Rust wgpu's Device::create_tlas.
func (d *Device) CreateTlas(desc *gputypes.CreateTlasDescriptor) (*resource.Tlas, error) {
	d = orZero(d)
	if d.released.Load() {
		return nil, core.ErrReleased
	}
	coreTlas, err := d.core.CreateTlas(desc)
	if err != nil {
		return nil, err
	}
	return resource.NewTlas(coreTlas), nil
}
