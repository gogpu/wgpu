// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

// Package dx12 provides the DirectX 12 HAL backend for Windows.
//
// This backend requires Windows 10 (1809+) or Windows 11 and a DirectX 12
// capable GPU. It uses pure Go with syscall for all COM interactions,
// requiring no CGO.
//
// # Instance Creation
//
// The Instance manages DXGI factory creation, debug layer initialization,
// and adapter enumeration. It prefers discrete GPUs when enumerating adapters.
//
// # Debug Layer
//
// When InstanceFlagsDebug is set, the D3D12 debug layer is enabled via
// D3D12GetDebugInterface. This provides detailed validation messages but
// significantly impacts performance. Only use in development.
package dx12

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
	"github.com/gogpu/wgpu/hal/dx12/dxgi"
)

// Backend implements hal.Backend for DirectX 12.
type Backend struct{}

// Variant returns the backend type identifier.
func (Backend) Variant() gputypes.Backend {
	return gputypes.BackendDX12
}

// CreateInstance creates a new DirectX 12 instance.
func (Backend) CreateInstance(desc *hal.InstanceDescriptor) (hal.Instance, error) {
	instance := &Instance{}

	// Load DXGI library first
	dxgiLib, err := dxgi.LoadDXGI()
	if err != nil {
		return nil, fmt.Errorf("dx12: failed to load dxgi.dll: %w", err)
	}
	instance.dxgiLib = dxgiLib

	// Determine factory creation flags
	var factoryFlags uint32
	if desc != nil && desc.Flags&gputypes.InstanceFlagsDebug != 0 {
		factoryFlags |= dxgi.DXGI_CREATE_FACTORY_DEBUG
		instance.flags = desc.Flags
	}

	// Create DXGI factory
	factory, err := dxgiLib.CreateFactory2(factoryFlags)
	if err != nil {
		return nil, fmt.Errorf("dx12: failed to create DXGI factory: %w", err)
	}
	instance.factory = factory

	// Load D3D12 library
	d3d12Lib, err := d3d12.LoadD3D12()
	if err != nil {
		factory.Release()
		return nil, fmt.Errorf("dx12: failed to load d3d12.dll: %w", err)
	}
	instance.d3d12Lib = d3d12Lib

	// Enable debug layer if requested
	if desc != nil && desc.Flags&gputypes.InstanceFlagsDebug != 0 {
		if err := instance.enableDebugLayer(); err != nil {
			// Debug layer is optional; log but don't fail
			// In production, we might want to use a logger here
			_ = err
		}
	}

	// Check for tearing support (DXGI 1.5 feature)
	instance.checkTearingSupport()

	// Set a finalizer to ensure cleanup
	runtime.SetFinalizer(instance, (*Instance).Destroy)

	return instance, nil
}

// Instance implements hal.Instance for DirectX 12.
type Instance struct {
	factory      *dxgi.IDXGIFactory6
	d3d12Lib     *d3d12.D3D12Lib
	dxgiLib      *dxgi.DXGILib
	debugLayer   *d3d12.ID3D12Debug
	allowTearing bool
	flags        gputypes.InstanceFlags
}

// enableDebugLayer enables the D3D12 debug layer for validation.
func (i *Instance) enableDebugLayer() error {
	debug, err := i.d3d12Lib.GetDebugInterface()
	if err != nil {
		return fmt.Errorf("dx12: D3D12GetDebugInterface failed: %w", err)
	}

	debug.EnableDebugLayer()
	i.debugLayer = debug

	// Try to get ID3D12Debug1 for GPU-based validation
	debug1, err := i.d3d12Lib.GetDebugInterface1()
	if err == nil {
		// GPU-based validation is very slow but catches more errors
		// Only enable if explicitly requested (future: add separate flag)
		debug1.SetEnableGPUBasedValidation(false)
		debug1.Release()
	}

	return nil
}

// checkTearingSupport checks if variable refresh rate is supported.
func (i *Instance) checkTearingSupport() {
	// Query DXGI 1.5 feature for tearing support
	var allowTearing int32
	err := i.factory.CheckFeatureSupport(
		dxgi.DXGI_FEATURE_PRESENT_ALLOW_TEARING,
		unsafe.Pointer(&allowTearing),
		4, // sizeof(BOOL)
	)
	if err == nil && allowTearing != 0 {
		i.allowTearing = true
	}
}

