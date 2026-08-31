//go:build js && wasm

package wgpu

import (
	"syscall/js"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/browser"
)

// DeviceDescriptor configures device creation.
type DeviceDescriptor struct {
	Label            string
	RequiredFeatures gputypes.Features
	RequiredLimits   gputypes.Limits
}

// Adapter represents a physical GPU.
// On browser, this wraps a GPUAdapter via internal/browser.Adapter.
type Adapter struct {
	browser  *browser.Adapter
	info     gputypes.AdapterInfo
	features gputypes.Features
	limits   gputypes.Limits
	released bool
}

// Info returns adapter metadata.
func (a *Adapter) Info() gputypes.AdapterInfo { return a.info }

// Features returns supported features.
func (a *Adapter) Features() gputypes.Features { return a.features }

// Limits returns the adapter's resource limits.
func (a *Adapter) Limits() gputypes.Limits { return a.limits }

// DownlevelCapabilities returns backend capability flags for downlevel adapters.
// Browsers are fully WebGPU compliant, so this returns the default (full compliance).
func (a *Adapter) DownlevelCapabilities() gputypes.DownlevelCapabilities {
	return gputypes.DefaultDownlevelCapabilities()
}

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
	// Browser WebGPU supports these three formats per spec:
	// https://gpuweb.github.io/gpuweb/#supported-context-formats
	formats := []gputypes.TextureFormat{
		TextureFormatRGBA8Unorm,
		TextureFormatBGRA8Unorm,
		gputypes.TextureFormatRGBA16Float,
	}

	// Put the preferred format first (Rust wgpu does the same swap).
	if surface != nil && surface.browser != nil {
		preferredStr := surface.browser.GetPreferredCanvasFormat()
		preferredFmt := browser.TextureFormatFromJS(preferredStr)
		for i, f := range formats {
			if f == preferredFmt {
				formats[0], formats[i] = formats[i], formats[0]
				break
			}
		}
	}

	return &SurfaceCapabilities{
		Formats:      formats,
		PresentModes: []gputypes.PresentMode{gputypes.PresentModeFifo},
		AlphaModes:   []gputypes.CompositeAlphaMode{gputypes.CompositeAlphaModeOpaque},
	}
}

// Release releases the adapter.
func (a *Adapter) Release() {
	if a.released {
		return
	}
	a.released = true
}
