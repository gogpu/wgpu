// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package dx12

import (
	"fmt"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
	"github.com/gogpu/wgpu/hal/dx12/dxgi"
)

// defaultBufferCount is the default number of back buffers in the swapchain.
const defaultBufferCount = 2

// maxFrameLatency is the maximum number of frames that can be queued.
const maxFrameLatency = 2

// backBuffer represents a back buffer resource and its RTV.
type backBuffer struct {
	resource  *d3d12.ID3D12Resource
	rtvHandle d3d12.D3D12_CPU_DESCRIPTOR_HANDLE
	rtvIndex  uint32 // Heap index for recycling on release
}

// createSwapchain creates a new DXGI swapchain for the surface.
func (s *Surface) createSwapchain(device *Device, config *hal.SurfaceConfiguration) error {
	// Store device reference
	s.device = device

	// Determine DXGI format
	format := textureFormatToDXGI(config.Format)
	if format == dxgi.DXGI_FORMAT_UNKNOWN {
		return fmt.Errorf("dx12: unsupported surface format: %v", config.Format)
	}
	s.format = format
	s.halFormat = config.Format

	// Determine swapchain flags
	var swapchainFlags uint32
	if s.instance.allowTearing && config.PresentMode == hal.PresentModeImmediate {
		swapchainFlags |= uint32(dxgi.DXGI_SWAP_CHAIN_FLAG_ALLOW_TEARING)
		s.allowTearing = true
	} else {
		s.allowTearing = false
	}
	// Add frame latency waitable object flag for better frame pacing
	swapchainFlags |= uint32(dxgi.DXGI_SWAP_CHAIN_FLAG_FRAME_LATENCY_WAITABLE_OBJECT)

	// Create swapchain description
	desc := dxgi.DXGI_SWAP_CHAIN_DESC1{
		Width:       config.Width,
		Height:      config.Height,
		Format:      format,
		Stereo:      0,
		SampleDesc:  dxgi.DXGI_SAMPLE_DESC{Count: 1, Quality: 0},
		BufferUsage: dxgi.DXGI_USAGE_RENDER_TARGET_OUTPUT,
		BufferCount: defaultBufferCount,
		Scaling:     dxgi.DXGI_SCALING_STRETCH,
		SwapEffect:  dxgi.DXGI_SWAP_EFFECT_FLIP_DISCARD,
		AlphaMode:   compositeAlphaModeToDXGI(config.AlphaMode),
		Flags:       swapchainFlags,
	}

	// Create swapchain using factory and command queue
	swapchain1, err := s.instance.factory.CreateSwapChainForHwnd(
		unsafe.Pointer(device.directQueue),
		s.hwnd,
		&desc,
		nil, // fullscreen desc (windowed)
		nil, // restrict to output
	)
	if err != nil {
		return fmt.Errorf("dx12: CreateSwapChainForHwnd failed: %w", err)
	}

	// Query for IDXGISwapChain4 interface (required for GetCurrentBackBufferIndex)
	swapchain4, err := querySwapChain4(swapchain1)
	if err != nil {
		swapchain1.Release()
		return fmt.Errorf("dx12: failed to query IDXGISwapChain4: %w", err)
	}
	// Release the original swapchain1 reference (swapchain4 holds a reference)
	swapchain1.Release()

	s.swapchain = swapchain4
	s.width = config.Width
	s.height = config.Height
	s.presentMode = config.PresentMode
	s.swapchainFlags = swapchainFlags

	// Set maximum frame latency
	if err := swapchain4.SetMaximumFrameLatency(maxFrameLatency); err != nil {
		// Non-fatal, just log and continue
		_ = err
	}

	// Disable Alt+Enter fullscreen toggle
	if err := s.instance.factory.MakeWindowAssociation(s.hwnd, dxgi.DXGI_MWA_NO_ALT_ENTER); err != nil {
		// Non-fatal, just continue
		_ = err
	}

	// Create RTVs for back buffers
	if err := s.createBackBufferRTVs(); err != nil {
		swapchain4.Release()
		s.swapchain = nil
		return err
	}

	return nil
}