// CreateSurface creates a rendering surface from platform handles.
// displayHandle is not used on Windows (can be 0).
// windowHandle must be a valid HWND.
func (i *Instance) CreateSurface(displayHandle, windowHandle uintptr) (hal.Surface, error) {
	if windowHandle == 0 {
		return nil, fmt.Errorf("dx12: windowHandle (HWND) is required")
	}

	return &Surface{
		instance: i,
		hwnd:     windowHandle,
	}, nil
}

// EnumerateAdapters enumerates available physical GPUs.
// Adapters are sorted by preference: discrete GPUs first, then integrated,
// then others. Software adapters (WARP) are excluded unless explicitly requested.
func (i *Instance) EnumerateAdapters(surfaceHint hal.Surface) []hal.ExposedAdapter {
	var adapters []hal.ExposedAdapter

	// Enumerate adapters by GPU preference (high performance first)
	for idx := uint32(0); ; idx++ {
		raw, err := i.factory.EnumAdapterByGpuPreference(
			idx, dxgi.DXGI_GPU_PREFERENCE_HIGH_PERFORMANCE)
		if err != nil {
			// No more adapters or factory doesn't support EnumAdapterByGpuPreference
			if idx == 0 {
				// Fall back to legacy enumeration
				adapters = i.enumerateAdaptersLegacy(surfaceHint)
			}
			break
		}

		// Get adapter description
		desc, err := raw.GetDesc1()
		if err != nil {
			raw.Release()
			continue
		}

		// Skip software adapters unless debugging
		if desc.Flags&dxgi.DXGI_ADAPTER_FLAG_SOFTWARE != 0 {
			raw.Release()
			continue
		}

		adapter := &Adapter{
			raw:      raw,
			desc:     desc,
			instance: i,
		}

		// Probe adapter capabilities by creating a temporary device
		if err := adapter.probeCapabilities(); err != nil {
			raw.Release()
			continue
		}

		exposed := adapter.toExposedAdapter()
		adapters = append(adapters, exposed)
	}

	return adapters
}

// enumerateAdaptersLegacy uses the legacy IDXGIFactory1 enumeration method.
// This is a fallback for older Windows versions or when EnumAdapterByGpuPreference fails.
func (i *Instance) enumerateAdaptersLegacy(surfaceHint hal.Surface) []hal.ExposedAdapter {
	_ = surfaceHint // Surface hint not used in DX12; all adapters support all surfaces

	var adapters []hal.ExposedAdapter //nolint:prealloc // Size unknown until enumeration
	var discreteAdapters []hal.ExposedAdapter
	var integratedAdapters []hal.ExposedAdapter
	var otherAdapters []hal.ExposedAdapter

	for idx := uint32(0); ; idx++ {
		raw, err := i.factory.EnumAdapters1(idx)
		if err != nil {
			break
		}

		desc, err := raw.GetDesc1()
		if err != nil {
			raw.Release()
			continue
		}

		// Skip software adapters
		if desc.Flags&dxgi.DXGI_ADAPTER_FLAG_SOFTWARE != 0 {
			raw.Release()
			continue
		}

		adapter := &AdapterLegacy{
			raw:      raw,
			desc:     desc,
			instance: i,
		}

		if err := adapter.probeCapabilities(); err != nil {
			raw.Release()
			continue
		}

		exposed := adapter.toExposedAdapter()

		// Sort by device type for proper preference ordering
		switch exposed.Info.DeviceType {
		case gputypes.DeviceTypeDiscreteGPU:
			discreteAdapters = append(discreteAdapters, exposed)
		case gputypes.DeviceTypeIntegratedGPU:
			integratedAdapters = append(integratedAdapters, exposed)
		default:
			otherAdapters = append(otherAdapters, exposed)
		}
	}

	// Combine in preference order: discrete > integrated > other
	adapters = append(adapters, discreteAdapters...)
	adapters = append(adapters, integratedAdapters...)
	adapters = append(adapters, otherAdapters...)

	return adapters
}

