//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package surface

import (
	"testing"

	"github.com/gogpu/wgpu/internal/hal"
)

func TestDisplayHandleHAL(t *testing.T) {
	tests := []struct {
		in   DisplayHandle
		want hal.DisplayHandle
	}{
		{DisplayHandle{}, hal.DisplayHandle{}},
		{DisplayHandle{Kind: DisplayKindXlib, Ptr: 0x10}, hal.DisplayHandle{Kind: hal.DisplayKindXlib, Display: 0x10}},
		{DisplayHandle{Kind: DisplayKindWayland, Ptr: 0x20}, hal.DisplayHandle{Kind: hal.DisplayKindWayland, Display: 0x20}},
	}
	for _, test := range tests {
		if got := test.in.HAL(); got != test.want {
			t.Fatalf("HAL(%+v) = %+v, want %+v", test.in, got, test.want)
		}
	}
}
