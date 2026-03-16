package wgpu

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
)

// InstanceDescriptor configures instance creation.
type InstanceDescriptor struct {
	Backends Backends
}

// Instance is the entry point for GPU operations.
//
// Instance methods are safe for concurrent use, except Release() which
// must not be called concurrently with other methods.
type Instance struct {
	core     *core.Instance
	released bool
}

// CreateInstance creates a new GPU instance.
// If desc is nil, all available backends are used.
func CreateInstance(desc *InstanceDescriptor) (*Instance, error) {
	var gpuDesc *gputypes.InstanceDescriptor
	if desc != nil {
		d := gputypes.DefaultInstanceDescriptor()
		d.Backends = desc.Backends
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
	if i.released {
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

	hub := core.GetGlobal().Hub()
	coreAdapter, err := hub.GetAdapter(adapterID)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to get adapter: %w", err)
	}

	return &Adapter{
		id:       adapterID,
		core:     &coreAdapter,
		info:     info,
		features: features,
		limits:   limits,
		instance: i,
	}, nil
}

// Release releases the instance and all associated resources.
func (i *Instance) Release() {
	if i.released {
		return
	}
	i.released = true
	i.core.Destroy()
}
