// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package vulkan provides Pure Go Vulkan backend for the HAL.
//
// This backend uses goffi for cross-platform Vulkan API calls, requiring no CGO.
// Function pointers are loaded dynamically from vulkan-1.dll (Windows),
// libvulkan.so.1 (Linux), or MoltenVK (macOS).
//
// # Architecture
//
// The backend follows wgpu-hal patterns:
//   - Instance: VkInstance wrapper with extension loading
//   - Adapter: VkPhysicalDevice enumeration and capabilities
//   - Device: VkDevice with queues and memory allocator
//   - Queue: Command submission and synchronization
//   - Resources: Buffers, textures, pipelines with Vulkan objects
//
// # Memory Management
//
// Unlike OpenGL, Vulkan requires explicit memory allocation. This backend
// implements a pool-based memory allocator similar to gpu-allocator.
//
// # Platform Support
//
//   - Windows: vulkan-1.dll + VK_KHR_win32_surface
//   - Linux: libvulkan.so.1 + VK_KHR_xlib_surface/VK_KHR_xcb_surface (planned)
//   - macOS: MoltenVK + VK_EXT_metal_surface (planned)
package vulkan
