// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package vulkan

import (
	"strings"
	"testing"

	"github.com/gogpu/wgpu/hal/vulkan/vk"
	"github.com/gogpu/wgpu/types"
)

// TestVkMakeVersion tests Vulkan version creation.
func TestVkMakeVersion(t *testing.T) {
	tests := []struct {
		name                   string
		major, minor, patch    uint32
		expectedMajorExtracted uint32
		expectedMinorExtracted uint32
		expectedPatchExtracted uint32
	}{
		{"Vulkan 1.0.0", 1, 0, 0, 1, 0, 0},
		{"Vulkan 1.2.0", 1, 2, 0, 1, 2, 0},
		{"Vulkan 1.3.0", 1, 3, 0, 1, 3, 0},
		{"Vulkan 1.3.256", 1, 3, 256, 1, 3, 256},
		{"Vulkan 2.0.0", 2, 0, 0, 2, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version := vkMakeVersion(tt.major, tt.minor, tt.patch)

			gotMajor := vkVersionMajor(version)
			gotMinor := vkVersionMinor(version)
			gotPatch := vkVersionPatch(version)

			if gotMajor != tt.expectedMajorExtracted {
				t.Errorf("vkVersionMajor() = %d, want %d", gotMajor, tt.expectedMajorExtracted)
			}
			if gotMinor != tt.expectedMinorExtracted {
				t.Errorf("vkVersionMinor() = %d, want %d", gotMinor, tt.expectedMinorExtracted)
			}
			if gotPatch != tt.expectedPatchExtracted {
				t.Errorf("vkVersionPatch() = %d, want %d", gotPatch, tt.expectedPatchExtracted)
			}
		})
	}
}

// TestVkVersionMajor tests major version extraction.
func TestVkVersionMajor(t *testing.T) {
	tests := []struct {
		version uint32
		expect  uint32
	}{
		{vkMakeVersion(1, 0, 0), 1},
		{vkMakeVersion(2, 0, 0), 2},
		{vkMakeVersion(3, 5, 10), 3},
		{vkMakeVersion(10, 20, 30), 10},
	}

	for _, tt := range tests {
		got := vkVersionMajor(tt.version)
		if got != tt.expect {
			t.Errorf("vkVersionMajor(%d) = %d, want %d", tt.version, got, tt.expect)
		}
	}
}

// TestVkVersionMinor tests minor version extraction.
func TestVkVersionMinor(t *testing.T) {
	tests := []struct {
		version uint32
		expect  uint32
	}{
		{vkMakeVersion(1, 0, 0), 0},
		{vkMakeVersion(1, 2, 0), 2},
		{vkMakeVersion(1, 3, 0), 3},
		{vkMakeVersion(3, 15, 10), 15},
	}

	for _, tt := range tests {
		got := vkVersionMinor(tt.version)
		if got != tt.expect {
			t.Errorf("vkVersionMinor(%d) = %d, want %d", tt.version, got, tt.expect)
		}
	}
}

// TestVkVersionPatch tests patch version extraction.
func TestVkVersionPatch(t *testing.T) {
	tests := []struct {
		version uint32
		expect  uint32
	}{
		{vkMakeVersion(1, 0, 0), 0},
		{vkMakeVersion(1, 0, 10), 10},
		{vkMakeVersion(1, 2, 255), 255},
		{vkMakeVersion(3, 5, 1000), 1000},
	}

	for _, tt := range tests {
		got := vkVersionPatch(tt.version)
		if got != tt.expect {
			t.Errorf("vkVersionPatch(%d) = %d, want %d", tt.version, got, tt.expect)
		}
	}
}

// TestCStringToGo tests C string to Go string conversion.
func TestCStringToGo(t *testing.T) {
	tests := []struct {
		name   string
		input  []byte
		expect string
	}{
		{
			"Empty string",
			[]byte{0},
			"",
		},
		{
			"Simple string",
			[]byte{'H', 'e', 'l', 'l', 'o', 0},
			"Hello",
		},
		{
			"String with space",
			[]byte{'H', 'e', 'l', 'l', 'o', ' ', 'W', 'o', 'r', 'l', 'd', 0},
			"Hello World",
		},
		{
			"String with extra null",
			[]byte{'T', 'e', 's', 't', 0, 'X', 'Y', 'Z', 0},
			"Test",
		},
		{
			"No null terminator",
			[]byte{'A', 'B', 'C'},
			"ABC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cStringToGo(tt.input)
			if got != tt.expect {
				t.Errorf("cStringToGo() = %q, want %q", got, tt.expect)
			}
		})
	}
}

