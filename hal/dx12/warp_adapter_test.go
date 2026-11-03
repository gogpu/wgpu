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
// DX12 adapter and on WARP, the software D3D12 device that ships with every
// Windows (d3d10warp.dll). WARP executes command lists on many CPU threads
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
// and on WARP, once per shader path (HLSL through FXC, and naga DXIL).
func forEachDX12TestDevice(t *testing.T, fn func(t *testing.T, d dx12TestDevice)) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping GPU integration test in short mode")
	}
	for _, path := range []struct{ name, dxil string }{{"FXC", ""}, {"DXIL", "1"}} {
		t.Run(path.name, func(t *testing.T) {
			t.Setenv("GOGPU_DX12_DXIL", path.dxil)

			raw, err := Backend{}.CreateInstance(&hal.InstanceDescriptor{Backends: gputypes.BackendsDX12})
			if err != nil {
				t.Skipf("DX12 unavailable: %v", err)
			}
			t.Cleanup(raw.Destroy)
			instance := raw.(*Instance)

			adapters := instance.EnumerateAdapters(nil)
			adapters = append(adapters, openWARPAdapter(t, instance))
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
		})
	}
}

// openWARPAdapter returns the WARP adapter of the instance, which
// EnumerateAdapters leaves out. It fails the test when WARP is missing:
// Windows 10 and later always ship it.
func openWARPAdapter(t *testing.T, instance *Instance) hal.ExposedAdapter {
	t.Helper()
	raw, err := instance.factory.EnumWarpAdapter()
	if err != nil {
		t.Fatalf("EnumWarpAdapter: %v", err)
	}
	desc, err := raw.GetDesc1()
	if err != nil {
		raw.Release()
		t.Fatalf("WARP GetDesc1: %v", err)
	}
	adapter := &Adapter{raw: raw, desc: desc, instance: instance}
	if err := adapter.probeCapabilities(); err != nil {
		raw.Release()
		t.Fatalf("WARP probeCapabilities: %v", err)
	}
	t.Cleanup(adapter.Destroy)
	return adapter.toExposedAdapter()
}
