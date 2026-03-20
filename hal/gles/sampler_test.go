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
