// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package mslmap_test

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/backend"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/naga/msl"
	"github.com/gogpu/wgpu/hal/metal/mslmap"
)

// wgslRuntimeArray calls arrayLength() on a runtime-sized storage array and
// also reads a uniform buffer. The generated MSL therefore has both the sizes
// argument and a normal buffer binding.
const wgslRuntimeArray = `
struct Params {
	scale: u32,
}

@group(0) @binding(0) var<storage, read_write> out: array<u32>;
@group(0) @binding(1) var<uniform> params: Params;

@compute @workgroup_size(1)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
	if (gid.x < arrayLength(&out)) {
		out[gid.x] = arrayLength(&out) * params.scale;
	}
}
`

// bufferParamRe finds each MSL parameter name and its buffer slot, for example
// "out [[buffer(0)]]".
var bufferParamRe = regexp.MustCompile(`(\w+)\s*\[\[buffer\((\d+)\)\]\]`)

func compile(t *testing.T, source string, opts msl.Options) (string, msl.TranslationInfo) {
	t.Helper()

	out, info, err := msl.Compile(lower(t, source), opts)
	if err != nil {
		t.Fatalf("msl.Compile: %v", err)
	}
	return out, info
}

func bufferSlots(mslSource string) map[string]string {
	slots := make(map[string]string)
	for _, m := range bufferParamRe.FindAllStringSubmatch(mslSource, -1) {
		slots[m[1]] = m[2]
	}
	return slots
}

func bufferEntry(binding uint32, stages gputypes.ShaderStages, ty gputypes.BufferBindingType) gputypes.BindGroupLayoutEntry {
	return gputypes.BindGroupLayoutEntry{Binding: binding, Visibility: stages, Buffer: &gputypes.BufferBindingLayout{Type: ty}}
}

func stageLayout(t *testing.T, stage gputypes.ShaderStages, groups ...[]gputypes.BindGroupLayoutEntry) mslmap.StageLayout {
	t.Helper()
	sorted := make([][]gputypes.BindGroupLayoutEntry, len(groups))
	for i, g := range groups {
		sorted[i] = mslmap.SortEntries(g)
	}
	layout, err := mslmap.NewStageLayout(sorted, stage)
	if err != nil {
		t.Fatalf("NewStageLayout: %v", err)
	}
	checkGroupSlots(t, sorted, stage, layout)
	return layout
}

// checkGroupSlots checks the contract between the pipeline layout and the
// bind group: a bind group bound at group g puts entry i at GroupBase[g] plus
// its GroupSlots slot, which must be the slot the shader was compiled with.
func checkGroupSlots(t *testing.T, groups [][]gputypes.BindGroupLayoutEntry, stage gputypes.ShaderStages, layout mslmap.StageLayout) {
	t.Helper()
	for g, entries := range groups {
		slots, _ := mslmap.GroupSlots(entries, stage)
		for i := range entries {
			target, mapped := layout.Resources[ir.ResourceBinding{Group: uint32(g), Binding: entries[i].Binding}]
			if slots[i] < 0 {
				if mapped {
					t.Errorf("group %d binding %d: in the shader map but bound nowhere", g, entries[i].Binding)
				}
				continue
			}
			kind := mslmap.EntryKind(&entries[i])
			want := layout.GroupBase[g].Of(kind) + slots[i]
			var got int
			switch kind {
			case mslmap.KindBuffer:
				got = int(*target.Buffer)
			case mslmap.KindTexture:
				got = int(*target.Texture)
			case mslmap.KindSampler:
				got = int(target.Sampler.Slot)
			}
			if !mapped || got != want {
				t.Errorf("group %d binding %d: shader slot %d (mapped %v), bind group slot %d", g, entries[i].Binding, got, mapped, want)
			}
		}
	}
}

// TestDefaultOptionsLeaveSizesBufferUnbound records why the backend passes a
// map: with msl.DefaultOptions, naga reports that the sizes buffer is
// required, but it writes the argument without a [[buffer(N)]] attribute.
func TestDefaultOptionsLeaveSizesBufferUnbound(t *testing.T) {
	t.Parallel()

	source, info := compile(t, wgslRuntimeArray, msl.DefaultOptions())

	if !info.RequiresSizesBuffer {
		t.Fatal("RequiresSizesBuffer = false; want true for a runtime-sized array")
	}
	if !strings.Contains(source, "_buffer_sizes") {
		t.Fatal("generated MSL has no _buffer_sizes argument")
	}
	if slot, ok := bufferSlots(source)["_buffer_sizes"]; ok {
		t.Fatalf("_buffer_sizes has [[buffer(%s)]] under DefaultOptions; want no attribute", slot)
	}
}

