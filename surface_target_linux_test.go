//go:build linux && !rust

package wgpu

import "testing"

func TestLegacySurfaceTargetMappingLinux(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	if got, want := surfaceTargetFromLegacyHandles(1, 2), SurfaceTargetFromXlibWindow(1, 2); got != want {
		t.Fatalf("Xlib legacy target = %+v, want %+v", got, want)
	}

	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	if got, want := surfaceTargetFromLegacyHandles(3, 4), SurfaceTargetFromWaylandSurface(3, 4); got != want {
		t.Fatalf("Wayland legacy target = %+v, want %+v", got, want)
	}
}
