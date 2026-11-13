// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import "testing"

// TestAnAdapterRequestedForOneSurfaceAnswersAnothersCapabilities requests
// adapters for one window's surface, then asks each for the capabilities of a
// second window's surface. It used to answer only for the surface it was
// requested with, and nil for any other, so a second window was configured
// without capabilities.
//
// The test is skipped if no Vulkan GPU is available (headless CI).
func TestAnAdapterRequestedForOneSurfaceAnswersAnothersCapabilities(t *testing.T) {
	instance := createTestInstance(t)
	first := createTestSurface(t, instance)
	second := createTestSurface(t, instance)

	adapters := instance.EnumerateAdapters(first)
	if len(adapters) == 0 {
		t.Skip("no Vulkan adapter presents to the window")
	}
	for _, exposed := range adapters {
		if exposed.Adapter.SurfaceCapabilities(second) == nil {
			t.Errorf("%s: the adapter requested for the first surface answers no capabilities for the second", exposed.Info.Name)
		}
	}
}