// TestStageLayoutBindsSizesBuffer checks that the sizes argument goes to the
// buffer slot after the layout's buffers, and the bindings to their layout
// slots.
func TestStageLayoutBindsSizesBuffer(t *testing.T) {
	t.Parallel()

	module := lower(t, wgslRuntimeArray)
	layout := stageLayout(t, gputypes.ShaderStageCompute, []gputypes.BindGroupLayoutEntry{
		bufferEntry(0, gputypes.ShaderStageCompute, gputypes.BufferBindingTypeStorage),
		bufferEntry(1, gputypes.ShaderStageCompute, gputypes.BufferBindingTypeUniform),
	})
	if layout.SizesSlot != 2 {
		t.Fatalf("SizesSlot = %d, want 2", layout.SizesSlot)
	}
	source, _, err := msl.Compile(module, layout.Options(ir.StageCompute, "main"))
	if err != nil {
		t.Fatalf("msl.Compile: %v", err)
	}
	want := map[string]string{"out": "0", "params": "1", "_buffer_sizes": "2"}
	for name, slot := range want {
		if got := bufferSlots(source)[name]; got != slot {
			t.Errorf("%s at [[buffer(%s)]], want %s\n%s", name, got, slot, source)
		}
	}
}

// TestStageLayoutSlotsComeFromLayout guards one shader module shared by
// pipeline layouts that place its binding differently: the slot of
// @group(1) @binding(0) follows the buffers of group 0 of each layout, not
// the bindings the shader declares (it declares none in group 0).
func TestStageLayoutSlotsComeFromLayout(t *testing.T) {
	t.Parallel()

	const source = `
@group(1) @binding(0) var<storage, read_write> out: array<u32, 4>;

@compute @workgroup_size(1)
fn main() {
    out[0] = 7u;
}
`
	module := lower(t, source)
	storage := func(binding uint32) gputypes.BindGroupLayoutEntry {
		return bufferEntry(binding, gputypes.ShaderStageCompute, gputypes.BufferBindingTypeStorage)
	}
	outGroup := []gputypes.BindGroupLayoutEntry{storage(0)}
	for _, tc := range []struct {
		name   string
		group0 []gputypes.BindGroupLayoutEntry
		want   string
	}{
		{"one buffer in group 0", []gputypes.BindGroupLayoutEntry{storage(0)}, "1"},
		{"two buffers in group 0", []gputypes.BindGroupLayoutEntry{storage(0), storage(1)}, "2"},
	} {
		layout := stageLayout(t, gputypes.ShaderStageCompute, tc.group0, outGroup)
		out, _, err := msl.Compile(module, layout.Options(ir.StageCompute, "main"))
		if err != nil {
			t.Fatalf("%s: msl.Compile: %v", tc.name, err)
		}
		if got := bufferSlots(out)["out"]; got != tc.want {
			t.Errorf("%s: out at [[buffer(%s)]], want %s\n%s", tc.name, got, tc.want, out)
		}
	}
}

// TestStageLayoutBindingOrder checks that the slots follow the binding
// numbers, not the order the layout lists its entries in.
func TestStageLayoutBindingOrder(t *testing.T) {
	t.Parallel()

	const source = `
struct U { tag: vec4<u32> }
@group(0) @binding(0) var<uniform> u: U;
@group(0) @binding(1) var<storage, read> s: array<u32>;
@group(0) @binding(2) var<storage, read_write> out: array<u32>;

@compute @workgroup_size(1)
fn main() {
    out[0] = u.tag.x + s[0] + arrayLength(&s);
}

`
	stages := gputypes.ShaderStageCompute
	reversed := []gputypes.BindGroupLayoutEntry{
		bufferEntry(2, stages, gputypes.BufferBindingTypeStorage),
		bufferEntry(1, stages, gputypes.BufferBindingTypeReadOnlyStorage),
		bufferEntry(0, stages, gputypes.BufferBindingTypeUniform),
	}
	if slots, _ := mslmap.GroupSlots(mslmap.SortEntries(reversed), stages); len(slots) != 3 || slots[0] != 0 || slots[1] != 1 || slots[2] != 2 {
		t.Fatalf("GroupSlots of the sorted entries = %v, want [0 1 2]", slots)
	}
	layout := stageLayout(t, stages, reversed)
	out, _, err := msl.Compile(lower(t, source), layout.Options(ir.StageCompute, "main"))
	if err != nil {
		t.Fatalf("msl.Compile: %v", err)
	}
	want := map[string]string{"u": "0", "s": "1", "out": "2", "_buffer_sizes": "3"}
	got := bufferSlots(out)
	for name, slot := range want {
		if got[name] != slot {
			t.Errorf("%s at [[buffer(%s)]], want %s\n%s", name, got[name], slot, out)
		}
	}
}

func TestRuntimeArrayOutOfBoundsAccessIsRestricted(t *testing.T) {
	const source = `
struct A { values: array<u32> }
@group(0) @binding(0) var<storage, read_write> a: A;
@group(0) @binding(1) var<storage, read_write> b: array<u32, 4>;
@compute @workgroup_size(1)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
	let r = a.values[66u + gid.x];
	a.values[65u + gid.x] = 0xA1u;
	b[2] = r;
}`
	stages := gputypes.ShaderStageCompute
	layout := stageLayout(t, stages, []gputypes.BindGroupLayoutEntry{
		bufferEntry(0, stages, gputypes.BufferBindingTypeStorage),
		bufferEntry(1, stages, gputypes.BufferBindingTypeStorage),
	})
	out, _ := compile(t, source, layout.Options(ir.StageCompute, "main"))
	if !strings.Contains(out, "metal::min(unsigned(66u + gid.x)") || !strings.Contains(out, "metal::min(unsigned(65u + gid.x)") {
		t.Fatalf("runtime-array accesses are not clamped:\n%s", out)
	}
}

