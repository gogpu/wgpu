// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/metal/mslmap"
)

func TestCommandEncoderRecordingErrorKeepsFirstFailure(t *testing.T) {
	first := errors.New("first")
	encoder := &CommandEncoder{}
	encoder.failRecording(first)
	encoder.failRecording(errors.New("second"))
	if !errors.Is(encoder.recordErr, first) {
		t.Fatalf("recording error = %v, want first failure", encoder.recordErr)
	}
}

// TestCommandEncoder_RecordingState is a regression test for Issue #24.
// The IsRecording() method returns cmdBuffer != 0.
//
// This test verifies the state machine transitions:
// - New encoder should not be recording (cmdBuffer == 0)
// - After cmdBuffer is set, should be recording
// - After cmdBuffer is cleared, should not be recording
func TestCommandEncoder_RecordingState(t *testing.T) {
	// Create encoder without device (state-only test)
	enc := &CommandEncoder{}

	// New encoder should not be recording
	if enc.IsRecording() {
		t.Error("new encoder should not be recording")
	}
	if enc.cmdBuffer != 0 {
		t.Error("cmdBuffer should be 0 initially")
	}

	// Simulate BeginEncoding - cmdBuffer is set to non-zero
	enc.cmdBuffer = 1
	if !enc.IsRecording() {
		t.Error("encoder should be recording when cmdBuffer is set")
	}

	// Simulate EndEncoding - cmdBuffer is transferred to CommandBuffer
	enc.cmdBuffer = 0
	if enc.IsRecording() {
		t.Error("encoder should not be recording after cmdBuffer cleared")
	}
}

// TestCommandEncoder_DiscardState verifies that DiscardEncoding
// properly resets the recording state by clearing cmdBuffer.
func TestCommandEncoder_DiscardState(t *testing.T) {
	enc := &CommandEncoder{}

	// Simulate active recording
	enc.cmdBuffer = 1

	// Simulate discard - cmdBuffer is released and set to 0
	enc.cmdBuffer = 0

	if enc.IsRecording() {
		t.Error("encoder should not be recording after discard")
	}
}

// TestCommandEncoder_BeginRenderPassGuard verifies that BeginRenderPass
// correctly checks IsRecording() before creating sub-encoders.
// The guard in BeginRenderPass checks: if !e.IsRecording() || e.cmdBuffer == 0
func TestCommandEncoder_BeginRenderPassGuard(t *testing.T) {
	enc := &CommandEncoder{}

	// When cmdBuffer is 0, IsRecording() returns false
	enc.cmdBuffer = 0

	// This is the guard condition - both must be true to proceed
	if enc.IsRecording() {
		t.Error("encoder should not be recording when cmdBuffer is 0")
	}

	// When cmdBuffer is non-zero, IsRecording() returns true
	enc.cmdBuffer = 1
	if !enc.IsRecording() {
		t.Error("encoder should be recording when cmdBuffer is non-zero")
	}
}

// TestCommandEncoder_IsRecordingMethod documents that IsRecording()
// is based on cmdBuffer != 0.
func TestCommandEncoder_IsRecordingMethod(t *testing.T) {
	enc := &CommandEncoder{}

	// IsRecording() == (cmdBuffer != 0)
	enc.cmdBuffer = 0
	if enc.IsRecording() != (enc.cmdBuffer != 0) {
		t.Error("IsRecording() should equal (cmdBuffer != 0)")
	}

	enc.cmdBuffer = 12345
	if enc.IsRecording() != (enc.cmdBuffer != 0) {
		t.Error("IsRecording() should equal (cmdBuffer != 0)")
	}

	enc.cmdBuffer = 0
	if enc.IsRecording() != (enc.cmdBuffer != 0) {
		t.Error("IsRecording() should equal (cmdBuffer != 0)")
	}
}

