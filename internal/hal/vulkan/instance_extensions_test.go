//go:build !(js && wasm)

package vulkan

import (
	"slices"
	"testing"
)

func TestSelectAvailableExtensionsPreservesCandidateOrder(t *testing.T) {
	candidates := []string{
		extensionWaylandSurface,
		extensionXlibSurface,
		"VK_EXT_metal_surface\x00",
	}
	available := map[string]struct{}{
		"VK_KHR_xlib_surface":    {},
		"VK_KHR_wayland_surface": {},
	}
	want := []string{
		extensionWaylandSurface,
		extensionXlibSurface,
	}

	if got := selectAvailableExtensions(candidates, available); !slices.Equal(got, want) {
		t.Fatalf("selectAvailableExtensions() = %q, want %q", got, want)
	}
}
