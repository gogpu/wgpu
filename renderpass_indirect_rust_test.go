//go:build rust

package wgpu

import (
	"slices"
	"testing"
)

func TestLowerRustIndexedIndirectInvalidSpanDelegatesOneFailingCall(t *testing.T) {
	tests := []struct {
		name       string
		bufferSize uint64
		offset     uint64
		drawCount  uint32
		want       []uint64
	}{
		{name: "later record fails", bufferSize: 59, offset: 0, drawCount: 3, want: []uint64{40}},
		{name: "count one preserves offset", bufferSize: 20, offset: 24, drawCount: 1, want: []uint64{24}},
		{name: "arithmetic overflow", bufferSize: 64, offset: ^uint64(0) - 3, drawCount: 2, want: []uint64{64}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var offsets []uint64
			lowerRustIndexedIndirect(test.bufferSize, test.offset, test.drawCount, func(offset uint64) {
				offsets = append(offsets, offset)
			})

			if !slices.Equal(offsets, test.want) {
				t.Fatalf("underlying offsets = %v, want %v", offsets, test.want)
			}
		})
	}
}

func TestLowerRustIndexedIndirectValidSpanEmitsEveryRecordInOrder(t *testing.T) {
	var offsets []uint64
	lowerRustIndexedIndirect(64, 4, 3, func(offset uint64) {
		offsets = append(offsets, offset)
	})

	if want := []uint64{4, 24, 44}; !slices.Equal(offsets, want) {
		t.Fatalf("underlying offsets = %v, want %v", offsets, want)
	}
}
