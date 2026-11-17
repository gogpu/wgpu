//go:build linux && !android && !(js && wasm)

package vulkan

import (
	"slices"
	"testing"
)

func TestPlatformSurfaceExtensionsIgnoreSessionEnvironment(t *testing.T) {
	want := []string{
		extensionWaylandSurface,
		extensionXlibSurface,
	}

	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	if got := platformSurfaceExtensions(); !slices.Equal(got, want) {
		t.Fatalf("Wayland session extensions = %q, want %q", got, want)
	}

	t.Setenv("WAYLAND_DISPLAY", "")
	if got := platformSurfaceExtensions(); !slices.Equal(got, want) {
		t.Fatalf("X11 session extensions = %q, want %q", got, want)
	}
}
