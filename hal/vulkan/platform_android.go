//go:build android && arm64

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
	"github.com/gogpu/wgpu/hal"
)

type platformInstanceState struct {
	swapchain swapchainPlatformPolicy
}

func newPlatformInstanceState(desc *hal.InstanceDescriptor) (platformInstanceState, error) {
	if desc != nil {
		if err := validateAndroidInstanceFlags(desc.Flags); err != nil {
			return platformInstanceState{}, err
		}
	}

	sdk, err := loadAndroidSDKVersion()
	if err != nil {
		return platformInstanceState{}, fmt.Errorf("vulkan: query Android SDK version: %w", err)
	}
	if err := validateAndroidSDKVersion(sdk); err != nil {
		return platformInstanceState{}, err
	}

	return platformInstanceState{swapchain: androidSwapchainPlatformPolicy(sdk)}, nil
}

func loadAndroidSDKVersion() (uint32, error) {
	handle, err := ffi.LoadLibrary("libc.so")
	if err != nil {
		return 0, err
	}
	defer func() { _ = ffi.FreeLibrary(handle) }()

	fn, err := ffi.GetSymbol(handle, "android_get_device_api_level")
	if err != nil {
		return 0, err
	}
	var call types.CallInterface
	if err := ffi.PrepareCallInterface(&call, types.DefaultCall, types.SInt32TypeDescriptor, nil); err != nil {
		return 0, err
	}

	var sdk int32
	if _, err := ffi.CallFunction(&call, fn, unsafe.Pointer(&sdk), nil); err != nil {
		return 0, err
	}
	if sdk < 0 {
		return 0, fmt.Errorf("invalid negative SDK version %d", sdk)
	}
	return uint32(sdk), nil
}
