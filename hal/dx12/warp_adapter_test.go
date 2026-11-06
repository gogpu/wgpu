// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// The tests in this package that read results back run on every enumerated
// DX12 adapter, WARP included: the software D3D12 device that ships with every
// Windows (d3d10warp.dll), listed as an adapter of DeviceTypeCPU. WARP executes command lists on many CPU threads
// and checks descriptor types strictly, so it shows missing barriers and
// wrong descriptor kinds that a hardware driver tolerates; it is also the
// only DX12 adapter of a machine without a GPU, such as a CI runner.

// dx12TestDevice is one device under test.
type dx12TestDevice struct {
	name   string
	device hal.Device
	queue  hal.Queue
}

// forEachDX12TestDevice runs fn on a device of every enumerated DX12 adapter
// (WARP included), once per shader path (HLSL through FXC, and naga DXIL).
func forEachDX12TestDevice(t *testing.T, fn func(t *testing.T, d dx12TestDevice)) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping GPU integration test in short mode")
	}
	for _, path := range []struct{ name, dxil string }{{"FXC", ""}, {"DXIL", "1"}} {
		t.Run(path.name, func(t *testing.T) {
			t.Setenv("GOGPU_DX12_DXIL", path.dxil)
			forEachDX12Adapter(t, fn)
		})
	}
}

// forEachDX12Adapter runs fn on a device of every enumerated DX12 adapter,
// WARP included, for tests that do not compile shaders.
func forEachDX12Adapter(t *testing.T, fn func(t *testing.T, d dx12TestDevice)) {
	t.Helper()
	adapters := enumerateDX12TestAdapters(t)
	requireWARP(t, adapters)
	for i := range adapters {
		a := &adapters[i]
		t.Run(a.Info.Name, func(t *testing.T) {
			open, err := a.Adapter.Open(0, a.Capabilities.Limits)
			if err != nil {
				t.Fatalf("Open(%s): %v", a.Info.Name, err)
			}
			// Registered first so it runs after the resource cleanups.
			t.Cleanup(open.Device.Destroy)
			fn(t, dx12TestDevice{name: a.Info.Name, device: open.Device, queue: open.Queue})
		})
	}
}

// openWARPDevice opens a device on WARP, for tests whose result does not
// depend on the adapter.
func openWARPDevice(t *testing.T) (hal.Device, hal.Queue) {
	t.Helper()
	adapters := enumerateDX12TestAdapters(t)
	a := &adapters[requireWARP(t, adapters)]
	open, err := a.Adapter.Open(0, a.Capabilities.Limits)
	if err != nil {
		t.Fatalf("Open(WARP): %v", err)
	}
	t.Cleanup(open.Device.Destroy)
	return open.Device, open.Queue
}

// enumerateDX12TestAdapters returns the adapters of a new DX12 instance; the
// instance and the adapters are released when the test ends.
func enumerateDX12TestAdapters(t *testing.T) []hal.ExposedAdapter {
	t.Helper()
	raw, err := Backend{}.CreateInstance(&hal.InstanceDescriptor{Backends: gputypes.BackendsDX12})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	t.Cleanup(raw.Destroy)
	adapters := raw.(*Instance).EnumerateAdapters(nil)
	t.Cleanup(func() {
		for i := range adapters {
			adapters[i].Adapter.Destroy()
		}
	})
	return adapters
}

// requireWARP returns the index of WARP, the adapter of DeviceTypeCPU, in
// adapters. It fails the test when WARP is missing: Windows 10 and later
// always ship it, and EnumerateAdapters lists it.
func requireWARP(t *testing.T, adapters []hal.ExposedAdapter) int {
	t.Helper()
	for i := range adapters {
		if adapters[i].Info.DeviceType == gputypes.DeviceTypeCPU {
			return i
		}
	}
	t.Fatal("EnumerateAdapters did not list WARP")
	return -1
}
