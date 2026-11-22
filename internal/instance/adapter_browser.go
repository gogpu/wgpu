//go:build js && wasm

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package instance

// This file is the browser implementation of the adapter domain (ADR-070 S12),
// moved from the root package's adapter_browser.go. The root keeps the public
// Adapter declaration layer, converts the descriptors and wraps the device
// implementation this returns.

import (
	"syscall/js"

	"github.com/gogpu/gputypes"

	publichal "github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/browser"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/device"
	"github.com/gogpu/wgpu/internal/queue"
	"github.com/gogpu/wgpu/internal/surface"
)

// Adapter is the browser-build implementation of a physical GPU.
type Adapter struct {
	browser  *browser.Adapter
	info     gputypes.AdapterInfo
	features gputypes.Features
	limits   gputypes.Limits
	released bool
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
// Browsers are fully WebGPU compliant, so this returns the default (full
// compliance).
func (a *Adapter) DownlevelCapabilities() gputypes.DownlevelCapabilities {
	return gputypes.DefaultDownlevelCapabilities()
}

// Identity returns the adapter's hardware identity. The browser reports none:
// WebGPU defines no per-device identifier (ADR-084 §2.3), so every field is zero
// for a nil or live adapter alike; it is never an error.
func (a *Adapter) Identity() gputypes.AdapterIdentity {
	return gputypes.AdapterIdentity{}
}

// DeviceDescriptor is the implementation's mirror of the public descriptor: the
// declaration layer converts, and a nil pointer is the public nil.
type DeviceDescriptor struct {
	Label            string
	RequiredFeatures gputypes.Features
	RequiredLimits   gputypes.Limits
}

// RequestDevice creates a logical device from this adapter and returns its
// implementation and queue.
func (a *Adapter) RequestDevice(desc *DeviceDescriptor) (*device.Device, *queue.Queue, error) {
	if a == nil || a.released {
		return nil, nil, core.ErrReleased
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
		return nil, nil, err
	}

	// Extract the device's features and limits.
	deviceFeatures := browser.ExtractFeatures(bd.Features())
	deviceLimits := browser.ExtractLimits(bd.Limits())

	return device.NewBrowser(bd, deviceFeatures, deviceLimits), queue.New(bd.Queue()), nil
}

// SurfaceCapabilities mirrors the public capability answer.
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
// and placed first in the formats list. The answer does not depend on the
// adapter, so a nil receiver (the declaration layer's zero value) reports it too.
func (a *Adapter) GetSurfaceCapabilities(s *surface.Surface) *SurfaceCapabilities {
	// Browser WebGPU supports these three formats per spec:
	// https://gpuweb.github.io/gpuweb/#supported-context-formats
	formats := []gputypes.TextureFormat{
		gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureFormatBGRA8Unorm,
		gputypes.TextureFormatRGBA16Float,
	}

	// Put the preferred format first (Rust wgpu does the same swap).
	if s != nil {
		if bs := s.Browser(); bs != nil {
			preferredStr := bs.GetPreferredCanvasFormat()
			preferredFmt := browser.TextureFormatFromJS(preferredStr)
			for i, f := range formats {
				if f == preferredFmt {
					formats[0], formats[i] = formats[i], formats[0]
					break
				}
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
	if a == nil || a.released {
		return
	}
	a.released = true
}

// HAL is the browser-build body of the public Adapter.HAL. A nil receiver is
// the declaration layer's zero value, which reports the browser kind.
func (a *Adapter) HAL() publichal.Handles {
	if a != nil && a.released {
		return publichal.Handles{}
	}
	return publichal.Handles{Kind: publichal.KindBrowser}
}
