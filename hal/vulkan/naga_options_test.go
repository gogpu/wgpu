//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"testing"

	"github.com/gogpu/naga/backend"
	"github.com/gogpu/naga/spirv"
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
		opts := spirvOptions(c.device)
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
