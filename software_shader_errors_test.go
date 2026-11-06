//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal/software/shader"
)

// badMagicSPIRV is a SPIR-V header with a wrong magic number: a module the
// software interpreter cannot parse.
var badMagicSPIRV = []uint32{0xDEADBEEF, 0x00010300, 0, 8, 0}

// badMagicParseError is the start of the interpreter's parse error for
// badMagicSPIRV.
const badMagicParseError = "bad magic 0xdeadbeef"

// newSoftwareDevice returns a device on the software adapter.
func newSoftwareDevice(t *testing.T) *wgpu.Device {
	t.Helper()
	inst, err := wgpu.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	t.Cleanup(inst.Release)
	adapter, err := inst.RequestAdapter(&wgpu.RequestAdapterOptions{ForceFallbackAdapter: true})
	if err != nil {
		t.Fatalf("RequestAdapter (software): %v", err)
	}
	t.Cleanup(adapter.Release)
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		t.Fatalf("RequestDevice: %v", err)
	}
	t.Cleanup(device.Release)
	return device
}

// TestSoftwareShaderParseErrorReachesPipelineCreation checks that a SPIR-V
// module the software backend cannot parse fails pipeline creation with a
// typed core error that names the parse error. The error used to be
// dropped: CreateRenderPipeline succeeded and the pipeline drew nothing,
// and CreateComputePipeline gave an error that did not say why.
func TestSoftwareShaderParseErrorReachesPipelineCreation(t *testing.T) {
	device := newSoftwareDevice(t)

	module, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "bad-magic", SPIRV: badMagicSPIRV})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer module.Release()
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "empty"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	defer layout.Release()

	t.Run("render", func(t *testing.T) {
		pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
			Label:  "bad-magic-render",
			Layout: layout,
			Vertex: wgpu.VertexState{Module: module, EntryPoint: "vs_main"},
			Fragment: &wgpu.FragmentState{
				Module:     module,
				EntryPoint: "fs_main",
				Targets:    []gputypes.ColorTargetState{{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll}},
			},
			Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
			Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
		})
		if err == nil {
			pipeline.Release()
			t.Fatal("CreateRenderPipeline succeeded with an unparsable module")
		}
		var rpe *core.CreateRenderPipelineError
		if !errors.As(err, &rpe) || rpe.Kind != core.CreateRenderPipelineErrorHAL {
			t.Fatalf("error %v (%T), want *core.CreateRenderPipelineError of kind HAL", err, err)
		}
		if !strings.Contains(err.Error(), badMagicParseError) {
			t.Fatalf("error %q does not name the parse error %q", err, badMagicParseError)
		}
	})

	t.Run("compute", func(t *testing.T) {
		checkComputeHALError(t, device, layout, module, badMagicParseError)
	})
}

// unsupportedOpcodeSPIRV is a valid GLCompute module whose entry point "main"
// holds OpImageQueryLod (105), which the software interpreter does not
// execute: naga never emits it, since WGSL has no textureQueryLod.
var unsupportedOpcodeSPIRV = []uint32{
	0x07230203, 0x00010300, 0, 20, 0,
	2<<16 | 17, 1, // OpCapability Shader
	3<<16 | 14, 0, 1, // OpMemoryModel Logical GLSL450
	5<<16 | 15, 5, 3, 0x6E69616D, 0, // OpEntryPoint GLCompute %3 "main"
	6<<16 | 16, 3, 17, 1, 1, 1, // OpExecutionMode %3 LocalSize 1 1 1
	2<<16 | 19, 1, // %1 = OpTypeVoid
	3<<16 | 33, 2, 1, // %2 = OpTypeFunction %1
	5<<16 | 54, 1, 3, 0, 2, // %3 = OpFunction %1 None %2
	2<<16 | 248, 4, // %4 = OpLabel
	5<<16 | 105, 1, 10, 11, 12, // OpImageQueryLod
	1<<16 | 253, // OpReturn
	1<<16 | 56,  // OpFunctionEnd
}

// TestSoftwareUnsupportedOpcodeReachesPipelineCreation checks that a module
// with an opcode the software interpreter does not execute fails compute
// pipeline creation with the HAL kind (an internal error: the input is valid,
// the backend cannot run it) and an error that names the opcode and wraps
// shader.ErrUnsupported. Before ADR-078 step 7 the opcode was skipped and the
// pipeline computed with its result undefined.
func TestSoftwareUnsupportedOpcodeReachesPipelineCreation(t *testing.T) {
	device := newSoftwareDevice(t)
	module, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "query-lod", SPIRV: unsupportedOpcodeSPIRV})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer module.Release()
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "empty"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	defer layout.Release()

	err = checkComputeHALError(t, device, layout, module, "SPIR-V opcode 105 (OpImageQueryLod) is not supported")
	if !errors.Is(err, shader.ErrUnsupported) {
		t.Fatalf("error %v does not wrap shader.ErrUnsupported", err)
	}
}

// checkComputeHALError creates a compute pipeline from module, checks that it
// fails with a *core.CreateComputePipelineError of kind HAL whose text
// contains want, and returns the error.
func checkComputeHALError(t *testing.T, device *wgpu.Device, layout *wgpu.PipelineLayout, module *wgpu.ShaderModule, want string) error {
	t.Helper()
	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "failing-compute",
		Layout:     layout,
		Module:     module,
		EntryPoint: "main",
	})
	if err == nil {
		pipeline.Release()
		t.Fatal("CreateComputePipeline succeeded with a module the interpreter cannot run")
	}
	var cpe *core.CreateComputePipelineError
	if !errors.As(err, &cpe) || cpe.Kind != core.CreateComputePipelineErrorHAL {
		t.Fatalf("error %v (%T), want *core.CreateComputePipelineError of kind HAL", err, err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err, want)
	}
	return err
}