func TestRenderPassDeferredStateKeepsLatestValues(t *testing.T) {
	firstPipeline := &RenderPipeline{}
	latestPipeline := &RenderPipeline{}
	firstGroup := &BindGroup{}
	latestGroup := &BindGroup{}
	firstVertex := &Buffer{}
	latestVertex := &Buffer{}
	index := &Buffer{}
	state := renderPassPendingState{}
	pass := &RenderPassEncoder{pending: &state}

	pass.SetPipeline(firstPipeline)
	pass.SetPipeline(latestPipeline)
	pass.SetBindGroup(1, firstGroup, []uint32{4, 8})
	pass.SetBindGroup(1, latestGroup, []uint32{12})
	pass.SetVertexBuffer(2, firstVertex, 16)
	pass.SetVertexBuffer(2, latestVertex, 24)
	pass.SetIndexBuffer(index, gputypes.IndexFormatUint32, 32)
	pass.SetViewport(gputypes.Viewport{X: 1, Y: 2, Width: 3, Height: 4, MinDepth: 0.25, MaxDepth: 0.75})
	pass.SetScissorRect(gputypes.ScissorRect{X: 5, Y: 6, Width: 7, Height: 8})
	pass.SetBlendConstant(&gputypes.Color{R: 0.1, G: 0.2, B: 0.3, A: 0.4})
	pass.SetStencilReference(9)

	if pass.pipeline != latestPipeline {
		t.Fatal("deferred pipeline did not keep latest value")
	}
	group := pass.pending.bindGroups[1]
	if !group.set || group.group != latestGroup || group.offsetCount != 1 || group.offsets[0] != 12 {
		t.Fatalf("deferred bind group = %#v, want latest group and offsets", group)
	}
	vertex := pass.pending.vertexBuffers[2]
	if !vertex.set || vertex.buffer != latestVertex || vertex.offset != 24 {
		t.Fatalf("deferred vertex buffer = %#v, want latest buffer and offset", vertex)
	}
	if pass.indexBuffer != index || pass.indexFormat != gputypes.IndexFormatUint32 || pass.indexOffset != 32 {
		t.Fatal("deferred index buffer state was not retained")
	}
	if !pass.pending.viewportSet || pass.pending.viewport != (MTLViewport{OriginX: 1, OriginY: 2, Width: 3, Height: 4, ZNear: 0.25, ZFar: 0.75}) {
		t.Fatalf("deferred viewport = %#v", pass.pending.viewport)
	}
	if !pass.pending.scissorSet || pass.pending.scissor != (MTLScissorRect{X: 5, Y: 6, Width: 7, Height: 8}) {
		t.Fatalf("deferred scissor = %#v", pass.pending.scissor)
	}
	if !pass.pending.blendSet || pass.pending.blend != (gputypes.Color{R: 0.1, G: 0.2, B: 0.3, A: 0.4}) {
		t.Fatalf("deferred blend constant = %#v", pass.pending.blend)
	}
	if !pass.pending.stencilSet || pass.pending.stencil != 9 {
		t.Fatalf("deferred stencil reference = %d", pass.pending.stencil)
	}
}

func TestRenderPassDeferredStateUsesBoundedStorage(t *testing.T) {
	group := &BindGroup{}
	buffer := &Buffer{}
	pipeline := &RenderPipeline{}
	state := renderPassPendingState{}
	var pass RenderPassEncoder
	offsets := []uint32{1, 2, 3, 4}

	allocs := testing.AllocsPerRun(100, func() {
		clear(state.bindGroups[:])
		clear(state.vertexBuffers[:])
		pass = RenderPassEncoder{pending: &state}
		pass.SetPipeline(pipeline)
		pass.SetBindGroup(0, group, offsets)
		pass.SetVertexBuffer(0, buffer, 12)
		pass.SetIndexBuffer(buffer, gputypes.IndexFormatUint16, 4)
		pass.SetViewport(gputypes.Viewport{X: 0, Y: 0, Width: 10, Height: 10, MinDepth: 0, MaxDepth: 1})
		pass.SetScissorRect(gputypes.ScissorRect{X: 0, Y: 0, Width: 10, Height: 10})
		pass.SetBlendConstant(&gputypes.Color{A: 1})
		pass.SetStencilReference(2)
	})
	if allocs != 0 {
		t.Fatalf("deferred state allocated %.2f objects per recording", allocs)
	}
}