// createBackBufferRTVs creates render target views for each back buffer.
func (s *Surface) createBackBufferRTVs() error {
	// Get swapchain description to know buffer count
	desc, err := s.swapchain.GetDesc1()
	if err != nil {
		return fmt.Errorf("dx12: GetDesc1 failed: %w", err)
	}

	// Allocate back buffer array
	s.backBuffers = make([]backBuffer, desc.BufferCount)

	// Get D3D12 resource for each buffer and create RTV
	for i := uint32(0); i < desc.BufferCount; i++ {
		resourcePtr, err := s.swapchain.GetBuffer(i, &dxgi.IID_ID3D12Resource)
		if err != nil {
			// Clean up already created RTVs
			s.releaseBackBuffers()
			return fmt.Errorf("dx12: GetBuffer(%d) failed: %w", i, err)
		}

		resource := (*d3d12.ID3D12Resource)(resourcePtr)

		// Allocate RTV from device heap (supports recycling via free list)
		rtvHandle, err := s.device.rtvHeap.Allocate(1)
		if err != nil {
			resource.Release()
			s.releaseBackBuffers()
			return fmt.Errorf("dx12: failed to allocate RTV for buffer %d: %w", i, err)
		}

		// Track heap index for recycling on release
		rtvIndex := s.device.rtvHeap.HandleToIndex(rtvHandle)

		// Create RTV (nil desc = use resource format)
		s.device.raw.CreateRenderTargetView(resource, nil, rtvHandle)

		s.backBuffers[i] = backBuffer{
			resource:  resource,
			rtvHandle: rtvHandle,
			rtvIndex:  rtvIndex,
		}
	}

	return nil
}

// releaseBackBuffers releases all back buffer resources and recycles RTV descriptors.
func (s *Surface) releaseBackBuffers() {
	for i := range s.backBuffers {
		if s.backBuffers[i].resource != nil {
			s.backBuffers[i].resource.Release()
			s.backBuffers[i].resource = nil
		}
		// Recycle RTV descriptor slot for reuse (prevents heap exhaustion on resize)
		if s.device != nil {
			s.device.rtvHeap.Free(s.backBuffers[i].rtvIndex, 1)
		}
	}
	s.backBuffers = nil
}

// resizeSwapchain resizes an existing swapchain.
func (s *Surface) resizeSwapchain(config *hal.SurfaceConfiguration) error {
	if s.swapchain == nil || s.device == nil {
		return fmt.Errorf("dx12: surface not configured")
	}

	// Wait for GPU to finish using the back buffers
	if err := s.device.waitForGPU(); err != nil {
		return fmt.Errorf("dx12: failed to wait for GPU before resize: %w", err)
	}

	// Release existing back buffer resources
	s.releaseBackBuffers()

	// Determine new format
	format := textureFormatToDXGI(config.Format)
	if format == dxgi.DXGI_FORMAT_UNKNOWN {
		return fmt.Errorf("dx12: unsupported surface format: %v", config.Format)
	}

	// Resize buffers (0 means keep current buffer count)
	err := s.swapchain.ResizeBuffers(
		0, // keep buffer count
		config.Width,
		config.Height,
		format,
		s.swapchainFlags,
	)
	if err != nil {
		return fmt.Errorf("dx12: ResizeBuffers failed: %w", err)
	}

	// Update stored dimensions
	s.width = config.Width
	s.height = config.Height
	s.format = format
	s.halFormat = config.Format
	s.presentMode = config.PresentMode

	// Recreate RTVs
	return s.createBackBufferRTVs()
}

// querySwapChain4 queries IDXGISwapChain1 for IDXGISwapChain4 interface.
func querySwapChain4(swapchain1 *dxgi.IDXGISwapChain1) (*dxgi.IDXGISwapChain4, error) {
	return swapchain1.QueryInterface()
}

