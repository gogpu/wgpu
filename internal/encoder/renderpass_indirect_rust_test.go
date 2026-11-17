//go:build rust

package encoder

import (
	"errors"
	"slices"
	"testing"

	"github.com/gogpu/wgpu/internal/core"
)

// The rust lowering's white-box test moved here with the function (S5's Move).

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

// TestRustRenderPassEndAfterRelease pins the released flag of the rust pass
// (ADR-070 S5's zero-value contract table): once End has released the pass, a
// second End returns ErrReleased without touching the go-webgpu pass. The
// first End needs a real wgpu-native pass, which a hermetic test cannot
// construct; its success path ends at the adapter call.
func TestRustRenderPassEndAfterRelease(t *testing.T) {
	pass := &RenderPassEncoder{released: true}
	if err := pass.End(); !errors.Is(err, core.ErrReleased) {
		t.Fatalf("End() on a released pass: error = %v, want ErrReleased", err)
	}
}
