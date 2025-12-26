// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package dxgi

// IDXGIObject is the base interface for DXGI objects.
type IDXGIObject struct {
	vtbl *idxgiObjectVtbl
}

type idxgiObjectVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IDXGIObject
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetPrivateData          uintptr
	GetParent               uintptr
}

// IDXGIFactory represents a factory for creating DXGI objects.
type IDXGIFactory struct {
	vtbl *idxgiFactoryVtbl
}

type idxgiFactoryVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IDXGIObject
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetPrivateData          uintptr
	GetParent               uintptr

	// IDXGIFactory
	EnumAdapters          uintptr
	MakeWindowAssociation uintptr
	GetWindowAssociation  uintptr
	CreateSwapChain       uintptr
	CreateSoftwareAdapter uintptr
}

// IDXGIFactory1 extends IDXGIFactory with adapter enumeration.
type IDXGIFactory1 struct {
	vtbl *idxgiFactory1Vtbl
}

type idxgiFactory1Vtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IDXGIObject
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetPrivateData          uintptr
	GetParent               uintptr

	// IDXGIFactory
	EnumAdapters          uintptr
	MakeWindowAssociation uintptr
	GetWindowAssociation  uintptr
	CreateSwapChain       uintptr
	CreateSoftwareAdapter uintptr

	// IDXGIFactory1
	EnumAdapters1 uintptr
	IsCurrent     uintptr
}

// IDXGIFactory2 extends IDXGIFactory1 with swap chain creation for HWND.
type IDXGIFactory2 struct {
	vtbl *idxgiFactory2Vtbl
}

type idxgiFactory2Vtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IDXGIObject
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetPrivateData          uintptr
	GetParent               uintptr

	// IDXGIFactory
	EnumAdapters          uintptr
	MakeWindowAssociation uintptr
	GetWindowAssociation  uintptr
	CreateSwapChain       uintptr
	CreateSoftwareAdapter uintptr

	// IDXGIFactory1
	EnumAdapters1 uintptr
	IsCurrent     uintptr

	// IDXGIFactory2
	IsWindowedStereoEnabled       uintptr
	CreateSwapChainForHwnd        uintptr
	CreateSwapChainForCoreWindow  uintptr
	GetSharedResourceAdapterLuid  uintptr
	RegisterStereoStatusWindow    uintptr
	RegisterStereoStatusEvent     uintptr
	UnregisterStereoStatus        uintptr
	RegisterOcclusionStatusWindow uintptr
	RegisterOcclusionStatusEvent  uintptr
	UnregisterOcclusionStatus     uintptr
	CreateSwapChainForComposition uintptr
}

// IDXGIFactory4 extends IDXGIFactory3 with adapter enumeration by LUID.
type IDXGIFactory4 struct {
	vtbl *idxgiFactory4Vtbl
}

type idxgiFactory4Vtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IDXGIObject
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetPrivateData          uintptr
	GetParent               uintptr

	// IDXGIFactory
	EnumAdapters          uintptr
	MakeWindowAssociation uintptr
	GetWindowAssociation  uintptr
	CreateSwapChain       uintptr
	CreateSoftwareAdapter uintptr

	// IDXGIFactory1
	EnumAdapters1 uintptr
	IsCurrent     uintptr

	// IDXGIFactory2
	IsWindowedStereoEnabled       uintptr
	CreateSwapChainForHwnd        uintptr
	CreateSwapChainForCoreWindow  uintptr
	GetSharedResourceAdapterLuid  uintptr
	RegisterStereoStatusWindow    uintptr
	RegisterStereoStatusEvent     uintptr
	UnregisterStereoStatus        uintptr
	RegisterOcclusionStatusWindow uintptr
	RegisterOcclusionStatusEvent  uintptr
	UnregisterOcclusionStatus     uintptr
	CreateSwapChainForComposition uintptr

	// IDXGIFactory3
	GetCreationFlags uintptr

	// IDXGIFactory4
	EnumAdapterByLuid uintptr
	EnumWarpAdapter   uintptr
}

// IDXGIFactory5 extends IDXGIFactory4 with feature support check.
type IDXGIFactory5 struct {
	vtbl *idxgiFactory5Vtbl
}

type idxgiFactory5Vtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IDXGIObject
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetPrivateData          uintptr
	GetParent               uintptr

	// IDXGIFactory
	EnumAdapters          uintptr
	MakeWindowAssociation uintptr
	GetWindowAssociation  uintptr
	CreateSwapChain       uintptr
	CreateSoftwareAdapter uintptr

	// IDXGIFactory1
	EnumAdapters1 uintptr
	IsCurrent     uintptr

	// IDXGIFactory2
	IsWindowedStereoEnabled       uintptr
	CreateSwapChainForHwnd        uintptr
	CreateSwapChainForCoreWindow  uintptr
	GetSharedResourceAdapterLuid  uintptr
	RegisterStereoStatusWindow    uintptr
	RegisterStereoStatusEvent     uintptr
	UnregisterStereoStatus        uintptr
	RegisterOcclusionStatusWindow uintptr
	RegisterOcclusionStatusEvent  uintptr
	UnregisterOcclusionStatus     uintptr
	CreateSwapChainForComposition uintptr

	// IDXGIFactory3
	GetCreationFlags uintptr

	// IDXGIFactory4
	EnumAdapterByLuid uintptr
	EnumWarpAdapter   uintptr

	// IDXGIFactory5
	CheckFeatureSupport uintptr
}

