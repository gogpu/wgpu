// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package d3d12

import (
	"syscall"
	"testing"
)

func TestResourceAddRefAndReleaseUseIUnknownVTable(t *testing.T) {
	addRefs := 0
	releases := 0
	resource := &ID3D12Resource{vtbl: &id3d12ResourceVtbl{
		AddRef: syscall.NewCallback(func(uintptr) uintptr {
			addRefs++
			return 2
		}),
		Release: syscall.NewCallback(func(uintptr) uintptr {
			releases++
			return 1
		}),
	}}

	if got := resource.AddRef(); got != 2 || addRefs != 1 {
		t.Fatalf("AddRef = (%d, calls=%d), want (2, 1)", got, addRefs)
	}
	if got := resource.Release(); got != 1 || releases != 1 {
		t.Fatalf("Release = (%d, calls=%d), want (1, 1)", got, releases)
	}
}
