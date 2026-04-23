//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import (
	"sync"
	"unsafe"
)

// MappedRange is a safe view over a region of a mapped GPU buffer.
//
// MappedRange replaces raw []byte return from the legacy Queue.ReadBuffer
// API. It is invalidated atomically on Buffer.Unmap via a per-buffer
// generation counter — calling Bytes() on a detached range returns nil
// rather than exposing freed memory, mitigating the UAF pattern that
// the WebGPU CVE history has accumulated (see ADR §Safety #4).
//
// Typical usage:
//
//	if err := buf.Map(ctx, wgpu.MapModeRead, 0, size); err != nil {
//	    return err
//	}
//	defer buf.Unmap()
//	rng, err := buf.MappedRange(0, size)
//	if err != nil { return err }
//	data := rng.Bytes() // valid until buf.Unmap()
//
// A single MappedRange is not thread-safe to share between goroutines
// without external synchronization; typical use is a single owner that
// copies data out before Unmap.
//
// MappedRange instances are pooled via sync.Pool for zero-allocation
// steady-state use. Callers that care about allocation counts should
// call Release() after consuming Bytes().
type MappedRange struct {
	buf    *Buffer
	offset uint64         // user-visible offset within the buffer
	size   uint64         // length of this range
	gen    uint64         // buffer generation captured at creation
	data   unsafe.Pointer // pointer to byte 0 of this range
}

var mappedRangePool = sync.Pool{
	New: func() any { return &MappedRange{} },
}

// acquireMappedRange fetches a zeroed MappedRange from the pool.
func acquireMappedRange() *MappedRange {
	return mappedRangePool.Get().(*MappedRange)
}

// Bytes returns the underlying byte slice for the mapped range.
//
// The slice is valid until the owning buffer is unmapped; after Unmap
// the generation counter advances and Bytes() returns nil. Callers
// should NOT cache the returned slice past Unmap.
//
// Bytes() is safe to call concurrently with itself but not with Unmap
// on the same buffer — that is the caller's responsibility to
// coordinate.
func (m *MappedRange) Bytes() []byte {
	if m == nil || m.buf == nil || m.data == nil {
		return nil
	}
	// Generation check detects Unmap-during-read races. The buffer bumps
	// its generation atomically inside Unmap before releasing the HAL
	// mapping, so observing the pre-bump generation means the underlying
	// memory is still valid.
	if m.buf.core.Generation() != m.gen {
		return nil
	}
	// m.size is validated at creation time to lie within the buffer's
	// mapped region, so it cannot exceed MaxInt on any sane allocation.
	return unsafe.Slice((*byte)(m.data), m.size) //nolint:gosec // range validated at MappedRange creation
}

// Len returns the size of the mapped range in bytes.
func (m *MappedRange) Len() int {
	if m == nil {
		return 0
	}
	return int(m.size) //nolint:gosec // range validated at MappedRange creation
}

// Offset returns the byte offset of the mapped range within its buffer.
func (m *MappedRange) Offset() uint64 {
	if m == nil {
		return 0
	}
	return m.offset
}

// Release returns the MappedRange to the pool. Calling this is optional —
// the pool simply fills back up on the next GC cycle — but keeping
// MappedRange calls zero-alloc in the primary Map path requires the
// caller to Release after the bytes have been consumed.
//
// After Release the MappedRange is invalid; calling Bytes() returns nil.
func (m *MappedRange) Release() {
	if m == nil {
		return
	}
	m.buf = nil
	m.data = nil
	m.offset = 0
	m.size = 0
	m.gen = 0
	mappedRangePool.Put(m)
}
