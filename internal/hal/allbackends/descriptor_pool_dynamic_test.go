// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestDynamicBufferBindGroupsAfterPlainOnes: bind groups whose layouts use
// dynamic uniform and storage buffers, created after a plain one. On Vulkan the
// plain group creates the first descriptor pool, and the dynamic groups are
// allocated from it, so every pool must hold the dynamic buffer descriptor
// types; the validation layer reports
// WARNING-CoreValidation-AllocateDescriptorSets-WrongType otherwise, and strict
// drivers return VK_ERROR_OUT_OF_POOL_MEMORY (P2-VK-016, from gogpu#461).
func TestDynamicBufferBindGroupsAfterPlainOnes(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		uniform := newPublicBuffer(t, device, 256, wgpu.BufferUsageUniform)
		bindBuffer := func(label string, layout gputypes.BufferBindingLayout, buf *wgpu.Buffer) {
			t.Helper()
			bgl := newBindGroupLayout(t, device, gputypes.BindGroupLayoutEntry{
				Binding: 0, Visibility: wgpu.ShaderStageCompute, Buffer: &layout,
			})
			group, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{Label: label, Layout: bgl,
				Entries: []wgpu.BindGroupEntry{{Binding: 0, Buffer: buf, Size: 64}}})
			if err != nil {
				t.Fatalf("CreateBindGroup %s: %v", label, err)
			}
			t.Cleanup(group.Release)
		}

		bindBuffer("plain-uniform", gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}, uniform)
		if device.Limits().MaxDynamicUniformBuffersPerPipelineLayout > 0 {
			bindBuffer("dynamic-uniform", gputypes.BufferBindingLayout{
				Type: gputypes.BufferBindingTypeUniform, HasDynamicOffset: true,
			}, uniform)
		}
		if device.Limits().MaxDynamicStorageBuffersPerPipelineLayout > 0 {
			storage := newPublicBuffer(t, device, 256, wgpu.BufferUsageStorage)
			bindBuffer("dynamic-storage", gputypes.BufferBindingLayout{
				Type: gputypes.BufferBindingTypeStorage, HasDynamicOffset: true,
			}, storage)
		}
	})
}
