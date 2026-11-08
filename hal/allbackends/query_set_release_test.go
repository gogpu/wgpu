// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// Query indices of the compute and the render pass of the release test.
const (
	releaseComputeBegin = iota
	releaseComputeEnd
	releaseRenderBegin
	releaseRenderEnd
	releaseQueryCount
)

// TestReleaseQuerySetBeforeSubmit: a timestamp query set written at the
// boundaries of a compute pass and of a render pass, then resolved, is
// released before Submit. Release is a drop, not a destroy (ADR-056): the
// command buffer holds the query set's reference, so the HAL query pool must
// stay alive until the submitted work completes (P1-CORE-021;
// VUID-vkDestroyQueryPool-queryPool-00793). The readback also proves the
// timestamps were still written.
func TestReleaseQuerySetBeforeSubmit(t *testing.T) {
	forEachOverrideBackendFeatures(t, 0, gputypes.Features(gputypes.FeatureTimestampQuery),
		func(t *testing.T, device *wgpu.Device) {
			if !device.Features().Contains(gputypes.FeatureTimestampQuery) {
				t.Skip("adapter does not offer TimestampQuery")
			}
			querySet, err := device.CreateQuerySet(&wgpu.QuerySetDescriptor{
				Label: "release-timestamps",
				Type:  wgpu.QueryTypeTimestamp,
				Count: releaseQueryCount,
			})
			if err != nil {
				t.Fatalf("CreateQuerySet: %v", err)
			}
			resolve := newPublicBuffer(t, device, releaseQueryCount*8,
				wgpu.BufferUsageQueryResolve|wgpu.BufferUsageCopySrc)
			staging := newPublicBuffer(t, device, releaseQueryCount*8,
				wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)

			enc := newEncoder(t, device)
			computeBegin, computeEnd := uint32(releaseComputeBegin), uint32(releaseComputeEnd)
			cpass, err := enc.BeginComputePass(&wgpu.ComputePassDescriptor{
				Label: "release-timestamps-compute",
				TimestampWrites: &wgpu.ComputePassTimestampWrites{
					QuerySet:                  querySet,
					BeginningOfPassWriteIndex: &computeBegin,
					EndOfPassWriteIndex:       &computeEnd,
				},
			})
			if err != nil {
				t.Fatalf("BeginComputePass: %v", err)
			}
			if err := cpass.End(); err != nil {
				t.Fatalf("compute End: %v", err)
			}

			target := newReleaseTexture(t, device, wgpu.TextureUsageRenderAttachment)
			t.Cleanup(target.Release)
			view, err := device.CreateTextureView(target, nil)
			if err != nil {
				t.Fatalf("CreateTextureView: %v", err)
			}
			t.Cleanup(view.Release)
			renderBegin, renderEnd := uint32(releaseRenderBegin), uint32(releaseRenderEnd)
			rpass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
				Label: "release-timestamps-render",
				ColorAttachments: []wgpu.RenderPassColorAttachment{{
					View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
					ClearValue: gputypes.Color{R: 0, G: 0, B: 1, A: 1},
				}},
				TimestampWrites: &wgpu.RenderPassTimestampWrites{
					QuerySet:                  querySet,
					BeginningOfPassWriteIndex: &renderBegin,
					EndOfPassWriteIndex:       &renderEnd,
				},
			})
			if err != nil {
				t.Fatalf("BeginRenderPass: %v", err)
			}
			if err := rpass.End(); err != nil {
				t.Fatalf("render End: %v", err)
			}

			enc.ResolveQuerySet(querySet, 0, releaseQueryCount, resolve, 0)
			enc.CopyBufferToBuffer(resolve, 0, staging, 0, releaseQueryCount*8)

			// Released after the work that uses it was recorded, before Submit.
			querySet.Release()
			submit(t, device, enc)

			ts := readQueryTicks(t, staging, releaseQueryCount)
			for _, pair := range [][2]int{
				{releaseComputeBegin, releaseComputeEnd},
				{releaseRenderBegin, releaseRenderEnd},
			} {
				if ts[pair[1]] == 0 {
					t.Errorf("timestamp %d is 0: the end-of-pass write did not happen", pair[1])
				}
				if ts[pair[1]] < ts[pair[0]] {
					t.Errorf("timestamp %d (%d) is before timestamp %d (%d)",
						pair[1], ts[pair[1]], pair[0], ts[pair[0]])
				}
			}
		})
}

// readQueryTicks maps the staging buffer and returns its timestamps in ticks.
func readQueryTicks(t *testing.T, staging *wgpu.Buffer, count int) []uint64 {
	t.Helper()
	size := uint64(count * 8)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := staging.Map(ctx, wgpu.MapModeRead, 0, size); err != nil {
		t.Fatalf("Map: %v", err)
	}
	rng, err := staging.MappedRange(0, size)
	if err != nil {
		t.Fatalf("MappedRange: %v", err)
	}
	ts := make([]uint64, count)
	for i := range ts {
		ts[i] = binary.LittleEndian.Uint64(rng.Bytes()[i*8:])
	}
	if err := staging.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}
	return ts
}
