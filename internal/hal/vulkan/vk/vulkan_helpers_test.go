//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vk

import (
	"bytes"
	"runtime"
	"testing"
	"unsafe"
)

// newTestInstance creates a Vulkan 1.1 instance with the given extensions and
// loads its commands. It skips the test when no Vulkan driver is available.
func newTestInstance(t *testing.T, extensions ...string) (*Commands, Instance) {
	t.Helper()
	if err := Init(); err != nil {
		t.Skipf("Vulkan not available: %v", err)
	}
	c := NewCommands()
	if err := c.LoadGlobal(); err != nil {
		t.Skipf("Vulkan not available: %v", err)
	}

	names, ptrs := testCStrings(extensions)
	app := ApplicationInfo{SType: StructureTypeApplicationInfo, ApiVersion: 1<<22 | 1<<12}
	info := InstanceCreateInfo{
		SType:                 StructureTypeInstanceCreateInfo,
		PApplicationInfo:      &app,
		EnabledExtensionCount: uint32(len(ptrs)),
	}
	if len(ptrs) > 0 {
		info.PpEnabledExtensionNames = uintptr(unsafe.Pointer(&ptrs[0]))
	}
	var instance Instance
	result := c.CreateInstance(&info, nil, &instance)
	runtime.KeepAlive(names)
	runtime.KeepAlive(ptrs)
	if result != Success {
		t.Skipf("vkCreateInstance failed: %d", result)
	}
	t.Cleanup(func() { c.DestroyInstance(instance, nil) })

	if err := c.LoadInstance(instance); err != nil {
		t.Fatalf("LoadInstance: %v", err)
	}
	SetDeviceProcAddr(instance)
	return c, instance
}

// newTestDevice creates a device with one graphics queue on the first physical
// device that supports all given extensions, and loads its commands. It skips
// the test when no physical device qualifies.
func newTestDevice(t *testing.T, c *Commands, instance Instance, extensions ...string) (PhysicalDevice, Device, uint32) {
	t.Helper()
	var count uint32
	c.EnumeratePhysicalDevices(instance, &count, nil)
	if count == 0 {
		t.Skip("no Vulkan physical device")
	}
	devices := make([]PhysicalDevice, count)
	c.EnumeratePhysicalDevices(instance, &count, &devices[0])

	for _, pd := range devices[:count] {
		family, ok := testGraphicsQueueFamily(c, pd)
		if !ok || !testSupportsDeviceExtensions(c, pd, extensions) {
			continue
		}
		names, ptrs := testCStrings(extensions)
		priority := float32(1)
		queueInfo := DeviceQueueCreateInfo{
			SType:            StructureTypeDeviceQueueCreateInfo,
			QueueFamilyIndex: family,
			QueueCount:       1,
			PQueuePriorities: &priority,
		}
		info := DeviceCreateInfo{
			SType:                 StructureTypeDeviceCreateInfo,
			QueueCreateInfoCount:  1,
			PQueueCreateInfos:     &queueInfo,
			EnabledExtensionCount: uint32(len(ptrs)),
		}
		if len(ptrs) > 0 {
			info.PpEnabledExtensionNames = uintptr(unsafe.Pointer(&ptrs[0]))
		}
		var device Device
		result := c.CreateDevice(pd, &info, nil, &device)
		runtime.KeepAlive(names)
		runtime.KeepAlive(ptrs)
		if result != Success {
			t.Fatalf("vkCreateDevice failed: %d", result)
		}
		t.Cleanup(func() { c.DestroyDevice(device, nil) })

		if err := c.LoadDevice(device); err != nil {
			t.Fatalf("LoadDevice: %v", err)
		}
		return pd, device, family
	}
	t.Skipf("no Vulkan physical device supports %v", extensions)
	return 0, 0, 0
}

func testGraphicsQueueFamily(c *Commands, pd PhysicalDevice) (uint32, bool) {
	var count uint32
	c.GetPhysicalDeviceQueueFamilyProperties(pd, &count, nil)
	if count == 0 {
		return 0, false
	}
	families := make([]QueueFamilyProperties, count)
	c.GetPhysicalDeviceQueueFamilyProperties(pd, &count, &families[0])
	for i, family := range families[:count] {
		if family.QueueFlags&QueueFlags(QueueGraphicsBit) != 0 {
			return uint32(i), true
		}
	}
	return 0, false
}

func testSupportsDeviceExtensions(c *Commands, pd PhysicalDevice, extensions []string) bool {
	var count uint32
	if c.EnumerateDeviceExtensionProperties(pd, 0, &count, nil) != Success {
		return false
	}
	available := make(map[string]bool, count)
	if count > 0 {
		props := make([]ExtensionProperties, count)
		if c.EnumerateDeviceExtensionProperties(pd, 0, &count, &props[0]) != Success {
			return false
		}
		for i := range props[:count] {
			name := props[i].ExtensionName[:]
			if n := bytes.IndexByte(name, 0); n >= 0 {
				name = name[:n]
			}
			available[string(name)] = true
		}
	}
	for _, ext := range extensions {
		if !available[ext] {
			return false
		}
	}
	return true
}

func testCStrings(strs []string) ([][]byte, []uintptr) {
	names := make([][]byte, len(strs))
	ptrs := make([]uintptr, len(strs))
	for i, s := range strs {
		names[i] = append([]byte(s), 0)
		ptrs[i] = uintptr(unsafe.Pointer(&names[i][0]))
	}
	return names, ptrs
}
