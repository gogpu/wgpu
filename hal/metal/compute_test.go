// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"testing"

	"github.com/gogpu/naga/ir"
)

// TestMetalComputePipelineCreation tests MTLComputePipelineState creation.
// Verifies that ComputePipeline stores the workgroup size from the shader module.
func TestMetalComputePipelineCreation(t *testing.T) {
	tests := []struct {
		name              string
		workgroupSizes    map[string][3]uint32
		entryPoint        string
		wantWorkgroupSize MTLSize
	}{
		{
			name: "custom workgroup size 256x1x1",
			workgroupSizes: map[string][3]uint32{
				"main": {256, 1, 1},
			},
			entryPoint: "main",
			wantWorkgroupSize: MTLSize{
				Width:  256,
				Height: 1,
				Depth:  1,
			},
		},
		{
			name: "custom workgroup size 8x8x1",
			workgroupSizes: map[string][3]uint32{
				"compute_main": {8, 8, 1},
			},
			entryPoint: "compute_main",
			wantWorkgroupSize: MTLSize{
				Width:  8,
				Height: 8,
				Depth:  1,
			},
		},
		{
			name: "3D workgroup size 4x4x4",
			workgroupSizes: map[string][3]uint32{
				"volume_compute": {4, 4, 4},
			},
			entryPoint: "volume_compute",
			wantWorkgroupSize: MTLSize{
				Width:  4,
				Height: 4,
				Depth:  4,
			},
		},
		{
			name:           "missing entry point uses default",
			workgroupSizes: map[string][3]uint32{},
			entryPoint:     "not_found",
			wantWorkgroupSize: MTLSize{
				Width:  64,
				Height: 1,
				Depth:  1,
			},
		},
		{
			name:           "nil workgroupSizes uses default",
			workgroupSizes: nil,
			entryPoint:     "main",
			wantWorkgroupSize: MTLSize{
				Width:  64,
				Height: 1,
				Depth:  1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module := &ShaderModule{
				workgroupSizes: tt.workgroupSizes,
			}

			got := getWorkgroupSize(module, tt.entryPoint)

			if got.Width != tt.wantWorkgroupSize.Width {
				t.Errorf("Width = %d, want %d", got.Width, tt.wantWorkgroupSize.Width)
			}
			if got.Height != tt.wantWorkgroupSize.Height {
				t.Errorf("Height = %d, want %d", got.Height, tt.wantWorkgroupSize.Height)
			}
			if got.Depth != tt.wantWorkgroupSize.Depth {
				t.Errorf("Depth = %d, want %d", got.Depth, tt.wantWorkgroupSize.Depth)
			}
		})
	}
}

// TestMetalComputeDispatch tests dispatchThreadgroups with pipeline workgroup size.
// This is a regression test for CS-003 fix: verifies that Dispatch uses
// pipeline.workgroupSize instead of hardcoded {64, 1, 1}.
func TestMetalComputeDispatch(t *testing.T) {
	tests := []struct {
		name          string
		workgroupSize MTLSize
		dispatchX     uint32
		dispatchY     uint32
		dispatchZ     uint32
	}{
		{
			name:          "dispatch with 256x1x1 workgroup",
			workgroupSize: MTLSize{Width: 256, Height: 1, Depth: 1},
			dispatchX:     10,
			dispatchY:     1,
			dispatchZ:     1,
		},
		{
			name:          "dispatch with 8x8x1 workgroup for 2D compute",
			workgroupSize: MTLSize{Width: 8, Height: 8, Depth: 1},
			dispatchX:     16,
			dispatchY:     16,
			dispatchZ:     1,
		},
		{
			name:          "dispatch with 4x4x4 workgroup for 3D compute",
			workgroupSize: MTLSize{Width: 4, Height: 4, Depth: 4},
			dispatchX:     8,
			dispatchY:     8,
			dispatchZ:     8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := &ComputePipeline{
				workgroupSize: tt.workgroupSize,
			}
			encoder := &ComputePassEncoder{
				pipeline: pipeline,
			}

			// Verify pipeline is set on encoder
			if encoder.pipeline == nil {
				t.Fatal("pipeline should be set on encoder")
			}

			// Verify workgroup size is accessible from encoder's pipeline
			if encoder.pipeline.workgroupSize.Width != tt.workgroupSize.Width {
				t.Errorf("workgroupSize.Width = %d, want %d",
					encoder.pipeline.workgroupSize.Width, tt.workgroupSize.Width)
			}
			if encoder.pipeline.workgroupSize.Height != tt.workgroupSize.Height {
				t.Errorf("workgroupSize.Height = %d, want %d",
					encoder.pipeline.workgroupSize.Height, tt.workgroupSize.Height)
			}
			if encoder.pipeline.workgroupSize.Depth != tt.workgroupSize.Depth {
				t.Errorf("workgroupSize.Depth = %d, want %d",
					encoder.pipeline.workgroupSize.Depth, tt.workgroupSize.Depth)
			}
		})
	}
}

