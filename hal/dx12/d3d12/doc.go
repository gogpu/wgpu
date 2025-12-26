// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

// Package d3d12 provides low-level Direct3D 12 COM bindings for Windows.
//
// This package uses syscall to directly call D3D12 COM vtable methods,
// providing a zero-CGO approach to GPU programming on Windows.
//
// # Architecture
//
// D3D12 uses COM (Component Object Model) interfaces. Each interface
// is represented as a Go struct containing a pointer to a vtable.
// The vtable contains function pointers for all methods.
//
//	type ID3D12Device struct {
//	    vtbl *id3d12DeviceVtbl
//	}
//
// # Usage
//
//	lib, err := d3d12.LoadD3D12()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	device, err := lib.CreateDevice(nil, D3D_FEATURE_LEVEL_12_0)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer device.Release()
//
//	queue, err := device.CreateCommandQueue(&D3D12_COMMAND_QUEUE_DESC{
//	    Type: D3D12_COMMAND_LIST_TYPE_DIRECT,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer queue.Release()
//
// # Error Handling
//
// All COM methods that return HRESULT are wrapped to return Go errors.
// Use HRESULTError to get the underlying error code.
//
//	if err != nil {
//	    if hr, ok := err.(HRESULTError); ok {
//	        fmt.Printf("HRESULT: 0x%08X\n", uint32(hr))
//	    }
//	}
//
// # Memory Management
//
// COM objects use reference counting. Call Release() when done:
//
//	device, _ := lib.CreateDevice(...)
//	defer device.Release()
//
// # References
//
//   - D3D12 API Reference: https://learn.microsoft.com/en-us/windows/win32/api/_direct3d12/
//   - gonutz/d3d9: https://github.com/gonutz/d3d9 (pattern reference)
package d3d12
