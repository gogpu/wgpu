//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
)

// TestRootSentinelsAreTheInternalValues pins the ADR-070 §15 contract that a
// root sentinel is the internal value itself, not a copy carrying the same
// message. A signature golden cannot see the difference — a redefinition keeps
// the name and the type — so identity is the evidence, and errors.Is through
// the public API is the callers' contract it protects.
func TestRootSentinelsAreTheInternalValues(t *testing.T) {
	tests := []struct {
		name     string
		public   error
		internal error
	}{
		{"ErrDeviceLost", ErrDeviceLost, hal.ErrDeviceLost},
		{"ErrOutOfMemory", ErrOutOfMemory, hal.ErrDeviceOutOfMemory},
		{"ErrSurfaceLost", ErrSurfaceLost, hal.ErrSurfaceLost},
		{"ErrSurfaceOutdated", ErrSurfaceOutdated, hal.ErrSurfaceOutdated},
		{"ErrTimeout", ErrTimeout, hal.ErrTimeout},
		{"ErrContextReleased", ErrContextReleased, hal.ErrContextReleased},
		{"ErrWorkgroupBarrierUnsupported", ErrWorkgroupBarrierUnsupported, hal.ErrWorkgroupBarrierUnsupported},
		{"ErrNoBackendRegistered", ErrNoBackendRegistered, core.ErrNoBackendRegistered},
		{"ErrNoHardwareAdapter", ErrNoHardwareAdapter, core.ErrNoHardwareAdapter},
		{"ErrNoFallbackAdapter", ErrNoFallbackAdapter, core.ErrNoFallbackAdapter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//nolint:errorlint // the assertion IS the identity check; errors.Is is the tested equivalence, lines below
			if tt.public != tt.internal {
				t.Fatalf("%s is not the internal value: %v != %v", tt.name, tt.public, tt.internal)
			}
			wrapped := fmt.Errorf("wrapped: %w", tt.internal)
			if !errors.Is(wrapped, tt.public) {
				t.Errorf("errors.Is(wrapped %v, %s) = false", wrapped, tt.name)
			}
		})
	}
}
