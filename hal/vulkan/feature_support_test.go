//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

func float16Int8(shaderFloat16 bool) *vk.PhysicalDeviceShaderFloat16Int8Features {
	f := &vk.PhysicalDeviceShaderFloat16Int8Features{}
	if shaderFloat16 {
		f.ShaderFloat16 = vk.Bool32(vk.True)
	}
	return f
}

func storage16(storageBuffer, uniformAndStorage bool) *vk.PhysicalDevice16BitStorageFeatures {
	f := &vk.PhysicalDevice16BitStorageFeatures{}
	if storageBuffer {
		f.StorageBuffer16BitAccess = vk.Bool32(vk.True)
	}
	if uniformAndStorage {
		f.UniformAndStorageBuffer16BitAccess = vk.Bool32(vk.True)
	}
	return f
}

// TestExtendedFeatureSupportShaderF16 checks the f16 decision of Rust
// wgpu-hal (vulkan/adapter.rs, PhysicalDeviceFeatures::to_wgpu): shaderFloat16
// and both 16-bit buffer access features, each struct reported by the device.
func TestExtendedFeatureSupportShaderF16(t *testing.T) {
	tests := []struct {
		name string
		s    extendedFeatureSupport
		want bool
	}{
		{"all three", extendedFeatureSupport{float16Int8: float16Int8(true), storage16: storage16(true, true)}, true},
		{"no shaderFloat16", extendedFeatureSupport{float16Int8: float16Int8(false), storage16: storage16(true, true)}, false},
		{"no storageBuffer16BitAccess", extendedFeatureSupport{float16Int8: float16Int8(true), storage16: storage16(false, true)}, false},
		{"no uniformAndStorageBuffer16BitAccess", extendedFeatureSupport{float16Int8: float16Int8(true), storage16: storage16(true, false)}, false},
		{"float16_int8 struct not reportable", extendedFeatureSupport{storage16: storage16(true, true)}, false},
		{"16bit_storage struct not reportable", extendedFeatureSupport{float16Int8: float16Int8(true)}, false},
		{"nothing reportable", extendedFeatureSupport{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.s.webGPUFeatures(vk.QueueFamilyProperties{}).Contains(gputypes.FeatureShaderF16)
			if got != tt.want {
				t.Errorf("ShaderF16 advertised = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExtendedFeatureSupportTimestampQuery checks the timestamp decision of
// Rust wgpu-hal (vulkan/adapter.rs, PhysicalDeviceFeatures::to_wgpu): the
// queue family must write timestamps, which Vulkan reports as 36 to 64 valid
// bits, or 0.
func TestExtendedFeatureSupportTimestampQuery(t *testing.T) {
	tests := []struct {
		validBits uint32
		want      bool
	}{
		{0, false},
		{35, false},
		{36, true},
		{48, true},
		{64, true},
	}
	for _, tt := range tests {
		var s extendedFeatureSupport
		got := s.webGPUFeatures(vk.QueueFamilyProperties{TimestampValidBits: tt.validBits}).Contains(gputypes.FeatureTimestampQuery)
		if got != tt.want {
			t.Errorf("timestampValidBits %d: TimestampQuery advertised = %v, want %v", tt.validBits, got, tt.want)
		}
	}
}

// TestShaderF16Extensions checks the device extensions enabled for f16 at
// each API version, as Rust wgpu-hal requires them (vulkan/adapter.rs,
// PhysicalDeviceProperties::get_required_extensions).
func TestShaderF16Extensions(t *testing.T) {
	tests := []struct {
		apiVersion uint32
		want       []string
	}{
		{vkMakeVersion(1, 0, 0), []string{ext16BitStorage, extStorageBufferStorageClass, extShaderFloat16Int8}},
		{vkMakeVersion(1, 1, 0), []string{extShaderFloat16Int8}},
		{vkMakeVersion(1, 2, 0), nil},
		{vkMakeVersion(1, 3, 0), nil},
	}
	for _, tt := range tests {
		s := extendedFeatureSupport{apiVersion: tt.apiVersion}
		got := s.shaderF16Extensions()
		if len(got) != len(tt.want) {
			t.Fatalf("API %#x: extensions %q, want %q", tt.apiVersion, got, tt.want)
		}
		for i := range got {
			if got[i] != tt.want[i]+"\x00" {
				t.Errorf("API %#x: extension %d = %q, want %q", tt.apiVersion, i, got[i], tt.want[i]+"\x00")
			}
		}
	}
}
