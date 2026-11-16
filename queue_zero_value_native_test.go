//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu/internal/hal/noop"
)

// TestQueueWriteTextureZeroValueTextureKeepsErrReleased pins the pre-move
// answer for a public Texture with no implementation handle (a Go zero value):
// the native path reached the released check through the wrapper's handle and
// reported ErrReleased, and the boundary conversion must not turn it into the
// destination's nil-texture error. The wasm and rust contexts report the same
// strings before and after the move and need no boundary guard.
func TestQueueWriteTextureZeroValueTextureKeepsErrReleased(t *testing.T) {
	q := newQueue(&noop.Queue{}, nil)

	var tex Texture
	dst := &ImageCopyTexture{Texture: &tex}

	err := q.WriteTexture(dst, nil, nil, nil)
	if !errors.Is(err, ErrReleased) {
		t.Fatalf("WriteTexture with a zero-value Texture = %v, want ErrReleased", err)
	}
}

// TestQueueWriteTextureZeroValueQueueKeepsQueueError pins the order of the
// pre-move guards: without a HAL queue the queue/destination error wins over
// the released-texture sentinel, exactly as before the move.
func TestQueueWriteTextureZeroValueQueueKeepsQueueError(t *testing.T) {
	var q Queue
	var tex Texture

	err := q.WriteTexture(&ImageCopyTexture{Texture: &tex}, nil, nil, nil)
	if err == nil || err.Error() != "wgpu: WriteTexture: queue or destination is nil" {
		t.Fatalf("WriteTexture on a zero-value queue = %v, want \"wgpu: WriteTexture: queue or destination is nil\"", err)
	}
}
