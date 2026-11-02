// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// Bind groups across pipeline changes: WebGPU lets a pass set a bind group
// before any pipeline, set a group the current pipeline does not use, and
// keep a group across a pipeline change while the new layout is compatible
// at that index. Core keeps the groups and their dynamic offsets and binds
// them to the backend at the next draw or dispatch, for the indices from the
// first one whose layout changed (Rust wgpu-core command/bind.rs Binder and
// pass.rs flush_bindings_helper). The backends used to get every
// SetBindGroup as it came: Vulkan dereferenced the missing pipeline (render)
// or dropped the bind (compute), and GLES lost the group.
//
// Each test runs this sequence, with group 0 = A and group 1 = B or C:
//
//	0. SetBindGroup(0, gA, a@slot1) before any pipeline; SetPipeline(P0),
//	   whose layout has group 0 only; SetBindGroup(1, gB, out@slot0, b@slot0),
//	   an index P0 does not have; draw or dispatch with P0, which writes nothing.
//	1. SetPipeline(P1 [LA, LB]): gA and gB are both bound at the draw.
//	2. SetPipeline(P2 [LA', LC]): LA' is a second layout equal to LA, so gA
//	   stays; LC is not LB, so gC is set at index 1.
//	3. SetPipeline(P1): gC does not fit LB, so gB is set again with other
//	   offsets (out@slot2, b@slot2); gA stays.
//	4. SetPipeline(P3 [LD, LB]): LD is not LA, so gD is set at index 0; gB
//	   stays, but a change at index 0 takes index 1 with it (Vulkan pipeline
//	   layout compatibility), so core binds gB again, with its offsets.

// Tags read through the uniform bindings; the readback names each draw or
// dispatch by them. Every value fits an 8-bit color channel.
const (
	rebindTagJunk = 99 // slot 0 of the A buffer: read only if an offset is lost
	rebindTagA    = 41 // gA, dynamic offset 256
	rebindTagD    = 43 // gD, fixed at offset 512
	rebindTagB0   = 31 // gB at offset 0
	rebindTagC    = 32 // gC, fixed at offset 256
	rebindTagB2   = 33 // gB at offset 512
)

// rebindBindingSize is the size of every buffer binding: two vec4 words.
const rebindBindingSize = 32

// rebindLayouts are the bind group layouts of one test.
type rebindLayouts struct {
	la, laTwin, ld *wgpu.BindGroupLayout // group 0: A (dynamic), its twin, D (fixed)
	lb, lc         *wgpu.BindGroupLayout // group 1: B (dynamic), C (fixed)
}

// rebindComputeDefects are the backends whose compute pass fails this test
// for a defect outside the bind group rebinding; each is skipped with it
// until its owner fixes it.
var rebindComputeDefects = map[string]string{
	"software": "software backend: a uniform vec4<u32> member reads back as the bits of the f32 (41 -> 0x42240000); the groups and offsets it reads are the right ones",
}

// TestBindGroupRebindCompute runs the sequence above in a compute pass. P1
// and P2 write (a.tag, b.tag, marker) at words 0..2 of their output binding,
// P3 at words 4..6, so step 4 does not overwrite step 3.
func TestBindGroupRebindCompute(t *testing.T) {
	forEachDynamicOffsetDevice(t, gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, b overrideBackend, device *wgpu.Device) {
		if defect, ok := rebindComputeDefects[b.name]; ok {
			t.Skipf("known defect, not the bind group rebinding: %s", defect)
		}
		checkBindGroupRebindCompute(t, device)
	})
}

// TestBindGroupRebindRender runs the sequence above in a render pass. Group
// 1 carries the quad's x range, P1 and P2 draw in the top half and P3 in the
// bottom half, and each quad is colored (a.tag, b.tag, marker, 255).
func TestBindGroupRebindRender(t *testing.T) {
	forEachDynamicOffsetDevice(t, 0, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		checkBindGroupRebindRender(t, device)
	})
}

