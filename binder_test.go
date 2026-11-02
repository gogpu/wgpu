//go:build !rust && !(js && wasm)

package wgpu

import (
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/internal/shaderir"
)

func TestBinderReset(t *testing.T) {
	var b binder

	layout := &BindGroupLayout{}
	b.assign(0, layout)
	b.updateExpectations([]*BindGroupLayout{layout})

	b.reset()

	if b.maxSlots != 0 {
		t.Errorf("maxSlots = %d after reset, want 0", b.maxSlots)
	}
	for i := range b.assigned {
		if b.assigned[i] != nil {
			t.Errorf("assigned[%d] = %v after reset, want nil", i, b.assigned[i])
		}
	}
	for i := range b.expected {
		if b.expected[i] != nil {
			t.Errorf("expected[%d] = %v after reset, want nil", i, b.expected[i])
		}
	}
}

func TestBinderUpdateExpectations(t *testing.T) {
	var b binder

	l0 := &BindGroupLayout{}
	l1 := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{l0, l1})

	if b.maxSlots != 2 {
		t.Errorf("maxSlots = %d, want 2", b.maxSlots)
	}
	if b.expected[0] != l0 {
		t.Error("expected[0] should be l0")
	}
	if b.expected[1] != l1 {
		t.Error("expected[1] should be l1")
	}
	for i := uint32(2); i < MaxBindGroups; i++ {
		if b.expected[i] != nil {
			t.Errorf("expected[%d] = %v, want nil", i, b.expected[i])
		}
	}
}

func TestBinderUpdateExpectationsClearsPrevious(t *testing.T) {
	var b binder

	l0 := &BindGroupLayout{}
	l1 := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{l0, l1})

	// Switch to a pipeline with only 1 bind group.
	l2 := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{l2})

	if b.maxSlots != 1 {
		t.Errorf("maxSlots = %d, want 1", b.maxSlots)
	}
	if b.expected[0] != l2 {
		t.Error("expected[0] should be l2")
	}
	if b.expected[1] != nil {
		t.Error("expected[1] should be nil after switching to smaller pipeline")
	}
}

func TestBinderAssign(t *testing.T) {
	var b binder
	l := &BindGroupLayout{}

	b.assign(3, l)
	if b.assigned[3] != l {
		t.Error("assigned[3] should be l after assign")
	}
}

func TestBinderAssignOutOfRange(t *testing.T) {
	var b binder
	l := &BindGroupLayout{}

	// Should not panic when index >= MaxBindGroups.
	b.assign(MaxBindGroups, l)
	b.assign(MaxBindGroups+1, l)
}

func TestBinderCheckCompatibilityAllSatisfied(t *testing.T) {
	var b binder

	l0 := &BindGroupLayout{}
	l1 := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{l0, l1})
	b.assign(0, l0)
	b.assign(1, l1)

	if err := b.checkCompatibility(); err != nil {
		t.Errorf("checkCompatibility() = %v, want nil", err)
	}
}

func TestBinderCheckCompatibilityMissingBindGroup(t *testing.T) {
	var b binder

	l0 := &BindGroupLayout{}
	l1 := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{l0, l1})
	b.assign(0, l0)
	// Slot 1 is not assigned.

	err := b.checkCompatibility()
	if err == nil {
		t.Fatal("checkCompatibility() = nil, want error for missing bind group at index 1")
	}
	if !strings.Contains(err.Error(), "index 1") {
		t.Errorf("error should mention index 1: %v", err)
	}
	if !strings.Contains(err.Error(), "not set") {
		t.Errorf("error should mention 'not set': %v", err)
	}
}

func TestBinderCheckCompatibilityIncompatibleLayout(t *testing.T) {
	var b binder

	expected := &BindGroupLayout{
		entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageVertex},
		},
	}
	wrong := &BindGroupLayout{
		entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageFragment},
		},
	}
	b.updateExpectations([]*BindGroupLayout{expected})
	b.assign(0, wrong)

	err := b.checkCompatibility()
	if err == nil {
		t.Fatal("checkCompatibility() = nil, want error for incompatible layout")
	}
	if !strings.Contains(err.Error(), "incompatible") {
		t.Errorf("error should mention 'incompatible': %v", err)
	}
}

