//go:build !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// TestBeginEncodingNilDevice verifies that BeginEncoding returns an error
// when the device is nil (VK-001).
func TestBeginEncodingNilDevice(t *testing.T) {
	enc := &CommandEncoder{
		device: nil,
	}

	err := enc.BeginEncoding("test")
	if err == nil {
		t.Fatal("BeginEncoding with nil device should return error")
	}

	expected := "vulkan: BeginEncoding called with nil device"
	if err.Error() != expected {
		t.Errorf("error = %q, want %q", err.Error(), expected)
	}
}

// TestEndEncodingNotRecording verifies that EndEncoding returns an error
// when the encoder is not recording (active == 0).
func TestEndEncodingNotRecording(t *testing.T) {
	enc := &CommandEncoder{
		device: &Device{},
		active: 0, // not recording
	}

	_, err := enc.EndEncoding()
	if err == nil {
		t.Fatal("EndEncoding when not recording should return error")
	}

	expected := "vulkan: command encoder is not recording"
	if err.Error() != expected {
		t.Errorf("error = %q, want %q", err.Error(), expected)
	}
}

// TestTransitionTexturesNullActive verifies that TransitionTextures
// silently returns when active is 0 (VK-001 defense-in-depth).
func TestTransitionTexturesNullActive(t *testing.T) {
	enc := &CommandEncoder{
		device: &Device{},
		active: 0,
	}

	// Should not panic — active == 0 guard catches null handle.
	enc.TransitionTextures([]hal.TextureBarrier{
		{}, // dummy barrier
	})
}

// TestTransitionTexturesNotRecording verifies that TransitionTextures
// silently returns when not recording (active == 0).
func TestTransitionTexturesNotRecording(t *testing.T) {
	enc := &CommandEncoder{
		device: &Device{},
		active: 0,
	}

	// Should not panic.
	enc.TransitionTextures([]hal.TextureBarrier{
		{},
	})
}

// TestTransitionBuffersNullActive verifies that TransitionBuffers
// silently returns when active is 0 (VK-001 defense-in-depth).
func TestTransitionBuffersNullActive(t *testing.T) {
	enc := &CommandEncoder{
		device: &Device{},
		active: 0,
	}

	// Should not panic.
	enc.TransitionBuffers([]hal.BufferBarrier{
		{},
	})
}

// TestClearBufferNullActive verifies that ClearBuffer silently returns
// when active is 0 (VK-001 defense-in-depth).
func TestClearBufferNullActive(t *testing.T) {
	enc := &CommandEncoder{
		device: &Device{},
		active: 0,
	}

	// Should not panic.
	enc.ClearBuffer(&Buffer{handle: 1}, 0, 256)
}

// TestRenderPassEncoderDrawNullActive verifies that Draw silently returns
// when the underlying active command buffer is 0 (VK-001 defense-in-depth).
func TestRenderPassEncoderDrawNullActive(t *testing.T) {
	enc := &CommandEncoder{
		device: &Device{},
		active: 0,
	}
	rpe := &RenderPassEncoder{encoder: enc}

	// Should not panic.
	rpe.Draw(3, 1, 0, 0)
}

// TestRenderPassEncoderEndNullActive verifies that End silently returns
// when the underlying active command buffer is 0 (VK-001 defense-in-depth).
func TestRenderPassEncoderEndNullActive(t *testing.T) {
	enc := &CommandEncoder{
		device: &Device{},
		active: 0,
	}
	rpe := &RenderPassEncoder{encoder: enc}

	// Should not panic.
	rpe.End()
}

// TestComputePassEncoderEndNullActive verifies that End silently returns
// when the underlying active command buffer is 0 (VK-001 defense-in-depth).
func TestComputePassEncoderEndNullActive(t *testing.T) {
	enc := &CommandEncoder{
		device: &Device{},
		active: 0,
	}
	cpe := &ComputePassEncoder{encoder: enc}

	// Should not panic.
	cpe.End()
}

// TestComputePassEncoderDispatchNullActive verifies that Dispatch silently
// returns when the underlying active command buffer is 0 (VK-001 defense-in-depth).
func TestComputePassEncoderDispatchNullActive(t *testing.T) {
	enc := &CommandEncoder{
		device: &Device{},
		active: 0,
	}
	cpe := &ComputePassEncoder{encoder: enc}

	// Should not panic.
	cpe.Dispatch(1, 1, 1)
}

