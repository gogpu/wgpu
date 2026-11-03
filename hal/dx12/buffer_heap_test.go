// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

// TestMappableBufferHeapProperties checks that CPU-visible buffers live in a
// CUSTOM heap with the CPU page property and memory pool the adapter reports
// for the UPLOAD and READBACK heap types (GetCustomHeapProperties). Those
// depend on the memory architecture: a cache-coherent UMA adapter such as
// WARP uses WRITE_BACK for uploads, and the debug layer warns that
// "Specifying D3D12_CPU_PAGE_PROPERTY_WRITE_COMBINE on pDesc can have a large
// performance impact" there.
func TestMappableBufferHeapProperties(t *testing.T) {
	forEachDX12Adapter(t, func(t *testing.T, d dx12TestDevice) {
		raw := d.device.(*Device).raw
		tests := []struct {
			name             string
			usage            gputypes.BufferUsage
			mappedAtCreation bool
			equivalent       d3d12.D3D12_HEAP_TYPE
		}{
			{"upload", gputypes.BufferUsageMapWrite | gputypes.BufferUsageCopySrc, false, d3d12.D3D12_HEAP_TYPE_UPLOAD},
			{"mapped at creation", gputypes.BufferUsageStorage, true, d3d12.D3D12_HEAP_TYPE_UPLOAD},
			{"readback", gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst, false, d3d12.D3D12_HEAP_TYPE_READBACK},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				buf, err := d.device.CreateBuffer(&hal.BufferDescriptor{Label: tt.name, Size: 256, Usage: tt.usage, MappedAtCreation: tt.mappedAtCreation})
				if err != nil {
					t.Fatalf("CreateBuffer: %v", err)
				}
				t.Cleanup(func() { d.device.DestroyBuffer(buf) })

				got, _, err := buf.(*Buffer).raw.GetHeapProperties()
				if err != nil {
					t.Fatalf("GetHeapProperties: %v", err)
				}
				want := raw.GetCustomHeapProperties(0, tt.equivalent)
				if got.Type != d3d12.D3D12_HEAP_TYPE_CUSTOM ||
					got.CPUPageProperty != want.CPUPageProperty ||
					got.MemoryPoolPreference != want.MemoryPoolPreference {
					t.Errorf("heap = type %d, page %d, pool %d; want CUSTOM, page %d, pool %d (the adapter's %d heap)",
						got.Type, got.CPUPageProperty, got.MemoryPoolPreference,
						want.CPUPageProperty, want.MemoryPoolPreference, tt.equivalent)
				}
			})
		}
	})
}
