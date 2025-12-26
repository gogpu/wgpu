// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package dxgi

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

var (
	dxgiLib     *DXGILib
	dxgiLibOnce sync.Once
	dxgiLibErr  error
)

// DXGILib provides access to DXGI functions.
type DXGILib struct {
	dll                *syscall.LazyDLL
	createDXGIFactory1 *syscall.LazyProc
	createDXGIFactory2 *syscall.LazyProc
}

// LoadDXGI loads the DXGI library. Safe to call multiple times.
func LoadDXGI() (*DXGILib, error) {
	dxgiLibOnce.Do(func() {
		dxgiLib, dxgiLibErr = loadDXGIInternal()
	})
	return dxgiLib, dxgiLibErr
}

func loadDXGIInternal() (*DXGILib, error) {
	dll := syscall.NewLazyDLL("dxgi.dll")
	if err := dll.Load(); err != nil {
		return nil, fmt.Errorf("failed to load dxgi.dll: %w", err)
	}

	lib := &DXGILib{
		dll:                dll,
		createDXGIFactory1: dll.NewProc("CreateDXGIFactory1"),
		createDXGIFactory2: dll.NewProc("CreateDXGIFactory2"),
	}

	return lib, nil
}

// CreateFactory1 creates a DXGI factory (IDXGIFactory1).
func (lib *DXGILib) CreateFactory1() (*IDXGIFactory1, error) {
	var factory *IDXGIFactory1

	ret, _, _ := lib.createDXGIFactory1.Call(
		uintptr(unsafe.Pointer(&IID_IDXGIFactory1)),
		uintptr(unsafe.Pointer(&factory)),
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return factory, nil
}

// CreateFactory2 creates a DXGI factory with debug flags (IDXGIFactory6).
// Use DXGI_CREATE_FACTORY_DEBUG for debug mode.
func (lib *DXGILib) CreateFactory2(flags uint32) (*IDXGIFactory6, error) {
	var factory *IDXGIFactory6

	ret, _, _ := lib.createDXGIFactory2.Call(
		uintptr(flags),
		uintptr(unsafe.Pointer(&IID_IDXGIFactory6)),
		uintptr(unsafe.Pointer(&factory)),
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return factory, nil
}

// CreateFactory4 creates a DXGI factory (IDXGIFactory4).
func (lib *DXGILib) CreateFactory4(flags uint32) (*IDXGIFactory4, error) {
	var factory *IDXGIFactory4

	ret, _, _ := lib.createDXGIFactory2.Call(
		uintptr(flags),
		uintptr(unsafe.Pointer(&IID_IDXGIFactory4)),
		uintptr(unsafe.Pointer(&factory)),
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return factory, nil
}

// -----------------------------------------------------------------------------
// IDXGIFactory1 methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (f *IDXGIFactory1) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		f.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(f)),
		0, 0,
	)
	return uint32(ret)
}

// EnumAdapters1 enumerates local adapters.
func (f *IDXGIFactory1) EnumAdapters1(index uint32) (*IDXGIAdapter1, error) {
	var adapter *IDXGIAdapter1

	ret, _, _ := syscall.Syscall(
		f.vtbl.EnumAdapters1,
		3,
		uintptr(unsafe.Pointer(f)),
		uintptr(index),
		uintptr(unsafe.Pointer(&adapter)),
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return adapter, nil
}

// -----------------------------------------------------------------------------
// IDXGIFactory4 methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (f *IDXGIFactory4) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		f.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(f)),
		0, 0,
	)
	return uint32(ret)
}

