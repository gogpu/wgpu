//go:build rust

package queue

import (
	"fmt"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"

	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/resource"
)

// Queue handles command submission and data transfers.
// On Rust backend, this wraps go-webgpu/webgpu Queue.
//
// It is the domain's implementation (ADR-070 S9): the root package keeps the
// public Queue as a declaration layer over it and unwraps the public command
// buffer handles.
type Queue struct {
	r        *rwgpu.Queue
	released bool
}

// New returns the queue over the go-webgpu/webgpu queue.
func New(rq *rwgpu.Queue) *Queue {
	return &Queue{r: rq}
}

// Submit submits command buffers for execution.
// Returns a submission index that can be used to track completion.
func (q *Queue) Submit(commandBuffers []*encoder.CommandBuffer) (uint64, error) {
	if q == nil || q.released {
		return 0, core.ErrReleased
	}

	rBuffers := make([]*rwgpu.CommandBuffer, 0, len(commandBuffers))
	for _, cb := range commandBuffers {
		if cb == nil {
			continue
		}
		// Always mark non-nil command buffers as submitted to prevent reuse,
		// even if the underlying rust buffer is nil (e.g., discarded encoding).
		cb.MarkSubmitted()
		if r := cb.R(); r != nil {
			rBuffers = append(rBuffers, r)
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
func (q *Queue) WriteBuffer(buffer *resource.Buffer, offset uint64, data []byte) error {
	if q == nil || q.released {
		return core.ErrReleased
	}
	if buffer == nil || buffer.R() == nil {
		return fmt.Errorf("wgpu: WriteBuffer: buffer is nil")
	}
	return q.r.WriteBuffer(buffer.R(), offset, data)
}

// WriteTexture writes data to a texture.
func (q *Queue) WriteTexture(dst *ImageCopyTexture, data []byte, layout *ImageDataLayout, size *Extent3D) error {
	if q == nil || q.released {
		return core.ErrReleased
	}
	if dst == nil || dst.Texture == nil || dst.Texture.Rust() == nil {
		return fmt.Errorf("wgpu: WriteTexture: destination is nil")
	}

	rDst := &rwgpu.ImageCopyTexture{
		Texture:  dst.Texture.Rust(),
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

// R returns the queue's go-webgpu/webgpu queue. The root package reads it for
// the device's HAL handles.
func (q *Queue) R() *rwgpu.Queue {
	if q == nil {
		return nil
	}
	return q.r
}
