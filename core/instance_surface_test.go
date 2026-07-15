//go:build !(js && wasm)

package core

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

type surfaceQualificationAdapter struct {
	compatible bool
	qualifies  *int
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

func (a *surfaceQualificationAdapter) Destroy() {}

func (a *surfaceQualificationAdapter) QualifySurface(_ hal.Surface) (hal.Adapter, error) {
	if a.qualifies != nil {
		(*a.qualifies)++
	}
	if !a.compatible {
		return nil, errors.New("surface is not supported")
	}
	return &surfaceQualificationAdapter{compatible: true}, nil
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
