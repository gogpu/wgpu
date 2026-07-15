//go:build !rust && !(js && wasm)

package wgpu

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
)

// DeviceDescriptor configures device creation.
type DeviceDescriptor struct {
	Label            string
	RequiredFeatures Features
	RequiredLimits   Limits
}

// Adapter represents a physical GPU.
type Adapter struct {
	id       core.AdapterID
	core     *core.Adapter
	info     AdapterInfo
	features Features
	limits   Limits
	instance *Instance
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
	if a == nil || a.released || a.instance == nil || a.instance.isReleased() {
		return nil, ErrReleased
	}

	var (
		device *Device
		err    error
	)
	if a.core.HasHAL() {
		device, err = a.requestDeviceHAL(desc)
	} else {
		device, err = a.requestDeviceCore(desc)
	}
	if err != nil {
		return nil, err
	}
	if err := a.instance.adoptDevice(device); err != nil {
		device.Release()
		return nil, err
	}
	return device, nil
}

func (a *Adapter) requestDeviceHAL(desc *DeviceDescriptor) (*Device, error) {
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
	// limit in adapter.limits." When user doesn't specify limits, device gets
	// full hardware capabilities (e.g., Intel Iris Xe reports 200 storage buffers,
	// not the WebGPU minimum of 8).
	// Matches Rust wgpu which returns adapter limits by default.
	if limits == (gputypes.Limits{}) {
		limits = a.limits
	}

	openDevice, err := a.core.HALAdapter().Open(features, limits)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to open device: %w", err)
	}

	coreDevice := core.NewDevice(openDevice.Device, a.core, features, limits, label)

	// Single shared encoder pool for both user command encoders (CreateCommandEncoder)
	// and internal staging encoders (PendingWrites). Matches Rust wgpu-core which uses
	// one device.command_allocator for both (queue.rs:1373).
	pool := newEncoderPool(openDevice.Device)

	queue := &Queue{
		hal:       openDevice.Queue,
		halDevice: openDevice.Device,
		pending:   newPendingWrites(openDevice.Device, openDevice.Queue, pool),
	}

	coreDevice.SetAssociatedQueue(&core.Queue{Label: label + " Queue"})

	device := &Device{
		core:           coreDevice,
		queue:          queue,
		cmdEncoderPool: pool,
	}
	queue.device = device

	return device, nil
}

func (a *Adapter) requestDeviceCore(desc *DeviceDescriptor) (*Device, error) {
	var gpuDesc *gputypes.DeviceDescriptor
	if desc != nil {
		gpuDesc = &gputypes.DeviceDescriptor{
			Label:          desc.Label,
			RequiredLimits: desc.RequiredLimits,
		}
	}

	_, err := core.RequestDevice(a.id, gpuDesc)
	if err != nil {
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

	return &Device{core: coreDevice}, nil
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
	if a == nil || a.released || a.instance == nil || a.instance.isReleased() ||
		surface == nil || surface.released || surface.instance != a.instance {
		return nil
	}

	if !a.core.HasHAL() {
		// Core-only path: return safe defaults (Fifo is guaranteed by Vulkan spec).
		return &SurfaceCapabilities{
			PresentModes: []gputypes.PresentMode{gputypes.PresentModeFifo},
		}
	}

	halSurface := surface.HAL()
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
