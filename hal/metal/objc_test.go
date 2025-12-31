// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"math"
	"testing"
	"unsafe"
)

func TestObjCRuntimeBasics(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	pool := NewAutoreleasePool()
	if pool == nil || pool.pool == 0 {
		t.Fatal("NewAutoreleasePool returned nil")
	}
	defer pool.Drain()

	nsObject := GetClass("NSObject")
	if nsObject == 0 {
		t.Fatal("GetClass(NSObject) returned nil")
	}

	nsString := GetClass("NSString")
	if nsString == 0 {
		t.Fatal("GetClass(NSString) returned nil")
	}

	alloc := RegisterSelector("alloc")
	initSel := RegisterSelector("init")
	releaseSel := RegisterSelector("release")
	if alloc == 0 || initSel == 0 || releaseSel == 0 {
		t.Fatal("RegisterSelector returned nil")
	}

	value := "wgpu"
	ns := NSString(value)
	if ns == 0 {
		t.Fatal("NSString returned nil")
	}

	length := MsgSendUint(ns, Sel("length"))
	if length != uint(len(value)) {
		t.Fatalf("NSString length = %d, want %d", length, len(value))
	}

	got := GoString(ns)
	if got != value {
		t.Fatalf("GoString = %q, want %q", got, value)
	}

	ns2 := NSString(value)
	if ns2 == 0 {
		t.Fatal("NSString second value returned nil")
	}

	if !MsgSendBool(ns, Sel("isEqualToString:"), uintptr(ns2)) {
		t.Fatal("NSString isEqualToString returned false")
	}

	Release(ns2)
	Release(ns)

	obj := MsgSend(ID(nsObject), alloc)
	if obj == 0 {
		t.Fatal("NSObject alloc returned nil")
	}
	obj = MsgSend(obj, initSel)
	if obj == 0 {
		t.Fatal("NSObject init returned nil")
	}
	_ = MsgSend(obj, releaseSel)
}

func TestMetalDeviceQueries(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	devices := CopyAllDevices()
	if len(devices) == 0 {
		t.Skip("no Metal devices available")
	}
	defer func() {
		for _, device := range devices {
			Release(device)
		}
	}()

	for _, device := range devices {
		name := DeviceName(device)
		if name == "" {
			t.Fatal("DeviceName returned empty string")
		}

		_ = DeviceSupportsFamily(device, MTLGPUFamilyMetal3)
		_ = DeviceRegistryID(device)
		_ = DeviceIsLowPower(device)
		_ = DeviceIsHeadless(device)
		_ = DeviceIsRemovable(device)

		maxBuf := DeviceMaxBufferLength(device)
		if maxBuf == 0 {
			t.Fatal("DeviceMaxBufferLength returned 0")
		}

		_ = DeviceRecommendedMaxWorkingSetSize(device)
	}
}

func TestCAMetalLayerDrawableSize(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	device := CreateSystemDefaultDevice()
	if device == 0 {
		t.Skip("no Metal device available")
	}
	defer Release(device)

	pool := NewAutoreleasePool()
	defer pool.Drain()

	layer := MsgSend(ID(GetClass("CAMetalLayer")), Sel("new"))
	if layer == 0 {
		t.Fatal("CAMetalLayer new returned nil")
	}
	defer Release(layer)

	_ = MsgSend(layer, Sel("setDevice:"), uintptr(device))
	_ = MsgSend(layer, Sel("setPixelFormat:"), uintptr(MTLPixelFormatBGRA8Unorm))

	expected := CGSize{Width: 64, Height: 32}
	msgSendCGSize(layer, Sel("setDrawableSize:"), expected)

	got := msgSendCGSizeReturn(t, layer, Sel("drawableSize"))
	if math.Abs(float64(got.Width-expected.Width)) > 1e-6 || math.Abs(float64(got.Height-expected.Height)) > 1e-6 {
		t.Fatalf("drawableSize = %+v, want %+v", got, expected)
	}
}

func TestRenderPassDescriptorClearColor(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	pool := NewAutoreleasePool()
	defer pool.Drain()

	desc := MsgSend(ID(GetClass("MTLRenderPassDescriptor")), Sel("renderPassDescriptor"))
	if desc == 0 {
		t.Fatal("MTLRenderPassDescriptor renderPassDescriptor returned nil")
	}
	defer Release(desc)

	attachments := MsgSend(desc, Sel("colorAttachments"))
	if attachments == 0 {
		t.Fatal("colorAttachments returned nil")
	}
	attachment := MsgSend(attachments, Sel("objectAtIndexedSubscript:"), 0)
	if attachment == 0 {
		t.Fatal("color attachment returned nil")
	}

	expected := MTLClearColor{Red: 0.1, Green: 0.2, Blue: 0.3, Alpha: 1.0}
	msgSendClearColor(attachment, Sel("setClearColor:"), expected)

	got := msgSendClearColorReturn(t, attachment, Sel("clearColor"))
	if math.Abs(got.Red-expected.Red) > 1e-6 || math.Abs(got.Green-expected.Green) > 1e-6 || math.Abs(got.Blue-expected.Blue) > 1e-6 || math.Abs(got.Alpha-expected.Alpha) > 1e-6 {
		t.Fatalf("clearColor = %+v, want %+v", got, expected)
	}
}

func msgSendCGSizeReturn(t *testing.T, obj ID, sel SEL) CGSize {
	t.Helper()
	if obj == 0 || sel == 0 {
		t.Fatal("msgSendCGSizeReturn requires non-nil object and selector")
	}
	var result CGSize
	if err := msgSend(obj, sel, cgSizeType, unsafe.Pointer(&result)); err != nil {
		t.Fatalf("msgSend failed: %v", err)
	}
	return result
}

func msgSendClearColorReturn(t *testing.T, obj ID, sel SEL) MTLClearColor {
	t.Helper()
	if obj == 0 || sel == 0 {
		t.Fatal("msgSendClearColorReturn requires non-nil object and selector")
	}
	var result MTLClearColor
	if err := msgSend(obj, sel, mtlClearColorType, unsafe.Pointer(&result)); err != nil {
		t.Fatalf("msgSend failed: %v", err)
	}
	return result
}
