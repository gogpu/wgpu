//go:build js && wasm

package wgpu_test

import (
	"testing"

	"github.com/gogpu/wgpu"
)

// TestSurfaceNilReceiverKeepsPreMoveAnswers pins the browser declaration
// layer's nil-receiver answers: the three methods that never dereferenced the
// receiver before the move still answer without a surface. The rust layer
// carries the same guards; it has no runtime to execute them here.
func TestSurfaceNilReceiverKeepsPreMoveAnswers(t *testing.T) {
	var s *wgpu.Surface

	if err := s.PresentPixels(nil, 0, 0, nil); err == nil || err.Error() != "wgpu: PresentPixels is not supported on the browser backend" {
		t.Fatalf("PresentPixels on a nil surface = %v, want the unsupported answer", err)
	}
	if err := s.WritePixels(nil, 0, 0); err == nil || err.Error() != "wgpu: WritePixels is not supported on the browser backend" {
		t.Fatalf("WritePixels on a nil surface = %v, want the unsupported answer", err)
	}
	s.SetPresentsWithTransaction(true)
}
