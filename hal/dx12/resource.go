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
)

// -----------------------------------------------------------------------------
// Buffer Implementation
// -----------------------------------------------------------------------------

// Buffer implements hal.Buffer for DirectX 12.
type Buffer struct {
	raw           *d3d12.ID3D12Resource
	size          uint64
	usage         gputypes.BufferUsage
	heapType      d3d12.D3D12_HEAP_TYPE
	gpuVA         uint64 // GPU virtual address for binding
	device        *Device
	mappedPointer unsafe.Pointer // Non-nil if buffer is currently mapped
}

// Destroy releases the buffer resources.
func (b *Buffer) Destroy() {
	if b.raw != nil {
		// Unmap if still mapped
		if b.mappedPointer != nil {
			b.raw.Unmap(0, nil)
			b.mappedPointer = nil
		}
		b.raw.Release()
		b.raw = nil
	}
}

// Map maps the buffer memory for CPU access.
// Only valid for buffers with MapRead or MapWrite usage.
func (b *Buffer) Map(offset, size uint64) (unsafe.Pointer, error) {
	if b.mappedPointer != nil {
		return nil, fmt.Errorf("dx12: buffer is already mapped")
	}

	if b.heapType != d3d12.D3D12_HEAP_TYPE_UPLOAD && b.heapType != d3d12.D3D12_HEAP_TYPE_READBACK {
		return nil, fmt.Errorf("dx12: buffer is not mappable (heap type: %d)", b.heapType)
	}

	// For read-back buffers, specify the range to read
	var readRange *d3d12.D3D12_RANGE
	if b.heapType == d3d12.D3D12_HEAP_TYPE_READBACK {
		readRange = &d3d12.D3D12_RANGE{
			Begin: uintptr(offset),
			End:   uintptr(offset + size),
		}
	} else {
		// Upload buffers: range of 0 means we won't read
		readRange = &d3d12.D3D12_RANGE{Begin: 0, End: 0}
	}

	ptr, err := b.raw.Map(0, readRange)
	if err != nil {
		return nil, fmt.Errorf("dx12: buffer Map failed: %w", err)
	}

	b.mappedPointer = ptr
	// Return pointer offset by the requested offset
	return unsafe.Pointer(uintptr(ptr) + uintptr(offset)), nil
}

// Unmap unmaps the buffer memory.
func (b *Buffer) Unmap(offset, size uint64) {
	if b.mappedPointer == nil {
		return
	}

	// For upload buffers, specify the written range
	var writtenRange *d3d12.D3D12_RANGE
	if b.heapType == d3d12.D3D12_HEAP_TYPE_UPLOAD {
		writtenRange = &d3d12.D3D12_RANGE{
			Begin: uintptr(offset),
			End:   uintptr(offset + size),
		}
	}
	// For read-back buffers, pass nil (no writes)

	b.raw.Unmap(0, writtenRange)
	b.mappedPointer = nil
}

// Raw returns the underlying D3D12 resource.
func (b *Buffer) Raw() *d3d12.ID3D12Resource {
	return b.raw
}

// GPUVirtualAddress returns the GPU virtual address for this buffer.
func (b *Buffer) GPUVirtualAddress() uint64 {
	return b.gpuVA
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() uint64 {
	return b.size
}

// -----------------------------------------------------------------------------
// Texture Implementation
// -----------------------------------------------------------------------------

// Texture implements hal.Texture for DirectX 12.
type Texture struct {
	raw        *d3d12.ID3D12Resource
	format     gputypes.TextureFormat
	dimension  gputypes.TextureDimension
	size       hal.Extent3D
	mipLevels  uint32
	samples    uint32
	usage      gputypes.TextureUsage
	device     *Device
	isExternal bool // True for swapchain images (not owned)
}

// Destroy releases the texture resources.
func (t *Texture) Destroy() {
	if t.raw != nil && !t.isExternal {
		t.raw.Release()
		t.raw = nil
	}
}

// Raw returns the underlying D3D12 resource.
func (t *Texture) Raw() *d3d12.ID3D12Resource {
	return t.raw
}

// Format returns the texture format.
func (t *Texture) Format() gputypes.TextureFormat {
	return t.format
}

// Dimension returns the texture dimension.
func (t *Texture) Dimension() gputypes.TextureDimension {
	return t.dimension
}

// -----------------------------------------------------------------------------
// TextureView Implementation
// -----------------------------------------------------------------------------

// TextureView implements hal.TextureView for DirectX 12.
type TextureView struct {
	texture      *Texture
	format       gputypes.TextureFormat
	dimension    gputypes.TextureViewDimension
	baseMip      uint32
	mipCount     uint32
	baseLayer    uint32
	layerCount   uint32
	device       *Device
	srvHandle    d3d12.D3D12_CPU_DESCRIPTOR_HANDLE // Shader resource view (for sampling)
	rtvHandle    d3d12.D3D12_CPU_DESCRIPTOR_HANDLE // Render target view
	dsvHandle    d3d12.D3D12_CPU_DESCRIPTOR_HANDLE // Depth stencil view
	hasSRV       bool
	hasRTV       bool
	hasDSV       bool
	srvHeapIndex uint32
	rtvHeapIndex uint32
	dsvHeapIndex uint32
}

// Destroy releases the texture view resources.
// Note: Descriptors are not freed back to the heap in this simple implementation.
// A proper implementation would use a free list.
func (v *TextureView) Destroy() {
	// Descriptors are managed by heaps; we just clear references
	v.hasSRV = false
	v.hasRTV = false
	v.hasDSV = false
}

// Texture returns the parent texture.
func (v *TextureView) Texture() *Texture {
	return v.texture
}

// RTVHandle returns the render target view descriptor handle.
func (v *TextureView) RTVHandle() d3d12.D3D12_CPU_DESCRIPTOR_HANDLE {
	return v.rtvHandle
}

// DSVHandle returns the depth stencil view descriptor handle.
func (v *TextureView) DSVHandle() d3d12.D3D12_CPU_DESCRIPTOR_HANDLE {
	return v.dsvHandle
}

// SRVHandle returns the shader resource view descriptor handle.
func (v *TextureView) SRVHandle() d3d12.D3D12_CPU_DESCRIPTOR_HANDLE {
	return v.srvHandle
}

// HasRTV returns true if this view has a render target view.
func (v *TextureView) HasRTV() bool {
	return v.hasRTV
}

// HasDSV returns true if this view has a depth stencil view.
func (v *TextureView) HasDSV() bool {
	return v.hasDSV
}

// HasSRV returns true if this view has a shader resource view.
func (v *TextureView) HasSRV() bool {
	return v.hasSRV
}

// -----------------------------------------------------------------------------
// Sampler Implementation
// -----------------------------------------------------------------------------

// Sampler implements hal.Sampler for DirectX 12.
type Sampler struct {
	handle    d3d12.D3D12_CPU_DESCRIPTOR_HANDLE
	heapIndex uint32
	device    *Device
}

// Destroy releases the sampler resources.
func (s *Sampler) Destroy() {
	// Sampler descriptors are managed by the heap
}

// Handle returns the sampler descriptor handle.
func (s *Sampler) Handle() d3d12.D3D12_CPU_DESCRIPTOR_HANDLE {
	return s.handle
}

// -----------------------------------------------------------------------------
// Compile-time interface assertions
// -----------------------------------------------------------------------------

var (
	_ hal.Buffer      = (*Buffer)(nil)
	_ hal.Texture     = (*Texture)(nil)
	_ hal.TextureView = (*TextureView)(nil)
	_ hal.Sampler     = (*Sampler)(nil)
)
