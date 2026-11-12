// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// fakeHALAdapter is a HAL adapter that carries only its info. Adapter
// enumeration and selection never call it; Destroy is a no-op.
type fakeHALAdapter struct{ hal.Adapter }

func (fakeHALAdapter) Destroy() {}

// fakeHALInstance returns a fixed adapter list.
type fakeHALInstance struct {
	adapters []hal.ExposedAdapter
}

func (i *fakeHALInstance) CreateSurface(hal.SurfaceTarget) (hal.Surface, error) {
	return nil, hal.ErrUnsupportedSurfaceTarget
}

func (i *fakeHALInstance) EnumerateAdapters(hal.Surface) []hal.ExposedAdapter { return i.adapters }

func (i *fakeHALInstance) Destroy() {}

func fakeExposed(name string, backend gputypes.Backend, deviceType gputypes.DeviceType) hal.ExposedAdapter {
	return hal.ExposedAdapter{
		Adapter:      fakeHALAdapter{},
		Info:         gputypes.AdapterInfo{Name: name, Backend: backend, DeviceType: deviceType},
		Capabilities: hal.Capabilities{Limits: gputypes.DefaultLimits()},
	}
}

// useProviders replaces the registered backend providers for one test and
// restores them afterwards. RegisterHALBackends runs first so its one-shot
// registration cannot add real backends to the replaced map.
func useProviders(t *testing.T, ps ...BackendProvider) {
	t.Helper()
	RegisterHALBackends()
	providersMu.Lock()
	saved := providers
	replacement := make(map[gputypes.Backend]BackendProvider, len(ps))
	for _, p := range ps {
		replacement[p.Variant()] = p
	}
	providers = replacement
	providersMu.Unlock()
	t.Cleanup(func() {
		providersMu.Lock()
		providers = saved
		providersMu.Unlock()
		GetGlobal().Clear()
	})
}

// TestInstanceRequestedBackendWithoutAdapter is the Phase 0 contract: a
// backend registered in the binary but without an adapter on the machine
// produces an error, never another backend's adapter (ADR-083 §2.3).
func TestInstanceRequestedBackendWithoutAdapter(t *testing.T) {
	GetGlobal().Clear()
	useProviders(t, &testProvider{
		variant:   gputypes.BackendVulkan,
		available: true,
		instance:  &fakeHALInstance{},
	})

	instance := NewInstance(&gputypes.InstanceDescriptor{Backends: gputypes.BackendsVulkan})
	t.Cleanup(instance.Destroy)

	if adapters := instance.EnumerateAdapters(); len(adapters) != 0 {
		t.Fatalf("adapters = %d, want 0", len(adapters))
	}
	if _, err := instance.RequestAdapter(nil); err == nil {
		t.Fatal("RequestAdapter: want an error when the requested backend has no adapter")
	}
}

// TestInstanceMaskExcludesOtherRegisteredBackends pins the mask contract: a
// Vulkan-only request never falls through to a registered DX12 CPU adapter.
func TestInstanceMaskExcludesOtherRegisteredBackends(t *testing.T) {
	GetGlobal().Clear()
	useProviders(t,
		&testProvider{variant: gputypes.BackendVulkan, available: true, instance: &fakeHALInstance{}},
		&testProvider{
			variant:   gputypes.BackendDX12,
			available: true,
			instance: &fakeHALInstance{adapters: []hal.ExposedAdapter{
				fakeExposed("Microsoft Basic Render Driver", gputypes.BackendDX12, gputypes.DeviceTypeCPU),
			}},
		},
	)

	instance := NewInstance(&gputypes.InstanceDescriptor{Backends: gputypes.BackendsVulkan})
	t.Cleanup(instance.Destroy)

	if _, err := instance.RequestAdapter(nil); err == nil {
		t.Fatal("RequestAdapter: want an error; the DX12 adapter is outside the Vulkan mask")
	}
}

// TestInstanceHardwarePickAcrossRegisteredBackends checks the ranking across
// registered hardware backends: a default request picks the Vulkan hardware
// adapter over a DX12 CPU adapter.
func TestInstanceHardwarePickAcrossRegisteredBackends(t *testing.T) {
	GetGlobal().Clear()
	useProviders(t,
		&testProvider{
			variant:   gputypes.BackendVulkan,
			available: true,
			instance: &fakeHALInstance{adapters: []hal.ExposedAdapter{
				fakeExposed("Iris Xe", gputypes.BackendVulkan, gputypes.DeviceTypeIntegratedGPU),
			}},
		},
		&testProvider{
			variant:   gputypes.BackendDX12,
			available: true,
			instance: &fakeHALInstance{adapters: []hal.ExposedAdapter{
				fakeExposed("Microsoft Basic Render Driver", gputypes.BackendDX12, gputypes.DeviceTypeCPU),
			}},
		},
	)

	instance := NewInstance(&gputypes.InstanceDescriptor{Backends: gputypes.BackendsPrimary})
	t.Cleanup(instance.Destroy)

	id, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("RequestAdapter: %v", err)
	}
	adapter, err := GetGlobal().Hub().GetAdapter(id)
	if err != nil {
		t.Fatalf("GetAdapter: %v", err)
	}
	if adapter.Info.Backend != gputypes.BackendVulkan {
		t.Errorf("pick = %q (%v), want the Vulkan hardware adapter", adapter.Info.Name, adapter.Info.Backend)
	}
}

// TestInstanceExplicitSoftwareRegistration documents the Phase 0 behavior for
// a program that imports the software backend itself: the pre-v0.35.0 mask
// bypass admits it (ADR-083 §2.3). v0.35.0 removes the bypass; this test must
// then expect the same error as TestInstanceRequestedBackendWithoutAdapter.
func TestInstanceExplicitSoftwareRegistration(t *testing.T) {
	GetGlobal().Clear()
	useProviders(t, &testProvider{
		variant:   gputypes.BackendEmpty,
		available: true,
		instance: &fakeHALInstance{adapters: []hal.ExposedAdapter{
			fakeExposed("Software Renderer", gputypes.BackendEmpty, gputypes.DeviceTypeCPU),
		}},
	})

	instance := NewInstance(&gputypes.InstanceDescriptor{Backends: gputypes.BackendsVulkan})
	t.Cleanup(instance.Destroy)

	id, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("RequestAdapter: %v", err)
	}
	adapter, err := GetGlobal().Hub().GetAdapter(id)
	if err != nil {
		t.Fatalf("GetAdapter: %v", err)
	}
	if adapter.Info.Backend != gputypes.BackendEmpty {
		t.Errorf("pick = %q (%v), want the explicitly registered software adapter", adapter.Info.Name, adapter.Info.Backend)
	}
}
