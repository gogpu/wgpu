//go:build js && wasm

package wgpu

import "github.com/gogpu/wgpu/hal"

// halHandles is the browser-build body of Instance.HAL. The public zero-value
// contract lives here: the declaration layer records its own released flag (a
// wrapper without an implementation cannot record it in the implementation),
// and the implementation reports the browser kind while it lives.
func (i *Instance) halHandles() hal.Handles {
	if i == nil || i.released {
		return hal.Handles{}
	}
	return i.impl.HAL()
}

// halHandles is the browser-build body of Adapter.HAL. The public zero-value
// contract lives here, as for Instance.HAL.
func (a *Adapter) halHandles() hal.Handles {
	if a == nil || a.released {
		return hal.Handles{}
	}
	return a.impl.HAL()
}

// halHandles is the browser-build body of Device.HAL.
func (d *Device) halHandles() hal.Handles {
	if d == nil || d.impl == nil || d.impl.IsReleased() {
		return hal.Handles{}
	}
	return hal.Handles{Kind: hal.KindBrowser}
}

// halHandles is the browser-build body of Surface.HAL.
func (s *Surface) halHandles() hal.Handles {
	if s == nil || s.impl.Released() {
		return hal.Handles{}
	}
	return hal.Handles{Kind: hal.KindBrowser}
}
