// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// flowShortCircuitOrRTWGSL is naga's readback case flow/short-circuit-or-rt
// (naga testdata/readback/flow/short-circuit-or-rt.wgsl, naga fix for
// P2-DXIL-107/092, released in v0.21.7). A short-circuit `||` chain over
// runtime values decides an if/else, and the chain's bool local meets at two
// if merges. As `phi i1` WARP ran neither arm's stores; as `alloca i1` D3D12's
// GPU-based validation dropped them. The case is about values: naga's own
// dxil tests assert the emitted shape.
const flowShortCircuitOrRTWGSL = `
@group(0) @binding(0) var<storage, read> src: array<u32>;
@group(0) @binding(1) var<storage, read_write> dst: array<u32>;

@compute @workgroup_size(4)
fn main(@builtin(local_invocation_index) li: u32) {
    let x = src[li * 3u];
    let y = src[li * 3u + 1u];
    let z = src[li * 3u + 2u];

    let limit = 65535u;
    if (x > limit || y > limit || z > limit) {
        dst[li] = 0u;
    } else {
        dst[li] = x + y + z;
    }
}
`

const (
	flowSrcWords = 12
	flowDstWords = 5
	// flowFill pre-fills dst so "wrote zero" and "wrote nothing" cannot be
	// confused: the sentinel surviving in word 4 also proves nothing wrote
	// past the four invocations' range.
	flowFill = 0xDEADBEEF
)

// TestFlowShortCircuitOrRT runs naga's flow/short-circuit-or-rt case through
// the public API. src holds three words per invocation: (10,20,30) sums to 60,
// and each of the other three trips the chain on its first, middle or last
// term and must store 0. Word 1 is the one WARP got wrong before naga v0.21.7.
func TestFlowShortCircuitOrRT(t *testing.T) {
	forEachOverrideBackend(t, gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, device *wgpu.Device) {
		src := newPublicBuffer(t, device, flowSrcWords*4, wgpu.BufferUsageStorage|wgpu.BufferUsageCopyDst)
		writeWords(t, device, src, 10, 20, 30, 70000, 20, 30, 10, 70000, 30, 10, 20, 70000)

		dst := newPublicBuffer(t, device, flowDstWords*4,
			wgpu.BufferUsageStorage|wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
		fill := make([]uint32, flowDstWords)
		for i := range fill {
			fill[i] = flowFill
		}
		writeWords(t, device, dst, fill...)

		staging := newPublicBuffer(t, device, flowDstWords*4, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)

		bgl := newBindGroupLayout(t, device,
			gputypes.BindGroupLayoutEntry{Binding: 0, Visibility: wgpu.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}},
			gputypes.BindGroupLayoutEntry{Binding: 1, Visibility: wgpu.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
		)
		group, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{Layout: bgl, Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: src, Size: flowSrcWords * 4},
			{Binding: 1, Buffer: dst, Size: flowDstWords * 4},
		}})
		if err != nil {
			t.Fatalf("CreateBindGroup: %v", err)
		}
		t.Cleanup(group.Release)
		pipeline := newComputePipeline(t, device, flowShortCircuitOrRTWGSL, bgl)

		enc := newEncoder(t, device)
		pass, err := enc.BeginComputePass(nil)
		if err != nil {
			t.Fatalf("BeginComputePass: %v", err)
		}
		pass.SetPipeline(pipeline)
		pass.SetBindGroup(0, group, nil)
		pass.Dispatch(1, 1, 1)
		if err := pass.End(); err != nil {
			t.Fatalf("ComputePass.End: %v", err)
		}
		enc.CopyBufferToBuffer(dst, 0, staging, 0, flowDstWords*4)
		submit(t, device, enc)

		checkWords(t, "flow/short-circuit-or-rt", readPublicBuffer(t, staging, flowDstWords),
			[]uint32{60, 0, 0, 0, flowFill})
	})
}
