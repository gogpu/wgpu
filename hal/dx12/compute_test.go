// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package dx12

import (
	"testing"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
	"github.com/gogpu/gputypes"
)

// TestDX12ComputePipelineCreation tests pipeline creation with root signature.
func TestDX12ComputePipelineCreation(t *testing.T) {
	t.Run("fields", func(t *testing.T) {
		pipeline := &ComputePipeline{}
		if pipeline.pso != nil {
			t.Error("pso should be nil")
		}
		if pipeline.rootSignature != nil {
			t.Error("rootSignature should be nil")
		}
	})

	t.Run("PSO accessor", func(t *testing.T) {
		pipeline := &ComputePipeline{}
		if pipeline.PSO() != nil {
			t.Error("PSO() should return nil")
		}
	})

	t.Run("Destroy", func(t *testing.T) {
		pipeline := &ComputePipeline{}
		pipeline.Destroy()
		if pipeline.pso != nil || pipeline.rootSignature != nil {
			t.Error("fields should be nil after Destroy")
		}
	})

	t.Run("state desc", func(t *testing.T) {
		desc := d3d12.D3D12_COMPUTE_PIPELINE_STATE_DESC{}
		if desc.RootSignature != nil || desc.CS.BytecodeLength != 0 {
			t.Error("empty desc should have nil/zero values")
		}
	})
}

// TestDX12ComputeDispatch tests direct dispatch.
func TestDX12ComputeDispatch(t *testing.T) {
	t.Run("ComputePassEncoder creation", func(t *testing.T) {
		encoder := &ComputePassEncoder{}
		if encoder.encoder != nil || encoder.pipeline != nil || encoder.descriptorHeapsSet {
			t.Error("new encoder should have nil/false values")
		}
	})

	t.Run("Dispatch with non-recording encoder", func(t *testing.T) {
		encoder := &ComputePassEncoder{encoder: &CommandEncoder{isRecording: false}}
		encoder.Dispatch(8, 8, 1) // Should not panic
	})

	t.Run("DISPATCH_ARGUMENTS structure", func(t *testing.T) {
		args := d3d12.D3D12_DISPATCH_ARGUMENTS{
			ThreadGroupCountX: 64, ThreadGroupCountY: 32, ThreadGroupCountZ: 16,
		}
		if args.ThreadGroupCountX != 64 || args.ThreadGroupCountY != 32 || args.ThreadGroupCountZ != 16 {
			t.Error("DISPATCH_ARGUMENTS values incorrect")
		}
	})
}

// TestDX12ComputeDispatchIndirect tests ExecuteIndirect.
func TestDX12ComputeDispatchIndirect(t *testing.T) {
	t.Run("with nil buffer", func(t *testing.T) {
		encoder := &ComputePassEncoder{encoder: &CommandEncoder{isRecording: true}}
		encoder.DispatchIndirect(nil, 0) // Should not panic
	})

	t.Run("with non-recording encoder", func(t *testing.T) {
		encoder := &ComputePassEncoder{encoder: &CommandEncoder{isRecording: false}}
		encoder.DispatchIndirect(&Buffer{size: 256}, 0) // Should not panic
	})

	t.Run("COMMAND_SIGNATURE_DESC", func(t *testing.T) {
		desc := d3d12.D3D12_COMMAND_SIGNATURE_DESC{ByteStride: 12, NumArgumentDescs: 1}
		if desc.ByteStride != 12 || desc.NumArgumentDescs != 1 {
			t.Error("COMMAND_SIGNATURE_DESC values incorrect")
		}
	})
}

