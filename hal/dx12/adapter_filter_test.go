// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/dxgi"
)

func adapterDesc(name string, vendor, device uint32, flags dxgi.DXGI_ADAPTER_FLAG) dxgi.DXGI_ADAPTER_DESC1 {
	var d dxgi.DXGI_ADAPTER_DESC1
	copy(d.Description[:len(d.Description)-1], utf16.Encode([]rune(name)))
	d.VendorID = vendor
	d.DeviceID = device
	d.Flags = flags
	return d
}

// TestShouldKeepAdapter checks the enumeration filter against Rust wgpu-hal
// (wgpu-hal/src/auxil/dxgi/factory.rs, should_keep_adapter).
func TestShouldKeepAdapter(t *testing.T) {
	for _, tc := range []struct {
		name string
		desc dxgi.DXGI_ADAPTER_DESC1
		keep bool
	}{
		{
			name: "flagged WARP",
			desc: adapterDesc(warpAdapterName, warpVendorID, 0x8c, dxgi.DXGI_ADAPTER_FLAG_SOFTWARE),
			keep: true,
		},
		{
			// The unflagged twin Windows exposes on GPU-less machines.
			name: "unflagged Basic Render Driver",
			desc: adapterDesc(warpAdapterName, warpVendorID, 0x8c, 0),
			keep: false,
		},
		{
			// Other Microsoft adapters without the flag stay (only the name
			// identifies the disguised WARP).
			name: "unflagged other Microsoft adapter",
			desc: adapterDesc("Microsoft Remote Display Adapter", warpVendorID, 0x1, 0),
			keep: true,
		},
		{
			name: "NVIDIA GTX 1660 SUPER",
			desc: adapterDesc("NVIDIA GeForce GTX 1660 SUPER", 0x10de, 0x21c4, 0),
			keep: true,
		},
		{
			name: "Intel Iris Xe",
			desc: adapterDesc("Intel(R) Iris(R) Xe Graphics", 0x8086, 0x9a49, 0),
			keep: true,
		},
		{
			// D3D12 support was removed from Haswell iGPUs for a security
			// vulnerability; Rust filters the whole family.
			name: "Intel Haswell HD 4600",
			desc: adapterDesc("Intel(R) HD Graphics 4600", 0x8086, 0x0412, 0),
			keep: false,
		},
		{
			name: "Haswell device ID from another vendor",
			desc: adapterDesc("Other", 0x10de, 0x0412, 0),
			keep: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldKeepAdapter(&tc.desc); got != tc.keep {
				t.Errorf("shouldKeepAdapter(%q, vendor 0x%04X, flags %d) = %v, want %v",
					tc.desc.DescriptionString(), tc.desc.VendorID, tc.desc.Flags, got, tc.keep)
			}
		})
	}
}

// TestDeviceTypeWARPIsCPU checks that the flagged WARP reports DeviceTypeCPU
// in both adapter paths, as Rust wgpu-hal's dx12/adapter.rs does.
func TestDeviceTypeWARPIsCPU(t *testing.T) {
	desc := adapterDesc(warpAdapterName, warpVendorID, 0x8c, dxgi.DXGI_ADAPTER_FLAG_SOFTWARE)
	if got := (&Adapter{desc: desc}).deviceType(); got != gputypes.DeviceTypeCPU {
		t.Errorf("Adapter.deviceType(WARP) = %v, want CPU", got)
	}
	if got := (&AdapterLegacy{desc: desc}).deviceType(); got != gputypes.DeviceTypeCPU {
		t.Errorf("AdapterLegacy.deviceType(WARP) = %v, want CPU", got)
	}
}

// TestEnumerateAdaptersExposesWARP enumerates the real DXGI adapters. Every
// Windows 10+ ships WARP (d3d10warp.dll), so it must be listed exactly once,
// as a DX12 adapter of type CPU, after every hardware adapter, and the
// unflagged "Microsoft Basic Render Driver" must not be listed.
func TestEnumerateAdaptersExposesWARP(t *testing.T) {
	raw, err := Backend{}.CreateInstance(&hal.InstanceDescriptor{Backends: gputypes.BackendsDX12})
	if err != nil {
		t.Skipf("CreateInstance: %v", err)
	}
	defer raw.Destroy()
	adapters := raw.(*Instance).EnumerateAdapters(nil)
	defer func() {
		for i := range adapters {
			adapters[i].Adapter.Destroy()
		}
	}()

	warp := 0
	for i := range adapters {
		info := adapters[i].Info
		t.Logf("adapter %d: %q (%v, %v, vendor 0x%04X)", i, info.Name, info.DeviceType, info.Backend, info.VendorID)
		if info.Backend != gputypes.BackendDX12 {
			t.Errorf("adapter %q: Backend = %v, want DX12", info.Name, info.Backend)
		}
		if !strings.Contains(info.Name, warpAdapterName) {
			if warp > 0 {
				t.Errorf("hardware adapter %q listed after WARP", info.Name)
			}
			continue
		}
		warp++
		if info.DeviceType != gputypes.DeviceTypeCPU {
			t.Errorf("%q: DeviceType = %v, want CPU (the unflagged twin must be dropped)", info.Name, info.DeviceType)
		}
	}
	if warp != 1 {
		t.Fatalf("WARP listed %d times, want 1", warp)
	}
}
