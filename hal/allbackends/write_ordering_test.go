// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// fillShader writes 0xAAAAAAAA into every word of the bound storage buffer.
const fillShader = `
@group(0) @binding(0) var<storage, read_write> b: array<u32>;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    if (gid.x < arrayLength(&b)) {
        b[gid.x] = 0xAAAAAAAAu;
    }
}
`

// writeOrderingRuns is how often the race of #371 is given a chance. On an
// NVIDIA GT 1030 about a quarter of the runs read stale data before the fix.
const writeOrderingRuns = 200

// TestWriteBufferAfterDispatchIsOrdered is the #371 regression (P1-CORE-008):
// a compute pass fills a storage buffer, the host waits for it, overwrites it
// with Queue.WriteBuffer(zeros) and copies it to a staging buffer in the next
// submit. The pending-write copy and the readback copy must be ordered: the
// write goes through the device tracker (STORAGE → COPY_DST before the
// staging copy) and the readback's COPY_DST → COPY_SRC barrier is recorded in
// a transit command buffer after the pending writes, not before them.
func TestWriteBufferAfterDispatchIsOrdered(t *testing.T) {
	forEachOverrideBackend(t, gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, device *wgpu.Device) {
		const words = 12
		size := uint64(words * 4)
		buf := newPublicBuffer(t, device, size, wgpu.BufferUsageStorage|wgpu.BufferUsageCopyDst|wgpu.BufferUsageCopySrc)
		staging := newPublicBuffer(t, device, size, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)

		bgl, pipeline, err := newPublicComputePipeline(t, device, fillShader, nil, 1)
		if err != nil {
			t.Fatal(err)
		}
		bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
			Label: "fill-bg", Layout: bgl,
			Entries: []wgpu.BindGroupEntry{{Binding: 0, Buffer: buf, Size: size}},
		})
		if err != nil {
			t.Fatalf("CreateBindGroup: %v", err)
		}
		defer bg.Release()

		zeros := make([]byte, size)
		stale := 0
		for run := 0; run < writeOrderingRuns; run++ {
			enc, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			pass, err := enc.BeginComputePass(nil)
			if err != nil {
				t.Fatalf("BeginComputePass: %v", err)
			}
			pass.SetPipeline(pipeline)
			pass.SetBindGroup(0, bg, nil)
			pass.Dispatch(1, 1, 1)
			if err := pass.End(); err != nil {
				t.Fatalf("End: %v", err)
			}
			fill, err := enc.Finish()
			if err != nil {
				t.Fatalf("Finish: %v", err)
			}
			if _, err := device.Queue().Submit(fill); err != nil {
				t.Fatalf("Submit fill: %v", err)
			}
			device.Poll(wgpu.PollWait)

			if err := device.Queue().WriteBuffer(buf, 0, zeros); err != nil {
				t.Fatalf("WriteBuffer: %v", err)
			}
			submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
				enc.CopyBufferToBuffer(buf, 0, staging, 0, size)
			})
			for _, w := range readPublicBuffer(t, staging, words) {
				if w != 0 {
					stale++
					break
				}
			}
		}
		if stale != 0 {
			t.Errorf("readback after WriteBuffer(zeros) showed the compute pass's data in %d of %d runs", stale, writeOrderingRuns)
		}
	})
}