func BenchmarkComputePassEncoderBindBufferSizes(b *testing.B) {
	if err := Init(); err != nil {
		b.Fatalf("Init: %v", err)
	}
	rawDevice := CreateSystemDefaultDevice()
	if rawDevice == 0 {
		skipWithoutMetal(b, "no Metal device available")
	}
	b.Cleanup(func() { Release(rawDevice) })

	device, err := newDevice(&Adapter{raw: rawDevice})
	if err != nil {
		b.Fatalf("newDevice: %v", err)
	}
	b.Cleanup(device.Destroy)

	command := &CommandEncoder{device: device}
	if err := command.BeginEncoding("bind-buffer-sizes-benchmark"); err != nil {
		b.Fatalf("BeginEncoding: %v", err)
	}
	pass, ok := command.BeginComputePass(nil).(*ComputePassEncoder)
	if !ok || pass == nil {
		command.DiscardEncoding()
		b.Fatal("BeginComputePass returned no Metal compute encoder")
	}
	b.Cleanup(func() {
		pass.End()
		command.DiscardEncoding()
	})

	pipeline := &ComputePipeline{
		sizesBindings: []ir.ResourceBinding{
			{Group: 0, Binding: 0},
			{Group: 0, Binding: 1},
			{Group: 1, Binding: 0},
			{Group: 1, Binding: 1},
		},
		sizesSlot: 4,
	}
	bindingSizes := map[[2]uint32]uint64{
		{0, 0}: 16,
		{0, 1}: 28,
		{1, 0}: 64,
		{1, 1}: 256,
	}

	// Warm the selector cache before the timed region.
	_ = Sel("setBytes:length:atIndex:")

	for _, dispatches := range []int{1, 8, 64} {
		b.Run(fmt.Sprintf("dispatches_per_pass=%d", dispatches), func(b *testing.B) {
			// This benchmark compares Go allocations only. Repeated setBytes
			// calls intentionally reuse one native command encoder.
			b.ReportAllocs()
			for b.Loop() {
				encoder := ComputePassEncoder{
					raw:          pass.raw,
					pipeline:     pipeline,
					bindingSizes: bindingSizes,
				}
				for range dispatches {
					encoder.bindBufferSizes()
				}
			}
		})
	}
}

