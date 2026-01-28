package noop

import (
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/gputypes"
)

// API implements hal.Backend for the noop backend.
type API struct{}

// Variant returns the backend type identifier.
func (API) Variant() gputypes.Backend {
	return gputypes.BackendEmpty
}

// CreateInstance creates a new noop instance.
// Always succeeds and returns a placeholder instance.
func (API) CreateInstance(_ *hal.InstanceDescriptor) (hal.Instance, error) {
	return &Instance{}, nil
}

// Instance implements hal.Instance for the noop backend.
type Instance struct{}

// CreateSurface creates a noop surface.
// Always succeeds regardless of display/window handles.
func (i *Instance) CreateSurface(_, _ uintptr) (hal.Surface, error) {
	return &Surface{}, nil
}

// EnumerateAdapters returns a single default noop adapter.
// The surfaceHint is ignored.
func (i *Instance) EnumerateAdapters(_ hal.Surface) []hal.ExposedAdapter {
	return []hal.ExposedAdapter{
		{
			Adapter: &Adapter{},
			Info: gputypes.AdapterInfo{
				Name:       "Noop Adapter",
				Vendor:     "GoGPU",
				VendorID:   0,
				DeviceID:   0,
				DeviceType: gputypes.DeviceTypeOther,
				Driver:     "noop-1.0",
				DriverInfo: "No-operation backend for testing",
				Backend:    gputypes.BackendEmpty,
			},
			Features: 0, // No features supported
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

// Destroy is a no-op for the noop instance.
func (i *Instance) Destroy() {}
