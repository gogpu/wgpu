//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/internal/resource"
	"github.com/gogpu/wgpu/internal/shaderir"
)

// These tests pin the root-side pipeline helpers that feed the binder's late
// buffer binding data: the shader module's per-entry-point minimum binding
// sizes and makeLateSizedBufferGroups. They lived in binder_test.go until the
// S5 Move; the binder's own tests followed the binder into internal/encoder,
// while these stay with the root helpers they exercise.

// newLateBindingLayout builds a layout over its entries with no device or HAL
// object.
func newLateBindingLayout(entries []gputypes.BindGroupLayoutEntry) *resource.BindGroupLayout {
	return resource.NewBindGroupLayout(nil, nil, entries)
}

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

func TestMakeLateSizedBufferGroups(t *testing.T) {
	shaderSizes := map[ir.ResourceBinding]uint64{
		{Group: 0, Binding: 0}: 64,
		{Group: 0, Binding: 2}: 128,
		{Group: 1, Binding: 0}: 32,
	}

	layouts := []*resource.BindGroupLayout{
		newLateBindingLayout([]gputypes.BindGroupLayoutEntry{
			{Binding: 0, Buffer: &gputypes.BufferBindingLayout{MinBindingSize: 0}},                            // late: should get 64
			{Binding: 1, Buffer: &gputypes.BufferBindingLayout{MinBindingSize: 256}},                          // NOT late: MinBindingSize != 0
			{Binding: 2, Buffer: &gputypes.BufferBindingLayout{MinBindingSize: 0}},                            // late: should get 128
			{Binding: 3, Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering}}, // not buffer
		}),
		newLateBindingLayout([]gputypes.BindGroupLayoutEntry{
			{Binding: 0, Buffer: &gputypes.BufferBindingLayout{MinBindingSize: 0}}, // late: should get 32
		}),
	}

	groups := makeLateSizedBufferGroups(shaderSizes, layouts)

	// Group 0: 2 late entries (bindings 0 and 2)
	if len(groups[0].ShaderSizes) != 2 {
		t.Fatalf("group 0 has %d late entries, want 2", len(groups[0].ShaderSizes))
	}
	if groups[0].ShaderSizes[0] != 64 {
		t.Errorf("group 0 entry 0 = %d, want 64", groups[0].ShaderSizes[0])
	}
	if groups[0].ShaderSizes[1] != 128 {
		t.Errorf("group 0 entry 1 = %d, want 128", groups[0].ShaderSizes[1])
	}

	// Group 1: 1 late entry (binding 0)
	if len(groups[1].ShaderSizes) != 1 {
		t.Fatalf("group 1 has %d late entries, want 1", len(groups[1].ShaderSizes))
	}
	if groups[1].ShaderSizes[0] != 32 {
		t.Errorf("group 1 entry 0 = %d, want 32", groups[1].ShaderSizes[0])
	}

	// Groups 2-7: no late entries
	for i := 2; i < MaxBindGroups; i++ {
		if len(groups[i].ShaderSizes) != 0 {
			t.Errorf("group %d has %d late entries, want 0", i, len(groups[i].ShaderSizes))
		}
	}
}
