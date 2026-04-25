//go:build !(js && wasm)

package noop

import (
	"fmt"
	"image"

	"github.com/gogpu/wgpu/hal"
)

// Queue implements hal.Queue for the noop backend.
type Queue struct {
	submissionIndex uint64
}

// Submit simulates command buffer submission.
// Returns a monotonically increasing submission index.
func (q *Queue) Submit(_ []hal.CommandBuffer) (uint64, error) {
	q.submissionIndex++
	return q.submissionIndex, nil
}

// PollCompleted returns the highest submission index known to be completed.
// Noop backend is synchronous — all submissions are immediately complete.
func (q *Queue) PollCompleted() uint64 {
	return q.submissionIndex
}

// WriteBuffer simulates immediate buffer writes.
// If the buffer has storage, copies data to it.
func (q *Queue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) error {
	switch b := buffer.(type) {
	case *Buffer:
		if b.data != nil {
			copy(b.data[offset:], data)
		}
		return nil
	case *Resource:
		// Non-mapped buffer — no data to write, just a no-op.
		_ = b
		return nil
	default:
		return fmt.Errorf("noop: WriteBuffer: invalid buffer type %T", buffer)
	}
}

// WriteTexture simulates immediate texture writes.
// This is a no-op since textures don't store data.
func (q *Queue) WriteTexture(_ *hal.ImageCopyTexture, _ []byte, _ *hal.ImageDataLayout, _ *hal.Extent3D) error {
	return nil
}

// Present simulates surface presentation.
// Always succeeds. damageRects is accepted and ignored (noop backend).
func (q *Queue) Present(_ hal.Surface, _ hal.SurfaceTexture, _ []image.Rectangle) error {
	return nil
}

// GetTimestampPeriod returns 1.0 nanosecond timestamp period.
func (q *Queue) GetTimestampPeriod() float32 {
	return 1.0
}

// SupportsCommandBufferCopies returns false for the noop backend.
// Writes are handled directly without command buffer batching.
func (q *Queue) SupportsCommandBufferCopies() bool {
	return false
}

// SetSwapchainSuppressed is a no-op on the noop backend.
// See BUG-WGPU-VK-005 (Vulkan-specific).
func (q *Queue) SetSwapchainSuppressed(_ bool) {}
