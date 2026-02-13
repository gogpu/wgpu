// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package dx12

import (
	"fmt"
	"time"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
	"github.com/gogpu/wgpu/hal/dx12/dxgi"
)

// Queue implements hal.Queue for DirectX 12.
// It manages command submission and presentation to surfaces.
type Queue struct {
	device *Device
	raw    *d3d12.ID3D12CommandQueue
}

// newQueue creates a new Queue wrapping the device's command queue.
func newQueue(device *Device) *Queue {
	return &Queue{
		device: device,
		raw:    device.directQueue,
	}
}

// Submit submits command buffers to the GPU.
// If fence is not nil, it will be signaled with fenceValue when commands complete.
func (q *Queue) Submit(commandBuffers []hal.CommandBuffer, fence hal.Fence, fenceValue uint64) error {
	if len(commandBuffers) == 0 && fence == nil {
		return nil
	}

	// Convert command buffers to D3D12 command lists
	if len(commandBuffers) > 0 {
		cmdLists := make([]*d3d12.ID3D12GraphicsCommandList, len(commandBuffers))
		for i, cb := range commandBuffers {
			dx12CB, ok := cb.(*CommandBuffer)
			if !ok {
				return fmt.Errorf("dx12: command buffer is not a DX12 command buffer")
			}
			cmdLists[i] = dx12CB.cmdList
		}

		// Execute command lists
		q.raw.ExecuteCommandLists(uint32(len(cmdLists)), &cmdLists[0])

		// Check for immediate device removal after execution.
		// ExecuteCommandLists is async (void), but some drivers detect errors immediately.
		if reason := q.device.raw.GetDeviceRemovedReason(); reason != nil {
			return fmt.Errorf("dx12: device removed after ExecuteCommandLists: %w", reason)
		}
	}

	// Signal the fence if provided
	if fence != nil {
		dx12Fence, ok := fence.(*Fence)
		if !ok {
			return fmt.Errorf("dx12: fence is not a DX12 fence")
		}

		if err := q.raw.Signal(dx12Fence.raw, fenceValue); err != nil {
			return fmt.Errorf("dx12: queue Signal failed: %w", err)
		}
	}

	return nil
}

// ReadBuffer reads data from a GPU buffer into the provided byte slice.
// The buffer must have been created with MapRead usage (readback heap).
// If the buffer is already mapped, data is copied directly.
// Otherwise, the buffer is temporarily mapped for the read.
func (q *Queue) ReadBuffer(buffer hal.Buffer, offset uint64, data []byte) error {
	if len(data) == 0 {
		return nil
	}

	buf, ok := buffer.(*Buffer)
	if !ok || buf.raw == nil {
		return fmt.Errorf("dx12: invalid buffer for ReadBuffer")
	}

	if buf.mappedPointer != nil {
		// Buffer is already mapped — copy directly
		src := unsafe.Slice((*byte)(unsafe.Add(buf.mappedPointer, int(offset))), len(data))
		copy(data, src)
		return nil
	}

	// Buffer is not mapped — temporarily map, copy, unmap
	if buf.heapType != d3d12.D3D12_HEAP_TYPE_READBACK {
		return fmt.Errorf("dx12: buffer is not in readback heap, cannot read")
	}

	readRange := &d3d12.D3D12_RANGE{
		Begin: uintptr(offset),
		End:   uintptr(offset + uint64(len(data))),
	}
	ptr, err := buf.raw.Map(0, readRange)
	if err != nil {
		return fmt.Errorf("dx12: buffer Map failed for ReadBuffer: %w", err)
	}

	src := unsafe.Slice((*byte)(unsafe.Add(ptr, int(offset))), len(data))
	copy(data, src)

	// Unmap with nil written range (we only read, not write)
	buf.raw.Unmap(0, nil)

	return nil
}

// WriteBuffer writes data to a buffer immediately.
// For upload heap buffers, data is copied directly via CPU mapping.
// For default heap buffers, a staging buffer + GPU copy command is used.
func (q *Queue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) {
	if buffer == nil || len(data) == 0 {
		return
	}

	buf, ok := buffer.(*Buffer)
	if !ok || buf.raw == nil {
		return
	}

	// Upload heap buffers can be written directly via CPU mapping
	if buf.heapType == d3d12.D3D12_HEAP_TYPE_UPLOAD {
		q.writeBufferDirect(buf, offset, data)
		return
	}

	// Default heap buffers require staging buffer + GPU copy
	q.writeBufferStaged(buf, offset, data)
}