// textureFormatToDXGI converts WebGPU TextureFormat to DXGI_FORMAT.
func textureFormatToDXGI(format gputypes.TextureFormat) dxgi.DXGI_FORMAT {
	switch format {
	// 8-bit formats
	case gputypes.TextureFormatR8Unorm:
		return dxgi.DXGI_FORMAT_R8_UNORM
	case gputypes.TextureFormatR8Snorm:
		return dxgi.DXGI_FORMAT_R8_SNORM
	case gputypes.TextureFormatR8Uint:
		return dxgi.DXGI_FORMAT_R8_UINT
	case gputypes.TextureFormatR8Sint:
		return dxgi.DXGI_FORMAT_R8_SINT

	// 16-bit formats
	case gputypes.TextureFormatR16Uint:
		return dxgi.DXGI_FORMAT_R16_UINT
	case gputypes.TextureFormatR16Sint:
		return dxgi.DXGI_FORMAT_R16_SINT
	case gputypes.TextureFormatR16Float:
		return dxgi.DXGI_FORMAT_R16_FLOAT
	case gputypes.TextureFormatRG8Unorm:
		return dxgi.DXGI_FORMAT_R8G8_UNORM
	case gputypes.TextureFormatRG8Snorm:
		return dxgi.DXGI_FORMAT_R8G8_SNORM
	case gputypes.TextureFormatRG8Uint:
		return dxgi.DXGI_FORMAT_R8G8_UINT
	case gputypes.TextureFormatRG8Sint:
		return dxgi.DXGI_FORMAT_R8G8_SINT

	// 32-bit formats
	case gputypes.TextureFormatR32Uint:
		return dxgi.DXGI_FORMAT_R32_UINT
	case gputypes.TextureFormatR32Sint:
		return dxgi.DXGI_FORMAT_R32_SINT
	case gputypes.TextureFormatR32Float:
		return dxgi.DXGI_FORMAT_R32_FLOAT
	case gputypes.TextureFormatRG16Uint:
		return dxgi.DXGI_FORMAT_R16G16_UINT
	case gputypes.TextureFormatRG16Sint:
		return dxgi.DXGI_FORMAT_R16G16_SINT
	case gputypes.TextureFormatRG16Float:
		return dxgi.DXGI_FORMAT_R16G16_FLOAT
	case gputypes.TextureFormatRGBA8Unorm:
		return dxgi.DXGI_FORMAT_R8G8B8A8_UNORM
	case gputypes.TextureFormatRGBA8UnormSrgb:
		return dxgi.DXGI_FORMAT_R8G8B8A8_UNORM_SRGB
	case gputypes.TextureFormatRGBA8Snorm:
		return dxgi.DXGI_FORMAT_R8G8B8A8_SNORM
	case gputypes.TextureFormatRGBA8Uint:
		return dxgi.DXGI_FORMAT_R8G8B8A8_UINT
	case gputypes.TextureFormatRGBA8Sint:
		return dxgi.DXGI_FORMAT_R8G8B8A8_SINT
	case gputypes.TextureFormatBGRA8Unorm:
		return dxgi.DXGI_FORMAT_B8G8R8A8_UNORM
	case gputypes.TextureFormatBGRA8UnormSrgb:
		return dxgi.DXGI_FORMAT_B8G8R8A8_UNORM_SRGB

	// Packed 32-bit formats
	case gputypes.TextureFormatRGB10A2Uint:
		return dxgi.DXGI_FORMAT_R10G10B10A2_UINT
	case gputypes.TextureFormatRGB10A2Unorm:
		return dxgi.DXGI_FORMAT_R10G10B10A2_UNORM
	case gputypes.TextureFormatRG11B10Ufloat:
		return dxgi.DXGI_FORMAT_R11G11B10_FLOAT

	// 64-bit formats
	case gputypes.TextureFormatRG32Uint:
		return dxgi.DXGI_FORMAT_R32G32_UINT
	case gputypes.TextureFormatRG32Sint:
		return dxgi.DXGI_FORMAT_R32G32_SINT
	case gputypes.TextureFormatRG32Float:
		return dxgi.DXGI_FORMAT_R32G32_FLOAT
	case gputypes.TextureFormatRGBA16Uint:
		return dxgi.DXGI_FORMAT_R16G16B16A16_UINT
	case gputypes.TextureFormatRGBA16Sint:
		return dxgi.DXGI_FORMAT_R16G16B16A16_SINT
	case gputypes.TextureFormatRGBA16Float:
		return dxgi.DXGI_FORMAT_R16G16B16A16_FLOAT

	// 128-bit formats
	case gputypes.TextureFormatRGBA32Uint:
		return dxgi.DXGI_FORMAT_R32G32B32A32_UINT
	case gputypes.TextureFormatRGBA32Sint:
		return dxgi.DXGI_FORMAT_R32G32B32A32_SINT
	case gputypes.TextureFormatRGBA32Float:
		return dxgi.DXGI_FORMAT_R32G32B32A32_FLOAT

	// Depth formats
	case gputypes.TextureFormatDepth16Unorm:
		return dxgi.DXGI_FORMAT_D16_UNORM
	case gputypes.TextureFormatDepth24Plus:
		return dxgi.DXGI_FORMAT_D24_UNORM_S8_UINT
	case gputypes.TextureFormatDepth24PlusStencil8:
		return dxgi.DXGI_FORMAT_D24_UNORM_S8_UINT
	case gputypes.TextureFormatDepth32Float:
		return dxgi.DXGI_FORMAT_D32_FLOAT
	case gputypes.TextureFormatDepth32FloatStencil8:
		return dxgi.DXGI_FORMAT_D32_FLOAT_S8X24_UINT

	// BC compressed formats
	case gputypes.TextureFormatBC1RGBAUnorm:
		return dxgi.DXGI_FORMAT_BC1_UNORM
	case gputypes.TextureFormatBC1RGBAUnormSrgb:
		return dxgi.DXGI_FORMAT_BC1_UNORM_SRGB
	case gputypes.TextureFormatBC2RGBAUnorm:
		return dxgi.DXGI_FORMAT_BC2_UNORM
	case gputypes.TextureFormatBC2RGBAUnormSrgb:
		return dxgi.DXGI_FORMAT_BC2_UNORM_SRGB
	case gputypes.TextureFormatBC3RGBAUnorm:
		return dxgi.DXGI_FORMAT_BC3_UNORM
	case gputypes.TextureFormatBC3RGBAUnormSrgb:
		return dxgi.DXGI_FORMAT_BC3_UNORM_SRGB
	case gputypes.TextureFormatBC4RUnorm:
		return dxgi.DXGI_FORMAT_BC4_UNORM
	case gputypes.TextureFormatBC4RSnorm:
		return dxgi.DXGI_FORMAT_BC4_SNORM
	case gputypes.TextureFormatBC5RGUnorm:
		return dxgi.DXGI_FORMAT_BC5_UNORM
	case gputypes.TextureFormatBC5RGSnorm:
		return dxgi.DXGI_FORMAT_BC5_SNORM
	case gputypes.TextureFormatBC6HRGBUfloat:
		return dxgi.DXGI_FORMAT_BC6H_UF16
	case gputypes.TextureFormatBC6HRGBFloat:
		return dxgi.DXGI_FORMAT_BC6H_SF16
	case gputypes.TextureFormatBC7RGBAUnorm:
		return dxgi.DXGI_FORMAT_BC7_UNORM
	case gputypes.TextureFormatBC7RGBAUnormSrgb:
		return dxgi.DXGI_FORMAT_BC7_UNORM_SRGB

	default:
		return dxgi.DXGI_FORMAT_UNKNOWN
	}
}

