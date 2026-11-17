//go:build !rust && !(js && wasm)

package encoder

import (
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

// newTestBindGroupLayout builds a layout over its entries with no device or
// HAL object, for binder fixtures: the type's fields moved to internal/resource
// (ADR-070), so a test constructs it through the destination constructor.
func newTestBindGroupLayout(entries []gputypes.BindGroupLayoutEntry) *resource.BindGroupLayout {
	return resource.NewBindGroupLayout(nil, nil, entries)
}

// newTestBinderBindGroup builds a bind group fixture carrying the late buffer
// binding infos the binder reads, with no device or HAL object.
func newTestBinderBindGroup(layout *resource.BindGroupLayout, lateInfos []resource.LateBufferBindingInfo) *resource.BindGroup {
	return resource.NewBindGroup(&resource.BindGroupInit{Layout: layout, LateBufferBindingInfos: lateInfos})
}

func TestBinderReset(t *testing.T) {
	var b Binder

	layout := &resource.BindGroupLayout{}
	b.assign(0, layout)
	b.UpdateExpectations([]*resource.BindGroupLayout{layout})

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
	var b Binder

	l0 := &resource.BindGroupLayout{}
	l1 := &resource.BindGroupLayout{}
	b.UpdateExpectations([]*resource.BindGroupLayout{l0, l1})

	if b.maxSlots != 2 {
		t.Errorf("maxSlots = %d, want 2", b.maxSlots)
	}
	if b.expected[0] != l0 {
		t.Error("expected[0] should be l0")
	}
	if b.expected[1] != l1 {
		t.Error("expected[1] should be l1")
	}
	for i := uint32(2); i < core.MaxBindGroups; i++ {
		if b.expected[i] != nil {
			t.Errorf("expected[%d] = %v, want nil", i, b.expected[i])
		}
	}
}

func TestBinderUpdateExpectationsClearsPrevious(t *testing.T) {
	var b Binder

	l0 := &resource.BindGroupLayout{}
	l1 := &resource.BindGroupLayout{}
	b.UpdateExpectations([]*resource.BindGroupLayout{l0, l1})

	// Switch to a pipeline with only 1 bind group.
	l2 := &resource.BindGroupLayout{}
	b.UpdateExpectations([]*resource.BindGroupLayout{l2})

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
	var b Binder
	l := &resource.BindGroupLayout{}

	b.assign(3, l)
	if b.assigned[3] != l {
		t.Error("assigned[3] should be l after assign")
	}
}

func TestBinderAssignOutOfRange(t *testing.T) {
	var b Binder
	l := &resource.BindGroupLayout{}

	// Should not panic when index >= core.MaxBindGroups.
	b.assign(core.MaxBindGroups, l)
	b.assign(core.MaxBindGroups+1, l)
}

func TestBinderCheckCompatibilityAllSatisfied(t *testing.T) {
	var b Binder

	l0 := &resource.BindGroupLayout{}
	l1 := &resource.BindGroupLayout{}
	b.UpdateExpectations([]*resource.BindGroupLayout{l0, l1})
	b.assign(0, l0)
	b.assign(1, l1)

	if err := b.CheckCompatibility(); err != nil {
		t.Errorf("checkCompatibility() = %v, want nil", err)
	}
}

func TestBinderCheckCompatibilityMissingBindGroup(t *testing.T) {
	var b Binder

	l0 := &resource.BindGroupLayout{}
	l1 := &resource.BindGroupLayout{}
	b.UpdateExpectations([]*resource.BindGroupLayout{l0, l1})
	b.assign(0, l0)
	// Slot 1 is not assigned.

	err := b.CheckCompatibility()
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
	var b Binder

	expected := newTestBindGroupLayout([]gputypes.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gputypes.ShaderStageVertex},
	})
	wrong := newTestBindGroupLayout([]gputypes.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gputypes.ShaderStageFragment},
	})
	b.UpdateExpectations([]*resource.BindGroupLayout{expected})
	b.assign(0, wrong)

	err := b.CheckCompatibility()
	if err == nil {
		t.Fatal("checkCompatibility() = nil, want error for incompatible layout")
	}
	if !strings.Contains(err.Error(), "incompatible") {
		t.Errorf("error should mention 'incompatible': %v", err)
	}
}

