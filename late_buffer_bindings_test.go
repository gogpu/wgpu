//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/internal/resource"
	"github.com/gogpu/wgpu/internal/shaderir"
)

// These tests pin the shader module's per-entry-point minimum binding sizes,
// the input to the binder's late buffer binding data. They lived in
// binder_test.go until the S5 Move; the binder's own tests followed the binder
// into internal/encoder, while these stay in the root package, which owns the
// shaderir lowering they exercise. makeLateSizedBufferGroups, the other root
// helper of that data, moved with the device into internal/device at S11 and is
// tested there.

// sharedBindingWGSL has two compute entry points whose globals share
// @group(0) @binding(0) with different types, which naga accepts since
// v0.21.0 (Rust's BindingCollision is per entry point), plus a vertex and a
// fragment entry point.
const sharedBindingWGSL = `
struct U { m: mat4x4<f32> }
@group(0) @binding(0) var<storage, read_write> small: array<u32, 4>;
@group(0) @binding(0) var<storage, read_write> large: array<u32, 64>;
@group(0) @binding(0) var tex: texture_2d<f32>;
@group(0) @binding(1) var<uniform> uniforms: U;
@group(0) @binding(2) var<storage, read_write> unused: array<u32, 32>;
@group(1) @binding(0) var samp: sampler;

@compute @workgroup_size(1) fn use_small() { small[0] = u32(uniforms.m[0][0]); }
@compute @workgroup_size(1) fn use_large() { large[0] = 1u; }
@compute @workgroup_size(1) fn use_tex() { _ = textureLoad(tex, vec2i(0), 0); }
@vertex fn vs() -> @builtin(position) vec4f { return uniforms.m[0]; }
@fragment fn fs() -> @location(0) vec4f { return textureSample(tex, samp, vec2f(0.0)); }
`

// TestExtractShaderBindingSizes checks that the minimum binding sizes are
// read per entry point from the globals it uses, as Rust wgpu-core
// Interface::check_stage reads the resources of one entry point
// (wgpu-core/src/validation.rs). Opaque types (textures, samplers) have no
// size, and an unused global contributes nothing.
func TestExtractShaderBindingSizes(t *testing.T) {
	lowered, err := shaderir.Lower(sharedBindingWGSL)
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	module := resource.NewShaderModule(nil, nil, lowered.IR, lowered.Info)
	rb00 := ir.ResourceBinding{Group: 0, Binding: 0}
	rb01 := ir.ResourceBinding{Group: 0, Binding: 1}

	tests := []struct {
		name       string
		stage      ir.ShaderStage
		entryPoint string
		want       map[ir.ResourceBinding]uint64
	}{
		{"small", ir.StageCompute, "use_small", map[ir.ResourceBinding]uint64{rb00: 16, rb01: 64}},
		{"large", ir.StageCompute, "use_large", map[ir.ResourceBinding]uint64{rb00: 256}},
		{"texture at a buffer's binding", ir.StageCompute, "use_tex", map[ir.ResourceBinding]uint64{}},
		{"vertex", ir.StageVertex, "vs", map[ir.ResourceBinding]uint64{rb01: 64}},
		{"vertex by omitted name", ir.StageVertex, "", map[ir.ResourceBinding]uint64{rb01: 64}},
		{"fragment", ir.StageFragment, "fs", map[ir.ResourceBinding]uint64{}},
		{"stage mismatch", ir.StageFragment, "vs", nil},
		{"ambiguous omitted name", ir.StageCompute, "", nil},
		{"missing", ir.StageCompute, "nope", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := module.BindingSizes(tt.stage, tt.entryPoint)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("sizes = %v, want nil", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("sizes = %v, want %v", got, tt.want)
			}
			for rb, size := range tt.want {
				if got[rb] != size {
					t.Errorf("binding %v size = %d, want %d (all: %v)", rb, got[rb], size, got)
				}
			}
		})
	}
}

func TestExtractShaderBindingSizesNilModule(t *testing.T) {
	var nilModule *resource.ShaderModule
	if sizes := nilModule.BindingSizes(ir.StageCompute, "main"); sizes != nil {
		t.Errorf("bindingSizes on nil module = %v, want nil", sizes)
	}
	spirv := &resource.ShaderModule{}
	if sizes := spirv.BindingSizes(ir.StageCompute, "main"); sizes != nil {
		t.Errorf("bindingSizes without IR = %v, want nil", sizes)
	}
}
