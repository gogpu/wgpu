//go:build !(js && wasm)

package software

import (
	"fmt"
	"runtime"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

const (
	goosWindows = "windows"
	goosLinux   = "linux"
	goosDarwin  = "darwin"
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
// The target kind remains attached to the Surface so deferred platform setup
// never has to infer the window system from process-global state.
func (i *Instance) CreateSurface(target hal.SurfaceTarget) (hal.Surface, error) {
	if !supportsSurfaceTarget(runtime.GOOS, target.Kind) {
		return nil, fmt.Errorf("software: %w: got %s on %s", hal.ErrUnsupportedSurfaceTarget, target.Kind, runtime.GOOS)
	}
	return &Surface{
		targetKind:    target.Kind,
		displayHandle: target.DisplayHandle,
		hwnd:          target.WindowHandle,
	}, nil
}

func supportsSurfaceTarget(goos string, kind hal.SurfaceTargetKind) bool {
	if kind == hal.SurfaceTargetHeadless {
		return true
	}
	switch goos {
	case goosWindows:
		return kind == hal.SurfaceTargetWindowsHWND
	case goosLinux:
		return kind == hal.SurfaceTargetXlibWindow || kind == hal.SurfaceTargetWaylandSurface
	case goosDarwin:
		return kind == hal.SurfaceTargetMetalLayer
	default:
		return false
	}
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
					Flags:       hal.DownlevelFlagsComputeShaders,
				},
			},
		},
	}
}

// Destroy is a no-op for the software instance.
func (i *Instance) Destroy() {}
