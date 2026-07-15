//go:build !(js && wasm)

package core

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

type surfaceQualificationAdapter struct {
	compatible bool
	qualifies  *int
	destroys   *int
}

func (a *surfaceQualificationAdapter) Open(_ gputypes.Features, _ gputypes.Limits) (hal.OpenDevice, error) {
	return hal.OpenDevice{}, nil
}

func (a *surfaceQualificationAdapter) TextureFormatCapabilities(_ gputypes.TextureFormat) hal.TextureFormatCapabilities {
	return hal.TextureFormatCapabilities{}
}

func (a *surfaceQualificationAdapter) SurfaceCapabilities(_ hal.Surface) *hal.SurfaceCapabilities {
	return nil
}

func (a *surfaceQualificationAdapter) Destroy() {
	if a.destroys != nil {
		(*a.destroys)++
	}
}

func (a *surfaceQualificationAdapter) QualifySurface(_ hal.Surface) (hal.Adapter, error) {
	if a.qualifies != nil {
		(*a.qualifies)++
	}
	if !a.compatible {
		return nil, errors.New("surface is not supported")
	}
	return &surfaceQualificationAdapter{compatible: true, destroys: a.destroys}, nil
}

func TestRequestAdapterWithSurfaceReleasesUnselectedQualifiedAdapters(t *testing.T) {
	GetGlobal().Clear()
	hub := GetGlobal().Hub()

	destroys := 0
	firstHAL := &surfaceQualificationAdapter{compatible: true, destroys: &destroys}
	secondHAL := &surfaceQualificationAdapter{compatible: true, destroys: &destroys}
	firstID := hub.RegisterAdapter(&Adapter{
		Info:       gputypes.AdapterInfo{DeviceType: gputypes.DeviceTypeDiscreteGPU, Backend: gputypes.BackendVulkan},
		Limits:     gputypes.DefaultLimits(),
		Backend:    gputypes.BackendVulkan,
		halAdapter: firstHAL,
	})
	secondID := hub.RegisterAdapter(&Adapter{
		Info:       gputypes.AdapterInfo{DeviceType: gputypes.DeviceTypeIntegratedGPU, Backend: gputypes.BackendVulkan},
		Limits:     gputypes.DefaultLimits(),
		Backend:    gputypes.BackendVulkan,
		halAdapter: secondHAL,
	})

	instance := &Instance{
		backends: gputypes.BackendsVulkan,
		adapters: []AdapterID{firstID, secondID},
	}
	selectedID, err := instance.RequestAdapterWithSurface(nil, &stubHALSurface{id: 9})
	if err != nil {
		t.Fatalf("RequestAdapterWithSurface() error: %v", err)
	}
	if len(instance.surfaceAdapters) != 1 {
		t.Fatalf("surface adapter ownership = %d, want 1", len(instance.surfaceAdapters))
	}
	if selectedID != instance.surfaceAdapters[0] {
		t.Fatalf("selected ID = %v, tracked surface adapter = %v", selectedID, instance.surfaceAdapters[0])
	}
	if destroys != 1 {
		t.Fatalf("qualified adapter destroys after selection = %d, want 1", destroys)
	}

	instance.ReleaseSurfaceAdapter(selectedID)
	if destroys != 2 {
		t.Fatalf("qualified adapter destroys after selected release = %d, want 2", destroys)
	}
	instance.Destroy()
}

