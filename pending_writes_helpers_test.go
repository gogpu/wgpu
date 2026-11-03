//go:build !rust && !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import (
	"testing"
)

func TestAlignUp(t *testing.T) {
	tests := []struct {
		name      string
		n         uint32
		alignment uint32
		want      uint32
	}{
		{"100 aligned to 256", 100, 256, 256},
		{"256 aligned to 256", 256, 256, 256},
		{"0 aligned to 256", 0, 256, 0},
		{"1 aligned to 1", 1, 1, 1},
		{"257 aligned to 256", 257, 256, 512},
		{"1 aligned to 256", 1, 256, 256},
		{"255 aligned to 256", 255, 256, 256},
		{"512 aligned to 256", 512, 256, 512},
		{"0 aligned to 0 (degenerate)", 0, 0, 0},
		{"100 aligned to 0 (degenerate)", 100, 0, 100},
		{"3 aligned to 4", 3, 4, 4},
		{"4 aligned to 4", 4, 4, 4},
		{"5 aligned to 4", 5, 4, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := alignUp(tt.n, tt.alignment)
			if got != tt.want {
				t.Errorf("alignUp(%d, %d) = %d, want %d", tt.n, tt.alignment, got, tt.want)
			}
		})
	}
}
