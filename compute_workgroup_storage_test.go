//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/core"
)

// TestCreateComputePipelineWorkgroupStorageLimit checks the WebGPU
// createComputePipeline rule: the workgroup storage the entry point uses
// statically, each variable rounded up to 16 bytes, must not exceed
// maxComputeWorkgroupStorageSize (Dawn ShaderModule.cpp; Rust wgpu has no
// check). An override-sized array is measured with the pipeline's constants.
func TestCreateComputePipelineWorkgroupStorageLimit(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	limit := device.Limits().MaxComputeWorkgroupStorageSize
	if limit < 32 {
		t.Fatalf("maxComputeWorkgroupStorageSize %d is below the WebGPU minimum", limit)
	}
	atLimit := limit / 16 // vec4<f32> elements
	tests := []struct {
		name      string
		source    string
		constants map[string]float64
		wantErr   bool
	}{
		{"exactly at the limit", workgroupArrayWGSL(atLimit), nil, false},
		{"one element over", workgroupArrayWGSL(atLimit + 1), nil, true},
		{"unused variable not counted", fmt.Sprintf(`var<workgroup> big: array<vec4<f32>, %d>;
@compute @workgroup_size(1) fn main() {}`, atLimit*2), nil, false},
		// Each f32 takes 16 bytes: limit/16 of them fit, one more does not.
		{"scalars rounded up to 16", scalarVarsWGSL(atLimit + 1), nil, true},
		{"override-sized array over", `override n: u32 = 4u;
var<workgroup> arr: array<vec4<f32>, n>;
@compute @workgroup_size(1) fn main() { arr[0] = vec4<f32>(1.0); }`,
			map[string]float64{"n": float64(atLimit + 1)}, true},
		{"override-sized array at default", `override n: u32 = 4u;
var<workgroup> arr: array<vec4<f32>, n>;
@compute @workgroup_size(1) fn main() { arr[0] = vec4<f32>(1.0); }`, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: tt.name, WGSL: tt.source})
			if err != nil {
				t.Fatalf("CreateShaderModule: %v", err)
			}
			defer module.Release()
			pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
				Label:      tt.name,
				Module:     module,
				EntryPoint: "main",
				Constants:  tt.constants,
			})
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("CreateComputePipeline: %v", err)
				}
				pipeline.Release()
				return
			}
			if err == nil {
				pipeline.Release()
				t.Fatal("CreateComputePipeline succeeded")
			}
			var ccpe *core.CreateComputePipelineError
			if !errors.As(err, &ccpe) || ccpe.Kind != core.CreateComputePipelineErrorWorkgroupStorageExceeded {
				t.Fatalf("error %v (%T), want CreateComputePipelineErrorWorkgroupStorageExceeded", err, err)
			}
			if ccpe.WorkgroupStorage <= uint64(limit) || ccpe.Limit != limit {
				t.Fatalf("storage %d limit %d, want above %d", ccpe.WorkgroupStorage, ccpe.Limit, limit)
			}
		})
	}
}

func workgroupArrayWGSL(elements uint32) string {
	return fmt.Sprintf(`var<workgroup> arr: array<vec4<f32>, %d>;
@compute @workgroup_size(1) fn main() { arr[0] = vec4<f32>(1.0); }`, elements)
}

func scalarVarsWGSL(count uint32) string {
	decls, uses := "", ""
	for i := range count {
		decls += fmt.Sprintf("var<workgroup> v%d: f32;\n", i)
		uses += fmt.Sprintf("v%d = 1.0; ", i)
	}
	return decls + "@compute @workgroup_size(1) fn main() { " + uses + "}"
}

// TestCreateComputePipelineOverrides checks that the compute limits are
// checked on the module specialized with the pipeline's constants: a
// workgroup size given by an override has its value, and a constant naming
// no override is a typed error.
func TestCreateComputePipelineOverrides(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	module, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "override wg", WGSL: `override wg: u32 = 8u;
@group(0) @binding(0) var<storage, read_write> out: array<u32>;
@compute @workgroup_size(wg) fn main(@builtin(local_invocation_index) i: u32) { out[i] = i; }`})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer module.Release()

	tooLarge := float64(device.Limits().MaxComputeWorkgroupSizeX + 1)
	tests := []struct {
		name      string
		constants map[string]float64
		wantKind  core.CreateComputePipelineErrorKind
		wantErr   bool
	}{
		{"default override size", nil, 0, false},
		{"constant size", map[string]float64{"wg": 4}, 0, false},
		{"constant size over the limit", map[string]float64{"wg": tooLarge}, core.CreateComputePipelineErrorWorkgroupSizeExceeded, true},
		{"unknown constant", map[string]float64{"nope": 1}, core.CreateComputePipelineErrorPipelineConstants, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
				Label: tt.name, Module: module, EntryPoint: "main", Constants: tt.constants,
			})
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("CreateComputePipeline: %v", err)
				}
				pipeline.Release()
				return
			}
			var ccpe *core.CreateComputePipelineError
			if !errors.As(err, &ccpe) || ccpe.Kind != tt.wantKind {
				if err == nil {
					pipeline.Release()
				}
				t.Fatalf("error %v (%T), want kind %d", err, err, tt.wantKind)
			}
		})
	}
}
