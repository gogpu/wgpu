//go:build js && wasm

package queue

import (
	"syscall/js"

	"github.com/gogpu/wgpu/internal/browser"
	"github.com/gogpu/wgpu/internal/resource"
)

// Queue handles command submission and data transfers.
// On browser, this wraps a GPUQueue via internal/browser.Queue.
//
// It is the domain's implementation (ADR-070 S9): the root package keeps the
// public Queue as a declaration layer over it and does the command-buffer
// unwrapping the browser API cannot see.
type Queue struct {
	browser  *browser.Queue
	released bool
}

// New returns the queue over the browser device's GPUQueue.
func New(bq *browser.Queue) *Queue {
	return &Queue{browser: bq}
}

// Submit submits the browser command buffers for execution.
// Returns 0 for the submission index (browser does not track indices).
// Matches Rust wgpu WebQueue::submit which collects into js_sys::Array.
func (q *Queue) Submit(refs []js.Value) (uint64, error) {
	if q == nil || q.released {
		return 0, resource.ErrReleased
	}
	q.browser.Submit(refs)
	return 0, nil
}

// Poll returns the last completed submission index.
// On browser, the GPU is polled automatically. Returns 0.
func (q *Queue) Poll() uint64 {
	return 0
}

// WriteBuffer writes data to a buffer.
// Uses js.CopyBytesToJS for Go-to-JS data transfer (same pattern as Rust's
// Uint8Array::from(data).buffer()).
func (q *Queue) WriteBuffer(buffer *resource.Buffer, offset uint64, data []byte) error {
	if q == nil || q.released {
		return resource.ErrReleased
	}
	if buffer == nil || buffer.Browser() == nil {
		return resource.ErrReleased
	}
	q.browser.WriteBuffer(buffer.Browser().Ref(), offset, data)
	return nil
}

// WriteTexture writes data to a texture.
// Matches Rust wgpu WebQueue::write_texture layout: offset/bytesPerRow/rowsPerImage
// are set on a GPUTexelCopyBufferLayout JS object.
func (q *Queue) WriteTexture(dst *ImageCopyTexture, data []byte, layout *ImageDataLayout, size *Extent3D) error {
	if q == nil || q.released {
		return resource.ErrReleased
	}
	if dst == nil || dst.Texture == nil || dst.Texture.Browser() == nil {
		return resource.ErrReleased
	}

	jsDst := browser.BuildImageCopyTexture(
		dst.Texture.Browser().Ref(),
		dst.MipLevel,
		dst.Origin.X, dst.Origin.Y, dst.Origin.Z,
	)

	var jsLayout js.Value
	if layout != nil {
		jsLayout = browser.BuildTexelCopyBufferLayout(
			layout.Offset,
			layout.BytesPerRow,
			layout.RowsPerImage,
		)
	} else {
		jsLayout = browser.BuildTexelCopyBufferLayout(0, 0, 0)
	}

	var jsSize js.Value
	if size != nil {
		jsSize = browser.BuildExtent3D(size.Width, size.Height, size.DepthOrArrayLayers)
	} else {
		jsSize = browser.BuildExtent3D(0, 0, 0)
	}

	q.browser.WriteTexture(jsDst, data, jsLayout, jsSize)
	return nil
}

// SetSwapchainSuppressed is a no-op on the browser backend.
// WebGPU browser API handles swapchain synchronization internally.
func (q *Queue) SetSwapchainSuppressed(_ bool) {}

// LastSubmissionIndex returns the most recent submission index.
// On browser, submission indices are not tracked. Returns 0.
func (q *Queue) LastSubmissionIndex() uint64 {
	return 0
}
