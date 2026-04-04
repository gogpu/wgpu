package wgpu

import (
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

// Buffer represents a GPU buffer.
type Buffer struct {
	core     *core.Buffer
	device   *Device
	released bool
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() uint64 { return b.core.Size() }

// Usage returns the buffer's usage flags.
func (b *Buffer) Usage() BufferUsage { return b.core.Usage() }

// Label returns the buffer's debug label.
func (b *Buffer) Label() string { return b.core.Label() }

// Release destroys the buffer. The underlying HAL buffer is not freed
// immediately — destruction is deferred until the GPU completes any submission
// that may reference it. This prevents use-after-free on DX12/Vulkan when a
// buffer is released while the GPU is still reading from it (BUG-DX12-TDR).
func (b *Buffer) Release() {
	if b.released {
		return
	}
	b.released = true

	if b.device == nil {
		b.core.Destroy()
		return
	}

	dq := b.device.destroyQueue()
	if dq == nil {
		// No DestroyQueue (legacy path or no HAL) — destroy immediately.
		b.core.Destroy()
		return
	}

	// Defer destruction until GPU completes the latest known submission.
	subIdx := b.device.lastSubmissionIndex()
	label := b.core.Label()
	dq.Defer(subIdx, "Buffer:"+label, func() {
		b.core.Destroy()
	})
}

// coreBuffer returns the underlying core.Buffer.
func (b *Buffer) coreBuffer() *core.Buffer { return b.core }

// halBuffer returns the underlying HAL buffer.
func (b *Buffer) halBuffer() hal.Buffer {
	if b.core == nil || b.device == nil {
		return nil
	}
	if !b.core.HasHAL() {
		return nil
	}
	guard := b.device.core.SnatchLock().Read()
	defer guard.Release()
	return b.core.Raw(guard)
}