// EnumAdapters1 enumerates local adapters.
func (f *IDXGIFactory4) EnumAdapters1(index uint32) (*IDXGIAdapter1, error) {
	var adapter *IDXGIAdapter1

	ret, _, _ := syscall.Syscall(
		f.vtbl.EnumAdapters1,
		3,
		uintptr(unsafe.Pointer(f)),
		uintptr(index),
		uintptr(unsafe.Pointer(&adapter)),
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return adapter, nil
}

// EnumAdapterByLuid enumerates a specific adapter by LUID.
func (f *IDXGIFactory4) EnumAdapterByLuid(luid LUID) (*IDXGIAdapter1, error) {
	var adapter *IDXGIAdapter1

	ret, _, _ := syscall.Syscall6(
		f.vtbl.EnumAdapterByLuid,
		4,
		uintptr(unsafe.Pointer(f)),
		uintptr(*(*uint64)(unsafe.Pointer(&luid))), // LUID as 64-bit value
		uintptr(unsafe.Pointer(&IID_IDXGIAdapter1)),
		uintptr(unsafe.Pointer(&adapter)),
		0, 0,
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return adapter, nil
}

// EnumWarpAdapter enumerates the WARP (software) adapter.
func (f *IDXGIFactory4) EnumWarpAdapter() (*IDXGIAdapter1, error) {
	var adapter *IDXGIAdapter1

	ret, _, _ := syscall.Syscall(
		f.vtbl.EnumWarpAdapter,
		3,
		uintptr(unsafe.Pointer(f)),
		uintptr(unsafe.Pointer(&IID_IDXGIAdapter1)),
		uintptr(unsafe.Pointer(&adapter)),
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return adapter, nil
}

// MakeWindowAssociation associates a window with the factory.
func (f *IDXGIFactory4) MakeWindowAssociation(hwnd uintptr, flags DXGI_MWA) error {
	ret, _, _ := syscall.Syscall(
		f.vtbl.MakeWindowAssociation,
		3,
		uintptr(unsafe.Pointer(f)),
		hwnd,
		uintptr(flags),
	)

	if ret != 0 {
		return d3d12.HRESULTError(ret)
	}
	return nil
}

// CreateSwapChainForHwnd creates a swap chain for an HWND.
func (f *IDXGIFactory4) CreateSwapChainForHwnd(
	device unsafe.Pointer, // ID3D12CommandQueue or other
	hwnd uintptr,
	desc *DXGI_SWAP_CHAIN_DESC1,
	fullscreenDesc *DXGI_SWAP_CHAIN_FULLSCREEN_DESC,
	restrictToOutput *IDXGIOutput,
) (*IDXGISwapChain1, error) {
	var swapChain *IDXGISwapChain1

	ret, _, _ := syscall.Syscall9(
		f.vtbl.CreateSwapChainForHwnd,
		7,
		uintptr(unsafe.Pointer(f)),
		uintptr(device),
		hwnd,
		uintptr(unsafe.Pointer(desc)),
		uintptr(unsafe.Pointer(fullscreenDesc)),
		uintptr(unsafe.Pointer(restrictToOutput)),
		uintptr(unsafe.Pointer(&swapChain)),
		0, 0,
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return swapChain, nil
}

// -----------------------------------------------------------------------------
// IDXGIFactory6 methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (f *IDXGIFactory6) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		f.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(f)),
		0, 0,
	)
	return uint32(ret)
}

// EnumAdapters1 enumerates local adapters.
func (f *IDXGIFactory6) EnumAdapters1(index uint32) (*IDXGIAdapter1, error) {
	var adapter *IDXGIAdapter1

	ret, _, _ := syscall.Syscall(
		f.vtbl.EnumAdapters1,
		3,
		uintptr(unsafe.Pointer(f)),
		uintptr(index),
		uintptr(unsafe.Pointer(&adapter)),
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return adapter, nil
}

// EnumAdapterByGpuPreference enumerates adapters by GPU preference.
func (f *IDXGIFactory6) EnumAdapterByGpuPreference(
	index uint32,
	preference DXGI_GPU_PREFERENCE,
) (*IDXGIAdapter4, error) {
	var adapter *IDXGIAdapter4

	ret, _, _ := syscall.Syscall6(
		f.vtbl.EnumAdapterByGpuPreference,
		5,
		uintptr(unsafe.Pointer(f)),
		uintptr(index),
		uintptr(preference),
		uintptr(unsafe.Pointer(&IID_IDXGIAdapter4)),
		uintptr(unsafe.Pointer(&adapter)),
		0,
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return adapter, nil
}

// EnumAdapterByLuid enumerates a specific adapter by LUID.
func (f *IDXGIFactory6) EnumAdapterByLuid(luid LUID) (*IDXGIAdapter4, error) {
	var adapter *IDXGIAdapter4

	ret, _, _ := syscall.Syscall6(
		f.vtbl.EnumAdapterByLuid,
		4,
		uintptr(unsafe.Pointer(f)),
		uintptr(*(*uint64)(unsafe.Pointer(&luid))), // LUID as 64-bit value
		uintptr(unsafe.Pointer(&IID_IDXGIAdapter4)),
		uintptr(unsafe.Pointer(&adapter)),
		0, 0,
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return adapter, nil
}

// EnumWarpAdapter enumerates the WARP (software) adapter.
func (f *IDXGIFactory6) EnumWarpAdapter() (*IDXGIAdapter4, error) {
	var adapter *IDXGIAdapter4

	ret, _, _ := syscall.Syscall(
		f.vtbl.EnumWarpAdapter,
		3,
		uintptr(unsafe.Pointer(f)),
		uintptr(unsafe.Pointer(&IID_IDXGIAdapter4)),
		uintptr(unsafe.Pointer(&adapter)),
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return adapter, nil
}

// MakeWindowAssociation associates a window with the factory.
func (f *IDXGIFactory6) MakeWindowAssociation(hwnd uintptr, flags DXGI_MWA) error {
	ret, _, _ := syscall.Syscall(
		f.vtbl.MakeWindowAssociation,
		3,
		uintptr(unsafe.Pointer(f)),
		hwnd,
		uintptr(flags),
	)

	if ret != 0 {
		return d3d12.HRESULTError(ret)
	}
	return nil
}

// CreateSwapChainForHwnd creates a swap chain for an HWND.
func (f *IDXGIFactory6) CreateSwapChainForHwnd(
	device unsafe.Pointer, // ID3D12CommandQueue or other
	hwnd uintptr,
	desc *DXGI_SWAP_CHAIN_DESC1,
	fullscreenDesc *DXGI_SWAP_CHAIN_FULLSCREEN_DESC,
	restrictToOutput *IDXGIOutput,
) (*IDXGISwapChain1, error) {
	var swapChain *IDXGISwapChain1

	ret, _, _ := syscall.Syscall9(
		f.vtbl.CreateSwapChainForHwnd,
		7,
		uintptr(unsafe.Pointer(f)),
		uintptr(device),
		hwnd,
		uintptr(unsafe.Pointer(desc)),
		uintptr(unsafe.Pointer(fullscreenDesc)),
		uintptr(unsafe.Pointer(restrictToOutput)),
		uintptr(unsafe.Pointer(&swapChain)),
		0, 0,
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return swapChain, nil
}

// CheckFeatureSupport checks for DXGI feature support.
func (f *IDXGIFactory6) CheckFeatureSupport(feature DXGI_FEATURE, featureData unsafe.Pointer, featureDataSize uint32) error {
	ret, _, _ := syscall.Syscall6(
		f.vtbl.CheckFeatureSupport,
		4,
		uintptr(unsafe.Pointer(f)),
		uintptr(feature),
		uintptr(featureData),
		uintptr(featureDataSize),
		0, 0,
	)

	if ret != 0 {
		return d3d12.HRESULTError(ret)
	}
	return nil
}

// -----------------------------------------------------------------------------
// IDXGIAdapter1 methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (a *IDXGIAdapter1) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		a.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(a)),
		0, 0,
	)
	return uint32(ret)
}

// GetDesc1 returns the adapter description.
func (a *IDXGIAdapter1) GetDesc1() (DXGI_ADAPTER_DESC1, error) {
	var desc DXGI_ADAPTER_DESC1

	ret, _, _ := syscall.Syscall(
		a.vtbl.GetDesc1,
		2,
		uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(&desc)),
		0,
	)

	if ret != 0 {
		return desc, d3d12.HRESULTError(ret)
	}
	return desc, nil
}

// EnumOutputs enumerates the adapter outputs.
func (a *IDXGIAdapter1) EnumOutputs(index uint32) (*IDXGIOutput, error) {
	var output *IDXGIOutput

	ret, _, _ := syscall.Syscall(
		a.vtbl.EnumOutputs,
		3,
		uintptr(unsafe.Pointer(a)),
		uintptr(index),
		uintptr(unsafe.Pointer(&output)),
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return output, nil
}

// CheckInterfaceSupport checks if the adapter supports a specific interface.
// Returns the driver version if supported.
func (a *IDXGIAdapter1) CheckInterfaceSupport(interfaceName *GUID) (int64, error) {
	var version int64

	ret, _, _ := syscall.Syscall(
		a.vtbl.CheckInterfaceSupport,
		3,
		uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(interfaceName)),
		uintptr(unsafe.Pointer(&version)),
	)

	if ret != 0 {
		return 0, d3d12.HRESULTError(ret)
	}
	return version, nil
}

// -----------------------------------------------------------------------------
// IDXGIAdapter4 methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (a *IDXGIAdapter4) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		a.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(a)),
		0, 0,
	)
	return uint32(ret)
}

// GetDesc1 returns the adapter description.
func (a *IDXGIAdapter4) GetDesc1() (DXGI_ADAPTER_DESC1, error) {
	var desc DXGI_ADAPTER_DESC1

	ret, _, _ := syscall.Syscall(
		a.vtbl.GetDesc1,
		2,
		uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(&desc)),
		0,
	)

	if ret != 0 {
		return desc, d3d12.HRESULTError(ret)
	}
	return desc, nil
}

// GetDesc3 returns the extended adapter description.
func (a *IDXGIAdapter4) GetDesc3() (DXGI_ADAPTER_DESC3, error) {
	var desc DXGI_ADAPTER_DESC3

	ret, _, _ := syscall.Syscall(
		a.vtbl.GetDesc3,
		2,
		uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(&desc)),
		0,
	)

	if ret != 0 {
		return desc, d3d12.HRESULTError(ret)
	}
	return desc, nil
}

