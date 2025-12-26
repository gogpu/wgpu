// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package d3d12

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

var (
	d3d12Lib     *D3D12Lib
	d3d12LibOnce sync.Once
	d3d12LibErr  error
)

// D3D12Lib provides access to D3D12 functions.
type D3D12Lib struct {
	dll                                  *syscall.LazyDLL
	d3d12CreateDevice                    *syscall.LazyProc
	d3d12GetDebugInterface               *syscall.LazyProc
	d3d12SerializeRootSignature          *syscall.LazyProc
	d3d12SerializeVersionedRootSignature *syscall.LazyProc
}

// LoadD3D12 loads the D3D12 library. Safe to call multiple times.
func LoadD3D12() (*D3D12Lib, error) {
	d3d12LibOnce.Do(func() {
		d3d12Lib, d3d12LibErr = loadD3D12Internal()
	})
	return d3d12Lib, d3d12LibErr
}

func loadD3D12Internal() (*D3D12Lib, error) {
	dll := syscall.NewLazyDLL("d3d12.dll")
	if err := dll.Load(); err != nil {
		return nil, fmt.Errorf("failed to load d3d12.dll: %w", err)
	}

	lib := &D3D12Lib{
		dll:                                  dll,
		d3d12CreateDevice:                    dll.NewProc("D3D12CreateDevice"),
		d3d12GetDebugInterface:               dll.NewProc("D3D12GetDebugInterface"),
		d3d12SerializeRootSignature:          dll.NewProc("D3D12SerializeRootSignature"),
		d3d12SerializeVersionedRootSignature: dll.NewProc("D3D12SerializeVersionedRootSignature"),
	}

	return lib, nil
}

// CreateDevice creates a D3D12 device.
//
// adapter can be nil to use the default adapter.
// minFeatureLevel is the minimum feature level required.
func (lib *D3D12Lib) CreateDevice(adapter unsafe.Pointer, minFeatureLevel D3D_FEATURE_LEVEL) (*ID3D12Device, error) {
	var device *ID3D12Device

	ret, _, _ := lib.d3d12CreateDevice.Call(
		uintptr(adapter),
		uintptr(minFeatureLevel),
		uintptr(unsafe.Pointer(&IID_ID3D12Device)),
		uintptr(unsafe.Pointer(&device)),
	)

	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	return device, nil
}

// GetDebugInterface retrieves the D3D12 debug interface.
func (lib *D3D12Lib) GetDebugInterface() (*ID3D12Debug, error) {
	var debug *ID3D12Debug

	ret, _, _ := lib.d3d12GetDebugInterface.Call(
		uintptr(unsafe.Pointer(&IID_ID3D12Debug)),
		uintptr(unsafe.Pointer(&debug)),
	)

	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	return debug, nil
}

// GetDebugInterface1 retrieves the D3D12Debug1 debug interface.
func (lib *D3D12Lib) GetDebugInterface1() (*ID3D12Debug1, error) {
	var debug *ID3D12Debug1

	ret, _, _ := lib.d3d12GetDebugInterface.Call(
		uintptr(unsafe.Pointer(&IID_ID3D12Debug1)),
		uintptr(unsafe.Pointer(&debug)),
	)

	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	return debug, nil
}

// GetDebugInterface3 retrieves the D3D12Debug3 debug interface.
func (lib *D3D12Lib) GetDebugInterface3() (*ID3D12Debug3, error) {
	var debug *ID3D12Debug3

	ret, _, _ := lib.d3d12GetDebugInterface.Call(
		uintptr(unsafe.Pointer(&IID_ID3D12Debug3)),
		uintptr(unsafe.Pointer(&debug)),
	)

	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	return debug, nil
}

// SerializeRootSignature serializes a root signature.
func (lib *D3D12Lib) SerializeRootSignature(
	desc *D3D12_ROOT_SIGNATURE_DESC,
	version D3D12_ROOT_SIGNATURE_VERSION,
) (*ID3DBlob, *ID3DBlob, error) {
	var blob *ID3DBlob
	var errorBlob *ID3DBlob

	ret, _, _ := lib.d3d12SerializeRootSignature.Call(
		uintptr(unsafe.Pointer(desc)),
		uintptr(version),
		uintptr(unsafe.Pointer(&blob)),
		uintptr(unsafe.Pointer(&errorBlob)),
	)

	if ret != 0 {
		return nil, errorBlob, HRESULTError(ret)
	}
	return blob, nil, nil
}
