//go:build rust && windows

package wgpu

import (
	"fmt"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

func surfaceTargetFromLegacyHandles(displayHandle, windowHandle uintptr) SurfaceTargetUnsafe {
	return SurfaceTargetFromWindowsHWND(displayHandle, windowHandle)
}

// createPlatformSurfaceTarget creates a rendering surface on Windows via HWND.
func createPlatformSurfaceTarget(instance *rwgpu.Instance, target SurfaceTargetUnsafe) (*rwgpu.Surface, error) {
	if target.kind != surfaceTargetWindowsHWND {
		return nil, fmt.Errorf("%w: Windows backend requires a Win32 HWND", ErrUnsupportedSurfaceTarget)
	}
	return instance.CreateSurfaceFromWindowsHWND(target.displayHandle, target.windowHandle)
}
