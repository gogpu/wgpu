// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package gles

import (
	"testing"

	"github.com/gogpu/gputypes"
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
			wantDataType:   gl.UNSIGNED_INT,
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
