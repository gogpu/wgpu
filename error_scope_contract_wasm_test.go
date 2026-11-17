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
	"strings"
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

// TestErrorScopeBrowserPhase2PanicPinned records the staged state: until stage
// A4 implements the JS scopes, Push/Pop panic. Stage A4 removes the panics and
// replaces this test with the working contract — the test exists so the panic
// cannot vanish silently or come back.
func TestErrorScopeBrowserPhase2PanicPinned(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("browser PopErrorScope must still panic until stage A4")
		}
		if s, ok := r.(string); !ok || !strings.Contains(s, "not yet implemented (Phase 2)") {
			t.Fatalf("unexpected panic value: %v", r)
		}
	}()
	var d wgpu.Device
	_, _ = d.PopErrorScope()
}