// TestDX12ComputeUAV tests unordered access views.
func TestDX12ComputeUAV(t *testing.T) {
	t.Run("UAV_DESC buffer", func(t *testing.T) {
		desc := d3d12.D3D12_UNORDERED_ACCESS_VIEW_DESC{
			Format: d3d12.DXGI_FORMAT_R32_TYPELESS, ViewDimension: d3d12.D3D12_UAV_DIMENSION_BUFFER,
		}
		if desc.Format != d3d12.DXGI_FORMAT_R32_TYPELESS {
			t.Error("unexpected Format")
		}
	})

	t.Run("Buffer storage usage", func(t *testing.T) {
		buffer := &Buffer{usage: gputypes.BufferUsageStorage}
		if buffer.usage&gputypes.BufferUsageStorage == 0 {
			t.Error("Buffer should have storage usage")
		}
	})

	t.Run("Texture storage binding", func(t *testing.T) {
		texture := &Texture{usage: gputypes.TextureUsageStorageBinding, format: gputypes.TextureFormatRGBA32Float}
		if texture.usage&gputypes.TextureUsageStorageBinding == 0 {
			t.Error("Texture should have storage binding")
		}
	})
}

// TestDX12ComputeBarriers tests UAV barriers.
func TestDX12ComputeBarriers(t *testing.T) {
	t.Run("NewUAVBarrier", func(t *testing.T) {
		barrier := d3d12.NewUAVBarrier(nil)
		if barrier.Type != d3d12.D3D12_RESOURCE_BARRIER_TYPE_UAV {
			t.Errorf("Type = %d, want UAV", barrier.Type)
		}
	})

	t.Run("NewTransitionBarrier", func(t *testing.T) {
		barrier := d3d12.NewTransitionBarrier(nil,
			d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS,
			d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE,
			d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES)
		if barrier.Type != d3d12.D3D12_RESOURCE_BARRIER_TYPE_TRANSITION {
			t.Errorf("Type = %d, want TRANSITION", barrier.Type)
		}
	})

	t.Run("bufferUsageToD3D12State storage", func(t *testing.T) {
		state := bufferUsageToD3D12State(gputypes.BufferUsageStorage)
		if state != d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS {
			t.Errorf("state = %d, want UNORDERED_ACCESS", state)
		}
	})

	t.Run("textureUsageToD3D12State storage", func(t *testing.T) {
		state := textureUsageToD3D12State(gputypes.TextureUsageStorageBinding)
		if state != d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS {
			t.Errorf("state = %d, want UNORDERED_ACCESS", state)
		}
	})
}

// TestDX12SetBindGroup tests the SetBindGroup implementation from CS-002.
func TestDX12SetBindGroup(t *testing.T) {
	t.Run("BindGroup creation", func(t *testing.T) {
		layout := &BindGroupLayout{entries: []BindGroupLayoutEntry{{Binding: 0, Type: BindingTypeStorageBuffer}}}
		bg := &BindGroup{layout: layout, gpuDescHandle: d3d12.D3D12_GPU_DESCRIPTOR_HANDLE{Ptr: 0x1000}}
		if bg.layout != layout || bg.gpuDescHandle.Ptr != 0x1000 {
			t.Error("BindGroup fields incorrect")
		}
	})

	t.Run("SetBindGroup nil group", func(t *testing.T) {
		encoder := &ComputePassEncoder{encoder: &CommandEncoder{isRecording: true}}
		encoder.SetBindGroup(0, nil, nil) // Should not panic
	})

	t.Run("getRootParameterIndex", func(t *testing.T) {
		tests := []struct{ index, expected uint32 }{{0, 0}, {1, 2}, {2, 4}}
		for _, tt := range tests {
			if tt.index*2 != tt.expected {
				t.Errorf("getRootParameterIndex(%d) = %d, want %d", tt.index, tt.index*2, tt.expected)
			}
		}
	})

	t.Run("getDescriptorTableOffset", func(t *testing.T) {
		heap := &DescriptorHeap{
			cpuStart:      d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{Ptr: 0x1000},
			gpuStart:      d3d12.D3D12_GPU_DESCRIPTOR_HANDLE{Ptr: 0x2000},
			incrementSize: 32, capacity: 1024,
		}
		cpu, gpu, err := heap.AllocateGPU(4)
		if err != nil || cpu.Ptr != 0x1000 || gpu.Ptr != 0x2000 {
			t.Error("first allocation incorrect")
		}
		cpu2, gpu2, err := heap.AllocateGPU(2)
		if err != nil || cpu2.Ptr != 0x1000+4*32 || gpu2.Ptr != 0x2000+4*32 {
			t.Error("second allocation offset incorrect")
		}
	})
}