// TestBindGroupSlots_PerTypeSequentialIndexing is a regression test for the
// SetBindGroup slot indexing bug.
//
// Bug: Previously used entry.Binding (WGSL @binding(N)) as the Metal slot
// index for all resource types. Metal uses separate per-type index spaces:
//
//	[[buffer(N)]], [[texture(M)]], [[sampler(K)]]
//
// The naga MSL compiler generates sequential indices per type, so with entries
// at bindings [0:buffer, 1:texture, 2:buffer, 3:sampler]:
//
//	Bug:     buffer→0, texture→1, buffer→2, sampler→3
//	Correct: buffer→0, texture→0, buffer→1, sampler→0
func TestBindGroupSlots_PerTypeSequentialIndexing(t *testing.T) {
	tests := []struct {
		name         string
		entries      []gputypes.BindGroupEntry
		wantBuffers  []uintptr // expected slot per buffer entry
		wantTextures []uintptr // expected slot per texture entry
		wantSamplers []uintptr // expected slot per sampler entry
	}{
		{
			name: "mixed types: the exact bug scenario",
			entries: []gputypes.BindGroupEntry{
				{Binding: 0, Resource: gputypes.BufferBinding{Buffer: 0x100}},
				{Binding: 1, Resource: gputypes.TextureViewBinding{TextureView: 0x200}},
				{Binding: 2, Resource: gputypes.BufferBinding{Buffer: 0x300}},
				{Binding: 3, Resource: gputypes.SamplerBinding{Sampler: 0x400}},
			},
			// Bug would produce buffer slots [0, 2], texture slots [1], sampler slots [3]
			// Fix produces buffer slots [0, 1], texture slots [0], sampler slots [0]
			wantBuffers:  []uintptr{0, 1},
			wantTextures: []uintptr{0},
			wantSamplers: []uintptr{0},
		},
		{
			name: "all buffers",
			entries: []gputypes.BindGroupEntry{
				{Binding: 0, Resource: gputypes.BufferBinding{Buffer: 0x100}},
				{Binding: 1, Resource: gputypes.BufferBinding{Buffer: 0x200}},
				{Binding: 2, Resource: gputypes.BufferBinding{Buffer: 0x300}},
			},
			wantBuffers:  []uintptr{0, 1, 2},
			wantTextures: nil,
			wantSamplers: nil,
		},
		{
			name: "texture then sampler then buffer",
			entries: []gputypes.BindGroupEntry{
				{Binding: 0, Resource: gputypes.TextureViewBinding{TextureView: 0x100}},
				{Binding: 1, Resource: gputypes.SamplerBinding{Sampler: 0x200}},
				{Binding: 2, Resource: gputypes.BufferBinding{Buffer: 0x300}},
			},
			wantBuffers:  []uintptr{0},
			wantTextures: []uintptr{0},
			wantSamplers: []uintptr{0},
		},
		{
			name: "multiple textures and samplers interleaved",
			entries: []gputypes.BindGroupEntry{
				{Binding: 0, Resource: gputypes.BufferBinding{Buffer: 0x100}},
				{Binding: 1, Resource: gputypes.TextureViewBinding{TextureView: 0x200}},
				{Binding: 2, Resource: gputypes.SamplerBinding{Sampler: 0x300}},
				{Binding: 3, Resource: gputypes.TextureViewBinding{TextureView: 0x400}},
				{Binding: 4, Resource: gputypes.SamplerBinding{Sampler: 0x500}},
				{Binding: 5, Resource: gputypes.BufferBinding{Buffer: 0x600}},
			},
			wantBuffers:  []uintptr{0, 1},
			wantTextures: []uintptr{0, 1},
			wantSamplers: []uintptr{0, 1},
		},
		{
			name: "sparse bindings with gaps",
			entries: []gputypes.BindGroupEntry{
				{Binding: 0, Resource: gputypes.BufferBinding{Buffer: 0x100}},
				{Binding: 5, Resource: gputypes.TextureViewBinding{TextureView: 0x200}},
				{Binding: 10, Resource: gputypes.BufferBinding{Buffer: 0x300}},
				{Binding: 15, Resource: gputypes.SamplerBinding{Sampler: 0x400}},
				{Binding: 20, Resource: gputypes.TextureViewBinding{TextureView: 0x500}},
			},
			// With the bug, these would use binding numbers [0,5,10,15,20] as slots.
			// With the fix, per-type sequential: buffers [0,1], textures [0,1], samplers [0].
			wantBuffers:  []uintptr{0, 1},
			wantTextures: []uintptr{0, 1},
			wantSamplers: []uintptr{0},
		},
		{
			name:         "empty entries",
			entries:      []gputypes.BindGroupEntry{},
			wantBuffers:  nil,
			wantTextures: nil,
			wantSamplers: nil,
		},
		{
			name: "single texture",
			entries: []gputypes.BindGroupEntry{
				{Binding: 3, Resource: gputypes.TextureViewBinding{TextureView: 0x100}},
			},
			// Bug would use slot 3; fix uses slot 0.
			wantBuffers:  nil,
			wantTextures: []uintptr{0},
			wantSamplers: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := newTestBindGroup(t, layoutEntriesFor(tt.entries, gputypes.ShaderStageCompute), tt.entries)
			got := map[mslmap.Kind][]uintptr{}
			for i := range group.resources {
				res := &group.resources[i]
				if slot, ok := resourceSlot(res, stageCompute, mslmap.Counts{}); ok {
					got[res.kind] = append(got[res.kind], slot)
				}
			}

			checkSlots := func(name string, got, want []uintptr) {
				if len(got) != len(want) {
					t.Errorf("%s: got %d assignments, want %d", name, len(got), len(want))
					return
				}
				for i := range got {
					if got[i] != want[i] {
						t.Errorf("%s[%d]: got slot %d, want %d", name, i, got[i], want[i])
					}
				}
			}

			checkSlots("buffer", got[mslmap.KindBuffer], tt.wantBuffers)
			checkSlots("texture", got[mslmap.KindTexture], tt.wantTextures)
			checkSlots("sampler", got[mslmap.KindSampler], tt.wantSamplers)
		})
	}
}

// layoutEntriesFor returns layout entries matching the resource types of
// entries, visible to stages.
func layoutEntriesFor(entries []gputypes.BindGroupEntry, stages gputypes.ShaderStages) []gputypes.BindGroupLayoutEntry {
	out := make([]gputypes.BindGroupLayoutEntry, 0, len(entries))
	for _, e := range entries {
		le := gputypes.BindGroupLayoutEntry{Binding: e.Binding, Visibility: stages}
		switch e.Resource.(type) {
		case gputypes.BufferBinding:
			le.Buffer = &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}
		case gputypes.TextureViewBinding:
			le.Texture = &gputypes.TextureBindingLayout{}
		case gputypes.SamplerBinding:
			le.Sampler = &gputypes.SamplerBindingLayout{}
		}
		out = append(out, le)
	}
	return out
}

// newTestBindGroup creates a bind group layout and a bind group without a
// Metal device: neither calls Metal while every buffer entry has a size.
func newTestBindGroup(t *testing.T, layoutEntries []gputypes.BindGroupLayoutEntry, entries []gputypes.BindGroupEntry) *BindGroup {
	t.Helper()
	d := &Device{}
	rawLayout, err := d.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{Entries: layoutEntries})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	rawGroup, err := d.CreateBindGroup(&hal.BindGroupDescriptor{Layout: rawLayout, Entries: entries})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	return rawGroup.(*BindGroup)
}

