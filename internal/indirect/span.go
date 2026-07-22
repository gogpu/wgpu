// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package indirect provides overflow-safe arithmetic for indirect draw spans.
package indirect

// RangeFits reports whether count consecutive records fit in a buffer.
func RangeFits(bufferSize, offset, recordSize uint64, count uint32) bool {
	if recordSize == 0 || offset > bufferSize {
		return false
	}
	return uint64(count) <= (bufferSize-offset)/recordSize
}

// RecordOffset returns the byte offset of a record without uint64 wraparound.
func RecordOffset(offset, recordSize uint64, index uint32) (uint64, bool) {
	if index != 0 && recordSize > ^uint64(0)/uint64(index) {
		return 0, false
	}
	delta := uint64(index) * recordSize
	if offset > ^uint64(0)-delta {
		return 0, false
	}
	return offset + delta, true
}

// DelegatedValidationOffset returns the last record offset for a span, or the
// buffer size when offset arithmetic overflows. Backends with single-record
// validation can use it to delegate one deterministic failing operation.
func DelegatedValidationOffset(bufferSize, offset, recordSize uint64, count uint32) uint64 {
	if count == 0 {
		return offset
	}
	lastOffset, ok := RecordOffset(offset, recordSize, count-1)
	if !ok {
		return bufferSize
	}
	return lastOffset
}
