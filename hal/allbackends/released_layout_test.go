// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// A bind group layout has no destroy() in WebGPU; Release is Rust's drop
// (ADR-056). An object created from it after that must be refused, as Rust
// refuses anything that needs a value it no longer has: the HAL bind group does
// not keep the layout (Rust wgpu-hal's Vulkan BindGroup holds only the
// descriptor set), so a new bind group or pipeline layout would name a HAL
// layout whose destruction is already queued. P2-CORE-023.

// TestCreateBindGroupWithReleasedLayout: CreateBindGroup must refuse a layout
// the application released, instead of allocating a descriptor set from a
// destroyed VkDescriptorSetLayout (VUID-vkAllocateDescriptorSets-pAllocateInfo).
func TestCreateBindGroupWithReleasedLayout(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		bgl := newBindGroupLayout(t, device, gputypes.BindGroupLayoutEntry{
			Binding: 0, Visibility: wgpu.ShaderStageCompute,
			Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeNonFiltering},
		})
		sampler, err := device.CreateSampler(&wgpu.SamplerDescriptor{Label: "released-layout-sampler"})
		if err != nil {
			t.Fatalf("CreateSampler: %v", err)
		}
		t.Cleanup(sampler.Release)

		bgl.Release()
		flushDeferredDestruction(t, device)

		group, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
			Label:   "released-layout-group",
			Layout:  bgl,
			Entries: []wgpu.BindGroupEntry{{Binding: 0, Sampler: sampler}},
		})
		if !errors.Is(err, wgpu.ErrReleased) {
			if group != nil {
				group.Release()
			}
			t.Fatalf("CreateBindGroup with a released layout = %v, want ErrReleased", err)
		}
	})
}

// TestCreatePipelineLayoutWithReleasedLayout: CreatePipelineLayout must refuse
// a bind group layout the application released, as its HAL call reads the
// descriptor set layouts the pipeline layout is built from.
func TestCreatePipelineLayoutWithReleasedLayout(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		bgl := newBindGroupLayout(t, device, gputypes.BindGroupLayoutEntry{
			Binding: 0, Visibility: wgpu.ShaderStageCompute,
			Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeNonFiltering},
		})
		bgl.Release()
		flushDeferredDestruction(t, device)

		layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
			Label:            "released-layout-pipeline",
			BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
		})
		if !errors.Is(err, wgpu.ErrReleased) {
			if layout != nil {
				layout.Release()
			}
			t.Fatalf("CreatePipelineLayout with a released layout = %v, want ErrReleased", err)
		}
	})
}

// flushDeferredDestruction submits an empty command buffer and waits for the
// device, so the destroy queue's Triage runs the destruction a Release
// deferred. Without it the HAL layout may still be alive and the check under
// test would not be exercised.
func flushDeferredDestruction(t *testing.T, device *wgpu.Device) {
	t.Helper()
	enc := newEncoder(t, device)
	submit(t, device, enc)
	if err := device.WaitIdle(); err != nil {
		t.Fatalf("WaitIdle: %v", err)
	}
}
