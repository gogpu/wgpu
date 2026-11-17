//go:build rust

package wgpu

import "github.com/gogpu/wgpu/hal"

// halHandles is the "rust"-build body of Instance.HAL. The implementation
// reports the wgpu-native instance handle, and the zero Handles once it is
// released; a declaration layer without an implementation (the zero value)
// reports the zero value too.
func (i *Instance) halHandles() hal.Handles {
	if i == nil || i.impl == nil {
		return hal.Handles{}
	}
	return i.impl.HAL()
}

// halHandles is the "rust"-build body of Adapter.HAL.
func (a *Adapter) halHandles() hal.Handles {
	// wgpu-native's WGPUAdapterImpl owns its own Arc<Context>, so the adapter
	// handle stays valid after the instance is released. The zero Handles after
	// either release is the contract hal_interop.go promises, consistent with
	// Instance.HAL(), not a lifetime requirement (ADR-070 §15.5).
	if a == nil || a.impl == nil {
		return hal.Handles{}
	}
	return a.impl.HAL()
}

// halHandles is the "rust"-build body of Device.HAL. Adapter stays zero: the
// wrapper does not keep the adapter it was requested from; call Adapter.HAL.
func (d *Device) halHandles() hal.Handles {
	if d == nil || d.impl == nil || d.impl.IsReleased() || d.impl.R() == nil {
		return hal.Handles{}
	}
	h := hal.WebGPUHandles{Device: d.impl.R().Handle()}
	if d.instance != nil && d.instance.impl != nil && d.instance.impl.InstanceR() != nil {
		h.Instance = d.instance.impl.InstanceR().Handle()
	}
	if d.queue != nil && d.queue.impl.R() != nil {
		h.Queue = d.queue.impl.R().Handle()
	}
	return hal.Handles{Kind: hal.KindWebGPU, WebGPU: h}
}

// halHandles is the "rust"-build body of Surface.HAL. WebGPUHandles names no
// surface object, so only the Kind is set.
func (s *Surface) halHandles() hal.Handles {
	if s == nil || s.impl.Released() {
		return hal.Handles{}
	}
	return hal.Handles{Kind: hal.KindWebGPU}
}
