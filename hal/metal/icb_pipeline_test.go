// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

func TestRenderPipelineICBCandidateIsPrivateAndBufferOnly(t *testing.T) {
	bufferLayout := &BindGroupLayout{entries: []gputypes.BindGroupLayoutEntry{{Buffer: &gputypes.BufferBindingLayout{}}}}
	readOnlyStorageLayout := &BindGroupLayout{entries: []gputypes.BindGroupLayoutEntry{{Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}}}}
	writableStorageLayout := &BindGroupLayout{entries: []gputypes.BindGroupLayoutEntry{{Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}}}}
	textureLayout := &BindGroupLayout{entries: []gputypes.BindGroupLayoutEntry{{Texture: &gputypes.TextureBindingLayout{}}}}

	tests := []struct {
		name string
		desc hal.RenderPipelineDescriptor
		want bool
	}{
		{
			name: "triangle buffer-only",
			desc: hal.RenderPipelineDescriptor{
				Primitive: gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
				Layout:    &PipelineLayout{layouts: []hal.BindGroupLayout{bufferLayout}},
			},
			want: true,
		},
		{
			name: "triangle without bindings",
			desc: hal.RenderPipelineDescriptor{
				Primitive: gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
				Layout:    &PipelineLayout{},
			},
			want: true,
		},
		{
			name: "read-only storage binding",
			desc: hal.RenderPipelineDescriptor{
				Primitive: gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
				Layout:    &PipelineLayout{layouts: []hal.BindGroupLayout{readOnlyStorageLayout}},
			},
			want: true,
		},
		{
			name: "writable storage binding",
			desc: hal.RenderPipelineDescriptor{
				Primitive: gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
				Layout:    &PipelineLayout{layouts: []hal.BindGroupLayout{writableStorageLayout}},
			},
		},
		{
			name: "texture binding",
			desc: hal.RenderPipelineDescriptor{
				Primitive: gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
				Layout:    &PipelineLayout{layouts: []hal.BindGroupLayout{textureLayout}},
			},
		},
		{
			name: "non-triangle topology",
			desc: hal.RenderPipelineDescriptor{
				Primitive: gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyLineList},
				Layout:    &PipelineLayout{layouts: []hal.BindGroupLayout{bufferLayout}},
			},
		},
		{
			name: "foreign layout",
			desc: hal.RenderPipelineDescriptor{Primitive: gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderPipelineICBCandidate(&tt.desc); got != tt.want {
				t.Fatalf("candidate = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCreateRenderPipelineStateRetriesOrdinaryAfterICBFailure(t *testing.T) {
	var attempts []bool
	pipeline, compatible := createRenderPipelineState(true, func(icb bool) ID {
		attempts = append(attempts, icb)
		if icb {
			return 0
		}
		return 42
	})
	if pipeline != 42 || compatible {
		t.Fatalf("result = (%d, %v), want (42, false)", pipeline, compatible)
	}
	if len(attempts) != 2 || !attempts[0] || attempts[1] {
		t.Fatalf("attempts = %v, want [true false]", attempts)
	}
}

func TestCreateRenderPipelineStateSkipsFlagForIneligiblePipeline(t *testing.T) {
	var attempts []bool
	pipeline, compatible := createRenderPipelineState(false, func(icb bool) ID {
		attempts = append(attempts, icb)
		return 7
	})
	if pipeline != 7 || compatible {
		t.Fatalf("result = (%d, %v), want (7, false)", pipeline, compatible)
	}
	if len(attempts) != 1 || attempts[0] {
		t.Fatalf("attempts = %v, want [false]", attempts)
	}
}