func checkBindGroupRebindCompute(t *testing.T, device *wgpu.Device) {
	t.Helper()
	stages := wgpu.ShaderStageCompute
	ls := newRebindLayouts(t, device, stages, []rebindEntry{
		{binding: 0, ty: gputypes.BufferBindingTypeStorage},
		{binding: 1, ty: gputypes.BufferBindingTypeUniform},
	})
	p0 := newRebindComputePipeline(t, device, rebindComputeShaderP0, ls.la)
	p1 := newRebindComputePipeline(t, device, rebindComputeShader(1, 0), ls.la, ls.lb)
	p2 := newRebindComputePipeline(t, device, rebindComputeShader(2, 0), ls.laTwin, ls.lc)
	p3 := newRebindComputePipeline(t, device, rebindComputeShader(3, 4), ls.ld, ls.lb)

	aBuf := newRebindTagBuffer(t, device, []uint32{rebindTagJunk, rebindTagA, rebindTagD}, nil)
	bBuf := newRebindTagBuffer(t, device, []uint32{rebindTagB0, rebindTagC, rebindTagB2}, nil)
	const outSize = 3 * dynamicSlot
	out := newPublicBuffer(t, device, outSize, wgpu.BufferUsageStorage|wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
	if err := device.Queue().WriteBuffer(out, 0, make([]byte, outSize)); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	gA := newRebindBindGroup(t, device, ls.la, rebindBinding{0, aBuf, 0})
	gD := newRebindBindGroup(t, device, ls.ld, rebindBinding{0, aBuf, 2 * dynamicSlot})
	gB := newRebindBindGroup(t, device, ls.lb, rebindBinding{0, out, 0}, rebindBinding{1, bBuf, 0})
	gC := newRebindBindGroup(t, device, ls.lc, rebindBinding{0, out, dynamicSlot}, rebindBinding{1, bBuf, dynamicSlot})

	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := encoder.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	pass.SetBindGroup(0, gA, []uint32{dynamicSlot})
	pass.SetPipeline(p0)
	pass.SetBindGroup(1, gB, []uint32{0, 0})
	pass.Dispatch(1, 1, 1)
	pass.SetPipeline(p1)
	pass.Dispatch(1, 1, 1)
	pass.SetPipeline(p2)
	pass.SetBindGroup(1, gC, nil)
	pass.Dispatch(1, 1, 1)
	pass.SetPipeline(p1)
	pass.SetBindGroup(1, gB, []uint32{2 * dynamicSlot, 2 * dynamicSlot})
	pass.Dispatch(1, 1, 1)
	pass.SetPipeline(p3)
	pass.SetBindGroup(0, gD, nil)
	pass.Dispatch(1, 1, 1)
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	staging := newPublicBuffer(t, device, outSize, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
	encoder.CopyBufferToBuffer(out, 0, staging, 0, outSize)
	cmd, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	got := readPublicBuffer(t, staging, outSize/4)

	slot := func(i, word int) []uint32 { return got[i*dynamicSlot/4+word:][:3] }
	checkRebindWords(t, "step 1 (P1: gA, gB set before it)", slot(0, 0), []uint32{rebindTagA, rebindTagB0, 1})
	checkRebindWords(t, "step 2 (P2: gA kept, gC)", slot(1, 0), []uint32{rebindTagA, rebindTagC, 2})
	checkRebindWords(t, "step 3 (P1: gA kept, gB set again)", slot(2, 0), []uint32{rebindTagA, rebindTagB2, 1})
	checkRebindWords(t, "step 4 (P3: gD, gB kept with its offsets)", slot(2, 4), []uint32{rebindTagD, rebindTagB2, 3})
}

func checkBindGroupRebindRender(t *testing.T, device *wgpu.Device) {
	t.Helper()
	stages := wgpu.ShaderStageVertex | wgpu.ShaderStageFragment
	ls := newRebindLayouts(t, device, stages, []rebindEntry{
		{binding: 0, ty: gputypes.BufferBindingTypeUniform},
	})
	p0 := newRebindRenderPipeline(t, device, rebindRenderShaderP0, ls.la)
	p1 := newRebindRenderPipeline(t, device, rebindRenderShader(1, false), ls.la, ls.lb)
	p2 := newRebindRenderPipeline(t, device, rebindRenderShader(2, false), ls.laTwin, ls.lc)
	p3 := newRebindRenderPipeline(t, device, rebindRenderShader(3, true), ls.ld, ls.lb)

	aBuf := newRebindTagBuffer(t, device, []uint32{rebindTagJunk, rebindTagA, rebindTagD}, nil)
	// Slot i of the B buffer draws over column quarter i of the target.
	quarters := [][2]float32{{-1, -0.5}, {-0.5, 0}, {0, 0.5}}
	bBuf := newRebindTagBuffer(t, device, []uint32{rebindTagB0, rebindTagC, rebindTagB2}, quarters)
	gA := newRebindBindGroup(t, device, ls.la, rebindBinding{0, aBuf, 0})
	gD := newRebindBindGroup(t, device, ls.ld, rebindBinding{0, aBuf, 2 * dynamicSlot})
	gB := newRebindBindGroup(t, device, ls.lb, rebindBinding{0, bBuf, 0})
	gC := newRebindBindGroup(t, device, ls.lc, rebindBinding{0, bBuf, dynamicSlot})

	draw := gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1}
	px := renderDynamicTarget(t, device, func(pass *wgpu.RenderPassEncoder) {
		pass.SetBindGroup(0, gA, []uint32{dynamicSlot})
		pass.SetPipeline(p0)
		pass.SetBindGroup(1, gB, []uint32{0})
		pass.Draw(draw)
		pass.SetPipeline(p1)
		pass.Draw(draw)
		pass.SetPipeline(p2)
		pass.SetBindGroup(1, gC, nil)
		pass.Draw(draw)
		pass.SetPipeline(p1)
		pass.SetBindGroup(1, gB, []uint32{2 * dynamicSlot})
		pass.Draw(draw)
		pass.SetPipeline(p3)
		pass.SetBindGroup(0, gD, nil)
		pass.Draw(draw)
	})

	const quarter = featureTargetWidth / 4
	at := func(row, q int) uint32 { return px[row*featureTargetWidth+q*quarter+quarter/2] }
	top, bottom := 1, featureTargetHeight-2
	checkPixel(t, "step 1 (P1: gA, gB set before it)", at(top, 0), [4]uint32{rebindTagA, rebindTagB0, 1, 255})
	checkPixel(t, "step 2 (P2: gA kept, gC)", at(top, 1), [4]uint32{rebindTagA, rebindTagC, 2, 255})
	checkPixel(t, "step 3 (P1: gA kept, gB set again)", at(top, 2), [4]uint32{rebindTagA, rebindTagB2, 1, 255})
	checkPixel(t, "step 4 (P3: gD, gB kept with its offsets)", at(bottom, 2), [4]uint32{rebindTagD, rebindTagB2, 3, 255})
	checkPixel(t, "untouched quarter (P0 draws nothing)", at(top, 3), [4]uint32{255, 255, 255, 255})
}

func checkRebindWords(t *testing.T, what string, got, want []uint32) {
	t.Helper()
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: got %v, want %v (a.tag, b.tag, pipeline)", what, got[:len(want)], want)
			return
		}
	}
}

