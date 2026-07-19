//go:build rust && android

package wgpu

import (
	"fmt"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

func surfaceTargetFromLegacyHandles(_, windowHandle uintptr) SurfaceTargetUnsafe {
	return SurfaceTargetFromAndroidNativeWindow(windowHandle)
}

func createPlatformSurfaceTarget(instance *rwgpu.Instance, target SurfaceTargetUnsafe) (*rwgpu.Surface, error) {
	if target.kind != surfaceTargetAndroidNativeWindow {
		return nil, fmt.Errorf("%w: Android backend requires an ANativeWindow", ErrUnsupportedSurfaceTarget)
	}
	return instance.CreateSurfaceFromAndroidNativeWindow(target.windowHandle)
}
