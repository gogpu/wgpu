// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package vulkan

import (
	"slices"
	"testing"
)

// TestMaintenance1Support checks when a device has what maintenance1 gives
// (2D views of a 3D image's depth slices, negative viewport heights) and
// which extension that needs: Vulkan 1.1 has it in core, Vulkan 1.0 needs
// VK_KHR_maintenance1 enabled. EnumerateAdapters hides an adapter for which
// has2DViewsOf3D is false, so the vulkan-1.0 case is an adapter we never
// expose.
func TestMaintenance1Support(t *testing.T) {
	tests := []struct {
		name     string
		version  uint32
		hasExt   bool
		wantView bool
		wantExts []string
	}{
		{"vulkan-1.0", vkMakeVersion(1, 0, 0), false, false, nil},
		{"vulkan-1.0-maintenance1", vkMakeVersion(1, 0, 0), true, true, []string{extMaintenance1 + "\x00"}},
		{"vulkan-1.1", vkMakeVersion(1, 1, 0), false, true, nil},
		{"vulkan-1.2-maintenance1", vkMakeVersion(1, 2, 0), true, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &extendedFeatureSupport{apiVersion: tt.version, extensions: map[string]bool{extMaintenance1: tt.hasExt}}
			if got := s.has2DViewsOf3D(); got != tt.wantView {
				t.Errorf("has2DViewsOf3D() = %v, want %v", got, tt.wantView)
			}
			if got := s.maintenance1Extensions(); !slices.Equal(got, tt.wantExts) {
				t.Errorf("maintenance1Extensions() = %q, want %q", got, tt.wantExts)
			}
		})
	}
}
