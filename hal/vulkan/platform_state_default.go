//go:build !android && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import "github.com/gogpu/wgpu/hal"

type platformInstanceState struct {
	swapchain swapchainPlatformPolicy
}

func newPlatformInstanceState(_ *hal.InstanceDescriptor) (platformInstanceState, error) {
	return platformInstanceState{swapchain: defaultSwapchainPlatformPolicy()}, nil
}
