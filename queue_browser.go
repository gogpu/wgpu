//go:build js && wasm

package wgpu

import (
	"syscall/js"

	"github.com/gogpu/wgpu/internal/queue"
	"github.com/gogpu/wgpu/internal/resource"
)

// Queue handles command submission and data transfers.
// On browser, this wraps a GPUQueue via internal/browser.Queue.
//
// The implementation lives in internal/queue (ADR-070 S9); this type is the
// declaration layer the public API is frozen on. The command-buffer
// pre-processing stays here: skipping the buffers the browser API cannot take
// and collecting their refs is a public-handle concern the destination cannot
// see.
type Queue struct {
	impl *queue.Queue
}

// Submit submits command buffers for execution.
// Returns 0 for the submission index (browser does not track indices).
// Matches Rust wgpu WebQueue::submit which collects into js_sys::Array.
func (q *Queue) Submit(commandBuffers ...*CommandBuffer) (uint64, error) {
	jsBuffers := make([]js.Value, 0, len(commandBuffers))
	for _, cb := range commandBuffers {
		if cb == nil || cb.impl == nil || cb.impl.Browser() == nil || cb.impl.Released() {
			continue
		}
		jsBuffers = append(jsBuffers, cb.impl.Browser().Ref())
		cb.impl.MarkSubmitted()
	}
	return q.impl.Submit(jsBuffers)
}

// Poll returns the last completed submission index.
// On browser, the GPU is polled automatically. Returns 0.
func (q *Queue) Poll() uint64 {
	return q.impl.Poll()
}

// WriteBuffer writes data to a buffer.
// Uses js.CopyBytesToJS for Go-to-JS data transfer (same pattern as Rust's
// Uint8Array::from(data).buffer()).
func (q *Queue) WriteBuffer(buffer *Buffer, offset uint64, data []byte) error {
	return q.impl.WriteBuffer(bufferImpl(buffer), offset, data)
}

// WriteTexture writes data to a texture.
// Matches Rust wgpu WebQueue::write_texture layout: offset/bytesPerRow/rowsPerImage
// are set on a GPUTexelCopyBufferLayout JS object.
func (q *Queue) WriteTexture(dst *ImageCopyTexture, data []byte, layout *ImageDataLayout, size *Extent3D) error {
	return q.impl.WriteTexture(queueImageCopyTexture(dst), data, queueImageDataLayout(layout), queueExtent3D(size))
}

// SetSwapchainSuppressed is a no-op on the browser backend.
// WebGPU browser API handles swapchain synchronization internally.
func (q *Queue) SetSwapchainSuppressed(_ bool) {}

// LastSubmissionIndex returns the most recent submission index.
// On browser, submission indices are not tracked. Returns 0.
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
