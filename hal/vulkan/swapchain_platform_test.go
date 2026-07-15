//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"errors"
	"math"
	"testing"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

func TestSwapchainPlatformPolicyAcquireTimeout(t *testing.T) {
	const requested = uint64(1_000_000_000)
	tests := []struct {
		name   string
		policy swapchainPlatformPolicy
		want   uint64
	}{
		{name: "desktop", policy: defaultSwapchainPlatformPolicy(), want: requested},
		{name: "Android API 29", policy: androidSwapchainPlatformPolicy(29), want: math.MaxUint64},
		{name: "Android API 30", policy: androidSwapchainPlatformPolicy(30), want: requested},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.policy.acquireTimeout(requested); got != test.want {
				t.Fatalf("acquireTimeout(%d) = %d, want %d", requested, got, test.want)
			}
		})
	}
}

func TestSwapchainPlatformPolicyTransform(t *testing.T) {
	capabilities := vk.SurfaceCapabilitiesKHR{
		CurrentTransform: vk.SurfaceTransformRotate90BitKhr,
		SupportedTransforms: vk.SurfaceTransformFlagsKHR(
			vk.SurfaceTransformIdentityBitKhr | vk.SurfaceTransformRotate90BitKhr,
		),
	}
	desktop, err := defaultSwapchainPlatformPolicy().preTransform(capabilities)
	if err != nil {
		t.Fatalf("desktop preTransform() error: %v", err)
	}
	if desktop != vk.SurfaceTransformRotate90BitKhr {
		t.Fatalf("desktop transform = %v, want current transform", desktop)
	}

	android, err := androidSwapchainPlatformPolicy(29).preTransform(capabilities)
	if err != nil {
		t.Fatalf("Android preTransform() error: %v", err)
	}
	if android != vk.SurfaceTransformIdentityBitKhr {
		t.Fatalf("Android transform = %v, want identity", android)
	}

	capabilities.SupportedTransforms = vk.SurfaceTransformFlagsKHR(vk.SurfaceTransformRotate90BitKhr)
	if _, err := androidSwapchainPlatformPolicy(29).preTransform(capabilities); err == nil {
		t.Fatal("Android identity transform was accepted when unsupported")
	}
}

func TestSwapchainPlatformPolicySuboptimal(t *testing.T) {
	if !defaultSwapchainPlatformPolicy().reportSuboptimal(true) {
		t.Fatal("desktop suboptimal result was suppressed")
	}
	if androidSwapchainPlatformPolicy(29).reportSuboptimal(true) {
		t.Fatal("Android suboptimal result was reported")
	}
}

func TestMapVulkanResultPreservesRecoverableErrors(t *testing.T) {
	tests := []struct {
		result vk.Result
		want   error
	}{
		{result: vk.ErrorOutOfHostMemory, want: hal.ErrDeviceOutOfMemory},
		{result: vk.ErrorOutOfDeviceMemory, want: hal.ErrDeviceOutOfMemory},
		{result: vk.ErrorDeviceLost, want: hal.ErrDeviceLost},
		{result: vk.ErrorSurfaceLostKhr, want: hal.ErrSurfaceLost},
		{result: vk.ErrorOutOfDateKhr, want: hal.ErrSurfaceOutdated},
		{result: vk.Timeout, want: hal.ErrTimeout},
		{result: vk.NotReady, want: hal.ErrNotReady},
	}
	for _, test := range tests {
		if err := mapVulkanResult("operation", test.result); !errors.Is(err, test.want) {
			t.Fatalf("mapVulkanResult(%d) = %v, want %v", test.result, err, test.want)
		}
	}
	if err := mapVulkanResult("operation", vk.Success); err != nil {
		t.Fatalf("mapVulkanResult(Success) = %v, want nil", err)
	}
}

func TestMapSwapchainCreateResultPreservesSurfaceErrors(t *testing.T) {
	for _, result := range []vk.Result{vk.ErrorSurfaceLostKhr, vk.ErrorInitializationFailed} {
		if err := mapSwapchainCreateResult(result); !errors.Is(err, hal.ErrSurfaceLost) {
			t.Fatalf("mapSwapchainCreateResult(%d) = %v, want ErrSurfaceLost", result, err)
		}
	}
	err := mapSwapchainCreateResult(vk.ErrorNativeWindowInUseKhr)
	if err == nil || errors.Is(err, hal.ErrSurfaceLost) {
		t.Fatalf("native-window-in-use error = %v, want untyped ownership conflict", err)
	}
}
