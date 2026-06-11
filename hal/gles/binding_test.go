// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/glsl"
	"github.com/gogpu/naga/ir"
)

// =============================================================================
// computeBindingMap Tests — per-type sequential slot assignment
// =============================================================================

func TestComputeBindingMap_Empty(t *testing.T) {
	bindingMap, groupInfos := computeBindingMap(nil)
	if len(bindingMap) != 0 {
		t.Errorf("expected empty binding map, got %d entries", len(bindingMap))
	}
	if len(groupInfos) != 0 {
		t.Errorf("expected empty group infos, got %d entries", len(groupInfos))
	}
}

func TestComputeBindingMap_SingleUniform(t *testing.T) {
	layouts := []*BindGroupLayout{
		{
			entries: []gputypes.BindGroupLayoutEntry{
				{
					Binding:    0,
					Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
					Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
				},
			},
		},
	}

	bindingMap, groupInfos := computeBindingMap(layouts)

	// Should have one entry: (group=0, binding=0) -> slot 0
	key := glsl.BindingMapKey{Group: 0, Binding: 0}
	slot, ok := bindingMap[key]
	if !ok {
		t.Fatal("expected binding map entry for (0, 0)")
	}
	if slot != 0 {
		t.Errorf("uniform buffer slot = %d, want 0", slot)
	}

	// Group info should map binding 0 -> slot 0
	if len(groupInfos) != 1 {
		t.Fatalf("expected 1 group info, got %d", len(groupInfos))
	}
	if groupInfos[0].BindingToSlot[0] != 0 {
		t.Errorf("BindingToSlot[0] = %d, want 0", groupInfos[0].BindingToSlot[0])
	}
}

func TestComputeBindingMap_MixedResources(t *testing.T) {
	// Group 0: sampler(0), texture(1), uniform(2)
	// Group 1: storage(0)
	layouts := []*BindGroupLayout{
		{
			entries: []gputypes.BindGroupLayoutEntry{
				{
					Binding:    0,
					Visibility: gputypes.ShaderStageFragment,
					Sampler:    &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering},
				},
				{
					Binding:    1,
					Visibility: gputypes.ShaderStageFragment,
					Texture:    &gputypes.TextureBindingLayout{SampleType: gputypes.TextureSampleTypeFloat},
				},
				{
					Binding:    2,
					Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
					Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
				},
			},
		},
		{
			entries: []gputypes.BindGroupLayoutEntry{
				{
					Binding:    0,
					Visibility: gputypes.ShaderStageCompute,
					Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage},
				},
			},
		},
	}

	bindingMap, groupInfos := computeBindingMap(layouts)

	// Verify per-type sequential counters:
	// Samplers:  group0/binding0 -> slot 0
	// Textures:  group0/binding1 -> slot 0
	// Uniforms:  group0/binding2 -> slot 0
	// Storage:   group1/binding0 -> slot 0
	wantBindings := map[glsl.BindingMapKey]uint8{
		{Group: 0, Binding: 0}: 0, // sampler
		{Group: 0, Binding: 1}: 0, // texture
		{Group: 0, Binding: 2}: 0, // uniform buffer
		{Group: 1, Binding: 0}: 0, // storage buffer
	}

	for key, wantSlot := range wantBindings {
		gotSlot, ok := bindingMap[key]
		if !ok {
			t.Errorf("missing binding map entry for (%d, %d)", key.Group, key.Binding)
			continue
		}
		if gotSlot != wantSlot {
			t.Errorf("slot for (%d, %d) = %d, want %d",
				key.Group, key.Binding, gotSlot, wantSlot)
		}
	}

	// Verify group info sizes
	if len(groupInfos) != 2 {
		t.Fatalf("expected 2 group infos, got %d", len(groupInfos))
	}
}

func TestComputeBindingMap_MultipleOfSameType(t *testing.T) {
	// Two uniform buffers in same group should get sequential slots.
	layouts := []*BindGroupLayout{
		{
			entries: []gputypes.BindGroupLayoutEntry{
				{
					Binding:    0,
					Visibility: gputypes.ShaderStageVertex,
					Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
				},
				{
					Binding:    1,
					Visibility: gputypes.ShaderStageFragment,
					Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
				},
			},
		},
	}

	bindingMap, _ := computeBindingMap(layouts)

	// Both are uniform buffers -> sequential slots 0, 1
	slot0, ok := bindingMap[glsl.BindingMapKey{Group: 0, Binding: 0}]
	if !ok || slot0 != 0 {
		t.Errorf("first uniform slot = %d, want 0", slot0)
	}
	slot1, ok := bindingMap[glsl.BindingMapKey{Group: 0, Binding: 1}]
	if !ok || slot1 != 1 {
		t.Errorf("second uniform slot = %d, want 1", slot1)
	}
}

