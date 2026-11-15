//go:build js && wasm

package wgpu

import "github.com/gogpu/wgpu/hal"

// halHandles is the browser-build body of Instance.HAL.
func (i *Instance) halHandles() hal.Handles {
	if i == nil || i.released {
		return hal.Handles{}
	}
	return hal.Handles{Kind: hal.KindBrowser}
}

// halHandles is the browser-build body of Adapter.HAL.
func (a *Adapter) halHandles() hal.Handles {
	if a == nil || a.released {
		return hal.Handles{}
	}
	return hal.Handles{Kind: hal.KindBrowser}
}

// halHandles is the browser-build body of Device.HAL.
func (d *Device) halHandles() hal.Handles {
	if d == nil || d.released {
		return hal.Handles{}
	}
	return hal.Handles{Kind: hal.KindBrowser}
}

// halHandles is the browser-build body of Surface.HAL.
func (s *Surface) halHandles() hal.Handles {
	if s == nil || s.released {
		return hal.Handles{}
	}
	return hal.Handles{Kind: hal.KindBrowser}
}
