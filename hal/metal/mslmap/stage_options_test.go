// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package mslmap_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga"
	"github.com/gogpu/naga/backend"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/naga/msl"
	"github.com/gogpu/naga/valid"
	"github.com/gogpu/naga/wgsl"
	"github.com/gogpu/wgpu/hal/metal/mslmap"
)

// wgslTwoStages has two entry points that use different buffers. After
// naga.ProcessOverrides keeps one entry point, only its buffers are left.
const wgslTwoStages = `
override scale: u32 = 3u;

@group(0) @binding(0) var<storage, read_write> first: array<u32, 4>;
@group(0) @binding(1) var<storage, read_write> second: array<u32, 4>;

@compute @workgroup_size(1)
fn a() {
	first[0] = scale;
}

@compute @workgroup_size(1)
fn b() {
	second[0] = scale;
}
`

func lower(t *testing.T, source string) *ir.Module {
	t.Helper()
	module, _, err := wgsl.Parse(source, wgsl.ParseOptions{})
	if err != nil {
		t.Fatalf("wgsl.Parse: %v", err)
	}
	return module
}

// specialize keeps one entry point of source, as the Metal HAL does for a
// pipeline stage: source is validated first, as naga.ProcessOverrides
// requires.
func specialize(t *testing.T, source *ir.Module, stage ir.ShaderStage, name string) *ir.Module {
	t.Helper()
	info, err := valid.Validate(source, valid.Options{Capabilities: valid.CapAll})
	if err != nil {
		t.Fatalf("valid.Validate: %v", err)
	}
	out, _, err := naga.ProcessOverrides(source, info, naga.OverrideOptions{
		EntryPoint: &backend.EntryPointSelector{Stage: stage, Name: name},
	})
	if err != nil {
		t.Fatalf("ProcessOverrides: %v", err)
	}
	return out
}

// TestStageLayoutKeepsSlotsOfSpecializedModule checks that a module
// specialized for entry point b keeps binding 1 at buffer slot 1, its slot in
// the pipeline layout, which the encoder binds. naga's own numbering of the
// specialized module, where binding 1 is the only buffer, would put it at
// slot 0.
func TestStageLayoutKeepsSlotsOfSpecializedModule(t *testing.T) {
	t.Parallel()

	source := lower(t, wgslTwoStages)
	stage := specialize(t, source, ir.StageCompute, "b")

	own, _, err := msl.Compile(stage, msl.DefaultOptions())
	if err != nil {
		t.Fatalf("msl.Compile (default options): %v", err)
	}
	t.Logf("naga's own numbering of the specialized module: second at [[buffer(%s)]]", bufferSlots(own)["second"])

	layout := stageLayout(t, gputypes.ShaderStageCompute, []gputypes.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
		{Binding: 1, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
	})
	out, _, err := msl.Compile(stage, layout.Options(ir.StageCompute, "b"))
	if err != nil {
		t.Fatalf("msl.Compile: %v", err)
	}
	if got := bufferSlots(out)["second"]; got != "1" {
		t.Errorf("second at [[buffer(%s)]], want 1 (its slot in the whole module)\n%s", got, out)
	}
}

// TestStageLayoutRenderStageReadsRuntimeArray guards P1-MTL-005. Since naga
// v0.20.5 (Rust parity) msl.Compile refuses a mapped entry point that needs
// the `_buffer_sizes` argument when the map gives it no slot; the render
// path once passed none and failed with `entry point "vs" has no sizes
// buffer`. A stage that sees a storage buffer gets the slot after its
// buffers, as compute does.
func TestStageLayoutRenderStageReadsRuntimeArray(t *testing.T) {
	t.Parallel()

	source := lower(t, `
override scale: f32 = 1.0;

@group(0) @binding(0) var<storage, read> positions: array<vec4<f32>>;

@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
	return positions[i] * scale;
}
`)
	stage := specialize(t, source, ir.StageVertex, "vs")
	bindings, err := mslmap.RuntimeArrayBindings(stage)
	if err != nil {
		t.Fatalf("RuntimeArrayBindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0] != (ir.ResourceBinding{Group: 0, Binding: 0}) {
		t.Fatalf("RuntimeArrayBindings = %v, want only @group(0) @binding(0)", bindings)
	}
	// One buffer in the layout, so the sizes argument goes to slot 1.
	layout := stageLayout(t, gputypes.ShaderStageVertex, []gputypes.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gputypes.ShaderStageVertex, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}},
	})
	out, _, err := msl.Compile(stage, layout.Options(ir.StageVertex, "vs"))
	if err != nil {
		t.Fatalf("msl.Compile: %v", err)
	}
	if got := bufferSlots(out)["_buffer_sizes"]; got != "1" {
		t.Errorf("_buffer_sizes at [[buffer(%s)]], want 1\n%s", got, out)
	}
}

// TestOverrideSizedArrayIsNotRuntimeSized checks that an array sized by an
// override does not get a `_buffer_sizes` field.
func TestOverrideSizedArrayIsNotRuntimeSized(t *testing.T) {
	t.Parallel()

	module := lower(t, `
override n: u32 = 4u;
var<workgroup> a: array<u32, n>;
@group(0) @binding(0) var<storage, read_write> out: array<u32>;

@compute @workgroup_size(1)
fn main() {
	a[0] = 1u;
	out[0] = a[0] + arrayLength(&out);
}
`)
	bindings, err := mslmap.RuntimeArrayBindings(module)
	if err != nil {
		t.Fatalf("RuntimeArrayBindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0] != (ir.ResourceBinding{Group: 0, Binding: 0}) {
		t.Errorf("RuntimeArrayBindings = %v, want only @group(0) @binding(0)", bindings)
	}
}