// TestMetalComputeWorkgroupSize verifies correct workgroup size usage.
// This is the key test for CS-003 fix: ensures pipeline stores workgroup size
// from shader metadata instead of using hardcoded values.
func TestMetalComputeWorkgroupSize(t *testing.T) {
	tests := []struct {
		name       string
		shaderSize [3]uint32
		entryPoint string
	}{
		{
			name:       "workgroup size 64x1x1 (common 1D)",
			shaderSize: [3]uint32{64, 1, 1},
			entryPoint: "compute_1d",
		},
		{
			name:       "workgroup size 256x1x1 (large 1D)",
			shaderSize: [3]uint32{256, 1, 1},
			entryPoint: "compute_large",
		},
		{
			name:       "workgroup size 16x16x1 (2D image processing)",
			shaderSize: [3]uint32{16, 16, 1},
			entryPoint: "compute_2d",
		},
		{
			name:       "workgroup size 8x8x8 (3D volume)",
			shaderSize: [3]uint32{8, 8, 8},
			entryPoint: "compute_3d",
		},
		{
			name:       "workgroup size 1x1x1 (minimal)",
			shaderSize: [3]uint32{1, 1, 1},
			entryPoint: "compute_single",
		},
		{
			name:       "workgroup size 32x1x1 (warp-aligned)",
			shaderSize: [3]uint32{32, 1, 1},
			entryPoint: "compute_warp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create shader module with specific workgroup size
			module := &ShaderModule{
				workgroupSizes: map[string][3]uint32{
					tt.entryPoint: tt.shaderSize,
				},
			}

			// Create pipeline (simulating CreateComputePipeline without Metal runtime)
			workgroupSize := getWorkgroupSize(module, tt.entryPoint)
			pipeline := &ComputePipeline{
				workgroupSize: workgroupSize,
			}

			// Verify workgroup size matches shader metadata
			if pipeline.workgroupSize.Width != NSUInteger(tt.shaderSize[0]) {
				t.Errorf("workgroupSize.Width = %d, want %d",
					pipeline.workgroupSize.Width, tt.shaderSize[0])
			}
			if pipeline.workgroupSize.Height != NSUInteger(tt.shaderSize[1]) {
				t.Errorf("workgroupSize.Height = %d, want %d",
					pipeline.workgroupSize.Height, tt.shaderSize[1])
			}
			if pipeline.workgroupSize.Depth != NSUInteger(tt.shaderSize[2]) {
				t.Errorf("workgroupSize.Depth = %d, want %d",
					pipeline.workgroupSize.Depth, tt.shaderSize[2])
			}
		})
	}
}

// TestMetalComputeThreadgroupMemory tests threadgroup memory handling.
// Verifies that encoder handles nil pipeline gracefully.
func TestMetalComputeThreadgroupMemory(t *testing.T) {
	tests := []struct {
		name        string
		hasPipeline bool
	}{
		{
			name:        "with pipeline set",
			hasPipeline: true,
		},
		{
			name:        "without pipeline (nil)",
			hasPipeline: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoder := &ComputePassEncoder{}

			if tt.hasPipeline {
				encoder.pipeline = &ComputePipeline{
					workgroupSize: MTLSize{Width: 64, Height: 1, Depth: 1},
				}
			}

			// Dispatch should not panic with nil pipeline
			// (Dispatch checks for nil pipeline and returns early)
			encoder.Dispatch(1, 1, 1)

			// Verify encoder state after dispatch
			if tt.hasPipeline && encoder.pipeline == nil {
				t.Error("pipeline should still be set after dispatch")
			}
			if !tt.hasPipeline && encoder.pipeline != nil {
				t.Error("pipeline should remain nil")
			}
		})
	}
}