// TestVendorIDToName tests vendor ID to name mapping.
func TestVendorIDToName(t *testing.T) {
	tests := []struct {
		vendorID uint32
		expect   string
	}{
		{0x1002, "AMD"},
		{0x10DE, "NVIDIA"},
		{0x8086, "Intel"},
		{0x5143, "Qualcomm"},
		{0x1010, "ImgTec"},
		{0x13B5, "ARM"},
		{0x106B, "0x106B"}, // Apple not in map - returns hex
		{0x9999, "0x9999"}, // Unknown vendor - returns hex
		{0x0000, "0x0000"}, // Unknown vendor - returns hex
	}

	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			got := vendorIDToName(tt.vendorID)
			if got != tt.expect {
				t.Errorf("vendorIDToName(0x%X) = %q, want %q", tt.vendorID, got, tt.expect)
			}
		})
	}
}

// TestLimitsFromProps tests limits extraction from Vulkan properties.
// NOTE: Currently returns DefaultLimits() - this test verifies current behavior.
func TestLimitsFromProps(t *testing.T) {
	props := &vk.PhysicalDeviceProperties{
		Limits: vk.PhysicalDeviceLimits{
			MaxImageDimension2D:                   4096,
			MaxImageDimension3D:                   2048,
			MaxImageDimensionCube:                 4096,
			MaxImageArrayLayers:                   2048,
			MaxTexelBufferElements:                128 * 1024 * 1024,
			MaxUniformBufferRange:                 65536,
			MaxStorageBufferRange:                 2 << 27,
			MaxPushConstantsSize:                  256,
			MaxMemoryAllocationCount:              4096,
			MaxSamplerAllocationCount:             4000,
			BufferImageGranularity:                0x20000,
			SparseAddressSpaceSize:                0x80000000,
			MaxBoundDescriptorSets:                4,
			MaxPerStageDescriptorSamplers:         16,
			MaxPerStageDescriptorUniformBuffers:   15,
			MaxPerStageDescriptorStorageBuffers:   16,
			MaxPerStageDescriptorSampledImages:    16,
			MaxPerStageDescriptorStorageImages:    8,
			MaxPerStageDescriptorInputAttachments: 8,
			MaxPerStageResources:                  128,
			MaxDescriptorSetSamplers:              80,
			MaxDescriptorSetUniformBuffers:        90,
			MaxDescriptorSetUniformBuffersDynamic: 8,
			MaxDescriptorSetStorageBuffers:        96,
			MaxDescriptorSetStorageBuffersDynamic: 8,
			MaxDescriptorSetSampledImages:         96,
			MaxDescriptorSetStorageImages:         48,
			MaxDescriptorSetInputAttachments:      8,
			MaxVertexInputAttributes:              32,
			MaxVertexInputBindings:                32,
			MaxVertexInputAttributeOffset:         2047,
			MaxVertexInputBindingStride:           2048,
			MaxVertexOutputComponents:             128,
			MaxFragmentInputComponents:            128,
			MaxFragmentOutputAttachments:          8,
			MaxFragmentDualSrcAttachments:         1,
			MaxFragmentCombinedOutputResources:    16,
			MaxComputeSharedMemorySize:            32768,
			MaxComputeWorkGroupCount:              [3]uint32{65535, 65535, 65535},
			MaxComputeWorkGroupInvocations:        1024,
			MaxComputeWorkGroupSize:               [3]uint32{1024, 1024, 64},
			SubPixelPrecisionBits:                 8,
			SubTexelPrecisionBits:                 8,
			MipmapPrecisionBits:                   8,
			MaxDrawIndexedIndexValue:              0xFFFFFFFF,
			MaxDrawIndirectCount:                  0xFFFFFFFF,
			MaxSamplerLodBias:                     15,
			MaxSamplerAnisotropy:                  16,
			MaxViewports:                          16,
			MaxViewportDimensions:                 [2]uint32{16384, 16384},
			ViewportBoundsRange:                   [2]float32{-32768.0, 32767.0},
			ViewportSubPixelBits:                  8,
			MinMemoryMapAlignment:                 64,
			MinTexelBufferOffsetAlignment:         0x10,
			MinUniformBufferOffsetAlignment:       0x100,
			MinStorageBufferOffsetAlignment:       0x10,
			MinTexelOffset:                        -8,
			MaxTexelOffset:                        7,
			MinTexelGatherOffset:                  -32,
			MaxTexelGatherOffset:                  31,
			MinInterpolationOffset:                -0.5,
			MaxInterpolationOffset:                0.4375,
			SubPixelInterpolationOffsetBits:       4,
			MaxFramebufferWidth:                   16384,
			MaxFramebufferHeight:                  16384,
			MaxFramebufferLayers:                  2048,
			FramebufferColorSampleCounts:          0x7F,
			FramebufferDepthSampleCounts:          0x7F,
			FramebufferStencilSampleCounts:        0x7F,
			FramebufferNoAttachmentsSampleCounts:  0x7F,
			MaxColorAttachments:                   8,
			SampledImageColorSampleCounts:         0x7F,
			SampledImageIntegerSampleCounts:       0x7F,
			SampledImageDepthSampleCounts:         0x7F,
			SampledImageStencilSampleCounts:       0x7F,
			StorageImageSampleCounts:              0x7F,
			MaxSampleMaskWords:                    1,
			TimestampComputeAndGraphics:           1,
			TimestampPeriod:                       1.0,
			MaxClipDistances:                      8,
			MaxCullDistances:                      8,
			MaxCombinedClipAndCullDistances:       8,
			DiscreteQueuePriorities:               2,
			PointSizeRange:                        [2]float32{1.0, 64.0},
			LineWidthRange:                        [2]float32{1.0, 8.0},
			PointSizeGranularity:                  0.125,
			LineWidthGranularity:                  0.125,
			StrictLines:                           0,
			StandardSampleLocations:               1,
			OptimalBufferCopyOffsetAlignment:      0x10,
			OptimalBufferCopyRowPitchAlignment:    0x10,
			NonCoherentAtomSize:                   256,
		},
	}

	limits := limitsFromProps(props)

	// Currently returns DefaultLimits() - test that it returns valid defaults
	defaultLimits := types.DefaultLimits()

	if limits.MaxTextureDimension2D != defaultLimits.MaxTextureDimension2D {
		t.Errorf("MaxTextureDimension2D = %d, want %d", limits.MaxTextureDimension2D, defaultLimits.MaxTextureDimension2D)
	}
	if limits.MaxTextureDimension3D != defaultLimits.MaxTextureDimension3D {
		t.Errorf("MaxTextureDimension3D = %d, want %d", limits.MaxTextureDimension3D, defaultLimits.MaxTextureDimension3D)
	}
	if limits.MaxUniformBufferBindingSize != defaultLimits.MaxUniformBufferBindingSize {
		t.Errorf("MaxUniformBufferBindingSize = %d, want %d", limits.MaxUniformBufferBindingSize, defaultLimits.MaxUniformBufferBindingSize)
	}
}

