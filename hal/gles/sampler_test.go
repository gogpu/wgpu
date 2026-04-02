// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows || linux

package gles

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

func TestMapFilterMode(t *testing.T) {
	tests := []struct {
		name string
		mode gputypes.FilterMode
		want int32
	}{
		{"Nearest", gputypes.FilterModeNearest, gl.NEAREST},
		{"Linear", gputypes.FilterModeLinear, gl.LINEAR},
		{"Undefined defaults to Nearest", gputypes.FilterModeUndefined, gl.NEAREST},
		{"Unknown defaults to Nearest", gputypes.FilterMode(99), gl.NEAREST},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapFilterMode(tt.mode)
			if got != tt.want {
				t.Errorf("mapFilterMode(%v) = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestMapMinFilter(t *testing.T) {
	tests := []struct {
		name         string
		minFilter    gputypes.FilterMode
		mipmapFilter gputypes.FilterMode
		want         int32
	}{
		{"Nearest+Nearest", gputypes.FilterModeNearest, gputypes.FilterModeNearest, gl.NEAREST_MIPMAP_NEAREST},
		{"Nearest+Linear", gputypes.FilterModeNearest, gputypes.FilterModeLinear, gl.NEAREST_MIPMAP_LINEAR},
		{"Linear+Nearest", gputypes.FilterModeLinear, gputypes.FilterModeNearest, gl.LINEAR_MIPMAP_NEAREST},
		{"Linear+Linear", gputypes.FilterModeLinear, gputypes.FilterModeLinear, gl.LINEAR_MIPMAP_LINEAR},
		{"Nearest+Undefined", gputypes.FilterModeNearest, gputypes.FilterModeUndefined, gl.NEAREST_MIPMAP_NEAREST},
		{"Linear+Undefined", gputypes.FilterModeLinear, gputypes.FilterModeUndefined, gl.LINEAR_MIPMAP_NEAREST},
		{"Default (both zero)", gputypes.FilterMode(0), gputypes.FilterMode(0), gl.NEAREST_MIPMAP_NEAREST},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapMinFilter(tt.minFilter, tt.mipmapFilter)
			if got != tt.want {
				t.Errorf("mapMinFilter(%v, %v) = %v, want %v", tt.minFilter, tt.mipmapFilter, got, tt.want)
			}
		})
	}
}

func TestMapAddressMode(t *testing.T) {
	tests := []struct {
		name string
		mode gputypes.AddressMode
		want int32
	}{
		{"Repeat", gputypes.AddressModeRepeat, gl.REPEAT},
		{"MirrorRepeat", gputypes.AddressModeMirrorRepeat, gl.MIRRORED_REPEAT},
		{"ClampToEdge", gputypes.AddressModeClampToEdge, gl.CLAMP_TO_EDGE},
		{"Undefined defaults to ClampToEdge", gputypes.AddressModeUndefined, gl.CLAMP_TO_EDGE},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapAddressMode(tt.mode)
			if got != tt.want {
				t.Errorf("mapAddressMode(%v) = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestMapCompareFunction(t *testing.T) {
	tests := []struct {
		name string
		fn   gputypes.CompareFunction
		want int32
	}{
		{"Never", gputypes.CompareFunctionNever, gl.NEVER},
		{"Less", gputypes.CompareFunctionLess, gl.LESS},
		{"Equal", gputypes.CompareFunctionEqual, gl.EQUAL},
		{"LessEqual", gputypes.CompareFunctionLessEqual, gl.LEQUAL},
		{"Greater", gputypes.CompareFunctionGreater, gl.GREATER},
		{"NotEqual", gputypes.CompareFunctionNotEqual, gl.NOTEQUAL},
		{"GreaterEqual", gputypes.CompareFunctionGreaterEqual, gl.GEQUAL},
		{"Always", gputypes.CompareFunctionAlways, gl.ALWAYS},
		{"Undefined defaults to Always", gputypes.CompareFunctionUndefined, gl.ALWAYS},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapCompareFunction(tt.fn)
			if got != tt.want {
				t.Errorf("mapCompareFunction(%v) = %v, want %v", tt.fn, got, tt.want)
			}
		})
	}
}

// TestSamplerBindMap verifies the SamplerBindMap lookup logic used in SetBindGroupCommand.
// Given samplerBindMap[texUnit] = samplerGLBinding, searching for glBinding should return texUnit.
func TestSamplerBindMap(t *testing.T) {
	var bindMap [maxTextureSlots]int8
	for i := range bindMap {
		bindMap[i] = -1 // no sampler (default)
	}
	// Texture unit 1 paired with sampler glBinding 2.
	bindMap[1] = 2
	// Texture unit 5 paired with sampler glBinding 3.
	bindMap[5] = 3

	tests := []struct {
		name        string
		glBinding   int8
		wantTexUnit int // -1 if not found
	}{
		{"sampler 2 maps to texUnit 1", 2, 1},
		{"sampler 3 maps to texUnit 5", 3, 5},
		{"sampler 0 not mapped", 0, -1},
		{"sampler 7 not mapped", 7, -1},
		{"sampler 1 not mapped (only 2 and 3 are)", 1, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the lookup from SetBindGroupCommand.Execute (command.go:952).
			found := -1
			for texUnit := range bindMap {
				if bindMap[texUnit] == tt.glBinding {
					found = texUnit
					break
				}
			}
			if found != tt.wantTexUnit {
				t.Errorf("lookup glBinding=%d: got texUnit=%d, want %d", tt.glBinding, found, tt.wantTexUnit)
			}
		})
	}
}

func TestIsNonFilterableFormat(t *testing.T) {
	tests := []struct {
		name string
		fmt  gputypes.TextureFormat
		want bool
	}{
		// Integer formats are non-filterable.
		{"R8Uint", gputypes.TextureFormatR8Uint, true},
		{"RGBA8Sint", gputypes.TextureFormatRGBA8Sint, true},
		{"R32Uint", gputypes.TextureFormatR32Uint, true},
		// 32-bit float formats are non-filterable.
		{"R32Float", gputypes.TextureFormatR32Float, true},
		{"RGBA32Float", gputypes.TextureFormatRGBA32Float, true},
		// Depth32Float and Stencil8 are non-filterable.
		{"Depth32Float", gputypes.TextureFormatDepth32Float, true},
		{"Stencil8", gputypes.TextureFormatStencil8, true},
		// Standard filterable formats.
		{"RGBA8Unorm", gputypes.TextureFormatRGBA8Unorm, false},
		{"R16Float", gputypes.TextureFormatR16Float, false},
		{"RGBA16Float", gputypes.TextureFormatRGBA16Float, false},
		{"Depth24Plus", gputypes.TextureFormatDepth24Plus, false},
		{"BGRA8Unorm", gputypes.TextureFormatBGRA8Unorm, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNonFilterableFormat(tt.fmt)
			if got != tt.want {
				t.Errorf("isNonFilterableFormat(%v) = %v, want %v", tt.fmt, got, tt.want)
			}
		})
	}
}