func TestBinderCheckCompatibilityNoPipeline(t *testing.T) {
	var b Binder

	// No pipeline set, maxSlots = 0. Should pass (no expectations).
	if err := b.CheckCompatibility(); err != nil {
		t.Errorf("checkCompatibility() with no pipeline = %v, want nil", err)
	}
}

func TestBinderCheckCompatibilityPipelineWithNoBindGroups(t *testing.T) {
	var b Binder

	// Pipeline with zero bind group layouts.
	b.UpdateExpectations(nil)

	if err := b.CheckCompatibility(); err != nil {
		t.Errorf("checkCompatibility() with empty pipeline = %v, want nil", err)
	}
}

func TestBinderAssignedPreservedAcrossPipelineSwitch(t *testing.T) {
	var b Binder

	l0 := &resource.BindGroupLayout{}
	l1 := &resource.BindGroupLayout{}

	// Set bind groups before pipeline.
	b.assign(0, l0)
	b.assign(1, l1)

	// Now set pipeline that expects these layouts.
	b.UpdateExpectations([]*resource.BindGroupLayout{l0, l1})

	// Assignments should still be valid.
	if err := b.CheckCompatibility(); err != nil {
		t.Errorf("checkCompatibility() = %v, want nil (bind groups set before pipeline)", err)
	}
}

func TestBinderCrashScenario(t *testing.T) {
	// Reproduces the crash scenario from the research report:
	// Pipeline has 1 bind group layout (index 0).
	// User calls SetBindGroup(1, ...) — no bind group at index 0.
	var b Binder

	expected0 := &resource.BindGroupLayout{}
	b.UpdateExpectations([]*resource.BindGroupLayout{expected0})

	wrong := &resource.BindGroupLayout{}
	b.assign(1, wrong) // Bind at index 1, but pipeline only expects index 0.

	err := b.CheckCompatibility()
	if err == nil {
		t.Fatal("checkCompatibility() = nil, want error (index 0 not satisfied)")
	}
	if !strings.Contains(err.Error(), "index 0") {
		t.Errorf("error should reference index 0 (missing): %v", err)
	}
}

func TestBindGroupLayoutIsCompatibleWithSamePointer(t *testing.T) {
	layout := newTestBindGroupLayout([]gputypes.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gputypes.ShaderStageVertex},
	})
	if !layout.IsCompatibleWith(layout) {
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

	layout1 := newTestBindGroupLayout(make([]gputypes.BindGroupLayoutEntry, len(entries)))
	copy(layout1.Entries(), entries)

	// Create a second layout with identical entries but separate pointer allocations.
	layout2 := newTestBindGroupLayout([]gputypes.BindGroupLayoutEntry{
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
	})

	if !layout1.IsCompatibleWith(layout2) {
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
			l1 := newTestBindGroupLayout(tt.entries1)
			l2 := newTestBindGroupLayout(tt.entries2)
			if l1.IsCompatibleWith(l2) {
				t.Error("layouts with different entries should NOT be compatible")
			}
		})
	}
}

func TestBindGroupLayoutIsCompatibleWithEmptyEntries(t *testing.T) {
	l1 := newTestBindGroupLayout([]gputypes.BindGroupLayoutEntry{})
	l2 := newTestBindGroupLayout([]gputypes.BindGroupLayoutEntry{})
	if !l1.IsCompatibleWith(l2) {
		t.Error("two empty layouts should be compatible")
	}
}

func TestBinderCheckCompatibilityEntryByEntry(t *testing.T) {
	// The key scenario: two separate BindGroupLayout pointers with
	// identical entries should be considered compatible by the binder.
	var b Binder

	entries := []gputypes.BindGroupLayoutEntry{
		{
			Binding:    0,
			Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
			Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: 64},
		},
	}

	pipelineLayout := newTestBindGroupLayout(make([]gputypes.BindGroupLayoutEntry, len(entries)))
	copy(pipelineLayout.Entries(), entries)

	bindGroupLayout := newTestBindGroupLayout([]gputypes.BindGroupLayoutEntry{
		{
			Binding:    0,
			Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
			Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: 64},
		},
	})

	// Different pointers but same entries.
	if pipelineLayout == bindGroupLayout {
		t.Fatal("test setup error: layouts should be different pointers")
	}

	b.UpdateExpectations([]*resource.BindGroupLayout{pipelineLayout})
	b.assign(0, bindGroupLayout)

	if err := b.CheckCompatibility(); err != nil {
		t.Errorf("checkCompatibility() = %v, want nil (equivalent entries from separate CreateBindGroupLayout calls)", err)
	}
}