// writeBufferDirect copies data to a mappable (upload heap) buffer.
func (q *Queue) writeBufferDirect(buf *Buffer, offset uint64, data []byte) {
	if buf.mappedPointer != nil {
		// Already mapped — copy directly
		dst := unsafe.Slice((*byte)(unsafe.Add(buf.mappedPointer, int(offset))), len(data))
		copy(dst, data)
		return
	}

	// Temporarily map, copy, unmap
	readRange := &d3d12.D3D12_RANGE{Begin: 0, End: 0} // No reads
	ptr, err := buf.raw.Map(0, readRange)
	if err != nil {
		return
	}
	dst := unsafe.Slice((*byte)(unsafe.Add(ptr, int(offset))), len(data))
	copy(dst, data)

	writtenRange := &d3d12.D3D12_RANGE{
		Begin: uintptr(offset),
		End:   uintptr(offset + uint64(len(data))),
	}
	buf.raw.Unmap(0, writtenRange)
}

// writeBufferStaged copies data to a GPU-only (default heap) buffer
// via an upload heap staging buffer and CopyBufferRegion command.
func (q *Queue) writeBufferStaged(buf *Buffer, offset uint64, data []byte) {
	// Create upload heap staging buffer (mapped at creation for immediate write)
	staging, err := q.device.CreateBuffer(&hal.BufferDescriptor{
		Label:            "write-buffer-staging",
		Size:             uint64(len(data)),
		Usage:            gputypes.BufferUsageCopySrc | gputypes.BufferUsageMapWrite,
		MappedAtCreation: true,
	})
	if err != nil {
		return
	}
	defer q.device.DestroyBuffer(staging)

	stagingBuf := staging.(*Buffer)

	// Copy data to mapped staging buffer
	dst := unsafe.Slice((*byte)(stagingBuf.mappedPointer), len(data))
	copy(dst, data)

	// Unmap staging buffer
	writtenRange := &d3d12.D3D12_RANGE{Begin: 0, End: uintptr(len(data))}
	stagingBuf.raw.Unmap(0, writtenRange)
	stagingBuf.mappedPointer = nil

	// Create one-shot command encoder for the copy
	cmdEncoder, err := q.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "write-buffer-copy",
	})
	if err != nil {
		return
	}

	encoder := cmdEncoder.(*CommandEncoder)
	if err := encoder.BeginEncoding("write-buffer-copy"); err != nil {
		return
	}

	// D3D12 auto-promotes buffers from COMMON to COPY_DEST.
	// After command list execution, buffers auto-decay back to COMMON.
	encoder.cmdList.CopyBufferRegion(buf.raw, offset, stagingBuf.raw, 0, uint64(len(data)))

	cmdBuffer, err := encoder.EndEncoding()
	if err != nil {
		return
	}

	// Submit and wait for GPU completion
	fence, err := q.device.CreateFence()
	if err != nil {
		return
	}
	defer q.device.DestroyFence(fence)

	if err := q.Submit([]hal.CommandBuffer{cmdBuffer}, fence, 1); err != nil {
		return
	}
	_, _ = q.device.Wait(fence, 1, 60*time.Second)
	q.device.FreeCommandBuffer(cmdBuffer)
}

// d3d12TexturePitchAlignment is the required row pitch alignment for texture data.
const d3d12TexturePitchAlignment = 256

