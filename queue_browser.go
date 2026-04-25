//go:build js && wasm

package wgpu

// Queue handles command submission and data transfers.
type Queue struct {
	released bool
}

// Submit submits command buffers for execution.
func (q *Queue) Submit(commandBuffers ...*CommandBuffer) (uint64, error) {
	panic("wgpu: browser backend not yet implemented")
}

// Poll returns the last completed submission index.
func (q *Queue) Poll() uint64 {
	panic("wgpu: browser backend not yet implemented")
}

// WriteBuffer writes data to a buffer.
func (q *Queue) WriteBuffer(buffer *Buffer, offset uint64, data []byte) error {
	panic("wgpu: browser backend not yet implemented")
}

// WriteTexture writes data to a texture.
func (q *Queue) WriteTexture(dst *ImageCopyTexture, data []byte, layout *ImageDataLayout, size *Extent3D) error {
	panic("wgpu: browser backend not yet implemented")
}

// SetSwapchainSuppressed is a no-op on the browser backend.
// WebGPU browser API handles swapchain synchronization internally.
func (q *Queue) SetSwapchainSuppressed(_ bool) {}

// LastSubmissionIndex returns the most recent submission index.
func (q *Queue) LastSubmissionIndex() uint64 {
	return 0
}