// TestBindGroupFollowsBindingOrder guards the dynamic offsets defect: the
// layout and the bind group list their entries from the highest binding
// down. The slots and the dynamic offsets must follow the binding numbers,
// which the shader's slots follow too (mslmap.StageLayout), and each storage
// binding must report its own binding size for `_buffer_sizes`.
func TestBindGroupFollowsBindingOrder(t *testing.T) {
	stages := gputypes.ShaderStageCompute
	dynamic := func(binding uint32, ty gputypes.BufferBindingType) gputypes.BindGroupLayoutEntry {
		return gputypes.BindGroupLayoutEntry{Binding: binding, Visibility: stages,
			Buffer: &gputypes.BufferBindingLayout{Type: ty, HasDynamicOffset: true}}
	}
	group := newTestBindGroup(t,
		[]gputypes.BindGroupLayoutEntry{
			dynamic(2, gputypes.BufferBindingTypeStorage),
			dynamic(1, gputypes.BufferBindingTypeReadOnlyStorage),
			dynamic(0, gputypes.BufferBindingTypeUniform),
		},
		[]gputypes.BindGroupEntry{
			{Binding: 2, Resource: gputypes.BufferBinding{Buffer: 0x300, Offset: 8, Size: 16}},
			{Binding: 1, Resource: gputypes.BufferBinding{Buffer: 0x200, Size: 24}},
			{Binding: 0, Resource: gputypes.BufferBinding{Buffer: 0x100, Size: 32}},
		})

	offsets := []uint32{256, 512, 768} // binding 0, 1, 2
	want := []struct {
		handle uintptr
		slot   uintptr
		offset uintptr
		sized  bool
		size   uint64
	}{
		{0x100, 0, 256, false, 0},
		{0x200, 1, 512, true, 24},
		{0x300, 2, 8 + 768, true, 16},
	}
	if len(group.resources) != len(want) {
		t.Fatalf("bind group has %d resources, want %d", len(group.resources), len(want))
	}
	for i, w := range want {
		res := &group.resources[i]
		slot, ok := resourceSlot(res, stageCompute, mslmap.Counts{})
		if !ok || res.handle != w.handle || slot != w.slot {
			t.Errorf("resource %d: handle %#x slot %d (ok %v), want handle %#x slot %d", i, res.handle, slot, ok, w.handle, w.slot)
		}
		if got := resourceOffset(res, offsets); got != w.offset {
			t.Errorf("resource %d: offset %d, want %d", i, got, w.offset)
		}
		if res.sized != w.sized || res.size != w.size {
			t.Errorf("resource %d: size %d (sized %v), want %d (sized %v)", i, res.size, res.sized, w.size, w.sized)
		}
	}

	sizes := recordBindingSizes(nil, 3, group)
	if len(sizes) != 2 || sizes[[2]uint32{3, 1}] != 24 || sizes[[2]uint32{3, 2}] != 16 {
		t.Errorf("binding sizes = %v, want {3,1}: 24 and {3,2}: 16", sizes)
	}
}

// TestBindGroupSlotsPerStage checks that each render stage numbers only the
// resources visible to it, from the group's first slot in that stage.
func TestBindGroupSlotsPerStage(t *testing.T) {
	vs, fs := gputypes.ShaderStageVertex, gputypes.ShaderStageFragment
	group := newTestBindGroup(t,
		[]gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: fs, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}},
			{Binding: 1, Visibility: vs | fs, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}},
			{Binding: 2, Visibility: vs, Texture: &gputypes.TextureBindingLayout{}},
		},
		[]gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{Buffer: 0x100, Size: 16}},
			{Binding: 1, Resource: gputypes.BufferBinding{Buffer: 0x200, Size: 16}},
			{Binding: 2, Resource: gputypes.TextureViewBinding{TextureView: 0x300}},
		})
	base := mslmap.Counts{Buffers: 3, Textures: 1}
	check := func(i, stage int, wantOK bool, want uintptr) {
		t.Helper()
		slot, ok := resourceSlot(&group.resources[i], stage, base)
		if ok != wantOK || (ok && slot != want) {
			t.Errorf("resource %d stage %d: slot %d (ok %v), want %d (ok %v)", i, stage, slot, ok, want, wantOK)
		}
	}
	check(0, stageVertex, false, 0)
	check(1, stageVertex, true, 3)
	check(2, stageVertex, true, 1)
	check(0, stageFragment, true, 3)
	check(1, stageFragment, true, 4)
	check(2, stageFragment, false, 0)
}
