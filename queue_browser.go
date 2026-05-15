//go:build js && wasm

package wgpu

import (
	"github.com/gogpu/wgpu/internal/browser"
)

// Queue handles command submission and data transfers.
// On browser, this wraps a GPUQueue via internal/browser.Queue.
type Queue struct {
	browser  *browser.Queue
	released bool
}

// Submit submits command buffers for execution.
// Phase 3 — not yet implemented for browser.
func (q *Queue) Submit(commandBuffers ...*CommandBuffer) (uint64, error) {
	panic("wgpu: browser Queue.Submit not yet implemented (Phase 3)")
}

// Poll returns the last completed submission index.
// On browser, the GPU is polled automatically. Returns 0.
func (q *Queue) Poll() uint64 {
	return 0
}

// WriteBuffer writes data to a buffer.
// Phase 3 — not yet implemented for browser.
func (q *Queue) WriteBuffer(buffer *Buffer, offset uint64, data []byte) error {
	panic("wgpu: browser Queue.WriteBuffer not yet implemented (Phase 3)")
}

// WriteTexture writes data to a texture.
// Phase 3 — not yet implemented for browser.
func (q *Queue) WriteTexture(dst *ImageCopyTexture, data []byte, layout *ImageDataLayout, size *Extent3D) error {
	panic("wgpu: browser Queue.WriteTexture not yet implemented (Phase 3)")
}

// SetSwapchainSuppressed is a no-op on the browser backend.
// WebGPU browser API handles swapchain synchronization internally.
func (q *Queue) SetSwapchainSuppressed(_ bool) {}

// LastSubmissionIndex returns the most recent submission index.
// On browser, submission indices are not tracked. Returns 0.
func (q *Queue) LastSubmissionIndex() uint64 {
	return 0
}
