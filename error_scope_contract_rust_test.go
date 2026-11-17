//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

// The error-scope reshape contract (issue #421, stage A1) on the rust build.
// This file exists and compiles on every build that carries the rust tag; it
// runs wherever wgpu-native loads (this machine). The empty-stack case on rust
// is reported by wgpu-native itself and comes back wrapped — the per-build
// limit recorded in error_rust.go and in the API-CHANGE.

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu"
)

// TestErrorScopeContractRustGuard pins the nil-receiver guard: a nil device
// answers (nil, nil), never a panic.
func TestErrorScopeContractRustGuard(t *testing.T) {
	var d *wgpu.Device
	gpuErr, err := d.PopErrorScope()
	if gpuErr != nil || err != nil {
		t.Fatalf("nil-device PopErrorScope = (%v, %v), want (nil, nil)", gpuErr, err)
	}
}

// TestGPUErrorUnwrapIdentityRust pins the category identity through
// GPUError.Unwrap with the rust build's own sentinels.
func TestGPUErrorUnwrapIdentityRust(t *testing.T) {
	var err error = &wgpu.GPUError{Type: wgpu.ErrorFilterValidation, Message: "x"}
	if !errors.Is(err, wgpu.ErrValidation) {
		t.Fatalf("errors.Is(%v, wgpu.ErrValidation) = false, want true", err)
	}
	var oom error = &wgpu.GPUError{Type: wgpu.ErrorFilterOutOfMemory, Message: "y"}
	if !errors.Is(oom, wgpu.ErrOutOfMemory) {
		t.Fatalf("errors.Is(%v, wgpu.ErrOutOfMemory) = false, want true", oom)
	}
}
