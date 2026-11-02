// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package vulkan

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/backend"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// TestDeviceShaderSafetyOptions opens every Vulkan adapter here and checks
// that the device compiles shaders with the bounds checks and workgroup
// zeroing its enabled features call for: the feature query, the device
// creation chain and spirvOptions agree on real drivers (P1-HAL-002,
// P1-VK-013).
func TestDeviceShaderSafetyOptions(t *testing.T) {
	if err := vk.Init(); err != nil {
		t.Skipf("Vulkan not available: %v", err)
	}
	instance, err := Backend{}.CreateInstance(&hal.InstanceDescriptor{Backends: gputypes.BackendsVulkan})
	if err != nil {
		t.Skipf("Vulkan instance creation failed: %v", err)
	}
	defer instance.Destroy()
	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		t.Skip("no Vulkan adapters found")
	}
	for _, exposed := range adapters {
		t.Run(exposed.Info.Name, func(t *testing.T) {
			a, ok := exposed.Adapter.(*Adapter)
			if !ok {
				t.Skipf("adapter %T", exposed.Adapter)
			}
			open, err := a.Open(0, exposed.Capabilities.Limits)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			dev := open.Device.(*Device)
			defer func() {
				_ = dev.WaitIdle()
				dev.Destroy()
			}()
			got := dev.spirvOptions
			t.Logf("robustBufferAccess2 %v, robust image access %v, zero-init workgroup memory %v",
				a.ext.robustBufferAccess2(), a.ext.robustImageAccess(), a.ext.zeroInitializeWorkgroupMemory())
			if want := checkedUnless(a.ext.robustBufferAccess2()); got.BoundsCheckPolicies.Buffer != want {
				t.Errorf("buffer policy %v, want %v", got.BoundsCheckPolicies.Buffer, want)
			}
			if want := checkedUnless(a.ext.robustImageAccess()); got.BoundsCheckPolicies.ImageLoad != want {
				t.Errorf("image-load policy %v, want %v", got.BoundsCheckPolicies.ImageLoad, want)
			}
			wantZero := backend.ZeroInitWorkgroupPolyfill
			if a.ext.zeroInitializeWorkgroupMemory() {
				wantZero = backend.ZeroInitWorkgroupNative
			}
			if got.ZeroInitializeWorkgroupMemory != wantZero {
				t.Errorf("workgroup zeroing %v, want %v", got.ZeroInitializeWorkgroupMemory, wantZero)
			}
		})
	}
}
