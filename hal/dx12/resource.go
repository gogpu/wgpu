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
	raw             *d3d12.ID3D12Resource
	size            uint64
	usage           gputypes.BufferUsage
	heapType        d3d12.D3D12_HEAP_TYPE
	cpuPageProperty d3d12.D3D12_CPU_PAGE_PROPERTY // for CUSTOM heap type
	gpuVA           uint64                        // GPU virtual address for binding
	device          *Device
	mappedPointer   unsafe.Pointer // Non-nil if buffer is currently mapped
}

// isMappable returns true if the buffer can be mapped for CPU access.
// UPLOAD, READBACK, and CUSTOM heaps with CPU page properties are mappable.
func (b *Buffer) isMappable() bool {
	switch b.heapType {
	case d3d12.D3D12_HEAP_TYPE_UPLOAD, d3d12.D3D12_HEAP_TYPE_READBACK:
		return true
	case d3d12.D3D12_HEAP_TYPE_CUSTOM:
		return b.cpuPageProperty == d3d12.D3D12_CPU_PAGE_PROPERTY_WRITE_COMBINE ||
			b.cpuPageProperty == d3d12.D3D12_CPU_PAGE_PROPERTY_WRITE_BACK
	default:
		return false
	}
}

// isReadback returns true if the buffer is for GPU→CPU reads.
func (b *Buffer) isReadback() bool {
	return b.heapType == d3d12.D3D12_HEAP_TYPE_READBACK ||
		(b.heapType == d3d12.D3D12_HEAP_TYPE_CUSTOM && b.cpuPageProperty == d3d12.D3D12_CPU_PAGE_PROPERTY_WRITE_BACK)
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

	if !b.isMappable() {
		return nil, fmt.Errorf("dx12: buffer is not mappable (heap type: %d, cpuPage: %d)", b.heapType, b.cpuPageProperty)
	}

	// For read-back buffers, specify the range to read
	var readRange *d3d12.D3D12_RANGE
	if b.isReadback() {
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

	// For CPU-writable buffers, specify the written range
	var writtenRange *d3d12.D3D12_RANGE
	if !b.isReadback() {
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

// NativeHandle returns an opaque handle to this Buffer struct.
// DX12 bind groups need the full Go struct to access GPU virtual address and size.
func (b *Buffer) NativeHandle() uintptr {
	return uintptr(unsafe.Pointer(b))
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
	raw          *d3d12.ID3D12Resource
	format       gputypes.TextureFormat
	dimension    gputypes.TextureDimension
	size         hal.Extent3D
	mipLevels    uint32
	samples      uint32
	usage        gputypes.TextureUsage
	device       *Device
	isExternal   bool                        // True for swapchain images (not owned)
	currentState d3d12.D3D12_RESOURCE_STATES // Tracked resource state for barrier correctness
	pendingRefs  int32                       // >0 = PendingWrites in-flight, defer Destroy (BUG-DX12-006)
	pendingDeath bool                        // true = Destroy was called while pendingRefs > 0
}

// CurrentUsage returns the texture's tracked D3D12 resource state mapped to gputypes.TextureUsage.
// Used by PendingWrites to determine the correct "before" state for copy barriers.
func (t *Texture) CurrentUsage() gputypes.TextureUsage {
	return d3d12StateToTextureUsage(t.currentState)
}

// AddPendingRef increments the pending reference count.
// Called by PendingWrites when recording CopyBufferToTexture.
func (t *Texture) AddPendingRef() {
	t.pendingRefs++
	hal.Logger().Debug("dx12: texture AddPendingRef", "refs", t.pendingRefs)
}

// DecPendingRef decrements the pending reference count.
// If Destroy was deferred (pendingDeath=true) and refcount reaches 0, releases now.
func (t *Texture) DecPendingRef() {
	t.pendingRefs--
	if t.pendingRefs <= 0 && t.pendingDeath {
		t.doDestroy()
	}
}

// Destroy releases the texture resources.
// If PendingWrites has in-flight references (pendingRefs > 0), destruction is
// deferred until all references are released. This prevents use-after-free when
// the GPU is still executing CopyBufferToTexture (BUG-DX12-006).
func (t *Texture) Destroy() {
	hal.Logger().Debug("dx12: texture Destroy", "refs", t.pendingRefs, "deferred", t.pendingDeath)
	if t.pendingRefs > 0 {
		t.pendingDeath = true
		hal.Logger().Debug("dx12: texture destroy deferred (pending refs)")
		return
	}
	t.doDestroy()
}

func (t *Texture) doDestroy() {
	if t.raw != nil && !t.isExternal {
		t.raw.Release()
		t.raw = nil
	}
}

// Raw returns the underlying D3D12 resource.
func (t *Texture) Raw() *d3d12.ID3D12Resource {
	return t.raw
}

// NativeHandle returns the raw ID3D12Resource pointer.
func (t *Texture) NativeHandle() uintptr {
	if t.raw != nil {
		return uintptr(unsafe.Pointer(t.raw))
	}
	return 0
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

// Destroy releases the texture view resources and recycles descriptor heap slots.
// For external (surface) texture views, descriptor slots are NOT freed because
// the Surface owns them and manages their lifecycle via releaseBackBuffers().
func (v *TextureView) Destroy() {
	if v.device != nil && (v.texture == nil || !v.texture.isExternal) {
		if v.hasSRV && v.device.stagingViewHeap != nil {
			v.device.stagingViewHeap.Free(v.srvHeapIndex, 1)
		}
		if v.hasRTV && v.device.rtvHeap != nil {
			v.device.rtvHeap.Free(v.rtvHeapIndex, 1)
		}
		if v.hasDSV && v.device.dsvHeap != nil {
			v.device.dsvHeap.Free(v.dsvHeapIndex, 1)
		}
	}
	v.hasSRV = false
	v.hasRTV = false
	v.hasDSV = false
}

// Texture returns the parent texture.
func (v *TextureView) Texture() *Texture {
	return v.texture
}

// NativeHandle returns an opaque handle to this TextureView struct.
// DX12 bind groups need the full Go struct to access SRV descriptor handles.
func (v *TextureView) NativeHandle() uintptr {
	return uintptr(unsafe.Pointer(v))
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
// Each sampler occupies a slot in both the staging heap (for CopyDescriptors source)
// and the shader-visible sampler heap (global sampler pool, referenced by index
// from sampler index buffers in bind groups).
type Sampler struct {
	handle          d3d12.D3D12_CPU_DESCRIPTOR_HANDLE // staging heap handle
	heapIndex       uint32                            // staging heap index
	samplerPoolSlot uint32                            // index in shader-visible sampler heap (global pool)
	device          *Device
}

// Destroy releases the sampler resources and recycles descriptor heap slots.
func (s *Sampler) Destroy() {
	if s.device != nil {
		s.device.stagingSamplerHeap.Free(s.heapIndex, 1)
		s.device.samplerHeap.Free(s.samplerPoolSlot, 1)
	}
}

// Handle returns the sampler descriptor handle.
func (s *Sampler) Handle() d3d12.D3D12_CPU_DESCRIPTOR_HANDLE {
	return s.handle
}

// NativeHandle returns an opaque handle to this Sampler struct.
// DX12 bind groups need the full Go struct to access the sampler descriptor handle.
func (s *Sampler) NativeHandle() uintptr { return uintptr(unsafe.Pointer(s)) }

// -----------------------------------------------------------------------------
// Compile-time interface assertions
// -----------------------------------------------------------------------------

var (
	_ hal.Buffer      = (*Buffer)(nil)
	_ hal.Texture     = (*Texture)(nil)
	_ hal.TextureView = (*TextureView)(nil)
	_ hal.Sampler     = (*Sampler)(nil)
)
