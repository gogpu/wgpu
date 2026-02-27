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
	if a.released {
		return nil, ErrReleased
	}

	if a.core.HasHAL() {
		return a.requestDeviceHAL(desc)
	}

	return a.requestDeviceCore(desc)
}

func (a *Adapter) requestDeviceHAL(desc *DeviceDescriptor) (*Device, error) {
	var features gputypes.Features
	var limits gputypes.Limits
	var label string

	if desc != nil {
		features = desc.RequiredFeatures
		limits = desc.RequiredLimits
		label = desc.Label
	} else {
		limits = gputypes.DefaultLimits()
	}

	openDevice, err := a.core.HALAdapter().Open(features, limits)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to open device: %w", err)
	}

	coreDevice := core.NewDevice(openDevice.Device, a.core, features, limits, label)

	fence, err := openDevice.Device.CreateFence()
	if err != nil {
		coreDevice.Destroy()
		return nil, fmt.Errorf("wgpu: failed to create fence: %w", err)
	}

	queue := &Queue{
		hal:       openDevice.Queue,
		halDevice: openDevice.Device,
		fence:     fence,
	}

	coreDevice.SetAssociatedQueue(&core.Queue{Label: label + " Queue"})

	device := &Device{
		core:  coreDevice,
		queue: queue,
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

// Release releases the adapter.
func (a *Adapter) Release() {
	if a.released {
		return
	}
	a.released = true
}