// TestRenderPassEncoderSetViewportNullActive verifies that SetViewport
// silently returns when active is 0 (VK-001 defense-in-depth).
func TestRenderPassEncoderSetViewportNullActive(t *testing.T) {
	enc := &CommandEncoder{
		device: &Device{},
		active: 0,
	}
	rpe := &RenderPassEncoder{encoder: enc}

	// Should not panic.
	rpe.SetViewport(0, 0, 800, 600, 0, 1)
}

// TestRenderPassEncoderSetScissorNullActive verifies that SetScissorRect
// silently returns when active is 0 (VK-001 defense-in-depth).
func TestRenderPassEncoderSetScissorNullActive(t *testing.T) {
	enc := &CommandEncoder{
		device: &Device{},
		active: 0,
	}
	rpe := &RenderPassEncoder{encoder: enc}

	// Should not panic.
	rpe.SetScissorRect(0, 0, 800, 600)
}

// TestCopyBufferToBufferNullActive verifies that CopyBufferToBuffer
// silently returns when active is 0 (VK-001 defense-in-depth).
func TestCopyBufferToBufferNullActive(t *testing.T) {
	enc := &CommandEncoder{
		device: &Device{},
		active: 0,
	}

	// Should not panic.
	enc.CopyBufferToBuffer(&Buffer{handle: 1}, &Buffer{handle: 2}, []hal.BufferCopy{
		{SrcOffset: 0, DstOffset: 0, Size: 256},
	})
}

// TestBeginRenderPassNullActive verifies that BeginRenderPass returns
// an empty encoder when active is 0 (VK-001 defense-in-depth).
func TestBeginRenderPassNullActive(t *testing.T) {
	enc := &CommandEncoder{
		device: &Device{},
		active: 0,
	}

	rpe := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{
				ClearValue: gputypes.Color{R: 0, G: 0, B: 0, A: 1},
			},
		},
	})

	// Should return a non-nil encoder that does nothing.
	if rpe == nil {
		t.Fatal("BeginRenderPass should return non-nil encoder even with null active")
	}
}

// TestDiscardEncodingNotRecording verifies that DiscardEncoding does not
// panic when active is 0 (not recording).
func TestDiscardEncodingNotRecording(t *testing.T) {
	enc := &CommandEncoder{
		device: &Device{},
		active: 0,
	}

	// Should not panic.
	enc.DiscardEncoding()
}

// TestDiscardEncodingTracksActive verifies that DiscardEncoding moves the
// active command buffer to the discarded list (VK-CMD-001).
func TestDiscardEncodingTracksActive(t *testing.T) {
	enc := &CommandEncoder{
		device:      &Device{},
		active:      42, // simulated active handle
		poolManaged: true,
	}

	enc.DiscardEncoding()

	if enc.active != 0 {
		t.Errorf("active = %d, want 0 after discard", enc.active)
	}
	if len(enc.discarded) != 1 || enc.discarded[0] != 42 {
		t.Errorf("discarded = %v, want [42]", enc.discarded)
	}
}

// TestResetAllRecyclesCBs verifies that ResetAll moves completed and
// discarded command buffers back to the free list (VK-CMD-001).
func TestResetAllRecyclesCBs(t *testing.T) {
	enc := &CommandEncoder{
		device:    &Device{},
		discarded: []vk.CommandBuffer{10, 20},
	}

	// Simulate completed command buffers returned from GPU.
	completed := []hal.CommandBuffer{
		&CommandBuffer{handle: 30},
		&CommandBuffer{handle: 40},
	}

	enc.ResetAll(completed)

	// Completed + discarded should be in free list.
	if len(enc.free) != 4 {
		t.Fatalf("free = %d, want 4", len(enc.free))
	}
	if len(enc.discarded) != 0 {
		t.Errorf("discarded = %d, want 0", len(enc.discarded))
	}

	// Verify the handles are present (order: completed first, then discarded).
	expected := map[vk.CommandBuffer]bool{10: true, 20: true, 30: true, 40: true}
	for _, h := range enc.free {
		if !expected[h] {
			t.Errorf("unexpected handle %d in free list", h)
		}
	}
}

// TestAllocationGranularity verifies the batch allocation constant.
func TestAllocationGranularity(t *testing.T) {
	if allocationGranularity != 16 {
		t.Errorf("allocationGranularity = %d, want 16 (Rust wgpu-hal parity)", allocationGranularity)
	}
}
