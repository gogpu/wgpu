//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

func features(fs ...gputypes.Feature) gputypes.Features {
	var r gputypes.Features
	for _, f := range fs {
		r.Insert(f)
	}
	return r
}

// TestShaderCapabilityFeatures checks each shader capability feature against
// the Vulkan feature it comes from, as Rust wgpu-hal detects it
// (vulkan/adapter.rs, PhysicalDeviceFeatures::to_wgpu), with Rust's groups.
func TestShaderCapabilityFeatures(t *testing.T) {
	on := vk.Bool32(vk.True)
	tests := []struct {
		name string
		s    extendedFeatureSupport
		want gputypes.Features
	}{
		{"nothing", extendedFeatureSupport{}, 0},
		{"geometryShader", extendedFeatureSupport{core: vk.PhysicalDeviceFeatures{GeometryShader: on}}, features(gputypes.FeaturePrimitiveIndex)},
		{"shaderClipDistance", extendedFeatureSupport{core: vk.PhysicalDeviceFeatures{ShaderClipDistance: on}}, features(gputypes.FeatureClipDistances)},
		{"shaderInt64", extendedFeatureSupport{core: vk.PhysicalDeviceFeatures{ShaderInt64: on}}, features(gputypes.FeatureShaderInt64)},
		{
			"64-bit atomics on buffers and workgroup memory",
			extendedFeatureSupport{atomicInt64: &vk.PhysicalDeviceShaderAtomicInt64Features{ShaderBufferInt64Atomics: on, ShaderSharedInt64Atomics: on}},
			features(gputypes.FeatureShaderInt64AtomicMinMax, gputypes.FeatureShaderInt64AtomicAllOps),
		},
		{"64-bit atomics on buffers only", extendedFeatureSupport{atomicInt64: &vk.PhysicalDeviceShaderAtomicInt64Features{ShaderBufferInt64Atomics: on}}, 0},
		{
			"f32 atomic add",
			extendedFeatureSupport{atomicFloat: &vk.PhysicalDeviceShaderAtomicFloatFeaturesEXT{ShaderBufferFloat32Atomics: on, ShaderBufferFloat32AtomicAdd: on}},
			features(gputypes.FeatureShaderFloat32Atomic),
		},
		{"f32 atomics without add", extendedFeatureSupport{atomicFloat: &vk.PhysicalDeviceShaderAtomicFloatFeaturesEXT{ShaderBufferFloat32Atomics: on}}, 0},
		{
			"fragmentShaderBarycentric",
			extendedFeatureSupport{barycentric: &vk.PhysicalDeviceFragmentShaderBarycentricFeaturesKHR{FragmentShaderBarycentric: on}},
			features(gputypes.FeatureShaderBarycentrics, gputypes.FeatureShaderPerVertex),
		},
		{"shaderDrawParameters", extendedFeatureSupport{drawParams: &vk.PhysicalDeviceShaderDrawParametersFeatures{ShaderDrawParameters: on}}, features(gputypes.FeatureShaderDrawIndex)},
		{
			"shaderDrawParameters struct says no",
			extendedFeatureSupport{drawParams: &vk.PhysicalDeviceShaderDrawParametersFeatures{}, extensions: map[string]bool{extShaderDrawParameters: true}},
			0,
		},
		{"draw parameters extension below Vulkan 1.1", extendedFeatureSupport{extensions: map[string]bool{extShaderDrawParameters: true}}, features(gputypes.FeatureShaderDrawIndex)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.want | features(gputypes.FeatureShaderEarlyDepthTest) // always, as Rust
			if got := tt.s.shaderCapabilityFeatures(); got != want {
				t.Errorf("shaderCapabilityFeatures() = %v, want %v", got, want)
			}
		})
	}
}

// TestEnableNothingRequested checks that the gated core features are off
// without a request and the other core features keep their values.
func TestEnableNothingRequested(t *testing.T) {
	on := vk.Bool32(vk.True)
	s := extendedFeatureSupport{
		apiVersion: vkMakeVersion(1, 2, 0),
		core:       vk.PhysicalDeviceFeatures{GeometryShader: on, ShaderClipDistance: on, ShaderInt64: on, ShaderInt16: on, SamplerAnisotropy: on},
	}
	var c deviceFeatureChain
	pNext, exts := s.enable(&c, 0, false)
	if pNext != nil || len(exts) != 0 {
		t.Fatalf("pNext %v, extensions %q, want none", pNext, exts)
	}
	if c.core.GeometryShader != 0 || c.core.ShaderClipDistance != 0 || c.core.ShaderInt64 != 0 || c.core.ShaderInt16 != 0 {
		t.Errorf("gated core features enabled without a request: %+v", c.core)
	}
	if c.core.SamplerAnisotropy == 0 {
		t.Error("an ungated core feature was turned off")
	}

	s.enable(&c, features(gputypes.FeaturePrimitiveIndex, gputypes.FeatureClipDistances, gputypes.FeatureShaderInt64), false)
	if c.core.GeometryShader == 0 || c.core.ShaderClipDistance == 0 || c.core.ShaderInt64 == 0 || c.core.ShaderInt16 != 0 {
		t.Errorf("core features do not follow the request: %+v", c.core)
	}
}

