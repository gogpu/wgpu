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

// Release destroys the buffer.
func (b *Buffer) Release() {
	if b.released {
		return
	}
	b.released = true
	b.core.Destroy()
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
