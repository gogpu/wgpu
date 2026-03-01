// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package gles

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

func TestTextureFormatToGL(t *testing.T) {
	tests := []struct {
		name           string
		format         gputypes.TextureFormat
		wantInternal   uint32
		wantDataFormat uint32
		wantDataType   uint32
	}{
		{
			name:           "R8Unorm",
			format:         gputypes.TextureFormatR8Unorm,
			wantInternal:   gl.R8,
			wantDataFormat: gl.RED,
			wantDataType:   gl.UNSIGNED_BYTE,
		},
		{
			name:           "RG8Unorm",
			format:         gputypes.TextureFormatRG8Unorm,
			wantInternal:   gl.RG8,
			wantDataFormat: gl.RG,
			wantDataType:   gl.UNSIGNED_BYTE,
		},
		{
			name:           "RGBA8Unorm",
			format:         gputypes.TextureFormatRGBA8Unorm,
			wantInternal:   gl.RGBA8,
			wantDataFormat: gl.RGBA,
			wantDataType:   gl.UNSIGNED_BYTE,
		},
		{
			name:           "RGBA8UnormSrgb",
			format:         gputypes.TextureFormatRGBA8UnormSrgb,
			wantInternal:   gl.SRGB8_ALPHA8,
			wantDataFormat: gl.RGBA,
			wantDataType:   gl.UNSIGNED_BYTE,
		},
		{
			name:           "BGRA8Unorm",
			format:         gputypes.TextureFormatBGRA8Unorm,
			wantInternal:   gl.RGBA8,
			wantDataFormat: gl.BGRA,
			wantDataType:   gl.UNSIGNED_BYTE,
		},
		{
			name:           "R16Float",
			format:         gputypes.TextureFormatR16Float,
			wantInternal:   gl.R16F,
			wantDataFormat: gl.RED,
			wantDataType:   gl.HALF_FLOAT,
		},
		{
			name:           "RGBA16Float",
			format:         gputypes.TextureFormatRGBA16Float,
			wantInternal:   gl.RGBA16F,
			wantDataFormat: gl.RGBA,
			wantDataType:   gl.HALF_FLOAT,
		},
		{
			name:           "R32Float",
			format:         gputypes.TextureFormatR32Float,
			wantInternal:   gl.R32F,
			wantDataFormat: gl.RED,
			wantDataType:   gl.FLOAT,
		},
		{
			name:           "RGBA32Float",
			format:         gputypes.TextureFormatRGBA32Float,
			wantInternal:   gl.RGBA32F,
			wantDataFormat: gl.RGBA,
			wantDataType:   gl.FLOAT,
		},
		{
			name:           "Depth16Unorm",
			format:         gputypes.TextureFormatDepth16Unorm,
			wantInternal:   gl.DEPTH_COMPONENT16,
			wantDataFormat: gl.DEPTH_COMPONENT,
			wantDataType:   gl.UNSIGNED_SHORT,
		},
		{
			name:           "Depth24Plus",
			format:         gputypes.TextureFormatDepth24Plus,
			wantInternal:   gl.DEPTH_COMPONENT24,
			wantDataFormat: gl.DEPTH_COMPONENT,
			wantDataType:   gl.UNSIGNED_INT,
		},
		{
			name:           "Depth24PlusStencil8",
			format:         gputypes.TextureFormatDepth24PlusStencil8,
			wantInternal:   gl.DEPTH24_STENCIL8,
			wantDataFormat: gl.DEPTH_STENCIL,
			wantDataType:   gl.UNSIGNED_INT_24_8,
		},
		{
			name:           "Depth32Float",
			format:         gputypes.TextureFormatDepth32Float,
			wantInternal:   gl.DEPTH_COMPONENT32,
			wantDataFormat: gl.DEPTH_COMPONENT,
			wantDataType:   gl.FLOAT,
		},
		{
			name:           "Unknown defaults to RGBA8",
			format:         gputypes.TextureFormat(9999),
			wantInternal:   gl.RGBA8,
			wantDataFormat: gl.RGBA,
			wantDataType:   gl.UNSIGNED_BYTE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			internal, dataFormat, dataType := textureFormatToGL(tt.format)

			if internal != tt.wantInternal {
				t.Errorf("internalFormat = %#x, want %#x", internal, tt.wantInternal)
			}
			if dataFormat != tt.wantDataFormat {
				t.Errorf("dataFormat = %#x, want %#x", dataFormat, tt.wantDataFormat)
			}
			if dataType != tt.wantDataType {
				t.Errorf("dataType = %#x, want %#x", dataType, tt.wantDataType)
			}
		})
	}
}

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

