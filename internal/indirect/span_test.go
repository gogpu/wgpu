// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package indirect

import (
	"math"
	"testing"
)

func TestRangeFits(t *testing.T) {
	tests := []struct {
		name                           string
		bufferSize, offset, recordSize uint64
		count                          uint32
		want                           bool
	}{
		{name: "empty range", bufferSize: 16, offset: 16, recordSize: 16, want: true},
		{name: "exact range", bufferSize: 48, offset: 16, recordSize: 16, count: 2, want: true},
		{name: "range too large", bufferSize: 47, offset: 16, recordSize: 16, count: 2},
		{name: "offset past buffer", bufferSize: 16, offset: 17, recordSize: 16},
		{name: "zero record size", bufferSize: 16, recordSize: 0, count: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := RangeFits(test.bufferSize, test.offset, test.recordSize, test.count); got != test.want {
				t.Fatalf("RangeFits(%d, %d, %d, %d) = %v, want %v", test.bufferSize, test.offset, test.recordSize, test.count, got, test.want)
			}
		})
	}
}

func TestRecordOffset(t *testing.T) {
	if got, ok := RecordOffset(4, 20, 3); !ok || got != 64 {
		t.Fatalf("RecordOffset(4, 20, 3) = (%d, %v), want (64, true)", got, ok)
	}
	if got, ok := RecordOffset(math.MaxUint64-3, 20, 1); ok || got != 0 {
		t.Fatalf("overflowing RecordOffset = (%d, %v), want (0, false)", got, ok)
	}
	if got, ok := RecordOffset(0, math.MaxUint64, 2); ok || got != 0 {
		t.Fatalf("multiplication-overflow RecordOffset = (%d, %v), want (0, false)", got, ok)
	}
}
