//go:build js && wasm

package wgpu

// This file is the public declaration layer over internal/instance on the
// browser build (ADR-070 S12). The implementation moved to internal/instance;
// what stays here is the frozen public Adapter type, the descriptor boundary and
// the conversion of the static browser capability answer.

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/instance"
	surfacepkg "github.com/gogpu/wgpu/internal/surface"
)

// DeviceDescriptor configures device creation.
type DeviceDescriptor struct {
	Label            string
	RequiredFeatures gputypes.Features
	RequiredLimits   gputypes.Limits
}

// Adapter represents a physical GPU.
// On browser, this wraps a GPUAdapter via internal/browser.Adapter.
//
// The implementation lives in internal/instance (ADR-070 S12); this type is the
// declaration layer the public API is frozen on.
//
// released is the public zero-value contract: a wrapper without an
// implementation cannot record its release in it, and the browser build's zero
// value answers its HAL kind until it is released.
type Adapter struct {
	impl     *instance.Adapter
	released bool
}

// Info returns adapter metadata.
func (a *Adapter) Info() gputypes.AdapterInfo { return a.impl.Info() }

// Features returns supported features.
func (a *Adapter) Features() gputypes.Features { return a.impl.Features() }

// Limits returns the adapter's resource limits.
func (a *Adapter) Limits() gputypes.Limits { return a.impl.Limits() }

// DownlevelCapabilities returns backend capability flags for downlevel adapters.
// Browsers are fully WebGPU compliant, so this returns the default (full compliance).
func (a *Adapter) DownlevelCapabilities() gputypes.DownlevelCapabilities {
	return a.impl.DownlevelCapabilities()
}

// RequestDevice creates a logical device from this adapter.
// If desc is nil, default features and limits are used.
func (a *Adapter) RequestDevice(desc *DeviceDescriptor) (*Device, error) {
	if a.released {
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
	return &Device{impl: impl, queue: &Queue{impl: q}}, nil
}

// SurfaceCapabilities describes what a surface supports on this adapter.
type SurfaceCapabilities struct {
	Formats      []gputypes.TextureFormat
	PresentModes []gputypes.PresentMode
	AlphaModes   []gputypes.CompositeAlphaMode
}

// GetSurfaceCapabilities returns the capabilities of a surface for this adapter.
//
// On browser, capabilities are statically defined per the WebGPU spec:
//   - Formats: rgba8unorm, bgra8unorm, rgba16float (preferred format first)
//   - PresentModes: Fifo only (browser controls VSync)
//   - AlphaModes: Opaque only (browser default)
//
// The preferred format is obtained from navigator.gpu.getPreferredCanvasFormat()
// and placed first in the formats list.
//
// Matches Rust wgpu SurfaceInterface::get_capabilities for WebSurface.
func (a *Adapter) GetSurfaceCapabilities(surface *Surface) *SurfaceCapabilities {
	// The answer does not depend on the adapter, so a nil or zero-value
	// adapter reports it too, as the build always did.
	var impl *instance.Adapter
	if a != nil {
		impl = a.impl
	}
	var s *surfacepkg.Surface
	if surface != nil {
		s = surface.impl
	}

	caps := impl.GetSurfaceCapabilities(s)
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
	a.released = true
	a.impl.Release()
}