const rebindStructs = `
struct A { tag: vec4<u32>, pad: vec4<u32> }
struct B { rect: vec4<f32>, tag: vec4<u32> }
`

// rebindComputeShaderP0 uses group 0 only and writes nothing visible.
const rebindComputeShaderP0 = rebindStructs + `
@group(0) @binding(0) var<uniform> a: A;
var<workgroup> w: u32;

@compute @workgroup_size(1)
fn main() {
    w = a.tag.x;
}
`

// rebindComputeShader writes (a.tag, b.tag, marker) at words base..base+2.
func rebindComputeShader(marker, base uint32) string {
	return rebindStructs + fmt.Sprintf(`
@group(0) @binding(0) var<uniform> a: A;
@group(1) @binding(0) var<storage, read_write> out: array<u32>;
@group(1) @binding(1) var<uniform> b: A;

@compute @workgroup_size(1)
fn main() {
    out[%[2]du] = a.tag.x;
    out[%[2]du + 1u] = b.tag.x;
    out[%[2]du + 2u] = %[1]du;
}
`, marker, base)
}

// rebindRenderShaderP0 uses group 0 only; its triangle lies behind the far
// plane, so it draws nothing.
const rebindRenderShaderP0 = rebindStructs + `
@group(0) @binding(0) var<uniform> a: A;

struct VOut { @builtin(position) pos: vec4<f32> }

@vertex
fn vs(@builtin(vertex_index) v: u32) -> VOut {
    return VOut(vec4<f32>(f32(a.tag.x) * 0.0 + f32(v % 2u), 0.0, 2.0, 1.0));
}

@fragment
fn fs() -> @location(0) vec4<f32> {
    return vec4<f32>(0.0, 0.0, 0.0, 1.0);
}
`

