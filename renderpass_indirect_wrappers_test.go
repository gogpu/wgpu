//go:build rust || (js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/wgpu/internal/indirect"
)

func TestIndexedIndirectDelegatedValidationOffset(t *testing.T) {
	tests := []struct {
		name       string
		bufferSize uint64
		offset     uint64
		drawCount  uint32
		want       uint64
	}{
		{
			name:       "count one preserves caller offset",
			bufferSize: 20,
			offset:     24,
			drawCount:  1,
			want:       24,
		},
		{
			name:       "later record delegates only failing offset",
			bufferSize: 59,
			offset:     0,
			drawCount:  3,
			want:       40,
		},
		{
			name:       "arithmetic overflow uses end of buffer",
			bufferSize: 64,
			offset:     ^uint64(0) - 3,
			drawCount:  2,
			want:       64,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := indirect.DelegatedValidationOffset(test.bufferSize, test.offset, drawIndexedIndirectRecordSize, test.drawCount); got != test.want {
				t.Fatalf("indexedIndirectDelegatedValidationOffset(%d, %d, %d) = %d, want %d", test.bufferSize, test.offset, test.drawCount, got, test.want)
			}
		})
	}
}