// EnumOutputs enumerates the adapter outputs.
func (a *IDXGIAdapter4) EnumOutputs(index uint32) (*IDXGIOutput, error) {
	var output *IDXGIOutput

	ret, _, _ := syscall.Syscall(
		a.vtbl.EnumOutputs,
		3,
		uintptr(unsafe.Pointer(a)),
		uintptr(index),
		uintptr(unsafe.Pointer(&output)),
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return output, nil
}

// CheckInterfaceSupport checks if the adapter supports a specific interface.
func (a *IDXGIAdapter4) CheckInterfaceSupport(interfaceName *GUID) (int64, error) {
	var version int64

	ret, _, _ := syscall.Syscall(
		a.vtbl.CheckInterfaceSupport,
		3,
		uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(interfaceName)),
		uintptr(unsafe.Pointer(&version)),
	)

	if ret != 0 {
		return 0, d3d12.HRESULTError(ret)
	}
	return version, nil
}

// -----------------------------------------------------------------------------
// IDXGIOutput methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (o *IDXGIOutput) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		o.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(o)),
		0, 0,
	)
	return uint32(ret)
}

// GetDesc returns the output description.
func (o *IDXGIOutput) GetDesc() (DXGI_OUTPUT_DESC, error) {
	var desc DXGI_OUTPUT_DESC

	ret, _, _ := syscall.Syscall(
		o.vtbl.GetDesc,
		2,
		uintptr(unsafe.Pointer(o)),
		uintptr(unsafe.Pointer(&desc)),
		0,
	)

	if ret != 0 {
		return desc, d3d12.HRESULTError(ret)
	}
	return desc, nil
}

