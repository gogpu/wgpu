//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"errors"
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

func TestAndroidSurfaceRequestRejectsNullNativeWindow(t *testing.T) {
	if err := validateAndroidSurfaceRequest(0); err == nil {
		t.Fatal("null ANativeWindow was accepted")
	}
}

func TestAndroidSurfaceRequestIsWindowOnlyAndStateless(t *testing.T) {
	// The helper deliberately has no display/generation parameter or retained
	// state: two live windows are independently valid requests.
	for _, window := range []uintptr{0x12345678, 0x23456789} {
		if err := validateAndroidSurfaceRequest(window); err != nil {
			t.Fatalf("ANativeWindow %#x rejected: %v", window, err)
		}
	}
}

func TestAndroidSurfaceCreateInfoPreservesIndependentRawWindows(t *testing.T) {
	firstWindow := uintptr(0x87654321) | uintptr(0x10)<<32
	secondWindow := uintptr(0x23456789)

	first := vk.AndroidSurfaceCreateInfoKHR{SType: vk.StructureTypeAndroidSurfaceCreateInfoKhr}
	second := vk.AndroidSurfaceCreateInfoKHR{SType: vk.StructureTypeAndroidSurfaceCreateInfoKhr}
	setAndroidSurfaceNativeWindow(&first, firstWindow)
	setAndroidSurfaceNativeWindow(&second, secondWindow)

	if got := *(*uintptr)(unsafe.Pointer(&first.Window)); got != firstWindow {
		t.Fatalf("first raw ANativeWindow = %#x, want %#x", got, firstWindow)
	}
	if got := *(*uintptr)(unsafe.Pointer(&second.Window)); got != secondWindow {
		t.Fatalf("second raw ANativeWindow = %#x, want %#x", got, secondWindow)
	}
}

func TestAndroidSurfaceCreateInfoMatchesNDKArm64ABI(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("Android preview supports arm64 only")
	}

	var info vk.AndroidSurfaceCreateInfoKHR
	if got := unsafe.Sizeof(info); got != 32 {
		t.Fatalf("VkAndroidSurfaceCreateInfoKHR size = %d, want 32", got)
	}
	if got := unsafe.Offsetof(info.SType); got != 0 {
		t.Fatalf("sType offset = %d, want 0", got)
	}
	if got := unsafe.Offsetof(info.PNext); got != 8 {
		t.Fatalf("pNext offset = %d, want 8", got)
	}
	if got := unsafe.Offsetof(info.Flags); got != 16 {
		t.Fatalf("flags offset = %d, want 16", got)
	}
	if got := unsafe.Offsetof(info.Window); got != 24 {
		t.Fatalf("window offset = %d, want 24", got)
	}
}

func TestAndroidSurfaceSupportRequiresExtensionAndCommands(t *testing.T) {
	if err := validateAndroidSurfaceSupport(true, true); err != nil {
		t.Fatalf("complete Android WSI support rejected: %v", err)
	}
	for _, support := range [][2]bool{
		{false, true},
		{true, false},
	} {
		if err := validateAndroidSurfaceSupport(support[0], support[1]); err == nil {
			t.Fatalf("incomplete Android WSI support accepted: %v", support)
		}
	}
}

func TestAndroidSurfaceCreateErrorsRemainRecoverable(t *testing.T) {
	for _, test := range []struct {
		result vk.Result
		want   error
	}{
		{result: vk.ErrorSurfaceLostKhr, want: hal.ErrSurfaceLost},
		{result: vk.ErrorInitializationFailed, want: hal.ErrSurfaceLost},
		{result: vk.ErrorDeviceLost, want: hal.ErrDeviceLost},
		{result: vk.ErrorOutOfHostMemory, want: hal.ErrDeviceOutOfMemory},
		{result: vk.ErrorOutOfDeviceMemory, want: hal.ErrDeviceOutOfMemory},
	} {
		if err := mapAndroidSurfaceCreateError(test.result); !errors.Is(err, test.want) {
			t.Fatalf("mapAndroidSurfaceCreateError(%d) = %v, want %v", test.result, err, test.want)
		}
	}
	err := mapAndroidSurfaceCreateError(vk.ErrorNativeWindowInUseKhr)
	if err == nil || errors.Is(err, hal.ErrSurfaceLost) {
		t.Fatalf("native-window-in-use error = %v, want untyped ownership conflict", err)
	}
}

func TestAndroidSDKPolicyStartsAtAPI29(t *testing.T) {
	if err := validateAndroidSDKVersion(28); err == nil {
		t.Fatal("Android API 28 was accepted")
	}
	for _, sdk := range []uint32{29, 30} {
		if err := validateAndroidSDKVersion(sdk); err != nil {
			t.Fatalf("Android API %d rejected: %v", sdk, err)
		}
	}
}

func TestAndroidInstancePolicyRejectsDebugCallbacks(t *testing.T) {
	if err := validateAndroidInstanceFlags(gputypes.InstanceFlagsDebug); err == nil {
		t.Fatal("Android debug callback request was accepted")
	}
	if err := validateAndroidInstanceFlags(gputypes.InstanceFlagsNone); err != nil {
		t.Fatalf("ordinary Android instance flags rejected: %v", err)
	}
}
