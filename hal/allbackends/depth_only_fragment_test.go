// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package allbackends_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestDepthOnlyPipelineWithFragmentStage: a render pipeline with a fragment
// stage, no color targets and a depth-stencil state (a shadow map pass) is
// created and draws into a pass with only a depth attachment. WebGPU needs at
// least one attachment, a color target or the depth-stencil state
// (P1-VAL-016). The depth the triangle wrote (z = 0) is read back.
func TestDepthOnlyPipelineWithFragmentStage(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		skipKnownGap(t, "software", "P1-SW-015", "depth is never written back to the texture")
		r := newDepthRig(t, device, gputypes.TextureFormatDepth32Float,
			wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageTextureBinding)
		shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
			Label: "depth-only", WGSL: cullShader + "\n@fragment fn fs_depth() {}\n",
		})
		if err != nil {
			t.Fatalf("CreateShaderModule: %v", err)
		}
		t.Cleanup(shader.Release)
		layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "depth-only"})
		if err != nil {
			t.Fatalf("CreatePipelineLayout: %v", err)
		}
		t.Cleanup(layout.Release)
		pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
			Label:     "depth-only",
			Layout:    layout,
			Vertex:    wgpu.VertexState{Module: shader, EntryPoint: "vs"},
			Fragment:  &wgpu.FragmentState{Module: shader, EntryPoint: "fs_depth"},
			Primitive: gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
			DepthStencil: &wgpu.DepthStencilState{
				Format:            gputypes.TextureFormatDepth32Float,
				DepthWriteEnabled: true,
				DepthCompare:      gputypes.CompareFunctionLess,
				StencilFront:      keepStencil,
				StencilBack:       keepStencil,
			},
			Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
		})
		if err != nil {
			t.Fatalf("CreateRenderPipeline: %v", err)
		}
		t.Cleanup(pipeline.Release)

		enc := newEncoder(t, device)
		pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
			DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
				View:        r.depthView,
				DepthLoadOp: gputypes.LoadOpClear, DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: 1,
			},
		})
		if err != nil {
			t.Fatalf("BeginRenderPass: %v", err)
		}
		pass.SetPipeline(pipeline)
		pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
		if err := pass.End(); err != nil {
			t.Fatalf("End: %v", err)
		}
		submit(t, device, enc)
		if got := r.sampleDepth(t); got != 0 {
			t.Fatalf("sampled depth = %v, want 0 (the triangle's z)", got)
		}
	})
}