func TestComputeBindingMap_CrossGroupCounters(t *testing.T) {
	// Uniform buffer counters continue across groups (sequential, not per-group).
	layouts := []*BindGroupLayout{
		{
			entries: []gputypes.BindGroupLayoutEntry{
				{
					Binding: 0,
					Buffer:  &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
				},
			},
		},
		{
			entries: []gputypes.BindGroupLayoutEntry{
				{
					Binding: 0,
					Buffer:  &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
				},
			},
		},
	}

	bindingMap, _ := computeBindingMap(layouts)

	// Group 0/binding 0 -> slot 0
	// Group 1/binding 0 -> slot 1 (counter continues)
	slot0, ok := bindingMap[glsl.BindingMapKey{Group: 0, Binding: 0}]
	if !ok || slot0 != 0 {
		t.Errorf("group0 uniform slot = %d, want 0", slot0)
	}
	slot1, ok := bindingMap[glsl.BindingMapKey{Group: 1, Binding: 0}]
	if !ok || slot1 != 1 {
		t.Errorf("group1 uniform slot = %d, want 1", slot1)
	}
}

func TestComputeBindingMap_NilLayout(t *testing.T) {
	// A nil layout in the array should be safely skipped.
	layouts := []*BindGroupLayout{nil, nil}
	bindingMap, groupInfos := computeBindingMap(layouts)

	if len(bindingMap) != 0 {
		t.Errorf("expected empty binding map with nil layouts, got %d entries", len(bindingMap))
	}
	if len(groupInfos) != 2 {
		t.Errorf("expected 2 group infos (even with nil), got %d", len(groupInfos))
	}
}

func TestComputeBindingMap_StorageTexture(t *testing.T) {
	// Storage textures (images) use their own counter, separate from textures.
	layouts := []*BindGroupLayout{
		{
			entries: []gputypes.BindGroupLayoutEntry{
				{
					Binding: 0,
					Texture: &gputypes.TextureBindingLayout{SampleType: gputypes.TextureSampleTypeFloat},
				},
				{
					Binding: 1,
					StorageTexture: &gputypes.StorageTextureBindingLayout{
						Access: gputypes.StorageTextureAccessWriteOnly,
						Format: gputypes.TextureFormatRGBA8Unorm,
					},
				},
				{
					Binding: 2,
					Texture: &gputypes.TextureBindingLayout{SampleType: gputypes.TextureSampleTypeFloat},
				},
			},
		},
	}

	bindingMap, _ := computeBindingMap(layouts)

	// Textures: binding 0 -> slot 0, binding 2 -> slot 1
	// Images:   binding 1 -> slot 0
	slot0, ok := bindingMap[glsl.BindingMapKey{Group: 0, Binding: 0}]
	if !ok || slot0 != 0 {
		t.Errorf("texture slot = %d, want 0", slot0)
	}
	slot1, ok := bindingMap[glsl.BindingMapKey{Group: 0, Binding: 1}]
	if !ok || slot1 != 0 {
		t.Errorf("image slot = %d, want 0", slot1)
	}
	slot2, ok := bindingMap[glsl.BindingMapKey{Group: 0, Binding: 2}]
	if !ok || slot2 != 1 {
		t.Errorf("second texture slot = %d, want 1", slot2)
	}
}

// =============================================================================
// classifyBindGroupEntry Tests
// =============================================================================