// compositeAlphaModeToDXGI converts HAL CompositeAlphaMode to DXGI_ALPHA_MODE.
func compositeAlphaModeToDXGI(mode hal.CompositeAlphaMode) dxgi.DXGI_ALPHA_MODE {
	switch mode {
	case hal.CompositeAlphaModePremultiplied:
		return dxgi.DXGI_ALPHA_MODE_PREMULTIPLIED
	case hal.CompositeAlphaModeUnpremultiplied:
		return dxgi.DXGI_ALPHA_MODE_STRAIGHT
	case hal.CompositeAlphaModeInherit:
		return dxgi.DXGI_ALPHA_MODE_UNSPECIFIED
	default:
		// CompositeAlphaModeOpaque and any unknown value default to IGNORE
		return dxgi.DXGI_ALPHA_MODE_IGNORE
	}
}

// -----------------------------------------------------------------------------
// SurfaceTexture implementation
// -----------------------------------------------------------------------------

// SurfaceTexture implements hal.SurfaceTexture for DirectX 12.
// It wraps a swapchain back buffer for rendering.
type SurfaceTexture struct {
	surface    *Surface
	index      uint32
	resource   *d3d12.ID3D12Resource
	rtvHandle  d3d12.D3D12_CPU_DESCRIPTOR_HANDLE
	format     gputypes.TextureFormat
	width      uint32
	height     uint32
	suboptimal bool
}

// Destroy implements hal.SurfaceTexture.
// Surface textures are owned by the swapchain and should not be destroyed individually.
func (t *SurfaceTexture) Destroy() {
	// No-op: surface textures are owned by the swapchain
}

// GetRTVHandle returns the RTV handle for this texture.
func (t *SurfaceTexture) GetRTVHandle() d3d12.D3D12_CPU_DESCRIPTOR_HANDLE {
	return t.rtvHandle
}

// GetResource returns the D3D12 resource for this texture.
func (t *SurfaceTexture) GetResource() *d3d12.ID3D12Resource {
	return t.resource
}

// GetFormat returns the texture format.
func (t *SurfaceTexture) GetFormat() gputypes.TextureFormat {
	return t.format
}

// GetWidth returns the texture width.
func (t *SurfaceTexture) GetWidth() uint32 {
	return t.width
}

// GetHeight returns the texture height.
func (t *SurfaceTexture) GetHeight() uint32 {
	return t.height
}

// NativeHandle returns the raw ID3D12Resource pointer.
func (t *SurfaceTexture) NativeHandle() uintptr {
	if t.resource != nil {
		return uintptr(unsafe.Pointer(t.resource))
	}
	return 0
}

// Compile-time interface assertion.
var _ hal.SurfaceTexture = (*SurfaceTexture)(nil)
