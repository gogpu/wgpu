//go:build !rust && !(js && wasm)

package wgpu

import "github.com/gogpu/wgpu/hal"

// halHandles is the native-build body of Instance.HAL.
func (i *Instance) halHandles() hal.Handles {
	if i == nil || i.core == nil {
		return hal.Handles{}
	}
	return i.core.HALHandles()
}

// halHandles is the native-build body of Adapter.HAL.
func (a *Adapter) halHandles() hal.Handles {
	// A cached adapter is instance-owned: Adapter.Release only marks the
	// wrapper, and releasing the instance destroys the HAL object underneath.
	// Report nothing once either is gone, so a caller never receives a stale
	// native pointer; hal_interop.go promises the zero Handles after release.
	if a == nil || a.core == nil || a.released || (a.instance != nil && a.instance.isReleased()) {
		return hal.Handles{}
	}
	return a.core.HALHandles()
}

// halHandles is the native-build body of Device.HAL.
func (d *Device) halHandles() hal.Handles {
	if d == nil || d.core == nil {
		return hal.Handles{}
	}
	return d.core.HALHandles()
}

// halHandles is the native-build body of Surface.HAL.
func (s *Surface) halHandles() hal.Handles {
	if s == nil {
		return hal.Handles{}
	}
	coreSurface := s.impl.Core()
	if coreSurface == nil {
		return hal.Handles{}
	}
	return coreSurface.HALHandles()
}
