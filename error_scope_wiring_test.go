//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

// Stage A2 (issue #421): the deferred validation errors that an encoder raises
// while recording must reach the device's error scopes — today they surface
// only from Finish. This test FAILS on the pre-wiring tree (the scope captures
// nothing) and is the fail-first evidence for the wiring.

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu"
)

func TestErrorScopeCapturesDeferredValidationError(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	dstBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "scope-capture-dst",
		Size:  64,
		Usage: wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer dstBuf.Release()

	if err := device.PushErrorScope(wgpu.ErrorFilterValidation); err != nil {
		t.Fatalf("PushErrorScope: %v", err)
	}

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	// A nil source is a deferred validation error: recorded now, surfaced by
	// Finish. It must also land in the open validation scope.
	enc.CopyBufferToBuffer(nil, 0, dstBuf, 0, 64)
	if _, err := enc.Finish(); err == nil {
		t.Fatal("Finish should fail: CopyBufferToBuffer with nil src")
	}

	gpuErr, popErr := device.PopErrorScope()
	if popErr != nil {
		t.Fatalf("PopErrorScope: unexpected error: %v", popErr)
	}
	if gpuErr == nil {
		t.Fatal("the validation scope captured nothing: the deferred encoder error never reached the device's error scope (stage A2 wiring missing)")
	}
	if gpuErr.Type != wgpu.ErrorFilterValidation {
		t.Fatalf("captured GPUError.Type = %v, want %v", gpuErr.Type, wgpu.ErrorFilterValidation)
	}
	if !errors.Is(gpuErr, wgpu.ErrValidation) {
		t.Fatalf("errors.Is(%v, wgpu.ErrValidation) = false, want true", gpuErr)
	}
}