// TestEnableInt64Atomics checks that on Vulkan 1.2 the 64-bit atomics go into
// the Vulkan 1.2 struct (VUID-VkDeviceCreateInfo-pNext-02830) and below it
// into the KHR struct with its extension.
func TestEnableInt64Atomics(t *testing.T) {
	s := extendedFeatureSupport{apiVersion: vkMakeVersion(1, 2, 0)}
	var c deviceFeatureChain
	pNext, exts := s.enable(&c, features(gputypes.FeatureShaderInt64AtomicMinMax), false)
	if pNext != (*uintptr)(unsafe.Pointer(&c.vulkan12)) || c.vulkan12.ShaderBufferInt64Atomics == 0 || c.vulkan12.ShaderSharedInt64Atomics == 0 {
		t.Fatalf("Vulkan 1.2: 64-bit atomics not in the Vulkan 1.2 struct: %+v", c.vulkan12)
	}
	if c.atomicInt64.SType != 0 || len(exts) != 0 {
		t.Errorf("Vulkan 1.2: KHR struct or extension used: %+v %q", c.atomicInt64, exts)
	}

	s = extendedFeatureSupport{apiVersion: vkMakeVersion(1, 1, 0)}
	c = deviceFeatureChain{}
	pNext, exts = s.enable(&c, features(gputypes.FeatureShaderInt64AtomicAllOps), false)
	if pNext != (*uintptr)(unsafe.Pointer(&c.atomicInt64)) || c.atomicInt64.ShaderBufferInt64Atomics == 0 {
		t.Fatalf("Vulkan 1.1: KHR 64-bit atomics struct not chained: %+v", c.atomicInt64)
	}
	if len(exts) != 1 || exts[0] != extShaderAtomicInt64+"\x00" {
		t.Errorf("Vulkan 1.1: extensions %q, want %s", exts, extShaderAtomicInt64)
	}
}

// TestEnableFeatureStructChain checks the pNext chain and extensions for the
// struct-backed features, and the timeline semaphore staying on.
func TestEnableFeatureStructChain(t *testing.T) {
	s := extendedFeatureSupport{apiVersion: vkMakeVersion(1, 2, 0)}
	var c deviceFeatureChain
	pNext, exts := s.enable(&c, features(gputypes.FeatureShaderFloat32Atomic, gputypes.FeatureShaderPerVertex, gputypes.FeatureShaderDrawIndex), true)
	structs := []unsafe.Pointer{unsafe.Pointer(&c.vulkan12), unsafe.Pointer(&c.atomicFloat), unsafe.Pointer(&c.barycentric), unsafe.Pointer(&c.drawParams)}
	links := []*uintptr{pNext, c.vulkan12.PNext, c.atomicFloat.PNext, c.barycentric.PNext}
	for i := range structs {
		if links[i] != (*uintptr)(structs[i]) {
			t.Fatalf("link %d of the pNext chain is wrong", i)
		}
	}
	if c.drawParams.PNext != nil {
		t.Error("chain does not end at the draw parameters struct")
	}
	if c.vulkan12.TimelineSemaphore == 0 || c.atomicFloat.ShaderBufferFloat32AtomicAdd == 0 ||
		c.barycentric.FragmentShaderBarycentric == 0 || c.drawParams.ShaderDrawParameters == 0 {
		t.Errorf("a requested feature is off: %+v %+v %+v %+v", c.vulkan12, c.atomicFloat, c.barycentric, c.drawParams)
	}
	want := []string{extShaderAtomicFloat + "\x00", extFragmentShaderBarycentric + "\x00"}
	if len(exts) != len(want) || exts[0] != want[0] || exts[1] != want[1] {
		t.Errorf("extensions %q, want %q", exts, want)
	}

	s = extendedFeatureSupport{apiVersion: vkMakeVersion(1, 0, 0)}
	c = deviceFeatureChain{}
	pNext, exts = s.enable(&c, features(gputypes.FeatureShaderDrawIndex), false)
	if pNext != nil || len(exts) != 1 || exts[0] != extShaderDrawParameters+"\x00" {
		t.Errorf("Vulkan 1.0 draw index: pNext %v, extensions %q, want only %s", pNext, exts, extShaderDrawParameters)
	}
}

// TestUnsupportedRequest checks that Open refuses enableable features the
// adapter does not offer and leaves other features to core.
func TestUnsupportedRequest(t *testing.T) {
	s := extendedFeatureSupport{core: vk.PhysicalDeviceFeatures{GeometryShader: vk.Bool32(vk.True)}}
	requested := features(gputypes.FeaturePrimitiveIndex, gputypes.FeatureShaderInt64, gputypes.FeatureTimestampQuery)
	if got, want := s.unsupportedRequest(requested), features(gputypes.FeatureShaderInt64); got != want {
		t.Errorf("unsupportedRequest() = %v, want %v", got, want)
	}
}
