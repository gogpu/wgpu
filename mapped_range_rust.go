//go:build rust

package wgpu

import rwgpu "github.com/go-webgpu/webgpu/wgpu"

// MappedRange is a safe view over a region of a mapped GPU buffer.
// On Rust backend, this wraps go-webgpu/webgpu MappedRange.
type MappedRange struct {
	r *rwgpu.MappedRange
}

// Bytes returns the mapped data as a read-only byte slice.
func (m *MappedRange) Bytes() []byte {
	if m == nil || m.r == nil {
		return nil
	}
	return m.r.Bytes()
}

// BytesMut returns a mutable byte slice for writing into the mapped region.
// On Rust backend, this returns the same underlying data as Bytes() since
// the mapped range is already writable through the FFI pointer.
func (m *MappedRange) BytesMut() []byte {
	return m.Bytes()
}

// Flush writes the cached data back. On Rust backend, data is written through
// the direct pointer, so this is a no-op.
func (m *MappedRange) Flush() error {
	return nil
}

// Len returns the size of the mapped range in bytes.
func (m *MappedRange) Len() int {
	if m == nil || m.r == nil {
		return 0
	}
	return m.r.Len()
}

// Offset returns the byte offset of the mapped range within its buffer.
func (m *MappedRange) Offset() uint64 {
	if m == nil || m.r == nil {
		return 0
	}
	return m.r.Offset()
}

// Release invalidates the MappedRange.
func (m *MappedRange) Release() {
	if m == nil {
		return
	}
	m.r = nil
}
