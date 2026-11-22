//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package instance

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
)

// enumFakeAdapter is a HAL adapter that carries no state: enumeration and
// selection never call it.
type enumFakeAdapter struct{ hal.Adapter }

func (enumFakeAdapter) Destroy() {}

// enumFakeInstance returns a fixed adapter list.
type enumFakeInstance struct {
	adapters []hal.ExposedAdapter
}

func (f *enumFakeInstance) CreateSurface(hal.SurfaceTarget) (hal.Surface, error) {
	return nil, hal.ErrUnsupportedSurfaceTarget
}

func (f *enumFakeInstance) EnumerateAdapters(hal.Surface) []hal.ExposedAdapter { return f.adapters }

func (f *enumFakeInstance) Destroy() {}

// enumFakeProvider registers a fixed fake instance under one backend. A
// provider with no instance is inert: it exists only to displace this
// package's fake after the tests, because the provider registry has no delete.
type enumFakeProvider struct {
	inst hal.Instance
}

func (p *enumFakeProvider) Variant() gputypes.Backend { return gputypes.BackendGL }

func (p *enumFakeProvider) CreateInstance(*hal.InstanceDescriptor) (hal.Instance, error) {
	if p.inst == nil {
		return nil, errEnumProviderInert
	}
	return p.inst, nil
}

func (p *enumFakeProvider) IsAvailable() bool { return p.inst != nil }

var errEnumProviderInert = errors.New("instance: enumeration test provider is inert")

func enumExposed(name string, deviceType gputypes.DeviceType) hal.ExposedAdapter {
	return hal.ExposedAdapter{
		Adapter:      enumFakeAdapter{},
		Info:         gputypes.AdapterInfo{Name: name, Backend: gputypes.BackendGL, DeviceType: deviceType},
		Capabilities: hal.Capabilities{Limits: gputypes.DefaultLimits()},
	}
}

// newEnumInstance registers the fake provider (the package's other tests build
// no real instances) and returns the implementation over a GL-only core
// instance. The provider is restored on cleanup, or displaced by an inert one
// when there was none to restore: the registry has no delete, and a leftover
// serving test adapters would poison every later test in this binary.
func newEnumInstance(t *testing.T, adapters ...hal.ExposedAdapter) *Instance {
	t.Helper()
	core.GetGlobal().Clear()
	t.Cleanup(core.GetGlobal().Clear)

	previous, hadPrevious := core.GetBackendProvider(gputypes.BackendGL)
	core.RegisterBackendProvider(&enumFakeProvider{inst: &enumFakeInstance{adapters: adapters}})
	t.Cleanup(func() {
		if hadPrevious {
			core.RegisterBackendProvider(previous)
			return
		}
		core.RegisterBackendProvider(&enumFakeProvider{})
	})

	inst := New(core.NewInstance(&gputypes.InstanceDescriptor{Backends: gputypes.BackendsGL}))
	t.Cleanup(inst.Release)
	return inst
}

// TestEnumerateAdaptersHidesNonOpenablePlaceholder pins P1-API-040 R1 through
// the implementation layer: the GLES placeholder that exists for selection
// compatibility only is never enumerated, while the openable adapter beside it
// is. The GL backend enumerates lazily, so the request that the application
// would make anyway comes first.
func TestEnumerateAdaptersHidesNonOpenablePlaceholder(t *testing.T) {
	placeholder := enumExposed("OpenGL Adapter", gputypes.DeviceTypeOther)
	placeholder.Placeholder = true
	inst := newEnumInstance(t, placeholder, enumExposed("GL Hardware", gputypes.DeviceTypeDiscreteGPU))

	if adapters := inst.EnumerateAdapters(); len(adapters) != 0 {
		t.Fatalf("EnumerateAdapters before the first request = %d adapters, want 0 (the GL backend defers)", len(adapters))
	}
	if _, err := inst.RequestAdapter(nil, nil); err != nil {
		t.Fatalf("RequestAdapter: %v", err)
	}

	adapters := inst.EnumerateAdapters()
	if len(adapters) != 1 {
		t.Fatalf("EnumerateAdapters = %d adapters, want 1: the placeholder must be hidden (R1)", len(adapters))
	}
	if got := adapters[0].Info().Name; got != "GL Hardware" {
		t.Fatalf("enumerated %q, want the openable adapter", got)
	}
}

// TestEnumerateAdaptersKeepsSelectionUntouched pins the other half of R1: the
// placeholder stays selectable, so a surface-less GLES request keeps failing
// at Open with its descriptive error instead of reporting no adapter at all.
func TestEnumerateAdaptersKeepsSelectionUntouched(t *testing.T) {
	placeholder := enumExposed("OpenGL Adapter", gputypes.DeviceTypeOther)
	placeholder.Placeholder = true
	inst := newEnumInstance(t, placeholder)

	if _, err := inst.RequestAdapter(nil, nil); err != nil {
		t.Fatalf("RequestAdapter: %v (selection must keep seeing the placeholder)", err)
	}
	if adapters := inst.EnumerateAdapters(); len(adapters) != 0 {
		t.Fatalf("EnumerateAdapters = %d adapters, want 0", len(adapters))
	}
}

// TestEnumerateAdaptersOnReleasedInstance pins the released-instance answer:
// no adapters, no panic.
func TestEnumerateAdaptersOnReleasedInstance(t *testing.T) {
	inst := newEnumInstance(t, enumExposed("GL Hardware", gputypes.DeviceTypeDiscreteGPU))
	inst.Release()
	if adapters := inst.EnumerateAdapters(); adapters != nil {
		t.Fatalf("EnumerateAdapters after Release = %d adapters, want nil", len(adapters))
	}
}

