//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package instance

// This file is the native implementation of the adapter domain (ADR-070 S12),
// moved from the root package's adapter_native.go. The root keeps the public
// Adapter declaration layer and the descriptor boundary: it converts the public
// descriptors to this package's scalars and handles, and wraps the device it
// gets back.

import (
	"fmt"

	"github.com/gogpu/gputypes"

	publichal "github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/device"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/pendingwrites"
	"github.com/gogpu/wgpu/internal/queue"
	"github.com/gogpu/wgpu/internal/surface"
)

// Adapter is the native implementation of a physical GPU. The declaration
// layer wraps it and keeps the public instance identity cache.
type Adapter struct {
	id        core.AdapterID
	core      *core.Adapter
	info      gputypes.AdapterInfo
	features  gputypes.Features
	limits    gputypes.Limits
	downlevel gputypes.DownlevelCapabilities
	// placeholder marks a selection-only entry that cannot be opened
	// (core.Adapter.Placeholder). RequestAdapter still returns such an adapter
	// so Open fails with its descriptive error; EnumerateAdapters drops it
	// (P1-API-040 R1).
	placeholder bool
	instance    *Instance
	released    bool
}

// Info returns adapter metadata.
func (a *Adapter) Info() gputypes.AdapterInfo {
	if a == nil {
		return gputypes.AdapterInfo{}
	}
	return a.info
}

// Features returns supported features.
func (a *Adapter) Features() gputypes.Features {
	if a == nil {
		return 0
	}
	return a.features
}

// Limits returns the adapter's resource limits.
func (a *Adapter) Limits() gputypes.Limits {
	if a == nil {
		return gputypes.Limits{}
	}
	return a.limits
}

// DownlevelCapabilities returns backend capability flags for downlevel adapters.
func (a *Adapter) DownlevelCapabilities() gputypes.DownlevelCapabilities {
	if a == nil {
		return gputypes.DownlevelCapabilities{}
	}
	return a.downlevel
}

// DeviceDescriptor is the implementation's mirror of the public descriptor: the
// declaration layer converts, and a nil pointer is the public nil, which the two
// request paths distinguish.
type DeviceDescriptor struct {
	Label            string
	RequiredFeatures gputypes.Features
	RequiredLimits   gputypes.Limits
}

// RequestDevice creates a logical device from this adapter and returns its
// implementation and queue (nil for the core-only path). The device is adopted
// by the instance and wired to it as its InstanceHost before it is returned, in
// the pre-move order.
func (a *Adapter) RequestDevice(desc *DeviceDescriptor) (*device.Device, *queue.Queue, error) {
	if a == nil || a.released || a.instance == nil || a.instance.IsReleased() {
		return nil, nil, core.ErrReleased
	}

	var (
		impl *device.Device
		q    *queue.Queue
		err  error
	)
	if a.core.HasHAL() {
		impl, q, err = a.requestDeviceHAL(desc)
	} else {
		impl, err = a.requestDeviceCore(desc)
	}
	if err != nil {
		return nil, nil, err
	}
	if err := a.instance.AdoptDevice(impl); err != nil {
		impl.Release()
		return nil, nil, err
	}
	impl.SetInstance(a.instance)
	return impl, q, nil
}

