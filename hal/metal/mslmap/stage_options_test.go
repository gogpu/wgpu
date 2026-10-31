// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package mslmap_test

import (
	"testing"

	"github.com/gogpu/naga"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/naga/msl"
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
	ast, err := naga.Parse(source)
	if err != nil {
		t.Fatalf("naga.Parse: %v", err)
	}
	module, err := naga.LowerWithSource(ast, source)
	if err != nil {
		t.Fatalf("naga.LowerWithSource: %v", err)
	}
	return module
}

// TestStageOptionsKeepWholeModuleSlots checks that a module specialized for
// entry point b keeps binding 1 at buffer slot 1, its slot in the whole
// module, which the encoder binds. naga's own numbering of the specialized
// module, where binding 1 is the only buffer, would put it at slot 0.
func TestStageOptionsKeepWholeModuleSlots(t *testing.T) {
	t.Parallel()

	source := lower(t, wgslTwoStages)
	stage, err := naga.ProcessOverrides(source, naga.OverrideOptions{
		EntryPoint: &naga.EntryPointSelector{Stage: ir.StageCompute, Name: "b"},
	})
	if err != nil {
		t.Fatalf("ProcessOverrides: %v", err)
	}

	own, _, err := msl.Compile(stage, msl.DefaultOptions())
	if err != nil {
		t.Fatalf("msl.Compile (default options): %v", err)
	}
	t.Logf("naga's own numbering of the specialized module: second at [[buffer(%s)]]", bufferSlots(own)["second"])

	opts, err := mslmap.StageOptions(source, stage, -1)
	if err != nil {
		t.Fatalf("StageOptions: %v", err)
	}
	if opts.PerEntryPointMap["b"].SizesBuffer != nil {
		t.Error("StageOptions with sizesSlot -1 set a sizes buffer slot")
	}
	out, _, err := msl.Compile(stage, opts)
	if err != nil {
		t.Fatalf("msl.Compile: %v", err)
	}
	if got := bufferSlots(out)["second"]; got != "1" {
		t.Errorf("second at [[buffer(%s)]], want 1 (its slot in the whole module)\n%s", got, out)
	}
}

// TestStageOptionsRenderStageReadsRuntimeArray guards P1-MTL-005. A render
// stage of a module with an override is specialized, so it gets an explicit
// entry point map. Since naga v0.20.5 (Rust parity) msl.Compile refuses a
// mapped entry point that needs the `_buffer_sizes` argument when the map
// gives it no slot; with sizesSlot -1, as the render path used to pass, this
// test failed with `entry point "vs" has no sizes buffer`. The render path now
// passes the slot after the layout's buffers whenever the stage has a
// runtime-sized array, as compute does.
func TestStageOptionsRenderStageReadsRuntimeArray(t *testing.T) {
	t.Parallel()

	source := lower(t, `
override scale: f32 = 1.0;

@group(0) @binding(0) var<storage, read> positions: array<vec4<f32>>;

@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
	return positions[i] * scale;
}
`)
	stage, err := naga.ProcessOverrides(source, naga.OverrideOptions{
		EntryPoint: &naga.EntryPointSelector{Stage: ir.StageVertex, Name: "vs"},
	})
	if err != nil {
		t.Fatalf("ProcessOverrides: %v", err)
	}
	bindings, err := mslmap.RuntimeArrayBindings(stage)
	if err != nil {
		t.Fatalf("RuntimeArrayBindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0] != (ir.ResourceBinding{Group: 0, Binding: 0}) {
		t.Fatalf("RuntimeArrayBindings = %v, want only @group(0) @binding(0)", bindings)
	}
	// One buffer in the layout, so the sizes argument goes to slot 1.
	opts, err := mslmap.StageOptions(source, stage, 1)
	if err != nil {
		t.Fatalf("StageOptions: %v", err)
	}
	out, _, err := msl.Compile(stage, opts)
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