// TestLimitsFromPropsMinimal tests that function returns valid limits.
func TestLimitsFromPropsMinimal(t *testing.T) {
	props := &vk.PhysicalDeviceProperties{
		Limits: vk.PhysicalDeviceLimits{
			MaxImageDimension2D:   256,
			MaxImageDimension3D:   128,
			MaxUniformBufferRange: 16384,
		},
	}

	limits := limitsFromProps(props)

	// Currently returns DefaultLimits() - verify it's not zero
	if limits.MaxTextureDimension2D == 0 {
		t.Error("MaxTextureDimension2D should not be zero")
	}
	if limits.MaxTextureDimension3D == 0 {
		t.Error("MaxTextureDimension3D should not be zero")
	}
	if limits.MaxUniformBufferBindingSize == 0 {
		t.Error("MaxUniformBufferBindingSize should not be zero")
	}
}

// TestBackendVariant tests Backend Variant method.
func TestBackendVariant(t *testing.T) {
	backend := &Backend{}
	variant := backend.Variant()

	if variant != types.BackendVulkan {
		t.Errorf("Variant() = %v, want BackendVulkan", variant)
	}
}

// TestVkMakeVersionRoundtrip tests version encoding/decoding roundtrip.
func TestVkMakeVersionRoundtrip(t *testing.T) {
	tests := []struct {
		major uint32
		minor uint32
		patch uint32
	}{
		{0, 0, 0},
		{1, 0, 0},
		{1, 2, 3},
		{2, 5, 10},
		{10, 20, 30},
		{127, 1023, 4095}, // Max reasonable values
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			version := vkMakeVersion(tt.major, tt.minor, tt.patch)

			major := vkVersionMajor(version)
			minor := vkVersionMinor(version)
			patch := vkVersionPatch(version)

			if major != tt.major {
				t.Errorf("major = %d, want %d", major, tt.major)
			}
			if minor != tt.minor {
				t.Errorf("minor = %d, want %d", minor, tt.minor)
			}
			if patch != tt.patch {
				t.Errorf("patch = %d, want %d", patch, tt.patch)
			}
		})
	}
}

// TestCStringToGoEmpty tests empty string conversion.
func TestCStringToGoEmpty(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"Just null", []byte{0}},
		{"Multiple nulls", []byte{0, 0, 0}},
		{"Empty slice", []byte{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cStringToGo(tt.input)
			if got != "" {
				t.Errorf("cStringToGo() = %q, want empty string", got)
			}
		})
	}
}

