package core

import (
	"fmt"

	"github.com/gogpu/wgpu/types"
)

// GetQueue retrieves queue data.
// Returns an error if the queue ID is invalid.
func GetQueue(id QueueID) (*Queue, error) {
	hub := GetGlobal().Hub()
	queue, err := hub.GetQueue(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue: %w", err)
	}
	return &queue, nil
}

// QueueSubmit submits command buffers to the queue.
// This is a placeholder implementation that will be expanded later.
//
// The command buffers are executed in order. After submission,
// the command buffer IDs become invalid and cannot be reused.
//
// Returns an error if the queue ID is invalid or if submission fails.
func QueueSubmit(id QueueID, commandBuffers []CommandBufferID) error {
	hub := GetGlobal().Hub()

	// Verify the queue exists
	_, err := hub.GetQueue(id)
	if err != nil {
		return fmt.Errorf("invalid queue: %w", err)
	}

	// TODO: Validate command buffers
	// TODO: Submit command buffers to GPU
	// TODO: Mark command buffers as consumed

	// Placeholder: verify all command buffers exist
	for _, cmdBufID := range commandBuffers {
		_, err := hub.GetCommandBuffer(cmdBufID)
		if err != nil {
			return fmt.Errorf("invalid command buffer: %w", err)
		}
	}

	return nil
}

// QueueWriteBuffer writes data to a buffer through the queue.
// This is a placeholder implementation that will be expanded later.
//
// This is a convenience method for updating buffer data without
// creating a staging buffer. The data is written at the specified
// offset in the buffer.
//
// Returns an error if the queue ID or buffer ID is invalid,
// or if the write operation fails.
func QueueWriteBuffer(id QueueID, buffer BufferID, offset uint64, data []byte) error {
	hub := GetGlobal().Hub()

	// Verify the queue exists
	_, err := hub.GetQueue(id)
	if err != nil {
		return fmt.Errorf("invalid queue: %w", err)
	}

	// Verify the buffer exists
	_, err = hub.GetBuffer(buffer)
	if err != nil {
		return fmt.Errorf("invalid buffer: %w", err)
	}

	// TODO: Validate offset and data size
	// TODO: Write data to buffer

	return nil
}

// QueueWriteTexture writes data to a texture through the queue.
// This is a placeholder implementation that will be expanded later.
//
// This is a convenience method for updating texture data without
// creating a staging buffer. The data is written to the specified
// texture region.
//
// Returns an error if the queue ID or texture ID is invalid,
// or if the write operation fails.
func QueueWriteTexture(id QueueID, dst *types.ImageCopyTexture, data []byte, layout *types.TextureDataLayout, size *types.Extent3D) error {
	hub := GetGlobal().Hub()

	// Verify the queue exists
	_, err := hub.GetQueue(id)
	if err != nil {
		return fmt.Errorf("invalid queue: %w", err)
	}

	if dst == nil {
		return fmt.Errorf("destination texture is required")
	}

	if layout == nil {
		return fmt.Errorf("texture data layout is required")
	}

	if size == nil {
		return fmt.Errorf("texture size is required")
	}

	// TODO: Validate texture and parameters
	// TODO: Write data to texture

	return nil
}

// QueueOnSubmittedWorkDone returns when all submitted work completes.
// In the Pure Go implementation, this is synchronous for now.
//
// This function blocks until all work submitted to the queue before
// this call has completed execution on the GPU.
//
// Returns an error if the queue ID is invalid.
func QueueOnSubmittedWorkDone(id QueueID) error {
	hub := GetGlobal().Hub()

	// Verify the queue exists
	_, err := hub.GetQueue(id)
	if err != nil {
		return fmt.Errorf("invalid queue: %w", err)
	}

	// TODO: Implement actual synchronization
	// For now, this is a no-op since we don't have GPU execution yet

	return nil
}
