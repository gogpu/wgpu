//go:build darwin && !(js && wasm)

package metal

import (
	publichal "github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/hal"
)

var (
	_ hal.NativeHandlesProvider = (*Instance)(nil)
	_ hal.NativeHandlesProvider = (*Adapter)(nil)
	_ hal.NativeHandlesProvider = (*Device)(nil)
	_ hal.NativeHandlesProvider = (*Surface)(nil)
)

// NativeHandles reports the Metal objects behind the instance. Metal has no
// instance object, so only the Kind is set.
func (i *Instance) NativeHandles() publichal.Handles {
	if i == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{Kind: hal.PublicKind(hal.BackendIDMetal)}
}

// NativeHandles reports the MTLDevice behind the adapter. Metal has no
// separate instance or physical-device object, so that is the only handle.
func (a *Adapter) NativeHandles() publichal.Handles {
	if a == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{
		Kind:  hal.PublicKind(hal.BackendIDMetal),
		Metal: publichal.MetalHandles{Device: uintptr(a.raw)},
	}
}

// NativeHandles reports the MTLDevice and its MTLCommandQueue behind the
// device.
func (d *Device) NativeHandles() publichal.Handles {
	if d == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{
		Kind: hal.PublicKind(hal.BackendIDMetal),
		Metal: publichal.MetalHandles{
			Device:       uintptr(d.raw),
			CommandQueue: uintptr(d.commandQueue),
		},
	}
}

// NativeHandles reports the CAMetalLayer of the surface.
func (s *Surface) NativeHandles() publichal.Handles {
	if s == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{
		Kind:  hal.PublicKind(hal.BackendIDMetal),
		Metal: publichal.MetalHandles{Layer: uintptr(s.layer)},
	}
}