// TestStageLayoutVisibility checks that each stage numbers only the
// bindings visible to it, and reserves a sizes slot only when a storage
// buffer is visible to it (Rust wgpu-hal create_pipeline_layout).
func TestStageLayoutVisibility(t *testing.T) {
	t.Parallel()

	vs, fs := gputypes.ShaderStageVertex, gputypes.ShaderStageFragment
	group0 := []gputypes.BindGroupLayoutEntry{
		bufferEntry(0, vs|fs, gputypes.BufferBindingTypeUniform),
		bufferEntry(1, fs, gputypes.BufferBindingTypeReadOnlyStorage),
		{Binding: 2, Visibility: vs, Texture: &gputypes.TextureBindingLayout{}},
		{Binding: 3, Visibility: fs, Sampler: &gputypes.SamplerBindingLayout{}},
	}
	group1 := []gputypes.BindGroupLayoutEntry{
		bufferEntry(0, vs|fs, gputypes.BufferBindingTypeUniform),
		{Binding: 1, Visibility: vs | fs, Texture: &gputypes.TextureBindingLayout{}},
	}

	vertex := stageLayout(t, vs, group0, group1)
	if vertex.SizesSlot != -1 {
		t.Errorf("vertex SizesSlot = %d, want -1 (no storage buffer visible)", vertex.SizesSlot)
	}
	if want := (mslmap.Counts{Buffers: 1, Textures: 1}); vertex.GroupBase[1] != want {
		t.Errorf("vertex GroupBase[1] = %+v, want %+v", vertex.GroupBase[1], want)
	}
	if _, ok := vertex.Resources[ir.ResourceBinding{Group: 0, Binding: 1}]; ok {
		t.Error("vertex map holds the fragment-only storage buffer")
	}

	fragment := stageLayout(t, fs, group0, group1)
	if fragment.SizesSlot != 3 {
		t.Errorf("fragment SizesSlot = %d, want 3 (after 3 buffers)", fragment.SizesSlot)
	}
	if want := (mslmap.Counts{Buffers: 2, Samplers: 1}); fragment.GroupBase[1] != want {
		t.Errorf("fragment GroupBase[1] = %+v, want %+v", fragment.GroupBase[1], want)
	}
	if target := fragment.Resources[ir.ResourceBinding{Group: 1, Binding: 1}]; target.Texture == nil || *target.Texture != 0 {
		t.Errorf("fragment texture of group 1 = %+v, want texture slot 0", target)
	}
}

// TestStageLayoutWritesOneEntryPoint checks that only the selected entry
// point is written: another entry point of the module uses a binding the
// pipeline layout does not have, which would otherwise fail with
// backend.ErrMissingBinding.
func TestStageLayoutWritesOneEntryPoint(t *testing.T) {
	t.Parallel()

	module := lower(t, `
@group(0) @binding(0) var<storage, read_write> a: array<u32>;
@group(0) @binding(1) var<storage, read_write> b: array<u32>;

@compute @workgroup_size(1)
fn first() { a[0] = arrayLength(&a); }

@compute @workgroup_size(1)
fn second() { b[0] = arrayLength(&b); }
`)
	layout := stageLayout(t, gputypes.ShaderStageCompute, []gputypes.BindGroupLayoutEntry{
		bufferEntry(0, gputypes.ShaderStageCompute, gputypes.BufferBindingTypeStorage),
	})
	out, _, err := msl.Compile(module, layout.Options(ir.StageCompute, "first"))
	if err != nil {
		t.Fatalf("msl.Compile: %v", err)
	}
	if strings.Contains(out, "second") {
		t.Errorf("MSL holds the entry point that was not selected\n%s", out)
	}

	_, _, err = msl.Compile(module, layout.Options(ir.StageCompute, "second"))
	if !errors.Is(err, backend.ErrMissingBinding) {
		t.Errorf("entry point using a binding outside the layout: err = %v, want ErrMissingBinding", err)
	}
}

// TestStageLayoutRejectsOutOfRangeSlot checks the guard for a slot that does
// not fit in a uint8.
func TestStageLayoutRejectsOutOfRangeSlot(t *testing.T) {
	t.Parallel()

	entries := make([]gputypes.BindGroupLayoutEntry, 300)
	for i := range entries {
		entries[i] = bufferEntry(uint32(i), gputypes.ShaderStageCompute, gputypes.BufferBindingTypeUniform)
	}
	if _, err := mslmap.NewStageLayout([][]gputypes.BindGroupLayoutEntry{entries}, gputypes.ShaderStageCompute); err == nil {
		t.Error("NewStageLayout with 300 buffers = nil error; want an out-of-range error")
	}
}
