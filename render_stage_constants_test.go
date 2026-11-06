//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"errors"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/internal/pipeconst"
)

// renderConstantsWGSL has an override in each render stage: vg only in the
// vertex stage, the @id(1000) override fb only in the fragment stage, plus
// overrides for the out-of-range and missing-value cases.
const renderConstantsWGSL = `
override vg: f32 = 1.0;
@id(1000) override fb: f32 = 0.0;
override vsmall: i32 = 0;
override fsmall: i32 = 0;

struct VOut {
    @builtin(position) pos: vec4<f32>,
    @location(0) g: f32,
}

@vertex
fn vs(@builtin(vertex_index) i: u32) -> VOut {
    var p = array<vec2<f32>, 3>(vec2(-1.0, -1.0), vec2(3.0, -1.0), vec2(-1.0, 3.0));
    var out: VOut;
    out.pos = vec4<f32>(p[i], f32(vsmall), 1.0);
    out.g = vg;
    return out;
}

@fragment
fn fs(in: VOut) -> @location(0) vec4<f32> {
    return vec4<f32>(f32(fsmall), in.g, fb, 1.0);
}
`

// missingConstantsWGSL has an override without a default in each stage.
const missingConstantsWGSL = `
override vneed: f32;
override fneed: f32;

@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
    return vec4<f32>(0.0, 0.0, vneed, 1.0);
}

@fragment
fn fs() -> @location(0) vec4<f32> {
    return vec4<f32>(fneed, 0.0, 0.0, 1.0);
}
`

// TestCreateRenderPipelineStageConstants checks the WebGPU "validating
// GPUProgrammableStage" rules for the vertex and the fragment stage through
// the public API: each stage is specialized with its own constants, and a
// bad constant is a typed CreateRenderPipelineError that names the stage.
func TestCreateRenderPipelineStageConstants(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	module := newConstantsModule(t, device, renderConstantsWGSL)
	missing := newConstantsModule(t, device, missingConstantsWGSL)

	type consts = map[string]float64
	tests := []struct {
		name      string
		module    *wgpu.ShaderModule
		vertex    consts
		fragment  consts
		wantStage string // "" = success
		wantIs    error  // sentinel the error must match
	}{
		{name: "defaults", module: module},
		{name: "vertex set", module: module, vertex: consts{"vg": 0}},
		{name: "fragment set by id", module: module, fragment: consts{"1000": 1}},
		{name: "both set", module: module, vertex: consts{"vg": 0.25}, fragment: consts{"1000": 0.5}},
		// A key names an override of the module, not of the stage: WebGPU
		// checks keys against the module, so this is valid and has no effect.
		{name: "vertex override set in fragment", module: module, fragment: consts{"vg": 0}},
		{name: "missing values given", module: missing, vertex: consts{"vneed": 0.5}, fragment: consts{"fneed": 1}},

		{name: "vertex unknown", module: module, vertex: consts{"nope": 1}, wantStage: "vertex", wantIs: naga.ErrOverrideNotFound},
		{name: "fragment unknown", module: module, fragment: consts{"nope": 1}, wantStage: "fragment", wantIs: naga.ErrOverrideNotFound},
		{name: "vertex out of range", module: module, vertex: consts{"vsmall": 1e10}, wantStage: "vertex", wantIs: naga.ErrOverrideValueOutOfRange},
		{name: "fragment out of range", module: module, fragment: consts{"fsmall": -1e10}, wantStage: "fragment", wantIs: naga.ErrOverrideValueOutOfRange},
		{name: "vertex missing value", module: missing, fragment: consts{"fneed": 1}, wantStage: "vertex", wantIs: naga.ErrOverrideMissingValue},
		{name: "fragment missing value", module: missing, vertex: consts{"vneed": 1}, wantStage: "fragment", wantIs: naga.ErrOverrideMissingValue},
		{name: "vertex NaN", module: module, vertex: consts{"vg": math.NaN()}, wantStage: "vertex", wantIs: pipeconst.ErrNotFinite},
		{name: "fragment +Inf", module: module, fragment: consts{"1000": math.Inf(1)}, wantStage: "fragment", wantIs: pipeconst.ErrNotFinite},
		{name: "vertex empty key", module: module, vertex: consts{"": 1}, wantStage: "vertex", wantIs: pipeconst.ErrEmptyKey},
		{name: "fragment empty key", module: module, fragment: consts{"": 1}, wantStage: "fragment", wantIs: pipeconst.ErrEmptyKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := device.CreateRenderPipeline(constantsPipelineDesc(tt.name, tt.module, tt.vertex, tt.fragment))
			if tt.wantStage == "" {
				if err != nil {
					t.Fatalf("CreateRenderPipeline: %v", err)
				}
				pipeline.Release()
				return
			}
			if err == nil {
				pipeline.Release()
				t.Fatalf("CreateRenderPipeline succeeded, want a %s stage error", tt.wantStage)
			}
			var rpe *core.CreateRenderPipelineError
			if !errors.As(err, &rpe) || rpe.Kind != core.CreateRenderPipelineErrorPipelineConstants {
				t.Fatalf("error %v (%T), want a CreateRenderPipelineError of kind PipelineConstants", err, err)
			}
			if rpe.Stage != tt.wantStage {
				t.Errorf("error stage %q, want %q (%v)", rpe.Stage, tt.wantStage, err)
			}
			if !errors.Is(err, tt.wantIs) {
				t.Errorf("error %q does not match %v", err, tt.wantIs)
			}
			t.Logf("error: %v", err)
		})
	}
}

// TestCreateComputePipelineNonFiniteConstant checks that the compute stage
// shares the render stages' check of keys and values.
func TestCreateComputePipelineNonFiniteConstant(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	module := newConstantsModule(t, device, `override wg: u32 = 8u;
@compute @workgroup_size(wg) fn main() {}`)
	for name, constants := range map[string]map[string]float64{
		"NaN":       {"wg": math.NaN()},
		"-Inf":      {"wg": math.Inf(-1)},
		"empty key": {"": 1},
	} {
		t.Run(name, func(t *testing.T) {
			pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
				Label: name, Module: module, EntryPoint: "main", Constants: constants,
			})
			if err == nil {
				pipeline.Release()
				t.Fatal("CreateComputePipeline succeeded, want an error")
			}
			var ccpe *core.CreateComputePipelineError
			if !errors.As(err, &ccpe) || ccpe.Kind != core.CreateComputePipelineErrorPipelineConstants {
				t.Fatalf("error %v (%T), want kind PipelineConstants", err, err)
			}
			var pce *pipeconst.Error
			if !errors.As(err, &pce) {
				t.Errorf("error %v has no *pipeconst.Error", err)
			}
		})
	}
}

func newConstantsModule(t *testing.T, device *wgpu.Device, wgsl string) *wgpu.ShaderModule {
	t.Helper()
	module, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "constants", WGSL: wgsl})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(module.Release)
	return module
}

func constantsPipelineDesc(label string, module *wgpu.ShaderModule, vertex, fragment map[string]float64) *wgpu.RenderPipelineDescriptor {
	return &wgpu.RenderPipelineDescriptor{
		Label:  label,
		Vertex: wgpu.VertexState{Module: module, EntryPoint: "vs", Constants: vertex},
		Fragment: &wgpu.FragmentState{
			Module:     module,
			EntryPoint: "fs",
			Constants:  fragment,
			Targets: []gputypes.ColorTargetState{
				{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll},
			},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	}
}
