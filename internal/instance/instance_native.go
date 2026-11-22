//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package instance

// This file is the native implementation of the instance domain (ADR-070 S12),
// moved from the root package's instance_native.go. The root keeps the public
// Instance declaration layer over it; this type owns the state and the release
// order: adopted devices first, then adopted surfaces, then the core instance
// (instance_native.go:238-254 pre-move).

import (
	"fmt"
	"sync"

	"github.com/gogpu/gputypes"

	publichal "github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/surface"
)

// Instance is the native implementation of the GPU instance. The declaration
// layer wraps it and delegates every public method.
type Instance struct {
	core     *core.Instance
	mu       sync.Mutex
	released bool
	devices  map[core.DeviceView]deviceRef
	surfaces map[*surface.Surface]struct{}
}

// Create builds the instance over the descriptor and display the declaration
// layer converted: the root validates the public Display handle and converts it
// at the boundary, and this constructor owns the core instance. A nil
// descriptor is the public nil, which core resolves to its defaults.
func Create(gpuDesc *gputypes.InstanceDescriptor, display hal.DisplayHandle) *Instance {
	return New(core.NewInstanceWithDisplay(gpuDesc, display))
}

// New wraps an already-built core instance. The declaration layer's surface
// glue and the root tests reach the core instance through Core().
func New(coreInstance *core.Instance) *Instance {
	return &Instance{core: coreInstance}
}

// Core returns the core instance the declaration layer's surface glue records
// surfaces against, or nil for a zero implementation.
func (i *Instance) Core() *core.Instance {
	if i == nil {
		return nil
	}
	return i.core
}

// IsReleased reports whether the instance has been released. A nil receiver
// answers true: it is the state the public nil pointer stands for.
func (i *Instance) IsReleased() bool {
	if i == nil {
		return true
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.released
}

// RequestAdapter requests an adapter for the options the declaration layer
// converted (nil is the public nil, preserved so core sees the same value) and
// the compatible surface implementation, or nil. When a compatible surface is
// given, the surface-aware core path triggers deferred GLES enumeration, as the
// public method did.
func (i *Instance) RequestAdapter(opts *gputypes.RequestAdapterOptions, compatible *surface.Surface) (*Adapter, error) {
	if i.IsReleased() {
		return nil, core.ErrReleased
	}

	var adapterID core.AdapterID
	var err error
	if compatible != nil {
		adapterID, err = i.core.RequestAdapterWithSurfaces(opts, compatible.HALSurfacesForAdapterRequest())
	} else {
		adapterID, err = i.core.RequestAdapter(opts)
	}
	if err != nil {
		return nil, err
	}

	adapter, err := i.wrapAdapter(adapterID)
	if err != nil {
		// A request-local surface adapter is not kept by a failed wrap; the
		// cached adapters this releases nothing for (ReleaseSurfaceAdapter
		// only scans its own list).
		i.core.ReleaseSurfaceAdapter(adapterID)
		return nil, err
	}

	hal.Logger().Info("wgpu: adapter selected",
		"name", adapter.info.Name,
		"backend", adapter.info.Backend,
		"type", adapter.info.DeviceType,
	)

	return adapter, nil
}

// wrapAdapter builds the implementation adapter for a core adapter the
// instance has already registered: one metadata read per snapshot field and a
// copy of the core adapter, which owns the HAL handle the device open path
// needs.
func (i *Instance) wrapAdapter(adapterID core.AdapterID) (*Adapter, error) {
	info, err := core.GetAdapterInfo(adapterID)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to get adapter info: %w", err)
	}
	features, err := core.GetAdapterFeatures(adapterID)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to get adapter features: %w", err)
	}
	limits, err := core.GetAdapterLimits(adapterID)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to get adapter limits: %w", err)
	}

	hub := core.GetGlobal().Hub()
	coreAdapter, err := hub.GetAdapter(adapterID)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to get adapter: %w", err)
	}

	return &Adapter{
		id:          adapterID,
		core:        &coreAdapter,
		info:        info,
		features:    features,
		limits:      limits,
		downlevel:   coreAdapter.DownlevelCapabilities,
		placeholder: coreAdapter.Placeholder,
		instance:    i,
	}, nil
}