func TestRequestAdapterWithSurfaceReleasesQualifiedAdaptersOnSelectionError(t *testing.T) {
	GetGlobal().Clear()
	hub := GetGlobal().Hub()

	destroys := 0
	firstID := hub.RegisterAdapter(&Adapter{
		Info:       gputypes.AdapterInfo{DeviceType: gputypes.DeviceTypeDiscreteGPU, Backend: gputypes.BackendVulkan},
		Limits:     gputypes.DefaultLimits(),
		Backend:    gputypes.BackendVulkan,
		halAdapter: &surfaceQualificationAdapter{compatible: true, destroys: &destroys},
	})
	secondID := hub.RegisterAdapter(&Adapter{
		Info:       gputypes.AdapterInfo{DeviceType: gputypes.DeviceTypeIntegratedGPU, Backend: gputypes.BackendVulkan},
		Limits:     gputypes.DefaultLimits(),
		Backend:    gputypes.BackendVulkan,
		halAdapter: &surfaceQualificationAdapter{compatible: true, destroys: &destroys},
	})

	instance := &Instance{
		backends: gputypes.BackendsVulkan,
		adapters: []AdapterID{firstID, secondID},
	}
	_, err := instance.RequestAdapterWithSurface(&gputypes.RequestAdapterOptions{ForceFallbackAdapter: true}, &stubHALSurface{id: 10})
	if err == nil {
		t.Fatal("RequestAdapterWithSurface() unexpectedly selected a non-fallback adapter")
	}
	if len(instance.surfaceAdapters) != 0 {
		t.Fatalf("surface adapter ownership after selection error = %d, want 0", len(instance.surfaceAdapters))
	}
	if destroys != 2 {
		t.Fatalf("qualified adapter destroys after selection error = %d, want 2", destroys)
	}
	instance.Destroy()
}

func TestRequestAdapterWithSurfaceUsesRequestLocalQualification(t *testing.T) {
	GetGlobal().Clear()
	hub := GetGlobal().Hub()

	firstCalls := 0
	secondCalls := 0
	firstHAL := &surfaceQualificationAdapter{qualifies: &firstCalls}
	secondHAL := &surfaceQualificationAdapter{compatible: true, qualifies: &secondCalls}
	firstID := hub.RegisterAdapter(&Adapter{
		Info:       gputypes.AdapterInfo{DeviceType: gputypes.DeviceTypeDiscreteGPU, Backend: gputypes.BackendVulkan},
		Limits:     gputypes.DefaultLimits(),
		Backend:    gputypes.BackendVulkan,
		halAdapter: firstHAL,
	})
	secondID := hub.RegisterAdapter(&Adapter{
		Info:       gputypes.AdapterInfo{DeviceType: gputypes.DeviceTypeIntegratedGPU, Backend: gputypes.BackendVulkan},
		Limits:     gputypes.DefaultLimits(),
		Backend:    gputypes.BackendVulkan,
		halAdapter: secondHAL,
	})

	instance := &Instance{
		backends: gputypes.BackendsVulkan,
		adapters: []AdapterID{firstID, secondID},
	}
	defer instance.Destroy()

	selectedID, err := instance.RequestAdapterWithSurface(nil, &stubHALSurface{id: 7})
	if err != nil {
		t.Fatalf("RequestAdapterWithSurface() error: %v", err)
	}
	if selectedID == firstID || selectedID == secondID {
		t.Fatalf("selected cached adapter %v; want request-local qualified adapter", selectedID)
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("qualification calls = (%d, %d), want (1, 1)", firstCalls, secondCalls)
	}

	selected, err := hub.GetAdapter(selectedID)
	if err != nil {
		t.Fatalf("GetAdapter(selected) error: %v", err)
	}
	if selected.halAdapter == secondHAL {
		t.Fatal("selected adapter retained cached HAL adapter")
	}
	if selected.Info.DeviceType != gputypes.DeviceTypeIntegratedGPU {
		t.Fatalf("selected device type = %v, want integrated GPU", selected.Info.DeviceType)
	}
	if len(instance.adapters) != 2 || len(instance.surfaceAdapters) != 1 {
		t.Fatalf("adapter ownership = (%d ordinary, %d surface), want (2, 1)", len(instance.adapters), len(instance.surfaceAdapters))
	}

	ordinaryID, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("ordinary RequestAdapter() error: %v", err)
	}
	if ordinaryID != firstID {
		t.Fatalf("ordinary RequestAdapter() = %v, want cached first adapter %v", ordinaryID, firstID)
	}

	selectedAgain, err := instance.RequestAdapterWithSurface(nil, &stubHALSurface{id: 8})
	if err != nil {
		t.Fatalf("second RequestAdapterWithSurface() error: %v", err)
	}
	if selectedAgain == firstID || selectedAgain == secondID {
		t.Fatal("second surface request returned a cached adapter")
	}
	if len(instance.adapters) != 2 || len(instance.surfaceAdapters) != 2 {
		t.Fatalf("adapter ownership after second request = (%d ordinary, %d surface), want (2, 2)", len(instance.adapters), len(instance.surfaceAdapters))
	}
	if firstCalls != 2 || secondCalls != 2 {
		t.Fatalf("qualification calls after second request = (%d, %d), want (2, 2)", firstCalls, secondCalls)
	}

	// The cached adapter remains unchanged and is still available for a
	// surface-independent request.
	cached, err := hub.GetAdapter(secondID)
	if err != nil {
		t.Fatalf("GetAdapter(cached) error: %v", err)
	}
	if cached.halAdapter != secondHAL {
		t.Fatal("surface request mutated cached adapter")
	}

	instance.ReleaseSurfaceAdapter(selectedID)
	instance.ReleaseSurfaceAdapter(selectedAgain)
	if len(instance.surfaceAdapters) != 0 {
		t.Fatalf("surface adapter ownership after release = %d, want 0", len(instance.surfaceAdapters))
	}
	if _, err := hub.GetAdapter(selectedID); err == nil {
		t.Fatal("first released surface adapter remains registered")
	}
	if _, err := hub.GetAdapter(selectedAgain); err == nil {
		t.Fatal("second released surface adapter remains registered")
	}
}

