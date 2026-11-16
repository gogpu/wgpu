//go:build !rust && !(js && wasm)

package queue

import (
	"testing"

	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/resource"
)

// TestPostSubmitDropsSetsWithoutDestroyQueue verifies that a command buffer is
// marked submitted and has its validation sets dropped even when the device has
// no DestroyQueue. postSubmit returns early in that case; the bookkeeping must
// happen before the early return, since the HAL submit has already succeeded.
//
// It moved here with the queue (ADR-070 S9): postSubmit is the queue's own
// path, so the assertion lives with it.
func TestPostSubmitDropsSetsWithoutDestroyQueue(t *testing.T) {
	q := New(nil, nil) // nil device — destroyQueue() returns nil
	if q.destroyQueue() != nil {
		t.Fatal("test precondition: expected nil destroy queue")
	}

	cb := &encoder.CommandBuffer{}
	cb.SetUsedSetsForTest(
		map[*resource.Buffer]struct{}{{}: {}},
		map[*resource.Texture]struct{}{{}: {}},
		map[*resource.BindGroup]struct{}{{}: {}},
	)

	q.postSubmit(1, []*encoder.CommandBuffer{cb})

	if !cb.IsSubmitted() {
		t.Error("postSubmit left cb.submitted false — a double submit would not be caught")
	}
	if cb.UsedBuffers() != nil || cb.UsedTextures() != nil || cb.UsedBindGroups() != nil {
		t.Errorf("postSubmit left validation sets populated: buffers=%v textures=%v bindGroups=%v",
			cb.UsedBuffers(), cb.UsedTextures(), cb.UsedBindGroups())
	}
}
