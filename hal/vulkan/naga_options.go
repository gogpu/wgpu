//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"github.com/gogpu/gputypes"
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
//   - Discard: OpDemoteToHelperInvocation (WGSL's helper-invocation
//     semantics) where the device enabled shaderDemoteToHelperInvocation, as
//     Dawn lowers it; OpKill otherwise, which is Rust naga's only lowering.
//   - Loop bounding and ray-query init tracking keep naga's defaults, which
//     are what wgpu-hal passes.
//
// AllowedCapabilities depends on the features the device is opened with and
// is set by Open (spirvCapabilities).
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
		DemoteToHelperInvocation:      ext.demoteToHelperInvocation(),
	}
}

// SPIR-V capabilities that naga's spirv package does not name (values from
// the SPIR-V specification, Capability table; checked against the Vulkan
// SDK's spirv-headers/spirv.h).
const (
	capabilityStorageImageWriteWithoutFormat spirv.Capability = 56
	capabilityDrawParameters                 spirv.Capability = 4427
	capabilityRayQueryKHR                    spirv.Capability = 4472
	capabilityDotProductInputAll             spirv.Capability = 6016
	capabilityDotProductInput4x8Bit          spirv.Capability = 6017
)

// spirvCapabilities returns the SPIR-V capabilities a module may use on a
// device opened with requested, as Rust wgpu-hal lists them for naga
// (vulkan/adapter.rs, Adapter::open): the capabilities every Vulkan device
// accepts, then each one only where the device has what grants it. naga
// checks the set where it has a fallback — today the packed dot product,
// which it polyfills without the DotProduct capabilities — so a nil set
// (every capability) let it emit instructions a device without
// shaderIntegerDotProduct rejects (P2-VK-014).
//
// Rust entries for features our Vulkan backend has no counterpart for
// (multiview, non-uniform binding-array indexing, 64-bit texture atomics,
// ray-tracing pipelines, mesh shaders, cooperative matrices, Int8) are left
// out; they join the list with the feature. Demote-to-helper has no Rust
// entry (Rust naga never emits it) and is listed where the device enabled it.
func spirvCapabilities(requested gputypes.Features, downlevel gputypes.DownlevelFlags, ext *extendedFeatureSupport) map[spirv.Capability]struct{} {
	set := map[spirv.Capability]struct{}{}
	add := func(on bool, caps ...spirv.Capability) {
		if !on {
			return
		}
		for _, c := range caps {
			set[c] = struct{}{}
		}
	}
	// Vulkan spec, "SPIR-V Environment": always available.
	add(true, spirv.CapabilityShader, spirv.CapabilityMatrix, spirv.CapabilitySampled1D, spirv.CapabilityImage1D,
		spirv.CapabilityImageQuery, spirv.CapabilityDerivativeControl, spirv.CapabilityStorageImageExtendedFormats)
	add(downlevel.Contains(gputypes.DownlevelFlagsCubeArrayTextures), spirv.CapabilitySampledCubeArray)
	add(downlevel.Contains(gputypes.DownlevelFlagsMultisampledShading), spirv.CapabilitySampleRateShading)
	add(requested.Contains(gputypes.FeaturePrimitiveIndex), spirv.CapabilityGeometry)
	add(requested.Contains(gputypes.FeatureSubgroupOperations) || requested.Contains(gputypes.FeatureSubgroupVertex),
		spirv.CapabilityGroupNonUniform, spirv.CapabilityGroupNonUniformVote, spirv.CapabilityGroupNonUniformArithmetic,
		spirv.CapabilityGroupNonUniformBallot, spirv.CapabilityGroupNonUniformShuffle,
		spirv.CapabilityGroupNonUniformShuffleRel, spirv.CapabilityGroupNonUniformQuad)
	add(requested.Contains(gputypes.FeatureBGRA8UnormStorage), capabilityStorageImageWriteWithoutFormat)
	add(requested.Contains(gputypes.FeatureRayQuery), capabilityRayQueryKHR)
	add(requested.Contains(gputypes.FeatureShaderInt64), spirv.CapabilityInt64)
	add(requested.Contains(gputypes.FeatureShaderF16), spirv.CapabilityFloat16)
	add(requested.Contains(gputypes.FeatureShaderI16), spirv.CapabilityInt16)
	add(requested.Contains(gputypes.FeatureShaderInt64AtomicMinMax) || requested.Contains(gputypes.FeatureShaderInt64AtomicAllOps),
		spirv.CapabilityInt64Atomics)
	add(requested.Contains(gputypes.FeatureShaderFloat32Atomic), spirv.CapabilityAtomicFloat32AddEXT)
	add(requested.Contains(gputypes.FeatureClipDistances), spirv.CapabilityClipDistance)
	// Vulkan bundles barycentrics and per-vertex inputs under one feature.
	add(requested.Contains(gputypes.FeatureShaderBarycentrics) || requested.Contains(gputypes.FeatureShaderPerVertex),
		spirv.CapabilityFragmentBarycentricKHR)
	add(requested.Contains(gputypes.FeatureShaderDrawIndex), capabilityDrawParameters)
	// VK_KHR_shader_integer_dot_product's SPIR-V capabilities, enabled with
	// the feature wherever supported (extendedFeatureSupport.enable).
	add(ext.shaderIntegerDotProduct(), capabilityDotProductInputAll, capabilityDotProductInput4x8Bit,
		spirv.CapabilityDotProductInput4x8BitPacked, spirv.CapabilityDotProduct)
	add(ext.demoteToHelperInvocation(), spirv.CapabilityDemoteToHelperInvocation)
	return set
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
