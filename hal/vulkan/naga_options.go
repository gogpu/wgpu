//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"github.com/gogpu/naga/backend"
	"github.com/gogpu/naga/spirv"
)

// instanceAPIVersion is the Vulkan version the instance is created with. A
// device is used at the lower of this and its own version.
var instanceAPIVersion = vkMakeVersion(1, 2, 0)

// spirvOptions returns the naga SPIR-V options for a device that reports
// deviceAPIVersion and has the extended features ext, as Rust wgpu-hal builds
// them per adapter (vulkan/adapter.rs, Adapter::open's naga_options).
//
//   - Version: the highest SPIR-V version the device accepts at the API
//     version it is used with. naga never raises it.
//   - ForcePointSize: a point-list pipeline must write PointSize; Rust sets
//     it for every pipeline.
//   - AdjustCoordinateSpace stays off: we flip y with a negative viewport
//     height, and Rust sets it only for devices that ignore one
//     (IGNORED_NEGATIVE_VIEWPORT_HEIGHT), a workaround we do not have.
//   - Bounds checks: Restrict on indices always. Buffer accesses and image
//     loads are Restrict unless the device checks them itself: Unchecked
//     with robustBufferAccess2 and with robust image access, which Open
//     enables wherever supported. Binding arrays are Unchecked, as in Rust.
//   - Workgroup zeroing: Native (null initializers the driver honors) where
//     the device enabled shaderZeroInitializeWorkgroupMemory, Polyfill (zero
//     stores in the shader) otherwise. A compute stage that turns zeroing
//     off gets None (stageSPIRVOptions).
//   - Loop bounding and ray-query init tracking keep naga's defaults, which
//     are what wgpu-hal passes.
func spirvOptions(deviceAPIVersion uint32, ext *extendedFeatureSupport) spirv.Options {
	zeroInit := backend.ZeroInitWorkgroupPolyfill
	if ext.zeroInitializeWorkgroupMemory() {
		zeroInit = backend.ZeroInitWorkgroupNative
	}
	return spirv.Options{
		Version:        spirvVersion(min(deviceAPIVersion, instanceAPIVersion)),
		ForcePointSize: true,
		BoundsCheckPolicies: backend.BoundsCheckPolicies{
			Index:        backend.BoundsCheckRestrict,
			Buffer:       checkedUnless(ext.robustBufferAccess2()),
			ImageLoad:    checkedUnless(ext.robustImageAccess()),
			BindingArray: backend.BoundsCheckUnchecked,
		},
		ZeroInitializeWorkgroupMemory: zeroInit,
	}
}

// stageSPIRVOptions returns device's SPIR-V options for one pipeline stage:
// a stage with zeroInitializeWorkgroupMemory false compiles with naga's None
// mode, as Rust wgpu-hal's compile_stage does for
// ProgrammableStage::zero_initialize_workgroup_memory (vulkan/device.rs).
func stageSPIRVOptions(device spirv.Options, zeroInitializeWorkgroupMemory bool) spirv.Options {
	if !zeroInitializeWorkgroupMemory {
		device.ZeroInitializeWorkgroupMemory = backend.ZeroInitWorkgroupNone
	}
	return device
}

// checkedUnless returns Unchecked when the device performs the bounds check
// itself and Restrict otherwise.
func checkedUnless(deviceChecks bool) backend.BoundsCheckPolicy {
	if deviceChecks {
		return backend.BoundsCheckUnchecked
	}
	return backend.BoundsCheckRestrict
}

// spirvVersion returns the highest SPIR-V version that Vulkan apiVersion
// requires every device to accept (Vulkan spec, appendix "Vulkan
// Environment for SPIR-V"): 1.0 → 1.0, 1.1 → 1.3, 1.2 → 1.5, 1.3+ → 1.6.
func spirvVersion(apiVersion uint32) spirv.Version {
	major, minor := vkVersionMajor(apiVersion)&0x7F, vkVersionMinor(apiVersion)
	switch {
	case major > 1 || minor >= 3:
		return spirv.Version1_6
	case minor == 2:
		return spirv.Version1_5
	case minor == 1:
		return spirv.Version1_3
	default:
		return spirv.Version1_0
	}
}
