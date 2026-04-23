//go:build js && wasm

package wgpu

// InstanceDescriptor configures instance creation.
type InstanceDescriptor struct {
	Backends Backends
	Flags    uint32
}

// Instance is the entry point for GPU operations.
type Instance struct {
	released bool
}

// CreateInstance creates a new GPU instance.
func CreateInstance(desc *InstanceDescriptor) (*Instance, error) {
	panic("wgpu: browser backend not yet implemented")
}

// RequestAdapter requests a GPU adapter matching the options.
func (i *Instance) RequestAdapter(opts *RequestAdapterOptions) (*Adapter, error) {
	panic("wgpu: browser backend not yet implemented")
}

// CreateSurface creates a rendering surface from platform-specific handles.
func (i *Instance) CreateSurface(displayHandle, windowHandle uintptr) (*Surface, error) {
	panic("wgpu: browser backend not yet implemented")
}

// Release releases the instance and all associated resources.
func (i *Instance) Release() {
	if i.released {
		return
	}
	i.released = true
}
