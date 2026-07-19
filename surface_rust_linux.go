//go:build rust && linux && !android

package wgpu

import (
	"fmt"
	"os"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

func surfaceTargetFromLegacyHandles(displayHandle, windowHandle uintptr) SurfaceTargetUnsafe {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return SurfaceTargetFromWaylandSurface(displayHandle, windowHandle)
	}
	return SurfaceTargetFromXlibWindow(displayHandle, windowHandle)
}

// createPlatformSurfaceTarget creates a rendering surface from an explicit
// Xlib or Wayland target. Environment detection is limited to the legacy API.
func createPlatformSurfaceTarget(instance *rwgpu.Instance, target SurfaceTargetUnsafe) (*rwgpu.Surface, error) {
	switch target.kind {
	case surfaceTargetXlibWindow:
		return instance.CreateSurfaceFromXlibWindow(target.displayHandle, uint64(target.windowHandle))
	case surfaceTargetWaylandSurface:
		return instance.CreateSurfaceFromWaylandSurface(target.displayHandle, target.windowHandle)
	default:
		return nil, fmt.Errorf("%w: Linux backend requires an Xlib or Wayland target", ErrUnsupportedSurfaceTarget)
	}
}
