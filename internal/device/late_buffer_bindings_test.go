//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/resource"
)

// newLateBindingLayout builds a layout over its entries with no device or HAL
// object.
func newLateBindingLayout(entries []gputypes.BindGroupLayoutEntry) *resource.BindGroupLayout {
	return resource.NewBindGroupLayout(nil, nil, entries)
}

func TestMakeLateSizedBufferGroups(t *testing.T) {
	shaderSizes := map[ir.ResourceBinding]uint64{
		{Group: 0, Binding: 0}: 64,
		{Group: 0, Binding: 2}: 128,
		{Group: 1, Binding: 0}: 32,
	}

	layouts := []*resource.BindGroupLayout{
		newLateBindingLayout([]gputypes.BindGroupLayoutEntry{
			{Binding: 0, Buffer: &gputypes.BufferBindingLayout{MinBindingSize: 0}},                            // late: should get 64
			{Binding: 1, Buffer: &gputypes.BufferBindingLayout{MinBindingSize: 256}},                          // NOT late: MinBindingSize != 0
			{Binding: 2, Buffer: &gputypes.BufferBindingLayout{MinBindingSize: 0}},                            // late: should get 128
			{Binding: 3, Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering}}, // not buffer
		}),
		newLateBindingLayout([]gputypes.BindGroupLayoutEntry{
			{Binding: 0, Buffer: &gputypes.BufferBindingLayout{MinBindingSize: 0}}, // late: should get 32
		}),
	}

	groups := makeLateSizedBufferGroups(shaderSizes, layouts)

	// Group 0: 2 late entries (bindings 0 and 2)
	if len(groups[0].ShaderSizes) != 2 {
		t.Fatalf("group 0 has %d late entries, want 2", len(groups[0].ShaderSizes))
	}
	if groups[0].ShaderSizes[0] != 64 {
		t.Errorf("group 0 entry 0 = %d, want 64", groups[0].ShaderSizes[0])
	}
	if groups[0].ShaderSizes[1] != 128 {
		t.Errorf("group 0 entry 1 = %d, want 128", groups[0].ShaderSizes[1])
	}

	// Group 1: 1 late entry (binding 0)
	if len(groups[1].ShaderSizes) != 1 {
		t.Fatalf("group 1 has %d late entries, want 1", len(groups[1].ShaderSizes))
	}
	if groups[1].ShaderSizes[0] != 32 {
		t.Errorf("group 1 entry 0 = %d, want 32", groups[1].ShaderSizes[0])
	}

	// Groups 2-7: no late entries
	for i := 2; i < core.MaxBindGroups; i++ {
		if len(groups[i].ShaderSizes) != 0 {
			t.Errorf("group %d has %d late entries, want 0", i, len(groups[i].ShaderSizes))
		}
	}
}
