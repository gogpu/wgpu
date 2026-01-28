// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux || windows

package vulkan

import (
	"testing"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
	"github.com/gogpu/gputypes"
)

// TestVulkanComputePipelineCreation tests pipeline creation and destruction.
func TestVulkanComputePipelineCreation(t *testing.T) {
	t.Run("struct fields", func(t *testing.T) {
		pipeline := &ComputePipeline{
			handle: vk.Pipeline(12345),
			layout: vk.PipelineLayout(67890),
		}
		if pipeline.handle != vk.Pipeline(12345) {
			t.Errorf("handle = %v, want 12345", pipeline.handle)
		}
		if pipeline.layout != vk.PipelineLayout(67890) {
			t.Errorf("layout = %v, want 67890", pipeline.layout)
		}
	})

	t.Run("nil device destroy", func(t *testing.T) {
		pipeline := &ComputePipeline{handle: vk.Pipeline(100), device: nil}
		pipeline.Destroy() // Should not panic
		if pipeline.handle != vk.Pipeline(100) {
			t.Error("Handle should remain valid after Destroy with nil device")
		}
	})

	t.Run("descriptor validation", func(t *testing.T) {
		device := &Device{handle: 0, cmds: nil}

		// nil descriptor
		if _, err := device.CreateComputePipeline(nil); err == nil {
			t.Error("expected error for nil descriptor")
		}

		// nil compute module
		desc := &hal.ComputePipelineDescriptor{
			Compute: hal.ComputeState{Module: nil, EntryPoint: "main"},
		}
		if _, err := device.CreateComputePipeline(desc); err == nil {
			t.Error("expected error for nil compute module")
		}
	})
}

// TestVulkanComputeDispatch tests basic dispatch execution.
func TestVulkanComputeDispatch(t *testing.T) {
	t.Run("encoder initialization", func(t *testing.T) {
		encoder := &CommandEncoder{isRecording: true}
		cpe := &ComputePassEncoder{encoder: encoder}
		if cpe.encoder != encoder {
			t.Error("encoder not set correctly")
		}
		if cpe.pipeline != nil {
			t.Error("pipeline should be nil initially")
		}
	})

	t.Run("End does not panic", func(t *testing.T) {
		cpe := &ComputePassEncoder{encoder: nil}
		cpe.End() // Should not panic with nil encoder
	})

	t.Run("workgroup configurations", func(t *testing.T) {
		tests := []struct{ x, y, z uint32 }{
			{1, 1, 1}, {64, 1, 1}, {8, 8, 8}, {256, 256, 1}, {0, 0, 0},
		}
		for _, tt := range tests {
			cpe := &ComputePassEncoder{encoder: &CommandEncoder{isRecording: false}}
			cpe.Dispatch(tt.x, tt.y, tt.z) // Should not panic
		}
	})
}

// TestVulkanComputeDispatchIndirect tests indirect dispatch from buffer.
func TestVulkanComputeDispatchIndirect(t *testing.T) {
	t.Run("nil buffer", func(t *testing.T) {
		cpe := &ComputePassEncoder{encoder: &CommandEncoder{isRecording: true}}
		cpe.DispatchIndirect(nil, 0) // Should not panic
	})

	t.Run("offset validation", func(t *testing.T) {
		offsets := []uint64{0, 16, 256, 4096}
		for _, offset := range offsets {
			cpe := &ComputePassEncoder{encoder: &CommandEncoder{isRecording: false}}
			cpe.DispatchIndirect(nil, offset) // Should not panic
		}
	})

	t.Run("valid buffer", func(t *testing.T) {
		buffer := &Buffer{
			handle: vk.Buffer(12345),
			size:   256,
			usage:  gputypes.BufferUsageIndirect | gputypes.BufferUsageStorage,
		}
		cpe := &ComputePassEncoder{encoder: &CommandEncoder{isRecording: false}}
		cpe.DispatchIndirect(buffer, 0) // Should not panic
	})
}

// TestVulkanComputeStorageBuffer tests read/write storage buffer operations.
func TestVulkanComputeStorageBuffer(t *testing.T) {
	t.Run("usage flags conversion", func(t *testing.T) {
		tests := []struct {
			usage  gputypes.BufferUsage
			expect vk.BufferUsageFlags
		}{
			{gputypes.BufferUsageStorage, vk.BufferUsageFlags(vk.BufferUsageStorageBufferBit)},
			{gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc,
				vk.BufferUsageFlags(vk.BufferUsageStorageBufferBit) | vk.BufferUsageFlags(vk.BufferUsageTransferSrcBit)},
			{gputypes.BufferUsageStorage | gputypes.BufferUsageCopyDst,
				vk.BufferUsageFlags(vk.BufferUsageStorageBufferBit) | vk.BufferUsageFlags(vk.BufferUsageTransferDstBit)},
		}
		for _, tt := range tests {
			if got := bufferUsageToVk(tt.usage); got != tt.expect {
				t.Errorf("bufferUsageToVk(%v) = %v, want %v", tt.usage, got, tt.expect)
			}
		}
	})

	t.Run("buffer struct", func(t *testing.T) {
		buffer := &Buffer{
			handle: vk.Buffer(100),
			size:   4096,
			usage:  gputypes.BufferUsageStorage,
		}
		if buffer.Size() != 4096 {
			t.Errorf("Size() = %d, want 4096", buffer.Size())
		}
		if buffer.Handle() != vk.Buffer(100) {
			t.Errorf("Handle() = %v, want 100", buffer.Handle())
		}
	})

	t.Run("binding type conversion", func(t *testing.T) {
		tests := []struct {
			bindingType gputypes.BufferBindingType
			expect      vk.DescriptorType
		}{
			{gputypes.BufferBindingTypeStorage, vk.DescriptorTypeStorageBuffer},
			{gputypes.BufferBindingTypeReadOnlyStorage, vk.DescriptorTypeStorageBuffer},
			{gputypes.BufferBindingTypeUniform, vk.DescriptorTypeUniformBuffer},
		}
		for _, tt := range tests {
			if got := bufferBindingTypeToVk(tt.bindingType); got != tt.expect {
				t.Errorf("bufferBindingTypeToVk(%v) = %v, want %v", tt.bindingType, got, tt.expect)
			}
		}
	})
}

