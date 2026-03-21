// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// TestBeginEncodingNullCmdBuffer verifies that BeginEncoding returns an error
// when the command buffer handle is null (VK-001).
func TestBeginEncodingNullCmdBuffer(t *testing.T) {
	enc := &CommandEncoder{
		device:    &Device{},
		cmdBuffer: 0, // null handle
	}

	err := enc.BeginEncoding("test")
	if err == nil {
		t.Fatal("BeginEncoding with null cmdBuffer should return error")
	}

	expected := "vulkan: BeginEncoding called with null command buffer handle"
	if err.Error() != expected {
		t.Errorf("error = %q, want %q", err.Error(), expected)
	}
}

// TestBeginEncodingNilDevice verifies that BeginEncoding returns an error
// when the device is nil (VK-001).
func TestBeginEncodingNilDevice(t *testing.T) {
	enc := &CommandEncoder{
		device:    nil,
		cmdBuffer: 42, // non-null handle
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

// TestEndEncodingNullCmdBuffer verifies that EndEncoding returns an error
// when the command buffer handle is null (VK-001).
func TestEndEncodingNullCmdBuffer(t *testing.T) {
	enc := &CommandEncoder{
		device:      &Device{},
		cmdBuffer:   0, // null handle
		isRecording: true,
	}

	_, err := enc.EndEncoding()
	if err == nil {
		t.Fatal("EndEncoding with null cmdBuffer should return error")
	}

	expected := "vulkan: EndEncoding called with null command buffer handle"
	if err.Error() != expected {
		t.Errorf("error = %q, want %q", err.Error(), expected)
	}
}

// TestEndEncodingNotRecording verifies that EndEncoding returns an error
// when the encoder is not recording.
func TestEndEncodingNotRecording(t *testing.T) {
	enc := &CommandEncoder{
		device:      &Device{},
		cmdBuffer:   42,
		isRecording: false,
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

// TestTransitionTexturesNullCmdBuffer verifies that TransitionTextures
// silently returns when cmdBuffer is null (VK-001 defense-in-depth).
func TestTransitionTexturesNullCmdBuffer(t *testing.T) {
	enc := &CommandEncoder{
		device:      &Device{},
		cmdBuffer:   0,
		isRecording: true, // Simulates inconsistent state
	}

	// Should not panic — defense-in-depth guard catches null handle.
	enc.TransitionTextures([]hal.TextureBarrier{
		{}, // dummy barrier
	})
}

// TestTransitionTexturesNotRecording verifies that TransitionTextures
// silently returns when not recording.
func TestTransitionTexturesNotRecording(t *testing.T) {
	enc := &CommandEncoder{
		device:      &Device{},
		cmdBuffer:   42,
		isRecording: false,
	}

	// Should not panic.
	enc.TransitionTextures([]hal.TextureBarrier{
		{},
	})
}

// TestTransitionBuffersNullCmdBuffer verifies that TransitionBuffers
// silently returns when cmdBuffer is null (VK-001 defense-in-depth).
func TestTransitionBuffersNullCmdBuffer(t *testing.T) {
	enc := &CommandEncoder{
		device:      &Device{},
		cmdBuffer:   0,
		isRecording: true,
	}

	// Should not panic.
	enc.TransitionBuffers([]hal.BufferBarrier{
		{},
	})
}

// TestClearBufferNullCmdBuffer verifies that ClearBuffer silently returns
// when cmdBuffer is null (VK-001 defense-in-depth).
func TestClearBufferNullCmdBuffer(t *testing.T) {
	enc := &CommandEncoder{
		device:      &Device{},
		cmdBuffer:   0,
		isRecording: true,
	}

	// Should not panic.
	enc.ClearBuffer(&Buffer{handle: 1}, 0, 256)
}

// TestRenderPassEncoderDrawNullCmdBuffer verifies that Draw silently returns
// when the underlying command buffer is null (VK-001 defense-in-depth).
func TestRenderPassEncoderDrawNullCmdBuffer(t *testing.T) {
	enc := &CommandEncoder{
		device:      &Device{},
		cmdBuffer:   0,
		isRecording: true,
	}
	rpe := &RenderPassEncoder{encoder: enc}

	// Should not panic.
	rpe.Draw(3, 1, 0, 0)
}

// TestRenderPassEncoderEndNullCmdBuffer verifies that End silently returns
// when the underlying command buffer is null (VK-001 defense-in-depth).
func TestRenderPassEncoderEndNullCmdBuffer(t *testing.T) {
	enc := &CommandEncoder{
		device:      &Device{},
		cmdBuffer:   0,
		isRecording: true,
	}
	rpe := &RenderPassEncoder{encoder: enc}

	// Should not panic.
	rpe.End()
}

// TestComputePassEncoderEndNullCmdBuffer verifies that End silently returns
// when the underlying command buffer is null (VK-001 defense-in-depth).
func TestComputePassEncoderEndNullCmdBuffer(t *testing.T) {
	enc := &CommandEncoder{
		device:      &Device{},
		cmdBuffer:   0,
		isRecording: true,
	}
	cpe := &ComputePassEncoder{encoder: enc}

	// Should not panic.
	cpe.End()
}

// TestComputePassEncoderDispatchNullCmdBuffer verifies that Dispatch silently
// returns when the underlying command buffer is null (VK-001 defense-in-depth).
func TestComputePassEncoderDispatchNullCmdBuffer(t *testing.T) {
	enc := &CommandEncoder{
		device:      &Device{},
		cmdBuffer:   0,
		isRecording: true,
	}
	cpe := &ComputePassEncoder{encoder: enc}

	// Should not panic.
	cpe.Dispatch(1, 1, 1)
}

// TestRenderPassEncoderSetViewportNullCmdBuffer verifies that SetViewport
// silently returns when the command buffer is null (VK-001 defense-in-depth).
func TestRenderPassEncoderSetViewportNullCmdBuffer(t *testing.T) {
	enc := &CommandEncoder{
		device:      &Device{},
		cmdBuffer:   0,
		isRecording: true,
	}
	rpe := &RenderPassEncoder{encoder: enc}

	// Should not panic.
	rpe.SetViewport(0, 0, 800, 600, 0, 1)
}

// TestRenderPassEncoderSetScissorNullCmdBuffer verifies that SetScissorRect
// silently returns when the command buffer is null (VK-001 defense-in-depth).
func TestRenderPassEncoderSetScissorNullCmdBuffer(t *testing.T) {
	enc := &CommandEncoder{
		device:      &Device{},
		cmdBuffer:   0,
		isRecording: true,
	}
	rpe := &RenderPassEncoder{encoder: enc}

	// Should not panic.
	rpe.SetScissorRect(0, 0, 800, 600)
}

// TestCopyBufferToBufferNullCmdBuffer verifies that CopyBufferToBuffer
// silently returns when cmdBuffer is null (VK-001 defense-in-depth).
func TestCopyBufferToBufferNullCmdBuffer(t *testing.T) {
	enc := &CommandEncoder{
		device:      &Device{},
		cmdBuffer:   0,
		isRecording: true,
	}

	// Should not panic.
	enc.CopyBufferToBuffer(&Buffer{handle: 1}, &Buffer{handle: 2}, []hal.BufferCopy{
		{SrcOffset: 0, DstOffset: 0, Size: 256},
	})
}

// TestBeginRenderPassNullCmdBuffer verifies that BeginRenderPass returns
// an empty encoder when cmdBuffer is null (VK-001 defense-in-depth).
func TestBeginRenderPassNullCmdBuffer(t *testing.T) {
	enc := &CommandEncoder{
		device:      &Device{},
		cmdBuffer:   0,
		isRecording: true,
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
		t.Fatal("BeginRenderPass should return non-nil encoder even with null cmdBuffer")
	}
}

// TestDiscardEncodingNullCmdBuffer verifies that DiscardEncoding does not
// panic when cmdBuffer is null.
func TestDiscardEncodingNullCmdBuffer(t *testing.T) {
	enc := &CommandEncoder{
		device:      &Device{},
		cmdBuffer:   0,
		isRecording: false,
	}

	// Should not panic.
	enc.DiscardEncoding()
}
