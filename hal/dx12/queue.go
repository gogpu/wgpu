// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package dx12

import (
	"fmt"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
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
			cmdLists[i] = dx12CB.raw
		}

		// Execute command lists
		q.raw.ExecuteCommandLists(uint32(len(cmdLists)), &cmdLists[0])
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

// WriteBuffer writes data to a buffer immediately.
// This is a convenience method that creates a staging buffer internally.
func (q *Queue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) {
	if buffer == nil || len(data) == 0 {
		return
	}

	// TODO: Implement in TASK-DX12-006 (Resource Creation)
	// For now this is a no-op stub. Full implementation requires:
	// 1. Create upload heap staging buffer
	// 2. Map staging buffer and copy data
	// 3. Create command list with CopyBufferRegion
	// 4. Execute and wait for completion
	// 5. Release staging buffer
}

// WriteTexture writes data to a texture immediately.
// This is a convenience method that creates a staging buffer internally.
func (q *Queue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) {
	if dst == nil || dst.Texture == nil || len(data) == 0 || size == nil {
		return
	}

	// TODO: Implement in TASK-DX12-006 (Resource Creation)
	// For now this is a no-op stub. Full implementation requires:
	// 1. Create upload heap staging buffer with proper row pitch alignment
	// 2. Map staging buffer and copy data (handling row pitch)
	// 3. Create command list with CopyTextureRegion
	// 4. Execute and wait for completion
	// 5. Release staging buffer
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

	// TODO: Implement in TASK-DX12-005 (Swap Chain)
	// Full implementation requires:
	// 1. Get current back buffer index
	// 2. Insert resource barriers (render target -> present)
	// 3. Present via swapchain
	// 4. Handle tearing flags based on present mode

	return fmt.Errorf("dx12: Present not yet implemented")
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
// CommandBuffer placeholder (will be fully implemented in TASK-DX12-008)
// -----------------------------------------------------------------------------

// CommandBuffer implements hal.CommandBuffer for DirectX 12.
type CommandBuffer struct {
	raw       *d3d12.ID3D12GraphicsCommandList
	allocator *d3d12.ID3D12CommandAllocator
	device    *Device
}

// Destroy releases the command buffer resources.
func (cb *CommandBuffer) Destroy() {
	if cb.raw != nil {
		cb.raw.Release()
		cb.raw = nil
	}
	if cb.allocator != nil {
		cb.allocator.Release()
		cb.allocator = nil
	}
	cb.device = nil
}

// -----------------------------------------------------------------------------
// Compile-time interface assertions
// -----------------------------------------------------------------------------

var (
	_ hal.Queue         = (*Queue)(nil)
	_ hal.CommandBuffer = (*CommandBuffer)(nil)
)