func TestBinderCheckCompatibilityEntryByEntryMismatch(t *testing.T) {
	var b Binder

	pipelineLayout := newTestBindGroupLayout([]gputypes.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gputypes.ShaderStageVertex, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}},
	})
	bindGroupLayout := newTestBindGroupLayout([]gputypes.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gputypes.ShaderStageVertex, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
	})

	b.UpdateExpectations([]*resource.BindGroupLayout{pipelineLayout})
	b.assign(0, bindGroupLayout)

	err := b.CheckCompatibility()
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
			var b Binder

			// Create distinct layouts for each expected slot.
			layouts := make([]*resource.BindGroupLayout, tt.expected)
			for i := range layouts {
				layouts[i] = &resource.BindGroupLayout{}
			}
			b.UpdateExpectations(layouts)

			// Assign the specified slots with the matching layout.
			for _, idx := range tt.assigned {
				if idx < uint32(len(layouts)) {
					b.assign(idx, layouts[idx])
				}
			}

			err := b.CheckCompatibility()
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

func TestCheckLateBufferBindingsPass(t *testing.T) {
	var b Binder
	l := &resource.BindGroupLayout{}
	b.UpdateExpectations([]*resource.BindGroupLayout{l})

	// Pipeline requires 64 bytes, buffer has 128 bytes — should pass.
	var lateGroups [core.MaxBindGroups]pipeline.LateSizedBufferGroup
	lateGroups[0] = pipeline.LateSizedBufferGroup{ShaderSizes: []uint64{64}}
	b.UpdateLateBufferBindingsFromPipeline(lateGroups)

	bg := newTestBinderBindGroup(l, []resource.LateBufferBindingInfo{{BindingIndex: 0, Size: 128}})
	b.assignBindGroup(0, bg)

	if err := b.CheckLateBufferBindings(); err != nil {
		t.Errorf("checkLateBufferBindings() = %v, want nil (buffer >= shader requirement)", err)
	}
}

func TestCheckLateBufferBindingsExactSize(t *testing.T) {
	var b Binder
	l := &resource.BindGroupLayout{}
	b.UpdateExpectations([]*resource.BindGroupLayout{l})

	// Pipeline requires 64 bytes, buffer has exactly 64 bytes — should pass.
	var lateGroups [core.MaxBindGroups]pipeline.LateSizedBufferGroup
	lateGroups[0] = pipeline.LateSizedBufferGroup{ShaderSizes: []uint64{64}}
	b.UpdateLateBufferBindingsFromPipeline(lateGroups)

	bg := newTestBinderBindGroup(l, []resource.LateBufferBindingInfo{{BindingIndex: 0, Size: 64}})
	b.assignBindGroup(0, bg)

	if err := b.CheckLateBufferBindings(); err != nil {
		t.Errorf("checkLateBufferBindings() = %v, want nil (buffer == shader requirement)", err)
	}
}

func TestCheckLateBufferBindingsFail(t *testing.T) {
	var b Binder
	l := &resource.BindGroupLayout{}
	b.UpdateExpectations([]*resource.BindGroupLayout{l})

	// Pipeline requires 64 bytes, buffer has only 32 — should fail.
	var lateGroups [core.MaxBindGroups]pipeline.LateSizedBufferGroup
	lateGroups[0] = pipeline.LateSizedBufferGroup{ShaderSizes: []uint64{64}}
	b.UpdateLateBufferBindingsFromPipeline(lateGroups)

	bg := newTestBinderBindGroup(l, []resource.LateBufferBindingInfo{{BindingIndex: 5, Size: 32}})
	b.assignBindGroup(0, bg)

	err := b.CheckLateBufferBindings()
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
	var b Binder
	l := &resource.BindGroupLayout{}
	b.UpdateExpectations([]*resource.BindGroupLayout{l})

	// No late groups — should always pass.
	var lateGroups [core.MaxBindGroups]pipeline.LateSizedBufferGroup
	b.UpdateLateBufferBindingsFromPipeline(lateGroups)

	if err := b.CheckLateBufferBindings(); err != nil {
		t.Errorf("checkLateBufferBindings() = %v, want nil (no late entries)", err)
	}
}

