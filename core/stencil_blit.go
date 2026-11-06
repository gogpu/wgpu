//go:build !(js && wasm)

package core

import "sync"

// StencilBlitResources are the per-device resources of the frontend's stencil
// upload blit (the pipelines of internal/stencilblit, built through the public
// API by the root package). core only owns their lifetime: they are released
// when the device is destroyed, before its deferred destructions are flushed.
//
// Dawn keeps the same pipelines in its device's internal pipeline store
// (src/dawn/native/BlitBufferToDepthStencil.cpp, GetOrCreateR8ToStencilPipelines).
type StencilBlitResources interface {
	// Dispose releases every resource. It is called once.
	Dispose()
}

// stencilBlitSlot holds a device's StencilBlitResources.
type stencilBlitSlot struct {
	mu        sync.Mutex
	resources StencilBlitResources
	disposed  bool
}

// StencilBlitResources returns the device's stencil blit resources, calling
// create on the first call. It returns nil after Destroy, and for a device
// without HAL integration.
func (d *Device) StencilBlitResources(create func() StencilBlitResources) StencilBlitResources {
	slot := d.stencilBlit
	if slot == nil {
		return nil
	}
	slot.mu.Lock()
	defer slot.mu.Unlock()
	if slot.disposed {
		return nil
	}
	if slot.resources == nil {
		slot.resources = create()
	}
	return slot.resources
}

// disposeStencilBlit releases the device's stencil blit resources, if any, and
// makes later StencilBlitResources calls return nil.
func (d *Device) disposeStencilBlit() {
	slot := d.stencilBlit
	if slot == nil {
		return
	}
	slot.mu.Lock()
	resources := slot.resources
	slot.resources, slot.disposed = nil, true
	slot.mu.Unlock()
	if resources != nil {
		resources.Dispose()
	}
}
