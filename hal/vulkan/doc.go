// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package vulkan provides a Vulkan backend for the HAL.
//
// # Status: PLANNED
//
// This is the primary backend for Linux, Windows, and Android.
//
// # References
//
//   - vulkan-go/vulkan - Vulkan bindings
//   - ash (Rust)       - Low-level Vulkan patterns
//   - Gio              - Has Vulkan backend
//
// # Pure Go Approach
//
// Use purego or direct syscall for Vulkan loader.
// No CGO required.
package vulkan
