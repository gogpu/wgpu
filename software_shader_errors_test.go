//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/core"
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
		pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
			Label:      "bad-magic-compute",
			Layout:     layout,
			Module:     module,
			EntryPoint: "main",
		})
		if err == nil {
			pipeline.Release()
			t.Fatal("CreateComputePipeline succeeded with an unparsable module")
		}
		var cpe *core.CreateComputePipelineError
		if !errors.As(err, &cpe) || cpe.Kind != core.CreateComputePipelineErrorHAL {
			t.Fatalf("error %v (%T), want *core.CreateComputePipelineError of kind HAL", err, err)
		}
		if !strings.Contains(err.Error(), badMagicParseError) {
			t.Fatalf("error %q does not name the parse error %q", err, badMagicParseError)
		}
	})
}