// WriteTexture writes data to a texture immediately.
// Creates an upload heap staging buffer, copies data with proper row pitch
// alignment, and uses CopyTextureRegion to transfer to the GPU texture.
func (q *Queue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) {
	if dst == nil || dst.Texture == nil || len(data) == 0 || size == nil {
		return
	}

	dstTex, ok := dst.Texture.(*Texture)
	if !ok || dstTex.raw == nil {
		return
	}

	// Calculate layout parameters
	bytesPerRow := layout.BytesPerRow
	if bytesPerRow == 0 {
		bytesPerRow = size.Width * 4 // Assume RGBA8 (4 bytes per pixel)
	}

	rowsPerImage := layout.RowsPerImage
	if rowsPerImage == 0 {
		rowsPerImage = size.Height
	}

	depthOrLayers := size.DepthOrArrayLayers
	if depthOrLayers == 0 {
		depthOrLayers = 1
	}

	// D3D12 requires RowPitch to be aligned to 256 bytes
	alignedRowPitch := (bytesPerRow + d3d12TexturePitchAlignment - 1) &^ (d3d12TexturePitchAlignment - 1)

	// Calculate staging buffer size with aligned pitch
	stagingSize := uint64(alignedRowPitch) * uint64(rowsPerImage) * uint64(depthOrLayers)

	// Create upload heap staging buffer
	staging, err := q.device.CreateBuffer(&hal.BufferDescriptor{
		Label:            "write-texture-staging",
		Size:             stagingSize,
		Usage:            gputypes.BufferUsageCopySrc | gputypes.BufferUsageMapWrite,
		MappedAtCreation: true,
	})
	if err != nil {
		return
	}
	defer q.device.DestroyBuffer(staging)

	stagingBuf := staging.(*Buffer)

	// Copy data to staging buffer with proper row pitch alignment
	srcOffset := layout.Offset
	if bytesPerRow == alignedRowPitch {
		// No alignment padding needed — single copy
		srcData := data[srcOffset:]
		if uint64(len(srcData)) > stagingSize {
			srcData = srcData[:stagingSize]
		}
		d := unsafe.Slice((*byte)(stagingBuf.mappedPointer), len(srcData))
		copy(d, srcData)
	} else {
		// Row-by-row copy to handle alignment padding
		for z := uint32(0); z < depthOrLayers; z++ {
			for row := uint32(0); row < rowsPerImage; row++ {
				srcStart := srcOffset + uint64(z)*uint64(bytesPerRow)*uint64(rowsPerImage) + uint64(row)*uint64(bytesPerRow)
				dstStart := uint64(z)*uint64(alignedRowPitch)*uint64(rowsPerImage) + uint64(row)*uint64(alignedRowPitch)

				if srcStart+uint64(bytesPerRow) > uint64(len(data)) {
					break
				}

				src := data[srcStart : srcStart+uint64(bytesPerRow)]
				d := unsafe.Slice((*byte)(unsafe.Add(stagingBuf.mappedPointer, int(dstStart))), bytesPerRow)
				copy(d, src)
			}
		}
	}

	// Unmap staging buffer
	writtenRange := &d3d12.D3D12_RANGE{Begin: 0, End: uintptr(stagingSize)}
	stagingBuf.raw.Unmap(0, writtenRange)
	stagingBuf.mappedPointer = nil

	// Create one-shot command encoder
	cmdEncoder, err := q.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "write-texture-copy",
	})
	if err != nil {
		return
	}

	encoder := cmdEncoder.(*CommandEncoder)
	if err := encoder.BeginEncoding("write-texture-copy"); err != nil {
		return
	}

	// Transition texture to COPY_DEST (textures don't auto-promote for writes)
	barrierToCopy := d3d12.NewTransitionBarrier(
		dstTex.raw,
		d3d12.D3D12_RESOURCE_STATE_COMMON,
		d3d12.D3D12_RESOURCE_STATE_COPY_DEST,
		d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES,
	)
	encoder.cmdList.ResourceBarrier(1, &barrierToCopy)

	// Source location (staging buffer with placed footprint)
	srcLoc := d3d12.D3D12_TEXTURE_COPY_LOCATION{
		Resource: stagingBuf.raw,
		Type:     d3d12.D3D12_TEXTURE_COPY_TYPE_PLACED_FOOTPRINT,
	}
	srcLoc.SetPlacedFootprint(d3d12.D3D12_PLACED_SUBRESOURCE_FOOTPRINT{
		Offset: 0,
		Footprint: d3d12.D3D12_SUBRESOURCE_FOOTPRINT{
			Format:   textureFormatToD3D12(dstTex.format),
			Width:    size.Width,
			Height:   size.Height,
			Depth:    depthOrLayers,
			RowPitch: alignedRowPitch,
		},
	})

	// Destination location (texture subresource)
	subresource := dst.MipLevel + dst.Origin.Z*dstTex.mipLevels
	dstLoc := d3d12.D3D12_TEXTURE_COPY_LOCATION{
		Resource: dstTex.raw,
		Type:     d3d12.D3D12_TEXTURE_COPY_TYPE_SUBRESOURCE_INDEX,
	}
	dstLoc.SetSubresourceIndex(subresource)

	encoder.cmdList.CopyTextureRegion(
		&dstLoc,
		dst.Origin.X, dst.Origin.Y, dst.Origin.Z,
		&srcLoc,
		nil, // Copy entire source
	)

	// Transition texture to shader resource state (ready for rendering)
	barrierToShader := d3d12.NewTransitionBarrier(
		dstTex.raw,
		d3d12.D3D12_RESOURCE_STATE_COPY_DEST,
		d3d12.D3D12_RESOURCE_STATE_PIXEL_SHADER_RESOURCE|d3d12.D3D12_RESOURCE_STATE_NON_PIXEL_SHADER_RESOURCE,
		d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES,
	)
	encoder.cmdList.ResourceBarrier(1, &barrierToShader)

	// End encoding
	cmdBuffer, err := encoder.EndEncoding()
	if err != nil {
		return
	}

	// Submit and wait for GPU completion
	fence, err := q.device.CreateFence()
	if err != nil {
		return
	}
	defer q.device.DestroyFence(fence)

	if err := q.Submit([]hal.CommandBuffer{cmdBuffer}, fence, 1); err != nil {
		return
	}
	_, _ = q.device.Wait(fence, 1, 60*time.Second)
	q.device.FreeCommandBuffer(cmdBuffer)
}

