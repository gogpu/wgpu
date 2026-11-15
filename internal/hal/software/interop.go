//go:build !(js && wasm)

package software

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

// NativeHandles reports the software backend behind the instance. The CPU
// rasterizer has no native objects, so only the Kind is set.
func (i *Instance) NativeHandles() publichal.Handles {
	if i == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{Kind: hal.PublicKind(hal.BackendIDSoftware)}
}

// NativeHandles reports the software backend behind the adapter. There are no
// native objects, so only the Kind is set.
func (a *Adapter) NativeHandles() publichal.Handles {
	if a == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{Kind: hal.PublicKind(hal.BackendIDSoftware)}
}

// NativeHandles reports the software backend behind the device. There are no
// native objects, so only the Kind is set.
func (d *Device) NativeHandles() publichal.Handles {
	if d == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{Kind: hal.PublicKind(hal.BackendIDSoftware)}
}

// NativeHandles reports the software backend behind the surface. There are no
// native objects, so only the Kind is set.
func (s *Surface) NativeHandles() publichal.Handles {
	if s == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{Kind: hal.PublicKind(hal.BackendIDSoftware)}
}