// TestVulkanComputeMultipleBindGroups tests multiple descriptor sets.
func TestVulkanComputeMultipleBindGroups(t *testing.T) {
	t.Run("BindGroup fields", func(t *testing.T) {
		bg := &BindGroup{handle: vk.DescriptorSet(12345)}
		if bg.Handle() != vk.DescriptorSet(12345) {
			t.Errorf("Handle() = %v, want 12345", bg.Handle())
		}
	})

	t.Run("BindGroupLayout with storage", func(t *testing.T) {
		layout := &BindGroupLayout{
			handle: vk.DescriptorSetLayout(100),
			counts: DescriptorCounts{StorageBuffers: 4},
		}
		if layout.Counts().StorageBuffers != 4 {
			t.Errorf("StorageBuffers = %d, want 4", layout.Counts().StorageBuffers)
		}
	})

	t.Run("SetBindGroup nil group", func(t *testing.T) {
		cpe := &ComputePassEncoder{
			encoder:  &CommandEncoder{isRecording: true},
			pipeline: &ComputePipeline{handle: vk.Pipeline(100), layout: vk.PipelineLayout(200)},
		}
		cpe.SetBindGroup(0, nil, nil) // Should not panic
	})

	t.Run("SetBindGroup indices", func(t *testing.T) {
		tests := []struct {
			index   uint32
			offsets []uint32
		}{
			{0, nil}, {0, []uint32{0, 256}}, {1, nil}, {2, []uint32{0, 128}},
		}
		for _, tt := range tests {
			cpe := &ComputePassEncoder{encoder: &CommandEncoder{isRecording: false}}
			bg := &BindGroup{handle: vk.DescriptorSet(100)}
			cpe.SetBindGroup(tt.index, bg, tt.offsets) // Should not panic
		}
	})

	t.Run("multiple bind groups", func(t *testing.T) {
		bg0, bg1 := &BindGroup{handle: vk.DescriptorSet(100)}, &BindGroup{handle: vk.DescriptorSet(200)}
		if bg0.Handle() == bg1.Handle() {
			t.Error("bind groups should have different handles")
		}
	})

	t.Run("descriptor counts", func(t *testing.T) {
		tests := []struct {
			counts DescriptorCounts
			total  uint32
		}{
			{DescriptorCounts{StorageBuffers: 1}, 1},
			{DescriptorCounts{StorageBuffers: 4}, 4},
			{DescriptorCounts{StorageBuffers: 2, StorageImages: 1, UniformBuffers: 1}, 4},
		}
		for _, tt := range tests {
			if got := tt.counts.Total(); got != tt.total {
				t.Errorf("Total() = %d, want %d", got, tt.total)
			}
		}
	})
}

// TestVulkanComputeShaderStages tests compute shader stage conversions.
func TestVulkanComputeShaderStages(t *testing.T) {
	got := shaderStagesToVk(gputypes.ShaderStageCompute)
	expect := vk.ShaderStageFlags(vk.ShaderStageComputeBit)
	if got != expect {
		t.Errorf("shaderStagesToVk(Compute) = %v, want %v", got, expect)
	}
}

// TestVulkanComputeSetPipeline tests compute pipeline binding.
func TestVulkanComputeSetPipeline(t *testing.T) {
	t.Run("nil pipeline", func(t *testing.T) {
		cpe := &ComputePassEncoder{encoder: &CommandEncoder{isRecording: true}}
		cpe.SetPipeline(nil) // Should not panic
		if cpe.pipeline != nil {
			t.Error("pipeline should remain nil")
		}
	})

	t.Run("not recording", func(t *testing.T) {
		cpe := &ComputePassEncoder{encoder: &CommandEncoder{isRecording: false}}
		pipeline := &ComputePipeline{handle: vk.Pipeline(12345)}
		cpe.SetPipeline(pipeline)
		if cpe.pipeline != nil {
			t.Error("pipeline should not be set when not recording")
		}
	})
}

// TestVulkanBeginComputePass tests compute pass creation.
func TestVulkanBeginComputePass(t *testing.T) {
	t.Run("returns encoder", func(t *testing.T) {
		cmdEncoder := &CommandEncoder{isRecording: true}
		cpe := cmdEncoder.BeginComputePass(&hal.ComputePassDescriptor{Label: "test"})
		if cpe == nil {
			t.Fatal("BeginComputePass returned nil")
		}
		if _, ok := cpe.(*ComputePassEncoder); !ok {
			t.Error("BeginComputePass did not return *ComputePassEncoder")
		}
	})

	t.Run("nil descriptor", func(t *testing.T) {
		cmdEncoder := &CommandEncoder{isRecording: true}
		if cpe := cmdEncoder.BeginComputePass(nil); cpe == nil {
			t.Fatal("BeginComputePass returned nil for nil descriptor")
		}
	})
}