// WaitForVBlank waits for the next vertical blank period.
func (o *IDXGIOutput) WaitForVBlank() error {
	ret, _, _ := syscall.Syscall(
		o.vtbl.WaitForVBlank,
		1,
		uintptr(unsafe.Pointer(o)),
		0, 0,
	)

	if ret != 0 {
		return d3d12.HRESULTError(ret)
	}
	return nil
}

// -----------------------------------------------------------------------------
// IDXGISwapChain1 methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (s *IDXGISwapChain1) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		s.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(s)),
		0, 0,
	)
	return uint32(ret)
}

// QueryInterface queries for IDXGISwapChain4 interface.
func (s *IDXGISwapChain1) QueryInterface() (*IDXGISwapChain4, error) {
	var swapchain4 *IDXGISwapChain4

	ret, _, _ := syscall.Syscall(
		s.vtbl.QueryInterface,
		3,
		uintptr(unsafe.Pointer(s)),
		uintptr(unsafe.Pointer(&IID_IDXGISwapChain4)),
		uintptr(unsafe.Pointer(&swapchain4)),
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return swapchain4, nil
}

// Present presents a rendered frame.
func (s *IDXGISwapChain1) Present(syncInterval, flags uint32) error {
	ret, _, _ := syscall.Syscall(
		s.vtbl.Present,
		3,
		uintptr(unsafe.Pointer(s)),
		uintptr(syncInterval),
		uintptr(flags),
	)

	if ret != 0 {
		return d3d12.HRESULTError(ret)
	}
	return nil
}

// GetBuffer retrieves a back buffer from the swap chain.
func (s *IDXGISwapChain1) GetBuffer(index uint32, riid *GUID) (unsafe.Pointer, error) {
	var resource unsafe.Pointer

	ret, _, _ := syscall.Syscall6(
		s.vtbl.GetBuffer,
		4,
		uintptr(unsafe.Pointer(s)),
		uintptr(index),
		uintptr(unsafe.Pointer(riid)),
		uintptr(unsafe.Pointer(&resource)),
		0, 0,
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return resource, nil
}

// ResizeBuffers resizes the swap chain buffers.
func (s *IDXGISwapChain1) ResizeBuffers(bufferCount, width, height uint32, format DXGI_FORMAT, flags uint32) error {
	ret, _, _ := syscall.Syscall6(
		s.vtbl.ResizeBuffers,
		6,
		uintptr(unsafe.Pointer(s)),
		uintptr(bufferCount),
		uintptr(width),
		uintptr(height),
		uintptr(format),
		uintptr(flags),
	)

	if ret != 0 {
		return d3d12.HRESULTError(ret)
	}
	return nil
}

// GetDesc1 returns the swap chain description.
func (s *IDXGISwapChain1) GetDesc1() (DXGI_SWAP_CHAIN_DESC1, error) {
	var desc DXGI_SWAP_CHAIN_DESC1

	ret, _, _ := syscall.Syscall(
		s.vtbl.GetDesc1,
		2,
		uintptr(unsafe.Pointer(s)),
		uintptr(unsafe.Pointer(&desc)),
		0,
	)

	if ret != 0 {
		return desc, d3d12.HRESULTError(ret)
	}
	return desc, nil
}

// SetFullscreenState sets the swap chain fullscreen state.
func (s *IDXGISwapChain1) SetFullscreenState(fullscreen int32, target *IDXGIOutput) error {
	ret, _, _ := syscall.Syscall(
		s.vtbl.SetFullscreenState,
		3,
		uintptr(unsafe.Pointer(s)),
		uintptr(fullscreen),
		uintptr(unsafe.Pointer(target)),
	)

	if ret != 0 {
		return d3d12.HRESULTError(ret)
	}
	return nil
}

// GetFullscreenState returns the fullscreen state.
func (s *IDXGISwapChain1) GetFullscreenState() (bool, *IDXGIOutput, error) {
	var fullscreen int32
	var target *IDXGIOutput

	ret, _, _ := syscall.Syscall(
		s.vtbl.GetFullscreenState,
		3,
		uintptr(unsafe.Pointer(s)),
		uintptr(unsafe.Pointer(&fullscreen)),
		uintptr(unsafe.Pointer(&target)),
	)

	if ret != 0 {
		return false, nil, d3d12.HRESULTError(ret)
	}
	return fullscreen != 0, target, nil
}

// -----------------------------------------------------------------------------
// IDXGISwapChain3 methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (s *IDXGISwapChain3) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		s.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(s)),
		0, 0,
	)
	return uint32(ret)
}

