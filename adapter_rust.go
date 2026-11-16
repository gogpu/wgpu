//go:build rust

package wgpu

// This file is the public declaration layer over internal/instance on the rust
// build (ADR-070 S12). The implementation moved to internal/instance; what stays
// here is the frozen public Adapter type, the descriptor boundary and the root
// instance identity cache the returned device records.

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
// On Rust backend, this wraps go-webgpu/webgpu Adapter.
//
// The implementation lives in internal/instance (ADR-070 S12); this type is the
// declaration layer the public API is frozen on. It keeps the root instance the
// adapter was requested from, written once at construction: the returned device
// records it as its identity cache.
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
// Rust wgpu handles real values internally; from Go FFI we return fully compliant defaults.
//
// A nil receiver answers them too: the pre-move method was a constant and
// ignored the receiver, and the declaration layer must not turn that into a
// panic (ADR-070 S12 settle).
func (a *Adapter) DownlevelCapabilities() gputypes.DownlevelCapabilities {
	if a == nil {
		return gputypes.DefaultDownlevelCapabilities()
	}
	return a.impl.DownlevelCapabilities()
}

// RequestDevice creates a logical device from this adapter.
// If desc is nil, default features and limits are used.
func (a *Adapter) RequestDevice(desc *DeviceDescriptor) (*Device, error) {
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
	return &Device{impl: impl, instance: a.instance, queue: &Queue{impl: q}}, nil
}

// SurfaceCapabilities describes what a surface supports on this adapter.
type SurfaceCapabilities struct {
	Formats      []gputypes.TextureFormat
	PresentModes []gputypes.PresentMode
	AlphaModes   []gputypes.CompositeAlphaMode
}

// GetSurfaceCapabilities returns the capabilities of a surface for this adapter.
//
// A nil receiver answers nil. Unlike DownlevelCapabilities, the pre-move method
// read its receiver (a.released) and panicked on a nil one, so this guard is a
// safe-direction delta, not a restored default (ADR-070 S12 settle, P2-CORE-034).
func (a *Adapter) GetSurfaceCapabilities(surface *Surface) *SurfaceCapabilities {
	if a == nil || surface == nil {
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
	a.impl.Release()
}