// IDXGIFactory6 extends IDXGIFactory5 with GPU preference enumeration.
type IDXGIFactory6 struct {
	vtbl *idxgiFactory6Vtbl
}

type idxgiFactory6Vtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IDXGIObject
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetPrivateData          uintptr
	GetParent               uintptr

	// IDXGIFactory
	EnumAdapters          uintptr
	MakeWindowAssociation uintptr
	GetWindowAssociation  uintptr
	CreateSwapChain       uintptr
	CreateSoftwareAdapter uintptr

	// IDXGIFactory1
	EnumAdapters1 uintptr
	IsCurrent     uintptr

	// IDXGIFactory2
	IsWindowedStereoEnabled       uintptr
	CreateSwapChainForHwnd        uintptr
	CreateSwapChainForCoreWindow  uintptr
	GetSharedResourceAdapterLuid  uintptr
	RegisterStereoStatusWindow    uintptr
	RegisterStereoStatusEvent     uintptr
	UnregisterStereoStatus        uintptr
	RegisterOcclusionStatusWindow uintptr
	RegisterOcclusionStatusEvent  uintptr
	UnregisterOcclusionStatus     uintptr
	CreateSwapChainForComposition uintptr

	// IDXGIFactory3
	GetCreationFlags uintptr

	// IDXGIFactory4
	EnumAdapterByLuid uintptr
	EnumWarpAdapter   uintptr

	// IDXGIFactory5
	CheckFeatureSupport uintptr

	// IDXGIFactory6
	EnumAdapterByGpuPreference uintptr
}

// IDXGIAdapter represents a display adapter.
type IDXGIAdapter struct {
	vtbl *idxgiAdapterVtbl
}

type idxgiAdapterVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IDXGIObject
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetPrivateData          uintptr
	GetParent               uintptr

	// IDXGIAdapter
	EnumOutputs           uintptr
	GetDesc               uintptr
	CheckInterfaceSupport uintptr
}

// IDXGIAdapter1 extends IDXGIAdapter with GetDesc1.
type IDXGIAdapter1 struct {
	vtbl *idxgiAdapter1Vtbl
}

type idxgiAdapter1Vtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IDXGIObject
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetPrivateData          uintptr
	GetParent               uintptr

	// IDXGIAdapter
	EnumOutputs           uintptr
	GetDesc               uintptr
	CheckInterfaceSupport uintptr

	// IDXGIAdapter1
	GetDesc1 uintptr
}

// IDXGIAdapter4 extends IDXGIAdapter3 with GetDesc3.
type IDXGIAdapter4 struct {
	vtbl *idxgiAdapter4Vtbl
}

type idxgiAdapter4Vtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IDXGIObject
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetPrivateData          uintptr
	GetParent               uintptr

	// IDXGIAdapter
	EnumOutputs           uintptr
	GetDesc               uintptr
	CheckInterfaceSupport uintptr

	// IDXGIAdapter1
	GetDesc1 uintptr

	// IDXGIAdapter2
	GetDesc2 uintptr

	// IDXGIAdapter3
	RegisterHardwareContentProtectionTeardownStatusEvent uintptr
	UnregisterHardwareContentProtectionTeardownStatus    uintptr
	QueryVideoMemoryInfo                                 uintptr
	SetVideoMemoryReservation                            uintptr
	RegisterVideoMemoryBudgetChangeNotificationEvent     uintptr
	UnregisterVideoMemoryBudgetChangeNotification        uintptr

	// IDXGIAdapter4
	GetDesc3 uintptr
}

// IDXGIOutput represents an adapter output (display).
type IDXGIOutput struct {
	vtbl *idxgiOutputVtbl
}

type idxgiOutputVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IDXGIObject
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetPrivateData          uintptr
	GetParent               uintptr

	// IDXGIOutput
	GetDesc                     uintptr
	GetDisplayModeList          uintptr
	FindClosestMatchingMode     uintptr
	WaitForVBlank               uintptr
	TakeOwnership               uintptr
	ReleaseOwnership            uintptr
	GetGammaControlCapabilities uintptr
	SetGammaControl             uintptr
	GetGammaControl             uintptr
	SetDisplaySurface           uintptr
	GetDisplaySurfaceData       uintptr
	GetFrameStatistics          uintptr
}

// IDXGISwapChain represents a swap chain.
type IDXGISwapChain struct {
	vtbl *idxgiSwapChainVtbl
}

type idxgiSwapChainVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IDXGIObject
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetPrivateData          uintptr
	GetParent               uintptr

	// IDXGIDeviceSubObject
	GetDevice uintptr

	// IDXGISwapChain
	Present             uintptr
	GetBuffer           uintptr
	SetFullscreenState  uintptr
	GetFullscreenState  uintptr
	GetDesc             uintptr
	ResizeBuffers       uintptr
	ResizeTarget        uintptr
	GetContainingOutput uintptr
	GetFrameStatistics  uintptr
	GetLastPresentCount uintptr
}