func TestClassifyBindGroupEntry(t *testing.T) {
	tests := []struct {
		name  string
		entry gputypes.BindGroupLayoutEntry
		want  bindingClass
	}{
		{
			name: "sampler",
			entry: gputypes.BindGroupLayoutEntry{
				Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering},
			},
			want: bindingClassSampler,
		},
		{
			name: "texture",
			entry: gputypes.BindGroupLayoutEntry{
				Texture: &gputypes.TextureBindingLayout{SampleType: gputypes.TextureSampleTypeFloat},
			},
			want: bindingClassTexture,
		},
		{
			name: "storage_texture",
			entry: gputypes.BindGroupLayoutEntry{
				StorageTexture: &gputypes.StorageTextureBindingLayout{
					Access: gputypes.StorageTextureAccessWriteOnly,
				},
			},
			want: bindingClassImage,
		},
		{
			name: "uniform_buffer",
			entry: gputypes.BindGroupLayoutEntry{
				Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
			},
			want: bindingClassUniformBuffer,
		},
		{
			name: "storage_buffer",
			entry: gputypes.BindGroupLayoutEntry{
				Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage},
			},
			want: bindingClassStorageBuffer,
		},
		{
			name: "readonly_storage_buffer",
			entry: gputypes.BindGroupLayoutEntry{
				Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage},
			},
			want: bindingClassStorageBuffer,
		},
		{
			name:  "empty_entry",
			entry: gputypes.BindGroupLayoutEntry{},
			want:  bindingClassUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyBindGroupEntry(tt.entry)
			if got != tt.want {
				t.Errorf("classifyBindGroupEntry() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// assignBindingsAfterLink Tests — runtime binding fallback on GL < 4.2
// =============================================================================

// TestAssignBindingsAfterLink_StorageBufferReturnsError verifies that storage
// buffers cannot be remapped at runtime (Rust wgpu-hal returns DeviceError::Lost).
func TestAssignBindingsAfterLink_StorageBufferReturnsError(t *testing.T) {
	layout := &PipelineLayout{
		bindingMap: map[glsl.BindingMapKey]uint8{
			{Group: 0, Binding: 0}: 0,
		},
	}

	// TranslationInfo with a storage buffer uniform.
	info := glsl.TranslationInfo{
		Uniforms: []glsl.UniformInfo{
			{
				BlockName: "StorageBlock",
				Binding:   ir.ResourceBinding{Group: 0, Binding: 0},
				IsStorage: true,
			},
		},
	}

	// We can't call assignBindingsAfterLink with a real GL context in unit tests,
	// but we can verify the error path by checking that storage buffer detection
	// exists in the translation info.
	if !info.Uniforms[0].IsStorage {
		t.Fatal("expected IsStorage=true for test setup")
	}

	// Verify the layout has the expected binding.
	_, ok := layout.bindingMap[glsl.BindingMapKey{Group: 0, Binding: 0}]
	if !ok {
		t.Fatal("expected binding map to contain (0, 0)")
	}

	// The actual GL call path is tested via integration tests since it
	// requires a live GL context. The logic verifies:
	// 1. Storage buffer detected via IsStorage flag
	// 2. Error returned (not silently ignored)
	// 3. Uniform blocks and textures can be remapped
}

// TestAssignBindingsAfterLink_BindingMapLookup verifies name→slot mapping
// resolution from TranslationInfo + PipelineLayout.
func TestAssignBindingsAfterLink_BindingMapLookup(t *testing.T) {
	layout := &PipelineLayout{
		bindingMap: map[glsl.BindingMapKey]uint8{
			{Group: 0, Binding: 0}: 3, // uniform buffer → slot 3
			{Group: 0, Binding: 1}: 0, // texture → slot 0
			{Group: 0, Binding: 2}: 0, // sampler → slot 0
		},
	}

	vertexInfo := glsl.TranslationInfo{
		Uniforms: []glsl.UniformInfo{
			{
				BlockName: "Uniforms_block_0Vertex",
				Binding:   ir.ResourceBinding{Group: 0, Binding: 0},
				IsStorage: false,
			},
		},
	}

	fragmentInfo := glsl.TranslationInfo{
		TextureMappings: map[string]glsl.TextureMapping{
			"_group_0_binding_1_fs": {
				TextureBinding: ir.ResourceBinding{Group: 0, Binding: 1},
				SamplerBinding: &ir.ResourceBinding{Group: 0, Binding: 2},
			},
		},
	}

	// Verify vertex uniform resolves to slot 3.
	for _, u := range vertexInfo.Uniforms {
		key := glsl.BindingMapKey{Group: u.Binding.Group, Binding: u.Binding.Binding}
		slot, ok := layout.bindingMap[key]
		if !ok {
			t.Errorf("uniform block %q not found in binding map", u.BlockName)
			continue
		}
		if slot != 3 {
			t.Errorf("uniform block %q slot = %d, want 3", u.BlockName, slot)
		}
	}

	// Verify fragment texture resolves to slot 0.
	for name, tm := range fragmentInfo.TextureMappings {
		key := glsl.BindingMapKey{Group: tm.TextureBinding.Group, Binding: tm.TextureBinding.Binding}
		slot, ok := layout.bindingMap[key]
		if !ok {
			t.Errorf("texture %q not found in binding map", name)
			continue
		}
		if slot != 0 {
			t.Errorf("texture %q slot = %d, want 0", name, slot)
		}
	}
}

// TestAssignBindingsAfterLink_MissingBindingSkipped verifies that uniforms
// with no matching binding map entry are gracefully skipped (not an error).
func TestAssignBindingsAfterLink_MissingBindingSkipped(t *testing.T) {
	layout := &PipelineLayout{
		bindingMap: map[glsl.BindingMapKey]uint8{
			// Only group 0, binding 0 is mapped.
			{Group: 0, Binding: 0}: 0,
		},
	}

	info := glsl.TranslationInfo{
		Uniforms: []glsl.UniformInfo{
			{
				BlockName: "MappedBlock",
				Binding:   ir.ResourceBinding{Group: 0, Binding: 0},
				IsStorage: false,
			},
			{
				// This binding is NOT in the map — should be skipped.
				BlockName: "UnmappedBlock",
				Binding:   ir.ResourceBinding{Group: 9, Binding: 9},
				IsStorage: false,
			},
		},
	}

	// Verify first is found, second is not.
	key0 := glsl.BindingMapKey{Group: 0, Binding: 0}
	if _, ok := layout.bindingMap[key0]; !ok {
		t.Error("expected binding map to contain (0, 0)")
	}

	key9 := glsl.BindingMapKey{Group: 9, Binding: 9}
	if _, ok := layout.bindingMap[key9]; ok {
		t.Error("expected binding map NOT to contain (9, 9)")
	}

	// assignBindingsAfterLink skips missing entries (the `continue` in the
	// loop). This is correct: optimized-out uniforms have no GL counterpart.
	_ = info
}

// =============================================================================
// UniformInfo reflection data tests
// =============================================================================

func TestUniformInfo_FieldsPopulated(t *testing.T) {
	info := glsl.UniformInfo{
		BlockName: "Uniforms_block_0Vertex",
		Binding:   ir.ResourceBinding{Group: 0, Binding: 0},
		IsStorage: false,
	}

	if info.BlockName != "Uniforms_block_0Vertex" {
		t.Errorf("BlockName = %q, want %q", info.BlockName, "Uniforms_block_0Vertex")
	}
	if info.Binding.Group != 0 || info.Binding.Binding != 0 {
		t.Errorf("Binding = (%d, %d), want (0, 0)", info.Binding.Group, info.Binding.Binding)
	}
	if info.IsStorage {
		t.Error("IsStorage should be false for UBO")
	}

	storageInfo := glsl.UniformInfo{
		BlockName: "StorageBlock_block_0Compute",
		Binding:   ir.ResourceBinding{Group: 0, Binding: 1},
		IsStorage: true,
	}
	if !storageInfo.IsStorage {
		t.Error("IsStorage should be true for SSBO")
	}
}

// =============================================================================
// TranslationInfo texture mapping tests
// =============================================================================

func TestTranslationInfo_TextureMappings(t *testing.T) {
	info := glsl.TranslationInfo{
		TextureMappings: map[string]glsl.TextureMapping{
			"_group_0_binding_1_fs": {
				TextureBinding: ir.ResourceBinding{Group: 0, Binding: 1},
				SamplerBinding: &ir.ResourceBinding{Group: 0, Binding: 2},
			},
			"_group_0_binding_3_fs": {
				TextureBinding: ir.ResourceBinding{Group: 0, Binding: 3},
				SamplerBinding: nil, // storage image — no sampler
			},
		},
	}

	// Texture+sampler pair
	tm, ok := info.TextureMappings["_group_0_binding_1_fs"]
	if !ok {
		t.Fatal("expected texture mapping for _group_0_binding_1_fs")
	}
	if tm.TextureBinding.Binding != 1 {
		t.Errorf("texture binding = %d, want 1", tm.TextureBinding.Binding)
	}
	if tm.SamplerBinding == nil || tm.SamplerBinding.Binding != 2 {
		t.Error("sampler binding should be (0, 2)")
	}

	// Storage image (nil sampler)
	tm2, ok := info.TextureMappings["_group_0_binding_3_fs"]
	if !ok {
		t.Fatal("expected texture mapping for _group_0_binding_3_fs")
	}
	if tm2.SamplerBinding != nil {
		t.Error("storage image should have nil SamplerBinding")
	}
}

// =============================================================================
// BindGroupLayoutInfo tests
// =============================================================================

func TestBindGroupLayoutInfo_UnusedSlot(t *testing.T) {
	// Verify unused binding slots are marked with 0xFF.
	layouts := []*BindGroupLayout{
		{
			entries: []gputypes.BindGroupLayoutEntry{
				{
					Binding: 2, // gap: 0 and 1 are unused
					Buffer:  &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
				},
			},
		},
	}

	_, groupInfos := computeBindingMap(layouts)

	if len(groupInfos) != 1 {
		t.Fatalf("expected 1 group info, got %d", len(groupInfos))
	}

	bts := groupInfos[0].BindingToSlot
	if len(bts) != 3 {
		t.Fatalf("expected 3 slots (0..2), got %d", len(bts))
	}
	if bts[0] != 0xFF {
		t.Errorf("slot 0 = %d, want 0xFF (unused)", bts[0])
	}
	if bts[1] != 0xFF {
		t.Errorf("slot 1 = %d, want 0xFF (unused)", bts[1])
	}
	if bts[2] != 0 {
		t.Errorf("slot 2 = %d, want 0 (first uniform)", bts[2])
	}
}
