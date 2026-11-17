//go:build !(js && wasm)

package core

import (
	publichal "github.com/gogpu/wgpu/hal"
	halpkg "github.com/gogpu/wgpu/internal/hal"
)

// HALHandles reports the native objects behind the device, for
// (*wgpu.Device).HAL.
//
// It takes the snatch guard wgpu's own operations take, so a device released
// concurrently yields either its handles or the zero value, never a dangling
// pointer.
func (d *Device) HALHandles() publichal.Handles {
	if d == nil || d.raw == nil || d.snatchLock == nil {
		return publichal.Handles{}
	}
	guard := d.snatchLock.Read()
	defer guard.Release()
	halDevice := d.raw.Get(guard)
	if halDevice == nil {
		return publichal.Handles{}
	}
	return nativeHandles(*halDevice)
}

// HALHandles reports the native objects behind the adapter, for
// (*wgpu.Adapter).HAL. The HAL adapter is not snatchable: it lives as long as
// the core adapter.
func (a *Adapter) HALHandles() publichal.Handles {
	if a == nil {
		return publichal.Handles{}
	}
	return nativeHandles(a.halAdapter)
}

// HALHandles reports the native objects behind the HAL instance of the
// instance's highest-priority enabled backend, for (*wgpu.Instance).HAL.
func (i *Instance) HALHandles() publichal.Handles {
	if i == nil {
		return publichal.Handles{}
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	if len(i.halInstanceEntries) == 0 {
		return publichal.Handles{}
	}
	return nativeHandles(i.halInstanceEntries[0].Instance)
}

// HALHandles reports the native presentation object behind the surface, for
// (*wgpu.Surface).HAL. A destroyed surface has no raw HAL surface and yields
// the zero value.
func (s *Surface) HALHandles() publichal.Handles {
	if s == nil {
		return publichal.Handles{}
	}
	s.mu.Lock()
	raw := s.raw
	s.mu.Unlock()
	return nativeHandles(raw)
}

// nativeHandles asks a HAL object for its public handle snapshot. It knows no
// backend: the object either implements hal.NativeHandlesProvider or names
// nothing.
func nativeHandles(v any) publichal.Handles {
	if provider, ok := v.(halpkg.NativeHandlesProvider); ok {
		return provider.NativeHandles()
	}
	return publichal.Handles{}
}
