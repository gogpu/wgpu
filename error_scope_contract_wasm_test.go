//go:build js && wasm

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

// The error-scope reshape contract (issue #421) on the browser build, stage
// A1: the unified GPUError.Type field exists on js/wasm (this file does not
// compile without it — the named golden hunk's regression guard), Unwrap works
// with the browser's own sentinels, and the Phase-2 panic is PINNED here until
// stage A4 removes it with its own catch-test.

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu"
)

// TestGPUErrorUnwrapIdentityBrowser pins the category identity through
// GPUError.Unwrap with the js/wasm build's own sentinels.
func TestGPUErrorUnwrapIdentityBrowser(t *testing.T) {
	var err error = &wgpu.GPUError{Type: wgpu.ErrorFilterValidation, Message: "x"}
	if !errors.Is(err, wgpu.ErrValidation) {
		t.Fatalf("errors.Is(%v, wgpu.ErrValidation) = false, want true", err)
	}
	var internal error = &wgpu.GPUError{Type: wgpu.ErrorFilterInternal, Message: "z"}
	if !errors.Is(internal, wgpu.ErrInternal) {
		t.Fatalf("errors.Is(%v, wgpu.ErrInternal) = false, want true", internal)
	}
}

// TestErrorScopeContractZeroValueBrowser pins the browser contract after stage
// A4: a zero-value device answers (nil, nil) and never panics — the Phase-2
// panics are gone, and this test would catch their return.
func TestErrorScopeContractZeroValueBrowser(t *testing.T) {
	var d wgpu.Device
	if err := d.PushErrorScope(wgpu.ErrorFilterValidation); err != nil {
		t.Fatalf("zero-value PushErrorScope = %v, want nil", err)
	}
	gpuErr, err := d.PopErrorScope()
	if gpuErr != nil || err != nil {
		t.Fatalf("zero-value PopErrorScope = (%v, %v), want (nil, nil)", gpuErr, err)
	}
}
