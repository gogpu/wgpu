//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

// This file declares the device-local values the descriptor boundary needs
// (ADR-070 shape 1, S11 lead ruling): the root package converts a public
// descriptor with toHAL and unwraps the public handles, and the implementation
// here receives the HAL descriptor plus these few values the HAL descriptor
// cannot carry. No public root type appears: every handle below is an internal
// one, and every scalar is a gputypes value or a primitive.
//
// The structs are deliberately minimal. Each field exists because the pre-move
// body read it from the public descriptor and the HAL descriptor does not hold
// it; nothing here mirrors a public descriptor field the HAL descriptor already
// keeps.

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/resource"
)

// BindGroupDescriptor is the internal form of a public bind group descriptor:
// the label, the internal layout handle and the internal resource handles of
// each entry. The pre-move body read the wrappers' impls for validation, the
// tracking derivation and the reference clones, all of which need these
// handles rather than the flat gputypes entries.
type BindGroupDescriptor struct {
	Label   string
	Layout  *resource.BindGroupLayout
	Entries []BindGroupEntry
}

// BindGroupEntry is one binding of an internal bind group descriptor. Exactly
// one of Buffer, Sampler or TextureView is set, as in the public entry.
type BindGroupEntry struct {
	Binding     uint32
	Buffer      *resource.Buffer
	Offset      uint64
	Size        uint64
	Sampler     *resource.Sampler
	TextureView *resource.TextureView
}

// RenderPipelineExtras carries what the HAL render pipeline descriptor cannot:
// the pipeline-layout handle the refcount and group count come from, the
// shader-module handles the constants and binding-size checks read, and the
// fragment description the pass context and the blend-constant flag are derived
// from.
//
// HasFragment and FragmentTargets are separate from the HAL descriptor because
// the public toHAL drops the fragment stage when its module is nil: the pass
// context and the blend check must still see the fragment the caller passed, as
// the pre-move body did.
type RenderPipelineExtras struct {
	Layout          *resource.PipelineLayout
	VertexModule    *resource.ShaderModule
	FragmentModule  *resource.ShaderModule
	HasFragment     bool
	FragmentTargets []gputypes.ColorTargetState
}

// resolveDepthCompare gives the HAL CompareFunctionAlways for a depthCompare
// the descriptor left undefined. GPUDepthStencilState.depthCompare has no
// default: core validation, which runs first, rejects its absence where the
// spec requires it, and elsewhere the spec's "compare fragment" algorithm
// passes every fragment for an undefined compare, which is "always"
// (index.bs). The backends take the value literally: DX12 mapped Undefined to
// NEVER and drew nothing. Rust wgpu-hal does ds.depth_compare.unwrap_or_default().
func resolveDepthCompare(ds *hal.DepthStencilState) {
	if ds != nil && ds.DepthCompare == gputypes.CompareFunctionUndefined {
		ds.DepthCompare = gputypes.CompareFunctionAlways
	}
}
