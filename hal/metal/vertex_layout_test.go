// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestVertexBufferLayoutToMTL checks the MTLVertexBufferLayoutDescriptor
// setup, in particular arrayStride 0 (broadcast): Metal rejects a zero stride,
// so it must become the constant step function with step rate 0 and a stride
// that covers the attributes, rounded up to 4 bytes (Rust wgpu-hal
// src/metal/device.rs).
func TestVertexBufferLayoutToMTL(t *testing.T) {
	attr := func(offset uint64, format gputypes.VertexFormat) gputypes.VertexAttribute {
		return gputypes.VertexAttribute{Format: format, Offset: offset}
	}
	tests := []struct {
		name string
		buf  gputypes.VertexBufferLayout
		want mtlVertexBufferLayout
	}{
		{
			name: "per vertex",
			buf:  gputypes.VertexBufferLayout{ArrayStride: 32, StepMode: gputypes.VertexStepModeVertex, Attributes: []gputypes.VertexAttribute{attr(0, gputypes.VertexFormatFloat32x4)}},
			want: mtlVertexBufferLayout{stride: 32, stepFunc: MTLVertexStepFunctionPerVertex, stepRate: 1},
		},
		{
			name: "per instance",
			buf:  gputypes.VertexBufferLayout{ArrayStride: 8, StepMode: gputypes.VertexStepModeInstance, Attributes: []gputypes.VertexAttribute{attr(0, gputypes.VertexFormatFloat32x2)}},
			want: mtlVertexBufferLayout{stride: 8, stepFunc: MTLVertexStepFunctionPerInstance, stepRate: 1},
		},
		{
			name: "stride 0 per vertex is constant",
			buf:  gputypes.VertexBufferLayout{ArrayStride: 0, StepMode: gputypes.VertexStepModeVertex, Attributes: []gputypes.VertexAttribute{attr(0, gputypes.VertexFormatFloat32x4)}},
			want: mtlVertexBufferLayout{stride: 16, stepFunc: MTLVertexStepFunctionConstant, stepRate: 0},
		},
		{
			name: "stride 0 per instance is constant",
			buf:  gputypes.VertexBufferLayout{ArrayStride: 0, StepMode: gputypes.VertexStepModeInstance, Attributes: []gputypes.VertexAttribute{attr(0, gputypes.VertexFormatFloat32x4)}},
			want: mtlVertexBufferLayout{stride: 16, stepFunc: MTLVertexStepFunctionConstant, stepRate: 0},
		},
		{
			name: "stride 0 covers the furthest attribute",
			buf: gputypes.VertexBufferLayout{Attributes: []gputypes.VertexAttribute{
				attr(12, gputypes.VertexFormatFloat32x2), attr(0, gputypes.VertexFormatFloat32x3),
			}},
			want: mtlVertexBufferLayout{stride: 20, stepFunc: MTLVertexStepFunctionConstant, stepRate: 0},
		},
		{
			name: "stride 0 rounds up to 4 bytes",
			buf:  gputypes.VertexBufferLayout{Attributes: []gputypes.VertexAttribute{attr(4, gputypes.VertexFormatUnorm8x2)}},
			want: mtlVertexBufferLayout{stride: 8, stepFunc: MTLVertexStepFunctionConstant, stepRate: 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := vertexBufferLayoutToMTL(&tt.buf); got != tt.want {
				t.Errorf("vertexBufferLayoutToMTL = %+v, want %+v", got, tt.want)
			}
		})
	}
}
