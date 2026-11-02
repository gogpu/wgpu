// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/binary"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// These tests run DispatchIndirect through the public API, where core runs
// its indirect validation shader before every indirect dispatch
// (wgpu-core/src/indirect_validation/dispatch.rs), and count the invocations
// by readback. Every offset of a pass and of every encoder recorded before a
// submit must read its own arguments; Vulkan runs under the validation layer
// with synchronization validation, DX12 under the debug layer.

// dispatchCountShader counts its invocations: workgroup size 1, so the
// counter ends at x*y*z of every dispatch.
const dispatchCountShader = `
@group(0) @binding(0) var<storage, read_write> counter: array<atomic<u32>, 1>;

@compute @workgroup_size(1)
fn main() {
    atomicAdd(&counter[0], 1u);
}
`

// Indirect arguments at offsets that need different validation inputs: 0,
// 12 (not 16-byte aligned), 268 (past minStorageBufferOffsetAlignment, with a
// remainder) and 280, whose x exceeds every maxComputeWorkgroupsPerDimension,
// so validation turns it into an empty dispatch.
var dispatchIndirectArgs = []struct {
	offset  uint64
	x, y, z uint32
}{
	{0, 1, 1, 1},
	{12, 4, 1, 1},
	{268, 2, 3, 1},
	{280, 0xFFFF_FFFF, 1, 1},
}

const dispatchIndirectArgsSize = 512

// dispatchIndirectBackends are the backends the indirect tests run on; the
// GPU backends run on both power preferences, so on a hybrid machine both
// adapters.
var dispatchIndirectBackends = []overrideBackend{
	{name: "vulkan", backends: gputypes.BackendsVulkan, backend: gputypes.BackendVulkan},
	{name: "dx12-fxc", backends: gputypes.BackendsDX12, backend: gputypes.BackendDX12},
	{name: "dx12-dxil", backends: gputypes.BackendsDX12, backend: gputypes.BackendDX12, dxil: true},
	{name: "gles", backends: gputypes.BackendsGL, backend: gputypes.BackendGL},
	{name: "metal", backends: gputypes.BackendsMetal, backend: gputypes.BackendMetal},
	{name: "software", backends: gputypes.BackendsPrimary, software: true},
}

// TestDispatchIndirectReadsItsOwnArguments checks the invocation count of
// DispatchIndirect at single offsets, at several offsets in one pass and from
// two encoders recorded before either is submitted.
func TestDispatchIndirectReadsItsOwnArguments(t *testing.T) {
	forEachIndirectDevice(t, func(t *testing.T, device *wgpu.Device) {
		r := newIndirectRig(t, device)

		single := map[uint64]uint32{0: 1, 12: 4, 268: 6, 280: 0}
		for _, a := range dispatchIndirectArgs {
			if got, want := r.run(t, [][]uint64{{a.offset}}), single[a.offset]; got != want {
				t.Errorf("DispatchIndirect at offset %d: %d invocations, want %d", a.offset, got, want)
			}
		}
		if got := r.run(t, [][]uint64{{0, 12}}); got != 5 {
			t.Errorf("offsets 0 and 12 in one pass: %d invocations, want 5", got)
		}
		if got := r.run(t, [][]uint64{{268, 0, 280, 12}}); got != 11 {
			t.Errorf("offsets 268, 0, 280, 12 in one pass: %d invocations, want 11", got)
		}
		if got := r.run(t, [][]uint64{{0}, {12}}); got != 5 {
			t.Errorf("offset 0 and offset 12 from two encoders: %d invocations, want 5", got)
		}
		if got := r.run(t, [][]uint64{{12, 268}, {0}, {268}}); got != 17 {
			t.Errorf("offsets 12+268, 0, 268 from three encoders: %d invocations, want 17", got)
		}

		// The validation bind groups of a buffer released while its work
		// is in flight must live until that work completes (the layers
		// report a destroyed buffer or descriptor set in a submitted
		// command buffer).
		released := newIndirectRig(t, device)
		released.releaseArgs = true
		if got := released.run(t, [][]uint64{{268, 12}, {0}}); got != 11 {
			t.Errorf("offsets 268+12, 0 with the buffer released after submit: %d invocations, want 11", got)
		}
	})
}

// indirectRig holds the arguments buffer and the counting pipeline.
type indirectRig struct {
	device   *wgpu.Device
	args     *wgpu.Buffer
	bgl      *wgpu.BindGroupLayout
	pipeline *wgpu.ComputePipeline
	// releaseArgs releases the arguments buffer right after the submits,
	// while the GPU may still run the recorded commands.
	releaseArgs bool
}

