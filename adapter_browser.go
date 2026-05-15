//go:build js && wasm

package wgpu

import (
	"syscall/js"

	"github.com/gogpu/wgpu/internal/browser"
)

// DeviceDescriptor configures device creation.
type DeviceDescriptor struct {
	Label            string
	RequiredFeatures Features
	RequiredLimits   Limits
}

// Adapter represents a physical GPU.
// On browser, this wraps a GPUAdapter via internal/browser.Adapter.
type Adapter struct {
	browser  *browser.Adapter
	info     AdapterInfo
	features Features
	limits   Limits
	released bool
}

// Info returns adapter metadata.
func (a *Adapter) Info() AdapterInfo { return a.info }

// Features returns supported features.
func (a *Adapter) Features() Features { return a.features }

// Limits returns the adapter's resource limits.
func (a *Adapter) Limits() Limits { return a.limits }

// RequestDevice creates a logical device from this adapter.
// If desc is nil, default features and limits are used.
func (a *Adapter) RequestDevice(desc *DeviceDescriptor) (*Device, error) {
	if a.released {
		return nil, ErrReleased
	}

	// Build JS descriptor from Go types.
	var jsDesc js.Value
	if desc != nil {
		jsDesc = browser.BuildDeviceDescriptor(desc.Label, desc.RequiredFeatures, desc.RequiredLimits)
	} else {
		jsDesc = js.Undefined()
	}

	bd, err := a.browser.RequestDevice(jsDesc)
	if err != nil {
		return nil, err
	}

	// Extract the device's features and limits.
	deviceFeatures := browser.ExtractFeatures(bd.Features())
	deviceLimits := browser.ExtractLimits(bd.Limits())

	queue := &Queue{
		browser: bd.Queue(),
	}

	return &Device{
		browser:  bd,
		queue:    queue,
		features: deviceFeatures,
		limits:   deviceLimits,
	}, nil
}

// SurfaceCapabilities describes what a surface supports on this adapter.
type SurfaceCapabilities struct {
	Formats      []TextureFormat
	PresentModes []PresentMode
	AlphaModes   []CompositeAlphaMode
}

// GetSurfaceCapabilities returns the capabilities of a surface for this adapter.
// Phase 4 — not yet implemented for browser.
func (a *Adapter) GetSurfaceCapabilities(surface *Surface) *SurfaceCapabilities {
	panic("wgpu: browser GetSurfaceCapabilities not yet implemented (Phase 4)")
}

// Release releases the adapter.
func (a *Adapter) Release() {
	if a.released {
		return
	}
	a.released = true
}
