// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

func renderPipelineICBCandidate(desc *hal.RenderPipelineDescriptor) bool {
	if desc == nil || desc.Primitive.Topology != gputypes.PrimitiveTopologyTriangleList {
		return false
	}
	layout, ok := desc.Layout.(*PipelineLayout)
	if !ok || layout == nil {
		return false
	}
	for _, rawLayout := range layout.layouts {
		group, ok := rawLayout.(*BindGroupLayout)
		if !ok || group == nil {
			return false
		}
		for _, entry := range group.entries {
			if entry.Buffer == nil || entry.Buffer.Type == gputypes.BufferBindingTypeStorage ||
				entry.Texture != nil || entry.StorageTexture != nil || entry.Sampler != nil {
				return false
			}
		}
	}
	return true
}

func (d *Device) canCreateICBPipeline(desc *hal.RenderPipelineDescriptor, pipelineDesc ID) bool {
	if d == nil || d.raw == 0 || pipelineDesc == 0 || !renderPipelineICBCandidate(desc) {
		return false
	}
	// Metal family support is cumulative. Apple8+ devices also report Apple7,
	// so this admits the M1 proof family and later Apple GPU families while
	// leaving Intel and AMD devices on the ordinary path.
	if !DeviceSupportsFamily(d.raw, MTLGPUFamilyApple7) {
		return false
	}
	setter := Sel("setSupportIndirectCommandBuffers:")
	return setter != 0 && MsgSendBool(pipelineDesc, Sel("respondsToSelector:"), uintptr(setter))
}

// createRenderPipelineState centralizes the fail-closed policy: an ICB
// candidate gets one flagged attempt, followed by the ordinary attempt on any
// failure. Callers only observe failure when the ordinary pipeline also fails.
func createRenderPipelineState(candidate bool, create func(icb bool) ID) (ID, bool) {
	if candidate {
		if pipeline := create(true); pipeline != 0 {
			return pipeline, true
		}
	}
	return create(false), false
}