func newIndirectRig(t *testing.T, device *wgpu.Device) *indirectRig {
	t.Helper()
	args := newPublicBuffer(t, device, dispatchIndirectArgsSize, wgpu.BufferUsageIndirect|wgpu.BufferUsageCopyDst)
	data := make([]byte, dispatchIndirectArgsSize)
	for _, a := range dispatchIndirectArgs {
		binary.LittleEndian.PutUint32(data[a.offset:], a.x)
		binary.LittleEndian.PutUint32(data[a.offset+4:], a.y)
		binary.LittleEndian.PutUint32(data[a.offset+8:], a.z)
	}
	if err := device.Queue().WriteBuffer(args, 0, data); err != nil {
		t.Fatalf("WriteBuffer(args): %v", err)
	}
	bgl, pipeline, err := newPublicComputePipeline(t, device, dispatchCountShader, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	return &indirectRig{device: device, args: args, bgl: bgl, pipeline: pipeline}
}

// run records one encoder per element of encoders, each with one compute
// pass that dispatches indirectly at the listed offsets, submits them in
// order after all are recorded, and returns the invocation count.
func (r *indirectRig) run(t *testing.T, encoders [][]uint64) uint32 {
	t.Helper()
	counter := newPublicBuffer(t, r.device, 16, wgpu.BufferUsageStorage|wgpu.BufferUsageCopyDst|wgpu.BufferUsageCopySrc)
	staging := newPublicBuffer(t, r.device, 16, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
	if err := r.device.Queue().WriteBuffer(counter, 0, make([]byte, 16)); err != nil {
		t.Fatalf("WriteBuffer(counter): %v", err)
	}
	bg, err := r.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label: "indirect-counter", Layout: r.bgl,
		Entries: []wgpu.BindGroupEntry{{Binding: 0, Buffer: counter, Size: 16}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	defer bg.Release()

	cmds := make([]*wgpu.CommandBuffer, 0, len(encoders))
	for _, offsets := range encoders {
		cmds = append(cmds, r.record(t, bg, offsets))
	}
	for _, cmd := range cmds {
		if _, err := r.device.Queue().Submit(cmd); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}
	if r.releaseArgs {
		r.args.Release()
	}
	submitCopy(t, r.device, func(enc *wgpu.CommandEncoder) {
		enc.CopyBufferToBuffer(counter, 0, staging, 0, 16)
	})
	return readPublicBuffer(t, staging, 1)[0]
}

func (r *indirectRig) record(t *testing.T, bg *wgpu.BindGroup, offsets []uint64) *wgpu.CommandBuffer {
	t.Helper()
	enc, err := r.device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := enc.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	pass.SetPipeline(r.pipeline)
	pass.SetBindGroup(0, bg, nil)
	for _, off := range offsets {
		pass.DispatchIndirect(r.args, off)
	}
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	return cmd
}

// forEachIndirectDevice runs fn on every backend that has an adapter with
// compute shaders and indirect execution, on both power preferences.
func forEachIndirectDevice(t *testing.T, fn func(t *testing.T, device *wgpu.Device)) {
	t.Helper()
	// Core refuses INDIRECT buffers on an adapter without indirect execution.
	const required = gputypes.DownlevelFlagsComputeShaders | gputypes.DownlevelFlagsIndirectExecution
	for _, b := range dispatchIndirectBackends {
		for _, pref := range featurePowerPreferences {
			if b.software && pref.pref != gputypes.PowerPreferenceLowPower {
				continue // one CPU adapter
			}
			t.Run(b.name+"/"+pref.name, func(t *testing.T) {
				if b.dxil {
					t.Setenv("GOGPU_DX12_DXIL", "1")
				} else {
					t.Setenv("GOGPU_DX12_DXIL", "0")
				}
				fn(t, openIndirectDevice(t, b, pref.pref, required))
			})
		}
	}
}

// openIndirectDevice is openBackendDevice with the DX12 debug layer on as
// well as the Vulkan validation layer.
func openIndirectDevice(t *testing.T, b overrideBackend, pref gputypes.PowerPreference, required gputypes.DownlevelFlags) *wgpu.Device {
	t.Helper()
	var flags gputypes.InstanceFlags
	switch {
	case b.software:
	case b.backend == gputypes.BackendVulkan:
		flags = enableVulkanValidation(t)
	case b.backend == gputypes.BackendDX12:
		flags = enableDX12DebugLayer(t)
	}
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: b.backends, Flags: flags})
	if err != nil {
		t.Skipf("CreateInstance: %v", err)
	}
	t.Cleanup(instance.Release)
	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{PowerPreference: pref, ForceFallbackAdapter: b.software})
	if err != nil {
		t.Skipf("no adapter: %v", err)
	}
	t.Cleanup(adapter.Release)
	info := adapter.Info()
	isCPU := info.DeviceType == gputypes.DeviceTypeCPU
	if b.software != isCPU || (!b.software && info.Backend != b.backend) {
		t.Skipf("no %s adapter (got %s, backend %v)", b.name, info.Name, info.Backend)
	}
	if missing := required &^ adapter.DownlevelCapabilities().Flags; missing != 0 {
		t.Skipf("%s lacks required downlevel flags %#x", info.Name, uint32(missing))
	}
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		t.Skipf("RequestDevice: %v", err)
	}
	t.Cleanup(device.Release)
	t.Logf("adapter: %s (%v)", info.Name, info.Backend)
	return device
}
