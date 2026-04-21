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
// If a valid window handle is provided, Present() will automatically blit
// the framebuffer to the window via platform-native APIs (GDI on Windows,
// XPutImage on Linux X11).
// If window is 0 (headless mode), Present() is a no-op.
//
// displayHandle is platform-specific: X11 Display* on Linux, 0 elsewhere.
// windowHandle is the native window: HWND on Windows, X11 Window on Linux.
func (i *Instance) CreateSurface(displayHandle, window uintptr) (hal.Surface, error) {
	return &Surface{displayHandle: displayHandle, hwnd: window}, nil
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
