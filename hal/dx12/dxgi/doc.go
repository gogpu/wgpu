// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

// Package dxgi provides low-level DXGI (DirectX Graphics Infrastructure) COM bindings for Windows.
//
// DXGI provides functionality for enumerating graphics adapters, creating swap chains,
// and managing display output. This package uses syscall to directly call DXGI COM vtable
// methods, providing a zero-CGO approach to graphics programming on Windows.
//
// # Architecture
//
// DXGI uses COM (Component Object Model) interfaces. Each interface is represented
// as a Go struct containing a pointer to a vtable. The vtable contains function
// pointers for all methods.
//
//	type IDXGIFactory6 struct {
//	    vtbl *idxgiFactory6Vtbl
//	}
//
// # Usage
//
//	lib, err := dxgi.LoadDXGI()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	factory, err := lib.CreateFactory2(dxgi.DXGI_CREATE_FACTORY_DEBUG)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer factory.Release()
//
//	adapter, err := factory.EnumAdapterByGpuPreference(0, dxgi.DXGI_GPU_PREFERENCE_HIGH_PERFORMANCE)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer adapter.Release()
//
//	desc, err := adapter.GetDesc1()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Adapter: %s\n", desc.Description())
//
// # Error Handling
//
// All COM methods that return HRESULT are wrapped to return Go errors.
// This package reuses d3d12.HRESULTError for consistency.
//
// # Memory Management
//
// COM objects use reference counting. Call Release() when done:
//
//	factory, _ := lib.CreateFactory2(0)
//	defer factory.Release()
//
// # References
//
//   - DXGI Overview: https://learn.microsoft.com/en-us/windows/win32/direct3ddxgi/d3d10-graphics-programming-guide-dxgi
//   - DXGI API Reference: https://learn.microsoft.com/en-us/windows/win32/api/_direct3ddxgi/
package dxgi
