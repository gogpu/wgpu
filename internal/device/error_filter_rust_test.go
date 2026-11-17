//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

import (
	"testing"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
	"github.com/gogpu/wgpu/internal/core"
)

// TestRustErrorFilterToNative checks that each error scope filter reaches
// wgpu-native as the webgpu.h member of the same name. ErrorFilter has no
// gputypes counterpart: its values are core's (Validation = 0), as on the
// native build, while webgpu.h numbers them from 1, so a plain cast pushed a
// scope for the wrong error type (OutOfMemory became Validation). It moved with
// the mapping into internal/device at S11.
func TestRustErrorFilterToNative(t *testing.T) {
	for _, c := range []struct {
		filter core.ErrorFilter
		want   rwgpu.ErrorFilter
	}{
		{core.ErrorFilterValidation, rwgpu.ErrorFilterValidation},
		{core.ErrorFilterOutOfMemory, rwgpu.ErrorFilterOutOfMemory},
		{core.ErrorFilterInternal, rwgpu.ErrorFilterInternal},
	} {
		got, err := errorFilterToRust(c.filter)
		if err != nil {
			t.Errorf("%v: errorFilterToRust = %v, want wgpu-native filter %d", c.filter, err, c.want)
			continue
		}
		if got != c.want {
			t.Errorf("%v: wgpu-native filter %d, want %d", c.filter, got, c.want)
		}
	}
	// An unmapped filter is an error, never a silent Validation fallback:
	// wgpu-native panics on an unknown filter and the panic aborts the process
	// across the FFI boundary (P3-API-025).
	for _, f := range []core.ErrorFilter{-1, 3, 42} {
		if _, err := errorFilterToRust(f); err == nil {
			t.Errorf("errorFilterToRust(%d) = nil error, want an error", int(f))
		}
	}
}