// IDXGISwapChain1 extends IDXGISwapChain with extended presentation.
type IDXGISwapChain1 struct {
	vtbl *idxgiSwapChain1Vtbl
}

type idxgiSwapChain1Vtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IDXGIObject
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetPrivateData          uintptr
	GetParent               uintptr

	// IDXGIDeviceSubObject
	GetDevice uintptr

	// IDXGISwapChain
	Present             uintptr
	GetBuffer           uintptr
	SetFullscreenState  uintptr
	GetFullscreenState  uintptr
	GetDesc             uintptr
	ResizeBuffers       uintptr
	ResizeTarget        uintptr
	GetContainingOutput uintptr
	GetFrameStatistics  uintptr
	GetLastPresentCount uintptr

	// IDXGISwapChain1
	GetDesc1                 uintptr
	GetFullscreenDesc        uintptr
	GetHwnd                  uintptr
	GetCoreWindow            uintptr
	Present1                 uintptr
	IsTemporaryMonoSupported uintptr
	GetRestrictToOutput      uintptr
	SetBackgroundColor       uintptr
	GetBackgroundColor       uintptr
	SetRotation              uintptr
	GetRotation              uintptr
}

// IDXGISwapChain3 extends IDXGISwapChain2 with color space and HDR support.
type IDXGISwapChain3 struct {
	vtbl *idxgiSwapChain3Vtbl
}

type idxgiSwapChain3Vtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IDXGIObject
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetPrivateData          uintptr
	GetParent               uintptr

	// IDXGIDeviceSubObject
	GetDevice uintptr

	// IDXGISwapChain
	Present             uintptr
	GetBuffer           uintptr
	SetFullscreenState  uintptr
	GetFullscreenState  uintptr
	GetDesc             uintptr
	ResizeBuffers       uintptr
	ResizeTarget        uintptr
	GetContainingOutput uintptr
	GetFrameStatistics  uintptr
	GetLastPresentCount uintptr

	// IDXGISwapChain1
	GetDesc1                 uintptr
	GetFullscreenDesc        uintptr
	GetHwnd                  uintptr
	GetCoreWindow            uintptr
	Present1                 uintptr
	IsTemporaryMonoSupported uintptr
	GetRestrictToOutput      uintptr
	SetBackgroundColor       uintptr
	GetBackgroundColor       uintptr
	SetRotation              uintptr
	GetRotation              uintptr

	// IDXGISwapChain2
	SetSourceSize                 uintptr
	GetSourceSize                 uintptr
	SetMaximumFrameLatency        uintptr
	GetMaximumFrameLatency        uintptr
	GetFrameLatencyWaitableObject uintptr
	SetMatrixTransform            uintptr
	GetMatrixTransform            uintptr

	// IDXGISwapChain3
	GetCurrentBackBufferIndex uintptr
	CheckColorSpaceSupport    uintptr
	SetColorSpace1            uintptr
	ResizeBuffers1            uintptr
}

// IDXGISwapChain4 extends IDXGISwapChain3 with HDR metadata.
type IDXGISwapChain4 struct {
	vtbl *idxgiSwapChain4Vtbl
}

type idxgiSwapChain4Vtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IDXGIObject
	SetPrivateData          uintptr
	SetPrivateDataInterface uintptr
	GetPrivateData          uintptr
	GetParent               uintptr

	// IDXGIDeviceSubObject
	GetDevice uintptr

	// IDXGISwapChain
	Present             uintptr
	GetBuffer           uintptr
	SetFullscreenState  uintptr
	GetFullscreenState  uintptr
	GetDesc             uintptr
	ResizeBuffers       uintptr
	ResizeTarget        uintptr
	GetContainingOutput uintptr
	GetFrameStatistics  uintptr
	GetLastPresentCount uintptr

	// IDXGISwapChain1
	GetDesc1                 uintptr
	GetFullscreenDesc        uintptr
	GetHwnd                  uintptr
	GetCoreWindow            uintptr
	Present1                 uintptr
	IsTemporaryMonoSupported uintptr
	GetRestrictToOutput      uintptr
	SetBackgroundColor       uintptr
	GetBackgroundColor       uintptr
	SetRotation              uintptr
	GetRotation              uintptr

	// IDXGISwapChain2
	SetSourceSize                 uintptr
	GetSourceSize                 uintptr
	SetMaximumFrameLatency        uintptr
	GetMaximumFrameLatency        uintptr
	GetFrameLatencyWaitableObject uintptr
	SetMatrixTransform            uintptr
	GetMatrixTransform            uintptr

	// IDXGISwapChain3
	GetCurrentBackBufferIndex uintptr
	CheckColorSpaceSupport    uintptr
	SetColorSpace1            uintptr
	ResizeBuffers1            uintptr

	// IDXGISwapChain4
	SetHDRMetaData uintptr
}
