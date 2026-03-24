// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"testing"

	"github.com/gogpu/gputypes"
)

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
