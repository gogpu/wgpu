//go:build rust && darwin

package wgpu

import (
	"fmt"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

func surfaceTargetFromLegacyHandles(_, windowHandle uintptr) SurfaceTargetUnsafe {
	return SurfaceTargetFromMetalLayer(windowHandle)
}

// createPlatformSurfaceTarget creates a rendering surface on macOS via CAMetalLayer.
func createPlatformSurfaceTarget(instance *rwgpu.Instance, target SurfaceTargetUnsafe) (*rwgpu.Surface, error) {
	if target.kind != surfaceTargetMetalLayer {
		return nil, fmt.Errorf("%w: macOS backend requires a Metal layer", ErrUnsupportedSurfaceTarget)
	}
	return instance.CreateSurfaceFromMetalLayer(target.windowHandle)
}
