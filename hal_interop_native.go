//go:build !rust && !(js && wasm)

package wgpu

import "github.com/gogpu/wgpu/hal"

// halHandles is the native-build body of Instance.HAL. The implementation
// reports the handles of the core instance's highest-priority HAL backend, and
// the zero Handles once the core instance is gone; a declaration layer without
// an implementation (the zero value) reports the zero value too.
func (i *Instance) halHandles() hal.Handles {
	if i == nil || i.impl == nil {
		return hal.Handles{}
	}
	return i.impl.HAL()
}

// halHandles is the native-build body of Adapter.HAL.
func (a *Adapter) halHandles() hal.Handles {
	// A cached adapter is instance-owned: Adapter.Release only marks the
	// implementation wrapper, and releasing the instance destroys the HAL object
	// underneath. Report nothing once either is gone, so a caller never receives
	// a stale native pointer; hal_interop.go promises the zero Handles after
	// release.
	if a == nil || a.impl == nil {
		return hal.Handles{}
	}
	return a.impl.HAL()
}

// halHandles is the native-build body of Device.HAL.
func (d *Device) halHandles() hal.Handles {
	if d == nil || d.impl == nil || d.impl.Core() == nil {
		return hal.Handles{}
	}
	return d.impl.Core().HALHandles()
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
