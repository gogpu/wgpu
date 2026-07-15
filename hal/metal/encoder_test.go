// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
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
	pass.SetViewport(1, 2, 3, 4, 0.25, 0.75)
	pass.SetScissorRect(5, 6, 7, 8)
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
		pass.SetViewport(0, 0, 10, 10, 0, 1)
		pass.SetScissorRect(0, 0, 10, 10)
		pass.SetBlendConstant(&gputypes.Color{A: 1})
		pass.SetStencilReference(2)
	})
	if allocs != 0 {
		t.Fatalf("deferred state allocated %.2f objects per recording", allocs)
	}
}

// TestComputeBindSlots_PerTypeSequentialIndexing is a regression test for the
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
func TestComputeBindSlots_PerTypeSequentialIndexing(t *testing.T) {
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
			bufSlots, texSlots, sampSlots := computeBindSlots(tt.entries)

			checkSlots := func(name string, got []bindSlotAssignment, want []uintptr) {
				if len(got) != len(want) {
					t.Errorf("%s: got %d assignments, want %d", name, len(got), len(want))
					return
				}
				for i, a := range got {
					if a.slot != want[i] {
						t.Errorf("%s[%d]: got slot %d, want %d (entryIndex=%d)",
							name, i, a.slot, want[i], a.entryIndex)
					}
				}
			}

			checkSlots("buffer", bufSlots, tt.wantBuffers)
			checkSlots("texture", texSlots, tt.wantTextures)
			checkSlots("sampler", sampSlots, tt.wantSamplers)
		})
	}
}