func TestRequestAdapterWithSurfacePreservesExplicitMock(t *testing.T) {
	GetGlobal().Clear()
	instance := NewInstanceWithMock(nil)
	defer instance.Destroy()

	ordinary := instance.EnumerateAdapters()
	if len(ordinary) != 1 {
		t.Fatalf("ordinary adapters = %d, want 1", len(ordinary))
	}
	selected, err := instance.RequestAdapterWithSurface(nil, &stubHALSurface{id: 11})
	if err != nil {
		t.Fatalf("RequestAdapterWithSurface() error: %v", err)
	}
	if selected != ordinary[0] {
		t.Fatalf("selected ID = %v, want explicit mock ID %v", selected, ordinary[0])
	}
	if len(instance.surfaceAdapters) != 0 {
		t.Fatalf("explicit mock created %d request-local adapters, want 0", len(instance.surfaceAdapters))
	}
}

func TestRequestAdapterWithSurfaceNilHintUsesOrdinaryPath(t *testing.T) {
	GetGlobal().Clear()
	instance := NewInstanceWithMock(nil)
	defer instance.Destroy()

	ordinary, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("RequestAdapter() error: %v", err)
	}
	selected, err := instance.RequestAdapterWithSurface(nil, nil)
	if err != nil {
		t.Fatalf("RequestAdapterWithSurface(nil) error: %v", err)
	}
	if selected != ordinary {
		t.Fatalf("surface-independent selection = %v, want ordinary adapter %v", selected, ordinary)
	}
	if len(instance.surfaceAdapters) != 0 {
		t.Fatalf("nil surface hint created %d request-local adapters, want 0", len(instance.surfaceAdapters))
	}
}

