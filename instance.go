package wgpu

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
)

// InstanceDescriptor configures instance creation.
type InstanceDescriptor struct {
	Backends Backends
}

// Instance is the entry point for GPU operations.
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

	info, _ := core.GetAdapterInfo(adapterID)
	features, _ := core.GetAdapterFeatures(adapterID)
	limits, _ := core.GetAdapterLimits(adapterID)

	hub := core.GetGlobal().Hub()
	coreAdapter, _ := hub.GetAdapter(adapterID)

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
