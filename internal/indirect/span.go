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
