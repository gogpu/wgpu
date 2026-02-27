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
func (i *Instance) RequestAdapter(opts *RequestAdapterOptions) (*Adapter, error) {
	if i.released {
		return nil, ErrReleased
	}

	adapterID, err := i.core.RequestAdapter(opts)
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