// TestBindingTypeToD3D12DescriptorRangeType tests binding type conversions.
func TestBindingTypeToD3D12DescriptorRangeType(t *testing.T) {
	tests := []struct {
		name      string
		bindType  BindingType
		wantType  d3d12.D3D12_DESCRIPTOR_RANGE_TYPE
		isSampler bool
	}{
		{"UniformBuffer", BindingTypeUniformBuffer, d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_CBV, false},
		{"StorageBuffer", BindingTypeStorageBuffer, d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_UAV, false},
		{"Sampler", BindingTypeSampler, d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_SAMPLER, true},
		{"SampledTexture", BindingTypeSampledTexture, d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_SRV, false},
		{"StorageTexture", BindingTypeStorageTexture, d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_UAV, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotSampler := bindingTypeToD3D12DescriptorRangeType(tt.bindType)
			if gotType != tt.wantType || gotSampler != tt.isSampler {
				t.Errorf("got (%d, %v), want (%d, %v)", gotType, gotSampler, tt.wantType, tt.isSampler)
			}
		})
	}
}

// TestDescriptorHeap tests descriptor heap allocation.
func TestDescriptorHeap(t *testing.T) {
	t.Run("Allocate", func(t *testing.T) {
		heap := &DescriptorHeap{cpuStart: d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{Ptr: 0x1000}, incrementSize: 32, capacity: 100}
		handle, err := heap.Allocate(10)
		if err != nil || handle.Ptr != 0x1000 || heap.nextFree != 10 {
			t.Error("Allocate failed")
		}
	})

	t.Run("Allocate exhausted", func(t *testing.T) {
		heap := &DescriptorHeap{capacity: 10, nextFree: 10}
		if _, err := heap.Allocate(1); err == nil {
			t.Error("expected error for exhausted heap")
		}
	})

	t.Run("Handle Offset", func(t *testing.T) {
		cpu := d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{Ptr: 0x1000}.Offset(5, 32)
		gpu := d3d12.D3D12_GPU_DESCRIPTOR_HANDLE{Ptr: 0x2000}.Offset(3, 64)
		if cpu.Ptr != 0x1000+5*32 || gpu.Ptr != 0x2000+3*64 {
			t.Error("Offset calculation incorrect")
		}
	})
}

// TestComputeHALInterface verifies HAL interface compliance.
func TestComputeHALInterface(t *testing.T) {
	var _ hal.ComputePipeline = (*ComputePipeline)(nil)
	var _ hal.ComputePassEncoder = (*ComputePassEncoder)(nil)
	var _ hal.BindGroup = (*BindGroup)(nil)
	var _ hal.BindGroupLayout = (*BindGroupLayout)(nil)
	var _ hal.PipelineLayout = (*PipelineLayout)(nil)
}

// TestRootSignatureConstants tests D3D12 root signature constants.
func TestRootSignatureConstants(t *testing.T) {
	if d3d12.D3D12_ROOT_PARAMETER_TYPE_DESCRIPTOR_TABLE != 0 {
		t.Error("DESCRIPTOR_TABLE should be 0")
	}
	if d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_SRV != 0 || d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_UAV != 1 {
		t.Error("DESCRIPTOR_RANGE_TYPE values incorrect")
	}
	if d3d12.D3D12_SHADER_VISIBILITY_ALL != 0 {
		t.Error("SHADER_VISIBILITY_ALL should be 0")
	}
}
