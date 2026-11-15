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
	if a == nil || a.core == nil {
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
	if s == nil || s.core == nil {
		return hal.Handles{}
	}
	return s.core.HALHandles()
}