// TestCStringToGoSpecialChars tests special character handling.
func TestCStringToGoSpecialChars(t *testing.T) {
	tests := []struct {
		name   string
		input  []byte
		expect string
	}{
		{
			"Numbers",
			[]byte{'1', '2', '3', '4', '5', 0},
			"12345",
		},
		{
			"Punctuation",
			[]byte{'.', ',', '!', '?', 0},
			".,!?",
		},
		{
			"Path separator",
			[]byte{'/', 't', 'e', 's', 't', 0},
			"/test",
		},
		{
			"Device name",
			[]byte{'N', 'V', 'I', 'D', 'I', 'A', ' ', 'R', 'T', 'X', ' ', '4', '0', '9', '0', 0},
			"NVIDIA RTX 4090",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cStringToGo(tt.input)
			if got != tt.expect {
				t.Errorf("cStringToGo() = %q, want %q", got, tt.expect)
			}
		})
	}
}

// TestVendorIDToNameCoverage tests that all vendor IDs map correctly.
func TestVendorIDToNameCoverage(t *testing.T) {
	knownVendors := map[uint32]string{
		0x1002: "AMD",
		0x10DE: "NVIDIA",
		0x8086: "Intel",
		0x5143: "Qualcomm",
		0x1010: "ImgTec",
		0x13B5: "ARM",
	}

	for id, name := range knownVendors {
		got := vendorIDToName(id)
		if got != name {
			t.Errorf("vendorIDToName(0x%X) = %q, want %q", id, got, name)
		}

		// Test that name contains expected substring
		if !strings.Contains(got, name) {
			t.Errorf("vendorIDToName(0x%X) = %q, should contain %q", id, got, name)
		}
	}
}

// TestLimitsAllFields tests that all limits fields are populated.
func TestLimitsAllFields(t *testing.T) {
	props := &vk.PhysicalDeviceProperties{
		Limits: vk.PhysicalDeviceLimits{
			MaxImageDimension2D:                 8192,
			MaxTexelBufferElements:              128 * 1024 * 1024,
			MaxUniformBufferRange:               65536,
			MaxStorageBufferRange:               1 << 30,
			MaxVertexInputAttributes:            32,
			MaxVertexInputBindings:              32,
			MaxColorAttachments:                 8,
			MaxComputeWorkGroupSize:             [3]uint32{1024, 1024, 64},
			MaxComputeWorkGroupInvocations:      1024,
			MinMemoryMapAlignment:               64,
			MinUniformBufferOffsetAlignment:     256,
			MinStorageBufferOffsetAlignment:     32,
			MaxSamplerAnisotropy:                16,
			MaxViewports:                        16,
			MaxBoundDescriptorSets:              4,
			MaxPerStageDescriptorSamplers:       16,
			MaxPerStageDescriptorUniformBuffers: 15,
			MaxPerStageDescriptorStorageBuffers: 16,
			MaxPerStageDescriptorSampledImages:  16,
			MaxPerStageDescriptorStorageImages:  8,
			MaxDescriptorSetSamplers:            80,
			MaxDescriptorSetUniformBuffers:      90,
			MaxDescriptorSetStorageBuffers:      96,
			MaxDescriptorSetSampledImages:       96,
			MaxDescriptorSetStorageImages:       48,
			MaxPushConstantsSize:                256,
			MaxComputeSharedMemorySize:          32768,
			MaxComputeWorkGroupCount:            [3]uint32{65535, 65535, 65535},
			MaxImageDimension3D:                 2048,
			MaxImageArrayLayers:                 2048,
			MaxFramebufferWidth:                 8192,
			MaxFramebufferHeight:                8192,
			OptimalBufferCopyOffsetAlignment:    16,
			OptimalBufferCopyRowPitchAlignment:  16,
		},
	}

	limits := limitsFromProps(props)

	// Verify critical fields are set
	if limits.MaxTextureDimension2D == 0 {
		t.Error("MaxTextureDimension2D should not be 0")
	}
	if limits.MaxUniformBufferBindingSize == 0 {
		t.Error("MaxUniformBufferBindingSize should not be 0")
	}
	if limits.MaxStorageBufferBindingSize == 0 {
		t.Error("MaxStorageBufferBindingSize should not be 0")
	}
	if limits.MaxVertexAttributes == 0 {
		t.Error("MaxVertexAttributes should not be 0")
	}
	if limits.MaxVertexBuffers == 0 {
		t.Error("MaxVertexBuffers should not be 0")
	}
	if limits.MaxColorAttachments == 0 {
		t.Error("MaxColorAttachments should not be 0")
	}
	if limits.MaxComputeWorkgroupSizeX == 0 {
		t.Error("MaxComputeWorkgroupSizeX should not be 0")
	}
	if limits.MaxComputeInvocationsPerWorkgroup == 0 {
		t.Error("MaxComputeInvocationsPerWorkgroup should not be 0")
	}
}
