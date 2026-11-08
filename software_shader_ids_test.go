//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"testing"

	"github.com/gogpu/wgpu"
)

// idsComputeSPIRV is a GLCompute module "main" that adds two u32 values,
// %8 = OpIAdd %6 %7 operand, under header ID bound bound. With bound 9 and
// operand 7 it is valid.
func idsComputeSPIRV(bound, operand uint32) []uint32 {
	return []uint32{
		0x07230203, 0x00010300, 0, bound, 0,
		2<<16 | 17, 1, // OpCapability Shader
		3<<16 | 14, 0, 1, // OpMemoryModel Logical GLSL450
		5<<16 | 15, 5, 3, 0x6E69616D, 0, // OpEntryPoint GLCompute %3 "main"
		6<<16 | 16, 3, 17, 1, 1, 1, // OpExecutionMode %3 LocalSize 1 1 1
		2<<16 | 19, 1, // %1 = OpTypeVoid
		3<<16 | 33, 2, 1, // %2 = OpTypeFunction %1
		4<<16 | 21, 6, 32, 0, // %6 = OpTypeInt 32 0
		4<<16 | 43, 6, 7, 1, // %7 = OpConstant %6 1
		5<<16 | 54, 1, 3, 0, 2, // %3 = OpFunction %1 None %2
		2<<16 | 248, 4, // %4 = OpLabel
		5<<16 | 128, 6, 8, 7, operand, // %8 = OpIAdd %6 %7 operand
		1<<16 | 253, // OpReturn
		1<<16 | 56,  // OpFunctionEnd
	}
}

// TestSoftwareShaderIDsOutsideBound checks, through the public API, that a raw
// SPIR-V module whose IDs the software interpreter cannot index fails compute
// pipeline creation with a returned error. Before P1-SW-039 these modules
// created a pipeline and the dispatch panicked inside Queue.Submit with
// "index out of range". The valid module is dispatched to show that the
// rejections come from the changed words alone.
func TestSoftwareShaderIDsOutsideBound(t *testing.T) {
	device := newSoftwareDevice(t)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "empty"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	defer layout.Release()

	t.Run("valid", func(t *testing.T) {
		module, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "ids-valid", SPIRV: idsComputeSPIRV(9, 7)})
		if err != nil {
			t.Fatalf("CreateShaderModule: %v", err)
		}
		defer module.Release()
		pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
			Label: "ids-valid", Layout: layout, Module: module, EntryPoint: "main",
		})
		if err != nil {
			t.Fatalf("CreateComputePipeline: %v", err)
		}
		defer pipeline.Release()
		encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{})
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		pass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{})
		if err != nil {
			t.Fatalf("BeginComputePass: %v", err)
		}
		pass.SetPipeline(pipeline)
		pass.Dispatch(1, 1, 1)
		if err := pass.End(); err != nil {
			t.Fatalf("ComputePass.End: %v", err)
		}
		command, err := encoder.Finish()
		if err != nil {
			t.Fatalf("CommandEncoder.Finish: %v", err)
		}
		if _, err := device.Queue().Submit(command); err != nil {
			t.Fatalf("Queue.Submit: %v", err)
		}
	})

	tests := []struct {
		name           string
		bound, operand uint32
		want           string
	}{
		{"bound below result IDs", 5, 7, "outside [1, 5)"},
		{"bound zero", 0, 7, "header ID bound 0"},
		{"operand at bound", 9, 9, "not below the ID bound 9"},
		{"operand undefined", 9, 5, "does not define"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: tt.name, SPIRV: idsComputeSPIRV(tt.bound, tt.operand)})
			if err != nil {
				t.Fatalf("CreateShaderModule: %v", err)
			}
			defer module.Release()
			checkComputeHALError(t, device, layout, module, tt.want)
		})
	}
}
