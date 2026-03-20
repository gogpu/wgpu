// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows || linux

package gles

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// configureSampler allocates a GL sampler object and sets its parameters from the descriptor.
// Returns the GL sampler object ID, or 0 if sampler objects are not supported.
func configureSampler(glCtx *gl.Context, desc *hal.SamplerDescriptor) uint32 {
	if !glCtx.SupportsSamplerObjects() {
		return 0
	}

	id := glCtx.GenSamplers(1)
	if id == 0 {
		return 0
	}

	// Magnification filter (no mipmap involved).
	glCtx.SamplerParameteri(id, gl.TEXTURE_MAG_FILTER, mapFilterMode(desc.MagFilter))

	// Minification filter (combined with mipmap filter).
	glCtx.SamplerParameteri(id, gl.TEXTURE_MIN_FILTER, mapMinFilter(desc.MinFilter, desc.MipmapFilter))

	// Address modes.
	glCtx.SamplerParameteri(id, gl.TEXTURE_WRAP_S, mapAddressMode(desc.AddressModeU))
	glCtx.SamplerParameteri(id, gl.TEXTURE_WRAP_T, mapAddressMode(desc.AddressModeV))
	glCtx.SamplerParameteri(id, gl.TEXTURE_WRAP_R, mapAddressMode(desc.AddressModeW))

	// LOD clamps.
	glCtx.SamplerParameterf(id, gl.TEXTURE_MIN_LOD, desc.LodMinClamp)
	lodMax := desc.LodMaxClamp
	if lodMax == 0 {
		// WebGPU default: 32 (effectively no upper clamp).
		lodMax = 32.0
	}
	glCtx.SamplerParameterf(id, gl.TEXTURE_MAX_LOD, lodMax)

	// Anisotropic filtering (if requested and > 1).
	if desc.Anisotropy > 1 {
		aniso := float32(desc.Anisotropy)
		// Clamp to reasonable max (16 is typical hardware limit).
		if aniso > 16 {
			aniso = 16
		}
		glCtx.SamplerParameterf(id, gl.TEXTURE_MAX_ANISOTROPY, aniso)
	}

	// Comparison function (for depth/shadow samplers).
	if desc.Compare != gputypes.CompareFunctionUndefined {
		glCtx.SamplerParameteri(id, gl.TEXTURE_COMPARE_MODE, gl.COMPARE_REF_TO_TEXTURE)
		glCtx.SamplerParameteri(id, gl.TEXTURE_COMPARE_FUNC, mapCompareFunction(desc.Compare))
	}

	return id
}

// mapFilterMode maps a WebGPU FilterMode to a GL filter constant.
// Used for GL_TEXTURE_MAG_FILTER (no mipmap).
func mapFilterMode(mode gputypes.FilterMode) int32 {
	switch mode {
	case gputypes.FilterModeNearest:
		return gl.NEAREST
	case gputypes.FilterModeLinear:
		return gl.LINEAR
	default:
		// WebGPU default is Nearest.
		return gl.NEAREST
	}
}

// mapMinFilter maps WebGPU min filter + mipmap filter to a combined GL filter constant.
// GL_TEXTURE_MIN_FILTER uses combined values like GL_LINEAR_MIPMAP_LINEAR.
func mapMinFilter(minFilter, mipmapFilter gputypes.FilterMode) int32 {
	switch {
	case minFilter == gputypes.FilterModeNearest && (mipmapFilter == gputypes.FilterModeNearest || mipmapFilter == gputypes.FilterModeUndefined):
		return gl.NEAREST_MIPMAP_NEAREST
	case minFilter == gputypes.FilterModeNearest && mipmapFilter == gputypes.FilterModeLinear:
		return gl.NEAREST_MIPMAP_LINEAR
	case minFilter == gputypes.FilterModeLinear && (mipmapFilter == gputypes.FilterModeNearest || mipmapFilter == gputypes.FilterModeUndefined):
		return gl.LINEAR_MIPMAP_NEAREST
	case minFilter == gputypes.FilterModeLinear && mipmapFilter == gputypes.FilterModeLinear:
		return gl.LINEAR_MIPMAP_LINEAR
	default:
		return gl.NEAREST_MIPMAP_NEAREST
	}
}

// mapAddressMode maps a WebGPU AddressMode to a GL wrap constant.
func mapAddressMode(mode gputypes.AddressMode) int32 {
	switch mode {
	case gputypes.AddressModeRepeat:
		return gl.REPEAT
	case gputypes.AddressModeMirrorRepeat:
		return gl.MIRRORED_REPEAT
	case gputypes.AddressModeClampToEdge:
		return gl.CLAMP_TO_EDGE
	default:
		// WebGPU default is ClampToEdge.
		return gl.CLAMP_TO_EDGE
	}
}

// mapCompareFunction maps a WebGPU CompareFunction to a GL compare function constant.
func mapCompareFunction(fn gputypes.CompareFunction) int32 {
	switch fn {
	case gputypes.CompareFunctionNever:
		return gl.NEVER
	case gputypes.CompareFunctionLess:
		return gl.LESS
	case gputypes.CompareFunctionEqual:
		return gl.EQUAL
	case gputypes.CompareFunctionLessEqual:
		return gl.LEQUAL
	case gputypes.CompareFunctionGreater:
		return gl.GREATER
	case gputypes.CompareFunctionNotEqual:
		return gl.NOTEQUAL
	case gputypes.CompareFunctionGreaterEqual:
		return gl.GEQUAL
	case gputypes.CompareFunctionAlways:
		return gl.ALWAYS
	default:
		return gl.ALWAYS
	}
}