func TestVertexFormatToGL(t *testing.T) {
	tests := []struct {
		name     string
		format   gputypes.VertexFormat
		wantSize int32
		wantType uint32
	}{
		// Float32 formats
		{"Float32", gputypes.VertexFormatFloat32, 1, gl.FLOAT},
		{"Float32x2", gputypes.VertexFormatFloat32x2, 2, gl.FLOAT},
		{"Float32x3", gputypes.VertexFormatFloat32x3, 3, gl.FLOAT},
		{"Float32x4", gputypes.VertexFormatFloat32x4, 4, gl.FLOAT},

		// 8-bit unsigned
		{"Uint8x2", gputypes.VertexFormatUint8x2, 2, gl.UNSIGNED_BYTE},
		{"Uint8x4", gputypes.VertexFormatUint8x4, 4, gl.UNSIGNED_BYTE},

		// 8-bit signed
		{"Sint8x2", gputypes.VertexFormatSint8x2, 2, gl.BYTE},
		{"Sint8x4", gputypes.VertexFormatSint8x4, 4, gl.BYTE},

		// 16-bit unsigned
		{"Uint16x2", gputypes.VertexFormatUint16x2, 2, gl.UNSIGNED_SHORT},
		{"Uint16x4", gputypes.VertexFormatUint16x4, 4, gl.UNSIGNED_SHORT},

		// 16-bit signed
		{"Sint16x2", gputypes.VertexFormatSint16x2, 2, gl.SHORT},
		{"Sint16x4", gputypes.VertexFormatSint16x4, 4, gl.SHORT},

		// 32-bit unsigned int
		{"Uint32", gputypes.VertexFormatUint32, 1, gl.UNSIGNED_INT},
		{"Uint32x2", gputypes.VertexFormatUint32x2, 2, gl.UNSIGNED_INT},
		{"Uint32x3", gputypes.VertexFormatUint32x3, 3, gl.UNSIGNED_INT},
		{"Uint32x4", gputypes.VertexFormatUint32x4, 4, gl.UNSIGNED_INT},

		// 32-bit signed int
		{"Sint32", gputypes.VertexFormatSint32, 1, gl.INT},
		{"Sint32x2", gputypes.VertexFormatSint32x2, 2, gl.INT},
		{"Sint32x3", gputypes.VertexFormatSint32x3, 3, gl.INT},
		{"Sint32x4", gputypes.VertexFormatSint32x4, 4, gl.INT},

		// Unknown defaults to Float32x4
		{"Unknown", gputypes.VertexFormat(255), 4, gl.FLOAT},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSize, gotType := vertexFormatToGL(tt.format)
			if gotSize != tt.wantSize {
				t.Errorf("size = %d, want %d", gotSize, tt.wantSize)
			}
			if gotType != tt.wantType {
				t.Errorf("type = %#x, want %#x", gotType, tt.wantType)
			}
		})
	}
}

func TestStencilOpToGL(t *testing.T) {
	tests := []struct {
		name string
		op   hal.StencilOperation
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
		{"Unknown defaults to Keep", hal.StencilOperation(99), gl.KEEP},
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