// rebindRenderShader draws a quad over [b.rect.x, b.rect.y] in NDC x, in the
// top half (or the bottom half), colored (a.tag, b.tag, marker, 255).
func rebindRenderShader(marker uint32, bottom bool) string {
	yLo, yHi := "0.0", "1.0"
	if bottom {
		yLo, yHi = "-1.0", "0.0"
	}
	return rebindStructs + fmt.Sprintf(`
@group(0) @binding(0) var<uniform> a: A;
@group(1) @binding(0) var<uniform> b: B;

struct VOut { @builtin(position) pos: vec4<f32> }

@vertex
fn vs(@builtin(vertex_index) v: u32) -> VOut {
    var quad = array<vec2<f32>, 6>(
        vec2<f32>(-1.0, -1.0), vec2<f32>(1.0, -1.0), vec2<f32>(-1.0, 1.0),
        vec2<f32>(1.0, -1.0), vec2<f32>(1.0, 1.0), vec2<f32>(-1.0, 1.0),
    );
    let c = quad[v %% 6u];
    let x = select(b.rect.x, b.rect.y, c.x > 0.0);
    let y = select(%[2]s, %[3]s, c.y > 0.0);
    return VOut(vec4<f32>(x, y, 0.0, 1.0));
}

@fragment
fn fs() -> @location(0) vec4<f32> {
    return vec4<f32>(f32(a.tag.x), f32(b.tag.x), %[1]d.0, 255.0) / 255.0;
}
`, marker, yLo, yHi)
}

// rebindEntry is one buffer entry of the group 1 layouts.
type rebindEntry struct {
	binding uint32
	ty      gputypes.BufferBindingType
}

// newRebindLayouts creates the layouts of a test: LA and its twin LA' (a
// dynamic uniform buffer at binding 0), LD (the same entry without a dynamic
// offset), LB (group1 with dynamic offsets) and LC (group1 without).
func newRebindLayouts(t *testing.T, device *wgpu.Device, stages gputypes.ShaderStages, group1 []rebindEntry) rebindLayouts {
	t.Helper()
	uniformA := []rebindEntry{{binding: 0, ty: gputypes.BufferBindingTypeUniform}}
	return rebindLayouts{
		la:     newRebindLayout(t, device, "LA", stages, uniformA, true),
		laTwin: newRebindLayout(t, device, "LA'", stages, uniformA, true),
		ld:     newRebindLayout(t, device, "LD", stages, uniformA, false),
		lb:     newRebindLayout(t, device, "LB", stages, group1, true),
		lc:     newRebindLayout(t, device, "LC", stages, group1, false),
	}
}