// TestEnumerateAdaptersRegistrationOrder pins the documented order and its
// difference from selection (P1-API-040 R2): the list follows the backend's
// enumeration order, while RequestAdapter ranks the same candidates by power
// preference and picks the discrete adapter even though it is registered
// second.
func TestEnumerateAdaptersRegistrationOrder(t *testing.T) {
	inst := newEnumInstance(t,
		enumExposed("CPU Adapter", gputypes.DeviceTypeCPU),
		enumExposed("Discrete Adapter", gputypes.DeviceTypeDiscreteGPU),
	)

	// The first request runs the backend's one-shot enumeration.
	if _, err := inst.RequestAdapter(nil, nil); err != nil {
		t.Fatalf("RequestAdapter: %v", err)
	}

	adapters := inst.EnumerateAdapters()
	if len(adapters) != 2 {
		t.Fatalf("EnumerateAdapters = %d adapters, want 2", len(adapters))
	}
	if got := adapters[0].Info().Name; got != "CPU Adapter" {
		t.Fatalf("adapters[0] = %q, want the first-registered adapter: the list is registration order, not the ranked pick", got)
	}

	picked, err := inst.RequestAdapter(nil, nil)
	if err != nil {
		t.Fatalf("RequestAdapter: %v", err)
	}
	if got := picked.Info().Name; got != "Discrete Adapter" {
		t.Fatalf("RequestAdapter picked %q, want the ranked discrete adapter (the docs say the orders can differ)", got)
	}
}

// enumIdentity is a representative non-zero identity: a 16-byte UUID, a PCI
// slot, a LUID and a Metal registry ID, so a dropped or zeroed field is visible.
func enumIdentity() gputypes.AdapterIdentity {
	return gputypes.AdapterIdentity{
		DeviceUUID:      gputypes.DeviceUUID{0x28, 0x8b, 0xbc, 0x7b, 0x41, 0xe0, 0x9c, 0xf2, 0x8f, 0xf0, 0x59, 0x18, 0x50, 0x9a, 0x02, 0x40},
		PCIBusID:        gputypes.PCIBusID{Domain: 0, Bus: 4, Device: 0, Function: 0},
		AdapterLUID:     0xd5e26d2b,
		MetalRegistryID: 0x1234,
	}
}

// TestAdapterIdentityZeroAfterRelease pins the released and nil answers even
// when the adapter carries a non-zero identity: the guard, not the data, is
// what a caller sees after Release.
func TestAdapterIdentityZeroAfterRelease(t *testing.T) {
	want := enumIdentity()
	a := &Adapter{identity: want}
	if got := a.Identity(); got != want {
		t.Fatalf("live adapter identity = %+v, want %+v", got, want)
	}
	a.released = true
	if got := a.Identity(); !got.IsZero() {
		t.Fatalf("released adapter identity = %+v, want the zero value", got)
	}
	var nilAdapter *Adapter
	if got := nilAdapter.Identity(); !got.IsZero() {
		t.Fatalf("nil adapter identity = %+v, want the zero value", got)
	}
}

// TestAdapterIdentityPathParity pins ADR-084's RequestAdapter path parity: the
// adapter a RequestAdapter call returns and its enumerated counterpart read the
// same core record, so a set identity surfaces unchanged through both.
func TestAdapterIdentityPathParity(t *testing.T) {
	want := enumIdentity()
	exposed := enumExposed("Discrete With Identity", gputypes.DeviceTypeDiscreteGPU)
	exposed.Identity = want
	inst := newEnumInstance(t, exposed)

	// The first request runs the backend's one-shot enumeration.
	picked, err := inst.RequestAdapter(nil, nil)
	if err != nil {
		t.Fatalf("RequestAdapter: %v", err)
	}
	if got := picked.Identity(); got != want {
		t.Fatalf("RequestAdapter identity = %+v, want %+v", got, want)
	}

	adapters := inst.EnumerateAdapters()
	if len(adapters) != 1 {
		t.Fatalf("EnumerateAdapters = %d adapters, want 1", len(adapters))
	}
	if got := adapters[0].Identity(); got != want {
		t.Fatalf("EnumerateAdapters identity = %+v, want the same record as RequestAdapter (%+v)", got, want)
	}
}

// TestAdapterIdentityZeroPath pins the zero path: an adapter whose backend
// reports no identity answers IsZero through both paths, with no error
// (ADR-084 §2.2: all-zero = unknown, never an error).
func TestAdapterIdentityZeroPath(t *testing.T) {
	inst := newEnumInstance(t, enumExposed("GL Hardware", gputypes.DeviceTypeDiscreteGPU))

	picked, err := inst.RequestAdapter(nil, nil)
	if err != nil {
		t.Fatalf("RequestAdapter: %v", err)
	}
	if !picked.Identity().IsZero() {
		t.Fatalf("RequestAdapter identity = %+v, want zero", picked.Identity())
	}

	adapters := inst.EnumerateAdapters()
	if len(adapters) != 1 {
		t.Fatalf("EnumerateAdapters = %d adapters, want 1", len(adapters))
	}
	if !adapters[0].Identity().IsZero() {
		t.Fatalf("EnumerateAdapters identity = %+v, want zero", adapters[0].Identity())
	}
}
