//go:build !rust && !(js && wasm)

package wgpu

// This file is the public declaration layer over internal/instance on the
// native build (ADR-070 S12). The implementation moved to internal/instance;
// what stays here is the frozen public Adapter type, the descriptor boundary and
// the root instance identity cache the surface cross-check reads.

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/instance"
)

// DeviceDescriptor configures device creation.
type DeviceDescriptor struct {
	Label            string
	RequiredFeatures gputypes.Features
	RequiredLimits   gputypes.Limits
}

// Adapter represents a physical GPU.
//
// The implementation lives in internal/instance (ADR-070 S12); this type is the
// declaration layer the public API is frozen on. It keeps the root instance the
// adapter was requested from, written once at construction: the surface
// cross-check compares it to reject a device/surface pair from two instances,
// and the device wrapper records it as its identity cache.
type Adapter struct {
	impl     *instance.Adapter
	instance *Instance
}

// Info returns adapter metadata.
func (a *Adapter) Info() gputypes.AdapterInfo { return a.impl.Info() }

// Features returns supported features.
func (a *Adapter) Features() gputypes.Features { return a.impl.Features() }

// Limits returns the adapter's resource limits.
func (a *Adapter) Limits() gputypes.Limits { return a.impl.Limits() }

// DownlevelCapabilities returns backend capability flags for downlevel adapters.
// Matches Rust wgpu Adapter::get_downlevel_capabilities() (adapter.rs:174).
func (a *Adapter) DownlevelCapabilities() gputypes.DownlevelCapabilities {
	return a.impl.DownlevelCapabilities()
}

// RequestDevice creates a logical device from this adapter.
// If desc is nil, default features and limits are used.
func (a *Adapter) RequestDevice(desc *DeviceDescriptor) (*Device, error) {
	if a == nil {
		return nil, ErrReleased
	}

	// The descriptor boundary (shape 1): the public descriptor becomes the
	// implementation's mirror, and a nil pointer stays nil.
	var implDesc *instance.DeviceDescriptor
	if desc != nil {
		implDesc = &instance.DeviceDescriptor{
			Label:            desc.Label,
			RequiredFeatures: desc.RequiredFeatures,
			RequiredLimits:   desc.RequiredLimits,
		}
	}

	impl, q, err := a.impl.RequestDevice(implDesc)
	if err != nil {
		return nil, err
	}

	dev := &Device{impl: impl, instance: a.instance}
	if q != nil {
		dev.queue = &Queue{impl: q}
	}
	return dev, nil
}

// SurfaceCapabilities describes what a surface supports on this adapter.
type SurfaceCapabilities struct {
	// Formats lists the supported surface texture formats.
	Formats []gputypes.TextureFormat

	// PresentModes lists the supported presentation modes.
	PresentModes []gputypes.PresentMode

	// AlphaModes lists the supported composite alpha modes.
	AlphaModes []gputypes.CompositeAlphaMode
}

// GetSurfaceCapabilities returns the capabilities of a surface for this adapter.
// Returns nil if the adapter has no HAL (core-only path) or the surface is nil.
func (a *Adapter) GetSurfaceCapabilities(surface *Surface) *SurfaceCapabilities {
	// The cross-instance rejection is a root fact: the instance a surface is
	// registered with is this package's type.
	if a == nil || a.instance == nil || a.instance.isReleased() ||
		surface == nil || surface.impl.Released() || surface.instance != a.instance {
		return nil
	}

	caps := a.impl.GetSurfaceCapabilities(surface.impl)
	if caps == nil {
		return nil
	}
	return &SurfaceCapabilities{
		Formats:      caps.Formats,
		PresentModes: caps.PresentModes,
		AlphaModes:   caps.AlphaModes,
	}
}

// Release releases the adapter.
func (a *Adapter) Release() {
	if a == nil {
		return
	}
	a.impl.Release()
}
