//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

// The error-scope reshape contract (issue #421, stage A1) on the native build:
// the three-case return, the empty-stack sentinel instead of a panic, and the
// category identity through GPUError.Unwrap with the per-build sentinels.

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu"
)

// TestErrorScopeContractThreeCases pins the return contract through a real
// public device: (nil, nil) is the common captured-nothing path and
// (nil, ErrErrorScopeEmpty) is pop with no matching push — never a panic.
func TestErrorScopeContractThreeCases(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	if err := device.PushErrorScope(wgpu.ErrorFilterValidation); err != nil {
		t.Fatalf("PushErrorScope: %v", err)
	}
	gpuErr, err := device.PopErrorScope()
	if gpuErr != nil || err != nil {
		t.Fatalf("pop of a capture-nothing scope = (%v, %v), want (nil, nil) — the common path", gpuErr, err)
	}

	gpuErr, err = device.PopErrorScope()
	if gpuErr != nil {
		t.Fatalf("empty-stack pop returned a GPUError: %v", gpuErr)
	}
	if !errors.Is(err, wgpu.ErrErrorScopeEmpty) {
		t.Fatalf("empty-stack pop = %v, want wgpu.ErrErrorScopeEmpty", err)
	}
}

// TestErrorScopeContractZeroValueDevice pins the native nil-impl parity with
// the rust build (expert P3-1): a zero-value device answers (nil, nil) and
// never panics.
func TestErrorScopeContractZeroValueDevice(t *testing.T) {
	var d wgpu.Device
	gpuErr, err := d.PopErrorScope()
	if gpuErr != nil || err != nil {
		t.Fatalf("zero-value-device PopErrorScope = (%v, %v), want (nil, nil)", gpuErr, err)
	}
}

// TestGPUErrorUnwrapIdentityPerCategory pins errors.Is THROUGH
// GPUError.Unwrap for the three categories and the negative direction.
func TestGPUErrorUnwrapIdentityPerCategory(t *testing.T) {
	cases := []struct {
		name   string
		filter wgpu.ErrorFilter
		want   error
	}{
		{"validation", wgpu.ErrorFilterValidation, wgpu.ErrValidation},
		{"out-of-memory", wgpu.ErrorFilterOutOfMemory, wgpu.ErrOutOfMemory},
		{"internal", wgpu.ErrorFilterInternal, wgpu.ErrInternal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var err error = &wgpu.GPUError{Type: c.filter, Message: "x"}
			if !errors.Is(err, c.want) {
				t.Fatalf("errors.Is(%v, %v) = false, want true", err, c.want)
			}
			for _, other := range []error{wgpu.ErrValidation, wgpu.ErrOutOfMemory, wgpu.ErrInternal} {
				if other == c.want {
					continue
				}
				if errors.Is(err, other) {
					t.Fatalf("errors.Is(%v, %v) = true, want false", err, other)
				}
			}
		})
	}
}