func TestCheckLateBufferBindingsBindGroupBeforePipeline(t *testing.T) {
	// Test order-independence: bind group set BEFORE pipeline.
	// Matches WebGPU spec behavior and Rust wgpu-core Binder.
	var b Binder
	l := &resource.BindGroupLayout{}
	b.UpdateExpectations([]*resource.BindGroupLayout{l})
	b.assign(0, l)

	// Set bind group FIRST (no pipeline yet — lateBufferBindings empty).
	bg := newTestBinderBindGroup(l, []resource.LateBufferBindingInfo{{BindingIndex: 0, Size: 128}})
	b.assignBindGroup(0, bg)

	// Then set pipeline (should merge into existing entries).
	var lateGroups [core.MaxBindGroups]pipeline.LateSizedBufferGroup
	lateGroups[0] = pipeline.LateSizedBufferGroup{ShaderSizes: []uint64{64}}
	b.UpdateLateBufferBindingsFromPipeline(lateGroups)

	if err := b.CheckLateBufferBindings(); err != nil {
		t.Errorf("checkLateBufferBindings() = %v, want nil (bind group before pipeline, sufficient size)", err)
	}
}

func TestCheckLateBufferBindingsPipelineBeforeBindGroup(t *testing.T) {
	// Test order-independence: pipeline set BEFORE bind group.
	var b Binder
	l := &resource.BindGroupLayout{}
	b.UpdateExpectations([]*resource.BindGroupLayout{l})
	b.assign(0, l)

	// Set pipeline FIRST.
	var lateGroups [core.MaxBindGroups]pipeline.LateSizedBufferGroup
	lateGroups[0] = pipeline.LateSizedBufferGroup{ShaderSizes: []uint64{64}}
	b.UpdateLateBufferBindingsFromPipeline(lateGroups)

	// Then set bind group with insufficient size.
	bg := newTestBinderBindGroup(l, []resource.LateBufferBindingInfo{{BindingIndex: 3, Size: 32}})
	b.assignBindGroup(0, bg)

	err := b.CheckLateBufferBindings()
	if err == nil {
		t.Fatal("checkLateBufferBindings() = nil, want error (pipeline before bind group, undersized)")
	}
	if !strings.Contains(err.Error(), "size 32") {
		t.Errorf("error should mention bound size: %v", err)
	}
}

func TestCheckLateBufferBindingsMultipleGroups(t *testing.T) {
	var b Binder
	l0 := &resource.BindGroupLayout{}
	l1 := &resource.BindGroupLayout{}
	b.UpdateExpectations([]*resource.BindGroupLayout{l0, l1})
	b.assign(0, l0)
	b.assign(1, l1)

	// Group 0: ok (128 >= 64), Group 1: undersized (16 < 48).
	var lateGroups [core.MaxBindGroups]pipeline.LateSizedBufferGroup
	lateGroups[0] = pipeline.LateSizedBufferGroup{ShaderSizes: []uint64{64}}
	lateGroups[1] = pipeline.LateSizedBufferGroup{ShaderSizes: []uint64{48}}
	b.UpdateLateBufferBindingsFromPipeline(lateGroups)

	bg0 := newTestBinderBindGroup(l0, []resource.LateBufferBindingInfo{{BindingIndex: 0, Size: 128}})
	bg1 := newTestBinderBindGroup(l1, []resource.LateBufferBindingInfo{{BindingIndex: 0, Size: 16}})
	b.assignBindGroup(0, bg0)
	b.assignBindGroup(1, bg1)

	err := b.CheckLateBufferBindings()
	if err == nil {
		t.Fatal("checkLateBufferBindings() = nil, want error for group 1 undersized")
	}
	if !strings.Contains(err.Error(), "group 1") {
		t.Errorf("error should mention group 1: %v", err)
	}
}

func TestBinderResetClearsLateBindings(t *testing.T) {
	var b Binder
	l := &resource.BindGroupLayout{}
	b.UpdateExpectations([]*resource.BindGroupLayout{l})

	var lateGroups [core.MaxBindGroups]pipeline.LateSizedBufferGroup
	lateGroups[0] = pipeline.LateSizedBufferGroup{ShaderSizes: []uint64{64}}
	b.UpdateLateBufferBindingsFromPipeline(lateGroups)

	bg := newTestBinderBindGroup(l, []resource.LateBufferBindingInfo{{BindingIndex: 0, Size: 32}})
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