// Destroy releases the DirectX 12 instance and all associated resources.
func (i *Instance) Destroy() {
	if i == nil {
		return
	}

	// Clear finalizer to prevent double-free
	runtime.SetFinalizer(i, nil)

	if i.debugLayer != nil {
		i.debugLayer.Release()
		i.debugLayer = nil
	}

	if i.factory != nil {
		i.factory.Release()
		i.factory = nil
	}

	// Libraries are unloaded when their handles go out of scope
	// via their own finalizers
	i.d3d12Lib = nil
	i.dxgiLib = nil
}

// AllowsTearing returns whether variable refresh rate (tearing) is supported.
func (i *Instance) AllowsTearing() bool {
	return i.allowTearing
}

// Surface implements hal.Surface for DirectX 12.
type Surface struct {
	instance *Instance
	hwnd     uintptr
	device   *Device

	// Swapchain state
	swapchain      *dxgi.IDXGISwapChain4
	backBuffers    []backBuffer
	width          uint32
	height         uint32
	format         dxgi.DXGI_FORMAT
	halFormat      gputypes.TextureFormat
	presentMode    hal.PresentMode
	swapchainFlags uint32
	allowTearing   bool
}

// Configure configures the surface for presentation.
func (s *Surface) Configure(device hal.Device, config *hal.SurfaceConfiguration) error {
	if config.Width == 0 || config.Height == 0 {
		return hal.ErrZeroArea
	}

	// Type assert to DX12 Device
	dx12Device, ok := device.(*Device)
	if !ok {
		return fmt.Errorf("dx12: device is not a DX12 device")
	}

	// If we already have a swapchain with the same device, resize it
	if s.swapchain != nil && s.device == dx12Device {
		return s.resizeSwapchain(config)
	}

	// Destroy old swapchain if switching devices
	s.Unconfigure(device)

	// Create new swapchain (implementation in surface.go)
	return s.createSwapchain(dx12Device, config)
}

// Unconfigure removes surface configuration.
func (s *Surface) Unconfigure(_ hal.Device) {
	// Wait for GPU to finish using the swapchain
	if s.device != nil {
		_ = s.device.waitForGPU()
	}

	// Release back buffer resources
	s.releaseBackBuffers()

	// Release swapchain
	if s.swapchain != nil {
		s.swapchain.Release()
		s.swapchain = nil
	}

	s.device = nil
	s.width = 0
	s.height = 0
}

// AcquireTexture acquires the next surface texture for rendering.
func (s *Surface) AcquireTexture(_ hal.Fence) (*hal.AcquiredSurfaceTexture, error) {
	if s.swapchain == nil {
		return nil, fmt.Errorf("dx12: surface not configured")
	}

	// Get current back buffer index
	index := s.swapchain.GetCurrentBackBufferIndex()

	if int(index) >= len(s.backBuffers) {
		return nil, fmt.Errorf("dx12: back buffer index %d out of range", index)
	}

	bb := &s.backBuffers[index]

	// Create surface texture wrapper
	surfaceTexture := &SurfaceTexture{
		surface:   s,
		index:     index,
		resource:  bb.resource,
		rtvHandle: bb.rtvHandle,
		format:    s.halFormat,
		width:     s.width,
		height:    s.height,
		// Note: Suboptimal detection requires DXGI_STATUS_OCCLUDED/DXGI_ERROR_DEVICE_REMOVED checks.
		suboptimal: false,
	}

	return &hal.AcquiredSurfaceTexture{
		Texture:    surfaceTexture,
		Suboptimal: surfaceTexture.suboptimal,
	}, nil
}

// DiscardTexture discards a surface texture without presenting it.
func (s *Surface) DiscardTexture(_ hal.SurfaceTexture) {
	// For DX12 with flip model swapchains, discarding is a no-op.
	// The texture is simply not presented, and the swapchain will
	// continue to use the existing frame.
}

// Destroy releases the surface.
func (s *Surface) Destroy() {
	s.Unconfigure(nil)
}

// Compile-time interface assertions.
var (
	_ hal.Backend  = Backend{}
	_ hal.Instance = (*Instance)(nil)
	_ hal.Surface  = (*Surface)(nil)
)
