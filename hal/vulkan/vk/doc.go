// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

// Package vk provides Pure Go Vulkan bindings generated from vk.xml.
//
// This package contains low-level Vulkan types, constants, and function
// pointers for use with syscall.SyscallN. It does not use CGO.
//
// # Generation
//
// The bindings are generated from the official Khronos vk.xml specification
// using the vk-gen tool:
//
//	go run ./cmd/vk-gen -spec vk.xml -out hal/vulkan/vk/
//
// # Usage
//
// Initialize Vulkan and load function pointers:
//
//	if err := vk.Init(); err != nil {
//	    log.Fatal(err)
//	}
//
//	var cmds vk.Commands
//	cmds.LoadGlobal()
//
//	// Create instance...
//	cmds.LoadInstance(instance)
//
// # Platform Support
//
// - Windows: vulkan-1.dll
// - Linux: libvulkan.so.1 (planned)
// - macOS: libMoltenVK.dylib via MoltenVK (planned)
package vk