func TestRequestAdapterWithSurfaceRejectsIncompatibleCachedAdapter(t *testing.T) {
	GetGlobal().Clear()
	hub := GetGlobal().Hub()
	id := hub.RegisterAdapter(&Adapter{
		Info:       gputypes.AdapterInfo{DeviceType: gputypes.DeviceTypeDiscreteGPU, Backend: gputypes.BackendVulkan},
		Limits:     gputypes.DefaultLimits(),
		Backend:    gputypes.BackendVulkan,
		halAdapter: &surfaceQualificationAdapter{},
	})
	instance := &Instance{backends: gputypes.BackendsVulkan, adapters: []AdapterID{id}}
	defer instance.Destroy()

	if _, err := instance.RequestAdapterWithSurface(nil, &stubHALSurface{id: 12}); err == nil {
		t.Fatal("RequestAdapterWithSurface() accepted an incompatible cached adapter")
	}
	if len(instance.surfaceAdapters) != 0 {
		t.Fatalf("incompatible request retained %d request-local adapters, want 0", len(instance.surfaceAdapters))
	}
}

func TestRequestAdapterWithSurfaceUsesCheckedCapabilityFallback(t *testing.T) {
	GetGlobal().Clear()
	hub := GetGlobal().Hub()
	id := hub.RegisterAdapter(&Adapter{
		Info:       gputypes.AdapterInfo{DeviceType: gputypes.DeviceTypeDiscreteGPU, Backend: gputypes.BackendEmpty},
		Limits:     gputypes.DefaultLimits(),
		Backend:    gputypes.BackendEmpty,
		halAdapter: &noop.Adapter{},
	})
	instance := &Instance{adapters: []AdapterID{id}}
	defer instance.Destroy()

	selected, err := instance.RequestAdapterWithSurface(nil, &stubHALSurface{id: 13})
	if err != nil {
		t.Fatalf("RequestAdapterWithSurface() error: %v", err)
	}
	if selected != id {
		t.Fatalf("selected ID = %v, want checked cached ID %v", selected, id)
	}
	if len(instance.surfaceAdapters) != 0 {
		t.Fatalf("capability fallback created %d request-local adapters, want 0", len(instance.surfaceAdapters))
	}
}

func TestReleaseSurfaceAdapterIgnoresUnownedIDs(t *testing.T) {
	var nilInstance *Instance
	nilInstance.ReleaseSurfaceAdapter(AdapterID{})

	GetGlobal().Clear()
	hub := GetGlobal().Hub()
	id := hub.RegisterAdapter(&Adapter{
		Info:    gputypes.AdapterInfo{DeviceType: gputypes.DeviceTypeDiscreteGPU},
		Limits:  gputypes.DefaultLimits(),
		Backend: gputypes.BackendVulkan,
	})
	instance := &Instance{adapters: []AdapterID{id}}
	defer instance.Destroy()

	instance.ReleaseSurfaceAdapter(id)
	if _, err := hub.GetAdapter(id); err != nil {
		t.Fatalf("ordinary adapter was released: %v", err)
	}
	instance.ReleaseSurfaceAdapter(AdapterID{})
}

func TestInstanceDestroyReleasesOwnedSurfaceAdapters(t *testing.T) {
	GetGlobal().Clear()
	hub := GetGlobal().Hub()
	destroys := 0
	cachedID := hub.RegisterAdapter(&Adapter{
		Info:       gputypes.AdapterInfo{DeviceType: gputypes.DeviceTypeDiscreteGPU, Backend: gputypes.BackendVulkan},
		Limits:     gputypes.DefaultLimits(),
		Backend:    gputypes.BackendVulkan,
		halAdapter: &surfaceQualificationAdapter{compatible: true, destroys: &destroys},
	})
	instance := &Instance{backends: gputypes.BackendsVulkan, adapters: []AdapterID{cachedID}}

	selected, err := instance.RequestAdapterWithSurface(nil, &stubHALSurface{id: 14})
	if err != nil {
		t.Fatalf("RequestAdapterWithSurface() error: %v", err)
	}
	instance.Destroy()

	if destroys != 2 {
		t.Fatalf("adapter destroys = %d, want cached and request-local adapters", destroys)
	}
	if _, err := hub.GetAdapter(selected); err == nil {
		t.Fatal("destroyed instance left its request-local adapter registered")
	}
}