func TestBinderCheckCompatibilityNoPipeline(t *testing.T) {
	var b binder

	// No pipeline set, maxSlots = 0. Should pass (no expectations).
	if err := b.checkCompatibility(); err != nil {
		t.Errorf("checkCompatibility() with no pipeline = %v, want nil", err)
	}
}

func TestBinderCheckCompatibilityPipelineWithNoBindGroups(t *testing.T) {
	var b binder

	// Pipeline with zero bind group layouts.
	b.updateExpectations(nil)

	if err := b.checkCompatibility(); err != nil {
		t.Errorf("checkCompatibility() with empty pipeline = %v, want nil", err)
	}
}

func TestBinderAssignedPreservedAcrossPipelineSwitch(t *testing.T) {
	var b binder

	l0 := &BindGroupLayout{}
	l1 := &BindGroupLayout{}

	// Set bind groups before pipeline.
	b.assign(0, l0)
	b.assign(1, l1)

	// Now set pipeline that expects these layouts.
	b.updateExpectations([]*BindGroupLayout{l0, l1})

	// Assignments should still be valid.
	if err := b.checkCompatibility(); err != nil {
		t.Errorf("checkCompatibility() = %v, want nil (bind groups set before pipeline)", err)
	}
}

func TestBinderCrashScenario(t *testing.T) {
	// Reproduces the crash scenario from the research report:
	// Pipeline has 1 bind group layout (index 0).
	// User calls SetBindGroup(1, ...) — no bind group at index 0.
	var b binder

	expected0 := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{expected0})

	wrong := &BindGroupLayout{}
	b.assign(1, wrong) // Bind at index 1, but pipeline only expects index 0.

	err := b.checkCompatibility()
	if err == nil {
		t.Fatal("checkCompatibility() = nil, want error (index 0 not satisfied)")
	}
	if !strings.Contains(err.Error(), "index 0") {
		t.Errorf("error should reference index 0 (missing): %v", err)
	}
}

func TestBindGroupLayoutIsCompatibleWithSamePointer(t *testing.T) {
	layout := &BindGroupLayout{
		entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageVertex},
		},
	}
	if !layout.isCompatibleWith(layout) {
		t.Error("same pointer should be compatible (fast path)")
	}
}

func TestBindGroupLayoutIsCompatibleWithSameEntries(t *testing.T) {
	bufLayout := gputypes.BufferBindingLayout{
		Type:             gputypes.BufferBindingTypeUniform,
		HasDynamicOffset: false,
		MinBindingSize:   64,
	}
	entries := []gputypes.BindGroupLayoutEntry{
		{
			Binding:    0,
			Visibility: gputypes.ShaderStageVertex,
			Buffer:     &bufLayout,
		},
		{
			Binding:    1,
			Visibility: gputypes.ShaderStageFragment,
			Sampler:    &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering},
		},
	}

	layout1 := &BindGroupLayout{entries: make([]gputypes.BindGroupLayoutEntry, len(entries))}
	copy(layout1.entries, entries)

	// Create a second layout with identical entries but separate pointer allocations.
	layout2 := &BindGroupLayout{
		entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageVertex,
				Buffer: &gputypes.BufferBindingLayout{
					Type:             gputypes.BufferBindingTypeUniform,
					HasDynamicOffset: false,
					MinBindingSize:   64,
				},
			},
			{
				Binding:    1,
				Visibility: gputypes.ShaderStageFragment,
				Sampler:    &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering},
			},
		},
	}

	if !layout1.isCompatibleWith(layout2) {
		t.Error("layouts with identical entries (different pointers) should be compatible")
	}
}

