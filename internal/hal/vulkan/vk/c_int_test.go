//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vk

import (
	"testing"
	"unsafe"
)

func TestCIntFieldsAreFourBytes(t *testing.T) {
	fields := map[string]uintptr{
		"ImportMemoryFdInfoKHR.Fd":    unsafe.Sizeof(ImportMemoryFdInfoKHR{}.Fd),
		"ImportSemaphoreFdInfoKHR.Fd": unsafe.Sizeof(ImportSemaphoreFdInfoKHR{}.Fd),
		"ImportFenceFdInfoKHR.Fd":     unsafe.Sizeof(ImportFenceFdInfoKHR{}.Fd),
		"NativeBufferANDROID.Stride":  unsafe.Sizeof(NativeBufferANDROID{}.Stride),
		"NativeBufferANDROID.Format":  unsafe.Sizeof(NativeBufferANDROID{}.Format),
		"NativeBufferANDROID.Usage":   unsafe.Sizeof(NativeBufferANDROID{}.Usage),
	}
	for name, size := range fields {
		if size != 4 {
			t.Errorf("%s is %d bytes, C int is 4", name, size)
		}
	}
}