// Present presents a rendered frame.
func (s *IDXGISwapChain3) Present(syncInterval, flags uint32) error {
	ret, _, _ := syscall.Syscall(
		s.vtbl.Present,
		3,
		uintptr(unsafe.Pointer(s)),
		uintptr(syncInterval),
		uintptr(flags),
	)

	if ret != 0 {
		return d3d12.HRESULTError(ret)
	}
	return nil
}

// GetBuffer retrieves a back buffer from the swap chain.
func (s *IDXGISwapChain3) GetBuffer(index uint32, riid *GUID) (unsafe.Pointer, error) {
	var resource unsafe.Pointer

	ret, _, _ := syscall.Syscall6(
		s.vtbl.GetBuffer,
		4,
		uintptr(unsafe.Pointer(s)),
		uintptr(index),
		uintptr(unsafe.Pointer(riid)),
		uintptr(unsafe.Pointer(&resource)),
		0, 0,
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return resource, nil
}

// ResizeBuffers resizes the swap chain buffers.
func (s *IDXGISwapChain3) ResizeBuffers(bufferCount, width, height uint32, format DXGI_FORMAT, flags uint32) error {
	ret, _, _ := syscall.Syscall6(
		s.vtbl.ResizeBuffers,
		6,
		uintptr(unsafe.Pointer(s)),
		uintptr(bufferCount),
		uintptr(width),
		uintptr(height),
		uintptr(format),
		uintptr(flags),
	)

	if ret != 0 {
		return d3d12.HRESULTError(ret)
	}
	return nil
}

// GetCurrentBackBufferIndex returns the index of the current back buffer.
func (s *IDXGISwapChain3) GetCurrentBackBufferIndex() uint32 {
	ret, _, _ := syscall.Syscall(
		s.vtbl.GetCurrentBackBufferIndex,
		1,
		uintptr(unsafe.Pointer(s)),
		0, 0,
	)
	return uint32(ret)
}

// GetDesc1 returns the swap chain description.
func (s *IDXGISwapChain3) GetDesc1() (DXGI_SWAP_CHAIN_DESC1, error) {
	var desc DXGI_SWAP_CHAIN_DESC1

	ret, _, _ := syscall.Syscall(
		s.vtbl.GetDesc1,
		2,
		uintptr(unsafe.Pointer(s)),
		uintptr(unsafe.Pointer(&desc)),
		0,
	)

	if ret != 0 {
		return desc, d3d12.HRESULTError(ret)
	}
	return desc, nil
}

// SetMaximumFrameLatency sets the maximum frame latency.
func (s *IDXGISwapChain3) SetMaximumFrameLatency(maxLatency uint32) error {
	ret, _, _ := syscall.Syscall(
		s.vtbl.SetMaximumFrameLatency,
		2,
		uintptr(unsafe.Pointer(s)),
		uintptr(maxLatency),
		0,
	)

	if ret != 0 {
		return d3d12.HRESULTError(ret)
	}
	return nil
}

// GetFrameLatencyWaitableObject returns a waitable object for frame latency.
func (s *IDXGISwapChain3) GetFrameLatencyWaitableObject() uintptr {
	ret, _, _ := syscall.Syscall(
		s.vtbl.GetFrameLatencyWaitableObject,
		1,
		uintptr(unsafe.Pointer(s)),
		0, 0,
	)
	return ret
}

// -----------------------------------------------------------------------------
// IDXGISwapChain4 methods
// -----------------------------------------------------------------------------

// Release decrements the reference count.
func (s *IDXGISwapChain4) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		s.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(s)),
		0, 0,
	)
	return uint32(ret)
}

