//go:build rust

package wgpu

import (
	"fmt"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

// Queue handles command submission and data transfers.
// On Rust backend, this wraps go-webgpu/webgpu Queue.
type Queue struct {
	r        *rwgpu.Queue
	released bool
}

// Submit submits command buffers for execution.
// Returns a submission index that can be used to track completion.
func (q *Queue) Submit(commandBuffers ...*CommandBuffer) (uint64, error) {
	if q.released {
		return 0, ErrReleased
	}

	rBuffers := make([]*rwgpu.CommandBuffer, 0, len(commandBuffers))
	for _, cb := range commandBuffers {
		if cb == nil {
			continue
		}
		// Always mark non-nil command buffers as submitted to prevent reuse,
		// even if the underlying rust buffer is nil (e.g., discarded encoding).
		cb.submitted = true
		if cb.r != nil {
			rBuffers = append(rBuffers, cb.r)
		}
	}

	idx, err := q.r.Submit(rBuffers...)
	if err != nil {
		return 0, fmt.Errorf("wgpu: submit failed: %w", err)
	}

	return idx, nil
}

// Poll returns the last completed submission index. Non-blocking.
// On Rust backend, returns 0 (wgpu-native does not expose poll on queue).
func (q *Queue) Poll() uint64 {
	return 0
}

// WriteBuffer writes data to a buffer.
func (q *Queue) WriteBuffer(buffer *Buffer, offset uint64, data []byte) error {
	if q.released {
		return ErrReleased
	}
	if buffer == nil || buffer.r == nil {
		return fmt.Errorf("wgpu: WriteBuffer: buffer is nil")
	}
	return q.r.WriteBuffer(buffer.r, offset, data)
}

// WriteTexture writes data to a texture.
func (q *Queue) WriteTexture(dst *ImageCopyTexture, data []byte, layout *ImageDataLayout, size *Extent3D) error {
	if q.released {
		return ErrReleased
	}
	if dst == nil || dst.Texture == nil || dst.Texture.r == nil {
		return fmt.Errorf("wgpu: WriteTexture: destination is nil")
	}

	rDst := &rwgpu.ImageCopyTexture{
		Texture:  dst.Texture.r,
		MipLevel: dst.MipLevel,
		Origin:   rwgpu.Origin3D(dst.Origin),
		Aspect:   rwgpu.TextureAspect(dst.Aspect),
	}

	var rLayout *rwgpu.ImageDataLayout
	if layout != nil {
		rLayout = &rwgpu.ImageDataLayout{
			Offset:       layout.Offset,
			BytesPerRow:  layout.BytesPerRow,
			RowsPerImage: layout.RowsPerImage,
		}
	}

	var rSize *rwgpu.Extent3D
	if size != nil {
		rSize = &rwgpu.Extent3D{
			Width:              size.Width,
			Height:             size.Height,
			DepthOrArrayLayers: size.DepthOrArrayLayers,
		}
	}

	return q.r.WriteTexture(rDst, data, rLayout, rSize)
}

// SetSwapchainSuppressed is a no-op on the Rust backend.
func (q *Queue) SetSwapchainSuppressed(_ bool) {}

// LastSubmissionIndex returns the most recent submission index.
// On Rust backend, submission indices are not tracked. Returns 0.
func (q *Queue) LastSubmissionIndex() uint64 {
	return 0
}