// requestDeviceHAL opens a HAL device and builds the device implementation,
// its shared encoder pool and its queue.
func (a *Adapter) requestDeviceHAL(desc *DeviceDescriptor) (*device.Device, *queue.Queue, error) {
	var features gputypes.Features
	var limits gputypes.Limits
	var label string

	if desc != nil {
		features = desc.RequiredFeatures
		limits = desc.RequiredLimits
		label = desc.Label
	}

	// If no limits specified (nil descriptor or zero-value RequiredLimits),
	// use the adapter's actual hardware limits. This matches the WebGPU spec:
	// "Each limit in the returned device will be no worse than the corresponding
	// limit in adapter.limits." Matches Rust wgpu which returns adapter limits
	// by default.
	if limits == (gputypes.Limits{}) {
		limits = a.limits
	}

	if err := core.ValidateDeviceRequest(features, limits, a.features, a.limits); err != nil {
		return nil, nil, err
	}

	openDevice, err := a.core.HALAdapter().Open(features, limits)
	if err != nil {
		return nil, nil, fmt.Errorf("wgpu: failed to open device: %w", err)
	}

	coreDevice := core.NewDevice(openDevice.Device, a.core, features, limits, label)

	// Single shared encoder pool for both user command encoders
	// (CreateCommandEncoder) and internal staging encoders (PendingWrites).
	// Matches Rust wgpu-core which uses one device.command_allocator for both
	// (queue.rs:1373).
	pool := encoder.NewPool(openDevice.Device)

	q := queue.New(openDevice.Queue, pendingwrites.New(openDevice.Device, openDevice.Queue, pool, coreDevice.Tracker()))

	coreDevice.SetAssociatedQueue(&core.Queue{Label: label + " Queue"})

	impl := device.New(coreDevice, q, pool)
	q.SetDevice(impl)

	return impl, q, nil
}

// requestDeviceCore builds the core-only device for an adapter without a HAL
// implementation; such a device has no queue, so the public wrapper carries
// none.
func (a *Adapter) requestDeviceCore(desc *DeviceDescriptor) (*device.Device, error) {
	var gpuDesc *gputypes.DeviceDescriptor
	if desc != nil {
		gpuDesc = &gputypes.DeviceDescriptor{
			Label:          desc.Label,
			RequiredLimits: desc.RequiredLimits,
		}
	}

	if _, err := core.RequestDevice(a.id, gpuDesc); err != nil {
		return nil, fmt.Errorf("wgpu: failed to create device: %w", err)
	}

	coreDevice := &core.Device{
		Label:    "",
		Features: 0,
		Limits:   gputypes.DefaultLimits(),
	}
	if desc != nil {
		coreDevice.Label = desc.Label
	}

	return device.New(coreDevice, nil, nil), nil
}

// SurfaceCapabilities mirrors the public capability answer: the declaration
// layer converts the slices into the public struct.
type SurfaceCapabilities struct {
	Formats      []gputypes.TextureFormat
	PresentModes []gputypes.PresentMode
	AlphaModes   []gputypes.CompositeAlphaMode
}

// GetSurfaceCapabilities returns the capabilities of a surface for this
// adapter. Returns nil if the adapter has no HAL (core-only path), or the
// surface is nil or released. The declaration layer has already rejected the
// cross-instance pair.
func (a *Adapter) GetSurfaceCapabilities(s *surface.Surface) *SurfaceCapabilities {
	if a == nil || a.released || a.instance == nil || a.instance.IsReleased() ||
		s == nil || s.Released() {
		return nil
	}

	if !a.core.HasHAL() {
		// Core-only path: return safe defaults (Fifo is guaranteed by Vulkan spec).
		return &SurfaceCapabilities{
			PresentModes: []gputypes.PresentMode{gputypes.PresentModeFifo},
		}
	}

	halSurface := s.HALSurfaceForBackend(a.info.Backend)
	if halSurface == nil {
		return nil
	}
	halCaps := a.core.HALAdapter().SurfaceCapabilities(halSurface)
	if halCaps == nil {
		return nil
	}

	return &SurfaceCapabilities{
		Formats:      halCaps.Formats,
		PresentModes: halCaps.PresentModes,
		AlphaModes:   halCaps.AlphaModes,
	}
}

// Release releases the adapter.
func (a *Adapter) Release() {
	if a == nil || a.released {
		return
	}
	a.released = true
	if a.instance != nil && a.instance.core != nil {
		a.instance.core.ReleaseSurfaceAdapter(a.id)
	}
}

// HAL is the native-build body of the public Adapter.HAL.
func (a *Adapter) HAL() publichal.Handles {
	// A cached adapter is instance-owned: Adapter.Release only marks the
	// implementation, and releasing the instance destroys the HAL object
	// underneath. Report nothing once either is gone, so a caller never receives
	// a stale native pointer.
	if a == nil || a.core == nil || a.released || (a.instance != nil && a.instance.IsReleased()) {
		return publichal.Handles{}
	}
	return a.core.HALHandles()
}