// Present presents a rendered frame.
func (s *IDXGISwapChain4) Present(syncInterval, flags uint32) error {
	ret, _, _ := syscall.Syscall(
		s.vtbl.Present,
		3,
		uintptr(unsafe.Pointer(s)),
		uintptr(syncInterval),
		uintptr(flags),
	)

	if ret != 0 {
		return d3d12.HRESULTError(ret)
	}
	return nil
}

// GetBuffer retrieves a back buffer from the swap chain.
func (s *IDXGISwapChain4) GetBuffer(index uint32, riid *GUID) (unsafe.Pointer, error) {
	var resource unsafe.Pointer

	ret, _, _ := syscall.Syscall6(
		s.vtbl.GetBuffer,
		4,
		uintptr(unsafe.Pointer(s)),
		uintptr(index),
		uintptr(unsafe.Pointer(riid)),
		uintptr(unsafe.Pointer(&resource)),
		0, 0,
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return resource, nil
}

// ResizeBuffers resizes the swap chain buffers.
func (s *IDXGISwapChain4) ResizeBuffers(bufferCount, width, height uint32, format DXGI_FORMAT, flags uint32) error {
	ret, _, _ := syscall.Syscall6(
		s.vtbl.ResizeBuffers,
		6,
		uintptr(unsafe.Pointer(s)),
		uintptr(bufferCount),
		uintptr(width),
		uintptr(height),
		uintptr(format),
		uintptr(flags),
	)

	if ret != 0 {
		return d3d12.HRESULTError(ret)
	}
	return nil
}

// GetCurrentBackBufferIndex returns the index of the current back buffer.
func (s *IDXGISwapChain4) GetCurrentBackBufferIndex() uint32 {
	ret, _, _ := syscall.Syscall(
		s.vtbl.GetCurrentBackBufferIndex,
		1,
		uintptr(unsafe.Pointer(s)),
		0, 0,
	)
	return uint32(ret)
}

// GetDesc1 returns the swap chain description.
func (s *IDXGISwapChain4) GetDesc1() (DXGI_SWAP_CHAIN_DESC1, error) {
	var desc DXGI_SWAP_CHAIN_DESC1

	ret, _, _ := syscall.Syscall(
		s.vtbl.GetDesc1,
		2,
		uintptr(unsafe.Pointer(s)),
		uintptr(unsafe.Pointer(&desc)),
		0,
	)

	if ret != 0 {
		return desc, d3d12.HRESULTError(ret)
	}
	return desc, nil
}

// SetMaximumFrameLatency sets the maximum frame latency.
func (s *IDXGISwapChain4) SetMaximumFrameLatency(maxLatency uint32) error {
	ret, _, _ := syscall.Syscall(
		s.vtbl.SetMaximumFrameLatency,
		2,
		uintptr(unsafe.Pointer(s)),
		uintptr(maxLatency),
		0,
	)

	if ret != 0 {
		return d3d12.HRESULTError(ret)
	}
	return nil
}

// GetFrameLatencyWaitableObject returns a waitable object for frame latency.
func (s *IDXGISwapChain4) GetFrameLatencyWaitableObject() uintptr {
	ret, _, _ := syscall.Syscall(
		s.vtbl.GetFrameLatencyWaitableObject,
		1,
		uintptr(unsafe.Pointer(s)),
		0, 0,
	)
	return ret
}

// SetFullscreenState sets the swap chain fullscreen state.
func (s *IDXGISwapChain4) SetFullscreenState(fullscreen int32, target *IDXGIOutput) error {
	ret, _, _ := syscall.Syscall(
		s.vtbl.SetFullscreenState,
		3,
		uintptr(unsafe.Pointer(s)),
		uintptr(fullscreen),
		uintptr(unsafe.Pointer(target)),
	)

	if ret != 0 {
		return d3d12.HRESULTError(ret)
	}
	return nil
}

// GetFullscreenState returns the fullscreen state.
func (s *IDXGISwapChain4) GetFullscreenState() (bool, *IDXGIOutput, error) {
	var fullscreen int32
	var target *IDXGIOutput

	ret, _, _ := syscall.Syscall(
		s.vtbl.GetFullscreenState,
		3,
		uintptr(unsafe.Pointer(s)),
		uintptr(unsafe.Pointer(&fullscreen)),
		uintptr(unsafe.Pointer(&target)),
	)

	if ret != 0 {
		return false, nil, d3d12.HRESULTError(ret)
	}
	return fullscreen != 0, target, nil
}
