//go:build rust && linux

package wgpu

import (
	"os"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

// createPlatformSurface creates a rendering surface on Linux.
// Detects Wayland vs X11 based on WAYLAND_DISPLAY environment variable,
// matching the platform detection in gogpu's internal/platform/platform_linux.go
// and the rust backend in gogpu/gpu/backend/rust/rust_linux.go.
func createPlatformSurface(instance *rwgpu.Instance, displayHandle, windowHandle uintptr) (*rwgpu.Surface, error) {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return instance.CreateSurfaceFromWaylandSurface(displayHandle, windowHandle)
	}
	// X11 fallback: window handle must be uint64 for Xlib Window (XID).
	return instance.CreateSurfaceFromXlibWindow(displayHandle, uint64(windowHandle))
}
