// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package gles

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

func TestCompareFunctionToGL(t *testing.T) {
	tests := []struct {
		name string
		fn   gputypes.CompareFunction
		want uint32
	}{
		{"Never", gputypes.CompareFunctionNever, gl.NEVER},
		{"Less", gputypes.CompareFunctionLess, gl.LESS},
		{"Equal", gputypes.CompareFunctionEqual, gl.EQUAL},
		{"LessEqual", gputypes.CompareFunctionLessEqual, gl.LEQUAL},
		{"Greater", gputypes.CompareFunctionGreater, gl.GREATER},
		{"NotEqual", gputypes.CompareFunctionNotEqual, gl.NOTEQUAL},
		{"GreaterEqual", gputypes.CompareFunctionGreaterEqual, gl.GEQUAL},
		{"Always", gputypes.CompareFunctionAlways, gl.ALWAYS},
		{"Unknown", gputypes.CompareFunction(99), gl.ALWAYS}, // Unknown defaults to ALWAYS
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareFunctionToGL(tt.fn)
			if got != tt.want {
				t.Errorf("compareFunctionToGL(%v) = %#x, want %#x", tt.fn, got, tt.want)
			}
		})
	}
}

func TestMaxInt32(t *testing.T) {
	tests := []struct {
		a, b, want int32
	}{
		{1, 2, 2},
		{5, 3, 5},
		{0, 0, 0},
		{-1, -2, -1},
		{-5, 10, 10},
	}

	for _, tt := range tests {
		got := maxInt32(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("maxInt32(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

// TestVertexFormatToGL pins each format's GL description against Rust
// wgpu-hal src/gles/conv.rs describe_vertex_format: Uint and Sint formats are
// integer attributes (glVertexAttribIPointer), all others float.
func TestVertexFormatToGL(t *testing.T) {
	const (
		float   = false
		integer = true
	)
	tests := []struct {
		name   string
		format gputypes.VertexFormat
		want   vertexFormatGL
	}{
		{"Float32", gputypes.VertexFormatFloat32, vertexFormatGL{1, gl.FLOAT, false, float}},
		{"Float32x2", gputypes.VertexFormatFloat32x2, vertexFormatGL{2, gl.FLOAT, false, float}},
		{"Float32x3", gputypes.VertexFormatFloat32x3, vertexFormatGL{3, gl.FLOAT, false, float}},
		{"Float32x4", gputypes.VertexFormatFloat32x4, vertexFormatGL{4, gl.FLOAT, false, float}},
		{"Float16x2", gputypes.VertexFormatFloat16x2, vertexFormatGL{2, gl.HALF_FLOAT, false, float}},
		{"Float16x4", gputypes.VertexFormatFloat16x4, vertexFormatGL{4, gl.HALF_FLOAT, false, float}},

		{"Uint8x2", gputypes.VertexFormatUint8x2, vertexFormatGL{2, gl.UNSIGNED_BYTE, false, integer}},
		{"Uint8x4", gputypes.VertexFormatUint8x4, vertexFormatGL{4, gl.UNSIGNED_BYTE, false, integer}},
		{"Unorm8x2", gputypes.VertexFormatUnorm8x2, vertexFormatGL{2, gl.UNSIGNED_BYTE, true, float}},
		{"Unorm8x4", gputypes.VertexFormatUnorm8x4, vertexFormatGL{4, gl.UNSIGNED_BYTE, true, float}},
		{"Sint8x2", gputypes.VertexFormatSint8x2, vertexFormatGL{2, gl.BYTE, false, integer}},
		{"Sint8x4", gputypes.VertexFormatSint8x4, vertexFormatGL{4, gl.BYTE, false, integer}},
		{"Snorm8x2", gputypes.VertexFormatSnorm8x2, vertexFormatGL{2, gl.BYTE, true, float}},
		{"Snorm8x4", gputypes.VertexFormatSnorm8x4, vertexFormatGL{4, gl.BYTE, true, float}},

		{"Uint16x2", gputypes.VertexFormatUint16x2, vertexFormatGL{2, gl.UNSIGNED_SHORT, false, integer}},
		{"Uint16x4", gputypes.VertexFormatUint16x4, vertexFormatGL{4, gl.UNSIGNED_SHORT, false, integer}},
		{"Unorm16x2", gputypes.VertexFormatUnorm16x2, vertexFormatGL{2, gl.UNSIGNED_SHORT, true, float}},
		{"Unorm16x4", gputypes.VertexFormatUnorm16x4, vertexFormatGL{4, gl.UNSIGNED_SHORT, true, float}},
		{"Sint16x2", gputypes.VertexFormatSint16x2, vertexFormatGL{2, gl.SHORT, false, integer}},
		{"Sint16x4", gputypes.VertexFormatSint16x4, vertexFormatGL{4, gl.SHORT, false, integer}},
		{"Snorm16x2", gputypes.VertexFormatSnorm16x2, vertexFormatGL{2, gl.SHORT, true, float}},
		{"Snorm16x4", gputypes.VertexFormatSnorm16x4, vertexFormatGL{4, gl.SHORT, true, float}},

		{"Uint32", gputypes.VertexFormatUint32, vertexFormatGL{1, gl.UNSIGNED_INT, false, integer}},
		{"Uint32x2", gputypes.VertexFormatUint32x2, vertexFormatGL{2, gl.UNSIGNED_INT, false, integer}},
		{"Uint32x3", gputypes.VertexFormatUint32x3, vertexFormatGL{3, gl.UNSIGNED_INT, false, integer}},
		{"Uint32x4", gputypes.VertexFormatUint32x4, vertexFormatGL{4, gl.UNSIGNED_INT, false, integer}},
		{"Sint32", gputypes.VertexFormatSint32, vertexFormatGL{1, gl.INT, false, integer}},
		{"Sint32x2", gputypes.VertexFormatSint32x2, vertexFormatGL{2, gl.INT, false, integer}},
		{"Sint32x3", gputypes.VertexFormatSint32x3, vertexFormatGL{3, gl.INT, false, integer}},
		{"Sint32x4", gputypes.VertexFormatSint32x4, vertexFormatGL{4, gl.INT, false, integer}},

		{"Unorm1010102", gputypes.VertexFormatUnorm1010102, vertexFormatGL{4, gl.UNSIGNED_INT_2_10_10_10_REV, true, float}},

		// Unknown defaults to Float32x4
		{"Unknown", gputypes.VertexFormat(255), vertexFormatGL{4, gl.FLOAT, false, float}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := vertexFormatToGL(tt.format); got != tt.want {
				t.Errorf("vertexFormatToGL = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestStencilOpToGL(t *testing.T) {
	tests := []struct {
		name string
		op   gputypes.StencilOperation
		want uint32
	}{
		{"Keep", hal.StencilOperationKeep, gl.KEEP},
		{"Zero", hal.StencilOperationZero, gl.ZERO},
		{"Replace", hal.StencilOperationReplace, gl.REPLACE},
		{"Invert", hal.StencilOperationInvert, gl.INVERT},
		{"IncrementClamp", hal.StencilOperationIncrementClamp, gl.INCR},
		{"DecrementClamp", hal.StencilOperationDecrementClamp, gl.DECR},
		{"IncrementWrap", hal.StencilOperationIncrementWrap, gl.INCR_WRAP},
		{"DecrementWrap", hal.StencilOperationDecrementWrap, gl.DECR_WRAP},
		{"Unknown defaults to Keep", gputypes.StencilOperation(99), gl.KEEP},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stencilOpToGL(tt.op)
			if got != tt.want {
				t.Errorf("stencilOpToGL(%v) = %#x, want %#x", tt.op, got, tt.want)
			}
		})
	}
}

func TestBlendFactorToGL(t *testing.T) {
	tests := []struct {
		name   string
		factor gputypes.BlendFactor
		want   uint32
	}{
		{"Zero", gputypes.BlendFactorZero, gl.ZERO},
		{"One", gputypes.BlendFactorOne, gl.ONE},
		{"Src", gputypes.BlendFactorSrc, gl.SRC_COLOR},
		{"OneMinusSrc", gputypes.BlendFactorOneMinusSrc, gl.ONE_MINUS_SRC_COLOR},
		{"SrcAlpha", gputypes.BlendFactorSrcAlpha, gl.SRC_ALPHA},
		{"OneMinusSrcAlpha", gputypes.BlendFactorOneMinusSrcAlpha, gl.ONE_MINUS_SRC_ALPHA},
		{"Dst", gputypes.BlendFactorDst, gl.DST_COLOR},
		{"OneMinusDst", gputypes.BlendFactorOneMinusDst, gl.ONE_MINUS_DST_COLOR},
		{"DstAlpha", gputypes.BlendFactorDstAlpha, gl.DST_ALPHA},
		{"OneMinusDstAlpha", gputypes.BlendFactorOneMinusDstAlpha, gl.ONE_MINUS_DST_ALPHA},
		{"SrcAlphaSaturated", gputypes.BlendFactorSrcAlphaSaturated, gl.SRC_ALPHA_SATURATE},
		{"Constant", gputypes.BlendFactorConstant, gl.CONSTANT_COLOR},
		{"OneMinusConstant", gputypes.BlendFactorOneMinusConstant, gl.ONE_MINUS_CONSTANT_COLOR},
		{"Unknown defaults to One", gputypes.BlendFactor(99), gl.ONE},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blendFactorToGL(tt.factor)
			if got != tt.want {
				t.Errorf("blendFactorToGL(%v) = %#x, want %#x", tt.factor, got, tt.want)
			}
		})
	}
}

func TestBlendOperationToGL(t *testing.T) {
	tests := []struct {
		name string
		op   gputypes.BlendOperation
		want uint32
	}{
		{"Add", gputypes.BlendOperationAdd, gl.FUNC_ADD},
		{"Subtract", gputypes.BlendOperationSubtract, gl.FUNC_SUBTRACT},
		{"ReverseSubtract", gputypes.BlendOperationReverseSubtract, gl.FUNC_REVERSE_SUBTRACT},
		{"Min", gputypes.BlendOperationMin, gl.MIN},
		{"Max", gputypes.BlendOperationMax, gl.MAX},
		{"Unknown defaults to Add", gputypes.BlendOperation(99), gl.FUNC_ADD},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blendOperationToGL(tt.op)
			if got != tt.want {
				t.Errorf("blendOperationToGL(%v) = %#x, want %#x", tt.op, got, tt.want)
			}
		})
	}
}
