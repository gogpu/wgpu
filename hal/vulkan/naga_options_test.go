//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"testing"

	"github.com/gogpu/naga/backend"
	"github.com/gogpu/naga/spirv"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// TestSPIRVOptionsPerDevice pins the naga SPIR-V options to Rust wgpu-hal's
// (vulkan/adapter.rs:2893-2975), clamped to our instance version: naga's zero
// values (SPIR-V 1.0, Restrict on binding arrays) must never reach a device.
func TestSPIRVOptionsPerDevice(t *testing.T) {
	for _, c := range []struct {
		device uint32
		want   spirv.Version
	}{
		{vkMakeVersion(1, 0, 61), spirv.Version1_0},
		{vkMakeVersion(1, 1, 130), spirv.Version1_3},
		{vkMakeVersion(1, 2, 0), spirv.Version1_5},
		{vkMakeVersion(1, 3, 250), spirv.Version1_5}, // instance is 1.2
		{vkMakeVersion(1, 4, 309), spirv.Version1_5},
	} {
		opts := spirvOptions(c.device, &extendedFeatureSupport{})
		if opts.Version != c.want {
			t.Errorf("device %d.%d: SPIR-V %v, want %v", vkVersionMajor(c.device), vkVersionMinor(c.device), opts.Version, c.want)
		}
		if !opts.ForcePointSize || opts.AdjustCoordinateSpace {
			t.Errorf("ForcePointSize %v, AdjustCoordinateSpace %v; want true, false", opts.ForcePointSize, opts.AdjustCoordinateSpace)
		}
		want := backend.BoundsCheckPolicies{
			Index:        backend.BoundsCheckRestrict,
			Buffer:       backend.BoundsCheckRestrict,
			ImageLoad:    backend.BoundsCheckRestrict,
			BindingArray: backend.BoundsCheckUnchecked,
		}
		if opts.BoundsCheckPolicies != want {
			t.Errorf("bounds checks %+v, want %+v", opts.BoundsCheckPolicies, want)
		}
	}
}

// TestSPIRVBoundsChecksFollowRobustness pins the buffer and image-load
// policies to the device's robustness, as Rust wgpu-hal picks them
// (vulkan/adapter.rs, Adapter::open): Unchecked only where the driver checks
// the access itself (P1-HAL-002, BUG-VK-BOUNDS-001). Index checks never
// depend on robustness: robustness covers descriptors, not local arrays.
func TestSPIRVBoundsChecksFollowRobustness(t *testing.T) {
	on, off := vk.Bool32(vk.True), vk.Bool32(vk.False)
	rba := vk.PhysicalDeviceFeatures{RobustBufferAccess: on}
	for _, c := range []struct {
		name          string
		ext           extendedFeatureSupport
		buffer, image backend.BoundsCheckPolicy
	}{
		{"no robustness", extendedFeatureSupport{core: rba}, backend.BoundsCheckRestrict, backend.BoundsCheckRestrict},
		{"robustBufferAccess2", extendedFeatureSupport{core: rba, robustness2: &vk.PhysicalDeviceRobustness2FeaturesKHR{RobustBufferAccess2: on}},
			backend.BoundsCheckUnchecked, backend.BoundsCheckRestrict},
		{"robustBufferAccess2 without robustBufferAccess", extendedFeatureSupport{robustness2: &vk.PhysicalDeviceRobustness2FeaturesKHR{RobustBufferAccess2: on}},
			backend.BoundsCheckRestrict, backend.BoundsCheckRestrict},
		{"robustImageAccess2", extendedFeatureSupport{core: rba, robustness2: &vk.PhysicalDeviceRobustness2FeaturesKHR{RobustImageAccess2: on}},
			backend.BoundsCheckRestrict, backend.BoundsCheckUnchecked},
		{"image robustness", extendedFeatureSupport{core: rba, imageRobust: &vk.PhysicalDeviceImageRobustnessFeatures{RobustImageAccess: on}},
			backend.BoundsCheckRestrict, backend.BoundsCheckUnchecked},
		{"image robustness struct, feature off", extendedFeatureSupport{core: rba, imageRobust: &vk.PhysicalDeviceImageRobustnessFeatures{RobustImageAccess: off}},
			backend.BoundsCheckRestrict, backend.BoundsCheckRestrict},
		{"both", extendedFeatureSupport{core: rba, robustness2: &vk.PhysicalDeviceRobustness2FeaturesKHR{RobustBufferAccess2: on, RobustImageAccess2: on}},
			backend.BoundsCheckUnchecked, backend.BoundsCheckUnchecked},
	} {
		got := spirvOptions(vkMakeVersion(1, 2, 0), &c.ext).BoundsCheckPolicies
		want := backend.BoundsCheckPolicies{
			Index:        backend.BoundsCheckRestrict,
			Buffer:       c.buffer,
			ImageLoad:    c.image,
			BindingArray: backend.BoundsCheckUnchecked,
		}
		if got != want {
			t.Errorf("%s: bounds checks %+v, want %+v", c.name, got, want)
		}
	}
}

