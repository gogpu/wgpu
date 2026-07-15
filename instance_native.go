//go:build !rust && !(js && wasm)

package wgpu

import (
	"fmt"
	"sync"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

// InstanceDescriptor configures instance creation.
type InstanceDescriptor struct {
	Backends Backends
	// Flags controls instance features like debug layers and validation.
	// Use gputypes.InstanceFlagsDebug to enable GPU debug layer.
	Flags gputypes.InstanceFlags
}

// Instance is the entry point for GPU operations.
//
// Instance methods are safe for concurrent use, except Release() which
// must not be called concurrently with other methods.
type Instance struct {
	core     *core.Instance
	mu       sync.Mutex
	released bool
	devices  map[*Device]struct{}
	surfaces map[*Surface]struct{}
}

// CreateInstance creates a new GPU instance.
// If desc is nil, all available backends are used.
func CreateInstance(desc *InstanceDescriptor) (*Instance, error) {
	var gpuDesc *gputypes.InstanceDescriptor
	if desc != nil {
		d := gputypes.DefaultInstanceDescriptor()
		d.Backends = desc.Backends
		d.Flags = desc.Flags
		gpuDesc = &d
	}

	coreInstance := core.NewInstance(gpuDesc)

	return &Instance{core: coreInstance}, nil
}

// RequestAdapter requests a GPU adapter matching the options.
// If opts is nil, the best available adapter is returned.
//
// When opts.CompatibleSurface is set, backends that require a surface for
// adapter enumeration (GLES/OpenGL) will perform deferred enumeration using
// the surface's GL context. This follows the WebGPU spec pattern where
// requestAdapter accepts a compatible surface hint.
func (i *Instance) RequestAdapter(opts *RequestAdapterOptions) (*Adapter, error) {
	if i.isReleased() {
		return nil, ErrReleased
	}

	// Convert wgpu-level options to gputypes for core.
	var coreOpts *gputypes.RequestAdapterOptions
	if opts != nil {
		coreOpts = &gputypes.RequestAdapterOptions{
			PowerPreference:      opts.PowerPreference,
			ForceFallbackAdapter: opts.ForceFallbackAdapter,
		}
	}

	// If a compatible surface is provided, use the surface-aware path
	// that triggers deferred GLES adapter enumeration.
	var adapterID core.AdapterID
	var err error
	if opts != nil && opts.CompatibleSurface != nil {
		halSurface := opts.CompatibleSurface.HAL()
		adapterID, err = i.core.RequestAdapterWithSurface(coreOpts, halSurface)
	} else {
		adapterID, err = i.core.RequestAdapter(coreOpts)
	}
	if err != nil {
		return nil, err
	}
	keepAdapter := false
	defer func() {
		if !keepAdapter {
			i.core.ReleaseSurfaceAdapter(adapterID)
		}
	}()

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

	hal.Logger().Info("wgpu: adapter selected",
		"name", info.Name,
		"backend", info.Backend,
		"type", info.DeviceType,
	)

	hub := core.GetGlobal().Hub()
	coreAdapter, err := hub.GetAdapter(adapterID)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to get adapter: %w", err)
	}

	adapter := &Adapter{
		id:       adapterID,
		core:     &coreAdapter,
		info:     info,
		features: features,
		limits:   limits,
		instance: i,
	}
	keepAdapter = true
	return adapter, nil
}

func (i *Instance) isReleased() bool {
	if i == nil {
		return true
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.released
}

func (i *Instance) adoptDevice(device *Device) error {
	if device == nil {
		return fmt.Errorf("wgpu: cannot adopt a nil device")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.released {
		return ErrReleased
	}
	if i.devices == nil {
		i.devices = make(map[*Device]struct{})
	}
	i.devices[device] = struct{}{}
	device.instance = i
	return nil
}

func (i *Instance) unregisterDevice(device *Device) {
	if i == nil || device == nil {
		return
	}
	i.mu.Lock()
	delete(i.devices, device)
	i.mu.Unlock()
}

func (i *Instance) adoptSurface(surface *Surface) error {
	if surface == nil {
		return fmt.Errorf("wgpu: cannot adopt a nil surface")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.released {
		return ErrReleased
	}
	if i.surfaces == nil {
		i.surfaces = make(map[*Surface]struct{})
	}
	i.surfaces[surface] = struct{}{}
	surface.instance = i
	return nil
}

func (i *Instance) unregisterSurface(surface *Surface) {
	if i == nil || surface == nil {
		return
	}
	i.mu.Lock()
	delete(i.surfaces, surface)
	i.mu.Unlock()
}

func (i *Instance) surfacesForDevice(device *Device) []*Surface {
	if i == nil || device == nil {
		return nil
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	surfaces := make([]*Surface, 0, len(i.surfaces))
	for surface := range i.surfaces {
		if surface != nil && surface.device == device {
			surfaces = append(surfaces, surface)
		}
	}
	return surfaces
}

func (i *Instance) beginRelease() ([]*Device, []*Surface, bool) {
	if i == nil {
		return nil, nil, false
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.released {
		return nil, nil, false
	}
	i.released = true

	devices := make([]*Device, 0, len(i.devices))
	for device := range i.devices {
		devices = append(devices, device)
	}
	surfaces := make([]*Surface, 0, len(i.surfaces))
	for surface := range i.surfaces {
		surfaces = append(surfaces, surface)
	}
	return devices, surfaces, true
}

// Release releases the instance and all associated resources.
func (i *Instance) Release() {
	devices, surfaces, ok := i.beginRelease()
	if !ok {
		return
	}

	// Devices own configured swapchains, surfaces own platform surface handles,
	// and the instance owns the native instance. Release in that order.
	for _, device := range devices {
		device.Release()
	}
	for _, surface := range surfaces {
		surface.Release()
	}
	i.core.Destroy()
}