func TestBindGroupLayoutIsCompatibleWithDifferentEntries(t *testing.T) {
	tests := []struct {
		name     string
		entries1 []gputypes.BindGroupLayoutEntry
		entries2 []gputypes.BindGroupLayoutEntry
	}{
		{
			name: "different binding number",
			entries1: []gputypes.BindGroupLayoutEntry{
				{Binding: 0, Visibility: gputypes.ShaderStageVertex},
			},
			entries2: []gputypes.BindGroupLayoutEntry{
				{Binding: 1, Visibility: gputypes.ShaderStageVertex},
			},
		},
		{
			name: "different visibility",
			entries1: []gputypes.BindGroupLayoutEntry{
				{Binding: 0, Visibility: gputypes.ShaderStageVertex},
			},
			entries2: []gputypes.BindGroupLayoutEntry{
				{Binding: 0, Visibility: gputypes.ShaderStageFragment},
			},
		},
		{
			name: "different entry count",
			entries1: []gputypes.BindGroupLayoutEntry{
				{Binding: 0, Visibility: gputypes.ShaderStageVertex},
			},
			entries2: []gputypes.BindGroupLayoutEntry{
				{Binding: 0, Visibility: gputypes.ShaderStageVertex},
				{Binding: 1, Visibility: gputypes.ShaderStageFragment},
			},
		},
		{
			name: "buffer vs nil",
			entries1: []gputypes.BindGroupLayoutEntry{
				{Binding: 0, Visibility: gputypes.ShaderStageVertex, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}},
			},
			entries2: []gputypes.BindGroupLayoutEntry{
				{Binding: 0, Visibility: gputypes.ShaderStageVertex},
			},
		},
		{
			name: "different buffer type",
			entries1: []gputypes.BindGroupLayoutEntry{
				{Binding: 0, Visibility: gputypes.ShaderStageVertex, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}},
			},
			entries2: []gputypes.BindGroupLayoutEntry{
				{Binding: 0, Visibility: gputypes.ShaderStageVertex, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
			},
		},
		{
			name: "different texture sample type",
			entries1: []gputypes.BindGroupLayoutEntry{
				{Binding: 0, Visibility: gputypes.ShaderStageFragment, Texture: &gputypes.TextureBindingLayout{SampleType: gputypes.TextureSampleTypeFloat}},
			},
			entries2: []gputypes.BindGroupLayoutEntry{
				{Binding: 0, Visibility: gputypes.ShaderStageFragment, Texture: &gputypes.TextureBindingLayout{SampleType: gputypes.TextureSampleTypeDepth}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l1 := &BindGroupLayout{entries: tt.entries1}
			l2 := &BindGroupLayout{entries: tt.entries2}
			if l1.isCompatibleWith(l2) {
				t.Error("layouts with different entries should NOT be compatible")
			}
		})
	}
}

func TestBindGroupLayoutIsCompatibleWithEmptyEntries(t *testing.T) {
	l1 := &BindGroupLayout{entries: []gputypes.BindGroupLayoutEntry{}}
	l2 := &BindGroupLayout{entries: []gputypes.BindGroupLayoutEntry{}}
	if !l1.isCompatibleWith(l2) {
		t.Error("two empty layouts should be compatible")
	}
}

func TestBinderCheckCompatibilityEntryByEntry(t *testing.T) {
	// The key scenario: two separate BindGroupLayout pointers with
	// identical entries should be considered compatible by the binder.
	var b binder

	entries := []gputypes.BindGroupLayoutEntry{
		{
			Binding:    0,
			Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
			Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: 64},
		},
	}

	pipelineLayout := &BindGroupLayout{entries: make([]gputypes.BindGroupLayoutEntry, len(entries))}
	copy(pipelineLayout.entries, entries)

	bindGroupLayout := &BindGroupLayout{
		entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: 64},
			},
		},
	}

	// Different pointers but same entries.
	if pipelineLayout == bindGroupLayout {
		t.Fatal("test setup error: layouts should be different pointers")
	}

	b.updateExpectations([]*BindGroupLayout{pipelineLayout})
	b.assign(0, bindGroupLayout)

	if err := b.checkCompatibility(); err != nil {
		t.Errorf("checkCompatibility() = %v, want nil (equivalent entries from separate CreateBindGroupLayout calls)", err)
	}
}

