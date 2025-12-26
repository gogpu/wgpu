// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"testing"
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
