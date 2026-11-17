//go:build rust

package wgpu

import (
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/queue"
	"github.com/gogpu/wgpu/internal/resource"
)

// Queue handles command submission and data transfers.
// On Rust backend, this wraps go-webgpu/webgpu Queue.
//
// The implementation lives in internal/queue (ADR-070 S9); this type is the
// declaration layer the public API is frozen on. The command-buffer
// unwrapping stays here: a public handle is a concern the destination cannot
// see.
type Queue struct {
	impl *queue.Queue
}

// Submit submits command buffers for execution.
// Returns a submission index that can be used to track completion.
func (q *Queue) Submit(commandBuffers ...*CommandBuffer) (uint64, error) {
	impls := make([]*encoder.CommandBuffer, len(commandBuffers))
	for i, cb := range commandBuffers {
		if cb != nil {
			impls[i] = cb.impl
		}
	}
	return q.impl.Submit(impls)
}

// Poll returns the last completed submission index. Non-blocking.
// On Rust backend, returns 0 (wgpu-native does not expose poll on queue).
func (q *Queue) Poll() uint64 {
	return q.impl.Poll()
}

// WriteBuffer writes data to a buffer.
func (q *Queue) WriteBuffer(buffer *Buffer, offset uint64, data []byte) error {
	return q.impl.WriteBuffer(bufferImpl(buffer), offset, data)
}

// WriteTexture writes data to a texture.
func (q *Queue) WriteTexture(dst *ImageCopyTexture, data []byte, layout *ImageDataLayout, size *Extent3D) error {
	return q.impl.WriteTexture(queueImageCopyTexture(dst), data, queueImageDataLayout(layout), queueExtent3D(size))
}

// SetSwapchainSuppressed is a no-op on the Rust backend.
func (q *Queue) SetSwapchainSuppressed(_ bool) {}

// LastSubmissionIndex returns the most recent submission index.
// On Rust backend, submission indices are not tracked. Returns 0.
func (q *Queue) LastSubmissionIndex() uint64 {
	return q.impl.LastSubmissionIndex()
}

// bufferImpl returns the implementation handle of b, or nil for an absent
// buffer: the destination's own nil check reports it, exactly as before the
// move.
func bufferImpl(b *Buffer) *resource.Buffer {
	if b == nil {
		return nil
	}
	return b.impl
}

// queueImageCopyTexture converts a public upload descriptor into the
// implementation's. A nil dst or a nil texture becomes a nil field, so the
// destination's own nil checks keep their exact errors.
func queueImageCopyTexture(dst *ImageCopyTexture) *queue.ImageCopyTexture {
	if dst == nil {
		return nil
	}
	return &queue.ImageCopyTexture{
		Texture:  textureImpl(dst.Texture),
		MipLevel: dst.MipLevel,
		Origin:   queue.Origin3D(dst.Origin),
		Aspect:   dst.Aspect,
	}
}

// textureImpl returns the implementation handle of t, or nil for an
// absent texture.
func textureImpl(t *Texture) *resource.Texture {
	if t == nil {
		return nil
	}
	return t.impl
}

// queueImageDataLayout converts a public layout into the implementation's, or
// nil for an absent layout.
func queueImageDataLayout(l *ImageDataLayout) *queue.ImageDataLayout {
	if l == nil {
		return nil
	}
	return &queue.ImageDataLayout{
		Offset:       l.Offset,
		BytesPerRow:  l.BytesPerRow,
		RowsPerImage: l.RowsPerImage,
	}
}

// queueExtent3D converts a public extent into the implementation's, or nil for
// an absent extent.
func queueExtent3D(e *Extent3D) *queue.Extent3D {
	if e == nil {
		return nil
	}
	return &queue.Extent3D{
		Width:              e.Width,
		Height:             e.Height,
		DepthOrArrayLayers: e.DepthOrArrayLayers,
	}
}