func TestBinderCheckCompatibilityEntryByEntryMismatch(t *testing.T) {
	var b binder

	pipelineLayout := &BindGroupLayout{
		entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageVertex, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}},
		},
	}
	bindGroupLayout := &BindGroupLayout{
		entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageVertex, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
		},
	}

	b.updateExpectations([]*BindGroupLayout{pipelineLayout})
	b.assign(0, bindGroupLayout)

	err := b.checkCompatibility()
	if err == nil {
		t.Fatal("checkCompatibility() = nil, want error for mismatched buffer type")
	}
	if !strings.Contains(err.Error(), "incompatible") {
		t.Errorf("error should mention 'incompatible': %v", err)
	}
}

func TestBinderMultipleSlots(t *testing.T) {
	tests := []struct {
		name       string
		expected   int // number of expected layouts
		assigned   []uint32
		wantErr    bool
		errContain string
	}{
		{
			name:     "all 8 slots satisfied",
			expected: 8,
			assigned: []uint32{0, 1, 2, 3, 4, 5, 6, 7},
			wantErr:  false,
		},
		{
			name:       "missing slot 4 of 5",
			expected:   5,
			assigned:   []uint32{0, 1, 2, 3},
			wantErr:    true,
			errContain: "index 4",
		},
		{
			name:       "missing first slot",
			expected:   3,
			assigned:   []uint32{1, 2},
			wantErr:    true,
			errContain: "index 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b binder

			// Create distinct layouts for each expected slot.
			layouts := make([]*BindGroupLayout, tt.expected)
			for i := range layouts {
				layouts[i] = &BindGroupLayout{}
			}
			b.updateExpectations(layouts)

			// Assign the specified slots with the matching layout.
			for _, idx := range tt.assigned {
				if idx < uint32(len(layouts)) {
					b.assign(idx, layouts[idx])
				}
			}

			err := b.checkCompatibility()
			if (err != nil) != tt.wantErr {
				t.Errorf("checkCompatibility() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContain != "" {
				if !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContain)
				}
			}
		})
	}
}

// --- Late buffer binding tests (VAL-006) ---

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
	module := &ShaderModule{irModule: lowered.IR, irInfo: &lowered.Info}
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
			got := module.bindingSizes(tt.stage, tt.entryPoint)
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
	var nilModule *ShaderModule
	if sizes := nilModule.bindingSizes(ir.StageCompute, "main"); sizes != nil {
		t.Errorf("bindingSizes on nil module = %v, want nil", sizes)
	}
	spirv := &ShaderModule{}
	if sizes := spirv.bindingSizes(ir.StageCompute, "main"); sizes != nil {
		t.Errorf("bindingSizes without IR = %v, want nil", sizes)
	}
}

func TestCheckLateBufferBindingsPass(t *testing.T) {
	var b binder
	l := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{l})

	// Pipeline requires 64 bytes, buffer has 128 bytes — should pass.
	var lateGroups [MaxBindGroups]LateSizedBufferGroup
	lateGroups[0] = LateSizedBufferGroup{ShaderSizes: []uint64{64}}
	b.updateLateBufferBindingsFromPipeline(lateGroups)

	bg := &BindGroup{
		layout:                 l,
		lateBufferBindingInfos: []LateBufferBindingInfo{{BindingIndex: 0, Size: 128}},
	}
	b.assignBindGroup(0, bg)

	if err := b.checkLateBufferBindings(); err != nil {
		t.Errorf("checkLateBufferBindings() = %v, want nil (buffer >= shader requirement)", err)
	}
}

