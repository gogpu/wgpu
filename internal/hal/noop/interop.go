//go:build !(js && wasm)

package noop

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

// NativeHandles reports the no-op backend behind the instance. There are no
// native objects, so only the Kind is set.
func (i *Instance) NativeHandles() publichal.Handles {
	if i == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{Kind: hal.PublicKind(hal.BackendIDNoop)}
}

// NativeHandles reports the no-op backend behind the adapter. There are no
// native objects, so only the Kind is set.
func (a *Adapter) NativeHandles() publichal.Handles {
	if a == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{Kind: hal.PublicKind(hal.BackendIDNoop)}
}

// NativeHandles reports the no-op backend behind the device. There are no
// native objects, so only the Kind is set.
func (d *Device) NativeHandles() publichal.Handles {
	if d == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{Kind: hal.PublicKind(hal.BackendIDNoop)}
}

// NativeHandles reports the no-op backend behind the surface. There are no
// native objects, so only the Kind is set.
func (s *Surface) NativeHandles() publichal.Handles {
	if s == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{Kind: hal.PublicKind(hal.BackendIDNoop)}
}
