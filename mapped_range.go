// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import "github.com/gogpu/wgpu/internal/resource"

// MappedRange is a safe view over a region of a mapped GPU buffer. The
// implementation is pooled in internal/resource; this declaration is the
// public surface and keeps the invalidation contract documented there.
type MappedRange struct {
	impl *resource.MappedRange
}

// Bytes returns the underlying byte slice for the mapped range.
func (m *MappedRange) Bytes() []byte {
	if m == nil {
		return nil
	}
	return m.impl.Bytes()
}

// BytesMut returns a mutable byte slice for writing into the mapped region.
func (m *MappedRange) BytesMut() []byte {
	if m == nil {
		return nil
	}
	return m.impl.BytesMut()
}

// Flush writes the cached data back to the GPU.
func (m *MappedRange) Flush() error {
	if m == nil {
		return nil
	}
	return m.impl.Flush()
}

// Len returns the size of the mapped range in bytes.
func (m *MappedRange) Len() int {
	if m == nil {
		return 0
	}
	return m.impl.Len()
}

// Offset returns the byte offset of the mapped range within its buffer.
func (m *MappedRange) Offset() uint64 {
	if m == nil {
		return 0
	}
	return m.impl.Offset()
}

// Release returns the MappedRange to the implementation's pool.
func (m *MappedRange) Release() {
	if m == nil {
		return
	}
	m.impl.Release()
}