func TestCheckLateBufferBindingsExactSize(t *testing.T) {
	var b binder
	l := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{l})

	// Pipeline requires 64 bytes, buffer has exactly 64 bytes — should pass.
	var lateGroups [MaxBindGroups]LateSizedBufferGroup
	lateGroups[0] = LateSizedBufferGroup{ShaderSizes: []uint64{64}}
	b.updateLateBufferBindingsFromPipeline(lateGroups)

	bg := &BindGroup{
		layout:                 l,
		lateBufferBindingInfos: []LateBufferBindingInfo{{BindingIndex: 0, Size: 64}},
	}
	b.assignBindGroup(0, bg)

	if err := b.checkLateBufferBindings(); err != nil {
		t.Errorf("checkLateBufferBindings() = %v, want nil (buffer == shader requirement)", err)
	}
}

func TestCheckLateBufferBindingsFail(t *testing.T) {
	var b binder
	l := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{l})

	// Pipeline requires 64 bytes, buffer has only 32 — should fail.
	var lateGroups [MaxBindGroups]LateSizedBufferGroup
	lateGroups[0] = LateSizedBufferGroup{ShaderSizes: []uint64{64}}
	b.updateLateBufferBindingsFromPipeline(lateGroups)

	bg := &BindGroup{
		layout:                 l,
		lateBufferBindingInfos: []LateBufferBindingInfo{{BindingIndex: 5, Size: 32}},
	}
	b.assignBindGroup(0, bg)

	err := b.checkLateBufferBindings()
	if err == nil {
		t.Fatal("checkLateBufferBindings() = nil, want error for undersized buffer")
	}
	if !strings.Contains(err.Error(), "group 0") {
		t.Errorf("error should mention group 0: %v", err)
	}
	if !strings.Contains(err.Error(), "binding 5") {
		t.Errorf("error should mention binding 5: %v", err)
	}
	if !strings.Contains(err.Error(), "size 32") {
		t.Errorf("error should mention bound size 32: %v", err)
	}
	if !strings.Contains(err.Error(), "minimum of 64") {
		t.Errorf("error should mention shader minimum 64: %v", err)
	}
}

func TestCheckLateBufferBindingsNoLateEntries(t *testing.T) {
	var b binder
	l := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{l})

	// No late groups — should always pass.
	var lateGroups [MaxBindGroups]LateSizedBufferGroup
	b.updateLateBufferBindingsFromPipeline(lateGroups)

	if err := b.checkLateBufferBindings(); err != nil {
		t.Errorf("checkLateBufferBindings() = %v, want nil (no late entries)", err)
	}
}

func TestCheckLateBufferBindingsBindGroupBeforePipeline(t *testing.T) {
	// Test order-independence: bind group set BEFORE pipeline.
	// Matches WebGPU spec behavior and Rust wgpu-core Binder.
	var b binder
	l := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{l})
	b.assign(0, l)

	// Set bind group FIRST (no pipeline yet — lateBufferBindings empty).
	bg := &BindGroup{
		layout:                 l,
		lateBufferBindingInfos: []LateBufferBindingInfo{{BindingIndex: 0, Size: 128}},
	}
	b.assignBindGroup(0, bg)

	// Then set pipeline (should merge into existing entries).
	var lateGroups [MaxBindGroups]LateSizedBufferGroup
	lateGroups[0] = LateSizedBufferGroup{ShaderSizes: []uint64{64}}
	b.updateLateBufferBindingsFromPipeline(lateGroups)

	if err := b.checkLateBufferBindings(); err != nil {
		t.Errorf("checkLateBufferBindings() = %v, want nil (bind group before pipeline, sufficient size)", err)
	}
}