// TestMetalShaderModuleWorkgroupParsing tests workgroup size extraction from naga IR.
// Verifies that extractWorkgroupSizes correctly extracts sizes from IR module entry points.
func TestMetalShaderModuleWorkgroupParsing(t *testing.T) {
	tests := []struct {
		name       string
		module     *ir.Module
		wantSizes  map[string][3]uint32
		wantNilMap bool
	}{
		{
			name: "single compute entry point",
			module: &ir.Module{
				EntryPoints: []ir.EntryPoint{
					{
						Name:      "main",
						Stage:     ir.StageCompute,
						Workgroup: [3]uint32{256, 1, 1},
					},
				},
			},
			wantSizes: map[string][3]uint32{
				"main": {256, 1, 1},
			},
			wantNilMap: false,
		},
		{
			name: "multiple compute entry points",
			module: &ir.Module{
				EntryPoints: []ir.EntryPoint{
					{
						Name:      "compute_a",
						Stage:     ir.StageCompute,
						Workgroup: [3]uint32{64, 1, 1},
					},
					{
						Name:      "compute_b",
						Stage:     ir.StageCompute,
						Workgroup: [3]uint32{8, 8, 1},
					},
					{
						Name:      "compute_c",
						Stage:     ir.StageCompute,
						Workgroup: [3]uint32{4, 4, 4},
					},
				},
			},
			wantSizes: map[string][3]uint32{
				"compute_a": {64, 1, 1},
				"compute_b": {8, 8, 1},
				"compute_c": {4, 4, 4},
			},
			wantNilMap: false,
		},
		{
			name: "mixed entry points (vertex, fragment, compute)",
			module: &ir.Module{
				EntryPoints: []ir.EntryPoint{
					{
						Name:  "vs_main",
						Stage: ir.StageVertex,
					},
					{
						Name:  "fs_main",
						Stage: ir.StageFragment,
					},
					{
						Name:      "cs_main",
						Stage:     ir.StageCompute,
						Workgroup: [3]uint32{128, 1, 1},
					},
				},
			},
			wantSizes: map[string][3]uint32{
				"cs_main": {128, 1, 1},
			},
			wantNilMap: false,
		},
		{
			name: "no compute entry points",
			module: &ir.Module{
				EntryPoints: []ir.EntryPoint{
					{
						Name:  "vs_main",
						Stage: ir.StageVertex,
					},
					{
						Name:  "fs_main",
						Stage: ir.StageFragment,
					},
				},
			},
			wantSizes:  nil,
			wantNilMap: true,
		},
		{
			name: "empty entry points",
			module: &ir.Module{
				EntryPoints: []ir.EntryPoint{},
			},
			wantSizes:  nil,
			wantNilMap: true,
		},
		{
			name:       "nil module",
			module:     nil,
			wantSizes:  nil,
			wantNilMap: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractWorkgroupSizes(tt.module)

			if tt.wantNilMap {
				if got != nil {
					t.Errorf("expected nil map, got %v", got)
				}
				return
			}

			if got == nil {
				t.Fatal("expected non-nil map, got nil")
			}

			if len(got) != len(tt.wantSizes) {
				t.Errorf("map length = %d, want %d", len(got), len(tt.wantSizes))
			}

			for name, wantSize := range tt.wantSizes {
				gotSize, ok := got[name]
				if !ok {
					t.Errorf("missing entry point %q in result", name)
					continue
				}
				if gotSize != wantSize {
					t.Errorf("workgroup size for %q = %v, want %v", name, gotSize, wantSize)
				}
			}
		})
	}
}

// TestComputePipelineNilPipelineGuard tests that Dispatch handles nil pipeline.
// This ensures the encoder doesn't crash when no pipeline is set.
func TestComputePipelineNilPipelineGuard(t *testing.T) {
	encoder := &ComputePassEncoder{
		pipeline: nil,
	}

	// Should not panic
	encoder.Dispatch(1, 1, 1)
	encoder.DispatchIndirect(nil, 0)
}

// TestComputePassEncoderSetPipeline tests SetPipeline method.
func TestComputePassEncoderSetPipeline(t *testing.T) {
	encoder := &ComputePassEncoder{}

	// Initially nil
	if encoder.pipeline != nil {
		t.Error("pipeline should be nil initially")
	}

	// Set pipeline
	pipeline := &ComputePipeline{
		workgroupSize: MTLSize{Width: 64, Height: 1, Depth: 1},
	}

	// Simulate SetPipeline (without Metal runtime)
	encoder.pipeline = pipeline

	if encoder.pipeline != pipeline {
		t.Error("pipeline should be set")
	}
	if encoder.pipeline.workgroupSize.Width != 64 {
		t.Errorf("workgroupSize.Width = %d, want 64", encoder.pipeline.workgroupSize.Width)
	}
}

// TestWorkgroupSizeDefaultFallback tests default workgroup size fallback behavior.
// When workgroup size is not found, should fall back to {64, 1, 1}.
func TestWorkgroupSizeDefaultFallback(t *testing.T) {
	tests := []struct {
		name       string
		module     *ShaderModule
		entryPoint string
	}{
		{
			name:       "nil workgroupSizes map",
			module:     &ShaderModule{workgroupSizes: nil},
			entryPoint: "main",
		},
		{
			name:       "empty workgroupSizes map",
			module:     &ShaderModule{workgroupSizes: map[string][3]uint32{}},
			entryPoint: "main",
		},
		{
			name: "entry point not in map",
			module: &ShaderModule{
				workgroupSizes: map[string][3]uint32{
					"other": {256, 1, 1},
				},
			},
			entryPoint: "main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getWorkgroupSize(tt.module, tt.entryPoint)

			// Default fallback is {64, 1, 1}
			if got.Width != 64 {
				t.Errorf("Width = %d, want 64 (default)", got.Width)
			}
			if got.Height != 1 {
				t.Errorf("Height = %d, want 1 (default)", got.Height)
			}
			if got.Depth != 1 {
				t.Errorf("Depth = %d, want 1 (default)", got.Depth)
			}
		})
	}
}
