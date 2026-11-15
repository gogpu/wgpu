//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu"
)

// TestBufferZeroValueIsSafe pins the ADR-070 §15.5 zero-value contract: a
// zero-value Buffer behaves as the pre-move type did — Release is a no-op and
// the guarded methods report the released/unmapped zero values instead of
// panicking on a nil implementation pointer.
func TestBufferZeroValueIsSafe(t *testing.T) {
	var b wgpu.Buffer
	b.Release() // must not panic
	if got := b.MapState(); got != wgpu.MapStateUnmapped {
		t.Fatalf("zero-value MapState = %v, want MapStateUnmapped", got)
	}
	if err := b.Map(t.Context(), wgpu.MapModeRead, 0, 4); !errors.Is(err, wgpu.ErrReleased) {
		t.Fatalf("zero-value Map error = %v, want ErrReleased", err)
	}
	if _, err := b.MapAsync(wgpu.MapModeRead, 0, 4); !errors.Is(err, wgpu.ErrReleased) {
		t.Fatalf("zero-value MapAsync error = %v, want ErrReleased", err)
	}
	if _, err := b.MappedRange(0, 4); !errors.Is(err, wgpu.ErrReleased) {
		t.Fatalf("zero-value MappedRange error = %v, want ErrReleased", err)
	}
	if err := b.Unmap(); !errors.Is(err, wgpu.ErrReleased) {
		t.Fatalf("zero-value Unmap error = %v, want ErrReleased", err)
	}
}
