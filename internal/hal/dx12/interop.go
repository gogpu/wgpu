//go:build windows && !(js && wasm)

package dx12

import (
	"unsafe"

	publichal "github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/hal"
)

var (
	_ hal.NativeHandlesProvider = (*Instance)(nil)
	_ hal.NativeHandlesProvider = (*Adapter)(nil)
	_ hal.NativeHandlesProvider = (*Device)(nil)
	_ hal.NativeHandlesProvider = (*Surface)(nil)
)

func comPtr(v unsafe.Pointer) uintptr { return uintptr(v) }

// NativeHandles reports the DXGI factory behind the instance. The factory is a
// window into the same object an adapter's Factory names.
func (i *Instance) NativeHandles() publichal.Handles {
	if i == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{
		Kind: hal.PublicKind(hal.BackendIDDX12),
		DX12: publichal.DX12Handles{Factory: comPtr(unsafe.Pointer(i.factory))},
	}
}

// NativeHandles reports the DXGI factory and adapter behind the adapter.
// Handles that belong to a device are zero.
func (a *Adapter) NativeHandles() publichal.Handles {
	if a == nil {
		return publichal.Handles{}
	}
	h := publichal.DX12Handles{Adapter: comPtr(unsafe.Pointer(a.raw))}
	if a.instance != nil {
		h.Factory = comPtr(unsafe.Pointer(a.instance.factory))
	}
	return publichal.Handles{Kind: hal.PublicKind(hal.BackendIDDX12), DX12: h}
}

// NativeHandles reports the D3D12 device, its command queue and the DXGI
// objects behind the device.
//
// Adapter stays zero: the HAL device keeps no back-reference to the adapter it
// was opened from. Call Adapter.HAL for it.
func (d *Device) NativeHandles() publichal.Handles {
	if d == nil {
		return publichal.Handles{}
	}
	h := publichal.DX12Handles{
		Device: comPtr(unsafe.Pointer(d.raw)),
		Queue:  comPtr(unsafe.Pointer(d.directQueue)),
	}
	if d.instance != nil {
		h.Factory = comPtr(unsafe.Pointer(d.instance.factory))
	}
	return publichal.Handles{Kind: hal.PublicKind(hal.BackendIDDX12), DX12: h}
}

// NativeHandles reports the swapchain of the surface; it is zero while the
// surface is not configured.
func (s *Surface) NativeHandles() publichal.Handles {
	if s == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{
		Kind: hal.PublicKind(hal.BackendIDDX12),
		DX12: publichal.DX12Handles{SwapChain: comPtr(unsafe.Pointer(s.swapchain))},
	}
}