// EnumerateAdapters returns the adapters the instance knows, in registration
// order. Placeholders exist for selection compatibility only and cannot be
// opened (a GLES backend without an EGL context), so they are not enumerated:
// a caller never sees an adapter that cannot open (P1-API-040 R1).
//
// The method is read-only by contract (R4): it takes the core snapshot and
// wraps it, registering nothing, admitting no fallback adapters (that list is
// separate and never leaks here) and triggering no deferred GLES enumeration —
// the application's own surface-qualified RequestAdapter does that. An adapter
// whose registration vanished between the snapshot and the read drops out.
func (i *Instance) EnumerateAdapters() []*Adapter {
	if i == nil || i.core == nil || i.IsReleased() {
		return nil
	}

	ids := i.core.EnumerateAdapters()
	adapters := make([]*Adapter, 0, len(ids))
	for _, id := range ids {
		adapter, err := i.wrapAdapter(id)
		if err != nil || adapter.placeholder {
			continue
		}
		adapters = append(adapters, adapter)
	}
	return adapters
}

// AdoptDevice registers a device this instance owns. The adapter calls it once
// at the device's creation; Release releases every registered device before the
// core instance is destroyed. A device removes itself through UnregisterDevice
// when it releases on its own.
func (i *Instance) AdoptDevice(device deviceRef) error {
	if device == nil {
		return fmt.Errorf("wgpu: cannot adopt a nil device")
	}
	if i == nil {
		return core.ErrReleased
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.released {
		return core.ErrReleased
	}
	if i.devices == nil {
		i.devices = make(map[core.DeviceView]deviceRef)
	}
	i.devices[device] = device
	return nil
}

// UnregisterDevice removes the device behind view from the registry. The
// device's own release reports it, once, at the end of its teardown.
func (i *Instance) UnregisterDevice(view core.DeviceView) {
	if i == nil || view == nil {
		return
	}
	i.mu.Lock()
	delete(i.devices, view)
	i.mu.Unlock()
}

// AdoptSurface registers a surface this instance owns. The declaration layer's
// creation glue calls it once the root wrapper exists; Release releases every
// registered surface implementation after the devices.
func (i *Instance) AdoptSurface(s *surface.Surface) error {
	if s == nil {
		return fmt.Errorf("wgpu: cannot adopt a nil surface")
	}
	if i == nil {
		return core.ErrReleased
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.released {
		return core.ErrReleased
	}
	if i.surfaces == nil {
		i.surfaces = make(map[*surface.Surface]struct{})
	}
	i.surfaces[s] = struct{}{}
	return nil
}

// UnregisterSurface removes s from the surface registry. The root Surface
// declaration layer reports it when the caller releases a surface directly.
func (i *Instance) UnregisterSurface(s *surface.Surface) {
	if i == nil || s == nil {
		return
	}
	i.mu.Lock()
	delete(i.surfaces, s)
	i.mu.Unlock()
}

// SurfacesForDevice implements internal/device's InstanceHost: the surface
// implementations configured against view, as the device's teardown snapshot.
// It walks the registry under the instance lock and filters by the core device
// identity, exactly as the pre-move root host did.
func (i *Instance) SurfacesForDevice(view core.DeviceView) []*surface.Surface {
	if i == nil || view == nil {
		return nil
	}
	coreDevice := view.Core()
	i.mu.Lock()
	defer i.mu.Unlock()
	surfaces := make([]*surface.Surface, 0, len(i.surfaces))
	for s := range i.surfaces {
		if s != nil && s.SameDevice(coreDevice) {
			surfaces = append(surfaces, s)
		}
	}
	return surfaces
}

// beginRelease marks the instance released under the lock and returns the
// registries' contents. A second call, or a nil receiver, reports false.
func (i *Instance) beginRelease() ([]deviceRef, []*surface.Surface, bool) {
	if i == nil {
		return nil, nil, false
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.released {
		return nil, nil, false
	}
	i.released = true

	devices := make([]deviceRef, 0, len(i.devices))
	for _, device := range i.devices {
		devices = append(devices, device)
	}
	surfaces := make([]*surface.Surface, 0, len(i.surfaces))
	for s := range i.surfaces {
		surfaces = append(surfaces, s)
	}
	return devices, surfaces, true
}

// Release releases the instance and all associated resources.
//
// Devices own configured swapchains, surfaces own platform surface handles, and
// the instance owns the native instance; release in that order.
func (i *Instance) Release() {
	devices, surfaces, ok := i.beginRelease()
	if !ok {
		return
	}
	for _, device := range devices {
		device.Release()
	}
	for _, s := range surfaces {
		s.Release()
	}
	i.core.Destroy()
}

// HAL is the native-build body of the public Instance.HAL.
func (i *Instance) HAL() publichal.Handles {
	if i == nil || i.core == nil {
		return publichal.Handles{}
	}
	return i.core.HALHandles()
}
