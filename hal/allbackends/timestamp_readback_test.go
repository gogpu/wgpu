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

// timestampWorkShader gives the timed compute pass real work, so its end
// timestamp has something to come after.
const timestampWorkShader = `
@group(0) @binding(0) var<storage, read_write> data: array<u32, 256>;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    var x = id.x;
    for (var i = 0u; i < 4096u; i++) {
        x = x * 1664525u + 1013904223u;
    }
    data[id.x] = x;
}
`

// Query indices of the four pass-boundary timestamps.
const (
	tsComputeBegin = iota
	tsComputeEnd
	tsRenderBegin
	tsRenderEnd
	tsCount
)

// TestPassTimestampWrites writes timestamps at the boundaries of a compute
// pass and of a render pass through the public API, resolves them and checks
// that every end comes at or after its beginning (P1-API-016). It runs on the
// integrated and the discrete adapter of every backend that offers
// FeatureTimestampQuery; Vulkan runs under the validation layer with
// synchronization validation.
func TestPassTimestampWrites(t *testing.T) {
	for _, b := range []overrideBackend{
		{name: "vulkan", backends: gputypes.BackendsVulkan, backend: gputypes.BackendVulkan},
		{name: "dx12", backends: gputypes.BackendsDX12, backend: gputypes.BackendDX12},
	} {
		for _, pref := range []struct {
			name string
			pref gputypes.PowerPreference
		}{
			{"low-power", gputypes.PowerPreferenceLowPower},
			{"high-performance", gputypes.PowerPreferenceHighPerformance},
		} {
			t.Run(b.name+"/"+pref.name, func(t *testing.T) {
				device := openTimestampDevice(t, b, pref.pref)
				ts := recordPassTimestamps(t, device)
				t.Logf("ticks: compute %d..%d, render %d..%d; period %v ns",
					ts[tsComputeBegin], ts[tsComputeEnd], ts[tsRenderBegin], ts[tsRenderEnd],
					device.Queue().GetTimestampPeriod())
				for _, pair := range [][2]int{{tsComputeBegin, tsComputeEnd}, {tsRenderBegin, tsRenderEnd}} {
					if ts[pair[1]] == 0 {
						t.Errorf("timestamp %d is 0: the end-of-pass write did not happen", pair[1])
					}
					if ts[pair[1]] < ts[pair[0]] {
						t.Errorf("timestamp %d (%d) is before timestamp %d (%d)", pair[1], ts[pair[1]], pair[0], ts[pair[0]])
					}
				}
				if period := device.Queue().GetTimestampPeriod(); period <= 0 {
					t.Errorf("GetTimestampPeriod() = %v, want > 0", period)
				}
			})
		}
	}
}

// openTimestampDevice opens a device with FeatureTimestampQuery on the
// adapter pref selects, skipping when the backend has none that offers it.
func openTimestampDevice(t *testing.T, b overrideBackend, pref gputypes.PowerPreference) *wgpu.Device {
	t.Helper()
	var flags gputypes.InstanceFlags
	if b.backend == gputypes.BackendVulkan {
		flags = enableVulkanValidation(t)
	}
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: b.backends, Flags: flags})
	if err != nil {
		t.Skipf("CreateInstance: %v", err)
	}
	t.Cleanup(instance.Release)
	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{PowerPreference: pref})
	if err != nil {
		t.Skipf("no adapter: %v", err)
	}
	t.Cleanup(adapter.Release)
	info := adapter.Info()
	if info.Backend != b.backend {
		t.Skipf("no %s adapter (got %s, backend %v)", b.name, info.Name, info.Backend)
	}
	if !adapter.Features().Contains(gputypes.FeatureTimestampQuery) {
		t.Skipf("%s does not offer TimestampQuery", info.Name)
	}
	device, err := adapter.RequestDevice(&wgpu.DeviceDescriptor{
		Label:            "timestamps",
		RequiredFeatures: gputypes.Features(gputypes.FeatureTimestampQuery),
	})
	if err != nil {
		t.Fatalf("RequestDevice with TimestampQuery on %s: %v", info.Name, err)
	}
	t.Cleanup(device.Release)
	t.Logf("adapter: %s (%v)", info.Name, info.Backend)
	return device
}

// recordPassTimestamps runs a timed compute pass and a timed render pass and
// returns the four resolved timestamps in ticks.
func recordPassTimestamps(t *testing.T, device *wgpu.Device) [tsCount]uint64 {
	t.Helper()
	querySet, err := device.CreateQuerySet(&wgpu.QuerySetDescriptor{Label: "timestamps", Type: wgpu.QueryTypeTimestamp, Count: tsCount})
	if err != nil {
		t.Fatalf("CreateQuerySet: %v", err)
	}
	t.Cleanup(querySet.Release)

	bgl, pipeline, err := newPublicComputePipeline(t, device, timestampWorkShader, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	data := newPublicBuffer(t, device, 256*4, wgpu.BufferUsageStorage)
	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label: "timestamps", Layout: bgl,
		Entries: []wgpu.BindGroupEntry{{Binding: 0, Buffer: data, Size: 256 * 4}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	t.Cleanup(bg.Release)

	target, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label: "timestamps-target", Size: wgpu.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
		MipLevelCount: 1, SampleCount: 1, Dimension: gputypes.TextureDimension2D,
		Format: gputypes.TextureFormatRGBA8Unorm, Usage: gputypes.TextureUsageRenderAttachment,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(target.Release)
	view, err := device.CreateTextureView(target, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(view.Release)

	resolve := newPublicBuffer(t, device, tsCount*8, wgpu.BufferUsageQueryResolve|wgpu.BufferUsageCopySrc)
	staging := newPublicBuffer(t, device, tsCount*8, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)

	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	computeBegin, computeEnd := uint32(tsComputeBegin), uint32(tsComputeEnd)
	cpass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{
		Label: "timed-compute",
		TimestampWrites: &wgpu.ComputePassTimestampWrites{
			QuerySet: querySet, BeginningOfPassWriteIndex: &computeBegin, EndOfPassWriteIndex: &computeEnd,
		},
	})
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	cpass.SetPipeline(pipeline)
	cpass.SetBindGroup(0, bg, nil)
	cpass.Dispatch(4, 1, 1)
	if err := cpass.End(); err != nil {
		t.Fatalf("compute End: %v", err)
	}

	renderBegin, renderEnd := uint32(tsRenderBegin), uint32(tsRenderEnd)
	rpass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label: "timed-render",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 0, G: 0, B: 1, A: 1},
		}},
		TimestampWrites: &wgpu.RenderPassTimestampWrites{
			QuerySet: querySet, BeginningOfPassWriteIndex: &renderBegin, EndOfPassWriteIndex: &renderEnd,
		},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	if err := rpass.End(); err != nil {
		t.Fatalf("render End: %v", err)
	}

	encoder.ResolveQuerySet(querySet, 0, tsCount, resolve, 0)
	encoder.CopyBufferToBuffer(resolve, 0, staging, 0, tsCount*8)
	cmd, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := staging.Map(ctx, wgpu.MapModeRead, 0, tsCount*8); err != nil {
		t.Fatalf("Map: %v", err)
	}
	rng, err := staging.MappedRange(0, tsCount*8)
	if err != nil {
		t.Fatalf("MappedRange: %v", err)
	}
	var ts [tsCount]uint64
	for i := range ts {
		ts[i] = binary.LittleEndian.Uint64(rng.Bytes()[i*8:])
	}
	if err := staging.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}
	return ts
}