// TestRobustnessEnabledWhereSupported checks that Open enables the
// robustness features the device supports whatever was requested, with
// their extensions below the version that promoted them.
func TestRobustnessEnabledWhereSupported(t *testing.T) {
	on := vk.Bool32(vk.True)
	s := extendedFeatureSupport{
		apiVersion:  vkMakeVersion(1, 2, 0),
		core:        vk.PhysicalDeviceFeatures{RobustBufferAccess: on},
		imageRobust: &vk.PhysicalDeviceImageRobustnessFeatures{RobustImageAccess: on},
		robustness2: &vk.PhysicalDeviceRobustness2FeaturesKHR{RobustBufferAccess2: on, RobustImageAccess2: on, NullDescriptor: on},
	}
	var c deviceFeatureChain
	pNext, extensions := s.enable(&c, 0, false)
	if pNext == nil {
		t.Fatal("no feature struct chained")
	}
	if c.core.RobustBufferAccess != on {
		t.Error("core robustBufferAccess not enabled")
	}
	if c.imageRobust.RobustImageAccess != on || c.robustness2.RobustBufferAccess2 != on || c.robustness2.RobustImageAccess2 != on {
		t.Errorf("robustness features not enabled: %+v %+v", c.imageRobust, c.robustness2)
	}
	if c.robustness2.NullDescriptor != 0 {
		t.Error("nullDescriptor enabled: nothing uses it")
	}
	want := map[string]bool{extImageRobustness + "\x00": true, extRobustness2 + "\x00": true}
	for _, e := range extensions {
		delete(want, e)
	}
	if len(want) != 0 {
		t.Errorf("extensions %q miss %v", extensions, want)
	}

	var none deviceFeatureChain
	if _, ext := (&extendedFeatureSupport{apiVersion: vkMakeVersion(1, 2, 0)}).enable(&none, 0, false); len(ext) != 0 {
		t.Errorf("device without robustness enables %q", ext)
	}
}

// TestIntelRobustness2Exclusion pins Rust wgpu-hal's
// is_intel_igpu_outdated_for_robustness2 (gfx-rs/wgpu#4599).
func TestIntelRobustness2Exclusion(t *testing.T) {
	const good = 101<<14 | 2115
	igpu := func(vendor uint32, typ vk.PhysicalDeviceType, version uint32) *vk.PhysicalDeviceProperties {
		return &vk.PhysicalDeviceProperties{VendorID: vendor, DeviceType: typ, DriverVersion: version}
	}
	for _, c := range []struct {
		name   string
		props  *vk.PhysicalDeviceProperties
		driver vk.DriverId
		want   bool
	}{
		{"outdated Intel iGPU, Windows driver", igpu(0x8086, vk.PhysicalDeviceTypeIntegratedGpu, good-1), vk.DriverIdIntelProprietaryWindows, true},
		{"current Intel iGPU", igpu(0x8086, vk.PhysicalDeviceTypeIntegratedGpu, good), vk.DriverIdIntelProprietaryWindows, false},
		{"outdated, Mesa ANV", igpu(0x8086, vk.PhysicalDeviceTypeIntegratedGpu, good-1), vk.DriverIdIntelOpenSourceMesa, false},
		{"outdated, driver unknown", igpu(0x8086, vk.PhysicalDeviceTypeIntegratedGpu, good-1), 0, false},
		{"Intel discrete", igpu(0x8086, vk.PhysicalDeviceTypeDiscreteGpu, good-1), vk.DriverIdIntelProprietaryWindows, false},
		{"other vendor", igpu(0x10de, vk.PhysicalDeviceTypeIntegratedGpu, good-1), vk.DriverIdIntelProprietaryWindows, false},
	} {
		if got := intelIGPUOutdatedForRobustness2(c.props, c.driver); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSPIRVVersion(t *testing.T) {
	for _, c := range []struct {
		api  uint32
		want spirv.Version
	}{
		{vkMakeVersion(1, 0, 0), spirv.Version1_0},
		{vkMakeVersion(1, 1, 0), spirv.Version1_3},
		{vkMakeVersion(1, 2, 0), spirv.Version1_5},
		{vkMakeVersion(1, 3, 0), spirv.Version1_6},
		{vkMakeVersion(1, 4, 0), spirv.Version1_6},
	} {
		if got := spirvVersion(c.api); got != c.want {
			t.Errorf("spirvVersion(%d.%d) = %v, want %v", vkVersionMajor(c.api), vkVersionMinor(c.api), got, c.want)
		}
	}
}
