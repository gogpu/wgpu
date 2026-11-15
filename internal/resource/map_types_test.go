//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package resource

import (
	"testing"

	"github.com/gogpu/wgpu/internal/core"
)

// TestMapModeToInternal pins the public-to-core value mapping. It is
// unexported implementation detail, reachable only from inside the package,
// which is why it moves here with map_types.go (the root's public tests cover
// the same values through Buffer.Map).
func TestMapModeToInternal(t *testing.T) {
	tests := []struct {
		mode MapMode
		want core.MapModeInternal
	}{
		{MapModeRead, core.MapModeInternalRead},
		{MapModeWrite, core.MapModeInternalWrite},
		{MapMode(0), 0},
		{MapMode(3), 0},
	}
	for _, tt := range tests {
		if got := tt.mode.toInternal(); got != tt.want {
			t.Errorf("MapMode(%d).toInternal() = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

// TestMapStateFromCore pins the core-to-public state mapping, including the
// Destroyed terminal state the public enum carries.
func TestMapStateFromCore(t *testing.T) {
	tests := []struct {
		state core.BufferMapState
		want  MapState
	}{
		{core.BufferMapStateIdle, MapStateUnmapped},
		{core.BufferMapStatePending, MapStatePending},
		{core.BufferMapStateMapped, MapStateMapped},
		{core.BufferMapStateDestroyed, MapStateDestroyed},
	}
	for _, tt := range tests {
		if got := mapStateFromCore(tt.state); got != tt.want {
			t.Errorf("mapStateFromCore(%v) = %v, want %v", tt.state, got, tt.want)
		}
	}
}
