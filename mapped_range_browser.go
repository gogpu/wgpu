//go:build js && wasm

package wgpu

// MappedRange is a safe view over a region of a mapped GPU buffer.
//
// On the browser backend, MappedRange lazily copies data from the JS
// ArrayBuffer into a Go byte slice on first Bytes() call (read path),
// or provides a staging slice for writes that is flushed back to JS on
// Flush(). This matches the Rust wgpu WebBufferMappedRange pattern:
// actual_mapping (JS Uint8Array) + temporary_mapping (Rust/WASM heap copy).
//
// The MappedRange is invalidated when the owning buffer is unmapped.
type MappedRange struct {
	buf    *Buffer
	offset uint64
	size   uint64
	valid  bool

	// cached holds the Go-side copy of the mapped data. Lazily populated
	// on first Bytes() call (matching Rust temporary_mapping: OnceCell).
	cached []byte

	// dirty tracks whether the cached slice has been modified via
	// BytesMut(). If true, Flush() or Unmap will write data back to JS.
	// Matches Rust temporary_mapping_modified.
	dirty bool
}

// Bytes returns the mapped data as a read-only byte slice.
//
// The data is lazily copied from the JS ArrayBuffer on the first call.
// The returned slice is valid until the owning buffer is unmapped. After
// Unmap, Bytes() returns nil.
func (m *MappedRange) Bytes() []byte {
	if m == nil || !m.valid || m.buf == nil || m.buf.browser == nil {
		return nil
	}
	if m.cached == nil {
		data, err := m.buf.browser.GetMappedRangeBytes(m.offset, m.size)
		if err != nil {
			return nil
		}
		m.cached = data
	}
	return m.cached
}

// BytesMut returns a mutable byte slice for writing into the mapped region.
//
// The returned slice is a Go-heap copy. Changes are NOT visible on the GPU
// until Flush() is called (or the buffer is unmapped, which auto-flushes
// dirty ranges). This matches the Rust wgpu pattern where Drop on
// WebBufferMappedRange writes back if temporary_mapping_modified is true.
func (m *MappedRange) BytesMut() []byte {
	if m == nil || !m.valid || m.buf == nil || m.buf.browser == nil {
		return nil
	}
	if m.cached == nil {
		m.cached = make([]byte, m.size)
	}
	m.dirty = true
	return m.cached
}

// Flush writes the cached data back to the JS ArrayBuffer.
// Only needed after BytesMut(); Bytes()-only usage does not require Flush.
//
// This is the Go equivalent of the Rust WebBufferMappedRange Drop handler
// that copies temporary_mapping back to actual_mapping when modified.
func (m *MappedRange) Flush() error {
	if m == nil || !m.valid || !m.dirty || m.buf == nil || m.buf.browser == nil {
		return nil
	}
	err := m.buf.browser.WriteMappedRange(m.offset, m.cached)
	if err != nil {
		return err
	}
	m.dirty = false
	return nil
}

// Len returns the size of the mapped range in bytes.
func (m *MappedRange) Len() int {
	if m == nil {
		return 0
	}
	return int(m.size) //nolint:gosec // validated at creation
}

// Offset returns the byte offset of the mapped range within its buffer.
func (m *MappedRange) Offset() uint64 {
	if m == nil {
		return 0
	}
	return m.offset
}

// Release invalidates the MappedRange. After Release, Bytes() returns nil.
// If dirty data has not been flushed, Release auto-flushes it to JS.
func (m *MappedRange) Release() {
	if m == nil {
		return
	}
	// Auto-flush dirty data (matches Rust Drop behavior).
	if m.dirty && m.valid && m.buf != nil && m.buf.browser != nil {
		_ = m.buf.browser.WriteMappedRange(m.offset, m.cached)
	}
	m.valid = false
	m.buf = nil
	m.cached = nil
	m.dirty = false
}
