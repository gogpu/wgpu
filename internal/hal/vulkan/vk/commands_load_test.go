//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vk

import (
	"testing"
	"unsafe"
)

func TestLoadResolvesFormatAndExternalMemoryCommands(t *testing.T) {
	c, instance := newTestInstance(t, "VK_KHR_surface")
	newTestDevice(t, c, instance,
		"VK_KHR_external_memory_fd",
		"VK_KHR_image_format_list",
		"VK_EXT_image_drm_format_modifier",
	)

	commands := map[string]unsafe.Pointer{
		"vkGetPhysicalDeviceFormatProperties2":     c.getPhysicalDeviceFormatProperties2,
		"vkGetMemoryFdKHR":                         c.getMemoryFdKHR,
		"vkGetMemoryFdPropertiesKHR":               c.getMemoryFdPropertiesKHR,
		"vkGetImageDrmFormatModifierPropertiesEXT": c.getImageDrmFormatModifierPropertiesEXT,
	}
	for name, fn := range commands {
		if fn == nil {
			t.Errorf("%s was not loaded", name)
		}
	}
}