func TestCheckLateBufferBindingsPipelineBeforeBindGroup(t *testing.T) {
	// Test order-independence: pipeline set BEFORE bind group.
	var b binder
	l := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{l})
	b.assign(0, l)

	// Set pipeline FIRST.
	var lateGroups [MaxBindGroups]LateSizedBufferGroup
	lateGroups[0] = LateSizedBufferGroup{ShaderSizes: []uint64{64}}
	b.updateLateBufferBindingsFromPipeline(lateGroups)

	// Then set bind group with insufficient size.
	bg := &BindGroup{
		layout:                 l,
		lateBufferBindingInfos: []LateBufferBindingInfo{{BindingIndex: 3, Size: 32}},
	}
	b.assignBindGroup(0, bg)

	err := b.checkLateBufferBindings()
	if err == nil {
		t.Fatal("checkLateBufferBindings() = nil, want error (pipeline before bind group, undersized)")
	}
	if !strings.Contains(err.Error(), "size 32") {
		t.Errorf("error should mention bound size: %v", err)
	}
}

func TestCheckLateBufferBindingsMultipleGroups(t *testing.T) {
	var b binder
	l0 := &BindGroupLayout{}
	l1 := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{l0, l1})
	b.assign(0, l0)
	b.assign(1, l1)

	// Group 0: ok (128 >= 64), Group 1: undersized (16 < 48).
	var lateGroups [MaxBindGroups]LateSizedBufferGroup
	lateGroups[0] = LateSizedBufferGroup{ShaderSizes: []uint64{64}}
	lateGroups[1] = LateSizedBufferGroup{ShaderSizes: []uint64{48}}
	b.updateLateBufferBindingsFromPipeline(lateGroups)

	bg0 := &BindGroup{
		layout:                 l0,
		lateBufferBindingInfos: []LateBufferBindingInfo{{BindingIndex: 0, Size: 128}},
	}
	bg1 := &BindGroup{
		layout:                 l1,
		lateBufferBindingInfos: []LateBufferBindingInfo{{BindingIndex: 0, Size: 16}},
	}
	b.assignBindGroup(0, bg0)
	b.assignBindGroup(1, bg1)

	err := b.checkLateBufferBindings()
	if err == nil {
		t.Fatal("checkLateBufferBindings() = nil, want error for group 1 undersized")
	}
	if !strings.Contains(err.Error(), "group 1") {
		t.Errorf("error should mention group 1: %v", err)
	}
}

func TestMakeLateSizedBufferGroups(t *testing.T) {
	shaderSizes := map[ir.ResourceBinding]uint64{
		{Group: 0, Binding: 0}: 64,
		{Group: 0, Binding: 2}: 128,
		{Group: 1, Binding: 0}: 32,
	}

	layouts := []*BindGroupLayout{
		{entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Buffer: &gputypes.BufferBindingLayout{MinBindingSize: 0}},                            // late: should get 64
			{Binding: 1, Buffer: &gputypes.BufferBindingLayout{MinBindingSize: 256}},                          // NOT late: MinBindingSize != 0
			{Binding: 2, Buffer: &gputypes.BufferBindingLayout{MinBindingSize: 0}},                            // late: should get 128
			{Binding: 3, Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering}}, // not buffer
		}},
		{entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Buffer: &gputypes.BufferBindingLayout{MinBindingSize: 0}}, // late: should get 32
		}},
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

func TestBinderResetClearsLateBindings(t *testing.T) {
	var b binder
	l := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{l})

	var lateGroups [MaxBindGroups]LateSizedBufferGroup
	lateGroups[0] = LateSizedBufferGroup{ShaderSizes: []uint64{64}}
	b.updateLateBufferBindingsFromPipeline(lateGroups)

	bg := &BindGroup{
		layout:                 l,
		lateBufferBindingInfos: []LateBufferBindingInfo{{BindingIndex: 0, Size: 32}},
	}
	b.assignBindGroup(0, bg)

	// Before reset, should have late bindings.
	if b.payloads[0].lateBindingsEffectiveCount == 0 {
		t.Fatal("expected late bindings before reset")
	}

	b.reset()

	// After reset, late bindings should be cleared.
	if b.payloads[0].lateBindingsEffectiveCount != 0 {
		t.Error("lateBindingsEffectiveCount should be 0 after reset")
	}
	if len(b.payloads[0].lateBufferBindings) != 0 {
		t.Error("lateBufferBindings should be empty after reset")
	}
}
