//go:build !(js && wasm)

package vulkan

import (
	publichal "github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/hal/vulkan/vk"
)

var (
	_ hal.NativeHandlesProvider = (*Instance)(nil)
	_ hal.NativeHandlesProvider = (*Adapter)(nil)
	_ hal.NativeHandlesProvider = (*Device)(nil)
	_ hal.NativeHandlesProvider = (*Surface)(nil)
)

// NativeHandles reports the Vulkan objects behind the instance, in the terms
// of the public hal package.
func (i *Instance) NativeHandles() publichal.Handles {
	if i == nil {
		return publichal.Handles{}
	}
	return publichal.Handles{
		Kind: hal.PublicKind(hal.BackendIDVulkan),
		Vulkan: publichal.VulkanHandles{
			Instance:            uintptr(i.handle),
			GetInstanceProcAddr: vk.InstanceProcAddr(),
			GetDeviceProcAddr:   vk.DeviceProcAddr(),
		},
	}
}

// NativeHandles reports the Vulkan objects behind the adapter: its instance and
// physical device. Handles that belong to a device are zero.
func (a *Adapter) NativeHandles() publichal.Handles {
	if a == nil {
		return publichal.Handles{}
	}
	h := publichal.VulkanHandles{PhysicalDevice: uintptr(a.physicalDevice)}
	if a.instance != nil {
		h.Instance = uintptr(a.instance.handle)
	}
	return publichal.Handles{Kind: hal.PublicKind(hal.BackendIDVulkan), Vulkan: h}
}

// NativeHandles reports the Vulkan objects behind the device: its instance,
// physical device, device, queue and the loader's two proc-address functions.
func (d *Device) NativeHandles() publichal.Handles {
	if d == nil {
		return publichal.Handles{}
	}
	h := publichal.VulkanHandles{
		PhysicalDevice:      uintptr(d.physicalDevice),
		Device:              uintptr(d.handle),
		QueueFamilyIndex:    d.graphicsFamily,
		GetInstanceProcAddr: vk.InstanceProcAddr(),
		GetDeviceProcAddr:   vk.DeviceProcAddr(),
	}
	if d.instance != nil {
		h.Instance = uintptr(d.instance.handle)
	}
	if d.queue != nil {
		h.Queue = uintptr(d.queue.handle)
	}
	return publichal.Handles{Kind: hal.PublicKind(hal.BackendIDVulkan), Vulkan: h}
}

// NativeHandles reports the VkSurfaceKHR of the surface, with the instance and
// the physical device when the surface knows the device it was configured on.
func (s *Surface) NativeHandles() publichal.Handles {
	if s == nil {
		return publichal.Handles{}
	}
	h := publichal.VulkanHandles{Surface: uintptr(s.handle)}
	if s.instance != nil {
		h.Instance = uintptr(s.instance.handle)
	}
	if s.device != nil {
		h.PhysicalDevice = uintptr(s.device.physicalDevice)
	}
	return publichal.Handles{Kind: hal.PublicKind(hal.BackendIDVulkan), Vulkan: h}
}
