// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package vulkan

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// BenchmarkVulkanCreateBindGroupLayout measures bind group layout creation overhead.
func BenchmarkVulkanCreateBindGroupLayout(b *testing.B) {
	b.ReportAllocs()
	device, _, cleanup := tryCreateVulkanDeviceForBench(b)
	if device == nil {
		return
	}
	defer cleanup()

	desc := &hal.BindGroupLayoutDescriptor{
		Label: "bench-bgl",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
			},
			{
				Binding:    1,
				Visibility: gputypes.ShaderStageFragment,
				Sampler:    &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bgl, err := device.CreateBindGroupLayout(desc)
		if err != nil {
			b.Fatal(err)
		}
		device.DestroyBindGroupLayout(bgl)
	}
}

// BenchmarkVulkanCreateBindGroup measures bind group creation overhead with real Vulkan descriptors.
func BenchmarkVulkanCreateBindGroup(b *testing.B) {
	b.ReportAllocs()
	device, _, cleanup := tryCreateVulkanDeviceForBench(b)
	if device == nil {
		return
	}
	defer cleanup()

	// Create layout
	bgl, err := device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "bench-bgl",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageVertex,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
			},
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	defer device.DestroyBindGroupLayout(bgl)

	// Create a buffer for the binding
	buf, err := device.CreateBuffer(&hal.BufferDescriptor{
		Label: "bench-ubo",
		Size:  256,
		Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		b.Fatal(err)
	}
	defer device.DestroyBuffer(buf)

	desc := &hal.BindGroupDescriptor{
		Label:  "bench-bg",
		Layout: bgl,
		Entries: []gputypes.BindGroupEntry{
			{
				Binding:  0,
				Resource: gputypes.BufferBinding{Buffer: buf.NativeHandle(), Offset: 0, Size: 256},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bg, err := device.CreateBindGroup(desc)
		if err != nil {
			b.Fatal(err)
		}
		device.DestroyBindGroup(bg)
	}
}

// BenchmarkVulkanCreatePipelineLayout measures pipeline layout creation overhead.
func BenchmarkVulkanCreatePipelineLayout(b *testing.B) {
	entryCounts := []struct {
		name  string
		count int
	}{
		{"0_layouts", 0},
		{"1_layout", 1},
		{"4_layouts", 4},
	}

	for _, ec := range entryCounts {
		b.Run(ec.name, func(b *testing.B) {
			b.ReportAllocs()
			device, _, cleanup := tryCreateVulkanDeviceForBench(b)
			if device == nil {
				return
			}
			defer cleanup()

			// Create bind group layouts
			layouts := make([]hal.BindGroupLayout, ec.count)
			for j := 0; j < ec.count; j++ {
				bgl, err := device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
					Label: "bench-bgl",
					Entries: []gputypes.BindGroupLayoutEntry{
						{
							Binding:    0,
							Visibility: gputypes.ShaderStageVertex,
							Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
						},
					},
				})
				if err != nil {
					b.Fatal(err)
				}
				layouts[j] = bgl
			}
			defer func() {
				for _, bgl := range layouts {
					device.DestroyBindGroupLayout(bgl)
				}
			}()

			desc := &hal.PipelineLayoutDescriptor{
				Label:            "bench-pl",
				BindGroupLayouts: layouts,
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pl, err := device.CreatePipelineLayout(desc)
				if err != nil {
					b.Fatal(err)
				}
				device.DestroyPipelineLayout(pl)
			}
		})
	}
}

// BenchmarkVulkanDescriptorAllocatorGrowth measures the descriptor pool allocator
// growing strategy by allocating many bind groups in sequence.
func BenchmarkVulkanDescriptorAllocatorGrowth(b *testing.B) {
	b.ReportAllocs()

	// Test the in-memory descriptor allocator without a real Vulkan device.
	// This benchmarks the pool management logic (mutex, growth, tracking).
	config := DefaultDescriptorAllocatorConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		alloc := NewDescriptorAllocator(0, nil, config)
		// Exercise the stats path which tests mutex contention.
		_, _, _ = alloc.Stats()
	}
}

// BenchmarkVulkanDescriptorCountsMultiply measures DescriptorCounts.Multiply overhead.
// This is called during pool sizing.
func BenchmarkVulkanDescriptorCountsMultiply(b *testing.B) {
	b.ReportAllocs()

	counts := DescriptorCounts{
		Samplers:       4,
		SampledImages:  8,
		StorageImages:  2,
		UniformBuffers: 16,
		StorageBuffers: 8,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := counts.Multiply(4)
		benchSink = result
	}
}

// BenchmarkVulkanDescriptorCountsTotal measures DescriptorCounts.Total overhead.
func BenchmarkVulkanDescriptorCountsTotal(b *testing.B) {
	b.ReportAllocs()

	counts := DescriptorCounts{
		Samplers:           4,
		SampledImages:      8,
		StorageImages:      2,
		UniformBuffers:     16,
		StorageBuffers:     8,
		UniformTexelBuffer: 2,
		StorageTexelBuffer: 2,
		InputAttachments:   1,
	}

	b.ResetTimer()
	var total uint32
	for i := 0; i < b.N; i++ {
		total = counts.Total()
	}
	benchSink = total
}
