// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// Adapter fixtures for the selection policy. WARP is the DX12 software
// adapter (DXGI_ADAPTER_FLAG_SOFTWARE, DeviceTypeCPU); "software" is our CPU
// backend (Backend Empty, DeviceTypeCPU).
var (
	selDiscrete   = gputypes.AdapterInfo{Name: "GTX 1660 SUPER", DeviceType: gputypes.DeviceTypeDiscreteGPU, Backend: gputypes.BackendDX12}
	selIntegrated = gputypes.AdapterInfo{Name: "Iris Xe", DeviceType: gputypes.DeviceTypeIntegratedGPU, Backend: gputypes.BackendDX12}
	selWARP       = gputypes.AdapterInfo{Name: "Microsoft Basic Render Driver", DeviceType: gputypes.DeviceTypeCPU, Backend: gputypes.BackendDX12}
	selLavapipe   = gputypes.AdapterInfo{Name: "llvmpipe", DeviceType: gputypes.DeviceTypeCPU, Backend: gputypes.BackendVulkan}
	selSoftware   = gputypes.AdapterInfo{Name: "Software Renderer", DeviceType: gputypes.DeviceTypeCPU, Backend: gputypes.BackendEmpty}
)

func TestSelectAdapterIDsOrder(t *testing.T) {
	none := (*gputypes.RequestAdapterOptions)(nil)
	auto := &gputypes.RequestAdapterOptions{}
	high := &gputypes.RequestAdapterOptions{PowerPreference: gputypes.PowerPreferenceHighPerformance}
	low := &gputypes.RequestAdapterOptions{PowerPreference: gputypes.PowerPreferenceLowPower}
	fallback := &gputypes.RequestAdapterOptions{ForceFallbackAdapter: true}

	for _, tc := range []struct {
		name     string
		options  *gputypes.RequestAdapterOptions
		adapters []gputypes.AdapterInfo
		want     string
	}{
		// Auto selection: hardware > WARP (or another driver's CPU adapter) > software.
		{"nil options: hardware first", none, []gputypes.AdapterInfo{selSoftware, selWARP, selIntegrated}, selIntegrated.Name},
		{"nil options: WARP before software", none, []gputypes.AdapterInfo{selSoftware, selWARP}, selWARP.Name},
		{"nil options: software alone", none, []gputypes.AdapterInfo{selSoftware}, selSoftware.Name},
		{"auto: hardware first", auto, []gputypes.AdapterInfo{selSoftware, selWARP, selDiscrete}, selDiscrete.Name},
		{"auto: WARP before software", auto, []gputypes.AdapterInfo{selSoftware, selWARP}, selWARP.Name},
		{"auto: CPU ties keep list order", auto, []gputypes.AdapterInfo{selSoftware, selLavapipe, selWARP}, selLavapipe.Name},
		{"high performance: discrete", high, []gputypes.AdapterInfo{selSoftware, selWARP, selIntegrated, selDiscrete}, selDiscrete.Name},
		{"high performance: any GPU before WARP", high, []gputypes.AdapterInfo{selSoftware, selWARP, selIntegrated}, selIntegrated.Name},
		{"low power: WARP before software", low, []gputypes.AdapterInfo{selSoftware, selWARP}, selWARP.Name},

		// ForceFallbackAdapter means the software backend whenever it is a
		// candidate; without it, the first other CPU adapter (WARP).
		{"fallback: software over WARP", fallback, []gputypes.AdapterInfo{selDiscrete, selWARP, selSoftware}, selSoftware.Name},
		{"fallback: software over lavapipe", fallback, []gputypes.AdapterInfo{selLavapipe, selSoftware}, selSoftware.Name},
		{"fallback: WARP without software", fallback, []gputypes.AdapterInfo{selDiscrete, selIntegrated, selWARP}, selWARP.Name},
	} {
		t.Run(tc.name, func(t *testing.T) {
			GetGlobal().Clear()
			hub := GetGlobal().Hub()
			ids := make([]AdapterID, 0, len(tc.adapters))
			for _, info := range tc.adapters {
				ids = append(ids, hub.RegisterAdapter(&Adapter{Info: info, Backend: info.Backend, Limits: gputypes.DefaultLimits()}))
			}
			id, err := selectAdapterIDs(tc.options, ids)
			if err != nil {
				t.Fatalf("selectAdapterIDs: %v", err)
			}
			adapter, err := hub.GetAdapter(id)
			if err != nil {
				t.Fatalf("GetAdapter: %v", err)
			}
			if adapter.Info.Name != tc.want {
				t.Errorf("selected %q (%v, %v), want %q", adapter.Info.Name, adapter.Info.DeviceType, adapter.Info.Backend, tc.want)
			}
		})
	}
}

func TestSelectAdapterIDsFallbackWithoutCPU(t *testing.T) {
	GetGlobal().Clear()
	hub := GetGlobal().Hub()
	ids := []AdapterID{hub.RegisterAdapter(&Adapter{Info: selDiscrete, Backend: selDiscrete.Backend})}
	if _, err := selectAdapterIDs(&gputypes.RequestAdapterOptions{ForceFallbackAdapter: true}, ids); err == nil {
		t.Fatal("ForceFallbackAdapter with no CPU adapter: want an error")
	}
}
