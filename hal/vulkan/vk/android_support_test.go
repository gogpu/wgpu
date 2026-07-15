//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vk

import (
	"testing"
	"unsafe"
)

func TestVulkanLibraryNameForAndroidDoesNotUseDesktopSoname(t *testing.T) {
	if got := vulkanLibraryNameFor("android"); got != "libvulkan.so" {
		t.Fatalf("Android Vulkan library = %q, want libvulkan.so", got)
	}
	if got := vulkanLibraryNameFor("linux"); got != "libvulkan.so.1" {
		t.Fatalf("Linux Vulkan library = %q, want libvulkan.so.1", got)
	}
}

func TestAndroidSurfaceCommandSupportIsExplicit(t *testing.T) {
	command := unsafe.Pointer(new(byte))
	commands := Commands{
		createAndroidSurfaceKHR:                 command,
		destroySurfaceKHR:                       command,
		getPhysicalDeviceSurfaceSupportKHR:      command,
		getPhysicalDeviceSurfaceCapabilitiesKHR: command,
		getPhysicalDeviceSurfaceFormatsKHR:      command,
		getPhysicalDeviceSurfacePresentModesKHR: command,
	}
	if !commands.HasCreateAndroidSurfaceKHR() || !commands.HasWSIQueries() {
		t.Fatal("complete Android WSI command set was rejected")
	}

	commands.createAndroidSurfaceKHR = nil
	if commands.HasCreateAndroidSurfaceKHR() {
		t.Fatal("missing vkCreateAndroidSurfaceKHR was accepted")
	}
	commands.createAndroidSurfaceKHR = command
	commands.getPhysicalDeviceSurfaceSupportKHR = nil
	if commands.HasWSIQueries() {
		t.Fatal("incomplete VK_KHR_surface command set was accepted")
	}
}
