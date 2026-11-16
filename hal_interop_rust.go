//go:build rust

package wgpu

import "github.com/gogpu/wgpu/hal"

// halHandles is the "rust"-build body of Instance.HAL.
func (i *Instance) halHandles() hal.Handles {
	if i == nil || i.released || i.r == nil {
		return hal.Handles{}
	}
	return hal.Handles{
		Kind:   hal.KindWebGPU,
		WebGPU: hal.WebGPUHandles{Instance: i.r.Handle()},
	}
}

// halHandles is the "rust"-build body of Adapter.HAL.
func (a *Adapter) halHandles() hal.Handles {
	// wgpu-native's WGPUAdapterImpl owns its own Arc<Context>, so the adapter
	// handle stays valid after the instance is released. The zero Handles after
	// either release is the contract hal_interop.go promises, consistent with
	// Instance.HAL(), not a lifetime requirement (ADR-070 §15.5).
	if a == nil || a.released || a.r == nil || (a.instance != nil && a.instance.isReleased()) {
		return hal.Handles{}
	}
	h := hal.WebGPUHandles{Adapter: a.r.Handle()}
	if a.instance != nil && a.instance.r != nil {
		h.Instance = a.instance.r.Handle()
	}
	return hal.Handles{Kind: hal.KindWebGPU, WebGPU: h}
}

// halHandles is the "rust"-build body of Device.HAL. Adapter stays zero: the
// wrapper does not keep the adapter it was requested from; call Adapter.HAL.
func (d *Device) halHandles() hal.Handles {
	if d == nil || d.released || d.r == nil {
		return hal.Handles{}
	}
	h := hal.WebGPUHandles{Device: d.r.Handle()}
	if d.instance != nil && d.instance.r != nil {
		h.Instance = d.instance.r.Handle()
	}
	if d.queue != nil && d.queue.impl.R() != nil {
		h.Queue = d.queue.impl.R().Handle()
	}
	return hal.Handles{Kind: hal.KindWebGPU, WebGPU: h}
}

// halHandles is the "rust"-build body of Surface.HAL. WebGPUHandles names no
// surface object, so only the Kind is set.
func (s *Surface) halHandles() hal.Handles {
	if s == nil || s.released {
		return hal.Handles{}
	}
	return hal.Handles{Kind: hal.KindWebGPU}
}
