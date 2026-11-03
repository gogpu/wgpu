// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestVertexAttribDivisor checks the glVertexAttribDivisor value per layout:
// arrayStride 0 (broadcast) must get the largest divisor in both step modes,
// because glVertexAttribPointer reads a stride of 0 as tightly packed.
func TestVertexAttribDivisor(t *testing.T) {
	tests := []struct {
		name   string
		stride uint64
		mode   gputypes.VertexStepMode
		want   uint32
	}{
		{"per vertex", 16, gputypes.VertexStepModeVertex, 0},
		{"per instance", 16, gputypes.VertexStepModeInstance, 1},
		{"stride 0 per vertex", 0, gputypes.VertexStepModeVertex, broadcastDivisor},
		{"stride 0 per instance", 0, gputypes.VertexStepModeInstance, broadcastDivisor},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout := gputypes.VertexBufferLayout{ArrayStride: tt.stride, StepMode: tt.mode}
			if got := vertexAttribDivisor(&layout); got != tt.want {
				t.Errorf("vertexAttribDivisor = %#x, want %#x", got, tt.want)
			}
		})
	}
}
