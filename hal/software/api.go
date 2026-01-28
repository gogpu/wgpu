//go:build software

package software

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// API implements hal.Backend for the software backend.
type API struct{}

// Variant returns the backend type identifier.
func (API) Variant() gputypes.Backend {
	return gputypes.BackendEmpty
}

// CreateInstance creates a new software rendering instance.
// Always succeeds and returns a CPU-based rendering instance.
func (API) CreateInstance(_ *hal.InstanceDescriptor) (hal.Instance, error) {
	return &Instance{}, nil
}

// Instance implements hal.Instance for the software backend.
type Instance struct{}

// CreateSurface creates a software rendering surface.
// Always succeeds regardless of display/window handles.
func (i *Instance) CreateSurface(_, _ uintptr) (hal.Surface, error) {
	return &Surface{}, nil
}

// EnumerateAdapters returns a single default software adapter.
// The surfaceHint is ignored.
func (i *Instance) EnumerateAdapters(_ hal.Surface) []hal.ExposedAdapter {
	return []hal.ExposedAdapter{
		{
			Adapter: &Adapter{},
			Info: gputypes.AdapterInfo{
				Name:       "Software Renderer",
				Vendor:     "GoGPU",
				VendorID:   0,
				DeviceID:   0,
				DeviceType: gputypes.DeviceTypeCPU,
				Driver:     "software-1.0",
				DriverInfo: "CPU-based software rendering backend",
				Backend:    gputypes.BackendEmpty,
			},
			Features: 0, // No optional features supported
			Capabilities: hal.Capabilities{
				Limits: gputypes.DefaultLimits(),
				AlignmentsMask: hal.Alignments{
					BufferCopyOffset: 4,
					BufferCopyPitch:  256,
				},
				DownlevelCapabilities: hal.DownlevelCapabilities{
					ShaderModel: 0,
					Flags:       0,
				},
			},
		},
	}
}

// Destroy is a no-op for the software instance.
func (i *Instance) Destroy() {}