func newRebindLayout(t *testing.T, device *wgpu.Device, label string, stages gputypes.ShaderStages, entries []rebindEntry, dynamic bool) *wgpu.BindGroupLayout {
	t.Helper()
	layoutEntries := make([]gputypes.BindGroupLayoutEntry, len(entries))
	for i, e := range entries {
		layoutEntries[i] = gputypes.BindGroupLayoutEntry{
			Binding:    e.binding,
			Visibility: stages,
			Buffer:     &gputypes.BufferBindingLayout{Type: e.ty, HasDynamicOffset: dynamic, MinBindingSize: rebindBindingSize},
		}
	}
	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{Label: label, Entries: layoutEntries})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout %s: %v", label, err)
	}
	t.Cleanup(bgl.Release)
	return bgl
}

func newRebindPipelineLayout(t *testing.T, device *wgpu.Device, groups ...*wgpu.BindGroupLayout) *wgpu.PipelineLayout {
	t.Helper()
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "rebind", BindGroupLayouts: groups})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	return layout
}

func newRebindComputePipeline(t *testing.T, device *wgpu.Device, wgsl string, groups ...*wgpu.BindGroupLayout) *wgpu.ComputePipeline {
	t.Helper()
	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "rebind",
		Layout:     newRebindPipelineLayout(t, device, groups...),
		Module:     newDynamicShader(t, device, wgsl),
		EntryPoint: "main",
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)
	return pipeline
}

func newRebindRenderPipeline(t *testing.T, device *wgpu.Device, wgsl string, groups ...*wgpu.BindGroupLayout) *wgpu.RenderPipeline {
	t.Helper()
	shader := newDynamicShader(t, device, wgsl)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "rebind",
		Layout: newRebindPipelineLayout(t, device, groups...),
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs"},
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fs",
			Targets: []gputypes.ColorTargetState{
				{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll},
			},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)
	return pipeline
}

// newRebindTagBuffer creates a uniform buffer with one 256-byte slot per tag.
// Without rects a slot holds struct A (tag, pad); with rects it holds struct
// B (rect, tag), rect.xy = rects[i].
func newRebindTagBuffer(t *testing.T, device *wgpu.Device, tags []uint32, rects [][2]float32) *wgpu.Buffer {
	t.Helper()
	data := make([]byte, len(tags)*dynamicSlot)
	for i, tag := range tags {
		words := []uint32{tag, 0, 0, 0}
		if rects != nil {
			words = []uint32{math.Float32bits(rects[i][0]), math.Float32bits(rects[i][1]), 0, 0, tag, 0, 0, 0}
		}
		putWords(data[i*dynamicSlot:], words)
	}
	buf := newPublicBuffer(t, device, uint64(len(data)), wgpu.BufferUsageUniform|wgpu.BufferUsageCopyDst)
	if err := device.Queue().WriteBuffer(buf, 0, data); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	return buf
}

// rebindBinding binds rebindBindingSize bytes of buffer at offset.
type rebindBinding struct {
	binding uint32
	buffer  *wgpu.Buffer
	offset  uint64
}

func newRebindBindGroup(t *testing.T, device *wgpu.Device, bgl *wgpu.BindGroupLayout, bindings ...rebindBinding) *wgpu.BindGroup {
	t.Helper()
	entries := make([]wgpu.BindGroupEntry, len(bindings))
	for i, b := range bindings {
		entries[i] = wgpu.BindGroupEntry{Binding: b.binding, Buffer: b.buffer, Offset: b.offset, Size: rebindBindingSize}
	}
	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{Label: "rebind", Layout: bgl, Entries: entries})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	t.Cleanup(bg.Release)
	return bg
}
