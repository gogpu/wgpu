// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
)

// TestInstanceIdentityCopiedAtCreationRegistration pins the creation-site copy
// of the adapter identity (ADR-084): a backend that is not deferred registers
// through registerExposedAdapters, and the identity it reports must survive the
// copy into the core record the public Identity() reads.
func TestInstanceIdentityCopiedAtCreationRegistration(t *testing.T) {
	GetGlobal().Clear()
	want := gputypes.AdapterIdentity{
		DeviceUUID:      gputypes.DeviceUUID{0x28, 0x8b, 0xbc, 0x7b, 0x41, 0xe0, 0x9c, 0xf2, 0x8f, 0xf0, 0x59, 0x18, 0x50, 0x9a, 0x02, 0x40},
		PCIBusID:        gputypes.PCIBusID{Domain: 0, Bus: 4, Device: 0, Function: 0},
		AdapterLUID:     0x2b6de2d5,
		MetalRegistryID: 0,
	}
	exposed := fakeExposed("Vulkan With Identity", gputypes.BackendVulkan, gputypes.DeviceTypeDiscreteGPU)
	exposed.Identity = want
	useProviders(t, &testProvider{
		variant:   gputypes.BackendVulkan,
		available: true,
		instance:  &fakeHALInstance{adapters: []hal.ExposedAdapter{exposed}},
	})

	instance := NewInstance(&gputypes.InstanceDescriptor{Backends: gputypes.BackendsVulkan})
	t.Cleanup(instance.Destroy)

	ids := instance.EnumerateAdapters()
	if len(ids) != 1 {
		t.Fatalf("adapters = %d, want 1 (creation registration)", len(ids))
	}
	adapter, err := GetGlobal().Hub().GetAdapter(ids[0])
	if err != nil {
		t.Fatalf("GetAdapter: %v", err)
	}
	if adapter.Identity != want {
		t.Fatalf("core identity = %+v, want %+v (registerExposedAdapters lost the identity)", adapter.Identity, want)
	}
}