// Present presents a surface texture to the screen.
// The texture must have been acquired via Surface.AcquireTexture.
func (q *Queue) Present(surface hal.Surface, texture hal.SurfaceTexture) error {
	dx12Surface, ok := surface.(*Surface)
	if !ok {
		return fmt.Errorf("dx12: surface is not a DX12 surface")
	}

	if dx12Surface.swapchain == nil {
		return fmt.Errorf("dx12: surface not configured")
	}

	// Note: Resource barriers (render target -> present) should be
	// handled in the command buffer encoding before this call.
	// The present call here just flips the swapchain.

	// Determine sync interval and flags based on present mode
	var syncInterval uint32
	var presentFlags uint32

	switch dx12Surface.presentMode {
	case hal.PresentModeImmediate:
		// No vsync, immediate presentation
		syncInterval = 0
		if dx12Surface.allowTearing {
			presentFlags |= uint32(dxgi.DXGI_PRESENT_ALLOW_TEARING)
		}
	case hal.PresentModeMailbox:
		// VSync with triple buffering (latest frame wins)
		// Mailbox is simulated with syncInterval=0 + triple buffer
		syncInterval = 0
		if dx12Surface.allowTearing {
			presentFlags |= uint32(dxgi.DXGI_PRESENT_ALLOW_TEARING)
		}
	case hal.PresentModeFifo, hal.PresentModeFifoRelaxed:
		// VSync enabled
		syncInterval = 1
	default:
		// Default to vsync
		syncInterval = 1
	}

	// Present the frame
	if err := dx12Surface.swapchain.Present(syncInterval, presentFlags); err != nil {
		return fmt.Errorf("dx12: Present failed: %w", err)
	}

	// CRITICAL: Wait for GPU to finish presenting before allowing the next frame.
	// Without this, the next frame may start rendering to a swapchain buffer that
	// the GPU is still presenting, causing corruption or a hang.
	if err := q.device.waitForGPU(); err != nil {
		return fmt.Errorf("dx12: post-present GPU sync failed: %w", err)
	}

	return nil
}

// GetTimestampPeriod returns the timestamp period in nanoseconds.
// Used to convert timestamp query results to real time.
func (q *Queue) GetTimestampPeriod() float32 {
	freq, err := q.raw.GetTimestampFrequency()
	if err != nil || freq == 0 {
		// Default to 1.0 if we can't get the frequency
		return 1.0
	}

	// Convert frequency (Hz) to period (nanoseconds)
	// period = 1 / frequency (in seconds) = 1e9 / frequency (in nanoseconds)
	return float32(1e9) / float32(freq)
}

// -----------------------------------------------------------------------------
// Compile-time interface assertions
// -----------------------------------------------------------------------------

var _ hal.Queue = (*Queue)(nil)
